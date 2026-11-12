package api

import (
	"strings"
	"testing"
)

// A state report says the libraries a process holds two builds of at once
// (0317): each as the agent writes it is taken, one not is left out and the
// report taken all the same (0290), and the AP's alerts say it, one a
// library.
func TestReportMixedLibs(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := map[string]any{"version": version, "uptime": 4000, "mixed_libs": []any{
		map[string]any{"lib": "libubox.so", "builds": []string{"20260721", "20261103"}, "procs": []string{"aeolus-agent", "usteerd"}},
		map[string]any{"lib": "libblobmsg_json.so", "builds": []string{"20260721", "20261103"}, "procs": []string{"usteerd"}},
		map[string]any{"lib": "libubox.so", "builds": []string{"20260721"}, "procs": []string{"one build"}},
		map[string]any{"lib": "not a lib", "builds": []string{"1", "2"}, "procs": []string{"x"}},
	}}
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	_, view := f.do("GET", "/v1/alerts", "griff", nil)
	found := map[string]string{}
	for _, a := range view["alerts"].([]any) {
		if m := a.(map[string]any); m["kind"] == "mixed-libs" && m["ap"] == ap {
			found[m["key"].(string)] = m["severity"].(string) + " " + m["message"].(string)
		}
	}
	if len(found) != 2 {
		t.Fatalf("mixed-libs alerts: %v", found)
	}
	if a := found["mixed-libs:libubox.so"]; !strings.HasPrefix(a, "warning aeolus-agent and usteerd hold two builds of libubox.so at once (20260721, 20261103): a package was installed for another image than the AP's") {
		t.Errorf("libubox: %q", a)
	}
	if a := found["mixed-libs:libblobmsg_json.so"]; !strings.HasPrefix(a, "warning usteerd holds two builds of libblobmsg_json.so at once") {
		t.Errorf("blobmsg: %q", a)
	}
}
