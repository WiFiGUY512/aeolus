package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What of an AP's RRM, neighbours and transports changed between two of
// its reports, for its Activity (0338).
func TestActivityChangesBetweenReports(t *testing.T) {
	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)
	prevAt := now.Add(-5 * time.Minute)
	agent := &agentState{Version: "v0.302.0"}
	base := func() stateReport {
		return stateReport{
			Uptime: 86400, Agent: agent,
			RRM: &rrmState{
				Neighbours: []rrmNeighbour{{AP: "ap-garage", Address: "192.168.1.167", State: "up"}, {AP: "ap-office", Address: "192.168.1.69", State: "up"}, {AP: "ap-far", State: "heard"}},
				Moves:      []rrmMove{{Band: "5g", From: 36, To: 44, Why: "better", State: "moved", At: now.Add(-time.Hour).Unix()}},
			},
			VXLAN:  &vxlanState{Tunnels: []tunnelState{{VNI: 9003, Peer: "192.168.1.44", Up: true, Probe: &probeState{Verdict: "up"}}}},
			Radius: []radiusState{{Network: "corp", Server: "10.0.0.5", Port: 1812, Verdict: "up"}},
		}
	}
	names := map[hierarchy.NodeID]string{"ap-garage": "Garage-C-360", "ap-office": "Office", "ap-far": "Far", "ap-lr": "Living Room"}
	name := func(id hierarchy.NodeID) string { return names[id] }
	say := func(cur stateReport, restart bool) (string, []conditions.APEvent) {
		old := base()
		raw, _ := json.Marshal(old)
		cur.Uptime = old.Uptime + 300
		if restart {
			cur.Uptime = 40
		}
		evs := changes(&conditions.State{At: prevAt, Report: raw}, now, &cur, name)
		var out []string
		for _, e := range evs {
			out = append(out, e.Kind+": "+e.Text)
		}
		return strings.Join(out, "; "), evs
	}
	next := func(change func(*stateReport)) stateReport {
		r := base()
		change(&r)
		return r
	}
	for label, c := range map[string]struct {
		cur     stateReport
		restart bool
		want    string
	}{
		"nothing": {next(func(r *stateReport) {}), false, ""},
		"a claim": {next(func(r *stateReport) {
			r.RRM.Moves = append(r.RRM.Moves, rrmMove{Band: "6g", From: 37, To: 69, Why: "shared", State: "announced", At: now.Add(-time.Minute).Unix()})
		}), false, "rrm: 6 GHz: claims channel 69 in its hellos, to move from 37: a neighbour uses its channel"},
		"a move from long ago, newly said": {next(func(r *stateReport) {
			r.RRM.Moves = append(r.RRM.Moves, rrmMove{Band: "2g", From: 1, To: 6, Why: "start", State: "moved", At: now.Add(-3 * time.Hour).Unix()})
		}), false, ""},
		"a yield": {next(func(r *stateReport) {
			r.RRM.Moves = append(r.RRM.Moves, rrmMove{Band: "5g", From: 44, To: 149, Why: "better", State: "yielded", AP: "ap-lr", At: now.Add(-time.Minute).Unix()})
		}), false, "rrm: 5 GHz: let Living Room move first, and stays on channel 44"},
		"its clients followed": {next(func(r *stateReport) {
			r.RRM.Moves[0].Clients = &moveClients{Clients: 5, Kept: 3, Back: 1, Gone: 1}
		}), false, "rrm: 5 GHz: of the 5 clients it had on channel 36, 3 followed it to 44, 1 joined again, 1 are gone"},
		"hellos stopped, and one way": {next(func(r *stateReport) {
			r.RRM.Neighbours[0].State, r.RRM.Neighbours[1].State = "down", "one-way"
		}), false, "neighbour: hellos with Garage-C-360 stopped; neighbour: hellos from Office still come, but no longer list this AP"},
		"one gone from the list, forgotten": {next(func(r *stateReport) {
			r.RRM.Neighbours = r.RRM.Neighbours[1:]
		}), false, ""},
		"one only heard now, forgotten": {next(func(r *stateReport) {
			r.RRM.Neighbours[0].State = "heard"
		}), false, ""},
		"RRM started afresh": {next(func(r *stateReport) {
			up := int64(90)
			r.RRM.Up = &up
			r.RRM.Neighbours[0].State, r.RRM.Neighbours[1].State = "down", "heard"
			r.RRM.CannotScan = []rrmDeaf{{Band: "5g", Channel: 100, Why: "dfs"}}
		}), false, "rrm: RRM started again, 90 s ago: it finds its neighbours afresh"},
		"hellos began": {next(func(r *stateReport) {
			r.RRM.Neighbours[2].State = "up"
			r.RRM.Neighbours = append(r.RRM.Neighbours, rrmNeighbour{AP: "ap-lr", State: "up"})
		}), false, "neighbour: hellos with Far and Living Room began, both ways"},
		"stopped, just after a restart": {next(func(r *stateReport) {
			r.RRM.Neighbours[0].State = "heard"
		}), true, "restart: restarted: it had been up 24 h"},
		"a radio that can't scan": {next(func(r *stateReport) {
			r.RRM.CannotScan = []rrmDeaf{{Band: "5g", Channel: 100, Why: "dfs"}}
		}), false, "rrm: 5 GHz: can't scan on channel 100: on a DFS channel it must keep listening for radar, and rates its own channel by its own counts"},
		"a tunnel down, RADIUS silent": {next(func(r *stateReport) {
			r.VXLAN.Tunnels[0].Probe = &probeState{Verdict: "down"}
			r.Radius[0].Verdict = "silent"
		}), false, "tunnel: the tunnel to 192.168.1.44, VNI 9003, is down; it answers before; radius: the RADIUS server 10.0.0.5 is silent; it answers before"},
		"a tunnel starting is no news": {next(func(r *stateReport) {
			r.VXLAN.Tunnels[0].Probe = &probeState{Verdict: "unknown"}
		}), false, ""},
	} {
		got, evs := say(c.cur, c.restart)
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", label, got, c.want)
		}
		for _, e := range evs {
			if e.Kind != "restart" && len(e.Detail) == 0 {
				t.Errorf("%s: %s has no detail", label, e.Kind)
			}
		}
	}
	// A claim is kept at its own time; a stopped neighbour, a warning.
	_, evs := say(next(func(r *stateReport) {
		r.RRM.Moves = append(r.RRM.Moves, rrmMove{Band: "6g", From: 37, To: 69, Why: "shared", State: "failed", At: now.Add(-time.Minute).Unix()})
	}), false)
	if len(evs) != 1 || !evs[0].At.Equal(now.Add(-time.Minute)) || !strings.Contains(string(evs[0].Detail), `"severity":"warning"`) {
		t.Fatalf("a failed move: %+v", evs)
	}
}

