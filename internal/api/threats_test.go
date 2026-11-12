package api

import (
	"strings"
	"testing"
	"time"
)

// GET /v1/threats (0160): what each network the APs hear is, by the scan
// radio's wire check; the clients heard on the rogues and suspects, once
// the manager has asked after them; and the networks two DHCP servers
// answer on.
func TestThreats(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	wired, off := true, false
	report := map[string]any{"version": version, "uptime": 60, "bssids": []any{"02:00:00:00:00:01"},
		"rrm": map[string]any{"address": "192.168.1.38", "advertised": []any{}, "neighbours": []any{}, "ratings": []any{}, "moves": []any{},
			"others": []any{
				// One of Aeolus's SSIDs, only RRM hears it: a suspect, unchecked.
				map[string]any{"bssid": "e4:d1:24:08:6d:12", "ssid": "Home Wi-Fi", "band": "2g", "channel": 6, "signal": -80, "ago": 20},
			}},
		"airscan": map[string]any{"bss": []any{
			// Another name, on our wire on VLAN 10: a rogue for certain.
			map[string]any{"bssid": "a8:42:a1:00:00:07", "ssid": "Free WiFi", "band": "2g", "channel": 1, "signal": -55, "wired": wired, "vlans": []int{10}, "looks": 40, "ago": 5},
			// One of Aeolus's SSIDs, looked for and not on the wire: a suspect.
			map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "Home Wi-Fi", "band": "2g", "channel": 11, "signal": -70, "wired": off, "looks": 10409, "ago": 7},
			// The AP's own address, but for its first and last bytes, on the
			// wire: its own, renumbered.
			map[string]any{"bssid": "3e:ff:4d:02:26:c0", "ssid": "Home Wi-Fi", "band": "5g", "channel": 36, "signal": -15, "wired": wired, "vlans": []int{20}, "looks": 9, "ago": 3},
			// Another name, off the wire: nothing.
			map[string]any{"bssid": "70:4f:57:13:4e:f4", "ssid": "TP-LINK_Smart Plug_03F4", "band": "2g", "channel": 6, "signal": -84, "wired": off, "looks": 9, "ago": 3},
		}},
		"dhcp": map[string]any{"sweet": map[string]any{"servers": []any{
			map[string]any{"id": "192.168.20.254", "mac": "28:e7:1d:d2:d0:13", "answers": 40, "ago": 30},
			map[string]any{"id": "192.168.20.99", "mac": "f2:0d:e6:3f:00:32", "answers": 2, "ago": 300}}}},
		// A client the AP serves, as the APs have served it.
		"clients": []any{map[string]any{"mac": "34:b7:da:64:a6:58", "network": "sweet", "ssid": "Home Wi-Fi", "band": "2g", "signal": -60, "signal_avg": -61,
			"rx_rate": 72.2, "rx_mcs": 7, "rx_nss": nil, "tx_rate": 72.2, "tx_mcs": 7, "tx_nss": nil, "rx_bytes": 1000, "tx_bytes": 1000,
			"rx_packets": 10, "tx_packets": 10, "tx_retries": 0, "tx_failed": 0, "connected": 60, "inactive_ms": 40,
			"address": "192.168.20.61", "host": "kitchen-plug", "dhcp": "ok"}},
	}
	post := func() map[string]any {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil)
		if code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
		return body
	}
	// The manager asks after the rogue and the two suspects, and no other.
	body := post()
	if got := strings.Join(strs(body["watch"]), " "); got != "a8:42:a1:00:00:07 e4:d1:24:08:6d:11 e4:d1:24:08:6d:12" {
		t.Fatalf("watch = %s", got)
	}
	// The next report lists the clients the scan radio hears with them, and
	// one with another network, which is not kept.
	report["airscan"].(map[string]any)["clients"] = []any{
		map[string]any{"mac": "34:b7:da:64:a6:58", "bssid": "e4:d1:24:08:6d:11", "frames": 231026, "ago": 4},
		map[string]any{"mac": "f0:ad:4e:46:a9:1f", "bssid": "e4:d1:24:08:6d:11", "frames": 31946, "ago": 2},
		map[string]any{"mac": "78:e1:03:b0:0d:ba", "bssid": "a8:42:a1:00:00:07", "frames": 552, "ago": 1},
		map[string]any{"mac": "aa:bb:cc:dd:ee:09", "bssid": "70:4f:57:13:4e:f4", "frames": 900, "ago": 1},
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.blocked", "value": []any{"F0:AD:4E:46:A9:1F"}}); code != 200 {
		t.Fatalf("blocked: %d %v", code, b)
	}
	post()

	threats := func(who string) map[string]any {
		t.Helper()
		code, body := f.do("GET", "/v1/threats?under=office", who, nil)
		if code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		return body
	}
	body = threats("griff")
	var kinds []string
	for _, x := range body["rogues"].([]any) {
		m := x.(map[string]any)
		w := m["wire"].(map[string]any)
		kinds = append(kinds, m["bssid"].(string)[12:]+"="+m["kind"].(string)+"/"+w["state"].(string))
	}
	if got := strings.Join(kinds, " "); got != "00:07=rogue/on 6d:11=suspect/off 6d:12=suspect/unchecked" {
		t.Fatalf("rogues = %s", got)
	}
	first := body["rogues"].([]any)[0].(map[string]any)
	if w := first["wire"].(map[string]any); w["vlans"].([]any)[0] != 10.0 || w["looks"] != 40.0 || first["clients"] != 1.0 {
		t.Fatalf("rogue = %v", first)
	}
	// Who made each, by its BSSID.
	if second := body["rogues"].([]any)[1].(map[string]any); second["maker"] != "Mojo Networks (Arista)" || second["maker_derived"] != nil {
		t.Fatalf("the suspect's maker = %v %v", second["maker"], second["maker_derived"])
	}
	var clients []string
	for _, x := range body["clients"].([]any) {
		m := x.(map[string]any)
		s := m["mac"].(string)[12:] + "@" + m["bssid"].(string)[12:] + "=" + m["kind"].(string)
		if m["ours"] == true {
			s += " ours " + m["host"].(string)
		}
		if b, _ := m["blocked"].([]any); len(b) > 0 {
			s += " blocked on " + b[0].(string)
		}
		clients = append(clients, s)
	}
	if got := strings.Join(clients, ", "); got != "0d:ba@00:07=rogue, a9:1f@6d:11=suspect blocked on sweet, a6:58@6d:11=suspect ours kitchen-plug" {
		t.Fatalf("clients = %s", got)
	}
	d := body["dhcp"].([]any)
	if len(d) != 1 || d[0].(map[string]any)["network"] != "sweet" || d[0].(map[string]any)["ssid"] != "Home Wi-Fi" || len(d[0].(map[string]any)["servers"].([]any)) != 2 {
		t.Fatalf("dhcp = %v", d)
	}
	if c := body["counts"].(map[string]any); c["rogue"] != 1.0 || c["suspect"] != 2.0 || c["known"] != 0.0 || c["clients"] != 3.0 || c["dhcp"] != 1.0 {
		t.Fatalf("counts = %v", c)
	}

	// The alerts: the rogue critical, the suspects warnings, two DHCP
	// servers a warning; the AP's own renumbered BSSID none.
	_, al := f.do("GET", "/v1/alerts", "griff", nil)
	var as []string
	for _, a := range al["alerts"].([]any) {
		m := a.(map[string]any)
		if k := m["kind"].(string); strings.HasPrefix(k, "rogue") || k == "dhcp-servers" {
			as = append(as, m["severity"].(string)+":"+m["key"].(string))
		}
	}
	if got := strings.Join(as, " "); got != "critical:rogue-wired:a8:42:a1:00:00:07 warning:dhcp-servers:sweet warning:rogue:e4:d1:24:08:6d:12 warning:rogue:e4:d1:24:08:6d:11" {
		t.Fatalf("alerts = %s", got)
	}

	// The network says the servers it has (0160): with one of the two, the
	// other is still listed, marked as no server of its own; with both,
	// neither is, and no alert.
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.dhcp_servers", "value": []string{"192.168.20.254"}}); code != 200 {
		t.Fatalf("dhcp_servers: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/threats", "griff", nil)
	d = body["dhcp"].([]any)
	if len(d) != 1 {
		t.Fatalf("one of its own: dhcp = %v", d)
	}
	for _, s := range d[0].(map[string]any)["servers"].([]any) {
		m := s.(map[string]any)
		if (m["id"] == "192.168.20.254") != (m["known"] == true) {
			t.Fatalf("one of its own: server %v", m)
		}
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.dhcp_servers", "value": []string{"192.168.20.254", "F2:0D:E6:3F:00:32"}}); code != 200 {
		t.Fatalf("dhcp_servers: %d %v", code, b)
	}
	_, body = f.do("GET", "/v1/threats", "griff", nil)
	if d := body["dhcp"].([]any); len(d) != 0 {
		t.Fatalf("both its own: dhcp = %v", d)
	}
	_, al = f.do("GET", "/v1/alerts", "griff", nil)
	for _, a := range al["alerts"].([]any) {
		if a.(map[string]any)["kind"] == "dhcp-servers" {
			t.Fatalf("both its own, an alert: %v", a)
		}
	}

	// Known, the rogue on the wire is listed as known and raises no alert.
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "rogues.known", "value": []any{"A8:42:A1:00:00:07"}}); code != 200 {
		t.Fatalf("known: %d %v", code, b)
	}
	body = threats("griff")
	if last := body["rogues"].([]any)[2].(map[string]any); last["bssid"] != "a8:42:a1:00:00:07" || last["kind"] != "known" || last["wire"].(map[string]any)["state"] != "on" {
		t.Fatalf("known = %v", last)
	}
	// office has no role in Locations: nothing.
	if body := threats("office"); len(body["rogues"].([]any)) != 0 || len(body["clients"].([]any)) != 0 {
		t.Fatalf("office sees %v", body)
	}
	if code, _ := f.do("GET", "/v1/threats?hours=0", "griff", nil); code != 400 {
		t.Fatalf("hours=0: %d", code)
	}
}

