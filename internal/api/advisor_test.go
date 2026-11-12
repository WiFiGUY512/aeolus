package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WiFiGUY512/aeolus/internal/advisor"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/mcpadapter"
)

// The advisor (0205), with Anthropic's API stood in for: it reads as its
// own account through the MCP adapter, which the API holds to that
// account's roles and to the folder whose advisor it is (0212); its key is
// an admin's to set, at a folder, and no AP's; and what it finds is read
// and dismissed through the API, by those who may view and configure that
// folder.
func TestAdvisor(t *testing.T) {
	f := newFixture(t)
	h := f.api.Handler()
	as := func(method, path string, body any) int {
		t.Helper()
		var rd io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			rd = strings.NewReader(string(raw))
		}
		req := httptest.NewRequest(method, path, rd)
		rec := httptest.NewRecorder()
		As(h, advisor.Account).ServeHTTP(rec, req)
		return rec.Code
	}
	// No such account yet: it is nobody.
	if code := as("GET", "/v1/whoami", nil); code != 401 {
		t.Fatalf("whoami before the account: %d", code)
	}
	if code, b := f.change("griff", map[string]any{"kind": "add-account", "account": advisor.Account, "name": "The advisor"}); code != 200 {
		t.Fatalf("add-account: %d %v", code, b)
	}
	for _, tree := range []string{"locations", "services"} {
		if code, b := f.change("griff", map[string]any{"kind": "grant", "account": advisor.Account, "tree": tree, "node": "example", "role": "viewer"}); code != 200 {
			t.Fatalf("grant: %d %v", code, b)
		}
	}
	if code := as("GET", "/v1/whoami", nil); code != 200 {
		t.Fatalf("whoami as the advisor: %d", code)
	}
	// A viewer changes nothing, whatever it asks.
	if code := as("POST", "/v1/changes", map[string]any{"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 20}}); code != 403 {
		t.Fatalf("a change as the advisor: %d", code)
	}

	// Another folder beside House, with its own AP: House's advisor reads
	// nothing of it (0212).
	for _, op := range []map[string]any{
		{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "example"},
		{"kind": "add-ap", "tree": "locations", "node": "annex-ap", "name": "AnnexAP", "parent": "annex"},
	} {
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op["kind"], code, b)
		}
	}
	// Before any key, there is no advisor: nothing to show.
	if _, v := f.do("GET", "/v1/advisor", "claude", nil); len(v["scopes"].([]any)) != 0 {
		t.Fatalf("advisors before a key: %v", v["scopes"])
	}

	// The key is an admin's to set, at House, sealed, and sent to no AP.
	key := map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "system.advisor.key", "value": "sk-ant-test-0123456789abcdef"}
	if code, _ := f.change("claude", key); code != 403 {
		t.Fatalf("an operator set the key: %d", code)
	}
	if code, b := f.change("griff", key); code != 200 {
		t.Fatalf("the key: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "house", "values": map[string]any{"system.advisor.web": false, "system.advisor.budget": 1.5}}); code != 200 {
		t.Fatalf("settings: %d %v", code, b)
	}
	_, cfg := f.do("GET", "/v1/aps/office-ap/config", "griff", nil)
	if raw, _ := json.Marshal(cfg["document"]); strings.Contains(string(raw), "advisor") {
		t.Fatalf("an AP's config has the advisor's settings: %s", raw)
	}
	if raw, _ := json.Marshal(cfg); strings.Contains(string(raw), "sk-ant-test") {
		t.Fatalf("the key is shown: %s", raw)
	}

	// Anthropic's API, stood in for: a read, a finding, and a summary.
	var mu sync.Mutex
	var asked []map[string]any
	turns := []string{
		`{"type":"tool_use","id":"t1","name":"whoami","input":{}},{"type":"tool_use","id":"t2","name":"list_tree","input":{"tree":"locations"}}`,
		`{"type":"tool_use","id":"t3","name":"record_finding","input":{"key":"office-dhcp-slow","kind":"network","severity":"warning","title":"DHCP on VLAN 20 is slow","saw":"2 s offers","todo":"Look at the DHCP server","clients":[],"aps":["office-ap"],"networks":["sweet"]}}`,
		`{"type":"text","text":"One slow DHCP server; the rest is well."}`,
	}
	claude := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, req)
		stop := "tool_use"
		if len(turns) == 1 {
			stop = "end_turn"
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","content":[`+turns[0]+`],"stop_reason":"`+stop+
			`","usage":{"input_tokens":100,"output_tokens":100,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`)
		turns = turns[1:]
	}))
	defer claude.Close()
	f.api.adv.baseURL = claude.URL
	f.api.WithAdvisor(func(h http.Handler) *mcp.Server { return mcpadapter.Server(h, "test") })

	// Only an admin of House asks for a review: it costs House's key's money.
	if code, _ := f.do("POST", "/v1/advisor/run", "claude", map[string]any{"scope": "house"}); code != 404 {
		t.Fatalf("an operator asked for a review: %d", code)
	}
	if code, b := f.do("POST", "/v1/advisor/run", "griff", map[string]any{"scope": "annex"}); code != 409 {
		t.Fatalf("a review of a folder with no key: %d %v", code, b)
	}
	if code, b := f.do("POST", "/v1/advisor/run", "griff", map[string]any{"scope": "house"}); code != 202 {
		t.Fatalf("a review: %d %v", code, b)
	}
	var view map[string]any
	for deadline := time.Now().Add(10 * time.Second); ; {
		_, view = f.do("GET", "/v1/advisor", "claude", nil)
		if runs, _ := view["runs"].([]any); view["running"] == false && len(runs) == 1 && runs[0].(map[string]any)["outcome"] != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the review did not end: %v", view)
		}
		time.Sleep(20 * time.Millisecond)
	}
	run := view["runs"].([]any)[0].(map[string]any)
	if run["outcome"] != "done" || run["summary"] != "One slow DHCP server; the rest is well." || run["why"] != "asked by griff" || run["requests"] != float64(3) {
		t.Fatalf("the review = %v", run)
	}
	scopes := view["scopes"].([]any)
	if len(scopes) != 1 || view["scope"] != "house" || view["account_set"] != true {
		t.Fatalf("advisors = %v, scope %v", scopes, view["scope"])
	}
	set := scopes[0].(map[string]any)
	if set["scope"] != "house" || set["name"] != "House" || set["web"] != false || set["budget"] != 1.5 || set["model"] != "claude-opus-5-5" ||
		set["may_review"] != false || set["may_dismiss"] != true || set["open"] != float64(1) {
		t.Fatalf("House's advisor = %v", set)
	}
	if raw, _ := json.Marshal(view); strings.Contains(string(raw), "sk-ant-test") {
		t.Fatal("the key is shown")
	}

	// What it was offered: the adapter's reads, and no change, no action.
	var names []string
	for _, tool := range asked[0]["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	for _, never := range []string{"make_change", "ap_action", "preview_change", "ask_assurance_round", "trace_client", "get_ap_history", "web_search"} {
		if strings.Contains(" "+strings.Join(names, " ")+" ", " "+never+" ") {
			t.Fatalf("offered %s: %v", never, names)
		}
	}
	for _, want := range []string{"whoami", "list_alerts", "get_client_journey", "get_compatibility", "get_network", "record_finding"} {
		if !strings.Contains(" "+strings.Join(names, " ")+" ", " "+want+" ") {
			t.Fatalf("not offered %s: %v", want, names)
		}
	}
	// It read as itself.
	results := asked[1]["messages"].([]any)[2].(map[string]any)["content"].([]any)
	first := results[0].(map[string]any)
	if raw, _ := json.Marshal(first["content"]); !strings.Contains(string(raw), advisor.Account) || first["is_error"] == true {
		t.Fatalf("whoami came to %s", raw)
	}
	// It read House's tree, and nothing of the Annex, nor above House.
	tree, _ := json.Marshal(results[1].(map[string]any)["content"])
	if !strings.Contains(string(tree), "office-ap") || strings.Contains(string(tree), "annex") || strings.Contains(string(tree), `\"id\":\"example\"`) {
		t.Fatalf("House's advisor read the tree as %s", tree)
	}

	findings := view["findings"].([]any)
	if len(findings) != 1 || findings[0].(map[string]any)["key"] != "office-dhcp-slow" || findings[0].(map[string]any)["state"] != "open" ||
		findings[0].(map[string]any)["scope"] != "house" {
		t.Fatalf("findings = %v", findings)
	}
	id := int64(findings[0].(map[string]any)["id"].(float64))
	path := "/v1/advisor/findings/" + findingID(id)
	if code, _ := f.do("POST", path, "tenant", map[string]any{"state": "dismissed"}); code != 404 {
		t.Fatalf("a tenant dismissed it: %d", code)
	}
	if code, b := f.do("POST", path, "claude", map[string]any{"state": "dismissed"}); code != 200 {
		t.Fatalf("dismiss: %d %v", code, b)
	}
	if code, _ := f.do("POST", path, "claude", map[string]any{"state": "gone"}); code != 400 {
		t.Fatalf("a state that is none: %d", code)
	}
	_, view = f.do("GET", "/v1/advisor", "claude", nil)
	if fd := view["findings"].([]any)[0].(map[string]any); fd["state"] != "dismissed" || fd["state_by"] != "claude" {
		t.Fatalf("dismissed = %v", fd)
	}
	// An operator who fixed it says so, and why (0215); why is for that alone.
	if code, _ := f.do("POST", path, "claude", map[string]any{"state": "dismissed", "why": "x"}); code != 400 {
		t.Fatalf("why with dismissed: %d", code)
	}
	if code, b := f.do("POST", path, "claude", map[string]any{"state": "resolved", "why": " moved the DHCP server "}); code != 200 {
		t.Fatalf("resolve: %d %v", code, b)
	}
	_, view = f.do("GET", "/v1/advisor", "claude", nil)
	if fd := view["findings"].([]any)[0].(map[string]any); fd["state"] != "resolved" || fd["state_by"] != "claude" || fd["state_why"] != "moved the DHCP server" {
		t.Fatalf("resolved = %v", fd)
	}
	if set := view["scopes"].([]any)[0].(map[string]any); set["open"] != float64(0) {
		t.Fatalf("a resolved finding counted open: %v", set)
	}

	// A question, answered by a review of its own (0215).
	mu.Lock()
	turns, asked = []string{`{"type":"text","text":"The DHCP server at 192.168.20.254 is slow."}`}, nil
	mu.Unlock()
	if code, _ := f.do("POST", "/v1/advisor/run", "griff", map[string]any{"scope": "house", "question": "  "}); code != 400 {
		t.Fatalf("an empty question: %d", code)
	}
	if code, b := f.do("POST", "/v1/advisor/run", "griff", map[string]any{"scope": "house", "question": "Why is DHCP slow?"}); code != 202 || b["question"] != true {
		t.Fatalf("a question: %d %v", code, b)
	}
	for deadline := time.Now().Add(10 * time.Second); ; {
		_, view = f.do("GET", "/v1/advisor", "claude", nil)
		if runs, _ := view["runs"].([]any); view["running"] == false && len(runs) == 2 && runs[0].(map[string]any)["outcome"] != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the question was not answered: %v", view)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if run := view["runs"].([]any)[0].(map[string]any); run["question"] != "Why is DHCP slow?" || run["summary"] != "The DHCP server at 192.168.20.254 is slow." || run["outcome"] != "done" {
		t.Fatalf("the answer = %v", run)
	}
	mu.Lock()
	opening, _ := json.Marshal(asked[0]["messages"])
	mu.Unlock()
	if !strings.Contains(string(opening), "Why is DHCP slow?") {
		t.Fatalf("the question was not asked: %s", opening)
	}
	// The tenant views no folder of Locations: no advisor's to see.
	if code, v := f.do("GET", "/v1/advisor", "tenant", nil); code != 200 || len(v["scopes"].([]any)) != 0 || len(v["findings"].([]any)) != 0 {
		t.Fatalf("a tenant read the advice: %d %v", code, v)
	}
	if code, _ := f.do("GET", "/v1/advisor?scope=annex", "claude", nil); code != 404 {
		t.Fatalf("an advisor that is none: %d", code)
	}
}

