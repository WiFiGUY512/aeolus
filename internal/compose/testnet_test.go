package compose

import (
	"slices"
	"sort"
	"testing"
	"time"
)

// A test network is in an AP's config as it renders only while it is on
// there and its time is not up (0179); other networks are always in it.
func TestLive(t *testing.T) {
	now := time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)
	doc := func(on map[string]any) map[string]any {
		d := map[string]any{"network": map[string]any{
			"home":  map[string]any{"ssid": "Home"},
			"trial": map[string]any{"ssid": "Trial", "test": true},
			"other": map[string]any{"ssid": "Other trial", "test": true},
		}}
		if on != nil {
			d["test_network"] = on
		}
		return d
	}
	ids := func(d map[string]any) []string {
		var out []string
		for id := range d["network"].(map[string]any) {
			out = append(out, id)
		}
		sort.Strings(out)
		return out
	}
	until := float64(now.Add(time.Hour).Unix())
	for _, c := range []struct {
		name string
		on   map[string]any
		at   time.Time
		want []string
	}{
		{"none on", nil, now, []string{"home"}},
		{"one on", map[string]any{"network": "trial", "until": until}, now, []string{"home", "trial"}},
		{"its time up", map[string]any{"network": "trial", "until": until}, now.Add(time.Hour), []string{"home"}},
		{"a clock not set", map[string]any{"network": "trial", "until": until}, time.Unix(1000, 0), []string{"home"}},
		{"no until", map[string]any{"network": "trial"}, now, []string{"home"}},
	} {
		d := doc(c.on)
		if got := ids(Live(d, c.at)); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
		if len(d["network"].(map[string]any)) != 3 {
			t.Errorf("%s: Live changed the config it was given", c.name)
		}
	}
}
