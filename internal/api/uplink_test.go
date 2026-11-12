package api

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
)

// An AP's uplink, a minute at a time (0351): sent with its radios' minutes,
// each port kept once and one not well said left out, and read back for
// its page by whoever may view it, a point a minute or several.
func TestUplinkMinutesKeptAndRead(t *testing.T) {
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
	code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": "02:00:00:00:00:3a", "hostname": "North",
		"radios": []any{map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE80"}}}}, nil)
	if code != 201 {
		t.Fatalf("enroll: %d %v", code, body)
	}
	ap, token := body["ap"].(string), body["token"].(string)
	change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"})

	now := time.Now()
	minute := func(ago int) int64 { return (now.Unix()/60 - int64(ago)) * 60 }
	port := func(name string, rx, rxMax float64) map[string]any {
		return map[string]any{"port": name, "up": true, "speed": 2500, "full": true, "secs": 60, "rx": rx, "tx": 10, "rx_max": rxMax, "tx_max": 20,
			"rx_bytes": int64(rx * 7500000), "tx_bytes": 75000000, "errors": 1, "changes": 0, "said_later": true}
	}
	said := []any{
		map[string]any{"at": minute(3), "radios": []any{}, "uplink": []any{port("lan1", 100, 300)}},
		map[string]any{"at": minute(2), "radios": []any{}, "uplink": []any{port("lan1", 200, 2400), port("lan 1; rm", 1, 1),
			map[string]any{"port": "lan2", "up": true, "speed": 2500, "secs": 60, "rx": -5}}},
		// An agent from before: no uplink.
		map[string]any{"at": minute(1), "radios": []any{}},
	}
	for range 2 {
		code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"minutes": said, "sent": float64(time.Now().UnixMilli())}, nil)
		if code != 200 {
			t.Fatalf("live: %d %v", code, b)
		}
	}
	read := func(as string) (int, map[string]any) {
		return f.do("GET", "/v1/aps/"+ap+"/uplink?hours=1", as, nil)
	}
	code, v := read("griff")
	if code != 200 {
		t.Fatalf("uplink: %d %v", code, v)
	}
	var got []string
	for _, p := range v["ports"].([]any) {
		p := p.(map[string]any)
		var pts []string
		for _, x := range p["points"].([]any) {
			x := x.(map[string]any)
			pts = append(pts, fmt.Sprintf("%v/%v", x["rx"], x["rx_max"]))
		}
		got = append(got, p["port"].(string)+": "+strings.Join(pts, " "))
	}
	if g := strings.Join(got, "; "); g != "lan1: 100/300 200/2400" {
		t.Errorf("ports: %s", g)
	}
	if code, _ := read("tenant"); code != 403 && code != 404 {
		t.Errorf("tenant, of another folder: %d", code)
	}
	if code, _ := f.do("GET", "/v1/aps/"+ap+"/uplink?hours=1000", "griff", nil); code != 400 {
		t.Errorf("too many hours: %d", code)
	}
}

// Over many minutes, a point stands for several: their average weighed by
// the seconds each counted, their busiest, and their errors and changes.
func TestUplinkPointsOf(t *testing.T) {
	at := time.Date(2026, 11, 11, 6, 0, 0, 0, time.UTC)
	f := func(v float64) *float64 { return &v }
	speed := 2500
	var ms []conditions.UplinkMinute
	for i := range 5 {
		ms = append(ms, conditions.UplinkMinute{At: at.Add(time.Duration(i) * time.Minute), Port: "lan1", Up: true, Speed: &speed, Secs: 60 - 30*(i%2),
			Rx: f(float64(100 * (i + 1))), Tx: f(10), RxMax: f(float64(200 * (i + 1))), TxMax: f(20), Errors: 1, Changes: int64(i % 2)})
	}
	pts := uplinkPointsOf(ms, 2)
	if len(pts) != 2 {
		t.Fatalf("points: %+v", pts)
	}
	// 100 over 60 s, 200 over 30, 300 over 60: 1500+... = (6000+6000+18000)/150 = 200.
	if p := pts[0]; p.Minutes != 3 || *p.Rx != 200 || *p.RxMax != 600 || *p.Tx != 10 || p.Errors != 3 || p.Changes != 1 || !p.At.Equal(at) {
		t.Errorf("first: %+v rx %v max %v", p, *p.Rx, *p.RxMax)
	}
	if p := pts[1]; p.Minutes != 2 || *p.RxMax != 1000 || !p.At.Equal(at.Add(3*time.Minute)) {
		t.Errorf("second: %+v", p)
	}
	if pts := uplinkPointsOf(nil, 720); len(pts) != 0 {
		t.Errorf("none: %+v", pts)
	}
}
