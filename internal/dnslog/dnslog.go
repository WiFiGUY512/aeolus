// Package dnslog asks a DNS server's own log what a client looked up
// (0154). The server is the site's, which every client there asks, so its
// log has each lookup by the address that made it; the manager asks it when
// a person wants to know, and keeps nothing of the answer.
//
// One kind of server is known: a Technitium DNS Server with a query logs
// app, over its HTTP API.
package dnslog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry is one lookup as the server's log has it.
type Entry struct {
	At time.Time
	// The address that asked.
	Client string
	Name   string
	Type   string
	RCode  string
	// How the server answered: from its own zones, by asking on, from its
	// cache, or not at all because the name is blocked.
	Kind string
}

// Lookup is the lookups of one name: how many, of which types, the first
// and the last, and how many were blocked or failed.
type Lookup struct {
	Name    string    `json:"name"`
	Types   []string  `json:"types"`
	Times   int       `json:"times"`
	First   time.Time `json:"first"`
	Last    time.Time `json:"last"`
	Blocked int       `json:"blocked,omitempty"`
	// Answered with an error: no such name, or the server could not say.
	Failed int `json:"failed,omitempty"`
}

// Summarize puts entries together by name, the name last asked for first.
func Summarize(entries []Entry) []Lookup {
	by := map[string]*Lookup{}
	types := map[string]map[string]bool{}
	for _, e := range entries {
		name := strings.ToLower(strings.TrimSuffix(e.Name, "."))
		if name == "" {
			continue
		}
		l := by[name]
		if l == nil {
			l = &Lookup{Name: name, First: e.At, Last: e.At}
			by[name] = l
			types[name] = map[string]bool{}
		}
		l.Times++
		if e.At.Before(l.First) {
			l.First = e.At
		}
		if e.At.After(l.Last) {
			l.Last = e.At
		}
		if e.Type != "" && !types[name][e.Type] {
			types[name][e.Type] = true
			l.Types = append(l.Types, e.Type)
		}
		switch {
		case strings.Contains(e.Kind, "Blocked"):
			l.Blocked++
		case e.RCode != "" && !strings.EqualFold(e.RCode, "NoError"):
			l.Failed++
		}
	}
	out := make([]Lookup, 0, len(by))
	for _, l := range by {
		sort.Strings(l.Types)
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Place is a domain a client asks for names in: how many lookups, of how
// many names. What a device asks for most says what it is: a printer its
// maker's cloud, a television its services.
type Place struct {
	Domain string `json:"domain"`
	Times  int    `json:"times"`
	Names  int    `json:"names"`
}

// The second labels that, under a country's two letters, are not anyone's
// own: example.co.uk is a domain, co.uk is not.
var shared = map[string]bool{"co": true, "com": true, "net": true, "org": true, "gov": true, "ac": true, "edu": true, "or": true, "ne": true, "go": true, "gob": true, "mil": true, "nom": true, "sch": true}

// Domain is the domain a name is in, as near as its labels say: its last
// two, or its last three where the last two are a country's shared ones
// (example.co.uk). A name with one label, or under .local or .arpa, is its
// own kind: what is asked of the network itself, and addresses looked up
// backwards.
func Domain(name string) string {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	labels := strings.Split(name, ".")
	n := len(labels)
	switch {
	case n < 2:
		return name
	case labels[n-1] == "arpa":
		return "addresses, looked up backwards"
	case labels[n-1] == "local":
		return "names of this network (.local)"
	case n >= 3 && len(labels[n-1]) == 2 && shared[labels[n-2]]:
		return strings.Join(labels[n-3:], ".")
	}
	return strings.Join(labels[n-2:], ".")
}

// Places puts lookups together by the domain each name is in, the one
// asked for most first.
func Places(lookups []Lookup) []Place {
	by := map[string]*Place{}
	for _, l := range lookups {
		d := Domain(l.Name)
		p := by[d]
		if p == nil {
			p = &Place{Domain: d}
			by[d] = p
		}
		p.Times += l.Times
		p.Names++
	}
	out := make([]Place, 0, len(by))
	for _, p := range by {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Times != out[j].Times {
			return out[i].Times > out[j].Times
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

// Asked is what one address asked for: how many lookups in all, and the
// domains it asked for most.
type Asked struct {
	Total  int     `json:"total"`
	Places []Place `json:"places"`
}

// ByClient puts lookups together by the address that asked: for each, how
// many, and the n domains it asked for most.
func ByClient(entries []Entry, n int) map[string]Asked {
	by := map[string][]Entry{}
	for _, e := range entries {
		if e.Client != "" {
			by[e.Client] = append(by[e.Client], e)
		}
	}
	out := map[string]Asked{}
	for addr, list := range by {
		places := Places(Summarize(list))
		if len(places) > n {
			places = places[:n]
		}
		out[addr] = Asked{Total: len(list), Places: places}
	}
	return out
}

// The query logs app a Technitium server most often has, tried before the
// server is asked which it has.
const (
	sqliteApp   = "Query Logs (Sqlite)"
	sqliteClass = "QueryLogsSqlite.App"
	page        = 1000 // entries asked for at a time, the most the server gives
)

// ErrToken is the server refusing the token it was asked with.
var ErrToken = errors.New("the DNS server refuses the token")

type technitium struct {
	hc    *http.Client
	base  string
	token string
}

// call asks one of the server's API paths, with the token and the rest in
// the request's body, not its address: an address is what gets written in
// logs and errors, and the token is not to be.
func (t technitium) call(ctx context.Context, path string, form url.Values, out any) error {
	form.Set("token", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := t.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // without the address, which is said beside it
		}
		return fmt.Errorf("the DNS server at %s did not answer: %w", t.base, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("the DNS server at %s: %w", t.base, err)
	}
	var env struct {
		Status   string          `json:"status"`
		Error    string          `json:"errorMessage"`
		Response json.RawMessage `json:"response"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return fmt.Errorf("the DNS server at %s answered %s with what is no answer of a Technitium server's", t.base, res.Status)
	}
	switch env.Status {
	case "ok":
		return json.Unmarshal(env.Response, out)
	case "invalid-token":
		return ErrToken
	}
	if env.Error == "" {
		env.Error = "status " + env.Status
	}
	return fmt.Errorf("the DNS server at %s: %s", t.base, env.Error)
}

type logPage struct {
	TotalPages   int `json:"totalPages"`
	TotalEntries int `json:"totalEntries"`
	Entries      []struct {
		Timestamp    time.Time `json:"timestamp"`
		Client       string    `json:"clientIpAddress"`
		QName        string    `json:"qname"`
		QType        string    `json:"qtype"`
		RCode        string    `json:"rcode"`
		ResponseType string    `json:"responseType"`
	} `json:"entries"`
}

// logsApp asks the server which query logs app it has.
func (t technitium) logsApp(ctx context.Context) (name, class string, err error) {
	var list struct {
		Apps []struct {
			Name    string `json:"name"`
			DNSApps []struct {
				ClassPath   string `json:"classPath"`
				IsQueryLogs bool   `json:"isQueryLogs"`
			} `json:"dnsApps"`
		} `json:"apps"`
	}
	if err := t.call(ctx, "/api/apps/list", url.Values{}, &list); err != nil {
		return "", "", err
	}
	for _, a := range list.Apps {
		for _, d := range a.DNSApps {
			if d.IsQueryLogs {
				return a.Name, d.ClassPath, nil
			}
		}
	}
	return "", "", fmt.Errorf("the DNS server at %s has no query logs app installed, so it keeps no log to ask", t.base)
}

// Technitium asks a Technitium DNS Server what one address looked up from
// start to end, the latest first: at most `most` lookups. With no address,
// it is what every address looked up. total is how many the log has of it
// in that time. base is the server's web console, as http://host:5380;
// token an API token of a user who may view its logs.
func Technitium(ctx context.Context, hc *http.Client, base, token, addr string, start, end time.Time, most int) (entries []Entry, total int, err error) {
	t := technitium{hc: hc, base: strings.TrimRight(base, "/"), token: token}
	name, class := sqliteApp, sqliteClass
	asked := false
	for n := 1; len(entries) < most; n++ {
		form := url.Values{
			"name": {name}, "classPath": {class},
			"start": {start.UTC().Format(time.RFC3339)}, "end": {end.UTC().Format(time.RFC3339)},
			"pageNumber": {strconv.Itoa(n)}, "entriesPerPage": {strconv.Itoa(page)}, "descendingOrder": {"true"},
		}
		if addr != "" {
			form.Set("clientIpAddress", addr)
		}
		var p logPage
		if err := t.call(ctx, "/api/logs/query", form, &p); err != nil {
			// Another logs app than the usual one: the server says which.
			if errors.Is(err, ErrToken) || asked || n > 1 {
				return nil, 0, err
			}
			asked = true
			var e2 error
			if name, class, e2 = t.logsApp(ctx); e2 != nil {
				if errors.Is(e2, ErrToken) {
					return nil, 0, e2
				}
				return nil, 0, err
			}
			n = 0
			continue
		}
		total = p.TotalEntries
		for _, e := range p.Entries {
			if len(entries) == most {
				break
			}
			entries = append(entries, Entry{At: e.Timestamp, Client: e.Client, Name: e.QName, Type: e.QType, RCode: e.RCode, Kind: e.ResponseType})
		}
		if n >= p.TotalPages || len(p.Entries) == 0 {
			break
		}
	}
	return entries, total, nil
}
