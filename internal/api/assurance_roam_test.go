package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A test through the air roams, once on, to the other APs its tester tests
// the network on, on that band, where the folders say (0225); and what the
// roams came to is summed up a network and a band at a time.
func TestAssuranceRoams(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:4a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:4b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	say := func(tok string, more map[string]any) {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", tok, nil, nil)
		body := map[string]any{"version": cfg["version"], "uptime": 60}
		for k, v := range more {
			body[k] = v
		}
		if code, _, b := f.apDo("POST", "/v1/ap/state", tok, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	all := func() {
		say(plain[1], map[string]any{"bssids": []string{"02:00:00:00:00:4b", "02:00:00:00:00:5b"}})
		say(scanner[1], map[string]any{"bssids": []string{"02:00:00:00:00:4a"}, "rrm": map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"},
			"neighbours": []any{map[string]any{"ap": plain[0], "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
				"bands": []any{map[string]any{"band": "5g", "signal": -50, "their_signal": -50, "channel": 44, "width": 40}}}}, "ratings": []any{}, "moves": []any{}}})
	}
	all()
	set := func(path string, v any) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": path, "value": v}); code != 200 {
			t.Fatalf("%s: %d %v", path, code, b)
		}
	}
	set("assurance.tests.every", 15)

	type asked struct {
		ID     int64
		On     string
		Roam   [][]string
		RoamOn []string `json:"roam_on"`
	}
	ask := func() asked {
		t.Helper()
		got := f.api.assure(f.now())
		if len(got) != 1 {
			t.Fatalf("asked %v", got)
		}
		var a asked
		json.Unmarshal(got[0].Params, &a)
		a.ID = got[0].ID
		return a
	}
	done := func(a asked, detail map[string]any) int {
		t.Helper()
		f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
		d := map[string]any{"v": 1, "ssid": "Home Wi-Fi", "band": "5g", "bssid": "02:00:00:00:00:4a", "channel": 36, "client": "02:00:00:00:00:fe",
			"outcome": "passed", "stage": "done", "reason": "", "steps": []any{}}
		for k, v := range detail {
			d[k] = v
		}
		code, _, _ := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", a.ID), scanner[1], map[string]any{"ok": true, "result": "passed", "detail": d}, nil)
		return code
	}

	// Unasked, no test roams.
	first := ask()
	if first.On != scanner[0] || first.Roam != nil {
		t.Fatalf("roams unasked: %+v", first)
	}
	if code := done(first, nil); code != 200 {
		t.Fatalf("done: %d", code)
	}

	// Asked, in a round asked by hand: the test on the scanner itself
	// roams to Plain, by Plain's BSSIDs.
	set("assurance.tests.roam", true)
	if code, b := f.do("POST", "/v1/assurance/round", "griff", map[string]any{"under": "office", "over": "air"}); code != 200 {
		t.Fatalf("round: %d %v", code, b)
	}
	second := ask()
	if second.On != scanner[0] || len(second.Roam) != 1 || strings.Join(second.Roam[0], " ") != "02:00:00:00:00:4b 02:00:00:00:00:5b" || second.RoamOn[0] != plain[0] {
		t.Fatalf("the test on the scanner = %+v", second)
	}
	roam := func(bssid, kind string, ms int, ok bool) map[string]any {
		r := map[string]any{"bssid": bssid, "band": "5g", "ok": ok}
		if ok {
			r["ms"], r["kind"], r["key_mgmt"] = ms, kind, "FT-SAE"
		} else {
			r["why"] = "it was not let on there within 8 s"
		}
		return r
	}
	// What is not a roam is refused.
	if code := done(second, map[string]any{"roams": []any{map[string]any{"bssid": "02:00:00:00:00:4a", "ok": true, "ms": 12, "kind": "teleport"}}}); code != 400 {
		t.Fatalf("a roam of an unknown kind: %d", code)
	}
	if code := done(second, map[string]any{"steps": []any{map[string]any{"t": 900, "stage": "roam", "what": "roamed to 02:00:00:00:00:4a by 802.11r in 12 ms", "ok": true, "ms": 12}},
		"roams": []any{roam("02:00:00:00:00:4a", "ft", 12, true), roam("02:00:00:00:00:4b", "ft", 18, true), roam("02:00:00:00:00:4a", "full", 140, true), roam("02:00:00:00:00:4b", "", 0, false)}}); code != 200 {
		t.Fatalf("done with roams: %d", code)
	}

	_, view := f.do("GET", "/v1/assurance", "griff", nil)
	roams, _ := view["roams"].([]any)
	if len(roams) != 1 {
		t.Fatalf("roams = %v", view["roams"])
	}
	r := roams[0].(map[string]any)
	kinds := r["kinds"].([]any)
	ft, full := kinds[0].(map[string]any), kinds[1].(map[string]any)
	// No AP has recorded them yet: counted, and untimed, as the test's own
	// clock takes in the scan radio's going to the other channel.
	if r["network"] != "sweet" || r["band"] != "5g" || r["roams"] != 4.0 || r["failed"] != 1.0 || len(kinds) != 2 ||
		ft["kind"] != "ft" || ft["roams"] != 2.0 || ft["untimed"] != 2.0 || ft["typical_ms"] != nil || full["kind"] != "full" || full["untimed"] != 1.0 {
		t.Fatalf("roams = %v", r)
	}

	// The APs it went to recorded the client: the roam's own time is
	// theirs, from its first authentication there, where the test's own
	// took in the scan radio going to the other channel. One they recorded
	// nothing of is the test's.
	at := f.now()
	attempt := func(token string, ago time.Duration, how map[string]any) {
		t.Helper()
		rec := map[string]any{"mac": "02:00:00:00:00:fe", "bss": "phy0-ap0", "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g",
			"started": at.Add(-ago).UnixMilli(), "took_ms": 30, "outcome": "connected", "stage": "key", "reason": "", "events": []any{}, "how": how}
		if code, _, b := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{rec}}, nil); code != 200 || b["recorded"] != 1.0 {
			t.Fatalf("an attempt: %d %v", code, b)
		}
	}
	attempt(plain[1], 5*time.Second, map[string]any{"alg": "open", "on_ms": 40})
	// The same client on another network at the scanner, as the test of
	// that network just before or after: not this test's roam.
	other := func(token string, ago time.Duration, how map[string]any) {
		t.Helper()
		rec := map[string]any{"mac": "02:00:00:00:00:fe", "bss": "phy0-ap1", "network": "guest", "ssid": "Guest", "band": "5g",
			"started": at.Add(-ago).UnixMilli(), "took_ms": 30, "outcome": "connected", "stage": "key", "reason": "", "events": []any{}, "how": how}
		if code, _, b := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{rec}}, nil); code != 200 || b["recorded"] != 1.0 {
			t.Fatalf("an attempt: %d %v", code, b)
		}
	}
	other(plain[1], 6*time.Second, map[string]any{"alg": "open", "on_ms": 40})
	other(scanner[1], 3500*time.Millisecond, map[string]any{"re": true, "alg": "ft", "on_ms": 2})
	attempt(scanner[1], 4*time.Second, map[string]any{"re": true, "alg": "ft", "on_ms": 9})
	attempt(plain[1], 3*time.Second, map[string]any{"re": true, "alg": "ft", "on_ms": 11})
	_, view = f.do("GET", "/v1/assurance", "griff", nil)
	r = view["roams"].([]any)[0].(map[string]any)
	kinds = r["kinds"].([]any)
	ft, full = kinds[0].(map[string]any), kinds[1].(map[string]any)
	if ft["kind"] != "ft" || ft["roams"] != 2.0 || ft["untimed"] != nil || ft["low_ms"] != 9.0 || ft["high_ms"] != 11.0 || full["untimed"] != 1.0 || full["typical_ms"] != nil {
		t.Fatalf("roams as the APs recorded them = %v", r)
	}
}
