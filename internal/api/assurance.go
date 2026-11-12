package api

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/radio"
)

// Service assurance (0123): the service tried from a client's side, on a
// schedule. Where a folder says how often (assurance.tests.every), the
// manager asks each AP there that can make one for a client test that
// often, of the tests the folder asks for (0131): through the air, one with
// a scan radio (0121), each time one network, on one AP, on one band; over
// the wire, any whose agent can (0128), each time one network; turn and
// turn about. What a test finds is kept as any client test's is. A network that
// fails twice running where it is tested, or as many times as its folders or
// the network say (0182), is an alert (0099) on the AP it failed on.

const (
	// assureFloor is the weakest an AP may be heard, in dBm, to be tested
	// from here: weaker, a test fails for the distance and says nothing
	// of the service.
	assureFloor = -75
	// assureRetry is how soon a test that failed for the first time is
	// made again, to tell a network that does not work from one bad try.
	assureRetry = 2 * time.Minute
	// assureWindow is how far back the tests count for what is failing, at
	// the least: further where an AP's round is long, to three rounds of it.
	assureWindow = 24 * time.Hour
	// assureKeep is how long a client test is kept.
	assureKeep = 30 * 24 * time.Hour
	// assureSlack is how much sooner than its wait a test may be asked:
	// the manager looks once a minute, and a look that comes a moment
	// short of the wait would else put the test off a whole minute, every
	// time, so that every 15 minutes were every 16.
	assureSlack = 10 * time.Second
)

// assureCombo is one thing tested: a network, the AP it is joined on, the
// tester itself among them, and a band.
type assureCombo struct {
	Network string `json:"network"`
	On      string `json:"on"`
	Band    string `json:"band"`
}

// testRecord is one client test, asked of an AP: what was asked and, once
// it is done, what it found.
type testRecord struct {
	conditions.Action
	ask    clientTestAsk
	detail *clientTest
}

func readTest(a conditions.Action) testRecord {
	r := testRecord{Action: a}
	json.Unmarshal(a.Params, &r.ask)
	if len(a.Detail) > 0 {
		var d clientTest
		if json.Unmarshal(a.Detail, &d) == nil {
			r.detail = &d
		}
	}
	return r
}

func (r testRecord) combo() assureCombo {
	return assureCombo{Network: r.ask.Network, On: r.ask.On, Band: r.ask.Band}
}

// ran says whether the test was made: the radio taken, and the network
// looked for. One that never had the radio, or that its AP never took up,
// says nothing of the network.
func (r testRecord) ran() bool { return r.detail != nil && r.detail.Client != "" }

func (r testRecord) failed() bool { return r.ran() && r.detail.Outcome != "passed" }

func (r testRecord) open() bool { return r.State == "pending" || r.State == "running" }

// Which tests an AP is asked for (0131): through the air, with its scan
// radio (0121); over the wire, from its uplink (0128); or both.
const (
	overAir  = "air"
	overWire = "wire"
	overBoth = "both"
)

// byAir and byWire say whether tests through the air, and over the wire,
// are among those asked for.
func byAir(over string) bool  { return over != overWire }
func byWire(over string) bool { return over != overAir }

// assureSettings are an AP's, from its folders: how often it is asked for
// a test, none for never; whether a test on a schedule measures speed,
// against the local server and against the remote one (0134); and which
// tests it is asked for (0131): through the air unless said. The setting
// before, tests over the wire too (0128), says both where which is not
// said.
func assureSettings(state *change.State, ap hierarchy.NodeID) (every time.Duration, speed speedWant, over string) {
	over = overAir
	cfg, err := state.Org.ResolveAP(ap)
	if err != nil || cfg.Unassigned {
		return 0, speed, over
	}
	if r, ok := cfg.Location["assurance.tests.every"]; ok {
		switch v := r.Value.(type) {
		case float64:
			every = time.Duration(v) * time.Minute
		case int:
			every = time.Duration(v) * time.Minute
		case json.Number:
			if n, err := v.Int64(); err == nil {
				every = time.Duration(n) * time.Minute
			}
		}
	}
	if r, ok := cfg.Location["assurance.tests.speed"]; ok {
		speed.Local, _ = r.Value.(bool)
	}
	if r, ok := cfg.Location["assurance.tests.speed_remote"]; ok {
		speed.Remote, _ = r.Value.(bool)
	}
	if r, ok := cfg.Location["assurance.tests.wire"]; ok {
		if wire, _ := r.Value.(bool); wire {
			over = overBoth
		}
	}
	if r, ok := cfg.Location["assurance.tests.over"]; ok {
		if v, _ := r.Value.(string); v == overAir || v == overWire || v == overBoth {
			over = v
		}
	}
	return every, speed, over
}

// roamsOn says whether an AP's folders ask its tests through the air to
// roam, once on, as a client walking about would (0225).
func roamsOn(state *change.State, ap hierarchy.NodeID) bool {
	cfg, err := state.Org.ResolveAP(ap)
	if err != nil || cfg.Unassigned {
		return false
	}
	on, _ := cfg.Location["assurance.tests.roam"].Value.(bool)
	return on
}

// roamOf is where a test of a thing roams to once on (0225): each other AP
// of all the tester tests the same network on, on the same band, in the
// order all has them, by the BSSIDs its latest report says are its own; an
// AP that has said none yet is left out. At most roamMost.
func (s *Server) roamOf(c assureCombo, all []assureCombo) (bssids [][]string, on []string) {
	for _, o := range all {
		if o.Network != c.Network || o.Band != c.Band || o.On == c.On || o.Band == wireBand || slices.Contains(on, o.On) {
			continue
		}
		st, err := s.conds.LatestState(hierarchy.NodeID(o.On))
		if err != nil || st == nil {
			continue
		}
		var rep struct {
			BSSIDs []string `json:"bssids"`
		}
		if json.Unmarshal(st.Report, &rep) != nil || len(rep.BSSIDs) == 0 {
			continue
		}
		bssids = append(bssids, rep.BSSIDs[:min(len(rep.BSSIDs), 32)])
		on = append(on, o.On)
		if len(on) == roamMost {
			break
		}
	}
	return bssids, on
}

// tester says whether an AP can be asked for client tests on a schedule,
// of those its folders ask for (0131): through the air, where it has a
// scan radio to test with (0121); over the wire, where its agent can make
// them (0128).
func (s *Server) tester(state *change.State, ap hierarchy.NodeID) bool {
	_, _, over := assureSettings(state, ap)
	return (byAir(over) && scanRadio(state, ap)) || (byWire(over) && s.wireTests(ap))
}

