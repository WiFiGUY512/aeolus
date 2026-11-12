package api

import (
	"testing"
	"time"
)

// The live view (0207 part 2): someone watching the APs below a node has
// each that holds its line told at once to send its 5-second points, sees
// each point as the AP sends it, once, and those of the last minutes as it
// begins; one who may not view an AP sees none of its; and when no one
// watches an AP any more, it is told to stop. An AP that opens its line
// while it is watched is told at once.
func TestLiveView(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "annex", "role": "viewer"})

	line, hangLine := f.lineOf("/v1/ap/live", token)
	defer hangLine()
	next(t, line, "the AP's line opens", func(v map[string]any) bool { _, ok := v["trace"]; return ok })

	watching, stop := f.lineOf("/v1/live", f.tokens["griff"])
	head, _ := next(t, watching, "who is watched", func(v map[string]any) bool { return v["every"] != nil })
	var held bool
	for _, a := range head["aps"].([]any) {
		a := a.(map[string]any)
		if a["ap"] == ap {
			held = a["line"] == true
		}
	}
	if !held {
		t.Errorf("the AP, watched: %v", head)
	}
	if _, took := next(t, line, "told to send", func(v map[string]any) bool { return v["watch"] == float64(watchFor) }); took > 200*time.Millisecond {
		t.Errorf("the AP was told after %v", took)
	}

	now := time.Now().Unix() / 5 * 5
	point := func(at int64, busy float64) map[string]any {
		return map[string]any{"at": at, "radios": []any{map[string]any{"radio": "radio0", "band": "5g", "channel": 36, "width": 80, "power": 17, "busy": busy,
			"busy_max": busy, "rx": 1, "tx": 2, "noise": -95, "clients": 1, "clients_max": 1, "tx_packets": 50, "tx_retries": 1, "tx_failed": 0, "rx_packets": 40,
			"tx_bytes": 5000, "rx_bytes": 3000}},
			"clients": []any{map[string]any{"mac": "aa:bb:cc:00:00:01", "band": "5g", "ssid": "Lab", "signal": -60, "signal_min": -61, "tx_rate": 866.7, "rx_rate": 130,
				"tx_packets": 50, "tx_retries": 1, "tx_failed": 0, "rx_packets": 40, "tx_bytes": 5000, "rx_bytes": 3000, "secs": 5}}}
	}
	send := func(pts ...any) {
		t.Helper()
		code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"sent": float64(time.Now().UnixMilli()), "points": pts}, nil)
		if code != 200 {
			t.Fatalf("points: %d %v", code, b)
		}
	}
	busyOf := func(v map[string]any) any { return v["radios"].([]any)[0].(map[string]any)["busy"] }
	send(point(now-10, 12.5), point(now-5, 30))
	got, _ := next(t, watching, "the first point", func(v map[string]any) bool { return v["ap"] == ap })
	if busyOf(got) != 12.5 || got["ap_name"] == "" || len(got["clients"].([]any)) != 1 || got["clients"].([]any)[0].(map[string]any)["signal"] != float64(-60) {
		t.Errorf("the first point: %v", got)
	}
	if got, _ := next(t, watching, "the second", func(v map[string]any) bool { return v["ap"] == ap }); busyOf(got) != float64(30) {
		t.Errorf("the second point: %v", got)
	}
	// Sent again, as after an answer that was lost: not sent on again; a
	// new one is, and one not on the 5 seconds is left out.
	send(point(now-5, 30), point(now, 44), point(now+2, 99))
	if got, _ := next(t, watching, "the third", func(v map[string]any) bool { return v["ap"] == ap }); busyOf(got) != float64(44) {
		t.Errorf("after one sent again, the next: %v", got)
	}

	// Another watching, of everything, is shown the last minutes at once;
	// the tenant, who may not view the AP, nothing of it.
	other, stopOther := f.lineOf("/v1/live", f.tokens["griff"])
	next(t, other, "who is watched", func(v map[string]any) bool { return v["every"] != nil })
	var back []any
	for len(back) < 3 {
		v, _ := next(t, other, "the last minutes", func(v map[string]any) bool { return v["ap"] == ap })
		back = append(back, busyOf(v))
	}
	if back[0] != 12.5 || back[2] != float64(44) {
		t.Errorf("the last minutes: %v", back)
	}
	tenant, stopTenant := f.lineOf("/v1/live", f.tokens["tenant"])
	th, _ := next(t, tenant, "who the tenant watches", func(v map[string]any) bool { return v["every"] != nil })
	if aps := th["aps"].([]any); len(aps) != 0 {
		t.Errorf("the tenant watches: %v", aps)
	}
	stopTenant()

	// The AP opens its line again while watched: told at once.
	again, hangAgain := f.lineOf("/v1/ap/live", token)
	defer hangAgain()
	next(t, again, "told as its line opens", func(v map[string]any) bool { return v["watch"] == float64(watchFor) })

	// No one watching any more: told to stop.
	stop()
	stopOther()
	next(t, again, "told to stop", func(v map[string]any) bool { return v["watch"] == float64(0) })

	if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"sent": 1.0, "points": make([]any, 25)}, nil); code != 400 {
		t.Errorf("25 points at once: %d %v", code, b)
	}
}
