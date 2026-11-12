package api

import (
	"fmt"
	"testing"
	"time"
)

// The flight recorder (0207 part 3): an AP that sends a recording with its
// points has it kept, with the points that fall in it, each once; it is
// listed, and read with its points, by those who may view the AP and by
// no one else; and someone who may operate the APs below a node can ask
// them to record now, on their lines.
func TestRecordings(t *testing.T) {
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

	now := time.Now().Unix() / 5 * 5
	point := func(at int64, busy float64) map[string]any {
		return map[string]any{"at": at, "radios": []any{map[string]any{"radio": "radio0", "band": "5g", "busy": busy, "clients": 3, "tx_packets": 400, "tx_retries": 200, "tx_failed": 0}},
			"clients": []any{map[string]any{"mac": "aa:bb:cc:00:00:01", "band": "5g", "ssid": "Lab", "signal": -70, "tx_packets": 100, "tx_retries": 60, "secs": 5, "heard": -66}}}
	}
	rec := map[string]any{"at": now - 30, "why": "retries", "said": "5g: 50% of 400 tried again or lost in five seconds, against 5% as usual", "from": now - 60, "until": now + 600}
	send := func(body map[string]any) {
		t.Helper()
		body["sent"] = float64(time.Now().UnixMilli())
		if code, _, b := f.apDo("POST", "/v1/ap/live", token, body, nil); code != 200 {
			t.Fatalf("live: %d %v", code, b)
		}
	}
	// The ten minutes before, as kept, then one after; one from before the
	// recording is not kept.
	send(map[string]any{"record": rec, "points": []any{point(now-120, 10), point(now-60, 20), point(now-30, 90), point(now-25, 80)}})
	send(map[string]any{"record": rec, "points": []any{point(now-25, 80), point(now, 40)}})
	// Made longer by what went wrong again in it.
	longer := map[string]any{}
	for k, v := range rec {
		longer[k] = v
	}
	longer["until"] = now + 900
	send(map[string]any{"record": longer, "points": []any{point(now+5, 30)}})
	// One not well said is not kept.
	send(map[string]any{"record": map[string]any{"at": now, "why": "boredom", "from": now - 10, "until": now + 10}})

	code, v := f.do("GET", "/v1/recordings", "griff", nil)
	recs, _ := v["recordings"].([]any)
	if code != 200 || len(recs) != 1 {
		t.Fatalf("the recordings: %d %v", code, v)
	}
	r := recs[0].(map[string]any)
	if r["ap"] != ap || r["why"] != "retries" || r["points"] != float64(5) || r["ap_name"] == "" || r["said"] == "" {
		t.Errorf("the recording: %v", r)
	}
	if until, _ := time.Parse(time.RFC3339Nano, r["until"].(string)); until.Unix() != now+900 {
		t.Errorf("made longer to: %v", r["until"])
	}
	id := fmt.Sprint(int64(r["id"].(float64)))
	code, one := f.do("GET", "/v1/recordings/"+id, "griff", nil)
	pts, _ := one["points"].([]any)
	if code != 200 || len(pts) != 5 {
		t.Fatalf("the recording read: %d %v", code, one)
	}
	first := pts[0].(map[string]any)
	if at, _ := time.Parse(time.RFC3339Nano, first["at"].(string)); at.Unix() != now-60 {
		t.Errorf("its first point: %v", first["at"])
	}
	c0 := first["clients"].([]any)[0].(map[string]any)
	if first["radios"].([]any)[0].(map[string]any)["busy"] != float64(20) || c0["heard"] != float64(-66) || c0["signal"] != float64(-70) {
		t.Errorf("its first point: %v", first)
	}
	// The tenant, who may not view the AP, has none of it.
	if code, v := f.do("GET", "/v1/recordings", "tenant", nil); code != 200 || len(v["recordings"].([]any)) != 0 {
		t.Errorf("the tenant's recordings: %d %v", code, v)
	}
	if code, _ := f.do("GET", "/v1/recordings/"+id, "tenant", nil); code != 404 {
		t.Errorf("the tenant reads it: %d", code)
	}
	for _, bad := range []string{"0", "nope", "99999"} {
		if code, _ := f.do("GET", "/v1/recordings/"+bad, "griff", nil); code != 400 && code != 404 {
			t.Errorf("recording %s: %d", bad, code)
		}
	}

	// Asked to record now, the AP hears it on its line.
	line, hang := f.lineOf("/v1/ap/live", token)
	defer hang()
	next(t, line, "the line opens", func(v map[string]any) bool { _, ok := v["trace"]; return ok })
	code, asked := f.do("POST", "/v1/recordings", "griff", map[string]any{"under": "house", "before": 300})
	if code != 200 {
		t.Fatalf("record now: %d %v", code, asked)
	}
	said, _ := next(t, line, "told to record", func(v map[string]any) bool { return v["record"] != nil })
	if r := said["record"].(map[string]any); r["before"] != float64(300) || r["after"] != float64(600) || r["said"] != "asked by griff" {
		t.Errorf("told: %v", said)
	}
	if code, _ := f.do("POST", "/v1/recordings", "tenant", map[string]any{"under": "annex"}); code != 403 {
		t.Errorf("the tenant asks: %d", code)
	}
	if code, _ := f.do("POST", "/v1/recordings", "griff", map[string]any{"under": "house", "before": 4000}); code != 400 {
		t.Errorf("too long before: %d", code)
	}
}