// assureCombos are what an AP tests on its schedule, in the order it takes
// them, of the tests its folders ask for (0131). Through the air, one with
// a scan radio: each network it offers that a test can join, on itself and
// on each AP it hears well enough that offers it too, on each band the
// network is on there. Over the wire, one whose agent can (0128): each
// network it offers, after those. Its folders' controls (0180) narrow them:
// the networks, and the APs joined on and how weak.
func (s *Server) assureCombos(state *change.State, ap hierarchy.NodeID, docs map[hierarchy.NodeID]map[string]any) []assureCombo {
	_, _, over := assureSettings(state, ap)
	return s.combosOf(state, ap, docs, over, assureControls(state, ap))
}

// roundCombos are what an AP tests in a round asked by hand (0129), of the
// tests the round was asked for (0131), both unless it said: all it can of
// them, whatever its folders ask for on a schedule.
func (s *Server) roundCombos(state *change.State, ap hierarchy.NodeID, docs map[hierarchy.NodeID]map[string]any, over string) []assureCombo {
	if over == "" {
		over = overBoth
	}
	return s.combosOf(state, ap, docs, over, anyControls)
}

// combosOf is what an AP can test of the tests asked for: through the air,
// over the wire, or both, as ctl allows.
func (s *Server) combosOf(state *change.State, ap hierarchy.NodeID, docs map[hierarchy.NodeID]map[string]any, over string, ctl controls) []assureCombo {
	var out []assureCombo
	if byAir(over) && scanRadio(state, ap) {
		out = s.airCombos(state, ap, docs, ctl)
	}
	if byWire(over) && s.wireTests(ap) {
		if _, ok := docs[ap]; !ok {
			var nets map[string]any
			if res, err := s.compose(state, ap, s.reveal); err == nil && !res.Unassigned {
				nets, _ = res.Doc["network"].(map[string]any)
			}
			docs[ap] = nets
		}
		ids := make([]string, 0, len(docs[ap]))
		for id := range docs[ap] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			// Not one turned off here (0012), nor one left out (0180).
			if n, _ := docs[ap][id].(map[string]any); n["enabled"] != false && ctl.tests(id) {
				out = append(out, assureCombo{Network: id, On: string(ap), Band: wireBand})
			}
		}
	}
	return out
}