// An AP's Activity (0338): its lines from every source, newest first, a
// page at a time, by group.
func TestAPActivity(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	id := hierarchy.NodeID(ap)
	post := func(uptime int, rrm map[string]any) {
		body := map[string]any{"version": version, "uptime": uptime, "radios": []any{map[string]any{"radio": "radio0", "band": "5g", "channel": 36, "width": 80, "clients": 0, "up": true}}}
		if rrm != nil {
			body["rrm"] = rrm
		}
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	rrm := func(state string, moves ...any) map[string]any {
		if moves == nil {
			moves = []any{}
		}
		return map[string]any{"address": "192.168.1.10", "advertised": []string{}, "ratings": []any{},
			"neighbours": []any{map[string]any{"ap": "ap-0200000000ff", "address": "192.168.1.11", "state": state, "chosen": true, "hello_ago": 1, "bands": []any{}}},
			"moves":      moves}
	}
	post(5000, rrm("up"))
	post(5300, rrm("down", map[string]any{"band": "5g", "from": 36, "to": 149, "why": "better", "state": "announced", "at": time.Now().Unix()}))
	// Power steps and an apply, from the other sources.
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	from, to := 20, 17
	must(f.conds.RecordAPCEvents(id, []conditions.APCEvent{{At: time.Now().Add(-time.Hour), Band: "5g", From: &from, To: &to, Why: "above"}}))
	// Ten hours of accounts, to page through.
	for i := 1; i <= 10; i++ {
		must(f.conds.AddEvents(id, []conditions.APEvent{{At: time.Now().Add(-time.Duration(i) * time.Hour), Kind: "hour", Text: "the hour: 12 reports"}}))
	}

	read := func(q string) (lines []map[string]any, more bool) {
		t.Helper()
		code, got := f.do("GET", "/v1/aps/"+ap+"/activity"+q, "griff", nil)
		if code != 200 {
			t.Fatalf("%s: %d %v", q, code, got)
		}
		for _, l := range got["lines"].([]any) {
			lines = append(lines, l.(map[string]any))
		}
		return lines, got["more"] == true
	}
	all, _ := read("?limit=200")
	var said []string
	for _, l := range all {
		said = append(said, l["kind"].(string)+": "+l["text"].(string))
	}
	text := strings.Join(said, "\n")
	for _, want := range []string{"first: first report", "rrm: 5 GHz: claims channel 149 in its hellos, to move from 36: another rates better",
		"neighbour: hellos with ap-0200000000ff stopped", "power: 5 GHz: power down from 20 to 17 dBm: every neighbour hears it well above the target", "hour: the hour: 12 reports"} {
		if !strings.Contains(text, want) {
			t.Errorf("its Activity lacks %q:\n%s", want, text)
		}
	}
	for i := 1; i < len(all); i++ {
		a, _ := time.Parse(time.RFC3339Nano, all[i-1]["at"].(string))
		b, _ := time.Parse(time.RFC3339Nano, all[i]["at"].(string))
		if b.After(a) {
			t.Fatalf("not newest first at %d:\n%s", i, text)
		}
	}
	// A page at a time: each before the last's time, none twice, none left
	// out.
	seen := map[string]bool{}
	before := ""
	for pages := 0; ; pages++ {
		if pages > 20 {
			t.Fatal("paging does not end")
		}
		page, more := read("?limit=3" + before)
		for _, l := range page {
			if seen[l["id"].(string)] {
				t.Fatalf("%s twice", l["id"])
			}
			seen[l["id"].(string)] = true
		}
		if !more || len(page) == 0 {
			break
		}
		before = "&before=" + page[len(page)-1]["at"].(string)
	}
	if len(seen) != len(all) {
		t.Fatalf("paged %d of %d", len(seen), len(all))
	}
	// One group alone.
	nb, _ := read("?only=neighbours")
	if len(nb) != 1 || nb[0]["severity"] != "warning" || nb[0]["group"] != "neighbours" {
		t.Fatalf("neighbours = %v", nb)
	}
	radios, _ := read("?only=radios")
	for _, l := range radios {
		if l["group"] != "radios" {
			t.Fatalf("radios = %v", radios)
		}
	}
	if len(radios) != 2 {
		t.Fatalf("radios = %v", radios)
	}
	for _, q := range []string{"?only=nothing", "?limit=0", "?before=yesterday"} {
		if code, _ := f.do("GET", "/v1/aps/"+ap+"/activity"+q, "griff", nil); code != 400 {
			t.Errorf("%s: %d", q, code)
		}
	}
	if code, _ := f.do("GET", "/v1/aps/no-such-ap/activity", "griff", nil); code != 404 {
		t.Errorf("no such AP: %d", code)
	}
}

// An hour's account of itself (0338).
func TestHourSaid(t *testing.T) {
	hour := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)
	cpu, temp := 23, 51.6
	total, avail := int64(1000), int64(590)
	cur := &stateReport{Version: 812, RRM: &rrmState{Neighbours: []rrmNeighbour{{AP: "a", State: "up"}, {AP: "b", State: "up"}, {AP: "c", State: "down"}, {AP: "d", State: "heard"}}},
		VXLAN: &vxlanState{Tunnels: []tunnelState{{VNI: 1, Probe: &probeState{Verdict: "up"}}, {VNI: 2, Probe: &probeState{Verdict: "down"}}}}}
	e := hourSaid(hour, conditions.HealthPoint{Clients: 7, CPU: &cpu, Temp: &temp, MemTotal: &total, MemAvailable: &avail}, 12, hour.Add(2*time.Second), cur)
	want := "the hour: 12 reports; hellos both ways with 2 APs, 1 not; 1 of 2 tunnels answer; processor at most 23%; memory 41% in use; 52 °C; at most 7 clients"
	if e.Kind != "hour" || e.Text != want || !e.At.Equal(hour.Add(time.Hour)) {
		t.Fatalf("got %s %q at %s", e.Kind, e.Text, e.At)
	}
	if e = hourSaid(hour, conditions.HealthPoint{}, 4, hour.Add(time.Minute), &stateReport{}); !strings.Contains(e.Text, "4 reports of the 12 due") || !strings.Contains(string(e.Detail), `"severity":"warning"`) {
		t.Fatalf("a short hour: %q %s", e.Text, e.Detail)
	}
	// One that came at 17:45 is due four, and made them.
	if e = hourSaid(hour, conditions.HealthPoint{}, 4, hour.Add(45*time.Minute), &stateReport{}); e.Text != "the hour: 4 reports; at most 0 clients" || strings.Contains(string(e.Detail), "severity") {
		t.Fatalf("an hour begun late: %q %s", e.Text, e.Detail)
	}
}

