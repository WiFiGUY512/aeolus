package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// A client's way out (0168): from its AP's report, by the switch its AP's
// LLDP names, over the switches set for where it is, read by their eAPI.
// The switches' password is the manager's to use, and no AP is sent it.
func TestClientPath(t *testing.T) {
	const mac, addr, pass = "aa:bb:cc:00:11:22", "192.168.20.151", "Read-Only.Pass1"
	raw, err := os.ReadFile("../eapi/testdata/homelab.json")
	if err != nil {
		t.Fatal(err)
	}
	var answers []json.RawMessage
	json.Unmarshal(raw, &answers)
	asked := 0
	sw := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, _ := r.BasicAuth(); u != "aeolus" || p != pass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		asked++
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "aeolus", "result": answers})
	}))
	defer sw.Close()
	host := strings.TrimPrefix(sw.URL, "https://")

	f := newFixture(t)
	_, token, version := f.adopted()
	client := map[string]any{"mac": mac, "address": addr, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -52, "signal_avg": -52,
		"tx_rate": 400, "rx_rate": 300, "connected": 60, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1}
	report := map[string]any{"version": version, "uptime": 60, "clients": []any{client},
		"uplink_port":     map[string]any{"name": "eth1", "mac": "30:86:2d:ef:2f:ff", "mtu": 1500, "carrier_changes": 1},
		"uplink_neighbor": map[string]any{"chassis": "28:e7:1d:d2:d0:13", "chassis_kind": "mac", "system": "homelab.example.com", "port": "Ethernet13", "port_kind": "interface name", "ago": 10}}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	kinds := func(got map[string]any) string {
		var out []string
		for _, h := range got["hops"].([]any) {
			out = append(out, h.(map[string]any)["kind"].(string)+":"+h.(map[string]any)["name"].(string))
		}
		return strings.Join(out, " ")
	}

	// With no switches set: as far as the AP's own LLDP says.
	_, got := f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil)
	if k := kinds(got); k != "client:"+" ap:"+got["hops"].([]any)[1].(map[string]any)["name"].(string)+" device:homelab.example.com" ||
		!strings.Contains(got["ends"].(string), "not one the manager reads") || got["on"] != true {
		t.Fatalf("no switches: %s %v", k, got)
	}

	set := func(values map[string]any) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "values": values}); code != 200 {
			t.Fatalf("set: %d %v", code, b)
		}
	}
	// Some set, not all.
	set(map[string]any{"system.switches.addresses": []any{host}})
	if _, got = f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil); !strings.Contains(got["error"].(string), "a user and its password") {
		t.Fatalf("half set: %v", got)
	}

	set(map[string]any{"system.switches.user": "aeolus", "system.switches.password": pass})
	_, got = f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil)
	if k := kinds(got); !strings.HasSuffix(k, " switch:homelab device:tower-switch.example.com internet:internet") {
		t.Fatalf("way: %s %v", k, got)
	}
	hop := got["hops"].([]any)[2].(map[string]any)
	if hop["gateway"] != "Vlan20 192.168.20.1" || hop["seen"] != true || hop["vlan"] != float64(20) || hop["in"] != "Ethernet13 (Port-Channel2)" {
		t.Errorf("gateway hop = %v", hop)
	}
	if v, _ := hop["vendor"].(string); !strings.Contains(strings.ToLower(v), "arista") {
		t.Errorf("gateway's vendor = %q", v)
	}
	said := got["switches"].([]any)
	if len(said) != 1 || said[0].(map[string]any)["name"] != "homelab" || said[0].(map[string]any)["error"] != nil {
		t.Errorf("switches = %v", said)
	}
	// Asked again within the minute, the switch is not.
	f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil)
	if asked != 1 {
		t.Errorf("the switch was asked %d times", asked)
	}
	if code, _ := f.do("GET", "/v1/clients/"+mac+"/path", "tenant", nil); code != 404 {
		t.Errorf("to one who may not view the AP: %d", code)
	}

	// No AP is sent the switches or their password, and no one is told it.
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	b, _ := json.Marshal(cfg)
	if strings.Contains(string(b), "switches") || strings.Contains(string(b), pass) || strings.Contains(string(b), host) {
		t.Errorf("the AP's config has the switches in it: %s", b)
	}
	b, _ = json.Marshal(got)
	if strings.Contains(string(b), pass) {
		t.Errorf("the answer says the password: %s", b)
	}

	// Only an admin sets where the manager signs in, and with what: an
	// operator could otherwise send the password set above anywhere.
	for path, v := range map[string]any{"system.switches.addresses": []any{"evil.example:443"}, "system.dns_log.url": "http://evil.example"} {
		if code, b := f.change("claude", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": path, "value": v}); code != 403 {
			t.Errorf("an operator sets %s: %d %v", path, code, b)
		}
	}
	// Addresses set below where the password is are not used with it.
	ap := got["ap"].(string)
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "path": "system.switches.addresses", "value": []any{"evil.example:443"}}); code != 200 {
		t.Fatalf("set on the AP: %d %v", code, b)
	}
	_, below := f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil)
	if why, _ := below["error"].(string); !strings.Contains(why, "set in different places") || len(below["switches"].([]any)) != 0 {
		t.Errorf("addresses below the password: %v", below)
	}
	if code, b := f.change("griff", map[string]any{"kind": "unset", "tree": "locations", "node": ap, "path": "system.switches.addresses"}); code != 200 {
		t.Fatalf("unset: %d %v", code, b)
	}

	// A password the switch refuses: said as that.
	set(map[string]any{"system.switches.password": "Not.The.Pass"})
	_, got = f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil)
	said = got["switches"].([]any)
	if why, _ := said[0].(map[string]any)["error"].(string); !strings.Contains(why, "refuses the user or password") || strings.Contains(why, "Not.The.Pass") {
		t.Errorf("a refused password: %v", said)
	}
	if k := kinds(got); !strings.HasSuffix(k, " device:homelab.example.com") {
		t.Errorf("with the switch unread: %s", k)
	}
}