// airCombos are what an AP with a scan radio tests through the air, as ctl
// allows: on the APs it joins on, heard at its floor or better.
func (s *Server) airCombos(state *change.State, ap hierarchy.NodeID, docs map[hierarchy.NodeID]map[string]any, ctl controls) []assureCombo {
	doc := func(id hierarchy.NodeID) map[string]any {
		if d, ok := docs[id]; ok {
			return d
		}
		var nets map[string]any
		if res, err := s.compose(state, id, s.reveal); err == nil && !res.Unassigned {
			nets, _ = res.Doc["network"].(map[string]any)
		}
		docs[id] = nets
		return nets
	}
	// offered says whether a network, as an AP's config has it, is on a band there.
	offered := func(n map[string]any, band string) bool {
		sec, _ := n["security"].(string)
		if _, six := radio.SixGHzEncryption(sec); band == "6g" && !six {
			return false
		}
		list, ok := n["bands"].([]any)
		return !ok || slices.Contains(list, any(band))
	}
	type place struct {
		ap    hierarchy.NodeID
		bands []string
		off   map[string]bool // the networks it has off the air (0201)
	}
	var places []place
	if ctl.joinsOn(ap, ap) {
		places = append(places, place{ap: ap, bands: servingBands(state, ap)})
	}
	if st, err := s.conds.LatestState(ap); err == nil && st != nil && ctl.on != onSelf {
		var rep struct {
			RRM struct {
				Neighbours []rrmNeighbour `json:"neighbours"`
			} `json:"rrm"`
		}
		json.Unmarshal(st.Report, &rep)
		for _, n := range rep.RRM.Neighbours {
			id := hierarchy.NodeID(n.AP)
			if node, ok := state.Org.Locations.Node(id); !ok || node.Kind != hierarchy.KindAP || n.State == "down" || id == ap || !ctl.joinsOn(ap, id) {
				continue
			}
			// Heard at the floor or better both ways: a test client's
			// frames go to the AP as much as the AP's come to it, and an
			// AP across a yard heard at -73 that hears the tester at -78
			// failed its tests at DHCP for the distance, not the service.
			// Where the AP has not said how it hears the tester, as on a
			// neighbour heard one way, its own signal is taken alone.
			p := place{ap: id}
			for _, b := range n.Bands {
				if b.Signal != nil && *b.Signal >= ctl.floor && (b.TheirSignal == nil || *b.TheirSignal >= ctl.floor) {
					p.bands = append(p.bands, b.Band)
				}
			}
			if len(p.bands) > 0 {
				places = append(places, p)
			}
		}
	}
	for i := range places {
		places[i].off = s.offAir(places[i].ap)
	}
	// Itself first, where it joins on itself; the others by ID.
	sort.SliceStable(places, func(i, j int) bool {
		if (places[i].ap == ap) != (places[j].ap == ap) {
			return places[i].ap == ap
		}
		return places[i].ap < places[j].ap
	})
	mine := doc(ap)
	ids := make([]string, 0, len(mine))
	for id := range mine {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []assureCombo
	for _, id := range ids {
		n, _ := mine[id].(map[string]any)
		// Not one turned off here (0012), nor one a test cannot join, nor
		// one left out (0180).
		if n["enabled"] == false || !ctl.tests(id) || cannotTest(state, ap, id, n) != "" {
			continue
		}
		for _, p := range places {
			theirs, _ := doc(p.ap)[id].(map[string]any)
			if theirs == nil || theirs["ssid"] != n["ssid"] || theirs["enabled"] == false || p.off[id] {
				continue
			}
			for _, band := range []string{"2g", "5g", "6g"} {
				if slices.Contains(p.bands, band) && offered(theirs, band) && ctl.onBand(band) {
					out = append(out, assureCombo{Network: id, On: string(p.ap), Band: band})
				}
			}
		}
	}
	return out
}

// pollOf is how often an AP calls in, by its folders: a minute unless set.
func pollOf(state *change.State, ap hierarchy.NodeID) time.Duration {
	if r, ok := state.Org.Locations.Resolve(ap, hierarchy.Path("system.poll")); ok {
		if v, ok := r.Value.(float64); ok && v > 0 {
			return time.Duration(v) * time.Second
		}
	}
	return time.Minute
}

// servingBands are the bands an AP says it has a radio on that serves
// clients: not one another service owns (0081).
func servingBands(state *change.State, ap hierarchy.NodeID) []string {
	var f struct {
		Radios []struct {
			Band     string `json:"band"`
			Reserved bool   `json:"reserved"`
		} `json:"radios"`
	}
	json.Unmarshal(state.Facts[ap], &f)
	var out []string
	for _, r := range f.Radios {
		if !r.Reserved && bands[r.Band] && !slices.Contains(out, r.Band) {
			out = append(out, r.Band)
		}
	}
	return out
}

// assureRounds is how long each AP that is asked for tests takes to come
// back to the same thing, how often it is asked times how many things it
// tests; the things each is asked for in turn; and how far back the tests
// must be read for three rounds of the longest: a day at the least, and no
// further than tests are kept.
func (s *Server) assureRounds(state *change.State, docs map[hierarchy.NodeID]map[string]any) (map[hierarchy.NodeID]time.Duration, map[assureKey]bool, time.Duration) {
	rounds := map[hierarchy.NodeID]time.Duration{}
	inTurn := map[assureKey]bool{}
	window := assureWindow
	for _, ap := range state.Org.Locations.APs() {
		every, _, _ := assureSettings(state, ap)
		if every <= 0 || !s.tester(state, ap) {
			continue
		}
		combos := s.assureCombos(state, ap, docs)
		for _, c := range combos {
			inTurn[assureKey{tester: ap, combo: c}] = true
		}
		rounds[ap] = every * time.Duration(max(1, len(combos)))
		window = max(window, min(3*rounds[ap]+time.Hour, assureKeep))
	}
	return rounds, inTurn, window
}

// verdictStands says whether what a thing's latest test came to is said
// still: for three rounds of its AP's schedule and an hour after that
// test, or for an hour where the AP is asked for none. After that nothing
// is testing it, and what it last came to is old news.
func verdictStands(now, last time.Time, round time.Duration) bool {
	return now.Sub(last) <= 3*round+time.Hour
}

// turnOf is the round a thing's verdict stands for: its tester's, where
// the thing is among what that AP is asked for in turn; none where it is
// not, as where the AP no longer hears the AP it joined on, so a failure
// nothing tests again ends an hour after its last test, not three of the
// tester's rounds after, which may be a day and more.
func turnOf(rounds map[hierarchy.NodeID]time.Duration, inTurn map[assureKey]bool, k assureKey) time.Duration {
	if !inTurn[k] {
		return 0
	}
	return rounds[k.tester]
}

// assureVerdict is what the tests of one thing, by one AP, come to.
type assureVerdict struct {
	last    *testRecord // the latest that was made
	failing bool        // the latest made failed, as many running as make an alert (0182)
	since   time.Time   // when the first of the failures running was asked
	passed  int         // of those asked since the time counted from
	failed  int
}

// assureKey is one thing tested by one AP.
type assureKey struct {
	tester hierarchy.NodeID
	combo  assureCombo
}

// assureVerdicts reads the tests of a time, newest first, into what each
// thing tested by each AP comes to. One failure is not yet a network that
// does not work; two running are, or as many as the tester's folders, or
// the network, say (0182): after, where it is not nil. Passes and failures
// are counted from a time on, which may be later than the tests go back.
func assureVerdicts(list []testRecord, from time.Time, after func(hierarchy.NodeID, assureCombo) int) map[assureKey]*assureVerdict {
	out := map[assureKey]*assureVerdict{}
	run := map[assureKey]int{}    // the failures running, from the latest back
	ended := map[assureKey]bool{} // a test that passed has ended them
	for i := range list {
		r := list[i]
		if !r.ran() {
			continue
		}
		k := assureKey{tester: hierarchy.NodeID(r.AP), combo: r.combo()}
		v := out[k]
		if v == nil {
			v = &assureVerdict{last: &list[i]}
			out[k] = v
		}
		if !r.failed() {
			if !r.At.Before(from) {
				v.passed++
			}
			ended[k] = true
			continue
		}
		if !r.At.Before(from) {
			v.failed++
		}
		if !ended[k] {
			run[k]++
			v.since = r.At
		}
	}
	for k, v := range out {
		need := 2
		if after != nil {
			need = after(k.tester, k.combo)
		}
		if v.failing = run[k] >= need; !v.failing {
			v.since = time.Time{}
		}
	}
	return out
}

// tests are the client tests of a time, newest first, read.
func (s *Server) tests(since time.Time) ([]testRecord, error) {
	heads, err := s.conds.ClientTestHeads(since)
	if err != nil {
		return nil, err
	}
	return s.read.tests(heads, s.conds.ActionsByID, time.Now())
}

// assure asks each AP for the client test it is due, and returns what it
// asked. An AP is due when its folders say how often, it can make a test
// of those they ask for (0131), it has called in lately and runs its
// config, and no test of it is open.
// What it tests next is the thing it tested longest ago; but a thing that
// failed for the first time is tested again within minutes, and one that
// is failing every other time, so its mending is seen soon.
func (s *Server) assure(now time.Time) []conditions.Action {
	state := s.log.Snapshot()
	if state == nil {
		return nil
	}
	docs := map[hierarchy.NodeID]map[string]any{}
	_, _, window := s.assureRounds(state, docs)
	list, err := s.tests(now.Add(-window))
	if err != nil {
		slog.Warn("assurance: cannot read the tests", "err", err)
		return nil
	}
	verdicts := assureVerdicts(list, time.Time{}, newThresholds(state).after)
	var asked []conditions.Action
	loads := map[hierarchy.NodeID]*load{}
	for _, ap := range state.Org.Locations.APs() {
		every, speed, _ := assureSettings(state, ap)
		// Paused (0180), or out of its window (0181): nothing on the
		// schedule now, which is kept for after.
		ctl := assureControls(state, ap)
		// Nor while the AP is in a test window (0344).
		scheduled := every > 0 && !ctl.paused && ctl.open(now) && s.tester(state, ap) && !heldAt(state, ap, now)
		// A round asked by hand (0129) is worked at whatever the schedule.
		airRound, wireRound, inRound := s.rounds.of(state.Org.Locations, ap, now)
		if !scheduled && !inRound {
			continue
		}
		l, err := s.conds.Latest(ap)
		version, _ := s.log.Version(ap)
		if err != nil || l.Seen == nil || now.Sub(l.Seen.At) > alerts.Offline(pollOf(state, ap)) || l.Seen.Running == nil || *l.Seen.Running != version {
			continue
		}
		var last *testRecord // the latest the manager asked itself
		open := false
		for i := range list {
			r := &list[i]
			if hierarchy.NodeID(r.AP) != ap {
				continue
			}
			open = open || r.open()
			if last == nil && r.Actor == change.SystemActor {
				last = r
			}
		}
		// Not while one is open.
		if open {
			continue
		}
		// When each thing was last asked for by the manager: the one asked
		// longest ago comes first, and one never asked before any.
		at := map[assureCombo]time.Time{}
		for i := range list {
			r := &list[i]
			if hierarchy.NodeID(r.AP) == ap && r.Actor == change.SystemActor {
				if _, ok := at[r.combo()]; !ok {
					at[r.combo()] = r.At
				}
			}
		}
		// The first of an order that can be asked: one that cannot, as its
		// AP has not said its BSSIDs yet, does not hold up the others. Where
		// its folders say, a test through the air roams, once on, to the
		// other APs of all it tests the network on, on that band (0225).
		roams := roamsOn(state, ap)
		pick := func(order, all []assureCombo) (clientTestAsk, *assureCombo) {
			for i := range order {
				c := order[i]
				ask, err := s.clientTestOf(state, ap, c.Network, c.On, c.Band, speed, func(hierarchy.NodeID) bool { return true })
				if err == nil {
					if roams && c.Band != wireBand {
						ask.Roam, ask.RoamOn = s.roamOf(c, all)
					}
					return ask, &c
				}
				slog.Debug("assurance: a test cannot be asked", "ap", ap, "network", c.Network, "on", c.On, "band", c.Band, "err", err)
			}
			return clientTestAsk{}, nil
		}
		// What a round asked by hand (0129) has not had asked since comes
		// before anything on the schedule, and waits for no turn.
		var ask clientTestAsk
		var next *assureCombo
		if inRound {
			var due []assureCombo
			all := s.roundCombos(state, ap, docs, roundsOver(airRound, wireRound))
			for _, c := range all {
				if roundKeeps(airRound, wireRound, c) && at[c].Before(roundSince(airRound, wireRound, c)) {
					due = append(due, c)
				}
			}
			if ask, next = pick(due, all); next != nil {
				ask.Round = true
			}
		}
		// Nothing of a round to ask, or nothing of it that can be asked
		// now: the schedule's turn, which a round does not hold up. On the
		// schedule, no sooner than any wait could end.
		if next == nil {
			if !scheduled || (last != nil && now.Sub(last.At) < min(every, assureRetry)-assureSlack) {
				continue
			}
			// What its radios' load puts off waits for a look when it is
			// lighter (0180); the next thing in turn goes first.
			var ready []assureCombo
			all := s.assureCombos(state, ap, docs)
			for _, c := range scheduledOrder(ap, slices.Clone(all), verdicts, last, at, every, now) {
				why := s.putOff(ap, c, ctl, loads)
				// An AP in a test window (0344) is not tested from another.
				if why == "" && heldAt(state, hierarchy.NodeID(c.On), now) {
					why = "the AP it joins on is in a test window"
				}
				// The scan radio's budget for the hour spent (0182): a test
				// over the wire uses none.
				if why == "" && c.Band != wireBand && ctl.spent(list, ap, now) {
					why = "its scan radio's tests this hour are per_hour already"
				}
				if why != "" {
					slog.Debug("assurance: a test is put off", "ap", ap, "network", c.Network, "on", c.On, "band", c.Band, "why", why)
					continue
				}
				ready = append(ready, c)
			}
			ask, next = pick(ready, all)
		}
		if next == nil {
			continue
		}
		params, err := json.Marshal(ask)
		if err != nil {
			continue
		}
		a, err := s.conds.AddActionWith(ap, "client-test", "", change.SystemActor, params)
		if err != nil {
			slog.Warn("assurance: a test was not asked", "ap", ap, "err", err)
			continue
		}
		slog.Info("assurance: test asked", "ap", ap, "network", next.Network, "on", next.On, "band", next.Band, "id", a.ID)
		asked = append(asked, a)
	}
	return asked
}

// scheduledOrder is what an AP on a schedule is asked next, the likeliest
// first: nothing where nothing is due yet. combos are what it tests; last,
// the latest test the manager asked of it; at, when each thing was last
// asked for.
func scheduledOrder(ap hierarchy.NodeID, combos []assureCombo, verdicts map[assureKey]*assureVerdict, last *testRecord,
	at map[assureCombo]time.Time, every time.Duration, now time.Time) []assureCombo {
	if len(combos) == 0 {
		return nil
	}
	oldest := func(of []assureCombo) []assureCombo {
		sort.SliceStable(of, func(i, j int) bool { return at[of[i]].Before(at[of[j]]) })
		return of
	}
	var failing, rest []assureCombo
	for _, c := range combos {
		if v := verdicts[assureKey{tester: ap, combo: c}]; v != nil && v.failing {
			failing = append(failing, c)
		} else {
			rest = append(rest, c)
		}
	}
	// What to ask, the likeliest first. A thing that failed for the first
	// time is asked again, soon; else a failing thing every other time, and
	// the rest in their turn.
	var order []assureCombo
	wait := every
	if last != nil {
		v := verdicts[assureKey{tester: ap, combo: last.combo()}]
		if last.failed() && v != nil && !v.failing && slices.Contains(combos, last.combo()) {
			wait = min(every, assureRetry)
			order = []assureCombo{last.combo()}
		}
		if now.Sub(last.At) < wait-assureSlack {
			return nil
		}
	}
	if wasFailing := last != nil && slices.Contains(failing, last.combo()); len(failing) > 0 && !wasFailing {
		return append(append(order, oldest(failing)...), oldest(rest)...)
	}
	return append(append(order, oldest(rest)...), oldest(failing)...)
}

// AssureAt asks what is due at a time, once, and says how many tests it
// asked: for whoever drives the schedule by hand.
func (s *Server) AssureAt(now time.Time) int { return len(s.assure(now)) }

// Assure asks the client tests the folders want on a schedule (0123),
// looking every interval until ctx ends, and forgets tests older than a
// month.
func (s *Server) Assure(ctx context.Context, every time.Duration) {
	// Once ever (0141).
	s.countKeptTests()
	pruned := time.Time{}
	look := func() {
		now := time.Now()
		s.assure(now)
		if now.Sub(pruned) > time.Hour {
			pruned = now
			if n, err := s.conds.PruneClientTests(now.Add(-assureKeep)); err != nil {
				slog.Warn("assurance: old tests not forgotten", "err", err)
			} else if n > 0 {
				slog.Info("assurance: old tests forgotten", "tests", n)
			}
			// Power control's steps are kept a month (0206), forgotten here
			// with the tests, once an hour.
			if _, err := s.conds.PruneAPCEvents(now.Add(-apcKeep)); err != nil {
				slog.Warn("power control: old steps not forgotten", "err", err)
			}
			// What each minute came to (0207): the radios' a month, the
			// clients' a week.
			if _, _, err := s.conds.PruneMinutes(now.Add(-radioMinutesKeep), now.Add(-clientMinutesKeep)); err != nil {
				slog.Warn("old minutes not forgotten", "err", err)
			}
			// The uplinks' minutes as long as the radios' (0351).
			if _, err := s.conds.PruneUplinkMinutes(now.Add(-radioMinutesKeep)); err != nil {
				slog.Warn("old uplink minutes not forgotten", "err", err)
			}
			// The flight recorder's recordings: a month (0207 part 3).
			if _, err := s.conds.PruneRecordings(now.Add(-recordingsKeep)); err != nil {
				slog.Warn("old recordings not forgotten", "err", err)
			}
		}
	}
	look()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			look()
		}
	}
}

