package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Client tests on a schedule (0123): the manager asks each AP with a scan
// radio for one in turn, tries a failure again within minutes, and a
// network that fails twice running is an alert on the AP it failed on.
func TestAssurance(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:4a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:4b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	far := strings.Split(f.enrollAt("Far", "02:00:00:00:00:4c", "office", []any{serving}, map[string]int{"5g": 149}), "|")

	// Each says what it runs and its BSSIDs; the scanner hears Plain well on
	// 5 GHz, and Far only faintly.
	say := func(tok string, more map[string]any) {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", tok, nil, nil)
		body := map[string]any{"version": cfg["version"], "uptime": 60}
		for k, v := range more {
			body[k] = v
		}
		if code, _, b := f.apDo("POST", "/v1/ap/state", tok, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	hears := func(ap string, signal int) map[string]any {
		return map[string]any{"ap": ap, "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
			"bands": []any{map[string]any{"band": "5g", "signal": signal, "their_signal": signal, "channel": 44, "width": 40}}}
	}
	all := func() {
		say(plain[1], map[string]any{"bssids": []string{"02:00:00:00:00:4b"}})
		say(far[1], map[string]any{"bssids": []string{"02:00:00:00:00:4c"}})
		say(scanner[1], map[string]any{"bssids": []string{"02:00:00:00:00:4a"}, "rrm": map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"},
			"neighbours": []any{hears(plain[0], -50), hears(far[0], -82)}, "ratings": []any{}, "moves": []any{}}})
	}
	all()

	// Nothing is asked until a folder says how often.
	if got := f.api.assure(f.now()); len(got) != 0 {
		t.Fatalf("asked with no schedule: %v", got)
	}
	for path, v := range map[string]any{"assurance.tests.every": 15, "system.speed_server": "http://192.168.20.106"} {
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": path, "value": v}); code != 200 {
			t.Fatalf("%s: %d %v", path, code, b)
		}
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 7}); code == 200 {
		t.Fatalf("every 7 minutes was taken: %v", b)
	}
	// The schedule is the manager's: it is not in an AP's config, and makes
	// it no new version. The speed server is the AP's, and does.
	all()
	_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
	if _, ok := cfg["config"].(map[string]any)["assurance"]; ok {
		t.Fatalf("the schedule is in the AP's config: %v", cfg["config"])
	}

	// The scanner alone is asked: of its one testable network, on itself
	// first, then on Plain, which it hears well; never on Far.
	type asked struct {
		id      int64
		on      string
		speed   bool
		network string
	}
	next := func() *asked {
		t.Helper()
		got := f.api.assure(f.now())
		if len(got) == 0 {
			return nil
		}
		if len(got) != 1 || got[0].AP != scanner[0] || got[0].Actor != "aeolus" || got[0].Kind != "client-test" {
			t.Fatalf("asked %v", got)
		}
		var p struct {
			Network, On, Band, Speed string
			BSSIDs                   []string
		}
		json.Unmarshal(got[0].Params, &p)
		if p.Band != "5g" || len(p.BSSIDs) != 1 {
			t.Fatalf("asked for %v", string(got[0].Params))
		}
		return &asked{id: got[0].ID, on: p.On, speed: p.Speed != "", network: p.Network}
	}
	signal := -70
	done := func(a *asked, outcome string) {
		t.Helper()
		// The AP takes it up, and says what it found.
		if _, _, b := f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil); len(b["actions"].([]any)) != 1 {
			t.Fatalf("the AP was handed %v", b)
		}
		d := map[string]any{"v": 1, "ssid": "Home Wi-Fi", "band": "5g", "bssid": "02:00:00:00:00:4b", "channel": 44, "client": "02:00:00:00:00:fe",
			"outcome": outcome, "stage": "done", "reason": "", "steps": []any{}}
		if outcome != "passed" {
			d["stage"], d["reason"] = "dhcp", "no DHCP server gave it an address"
			d["link"] = map[string]any{"signal": signal, "tx": map[string]any{"mbps": 6}}
		}
		if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", a.id), scanner[1], map[string]any{"ok": outcome == "passed", "result": outcome, "detail": d}, nil); code != 200 {
			t.Fatalf("done: %d %v", code, b)
		}
	}
	later := func(d time.Duration) {
		*f.skew += d
		all()
	}
	first := next()
	if first == nil || first.network != "sweet" || first.on != scanner[0] || first.speed {
		t.Fatalf("the first asked = %+v", first)
	}
	// While one is open, and until its time comes round, no other is asked.
	if a := next(); a != nil {
		t.Fatalf("a second while the first is open: %+v", a)
	}
	done(first, "passed")
	later(5 * time.Minute)
	if a := next(); a != nil {
		t.Fatalf("a second after 5 minutes of 15: %+v", a)
	}
	later(11 * time.Minute)
	second := next()
	if second == nil || second.on != plain[0] {
		t.Fatalf("the second asked = %+v, for Plain %s", second, plain[0])
	}

	// It fails. That is tried again within minutes, not in its turn.
	done(second, "failed")
	alerts := func() []any {
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		var out []any
		for _, a := range body["alerts"].([]any) {
			if a.(map[string]any)["kind"] == "client-test" {
				out = append(out, a)
			}
		}
		return out
	}
	if got := alerts(); len(got) != 0 {
		t.Fatalf("an alert after one failure: %v", got)
	}
	later(time.Minute)
	if a := next(); a != nil {
		t.Fatalf("tried again after a minute: %+v", a)
	}
	later(2 * time.Minute)
	again := next()
	if again == nil || again.on != plain[0] {
		t.Fatalf("tried again = %+v", again)
	}
	done(again, "failed")

	// Twice running: an alert on the AP it failed on, saying who tried, and
	// how strongly it had the AP.
	got := alerts()
	if len(got) != 1 {
		t.Fatalf("alerts after two failures: %v", got)
	}
	a := got[0].(map[string]any)
	if a["ap"] != plain[0] || a["severity"] != "warning" || !strings.Contains(a["message"].(string), "Home Wi-Fi does not work here on 5 GHz, tried as a client from Scanner, at -70 dBm:") ||
		!strings.Contains(a["message"].(string), "failed at dhcp: no DHCP server gave it an address") {
		t.Fatalf("the alert = %v", a)
	}

	// Plain takes Home Wi-Fi off the air, its way on lost (0201): a test
	// could not find it there, which is no failing test, as the off-air
	// alert says why; nor is the scanner asked to look for it there.
	say(plain[1], map[string]any{"bssids": []string{"02:00:00:00:00:4b"}, "off_air": map[string]any{"sweet": map[string]any{"off": true, "why": "its VLAN 20 does not answer"}}})
	if got := alerts(); len(got) != 0 {
		t.Fatalf("a client-test alert on an AP with the network off the air: %v", got)
	}
	for _, c := range f.api.assureCombos(f.log.Snapshot(), hierarchy.NodeID(scanner[0]), map[hierarchy.NodeID]map[string]any{}) {
		if c.On == plain[0] && c.Band != wireBand {
			t.Fatalf("asked to test through the air on an AP with the network off the air: %+v", c)
		}
	}
	all()
	if got := alerts(); len(got) != 1 {
		t.Fatalf("alerts once it is on the air again: %v", got)
	}

	// What a person sees of it.
	_, view := f.do("GET", "/v1/assurance?under=office", "griff", nil)
	testers := view["testers"].([]any)
	checks := view["checks"].([]any)
	if len(testers) != 1 || testers[0].(map[string]any)["name"] != "Scanner" || testers[0].(map[string]any)["every"] != float64(15) || testers[0].(map[string]any)["things"] != float64(2) {
		t.Fatalf("testers = %v", testers)
	}
	if len(checks) != 2 || len(view["tests"].([]any)) != 3 {
		t.Fatalf("checks = %v, tests = %d", checks, len(view["tests"].([]any)))
	}
	worst := checks[0].(map[string]any)
	if worst["on"] != plain[0] || worst["on_name"] != "Plain" || worst["failing"] != true || worst["failed"] != float64(2) || worst["passed"] != float64(0) ||
		worst["ssid"] != "Home Wi-Fi" || worst["scheduled"] != true || worst["last"].(map[string]any)["state"] != "failed" {
		t.Fatalf("the failing check = %v", worst)
	}
	if ok := checks[1].(map[string]any); ok["on"] != scanner[0] || ok["failing"] != false || ok["passed"] != float64(1) {
		t.Fatalf("the passing check = %v", ok)
	}
	// Each with its tests in brief, the oldest first.
	runs := worst["runs"].([]any)
	if len(runs) != 2 || runs[0].(map[string]any)["passed"] != false || runs[0].(map[string]any)["stage"] != "dhcp" ||
		runs[0].(map[string]any)["at"].(string) > runs[1].(map[string]any)["at"].(string) {
		t.Fatalf("the failing check's runs = %v", runs)
	}
	if r := checks[1].(map[string]any)["runs"].([]any); len(r) != 1 || r[0].(map[string]any)["passed"] != true || r[0].(map[string]any)["stage"] != nil {
		t.Fatalf("the passing check's runs = %v", r)
	}
	// In brief, as the Overview asks (0285): the same checks, without their
	// runs, and their last test in brief (0287); no tests listed.
	_, brief := f.do("GET", "/v1/assurance?under=office&brief=1", "griff", nil)
	if _, ok := brief["tests"]; ok || len(brief["testers"].([]any)) != 1 || len(brief["checks"].([]any)) != 2 {
		t.Fatalf("brief = %v", brief)
	}
	bw := brief["checks"].([]any)[0].(map[string]any)
	if bw["failing"] != true || bw["runs"] != nil || bw["last"].(map[string]any)["state"] != "failed" {
		t.Fatalf("the failing check in brief = %v", bw)
	}
	if d, _ := bw["last"].(map[string]any)["detail"].(map[string]any); d == nil || d["outcome"] != "failed" || d["link"] != nil || d["way"] != nil {
		t.Fatalf("the failing check's last test in brief = %v", bw["last"])
	}
	// Each test in brief, as the Assurance tab asks (0287): a check's last
	// test and each one listed say how it came out and what it was of, not
	// what it found; the whole of one is fetched by itself.
	_, lean := f.do("GET", "/v1/assurance?under=office&tests=brief", "griff", nil)
	lw := lean["checks"].([]any)[0].(map[string]any)
	last := lw["last"].(map[string]any)
	if d := last["detail"].(map[string]any); d["outcome"] != "failed" || d["stage"] != "dhcp" || d["reason"] != "no DHCP server gave it an address" || d["link"] != nil || d["way"] != nil {
		t.Fatalf("the failing check's last test in brief = %v", last)
	}
	if p := last["params"].(map[string]any); p["network"] != "sweet" || p["on"] != plain[0] || p["band"] != "5g" || p["bssids"] != nil {
		t.Fatalf("its params in brief = %v", p)
	}
	if len(lw["runs"].([]any)) != 2 || len(lean["tests"].([]any)) != 3 {
		t.Fatalf("in brief, runs = %v, tests = %v", lw["runs"], lean["tests"])
	}
	if d := lean["tests"].([]any)[0].(map[string]any)["detail"].(map[string]any); d["outcome"] == nil || d["way"] != nil || d["ssid"] != nil {
		t.Fatalf("a test listed in brief = %v", d)
	}
	one := fmt.Sprintf("/v1/aps/%s/actions/%d", scanner[0], int64(last["id"].(float64)))
	if code, a := f.do("GET", one, "griff", nil); code != 200 || a["state"] != "failed" || a["detail"].(map[string]any)["link"] == nil || a["detail"].(map[string]any)["way"] == nil {
		t.Fatalf("the test in full: %d %v", code, a)
	}
	if code, _ := f.do("GET", fmt.Sprintf("/v1/aps/%s/actions/%d", plain[0], int64(last["id"].(float64))), "griff", nil); code != 404 {
		t.Fatalf("the test asked of an AP that did not make it: %d", code)
	}
	if code, _ := f.do("GET", one, "tenant", nil); code != 404 {
		t.Fatalf("the test asked by one who may not view its AP: %d", code)
	}
	if code, _ := f.do("GET", "/v1/aps/"+scanner[0]+"/actions/x", "griff", nil); code != 404 {
		t.Fatalf("a test by no number: %d", code)
	}
	if code, _ := f.do("GET", "/v1/assurance?hours=0", "griff", nil); code != 400 {
		t.Fatalf("hours=0: %d", code)
	}
	if _, v := f.do("GET", "/v1/assurance", "tenant", nil); len(v["testers"].([]any)) != 0 || len(v["tests"].([]any)) != 0 {
		t.Fatalf("a tenant sees %v", v)
	}

	// A failing thing is tested every other time, so its mending is seen
	// soon: next the scanner itself, in its turn, then Plain again.
	later(16 * time.Minute)
	turn := next()
	if turn == nil || turn.on != scanner[0] {
		t.Fatalf("after two failures, in its turn = %+v", turn)
	}
	done(turn, "passed")
	later(16 * time.Minute)
	back := next()
	if back == nil || back.on != plain[0] {
		t.Fatalf("the failing one again = %+v", back)
	}
	done(back, "passed")
	if got := alerts(); len(got) != 0 {
		t.Fatalf("alerts once it passed: %v", got)
	}

	// With speed asked for, a test on a schedule measures it.
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.speed", "value": true}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	later(16 * time.Minute)
	if a := next(); a == nil || !a.speed {
		t.Fatalf("with speed = %+v", a)
	}

	// An AP that stopped calling is asked nothing.
	*f.skew += 16 * time.Minute
	if a := f.api.assure(f.now()); len(a) != 0 {
		t.Fatalf("asked of an AP not heard for 16 minutes: %v", a)
	}

	// Old tests are forgotten: the five that were done, and the one its AP
	// never took up, which expired.
	if n, err := f.conds.PruneClientTests(f.now().Add(time.Hour)); err != nil || n != 6 {
		t.Fatalf("forgot %d, %v", n, err)
	}
	var left []conditions.Action
	if left, _ = f.conds.ClientTests(f.now().Add(-48 * time.Hour)); len(left) != 0 {
		t.Fatalf("left = %v", left)
	}
}

