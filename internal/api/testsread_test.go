package api

import (
	"slices"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
)

// A test that is over is read once; one still open is read again each time,
// until it is over; one gone from the heads is not given; one too old to be
// kept is let go (0285).
func TestTestsReadKeepsThoseOver(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	store := map[int64]conditions.Action{
		3: {ID: 3, Kind: "client-test", State: "running", At: now.Add(-time.Minute)},
		2: {ID: 2, Kind: "client-test", State: "done", At: now.Add(-time.Hour), Detail: []byte(`{"client":"0e:00:00:00:00:02"}`)},
		1: {ID: 1, Kind: "client-test", State: "failed", At: now.Add(-testsKept - time.Hour)},
	}
	var asked [][]int64
	byID := func(ids []int64) ([]conditions.Action, error) {
		asked = append(asked, slices.Clone(ids))
		var out []conditions.Action
		for _, id := range ids {
			if a, ok := store[id]; ok {
				out = append(out, a)
			}
		}
		return out, nil
	}
	heads := func() []conditions.TestHead {
		var out []conditions.TestHead
		for _, id := range []int64{3, 2, 1} {
			if a, ok := store[id]; ok {
				out = append(out, conditions.TestHead{ID: id, State: a.State})
			}
		}
		return out
	}
	var r testsRead
	ids := func(list []testRecord) []int64 {
		var out []int64
		for _, x := range list {
			out = append(out, x.ID)
		}
		return out
	}

	got, err := r.tests(heads(), byID, now)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(got), []int64{3, 2, 1}) || !slices.Equal(asked[0], []int64{3, 2, 1}) {
		t.Fatalf("first reading: got %v, asked %v", ids(got), asked)
	}
	if !got[1].ran() {
		t.Errorf("test 2 was not parsed: %+v", got[1])
	}

	// Again: 2 is kept; 3 is open, read again; 1 is too old to keep.
	if _, err := r.tests(heads(), byID, now); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(asked[1], []int64{3, 1}) {
		t.Fatalf("second reading asked %v, want [3 1]", asked[1])
	}

	// 3 is over: read once more, then kept.
	s := store[3]
	s.State = "done"
	store[3] = s
	r.tests(heads(), byID, now)
	r.tests(heads(), byID, now)
	if !slices.Equal(asked[2], []int64{3, 1}) || !slices.Equal(asked[3], []int64{1}) {
		t.Fatalf("after 3 was over, asked %v then %v", asked[2], asked[3])
	}

	// One gone from the store is not given, though it was kept.
	delete(store, 2)
	got, _ = r.tests(heads(), byID, now)
	if !slices.Equal(ids(got), []int64{3, 1}) {
		t.Errorf("after 2 was pruned: got %v", ids(got))
	}
	if _, ok := r.over[1]; ok {
		t.Error("test 1, older than testsKept, was kept")
	}
}
