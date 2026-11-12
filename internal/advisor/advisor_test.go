package advisor

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
)

// reads is a stand-in for the MCP adapter: a read, a read left out, and a
// change, which a review must never be offered.
func fakeReads() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "aeolus", Version: "test"}, nil)
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}
	type underIn struct {
		Under string `json:"under,omitempty" jsonschema:"a Locations folder or AP"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "list_alerts", Description: "What needs attention.", Annotations: ro},
		func(ctx context.Context, _ *mcp.CallToolRequest, in underIn) (*mcp.CallToolResult, any, error) {
			return nil, map[string]any{"alerts": []any{map[string]any{"ap": "ap-1", "kind": "rejoins", "message": "client aa:bb loops"}}, "under": in.Under}, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "preview_change", Description: "Preview a change.", Annotations: ro},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ underIn) (*mcp.CallToolResult, any, error) {
			return nil, map[string]any{}, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "make_change", Description: "Make one change."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ underIn) (*mcp.CallToolResult, any, error) {
			return nil, map[string]any{"made": true}, nil
		})
	return s
}

// claude is a stand-in for Anthropic's API: it answers each request with
// the next of turns, and keeps what it was asked.
type claude struct {
	mu    sync.Mutex
	turns []string
	asked []map[string]any
	beta  []string
}

func (c *claude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked = append(c.asked, req)
	c.beta = append(c.beta, r.Header.Get("anthropic-beta"))
	if len(c.turns) == 0 {
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"no more turns"}}`, 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, c.turns[0])
	c.turns = c.turns[1:]
}

func turn(stop string, content string) string {
	return `{"id":"msg","type":"message","role":"assistant","model":"claude-opus-5-5","content":[` + content + `],"stop_reason":"` + stop +
		`","stop_details":null,"usage":{"input_tokens":1000,"output_tokens":500,"cache_read_input_tokens":20000,"cache_creation_input_tokens":3000,"server_tool_use":{"web_search_requests":1,"web_fetch_requests":0}}}`
}