// Settings Break Hierarchy copies down go together, and with nothing set
// below: a folder's admin cannot have the password set above sent to an
// address of their own (0168).
func TestSwitchesSetTogether(t *testing.T) {
	const mac, pass = "aa:bb:cc:00:11:23", "Read-Only.Pass2"
	raw, _ := os.ReadFile("../eapi/testdata/homelab.json")
	var answers []json.RawMessage
	json.Unmarshal(raw, &answers)
	sw := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "aeolus", "result": answers})
	}))
	defer sw.Close()
	f := newFixture(t)
	_, token, version := f.adopted()
	client := map[string]any{"mac": mac, "address": "192.168.20.152", "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -52, "signal_avg": -52,
		"tx_rate": 400, "rx_rate": 300, "connected": 60, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "clients": []any{client},
		"uplink_neighbor": map[string]any{"system": "homelab.example.com", "port": "Ethernet13", "ago": 10}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	do := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	do(map[string]any{"kind": "set", "tree": "locations", "node": "house", "values": map[string]any{
		"system.switches.addresses": []any{strings.TrimPrefix(sw.URL, "https://")}, "system.switches.user": "aeolus", "system.switches.password": pass}})
	do(map[string]any{"kind": "break-hierarchy", "tree": "locations", "node": "office"})
	_, got := f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil)
	if s := got["switches"].([]any); got["error"] != nil || len(s) != 1 || s[0].(map[string]any)["name"] != "homelab" {
		t.Fatalf("copied down together: %v", got)
	}
	do(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "system.switches.addresses", "value": []any{"evil.example:443"}})
	_, got = f.do("GET", "/v1/clients/"+mac+"/path", "griff", nil)
	if why, _ := got["error"].(string); !strings.Contains(why, "set in different places") || len(got["switches"].([]any)) != 0 {
		t.Errorf("an address set where the password was copied: %v", got)
	}
}
