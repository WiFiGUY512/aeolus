package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// goodUCI carries the config an AP adopted into House › Office gets from the
// fixture: Home Wi-Fi on every radio, over VLAN 20, and 40 MHz on 5 GHz.
const goodUCI = `package wireless

config wifi-device 'radio0'
	option band '2g'

config wifi-device 'radio1'
	option band '5g'
	option htmode 'VHT40'
	option noscan '1'

config wifi-iface 'aeolus_sweet_radio0'
	option device 'radio0'
	option mode 'ap'
	option rnr '1'
	option ssid 'Home Wi-Fi'
	option encryption 'psk2'
	option key '` + passphrase + `'
	option network 'aeolus_sweet'

config wifi-iface 'aeolus_sweet_radio1'
	option device 'radio1'
	option mode 'ap'
	option rnr '1'
	option ssid 'Home Wi-Fi'
	option encryption 'psk2'
	option key '` + passphrase + `'
	option network 'aeolus_sweet'

package network

config interface 'aeolus_sweet'
	option proto 'none'
	option device 'lan.20'

config device
	option type '8021q'
	option ifname 'lan'
	option vid '20'
	option name 'lan.20'

package aeolus

config agent 'agent'
	option poll '60'

package system

config system
	option hostname 'ShedAP'
`

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// adopted enrolls the shed AP, moves it into House › Office and polls
// once, returning its ID, token and version.
func (f *fixture) adopted() (string, string, float64) {
	f.t.Helper()
	ap, token := f.enrolled()
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "office"}); code != 200 {
		f.t.Fatalf("adopt: %d %v", code, b)
	}
	code, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code != 200 || body["state"] != "ready" {
		f.t.Fatalf("poll: %d %v", code, body)
	}
	return ap, token, body["version"].(float64)
}

// A ready config says failed where the AP's last apply report was of that
// version and did not apply (0235), as after a sysupgrade onto an image
// without vxlan, where it runs the version from before: the agent then
// checks and says applied what renders to what runs. One that did apply, or
// a failure of another version, says nothing.
func TestPollSaysFailed(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	poll := func() map[string]any {
		t.Helper()
		code, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code != 200 || body["state"] != "ready" {
			t.Fatalf("poll: %d %v", code, body)
		}
		return body
	}
	if b := poll(); b["failed"] != nil {
		t.Errorf("nothing applied yet, failed: %v", b["failed"])
	}
	applied := func(v float64, ok bool) {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": v, "hash": sha(goodUCI), "ok": ok, "error": map[bool]string{false: "this AP cannot render it"}[ok]}, nil); code != 200 {
			t.Fatalf("applied: %d %v", code, body)
		}
	}
	applied(version-1, false)
	if b := poll(); b["failed"] != nil {
		t.Errorf("another version's failure says this one failed")
	}
	applied(version, false)
	if b := poll(); b["failed"] != true {
		t.Errorf("this version's failure not said: %v", b)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/config", token, nil, map[string]string{"If-None-Match": fmt.Sprintf(`"%d"`, int64(version))}); code != 304 {
		t.Errorf("asked by its version: %d, not 304", code)
	}
	applied(version, true)
	if b := poll(); b["failed"] != nil {
		t.Errorf("applied since, still failed")
	}
}

func (f *fixture) render(token string, version float64, text string) (int, map[string]any) {
	f.t.Helper()
	code, _, body := f.apDo("POST", "/v1/ap/render", token, map[string]any{"version": version, "uci": text}, nil)
	return code, body
}

func TestLandingZoneAPsCanOnlyPoll(t *testing.T) {
	f := newFixture(t)
	ap, token := f.enrolled()
	if code, body := f.render(token, 1, goodUCI); code != 409 {
		t.Fatalf("render from Landing Zone: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": 1, "hash": sha(goodUCI), "ok": true}, nil); code != 409 {
		t.Fatalf("applied from Landing Zone: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": 0, "uptime": 5}, nil); code != 409 {
		t.Fatalf("state from Landing Zone: %d %v", code, body)
	}
	_, hist := f.do("GET", "/v1/aps/"+ap+"/history", "griff", nil)
	if len(hist["checks"].([]any))+len(hist["applies"].([]any))+len(hist["states"].([]any)) != 0 {
		t.Fatalf("Landing Zone AP left records: %v", hist)
	}
	// It was seen, though.
	_, view := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	cond := view["condition"].(map[string]any)
	if seen, _ := cond["seen"].(map[string]any); seen == nil || seen["source"] != "127.0.0.1" || cond["in_sync"] != nil {
		t.Fatalf("condition = %v", cond)
	}
}

