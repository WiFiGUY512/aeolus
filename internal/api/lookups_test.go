package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

// What a client looks up (0154): asked of the site's DNS server's own log,
// by the address the client held, when someone wants to know; the names its
// APs noted as it came online are there without one. The server's token is
// the manager's to use, and no AP is sent it.
func TestClientLookups(t *testing.T) {
	const mac, addr, tok = "02:00:00:00:00:41", "192.168.20.61", "tok-AbC123.xyz"
	// A Technitium server's query log, of that address.
	var askedOf []string
	dns := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		say := func(v any) { json.NewEncoder(w).Encode(v) }
		if r.PostForm.Get("token") != tok {
			say(map[string]any{"status": "invalid-token"})
			return
		}
		if r.URL.Path != "/api/logs/query" {
			http.NotFound(w, r)
			return
		}
		askedOf = append(askedOf, r.PostForm.Get("clientIpAddress"))
		entry := func(ago time.Duration, name, kind, rcode, how string) map[string]any {
			return map[string]any{"timestamp": time.Now().Add(-ago).UTC().Format(time.RFC3339Nano), "clientIpAddress": addr,
				"qname": name, "qtype": kind, "rcode": rcode, "responseType": how}
		}
		say(map[string]any{"status": "ok", "response": map[string]any{"pageNumber": 1, "totalPages": 1, "totalEntries": 5, "entries": []any{
			entry(time.Minute, "mqtt.bambulab.com", "A", "NoError", "Recursive"),
			entry(2*time.Minute, "api.bambulab.com", "A", "NoError", "Cached"),
			entry(3*time.Minute, "mqtt.bambulab.com", "AAAA", "NoError", "Cached"),
			entry(4*time.Minute, "time.cloudflare.com", "A", "NoError", "Recursive"),
			entry(5*time.Minute, "ads.example.net", "A", "NxDomain", "Blocked"),
		}}})
	}))
	defer dns.Close()

	f := newFixture(t)
	_, token, version := f.adopted()
	client := map[string]any{"mac": mac, "address": addr, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -60, "signal_avg": -60,
		"tx_rate": 400, "rx_rate": 300, "connected": 60, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1}
	if code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "clients": []any{client}}, nil); code != 200 {
		t.Fatalf("state: %d", code)
	}
	// And an attempt to come online, with its first lookups noted.
	attempt := map[string]any{"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g",
		"started": time.Now().UnixMilli() - 60000, "took_ms": 300, "outcome": "online", "stage": "dns", "reason": "", "address": addr,
		"events": []any{
			map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": true},
			map[string]any{"t": 200, "stage": "dns", "what": "asks 192.168.20.1 for mqtt.bambulab.com (A)"},
			map[string]any{"t": 230, "stage": "dns", "what": "192.168.20.1 answers mqtt.bambulab.com: 1 record", "ok": true, "ms": 30},
			map[string]any{"t": 260, "stage": "dns", "what": "asks 192.168.20.1 for pool.ntp.org (A)"},
		}}
	if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{attempt}}, nil); code != 200 || body["recorded"] != float64(1) {
		t.Fatalf("attempt: %d %v", code, body)
	}

	// With no server set: what its APs noted as it came online, and no more.
	_, got := f.do("GET", "/v1/clients/"+mac+"/lookups", "griff", nil)
	if got["server"] != nil || got["lookups"] != nil {
		t.Fatalf("with no server set: %v", got)
	}
	joins := got["joins"].([]any)
	if len(joins) != 2 || joins[0].(map[string]any)["name"] != "pool.ntp.org" || joins[1].(map[string]any)["name"] != "mqtt.bambulab.com" {
		t.Fatalf("joins = %v", joins)
	}
	if p := got["join_places"].([]any); len(p) != 2 {
		t.Fatalf("join places = %v", p)
	}
	if got["joins_last"] == nil {
		t.Fatalf("no joins_last: %v", got)
	}

	// What its APs noted is of the hours asked (0337's follow-up, Griff
	// 2026-11-07: A day showed names from 26 days before): one from three
	// days ago is out of a day, and in a week.
	old := map[string]any{"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g",
		"started": time.Now().Add(-72 * time.Hour).UnixMilli(), "took_ms": 300, "outcome": "online", "stage": "dns", "reason": "", "address": addr,
		"events": []any{map[string]any{"t": 200, "stage": "dns", "what": "asks 192.168.20.1 for old.example.com (A)"}}}
	if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{old}}, nil); code != 200 || body["recorded"] != float64(1) {
		t.Fatalf("old attempt: %d %v", code, body)
	}
	names := func(hours string) string {
		_, got := f.do("GET", "/v1/clients/"+mac+"/lookups?hours="+hours, "griff", nil)
		var out []string
		for _, j := range got["joins"].([]any) {
			out = append(out, j.(map[string]any)["name"].(string))
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	if n := names("24"); n != "mqtt.bambulab.com pool.ntp.org" {
		t.Fatalf("a day's noted names: %s", n)
	}
	if n := names("168"); n != "mqtt.bambulab.com old.example.com pool.ntp.org" {
		t.Fatalf("a week's noted names: %s", n)
	}

	// The server set on the folder, with its token sealed.
	set := func(values map[string]any) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "values": values}); code != 200 {
			t.Fatalf("set: %d %v", code, b)
		}
	}
	set(map[string]any{"system.dns_log.url": dns.URL, "system.dns_log.token": tok})
	_, got = f.do("GET", "/v1/clients/"+mac+"/lookups", "griff", nil)
	server, _ := got["server"].(map[string]any)
	if server == nil || server["error"] != nil || server["url"] != dns.URL || server["queries"] != float64(5) || server["read"] != float64(5) {
		t.Fatalf("server = %v", got["server"])
	}
	asked := server["asked"].([]any)
	if len(asked) != 1 || asked[0].(map[string]any)["address"] != addr || len(askedOf) != 1 || askedOf[0] != addr {
		t.Fatalf("asked about %v, the server of %v", asked, askedOf)
	}
	lookups := got["lookups"].([]any)
	first := lookups[0].(map[string]any)
	if len(lookups) != 4 || first["name"] != "mqtt.bambulab.com" || first["times"] != float64(2) || len(first["types"].([]any)) != 2 {
		t.Fatalf("lookups = %v", lookups)
	}
	if last := lookups[3].(map[string]any); last["name"] != "ads.example.net" || last["blocked"] != float64(1) {
		t.Fatalf("a blocked name = %v", last)
	}
	places := got["places"].([]any)
	if top := places[0].(map[string]any); len(places) != 3 || top["domain"] != "bambulab.com" || top["times"] != float64(3) || top["names"] != float64(2) {
		t.Fatalf("places = %v", places)
	}

	// What each client on now asks for most, for the lists: read of the
	// whole log once, and kept a while, so asked twice the server is asked
	// once.
	before := len(askedOf)
	_, top := f.do("GET", "/v1/lookups/top", "griff", nil)
	mine, _ := top["asks"].(map[string]any)[mac].(map[string]any)
	if mine == nil || mine["total"] != float64(5) {
		t.Fatalf("top = %v", top)
	}
	if p := mine["places"].([]any); len(p) != 3 || p[0].(map[string]any)["domain"] != "bambulab.com" || p[0].(map[string]any)["times"] != float64(3) {
		t.Fatalf("its places = %v", p)
	}
	if sv := top["servers"].([]any); len(sv) != 1 || sv[0].(map[string]any)["url"] != dns.URL || sv[0].(map[string]any)["read"] != float64(5) {
		t.Fatalf("servers = %v", top["servers"])
	}
	f.do("GET", "/v1/lookups/top", "griff", nil)
	if len(askedOf) != before+1 || askedOf[before] != "" {
		t.Errorf("asked the server %d times, of %q", len(askedOf)-before, askedOf[before:])
	}
	if _, none := f.do("GET", "/v1/lookups/top", "tenant", nil); len(none["asks"].(map[string]any)) != 0 {
		t.Errorf("to one who may view no AP: %v", none)
	}

	// No AP is sent the server or its token, and the token is said to no one.
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	raw, _ := json.Marshal(cfg)
	if strings.Contains(string(raw), "dns_log") || strings.Contains(string(raw), tok) || strings.Contains(string(raw), dns.URL) {
		t.Errorf("the AP's config has the DNS log in it: %s", raw)
	}
	raw, _ = json.Marshal(got)
	if strings.Contains(string(raw), tok) {
		t.Errorf("the answer says the token: %s", raw)
	}

	// A token the server refuses: said as that, with what the APs noted.
	set(map[string]any{"system.dns_log.token": "another-token"})
	_, got = f.do("GET", "/v1/clients/"+mac+"/lookups", "griff", nil)
	server = got["server"].(map[string]any)
	if why, _ := server["error"].(string); !strings.Contains(why, "refuses the token") || strings.Contains(why, "another-token") || len(got["joins"].([]any)) != 2 {
		t.Fatalf("a refused token: %v", got)
	}

	// Who may not view its AP is told of no such client.
	if code, _ := f.do("GET", "/v1/clients/"+mac+"/lookups", "tenant", nil); code != 404 {
		t.Errorf("one who may not view its AP: %d", code)
	}
	if code, _ := f.do("GET", "/v1/clients/"+mac+"/lookups?hours=0", "griff", nil); code != 400 {
		t.Errorf("no hours: %d", code)
	}
}
