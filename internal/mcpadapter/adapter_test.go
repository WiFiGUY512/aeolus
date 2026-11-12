package mcpadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/api"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/changelog"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/schema"
	"github.com/WiFiGUY512/aeolus/internal/secret"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixture struct {
	url    string
	log    *changelog.Log
	tokens map[string]string
}

// newFixture serves the API and the adapter over a fresh log. griff is admin
// (create-org); claude is operator at both roots.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	sch, err := schema.V1()
	must(t, err)
	dir := t.TempDir()
	box, err := secret.LoadOrCreate(filepath.Join(dir, "secret.key"))
	must(t, err)
	log, err := changelog.Open(filepath.Join(dir, "aeolus.db"), changelog.Options{Check: api.Check(sch)})
	must(t, err)
	t.Cleanup(func() { log.Close() })
	for _, op := range []change.Op{
		{Kind: change.CreateOrg, Node: "example", Name: "Example", Account: "griff"},
		{Kind: change.AddFolder, Tree: change.Locations, Node: "house", Name: "House", Parent: "example"},
		{Kind: change.AddFolder, Tree: change.Services, Node: "household", Name: "Household", Parent: "example"},
		{Kind: change.AddAccount, Account: "claude", Name: "Claude"},
		{Kind: change.GrantRole, Account: "claude", Tree: change.Locations, Node: "example", Role: "operator"},
		{Kind: change.GrantRole, Account: "claude", Tree: change.Services, Node: "example", Role: "operator"},
	} {
		if _, err := log.Commit("griff", "fixture", op); err != nil {
			t.Fatal(err)
		}
	}
	tokens := map[string]string{}
	for _, who := range []string{"griff", "claude"} {
		plain, id, hash, err := access.NewToken()
		must(t, err)
		if _, err := log.Commit("griff", "fixture", change.Op{Kind: change.IssueToken, Account: access.AccountID(who), TokenID: id, TokenHash: hash}); err != nil {
			t.Fatal(err)
		}
		tokens[who] = plain
	}
	conds, err := conditions.Open(filepath.Join(dir, "conditions.db"), nil)
	must(t, err)
	t.Cleanup(func() { conds.Close() })
	apiHandler := api.New(log, sch, box, conds).Handler()
	mux := http.NewServeMux()
	mux.Handle("/mcp", New(apiHandler, "test"))
	mux.Handle("/", apiHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &fixture{url: srv.URL, log: log, tokens: tokens}
}

// bearer adds an Authorization header to every request.
type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

