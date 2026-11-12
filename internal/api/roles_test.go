package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"
)

// The roles at the API (0177): what a node's page and whoami say each may
// do, and what each may ask of an AP.
func TestRolesAtTheAPI(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	const mac = "72:bb:66:9f:76:94"
	role := func(kind, role string) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": kind, "account": "tenant", "tree": "locations", "node": "office", "role": role}); code != 200 {
			t.Fatalf("%s %s: %d %v", kind, role, code, b)
		}
	}
	strs := func(v any) []string {
		var out []string
		for _, s := range v.([]any) {
			out = append(out, s.(string))
		}
		return out
	}
	for _, c := range []struct {
		role string
		may  string
		// The code each asking gets.
		locate, reconnect, trace, capture int
	}{
		{"viewer", "[view]", 403, 403, 403, 403},
		{"installer", "[view aps ports actions test-ssid]", 200, 403, 403, 403},
		{"manager", "[view keys clients sign-ins]", 403, 200, 200, 403},
		{"operator", "[view configure keys clients aps ports actions test-ssid]", 200, 200, 200, 403},
		{"admin", "[view administer configure keys clients aps ports actions capture shell test-ssid sign-ins]", 200, 200, 200, 400},
	} {
		role("grant", c.role)
		_, page := f.do("GET", "/v1/trees/locations/nodes/"+ap, "tenant", nil)
		if got := fmt.Sprint(strs(page["may"])); got != c.may || page["role"] != c.role {
			t.Errorf("%s: the AP's page says %v, may %s; want may %s", c.role, page["role"], got, c.may)
		}
		_, who := f.do("GET", "/v1/whoami", "tenant", nil)
		if got, want := fmt.Sprint(strs(who["anywhere"])), c.may; got != want {
			t.Errorf("%s: whoami anywhere %s, want %s", c.role, got, want)
		}
		if rights := who["rights"].(map[string]any); len(rights) != 5 || !slices.Contains(strs(rights["installer"]), "ports") {
			t.Errorf("%s: whoami's rights: %v", c.role, rights)
		}
		for what, ask := range map[string]struct {
			path string
			body map[string]any
			want int
		}{
			"locate":    {"/v1/aps/" + ap + "/actions", map[string]any{"kind": "locate"}, c.locate},
			"reconnect": {"/v1/aps/" + ap + "/actions", map[string]any{"kind": "disconnect", "target": mac}, c.reconnect},
			"trace":     {"/v1/clients/" + mac + "/trace", map[string]any{}, c.trace},
			// Asked of nothing, so an admin is told what is missing.
			"capture": {"/v1/aps/" + ap + "/pcaps", map[string]any{}, c.capture},
		} {
			if code, b := f.do("POST", ask.path, "tenant", ask.body); code != ask.want {
				t.Errorf("%s asking for a %s: %d %v, want %d", c.role, what, code, b, ask.want)
			}
		}
		f.do("DELETE", "/v1/clients/"+mac+"/trace", "tenant", nil)
		role("revoke", c.role)
	}
}

// whoami says the manager's own release, for the page to show (and none
// where it was not told one).
func TestWhoamiSaysTheRelease(t *testing.T) {
	f := newFixture(t)
	if _, b := f.do("GET", "/v1/whoami", "tenant", nil); b["version"] != "" {
		t.Errorf("untold, whoami says version %v", b["version"])
	}
	// Told one, asked in this goroutine: the server is not changed under a
	// request in flight.
	f.api.WithVersion("v0.130.0")
	req := httptest.NewRequest("GET", "/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+f.tokens["tenant"])
	rec := httptest.NewRecorder()
	f.api.Handler().ServeHTTP(rec, req)
	var b map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || b["version"] != "v0.130.0" {
		t.Errorf("whoami says %d %v", rec.Code, b)
	}
}
