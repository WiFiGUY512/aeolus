package compose

import (
	"testing"
	"time"
)

// A test window (0344) is maintenance with hold and an end: held while its
// end has not come; hold without an end, or maintenance off, holds nothing.
func TestMaintenanceWindow(t *testing.T) {
	now := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	of := func(v map[string]any) Maintenance {
		return MaintenanceOf(func(p string) any { return v[p] })
	}
	cases := []struct {
		name     string
		v        map[string]any
		in, held bool
		until    string
	}{
		{"window", map[string]any{"maintenance.on": true, "maintenance.until": "2026-11-08T21:00Z", "maintenance.hold": true}, true, true, "2026-11-08T21:00:00Z"},
		{"a day", map[string]any{"maintenance.on": true, "maintenance.until": "2026-11-09", "maintenance.hold": true}, true, true, "2026-11-09T00:00:00Z"},
		{"over", map[string]any{"maintenance.on": true, "maintenance.until": "2026-11-08T19:59Z", "maintenance.hold": true}, false, false, "2026-11-08T19:59:00Z"},
		{"no end", map[string]any{"maintenance.on": true, "maintenance.hold": true}, true, false, ""},
		{"no hold", map[string]any{"maintenance.on": true, "maintenance.until": "2026-11-08T21:00Z"}, true, false, "2026-11-08T21:00:00Z"},
		{"off", map[string]any{"maintenance.on": false, "maintenance.until": "2026-11-08T21:00Z", "maintenance.hold": true}, false, false, "2026-11-08T21:00:00Z"},
		{"unreadable end", map[string]any{"maintenance.on": true, "maintenance.until": "soon", "maintenance.hold": true}, true, false, ""},
	}
	for _, c := range cases {
		m := of(c.v)
		got := ""
		if !m.Until.IsZero() {
			got = m.Until.Format(time.RFC3339)
		}
		if m.In(now) != c.in || m.Held(now) != c.held || got != c.until {
			t.Errorf("%s: in %v held %v until %q, want %v %v %q", c.name, m.In(now), m.Held(now), got, c.in, c.held, c.until)
		}
	}

	// The AP is sent the end and why of a window, come or not; nothing
	// otherwise.
	doc := map[string]any{}
	markHold(doc, of(cases[0].v))
	if h, _ := doc["hold"].(map[string]any); h["until"] != float64(time.Date(2026, 11, 8, 21, 0, 0, 0, time.UTC).Unix()) || h["why"] != nil {
		t.Fatalf("hold sent: %v", doc)
	}
	doc = map[string]any{}
	markHold(doc, of(map[string]any{"maintenance.on": true, "maintenance.until": "2026-11-08T19:59Z", "maintenance.hold": true, "maintenance.why": "a test"}))
	if h, _ := doc["hold"].(map[string]any); h["why"] != "a test" || h["until"] == nil {
		t.Fatalf("a window over is still sent: %v", doc)
	}
	for _, c := range cases[3:] {
		doc = map[string]any{}
		if markHold(doc, of(c.v)); doc["hold"] != nil {
			t.Errorf("%s: hold sent: %v", c.name, doc)
		}
	}
}