// What a folder's advisor reads (0212): its folder and below, but a folder
// below with an advisor of its own; and the service folders its APs use,
// with those above and below them, and no other.
func TestAdvisorHold(t *testing.T) {
	f := newFixture(t)
	for _, op := range []map[string]any{
		{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "example"},
		{"kind": "set", "tree": "locations", "node": "example", "path": "system.advisor.key", "value": "sk-ant-org-0123456789abcdef"},
		{"kind": "set", "tree": "locations", "node": "office", "path": "system.advisor.key", "value": "sk-ant-office-0123456789abc"},
	} {
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op["kind"], code, b)
		}
	}
	state := f.api.log.Snapshot()
	if got := advisorScopes(state); len(got) != 2 || got[0] != "example" || got[1] != "office" {
		t.Fatalf("advisors = %v", got)
	}
	if scopeOf(state, "office-ap") != "office" || scopeOf(state, "annex") != "example" || scopeOf(state, "house") != "example" {
		t.Fatalf("scopeOf: %s %s %s", scopeOf(state, "office-ap"), scopeOf(state, "annex"), scopeOf(state, "house"))
	}
	L, S := state.Org.Locations, state.Org.Services
	org, office := holdOf(state, "example"), holdOf(state, "office")
	for _, c := range []struct {
		hold *folderHold
		tree change.TreeName
		t    *hierarchy.Tree
		node hierarchy.NodeID
		want bool
	}{
		{org, change.Locations, L, "example", true},
		{org, change.Locations, L, "annex", true},
		{org, change.Locations, L, "house", true},
		{org, change.Locations, L, "office", false},
		{org, change.Locations, L, "office-ap", false},
		{office, change.Locations, L, "office", true},
		{office, change.Locations, L, "office-ap", true},
		{office, change.Locations, L, "house", false},
		{office, change.Locations, L, "annex", false},
		{office, change.Services, S, "household", true},
		{office, change.Services, S, "example", true},
		{office, change.Services, S, "guest", false},
	} {
		if got := c.hold.covers(c.tree, c.t.Ancestry(c.node)); got != c.want {
			t.Errorf("%s's advisor reads %s %s: %v, want %v", c.hold.node, c.tree, c.node, got, c.want)
		}
	}
}

func findingID(n int64) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

// The schedule: a review when the advisor is on, has a key, and the hours
// between reviews have passed.
func TestReviewDue(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	on := advisorSettings{Enabled: true, KeySet: true, Every: 24}
	for _, c := range []struct {
		name string
		set  advisorSettings
		last *time.Time
		want bool
	}{
		{"never reviewed", on, nil, true},
		{"reviewed an hour ago", on, ptrTime(now.Add(-time.Hour)), false},
		{"reviewed a day ago", on, ptrTime(now.Add(-24 * time.Hour)), true},
		{"off", advisorSettings{KeySet: true, Every: 24}, nil, false},
		{"no key", advisorSettings{Enabled: true, Every: 24}, nil, false},
	} {
		var last *conditions.AdviceRun
		if c.last != nil {
			last = &conditions.AdviceRun{Started: *c.last}
		}
		if got := reviewDue(c.set, last, now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
