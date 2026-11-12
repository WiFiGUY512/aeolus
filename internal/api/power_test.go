package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Power control (0077, 0206) as the page has it: each AP's settings, the
// defaults where none are set, and its radios as it last said, its lonely
// clients by name; and the steps its reports said, each kept once, the
// latest first. Of the APs the caller may view.
func TestPower(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "annex", "role": "viewer"})
	radio := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	enroll := func(name, mac, folder string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": []any{radio}}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder})
		return ap, token
	}
	north, northToken := enroll("North", "02:00:00:00:00:3a", "office")
	east, _ := enroll("East", "02:00:00:00:00:3c", "annex")
	set := func(node, path string, v any) {
		change(map[string]any{"kind": "set", "tree": "locations", "node": node, "path": path, "value": v})
	}
	set("house", "rrm.enabled", true)
	set("house", "apc.enabled", true)
	set("office", "apc.clients", "guard")
	set("office", "apc.client_floor", -75)

	now := time.Now()
	const lonely = "02:00:00:00:00:0a"
	step := func(ago time.Duration, from, to int, why string) map[string]any {
		return map[string]any{"at": now.Add(-ago).Unix(), "band": "5g", "from": from, "to": to, "why": why}
	}
	steps := []any{
		step(3*time.Hour, 20, 17, "above"),
		step(50*time.Minute, 17, 14, "above"),
		map[string]any{"at": now.Add(-40 * time.Minute).Unix(), "band": "5g", "from": 14, "to": 17, "why": "distress", "scanning": 3, "left": 0, "ratio": []any{0.05, 0.1}},
		// Of a clock not yet set: not kept.
		step(-time.Hour, 17, 14, "above"),
	}
	report := func(steps []any) {
		t.Helper()
		code, _, cfg := f.apDo("GET", "/v1/ap/config", northToken, nil, nil)
		if code != 200 {
			t.Fatalf("its config: %d %v", code, cfg)
		}
		body := map[string]any{"version": cfg["version"], "uptime": 86400,
			"clients": []any{map[string]any{"mac": lonely, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "host": "iot-device"}},
			"rrm": map[string]any{
				"apc": []any{map[string]any{"radio": "radio0", "band": "5g", "power": 17, "ceiling": 23, "wanted": 3, "target": -70, "count": 2, "weakest": -66,
					"step": 3, "why": "distress", "ago": 2400, "clients": "guard", "client_floor": -75, "floor": 17, "hold": map[string]any{"power": 17, "left": 80000},
					"lonely":   []any{map[string]any{"mac": lonely, "signal": -82, "est": -80, "need": 22, "other": -88}},
					"distress": map[string]any{"at": now.Add(-40 * time.Minute).Unix(), "why": "scanning", "scanning": 3, "left": 0, "ratio": []any{0.05, 0.1}, "acted": true}}},
				"apc_events": steps,
			}}
		if code, _, b := f.apDo("POST", "/v1/ap/state", northToken, body, nil); code != 200 {
			t.Fatalf("its report: %d %v", code, b)
		}
	}
	report(steps)
	// Said again with the next report: kept once.
	report(steps)

	get := func(as, query string) map[string]any {
		t.Helper()
		code, v := f.do("GET", "/v1/power"+query, as, nil)
		if code != 200 {
			t.Fatalf("power%s as %s: %d %v", query, as, code, v)
		}
		return v
	}
	v := get("griff", "")
	// North and East, and the fixture's own office AP.
	aps := v["aps"].([]any)
	if len(aps) != 3 {
		t.Fatalf("aps: %v", aps)
	}
	byID := map[string]map[string]any{}
	for _, a := range aps {
		a := a.(map[string]any)
		byID[a["ap"].(string)] = a
	}
	n, e := byID[north], byID[east]
	if n["name"] != "North" || n["on"] != true || n["clients"] != "guard" || n["client_floor"] != float64(-75) || n["hold"] != float64(24) || n["reported"] == nil {
		t.Errorf("North: %v", n)
	}
	// East reported nothing yet, and sets nothing of its clients: the defaults.
	if e["name"] != "East" || e["on"] != true || e["clients"] != "ask" || e["client_floor"] != float64(-80) || len(e["radios"].([]any)) != 0 || e["reported"] != nil {
		t.Errorf("East: %v", e)
	}
	r := n["radios"].([]any)[0].(map[string]any)
	l := r["lonely"].([]any)[0].(map[string]any)
	if r["power"] != float64(17) || r["why"] != "distress" || r["floor"] != float64(17) || fmt.Sprint(r["hold"]) != "map[left:80000 power:17]" ||
		l["mac"] != lonely || l["host"] != "iot-device" || l["est"] != float64(-80) || l["other"] != float64(-88) ||
		r["distress"].(map[string]any)["acted"] != true {
		t.Errorf("North's radio: %v", r)
	}
	var said []string
	for _, s := range v["steps"].([]any) {
		s := s.(map[string]any)
		said = append(said, fmt.Sprintf("%s %v>%v %s", s["ap_name"], s["from"], s["to"], s["why"]))
	}
	if got, want := strings.Join(said, ", "), "North 14>17 distress, North 17>14 above, North 20>17 above"; got != want {
		t.Errorf("steps:\n got %s\nwant %s", got, want)
	}
	if got := len(get("griff", "?hours=1")["steps"].([]any)); got != 2 {
		t.Errorf("steps of the last hour: %d", got)
	}
	// One who may view the annex alone sees East alone, and no step.
	mine := get("tenant", "")
	if aps := mine["aps"].([]any); len(aps) != 1 || aps[0].(map[string]any)["ap"] != east || len(mine["steps"].([]any)) != 0 {
		t.Errorf("the tenant's: %v", mine)
	}
	if under := get("griff", "?under=annex"); len(under["aps"].([]any)) != 1 || len(under["steps"].([]any)) != 0 {
		t.Errorf("under the annex: %v", under)
	}
	for _, q := range []string{"?hours=0", "?hours=721", "?hours=many"} {
		if code, b := f.do("GET", "/v1/power"+q, "griff", nil); code != 400 {
			t.Errorf("%s: %d %v", q, code, b)
		}
	}
	if code, b := f.do("GET", "/v1/power", "", nil); code != 401 {
		t.Errorf("no token: %d %v", code, b)
	}
}
