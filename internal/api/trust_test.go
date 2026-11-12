package api

import (
	"slices"
	"testing"
)

// An AP's airscan is handed what the fleet calls its own (0304, F3): every
// BSSID an AP's latest report names as its own, those its folders know, and
// the SSID of every network, beside the config; its version moves when they
// do, and an AP in Landing Zone is given nothing.
func TestAirscanTrust(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	trust := func() ([]string, []string, string) {
		t.Helper()
		code, _, body := f.apDo("GET", "/v1/ap/trust", token, nil, nil)
		if code != 200 {
			t.Fatalf("trust: %d %v", code, body)
		}
		var ssids, bssids []string
		for _, s := range body["ssids"].([]any) {
			ssids = append(ssids, s.(string))
		}
		for _, b := range body["bssids"].([]any) {
			bssids = append(bssids, b.(string))
		}
		return ssids, bssids, body["version"].(string)
	}
	poll := func() string {
		t.Helper()
		code, h, _ := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code != 200 {
			t.Fatalf("poll: %d", code)
		}
		return h.Get(TrustHeader)
	}

	ssids, bssids, v1 := trust()
	if len(ssids) == 0 || len(bssids) != 0 || poll() != v1 {
		t.Fatalf("before any report: ssids %v bssids %v, version %s, poll %s", ssids, bssids, v1, poll())
	}
	// Its report names its BSSIDs; one not well said is left out.
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60,
		"bssids": []any{"02:00:00:00:00:02", "02:00:00:00:00:01", "not-a-bssid"}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	// What its folder knows is no rogue.
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "rogues.known", "value": []any{"00:11:74:79:F0:B2"}}); code != 200 {
		t.Fatalf("rogues.known: %d %v", code, b)
	}
	_, bssids, v2 := trust()
	if want := []string{"00:11:74:79:f0:b2", "02:00:00:00:00:01", "02:00:00:00:00:02"}; !slices.Equal(bssids, want) {
		t.Fatalf("bssids %v, want %v", bssids, want)
	}
	if v2 == v1 || poll() != v2 {
		t.Fatalf("version %s after %s, poll %s", v2, v1, poll())
	}
	// A network's SSID is among them.
	if !slices.Contains(ssids, "Home Wi-Fi") {
		t.Fatalf("ssids %v", ssids)
	}

	// In Landing Zone: no header, and nothing to fetch.
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": "landing-zone"}); code != 200 {
		t.Fatalf("to Landing Zone: %d %v", code, b)
	}
	if h := poll(); h != "" {
		t.Fatalf("in Landing Zone, the header: %q", h)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/trust", token, nil, nil); code == 200 {
		t.Fatal("in Landing Zone, trust was handed out")
	}
}
