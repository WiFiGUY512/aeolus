// Package maps is what a folder's map is drawn from (0169): the map's
// tiles, and the places an address may be, both asked of OpenStreetMap by
// the manager, so that the page asks nothing of anyone else. Tiles are kept
// a while, so that a map looked at again is not asked for again, and an
// address is asked about at most once a second, as Nominatim asks.
package maps

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UserAgent is what the manager calls itself to OpenStreetMap, whose
// servers ask that each application say who it is.
const UserAgent = "Aeolus/1 (OpenWrt Wi-Fi manager; folder maps)"

// MaxZoom is the closest a map is drawn.
const MaxZoom = 19

// What tiles are kept, and for how long; how many may be fetched at once,
// and how fast. OpenStreetMap's tile servers are given to light use: a
// person looking at maps, not a copy of them taken.
const (
	keepBytes = 32 << 20
	keepFor   = 7 * 24 * time.Hour
	tileMax   = 1 << 20
	fetchers  = 2                // tiles fetched at once
	burst     = 120              // tiles fetched before the rate holds
	refill    = 4                // tiles a second, after
	busyFor   = 15 * time.Second // how long a caller waits for a turn
)

// Tiles fetches map tiles, and keeps the latest fetched up to keepBytes.
// A tile asked for while it is fetched is waited for, not fetched again;
// one the server cannot give is given as it was last kept, however old.
type Tiles struct {
	HTTP *http.Client
	Base string // as https://tile.openstreetmap.org, to which /z/x/y.png is added

	mu       sync.Mutex
	kept     map[string]*list.Element
	order    *list.List // the most recently used first
	bytes    int
	fetching map[string]*fetch
	tokens   float64
	filled   time.Time
	turns    chan struct{}
}

type tile struct {
	key  string
	data []byte
	at   time.Time
}

type fetch struct {
	done chan struct{}
	data []byte
	err  error
}

// NewTiles is a Tiles fetching from base.
func NewTiles(base string) *Tiles {
	return &Tiles{HTTP: &http.Client{Timeout: 15 * time.Second}, Base: base, kept: map[string]*list.Element{}, order: list.New(),
		fetching: map[string]*fetch{}, tokens: burst, filled: time.Now(), turns: make(chan struct{}, fetchers)}
}

// ErrNoTile is a tile that is not on any map: past MaxZoom, or off its edge.
var ErrNoTile = errors.New("no such tile")

// ErrBusy is more tiles asked for than OpenStreetMap is asked for at once.
var ErrBusy = errors.New("the map asks for tiles faster than the manager fetches them: they come in a moment")

// Valid says whether z/x/y is a tile on the map.
func Valid(z, x, y int) bool {
	return z >= 0 && z <= MaxZoom && x >= 0 && y >= 0 && x < 1<<z && y < 1<<z
}

