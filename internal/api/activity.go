package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/radio"
)

// An AP's Activity (0338) is everything it did and that was done to it,
// a line each, newest first, as a client's Activity is its attempts
// (0337): what changed between its reports, as kept since 0119, and now
// also its RRM's claims and moves, its neighbours' hellos beginning and
// ending, its tunnels', probes' and RADIUS servers' verdicts changing, and
// an hour's account of itself at the end of each hour; with the configs
// it applied, the alerts that began and ended on it, what it was asked to
// do, its power steps, what its steering asked of its clients, and its
// client tests, those it made and those made of it.

// activityGroup is the group each kind of thing kept in ap_events is
// shown under, as the page's filters pick them. A kind not named is its
// health's.
var activityGroup = map[string]string{
	"first": "health", "restart": "health", "silence": "health", "agent": "health", "firmware": "health", "uplink": "health", "hour": "health",
	"radio": "radios", "channel": "radios", "scan-radio": "radios", "rrm": "radios",
	"neighbour": "neighbours",
	"tunnel":    "network", "probe": "network", "radius": "network",
}

// activityGroups are the groups, in the order the page offers them.
var activityGroups = []string{"health", "radios", "neighbours", "network", "config", "alerts", "clients"}

// activityKinds is the kinds kept in ap_events of a group.
func activityKinds(group string) []string {
	var out []string
	for k, g := range activityGroup {
		if g == group {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// activityLine is one line of an AP's Activity: an ID that tells it from
// every other, of its source and its row; when; its group and kind; what,
// in words; trouble, where it is (critical, warning or info); the client
// it is of, where it is of one; and what more it opens to.
type activityLine struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Group    string    `json:"group"`
	Kind     string    `json:"kind"`
	Text     string    `json:"text"`
	Severity string    `json:"severity,omitempty"`
	MAC      string    `json:"mac,omitempty"`
	Detail   any       `json:"detail,omitempty"`
}

// activitySource reads one source of an AP's Activity: its lines before
// before, the newest first, at most n; of the group only, where it is
// given, which a source of more than one group reads itself. groups are
// those it gives lines to, so a page of another group does not read it.
type activitySource struct {
	groups []string
	read   func(s *Server, c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error)
}

var activitySources = []activitySource{
	{[]string{"health", "radios", "neighbours", "network"}, (*Server).activityEvents},
	{[]string{"config"}, (*Server).activityApplies},
	{[]string{"config"}, (*Server).activityActions},
	{[]string{"alerts"}, (*Server).activityAlerts},
	{[]string{"radios"}, (*Server).activityPower},
	{[]string{"clients"}, (*Server).activitySteer},
	{[]string{"clients"}, (*Server).activityTests},
}

// apActivity is a page of an AP's Activity: GET /v1/aps/{ap}/activity,
// ?before= a time (RFC 3339, now unless given), ?limit= how many (20 unless
// set, at most 200) and ?only= a group. Lines at the same time as the last
// come with it, so the next page, before that time, leaves none out. more
// says there may be older lines.
func (s *Server) apActivity(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	if err := apNode(c, id); err != nil {
		return err
	}
	q := r.URL.Query()
	before := time.Now().Add(time.Second)
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return badRequest("before: a time, as RFC 3339 has it")
		}
		before = t
	}
	limit := 20
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			return badRequest("limit: from 1 to 200")
		}
		limit = n
	}
	only := q.Get("only")
	if only != "" && !contains(activityGroups, only) {
		return badRequest("only: one of %s", strings.Join(activityGroups, ", "))
	}
	// Each source reads some more than a page, so lines at the last's time
	// come whole.
	n := limit + 20
	var all []activityLine
	more := false
	for _, src := range activitySources {
		if only != "" && !contains(src.groups, only) {
			continue
		}
		got, err := src.read(s, c, id, before, only, n)
		if err != nil {
			return err
		}
		if len(got) >= n {
			more = true
		}
		all = append(all, got...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].At.Equal(all[j].At) {
			return all[i].At.After(all[j].At)
		}
		return all[i].ID > all[j].ID
	})
	if len(all) > limit {
		cut := limit
		for cut < len(all) && all[cut].At.Equal(all[limit-1].At) {
			cut++
		}
		more = more || cut < len(all)
		all = all[:cut]
	}
	if all == nil {
		all = []activityLine{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": all, "more": more, "groups": activityGroups})
	return nil
}

func contains(list []string, x string) bool {
	for _, v := range list {
		if v == x {
			return true
		}
	}
	return false
}

// activityEvents is what changed between its reports (0119, 0338).
func (s *Server) activityEvents(c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error) {
	var kinds []string
	if only != "" {
		kinds = activityKinds(only)
	}
	evs, err := s.conds.EventsBefore(ap, before, kinds, n)
	if err != nil {
		return nil, err
	}
	out := make([]activityLine, 0, len(evs))
	for _, e := range evs {
		l := activityLine{ID: fmt.Sprintf("e%d", e.ID), At: e.At, Group: activityGroup[e.Kind], Kind: e.Kind, Text: e.Text}
		if l.Group == "" {
			l.Group = "health"
		}
		if e.Kind == "restart" || e.Kind == "silence" {
			l.Severity = "warning"
		}
		if len(e.Detail) > 0 {
			var d map[string]any
			if json.Unmarshal(e.Detail, &d) == nil {
				if sev, _ := d["severity"].(string); sev != "" {
					l.Severity = sev
				}
				l.Detail = d
			}
		}
		out = append(out, l)
	}
	return out, nil
}

// activityApplies is each config it applied, or could not.
func (s *Server) activityApplies(c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error) {
	ids, applies, err := s.conds.AppliedBefore(ap, before, n)
	if err != nil {
		return nil, err
	}
	out := make([]activityLine, 0, len(applies))
	for i, a := range applies {
		l := activityLine{ID: fmt.Sprintf("c%d", ids[i]), At: a.At, Group: "config", Kind: "config", Detail: a}
		if a.OK {
			l.Text = fmt.Sprintf("applied config version %d", a.Version)
		} else {
			l.Text, l.Severity = fmt.Sprintf("could not apply config version %d: %s", a.Version, a.Error), "warning"
		}
		out = append(out, l)
	}
	return out, nil
}

// activityActions is what it was asked to do once, but its client tests,
// which are its clients'.
func (s *Server) activityActions(c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error) {
	acts, err := s.conds.ActionsBefore(ap, before, nil, []string{"client-test"}, n)
	if err != nil {
		return nil, err
	}
	out := make([]activityLine, 0, len(acts))
	for _, a := range acts {
		text := fmt.Sprintf("%s asked it to %s", a.Actor, a.Kind)
		if a.Target != "" {
			text += " " + a.Target
		}
		text += ": " + a.State
		if a.Result != "" {
			text += ", " + a.Result
		}
		l := activityLine{ID: fmt.Sprintf("a%d", a.ID), At: a.At, Group: "config", Kind: "action", Text: text,
			Detail: map[string]any{"action": a.ID, "kind": a.Kind, "actor": a.Actor, "target": a.Target, "state": a.State, "result": a.Result, "done_at": a.DoneAt}}
		if a.State == "failed" || a.State == "expired" {
			l.Severity = "warning"
		}
		if a.Kind == "disconnect" && macRE.MatchString(a.Target) {
			l.MAC = strings.ToLower(a.Target)
		}
		out = append(out, l)
	}
	return out, nil
}

// activityAlerts is each alert that began on it, and each that ended.
func (s *Server) activityAlerts(c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error) {
	began, err := s.conds.AlertsBegunBefore(ap, before, n)
	if err != nil {
		return nil, err
	}
	ended, err := s.conds.AlertsEndedBefore(ap, before, n)
	if err != nil {
		return nil, err
	}
	out := make([]activityLine, 0, len(began)+len(ended))
	for _, a := range began {
		out = append(out, activityLine{ID: fmt.Sprintf("l%d", a.ID), At: a.Began, Group: "alerts", Kind: "alert", Text: a.Message, Severity: a.Severity, Detail: a})
	}
	for _, a := range ended {
		out = append(out, activityLine{ID: fmt.Sprintf("m%d", a.ID), At: *a.Ended, Group: "alerts", Kind: "alert", Text: "ended: " + a.Message, Detail: a})
	}
	return out, nil
}

// activityPower is each of its power control's steps (0206).
func (s *Server) activityPower(c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error) {
	ids, steps, err := s.conds.APCBefore(ap, before, n)
	if err != nil {
		return nil, err
	}
	out := make([]activityLine, 0, len(steps))
	for i, e := range steps {
		out = append(out, activityLine{ID: fmt.Sprintf("p%d", ids[i]), At: e.At, Group: "radios", Kind: "power", Text: powerSaid(e), Detail: e})
	}
	return out, nil
}

// powerWhy is why power control stepped, in words, as its report says.
var powerWhy = map[string]string{
	"new": "it took the radio over", "looking": "fewer neighbours hear it than it looks for", "below": "the weakest neighbour hears it below the target",
	"above": "every neighbour hears it well above the target", "target": "its neighbours hear it at the target", "ceiling": "it is at its most",
	"floor": "it is at its least", "failed": "its power could not be read or set", "guard": "a client only it reaches needs it",
	"hold": "held up after its clients took a step down badly", "distress": "its clients took the step down badly, so it stepped back",
	"distress-seen": "its clients took the step down badly; it did not step back",
}

func powerSaid(e conditions.APCEvent) string {
	name := radio.BandName(e.Band)
	why := powerWhy[e.Why]
	if why == "" {
		why = e.Why
	}
	switch {
	case e.From != nil && e.To != nil && *e.To > *e.From:
		return fmt.Sprintf("%s: power up from %d to %d dBm: %s", name, *e.From, *e.To, why)
	case e.From != nil && e.To != nil && *e.To < *e.From:
		return fmt.Sprintf("%s: power down from %d to %d dBm: %s", name, *e.From, *e.To, why)
	case e.To != nil:
		return fmt.Sprintf("%s: power at %d dBm: %s", name, *e.To, why)
	}
	return fmt.Sprintf("%s: power control: %s", name, why)
}

// activitySteer is each asking its steering made of a client (0139).
func (s *Server) activitySteer(c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error) {
	evs, err := s.conds.SteerBefore(ap, before, n)
	if err != nil {
		return nil, err
	}
	t := c.state.Org.Locations
	out := make([]activityLine, 0, len(evs))
	for _, e := range evs {
		d := map[string]any{"network": e.Network, "method": e.Method, "from": e.From, "to": e.To, "asked": e.Asked, "answer": e.Answer,
			"code": e.Code, "signal": e.Signal, "outcome": e.Outcome, "took_ms": e.TookMS, "why": e.Why}
		if e.Better != "" && may(c, change.Locations, t, hierarchy.NodeID(e.Better), access.View) {
			d["better_ap"], d["better_signal"] = e.Better, e.BetterS
		}
		out = append(out, activityLine{ID: fmt.Sprintf("s%d.%s.%s.%s", e.Said, e.MAC, e.Method, e.Outcome), At: e.At, Group: "clients", Kind: "steer",
			MAC: e.MAC, Text: steerSaid(e, t), Detail: d})
	}
	return out, nil
}

// The ways of steering, what each asked, and what came of it, in words,
// as the network's Steering says them (0139, 0289).
var (
	steerWay       = map[string]string{"suggest": "Suggest", "hold": "Hold back", "sticky": "Sticky", "keep": "Keep on 6 GHz"}
	steerAskedSaid = map[string]string{"btm": "asked it to move", "held": "held it off", "kick": "put it off", "none": "asked nothing"}
	steerCame      = map[string]map[string]string{
		"suggest": {"moved": "it moved up", "refused": "it said no", "stayed": "it stayed", "left": "it left", "back": "it came back down"},
		"hold":    {"moved": "it joined a higher band", "joined": "it joined 2.4 GHz all the same", "left": "it did not join here", "back": "it came back down"},
		"sticky":  {"moved": "it moved to a better band here", "left": "it left", "back": "it came back", "recovered": "it was heard well again", "stayed": "it stayed"},
		"keep":    {"stayed": "it was kept, and let go on 6 GHz", "left": "it was kept until it left"},
	}
)

func steerSaid(e conditions.SteerEvent, t *hierarchy.Tree) string {
	way := steerWay[e.Method]
	if way == "" {
		way = e.Method
	}
	if e.Method == "sticky" && e.Why == "better" {
		way = "Sticky, heard better"
		if n, ok := t.Node(hierarchy.NodeID(e.Better)); ok {
			way += " by " + n.Name
		}
	}
	asked := steerAskedSaid[e.Asked]
	if asked == "" {
		asked = e.Asked
	}
	text := fmt.Sprintf("%s: on %s", way, radio.BandName(e.From))
	if e.Signal != nil {
		text += fmt.Sprintf(" at %d dBm", *e.Signal)
	}
	text += ", " + asked
	switch e.Answer {
	case "accepted":
		text += "; it said yes"
	case "refused":
		text += "; it said no"
	}
	if came := steerCame[e.Method][e.Outcome]; came != "" {
		text += "; " + came
	} else if e.Outcome != "" {
		text += "; " + e.Outcome
	}
	return text
}

// activityTests is its client tests (0121): those it made, of its own
// networks or another AP's, and those another AP made of it.
func (s *Server) activityTests(c call, ap hierarchy.NodeID, before time.Time, only string, n int) ([]activityLine, error) {
	made, err := s.conds.ActionsBefore(ap, before, []string{"client-test"}, nil, n)
	if err != nil {
		return nil, err
	}
	of, err := s.conds.TestsOfBefore(ap, before, n)
	if err != nil {
		return nil, err
	}
	t := c.state.Org.Locations
	seen := map[int64]bool{}
	out := make([]activityLine, 0, len(made)+len(of))
	for _, a := range append(made, of...) {
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		var ask clientTestAsk
		json.Unmarshal(a.Params, &ask)
		var res clientTest
		json.Unmarshal(a.Detail, &res)
		ssid := res.SSID
		if ssid == "" {
			ssid = ask.SSID
		}
		d := map[string]any{"action": a.ID, "state": a.State, "ssid": ssid, "band": res.Band, "outcome": res.Outcome, "stage": res.Stage, "reason": res.Reason, "took_ms": res.TookMS}
		by, on := hierarchy.NodeID(a.AP), hierarchy.NodeID(ask.On)
		mine := by == ap
		name := func(id hierarchy.NodeID) string {
			if n, ok := t.Node(id); ok && may(c, change.Locations, t, id, access.View) {
				return n.Name
			}
			return "another AP"
		}
		// The AP it was made from, where the caller may see it, which the
		// test's own page is read from.
		if may(c, change.Locations, t, by, access.View) {
			d["ap"] = by
		}
		var text string
		switch {
		case mine && (on == "" || on == ap):
			text = fmt.Sprintf("tested its own %s", ssid)
		case mine:
			text = fmt.Sprintf("tested %s on %s", ssid, name(on))
			if may(c, change.Locations, t, on, access.View) {
				d["on"] = on
			}
		default:
			text = fmt.Sprintf("%s tested its %s", name(by), ssid)
		}
		if res.Wire != nil {
			text += " over the wire"
		} else if b := radio.BandName(res.Band); b != "" {
			text += " on " + b
		}
		l := activityLine{ID: fmt.Sprintf("t%d", a.ID), At: a.At, Group: "clients", Kind: "client-test", Detail: d}
		switch {
		case a.State == "pending" || a.State == "running":
			text += ": " + a.State
		case res.Outcome == "passed" || res.Outcome == "online":
			text += ": it worked"
			if res.TookMS > 0 {
				text += fmt.Sprintf(", in %.1f s", float64(res.TookMS)/1000)
			}
		case res.Outcome != "":
			text += fmt.Sprintf(": it failed at %s", res.Stage)
			if res.Reason != "" {
				text += ": " + res.Reason
			}
			l.Severity = "warning"
		default:
			text += ": " + a.State
			if a.Result != "" {
				text += ", " + a.Result
			}
			if a.State == "failed" || a.State == "expired" {
				l.Severity = "warning"
			}
		}
		l.Text = text
		out = append(out, l)
	}
	return out, nil
}

// reportChanges is what of an AP's RRM, neighbours and transports changed
// between its last report, at prevAt, and this one (0338). name names a
// node, an AP its neighbours call it by. fresh says its agent started
// again since, or the AP did: neighbours heard again then, and transports
// starting, are no news.
//
// RRM started afresh, as at every agent update, forgets its neighbours,
// the radios that can't scan and its plans, and finds them again within
// a minute or two: neither the forgetting nor the finding is news. It
// started since the last report where the time it says it has run went
// back, or, where the last did not say, is less than has passed since;
// the last report was its first minutes where that said it had run under
// three. An older agent does not say: there, a neighbour
// only heard where its hellos went both ways was forgotten, for hellos
// that stop leave it down for a quarter of an hour first.
func reportChanges(prevAt, now time.Time, old, cur *stateReport, fresh bool, name func(hierarchy.NodeID) string,
	at func(kind, format string, args ...any) conditions.APEvent) []conditions.APEvent {
	var out []conditions.APEvent
	with := func(e conditions.APEvent, d map[string]any) conditions.APEvent {
		e.Detail, _ = json.Marshal(d)
		return e
	}
	if old.RRM != nil && cur.RRM != nil {
		anew := cur.RRM.Up != nil && ((old.RRM.Up != nil && *cur.RRM.Up < *old.RRM.Up) ||
			(old.RRM.Up == nil && time.Duration(*cur.RRM.Up)*time.Second < now.Sub(prevAt)))
		young := old.RRM.Up != nil && *old.RRM.Up < 180
		quiet := fresh || anew || young
		if anew && !fresh {
			out = append(out, with(at("rrm", "RRM started again, %s ago: it finds its neighbours afresh", span(time.Duration(*cur.RRM.Up)*time.Second)),
				map[string]any{"up": *cur.RRM.Up}))
		}
		moved := movesChanged(prevAt, now, old.RRM, cur.RRM, name, at, with)
		out = append(out, moved...)
		if !quiet {
			out = append(out, neighboursChanged(old.RRM, cur.RRM, name, at, with)...)
			out = append(out, deafChanged(old.RRM, cur.RRM, at, with)...)
		}
		out = append(out, plansChanged(old.RRM, cur.RRM, moved, quiet, at, with)...)
	}
	out = append(out, verdictsChanged(old, cur, fresh, at, with)...)
	return out
}

// moveWhy is why RRM moved, in words, as Radios says it.
var moveWhy = map[string]string{
	"start": "the radio had just started", "shared": "a neighbour uses its channel", "better": "another rates better",
	"interference": "interference on its channel", "own": "too near its own 5 GHz", "radar": "radar was found on its channel",
	"wide": "widened to the width it is set to",
}

func moveKey(m rrmMove) string {
	return fmt.Sprintf("%s|%d|%d|%s|%s|%d|%s", m.Band, m.From, m.To, m.Why, m.State, m.At, m.AP)
}

// movesChanged is each move RRM came to since its last report, by what it
// came to: claimed, checking for radar, made, yielded, withdrawn or
// failed; and, two minutes after one was made, what its clients did. A
// move from before the last report is not news, as one an agent just
// turned on says again.
func movesChanged(prevAt, now time.Time, old, cur *rrmState, name func(hierarchy.NodeID) string,
	at func(kind, format string, args ...any) conditions.APEvent, with func(conditions.APEvent, map[string]any) conditions.APEvent) []conditions.APEvent {
	was := map[string]rrmMove{}
	for _, m := range old.Moves {
		was[moveKey(m)] = m
	}
	var out []conditions.APEvent
	for _, m := range cur.Moves {
		band := radio.BandName(m.Band)
		why := moveWhy[m.Why]
		if why == "" {
			why = m.Why
		}
		d := map[string]any{"band": m.Band, "from": m.From, "to": m.To, "why": m.Why, "state": m.State}
		if m.AP != "" {
			d["ap"] = m.AP
		}
		o, ok := was[moveKey(m)]
		if ok {
			// What its clients did, said two minutes on (0265).
			if o.Clients == nil && m.Clients != nil && m.Clients.Clients > 0 {
				c := m.Clients
				d["clients"] = c
				text := fmt.Sprintf("%s: of the %d clients it had on channel %d, %d followed it to %d", band, c.Clients, m.From, c.Kept, m.To)
				if c.Back > 0 {
					text += fmt.Sprintf(", %d joined again", c.Back)
				}
				if c.Gone > 0 {
					text += fmt.Sprintf(", %d are gone", c.Gone)
					d["severity"] = "warning"
				}
				out = append(out, with(at("rrm", "%s", text), d))
			}
			continue
		}
		when := time.Unix(m.At, 0)
		if when.Before(prevAt.Add(-2*time.Minute)) || when.After(now.Add(time.Minute)) {
			continue
		}
		var e conditions.APEvent
		switch m.State {
		case "announced":
			e = at("rrm", "%s: claims channel %d in its hellos, to move from %d: %s", band, m.To, m.From, why)
		case "checking":
			e = at("rrm", "%s: listens for radar on channel %d before it moves there from %d", band, m.To, m.From)
		case "moved":
			e = at("rrm", "%s: moved from channel %d to %d: %s", band, m.From, m.To, why)
		case "yielded":
			who := "a neighbour"
			if m.AP != "" {
				who = name(hierarchy.NodeID(m.AP))
			}
			e = at("rrm", "%s: let %s move first, and stays on channel %d", band, who, m.From)
		case "withdrawn":
			e = at("rrm", "%s: no longer moves from channel %d to %d: its reason went", band, m.From, m.To)
		case "failed":
			e = at("rrm", "%s: could not move from channel %d to %d", band, m.From, m.To)
			d["severity"] = "warning"
		default:
			e = at("rrm", "%s: the move from channel %d to %d: %s", band, m.From, m.To, m.State)
		}
		if when.Before(now) {
			e.At = when
		}
		out = append(out, with(e, d))
	}
	return out
}

// neighboursChanged is each neighbour whose hellos began going both ways,
// or stopped (0073), a line for each way it went, naming them all. Hellos
// that stop leave a neighbour down for a quarter of an hour: one that goes
// from both ways to only heard in the air, or from the list, was
// forgotten, as by RRM starting afresh, and is no news.
func neighboursChanged(old, cur *rrmState, name func(hierarchy.NodeID) string,
	at func(kind, format string, args ...any) conditions.APEvent, with func(conditions.APEvent, map[string]any) conditions.APEvent) []conditions.APEvent {
	was := map[string]rrmNeighbour{}
	for _, n := range old.Neighbours {
		was[n.AP] = n
	}
	type went struct {
		ids  []string
		addr []string
	}
	by := map[string]*went{}
	note := func(how string, n rrmNeighbour) {
		w := by[how]
		if w == nil {
			w = &went{}
			by[how] = w
		}
		w.ids, w.addr = append(w.ids, n.AP), append(w.addr, n.Address)
	}
	for _, n := range cur.Neighbours {
		o, had := was[n.AP]
		delete(was, n.AP)
		switch {
		case n.State == o.State || (n.State != "up" && o.State != "up"):
		case n.State == "up" && (!had || o.State == "heard"):
			note("began", n)
		case n.State == "up":
			note("again", n)
		case n.State == "one-way":
			note("one-way", n)
		case n.State == "down":
			note("stopped", n)
		}
	}
	var out []conditions.APEvent
	for _, how := range []string{"stopped", "one-way", "again", "began"} {
		w := by[how]
		if w == nil {
			continue
		}
		names := make([]string, len(w.ids))
		for i, id := range w.ids {
			names[i] = name(hierarchy.NodeID(id))
		}
		list := joinNames(names)
		d := map[string]any{"aps": w.ids, "addresses": w.addr, "went": how}
		var e conditions.APEvent
		switch how {
		case "began":
			e = at("neighbour", "hellos with %s began, both ways", list)
		case "again":
			e = at("neighbour", "hellos with %s go both ways again", list)
		case "one-way":
			e = at("neighbour", "hellos from %s still come, but no longer list this AP", list)
			d["severity"] = "warning"
		case "stopped":
			e = at("neighbour", "hellos with %s stopped", list)
			d["severity"] = "warning"
		}
		out = append(out, with(e, d))
	}
	return out
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// deafChanged is each radio that could scan and can't, or could not and
// can again (2026-10-06, 0120).
func deafChanged(old, cur *rrmState, at func(kind, format string, args ...any) conditions.APEvent,
	with func(conditions.APEvent, map[string]any) conditions.APEvent) []conditions.APEvent {
	was := map[string]rrmDeaf{}
	for _, d := range old.CannotScan {
		was[d.Band] = d
	}
	var out []conditions.APEvent
	for _, d := range cur.CannotScan {
		if _, ok := was[d.Band]; ok {
			delete(was, d.Band)
			continue
		}
		why := "every scan it asks for is refused"
		if d.Why == "dfs" {
			why = "on a DFS channel it must keep listening for radar, and rates its own channel by its own counts"
		}
		out = append(out, with(at("rrm", "%s: can't scan on channel %d: %s", radio.BandName(d.Band), d.Channel, why),
			map[string]any{"band": d.Band, "channel": d.Channel, "why": d.Why}))
	}
	for band, d := range was {
		out = append(out, with(at("rrm", "%s: scans again", radio.BandName(band)), map[string]any{"band": band, "channel": d.Channel}))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Text < out[j].Text })
	return out
}

// verdictWords is what a probe's verdict is called (0059, 0114).
var verdictWords = map[string]string{
	"up": "answers", "down": "is down", "unverified": "is unverified", "silent": "is silent", "off": "stands by", "unknown": "is starting",
}

// verdictsChanged is each of its transports, VXLAN tunnels and VLANs, and
// RADIUS servers, whose prober's verdict changed: a start, as from unknown
// to answering, is no news, nor is any change of a fresh agent's.
func verdictsChanged(old, cur *stateReport, fresh bool, at func(kind, format string, args ...any) conditions.APEvent,
	with func(conditions.APEvent, map[string]any) conditions.APEvent) []conditions.APEvent {
	var out []conditions.APEvent
	say := func(kind, what, was, now string, d map[string]any) {
		if was == now || now == "" || now == "unknown" || (was == "unknown" && now == "up") || fresh {
			return
		}
		if was == "" {
			return
		}
		d["verdict"], d["was"] = now, was
		if now == "down" || now == "silent" {
			d["severity"] = "warning"
		}
		w, ow := verdictWords[now], verdictWords[was]
		if w == "" {
			w = now
		}
		if ow == "" {
			ow = was
		}
		out = append(out, with(at(kind, "%s %s; it %s before", what, w, ow), d))
	}
	verdict := func(p *probeState) string {
		if p == nil {
			return ""
		}
		return p.Verdict
	}
	if old.VXLAN != nil && cur.VXLAN != nil {
		was := map[string]tunnelState{}
		for _, t := range old.VXLAN.Tunnels {
			was[fmt.Sprintf("%d|%s", t.VNI, t.Peer)] = t
		}
		for _, t := range cur.VXLAN.Tunnels {
			o, ok := was[fmt.Sprintf("%d|%s", t.VNI, t.Peer)]
			if !ok {
				continue
			}
			say("tunnel", fmt.Sprintf("the tunnel to %s, VNI %d,", t.Peer, t.VNI), verdict(o.Probe), verdict(t.Probe), map[string]any{"vni": t.VNI, "peer": t.Peer})
		}
	}
	was := map[string]vlanProbe{}
	for _, p := range old.VLANProbes {
		was[fmt.Sprintf("%d|%t", p.VLAN, p.Tagged)] = p
	}
	for _, p := range cur.VLANProbes {
		if o, ok := was[fmt.Sprintf("%d|%t", p.VLAN, p.Tagged)]; ok {
			say("probe", fmt.Sprintf("VLAN %d on the uplink", p.VLAN), verdict(o.Probe), verdict(p.Probe), map[string]any{"vlan": p.VLAN, "tagged": p.Tagged})
		}
	}
	rad := map[string]radiusState{}
	for _, r := range old.Radius {
		rad[r.Network+"|"+r.Server] = r
	}
	for _, r := range cur.Radius {
		if o, ok := rad[r.Network+"|"+r.Server]; ok {
			say("radius", fmt.Sprintf("the RADIUS server %s", r.Server), o.Verdict, r.Verdict, map[string]any{"network": r.Network, "server": r.Server, "port": r.Port})
		}
	}
	return out
}

// hourSaid is an AP's account of the hour that began at hour (0338), kept
// as the first report after it comes in: how many reports it made, of
// those due from its first in the hour, the most clients it had and its
// busiest processor, least memory free and hottest; with, as this report
// has them, how many of its neighbours its hellos go both ways with, how
// many of its tunnels and RADIUS servers answer, and the config it runs.
// An AP that came in the hour is due fewer: a silence before is said as
// one.
func hourSaid(hour time.Time, p conditions.HealthPoint, reports int, first time.Time, cur *stateReport) conditions.APEvent {
	due := int(time.Hour / stateEvery)
	if first.After(hour) {
		due = min(due, int(hour.Add(time.Hour).Sub(first)/stateEvery)+1)
	}
	d := map[string]any{"hour": hour, "reports": reports, "expected": due, "clients": p.Clients, "cpu": p.CPU, "temp": p.Temp,
		"version": cur.Version}
	parts := []string{fmt.Sprintf("%d report%s", reports, plural(reports))}
	if reports < due-1 {
		d["severity"] = "warning"
		parts[0] += fmt.Sprintf(" of the %d due", due)
	}
	if cur.RRM != nil {
		var up, oneWay, down []string
		for _, n := range cur.RRM.Neighbours {
			switch n.State {
			case "up":
				up = append(up, n.AP)
			case "one-way":
				oneWay = append(oneWay, n.AP)
			case "down":
				down = append(down, n.AP)
			}
		}
		d["neighbours"] = map[string]any{"up": up, "one_way": oneWay, "down": down}
		s := fmt.Sprintf("hellos both ways with %d AP%s", len(up), plural(len(up)))
		if len(oneWay)+len(down) > 0 {
			s += fmt.Sprintf(", %d not", len(oneWay)+len(down))
		}
		parts = append(parts, s)
	}
	if cur.VXLAN != nil && len(cur.VXLAN.Tunnels) > 0 {
		ok := 0
		for _, t := range cur.VXLAN.Tunnels {
			if t.Probe != nil && t.Probe.Verdict == "up" {
				ok++
			}
		}
		d["tunnels"] = map[string]int{"answer": ok, "all": len(cur.VXLAN.Tunnels)}
		parts = append(parts, fmt.Sprintf("%d of %d tunnels answer", ok, len(cur.VXLAN.Tunnels)))
	}
	if len(cur.Radius) > 0 {
		ok := 0
		for _, r := range cur.Radius {
			if r.Verdict == "up" {
				ok++
			}
		}
		d["radius"] = map[string]int{"answer": ok, "all": len(cur.Radius)}
		parts = append(parts, fmt.Sprintf("%d of %d RADIUS servers answer", ok, len(cur.Radius)))
	}
	if p.CPU != nil {
		parts = append(parts, fmt.Sprintf("processor at most %d%%", *p.CPU))
	}
	if p.MemTotal != nil && p.MemAvailable != nil && *p.MemTotal > 0 {
		used := int(100 - *p.MemAvailable*100 / *p.MemTotal)
		d["mem"] = used
		parts = append(parts, fmt.Sprintf("memory %d%% in use", used))
	}
	if p.Temp != nil {
		parts = append(parts, fmt.Sprintf("%.0f °C", *p.Temp))
	}
	parts = append(parts, fmt.Sprintf("at most %d client%s", p.Clients, plural(p.Clients)))
	return conditions.APEvent{At: hour.Add(time.Hour), Kind: "hour", Text: "the hour: " + strings.Join(parts, "; "), Detail: mustJSON(d)}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// noteHour keeps an AP's account of its last hour where this report, at
// now, is its first since the hour ended (0338).
func (s *Server) noteHour(ap hierarchy.NodeID, prev *conditions.State, now time.Time, cur *stateReport) error {
	if prev == nil || prev.At.Truncate(time.Hour).Equal(now.Truncate(time.Hour)) {
		return nil
	}
	hour := prev.At.Truncate(time.Hour)
	p, n, first, ok, err := s.conds.HourHealth(ap, hour)
	if err != nil || !ok {
		return err
	}
	return s.conds.AddEvents(ap, []conditions.APEvent{hourSaid(hour, p, n, first, cur)})
}

// plansChanged is RRM's evaluations (0338): each move a radio came to
// plan, and each it no longer plans that it did not claim. A radio's plan
// for another channel, by the same way, is one line. A plan held across
// reports is said once.
func plansChanged(old, cur *rrmState, moved []conditions.APEvent, quiet bool, at func(kind, format string, args ...any) conditions.APEvent,
	with func(conditions.APEvent, map[string]any) conditions.APEvent) []conditions.APEvent {
	type way struct {
		band  string
		radar bool
	}
	was, now := map[way]rrmPlan{}, map[way]rrmPlan{}
	for _, p := range old.Plans {
		was[way{p.Band, p.Radar}] = p
	}
	for _, p := range cur.Plans {
		now[way{p.Band, p.Radar}] = p
	}
	// A band RRM claimed or moved on in this report: its plan's end is the
	// claim's line.
	claimed := map[string]bool{}
	for _, e := range moved {
		var d struct{ Band, State string }
		if json.Unmarshal(e.Detail, &d) == nil && (d.State == "announced" || d.State == "checking" || d.State == "moved") {
			claimed[d.Band] = true
		}
	}
	say := func(p rrmPlan) string {
		band := radio.BandName(p.Band)
		switch {
		case p.Why == "wide":
			return fmt.Sprintf("%s: may widen on channel %d; it does in its move window, once that has held 10 minutes, after a radar check", band, p.To)
		case p.Radar:
			return fmt.Sprintf("%s: DFS channel %d rates clearly best; it moves there from %d in its move window, once that has held 10 minutes, after a radar check", band, p.To, p.From)
		case p.Why == "better":
			return fmt.Sprintf("%s: channel %d rates better than %d; it moves there once that has held 10 minutes, in its move window", band, p.To, p.From)
		case p.Why == "shared":
			return fmt.Sprintf("%s: a neighbour uses channel %d too; it moves to %d once that has held 10 minutes, in its move window", band, p.From, p.To)
		}
		why := moveWhy[p.Why]
		if why == "" {
			why = p.Why
		}
		return fmt.Sprintf("%s: looks to move from channel %d to %d: %s", band, p.From, p.To, why)
	}
	detail := func(p rrmPlan, state string) map[string]any {
		return map[string]any{"band": p.Band, "from": p.From, "to": p.To, "why": p.Why, "radar": p.Radar, "state": state}
	}
	var out []conditions.APEvent
	for k, p := range now {
		o, had := was[k]
		switch {
		case !had:
			out = append(out, with(at("rrm", "%s", say(p)), detail(p, "planned")))
		case o.To != p.To || o.Why != p.Why:
			out = append(out, with(at("rrm", "%s (in place of channel %d)", say(p), o.To), detail(p, "planned")))
		}
	}
	for k, o := range was {
		if _, ok := now[k]; ok || claimed[o.Band] || quiet {
			continue
		}
		out = append(out, with(at("rrm", "%s: no longer looks to move to channel %d: its reason went before it held long enough", radio.BandName(o.Band), o.To),
			detail(o, "dropped")))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Text < out[j].Text })
	return out
}
