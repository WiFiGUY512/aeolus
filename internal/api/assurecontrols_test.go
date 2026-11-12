package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The controls of the tests on a schedule (0180): the networks they join
// and leave out, which APs a tester joins on and how weak, the load that
// puts one off, and a pause that keeps the schedule.
func TestAssuranceControls(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "values": map[string]any{
		"network.lab.ssid": "Lab", "network.lab.security": "wpa2-psk", "network.lab.passphrase": "lab-passphrase-1",
		"network.lab.transport.primary.type": "vlan", "network.lab.transport.primary.vlan": 20,
	}}); code != 200 {
		t.Fatalf("lab network: %d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:4a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:4b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	far := strings.Split(f.enrollAt("Far", "02:00:00:00:00:4c", "office", []any{serving}, map[string]int{"5g": 149}), "|")
	names := map[string]string{scanner[0]: "scanner", plain[0]: "plain", far[0]: "far"}

	// say has an AP report, with how many clients its 5 GHz radio has and
	// how busy its channel is.
	say := func(tok string, clients, busy int, more map[string]any) {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", tok, nil, nil)
		body := map[string]any{"version": cfg["version"], "uptime": 60,
			"radios": []any{map[string]any{"radio": "radio0", "band": "5g", "channel": 44, "clients": clients}}}
		rrm := map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"}, "neighbours": []any{}, "moves": []any{},
			"ratings": []any{map[string]any{"band": "5g", "channel": 44, "cost": busy, "now": busy, "busy": busy, "noise": -95, "networks": 0, "visits": 5, "ago": 10, "own": true, "blotted_by": []string{}, "best": true}}}
		for k, v := range more {
			if k == "neighbours" {
				rrm["neighbours"] = v
				continue
			}
			body[k] = v
		}
		body["rrm"] = rrm
		if code, _, b := f.apDo("POST", "/v1/ap/state", tok, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	// hears is the scanner hearing an AP at signal, and that AP hearing
	// the scanner at theirs.
	hears := func(ap string, signal, theirs int) map[string]any {
		return map[string]any{"ap": ap, "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
			"bands": []any{map[string]any{"band": "5g", "signal": signal, "their_signal": theirs, "channel": 44, "width": 40}}}
	}
	report := func(scannerClients, plainClients, plainBusy int) {
		t.Helper()
		say(plain[1], plainClients, plainBusy, map[string]any{"bssids": []string{"02:00:00:00:00:4b"}})
		say(far[1], 0, 0, map[string]any{"bssids": []string{"02:00:00:00:00:4c"}})
		say(scanner[1], scannerClients, 0, map[string]any{"bssids": []string{"02:00:00:00:00:4a"},
			"neighbours": []any{hears(plain[0], -50, -52), hears(far[0], -70, -82)}})
	}
	set := func(field string, v any) {
		t.Helper()
		op := map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests." + field, "value": v}
		if v == nil {
			op = map[string]any{"kind": "unset", "tree": "locations", "node": "office", "path": "assurance.tests." + field}
		}
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%s: %d %v", field, code, b)
		}
	}
	report(0, 0, 0)
	set("every", 15)

	// What the scanner tests on its schedule, as network@AP.
	combos := func() string {
		var out []string
		for _, c := range f.api.assureCombos(f.log.Snapshot(), hierarchy.NodeID(scanner[0]), map[hierarchy.NodeID]map[string]any{}) {
			out = append(out, c.Network+"@"+names[c.On])
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		name  string
		field string
		value any
		want  string
	}{
		// Far is heard at -70, and hears the scanner at -82: the weaker way
		// is the path's.
		{"nothing set: every network, on itself and Plain, heard both ways above -75", "", nil, "lab@plain lab@scanner sweet@plain sweet@scanner"},
		{"a floor of -85 takes Far in", "floor", -85, "lab@far lab@plain lab@scanner sweet@far sweet@plain sweet@scanner"},
		{"on itself alone", "on", "self", "lab@scanner sweet@scanner"},
		{"on listed APs, itself not among them", "on", "listed", "lab@plain sweet@plain"},
		{"one network only", "networks", []string{"sweet"}, "sweet@plain"},
		{"a band it is not on (0181)", "bands", []string{"2g"}, ""},
		{"the band it is on", "bands", []string{"5g", "6g"}, "sweet@plain"},
		{"that network left out", "leave_out", []string{"sweet"}, ""},
	} {
		if c.field == "on" && c.value == "listed" {
			set("listed", []string{plain[0], far[0]})
			set("floor", nil)
		}
		if c.field != "" {
			set(c.field, c.value)
		}
		if got := combos(); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	for _, field := range []string{"on", "listed", "networks", "leave_out", "bands"} {
		set(field, nil)
	}
	if got := combos(); got != "lab@plain lab@scanner sweet@plain sweet@scanner" {
		t.Fatalf("all unset again: %q", got)
	}

	// asked is what one look asks of the scanner: network@AP, or "".
	asked := func() string {
		t.Helper()
		got := f.api.assure(f.now())
		if len(got) == 0 {
			return ""
		}
		var p struct{ Network, On string }
		json.Unmarshal(got[0].Params, &p)
		// Taken up and done, so the next look may ask again.
		f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil)
		f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", got[0].ID), scanner[1], map[string]any{"ok": false, "result": "cancelled", "detail": nil}, nil)
		return p.Network + "@" + names[p.On]
	}

	// Paused: nothing is asked, and the schedule is kept.
	set("paused", true)
	if a := asked(); a != "" {
		t.Errorf("paused, it asked %s", a)
	}
	set("paused", false)

	// Out of its window, nothing is asked; in it, the schedule goes on
	// (0181). No zone is set here, so the window is in UTC.
	hhmm := func(d time.Duration) string { return f.now().UTC().Add(d).Format("15:04") }
	set("window", hhmm(2*time.Hour)+"-"+hhmm(3*time.Hour))
	if a := asked(); a != "" {
		t.Errorf("out of its window, it asked %s", a)
	}
	set("window", hhmm(-time.Hour)+"-"+hhmm(time.Hour))
	if a := asked(); a == "" {
		t.Errorf("in its window, it asked nothing")
	}
	set("window", nil)
	// Past that test's turn, for the next look to ask again.
	*f.skew += 16 * time.Minute

	// Load: the scanner's own radio busy with clients puts every test
	// through the air off; Plain's puts off those joined on Plain.
	set("max_clients", 3)
	report(5, 0, 0)
	if a := asked(); a != "" {
		t.Errorf("the tester's radio over max_clients, it asked %s", a)
	}
	report(0, 5, 0)
	set("on", "listed")
	set("listed", []string{plain[0]})
	if a := asked(); a != "" {
		t.Errorf("Plain's radio over max_clients, it asked %s", a)
	}
	set("max_clients", nil)
	set("max_busy", 50)
	report(0, 0, 80)
	if a := asked(); a != "" {
		t.Errorf("Plain's channel busier than max_busy, it asked %s", a)
	}
	report(0, 0, 20)
	if a := asked(); !strings.HasSuffix(a, "@plain") {
		t.Errorf("with Plain's load light again, it asked %q", a)
	}

	// A network's own alert_after (0182) is read for its testers, and is
	// sent to no AP.
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.alert_after", "value": 3}); code != 200 {
		t.Fatalf("network alert_after: %d %v", code, b)
	}
	set("alert_after", 5)
	if c := assureControls(f.log.Snapshot(), hierarchy.NodeID(scanner[0])); c.failsAfter("sweet") != 3 || c.failsAfter("lab") != 5 {
		t.Errorf("sweet %d (want 3), lab %d (want 5)", c.failsAfter("sweet"), c.failsAfter("lab"))
	}
	_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
	if sweet, _ := cfg["config"].(map[string]any)["network"].(map[string]any)["sweet"].(map[string]any); sweet == nil || sweet["alert_after"] != nil {
		t.Errorf("the AP's sweet: %v", sweet)
	}

	// The scan radio's budget (0182): one test through the air an hour.
	// The one just asked spends it; the next turn waits for the hour.
	set("max_busy", nil)
	set("per_hour", 1)
	*f.skew += 16 * time.Minute
	report(0, 0, 0)
	if a := asked(); a != "" {
		t.Errorf("its budget spent, it asked %s", a)
	}
	*f.skew += 45 * time.Minute
	report(0, 0, 0)
	if a := asked(); a == "" {
		t.Errorf("an hour on, it asked nothing")
	}
}

// How many failures running make an alert (0182): two unless the tester's
// folders say, and a network's own number first.
func TestAlertAfter(t *testing.T) {
	c := anyControls
	if c.failsAfter("sweet") != 2 {
		t.Errorf("unset, %d", c.failsAfter("sweet"))
	}
	c.alertAfter = 4
	c.netAlert = map[string]int{"guest": 1}
	if c.failsAfter("sweet") != 4 || c.failsAfter("guest") != 1 {
		t.Errorf("folders 4, guest 1: sweet %d, guest %d", c.failsAfter("sweet"), c.failsAfter("guest"))
	}
	// Three failures running, then a pass before them.
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	rec := func(network, outcome string, ago time.Duration) testRecord {
		return testRecord{Action: conditions.Action{AP: "ap-000000000001", At: at.Add(-ago)},
			ask:    clientTestAsk{Network: network, On: "ap-000000000001", Band: "5g"},
			detail: &clientTest{Client: "02:00:00:00:00:fe", Outcome: outcome}}
	}
	var list []testRecord
	for _, n := range []string{"sweet", "guest"} {
		list = append(list, rec(n, "failed", time.Minute), rec(n, "failed", 2*time.Minute), rec(n, "failed", 3*time.Minute), rec(n, "passed", 4*time.Minute))
	}
	failing := func(after func(hierarchy.NodeID, assureCombo) int) string {
		var out []string
		for k, v := range assureVerdicts(list, time.Time{}, after) {
			if v.failing {
				out = append(out, k.combo.Network)
			}
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	if got := failing(nil); got != "guest sweet" {
		t.Errorf("two running make an alert: %q", got)
	}
	by := func(_ hierarchy.NodeID, combo assureCombo) int { return c.failsAfter(combo.Network) }
	if got := failing(by); got != "guest" {
		t.Errorf("sweet after 4, guest after 1, three running: %q", got)
	}
}

// A window in the AP's local time (0181): by its zone, past midnight where
// it ends before it starts, and the whole day where it starts where it
// ends.
func TestTestWindow(t *testing.T) {
	// 23:30 in Chicago, which is 04:30 UTC the next day in October.
	at := time.Date(2026, 10, 21, 4, 30, 0, 0, time.UTC)
	for _, c := range []struct {
		window, tz string
		open       bool
	}{
		{"", "", true},
		{"23:00-23:59", "America/Chicago", true},
		{"23:00-23:59", "", false},
		{"04:00-05:00", "", true},
		{"22:00-02:00", "America/Chicago", true},
		{"01:00-05:00", "America/Chicago", false},
		{"08:00-08:00", "America/Chicago", true},
		{"18:00-08:00", "America/Chicago", true},
		{"08:00-18:00", "America/Chicago", false},
	} {
		if got := (controls{window: c.window, tz: c.tz}).open(at); got != c.open {
			t.Errorf("%q in %q: open %v, want %v", c.window, c.tz, got, c.open)
		}
	}
}
