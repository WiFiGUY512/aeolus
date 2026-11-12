package api

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// Steering's history (0139): the askings an AP says in its state reports
// are kept by the manager, each once however often said, and read back as
// what steering came to over a time: for each network and way, and each AP
// of it, how many were asked and what came of them; the clients asked
// most; and the latest askings. Of the APs the caller may view, and not of
// the tests' own clients.
func TestSteering(t *testing.T) {
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
	east, eastToken := enroll("East", "02:00:00:00:00:3c", "annex", []any{serving}, nil)

	now := time.Now()
	asking := func(mac, network, method, asked, outcome string, ago time.Duration, more map[string]any) map[string]any {
		e := map[string]any{"at": now.Add(-ago).Unix(), "mac": mac, "network": network, "method": method, "from": "2g", "asked": asked, "outcome": outcome, "took": 2.5}
		for k, v := range more {
			e[k] = v
		}
		return e
	}
	report := func(token string, uptime int, clients []any, events ...any) {
		t.Helper()
		code, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code != 200 {
			t.Fatalf("its config: %d %v", code, cfg)
		}
		body := map[string]any{"version": cfg["version"], "uptime": uptime, "clients": clients, "steer": map[string]any{"running": true, "since": now.Add(-time.Hour).Unix(), "events": events}}
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, body, nil); code != 200 {
			t.Fatalf("its report: %d %v", code, b)
		}
	}
	const a, b, c = "02:00:00:00:00:0a", "02:00:00:00:00:0b", "02:00:00:00:00:0c"
	hour := 3600
	northSaid := []any{
		asking(a, "sweet", "suggest", "btm", "moved", 50*time.Minute, map[string]any{"to": []string{"6g", "5g"}, "answer": "accepted", "code": 0, "signal": -48}),
		asking(a, "sweet", "suggest", "none", "back", 49*time.Minute, nil),
		asking(b, "sweet", "suggest", "btm", "refused", 40*time.Minute, map[string]any{"answer": "refused", "code": 1}),
		asking(b, "sweet", "hold", "held", "joined", 30*time.Minute, nil),
		asking(c, "guest", "sticky", "kick", "left", 20*time.Minute, map[string]any{"from": "5g", "signal": -80}),
		// The scan radio's own, joining for a test: of no client.
		asking(scan, "sweet", "suggest", "btm", "moved", 10*time.Minute, nil),
		// Said to be of before the AP started, a day up: its clock was not
		// yet set, and the asking is of this run. Taken to be of now.
		asking(c, "sweet", "suggest", "btm", "stayed", 40*24*time.Hour, nil),
	}
	client := func(mac, host string) map[string]any {
		return map[string]any{"mac": mac, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "host": host}
	}
	report(northToken, 24*hour, []any{client(a, "phone")}, northSaid...)
	// Said again in the next report, with one more: only that one is new.
	report(northToken, 24*hour+60, nil, append(northSaid, asking(b, "sweet", "suggest", "btm", "moved", time.Minute, nil))...)
	report(eastToken, 60*24*hour, nil,
		asking(a, "sweet", "suggest", "btm", "moved", 5*time.Minute, nil),
		// First heard of more than a week after: one already kept, or let
		// go since. Not kept anew. The AP has been up long enough to have
		// made it.
		asking(a, "sweet", "suggest", "btm", "moved", 8*24*time.Hour, nil),
		// Two days old, and its AP up since before: kept with its time.
		asking(b, "sweet", "suggest", "btm", "refused", 48*time.Hour, nil))

	counts := func(v any) string {
		var out []string
		for method, c := range v.(map[string]any) {
			for what, n := range c.(map[string]any) {
				out = append(out, fmt.Sprintf("%s %s %v", method, what, n))
			}
		}
		slices.Sort(out)
		return strings.Join(out, ", ")
	}
	get := func(as, query string) map[string]any {
		t.Helper()
		code, v := f.do("GET", "/v1/steering"+query, as, nil)
		if code != 200 {
			t.Fatalf("steering%s as %s: %d %v", query, as, code, v)
		}
		return v
	}
	v := get("griff", "")
	if v["hours"] != float64(24) {
		t.Errorf("hours: %v", v["hours"])
	}
	nets := v["networks"].([]any)
	if len(nets) != 2 {
		t.Fatalf("networks: %v", nets)
	}
	guest, sweet := nets[0].(map[string]any), nets[1].(map[string]any)
	if guest["network"] != "guest" || counts(guest["counts"]) != "sticky asked 1, sticky left 1" {
		t.Errorf("guest: %v", guest)
	}
	if got, want := counts(sweet["counts"]), "hold asked 1, hold joined 1, suggest asked 5, suggest back 1, suggest moved 3, suggest refused 1, suggest stayed 1"; sweet["network"] != "sweet" || got != want {
		t.Errorf("sweet:\n got %s\nwant %s", got, want)
	}
	aps := sweet["aps"].([]any)
	if len(aps) != 2 {
		t.Fatalf("sweet's APs: %v", aps)
	}
	if e := aps[0].(map[string]any); e["ap"] != east || e["name"] != "East" || counts(e["counts"]) != "suggest asked 1, suggest moved 1" {
		t.Errorf("East: %v", e)
	}
	if n := aps[1].(map[string]any); n["ap"] != north || n["name"] != "North" || counts(n["counts"]) != "hold asked 1, hold joined 1, suggest asked 4, suggest back 1, suggest moved 2, suggest refused 1, suggest stayed 1" {
		t.Errorf("North: %v", n)
	}
	var who []string
	for _, cl := range v["clients"].([]any) {
		cl := cl.(map[string]any)
		who = append(who, fmt.Sprintf("%s %v asked %v moved %v refused %v back %v", cl["mac"].(string)[15:], cl["host"], cl["asked"], cl["moved"], cl["refused"], cl["back"]))
	}
	if got, want := strings.Join(who, "; "), "0b <nil> asked 3 moved 1 refused 1 back 0; 0c <nil> asked 2 moved 0 refused 0 back 0; 0a phone asked 2 moved 2 refused 0 back 1"; got != want {
		t.Errorf("clients:\n got %s\nwant %s", got, want)
	}
	latest := v["latest"].([]any)
	var ends []string
	for _, e := range latest {
		e := e.(map[string]any)
		ends = append(ends, fmt.Sprintf("%s %s %s %s", e["ap_name"], e["mac"].(string)[15:], e["method"], e["outcome"]))
	}
	// The one of a clock not set is of when it came in, the first report:
	// before the one made a minute ago, which the second brought.
	if got, want := strings.Join(ends, ", "), "North 0c suggest stayed, North 0b suggest moved, East 0a suggest moved, North 0c sticky left, North 0b hold joined, North 0b suggest refused, North 0a suggest back, North 0a suggest moved"; got != want {
		t.Errorf("latest:\n got %s\nwant %s", got, want)
	}
	last := latest[len(latest)-1].(map[string]any)
	if last["ap"] != north || last["host"] != "phone" || last["network"] != "sweet" || last["from"] != "2g" || fmt.Sprint(last["to"]) != "[6g 5g]" || last["asked"] != "btm" ||
		last["answer"] != "accepted" || last["code"] != float64(0) || last["signal"] != float64(-48) || last["took"] != 2.5 {
		t.Errorf("an asking, as said: %v", last)
	}
	if at, err := time.Parse(time.RFC3339Nano, last["at"].(string)); err != nil || at.Unix() != now.Add(-50*time.Minute).Unix() {
		t.Errorf("when it was: %v %v", last["at"], err)
	}

	// Over a longer time, the one of two days ago is there too; under a
	// folder, only its APs'.
	if got, want := counts(get("griff", "?hours=72")["networks"].([]any)[1].(map[string]any)["counts"]), "hold asked 1, hold joined 1, suggest asked 6, suggest back 1, suggest moved 3, suggest refused 2, suggest stayed 1"; got != want {
		t.Errorf("sweet over three days:\n got %s\nwant %s", got, want)
	}
	under := get("griff", "?under=annex")
	if nets := under["networks"].([]any); len(nets) != 1 || counts(nets[0].(map[string]any)["counts"]) != "suggest asked 1, suggest moved 1" || len(under["latest"].([]any)) != 1 || len(under["clients"].([]any)) != 1 {
		t.Errorf("under the annex: %v", under)
	}
	// One who may view the annex alone sees its askings alone, and no name
	// a client gave elsewhere.
	mine := get("tenant", "")
	if nets := mine["networks"].([]any); len(nets) != 1 || len(nets[0].(map[string]any)["aps"].([]any)) != 1 || len(mine["latest"].([]any)) != 1 {
		t.Errorf("the tenant's: %v", mine)
	}
	if cl := mine["clients"].([]any)[0].(map[string]any); cl["mac"] != a || cl["host"] != nil || cl["asked"] != float64(1) {
		t.Errorf("the tenant's client: %v", cl)
	}
	if nothing := get("tenant", "?under=office"); len(nothing["networks"].([]any)) != 0 || len(nothing["latest"].([]any)) != 0 || len(nothing["clients"].([]any)) != 0 {
		t.Errorf("the tenant, of the office: %v", nothing)
	}
	// North starts again, its clock once more not yet set, and says an
	// asking of the same time as one of its run before: another asking,
	// kept as well, and once however often it says it.
	again := asking(c, "sweet", "suggest", "btm", "stayed", 40*24*time.Hour, nil)
	report(northToken, 300, nil, again)
	report(northToken, 330, nil, again)
	if got, want := counts(get("griff", "?under=office")["networks"].([]any)[1].(map[string]any)["counts"]), "hold asked 1, hold joined 1, suggest asked 5, suggest back 1, suggest moved 2, suggest refused 1, suggest stayed 2"; got != want {
		t.Errorf("sweet at the office, North started again:\n got %s\nwant %s", got, want)
	}
	for _, q := range []string{"?hours=0", "?hours=721", "?hours=many"} {
		if code, b := f.do("GET", "/v1/steering"+q, "griff", nil); code != 400 {
			t.Errorf("%s: %d %v", q, code, b)
		}
	}
	if code, b := f.do("GET", "/v1/steering", "", nil); code != 401 {
		t.Errorf("no token: %d %v", code, b)
	}
}

