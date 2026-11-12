package api

import (
	"strings"
	"testing"
	"time"
)

// How each maker's clients fare on each model of AP (0193): Espressif
// clients that come online again every 45 s on one model, and stay on
// another, are trouble on the first, said with where they stay; a private
// address is no maker's, and is no trouble; an AP's own test client is
// left out. Once they loop on the other model too, the looping is no
// model's, and no trouble, but said as such (0200).
func TestCompatibility(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("builtins: %d %v", code, b)
	}
	enroll := func(name, mac, model string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "model": model}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
			t.Fatalf("move: %d %v", code, b)
		}
		return ap, token
	}
	_, c360 := enroll("Living", "02:00:00:00:00:61", "Arista C-360")
	_, one := enroll("Office", "02:00:00:00:00:62", "OpenWrt One")
	// The hours asked are reckoned by the manager's own clock when asked, a
	// millisecond or so after now, and Windows' clock may not have moved on
	// from now by then: an attempt on the edge of the hours asked is in or
	// out by chance (2026-10-29: one an hour old was in the last hour), so
	// none is near one.
	now := time.Now()
	attempt := func(mac string, ago time.Duration) map[string]any {
		return map[string]any{"mac": mac, "bss": "phy1-ap2", "network": "iot", "ssid": "Home_Wi-Fi_IoT", "band": "2g",
			"started": now.Add(-ago).UnixMilli(), "took_ms": 3000, "outcome": "online", "stage": "internet", "reason": "", "events": []any{}}
	}
	post := func(token string, list []any) {
		t.Helper()
		if code, _, b := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": list}, nil); code != 200 || b["recorded"] != float64(len(list)) {
			t.Fatalf("connections: %d %v", code, b)
		}
	}
	// The robot, every 45 s on the C-360 for half an hour; another
	// Espressif client twice in six hours on the OpenWrt One; a phone with
	// a private address on both, twice each.
	var looping, settled []any
	for i := 0; i < 40; i++ {
		looping = append(looping, attempt("70:b8:f6:5a:25:c4", time.Duration(40-i)*45*time.Second))
	}
	looping = append(looping, attempt("92:8e:1b:c6:4c:2c", 3*time.Hour), attempt("92:8e:1b:c6:4c:2c", 2*time.Hour))
	// An Espressif client that came online again once, a minute later: one
	// rejoin, an hour's rate of one, and no loop.
	looping = append(looping, attempt("24:58:7c:4e:ed:dc", 5*time.Hour), attempt("24:58:7c:4e:ed:dc", 5*time.Hour-time.Minute))
	settled = append(settled, attempt("14:33:5c:5a:ab:c4", 8*time.Hour), attempt("14:33:5c:5a:ab:c4", 2*time.Hour),
		attempt("92:8e:1b:c6:4c:2c", 5*time.Hour), attempt("92:8e:1b:c6:4c:2c", 4*time.Hour))
	post(c360, looping)
	post(one, settled)

	code, body := f.do("GET", "/v1/compatibility", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	groups := body["groups"].([]any)
	first := groups[0].(map[string]any)
	if first["maker"] != "Espressif Inc." || first["model"] != "Arista C-360" || first["band"] != "2g" || first["looping"] != 1.0 || first["clients"] != 2.0 {
		t.Fatalf("the first group = %v", first)
	}
	for _, w := range first["worst"].([]any) {
		if w := w.(map[string]any); w["mac"] == "24:58:7c:4e:ed:dc" && w["per_hour"] != 1.0 {
			t.Errorf("one rejoin a minute later: %v", w)
		}
	}
	want := "1 of its 2 Espressif Inc. clients here came online again about every 45s, as one that drops the connection itself; on OpenWrt One (2.4 GHz) they stay"
	if first["trouble"] != want {
		t.Errorf("trouble = %q\nwant      %q", first["trouble"], want)
	}
	if w := first["worst"].([]any)[0].(map[string]any); w["mac"] != "70:b8:f6:5a:25:c4" || w["joins"] != 40.0 || w["ap_name"] != "Living" {
		t.Errorf("worst = %v", w)
	}
	var seen []string
	for _, g := range groups[1:] {
		m := g.(map[string]any)
		seen = append(seen, m["maker"].(string)+"/"+m["model"].(string))
		if m["trouble"] != nil {
			t.Errorf("trouble where none is: %v", m)
		}
	}
	if got := strings.Join(seen, " "); got != "Espressif Inc./OpenWrt One a private address/Arista C-360 a private address/OpenWrt One" {
		t.Errorf("the other groups: %s", got)
	}
	// Narrowed to an hour: the robot alone, with nowhere it is seen to stay.
	_, body = f.do("GET", "/v1/compatibility?hours=1", "griff", nil)
	if g := body["groups"].([]any); len(g) != 1 || strings.Contains(g[0].(map[string]any)["trouble"].(string), "stay") {
		t.Errorf("an hour's groups: %v", g)
	}

	// The robot on the OpenWrt One, every 45 s too, in the same half hour:
	// it loops wherever it is, as one whose trouble is beyond the APs.
	var there []any
	for i := 0; i < 30; i++ {
		there = append(there, attempt("70:b8:f6:5a:25:c4", time.Duration(30-i)*45*time.Second+20*time.Second))
	}
	post(one, there)
	_, body = f.do("GET", "/v1/compatibility", "griff", nil)
	groups = body["groups"].([]any)
	wants := map[string]string{
		"Arista C-360": "1 of its 2 Espressif Inc. clients here came online again about every 45s, as one that drops the connection itself, and they do so on OpenWrt One (2.4 GHz) too: the looping is not this model's. They loop wherever they are, which points at the clients themselves or the path beyond the APs, such as their cloud connection stalling",
		"OpenWrt One":  "1 of its 2 Espressif Inc. clients here came online again about every 45s, as one that drops the connection itself, and they do so on Arista C-360 (2.4 GHz) too: the looping is not this model's",
	}
	for i, g := range groups {
		m := g.(map[string]any)
		if m["trouble"] != nil {
			t.Errorf("trouble where the looping is no model's: %v", m)
		}
		if m["maker"] != "Espressif Inc." {
			continue
		}
		if got, _ := m["everywhere"].(string); i > 1 || !strings.HasPrefix(got, wants[m["model"].(string)]) {
			t.Errorf("group %d, on %s: everywhere = %q, want it to begin %q", i, m["model"], got, wants[m["model"].(string)])
		}
	}
}