func store(t *testing.T) *conditions.Store {
	t.Helper()
	s, err := conditions.Open(filepath.Join(t.TempDir(), "conditions.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

const good = `{"key":"robots-rejoin-c360","kind":"compatibility","severity":"warning","title":"IoT devices rejoin on the C-360s","saw":"20 joins an hour","todo":"Look at the WAN","clients":["aa:bb:cc:dd:ee:ff"],"aps":["ap-1"],"networks":["iot"]}`

func TestAReviewReadsAndRecords(t *testing.T) {
	c := &claude{turns: []string{
		turn("tool_use", `{"type":"thinking","thinking":"","signature":"sig"},{"type":"tool_use","id":"t1","name":"list_alerts","input":{"under":"house"}},`+
			`{"type":"tool_use","id":"t2","name":"make_change","input":{}},{"type":"tool_use","id":"t3","name":"record_finding","input":{"key":"Bad Key"}}`),
		turn("tool_use", `{"type":"tool_use","id":"t4","name":"record_finding","input":`+good+`}`),
		turn("tool_use", `{"type":"tool_use","id":"t5","name":"record_finding","input":`+strings.Replace(good, "20 joins", "25 joins", 1)+`}`),
		turn("end_turn", `{"type":"text","text":"Two robots loop; the rest is well."}`),
	}}
	api := httptest.NewServer(c)
	defer api.Close()
	st := store(t)
	run, err := Review(context.Background(), Settings{Key: "sk-test-key-0123456789", Model: "claude-opus-5-5", Web: true, Budget: 2, BaseURL: api.URL},
		fakeReads(), st, "asked by griff", "", time.Date(2026, 10, 22, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if run.Outcome != "done" || run.Summary != "Two robots loop; the rest is well." || run.Requests != 4 {
		t.Fatalf("run = %+v", run)
	}
	// Each request: 1000 in at $4, 500 out at $20, 20000 read at $0.20,
	// 3000 written at $5, and a search at a cent.
	if want := 4 * (0.004 + 0.010 + 0.004 + 0.015 + 0.01); math.Abs(run.Cost-want) > 1e-9 {
		t.Fatalf("cost = %v, want %v", run.Cost, want)
	}

	first := c.asked[0]
	var names []string
	for _, tool := range first["tools"].([]any) {
		m := tool.(map[string]any)
		name, _ := m["name"].(string)
		names = append(names, name)
		if (name == "record_finding" || name == "resolve_finding") && m["strict"] != true {
			t.Errorf("record_finding is not strict: %v", m)
		}
	}
	if strings.Join(names, " ") != "list_alerts record_finding resolve_finding web_search" {
		t.Fatalf("tools offered: %v", names)
	}
	sys := first["system"].([]any)[0].(map[string]any)
	if sys["cache_control"] == nil || !strings.Contains(sys["text"].(string), "you never change anything") {
		t.Fatalf("system = %v", sys)
	}
	if first["model"] != "claude-opus-5-5" || first["thinking"].(map[string]any)["type"] != "adaptive" ||
		first["output_config"].(map[string]any)["effort"] != "high" || first["fallbacks"] != "default" || first["cache_control"] == nil {
		t.Fatalf("request = %v", first)
	}
	if !strings.Contains(c.beta[0], "server-side-fallback-2026-07-01") || !strings.Contains(c.beta[0], "task-budgets-2026-03-13") {
		t.Fatalf("betas = %q", c.beta[0])
	}
	open := first["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(open, "2026-10-22T04:00:00Z") || !strings.Contains(open, "No findings are recorded yet") {
		t.Fatalf("opening = %q", open)
	}

	// The second request carries the first answer, thinking and all, and
	// what each tool came to: the read, the change refused as no tool, and
	// the finding refused for its key.
	msgs := c.asked[1]["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("second request has %d messages", len(msgs))
	}
	if kind := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)["type"]; kind != "thinking" {
		t.Fatalf("the thinking block was not kept: %v", kind)
	}
	results := msgs[2].(map[string]any)["content"].([]any)
	got := func(i int) (string, bool) {
		r := results[i].(map[string]any)
		var text string
		switch c := r["content"].(type) {
		case string:
			text = c
		case []any:
			text = c[0].(map[string]any)["text"].(string)
		}
		isErr, _ := r["is_error"].(bool)
		return text, isErr
	}
	if text, isErr := got(0); isErr || !strings.Contains(text, "client aa:bb loops") || !strings.Contains(text, "house") {
		t.Fatalf("list_alerts came to %q, %v", text, isErr)
	}
	if text, isErr := got(1); !isErr || !strings.Contains(text, "no tool") {
		t.Fatalf("make_change came to %q, %v", text, isErr)
	}
	if text, isErr := got(2); !isErr || !strings.Contains(text, "key:") {
		t.Fatalf("a bad finding came to %q, %v", text, isErr)
	}

	// One finding, found twice: what it said last, counted twice.
	fs, err := st.Findings("", 10)
	if err != nil || len(fs) != 1 {
		t.Fatalf("findings = %v, %v", fs, err)
	}
	if f := fs[0]; f.Key != "robots-rejoin-c360" || f.Times != 2 || f.Saw != "25 joins an hour" || f.State != "open" || f.Run != run.ID ||
		len(f.Subjects.Clients) != 1 || f.Subjects.APs[0] != "ap-1" {
		t.Fatalf("finding = %+v", f)
	}
	runs, _ := st.AdviceRuns("", 5)
	if len(runs) != 1 || runs[0].Outcome != "done" || runs[0].Ended == nil || runs[0].Why != "asked by griff" {
		t.Fatalf("runs = %+v", runs)
	}
}

// The next review is told what is recorded, and a dismissed finding found
// again stays dismissed, unless it is worse.
func TestAReviewKnowsWhatIsRecorded(t *testing.T) {
	st := store(t)
	id, _, _ := st.RecordFinding(1, conditions.Finding{Key: "robots-rejoin-c360", Kind: "compatibility", Severity: "warning", Title: "Robots", Saw: "x", Todo: "y"})
	st.SetFindingState(id, "dismissed", "griff", "")
	c := &claude{turns: []string{
		turn("tool_use", `{"type":"tool_use","id":"t1","name":"record_finding","input":`+good+`}`),
		turn("end_turn", `{"type":"text","text":"Done."}`),
	}}
	api := httptest.NewServer(c)
	defer api.Close()
	if _, err := Review(context.Background(), Settings{Key: "sk-test-key-0123456789", Budget: 2, BaseURL: api.URL}, fakeReads(), st, "schedule", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	open := c.asked[0]["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(open, "robots-rejoin-c360 [dismissed, warning, compatibility") {
		t.Fatalf("opening = %q", open)
	}
	if _, ok := c.asked[0]["tools"].([]any); !ok {
		t.Fatal("no tools")
	}
	for _, tool := range c.asked[0]["tools"].([]any) {
		if tool.(map[string]any)["name"] == "web_search" {
			t.Fatal("web search offered with web off")
		}
	}
	fs, _ := st.Findings("", 10)
	if len(fs) != 1 || fs[0].State != "dismissed" || fs[0].Times != 2 || fs[0].Saw != "x" {
		t.Fatalf("a dismissed finding found again, no worse: %+v", fs)
	}
	if _, _, err := st.RecordFinding(2, conditions.Finding{Key: "robots-rejoin-c360", Kind: "compatibility", Severity: "critical", Title: "Robots", Saw: "worse", Todo: "y"}); err != nil {
		t.Fatal(err)
	}
	if fs, _ = st.Findings("", 10); fs[0].State != "open" || fs[0].Saw != "worse" || fs[0].StateBy != "" {
		t.Fatalf("a dismissed finding found worse: %+v", fs[0])
	}
}

// A review is told what each open finding concerns and saw, and to look
// again; it resolves one whose problem is gone, which a later review that
// finds it again opens again (0215).
func TestAReviewResolves(t *testing.T) {
	st := store(t)
	st.RecordFinding(1, conditions.Finding{Key: "robots-rejoin-c360", Kind: "compatibility", Severity: "warning", Title: "Robots",
		Saw: "20 joins an hour  on the C-360s", Todo: "y", Subjects: conditions.Subjects{Clients: []string{"aa:bb:cc:dd:ee:ff"}, APs: []string{"ap-1"}}})
	gone, _, _ := st.RecordFinding(1, conditions.Finding{Key: "old-one", Kind: "ap", Severity: "info", Title: "Old", Saw: "gone by itself", Todo: "y"})
	st.SetFindingState(gone, "dismissed", "griff", "")
	c := &claude{turns: []string{
		turn("tool_use", `{"type":"tool_use","id":"t1","name":"resolve_finding","input":{"key":"robots-rejoin-c360","why":"no rejoins in 24 h"}},`+
			`{"type":"tool_use","id":"t2","name":"resolve_finding","input":{"key":"old-one","why":"x"}},`+
			`{"type":"tool_use","id":"t3","name":"resolve_finding","input":{"key":"never-was","why":"x"}},`+
			`{"type":"tool_use","id":"t4","name":"resolve_finding","input":{"key":"robots-rejoin-c360","why":""}}`),
		turn("end_turn", `{"type":"text","text":"All well."}`),
	}}
	api := httptest.NewServer(c)
	defer api.Close()
	run, err := Review(context.Background(), Settings{Key: "sk-test-key-0123456789", Budget: 2, BaseURL: api.URL}, fakeReads(), st, "schedule", "", time.Now())
	if err != nil || run.Outcome != "done" {
		t.Fatalf("run = %+v, %v", run, err)
	}
	open := c.asked[0]["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	for _, want := range []string{"Concerns: clients aa:bb:cc:dd:ee:ff; APs ap-1", "Saw: 20 joins an hour on the C-360s", "Look again at each open finding"} {
		if !strings.Contains(open, want) {
			t.Errorf("the opening has no %q: %q", want, open)
		}
	}
	if strings.Contains(open, "gone by itself") {
		t.Errorf("a dismissed finding's evidence was given: %q", open)
	}
	results := c.asked[1]["messages"].([]any)[2].(map[string]any)["content"].([]any)
	for i, want := range []struct {
		text  string
		isErr bool
	}{{"resolved, finding 1", false}, {"no open finding", true}, {"no open finding", true}, {"why: 1 to 2000", true}} {
		r := results[i].(map[string]any)
		text, _ := r["content"].(string)
		if c, ok := r["content"].([]any); ok {
			text = c[0].(map[string]any)["text"].(string)
		}
		isErr, _ := r["is_error"].(bool)
		if !strings.Contains(text, want.text) || isErr != want.isErr {
			t.Errorf("resolve %d came to %q, %v", i, text, isErr)
		}
	}
	fs, _ := st.Findings("", 10)
	var robots conditions.Finding
	for _, f := range fs {
		if f.Key == "robots-rejoin-c360" {
			robots = f
		}
	}
	if robots.State != "resolved" || robots.StateBy != conditions.Resolver || robots.StateWhy != "no rejoins in 24 h" || robots.Run != run.ID || robots.StateAt == nil {
		t.Fatalf("resolved: %+v", robots)
	}
	if _, _, err := st.RecordFinding(9, conditions.Finding{Key: "robots-rejoin-c360", Kind: "compatibility", Severity: "warning", Title: "Robots", Saw: "back", Todo: "y"}); err != nil {
		t.Fatal(err)
	}
	fs, _ = st.Findings("", 10)
	if fs[0].Key != "robots-rejoin-c360" || fs[0].State != "open" || fs[0].StateBy != "" || fs[0].StateWhy != "" || fs[0].Saw != "back" {
		t.Fatalf("found again after it was resolved: %+v", fs[0])
	}
}

// A question is answered: the opening asks it, the review keeps it, and
// its summary is the answer (0215).
func TestAQuestion(t *testing.T) {
	st := store(t)
	c := &claude{turns: []string{turn("end_turn", `{"type":"text","text":"The kettle drops because its signal is -84 dBm."}`)}}
	api := httptest.NewServer(c)
	defer api.Close()
	run, err := Review(context.Background(), Settings{Key: "sk-test-key-0123456789", Budget: 2, BaseURL: api.URL}, fakeReads(), st,
		"asked by griff", "  Why does the kettle keep dropping?\n", time.Date(2026, 10, 24, 17, 0, 0, 0, time.UTC))
	if err != nil || run.Outcome != "done" || run.Question != "Why does the kettle keep dropping?" || !strings.Contains(run.Summary, "-84 dBm") {
		t.Fatalf("run = %+v, %v", run, err)
	}
	open := c.asked[0]["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	for _, want := range []string{"<question>\nWhy does the kettle keep dropping?\n</question>", "one question, not a review of everything", "This answer may cost at most $2.00"} {
		if !strings.Contains(open, want) {
			t.Errorf("the opening has no %q: %q", want, open)
		}
	}
	if strings.Contains(open, "Review the network now") {
		t.Errorf("a question was opened as a review: %q", open)
	}
	runs, _ := st.AdviceRuns("", 5)
	if len(runs) != 1 || runs[0].Question != "Why does the kettle keep dropping?" || runs[0].Why != "asked by griff" {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestClip(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Fatalf("clip = %q", got)
	}
	if got := clip("one two three four five six", 15); got != "one two three …" {
		t.Fatalf("clip = %q", got)
	}
}

// A review never makes a request that could pass its budget, nor one with
// no key; and says why it stopped.
func TestAReviewKeepsToItsBudget(t *testing.T) {
	c := &claude{turns: []string{turn("end_turn", `{"type":"text","text":"x"}`)}}
	api := httptest.NewServer(c)
	defer api.Close()
	st := store(t)
	run, err := Review(context.Background(), Settings{Key: "sk-test-key-0123456789", Model: "claude-opus-5-5", Budget: 0.1, BaseURL: api.URL}, fakeReads(), st, "schedule", "", time.Now())
	if err != nil || run.Outcome != "budget" || len(c.asked) != 0 || !strings.Contains(run.Error, "$0.10 budget") {
		t.Fatalf("run = %+v, asked %d, %v", run, len(c.asked), err)
	}
	run, _ = Review(context.Background(), Settings{Budget: 2, BaseURL: api.URL}, fakeReads(), st, "schedule", "", time.Now())
	if run.Outcome != "failed" || !strings.Contains(run.Error, "no Anthropic API key") {
		t.Fatalf("no key: %+v", run)
	}
}

// A request the API refuses ends the review, saying why.
func TestAReviewSaysWhyItFailed(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer api.Close()
	run, err := Review(context.Background(), Settings{Key: "sk-test-key-0123456789", Budget: 2, BaseURL: api.URL}, fakeReads(), store(t), "schedule", "", time.Now())
	if err != nil || run.Outcome != "failed" || !strings.Contains(run.Error, "refused the key") {
		t.Fatalf("run = %+v, %v", run, err)
	}
}

func TestCut(t *testing.T) {
	long := strings.Repeat("é", resultMax)
	got := cut(long)
	if !strings.Contains(got, "cut at") || !strings.HasPrefix(got, "é") || strings.ContainsRune(got[:len(got)-100], '�') {
		t.Fatalf("cut = %q…", got[:20])
	}
}
