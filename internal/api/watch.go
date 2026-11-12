package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The live view (0207 part 2). While someone watches the APs below a
// Locations node, each of them is told on its line (0145) to send what
// every 5 seconds came to on its radios, as a minute is reckoned; the
// manager keeps the last quarter hour of those in memory, and sends each
// to whoever watches as it comes. No one watching, nothing is sent.

const (
	// watchFor is how many seconds an AP is told to send for; watchRenew
	// how often a watcher's APs are told again while it watches.
	watchFor   = 120
	watchRenew = 60 * time.Second
	// watchKeep is how long an AP's points are kept in memory; watchBack
	// how far back a watcher is first shown; watchMost how long one
	// watching is held, after which it asks again.
	watchKeep = 15 * time.Minute
	watchBack = 5 * time.Minute
	watchMost = 30 * time.Minute
	// pointsBatch is how many points an AP may send at once; pointsLate
	// how long after its 5 seconds one is still taken.
	pointsBatch = 24
	pointsLate  = watchKeep
)

// livePoint is what 5 seconds came to on one AP (0207 part 2): its radios
// and their clients, as a minute's are; seq is its place among all the
// points the manager has had.
type livePoint struct {
	seq     int64
	AP      hierarchy.NodeID          `json:"ap"`
	At      time.Time                 `json:"at"`
	Radios  []conditions.RadioMinute  `json:"radios"`
	Clients []conditions.ClientMinute `json:"clients"`
}

// liveWatcher is one watching: the APs it watches, and what wakes it.
type liveWatcher struct {
	aps  map[hierarchy.NodeID]bool
	wake chan struct{}
}

// watchHub is who watches which APs, and the APs' last points.
type watchHub struct {
	mu       sync.Mutex
	next     int
	seq      int64
	watchers map[int]*liveWatcher
	points   map[hierarchy.NodeID][]livePoint
}

// join begins a watching of aps.
func (h *watchHub) join(aps []hierarchy.NodeID) (int, *liveWatcher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.watchers == nil {
		h.watchers = map[int]*liveWatcher{}
	}
	h.next++
	w := &liveWatcher{aps: map[hierarchy.NodeID]bool{}, wake: make(chan struct{}, 1)}
	for _, ap := range aps {
		w.aps[ap] = true
	}
	h.watchers[h.next] = w
	return h.next, w
}

// leave ends a watching, and says which of its APs no one watches now.
func (h *watchHub) leave(id int) []hierarchy.NodeID {
	h.mu.Lock()
	defer h.mu.Unlock()
	w := h.watchers[id]
	delete(h.watchers, id)
	if w == nil {
		return nil
	}
	var out []hierarchy.NodeID
	for ap := range w.aps {
		if !h.watchedLocked(ap) {
			out = append(out, ap)
		}
	}
	return out
}

func (h *watchHub) watchedLocked(ap hierarchy.NodeID) bool {
	for _, w := range h.watchers {
		if w.aps[ap] {
			return true
		}
	}
	return false
}

// watched says whether anyone watches an AP.
func (h *watchHub) watched(ap hierarchy.NodeID) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.watchedLocked(ap)
}