// Get is the tile z/x/y, a PNG: kept, or fetched.
func (t *Tiles) Get(ctx context.Context, z, x, y int) ([]byte, error) {
	if !Valid(z, x, y) {
		return nil, ErrNoTile
	}
	key := fmt.Sprintf("%d/%d/%d", z, x, y)
	t.mu.Lock()
	var stale []byte
	if e, ok := t.kept[key]; ok {
		k := e.Value.(*tile)
		t.order.MoveToFront(e)
		if time.Since(k.at) < keepFor {
			t.mu.Unlock()
			return k.data, nil
		}
		stale = k.data
	}
	f, ok := t.fetching[key]
	if !ok {
		f = &fetch{done: make(chan struct{})}
		t.fetching[key] = f
		go t.fetch(key, f)
	}
	t.mu.Unlock()
	select {
	case <-f.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if f.err != nil && stale != nil {
		return stale, nil
	}
	return f.data, f.err
}

// fetch fetches one tile, in its turn and within the rate, and keeps it.
func (t *Tiles) fetch(key string, f *fetch) {
	defer func() {
		t.mu.Lock()
		delete(t.fetching, key)
		t.mu.Unlock()
		close(f.done)
	}()
	if !t.allow() {
		f.err = ErrBusy
		return
	}
	select {
	case t.turns <- struct{}{}:
		defer func() { <-t.turns }()
	case <-time.After(busyFor):
		f.err = ErrBusy
		return
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(t.Base, "/")+"/"+key+".png", nil)
	if err != nil {
		f.err = err
		return
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := t.HTTP.Do(req)
	if err != nil {
		f.err = errors.New("the map's tiles could not be fetched: is the internet reachable from the manager?")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.err = fmt.Errorf("the map's tile server answered %d", resp.StatusCode)
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, tileMax+1))
	if err != nil || len(data) > tileMax {
		f.err = errors.New("the map's tile server sent what is not a tile")
		return
	}
	f.data = data

	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.kept[key]; ok {
		t.bytes -= len(e.Value.(*tile).data)
		t.order.Remove(e)
	}
	t.kept[key] = t.order.PushFront(&tile{key: key, data: data, at: time.Now()})
	t.bytes += len(data)
	for t.bytes > keepBytes {
		last := t.order.Back()
		k := last.Value.(*tile)
		t.order.Remove(last)
		delete(t.kept, k.key)
		t.bytes -= len(k.data)
	}
}

// allow takes one fetch from the rate: burst at first, refill a second
// after.
func (t *Tiles) allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.tokens = min(burst, t.tokens+now.Sub(t.filled).Seconds()*refill)
	t.filled = now
	if t.tokens < 1 {
		return false
	}
	t.tokens--
	return true
}

// Place is a place an address may be.
type Place struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Finder asks where an address is, at most once a second, and keeps what
// it was told a day, as Nominatim asks.
type Finder struct {
	HTTP *http.Client
	Base string // as https://nominatim.openstreetmap.org

	mu    sync.Mutex
	last  time.Time
	found map[string]foundAt
}

type foundAt struct {
	at     time.Time
	places []Place
}

// What is kept of addresses found.
const (
	foundFor  = 24 * time.Hour
	foundKept = 256
)

// NewFinder is a Finder asking base.
func NewFinder(base string) *Finder {
	return &Finder{HTTP: &http.Client{Timeout: 15 * time.Second}, Base: base, found: map[string]foundAt{}}
}