// A channel RRM moved to is said once, by RRM (0338); one it did not, as
// the radio's own.
func TestActivityChannelSaidOnce(t *testing.T) {
	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)
	yes := true
	report := func(channel int, moves []rrmMove) stateReport {
		return stateReport{Uptime: 86400, Agent: &agentState{Version: "v1"},
			Radios: []radioState{{Radio: "radio0", Band: "5g", Channel: channel, Width: 80, Up: &yes}},
			RRM:    &rrmState{Moves: moves}}
	}
	old := report(36, nil)
	raw, _ := json.Marshal(old)
	prev := &conditions.State{At: now.Add(-5 * time.Minute), Report: raw}
	name := func(id hierarchy.NodeID) string { return string(id) }
	say := func(cur stateReport) string {
		var out []string
		for _, e := range changes(prev, now, &cur, name) {
			out = append(out, e.Kind+": "+e.Text)
		}
		return strings.Join(out, "; ")
	}
	cur := report(149, []rrmMove{{Band: "5g", From: 36, To: 149, Why: "better", State: "moved", At: now.Add(-time.Minute).Unix()}})
	cur.Uptime += 300
	if got := say(cur); got != "rrm: 5 GHz: moved from channel 36 to 149: another rates better" {
		t.Errorf("moved by RRM: %q", got)
	}
	cur = report(149, nil)
	cur.Uptime += 300
	if got := say(cur); got != "channel: 5 GHz moved from channel 36 to 149" {
		t.Errorf("moved otherwise: %q", got)
	}
}