func TestRenderCheck(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()

	if code, body := f.render(token, version-1, goodUCI); code != 200 || body["result"] != "stale" || body["version"] != version {
		t.Fatalf("old version: %d %v", code, body)
	}
	code, body := f.render(token, version, strings.Replace(goodUCI, "option ssid 'Home Wi-Fi'", "option ssid 'Home Wi-Fi 2'", 1))
	if code != 200 || body["result"] != "refused" || !strings.Contains(jsonText(body["problems"]), "Home Wi-Fi 2") {
		t.Fatalf("wrong SSID: %d %v", code, body)
	}
	if _, body := f.render(token, version, "package wireless\nconfig wifi-iface 'x"); body["result"] != "refused" || !strings.Contains(jsonText(body["problems"]), "does not parse") {
		t.Fatalf("broken UCI: %v", body)
	}
	code, body = f.render(token, version, goodUCI)
	if code != 200 || body["result"] != "ok" || body["hash"] != sha(goodUCI) || len(body["problems"].([]any)) != 0 {
		t.Fatalf("good UCI: %d %v", code, body)
	}

	// Applying what passed is checked; applying anything else is not.
	if code, _, body := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": version, "hash": sha(goodUCI), "ok": true}, nil); code != 200 || body["checked"] != true {
		t.Fatalf("applied: %d %v", code, body)
	}
	if code, _, body := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": version, "hash": sha("other"), "ok": false, "error": "wifi failed"}, nil); code != 200 || body["checked"] != false {
		t.Fatalf("unchecked apply: %d %v", code, body)
	}
	if code, _, _ := f.apDo("POST", "/v1/ap/applied", token, map[string]any{"version": version, "hash": "nope", "ok": true}, nil); code != 400 {
		t.Fatalf("bad hash: %d", code)
	}

	_, view := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	cond := view["condition"].(map[string]any)
	if cond["in_sync"] != true || cond["check"].(map[string]any)["result"] != "ok" || cond["apply"].(map[string]any)["error"] != "wifi failed" {
		t.Fatalf("condition = %v", cond)
	}
	_, hist := f.do("GET", "/v1/aps/"+ap+"/history", "griff", nil)
	checks := hist["checks"].([]any)
	if n := len(checks); n != 4 {
		t.Fatalf("%d checks recorded, want 4: %v", n, hist)
	}
	if strings.Contains(jsonText(hist), passphrase) {
		t.Fatal("the history holds a passphrase")
	}
	// The kept UCI is what was sent, secrets blanked (0041); a stale one is
	// not kept.
	kept, _ := checks[0].(map[string]any)["uci"].(string)
	if kept != strings.ReplaceAll(goodUCI, passphrase, "<secret>") {
		t.Fatalf("kept UCI:\n%s", kept)
	}
	if stale := checks[3].(map[string]any); stale["result"] != "stale" || stale["uci"] != nil {
		t.Fatalf("stale check = %v", stale)
	}
	if strings.Contains(jsonText(cond["check"]), "package wireless") {
		t.Fatal("the summary carries the whole UCI")
	}

	// A change that breaks the config: the check refuses, and the AP is out
	// of sync until it runs the new version.
	if code, b := f.change("griff", map[string]any{"kind": "unset", "tree": "services", "node": "household", "path": "network.sweet.passphrase"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	_, view = f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	next := view["version"].(float64)
	if view["condition"].(map[string]any)["in_sync"] != false {
		t.Fatalf("in_sync after a change = %v", view["condition"])
	}
	if _, body := f.render(token, next, goodUCI); body["result"] != "refused" || !strings.Contains(jsonText(body["problems"]), "held") {
		t.Fatalf("held config: %v", body)
	}

	// Others cannot read an AP's history.
	if code, _ := f.do("GET", "/v1/aps/"+ap+"/history", "office", nil); code != 404 {
		t.Fatalf("office reading history: %d", code)
	}
}

// trimmedCases are TestStateReports' bad reports whose one bad thing is an
// element of a list the manager takes element by element (0290): each is
// taken without it. Two alike in a list is the list's, and still refused.
var trimmedCases = map[string]bool{
	"band": true, "beacons no time": true, "beacons too many": true, "bond member": true, "bss alone": true, "bss over": true,
	"client band": true, "client dhcp": true, "client gen": true, "client host": true, "client mac": true, "client params": true,
	"client signal": true, "client vlan": true, "heard": true, "lease mac": true, "missing ssid": true, "on-air iface": true,
	"on-air ssid": true, "on-air vlan": true, "port": true, "port max": true, "port stp": true, "port switch": true,
	"short, all on": true, "speed": true, "txpower": true, "vlan": true, "vlan probe": true, "vlan verdict": true, "watch": true,
}

func TestStateReports(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := map[string]any{
		"version": version, "uptime": 3600, "openwrt": "25.12.5",
		"radios": []any{map[string]any{"radio": "radio1", "band": "5g", "channel": 36, "width": 40, "clients": 3, "up": true, "txpower": 23, "bss": 3, "bss_want": 4, "short_for": 600, "missing": []string{"Home Wi-Fi"},
			"beacons": map[string]any{"sent": 11976, "failed": 24, "over": 300}}},
		// The VLANs watched on the uplink, and the switch it is on (0064).
		"uplink_vlans": []any{
			map[string]any{"vlan": 20, "tagged": true, "verdict": "present", "heard_ago": 12},
			map[string]any{"vlan": 30, "tagged": true, "verdict": "silent", "heard_ago": nil},
			map[string]any{"vlan": 50, "tagged": true, "verdict": "unknown", "heard_ago": nil},
		},
		"uplink_neighbor": map[string]any{"chassis": "28:e7:1d:d2:d0:13", "chassis_kind": "mac", "system": "homelab.example.com",
			"system_description": "Arista Networks EOS version 4.30.1F", "port": "Ethernet6", "port_kind": "interface name",
			"port_description": "Shed OpenWrt", "ttl": 120, "capabilities": []string{"bridge", "router"}, "enabled_capabilities": []string{"bridge", "router"},
			"management":  []any{map[string]any{"address": "172.16.0.4", "interface": 5000000, "interface_kind": "ifindex"}},
			"native_vlan": 1, "vlans": []int{1, 10, 20, 50, 1010}, "vlan_names": map[string]string{"1010": "PTP_Tower_Link"},
			"protocol_vlans": []int{}, "protocols": []string{"88cc"}, "aggregation": map[string]any{"capable": true, "enabled": false, "port": 0},
			"max_frame": 9416, "mac_phy": map[string]any{"autoneg_supported": true, "autoneg_enabled": true, "advertised": "6c01", "mau": 30, "mau_name": "1000BASE-T, full duplex"},
			"power": map[string]any{"pse": true, "supported": true, "enabled": true, "pair": "signal", "class": 4, "requested_w": 25.5, "allocated_w": 25.5},
			"med":   map[string]any{"capabilities": "0033", "class": 4, "policies": []any{map[string]any{"application": "voice", "unknown": false, "tagged": true, "vlan": 30, "priority": 5, "dscp": 46}}, "inventory": map[string]string{"software": "EOS-4.30.1F"}},
			"other": []any{map[string]any{"type": 127, "oui": "001c73", "subtype": 1, "data": "dead"}},
			"ago":   14},
		// The clients' DHCP on a network (0065): two servers, one a rogue, and
		// a client with a static address.
		"dhcp": map[string]any{"lab": map[string]any{
			"servers":  []any{map[string]any{"id": "192.168.20.254", "mac": "28:e7:1d:d2:d0:13", "answers": 4, "ago": 30}, map[string]any{"id": "192.168.20.99", "mac": "f2:0d:e6:3f:00:32", "answers": 1, "ago": 300}},
			"answered": 3, "unanswered": 1, "unanswered_clients": []string{"aa:bb:cc:dd:ee:01"},
			"duplicates": []any{map[string]any{"servers": []string{"192.168.20.254", "192.168.20.99"}, "client": "aa:bb:cc:dd:ee:02", "ago": 300}},
			"without":    []any{map[string]any{"mac": "aa:bb:cc:dd:ee:03", "joined_ago": 120, "address": "192.168.20.55"}, map[string]any{"mac": "aa:bb:cc:dd:ee:04", "joined_ago": 90, "address": nil}},
		}},
		// Its clock, as ntpd last said (0069).
		"time": map[string]any{"synced": true, "stratum": 3, "offset": -0.0012, "ago": 40, "servers": []string{"rutm50.example.com", "10.0.1.253"}},
		// Its radio neighbours (0073): one up on 2.4 GHz, heard both ways, and
		// one only heard in the air.
		"rrm": map[string]any{"address": "192.168.1.38", "advertised": []string{"2g", "5g"}, "neighbours": []any{
			map[string]any{"ap": "ap-2005b6df64e0", "address": "192.168.1.45", "state": "up", "chosen": true, "hello_ago": 4,
				"bands": []any{map[string]any{"band": "2g", "signal": -73, "their_signal": -73, "channel": 11, "width": 20,
					// Its radio's BSSID and how busy it is (0207).
					"bssid": "20:05:b6:df:64:e0", "load": map[string]any{"busy": 23.4, "clients": 7.0, "retried": 0.12}}}},
			map[string]any{"ap": "ap-a0046082275f", "address": "", "state": "heard", "chosen": true, "hello_ago": nil,
				"bands": []any{map[string]any{"band": "5g", "signal": -81, "their_signal": nil, "channel": nil, "width": nil}}}},
			// Its channel ratings: one in use, one blotted out by a neighbour.
			"ratings": []any{
				map[string]any{"band": "2g", "channel": 1, "cost": 38, "now": 41, "busy": 29, "energy": 29, "floor": -91, "noise": -92, "networks": 6, "visits": 12, "ago": 30, "own": false, "blotted_by": []string{}, "best": true},
				map[string]any{"band": "2g", "channel": 11, "cost": 12, "now": 9, "busy": 4, "noise": nil, "networks": 0, "visits": 40, "ago": 5, "own": true, "blotted_by": []string{"ap-2005b6df64e0"}}},
			// Its moves: one made, and one that yielded to a neighbour's claim.
			"moves": []any{
				map[string]any{"band": "2g", "from": 11, "to": 6, "why": "shared", "state": "moved", "at": 1791223832,
					// What came of it for its clients (0265).
					"clients": map[string]any{"clients": 12, "kept": 10, "back": 1, "gone": 1}},
				map[string]any{"band": "5g", "from": 157, "to": 44, "why": "better", "state": "yielded", "at": 1791224000, "ap": "ap-2005b6df64e0"},
				// To a DFS channel after a look, listening for radar (0120);
				// and off one where radar was found.
				map[string]any{"band": "5g", "from": 44, "to": 100, "why": "better", "state": "checking", "at": 1791224100},
				map[string]any{"band": "5g", "from": 100, "to": 44, "why": "radar", "state": "moved", "at": 1791224900}},
			// Its power control (0077): 2.4 GHz stepped down, 5 GHz at its ceiling.
			"apc": []any{
				map[string]any{"radio": "radio0", "band": "2g", "power": 19, "ceiling": 25, "wanted": 3, "target": -70, "count": 3, "weakest": -61, "step": -3, "why": "above", "ago": 120,
					// What it says of its clients (0206): one only it hears well,
					// and its clients' last bad turn, seen to.
					"clients": "ask", "client_floor": -80, "floor": nil, "hold": nil,
					"lonely": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "signal": -87, "est": -83, "need": 22, "other": nil},
						// One that says itself what it hears (802.11k).
						map[string]any{"mac": "aa:bb:cc:dd:ee:07", "signal": -80, "est": -71, "need": 0, "other": nil, "measured": true}},
					"distress": map[string]any{"at": 1791224000, "why": "retries", "scanning": 0, "left": 1, "ratio": []any{0.04, 0.22}, "acted": false, "after": 120}},
				map[string]any{"radio": "radio1", "band": "5g", "power": 25, "ceiling": 25, "wanted": 3, "target": -70, "count": 1, "weakest": -88, "step": 0, "why": "ceiling", "ago": nil}},
			// Its steps (0206): one down, and its clients' bad turn after it.
			"apc_events": []any{
				map[string]any{"at": 1791223000, "band": "2g", "from": 22, "to": 19, "why": "above", "scanning": nil, "left": nil, "ratio": nil},
				map[string]any{"at": 1791224000, "band": "2g", "from": 19, "to": 19, "why": "distress-seen", "scanning": 0, "left": 1, "ratio": []any{0.04, 0.22}}},
			// A radio on a DFS channel, which can't scan.
			// It steps off to 44 when its own channel grows worse (0120).
			"cannot_scan": []any{map[string]any{"band": "5g", "channel": 108, "why": "dfs", "ago": 600, "home": 44}}},
		// Every Wi-Fi client (0066): one on an Aeolus network, one on the AP's own.
		"clients": []any{
			map[string]any{"mac": "7e:2a:ea:04:b1:8f", "network": "lab", "ssid": "Aeolus Lab", "band": "5g", "signal": -49, "signal_avg": -50,
				"rx_rate": 72.2, "rx_mcs": 7, "rx_nss": nil, "tx_rate": 43.3, "tx_mcs": 4, "tx_nss": nil, "rx_bytes": 83667838, "tx_bytes": 1972710,
				"rx_packets": 22380, "tx_packets": 23095, "tx_retries": 12, "tx_failed": 0, "connected": 47, "inactive_ms": 40,
				"address": "192.168.20.61", "host": "Owners-iPhone", "dhcp": "ok",
				"vendor_class": nil, "params": "1,121,3,6,15,108,114,119,252,95,44,46", "gen": "ax", "k": true, "v": true, "w": true, "mbo": false, "wmm": true},
			map[string]any{"mac": "aa:bb:cc:dd:ee:06", "network": nil, "ssid": "Home_Wi-Fi_IoT", "band": "2g", "signal": -87, "signal_avg": nil,
				"rx_rate": 90.0, "rx_mcs": nil, "rx_nss": nil, "tx_rate": 120.0, "tx_mcs": nil, "tx_nss": nil, "rx_bytes": 2612304, "tx_bytes": 1972710,
				"rx_packets": 1, "tx_packets": 1, "tx_retries": 0, "tx_failed": 0, "connected": 90259, "inactive_ms": 1290,
				"address": nil, "host": nil, "dhcp": nil},
		},
		"uplink_port": map[string]any{"name": "wan", "mac": "a0:04:60:82:27:5e", "mtu": 1500, "carrier_changes": 3,
			// On each VLAN, the MAC the AP talks with there and what it holds (0186).
			"vlans": []any{map[string]any{"vlan": 1, "tagged": false, "mac": "a0:04:60:82:27:5e", "addresses": []string{"192.168.1.38/24", "fd00::38/64"}},
				map[string]any{"vlan": 20, "tagged": true, "mac": "a0:04:60:82:27:5e", "addresses": []string{}}}, "management_vlan": 1,
			"rx_bytes": 123456789, "tx_bytes": 98765432, "rx_packets": 1000, "tx_packets": 900, "rx_errors": 0, "tx_errors": 0, "rx_dropped": 12, "tx_dropped": 0},
		"transports": map[string]any{"sweet": map[string]any{"active": "primary", "primary": "up", "fallback": "unverified"},
			// With automatic switching: its last switch, and why it cannot switch now (0061).
			"lab": map[string]any{"active": "fallback", "primary": "down", "fallback": "up",
				"last_switch":   map[string]any{"from": "primary", "to": "fallback", "why": "the primary is down", "ago": 95},
				"cannot_switch": ""}},
		// A network's VLAN fallback, probed on the uplink (0061).
		"vlan_probes": []any{map[string]any{"vlan": 20, "tagged": true, "probe": map[string]any{
			"verdict": "up", "interval": 30, "asks": []string{"192.168.20.1"}, "underlay": true, "from": "192.168.20.1", "rtt_ms": 0.6, "answered_ago": 4,
			"lease": map[string]any{"address": "192.168.20.80", "server": "192.168.20.254", "router": "192.168.20.1", "expires_in": 86000, "mac": "02:82:27:5e:00:14"}}}},
		// A network the AP took off the air while it cannot reach its VLAN (0201).
		"off_air": map[string]any{"lab": map[string]any{"off": true, "ago": 120, "why": "its VLAN 20 does not answer", "stopped": []string{"phy0-ap1", "phy1-ap1"}}},
		// Managed on a VLAN its own Wi-Fi is on, and an address on a
		// bridge's own (0209).
		"on_air": []any{map[string]any{"interface": "vlan20", "management": true, "vlan": 20, "ssids": []string{"Home Wi-Fi", "Café"}},
			map[string]any{"interface": "aeolus_home_wi_fi_iot", "management": false, "vlan": nil, "ssids": []string{""}}},
		"steering": map[string]any{"installed": true, "running": true, "interval": 30000, "ssids": []string{"Home Wi-Fi"},
			"bss": []any{map[string]any{"ssid": "Home Wi-Fi", "band": "2g", "clients": 1, "steered_away": 4, "steered_in": 0}}},
		// lan1 blocked by spanning tree (0095).
		"ports": []any{map[string]any{"name": "lan1", "up": true, "carrier": false, "stp": "blocking"},
			// A bond, as the C-360's uplink is (0093): its members, each
			// faster than the switch port it is on.
			map[string]any{"name": "wan", "up": true, "carrier": true, "speed": "5000F", "uplink": true, "stp": "forwarding", "bond": map[string]any{"mode": "802.3ad", "aggregator": 1, "members": []any{
				map[string]any{"name": "eth0", "up": true, "carrier": true, "speed": "2500F", "max": 10000, "partner_max": 2500, "mii": "up", "aggregator": 1,
					"neighbor": map[string]any{"system": "homelab.example.com", "port": "Ethernet13"}},
				map[string]any{"name": "eth1", "up": true, "carrier": false, "max": 10000, "mii": "down", "aggregator": 2}}}}},
		"vxlan": map[string]any{"installed": true, "clamp": true, "prober": true, "tunnels": []any{
			// What the prober found (0059); what it does not know yet is null.
			map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789, "mtu": 1450, "up": true, "probe": map[string]any{
				"verdict": "up", "interval": 30, "asks": []string{"192.168.50.1"}, "underlay": true, "underlay_ms": 0.4,
				"from": "192.168.50.1", "rtt_ms": 0.8, "answered_ago": 12,
				"lease": map[string]any{"address": "192.168.50.6", "server": "192.168.50.254", "router": "192.168.50.1", "expires_in": 86000}}},
			// One that starts from VLAN 20, where the AP has an address (0063).
			map[string]any{"vni": 60, "peer": "1.1.1.2", "port": 4789, "mtu": 1450, "from_vlan": 20, "from_address": "192.168.20.74",
				"from_gateway": "192.168.20.1", "from_gateway_answers": true, "from_shared": []string{"wifi_trusted"}, "from_put_back_ago": 40,
				"up": true, "probe": map[string]any{
					"verdict": "unknown", "interval": 30, "asks": []string{"ff02::1"}, "underlay": nil, "underlay_ms": nil,
					"from": nil, "rtt_ms": nil, "answered_ago": nil}},
			map[string]any{"vni": 10, "peer": "2001:db8::2", "port": 4789, "mtu": 1450, "up": false, "standby": true},
			// Up to netifd, its device gone; one made again a minute ago (0187).
			map[string]any{"vni": 70, "peer": "1.1.1.2", "port": 4789, "mtu": 1450, "up": false, "gone": true, "probe": map[string]any{"verdict": "off", "interval": 30, "underlay": nil, "underlay_ms": nil, "rtt_ms": nil, "answered_ago": nil}},
			map[string]any{"vni": 80, "peer": "1.1.1.2", "port": 4789, "mtu": 1450, "up": true, "remade_ago": 60}},
			"loops": []any{map[string]any{"port": "lan3", "device": "lan3.30", "vni": 30, "came_in": "aeolus_30", "ago": 4}}},
	}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	// What the AP is on each VLAN of its uplink is kept as it said (0186).
	_, said := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	kept := said["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)
	if v := kept["uplink_port"].(map[string]any)["vlans"].([]any)[0].(map[string]any); v["mac"] != "a0:04:60:82:27:5e" || v["addresses"].([]any)[1] != "fd00::38/64" {
		t.Fatalf("uplink VLAN 1 = %v", v)
	}
	if l := kept["vlan_probes"].([]any)[0].(map[string]any)["probe"].(map[string]any)["lease"].(map[string]any); l["mac"] != "02:82:27:5e:00:14" {
		t.Fatalf("the probe's lease = %v", l)
	}
	for name, bad := range map[string]map[string]any{
		"band":             {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "60g"}}},
		"txpower":          {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "5g", "txpower": 99}}},
		"bss alone":        {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "5g", "bss": 2}}},
		"top many":         {"version": 1, "health": map[string]any{"top_mem": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}, map[string]any{"name": "c"}, map[string]any{"name": "d"}, map[string]any{"name": "e"}, map[string]any{"name": "f"}}}},
		"top cpu":          {"version": 1, "health": map[string]any{"top_cpu": []any{map[string]any{"name": "hostapd", "cpu": 120}}}},
		"top name":         {"version": 1, "health": map[string]any{"top_mem": []any{map[string]any{"name": "a\nb", "rss": 10}}}},
		"rrm listens":      {"version": 1, "rrm": map[string]any{"address": "192.168.1.2", "listens": "ears"}},
		"bss over":         {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "5g", "bss": 5, "bss_want": 4}}},
		"short, all on":    {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "5g", "bss": 4, "bss_want": 4, "short_for": 60}}},
		"missing ssid":     {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "5g", "bss": 3, "bss_want": 4, "short_for": 60, "missing": []string{""}}}},
		"beacons no time":  {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "5g", "beacons": map[string]any{"sent": 10, "failed": 0, "over": 0}}}},
		"beacons too many": {"version": 1, "radios": []any{map[string]any{"radio": "r", "band": "5g", "beacons": map[string]any{"sent": 1000000, "failed": 0, "over": 300}}}},
		"vlan":             {"version": 1, "uplink_vlans": []any{map[string]any{"vlan": 5000, "verdict": "present"}}},
		"repeat":           {"version": 1, "uplink_vlans": []any{map[string]any{"vlan": 20, "verdict": "present"}, map[string]any{"vlan": 20, "verdict": "silent"}}},
		"watch":            {"version": 1, "uplink_vlans": []any{map[string]any{"vlan": 20, "verdict": "missing"}}},
		"heard":            {"version": 1, "uplink_vlans": []any{map[string]any{"vlan": 20, "verdict": "present", "heard_ago": -1}}},
		"named":            {"version": 1, "uplink_neighbor": map[string]any{"system": "homelab", "vlans": []int{20, 20}}},
		"switch name":      {"version": 1, "uplink_neighbor": map[string]any{"system": "home\nlab"}},
		"old vlans":        {"version": 1, "vlans": []int{20}},
		"vlan name":        {"version": 1, "uplink_neighbor": map[string]any{"vlans": []int{20}, "vlan_names": map[string]string{"30": "x"}}},
		"other hex":        {"version": 1, "uplink_neighbor": map[string]any{"other": []any{map[string]any{"type": 9, "data": "xyz"}}}},
		"port max":         {"version": 1, "ports": []any{map[string]any{"name": "lan1", "up": true, "carrier": false, "max": -1}}},
		"port stp":         {"version": 1, "ports": []any{map[string]any{"name": "lan1", "up": true, "carrier": false, "stp": "discarding"}}},
		"port switch":      {"version": 1, "ports": []any{map[string]any{"name": "eth1", "up": true, "carrier": true, "uplink": true, "neighbor": map[string]any{"system": "home\nlab"}}}},
		"bond member":      {"version": 1, "ports": []any{map[string]any{"name": "bond0", "up": true, "carrier": true, "bond": map[string]any{"mode": "802.3ad", "members": []any{map[string]any{"name": "eth 0"}}}}}},
		"policy":           {"version": 1, "uplink_neighbor": map[string]any{"med": map[string]any{"policies": []any{map[string]any{"application": "voice", "dscp": 64}}}}},
		"port mac":         {"version": 1, "uplink_port": map[string]any{"name": "wan", "mac": "A0-04-60-82-27-5E"}},
		"counters":         {"version": 1, "uplink_port": map[string]any{"name": "wan", "mac": "a0:04:60:82:27:5e", "rx_errors": -1}},
		"port vlan":        {"version": 1, "uplink_port": map[string]any{"name": "wan", "mac": "a0:04:60:82:27:5e", "vlans": []any{map[string]any{"vlan": 0}}}},
		"vlan mac":         {"version": 1, "uplink_port": map[string]any{"name": "wan", "mac": "a0:04:60:82:27:5e", "vlans": []any{map[string]any{"vlan": 1, "mac": "a0-04"}}}},
		"vlan address":     {"version": 1, "uplink_port": map[string]any{"name": "wan", "mac": "a0:04:60:82:27:5e", "vlans": []any{map[string]any{"vlan": 1, "addresses": []string{"192.168.1.38"}}}}},
		"gone and up":      {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 70, "peer": "1.1.1.2", "port": 4789, "up": true, "gone": true}}}},
		"remade":           {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 70, "peer": "1.1.1.2", "port": 4789, "up": true, "remade_ago": -1}}}},
		"lease mac":        {"version": 1, "vlan_probes": []any{map[string]any{"vlan": 20, "probe": map[string]any{"verdict": "up", "lease": map[string]any{"address": "192.168.20.80", "mac": "probe"}}}}},
		"dhcp network":     {"version": 1, "dhcp": map[string]any{"Lab Net": map[string]any{}}},
		"off-air network":  {"version": 1, "off_air": map[string]any{"Lab Net": map[string]any{"off": true}}},
		"off-air ago":      {"version": 1, "off_air": map[string]any{"lab": map[string]any{"off": true, "ago": -1}}},
		"off-air iface":    {"version": 1, "off_air": map[string]any{"lab": map[string]any{"off": true, "stopped": []string{"phy0 ap1"}}}},
		"on-air iface":     {"version": 1, "on_air": []any{map[string]any{"interface": "vlan 20", "ssids": []string{"x"}}}},
		"on-air vlan":      {"version": 1, "on_air": []any{map[string]any{"interface": "vlan20", "vlan": 4095, "ssids": []string{"x"}}}},
		"on-air ssid":      {"version": 1, "on_air": []any{map[string]any{"interface": "vlan20", "ssids": []string{"a name longer than thirty-two bytes"}}}},
		"dhcp server":      {"version": 1, "dhcp": map[string]any{"lab": map[string]any{"servers": []any{map[string]any{"id": "fe80::1", "mac": "28:e7:1d:d2:d0:13"}}}}},
		"dhcp client":      {"version": 1, "dhcp": map[string]any{"lab": map[string]any{"without": []any{map[string]any{"mac": "phone"}}}}},
		"dhcp dup":         {"version": 1, "dhcp": map[string]any{"lab": map[string]any{"duplicates": []any{map[string]any{"servers": []string{"192.168.20.254"}, "client": "aa:bb:cc:dd:ee:02"}}}}},
		"dhcp count":       {"version": 1, "dhcp": map[string]any{"lab": map[string]any{"unanswered": -1}}},
		"client mac":       {"version": 1, "clients": []any{map[string]any{"mac": "phone"}}},
		"client band":      {"version": 1, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "band": "60g"}}},
		"client signal":    {"version": 1, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "signal": -200}}},
		"client dhcp":      {"version": 1, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "dhcp": "maybe"}}},
		"client host":      {"version": 1, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "host": "a\tb"}}},
		"client gen":       {"version": 1, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "gen": "wifi7"}}},
		"client params":    {"version": 1, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "params": "1;3;6"}}},
		"time stratum":     {"version": 1, "time": map[string]any{"synced": true, "stratum": 17}},
		"client vlan":      {"version": 1, "clients": []any{map[string]any{"mac": "aa:bb:cc:dd:ee:06", "vlan": 5000}}},
		"rrm ap":           {"version": 1, "rrm": map[string]any{"neighbours": []any{map[string]any{"ap": "office", "state": "up"}}}},
		"rrm state":        {"version": 1, "rrm": map[string]any{"neighbours": []any{map[string]any{"ap": "ap-2005b6df64e0", "state": "friends"}}}},
		"rrm signal":       {"version": 1, "rrm": map[string]any{"neighbours": []any{map[string]any{"ap": "ap-2005b6df64e0", "state": "up", "bands": []any{map[string]any{"band": "2g", "signal": 12}}}}}},
		"rrm address":      {"version": 1, "rrm": map[string]any{"address": "office.lan"}},
		"rrm rating":       {"version": 1, "rrm": map[string]any{"ratings": []any{map[string]any{"band": "2g", "channel": 1, "busy": 140}}}},
		"rrm blotter":      {"version": 1, "rrm": map[string]any{"ratings": []any{map[string]any{"band": "2g", "channel": 1, "blotted_by": []string{"office"}}}}},
		"rrm energy":       {"version": 1, "rrm": map[string]any{"ratings": []any{map[string]any{"band": "5g", "channel": 149, "busy": 15, "energy": 101}}}},
		"rrm floor":        {"version": 1, "rrm": map[string]any{"ratings": []any{map[string]any{"band": "5g", "channel": 149, "busy": 15, "floor": 3}}}},
		"rrm move why":     {"version": 1, "rrm": map[string]any{"moves": []any{map[string]any{"band": "2g", "from": 11, "to": 6, "why": "bored", "state": "moved"}}}},
		"rrm move to":      {"version": 1, "rrm": map[string]any{"moves": []any{map[string]any{"band": "2g", "from": 11, "to": 0, "why": "shared", "state": "moved"}}}},
		"rrm move ap":      {"version": 1, "rrm": map[string]any{"moves": []any{map[string]any{"band": "2g", "from": 11, "to": 6, "why": "shared", "state": "yielded", "ap": "office"}}}},
		"deaf why":         {"version": 1, "rrm": map[string]any{"cannot_scan": []any{map[string]any{"band": "5g", "channel": 108, "why": "tired", "ago": 1}}}},
		"deaf channel":     {"version": 1, "rrm": map[string]any{"cannot_scan": []any{map[string]any{"band": "5g", "channel": 0, "why": "dfs", "ago": 1}}}},
		"deaf home":        {"version": 1, "rrm": map[string]any{"cannot_scan": []any{map[string]any{"band": "5g", "channel": 108, "why": "dfs", "ago": 1, "home": 300}}}},
		"rrm move state":   {"version": 1, "rrm": map[string]any{"moves": []any{map[string]any{"band": "5g", "from": 44, "to": 100, "why": "better", "state": "waiting"}}}},
		"rrm move clients": {"version": 1, "rrm": map[string]any{"moves": []any{map[string]any{"band": "2g", "from": 11, "to": 6, "why": "shared", "state": "moved", "clients": map[string]any{"clients": 3, "kept": 3, "back": 1, "gone": 0}}}}},
		"apc why":          {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 20, "wanted": 3, "target": -70, "why": "louder"}}}},
		"apc power":        {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 50, "wanted": 3, "target": -70, "why": "target"}}}},
		"apc step":         {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 20, "wanted": 3, "target": -70, "step": 41, "why": "below"}}}},
		"apc clients":      {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 20, "wanted": 3, "target": -70, "why": "target", "clients": "shout"}}}},
		"apc lonely":       {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 20, "wanted": 3, "target": -70, "why": "target", "lonely": []any{map[string]any{"mac": "nobody", "signal": -80}}}}}},
		"apc distress":     {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 20, "wanted": 3, "target": -70, "why": "target", "distress": map[string]any{"at": 1, "why": "sulking"}}}}},
		"apc ratio":        {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 20, "wanted": 3, "target": -70, "why": "target", "distress": map[string]any{"at": 1, "why": "retries", "ratio": []any{0.1, 1.5}}}}}},
		"apc event":        {"version": 1, "rrm": map[string]any{"apc_events": []any{map[string]any{"at": 1, "band": "2g", "from": 20, "to": 17, "why": "whim"}}}},
		"rrm load":         {"version": 1, "rrm": map[string]any{"neighbours": []any{map[string]any{"ap": "ap-2005b6df64e0", "state": "up", "bands": []any{map[string]any{"band": "2g", "load": map[string]any{"busy": 140}}}}}}},
		"rrm bssid":        {"version": 1, "rrm": map[string]any{"neighbours": []any{map[string]any{"ap": "ap-2005b6df64e0", "state": "up", "bands": []any{map[string]any{"band": "2g", "bssid": "the office"}}}}}},
		"apc after":        {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio0", "band": "2g", "power": 20, "wanted": 3, "target": -70, "why": "target", "distress": map[string]any{"at": 1, "why": "retries", "after": -5}}}}},
		"apc radio":        {"version": 1, "rrm": map[string]any{"apc": []any{map[string]any{"radio": "radio 0; rm", "band": "2g", "power": 20, "wanted": 3, "target": -70, "why": "target"}}}},
		"time servers":     {"version": 1, "time": map[string]any{"servers": []string{"a\tb"}}},
		"active":           {"version": 1, "transports": map[string]any{"sweet": map[string]any{"active": "both"}}},
		"network":          {"version": 1, "transports": map[string]any{"Home Wi-Fi": map[string]any{"active": "none"}}},
		"unknown":          {"version": 1, "temperature": 40},
		"negative":         {"version": -1},
		"steering": {"version": 1, "steering": map[string]any{"installed": true, "running": true, "interval": 30000,
			"bss": []any{map[string]any{"ssid": "Home Wi-Fi", "band": "60g", "clients": 1}}}},
		"steered": {"version": 1, "steering": map[string]any{"installed": true, "bss": []any{map[string]any{"ssid": "x", "band": "5g", "steered_in": -1}}}},
		"port":    {"version": 1, "ports": []any{map[string]any{"name": "LAN 1"}}},
		"twice":   {"version": 1, "ports": []any{map[string]any{"name": "lan1"}, map[string]any{"name": "lan1"}}},
		"speed":   {"version": 1, "ports": []any{map[string]any{"name": "lan1", "speed": "fast"}}},
		"vni":     {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 16777216, "peer": "1.1.1.2", "port": 4789}}}},
		"peer":    {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "vtep.example.net", "port": 4789}}}},
		"verdict": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "fine", "interval": 30}}}}},
		"answered by": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "up", "interval": 30, "from": "gateway"}}}}},
		"rtt": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "up", "interval": 30, "rtt_ms": -1}}}}},
		"loop port": {"version": 1, "vxlan": map[string]any{"loops": []any{map[string]any{"port": "LAN 3", "device": "lan3", "ago": 1}}}},
		"lease": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"probe": map[string]any{"verdict": "up", "interval": 30, "lease": map[string]any{"address": "fe80::1", "expires_in": 10}}}}}},
		"loop device":  {"version": 1, "vxlan": map[string]any{"loops": []any{map[string]any{"port": "lan3", "device": "lan3; rm", "ago": 1}}}},
		"health":       {"version": 1, "transports": map[string]any{"sweet": map[string]any{"active": "primary", "primary": "fine"}}},
		"switch":       {"version": 1, "transports": map[string]any{"sweet": map[string]any{"active": "primary", "last_switch": map[string]any{"from": "primary", "to": "primary", "why": "x", "ago": 1}}}},
		"switch why":   {"version": 1, "transports": map[string]any{"sweet": map[string]any{"active": "primary", "last_switch": map[string]any{"from": "fallback", "to": "primary", "ago": 1}}}},
		"cannot":       {"version": 1, "transports": map[string]any{"sweet": map[string]any{"active": "primary", "cannot_switch": strings.Repeat("x", 201)}}},
		"vlan probe":   {"version": 1, "vlan_probes": []any{map[string]any{"vlan": 4095, "probe": map[string]any{"verdict": "up", "interval": 30}}}},
		"vlan verdict": {"version": 1, "vlan_probes": []any{map[string]any{"vlan": 20, "probe": map[string]any{"verdict": "great", "interval": 30}}}},
		"from vlan":    {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789, "from_vlan": 4095}}}},
		"from address": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_vlan": 20, "from_address": "fe80::1"}}}},
		"address, no vlan": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_address": "192.168.20.74"}}}},
		"gateway, no address": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_vlan": 20, "from_gateway": "192.168.20.1"}}}},
		"gateway answers, none": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_vlan": 20, "from_address": "192.168.20.74", "from_gateway_answers": true}}}},
		"shared, no vlan": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_shared": []string{"wifi_trusted"}}}}},
		"shared name": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_vlan": 20, "from_shared": []string{"wifi trusted; rm"}}}}},
		"put back": {"version": 1, "vxlan": map[string]any{"tunnels": []any{map[string]any{"vni": 50, "peer": "1.1.1.2", "port": 4789,
			"from_vlan": 20, "from_put_back_ago": -1}}}},
	} {
		// A list's element that is not well said is left out, and the rest
		// of the report taken (0290): those reports come back taken.
		want := 400
		if trimmedCases[name] {
			want = 200
		}
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, bad, nil); code != want {
			t.Errorf("%s: %d, want %d: %v", name, code, want, body)
		}
	}
	// The report from before them is the AP's latest again.
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state again: %d %v", code, body)
	}
	_, view := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
	cond := view["condition"].(map[string]any)
	state := cond["state"].(map[string]any)["report"].(map[string]any)
	steer, _ := state["steering"].(map[string]any)
	ports, _ := state["ports"].([]any)
	tunnels, _ := state["vxlan"].(map[string]any)["tunnels"].([]any)
	loops, _ := state["vxlan"].(map[string]any)["loops"].([]any)
	// The manager adds what it knows of each client (0067): a private MAC and
	// an iPhone by its name; the AP's own client's private MAC too.
	clients, _ := state["clients"].([]any)
	if len(clients) != 2 {
		t.Fatalf("clients: %v", state["clients"])
	}
	phone := clients[0].(map[string]any)
	if phone["private"] != true || phone["maker"] != nil || phone["kind"] != "phone" || phone["os"] != "iOS" || phone["basis"] != "host name" {
		t.Errorf("the phone as stored: %v", phone)
	}
	if cond["in_sync"] != true || state["openwrt"] != "25.12.5" || len(state["uplink_vlans"].([]any)) != 3 || steer["interval"] != 30000.0 || len(steer["bss"].([]any)) != 1 || len(ports) != 2 || len(tunnels) != 5 || len(loops) != 1 {
		t.Fatalf("condition = %v", cond)
	}
	// A tunnel whose device is gone, and one made again (0187).
	if g := tunnels[3].(map[string]any); g["gone"] != true || g["up"] != false || tunnels[4].(map[string]any)["remade_ago"] != 60.0 {
		t.Errorf("tunnels gone and made again: %v, %v", tunnels[3], tunnels[4])
	}
	// A bond's members, as the agent reported them (0093).
	if b, _ := ports[1].(map[string]any)["bond"].(map[string]any); b["mode"] != "802.3ad" || len(b["members"].([]any)) != 2 ||
		b["members"].([]any)[0].(map[string]any)["max"] != 10000.0 || b["members"].([]any)[1].(map[string]any)["aggregator"] != 2.0 {
		t.Fatalf("the bond as stored: %v", ports[1])
	}
	if ports[0].(map[string]any)["stp"] != "blocking" || ports[1].(map[string]any)["stp"] != "forwarding" {
		t.Fatalf("spanning tree as stored: %v", ports)
	}
	if m := ports[1].(map[string]any)["bond"].(map[string]any)["members"].([]any)[0].(map[string]any); m["neighbor"].(map[string]any)["port"] != "Ethernet13" {
		t.Fatalf("a member's switch port as stored: %v", m)
	}
	if p, _ := tunnels[0].(map[string]any)["probe"].(map[string]any); p["verdict"] != "up" || p["from"] != "192.168.50.1" || p["rtt_ms"] != 0.8 ||
		p["lease"].(map[string]any)["address"] != "192.168.50.6" {
		t.Fatalf("probe = %v", p)
	}
	if s := tunnels[1].(map[string]any); s["from_vlan"] != 20.0 || s["from_address"] != "192.168.20.74" || s["from_gateway"] != "192.168.20.1" || s["from_gateway_answers"] != true ||
		len(s["from_shared"].([]any)) != 1 || s["from_put_back_ago"] != 40.0 {
		t.Fatalf("where the tunnel starts = %v", s)
	}
	// Its radios' beacons, kept for the beacons alert's half hour (0339):
	// of both times the report was taken, and none of those refused.
	if b, err := f.conds.BeaconsSince(hierarchy.NodeID(ap), time.Time{}); err != nil || len(b) != 1 ||
		b[0] != (conditions.RadioBeacons{Radio: "radio1", Band: "5g", Sent: 2 * 11976, Failed: 2 * 24, Over: 2 * 300}) {
		t.Errorf("beacons kept: %+v %v", b, err)
	}
}

