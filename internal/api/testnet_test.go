package api

import (
	"testing"
	"time"
)

// A test network (0179): set up as any network with test set; switched on
// in a place by whoever has the test SSID right, an installer among them,
// for at most four hours; switched off again by unsetting it.
func TestTestNetwork(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "values": map[string]any{
		"network.trial.ssid": "Aeolus Trial", "network.trial.security": "wpa2-psk", "network.trial.passphrase": "trial-passphrase-1",
		"network.trial.transport.primary.type": "vlan", "network.trial.transport.primary.vlan": 20, "network.trial.test": true,
	}}); code != 200 {
		t.Fatalf("set the test network up: %d %v", code, b)
	}
	for who, role := range map[string]string{"tenant": "installer", "office": "viewer"} {
		if code, b := f.change("griff", map[string]any{"kind": "grant", "account": who, "tree": "locations", "node": "office", "role": role}); code != 200 {
			t.Fatalf("grant: %d %v", code, b)
		}
	}
	on := func(network string, until time.Time) map[string]any {
		return map[string]any{"kind": "set", "tree": "locations", "node": "office", "values": map[string]any{
			"test_network.network": network, "test_network.until": until.Unix()}}
	}
	now := time.Now()
	for _, c := range []struct {
		name string
		as   string
		op   map[string]any
		code int
	}{
		{"a viewer", "office", on("trial", now.Add(time.Hour)), 403},
		{"for more than four hours", "tenant", on("trial", now.Add(5*time.Hour)), 400},
		{"until a time gone", "tenant", on("trial", now.Add(-time.Minute)), 400},
		{"a network that is no test network", "tenant", on("sweet", now.Add(time.Hour)), 400},
		{"a network no folder here offers", "tenant", on("nowhere", now.Add(time.Hour)), 400},
		{"without its until", "tenant", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "test_network.network", "value": "trial"}, 400},
		{"an installer, for an hour", "tenant", on("trial", now.Add(time.Hour)), 200},
	} {
		if code, b := f.change(c.as, c.op); code != c.code {
			t.Errorf("%s: %d %v, want %d", c.name, code, b, c.code)
		}
	}
	// The APs below are sent it, with the network, as they poll.
	_, token, _ := f.adopted()
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	doc, _ := cfg["config"].(map[string]any)
	tn, _ := doc["test_network"].(map[string]any)
	nets, _ := doc["network"].(map[string]any)
	net, _ := nets["trial"].(map[string]any)
	if tn["network"] != "trial" || net["test"] != true {
		t.Errorf("the AP's config: test_network %v, trial %v", tn, net)
	}
	// Off again, by unsetting it: then it is sent to no AP, so one whose
	// agent knows no test networks never carries one only set up.
	if code, b := f.change("tenant", map[string]any{"kind": "unset", "tree": "locations", "node": "office", "paths": []string{"test_network.network", "test_network.until"}}); code != 200 {
		t.Errorf("switching it off: %d %v", code, b)
	}
	_, _, cfg = f.apDo("GET", "/v1/ap/config", token, nil, nil)
	doc, _ = cfg["config"].(map[string]any)
	nets, _ = doc["network"].(map[string]any)
	if _, sent := nets["trial"]; sent || nets["sweet"] == nil {
		t.Errorf("off, the AP's networks: %v", nets)
	}
}