// RRM's evaluations (0338): a plan begun, changed, given up, or ended by
// its claim.
func TestActivityPlans(t *testing.T) {
	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)
	plan := func(to int, why string, radar bool) rrmPlan {
		return rrmPlan{Band: "5g", From: 36, To: to, Why: why, Radar: radar, Ago: 60}
	}
	say := func(old, cur []rrmPlan, moves []rrmMove) string {
		o := stateReport{Uptime: 86400, Agent: &agentState{Version: "v1"}, RRM: &rrmState{Plans: old}}
		raw, _ := json.Marshal(o)
		c := stateReport{Uptime: 86700, Agent: &agentState{Version: "v1"}, RRM: &rrmState{Plans: cur, Moves: moves}}
		var out []string
		for _, e := range changes(&conditions.State{At: now.Add(-5 * time.Minute), Report: raw}, now, &c, func(id hierarchy.NodeID) string { return string(id) }) {
			out = append(out, e.Kind+": "+e.Text)
		}
		return strings.Join(out, "; ")
	}
	for label, c := range map[string]struct {
		old, cur []rrmPlan
		moves    []rrmMove
		want     string
	}{
		"held":   {[]rrmPlan{plan(149, "better", false)}, []rrmPlan{plan(149, "better", false)}, nil, ""},
		"begun":  {nil, []rrmPlan{plan(149, "better", false)}, nil, "rrm: 5 GHz: channel 149 rates better than 36; it moves there once that has held 10 minutes, in its move window"},
		"shared": {nil, []rrmPlan{plan(149, "shared", false)}, nil, "rrm: 5 GHz: a neighbour uses channel 36 too; it moves to 149 once that has held 10 minutes, in its move window"},
		"radar":  {nil, []rrmPlan{plan(100, "better", true)}, nil, "rrm: 5 GHz: DFS channel 100 rates clearly best; it moves there from 36 in its move window, once that has held 10 minutes, after a radar check"},
		"elsewhere": {[]rrmPlan{plan(149, "better", false)}, []rrmPlan{plan(157, "better", false)}, nil,
			"rrm: 5 GHz: channel 157 rates better than 36; it moves there once that has held 10 minutes, in its move window (in place of channel 149)"},
		"given up": {[]rrmPlan{plan(149, "better", false)}, nil, nil, "rrm: 5 GHz: no longer looks to move to channel 149: its reason went before it held long enough"},
		"claimed": {[]rrmPlan{plan(149, "better", false)}, nil, []rrmMove{{Band: "5g", From: 36, To: 149, Why: "better", State: "announced", At: now.Add(-time.Minute).Unix()}},
			"rrm: 5 GHz: claims channel 149 in its hellos, to move from 36: another rates better"},
	} {
		if got := say(c.old, c.cur, c.moves); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", label, got, c.want)
		}
	}
}