// A tunnel the AP's uplink cannot carry now, by what it last reported, is
// held at once: in the preview, and when the AP polls (0056).
func TestTunnelMustFitTheUplink(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := func(mtu int) {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
			"vxlan": map[string]any{"installed": true, "clamp": true, "uplink_mtu": mtu}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
	}
	set := func(tree, node, path string, v any) map[string]any {
		t.Helper()
		code, body := f.change("griff", map[string]any{"kind": "set", "tree": tree, "node": node, "path": path, "value": v})
		if code != 200 {
			t.Fatalf("%s: %d %v", path, code, body)
		}
		return body
	}
	poll := func() map[string]any {
		t.Helper()
		_, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		return body
	}
	report(1500)
	set("locations", "office", "concentrators.dc.address", "1.1.1.2")
	set("locations", "office", "concentrators.dc.port", 4789)
	set("services", "household", "network.sweet.transport.primary.concentrator", "dc")
	set("services", "household", "network.sweet.transport.primary.vni", 20)
	set("services", "household", "network.sweet.transport.primary.type", "vxlan")
	// The default MTU, 1450, fits a 1500-byte uplink.
	if body := poll(); body["state"] != "ready" {
		t.Fatalf("default MTU: %v", body)
	}
	// 1500 does not, and the change that sets it says so.
	body := set("locations", "office", "concentrators.dc.mtu", 1500)
	want := "network.sweet.transport.primary: tunnel dc's MTU of 1500 needs 1550 on the AP's uplink, which carries 1500 now"
	if !strings.Contains(fmt.Sprint(body["checks"]), want) {
		t.Fatalf("checks = %v", body["checks"])
	}
	if body := poll(); body["state"] != "held" || !strings.Contains(fmt.Sprint(body["problems"]), want) {
		t.Fatalf("1500 on a 1500 uplink: %v", body)
	}
	// Once the AP reports a jumbo uplink, it goes.
	report(9000)
	if body := poll(); body["state"] != "ready" {
		t.Fatalf("1500 on a 9000 uplink: %v", body)
	}
	_ = ap
}