// add keeps an AP's points, the last quarter hour of them, each once by
// its time, and wakes whoever watches it.
func (h *watchHub) add(ap hierarchy.NodeID, pts []livePoint, now time.Time) {
	if len(pts) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.points == nil {
		h.points = map[hierarchy.NodeID][]livePoint{}
	}
	have := h.points[ap]
	for _, p := range pts {
		dup := false
		for _, q := range have {
			if q.At.Equal(p.At) {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		h.seq++
		p.seq = h.seq
		have = append(have, p)
	}
	cut := 0
	for cut < len(have) && have[cut].At.Before(now.Add(-watchKeep)) {
		cut++
	}
	h.points[ap] = have[cut:]
	for _, w := range h.watchers {
		if w.aps[ap] {
			select {
			case w.wake <- struct{}{}:
			default:
			}
		}
	}
}

// since is the points of aps the manager had after seq, and from no
// earlier than from, the first had first; and the last seq of them.
func (h *watchHub) since(aps map[hierarchy.NodeID]bool, seq int64, from time.Time) ([]livePoint, int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []livePoint
	last := seq
	for ap := range aps {
		for _, p := range h.points[ap] {
			if p.seq > seq && !p.At.Before(from) {
				out = append(out, p)
			}
			if p.seq > last {
				last = p.seq
			}
		}
	}
	// By when the manager had them, so a watcher's cursor moves on.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].seq < out[j-1].seq; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, last
}

// tellWatch tells an AP on its line to send its points for secs seconds,
// or with 0, to stop; and whether it holds a line to hear it on.
func (s *Server) tellWatch(ap hierarchy.NodeID, secs int) bool {
	b, _ := json.Marshal(liveSay{Now: msOf(time.Now()), Watch: &secs})
	return s.live.tellOne(ap, append(b, '\n'))
}

// pointsOf is what an AP said of its last 5-second stretches.
func pointsOf(ap hierarchy.NodeID, said []json.RawMessage, now time.Time) []livePoint {
	var out []livePoint
	for _, raw := range said {
		at, rs, cs, ok := readStretch(ap, raw, 5, now, pointsLate)
		if ok {
			out = append(out, livePoint{AP: ap, At: at, Radios: rs, Clients: cs})
		}
	}
	return out
}

// liveRadio and liveClient are a point's radio and client as a watcher is
// sent them, without what the point says already.
type liveRadio struct {
	Radio string `json:"radio"`
	Band  string `json:"band"`
	minutePoint
}

type liveClient struct {
	MAC       string   `json:"mac"`
	Host      string   `json:"host,omitempty"`
	Band      string   `json:"band"`
	SSID      string   `json:"ssid"`
	Signal    *int     `json:"signal"`
	SignalMin *int     `json:"signal_min"`
	TxRate    *float64 `json:"tx_rate"`
	RxRate    *float64 `json:"rx_rate"`
	Secs      int      `json:"secs"`
	Heard     *int     `json:"heard,omitempty"`
	conditions.Counts
}

// liveSent is one point as a watcher is sent it.
type liveSent struct {
	AP      hierarchy.NodeID `json:"ap"`
	Name    string           `json:"ap_name"`
	At      time.Time        `json:"at"`
	Radios  []liveRadio      `json:"radios"`
	Clients []liveClient     `json:"clients"`
}

// liveAP is one AP watched, as a watcher is told of it: whether it holds
// its line, without which it is told nothing and sends nothing.
type liveAP struct {
	AP   hierarchy.NodeID `json:"ap"`
	Name string           `json:"ap_name"`
	Line bool             `json:"line"`
}

// liveView is the live view of the APs the caller may view, below one
// Locations node where ?under= names it (0207 part 2): GET /v1/live. The
// answer does not end, a line of JSON at a time: first {every, aps}, the
// seconds between two points and each AP watched with whether it holds
// its line, said again every minute; then each point of the last five
// minutes, and each one after as it comes, {ap, ap_name, at, radios,
// clients}, as GET /v1/minutes has them, at being the start of its 5
// seconds; and {} every ten seconds between. Each AP is told to send while
// anyone watches it, and to stop when no one does. It ends after half an
// hour, when the caller asks again.
func (s *Server) liveView(w http.ResponseWriter, r *http.Request, c call) error {
	aps := mayView(c, hierarchy.NodeID(r.URL.Query().Get("under")))
	put, err := held(w)
	if err != nil {
		return err
	}
	write := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return put(append(b, '\n'))
	}
	t := c.state.Org.Locations
	name := func(ap hierarchy.NodeID) string {
		if n, ok := t.Node(ap); ok {
			return n.Name
		}
		return string(ap)
	}
	id, watcher := s.watching.join(aps)
	defer func() {
		for _, ap := range s.watching.leave(id) {
			s.tellWatch(ap, 0)
		}
	}()
	tell := func() error {
		said := make([]liveAP, 0, len(aps))
		for _, ap := range aps {
			said = append(said, liveAP{AP: ap, Name: name(ap), Line: s.tellWatch(ap, watchFor)})
		}
		return write(map[string]any{"every": 5, "aps": said})
	}
	if tell() != nil {
		return nil
	}
	hosts := map[string]string{}
	host := func(mac string) string {
		if h, ok := hosts[mac]; ok {
			return h
		}
		got, _ := s.conds.Hosts([]string{mac}, aps)
		hosts[mac] = got[mac]
		return hosts[mac]
	}
	send := func(p livePoint) error {
		v := liveSent{AP: p.AP, Name: name(p.AP), At: p.At}
		v.Radios, v.Clients = slimPoint(p, host)
		return write(v)
	}
	var seq int64
	from := time.Now().Add(-watchBack)
	renew := time.NewTicker(watchRenew)
	defer renew.Stop()
	beat := time.NewTicker(liveBeat)
	defer beat.Stop()
	end := time.NewTimer(watchMost)
	defer end.Stop()
	for {
		pts, last := s.watching.since(watcher.aps, seq, from)
		seq = last
		for _, p := range pts {
			if send(p) != nil {
				return nil
			}
		}
		select {
		case <-r.Context().Done():
			return nil
		case <-end.C:
			return nil
		case <-watcher.wake:
		case <-renew.C:
			if tell() != nil {
				return nil
			}
		case <-beat.C:
			if write(struct{}{}) != nil {
				return nil
			}
		}
	}
}
