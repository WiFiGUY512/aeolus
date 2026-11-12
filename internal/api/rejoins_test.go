package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A client that comes online at an AP again and again (0190): an alert on
// that AP, by the name it gives itself, once it has 20 times in the last
// hour; and an issue in its journey, which takes back "ping-pong". One that
// came back a few times is neither.
func TestRejoins(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	const robot, phone = "70:b8:f6:5a:25:c4", "02:00:00:00:00:41"
	client := func(m, host string) map[string]any {
		return map[string]any{"mac": m, "host": host, "network": "sweet", "ssid": "Home Wi-Fi", "band": "2g", "signal": -47, "signal_avg": -47,
			"tx_rate": 65, "rx_rate": 65, "connected": 30, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1}
	}
	if code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
		"clients": []any{client(robot, "IoT device4"), client(phone, "")}}, nil); code != 200 {
		t.Fatalf("state: %d", code)
	}
	now := time.Now()
	var list []any
	attempt := func(mac string, ago time.Duration) {
		list = append(list, map[string]any{"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Home Wi-Fi", "band": "2g",
			"started": now.Add(-ago).UnixMilli(), "took_ms": 3000, "outcome": "online", "stage": "internet", "reason": "", "events": []any{}})
	}
	// The robot, every 45 s for 30 times; the phone, 5 times.
	for i := 0; i < 30; i++ {
		attempt(robot, time.Duration(30-i)*45*time.Second)
	}
	for i := 0; i < 5; i++ {
		attempt(phone, time.Duration(5-i)*10*time.Minute)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": list}, nil); code != 200 || body["recorded"] != float64(len(list)) {
		t.Fatalf("connections: %d %v", code, body)
	}

	_, body := f.do("GET", "/v1/alerts", "griff", nil)
	var said []string
	for _, a := range body["alerts"].([]any) {
		m := a.(map[string]any)
		if m["kind"] == "rejoins" {
			said = append(said, fmt.Sprintf("%s %s %s", m["ap"], m["key"], m["message"]))
		}
	}
	// On no other AP in the hour: nothing yet says whose it is (0200).
	want := ap + " rejoins:" + robot + " IoT device4 (" + robot + ") came online here 30 times in the last hour, about every 45s: it drops its connection itself and joins again. Nothing on another AP tells yet whether this AP's radio is the cause"
	if len(said) != 1 || !strings.HasPrefix(said[0], want) {
		t.Fatalf("rejoins alerts: %q, want one beginning %q", said, want)
	}

	issues := func(mac string) map[string]string {
		t.Helper()
		code, j := f.do("GET", "/v1/clients/"+mac, "griff", nil)
		if code != 200 {
			t.Fatalf("journey: %d %v", code, j)
		}
		out := map[string]string{}
		for _, i := range j["issues"].([]any) {
			m := i.(map[string]any)
			out[m["kind"].(string)] = m["message"].(string)
		}
		return out
	}
	if got := issues(robot)["rejoins"]; !strings.HasPrefix(got, "it came online on ShedAP 30 times within an hour, about every 45s") {
		t.Errorf("the robot's journey says %q", got)
	}
	if got := issues(phone); got["rejoins"] != "" {
		t.Errorf("the phone's journey says %v", got)
	}
}

