package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A change that ends itself (0345): a test window with a channel pin, made
// with until, listed in /v1/ends with where it puts back; its end moved
// later, then ended now by a person, which takes both the window and the
// pin away. Made with until in the past, it is refused.
func TestEnds(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	window := map[string]any{"kind": "set", "tree": "locations", "node": ap, "until": until.Format(time.RFC3339), "values": map[string]any{
		"maintenance.on": true, "maintenance.until": until.Format("2006-01-02T15:04Z"), "maintenance.hold": true, "radio.5g.channel": 36,
	}}
	code, b := f.change("griff", window)
	if code != 200 {
		t.Fatalf("window: %d %v", code, b)
	}
	seq := b["change"].(map[string]any)["seq"].(float64)

	_, body := f.do("GET", "/v1/ends", "griff", nil)
	ends := body["ends"].([]any)
	if len(ends) != 1 {
		t.Fatalf("ends = %v", body)
	}
	e := ends[0].(map[string]any)
	if e["seq"] != seq || e["until"] != until.Format(time.RFC3339) || len(e["steps"].([]any)) != 4 {
		t.Fatalf("end = %v", e)
	}
	if _, has := e["steps"].([]any)[0].(map[string]any)["was"]; has {
		t.Errorf("an end's values are shown: %v", e)
	}

	later := until.Add(time.Hour)
	path := fmt.Sprintf("/v1/ends/%d", int64(seq))
	if code, b := f.do("POST", path, "griff", map[string]any{"until": later.Format(time.RFC3339)}); code != 200 {
		t.Fatalf("move: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/ends", "griff", nil)
	if got := body["ends"].([]any)[0].(map[string]any)["until"]; got != later.Format(time.RFC3339) {
		t.Fatalf("moved to %v, want %s", got, later)
	}

	// The loop ends nothing that is not due.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.api.EndChanges(ctx, time.Hour)
	if len(f.log.Ends()) != 1 {
		t.Fatal("ended before its until")
	}

	code, b = f.do("POST", path, "griff", nil)
	if code != 200 {
		t.Fatalf("end now: %d %v", code, b)
	}
	if op := b["change"].(map[string]any)["op"].(map[string]any); op["kind"] != "end-change" || op["ends"] != seq || len(op["ops"].([]any)) != 4 {
		t.Fatalf("end = %v", op)
	}
	s := f.log.Snapshot()
	for _, p := range []string{"maintenance.on", "maintenance.until", "maintenance.hold", "radio.5g.channel"} {
		if v, ok := s.Org.Locations.Own(hierarchy.NodeID(ap), hierarchy.Path(p)); ok {
			t.Errorf("%s still set: %v", p, v)
		}
	}
	if code, _ := f.do("POST", path, "griff", nil); code != 404 {
		t.Errorf("ended twice: %d", code)
	}

	window["until"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if code, b := f.change("griff", window); code != 400 {
		t.Errorf("until past: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "end-change", "ends": seq}); code != 400 {
		t.Errorf("end-change as a change: %d %v", code, b)
	}
}
