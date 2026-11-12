package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // an AP's zone, whatever the manager's host has (0181)

	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// controls are what an AP's folders say of its tests on a schedule (0180):
// whether they are paused; the networks they join and leave out; which APs
// a tester joins on, and how weak it may hear them; and the load that puts
// a test through the air off. And the bands they join on, and when they are
// asked (0181). Tests and rounds asked by hand take none of them.
type controls struct {
	paused bool
	bands  []string // only these; nil, any
	window string   // HH:MM-HH:MM in the AP's local time; "", any time
	// Blocks of time, each on some days or every day (0188), which take the
	// window's place where set; and when a run of tests asked for a while
	// ends, in Unix seconds, 0 for none (0188).
	windows    []string
	until      int64
	tz         string   // the AP's zone, for the window: UTC unless set
	networks   []string // only these; nil, any
	leaveOut   []string
	on         string // self, heard or listed
	listed     []hierarchy.NodeID
	floor      int
	maxClients int // -1: however many
	maxBusy    int // 0: however busy
	// The scan radio's budget (0182): the most tests through the air an
	// hour; 0, as many as the schedule asks.
	perHour int
	// How many failures running make an alert (0182), and each network's
	// own, which comes first.
	alertAfter int
	netAlert   map[string]int
}

const (
	onSelf   = "self"
	onHeard  = "heard"
	onListed = "listed"
)

// anyControls are a round's, asked by hand, and a schedule's where nothing
// is set: every network, on itself and on the APs it hears at assureFloor
// or better, whatever the load.
var anyControls = controls{on: onHeard, floor: assureFloor, maxClients: -1, alertAfter: 2}

// assureControls are an AP's controls, from its folders (0180).
func assureControls(state *change.State, ap hierarchy.NodeID) controls {
	c := anyControls
	cfg, err := state.Org.ResolveAP(ap)
	if err != nil || cfg.Unassigned {
		return c
	}
	get := func(f string) (any, bool) {
		r, ok := cfg.Location[hierarchy.Path("assurance.tests."+f)]
		return r.Value, ok
	}
	if v, ok := get("paused"); ok {
		c.paused, _ = v.(bool)
	}
	if v, ok := get("bands"); ok {
		c.bands = textList(v)
	}
	if v, ok := get("window"); ok {
		c.window, _ = v.(string)
	}
	if v, ok := get("windows"); ok {
		c.windows = textList(v)
	}
	if v, ok := get("until"); ok {
		if n := number(v); n != nil {
			c.until = int64(*n)
		}
	}
	if r, ok := cfg.Location["system.tz"]; ok {
		c.tz, _ = r.Value.(string)
	}
	if v, ok := get("networks"); ok {
		c.networks = textList(v)
		if c.networks == nil {
			c.networks = []string{}
		}
	}
	if v, ok := get("leave_out"); ok {
		c.leaveOut = textList(v)
	}
	if v, ok := get("on"); ok {
		if s, _ := v.(string); s == onSelf || s == onHeard || s == onListed {
			c.on = s
		}
	}
	if v, ok := get("listed"); ok {
		for _, s := range textList(v) {
			c.listed = append(c.listed, hierarchy.NodeID(s))
		}
	}
	if v, ok := get("floor"); ok {
		if n := number(v); n != nil {
			c.floor = int(*n)
		}
	}
	if v, ok := get("max_clients"); ok {
		if n := number(v); n != nil {
			c.maxClients = int(*n)
		}
	}
	if v, ok := get("max_busy"); ok {
		if n := number(v); n != nil {
			c.maxBusy = int(*n)
		}
	}
	if v, ok := get("per_hour"); ok {
		if n := number(v); n != nil {
			c.perHour = int(*n)
		}
	}
	if v, ok := get("alert_after"); ok {
		if n := number(v); n != nil && *n >= 1 {
			c.alertAfter = int(*n)
		}
	}
	for id, n := range cfg.Networks {
		if r, ok := n.Fields[change.AlertAfter]; ok {
			if v := number(r.Value); v != nil && *v >= 1 {
				if c.netAlert == nil {
					c.netAlert = map[string]int{}
				}
				c.netAlert[id] = int(*v)
			}
		}
	}
	return c
}