// An AP silent for three reports' time hears nothing now: what it last
// heard is not shown, as its alerts are not; once it reports again, it is.
func TestThreatsOfASilentAP(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	report := map[string]any{"version": version, "uptime": 60,
		"rrm": map[string]any{"address": "192.168.1.38", "advertised": []any{}, "neighbours": []any{}, "ratings": []any{}, "moves": []any{},
			"others": []any{map[string]any{"bssid": "e4:d1:24:08:6d:12", "ssid": "Home Wi-Fi", "band": "2g", "channel": 6, "signal": -80, "ago": 20}}}}
	rogues := func() int {
		t.Helper()
		_, body := f.do("GET", "/v1/threats", "griff", nil)
		return len(body["rogues"].([]any))
	}
	*f.skew = -20 * time.Minute
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	if n := rogues(); n != 0 {
		t.Fatalf("an AP silent 20 minutes shows %d", n)
	}
	*f.skew = 0
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	if n := rogues(); n != 1 {
		t.Fatalf("reporting again, it shows %d", n)
	}
}

// A DHCP server heard with no address of its own is the server of its MAC
// (2026-10-25): the tower switch, heard as 192.168.1.254 and as
// 255.255.255.255, is one server, no rogue; beside another, two.
func TestThreatsDHCPNoAddress(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	const tower = "fc:bd:67:a6:67:97"
	servers := []any{map[string]any{"id": "192.168.1.254", "mac": tower, "answers": 30, "ago": 30},
		map[string]any{"id": "255.255.255.255", "mac": tower, "answers": 3, "ago": 10}}
	dhcp := func() []any {
		t.Helper()
		report := map[string]any{"version": version, "uptime": 60, "dhcp": map[string]any{"sweet": map[string]any{"servers": servers}}}
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
		_, body := f.do("GET", "/v1/threats", "griff", nil)
		return body["dhcp"].([]any)
	}
	if d := dhcp(); len(d) != 0 {
		t.Fatalf("one server: %v", d)
	}
	servers = append(servers, map[string]any{"id": "192.168.10.254", "mac": "aa:bb:cc:dd:ee:ff", "answers": 5, "ago": 60})
	d := dhcp()
	if len(d) != 1 {
		t.Fatalf("two servers: %v", d)
	}
	got := d[0].(map[string]any)["servers"].([]any)
	if first := got[0].(map[string]any); len(got) != 2 || first["id"] != "192.168.1.254" || first["answers"] != 33.0 {
		t.Fatalf("two servers, the tower's answers added up: %v", got)
	}
}

