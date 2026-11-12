package api

import (
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A radio that runs slowly (0242). A client test through the air that
// measures speed is a client at a place that does not move, so what each
// tester gets from each AP's radio has a usual. Where a radio's last three
// such tests, by any of its testers, each got under a third of that
// tester's usual, one way or the other, and no other tester got its usual
// from it since those testers' tests began to be slow (0308), the radio
// runs slowly: as the
// Living Room AP's did after a channel switch on the ath11k driver (B101),
// 7 Mbit/s down where 84 was usual, which nothing said. The usual is of the
// tester's tests at a like signal: a tester moved, as ce7f was into the
// Living Room AP's place, hears the radio otherwise, and has a usual to
// learn anew, where a radio gone slow is heard as before.
const (
	// slowUsualOver is how far back a tester's usual is told from, and
	// slowUsualMin how many of its tests of the radio it takes at the
	// least. The usual is the upper quartile of them: what the radio
	// does when it is well, which a long bad spell does not pull down.
	slowUsualOver = 7 * 24 * time.Hour
	slowUsualMin  = 5
	// slowLike is how far apart, in dB, two tests' signals may be and the
	// one still be a usual for the other.
	slowLike = 6.0
	// slowShare is the share of the usual a slow test is under.
	slowShare = 1.0 / 3
	// slowRun is how many of the radio's latest tests must each be slow,
	// and slowFresh how recent the newest of them must be, for the alert
	// to stand.
	slowRun   = 3
	slowFresh = 6 * time.Hour
)

// slowRadios are the radios that run slowly, by their AP, for its alerts.
func (s *Server) slowRadios(state *change.State, now time.Time) map[hierarchy.NodeID][]alerts.SlowRadio {
	list, err := s.tests(now.Add(-slowUsualOver))
	if err != nil {
		return map[hierarchy.NodeID][]alerts.SlowRadio{}
	}
	t := state.Org.Locations
	return slowRadiosOf(list, now, func(id hierarchy.NodeID) (string, bool) {
		n, ok := t.Node(id)
		return n.Name, ok
	}, s.channelNow())
}

// channelNow says which channel an AP's radio on a band is on now, as its
// latest report says (0268): the one with networks on the air where it has
// two there, as a C-260 has with its scan radio; 0 where it is not known.
func (s *Server) channelNow() func(hierarchy.NodeID, string) int {
	seen := map[hierarchy.NodeID][]radioState{}
	return func(ap hierarchy.NodeID, band string) int {
		radios, ok := seen[ap]
		if !ok {
			if l, err := s.conds.Latest(ap); err == nil && l.State != nil {
				var r struct {
					Radios []radioState `json:"radios"`
				}
				if json.Unmarshal(l.State.Report, &r) == nil {
					radios = r.Radios
				}
			}
			seen[ap] = radios
		}
		ch := 0
		for _, x := range radios {
			if x.Band != band || x.Channel == 0 {
				continue
			}
			if x.BSS != nil && *x.BSS > 0 {
				return x.Channel
			}
			if ch == 0 {
				ch = x.Channel
			}
		}
		return ch
	}
}

// slowRadiosOf works them out of the tests, the newest first: a test
// through the air, on an AP named, on a band, that got on and measured
// its speed down. A test that says how strongly it heard the AP is held to
// the tests that heard it within slowLike; one that does not, to all. A
// test on a channel the radio has since left, as chanNow says, speaks for
// that channel, not the radio's now (0268): the run ends at it.
func slowRadiosOf(list []testRecord, now time.Time, name func(hierarchy.NodeID) (string, bool), chanNow func(hierarchy.NodeID, string) int) map[hierarchy.NodeID][]alerts.SlowRadio {
	type pair struct {
		tester, on hierarchy.NodeID
		band       string
	}
	type radio struct {
		on   hierarchy.NodeID
		band string
	}
	type test struct {
		tester   hierarchy.NodeID
		at       time.Time
		down, up *float64
		sig      *float64
		channel  int
	}
	type sample struct {
		v   float64
		sig *float64
	}
	downs, ups := map[pair][]sample{}, map[pair][]sample{}
	tests := map[radio][]test{}
	for _, r := range list {
		if !r.ran() || r.failed() || r.ask.On == "" || r.ask.Band == "" || r.ask.Band == wireBand || r.detail.Speed == nil || r.detail.Speed.Down == nil {
			continue
		}
		sp, sig := r.detail.Speed, r.detail.signal()
		p := pair{hierarchy.NodeID(r.AP), hierarchy.NodeID(r.ask.On), r.ask.Band}
		downs[p] = append(downs[p], sample{*sp.Down, sig})
		if sp.Up != nil {
			ups[p] = append(ups[p], sample{*sp.Up, sig})
		}
		k := radio{p.on, p.band}
		tests[k] = append(tests[k], test{p.tester, r.At, sp.Down, sp.Up, sig, r.detail.Channel})
	}
	usual := func(list []sample, sig *float64) *float64 {
		var v []float64
		for _, x := range list {
			if sig == nil || (x.sig != nil && math.Abs(*x.sig-*sig) <= slowLike) {
				v = append(v, x.v)
			}
		}
		if len(v) < slowUsualMin {
			return nil
		}
		sort.Float64s(v)
		return &v[len(v)*3/4]
	}
	out := map[hierarchy.NodeID][]alerts.SlowRadio{}
	for k, ts := range tests {
		if _, ok := name(k.on); !ok {
			continue
		}
		cur := 0
		if chanNow != nil {
			cur = chanNow(k.on, k.band)
		}
		// judge is how a test went against its tester's usual: nothing to
		// judge by where the tester has no usual, or is no AP's here.
		judge := func(t test) (alerts.SlowTest, bool) {
			p := pair{t.tester, k.on, k.band}
			ud, uu := usual(downs[p], t.sig), usual(ups[p], t.sig)
			if ud == nil {
				return alerts.SlowTest{}, false
			}
			tester, ok := name(t.tester)
			if !ok {
				return alerts.SlowTest{}, false
			}
			return alerts.SlowTest{Tester: tester, Down: t.down, Up: t.up, UsualDown: ud, UsualUp: uu,
				SlowDown: *t.down < *ud*slowShare, SlowUp: t.up != nil && uu != nil && *t.up < *uu*slowShare}, true
		}
		slow := func(st alerts.SlowTest) bool { return st.SlowDown || st.SlowUp }
		// The radio's tests on the channel it is on now, the newest first.
		var here []test
		for _, t := range ts {
			if cur != 0 && t.channel != 0 && t.channel != cur {
				break
			}
			here = append(here, t)
		}
		var run []alerts.SlowTest
		var oldest time.Time
		by := map[hierarchy.NodeID]bool{}
		for _, t := range here {
			st, ok := judge(t)
			if !ok {
				continue
			}
			if len(run) == 0 && now.Sub(t.at) > slowFresh {
				break
			}
			if !slow(st) {
				break
			}
			run, oldest = append(run, st), t.at
			by[t.tester] = true
			if len(run) == slowRun {
				break
			}
		}
		if len(run) < slowRun {
			continue
		}
		// Where the run is of one tester's tests, or a few, another tester
		// that got its usual from the radio since they began to be slow
		// says the radio is well: what is slow is their way to it (0308),
		// as the C260's scan radio's to the Living Room's 2.4 GHz was on
		// 2026-11-04, while the Garage's got its usual.
		began := oldest
		for id := range by {
			for _, t := range here {
				if t.tester != id {
					continue
				}
				st, ok := judge(t)
				if !ok {
					continue
				}
				if !slow(st) {
					break
				}
				if t.at.Before(began) {
					began = t.at
				}
			}
		}
		latest := map[hierarchy.NodeID]bool{}
		well := false
		for _, t := range here {
			if by[t.tester] || latest[t.tester] {
				continue
			}
			latest[t.tester] = true
			if st, ok := judge(t); ok && !slow(st) && t.at.After(began) {
				well = true
				break
			}
		}
		if well {
			continue
		}
		since := oldest
		out[k.on] = append(out[k.on], alerts.SlowRadio{Band: k.band, Tests: run, Since: &since})
	}
	for _, list := range out {
		sort.Slice(list, func(i, j int) bool { return list[i].Band < list[j].Band })
	}
	return out
}
