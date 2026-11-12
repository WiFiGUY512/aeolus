package api

import (
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A round of tests, asked now (0129): every AP below a Locations node
// tests each thing it can, once, as soon as it can, whatever its schedule
// or none. It is what someone asks who has just built a site, or changed
// it. The manager asks the tests as it asks those on a schedule (0123),
// one of an AP at a time; what a round has not had asked since comes
// before the schedule's turn.

// roundLife is how long a round is worked at. What is left of it then, as
// of an AP that did not call in, is let go: an AP that comes back a day
// later is not asked for a round nobody is waiting for.
const roundLife = time.Hour

// round is one asked: at which node, when last, by the manager's clock,
// and by whom; and the tests it is of (0131), through the air, over the
// wire, or both, with when each kind was asked for. A round of one kind
// asked where one of the other is still worked at leaves that one going:
// the round is then of both, each from its own time.
type round struct {
	Node hierarchy.NodeID `json:"node"`
	At   time.Time        `json:"at"`
	By   string           `json:"by,omitempty"`
	Over string           `json:"over"`
	// What it is narrowed to (0188), of the kind it is said of: those of
	// a round of both are each kind's own.
	Filter roundFilter `json:"filter"`
	// When its tests through the air, and over the wire, were asked for:
	// the zero time for a kind it is not of; and what each kind was
	// narrowed to.
	air, wire   time.Time
	airF, wireF roundFilter
}

// roundFilter narrows a round to a troublesome few (Griff, 2026-10-21:
// "maybe I dont need to test everything, but instead a single troublesome
// group", 0188): the networks it tests, the bands its tests through the
// air join on, and the APs it joins on, the APs it tests; each, where
// empty, any.
type roundFilter struct {
	Networks []string           `json:"networks,omitempty"`
	Bands    []string           `json:"bands,omitempty"`
	On       []hierarchy.NodeID `json:"on,omitempty"`
	// Only these things, each a network joined on an AP on a band: what
	// failed, asked again (0129).
	Only []assureCombo `json:"only,omitempty"`
}

// keeps says whether a round so narrowed tests a thing. A test over the
// wire has no band, and is on the AP that makes it.
func (f roundFilter) keeps(c assureCombo) bool {
	return (len(f.Only) == 0 || slices.Contains(f.Only, c)) &&
		(len(f.Networks) == 0 || slices.Contains(f.Networks, c.Network)) &&
		(len(f.Bands) == 0 || c.Band == wireBand || slices.Contains(f.Bands, c.Band)) &&
		(len(f.On) == 0 || slices.Contains(f.On, hierarchy.NodeID(c.On)))
}

// roundLive says whether a kind asked for at a time is worked at still.
func roundLive(at, now time.Time) bool { return !at.IsZero() && now.Sub(at) <= roundLife }

// roundOver is the tests a round is of, by when each kind was asked for.
func roundOver(air, wire time.Time) string {
	switch {
	case !air.IsZero() && !wire.IsZero():
		return overBoth
	case !wire.IsZero():
		return overWire
	}
	return overAir
}

// rounds are those being worked at, the latest asked at each node. They
// are kept in memory: a manager that restarts forgets them, and whoever
// asked asks again.
type rounds struct {
	mu sync.Mutex
	at map[hierarchy.NodeID]round
}

// ask notes a round asked at a node now, for the tests over says, narrowed
// as f says (0188). It takes the place of one asked there before for those
// tests; one asked there for the others, and worked at still, goes on from
// its own time, as narrowed then.
func (r *rounds) ask(node hierarchy.NodeID, by string, now time.Time, over string, f roundFilter) round {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.at == nil {
		r.at = map[hierarchy.NodeID]round{}
	}
	for id, old := range r.at {
		if now.Sub(old.At) > roundLife {
			delete(r.at, id)
		}
	}
	old := r.at[node]
	one := round{Node: node, At: now, By: by, Filter: f}
	if byAir(over) {
		one.air, one.airF = now, f
	} else if roundLive(old.air, now) {
		one.air, one.airF = old.air, old.airF
	}
	if byWire(over) {
		one.wire, one.wireF = now, f
	} else if roundLive(old.wire, now) {
		one.wire, one.wireF = old.wire, old.wireF
	}
	one.Over = roundOver(one.air, one.wire)
	r.at[node] = one
	return one
}

// of is the rounds an AP is in, one of each kind of test: through the air,
// and over the wire, each the latest asked for at the AP or at a folder
// above it that is worked at still. Each is the round asked at its node,
// where, when last and by whom, of that kind alone, from when that kind
// was asked for there. One the AP is not in is the zero round; ok says it
// is in either. So an AP below a round over the wire, asked for one through
// the air itself, is in both, each from its own time.
func (r *rounds) of(t *hierarchy.Tree, ap hierarchy.NodeID, now time.Time) (air, wire round, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.at) == 0 {
		return air, wire, false
	}
	for _, id := range append(t.Ancestry(ap), ap) {
		one, found := r.at[id]
		if !found {
			continue
		}
		if roundLive(one.air, now) && one.air.After(air.air) {
			air = round{Node: one.Node, At: one.At, By: one.By, Over: overAir, Filter: one.airF, air: one.air, airF: one.airF}
		}
		if roundLive(one.wire, now) && one.wire.After(wire.wire) {
			wire = round{Node: one.Node, At: one.At, By: one.By, Over: overWire, Filter: one.wireF, wire: one.wire, wireF: one.wireF}
		}
	}
	return air, wire, !air.air.IsZero() || !wire.wire.IsZero()
}

