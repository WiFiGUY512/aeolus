package api

import (
	"testing"
)

// How the lending of an AP's Bluetooth radio to Home Assistant does (0156):
// kept with the report and shown with its health; not there where the AP
// says nothing of it; and a report that says it badly is refused.
func TestBluetoothState(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	post := func(ble map[string]any) int {
		body := map[string]any{"version": version, "uptime": 60}
		if ble != nil {
			body["ble"] = ble
		}
		code, _, _ := f.apDo("POST", "/v1/ap/state", token, body, nil)
		return code
	}
	now := func() map[string]any {
		_, hl := f.do("GET", "/v1/aps/"+ap+"/health", "griff", nil)
		got, _ := hl["now"].(map[string]any)["ble"].(map[string]any)
		return got
	}

	if code := post(nil); code != 200 {
		t.Fatalf("a report with nothing of it: %d", code)
	}
	if got := now(); got != nil {
		t.Fatalf("nothing said, and the health has %v", got)
	}

	// Lent and heard from: a Home Assistant connected.
	if code := post(map[string]any{
		"radio": true, "running": true, "scanning": true, "active": true, "port": 6053, "name": "c360-ap-ble",
		"heard": 4119, "sent": 129, "devices": 7,
		"clients": []any{map[string]any{"from": "192.168.20.75:41234", "subscribed": true}},
	}); code != 200 {
		t.Fatalf("a radio that is lent: %d", code)
	}
	got := now()
	if got["radio"] != true || got["running"] != true || got["scanning"] != true || got["active"] != true || got["port"] != float64(6053) ||
		got["devices"] != float64(7) || got["heard"] != float64(4119) || got["sent"] != float64(129) || got["name"] != "c360-ap-ble" {
		t.Fatalf("health now = %v", got)
	}
	if c := got["clients"].([]any)[0].(map[string]any); c["from"] != "192.168.20.75:41234" || c["subscribed"] != true {
		t.Fatalf("its client = %v", c)
	}

	// An AP with no such radio, and one whose proxy does not run.
	if code := post(map[string]any{"radio": false}); code != 200 {
		t.Fatalf("no radio: %d", code)
	}
	if got := now(); got == nil || got["radio"] != false {
		t.Fatalf("no radio: health now = %v", got)
	}
	if code := post(map[string]any{"radio": true, "running": false, "tools": false}); code != 200 {
		t.Fatalf("not running: %d", code)
	}
	if got := now(); got["radio"] != true || got["tools"] != false || got["running"] != nil {
		t.Fatalf("not running: health now = %v", got)
	}

	// Said badly.
	many := make([]any, 9)
	for i := range many {
		many[i] = map[string]any{"from": "192.168.20.75:1"}
	}
	for name, ble := range map[string]map[string]any{
		"a port past the last":  {"radio": true, "running": true, "port": 70000},
		"a count below nothing": {"radio": true, "running": true, "heard": -1},
		"too many clients":      {"radio": true, "running": true, "clients": many},
	} {
		if code := post(ble); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
}
