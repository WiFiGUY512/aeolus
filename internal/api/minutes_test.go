package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// What each minute came to (0207): an AP sends its minutes, and its news,
// on its line; the manager keeps each minute once, leaves out what is not
// well said, and reads them back by AP and radio, on one band, and for one
// client, of the APs the caller may view. Power control's steps sent as
// news are kept with those of the reports.
func TestMinutes(t *testing.T) {
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
	enroll := func(name, mac, folder string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name,
			"radios": []any{map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE80"}}}}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder})
		return ap, token
	}
	north, northToken := enroll("North", "02:00:00:00:00:3a", "office")
	_, eastToken := enroll("East", "02:00:00:00:00:3c", "annex")

	now := time.Now()
	minute := func(ago int) int64 { return (now.Unix()/60 - int64(ago)) * 60 }
	radio := func(name, band string, busy float64) map[string]any {
		return map[string]any{"radio": name, "band": band, "channel": 36, "width": 80, "power": 17, "busy": busy, "busy_max": busy + 20, "rx": 5.5,
			"tx": 3.0, "noise": -95, "clients": 1.5, "clients_max": 2, "tx_packets": 1000, "tx_retries": 50, "tx_failed": 2, "rx_packets": 900,
			"tx_bytes": 123456, "rx_bytes": 65432, "said_later": true}
	}
	const phone = "aa:bb:cc:00:00:01"
	client := func(signal int) map[string]any {
		return map[string]any{"mac": phone, "band": "5g", "ssid": "Lab", "signal": signal, "signal_min": signal - 4, "tx_rate": 866.7, "rx_rate": 130,
			"tx_packets": 100, "tx_retries": 3, "tx_failed": 0, "rx_packets": 90, "tx_bytes": 5000, "rx_bytes": 2000, "secs": 60}
	}
	send := func(token string, body map[string]any) map[string]any {
		t.Helper()
		body["sent"] = float64(time.Now().UnixMilli())
		code, _, b := f.apDo("POST", "/v1/ap/live", token, body, nil)
		if code != 200 {
			t.Fatalf("live: %d %v", code, b)
		}
		return b
	}
	said := []any{
		map[string]any{"at": minute(3), "radios": []any{radio("radio0", "5g", 20), radio("radio1", "2g", 60)}, "clients": []any{client(-60)}},
		map[string]any{"at": minute(2), "radios": []any{radio("radio0", "5g", 30), radio("radio 1; rm", "2g", 60), radio("radio2", "4g", 1)},
			"clients": []any{client(-62), map[string]any{"mac": "nobody", "band": "5g"}}},
		// Not on the minute, from two days ago, and yet to come: left out.
		map[string]any{"at": minute(1) + 7, "radios": []any{radio("radio0", "5g", 99)}},
		map[string]any{"at": minute(48 * 60), "radios": []any{radio("radio0", "5g", 99)}},
		map[string]any{"at": minute(-60), "radios": []any{radio("radio0", "5g", 99)}},
	}
	if b := send(northToken, map[string]any{"minutes": said}); b["minutes"] != float64(3) {
		t.Errorf("minutes kept: %v", b["minutes"])
	}
	// Sent again, as after an answer that was lost: kept once. With power
	// control's step as news.
	step := map[string]any{"t": "news", "kind": "apc", "at": now.Unix() - 30, "band": "5g", "from": 20, "to": 17, "why": "above", "scanning": nil, "left": nil, "ratio": nil}
	send(northToken, map[string]any{"minutes": said[:1], "news": []any{step, map[string]any{"t": "news", "kind": "weather", "rain": true}}})
	send(eastToken, map[string]any{"minutes": []any{map[string]any{"at": minute(1), "radios": []any{radio("radio0", "5g", 10)}, "clients": []any{client(-75)}}}})

	get := func(as, query string) map[string]any {
		t.Helper()
		code, v := f.do("GET", "/v1/minutes"+query, as, nil)
		if code != 200 {
			t.Fatalf("minutes%s as %s: %d %v", query, as, code, v)
		}
		return v
	}
	series := func(v map[string]any) string {
		var out []string
		for _, r := range v["radios"].([]any) {
			r := r.(map[string]any)
			var busy []string
			for _, m := range r["minutes"].([]any) {
				busy = append(busy, fmt.Sprint(m.(map[string]any)["busy"]))
			}
			out = append(out, fmt.Sprintf("%s %s %s: %s", r["ap_name"], r["radio"], r["band"], strings.Join(busy, " ")))
		}
		return strings.Join(out, "; ")
	}
	v := get("griff", "?hours=24")
	if got, want := series(v), "North radio0 5g: 20 30; North radio1 2g: 60; East radio0 5g: 10"; got != want {
		t.Errorf("radios:\n got %s\nwant %s", got, want)
	}
	// By AP ID, North's first; then by radio, and the oldest first.
	m := v["radios"].([]any)[0].(map[string]any)["minutes"].([]any)[1].(map[string]any)
	if m["busy_max"] != float64(50) || m["noise"] != float64(-95) || m["power"] != float64(17) || m["tx_retries"] != float64(50) || m["rx_bytes"] != float64(65432) || m["clients"] != 1.5 {
		t.Errorf("a minute: %v", m)
	}
	if at, err := time.Parse(time.RFC3339Nano, m["at"].(string)); err != nil || at.Unix() != minute(2) {
		t.Errorf("its time: %v %v", m["at"], err)
	}
	if got := series(get("griff", "?hours=24&band=2g")); got != "North radio1 2g: 60" {
		t.Errorf("2.4 GHz: %s", got)
	}
	// One client's minutes, on both APs, with no radios.
	c := get("griff", "?hours=24&mac=AA:BB:CC:00:00:01")
	if _, ok := c["radios"]; ok {
		t.Errorf("a client's minutes come alone: %v", c)
	}
	var sig []string
	for _, x := range c["client"].(map[string]any)["minutes"].([]any) {
		x := x.(map[string]any)
		sig = append(sig, fmt.Sprintf("%s %v", x["ap_name"], x["signal"]))
	}
	if got := strings.Join(sig, ", "); got != "North -60, North -62, East -75" {
		t.Errorf("the client: %s", got)
	}
	// The tenant sees the annex alone.
	if got := series(get("tenant", "?hours=24")); got != "East radio0 5g: 10" {
		t.Errorf("the tenant's: %s", got)
	}
	if n := len(get("tenant", "?hours=24&mac="+phone)["client"].(map[string]any)["minutes"].([]any)); n != 1 {
		t.Errorf("the tenant's client: %d minutes", n)
	}
	// Within the hour, the minute of three ago and of two; under the
	// office, North alone.
	if got := series(get("griff", "?under=office")); got != "North radio0 5g: 20 30; North radio1 2g: 60" {
		t.Errorf("the office's hour: %s", got)
	}
	// The step sent as news is power control's.
	p := get2(t, f, "/v1/power")
	if steps := p["steps"].([]any); len(steps) != 1 || steps[0].(map[string]any)["why"] != "above" || steps[0].(map[string]any)["ap"] != north {
		t.Errorf("the step sent as news: %v", p["steps"])
	}
	for _, q := range []string{"?hours=25", "?hours=0", "?band=4g", "?mac=nobody", "?mac=" + phone + "&hours=169"} {
		if code, b := f.do("GET", "/v1/minutes"+q, "griff", nil); code != 400 {
			t.Errorf("%s: %d %v", q, code, b)
		}
	}
	if code, _, b := f.apDo("POST", "/v1/ap/live", northToken, map[string]any{"sent": 1.0, "minutes": make([]any, 11)}, nil); code != 400 {
		t.Errorf("eleven minutes at once: %d %v", code, b)
	}
}

// get2 reads a path as griff.
func get2(t *testing.T, f *fixture, path string) map[string]any {
	t.Helper()
	code, v := f.do("GET", path, "griff", nil)
	if code != 200 {
		t.Fatalf("%s: %d %v", path, code, v)
	}
	return v
}
