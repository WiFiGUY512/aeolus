package dnslog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fake is a Technitium server's two answers this package asks for: its
// query log, of `have` lookups by one address, and its apps. It notes each
// request's address and body.
type fake struct {
	t      *testing.T
	app    string // the logs app it has; another name than asked is refused
	class  string
	token  string
	have   int
	asked  []string
	bodies []string
}

func (f *fake) handler(w http.ResponseWriter, r *http.Request) {
	f.asked = append(f.asked, r.Method+" "+r.URL.String())
	if err := r.ParseForm(); err != nil {
		f.t.Fatal(err)
	}
	f.bodies = append(f.bodies, r.PostForm.Encode())
	say := func(v any) { json.NewEncoder(w).Encode(v) }
	if r.PostForm.Get("token") != f.token {
		say(map[string]any{"status": "invalid-token", "errorMessage": "Invalid token or session expired."})
		return
	}
	switch r.URL.Path {
	case "/api/apps/list":
		say(map[string]any{"status": "ok", "response": map[string]any{"apps": []any{
			map[string]any{"name": "Advanced Blocking", "dnsApps": []any{map[string]any{"classPath": "AdvancedBlocking.App", "isQueryLogs": false}}},
			map[string]any{"name": f.app, "dnsApps": []any{map[string]any{"classPath": f.class, "isQueryLogs": true}}},
		}}})
	case "/api/logs/query":
		if r.PostForm.Get("name") != f.app || r.PostForm.Get("classPath") != f.class {
			say(map[string]any{"status": "error", "errorMessage": "DNS application was not found: " + r.PostForm.Get("name")})
			return
		}
		var n, per int
		fmt.Sscan(r.PostForm.Get("pageNumber"), &n)
		fmt.Sscan(r.PostForm.Get("entriesPerPage"), &per)
		var entries []any
		for i := (n - 1) * per; i < n*per && i < f.have; i++ {
			e := map[string]any{"rowNumber": f.have - i, "timestamp": time.Date(2026, 10, 13, 22, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Second).Format(time.RFC3339Nano),
				"clientIpAddress": r.PostForm.Get("clientIpAddress"), "protocol": "Udp", "responseType": "Recursive", "rcode": "NoError",
				"qname": fmt.Sprintf("host%d.example.com", i%3), "qtype": []string{"A", "AAAA"}[i%2], "qclass": "IN", "answer": "A 192.0.2.1"}
			entries = append(entries, e)
		}
		say(map[string]any{"status": "ok", "server": "dns.example", "response": map[string]any{
			"pageNumber": n, "totalPages": (f.have + per - 1) / per, "totalEntries": f.have, "entries": entries}})
	default:
		http.NotFound(w, r)
	}
}

func serve(t *testing.T, f *fake) *httptest.Server {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return srv
}

var from, to = time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