// withinReach is list without the tests through the air that failed with
// the AP they joined heard below their tester's floor: such a test failed
// for the distance and says nothing of the service (0180). It happens
// where power control has just turned that AP down (0077) and the tester's
// smoothed reading of it has not caught up, so the tester asked for it at
// a strength it no longer has. They are no evidence for an alert; the
// assurance page still shows them.
func withinReach(state *change.State, list []testRecord) []testRecord {
	floors := map[hierarchy.NodeID]int{}
	out := make([]testRecord, 0, len(list))
	for _, r := range list {
		if r.failed() && r.ask.Band != wireBand {
			if sig := r.detail.signal(); sig != nil {
				tester := hierarchy.NodeID(r.AP)
				f, ok := floors[tester]
				if !ok {
					f = assureControls(state, tester).floor
					floors[tester] = f
				}
				if *sig < float64(f) {
					continue
				}
			}
		}
		out = append(out, r)
	}
	return out
}

// failingTests are the things failing now, by the AP each fails on, for
// its alerts (0099): the network, where, and what the latest test said. A
// verdict stands while its AP is still asked for tests of that thing, or
// for an hour after the last of them.
func (s *Server) failingTests(state *change.State, now time.Time) map[hierarchy.NodeID][]alerts.FailingTest {
	out := map[hierarchy.NodeID][]alerts.FailingTest{}
	rounds, inTurn, window := s.assureRounds(state, map[hierarchy.NodeID]map[string]any{})
	list, err := s.tests(now.Add(-window))
	if err != nil {
		return out
	}
	list = withinReach(state, list)
	t := state.Org.Locations
	var owners map[string]hierarchy.NodeID // whose each BSSID is, read once if it is needed
	off := map[hierarchy.NodeID]map[string]bool{}
	for k, v := range assureVerdicts(list, time.Time{}, newThresholds(state).after) {
		if !v.failing {
			continue
		}
		tester, ok := t.Node(k.tester)
		if !ok {
			continue
		}
		if !verdictStands(now, v.last.At, turnOf(rounds, inTurn, k)) {
			continue
		}
		on := hierarchy.NodeID(k.combo.On)
		if k.combo.On == "" {
			// The strongest AP it heard: the one whose BSSID it joined, or
			// tried to. One that is no AP's here is nobody's alert.
			if owners == nil {
				owners = s.fleetBSSIDs(state)
			}
			if on, ok = owners[v.last.detail.BSSID]; !ok {
				continue
			}
		} else if _, ok := t.Node(on); !ok {
			continue
		}
		// One through the air of a network that AP has off the air (0201):
		// the off-air alert says why it was not found.
		if k.combo.Band != wireBand {
			if _, ok := off[on]; !ok {
				off[on] = s.offAir(on)
			}
			if off[on][k.combo.Network] {
				continue
			}
		}
		since := v.since
		out[on] = append(out[on], alerts.FailingTest{
			Network: k.combo.Network, SSID: v.last.ask.SSID, Band: k.combo.Band, Tester: tester.Name, Itself: on == k.tester,
			Signal: v.last.detail.signal(), Stage: v.last.detail.Stage, Reason: v.last.detail.Reason, Since: &since,
		})
	}
	for _, list := range out {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Network != list[j].Network {
				return list[i].Network < list[j].Network
			}
			return list[i].Band < list[j].Band
		})
	}
	return out
}