// A sticky asking says why it was made (0289): weak, or better, where
// another AP heard the client more strongly, which the asking names with
// how strongly. Kept, it is read back with that AP's name, where the
// caller may view that AP. One that says why badly is left out, and the
// report taken.
func TestSteeringHeardBetter(t *testing.T) {
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
	enroll := func(name, mac, folder string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": []any{serving}}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder})
		return ap, token
	}
	north, _ := enroll("North", "02:00:00:00:00:3a", "office")
	_, eastToken := enroll("East", "02:00:00:00:00:3c", "annex")

	now := time.Now()
	asking := func(mac string, ago time.Duration, more map[string]any) map[string]any {
		e := map[string]any{"at": now.Add(-ago).Unix(), "mac": mac, "network": "sweet", "method": "sticky", "from": "5g", "asked": "kick", "outcome": "left", "took": 60.0, "signal": -74}
		for k, v := range more {
			e[k] = v
		}
		return e
	}
	code, _, cfg := f.apDo("GET", "/v1/ap/config", eastToken, nil, nil)
	if code != 200 {
		t.Fatalf("its config: %d %v", code, cfg)
	}
	events := []any{
		asking("02:00:00:00:00:0a", 30*time.Minute, map[string]any{"why": "better", "elsewhere": map[string]any{"ap": north, "signal": -62}}),
		asking("02:00:00:00:00:0b", 20*time.Minute, map[string]any{"why": "weak"}),
		// Of an older agent: no why.
		asking("02:00:00:00:00:0c", 10*time.Minute, nil),
		// Said badly: left out.
		asking("02:00:00:00:00:0d", 9*time.Minute, map[string]any{"why": "louder"}),
		asking("02:00:00:00:00:0e", 8*time.Minute, map[string]any{"why": "weak", "elsewhere": map[string]any{"ap": north, "signal": -62}}),
		asking("02:00:00:00:00:0f", 7*time.Minute, map[string]any{"why": "better", "elsewhere": map[string]any{"ap": "north", "signal": -62}}),
		asking("02:00:00:00:00:10", 6*time.Minute, map[string]any{"method": "suggest", "why": "weak"}),
		// Suggest across APs names the AP it sent the client to (0299); keep
		// on 6 GHz is a way of its own (0300).
		asking("02:00:00:00:00:11", 5*time.Minute, map[string]any{"method": "suggest", "asked": "btm", "to": []any{"6g"}, "elsewhere": map[string]any{"ap": north, "signal": -48}}),
		asking("02:00:00:00:00:12", 4*time.Minute, map[string]any{"method": "keep", "from": "5g", "to": []any{"6g"}, "asked": "held", "outcome": "stayed", "took": 0.0}),
		// Sent where a person said it belongs (0319): the AP, and how it heard
		// the client only where it did; heard better needs how strongly.
		asking("02:00:00:00:00:13", 3*time.Minute, map[string]any{"why": "home", "elsewhere": map[string]any{"ap": north}}),
		asking("02:00:00:00:00:14", 2*time.Minute, map[string]any{"why": "better", "elsewhere": map[string]any{"ap": north}}),
		asking("02:00:00:00:00:15", 1*time.Minute, map[string]any{"method": "suggest", "why": "home"}),
		// Sent to the AP that hears it best (0321), with how it heard it.
		asking("02:00:00:00:00:16", 30*time.Second, map[string]any{"why": "nearest", "elsewhere": map[string]any{"ap": north, "signal": -62}}),
	}
	body := map[string]any{"version": cfg["version"], "uptime": 86400, "steer": map[string]any{"running": true, "since": now.Add(-time.Hour).Unix(), "events": events,
		"networks": []any{map[string]any{"network": "sweet", "suggest": false, "hold": false, "sticky": true, "weak": -75, "margin": 5, "floor": -65, "hold_down": 10}}}}
	if code, _, b := f.apDo("POST", "/v1/ap/state", eastToken, body, nil); code != 200 {
		t.Fatalf("its report: %d %v", code, b)
	}
	said := func(as string) string {
		t.Helper()
		code, v := f.do("GET", "/v1/steering", as, nil)
		if code != 200 {
			t.Fatalf("steering as %s: %d %v", as, code, v)
		}
		var out []string
		for _, e := range v["latest"].([]any) {
			e := e.(map[string]any)
			s := fmt.Sprintf("%s %v", e["mac"].(string)[15:], e["why"])
			if w, ok := e["elsewhere"].(map[string]any); ok {
				s += fmt.Sprintf(" %v %v %v", w["ap"] == north, w["name"], w["signal"])
			}
			out = append(out, s)
		}
		return strings.Join(out, ", ")
	}
	if got, want := said("griff"), "16 nearest true North -62, 13 home true North <nil>, 12 <nil>, 11 <nil> true North -48, 0c <nil>, 0b weak, 0a better true North -62"; got != want {
		t.Errorf("as griff:\n got %s\nwant %s", got, want)
	}
	// The tenant may view the annex alone: not the AP that heard it better.
	if got, want := said("tenant"), "16 nearest, 13 home, 12 <nil>, 11 <nil>, 0c <nil>, 0b weak, 0a better"; got != want {
		t.Errorf("as the tenant:\n got %s\nwant %s", got, want)
	}
}

