package api

import (
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Steering's history (0139). An AP's steering says what it asked of
// clients and what came of it (0138): a count since it started, and the
// last fifty askings. The count starts again whenever the steering does,
// as at every update of the agent, so it cannot say whether steering works
// over a day or a month. The manager keeps each asking as it comes in a
// state report, once, and counts them itself over the time asked for.

const (
	// steeringLatest is how many askings are listed, the latest.
	steeringLatest = 100
	// steeringClients is how many clients are listed, those asked most.
	steeringClients = 20
	// steerClock is how far an AP's clock may be from the manager's, as
	// with an attempt (0118).
	steerClock = 10 * time.Minute
	// steerHeard is how long after an asking the manager still takes its
	// first word of it. An AP says an asking again in every report until it
	// has fifty newer: one said to be older than this is one already kept,
	// or one kept and since let go with its time, and is not kept anew.
	steerHeard = 7 * 24 * time.Hour
	// steeringWeighed is how many askings, the latest, are weighed by where
	// their clients went (0291): each is a look or two by an index.
	steeringWeighed = 300
	// steerAfterWithin is how long after an asking's end where its client
	// was let on next still says what came of it.
	steerAfterWithin = 5 * time.Minute
)

// What came of an asking, by where its client was let on next (0291).
// steerAfterOf is the outcomes after which its client may have gone
// elsewhere, and so is looked for: told to go, held off a band, or come
// back. One that stayed, said no, or was heard well again went nowhere.
var steerAfterOf = map[string]bool{"moved": true, "left": true, "back": true, "joined": true}

// steerIsAsking is whether a record is an asking, as the store counts them
// (conditions' steerAsking): not the record that says a client came back
// down after a suggestion or a hold, nor the one a sticky client that came
// back within ten minutes is said in, asking nothing (0289). Each says what
// became of an asking whose own where-it-went already finds it, and is not
// followed again.
func steerIsAsking(e conditions.SteerEvent) bool {
	return !(e.Outcome == "back" && (e.Method != "sticky" || e.Asked == "none"))
}

// steerResults tell what a client steered came to, in order, the first
// that is so: back where it was asked from; on another band, whose signal
// is not set against the one it left (bands apart); or on the same band
// elsewhere, by how much more strongly it was heard there, where both are
// known. A kind added here is counted and shown as the rest are.
var steerResults = []struct {
	name string
	is   func(e conditions.SteerEvent, a conditions.SteerAfter) bool
}{
	{"back", func(e conditions.SteerEvent, a conditions.SteerAfter) bool { return a.AP == e.AP && a.Band == e.From }},
	{"band", func(e conditions.SteerEvent, a conditions.SteerAfter) bool { return a.Band != e.From }},
	{"unknown", func(e conditions.SteerEvent, a conditions.SteerAfter) bool { return e.Signal == nil || a.Signal == nil }},
	{"stronger", func(e conditions.SteerEvent, a conditions.SteerAfter) bool {
		return *a.Signal-*e.Signal >= steerBetterBy
	}},
	{"weaker", func(e conditions.SteerEvent, a conditions.SteerAfter) bool {
		return *e.Signal-*a.Signal >= steerBetterBy
	}},
	{"alike", func(e conditions.SteerEvent, a conditions.SteerAfter) bool { return true }},
}

// steerBetterBy is how many dB more or less strongly a client is heard
// where it went for it to be heard better or worse there.
const steerBetterBy = 3

// steerResult is what an asking came to, by where its client went.
func steerResult(e conditions.SteerEvent, a conditions.SteerAfter) string {
	for _, r := range steerResults {
		if r.is(e, a) {
			return r.name
		}
	}
	return ""
}

// askings is the askings of a report as the manager keeps them. Each has
// the time its AP said. An AP's steering holds its askings in memory, so
// none it says is from before it last started, and one said to be from
// before the AP itself did, or from a time yet to come, was said by a clock
// not yet set: it is of this run, and is taken to be of now. Such a clock
// says the same times again the next time its AP starts, so that asking is
// kept as of this run of the AP: of when it started, by the manager's
// clock. uptime is how long the AP says it has been up; where it does not
// say, no asking is taken to be of now on the first ground, and one taken
// to be of now on the other is of no run that can be told.
func (s *steerState) askings(now time.Time, uptime int64) []conditions.SteerEvent {
	if s == nil {
		return nil
	}
	var out []conditions.SteerEvent
	for _, e := range s.Events {
		said := time.Unix(e.At, 0)
		at, run := said, int64(0)
		switch {
		case said.After(now.Add(steerClock)), uptime > 0 && said.Before(now.Add(-time.Duration(uptime)*time.Second-steerClock)):
			at = now
			if uptime > 0 {
				run = now.Unix() - uptime
			}
		case said.Before(now.Add(-steerHeard)):
			continue
		}
		c := conditions.SteerEvent{
			At: at, Said: e.At, Run: run, MAC: strings.ToLower(e.MAC), Network: e.Network, Method: e.Method, From: e.From, To: e.To,
			Asked: e.Asked, Answer: e.Answer, Code: e.Code, Signal: e.Signal, Outcome: e.Outcome, TookMS: int(math.Round(e.Took * 1000)),
			Why: e.Why,
		}
		if e.Elsewhere != nil {
			c.Better, c.BetterS = e.Elsewhere.AP, e.Elsewhere.Signal
		}
		out = append(out, c)
	}
	return out
}

// steeringView is what steering came to over a time: GET /v1/steering.
type steeringView struct {
	Hours    int               `json:"hours"`
	Networks []steeringNetwork `json:"networks"`
	Clients  []steeringClient  `json:"clients"`
	Latest   []steeringAsking  `json:"latest"`
	// Weighed is how many askings, the latest, results counts (0291).
	Weighed int `json:"weighed"`
}

// steeringNetwork is one network's askings: for each way, how many were
// made (asked) and how many came to each thing, as an AP's own count has
// them (0138); and the same an AP at a time.
type steeringNetwork struct {
	Network string                    `json:"network"`
	Counts  map[string]map[string]int `json:"counts"`
	// Results is, of the askings weighed, how many came to each of
	// steerResults, by where their clients went (0291).
	Results map[string]int `json:"results,omitempty"`
	APs     []steeringAP   `json:"aps"`
}

type steeringAP struct {
	AP      hierarchy.NodeID          `json:"ap"`
	Name    string                    `json:"name"`
	Counts  map[string]map[string]int `json:"counts"`
	Results map[string]int            `json:"results,omitempty"`
}

// steeringClient is what was asked of one client: how many times, how
// many of those it moved for, how many it said no to, how many times it
// came back, and when the last was.
type steeringClient struct {
	MAC     string    `json:"mac"`
	Host    string    `json:"host,omitempty"`
	Asked   int       `json:"asked"`
	Moved   int       `json:"moved"`
	Refused int       `json:"refused"`
	Back    int       `json:"back"`
	Last    time.Time `json:"last"`
}

// steeringAsking is one asking, as its AP said it (steerEvent), with when
// the manager takes it to have been and the names of its AP and client;
// for sticky, why, and the AP that heard the client better (0289).
type steeringAsking struct {
	At      time.Time        `json:"at"`
	AP      hierarchy.NodeID `json:"ap"`
	APName  string           `json:"ap_name"`
	MAC     string           `json:"mac"`
	Host    string           `json:"host,omitempty"`
	Network string           `json:"network"`
	Method  string           `json:"method"`
	From    string           `json:"from"`
	To      []string         `json:"to,omitempty"`
	Asked   string           `json:"asked"`
	Answer  string           `json:"answer,omitempty"`
	Code    *int             `json:"code,omitempty"`
	Signal  *int             `json:"signal,omitempty"`
	Outcome string           `json:"outcome"`
	Took    float64          `json:"took"`
	Why     string           `json:"why,omitempty"`
	// Elsewhere is the AP that heard the client more strongly, by name,
	// and how strongly, in dBm.
	Elsewhere *steeringElsewhere `json:"elsewhere,omitempty"`
	// After is where the client was let on next, and Result what that
	// came to (0291).
	After  *steeringAfter `json:"after,omitempty"`
	Result string         `json:"result,omitempty"`
}

// steeringAfter is where a client was let on next after an asking: the AP,
// by ID and name where the caller may view it; its band; how strongly it
// was heard there, in dBm, where that is known; and when.
type steeringAfter struct {
	AP     hierarchy.NodeID `json:"ap,omitempty"`
	Name   string           `json:"ap_name,omitempty"`
	Band   string           `json:"band"`
	Signal *int             `json:"signal,omitempty"`
	At     time.Time        `json:"at"`
}

type steeringElsewhere struct {
	AP     hierarchy.NodeID `json:"ap"`
	Name   string           `json:"name"`
	Signal *int             `json:"signal,omitempty"`
}

// steering is what the steering of the APs the caller may view asked of
// clients and what came of it, below one Locations node where ?under=
// names it, over ?hours= (24 unless set, at most 720): GET /v1/steering.
func (s *Server) steering(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	hours := 24
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours: from 1 to 720")
		}
		hours = n
	}
	var tests []string
	for mac := range testClients(c.state) {
		tests = append(tests, mac)
	}
	aps := mayView(c, hierarchy.NodeID(q.Get("under")))
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	counts, err := s.conds.SteerCounts(since, aps, tests)
	if err != nil {
		return err
	}
	clients, err := s.conds.SteerClients(since, aps, tests, steeringClients)
	if err != nil {
		return err
	}
	weighed, err := s.conds.SteerSince(since, aps, tests, steeringWeighed)
	if err != nil {
		return err
	}
	latest := weighed[:min(len(weighed), steeringLatest)]
	var macs []string
	for _, cl := range clients {
		macs = append(macs, cl.MAC)
	}
	for _, e := range latest {
		macs = append(macs, e.MAC)
	}
	slices.Sort(macs)
	hosts, err := s.conds.Hosts(slices.Compact(macs), aps)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	name := func(ap hierarchy.NodeID) string {
		if n, ok := t.Node(ap); ok {
			return n.Name
		}
		return string(ap)
	}
	viewable := map[hierarchy.NodeID]bool{}
	for _, ap := range mayView(c, "") {
		viewable[ap] = true
	}
	view := steeringView{Hours: hours, Networks: []steeringNetwork{}, Clients: []steeringClient{}, Latest: []steeringAsking{}, Weighed: len(weighed)}
	nets := map[string]*steeringNetwork{}
	on := map[string]map[hierarchy.NodeID]*steeringAP{}
	add := func(to map[string]map[string]int, k conditions.SteerCount) {
		if to[k.Method] == nil {
			to[k.Method] = map[string]int{}
		}
		to[k.Method][k.Outcome] += k.N
	}
	for _, k := range counts {
		n := nets[k.Network]
		if n == nil {
			n = &steeringNetwork{Network: k.Network, Counts: map[string]map[string]int{}}
			nets[k.Network], on[k.Network] = n, map[hierarchy.NodeID]*steeringAP{}
		}
		a := on[k.Network][k.AP]
		if a == nil {
			a = &steeringAP{AP: k.AP, Name: name(k.AP), Counts: map[string]map[string]int{}}
			on[k.Network][k.AP] = a
		}
		add(n.Counts, k)
		add(a.Counts, k)
	}
	// Where each asking's client went, and what that came to (0291),
	// counted for its network and AP.
	afters := make([]*conditions.SteerAfter, len(weighed))
	results := make([]string, len(weighed))
	for i, e := range weighed {
		if !steerAfterOf[e.Outcome] || !steerIsAsking(e) {
			continue
		}
		a, ok, err := s.conds.SteerAfter(e.MAC, e.At, e.At.Add(time.Duration(e.TookMS)*time.Millisecond+steerAfterWithin))
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		afters[i], results[i] = &a, steerResult(e, a)
		count := func(m *map[string]int) {
			if *m == nil {
				*m = map[string]int{}
			}
			(*m)[results[i]]++
		}
		if n := nets[e.Network]; n != nil {
			count(&n.Results)
			if ap := on[e.Network][e.AP]; ap != nil {
				count(&ap.Results)
			}
		}
	}
	for id, n := range nets {
		for _, a := range on[id] {
			n.APs = append(n.APs, *a)
		}
		slices.SortFunc(n.APs, func(a, b steeringAP) int {
			if c := strings.Compare(a.Name, b.Name); c != 0 {
				return c
			}
			return strings.Compare(string(a.AP), string(b.AP))
		})
		view.Networks = append(view.Networks, *n)
	}
	slices.SortFunc(view.Networks, func(a, b steeringNetwork) int { return strings.Compare(a.Network, b.Network) })
	for _, cl := range clients {
		view.Clients = append(view.Clients, steeringClient{MAC: cl.MAC, Host: hosts[cl.MAC], Asked: cl.Asked, Moved: cl.Moved, Refused: cl.Refused, Back: cl.Back, Last: cl.Last})
	}
	for i, e := range latest {
		a := steeringAsking{
			At: e.At, AP: e.AP, APName: name(e.AP), MAC: e.MAC, Host: hosts[e.MAC], Network: e.Network, Method: e.Method, From: e.From, To: e.To,
			Asked: e.Asked, Answer: e.Answer, Code: e.Code, Signal: e.Signal, Outcome: e.Outcome, Took: float64(e.TookMS) / 1000, Why: e.Why,
		}
		// The AP that heard it better, where the caller may view it.
		if b := hierarchy.NodeID(e.Better); b != "" && viewable[b] {
			a.Elsewhere = &steeringElsewhere{AP: b, Name: name(b), Signal: e.BetterS}
		}
		// Where it went, the AP named where the caller may view it.
		if x := afters[i]; x != nil {
			a.After, a.Result = &steeringAfter{Band: x.Band, Signal: x.Signal, At: x.At}, results[i]
			if viewable[x.AP] {
				a.After.AP, a.After.Name = x.AP, name(x.AP)
			}
		}
		view.Latest = append(view.Latest, a)
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}