// A report's scan radio findings are held to what airscan gives.
func TestThreatsReportChecked(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	for name, a := range map[string]any{
		"bssid":  map[string]any{"bss": []any{map[string]any{"bssid": "E4:D1:24:08:6D:11", "ssid": "x", "band": "2g", "channel": 11, "signal": -70, "looks": 1, "ago": 1}}},
		"band":   map[string]any{"bss": []any{map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "x", "band": "60g", "channel": 11, "signal": -70, "looks": 1, "ago": 1}}},
		"vlan":   map[string]any{"bss": []any{map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "x", "band": "2g", "channel": 11, "signal": -70, "wired": true, "vlans": []int{5000}, "looks": 1, "ago": 1}}},
		"ssid":   map[string]any{"bss": []any{map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "Café", "band": "2g", "channel": 11, "signal": -70, "looks": 1, "ago": 1}}},
		"client": map[string]any{"bss": []any{}, "clients": []any{map[string]any{"mac": "phone", "bssid": "e4:d1:24:08:6d:11", "frames": 1, "ago": 1}}},
		"frames": map[string]any{"bss": []any{}, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:01", "bssid": "e4:d1:24:08:6d:11", "frames": -1, "ago": 1}}},
		"too many": map[string]any{"bss": func() []any {
			var out []any
			for i := 0; i < 65; i++ {
				out = append(out, map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "x", "band": "2g", "channel": 11, "signal": -70, "looks": 1, "ago": 1})
			}
			return out
		}()},
	} {
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "airscan": a}, nil); code != 400 {
			t.Errorf("%s: %d %v", name, code, body)
		}
	}
	// A hidden name is a network all the same.
	ok := map[string]any{"bss": []any{map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "", "band": "6g", "channel": 149, "signal": -91, "wired": nil, "looks": 0, "ago": 1}}}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "airscan": ok}, nil); code != 200 {
		t.Errorf("hidden: %d %v", code, body)
	}
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
