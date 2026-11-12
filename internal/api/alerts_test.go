package api

import (
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// GET /v1/alerts (0099): what needs attention on the APs the caller may
// view, most urgent first, with counts.
func TestAlerts(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	alertsOf := func(who, query string) []map[string]any {
		t.Helper()
		code, body := f.do("GET", "/v1/alerts"+query, who, nil)
		if code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		var out []map[string]any
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["ap"] == ap {
				out = append(out, m)
			}
		}
		if body["counts"] == nil {
			t.Fatalf("no counts: %v", body)
		}
		return out
	}
	if got := alertsOf("griff", ""); len(got) != 0 {
		t.Fatalf("a healthy AP: %v", got)
	}
	// Its netifd lost network.wireless: critical, as the AP says so.
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 90, "wireless_missing": true}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	got := alertsOf("griff", "")
	if len(got) != 1 || got[0]["kind"] != "wireless-missing" || got[0]["severity"] != "critical" || got[0]["name"] != "ShedAP" {
		t.Fatalf("alerts = %v", got)
	}
	// Under a folder it isn't in, none of its alerts.
	if got := alertsOf("griff", "?under=no-such-folder"); len(got) != 0 {
		t.Fatalf("under another folder: %v", got)
	}
	// office has no role in Locations, so it sees no AP's alerts.
	if got := alertsOf("office", ""); len(got) != 0 {
		t.Fatalf("office sees %v", got)
	}
}

// Where an AP's alerts go (0101): the folders' notify.* fields, nearest
// first, the URLs sealed in the log and opened only to send, and never in
// an AP's config.
func TestNotifyTarget(t *testing.T) {
	f := newFixture(t)
	for _, op := range []map[string]any{
		{"kind": "set", "tree": "locations", "node": "example", "path": "notify.ntfy", "value": "https://ntfy.example/example-alerts"},
		{"kind": "set", "tree": "locations", "node": "house", "path": "notify.severity", "value": "warning"},
		{"kind": "set", "tree": "locations", "node": "house", "path": "notify.resolved", "value": false},
	} {
		if code, body := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, body)
		}
	}
	got := f.api.notifyTarget(f.log.Snapshot(), "office-ap")
	if got.Ntfy != "https://ntfy.example/example-alerts" || got.Webhook != "" || got.Least != "warning" || got.Resolved {
		t.Fatalf("target = %+v", got)
	}
	// Sealed as it is kept, and shown so.
	_, page := f.do("GET", "/v1/trees/locations/nodes/example", "griff", nil)
	if v, _ := page["fields"].(map[string]any)["notify.ntfy"].(map[string]any)["value"].(map[string]any); v["sealed"] != true {
		t.Fatalf("notify.ntfy shows %v", page["fields"].(map[string]any)["notify.ntfy"])
	}
	// Not in the AP's config.
	_, cfg := f.do("GET", "/v1/aps/office-ap/config", "griff", nil)
	if doc, _ := cfg["document"].(map[string]any); doc["notify"] != nil {
		t.Fatalf("the AP's document has notify: %v", doc["notify"])
	}
}

// An alert calls a client a person named by that name, with its MAC after
// (0153); one already named, one no one named and a longer run of hex are
// left as they are.
func TestAlertsNameClients(t *testing.T) {
	f := newFixture(t)
	if err := f.api.conds.NameClient("a8:b5:8e:8c:93:89", "kettle", "griff"); err != nil {
		t.Fatal(err)
	}
	as := []alerts.Alert{
		{Message: "power control has 2.4 GHz at 8 dBm, where a8:b5:8e:8c:93:89, which no other AP hears better, hears it at -82 dBm"},
		{Message: "kettle (a8:b5:8e:8c:93:89) came online here 30 times in the last hour"},
		{Message: "30:86:2d:c3:0d:40, not one of the APs', is on your wire"},
		{Message: "a8:b5:8e:8c:93:89,24:58:7c:4e:ed:dc and fe80::a8:b5:8e:8c:93:89:aa"},
	}
	f.api.nameClients(as)
	want := []string{
		"power control has 2.4 GHz at 8 dBm, where kettle (a8:b5:8e:8c:93:89), which no other AP hears better, hears it at -82 dBm",
		"kettle (a8:b5:8e:8c:93:89) came online here 30 times in the last hour",
		"30:86:2d:c3:0d:40, not one of the APs', is on your wire",
		"kettle (a8:b5:8e:8c:93:89),24:58:7c:4e:ed:dc and fe80::a8:b5:8e:8c:93:89:aa",
	}
	for i, a := range as {
		if a.Message != want[i] {
			t.Errorf("%d: %q\nwant %q", i, a.Message, want[i])
		}
	}
}

// An AP in maintenance (0218): its alerts are listed, said to be in
// maintenance and counted apart; past until, it is in service again.
func TestAlertsInMaintenance(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 90, "wireless_missing": true}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	set := func(values map[string]any) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "values": values}); code != 200 {
			t.Fatalf("set: %d %v", code, b)
		}
	}
	look := func() (map[string]any, map[string]any) {
		t.Helper()
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["ap"] == ap && m["kind"] == "wireless-missing" {
				return m, body["counts"].(map[string]any)
			}
		}
		t.Fatalf("no wireless-missing alert: %v", body)
		return nil, nil
	}
	set(map[string]any{"maintenance.on": true, "maintenance.until": "2099-01-01T12:00Z", "maintenance.why": "moving it"})
	a, c := look()
	if a["maintenance"] != "in maintenance until 2099-01-01 12:00 UTC: moving it" || c["critical"] != 0.0 || c["maintenance"] != 1.0 {
		t.Fatalf("in maintenance: %v, counts %v", a, c)
	}
	set(map[string]any{"maintenance.until": "2020-01-01"})
	if a, c := look(); a["maintenance"] != nil || c["critical"] != 1.0 || c["maintenance"] != 0.0 {
		t.Fatalf("past until: %v, counts %v", a, c)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "values": map[string]any{"maintenance.until": "2026-13-01"}}); code == 200 {
		t.Fatalf("a month that is none: %d %v", code, b)
	}
}

