package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The alert log (0109): an alert that lasts alerts.Hold is logged from when
// it was first seen, or from when it says it began; a blip is not; its end
// is logged when it goes; and a manager that starts again ends what went
// while it was down. GET /v1/alerts/history reads it.
func TestAlertHistory(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	id := hierarchy.NodeID(ap)
	t0 := time.Now().Add(-30 * time.Minute)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	offline := alerts.Alert{AP: id, Name: "ShedAP", Severity: alerts.Critical, Kind: "offline", Key: "offline", Message: "not heard from"}
	blip := alerts.Alert{AP: id, Name: "ShedAP", Severity: alerts.Warning, Kind: "vlan-silent", Key: "vlan-silent:20", Message: "VLAN 20 is silent"}
	since := at(11).Add(-time.Hour)
	held := alerts.Alert{AP: id, Name: "ShedAP", Severity: alerts.Warning, Kind: "held", Key: "held", Message: "its config is held", Since: &since}
	r := newAlertRecorder()
	for _, step := range []struct {
		m   int
		now []alerts.Alert
	}{
		{0, []alerts.Alert{offline}},
		{1, []alerts.Alert{offline}},
		{2, []alerts.Alert{offline, blip}},
		{3, []alerts.Alert{offline}},
		{10, nil},
		{11, []alerts.Alert{held}},
	} {
		f.api.recordAlerts(r, at(step.m), step.now)
	}
	// The manager starts again; held went while it was down.
	f.api.recordAlerts(newAlertRecorder(), at(12), nil)
	// Another AP's alerts, newer and more than the limit, crowd out none of
	// this one's.
	for i := 0; i < 2100; i++ {
		if _, err := f.conds.BeginAlert(conditions.LoggedAlert{AP: "office-ap", Key: fmt.Sprintf("loop:%d", i), Kind: "loop", Severity: alerts.Critical, Message: "a loop", Began: at(20)}); err != nil {
			t.Fatal(err)
		}
	}
	history := func(who string) []any {
		t.Helper()
		code, body := f.do("GET", "/v1/alerts/history?under="+ap, who, nil)
		if code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		return body["alerts"].([]any)
	}
	got := history("griff")
	if len(got) != 2 {
		t.Fatalf("history = %v", got)
	}
	when := func(v any) time.Time {
		w, _ := time.Parse(time.RFC3339Nano, v.(string))
		return w
	}
	a, b := got[0].(map[string]any), got[1].(map[string]any)
	if a["key"] != "offline" || !when(a["began"]).Equal(at(0)) || !when(a["ended"]).Equal(at(10)) || a["name"] != "ShedAP" {
		t.Fatalf("offline = %v", a)
	}
	if b["key"] != "held" || !when(b["began"]).Equal(since) || !when(b["ended"]).Equal(at(12)) {
		t.Fatalf("held = %v", b)
	}
	if code, _ := f.do("GET", "/v1/alerts/history?hours=0", "griff", nil); code != 400 {
		t.Fatalf("hours=0: %d", code)
	}
	if got := history("office"); len(got) != 0 {
		t.Fatalf("office sees %v", got)
	}
}

// An alert lists since when it began, where the alert log has it earlier
// than its cause says: a rogue's report says when it was last heard, and it
// has been heard at each scan for hours.
func TestAlertSinceFromTheLog(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	report := map[string]any{"version": version, "uptime": 60, "bssids": []any{"02:00:00:00:00:01"},
		"rrm": map[string]any{"address": "192.168.1.38", "advertised": []any{}, "neighbours": []any{}, "ratings": []any{}, "moves": []any{},
			"others": []any{
				map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "Home Wi-Fi", "band": "2g", "channel": 11, "signal": -61, "ago": 20},
				map[string]any{"bssid": "e4:d1:24:08:6d:12", "ssid": "Home Wi-Fi", "band": "2g", "channel": 6, "signal": -70, "ago": 20},
			}}}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	began := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Second)
	if _, err := f.conds.BeginAlert(conditions.LoggedAlert{AP: hierarchy.NodeID(ap), Key: "rogue:e4:d1:24:08:6d:11", Kind: "rogue", Severity: alerts.Warning, Message: "a rogue", Began: began}); err != nil {
		t.Fatal(err)
	}
	_, body := f.do("GET", "/v1/alerts", "griff", nil)
	since := map[string]time.Time{}
	for _, a := range body["alerts"].([]any) {
		m := a.(map[string]any)
		if s, ok := m["since"].(string); ok {
			at, _ := time.Parse(time.RFC3339Nano, s)
			since[m["key"].(string)] = at
		}
	}
	if got := since["rogue:e4:d1:24:08:6d:11"]; !got.Equal(began) {
		t.Errorf("the logged rogue is since %s, want %s", got, began)
	}
	// One not logged yet is since its report says.
	if got := since["rogue:e4:d1:24:08:6d:12"]; time.Since(got) > time.Minute {
		t.Errorf("the rogue not logged is since %s", got)
	}
}

// An alert that comes back saying it began when its last row did is that
// row taken up again (0306): the same episode, whose cause flickered, as a
// failing test's alert does while its tester hears its AP now and then. One
// that says it began anew is a row of its own.
func TestAlertEpisodeTakenUpAgain(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	id := hierarchy.NodeID(ap)
	t0 := time.Now().Add(-30 * time.Minute)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	since := t0.Add(-10 * time.Hour)
	test := alerts.Alert{AP: id, Name: "ShedAP", Severity: alerts.Warning, Kind: "client-test", Key: "client-test:enterprise:2g:Garage", Message: "failed at dhcp", Since: &since}
	r := newAlertRecorder()
	for _, step := range []struct {
		m   int
		now []alerts.Alert
	}{{0, []alerts.Alert{test}}, {5, nil}, {20, []alerts.Alert{test}}, {25, []alerts.Alert{test}}} {
		f.api.recordAlerts(r, at(step.m), step.now)
	}
	rows, err := f.conds.AlertLog(t0.Add(-24*time.Hour), []hierarchy.NodeID{id}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Ended != nil || !rows[0].Began.Equal(since) {
		t.Fatalf("one row, open again: %+v", rows)
	}
	// It ends, and comes back saying it began anew: a row of its own.
	later := at(30)
	anew := test
	anew.Since = &later
	f.api.recordAlerts(r, at(28), nil)
	f.api.recordAlerts(r, at(33), []alerts.Alert{anew})
	if rows, _ = f.conds.AlertLog(t0.Add(-24*time.Hour), []hierarchy.NodeID{id}, 10); len(rows) != 2 {
		t.Fatalf("a new episode, a new row: %+v", rows)
	}
}