func (f *fixture) session(t *testing.T, as string) *mcp.ClientSession {
	t.Helper()
	hc := &http.Client{Transport: bearer{token: f.tokens[as], next: http.DefaultTransport}}
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	s, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: f.url + "/mcp", HTTPClient: hc, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	must(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func call(t *testing.T, s *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	must(t, err)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return res, text.String()
}

func TestRequestWithoutATokenIsRefused(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Post(f.url+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	must(t, err)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func TestToolsActAsTheCaller(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	tools, err := s.ListTools(context.Background(), nil)
	must(t, err)
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	want := "ap_action ask_assurance_round end_change get_advice get_ap_config get_ap_history get_assurance get_client_connections get_client_journey get_compatibility get_leaving get_library get_minutes get_network get_node get_power get_recording get_relayed_dhcp get_roaming get_steering get_usage list_airspace list_alert_history list_alerts list_changes list_clients list_detected list_ends list_keys list_recordings list_suggestions list_threats list_tree make_change name_client preview_change set_finding trace_client whoami"
	if strings.Join(names, " ") != want {
		t.Fatalf("tools = %v", names)
	}
	if _, text := call(t, s, "whoami", map[string]any{}); !strings.Contains(text, `"account":"claude"`) {
		t.Fatalf("whoami = %s", text)
	}
	// set_finding reaches the API as the caller (0215): a finding that is none
	// is refused there.
	if res, text := call(t, s, "set_finding", map[string]any{"id": 999, "state": "resolved", "why": "fixed"}); !res.IsError || !strings.Contains(text, "not found") {
		t.Fatalf("set_finding of none = %v %s", res.IsError, text)
	}
	// name_client too (0153): a client never seen is no one's to name.
	if res, text := call(t, s, "name_client", map[string]any{"mac": "aa:bb:cc:dd:ee:01", "name": "kettle"}); !res.IsError {
		t.Fatalf("name_client of one never seen = %v %s", res.IsError, text)
	}
}

func TestChangeIsLoggedUnderTheCallersNameWithTheReason(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	res, text := call(t, s, "make_change", map[string]any{
		"op":     map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 40},
		"reason": "wider channels for the house",
	})
	if res.IsError {
		t.Fatalf("make_change failed: %s", text)
	}
	entries, err := f.log.Entries(f.log.Seq()-1, 1)
	must(t, err)
	if e := entries[0]; e.Actor != "claude" || e.Reason != "wider channels for the house" || e.Op.Path != "radio.5g.width" {
		t.Fatalf("logged %+v", e)
	}
}

// A change needs no reason (0062): the log records who made it and when, and
// the tool's schema does not ask for one.
func TestChangeNeedsNoReason(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	tools, err := s.ListTools(context.Background(), nil)
	must(t, err)
	for _, tool := range tools.Tools {
		if tool.Name != "make_change" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		must(t, err)
		var schema struct {
			Required []string `json:"required"`
		}
		must(t, json.Unmarshal(raw, &schema))
		if strings.Join(schema.Required, " ") != "op" {
			t.Fatalf("make_change requires %v, want only op", schema.Required)
		}
	}
	for _, args := range []map[string]any{
		{"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 40}},
		{"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 80}, "reason": "  "},
	} {
		before := f.log.Seq()
		res, text := call(t, s, "make_change", args)
		if res.IsError {
			t.Fatalf("make_change %v failed: %s", args, text)
		}
		entries, err := f.log.Entries(before, 1)
		must(t, err)
		if e := entries[0]; e.Actor != "claude" || e.Reason != "" || e.At.IsZero() || e.Op.Path != "radio.5g.width" {
			t.Fatalf("logged %+v", e)
		}
	}
}

func TestRefusalsAreToolErrors(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"op": map[string]any{"kind": "lock", "tree": "locations", "node": "example", "path": "radio.5g.width"}, "reason": "x"}, "needs admin"},
		{map[string]any{"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 33}, "reason": "x"}, "value must be one of"},
	}
	for _, c := range cases {
		res, text := call(t, s, "make_change", c.args)
		if !res.IsError || !strings.Contains(text, c.want) {
			t.Errorf("%v: IsError=%v, text %q, want %q", c.args, res.IsError, text, c.want)
		}
	}
}

func TestPreviewRecordsNothing(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	seq := f.log.Seq()
	res, text := call(t, s, "preview_change", map[string]any{
		"op": map[string]any{"kind": "set", "tree": "locations", "node": "house", "path": "radio.5g.width", "value": 80},
	})
	if res.IsError || !strings.Contains(text, `"effect"`) {
		t.Fatalf("preview: %s", text)
	}
	if f.log.Seq() != seq {
		t.Fatal("preview committed a change")
	}
}

func TestSecretsStaySealed(t *testing.T) {
	f := newFixture(t)
	s := f.session(t, "claude")
	const pass = "a-secret-passphrase-9"
	res, text := call(t, s, "make_change", map[string]any{
		"op":     map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.passphrase", "value": pass},
		"reason": "set the household passphrase",
	})
	if res.IsError || strings.Contains(text, pass) {
		t.Fatalf("make_change: error=%v, text %s", res.IsError, text)
	}
	for _, args := range []map[string]any{{"tree": "services", "node": "household"}} {
		_, text := call(t, s, "get_node", args)
		if strings.Contains(text, pass) || !strings.Contains(text, `"sealed":true`) {
			t.Fatalf("get_node shows the passphrase or not the seal: %s", text)
		}
	}
	_, text = call(t, s, "list_changes", map[string]any{"limit": 1000})
	if strings.Contains(text, pass) {
		t.Fatal("list_changes shows the passphrase")
	}
	var out map[string]any
	must(t, json.Unmarshal([]byte(text), &out))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestTraceAClient: Live Trace by its tool (0144): read, start, read again,
// stop, as the caller.
func TestTraceAClient(t *testing.T) {
	f := newFixture(t)
	if _, err := f.log.Commit("griff", "fixture", change.Op{Kind: change.AddAP, Tree: change.Locations, Node: "house-ap", Name: "HouseAP", Parent: "house"}); err != nil {
		t.Fatal(err)
	}
	s := f.session(t, "claude")
	const mac = "7e:2a:ea:04:b1:8f"
	if res, text := call(t, s, "trace_client", map[string]any{"mac": mac}); res.IsError || !strings.Contains(text, `"tracing":false`) {
		t.Fatalf("before: %v %s", res.IsError, text)
	}
	if res, text := call(t, s, "trace_client", map[string]any{"mac": mac, "do": "start", "minutes": 5}); res.IsError || !strings.Contains(text, `"tracing":true`) || !strings.Contains(text, `"by":"claude"`) {
		t.Fatalf("start: %v %s", res.IsError, text)
	}
	if res, text := call(t, s, "trace_client", map[string]any{"mac": mac, "after": 3}); res.IsError || !strings.Contains(text, `"tracing":true`) || !strings.Contains(text, `"aps":1`) {
		t.Fatalf("read: %v %s", res.IsError, text)
	}
	if res, text := call(t, s, "trace_client", map[string]any{"mac": mac, "do": "stop"}); res.IsError || !strings.Contains(text, `"tracing":false`) {
		t.Fatalf("stop: %v %s", res.IsError, text)
	}
	if res, text := call(t, s, "trace_client", map[string]any{"mac": mac, "do": "pause"}); !res.IsError || !strings.Contains(text, "start, stop") {
		t.Errorf("a thing it cannot do: %v %s", res.IsError, text)
	}
	if res, text := call(t, s, "trace_client", map[string]any{"mac": "not a mac"}); !res.IsError || !strings.Contains(text, "MAC") {
		t.Errorf("no MAC: %v %s", res.IsError, text)
	}
}