// A client of the APs' heard on a rogue in the last quarter hour is an
// alert on the AP that heard it (0160): not one no AP served, nor one on a
// BSSID its folders know, nor on one of the APs' own, nor one heard long
// ago.
func TestClientOnRogue(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	id := hierarchy.NodeID(ap)
	c := f.api.conds
	if err := c.SeeClients(id, []conditions.ClientSeen{{MAC: "a8:b5:8e:8c:93:89", SSID: "Home Wi-Fi", Host: "kettle"}, {MAC: "24:58:7c:4e:ed:dc", SSID: "Home Wi-Fi"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.NameClient("24:58:7c:4e:ed:dc", "closet UPS", "griff"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := c.RecordRogueClients(id, []conditions.RogueClient{
		{MAC: "a8:b5:8e:8c:93:89", BSSID: "30:86:2d:c3:0d:50", SSID: "C230-test", LastAt: now, Frames: 40},
		{MAC: "24:58:7c:4e:ed:dc", BSSID: "aa:aa:aa:aa:aa:01", SSID: "Known", LastAt: now, Frames: 9},
		{MAC: "e2:e3:40:cb:59:c5", BSSID: "30:86:2d:c3:0d:50", SSID: "C230-test", LastAt: now, Frames: 107},
		{MAC: "24:58:7c:4e:ed:dc", BSSID: "30:86:2d:c3:0d:40", SSID: "C230-test", LastAt: now.Add(-time.Hour), Frames: 3},
	}); err != nil {
		t.Fatal(err)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "path": "rogues.known", "value": []string{"AA:AA:AA:AA:AA:01"}}); code != 200 {
		t.Fatalf("known: %d %v", code, b)
	}
	got := f.api.onRogues(f.api.log.Snapshot(), now)[id]
	if len(got) != 1 || got[0].MAC != "a8:b5:8e:8c:93:89" || got[0].Who != "kettle (a8:b5:8e:8c:93:89)" || got[0].BSSID != "30:86:2d:c3:0d:50" || got[0].SSID != "C230-test" {
		t.Fatalf("on rogues = %+v", got)
	}
	_, body := f.do("GET", "/v1/alerts", "griff", nil)
	var keys []string
	for _, a := range body["alerts"].([]any) {
		if m := a.(map[string]any); m["kind"] == "client-on-rogue" {
			keys = append(keys, m["key"].(string))
			if !strings.Contains(m["message"].(string), `kettle (a8:b5:8e:8c:93:89), a client of the APs', was heard sending to 30:86:2d:c3:0d:50 ("C230-test")`) {
				t.Errorf("message = %s", m["message"])
			}
		}
	}
	if strings.Join(keys, " ") != "client-on-rogue:a8:b5:8e:8c:93:89@30:86:2d:c3:0d:50" {
		t.Fatalf("alerts = %v", keys)
	}
}

// An AP that runs RRM's planned moves by its own clock, with no zone set
// and its clock on UTC, says so (0262); its window is the field's default
// where none is set, a zone set ends it, and the tests on a schedule go by
// system.tz whatever its clock.
func TestClockZoneAlert(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	set := func(values map[string]any) float64 {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "values": values}); code != 200 {
			t.Fatalf("set: %d %v", code, b)
		}
		v, _ := f.api.log.Version(hierarchy.NodeID(ap))
		return float64(v)
	}
	said := func(rule any) string {
		t.Helper()
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 90,
			"time": map[string]any{"synced": true, "zone": "UTC", "rule": rule}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["ap"] == ap && m["kind"] == "clock-zone" {
				return m["message"].(string)
			}
		}
		return ""
	}
	if got := said("GMT0"); got != "" {
		t.Fatalf("nothing by local hours: %s", got)
	}
	version = set(map[string]any{"rrm.enabled": true})
	if got := said("GMT0"); !strings.Contains(got, "RRM's planned moves (02:00-05:00) happen at those hours in UTC") {
		t.Fatalf("RRM on: %q", got)
	}
	if got := said("CST6CDT,M3.2.0,M11.1.0"); got != "" {
		t.Fatalf("its clock has a zone: %s", got)
	}
	// An agent that does not say its clock's zone says nothing.
	if got := said(nil); got != "" {
		t.Fatalf("no rule: %s", got)
	}
	version = set(map[string]any{"assurance.tests.every": 15, "assurance.tests.window": "01:00-05:00", "rrm.window": "03:00-04:00"})
	if got := said("CST6CDT,M3.2.0,M11.1.0"); !strings.Contains(got, "so the client tests on a schedule (01:00-05:00) happen") || strings.Contains(got, "RRM") {
		t.Fatalf("by the manager: %q", got)
	}
	if got := said("UTC0"); !strings.Contains(got, "RRM's planned moves (03:00-04:00) and the client tests on a schedule (01:00-05:00)") {
		t.Fatalf("both: %q", got)
	}
	version = set(map[string]any{"system.tz": "America/Chicago"})
	if got := said("UTC0"); got != "" {
		t.Fatalf("a zone set: %s", got)
	}
}