// What an AP reports it cannot run is held before it is sent (0057): a
// tunnel while netifd has not loaded vxlan, and band steering while hostapd
// lacks 802.11v.
func TestReportedLacksAreHeld(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	report := func(loaded, btm bool) {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
			"vxlan":    map[string]any{"installed": true, "loaded": loaded, "clamp": true, "uplink_mtu": 1500},
			"steering": map[string]any{"installed": true, "running": true, "interval": 0, "bss_transition": btm}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
	}
	set := func(tree, node, path string, v any) {
		t.Helper()
		if code, body := f.change("griff", map[string]any{"kind": "set", "tree": tree, "node": node, "path": path, "value": v}); code != 200 {
			t.Fatalf("%s: %d %v", path, code, body)
		}
	}
	poll := func() string {
		t.Helper()
		_, _, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		return fmt.Sprint(body["state"], body["problems"])
	}
	report(false, false)
	set("services", "household", "network.sweet.band_steering", true)
	if got := poll(); !strings.HasPrefix(got, "held") || !strings.Contains(got, "network.sweet: band steering and BSS transition need 802.11v, which this AP's hostapd lacks") {
		t.Fatalf("band steering without 802.11v: %s", got)
	}
	set("services", "household", "network.sweet.band_steering", false)
	set("locations", "office", "concentrators.dc.address", "1.1.1.2")
	set("locations", "office", "concentrators.dc.port", 4789)
	set("services", "household", "network.sweet.transport.primary.concentrator", "dc")
	set("services", "household", "network.sweet.transport.primary.vni", 20)
	set("services", "household", "network.sweet.transport.primary.type", "vxlan")
	if got := poll(); !strings.HasPrefix(got, "held") || !strings.Contains(got, "this AP's netifd has not loaded vxlan; restart its network") {
		t.Fatalf("a tunnel before netifd loaded vxlan: %s", got)
	}
	report(true, false)
	if got := poll(); !strings.HasPrefix(got, "ready") {
		t.Fatalf("once loaded: %s", got)
	}
}