// assuranceView is one AP that tests, for a person.
type assuranceTester struct {
	AP      hierarchy.NodeID `json:"ap"`
	Name    string           `json:"name"`
	Every   int              `json:"every"` // minutes; 0 where it is not asked on a schedule
	Speed   bool             `json:"speed"`
	Remote  bool             `json:"speed_remote"`     // those tests measure against the remote speed server too (0134)
	Things  int              `json:"things"`           // how many things it tests in turn
	Air     bool             `json:"air"`              // it has a scan radio, to test through the air with
	Wire    bool             `json:"wire"`             // it is asked for tests over the wire, and can make them (0128)
	CanWire bool             `json:"can_wire"`         // its agent can test over the wire, asked on a schedule or not
	Round   int              `json:"round"`            // how many things it tests in a round asked by hand, of both (0129)
	Over    string           `json:"over"`             // which tests its folders ask for on a schedule: air, wire or both (0131)
	Paused  bool             `json:"paused"`           // its folders pause its schedule, which is kept (0180)
	Bands   []string         `json:"bands,omitempty"`  // the bands its tests through the air join on, where its folders say (0181)
	Window  string           `json:"window,omitempty"` // when its tests on a schedule are asked, in its local time (0181)
	// Its blocks of time, which take the window's place, and when a run of
	// tests asked for a while ends (0188).
	Windows []string   `json:"windows,omitempty"`
	Until   *time.Time `json:"until,omitempty"`
	// Whether a test on a schedule may be asked now: not paused, in its
	// window or a block, and its run not over (0188).
	Running bool `json:"running"`
}

// assuranceCheck is one thing tested by one AP, and what its tests of the
// time come to.
type assuranceCheck struct {
	Tester     hierarchy.NodeID   `json:"tester"`
	TesterName string             `json:"tester_name"`
	Network    string             `json:"network"`
	SSID       string             `json:"ssid,omitempty"`
	On         string             `json:"on"`
	OnName     string             `json:"on_name"`
	Band       string             `json:"band,omitempty"`
	Scheduled  bool               `json:"scheduled"` // among what the AP is asked for in turn now
	Failing    bool               `json:"failing"`
	Since      *time.Time         `json:"since,omitempty"`
	Passed     int                `json:"passed"`
	Failed     int                `json:"failed"`
	Last       *conditions.Action `json:"last,omitempty"`
	Runs       []assuranceRun     `json:"runs,omitempty"`
	// Way is its tests' way online, a point at a time (0130): how many
	// passed each, how long it typically took them, and how many stopped
	// at it, over the tests of the time asked.
	Way []wayTypical `json:"way,omitempty"`
}

// briefChecks is the checks as the Overview needs them (0285): each with
// its counts and whether it fails, and its last test in brief; not its
// runs or its way online.
func briefChecks(checks []assuranceCheck) []assuranceCheck {
	out := make([]assuranceCheck, len(checks))
	for i, ch := range checks {
		ch.Runs, ch.Way = nil, nil
		out[i] = ch
	}
	return out
}