// failsAfter is how many failures running of a network make an alert
// (0182): the network's own number, else the tester's folders', else two.
func (c controls) failsAfter(network string) int {
	if n, ok := c.netAlert[network]; ok {
		return n
	}
	if c.alertAfter >= 1 {
		return c.alertAfter
	}
	return 2
}

// thresholds is each tester's failsAfter, read once a look (0182).
type thresholds struct {
	state *change.State
	of    map[hierarchy.NodeID]controls
}

func newThresholds(state *change.State) *thresholds {
	return &thresholds{state: state, of: map[hierarchy.NodeID]controls{}}
}

// after is how many failures running of combo, tested by tester, make an
// alert.
func (t *thresholds) after(tester hierarchy.NodeID, combo assureCombo) int {
	c, ok := t.of[tester]
	if !ok {
		c = assureControls(t.state, tester)
		t.of[tester] = c
	}
	return c.failsAfter(combo.Network)
}

// spent says whether a tester has made as many tests through the air in the
// hour before now as its budget allows (0182): those asked by hand count,
// as they took its scan radio too.
func (c controls) spent(list []testRecord, tester hierarchy.NodeID, now time.Time) bool {
	if c.perHour <= 0 {
		return false
	}
	n := 0
	for _, r := range list {
		if hierarchy.NodeID(r.AP) == tester && r.combo().Band != wireBand && now.Sub(r.At) < time.Hour {
			n++
		}
	}
	return n >= c.perHour
}

// textList is a setting's list of strings, as JSON or Go holds one.
func textList(v any) []string {
	var out []string
	switch x := v.(type) {
	case []string:
		out = append(out, x...)
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case json.RawMessage:
		_ = json.Unmarshal(x, &out)
	}
	return out
}

// onBand says whether the tests join on band (0181).
func (c controls) onBand(band string) bool {
	return c.bands == nil || slices.Contains(c.bands, band)
}

// open says whether a test on a schedule may be asked at now, by the AP's
// own clock: not once a run asked for a while has ended (0188); in one of
// its blocks of time where it has them (0188), else in its window (0181).
func (c controls) open(now time.Time) bool {
	if c.until > 0 && now.Unix() >= c.until {
		return false
	}
	if len(c.windows) > 0 {
		for _, b := range c.windows {
			if blockOpen(b, c.tz, now) {
				return true
			}
		}
		return false
	}
	return windowOpen(c.window, c.tz, now)
}

// weekdays are the days a block of time names, as the schema spells them.
var weekdays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// onDays says whether a day of the week is among those a block names:
// "mon-fri", "sat,sun", "mon,wed,fri", a range past Saturday such as
// "fri-mon"; every day where it names none.
func onDays(days string, d time.Weekday) bool {
	if days == "" {
		return true
	}
	for _, part := range strings.Split(days, ",") {
		from, to, ranged := strings.Cut(part, "-")
		a, b := slices.Index(weekdays, from), slices.Index(weekdays, from)
		if ranged {
			b = slices.Index(weekdays, to)
		}
		if a < 0 || b < 0 {
			continue
		}
		for i := a; ; i = (i + 1) % 7 {
			if i == int(d) {
				return true
			}
			if i == b {
				break
			}
		}
	}
	return false
}

// blockOpen says whether now is in a block of time (0188): its days, if it
// names any, then HH:MM-HH:MM, in zone tz. A block past midnight belongs to
// the day it starts on: "fri 22:00-02:00" runs into Saturday morning.
func blockOpen(block, tz string, now time.Time) bool {
	days, window := "", block
	if i := strings.LastIndex(block, " "); i >= 0 {
		days, window = block[:i], block[i+1:]
	}
	var h1, m1, h2, m2 int
	if n, err := fmt.Sscanf(window, "%d:%d-%d:%d", &h1, &m1, &h2, &m2); err != nil || n != 4 {
		return false
	}
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	t := now.In(loc)
	at, from, to := t.Hour()*60+t.Minute(), h1*60+m1, h2*60+m2
	switch {
	case from == to:
		return onDays(days, t.Weekday())
	case from < to:
		return at >= from && at < to && onDays(days, t.Weekday())
	case at >= from:
		return onDays(days, t.Weekday())
	case at < to:
		return onDays(days, t.AddDate(0, 0, -1).Weekday())
	}
	return false
}

