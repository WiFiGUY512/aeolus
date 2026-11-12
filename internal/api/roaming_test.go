package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Roaming (0136): the attempts of clients that came as ones already on the
// network are read back as roams: how many, of which kind, how long they
// took to be let on, where each came from, and those that failed or were
// slow.
func TestRoaming(t *testing.T) {
	f := newFixture(t)
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "add-builtins"})
	change(map[string]any{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "house"})
	change(map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "annex", "role": "viewer"})
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	enroll := func(name, mac, folder string, radios []any, macs []string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": radios, "macs": macs}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder})
		return ap, token
	}
	const scan = "02:00:00:00:0b:f0"
	north, northToken := enroll("North", "02:00:00:00:00:3a", "office", []any{serving, spare}, []string{"02:00:00:00:00:3a", scan})
	_, southToken := enroll("South", "02:00:00:00:00:3b", "office", []any{serving}, nil)
	_, eastToken := enroll("East", "02:00:00:00:00:3c", "annex", []any{serving}, nil)

	now := time.Now()
	// One attempt, at an AP: on a radio of it, so long ago, how it went,
	// and how the client came; none, as an agent before this says nothing.
	ssid := "Home Wi-Fi"
	attempt := func(token, mac, bss, band string, ago time.Duration, outcome, stage, reason string, how map[string]any) {
		t.Helper()
		rec := map[string]any{"mac": mac, "bss": bss, "network": "sweet", "ssid": ssid, "band": band,
			"started": now.Add(-ago).UnixMilli(), "took_ms": 300, "outcome": outcome, "stage": stage, "reason": reason, "events": []any{}}
		if how != nil {
			rec["how"] = how
		}
		if code, _, body := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{rec}}, nil); code != 200 || body["recorded"] != float64(1) || body["refused"] != float64(0) {
			t.Fatalf("an attempt of %s: %d %v", mac, code, body)
		}
	}
	client := func(n int) string { return fmt.Sprintf("72:bb:66:9f:76:%02x", n) }
	phone, laptop, tablet, watch, gadget, oldie, mixed, odd, relet := client(1), client(2), client(3), client(4), client(5), client(6), client(7), client(8), client(9)
	// A phone joins North afresh; goes to South and back by fast
	// transitions; to South once more with the four messages, slowly;
	// fails to get back on North; gets back on it; and asks there again as
	// one already on, on the radio it is on: that is no roam.
	attempt(northToken, phone, "phy0-ap0", "5g", 600*time.Second, "online", "internet", "", map[string]any{"alg": "open", "on_ms": 40})
	attempt(southToken, phone, "phy0-ap0", "5g", 500*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 17})
	attempt(northToken, phone, "phy0-ap0", "5g", 400*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 21})
	attempt(southToken, phone, "phy0-ap0", "5g", 300*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 280})
	attempt(northToken, phone, "phy0-ap0", "5g", 200*time.Second, "failed", "key", "no answer the AP could use to the first message of the key handshake, sent 4 times", map[string]any{"re": true, "alg": "open"})
	attempt(northToken, phone, "phy0-ap0", "5g", 100*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 35})
	attempt(northToken, phone, "phy0-ap0", "5g", 50*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 30})
	// A laptop comes back to North, which still held it: a second of SA
	// Queries, and its key from before. Then to South's 2.4 GHz radio with
	// a key made from its sign-in, and from that to South's other radio.
	attempt(northToken, laptop, "phy1-ap0", "2g", 90*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "pmk": "pmksa", "saq": 5, "on_ms": 1198})
	attempt(southToken, laptop, "phy1-ap0", "2g", 80*time.Second, "online", "dns", "", map[string]any{"re": true, "alg": "open", "pmk": "okc", "on_ms": 30})
	attempt(southToken, laptop, "phy0-ap0", "5g", 75*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 45})
	// A tablet asks South as one already on, and goes before it is let on.
	attempt(southToken, tablet, "phy0-ap0", "5g", 70*time.Second, "left", "assoc", "before it was let on: deauthenticated", map[string]any{"re": true})
	// A watch comes to South by a fast transition of which South's log
	// has the association and no more: let on, and not timed.
	attempt(southToken, watch, "phy0-ap0", "5g", 65*time.Second, "connected", "key", "", map[string]any{"alg": "ft", "on_ms": 1, "began": "assoc"})
	// A gadget goes from North to East, in another folder.
	attempt(northToken, gadget, "phy0-ap0", "5g", 260*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(eastToken, gadget, "phy0-ap0", "5g", 250*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 19})
	// One that joined North a day and more ago, and has asked there since
	// as one already on, on the radio it is on: no roam, however long ago
	// it came.
	attempt(northToken, oldie, "phy0-ap0", "5g", 30*time.Hour, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(northToken, oldie, "phy0-ap0", "5g", 2*time.Hour, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 20})
	attempt(northToken, oldie, "phy0-ap0", "5g", 1*time.Hour, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 20})
	// One that was on South in between, of which South's agent, an older
	// one, said only that it came online.
	attempt(northToken, mixed, "phy0-ap0", "5g", 900*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(southToken, mixed, "phy0-ap0", "5g", 850*time.Second, "online", "dns", "", nil)
	attempt(northToken, mixed, "phy0-ap0", "5g", 800*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "open", "on_ms": 33})
	// One comes to South as one already on, and is let on, by a record
	// that has no time for it, as an agent of the first release with
	// roaming wrote of a client that authenticated again once it was on:
	// it went well, and is not timed.
	attempt(northToken, relet, "phy0-ap0", "5g", 950*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(southToken, relet, "phy0-ap0", "5g", 940*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "sae"})
	// A client that says nothing of roaming, as many a phone: it joins
	// North, and a minute later South, as if for the first time, by WPA3's
	// exchange. It was on North moments before: a roam, and a hard one.
	hard, band, late, sitter, guest, failer, prober, looker := client(10), client(11), client(12), client(13), client(14), client(15), client(16), client(17)
	attempt(northToken, hard, "phy0-ap0", "5g", 700*time.Second, "online", "dns", "", map[string]any{"alg": "sae", "on_ms": 150})
	attempt(southToken, hard, "phy0-ap0", "5g", 640*time.Second, "online", "dns", "", map[string]any{"alg": "sae", "on_ms": 160})
	// One that goes from North's 5 GHz radio to its other, so.
	attempt(northToken, band, "phy0-ap0", "5g", 700*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(northToken, band, "phy1-ap0", "2g", 690*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 25})
	// One that joined North three hours ago, of which North has said
	// nothing since, and joins South: it comes back from somewhere, and
	// that is no roam.
	attempt(northToken, late, "phy0-ap0", "5g", 3*time.Hour, "online", "dns", "", map[string]any{"alg": "sae", "on_ms": 150})
	attempt(southToken, late, "phy0-ap0", "5g", 620*time.Second, "online", "dns", "", map[string]any{"alg": "sae", "on_ms": 140})
	// One that joined North three hours ago and is there yet, as North's
	// report of what it moved says: its join at South a moment after is a
	// roam.
	attempt(northToken, sitter, "phy0-ap0", "5g", 3*time.Hour, "online", "dns", "", map[string]any{"alg": "sae", "on_ms": 150})
	_, _, polled := f.apDo("GET", "/v1/ap/config", northToken, nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", northToken, map[string]any{"version": polled["version"], "uptime": 60, "clients": []any{
		map[string]any{"mac": sitter, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -60, "signal_avg": -60,
			"tx_rate": 400, "rx_rate": 300, "connected": 60, "inactive_ms": 10, "rx_bytes": 900, "tx_bytes": 1200, "rx_packets": 9, "tx_packets": 12}}}, nil); code != 200 {
		t.Fatalf("North's report: %d %v", code, b)
	}
	attempt(southToken, sitter, "phy0-ap0", "5g", -2*time.Second, "online", "dns", "", map[string]any{"alg": "sae", "on_ms": 130})
	// One that leaves a network on North for another on South: no roam.
	attempt(northToken, guest, "phy0-ap0", "5g", 700*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	ssid = "Guests"
	attempt(southToken, guest, "phy0-ap1", "5g", 650*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	ssid = "Home Wi-Fi"
	// One on North that tries South, and is not let on: a roam that
	// failed.
	attempt(northToken, failer, "phy0-ap0", "5g", 700*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(southToken, failer, "phy0-ap0", "5g", 660*time.Second, "failed", "key", "wrong passphrase", map[string]any{"alg": "open"})
	// One on North that authenticates at South and goes no further, with
	// nothing refused: given up, and no failure.
	attempt(northToken, prober, "phy0-ap0", "5g", 700*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(southToken, prober, "phy0-ap0", "5g", 655*time.Second, "failed", "assoc", "it was never let on", map[string]any{"alg": "open"})
	// One on North's 5 GHz radio that authenticates at its other and goes
	// no further: a band switch given up, which is nothing, and nowhere.
	attempt(northToken, looker, "phy0-ap0", "5g", 700*time.Second, "online", "dns", "", map[string]any{"alg": "open", "on_ms": 30})
	attempt(northToken, looker, "phy1-ap0", "2g", 645*time.Second, "failed", "assoc", "it was never let on", map[string]any{"alg": "open"})
	// North's own scan radio, testing: no client's roaming.
	attempt(southToken, scan, "phy0-ap0", "5g", 60*time.Second, "connected", "key", "", map[string]any{"re": true, "alg": "ft", "on_ms": 15})
	// How it came costs no record. What this manager cannot take of it is
	// left out and the attempt kept; a word of a later agent's is passed
	// over, and the rest taken.
	for i, how := range []map[string]any{{"re": true, "pmk": "magic"}, {"re": true, "alg": "Open System"}, {"re": true, "on_ms": 700000}, {"re": true, "saq": 65}, {"re": true, "alg": 7},
		{"alg": "open", "on_ms": 9, "hops": 3}} {
		attempt(southToken, odd, "phy0-ap0", "5g", time.Duration(40-i)*time.Second, "connected", "key", "", how)
	}
	_, kept := f.do("GET", "/v1/clients/"+odd+"/connections", "griff", nil)
	var hows []string
	for _, c := range kept["connections"].([]any) {
		c := c.(map[string]any)
		hows = append(hows, fmt.Sprintf("%v roam %v", c["record"].(map[string]any)["how"], c["roam"]))
	}
	if got, want := strings.Join(hows, "; "), "map[alg:open on_ms:9] roam <nil>; <nil> roam <nil>; <nil> roam <nil>; <nil> roam <nil>; <nil> roam <nil>; <nil> roam <nil>"; got != want {
		t.Errorf("how it came, where it cannot all be taken:\n got %s\nwant %s", got, want)
	}

	code, v := f.do("GET", "/v1/roaming", "griff", nil)
	if code != 200 {
		t.Fatalf("roaming: %d %v", code, v)
	}
	if got, want := fmt.Sprintf("%v roams, %v failed, %v slow, %v left, %v not timed, %v clients, typically %v ms, slow past %v, more %v", v["roams"], v["failed"], v["slow"], v["left"], v["untimed"], v["clients"], v["typical_ms"], v["slow_over_ms"], v["more"]),
		"14 roams, 2 failed, 2 slow, 2 left, 2 not timed, 11 clients, typically 35 ms, slow past 250, more <nil>"; got != want {
		t.Errorf("in all: %s, want %s", got, want)
	}
	// The clients that came most in the time, more than once: the phone
	// first, at both APs, with its one failure; the oldie's attempt of
	// thirty hours ago not counted; North's scan radio not at all.
	var came []string
	for _, j := range v["joins"].([]any) {
		j := j.(map[string]any)
		came = append(came, fmt.Sprintf("%s:%v/%v", j["mac"], j["attempts"], j["failed"]))
	}
	if got, want := strings.Join(came, " "), strings.Join([]string{phone + ":7/1", odd + ":6/0", laptop + ":3/0", mixed + ":3/0", gadget + ":2/0", oldie + ":2/0", relet + ":2/0", hard + ":2/0", band + ":2/0", late + ":2/0"}, " "); got != want {
		t.Errorf("joins:\n got %s\nwant %s", got, want)
	}
	if aps := v["joins"].([]any)[0].(map[string]any)["aps"]; fmt.Sprint(aps) != "[North South]" {
		t.Errorf("the phone came to %v", aps)
	}
	// From one radio of an AP to another is a band switch, and no roam:
	// the laptop's at South, and the one at North.
	if v["band_switches"] != float64(2) {
		t.Errorf("band switches: %v, want 2", v["band_switches"])
	}
	var kinds []string
	for _, k := range v["kinds"].([]any) {
		k := k.(map[string]any)
		kinds = append(kinds, fmt.Sprintf("%v: %v, %v failed, %v slow, %v ms (%v to %v)", k["kind"], k["roams"], k["failed"], k["slow"], k["typical_ms"], k["low_ms"], k["high_ms"]))
	}
	if got, want := strings.Join(kinds, "; "), "ft: 4, 0 failed, 0 slow, 19 ms (17 to 21); okc: 1, 0 failed, 0 slow, 30 ms (30 to 30); pmksa: 1, 0 failed, 1 slow, 1198 ms (1198 to 1198); keys: 5, 2 failed, 1 slow, 35 ms (33 to 280); full: 3, 0 failed, 0 slow, 160 ms (130 to 160)"; got != want {
		t.Errorf("a kind at a time:\n got %s\nwant %s", got, want)
	}
	said := func(list any) []string {
		var out []string
		for _, r := range list.([]any) {
			r := r.(map[string]any)
			from := "somewhere not known"
			if r["from_ap_name"] != nil {
				from = fmt.Sprintf("%v %v", r["from_ap_name"], r["from_band"])
			}
			s := fmt.Sprintf("%v to %v %v from %v, %v, %v", r["mac"].(string)[15:], r["ap_name"], r["band"], from, r["kind"], r["state"])
			if r["ms"] != nil {
				s += fmt.Sprintf(" in %v ms", r["ms"])
			}
			if r["untimed"] == true {
				s += ", not timed"
			}
			if r["band_switch"] == true {
				s += ", a band switch"
			}
			if r["sa_queries"] != nil {
				s += fmt.Sprintf(" after %v SA Queries", r["sa_queries"])
			}
			out = append(out, s)
		}
		return out
	}
	// The latest first. The phone's last ask, on the radio it was on, is
	// not among them; nor the scan radio's; nor any of the one that joined
	// North long ago and stayed.
	if got, want := strings.Join(said(v["latest"]), "\n"), strings.Join([]string{
		"0d to South 5g from North 5g, full, ok in 130 ms",
		"04 to South 5g from somewhere not known, ft, ok, not timed",
		"03 to South 5g from somewhere not known, keys, left",
		"02 to South 5g from South 2g, keys, ok in 45 ms, a band switch",
		"02 to South 2g from North 2g, okc, ok in 30 ms",
		"02 to North 2g from somewhere not known, pmksa, slow in 1198 ms after 5 SA Queries",
		"01 to North 5g from South 5g, keys, ok in 35 ms",
		"01 to North 5g from South 5g, keys, failed",
		"05 to East 5g from North 5g, ft, ok in 19 ms",
		"01 to South 5g from North 5g, keys, slow in 280 ms",
		"01 to North 5g from South 5g, ft, ok in 21 ms",
		"01 to South 5g from North 5g, ft, ok in 17 ms",
		"0a to South 5g from North 5g, full, ok in 160 ms",
		"10 to South 5g from North 5g, keys, left",
		"0f to South 5g from North 5g, keys, failed",
		"0b to North 2g from North 5g, keys, ok in 25 ms, a band switch",
		"07 to North 5g from South 5g, keys, ok in 33 ms",
		"09 to South 5g from North 5g, full, ok, not timed",
	}, "\n"); got != want {
		t.Errorf("the latest:\n%s\nwant:\n%s", got, want)
	}
	if got, want := strings.Join(said(v["bad"]), "\n"), strings.Join([]string{
		"03 to South 5g from somewhere not known, keys, left",
		"02 to North 2g from somewhere not known, pmksa, slow in 1198 ms after 5 SA Queries",
		"01 to North 5g from South 5g, keys, failed",
		"01 to South 5g from North 5g, keys, slow in 280 ms",
		"10 to South 5g from North 5g, keys, left",
		"0f to South 5g from North 5g, keys, failed",
	}, "\n"); got != want {
		t.Errorf("those that went badly:\n%s\nwant:\n%s", got, want)
	}
	if bad := v["bad"].([]any)[2].(map[string]any); !strings.Contains(fmt.Sprint(bad["reason"]), "first message of the key handshake") {
		t.Errorf("the roam that failed says why: %v", bad)
	}
	// A client's own attempts say which were roams, and not the one on
	// the radio it was on.
	_, own := f.do("GET", "/v1/clients/"+phone+"/connections", "griff", nil)
	var marks, froms []string
	for _, c := range own["connections"].([]any) {
		if m, _ := c.(map[string]any)["roam"].(map[string]any); m != nil {
			marks = append(marks, fmt.Sprintf("%v %v", m["kind"], m["ms"]))
			froms = append(froms, fmt.Sprintf("%v %v", m["from_ap_name"], m["from_band"]))
		} else {
			marks = append(marks, "-")
		}
	}
	if got, want := strings.Join(marks, ", "), "-, keys 35, keys <nil>, keys 280, ft 21, ft 17, -"; got != want {
		t.Errorf("the phone's attempts, the latest first = %s, want %s", got, want)
	}
	// And where each came from (0222), as the roams have it: the latest, to
	// North 5g, from South 5g.
	if len(froms) == 0 || froms[0] != "South 5g" {
		t.Errorf("the phone's roams came from %q, the latest want South 5g", froms)
	}
	// A test's own client's attempts are marked as no roams.
	_, tests := f.do("GET", "/v1/clients/"+scan+"/connections", "griff", nil)
	for _, c := range tests["connections"].([]any) {
		if c.(map[string]any)["roam"] != nil {
			t.Errorf("a test's own client, marked as roaming: %v", c)
		}
	}
	// Below one AP, its own alone, with where each came from all the same.
	_, one := f.do("GET", "/v1/roaming?under="+north+"&hours=1", "griff", nil)
	if one["roams"] != float64(5) || one["band_switches"] != float64(1) || one["hours"] != float64(1) {
		t.Errorf("below North: %v roams and %v band switches over %v hours, want 5 and 1 over 1", one["roams"], one["band_switches"], one["hours"])
	}
	if got := said(one["latest"]); len(got) != 6 || got[1] != "01 to North 5g from South 5g, keys, ok in 35 ms" {
		t.Errorf("below North, the latest: %q", got)
	}
	// One who may view East alone sees the roam to it, and not from where:
	// North is not theirs to see. One with no role on any AP sees none.
	_, part := f.do("GET", "/v1/roaming", "tenant", nil)
	if got, want := strings.Join(said(part["latest"]), "; "), "05 to East 5g from somewhere not known, ft, ok in 19 ms"; got != want || part["roams"] != float64(1) {
		t.Errorf("one who may view East alone: %v roams, %s", part["roams"], got)
	}
	// Nor the joins at North: the gadget came to East once, which is not
	// more than once.
	if j := part["joins"].([]any); len(j) != 0 {
		t.Errorf("one who may view East alone, the joins: %v", j)
	}
	if _, none := f.do("GET", "/v1/roaming", "office", nil); none["roams"] != float64(0) || len(none["latest"].([]any)) != 0 {
		t.Errorf("one with no role on the APs: %v", none)
	}
	if code, b := f.do("GET", "/v1/roaming?hours=0", "griff", nil); code != 400 {
		t.Errorf("no hours: %d %v", code, b)
	}
	// So many attempts are read, of the APs asked about and no others: a
	// folder with few is whole, whatever the next one has.
	most := roamingMost
	roamingMost = 2
	defer func() { roamingMost = most }()
	if _, few := f.do("GET", "/v1/roaming?under=annex", "griff", nil); few["roams"] != float64(1) || few["more"] != nil {
		t.Errorf("below Annex, with two attempts read at the most: %v roams, more %v", few["roams"], few["more"])
	}
	if _, many := f.do("GET", "/v1/roaming?under=office", "griff", nil); many["more"] != true {
		t.Errorf("below Office, with two attempts read at the most: more %v", many["more"])
	}
}

// The kind of a roam, from how the client came.
func TestRoamKind(t *testing.T) {
	signin := []connStep{{Stage: "signin", What: "signed in with PEAP"}}
	for what, c := range map[string]struct {
		how   *connHow
		steps []connStep
		signs bool
		kind  string
		said  bool
		known bool
	}{
		"nothing said of how it came":                    {nil, nil, false, "", false, false},
		"a join with a passphrase, with no word of it":   {&connHow{Alg: "open"}, nil, false, roamKeys, false, true},
		"WPA3's exchange, with no word of it":            {&connHow{Alg: "sae"}, nil, false, roamFull, false, true},
		"a key this AP held, with no word of it":         {&connHow{Alg: "open", PMK: "pmksa"}, nil, false, roamPMKSA, false, true},
		"a fast transition":                              {&connHow{Re: true, Alg: "ft"}, nil, false, roamFT, true, true},
		"a fast transition, with no debug lines":         {&connHow{Alg: "ft"}, nil, true, roamFT, true, true},
		"a key made from a sign-in elsewhere":            {&connHow{Re: true, Alg: "open", PMK: "okc"}, nil, false, roamOKC, true, true},
		"a key this AP held":                             {&connHow{Re: true, Alg: "open", PMK: "pmksa"}, nil, false, roamPMKSA, true, true},
		"a passphrase":                                   {&connHow{Re: true, Alg: "open"}, nil, false, roamKeys, true, true},
		"WPA3's exchange again":                          {&connHow{Re: true, Alg: "sae"}, nil, false, roamFull, true, true},
		"a sign-in again":                                {&connHow{Re: true, Alg: "open"}, signin, true, roamFull, true, true},
		"no sign-in, on a network that signs clients in": {&connHow{Re: true, Alg: "open"}, nil, true, roamCached, true, true},
		"a sign-in, with no word of it":                  {&connHow{Alg: "open"}, signin, true, roamFull, false, true},
		"how it came that cannot be taken":               {&connHow{Re: true, PMK: "magic"}, nil, false, "", false, false},
	} {
		kind, said, known := roamKind(roamRecord{How: c.how, Events: c.steps}, func() bool { return c.signs })
		if kind != c.kind || said != c.said || known != c.known {
			t.Errorf("%s: %q, said %v, known %v; want %q, %v, %v", what, kind, said, known, c.kind, c.said, c.known)
		}
	}
}

// Each AP's wait between a client's last key message and its being let on,
// a band at a time (0231): the radio storing the client's key, 1 or 2 ms
// where nothing holds it up, 30-60 where mac80211 did (B94). Only a join
// with the key messages counts: not one by 802.11r, nor one whose AP keeps
// no log of them.
func TestRoamingKeyWaits(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	radios := []any{map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20"}}, map[string]any{"radio": "radio1", "band": "2g", "htmodes": []string{"HE20"}}}
	slow := strings.Split(f.enrollAt("Slow", "02:00:00:00:00:5a", "office", radios, map[string]int{"5g": 36}), "|")
	quick := strings.Split(f.enrollAt("Quick", "02:00:00:00:00:5b", "office", radios, map[string]int{"5g": 44}), "|")
	now := time.Now()
	n := 0
	join := func(token, band string, m4, on int, how map[string]any) {
		t.Helper()
		n++
		events := []any{map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": true}}
		if m4 >= 0 {
			events = append(events, map[string]any{"t": m4 - 3, "stage": "key", "what": "message 3 of 4 sent to it", "k": "m3"},
				map[string]any{"t": m4, "stage": "key", "what": "message 4 of 4 came from it", "k": "m4"})
		}
		if how == nil {
			how = map[string]any{"alg": "open"}
		}
		how["on_ms"] = on
		rec := map[string]any{"mac": fmt.Sprintf("72:bb:66:12:2b:%02x", n), "bss": "phy0-ap0", "network": "sweet", "ssid": "Home Wi-Fi", "band": band,
			"started": now.Add(-time.Duration(n) * time.Minute).UnixMilli(), "took_ms": on, "outcome": "connected", "stage": "key", "reason": "", "events": events, "how": how}
		if code, _, b := f.apDo("POST", "/v1/ap/connections", token, map[string]any{"connections": []any{rec}}, nil); code != 200 || b["recorded"] != 1.0 {
			t.Fatalf("a join: %d %v", code, b)
		}
	}
	join(slow[1], "2g", 20, 61, nil)
	join(slow[1], "2g", 18, 52, nil)
	join(slow[1], "2g", 22, 94, nil)
	join(slow[1], "5g", 15, 17, nil)
	join(quick[1], "2g", 30, 31, nil)
	join(quick[1], "2g", 25, 27, nil)
	// None of these is a wait: 802.11r, no key messages in the log, and a
	// wait past keyWaitMost.
	join(quick[1], "2g", -1, 12, map[string]any{"re": true, "alg": "ft"})
	join(quick[1], "5g", -1, 40, nil)
	join(quick[1], "5g", 10, 9000, nil)

	_, view := f.do("GET", "/v1/roaming", "griff", nil)
	got, _ := json.Marshal(view["key_waits"])
	type w struct {
		AP    string `json:"ap_name"`
		Band  string `json:"band"`
		Joins int    `json:"joins"`
		MS    *int   `json:"typical_ms"`
		High  *int   `json:"high_ms"`
	}
	var waits []w
	json.Unmarshal(got, &waits)
	want := []w{{"Quick", "2g", 2, nil, nil}, {"Slow", "2g", 3, nil, nil}, {"Slow", "5g", 1, nil, nil}}
	if len(waits) != len(want) {
		t.Fatalf("key_waits = %s", got)
	}
	ms := [][2]int{{2, 2}, {41, 72}, {2, 2}} // middle takes the upper of two
	for i := range want {
		if waits[i].AP != want[i].AP || waits[i].Band != want[i].Band || waits[i].Joins != want[i].Joins ||
			waits[i].MS == nil || *waits[i].MS != ms[i][0] || waits[i].High == nil || *waits[i].High != ms[i][1] {
			t.Errorf("key_waits[%d] = %s", i, got)
		}
	}
}