// A thing that cannot be asked, as its AP has not said its BSSIDs, does not
// hold up the others.
func TestAssuranceAsksWhatItCan(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:5a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:5b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	say := func() {
		_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
		if code, _, b := f.apDo("POST", "/v1/ap/state", scanner[1], map[string]any{"version": cfg["version"], "uptime": 60, "bssids": []string{"02:00:00:00:00:5a"},
			"rrm": map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"}, "ratings": []any{}, "moves": []any{},
				"neighbours": []any{map[string]any{"ap": plain[0], "address": "192.0.2.2", "state": "up", "chosen": true, "hello_ago": 3,
					"bands": []any{map[string]any{"band": "5g", "signal": -50, "their_signal": -52, "channel": 44, "width": 40}}}}}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	on := func() string {
		t.Helper()
		say()
		got := f.api.assure(f.now())
		if len(got) != 1 {
			t.Fatalf("asked %v", got)
		}
		var p struct{ On string }
		json.Unmarshal(got[0].Params, &p)
		if code, _, b := f.apDo("GET", "/v1/ap/actions", scanner[1], nil, nil); code != 200 || len(b["actions"].([]any)) != 1 {
			t.Fatalf("handed %d %v", code, b)
		}
		d := map[string]any{"v": 1, "client": "02:00:00:00:00:fe", "outcome": "passed", "stage": "done", "reason": "", "steps": []any{}}
		if code, _, b := f.apDo("POST", fmt.Sprintf("/v1/ap/actions/%d", got[0].ID), scanner[1], map[string]any{"ok": true, "result": "passed", "detail": d}, nil); code != 200 {
			t.Fatalf("done: %d %v", code, b)
		}
		*f.skew += 16 * time.Minute
		return p.On
	}
	// Plain has not said its BSSIDs: the scanner tests itself, and again.
	if a, b := on(), on(); a != scanner[0] || b != scanner[0] {
		t.Fatalf("asked on %s then %s", a, b)
	}
	// Once it has, its turn comes.
	_, _, cfg := f.apDo("GET", "/v1/ap/config", plain[1], nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", plain[1], map[string]any{"version": cfg["version"], "uptime": 60, "bssids": []string{"02:00:00:00:00:5b"}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	if a := on(); a != plain[0] {
		t.Fatalf("asked on %s, for Plain %s", a, plain[0])
	}
}

// A test through the air that failed with the AP it joined heard below
// its tester's floor failed for the distance (0180): no evidence for an
// alert, as where power control has just turned that AP down. One at the
// floor, one with no signal said, and one over the wire are kept.
func TestWithinReach(t *testing.T) {
	f := newFixture(t)
	state := f.log.Snapshot()
	at := func(sig *float64, band, outcome string) testRecord {
		return testRecord{
			Action: conditions.Action{AP: "office-ap", State: "done"},
			ask:    clientTestAsk{Band: band},
			detail: &clientTest{Client: "02:00:00:00:00:fe", Outcome: outcome, Link: &testLink{Signal: sig}},
		}
	}
	f79, f75, f60 := -79.0, -75.0, -60.0
	list := []testRecord{
		at(&f79, "5g", "failed"),
		at(&f75, "5g", "failed"),
		at(&f60, "5g", "failed"),
		at(nil, "5g", "failed"),
		at(&f79, "5g", "passed"),
		at(&f79, wireBand, "failed"),
	}
	got := withinReach(state, list)
	if len(got) != 5 {
		t.Fatalf("kept %d of 6", len(got))
	}
	for _, r := range got {
		if r.detail.Link.Signal == &f79 && r.failed() && r.ask.Band != wireBand {
			t.Fatal("kept a failure below the floor")
		}
	}
}