// Find is the places q may be, the likeliest first, five at most.
func (f *Finder) Find(ctx context.Context, q string) ([]Place, error) {
	key := strings.ToLower(strings.Join(strings.Fields(q), " "))
	f.mu.Lock()
	if k, ok := f.found[key]; ok && time.Since(k.at) < foundFor {
		f.mu.Unlock()
		return k.places, nil
	}
	turn := slot(&f.last, time.Second)
	f.mu.Unlock()
	if err := await(ctx, turn); err != nil {
		return nil, err
	}

	v := url.Values{"q": {q}, "format": {"jsonv2"}, "limit": {"5"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(f.Base, "/")+"/search?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("OpenStreetMap could not be asked: is the internet reachable from the manager?")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenStreetMap answered %d", resp.StatusCode)
	}
	var found []struct {
		Name string `json:"display_name"`
		Lat  string `json:"lat"`
		Lon  string `json:"lon"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&found); err != nil {
		return nil, errors.New("OpenStreetMap's answer is not understood")
	}
	out := []Place{}
	for _, p := range found {
		lat, err1 := strconv.ParseFloat(p.Lat, 64)
		lon, err2 := strconv.ParseFloat(p.Lon, 64)
		if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			continue
		}
		out = append(out, Place{Name: p.Name, Latitude: lat, Longitude: lon})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.found) >= foundKept {
		var oldest string
		for k, v := range f.found {
			if oldest == "" || v.at.Before(f.found[oldest].at) {
				oldest = k
			}
		}
		delete(f.found, oldest)
	}
	f.found[key] = foundAt{at: time.Now(), places: out}
	return out, nil
}

// Suggester asks Photon, OpenStreetMap's geocoder made for it, where an
// address being typed may be: Nominatim's policy forbids searching as a
// person types. It asks at most every suggestEvery, and keeps what it was
// told a day.
type Suggester struct {
	HTTP *http.Client
	Base string // as https://photon.komoot.io

	mu    sync.Mutex
	last  time.Time
	found map[string]foundAt
}

// What is kept of suggestions, and how often Photon is asked.
const (
	suggestEvery = 250 * time.Millisecond
	suggestKept  = 512
)

// NewSuggester is a Suggester asking base.
func NewSuggester(base string) *Suggester {
	return &Suggester{HTTP: &http.Client{Timeout: 10 * time.Second}, Base: base, found: map[string]foundAt{}}
}

// Suggest is the places q may be, five at most, the likeliest first; near,
// where there is one, is where the person is looking, which Photon puts
// first among places alike.
func (s *Suggester) Suggest(ctx context.Context, q string, near *[2]float64) ([]Place, error) {
	key := strings.ToLower(strings.Join(strings.Fields(q), " "))
	v := url.Values{"q": {q}, "limit": {"5"}, "lang": {"en"}}
	if near != nil {
		// Near to a tenth of a degree: one kept answer for the area.
		lat, lon := strconv.FormatFloat(near[0], 'f', 1, 64), strconv.FormatFloat(near[1], 'f', 1, 64)
		v.Set("lat", lat)
		v.Set("lon", lon)
		key += "|" + lat + "," + lon
	}
	s.mu.Lock()
	if k, ok := s.found[key]; ok && time.Since(k.at) < foundFor {
		s.mu.Unlock()
		return k.places, nil
	}
	turn := slot(&s.last, suggestEvery)
	s.mu.Unlock()
	if err := await(ctx, turn); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(s.Base, "/")+"/api/?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Photon could not be asked: is the internet reachable from the manager?")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Photon answered %d", resp.StatusCode)
	}
	var found struct {
		Features []struct {
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&found); err != nil {
		return nil, errors.New("Photon's answer is not understood")
	}
	out := []Place{}
	for _, f := range found.Features {
		c := f.Geometry.Coordinates
		if len(c) != 2 || c[1] < -90 || c[1] > 90 || c[0] < -180 || c[0] > 180 {
			continue
		}
		name := photonName(f.Properties)
		if name == "" {
			continue
		}
		out = append(out, Place{Name: name, Latitude: c[1], Longitude: c[0]})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.found) >= suggestKept {
		var oldest string
		for k, v := range s.found {
			if oldest == "" || v.at.Before(s.found[oldest].at) {
				oldest = k
			}
		}
		delete(s.found, oldest)
	}
	s.found[key] = foundAt{at: time.Now(), places: out}
	return out, nil
}

// photonName writes a place as an address is written: its name, its
// number and street, its town, state, postcode and country, each once.
func photonName(p map[string]any) string {
	str := func(k string) string { s, _ := p[k].(string); return strings.TrimSpace(s) }
	street := strings.TrimSpace(str("housenumber") + " " + str("street"))
	var parts []string
	seen := map[string]bool{}
	for _, s := range []string{str("name"), street, str("district"), str("city"), str("state"), str("postcode"), str("country")} {
		if s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// slot takes the next time a server may be asked, every apart after last,
// and keeps it as last: what is kept, or another asking, does not wait for
// it. Called with the lock held.
func slot(last *time.Time, every time.Duration) time.Time {
	next := last.Add(every)
	if now := time.Now(); next.Before(now) {
		next = now
	}
	*last = next
	return next
}

// await waits for a turn, or for the asker to go.
func await(ctx context.Context, turn time.Time) error {
	d := time.Until(turn)
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