// briefTest is a client test as a line of a list says it (0287): how it
// came out, where it stopped and why, and what it was of; not its steps,
// its way online or what it found, which the line fetches when it is
// opened (GET /v1/aps/{ap}/actions/{id}).
func briefTest(a conditions.Action) conditions.Action {
	var d struct {
		Outcome string `json:"outcome,omitempty"`
		Stage   string `json:"stage,omitempty"`
		Reason  string `json:"reason,omitempty"`
		TookMS  int64  `json:"took_ms,omitempty"`
	}
	var p struct {
		Network string `json:"network,omitempty"`
		On      string `json:"on,omitempty"`
		SSID    string `json:"ssid,omitempty"`
		Band    string `json:"band,omitempty"`
		Round   bool   `json:"round,omitempty"`
	}
	if len(a.Detail) > 0 && json.Unmarshal(a.Detail, &d) == nil {
		a.Detail, _ = json.Marshal(d)
	} else {
		a.Detail = nil
	}
	if len(a.Params) > 0 && json.Unmarshal(a.Params, &p) == nil {
		a.Params, _ = json.Marshal(p)
	} else {
		a.Params = nil
	}
	return a
}

// assuranceRun is one test of a thing in brief, for how it did over the
// time asked: when, whether it passed, where it stopped if not, and how
// long each step took, in ms, with its speed in Mbit/s where measured.
type assuranceRun struct {
	At        time.Time `json:"at"`
	Passed    bool      `json:"passed"`
	Stage     string    `json:"stage,omitempty"`
	JoinedMS  int64     `json:"joined_ms,omitempty"`
	DHCPMS    int64     `json:"dhcp_ms,omitempty"`
	GatewayMS *float64  `json:"gateway_ms,omitempty"`
	DNSMS     *int64    `json:"dns_ms,omitempty"`
	Down      *float64  `json:"down,omitempty"`
	Up        *float64  `json:"up,omitempty"`
	// The same against the remote speed server (0134); and that it was
	// asked for and could not be measured, which fails no test.
	RemoteDown   *float64 `json:"remote_down,omitempty"`
	RemoteUp     *float64 `json:"remote_up,omitempty"`
	RemoteFailed bool     `json:"remote_failed,omitempty"`
}

// assuranceRunsMost is how many tests of a thing are given in brief.
const assuranceRunsMost = 48

// runOf is a test in brief.
func runOf(r testRecord) assuranceRun {
	d := r.detail
	out := assuranceRun{At: r.At, Passed: d.Outcome == "passed", JoinedMS: d.JoinedMS}
	if !out.Passed {
		out.Stage = d.Stage
	}
	if d.DHCP != nil {
		out.DHCPMS = d.DHCP.MS
	}
	if d.Gateway != nil {
		// Its pings, or over the wire (0128) its ARP answer.
		if out.GatewayMS = d.Gateway.Avg; out.GatewayMS == nil {
			out.GatewayMS = d.Gateway.ARPMS
		}
	}
	if d.DNS != nil {
		out.DNSMS = d.DNS.MS
	}
	if d.Speed != nil {
		out.Down, out.Up = d.Speed.Down, d.Speed.Up
	}
	if d.SpeedRemote != nil {
		out.RemoteDown, out.RemoteUp = d.SpeedRemote.Down, d.SpeedRemote.Up
		// Of a test that passed: one that failed before it has its own reason.
		out.RemoteFailed = out.Passed && d.SpeedRemote.Error != ""
	}
	return out
}

