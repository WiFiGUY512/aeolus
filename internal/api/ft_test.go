package api

import (
	"regexp"
	"testing"
)

// The APs of a site are handed each other's 802.11r key holders (0224):
// each BSS a network's AP said it runs, with a key for the network, beside
// the config; an AP of another site, or in Landing Zone, is not among them.
func TestFTKeyHolders(t *testing.T) {
	f := newFixture(t)
	_, pump := f.enrolled()
	enroll := func(mac, folder string) string {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": "OpenWrt"}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", mac, code, body)
		}
		if folder != "" {
			if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": body["ap"], "parent": folder}); code != 200 {
				t.Fatalf("adopt: %d %v", code, b)
			}
		}
		return body["token"].(string)
	}
	if code, b := f.change("griff", map[string]any{"kind": "move", "tree": "locations", "node": "ap-bcff4d02267a", "parent": "office"}); code != 200 {
		t.Fatalf("adopt: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "add-folder", "tree": "locations", "node": "cabin", "name": "Cabin", "parent": "example"}); code != 200 {
		t.Fatalf("add-folder: %d %v", code, b)
	}
	c360 := enroll("30:86:2d:ef:2f:ff", "house")
	cabin := enroll("30:86:2d:46:34:8f", "cabin")
	waiting := enroll("e4:d1:24:cf:f4:1f", "")

	state := func(token string, ft []any) {
		t.Helper()
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": 1, "ft": ft}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	bss := func(net, bssid, nasid string) map[string]any {
		return map[string]any{"network": net, "bssid": bssid, "nasid": nasid}
	}
	state(pump, []any{bss("sweet", "bc:ff:4d:02:26:7c", "ap-bcff4d02267a/radio0/5584392f"), bss("sweet", "bc:ff:4d:02:26:7d", "ap-bcff4d02267a/radio1/5584392f"),
		// What is not well said is left out.
		bss("sweet", "BC:FF:4D:02:26:7E", "x"), bss("sweet", "bc:ff:4d:02:26:7f", "has a space"), bss("Sweet", "bc:ff:4d:02:26:80", "x")})
	state(c360, []any{bss("sweet", "30:86:2d:65:4b:00", "ap-30862def2fff/radio1/5584392f")})
	state(cabin, []any{bss("sweet", "30:86:2d:46:34:90", "ap-30862d46348f/radio1/5584392f")})

	poll := func(token string) string {
		t.Helper()
		code, h, _ := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code != 200 {
			t.Fatalf("poll: %d", code)
		}
		return h.Get(FTHeader)
	}
	holders := func(token string) (map[string]any, string) {
		t.Helper()
		code, _, body := f.apDo("GET", "/v1/ap/ft", token, nil, nil)
		if code != 200 {
			t.Fatalf("ft: %d %v", code, body)
		}
		sweet, _ := body["networks"].(map[string]any)["sweet"].(map[string]any)
		return sweet, body["version"].(string)
	}
	bssids := func(n map[string]any) []string {
		var out []string
		for _, h := range n["holders"].([]any) {
			out = append(out, h.(map[string]any)["bssid"].(string))
		}
		return out
	}

	// The house's two APs, each its BSSes, under one key.
	v := poll(pump)
	sweet, version := holders(pump)
	if got := bssids(sweet); len(got) != 3 || got[0] != "30:86:2d:65:4b:00" || got[1] != "bc:ff:4d:02:26:7c" || got[2] != "bc:ff:4d:02:26:7d" {
		t.Fatalf("the house's key holders: %v", got)
	}
	if version != v || poll(c360) != v {
		t.Fatalf("versions: %s, %s, %s", v, version, poll(c360))
	}
	key, _ := sweet["key"].(string)
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(key) {
		t.Fatalf("key %q", key)
	}
	other, _ := holders(c360)
	if other["key"] != key {
		t.Fatal("two APs of a site have the network under different keys")
	}

	// The cabin's AP is a site of its own.
	sweet, _ = holders(cabin)
	if got := bssids(sweet); len(got) != 1 || got[0] != "30:86:2d:46:34:90" || sweet["key"] != key {
		t.Fatalf("the cabin's key holders: %v", sweet)
	}

	// One that changes changes the version; the same said again does not.
	state(c360, []any{bss("sweet", "30:86:2d:65:4b:00", "ap-30862def2fff/radio1/5584392f")})
	if poll(pump) != v {
		t.Fatal("the version changed with nothing new")
	}
	state(c360, []any{bss("sweet", "30:86:2d:65:4b:00", "ap-30862def2fff/radio1/5584392f"), bss("sweet", "30:86:2d:65:4b:01", "ap-30862def2fff/radio0/5584392f")})
	if poll(pump) == v {
		t.Fatal("a new key holder left the version as it was")
	}
	v = poll(pump)

	// Read from the reports again, as by a manager just started.
	f.api.ft.mu.Lock()
	f.api.ft.aps = nil
	f.api.ft.mu.Unlock()
	if poll(pump) != v {
		t.Fatal("the version read from the reports is another")
	}

	// In Landing Zone, nothing.
	if code, h, _ := f.apDo("GET", "/v1/ap/config", waiting, nil, nil); code != 200 || h.Get(FTHeader) != "" {
		t.Fatalf("Landing Zone poll: %d %v", code, h)
	}
	if code, _, _ := f.apDo("GET", "/v1/ap/ft", waiting, nil, nil); code != 409 {
		t.Fatalf("Landing Zone ft: %d", code)
	}
}
