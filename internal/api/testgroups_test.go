package api

import (
	"strings"
	"testing"
	"time"
)

// Blocks of time (0188): each on some days or every day, past midnight
// into the next, in the AP's zone.
func TestBlockOpen(t *testing.T) {
	chicago, _ := time.LoadLocation("America/Chicago")
	at := func(day, hhmm string) time.Time {
		d := map[string]int{"mon": 19, "tue": 20, "wed": 21, "thu": 22, "fri": 23, "sat": 24, "sun": 25}[day]
		h, m := int(hhmm[0]-'0')*10+int(hhmm[1]-'0'), int(hhmm[3]-'0')*10+int(hhmm[4]-'0')
		return time.Date(2026, 10, d, h, m, 0, 0, chicago)
	}
	for _, c := range []struct {
		block, day, at string
		want           bool
	}{
		{"08:00-17:00", "sun", "09:00", true},
		{"08:00-17:00", "sun", "17:00", false},
		{"mon-fri 08:00-17:00", "wed", "12:00", true},
		{"mon-fri 08:00-17:00", "sat", "12:00", false},
		{"sat,sun 10:00-12:00", "sun", "11:59", true},
		{"sat,sun 10:00-12:00", "mon", "11:00", false},
		{"mon,wed,fri 01:00-02:00", "wed", "01:30", true},
		{"mon,wed,fri 01:00-02:00", "thu", "01:30", false},
		// Past midnight: Friday's block runs into Saturday morning, and not
		// into Friday's own morning.
		{"fri 22:00-02:00", "fri", "23:00", true},
		{"fri 22:00-02:00", "sat", "01:00", true},
		{"fri 22:00-02:00", "fri", "01:00", false},
		{"fri 22:00-02:00", "sat", "23:00", false},
		// A range past Saturday.
		{"fri-mon 09:00-10:00", "sun", "09:30", true},
		{"fri-mon 09:00-10:00", "tue", "09:30", false},
		// The whole day, on its days.
		{"tue 00:00-00:00", "tue", "18:00", true},
		{"tue 00:00-00:00", "wed", "18:00", false},
		// One that does not read is never open.
		{"mon 25:00-26:00", "mon", "12:00", false},
	} {
		if got := blockOpen(c.block, "America/Chicago", at(c.day, c.at)); got != c.want {
			t.Errorf("%q on %s at %s: %v", c.block, c.day, c.at, got)
		}
	}
}

// The schedule's blocks take the window's place, and a run asked for a
// while ends when it says (0188).
func TestScheduleBlocksAndUntil(t *testing.T) {
	now := time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC) // a Wednesday
	c := controls{window: "01:00-02:00"}
	if c.open(now) {
		t.Fatal("the window alone, out of it")
	}
	c.windows = []string{"mon-fri 14:00-16:00"}
	if !c.open(now) {
		t.Fatal("a block, in it, though out of the window")
	}
	c.windows = []string{"sat 14:00-16:00", "sun 14:00-16:00"}
	if c.open(now) {
		t.Fatal("blocks on other days")
	}
	c = controls{until: now.Add(time.Hour).Unix()}
	if !c.open(now) {
		t.Fatal("a run of an hour, within it")
	}
	if c.open(now.Add(time.Hour)) {
		t.Fatal("a run of an hour, an hour on")
	}
}