// assurance is the client tests of the APs the caller may view, below one
// Locations node where ?under= names it, over ?hours= (24 unless set, at
// most 720): GET /v1/assurance. testers are the APs that can test, through the air or over the wire (0128);
// checks, each thing one of them is asked for in turn, or was, or that is
// failing, with what its tests come to and the latest 48 of them in brief
// (runs), the failing first; tests, the latest 50 of them, by hand or on
// the schedule, newest first; joins, how the clients' own attempts to come
// online went over the time asked, by network (0127).
func (s *Server) assurance(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	hours := 24
	// The Overview asks only how the tests stand (0285); the Assurance tab
	// asks for each test in brief, and fetches one in full when it opens it
	// (0287). Unasked, each is given in full, as a tool reads it.
	brief := r.URL.Query().Get("brief") == "1"
	lean := brief || r.URL.Query().Get("tests") == "brief"
	shown := withWay
	if lean {
		shown = briefTest
	}
	if h := r.URL.Query().Get("hours"); h != "" {
		n, err := strconv.Atoi(h)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours: 1 to 720")
		}
		hours = n
	}
	now := time.Now()
	from := now.Add(-time.Duration(hours) * time.Hour)
	docs := map[hierarchy.NodeID]map[string]any{}
	// What is failing is worked out as an alert is, over three rounds of
	// the schedule; the counts and the tests listed are of the time asked.
	turns, inTurn, window := s.assureRounds(c.state, docs)
	list, err := s.tests(now.Add(-max(window, now.Sub(from))))
	if err != nil {
		return err
	}
	// An AP the caller may not view is shown neither as a tester nor as
	// where a test joined: not its name, nor its BSSIDs.
	views := func(id string) bool {
		return id == "" || may(c, change.Locations, t, hierarchy.NodeID(id), access.View)
	}
	view := map[hierarchy.NodeID]bool{}
	testers := []assuranceTester{}
	checks := []assuranceCheck{}
	name := func(id hierarchy.NodeID) string {
		if n, ok := t.Node(id); ok {
			return n.Name
		}
		return string(id)
	}
	seen := map[assureKey]bool{}
	verdicts := assureVerdicts(list, from, newThresholds(c.state).after)
	// Each thing's latest tests of the time asked, in brief: read once, as
	// the tests are many.
	runs := map[assureKey][]assuranceRun{}
	ways := map[assureKey]*wayTally{}
	for _, r := range list {
		if r.At.Before(from) || !r.ran() {
			continue
		}
		k := assureKey{tester: hierarchy.NodeID(r.AP), combo: r.combo()}
		if len(runs[k]) < assuranceRunsMost {
			runs[k] = append(runs[k], runOf(r))
		}
		if ways[k] == nil {
			kind := wayAir
			if r.detail.Band == wireBand {
				kind = wayWire
			}
			ways[k] = newWayTally(kind)
		}
		ways[k].add(testWay(r.detail))
	}
	// The round asked by hand that the APs shown are in (0129), the latest
	// where they are in more than one, and how far it has got. It is of the
	// tests those APs are asked for in it (0131).
	var inRound *roundView
	roundAir, roundWire := false, false
	for _, id := range t.APs() {
		// An AP that can test: on a schedule, or only when asked, as one
		// with no scan radio whose folders ask for no tests over the wire.
		canWire := s.wireTests(id)
		if !may(c, change.Locations, t, id, access.View) || !(scanRadio(c.state, id) || canWire) {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		view[id] = true
		every, speed, over := assureSettings(c.state, id)
		if !s.tester(c.state, id) {
			every = 0
		}
		ctl := assureControls(c.state, id)
		combos := s.assureCombos(c.state, id, docs)
		could := s.roundCombos(c.state, id, docs, overBoth)
		things := len(could)
		if every > 0 {
			things = len(combos)
		}
		var until *time.Time
		if ctl.until > 0 {
			u := time.Unix(ctl.until, 0).UTC()
			until = &u
		}
		testers = append(testers, assuranceTester{AP: id, Name: name(id), Every: int(every / time.Minute), Speed: speed.Local, Remote: speed.Remote, Things: things, Paused: ctl.paused, Bands: ctl.bands, Window: ctl.window,
			Windows: ctl.windows, Until: until, Running: every > 0 && !ctl.paused && ctl.open(now),
			Air: scanRadio(c.state, id), Wire: byWire(over) && canWire, CanWire: canWire, Round: len(could), Over: over})
		// An AP is in a round where the round asks something of it: one
		// through the air says nothing of an AP with no scan radio. The
		// round said of it is the one asked last of those that do: where,
		// when and by whom are that one's. Two kinds asked at one node are
		// one round there.
		airRound, wireRound, ok := s.rounds.of(t, id, now)
		var mine []assureCombo
		if ok {
			for _, cb := range s.roundCombos(c.state, id, docs, roundsOver(airRound, wireRound)) {
				if roundKeeps(airRound, wireRound, cb) {
					mine = append(mine, cb)
				}
			}
		}
		var one round
		hasAir, hasWire := false, false
		for _, cb := range mine {
			if cb.Band == wireBand {
				hasWire = true
			} else {
				hasAir = true
			}
		}
		if hasAir {
			one = airRound
		}
		if hasWire && wireRound.At.After(one.At) {
			one = wireRound
		}
		if len(mine) > 0 {
			if inRound == nil || one.At.After(inRound.At) {
				// A round asked at a folder above is said to have been
				// asked, and when: its folder's name and who asked it only
				// to one who may view that folder, as the tree names it to
				// no other.
				inRound = &roundView{round: round{Node: one.Node, At: one.At, Filter: one.Filter}}
				roundAir, roundWire = false, false
				if may(c, change.Locations, t, one.Node, access.View) {
					inRound.Name, inRound.By = name(one.Node), one.By
				}
			}
			if one.At.Equal(inRound.At) && one.Node == inRound.Node {
				// What this AP tests in it: of the tests the round asked for.
				could := mine
				inRound.Of += len(could)
				roundAir, roundWire = roundAir || hasAir, roundWire || hasWire
				// Each thing once, by the first test of it since its kind
				// was asked for, which is the round's: the tests come newest
				// first, and the schedule may have come to the thing again.
				first := map[assureCombo]*testRecord{}
				for i := range list {
					r := &list[i]
					if hierarchy.NodeID(r.AP) == id && r.Actor == change.SystemActor && slices.Contains(could, r.combo()) && !r.At.Before(roundSince(airRound, wireRound, r.combo())) {
						first[r.combo()] = r
					}
				}
				for _, r := range first {
					inRound.Asked++
					if !r.open() {
						inRound.Done++
					}
				}
			}
		}
		ssid := func(network string) string {
			n, _ := docs[id][network].(map[string]any)
			v, _ := n["ssid"].(string)
			return v
		}
		add := func(k assureKey, scheduled bool) {
			if seen[k] || !views(k.combo.On) {
				return
			}
			seen[k] = true
			ch := assuranceCheck{Tester: id, TesterName: name(id), Network: k.combo.Network, SSID: ssid(k.combo.Network), On: k.combo.On,
				OnName: name(hierarchy.NodeID(k.combo.On)), Band: k.combo.Band, Scheduled: scheduled}
			if k.combo.On == "" {
				ch.OnName = ""
			}
			if v := verdicts[k]; v != nil {
				// Failing is said for as long as its alert is (0123): of a
				// thing no longer tested, not for as long as its last tests
				// are in the time shown.
				ch.Failing, ch.Passed, ch.Failed = v.failing && verdictStands(now, v.last.At, turnOf(turns, inTurn, k)), v.passed, v.failed
				if ch.Failing {
					since := v.since
					ch.Since = &since
				}
				last := shown(v.last.Action)
				ch.Last = &last
				if w := ways[k]; w != nil {
					ch.Way = w.typical()
				}
				if ch.SSID == "" {
					ch.SSID = v.last.ask.SSID
				}
				// Its tests of the time asked, in brief, the oldest first.
				ch.Runs = slices.Clone(runs[k])
				sort.SliceStable(ch.Runs, func(i, j int) bool { return ch.Runs[i].At.Before(ch.Runs[j].At) })
			}
			checks = append(checks, ch)
		}
		// What it is asked for in turn; and what it was, or that fails,
		// though it is asked for no longer. A thing a person tried once by
		// hand is among the tests, not here.
		if every > 0 {
			for _, combo := range combos {
				add(assureKey{tester: id, combo: combo}, true)
			}
		}
		for k, v := range verdicts {
			if k.tester == id && (v.failing || v.last.Actor == change.SystemActor) {
				add(k, false)
			}
		}
	}
	sort.SliceStable(checks, func(i, j int) bool {
		a, b := checks[i], checks[j]
		if a.Failing != b.Failing {
			return a.Failing
		}
		if a.TesterName != b.TesterName {
			return a.TesterName < b.TesterName
		}
		if a.SSID != b.SSID {
			return a.SSID < b.SSID
		}
		if a.OnName != b.OnName {
			return a.OnName < b.OnName
		}
		return a.Band < b.Band
	})
	// The Overview asks only how the tests stand (0285): not the tests
	// themselves, the clients' joins or the tests' roams, which are most of
	// the work and of the answer.
	if brief {
		writeJSON(w, http.StatusOK, map[string]any{"testers": testers, "checks": briefChecks(checks), "hours": hours})
		return nil
	}
	tests := []conditions.Action{}
	for _, r := range list {
		if view[hierarchy.NodeID(r.AP)] && views(r.ask.On) && !r.At.Before(from) && len(tests) < 50 {
			tests = append(tests, shown(r.Action))
		}
	}
	// And how the clients' own attempts to come online went (0127).
	joins, err := s.joinsView(c, under, from, now)
	if err != nil {
		return err
	}
	if inRound != nil {
		inRound.Over = overAir
		if roundWire {
			inRound.Over = overWire
			if roundAir {
				inRound.Over = overBoth
			}
		}
	}
	visible := func(r *testRecord) bool {
		if !view[hierarchy.NodeID(r.AP)] || !views(r.ask.On) {
			return false
		}
		for _, id := range r.ask.RoamOn {
			if !views(id) {
				return false
			}
		}
		return true
	}
	measured, err := s.roamsMeasured(c, list, from, visible)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"testers": testers, "checks": checks, "tests": tests, "joins": joins, "hours": hours, "round": inRound,
		"roams": testRoams(list, from, visible, measured)})
	return nil
}