// RRM started afresh is said once (0338): where the time it has run went
// back, not on each report of its first minutes.
func TestActivityRRMStartedOnce(t *testing.T) {
	now := time.Date(2026, 11, 7, 19, 30, 0, 0, time.UTC)
	up := func(n int64) *int64 { return &n }
	say := func(prevAgo time.Duration, was, is *int64) string {
		o := stateReport{Uptime: 86400, Agent: &agentState{Version: "v1"}, RRM: &rrmState{Up: was}}
		raw, _ := json.Marshal(o)
		c := stateReport{Uptime: 86400 + int64(prevAgo.Seconds()), Agent: &agentState{Version: "v1"}, RRM: &rrmState{Up: is}}
		var out []string
		for _, e := range changes(&conditions.State{At: now.Add(-prevAgo), Report: raw}, now, &c, func(id hierarchy.NodeID) string { return string(id) }) {
			out = append(out, e.Kind+": "+e.Text)
		}
		return strings.Join(out, "; ")
	}
	for label, c := range map[string]struct {
		ago     time.Duration
		was, is *int64
		want    string
	}{
		"went back":                {10 * time.Second, up(600), up(5), "rrm: RRM started again, 5 s ago: it finds its neighbours afresh"},
		"on, 10 s later":           {10 * time.Second, up(5), up(15), ""},
		"on, five minutes later":   {5 * time.Minute, up(15), up(315), ""},
		"an older agent before":    {5 * time.Minute, nil, up(90), "rrm: RRM started again, 90 s ago: it finds its neighbours afresh"},
		"an older agent, long run": {5 * time.Minute, nil, up(4000), ""},
	} {
		if got := say(c.ago, c.was, c.is); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", label, got, c.want)
		}
	}
}