// What came of an asking, by where its client was let on next (0291): the
// AP and band it joined within minutes of the asking's end, how strongly it
// was heard there, and that set against how its old AP heard it, on the
// same band only. One that went nowhere, or stayed, has no after. Counted
// for the network and the AP that asked; the AP it went to named only
// where the caller may view it.
func TestSteeringAfter(t *testing.T) {
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
	enroll := func(name, mac, folder string) (string, string) {
		t.Helper()
		code, _, body := f.apDo("POST", "/v1/enroll", "", map[string]any{"mac": mac, "hostname": name, "radios": []any{serving}}, nil)
		if code != 201 {
			t.Fatalf("enroll %s: %d %v", name, code, body)
		}
		ap, token := body["ap"].(string), body["token"].(string)
		change(map[string]any{"kind": "move", "tree": "locations", "node": ap, "parent": folder})
		return ap, token
	}
	north, northToken := enroll("North", "02:00:00:00:00:3a", "office")
	east, eastToken := enroll("East", "02:00:00:00:00:3c", "annex")

	now := time.Now()
	const a, b, c, d, e = "02:00:00:00:01:0a", "02:00:00:00:01:0b", "02:00:00:00:01:0c", "02:00:00:00:01:0d", "02:00:00:00:01:0e"
	asking := func(mac string, ago time.Duration, more map[string]any) map[string]any {
		e := map[string]any{"at": now.Add(-ago).Unix(), "mac": mac, "network": "sweet", "method": "sticky", "from": "5g", "asked": "kick", "outcome": "left", "took": 60.0, "signal": -77}
		for k, v := range more {
			e[k] = v
		}
		return e
	}
	code, _, cfg := f.apDo("GET", "/v1/ap/config", eastToken, nil, nil)
	if code != 200 {
		t.Fatalf("its config: %d %v", code, cfg)
	}
	events := []any{
		asking(a, 30*time.Minute, map[string]any{"why": "better", "elsewhere": map[string]any{"ap": north, "signal": -60}}),
		asking(b, 20*time.Minute, map[string]any{"why": "weak", "signal": -88}),
		asking(c, 15*time.Minute, map[string]any{"why": "weak", "signal": -80}),
		asking(d, 10*time.Minute, map[string]any{"method": "suggest", "asked": "btm", "outcome": "stayed", "from": "2g", "signal": -50}),
		// e, alone at East: put off at -93, it ended as left, and came back
		// 5.5 minutes on, which a record of its own says, asking nothing
		// (0289). Its asking is followed, and finds it back; the record is
		// not followed again, nor counted twice.
		asking(e, 45*time.Minute, map[string]any{"why": "weak", "from": "2g", "signal": -93}),
		asking(e, 39*time.Minute+30*time.Second, map[string]any{"why": "weak", "from": "2g", "asked": "none", "outcome": "back", "took": 330.0, "signal": nil}),
	}
	body := map[string]any{"version": cfg["version"], "uptime": 86400, "steer": map[string]any{"running": true, "since": now.Add(-time.Hour).Unix(), "events": events}}
	if code, _, b := f.apDo("POST", "/v1/ap/state", eastToken, body, nil); code != 200 {
		t.Fatalf("its report: %d %v", code, b)
	}
	// Where they went: a on North's 5 GHz 40 s after its asking, at -62; c
	// on North's 2.4 GHz; d, which stayed, on North too, but that is not
	// what its asking came to. b nowhere.
	joined := func(mac, band string, at time.Time, signal int) map[string]any {
		return map[string]any{"mac": mac, "bss": "phy0-ap0", "network": "sweet", "ssid": "Home Wi-Fi", "band": band,
			"started": at.UnixMilli(), "took_ms": 300, "outcome": "online", "stage": "internet", "reason": "", "signal": signal,
			"events": []any{map[string]any{"t": 0, "stage": "auth", "what": "authenticated", "ok": true}}}
	}
	conns := []any{
		joined(a, "5g", now.Add(-30*time.Minute+40*time.Second), -62),
		joined(c, "2g", now.Add(-15*time.Minute+30*time.Second), -55),
		joined(d, "5g", now.Add(-10*time.Minute+20*time.Second), -45),
	}
	if code, _, got := f.apDo("POST", "/v1/ap/connections", northToken, map[string]any{"connections": conns}, nil); code != 200 || got["recorded"] != float64(3) {
		t.Fatalf("its connections: %d %v", code, got)
	}
	back := []any{joined(e, "2g", now.Add(-45*time.Minute+5*time.Minute+30*time.Second), -92)}
	if code, _, got := f.apDo("POST", "/v1/ap/connections", eastToken, map[string]any{"connections": back}, nil); code != 200 || got["recorded"] != float64(1) {
		t.Fatalf("East's connections: %d %v", code, got)
	}
	said := func(as string) (string, map[string]any) {
		t.Helper()
		code, v := f.do("GET", "/v1/steering", as, nil)
		if code != 200 {
			t.Fatalf("steering as %s: %d %v", as, code, v)
		}
		var out []string
		for _, e := range v["latest"].([]any) {
			e := e.(map[string]any)
			s := e["mac"].(string)[15:]
			if x, ok := e["after"].(map[string]any); ok {
				s += fmt.Sprintf(" → %v %v %v %v %v", x["ap"] == north, x["ap_name"], x["band"], x["signal"], e["result"])
			}
			out = append(out, s)
		}
		var results any
		for _, n := range v["networks"].([]any) {
			n := n.(map[string]any)
			if n["network"] == "sweet" {
				results = n["results"]
				for _, ap := range n["aps"].([]any) {
					if ap := ap.(map[string]any); ap["ap"] == east && fmt.Sprint(ap["results"]) != fmt.Sprint(results) {
						t.Errorf("East's results %v, the network's %v", ap["results"], results)
					}
				}
			}
		}
		if v["weighed"] != float64(6) {
			t.Errorf("weighed %v, want 6", v["weighed"])
		}
		return strings.Join(out, ", "), map[string]any{"results": results}
	}
	got, more := said("griff")
	if want := "0d, 0c → true North 2g -55 band, 0b, 0a → true North 5g -62 stronger, 0e, 0e → false East 2g -92 back"; got != want {
		t.Errorf("as griff:\n got %s\nwant %s", got, want)
	}
	if fmt.Sprint(more["results"]) != "map[back:1 band:1 stronger:1]" {
		t.Errorf("results: %v", more["results"])
	}
	// The tenant may view the annex alone: where they went, not by name.
	if got, _ := said("tenant"); got != "0d, 0c → false <nil> 2g -55 band, 0b, 0a → false <nil> 5g -62 stronger, 0e, 0e → false East 2g -92 back" {
		t.Errorf("as the tenant: %s", got)
	}
}
