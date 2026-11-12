package api

import (
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// stickyMinutes are a client's minutes on an AP and band, one a minute
// from start, each at the signal given.
func stickyMinutes(mac string, ap hierarchy.NodeID, band string, start time.Time, sigs ...int) []conditions.ClientMinute {
	var out []conditions.ClientMinute
	for i, s := range sigs {
		s := s
		out = append(out, conditions.ClientMinute{MAC: mac, At: start.Add(time.Duration(i) * time.Minute), AP: ap, Band: band, SSID: "Home_Wi-Fi_IoT", Signal: &s, Secs: 60})
	}
	return out
}

// repeat is n of a signal.
func repeat(sig, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = sig
	}
	return out
}

// Where a client held on (0340): half an hour or more weak on one AP and
// band. The better place is another AP on the band that heard it 10 dB or
// more better over ten minutes; a late move is the next minute on another
// AP, 15 dB or more stronger. Not a place on another band, nor too short a
// time there, nor a move after a gap.
func TestStickyOf(t *testing.T) {
	name := func(id hierarchy.NodeID) string { return strings.ToUpper(string(id)) }
	now := time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC)
	start := now.Add(-3 * time.Hour)
	var m []conditions.ClientMinute
	// An hour on the shed at -84, then the living room at -50 the
	// next minute, for twenty; then an hour on the shed again, then
	// after a ten-minute gap the office at -55 (not a late move).
	m = append(m, stickyMinutes("aa:00:00:00:00:01", "pump", "2g", start, repeat(-84, 60)...)...)
	m = append(m, stickyMinutes("aa:00:00:00:00:01", "living", "2g", start.Add(60*time.Minute), repeat(-50, 20)...)...)
	m = append(m, stickyMinutes("aa:00:00:00:00:01", "pump", "2g", start.Add(80*time.Minute), repeat(-85, 60)...)...)
	m = append(m, stickyMinutes("aa:00:00:00:00:01", "office", "2g", start.Add(150*time.Minute), repeat(-55, 5)...)...)
	// Heard well on 5 GHz too, which is no place for 2.4 GHz.
	m = append(m, stickyMinutes("aa:00:00:00:00:01", "living", "5g", start.Add(156*time.Minute), repeat(-40, 20)...)...)
	got := stickyOf(m, now, name)
	if len(got) != 1 {
		t.Fatalf("places held on = %+v", got)
	}
	sc := got[0]
	if sc.AP != "pump" || sc.APName != "PUMP" || sc.Band != "2g" || sc.Minutes != 120 || sc.WeakMinutes != 120 || sc.Signal != -84 || sc.Now ||
		strings.Join(sc.Bands, ",") != "2g,5g" || sc.SSID != "Home_Wi-Fi_IoT" {
		t.Errorf("held on = %+v", sc)
	}
	if b := sc.Better; b == nil || b.AP != "living" || b.Signal != -50 || b.Minutes != 20 {
		t.Errorf("better = %+v", sc.Better)
	}
	if l := sc.Late; l == nil || l.Count != 1 || l.AP != "living" || l.From != -84 || l.To != -50 || !l.At.Equal(start.Add(60*time.Minute)) {
		t.Errorf("late = %+v", sc.Late)
	}

	// Twenty-nine weak minutes is not enough; nor is a client heard at
	// -77 for hours: -78 and weaker is weak.
	m = stickyMinutes("aa:00:00:00:00:02", "pump", "2g", start, repeat(-84, 29)...)
	m = append(m, stickyMinutes("aa:00:00:00:00:02", "pump", "2g", start.Add(30*time.Minute), repeat(-77, 120)...)...)
	if got := stickyOf(m, now, name); len(got) != 0 {
		t.Errorf("not held on = %+v", got)
	}
	// Weak, with nowhere better: listed, with no better place; there now.
	m = stickyMinutes("aa:00:00:00:00:03", "pump", "2g", now.Add(-40*time.Minute), repeat(-92, 40)...)
	if got := stickyOf(m, now, name); len(got) != 1 || got[0].Better != nil || got[0].Late != nil || !got[0].Now {
		t.Errorf("weak alone = %+v", got)
	}
}

