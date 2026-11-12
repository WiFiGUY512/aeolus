package api

import (
	"strings"
	"testing"
)

// A name a person gives a client (0153): a device that says no name of its
// own is called what someone calls it, wherever it is shown and searched
// for; who may operate its AP may name it, and who may view it reads the
// name.
func TestNamingAClient(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	const printer, phone = "02:00:00:00:00:31", "02:00:00:00:00:32"
	client := func(m, host string) map[string]any {
		return map[string]any{"mac": m, "host": host, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -60, "signal_avg": -60,
			"tx_rate": 400, "rx_rate": 300, "connected": 60, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1}
	}
	if code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
		"clients": []any{client(printer, ""), client(phone, "Owners-iPhone")}}, nil); code != 200 {
		t.Fatalf("state: %d", code)
	}
	name := func(who, mac string, body any) (int, map[string]any) {
		return f.do("PUT", "/v1/clients/"+mac+"/name", who, body)
	}

	// Named, with the spaces around and within it tidied.
	if code, got := name("griff", printer, map[string]any{"name": "  Bambu   printer "}); code != 200 || got["name"] != "Bambu printer" || got["mac"] != printer {
		t.Fatalf("naming: %d %v", code, got)
	}
	// By its MAC in any case.
	if code, got := name("griff", strings.ToUpper(phone), map[string]any{"name": "a phone"}); code != 200 || got["mac"] != phone {
		t.Fatalf("naming by a MAC in capitals: %d %v", code, got)
	}
	_, names := f.do("GET", "/v1/client-names", "griff", nil)
	if m := names["names"].(map[string]any); len(m) != 2 || m[printer] != "Bambu printer" || m[phone] != "a phone" {
		t.Fatalf("names = %v", names)
	}
	// And the name each last gave of itself: the phone's, though its AP's
	// next report no longer says it, as after the AP's agent starts again.
	if code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 120,
		"clients": []any{client(printer, ""), client(phone, "")}}, nil); code != 200 {
		t.Fatalf("state: %d", code)
	}
	_, names = f.do("GET", "/v1/client-names", "griff", nil)
	if m := names["hosts"].(map[string]any); len(m) != 1 || m[phone] != "Owners-iPhone" {
		t.Fatalf("hosts = %v", names["hosts"])
	}

	// The list of every client has it, and finds the client by it; the name
	// the device gives is still there beside it.
	_, found := f.do("GET", "/v1/clients?q=BAMBU", "griff", nil)
	hit := found["clients"].([]any)
	if len(hit) != 1 || hit[0].(map[string]any)["mac"] != printer || hit[0].(map[string]any)["name"] != "Bambu printer" {
		t.Fatalf("found by its name = %v", found)
	}
	_, found = f.do("GET", "/v1/clients?q=iphone", "griff", nil)
	if hit := found["clients"].([]any); len(hit) != 1 || hit[0].(map[string]any)["host"] != "Owners-iPhone" || hit[0].(map[string]any)["name"] != "a phone" {
		t.Fatalf("found by what it calls itself = %v", found)
	}
	// Its journey says the name, who gave it and when.
	_, j := f.do("GET", "/v1/clients/"+printer, "griff", nil)
	if j["name"] != "Bambu printer" || j["named_by"] != "griff" || j["named_at"] == nil {
		t.Fatalf("journey = %v %v %v", j["name"], j["named_by"], j["named_at"])
	}

	// Named again, the new name takes the old one's place; an empty one
	// takes the name away.
	if code, _ := name("griff", printer, map[string]any{"name": "X1 Carbon"}); code != 200 {
		t.Fatalf("naming again: %d", code)
	}
	if code, got := name("griff", phone, map[string]any{"name": "   "}); code != 200 || got["name"] != "" {
		t.Fatalf("taking a name away: %d %v", code, got)
	}
	_, names = f.do("GET", "/v1/client-names", "griff", nil)
	if m := names["names"].(map[string]any); len(m) != 1 || m[printer] != "X1 Carbon" {
		t.Fatalf("names after = %v", names)
	}
	if _, j := f.do("GET", "/v1/clients/"+phone, "griff", nil); j["name"] != nil {
		t.Fatalf("a name taken away is still said: %v", j["name"])
	}

	// What is no name is refused, and so is a client never seen.
	for what, body := range map[string]any{
		"too long":       map[string]any{"name": strings.Repeat("n", 65)},
		"a control code": map[string]any{"name": "a\u0007b"},
		"another field":  map[string]any{"name": "ok", "host": "x"},
	} {
		if code, got := name("griff", printer, body); code != 400 {
			t.Errorf("%s: %d %v", what, code, got)
		}
	}
	if code, _ := name("griff", "not-a-mac", map[string]any{"name": "x"}); code != 400 {
		t.Errorf("no MAC: %d", code)
	}
	if code, _ := name("griff", "02:00:00:00:00:99", map[string]any{"name": "x"}); code != 404 {
		t.Errorf("a client never seen: %d", code)
	}
	// Sixty-four characters, of any script, are a name.
	if code, _ := name("griff", printer, map[string]any{"name": strings.Repeat("é", 64)}); code != 200 {
		t.Errorf("64 characters: %d", code)
	}

	// Who may not view its AP sees no such client, and none of the names;
	// who may only view it reads the name and cannot give one.
	if code, _ := name("tenant", printer, map[string]any{"name": "mine"}); code != 404 {
		t.Errorf("one who may not view its AP: %d", code)
	}
	if _, names := f.do("GET", "/v1/client-names", "tenant", nil); len(names["names"].(map[string]any)) != 0 || len(names["hosts"].(map[string]any)) != 0 {
		t.Errorf("names to one who may view no AP: %v", names)
	}
	if code, b := f.change("griff", map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "viewer"}); code != 200 {
		t.Fatalf("grant: %d %v", code, b)
	}
	if code, _ := name("tenant", printer, map[string]any{"name": "mine"}); code != 403 {
		t.Errorf("a viewer naming it: %d", code)
	}
	if _, names := f.do("GET", "/v1/client-names", "tenant", nil); len(names["names"].(map[string]any)) != 1 {
		t.Errorf("names to a viewer of its AP: %v", names)
	}
}
