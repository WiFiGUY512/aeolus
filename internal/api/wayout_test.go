package api

import (
	"fmt"
	"strings"
	"testing"
)

// TestATestFindsTheWayOutLost (0199): a client test that found full-size
// packets lost on the way out, where TCP sends more than gets through, is
// taken with its way out, fails at it, and is drawn stopped there; one
// whose way out is not the shape of one is refused.
func TestATestFindsTheWayOutLost(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:7a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", scanner[1], map[string]any{"version": cfg["version"], "uptime": 60, "bssids": []string{"02:00:00:00:00:7a"}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	ask := func() string {
		t.Helper()
		code, body := f.do("POST", "/v1/aps/"+scanner[0]+"/actions", "griff", map[string]any{"kind": "client-test", "network": "sweet", "on": scanner[0], "band": "5g"})
		if code != 200 {
			t.Fatalf("asked: %d %v", code, body)
		}
		f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
		return fmt.Sprintf("%d", int64(body["action"].(map[string]any)["id"].(float64)))
	}
	why := "packets over 1408 bytes without DF are lost on the way out, and TCP here sends 1472, so IoT clients that send without DF stall: clamp TCP's MSS to 1368 or less at the internet router"
	yes, no := true, false
	lost := map[string]any{
		"v": 1, "ssid": "Home Wi-Fi", "band": "5g", "bssid": "02:00:00:00:00:7a", "outcome": "failed", "stage": "path", "reason": why,
		"steps": []any{
			map[string]any{"t": 900, "stage": "dhcp", "what": "given 192.168.20.77", "ok": yes, "ms": 300},
			map[string]any{"t": 1200, "stage": "gateway", "what": "192.168.20.1, its gateway, answers 3 of 3 pings", "ok": yes, "ms": 4},
			map[string]any{"t": 1300, "stage": "dns", "what": "192.168.20.254 answers example.com", "ok": yes, "ms": 40},
			map[string]any{"t": 9300, "stage": "path", "what": "packets over 1408 bytes sent without DF are lost on the way to one.one.one.one, and TCP here sends 1472", "ok": no},
		},
		"path": map[string]any{"target": "one.one.one.one", "largest": 1408, "tcp": 1472, "lost": true, "what": "packets over 1408 bytes sent without DF are lost", "why": why},
	}
	id := ask()
	if code, _, b := f.apDo("POST", "/v1/ap/actions/"+id, scanner[1], map[string]any{"ok": false, "result": "failed at path: " + why, "detail": lost}, nil); code != 200 {
		t.Fatalf("a way out lost: %d %v", code, b)
	}
	code, list := f.do("GET", "/v1/aps/"+scanner[0]+"/actions", "griff", nil)
	if code != 200 {
		t.Fatalf("actions: %d %v", code, list)
	}
	s := string(mustJSON(list))
	for _, want := range []string{`"stage":"path"`, `"largest":1408`, `"tcp":1472`, "clamp TCP's MSS to 1368", `"k":"path"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the test's record has no %s: %s", want, s)
		}
	}

	// A way out that is not one is refused.
	bad := map[string]any{"v": 1, "ssid": "Home Wi-Fi", "band": "5g", "bssid": "02:00:00:00:00:7a", "outcome": "passed", "stage": "done", "reason": "", "steps": []any{},
		"path": map[string]any{"target": "", "largest": 99999}}
	id = ask()
	if code, _, b := f.apDo("POST", "/v1/ap/actions/"+id, scanner[1], map[string]any{"ok": true, "result": "passed", "detail": bad}, nil); code != 400 {
		t.Errorf("a way out with no target, and too large: %d %v", code, b)
	}
}