// windowOpen says whether now is in a window, HH:MM-HH:MM in zone tz (UTC
// where it is unset or unknown): the whole day where none is set, or where
// it starts where it ends; past midnight where it ends before it starts.
// One that does not read is no window, as the schema keeps it from being
// set (0181, 0183).
func windowOpen(window, tz string, now time.Time) bool {
	var h1, m1, h2, m2 int
	if window == "" {
		return true
	}
	if n, err := fmt.Sscanf(window, "%d:%d-%d:%d", &h1, &m1, &h2, &m2); err != nil || n != 4 {
		return true
	}
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	t := now.In(loc)
	at, from, to := t.Hour()*60+t.Minute(), h1*60+m1, h2*60+m2
	switch {
	case from == to:
		return true
	case from < to:
		return at >= from && at < to
	}
	return at >= from || at < to
}

// tests says whether the tests join network id.
func (c controls) tests(id string) bool {
	return (c.networks == nil || slices.Contains(c.networks, id)) && !slices.Contains(c.leaveOut, id)
}

// joinsOn says whether a tester joins networks on AP on through the air:
// itself, unless only listed APs are and it is not one; another AP it hears
// at the floor or better, unless it joins only itself, or only listed APs
// and that one is not.
func (c controls) joinsOn(tester, on hierarchy.NodeID) bool {
	switch c.on {
	case onSelf:
		return on == tester
	case onListed:
		return slices.Contains(c.listed, on)
	}
	return true
}

// load is what an AP last said of its radios' load (0180): its clients on
// each band, and how busy others kept the channel its radio is on.
type load struct {
	clients map[string]int
	busy    map[string]int
}

// loadOf is an AP's load by its latest state report; nil where it has
// none.
func (s *Server) loadOf(ap hierarchy.NodeID) *load {
	st, err := s.conds.LatestState(ap)
	if err != nil || st == nil {
		return nil
	}
	var rep struct {
		Radios []struct {
			Band    string `json:"band"`
			Clients int    `json:"clients"`
		} `json:"radios"`
		RRM struct {
			Ratings []struct {
				Band string `json:"band"`
				Busy int    `json:"busy"`
				Own  bool   `json:"own"`
			} `json:"ratings"`
		} `json:"rrm"`
	}
	if json.Unmarshal(st.Report, &rep) != nil {
		return nil
	}
	l := &load{clients: map[string]int{}, busy: map[string]int{}}
	for _, r := range rep.Radios {
		l.clients[r.Band] += r.Clients
	}
	for _, r := range rep.RRM.Ratings {
		if r.Own {
			l.busy[r.Band] = max(l.busy[r.Band], r.Busy)
		}
	}
	return l
}

// putOff says why a test through the air waits for now (0180): the
// tester's radio, or the joined AP's, on the band carries more clients than
// max_clients, or the channel it joins on is busier than max_busy, by their
// latest reports. "" where nothing puts it off; a test over the wire never
// waits, as it uses no radio. loads keeps what was read in one look.
func (s *Server) putOff(tester hierarchy.NodeID, c assureCombo, ctl controls, loads map[hierarchy.NodeID]*load) string {
	if c.Band == wireBand || (ctl.maxClients < 0 && ctl.maxBusy == 0) {
		return ""
	}
	of := func(ap hierarchy.NodeID) *load {
		if l, ok := loads[ap]; ok {
			return l
		}
		loads[ap] = s.loadOf(ap)
		return loads[ap]
	}
	on := hierarchy.NodeID(c.On)
	for _, ap := range []hierarchy.NodeID{tester, on} {
		if l := of(ap); ctl.maxClients >= 0 && l != nil && l.clients[c.Band] > ctl.maxClients {
			return "its radio has more clients than max_clients"
		}
	}
	if l := of(on); ctl.maxBusy > 0 && l != nil && l.busy[c.Band] > ctl.maxBusy {
		return "its channel is busier than max_busy"
	}
	return ""
}