// What a listed client is, and what may help it (0340).
func TestStickyHelp(t *testing.T) {
	better := &stickyPlace{AP: "living", APName: "Living"}
	for _, c := range []struct {
		what  string
		sc    stickyClient
		class string
		help  string
		ap    string
	}{
		{"pinned here", stickyClient{AP: "pump", Better: better, Steered: true, Rule: &stickyRule{APs: []stickyAPName{{ID: "pump"}}}}, "pinned", stickyHelpPinned, ""},
		{"pinned here, moved", stickyClient{AP: "pump", Signal: -56, Recent: ptr(-85), Rule: &stickyRule{APs: []stickyAPName{{ID: "pump"}}}}, "pinned", stickyHelpPinMoved, ""},
		{"pinned here, weak as ever", stickyClient{AP: "pump", Signal: -82, Recent: ptr(-85), Rule: &stickyRule{APs: []stickyAPName{{ID: "pump"}}}}, "pinned", stickyHelpPinned, ""},
		{"nowhere better", stickyClient{AP: "pump", Steered: true}, "weak", stickyHelpCoverage, ""},
		{"not steered there", stickyClient{AP: "pump", Better: better, Bands: []string{"2g"}}, "sticky", stickyHelpSteerOff, ""},
		{"has a rule", stickyClient{AP: "pump", Better: better, Steered: true, Rule: &stickyRule{Nearest: true}}, "sticky", stickyHelpRuleIdle, ""},
		{"pinned elsewhere, held here", stickyClient{AP: "pump", Better: better, Steered: true, Rule: &stickyRule{APs: []stickyAPName{{ID: "living"}}}}, "sticky", stickyHelpRuleIdle, ""},
		{"one band, one better AP", stickyClient{AP: "pump", Better: better, Steered: true, Bands: []string{"2g"}, Late: &stickyLate{AP: "living"}}, "sticky", stickyHelpPin, "living"},
		{"late moves alone", stickyClient{AP: "pump", Steered: true, Bands: []string{"2g"}, Late: &stickyLate{AP: "office", APName: "Office"}}, "sticky", stickyHelpPin, "office"},
		{"moves about", stickyClient{AP: "pump", Better: better, Steered: true, Bands: []string{"2g"}, Late: &stickyLate{AP: "office"}}, "sticky", stickyHelpNearest, ""},
		{"several bands", stickyClient{AP: "pump", Better: better, Steered: true, Bands: []string{"2g", "5g"}}, "sticky", stickyHelpNearest, ""},
	} {
		class, help, ap := stickyHelp(&c.sc)
		if class != c.class || help != c.help || (ap == nil) != (c.ap == "") || (ap != nil && ap.ID != c.ap) {
			t.Errorf("%s: %s %s %+v", c.what, class, help, ap)
		}
	}
}