// roamTestSlack is how far from a test's own time the attempt an AP
// recorded of one of its roams may be: the AP's clock, and the test's
// waiting for the scan radio.
const roamTestSlack = time.Minute

// roamsMeasured matches each roam a test made to the attempt the AP it went
// to recorded of the test's client (0225), as the roaming page reads the
// clients' own (0136): how that AP says the client came by its keys, and
// how long from its first authentication there until it was let on. That
// is the roam's own time: the test's own, from its side, takes in the scan
// radio going to the other channel, a third of a second on a C-360's. The
// attempt is on the test's network: the same client tests one network
// after another, a minute or so apart. It gives, for a roam, the kind and
// ms; or none, where the AP has recorded nothing of it, or nothing timed.
func (s *Server) roamsMeasured(c call, list []testRecord, from time.Time, shown func(*testRecord) bool) (func(*testRecord, testRoam) (string, *int64), error) {
	var macs []string
	for i := range list {
		r := &list[i]
		if !r.At.Before(from) && r.ran() && r.detail != nil && len(r.detail.Roams) > 0 && r.detail.Client != "" && shown(r) && !slices.Contains(macs, r.detail.Client) {
			macs = append(macs, r.detail.Client)
		}
	}
	none := func(*testRecord, testRoam) (string, *int64) { return "", nil }
	if len(macs) == 0 {
		return none, nil
	}
	conns, err := s.conds.ConnectionsOfClients(macs, from.Add(-roamTestSlack), roamingMost)
	if err != nil {
		return nil, err
	}
	roams, err := s.roamsAmong(c, conns)
	if err != nil {
		return nil, err
	}
	owners := s.fleetBSSIDs(c.state)
	used := map[int64]bool{}
	return func(r *testRecord, h testRoam) (string, *int64) {
		ap, ok := owners[h.BSSID]
		if !ok {
			return "", nil
		}
		end := r.At.Add(15 * time.Minute)
		if r.DoneAt != nil {
			end = *r.DoneAt
		}
		for _, x := range roams {
			if x.MAC != r.detail.Client || x.AP != ap || x.Network != r.ask.Network || used[x.ID] || x.MS == nil || x.At.Before(r.At.Add(-roamTestSlack)) || x.At.After(end.Add(roamTestSlack)) {
				continue
			}
			used[x.ID] = true
			ms := int64(*x.MS)
			return x.Kind, &ms
		}
		return "", nil
	}, nil
}

// assuranceRoams is what the tests' roams came to (0225), a network and a
// band at a time, of the time asked: how many roams, how many were not let
// on, and each kind's count and typical, least and most ms, as the roaming
// page says the clients' own (0136).
type assuranceRoams struct {
	Network string              `json:"network"`
	SSID    string              `json:"ssid"`
	Band    string              `json:"band"`
	Roams   int                 `json:"roams"`
	Failed  int                 `json:"failed"`
	Kinds   []assuranceRoamKind `json:"kinds"`
}

// assuranceRoamKind is one kind's roams: how many, how many of them the AP
// gone to could not time, and the typical, least and most ms of the rest,
// none where it timed none.
type assuranceRoamKind struct {
	Kind      string `json:"kind"`
	Roams     int    `json:"roams"`
	Untimed   int    `json:"untimed,omitempty"`
	TypicalMS *int64 `json:"typical_ms,omitempty"`
	LowMS     *int64 `json:"low_ms,omitempty"`
	HighMS    *int64 `json:"high_ms,omitempty"`
}

// testRoams is assuranceRoams of the tests since from that shown says the
// caller may see, the network's name first: each roam as measured says the
// AP it went to recorded it. One that AP did not time is counted, of the
// kind the test says, and left out of the times: the test's own clock takes
// in the scan radio going to the other channel, a third of a second. An AP
// times no 802.11r roam on a network that signs clients in by 802.1X, as
// it says nothing of their authentication there (0133). A key held,
// whether the AP says by OKC or PMKSA caching, is one kind, cached.
func testRoams(list []testRecord, from time.Time, shown func(*testRecord) bool, measured func(*testRecord, testRoam) (string, *int64)) []assuranceRoams {
	type key struct{ network, band string }
	got := map[key]*assuranceRoams{}
	ms := map[key]map[string][]int64{}
	untimed := map[key]map[string]int{}
	for i := range list {
		r := &list[i]
		if r.At.Before(from) || !r.ran() || r.detail == nil || len(r.detail.Roams) == 0 || !shown(r) {
			continue
		}
		for _, h := range r.detail.Roams {
			k := key{r.ask.Network, cmp.Or(h.Band, r.detail.Band)}
			if got[k] == nil {
				got[k] = &assuranceRoams{Network: k.network, SSID: r.ask.SSID, Band: k.band, Kinds: []assuranceRoamKind{}}
				ms[k], untimed[k] = map[string][]int64{}, map[string]int{}
			}
			got[k].Roams++
			if !h.OK || h.MS == nil {
				got[k].Failed++
				continue
			}
			kind, took := measured(r, h)
			if took == nil {
				kind = h.Kind
			}
			if kind == roamOKC || kind == roamPMKSA {
				kind = roamCached
			}
			if took == nil {
				untimed[k][kind]++
				continue
			}
			ms[k][kind] = append(ms[k][kind], *took)
		}
	}
	out := []assuranceRoams{}
	for k, v := range got {
		for _, kind := range []string{roamFT, roamCached, roamKeys, roamFull} {
			times, n := ms[k][kind], untimed[k][kind]
			if len(times)+n == 0 {
				continue
			}
			one := assuranceRoamKind{Kind: kind, Roams: len(times) + n, Untimed: n}
			if len(times) > 0 {
				slices.Sort(times)
				one.TypicalMS, one.LowMS, one.HighMS = &times[(len(times)-1)/2], &times[0], &times[len(times)-1]
			}
			v.Kinds = append(v.Kinds, one)
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SSID != out[j].SSID {
			return out[i].SSID < out[j].SSID
		}
		return out[i].Band < out[j].Band
	})
	return out
}