// Where else a client that comes back again and again did so in the hour
// (0200): one that did on two APs drops its connection itself wherever it
// is, and each AP's alert says that it is the client or what is beyond the
// APs, not this AP's radio, as was so of the IoT devices; one that came
// back on one AP and then stayed on the other blames the first one's radio.
// Its journey says the same.
func TestRejoinsElsewhere(t *testing.T) {
	f := newFixture(t)
	ph, phToken, _ := f.adopted()
	code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": "02:00:00:00:00:61", "hostname": "Living", "model": "Arista C-360"}, nil)
	if code != 201 {
		t.Fatalf("enroll: %d %v", code, body)
	}
	living, livingToken := body["ap"].(string), body["token"].(string)
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": living, "parent": "office"}); code != 200 {
		t.Fatalf("move: %d %v", code, b)
	}
	now := time.Now()
	attempt := func(mac string, ago time.Duration) map[string]any {
		return map[string]any{"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Home Wi-Fi", "band": "2g",
			"started": now.Add(-ago).UnixMilli(), "took_ms": 3000, "outcome": "online", "stage": "internet", "reason": "", "events": []any{}}
	}
	every45 := func(mac string, from time.Duration, n int) []any {
		var out []any
		for i := 0; i < n; i++ {
			out = append(out, attempt(mac, from-time.Duration(i)*45*time.Second))
		}
		return out
	}
	post := func(token string, list []any) {
		t.Helper()
		if code, _, b := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": list}, nil); code != 200 || b["recorded"] != float64(len(list)) {
			t.Fatalf("connections: %d %v", code, b)
		}
	}
	// The robot, 30 times on ShedAP from 50 minutes ago, then 25 on
	// Living from 20 minutes ago. The other, 25 times on ShedAP from 55
	// minutes ago, then on Living once, half an hour ago, and nowhere since.
	const robot, other = "70:b8:f6:5a:25:c4", "14:33:5c:5a:ab:c4"
	post(phToken, append(every45(robot, 50*time.Minute, 30), every45(other, 55*time.Minute, 25)...))
	post(livingToken, append(every45(robot, 20*time.Minute, 25), attempt(other, 30*time.Minute)))
	// An ESP32 heard at -79 dBm on ShedAP, 25 times from 55 minutes
	// ago, that stayed on Living, heard there at -62, half an hour ago
	// (0312): the weaker link, not the radio.
	const esp = "14:63:93:66:a7:98"
	at := func(list []any, signal int) []any {
		for _, a := range list {
			a.(map[string]any)["signal"] = signal
		}
		return list
	}
	post(phToken, at(every45(esp, 55*time.Minute, 25), -79))
	post(livingToken, at([]any{attempt(esp, 30*time.Minute)}, -62))

	_, body = f.do("GET", "/v1/alerts", "griff", nil)
	said := map[string]string{}
	for _, a := range body["alerts"].([]any) {
		m := a.(map[string]any)
		if m["kind"] == "rejoins" {
			said[m["ap"].(string)+" "+m["key"].(string)] = m["message"].(string)
		}
	}
	wants := map[string]string{
		ph + " rejoins:" + robot: robot + " came online here 30 times in the last hour, about every 45s, and did the same on Living: it drops its connection itself on every AP it joins, so it is not this AP's radio. " +
			"It is the client itself, or something beyond the APs: the network or internet path its own traffic takes, as when its cloud connection stalls. Look there, not at the radio",
		living + " rejoins:" + robot: robot + " came online here 25 times in the last hour, about every 45s, and did the same on ShedAP: it drops its connection itself on every AP it joins, so it is not this AP's radio",
		ph + " rejoins:" + other:     other + " came online here 25 times in the last hour, about every 45s: it drops its connection itself and joins again. On Living it stays, so something of this AP's radio does not suit it",
		ph + " rejoins:" + esp: esp + " came online here 25 times in the last hour, about every 45s: it drops its connection itself and joins again. On Living it stays, heard at -62 dBm there against -79 dBm here: " +
			"a link 17 dB weaker is the likelier cause, not this AP's radio",
	}
	if len(said) != len(wants) {
		t.Errorf("rejoins alerts: %q", said)
	}
	for k, want := range wants {
		if !strings.HasPrefix(said[k], want) {
			t.Errorf("%s says %q, want it to begin %q", k, said[k], want)
		}
	}

	// The robot's journey: on each AP, the client or beyond, not the radio.
	code, j := f.do("GET", "/v1/clients/"+robot, "griff", nil)
	if code != 200 {
		t.Fatalf("journey: %d %v", code, j)
	}
	var issues []string
	for _, i := range j["issues"].([]any) {
		if m := i.(map[string]any); m["kind"] == "rejoins" {
			issues = append(issues, m["message"].(string))
		}
	}
	if len(issues) != 2 {
		t.Fatalf("the robot's journey says %q", issues)
	}
	for _, i := range issues {
		if !strings.Contains(i, "it drops its connection itself on every AP it joins, so it is not that AP's radio") {
			t.Errorf("the robot's journey says %q", i)
		}
	}
}