// The sticky list (0340): of the APs one may view, each client and place it
// held on, the one there now first; with what steering asked of it, and its
// network's steering where it held on. The APs' own test clients are not
// listed.
func TestStickyList(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	radios := []any{map[string]any{"radio": "radio0", "band": "2g", "htmodes": []string{"HE20"}}}
	pump := hierarchy.NodeID(strings.Split(f.enrollAt("Pump", "02:00:00:00:00:71", "office", radios, map[string]int{"2g": 1}), "|")[0])
	living := hierarchy.NodeID(strings.Split(f.enrollAt("Living", "02:00:00:00:00:72", "office", radios, map[string]int{"2g": 6}), "|")[0])
	now := time.Now().Truncate(time.Minute)
	const iot, far = "14:63:93:66:a7:98", "84:0d:8e:73:9a:f7"
	var m []conditions.ClientMinute
	m = append(m, stickyMinutes(iot, living, "2g", now.Add(-5*time.Hour), repeat(-50, 30)...)...)
	m = append(m, stickyMinutes(iot, pump, "2g", now.Add(-4*time.Hour), repeat(-84, 90)...)...)
	m = append(m, stickyMinutes(far, pump, "2g", now.Add(-40*time.Minute), repeat(-92, 39)...)...)
	if err := f.api.conds.RecordMinutes(nil, m); err != nil {
		t.Fatal(err)
	}
	must(t, f.api.conds.NameClient(iot, "Patio switch", "griff"))
	code, body := f.do("GET", "/v1/sticky", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	list := body["clients"].([]any)
	if len(list) != 2 || body["weak"] != -78.0 || body["bands"].(map[string]any)["2g"] != 2.0 {
		t.Fatalf("sticky = %v", body)
	}
	there, held := list[0].(map[string]any), list[1].(map[string]any)
	if there["mac"] != far || there["now"] != true || there["class"] != "weak" || there["help"] != stickyHelpCoverage || there["better"] != nil {
		t.Errorf("there now = %v", there)
	}
	if held["mac"] != iot || held["name"] != "Patio switch" || held["ap_name"] != "Pump" || held["weak_minutes"] != 90.0 || held["class"] != "sticky" ||
		held["help"] != stickyHelpSteerOff || held["steered"] != false || held["rule"] != nil || held["better"].(map[string]any)["ap_name"] != "Living" {
		t.Errorf("held on = %v", held)
	}
	if code, body := f.do("GET", "/v1/sticky?hours=169", "griff", nil); code != 400 {
		t.Errorf("hours 169: %d %v", code, body)
	}
}

// A client pinned to an AP (0319) that the AP has heard far weaker than its
// usual over the last half hour warns (0340): it may have been moved, and be
// pinned to an AP no longer its best; where it did better lately is said.
// Not one whose signal there is as usual, nor one of the nearest.
func TestPinMoved(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	radios := []any{map[string]any{"radio": "radio0", "band": "2g", "htmodes": []string{"HE20"}}}
	pump := hierarchy.NodeID(strings.Split(f.enrollAt("Pump", "02:00:00:00:00:81", "office", radios, map[string]int{"2g": 1}), "|")[0])
	living := hierarchy.NodeID(strings.Split(f.enrollAt("Living", "02:00:00:00:00:82", "office", radios, map[string]int{"2g": 6}), "|")[0])
	const moved, steady, near = "14:63:93:66:a7:98", "14:63:93:66:a7:99", "14:63:93:66:a7:9a"
	for _, mac := range []string{moved, steady} {
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "homes." + mac + ".aps", "value": []string{string(pump)}}); code != 200 {
			t.Fatalf("pin %s: %d %v", mac, code, b)
		}
	}
	if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "homes." + near + ".nearest", "value": true}); code != 200 {
		t.Fatalf("nearest: %d %v", code, b)
	}
	must(t, f.api.conds.NameClient(moved, "Patio switch", "griff"))
	now := time.Now().Truncate(time.Minute)
	var m []conditions.ClientMinute
	for _, mac := range []string{moved, steady, near} {
		m = append(m, stickyMinutes(mac, pump, "2g", now.Add(-5*time.Hour), repeat(-55, 120)...)...)
	}
	m = append(m, stickyMinutes(moved, living, "2g", now.Add(-150*time.Minute), repeat(-50, 20)...)...)
	m = append(m, stickyMinutes(moved, pump, "2g", now.Add(-25*time.Minute), repeat(-84, 25)...)...)
	m = append(m, stickyMinutes(near, pump, "2g", now.Add(-25*time.Minute), repeat(-84, 25)...)...)
	m = append(m, stickyMinutes(steady, pump, "2g", now.Add(-25*time.Minute), repeat(-56, 25)...)...)
	must(t, f.api.conds.RecordMinutes(nil, m))
	_, view := f.do("GET", "/v1/alerts", "griff", nil)
	var found []map[string]any
	for _, a := range view["alerts"].([]any) {
		if a := a.(map[string]any); a["kind"] == "pin-moved" {
			found = append(found, a)
		}
	}
	if len(found) != 1 {
		t.Fatalf("pin-moved alerts: %v", found)
	}
	a := found[0]
	if a["ap"] != string(pump) || a["key"] != "pin-moved:"+moved || a["severity"] != "warning" || a["short"] != "Patio switch pinned, moved?" ||
		a["message"] != "Patio switch (14:63:93:66:a7:98) is pinned here, and over the last half hour it was heard on 2.4 GHz at -84 dBm, 29 dB weaker than its usual -55 dBm here: it may have been moved, and be pinned to an AP that is no longer its best. In the last day it did better on Living (2.4 GHz), at -50 dBm over 20 minutes. Its pin is in its Steering" {
		t.Errorf("pin-moved: %v", a)
	}
}

func ptr(v int) *int { return &v }