func TestFleetView(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	code, body := f.do("GET", "/v1/aps", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	byID := map[string]map[string]any{}
	for _, a := range body["aps"].([]any) {
		m := a.(map[string]any)
		byID[m["id"].(string)] = m
	}
	got := byID[ap]
	if got == nil || got["config"] != "ready" || got["in_sync"] != true || got["name"] != "ShedAP" || got["seen"] == nil || got["wireless_missing"] != nil {
		t.Fatalf("adopted AP = %v", got)
	}
	// An AP whose netifd lost its network.wireless object says so, and the
	// fleet view flags it.
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 90, "wireless_missing": true}, nil); code != 200 {
		t.Fatalf("state, wireless missing: %d %v", code, body)
	}
	_, body = f.do("GET", "/v1/aps", "griff", nil)
	for _, a := range body["aps"].([]any) {
		if m := a.(map[string]any); m["id"] == ap && m["wireless_missing"] != true {
			t.Fatalf("the fleet view doesn't flag the lost object: %v", m)
		}
	}
	if office := byID["office-ap"]; office == nil || office["seen"] != nil || office["in_sync"] != nil {
		t.Fatalf("office-ap = %v", office)
	}
	// office has no role in Locations, so it sees no APs.
	if _, body := f.do("GET", "/v1/aps", "office", nil); len(body["aps"].([]any)) != 0 {
		t.Fatalf("office sees %v", body["aps"])
	}
}
