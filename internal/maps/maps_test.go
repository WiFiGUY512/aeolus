package maps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTiles(t *testing.T) {
	asked := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != UserAgent {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		asked[r.URL.Path]++
		if r.URL.Path == "/3/1/9.png" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("png of " + r.URL.Path))
	}))
	defer srv.Close()
	tl := NewTiles(srv.URL)
	ctx := context.Background()

	got, err := tl.Get(ctx, 2, 1, 3)
	if err != nil || string(got) != "png of /2/1/3.png" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	// Kept: asked again, the server is not.
	tl.Get(ctx, 2, 1, 3)
	if asked["/2/1/3.png"] != 1 {
		t.Errorf("the server was asked %d times", asked["/2/1/3.png"])
	}
	for _, bad := range [][3]int{{20, 0, 0}, {2, 4, 0}, {2, 0, -1}, {-1, 0, 0}} {
		if _, err := tl.Get(ctx, bad[0], bad[1], bad[2]); err != ErrNoTile {
			t.Errorf("Get(%v) = %v", bad, err)
		}
	}
	if _, err := tl.Get(ctx, 3, 1, 1); err != nil {
		t.Error(err)
	}
	if _, err := tl.Get(ctx, 3, 1, 9); err != ErrNoTile {
		// Off the map: refused before the server is asked.
		t.Errorf("off the map: %v", err)
	}

	// The oldest go first once more is kept than may be.
	tl.bytes = keepBytes
	tl.Get(ctx, 4, 0, 0)
	if _, ok := tl.kept["2/1/3"]; ok || tl.bytes > keepBytes {
		t.Errorf("kept %d bytes, 2/1/3 still: %v", tl.bytes, ok)
	}
	// A tile kept past its week is given as it was when the server fails.
	tl.Get(ctx, 3, 2, 2)
	tl.kept["3/2/2"].Value.(*tile).at = time.Now().Add(-keepFor - time.Minute)
	srv.Close()
	if got, err := tl.Get(ctx, 3, 2, 2); err != nil || string(got) != "png of /3/2/2.png" {
		t.Errorf("stale = %q, %v", got, err)
	}
	if _, err := tl.Get(ctx, 5, 0, 0); err == nil || !strings.Contains(err.Error(), "internet") {
		t.Errorf("no server: %v", err)
	}
	// Past the rate, none is fetched.
	tl.tokens, tl.filled = 0, time.Now()
	if _, err := tl.Get(ctx, 5, 1, 0); err != ErrBusy {
		t.Errorf("past the rate: %v", err)
	}
}

func TestTilesFetchedOnce(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked++
		mu.Unlock()
		<-release
		w.Write([]byte("png"))
	}))
	defer srv.Close()
	tl := NewTiles(srv.URL)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b, err := tl.Get(context.Background(), 1, 0, 0); err != nil || string(b) != "png" {
				t.Errorf("Get = %q, %v", b, err)
			}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if asked != 1 {
		t.Errorf("five at once asked the server %d times", asked)
	}
}

func TestFind(t *testing.T) {
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at = append(at, time.Now())
		if r.URL.Path != "/search" || r.URL.Query().Get("format") != "jsonv2" || r.Header.Get("User-Agent") != UserAgent {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write([]byte(`[{"display_name":"Montgomery, Alabama, United States","lat":"32.3792","lon":"-86.3077"},{"display_name":"nowhere","lat":"x","lon":"1"}]`))
	}))
	defer srv.Close()
	f := NewFinder(srv.URL)
	got, err := f.Find(context.Background(), "Montgomery AL")
	if err != nil || len(got) != 1 || got[0].Latitude != 32.3792 || got[0].Longitude != -86.3077 || !strings.HasPrefix(got[0].Name, "Montgomery") {
		t.Fatalf("Find = %+v, %v", got, err)
	}
	// Asked again, it is kept: the server is not asked.
	if again, _ := f.Find(context.Background(), "  montgomery   al"); len(at) != 1 || len(again) != 1 {
		t.Errorf("asked %d times", len(at))
	}
	// Another at once waits its second.
	f.Find(context.Background(), "Selma AL")
	if len(at) != 2 || at[1].Sub(at[0]) < 900*time.Millisecond {
		t.Errorf("asked %d times, %v apart", len(at), at[len(at)-1].Sub(at[0]))
	}
}

func TestSuggest(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.RawQuery)
		if r.URL.Path != "/api/" || r.Header.Get("User-Agent") != UserAgent {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"type":"FeatureCollection","features":[
			{"type":"Feature","geometry":{"type":"Point","coordinates":[-86.3010,32.3777]},"properties":{"name":"Alabama State Capitol","housenumber":"600","street":"Dexter Avenue","city":"Montgomery","state":"Alabama","postcode":"36104","country":"United States"}},
			{"type":"Feature","geometry":{"type":"Point","coordinates":[-86.3,32.38]},"properties":{"city":"Montgomery","state":"Alabama","country":"United States","name":"Montgomery"}},
			{"type":"Feature","geometry":{"type":"Point","coordinates":[-86.3]},"properties":{"name":"broken"}}]}`))
	}))
	defer srv.Close()
	s := NewSuggester(srv.URL)
	ctx := context.Background()
	got, err := s.Suggest(ctx, "600 dexter", &[2]float64{32.3771, -86.3017})
	if err != nil || len(got) != 2 {
		t.Fatalf("Suggest = %+v, %v", got, err)
	}
	if got[0].Name != "Alabama State Capitol, 600 Dexter Avenue, Montgomery, Alabama, 36104, United States" || got[0].Latitude != 32.3777 || got[0].Longitude != -86.301 {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Name != "Montgomery, Alabama, United States" {
		t.Errorf("a town, named once = %q", got[1].Name)
	}
	if q := asked[0]; !strings.Contains(q, "lat=32.4") || !strings.Contains(q, "lon=-86.3") || !strings.Contains(q, "limit=5") {
		t.Errorf("asked %s", q)
	}
	// Kept: asked again near the same place, Photon is not.
	s.Suggest(ctx, "600  Dexter", &[2]float64{32.39, -86.31})
	if len(asked) != 1 {
		t.Errorf("asked %d times", len(asked))
	}
	// Another at once waits its turn.
	start := time.Now()
	s.Suggest(ctx, "600 dext", nil)
	if len(asked) != 2 || time.Since(start) < 150*time.Millisecond || strings.Contains(asked[1], "lat=") {
		t.Errorf("asked %v after %v", asked, time.Since(start))
	}
}

func TestSuggestWaitsApart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"features":[]}`))
	}))
	defer srv.Close()
	s := NewSuggester(srv.URL)
	ctx := context.Background()
	s.Suggest(ctx, "first", nil)
	// One waiting its turn holds no one else up: a kept answer comes at once.
	done := make(chan struct{})
	go func() { s.Suggest(ctx, "second", nil); close(done) }()
	for {
		s.mu.Lock()
		taken := s.last.After(time.Now())
		s.mu.Unlock()
		if taken {
			break
		}
		runtime.Gosched()
	}
	start := time.Now()
	if _, err := s.Suggest(ctx, "first", nil); err != nil || time.Since(start) > 100*time.Millisecond {
		t.Errorf("a kept answer took %v: %v", time.Since(start), err)
	}
	// One whose asker goes stops waiting.
	gone, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Suggest(gone, "third", nil); err != context.Canceled {
		t.Errorf("gone: %v", err)
	}
	<-done
}