// A round narrowed to a troublesome few (0188): to the things joined on one
// AP, or on one band, or of one network; refused where none is left.
func TestAssuranceRoundNarrowed(t *testing.T) {
	f := newFixture(t)
	change := func(body map[string]any) {
		t.Helper()
		if code, b := f.change("griff", body); code != 200 {
			t.Fatalf("%v: %d %v", body["kind"], code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:8a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	plain := strings.Split(f.enrollAt("Plain", "02:00:00:00:00:8b", "office", []any{serving}, map[string]int{"5g": 44}), "|")
	for _, tok := range []string{plain[1], scanner[1]} {
		_, _, cfg := f.apDo("GET", "/v1/ap/config", tok, nil, nil)
		body := map[string]any{"version": cfg["version"], "uptime": 60}
		if tok == scanner[1] {
			body["bssids"] = []string{"02:00:00:00:00:8a"}
			body["rrm"] = map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"}, "ratings": []any{}, "moves": []any{},
				"neighbours": []any{map[string]any{"ap": plain[0], "address": "192.0.2.9", "state": "up", "chosen": true, "hello_ago": 3,
					"bands": []any{map[string]any{"band": "5g", "signal": -50, "their_signal": -50, "channel": 44, "width": 40}}}}}
		}
		if code, _, b := f.apDo("POST", "/v1/ap/state", tok, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	round := func(body map[string]any) (int, map[string]any) {
		body["under"] = "office"
		return f.do("POST", "/v1/assurance/round", "griff", body)
	}
	// All of it: on itself and on Plain.
	if code, b := round(map[string]any{}); code != 200 || b["things"] != 2.0 {
		t.Fatalf("a round of everything: %d %v", code, b)
	}
	// Only what joins on the scanner itself, which is asked; and the round
	// says what it is narrowed to.
	if code, b := round(map[string]any{"on": []string{scanner[0]}}); code != 200 || b["things"] != 1.0 {
		t.Fatalf("a round on one AP: %d %v", code, b)
	}
	got := f.api.assure(f.now())
	if len(got) != 1 || got[0].AP != scanner[0] || !strings.Contains(string(got[0].Params), scanner[0]) {
		t.Fatalf("asked %v", got)
	}
	_, view := f.do("GET", "/v1/assurance?under=office", "griff", nil)
	r, _ := view["round"].(map[string]any)
	if r["of"] != 1.0 || r["filter"].(map[string]any)["on"].([]any)[0] != scanner[0] {
		t.Fatalf("the round as seen: %v", r)
	}
	// Only on 5 GHz: both; only on 2.4 GHz, which no AP here is on: none.
	if code, b := round(map[string]any{"bands": []string{"5g"}}); code != 200 || b["things"] != 2.0 {
		t.Fatalf("a round on 5 GHz: %d %v", code, b)
	}
	if code, b := round(map[string]any{"bands": []string{"2g"}}); code != 400 || !strings.Contains(b["error"].(string), "can test those") {
		t.Fatalf("a round on 2.4 GHz: %d %v", code, b)
	}
	// A network none offers, a band that is none, and an AP that is not.
	if code, b := round(map[string]any{"networks": []string{"no-such-network"}}); code != 400 {
		t.Fatalf("a network none offers: %d %v", code, b)
	}
	if code, b := round(map[string]any{"bands": []string{"60g"}}); code != 400 {
		t.Fatalf("a band that is none: %d %v", code, b)
	}
	if code, b := round(map[string]any{"on": []string{"office"}}); code != 400 {
		t.Fatalf("a folder as an AP: %d %v", code, b)
	}
	// Only the things named, as what failed is asked again (0129): the
	// network joined on Plain on 5 GHz, and not the same on the scanner.
	only := func(band string) []any {
		return []any{map[string]any{"network": "sweet", "on": plain[0], "band": band}}
	}
	if code, b := round(map[string]any{"only": only("5g")}); code != 200 || b["things"] != 1.0 {
		t.Fatalf("only one thing: %d %v", code, b)
	}
	if code, b := round(map[string]any{"only": only("2g")}); code != 400 || !strings.Contains(b["error"].(string), "can test those") {
		t.Fatalf("only a thing no AP tests: %d %v", code, b)
	}
	if code, b := round(map[string]any{"only": only("60g")}); code != 400 || !strings.Contains(b["error"].(string), "only:") {
		t.Fatalf("only a thing on a band that is none: %d %v", code, b)
	}
}

// The schema takes blocks as days and times, and refuses what is not
// (0188); a run's end is whole seconds.
func TestScheduleBlocksSchema(t *testing.T) {
	f := newFixture(t)
	set := func(path string, v any) int {
		code, _ := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": path, "value": v})
		return code
	}
	if c := set("assurance.tests.windows", []string{"mon-fri 08:00-17:00", "sat,sun 10:00-12:00", "02:00-05:00", "fri-mon 22:00-02:00"}); c != 200 {
		t.Fatalf("good blocks: %d", c)
	}
	for _, bad := range []string{"monday 08:00-17:00", "mon-fri 8:00-17:00", "mon 08:00-24:00", "mon,  08:00-17:00", "08:00"} {
		if c := set("assurance.tests.windows", []string{bad}); c != 400 {
			t.Errorf("%q: %d", bad, c)
		}
	}
	if c := set("assurance.tests.until", 1792600000); c != 200 {
		t.Fatalf("a run's end: %d", c)
	}
	if c := set("assurance.tests.until", -1); c != 400 {
		t.Fatalf("a run's end before the epoch: %d", c)
	}
}
