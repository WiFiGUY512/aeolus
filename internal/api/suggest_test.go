package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Anyone signed in may suggest a feature, from the page they were on
// (0323): it is kept with who asked, the page and the release; what is too
// long, empty, or more than twenty an hour is refused. Those who view the
// Org read them; whoami says where they go on to.
func TestSuggestions(t *testing.T) {
	f := newFixture(t)
	f.api.WithVersion("v9.9.9-test")
	code, body := f.do("POST", "/v1/suggestions", "tenant", map[string]any{
		"text": "  Show each client's vendor beside its name  ", "contact": "user@example.com",
		"route": "#/clients/7e:2a:ea:04:b1:8f", "title": "Owners-iPhone " + strings.Repeat("x", 400),
	})
	if code != 201 {
		t.Fatalf("suggest: %d %v", code, body)
	}
	x := body["suggestion"].(map[string]any)
	if x["text"] != "Show each client's vendor beside its name" || x["account"] != "tenant" || x["version"] != "v9.9.9-test" || x["route"] != "#/clients/7e:2a:ea:04:b1:8f" {
		t.Fatalf("kept as %v", x)
	}
	if n := len([]rune(x["title"].(string))); n != suggestPageMax {
		t.Fatalf("title kept at %d characters, want it cut to %d", n, suggestPageMax)
	}
	if body["to"] != "" {
		t.Fatalf("to = %v with nowhere set", body["to"])
	}
	// A page that is not one of the manager's own routes is not kept.
	if _, body := f.do("POST", "/v1/suggestions", "tenant", map[string]any{"text": "x", "route": "javascript:alert(1)"}); body["suggestion"].(map[string]any)["route"] != nil {
		t.Fatalf("kept a route that is a script: %v", body)
	}
	for name, req := range map[string]map[string]any{
		"empty":      {"text": "   "},
		"too long":   {"text": strings.Repeat("a", suggestMax+1)},
		"long reach": {"text": "a", "contact": strings.Repeat("b", suggestContactMax+1)},
	} {
		if code, body := f.do("POST", "/v1/suggestions", "tenant", req); code != 400 {
			t.Errorf("%s: %d %v", name, code, body)
		}
	}

	// Those who view the Org read them; one who views a Services folder
	// alone does not.
	code, body = f.do("GET", "/v1/suggestions", "claude", nil)
	if list := body["suggestions"].([]any); code != 200 || len(list) != 2 {
		t.Fatalf("list: %d %v", code, body)
	}
	if code, _ := f.do("GET", "/v1/suggestions", "tenant", nil); code != 403 {
		t.Fatalf("a Services viewer reads them: %d", code)
	}

	// Twenty an hour, then no more.
	for i := 0; i < suggestPerHour-1; i++ {
		f.do("POST", "/v1/suggestions", "office", map[string]any{"text": "more"})
	}
	if code, _ := f.do("POST", "/v1/suggestions", "office", map[string]any{"text": "one too many"}); code != 201 {
		t.Fatalf("the twentieth: %d", code)
	}
	if code, _ := f.do("POST", "/v1/suggestions", "office", map[string]any{"text": "twenty-first"}); code != 429 {
		t.Fatalf("the twenty-first: %d", code)
	}

	// whoami says where they go on to: its host alone.
	if _, err := f.api.WithSuggestTo("https://feedback.example.org/aeolus/in?k=1"); err != nil {
		t.Fatal(err)
	}
	if _, body := f.do("GET", "/v1/whoami", "tenant", nil); body["suggest_to"] != "feedback.example.org" {
		t.Fatalf("whoami suggest_to = %v", body["suggest_to"])
	}
	for _, bad := range []string{"ftp://x.example/in", "feedback.example.org", "https://"} {
		if _, err := f.api.WithSuggestTo(bad); err == nil {
			t.Errorf("%q taken as where suggestions go", bad)
		}
	}
}

// Suggestions are sent on where the manager says, each once (0323): with
// what was written, the page, the release and the Org, never the account.
// One that does not get there is kept, and tried again later.
func TestSendingSuggestions(t *testing.T) {
	f := newFixture(t)
	var mu sync.Mutex
	var got []map[string]any
	fail := true
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			http.Error(w, "down for a while", http.StatusServiceUnavailable)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		json.Unmarshal(raw, &m)
		got = append(got, m)
	}))
	defer dest.Close()
	if _, err := f.api.WithSuggestTo(dest.URL + "/in"); err != nil {
		t.Fatal(err)
	}
	f.do("POST", "/v1/suggestions", "tenant", map[string]any{"text": "A dark map", "route": "#/locations/house/overview", "title": "House"})

	// Down: kept, with why, and not tried again at once.
	f.api.sendSuggestions(context.Background(), dest.Client())
	list, _ := f.conds.Suggestions(10)
	if len(list) != 1 || list[0].SentAt != nil || list[0].Tries != 1 || !strings.Contains(list[0].LastError, "503") {
		t.Fatalf("after a failed try: %+v", list)
	}
	mu.Lock()
	fail = false
	mu.Unlock()
	f.api.sendSuggestions(context.Background(), dest.Client())
	if len(got) != 0 {
		t.Fatalf("tried again before its time: %v", got)
	}

	// Its time come, sent, and once.
	f.conds.SuggestionFailed(list[0].ID, list[0].LastError, list[0].At) // due now
	f.api.sendSuggestions(context.Background(), dest.Client())
	f.api.sendSuggestions(context.Background(), dest.Client())
	if len(got) != 1 {
		t.Fatalf("sent %d times", len(got))
	}
	m := got[0]
	page, _ := m["page"].(map[string]any)
	if m["kind"] != "aeolus-suggestion" || m["text"] != "A dark map" || m["org"] != "example" || page["route"] != "#/locations/house/overview" || m["key"] == "" {
		t.Fatalf("sent %v", m)
	}
	if _, ok := m["account"]; ok {
		t.Fatalf("the account went with it: %v", m)
	}
	list, _ = f.conds.Suggestions(10)
	if list[0].SentAt == nil || list[0].SentTo == "" {
		t.Fatalf("not recorded as sent: %+v", list[0])
	}
}