// roundsOver is the tests the two rounds an AP is in are of.
func roundsOver(air, wire round) string { return roundOver(air.air, wire.wire) }

// roundKeeps says whether the round of a thing's kind, of the two an AP is
// in, tests it, as that round was narrowed (0188).
func roundKeeps(air, wire round, c assureCombo) bool {
	if c.Band == wireBand {
		return wire.wireF.keeps(c)
	}
	return air.airF.keeps(c)
}

// roundSince is when a round's tests of a thing's kind were asked for, of
// the two an AP is in: a test of the thing made since then is the round's.
func roundSince(air, wire round, c assureCombo) time.Time {
	if c.Band == wireBand {
		return wire.wire
	}
	return air.air
}

// roundView is a round as a person sees it: of how many things, how many
// have been asked for since, and how many of those are done. Its folder's
// name, and who asked it, are for one who may view that folder.
type roundView struct {
	round
	Name  string `json:"name,omitempty"` // its node's, for one who may view that node
	Of    int    `json:"of"`
	Asked int    `json:"asked"`
	Done  int    `json:"done"`
}

// askRound asks every AP below a Locations node for a round of tests, now
// (0129): POST /v1/assurance/round, {under, over?, networks?, bands?, on?,
// only?}. over says which tests (0131): air, wire, or both, which it is
// unless said. networks, bands and on narrow it (0188): only those
// networks, those bands through the air, and joined on those APs. only
// narrows it to those things alone, each {network, on, band}: what failed,
// asked again (0129). It needs operator on the node. It is refused where
// no AP there can make a test of those.
func (s *Server) askRound(w http.ResponseWriter, r *http.Request, c call) error {
	var body struct {
		Under    string        `json:"under"`
		Over     string        `json:"over"`
		Networks []string      `json:"networks"`
		Bands    []string      `json:"bands"`
		On       []string      `json:"on"`
		Only     []assureCombo `json:"only"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Over == "" {
		body.Over = overBoth
	}
	if body.Over != overAir && body.Over != overWire && body.Over != overBoth {
		return badRequest("over: air, wire or both")
	}
	if len(body.Networks) > 32 || len(body.Bands) > 3 || len(body.On) > 64 || len(body.Only) > 256 {
		return badRequest("at most 32 networks, the three bands, 64 APs and 256 things")
	}
	for _, o := range body.Only {
		if o.Network == "" || o.On == "" || (o.Band != "2g" && o.Band != "5g" && o.Band != "6g" && o.Band != wireBand) {
			return badRequest("only: each thing a network, the AP it is joined on, and a band: 2g, 5g, 6g or wire")
		}
	}
	f := roundFilter{Networks: body.Networks, Bands: body.Bands, Only: body.Only}
	for _, b := range body.Bands {
		if b != "2g" && b != "5g" && b != "6g" {
			return badRequest("bands: 2g, 5g or 6g")
		}
	}
	t := c.state.Org.Locations
	node := hierarchy.NodeID(body.Under)
	if _, ok := t.Node(node); !ok || !may(c, change.Locations, t, node, access.View) {
		return errNotFound
	}
	for _, id := range body.On {
		if n, ok := t.Node(hierarchy.NodeID(id)); !ok || n.Kind != hierarchy.KindAP || !may(c, change.Locations, t, hierarchy.NodeID(id), access.View) {
			return badRequest("on: %q is not an AP here", id)
		}
		f.On = append(f.On, hierarchy.NodeID(id))
	}
	if !may(c, change.Locations, t, node, access.Act) {
		return &apiError{http.StatusForbidden, "asking for a round of tests needs the actions right here (0177)"}
	}
	docs := map[hierarchy.NodeID]map[string]any{}
	things := 0
	for _, ap := range t.APs() {
		if ap == node || slices.Contains(t.Ancestry(ap), node) {
			for _, cb := range s.roundCombos(c.state, ap, docs, body.Over) {
				if f.keeps(cb) {
					things++
				}
			}
		}
	}
	if things == 0 && (len(f.Networks) > 0 || len(f.Bands) > 0 || len(f.On) > 0 || len(f.Only) > 0) {
		return badRequest("no AP here can test those: none tests those networks, on those bands or those APs")
	}
	if things == 0 {
		switch body.Over {
		case overAir:
			return badRequest("no AP here can test anything through the air: one needs a scan radio, and a network a test can join")
		case overWire:
			return badRequest("no AP here can test anything over the wire: one needs an agent that tests over the wire, and a network to test")
		}
		return badRequest("no AP here can test anything: one needs a scan radio, or an agent that tests over the wire, and a network to test")
	}
	// By the clock the tests are stamped with: what a round has had asked
	// is told by comparing the two.
	one := s.rounds.ask(node, string(c.actor), s.conds.Now(), body.Over, f)
	slog.Info("assurance: a round asked", "under", node, "by", c.actor, "over", body.Over, "things", things, "networks", f.Networks, "bands", f.Bands, "on", f.On, "only", len(f.Only))
	writeJSON(w, http.StatusOK, map[string]any{"round": one, "things": things})
	return nil
}
