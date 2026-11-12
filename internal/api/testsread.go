package api

import (
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
)

// testsRead keeps the client tests that are over as they were read (0285).
// The Overview, the Assurance tab and the alerts each read the tests of up
// to a week, every half minute a page is open, and every test keeps every
// step it took: some 20 MB in the lab. A test that is over is as it will
// stay, so it is read and parsed once, and after that each reading asks the
// store only which tests there are and how each stands.
type testsRead struct {
	mu   sync.Mutex
	over map[int64]testRecord
}

// testsKept is how far back a test that is over is kept read: further than
// any alert reads, slow radios' usual (a week), and a day to spare. The
// Assurance tab's longer spans read the older ones from the store.
const testsKept = slowUsualOver + 24*time.Hour

// testOver says whether a test in that state is as it will stay.
func testOver(state string) bool {
	return state == "done" || state == "failed" || state == "expired"
}

// tests is the tests of heads, in their order: each kept one whose state is
// as heads says from what was kept, and the rest read by byID now, and kept
// where they are over and recent enough.
func (t *testsRead) tests(heads []conditions.TestHead, byID func([]int64) ([]conditions.Action, error), now time.Time) ([]testRecord, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.over == nil {
		t.over = map[int64]testRecord{}
	}
	var want []int64
	for _, h := range heads {
		if r, ok := t.over[h.ID]; !ok || r.State != h.State {
			want = append(want, h.ID)
		}
	}
	keep := now.Add(-testsKept)
	fresh := map[int64]testRecord{}
	if len(want) > 0 {
		list, err := byID(want)
		if err != nil {
			return nil, err
		}
		for _, a := range list {
			r := readTest(a)
			fresh[a.ID] = r
			if testOver(a.State) && a.At.After(keep) {
				t.over[a.ID] = r
			}
		}
	}
	out := make([]testRecord, 0, len(heads))
	for _, h := range heads {
		if r, ok := fresh[h.ID]; ok {
			out = append(out, r)
		} else if r, ok := t.over[h.ID]; ok {
			out = append(out, r)
		}
	}
	// What no reading reaches any more is let go.
	for id, r := range t.over {
		if r.At.Before(keep) {
			delete(t.over, id)
		}
	}
	return out, nil
}