// The log is asked a page at a time until it has no more, or enough has
// come; and the token is in no request's address, where it would be logged.
func TestTechnitiumAsksPageByPage(t *testing.T) {
	f := &fake{app: sqliteApp, class: sqliteClass, token: "tok-1234567", have: 2500}
	srv := serve(t, f)
	entries, total, err := Technitium(context.Background(), srv.Client(), srv.URL+"/", f.token, "192.168.20.61", from, to, 5000)
	if err != nil || total != 2500 || len(entries) != 2500 {
		t.Fatalf("all of them: %d of %d, %v", len(entries), total, err)
	}
	if len(f.asked) != 3 {
		t.Errorf("asked %d times for three pages: %v", len(f.asked), f.asked)
	}
	if entries[0].Name != "host0.example.com" || entries[0].Type != "A" || entries[0].Kind != "Recursive" || !entries[0].At.Equal(time.Date(2026, 10, 13, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("the latest = %+v", entries[0])
	}
	for i, a := range f.asked {
		if !strings.HasPrefix(a, "POST ") || strings.Contains(a, f.token) || strings.Contains(a, "?") {
			t.Errorf("request %d has more than its path in its address: %s", i, a)
		}
		if !strings.Contains(f.bodies[i], "clientIpAddress=192.168.20.61") || !strings.Contains(f.bodies[i], "start=2026-10-13T00%3A00%3A00Z") || !strings.Contains(f.bodies[i], "descendingOrder=true") {
			t.Errorf("request %d asked %s", i, f.bodies[i])
		}
	}

	// No more than is wanted, and it says how many there are.
	f.asked = nil
	entries, total, err = Technitium(context.Background(), srv.Client(), srv.URL, f.token, "192.168.20.61", from, to, 1200)
	if err != nil || total != 2500 || len(entries) != 1200 || len(f.asked) != 2 {
		t.Fatalf("at most 1200: %d of %d in %d requests, %v", len(entries), total, len(f.asked), err)
	}
}

// A server with another logs app than the usual one is asked which it has.
func TestTechnitiumFindsTheLogsApp(t *testing.T) {
	f := &fake{app: "Query Logs (MySQL)", class: "QueryLogsMySql.App", token: "tok-1234567", have: 3}
	srv := serve(t, f)
	entries, _, err := Technitium(context.Background(), srv.Client(), srv.URL, f.token, "10.0.0.5", from, to, 100)
	if err != nil || len(entries) != 3 {
		t.Fatalf("with another app: %d, %v", len(entries), err)
	}
	if len(f.asked) != 3 || !strings.Contains(f.asked[1], "/api/apps/list") {
		t.Errorf("asked: %v", f.asked)
	}
}

func TestTechnitiumSaysWhatIsWrong(t *testing.T) {
	f := &fake{app: sqliteApp, class: sqliteClass, token: "tok-1234567", have: 3}
	srv := serve(t, f)
	// A token it refuses is said as that, and the token itself never is.
	_, _, err := Technitium(context.Background(), srv.Client(), srv.URL, "wrong-token", "10.0.0.5", from, to, 100)
	if !errors.Is(err, ErrToken) || strings.Contains(err.Error(), "wrong-token") {
		t.Errorf("a refused token: %v", err)
	}
	// A server that is not there: its address is said, and no token.
	srv.Close()
	_, _, err = Technitium(context.Background(), &http.Client{Timeout: 2 * time.Second}, srv.URL, "tok-1234567", "10.0.0.5", from, to, 100)
	if err == nil || !strings.Contains(err.Error(), "did not answer") || strings.Contains(err.Error(), "tok-1234567") {
		t.Errorf("no server: %v", err)
	}
	// What is no Technitium server answers with something else.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>hello</html>")) }))
	defer other.Close()
	_, _, err = Technitium(context.Background(), other.Client(), other.URL, "tok-1234567", "10.0.0.5", from, to, 100)
	if err == nil || !strings.Contains(err.Error(), "no answer of a Technitium") {
		t.Errorf("another kind of server: %v", err)
	}
}

// The domain a name is in, and a client's lookups by domain: what it asks
// for most says what it is.
func TestPlaces(t *testing.T) {
	for name, want := range map[string]string{
		"mqtt.bambulab.com":         "bambulab.com",
		"API.Bambulab.com.":         "bambulab.com",
		"bambulab.com":              "bambulab.com",
		"a.b.c.example.co.uk":       "example.co.uk",
		"www.example.com.au":        "example.com.au",
		"printer":                   "printer",
		"living-room.local":         "names of this network (.local)",
		"1.20.168.192.in-addr.arpa": "addresses, looked up backwards",
		"time.nist.gov":             "nist.gov",
		"x.co":                      "x.co",
	} {
		if got := Domain(name); got != want {
			t.Errorf("Domain(%q) = %q, want %q", name, got, want)
		}
	}
	got := Places([]Lookup{
		{Name: "mqtt.bambulab.com", Times: 40}, {Name: "api.bambulab.com", Times: 12},
		{Name: "time.cloudflare.com", Times: 30}, {Name: "pool.ntp.org", Times: 30}, {Name: "0.pool.ntp.org", Times: 1},
	})
	if len(got) != 3 || got[0] != (Place{"bambulab.com", 52, 2}) || got[1] != (Place{"ntp.org", 31, 2}) || got[2] != (Place{"cloudflare.com", 30, 1}) {
		t.Errorf("places = %+v", got)
	}
}

// Every address's lookups, each with the domains it asked for most: the
// log asked with no address is all of them.
func TestByClient(t *testing.T) {
	f := &fake{app: sqliteApp, class: sqliteClass, token: "tok-1234567", have: 9}
	srv := serve(t, f)
	entries, _, err := Technitium(context.Background(), srv.Client(), srv.URL, f.token, "", from, to, 100)
	if err != nil || len(entries) != 9 || strings.Contains(f.bodies[0], "clientIpAddress") {
		t.Fatalf("with no address: %d, %v, asked %v", len(entries), err, f.bodies)
	}
	got := ByClient([]Entry{
		{Client: "10.0.0.5", Name: "mqtt.bambulab.com"}, {Client: "10.0.0.5", Name: "api.bambulab.com"}, {Client: "10.0.0.5", Name: "pool.ntp.org"},
		{Client: "10.0.0.5", Name: "a.example.com"}, {Client: "10.0.0.6", Name: "www.netflix.com"}, {Name: "nobody.example"},
	}, 2)
	if len(got) != 2 || got["10.0.0.5"].Total != 4 || len(got["10.0.0.5"].Places) != 2 || got["10.0.0.5"].Places[0] != (Place{"bambulab.com", 2, 2}) ||
		got["10.0.0.6"].Total != 1 || got["10.0.0.6"].Places[0].Domain != "netflix.com" {
		t.Errorf("by client = %+v", got)
	}
}

// Lookups are put together by name: how many, of which types, when, and how
// many were blocked or failed; the name asked for last comes first.
func TestSummarize(t *testing.T) {
	at := func(s int) time.Time { return time.Date(2026, 10, 13, 12, 0, s, 0, time.UTC) }
	got := Summarize([]Entry{
		{At: at(50), Name: "MQTT.Example.com.", Type: "A", RCode: "NoError", Kind: "Recursive"},
		{At: at(40), Name: "mqtt.example.com", Type: "AAAA", RCode: "NoError", Kind: "Cached"},
		{At: at(30), Name: "ads.example.net", Type: "A", RCode: "NxDomain", Kind: "Blocked"},
		{At: at(20), Name: "mqtt.example.com", Type: "A", RCode: "NoError", Kind: "Cached"},
		{At: at(10), Name: "gone.example.org", Type: "A", RCode: "NxDomain", Kind: "Recursive"},
		{At: at(5), Name: "", Type: "A"},
	})
	if len(got) != 3 {
		t.Fatalf("names = %+v", got)
	}
	m := got[0]
	if m.Name != "mqtt.example.com" || m.Times != 3 || strings.Join(m.Types, ",") != "A,AAAA" || !m.First.Equal(at(20)) || !m.Last.Equal(at(50)) || m.Blocked != 0 || m.Failed != 0 {
		t.Errorf("the name asked for most lately = %+v", m)
	}
	if got[1].Name != "ads.example.net" || got[1].Blocked != 1 || got[1].Failed != 0 {
		t.Errorf("a blocked name = %+v", got[1])
	}
	if got[2].Name != "gone.example.org" || got[2].Failed != 1 {
		t.Errorf("a name that is not there = %+v", got[2])
	}
}
