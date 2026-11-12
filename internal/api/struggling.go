package api

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/journey"
)

// A client a person named that struggles (0233): over the last two hours it
// went back and forth between an AP's bands again and again, or what was
// sent to it was sent again and again, or it ran slowly on 5 or 6 GHz for
// how well it heard the AP. Resends count again (0242): they were the one
// sign the Living Room AP's radio fault gave, on a phone, before 0236
// took them out. A named client is one someone cares about: a phone, a
// laptop, a TV. Each of these can be told a minute at a time (0207), and
// none of them is an alert on its own, as rejoins are (0190): together over
// two hours, they are what a person calls bad Wi-Fi.
const (
	struggleWindow = 2 * time.Hour
	// struggleNow is how long ago the client must have been heard at an AP
	// for the alert to stand: one that has gone is said again when it is
	// back. The alert is on the AP it struggles on (0306), wherever it is
	// now, so that it does not move from AP to AP with the client.
	struggleNow = 10 * time.Minute
	// struggleFlips is how many times in the window a client must have
	// moved between the bands of one AP: four, two back and forth.
	struggleFlips = 4
	// A busy minute is one the client moved a megabyte in, as its journey
	// counts its retries over (0303); the resends and the rate are told
	// over ten of them at least.
	struggleBusy    = journey.BusyBytes
	struggleMinutes = 10
	// struggleResent is the share of frames to it sent again.
	struggleResent = 0.30
	// On 5 or 6 GHz, a client heard at struggleHeard dBm or better whose
	// middle rate is under struggleRate Mbit/s is slow for its signal.
	// Heard less well, the bar falls by struggleSlope a dB, to its last at
	// struggleFloor dBm; heard less well than that, a client runs slowly
	// on any AP, and none is set (0242). Both stand 5 dB higher than 0242
	// set them, on ath11k's readings 5 dB low until port -62 (0250).
	struggleHeard = -60
	struggleRate  = 120.0
	struggleSlope = 6.5
	struggleFloor = -75
	// struggleRead is the most attempts read for the clients' band flips.
	struggleRead = 2000
	// A struggling client did better elsewhere lately (0309) where, over
	// the last struggleLookBack, another AP and band heard it, over
	// struggleMinutes busy minutes at least, struggleBetterDB stronger in
	// the middle than where it struggles, and sent at most half as much of
	// what it sent it again.
	struggleLookBack = 48 * time.Hour
	struggleBetterDB = 6
)

// struggling are the named clients that struggle, by the AP each is on.
func (s *Server) struggling(state *change.State, now time.Time) map[hierarchy.NodeID][]alerts.Struggling {
	out := map[hierarchy.NodeID][]alerts.Struggling{}
	aps := state.Org.Locations.APs()
	names, err := s.conds.ClientNames(aps)
	if err != nil || len(names) == 0 {
		return out
	}
	since := now.Add(-struggleWindow)
	var macs []string
	for mac := range names {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	attempts, _ := s.conds.ConnectionsOfClients(macs, since, struggleRead)
	tried := map[string][]conditions.Connection{}
	for _, a := range attempts {
		mac := strings.ToLower(a.MAC)
		tried[mac] = append(tried[mac], a)
	}
	name := func(id hierarchy.NodeID) string {
		if n, ok := state.Org.Locations.Node(id); ok {
			return n.Name
		}
		return string(id)
	}
	for _, mac := range macs {
		minutes, err := s.conds.ClientMinutes(mac, since, aps)
		if err != nil || len(minutes) == 0 {
			continue
		}
		last := minutes[len(minutes)-1]
		if now.Sub(last.At) > struggleNow {
			continue
		}
		why, first, where := struggles(minutes, tried[strings.ToLower(mac)], name)
		if len(why) == 0 {
			continue
		}
		if where == "" {
			where = last.AP
		}
		st := alerts.Struggling{MAC: mac, Who: names[mac] + " (" + mac + ")", Why: why, Since: &first}
		if lately, err := s.conds.ClientMinutes(mac, now.Add(-struggleLookBack), aps); err == nil {
			st.Better = betterPlace(minutes, lately, where, name)
		}
		out[where] = append(out[where], st)
	}
	return out
}

// struggles says how a client struggles, of its minutes and its attempts in
// the window, the oldest minute first and the latest attempt first; since
// when the first of what it says began; and the AP it struggles on (0306):
// the one whose bands it moved between, or else the one it was busy on the
// most minutes.
func struggles(minutes []conditions.ClientMinute, attempts []conditions.Connection, name func(hierarchy.NodeID) string) ([]string, time.Time, hierarchy.NodeID) {
	var why []string
	first := minutes[len(minutes)-1].At
	seen := func(t time.Time) {
		if t.Before(first) {
			first = t
		}
	}
	// Back and forth between the bands of one AP: one attempt let on where
	// the one before it was let on, at the same AP, on another band.
	type at struct {
		ap    hierarchy.NodeID
		band  string
		start time.Time
	}
	var on []at
	for i := len(attempts) - 1; i >= 0; i-- {
		a := attempts[i]
		if a.Outcome == "online" || a.Outcome == "connected" {
			on = append(on, at{a.AP, a.Band, a.Started})
		}
	}
	flips := map[hierarchy.NodeID]int{}
	bands := map[hierarchy.NodeID]map[string]bool{}
	starts := map[hierarchy.NodeID]time.Time{}
	for i := 1; i < len(on); i++ {
		p, c := on[i-1], on[i]
		if p.ap != c.ap || p.band == c.band {
			continue
		}
		flips[c.ap]++
		if bands[c.ap] == nil {
			bands[c.ap] = map[string]bool{}
			starts[c.ap] = p.start
		}
		bands[c.ap][p.band], bands[c.ap][c.band] = true, true
	}
	var most hierarchy.NodeID
	for ap, n := range flips {
		if n > flips[most] || (n == flips[most] && ap < most) {
			most = ap
		}
	}
	var where hierarchy.NodeID
	if flips[most] >= struggleFlips {
		where = most
		var bs []string
		for _, b := range []string{"2g", "5g", "6g"} {
			if bands[most][b] {
				bs = append(bs, bandWords(b))
			}
		}
		why = append(why, fmt.Sprintf("it moved between %s on %s %d times in %s", strings.Join(bs, " and "), name(most), flips[most], durationWords(struggleWindow)))
		seen(starts[most])
	}
	// Its busy minutes: frames to it sent again, and how fast it ran for
	// how well it heard the AP, a band at a time.
	var packets, retries int64
	var busy int
	byBand := map[string][]conditions.ClientMinute{}
	busyOn := map[hierarchy.NodeID]int{}
	var busyFirst time.Time
	for _, m := range minutes {
		if m.TxBytes+m.RxBytes < struggleBusy {
			continue
		}
		if busy == 0 {
			busyFirst = m.At
		}
		busy++
		busyOn[m.AP]++
		packets += m.TxPackets
		retries += m.TxRetries
		if (m.Band == "5g" || m.Band == "6g") && m.Signal != nil && m.TxRate != nil {
			byBand[m.Band] = append(byBand[m.Band], m)
		}
	}
	if busy >= struggleMinutes && packets > 0 && float64(retries)/float64(packets) >= struggleResent {
		why = append(why, fmt.Sprintf("%.0f%% of the frames sent to it were sent again", 100*float64(retries)/float64(packets)))
		seen(busyFirst)
	}
	for _, b := range []string{"5g", "6g"} {
		ms := byBand[b]
		if len(ms) < struggleMinutes {
			continue
		}
		var sig []int
		var rate []float64
		for _, m := range ms {
			sig = append(sig, *m.Signal)
			rate = append(rate, *m.TxRate)
		}
		sort.Ints(sig)
		sort.Float64s(rate)
		heard, ran := sig[len(sig)/2], rate[len(rate)/2]
		if bar, ok := slowUnder(heard); ok && ran < bar {
			why = append(why, fmt.Sprintf("on %s it ran at %.0f Mbit/s, heard at %d dBm, where under %.0f is slow", bandWords(b), ran, heard, bar))
			seen(ms[0].At)
		}
	}
	if where == "" && len(why) > 0 {
		for ap, n := range busyOn {
			if n > busyOn[where] || (n == busyOn[where] && ap < where) {
				where = ap
			}
		}
	}
	return why, first, where
}

// slowUnder is the rate under which a client heard at sig dBm runs slowly
// on 5 or 6 GHz (0242, 0250): 120 Mbit/s heard at -60 dBm or better, less
// by 6.5 a dB below that, to 22.5 at -75; and none heard less well, where a
// client runs slowly whatever the AP. On the Living Room AP's faulty radio,
// named clients ran at 32 Mbit/s heard at -61 to -65 dBm, the middle of a
// week; on ce7f's, at 34 heard at -76 to -80: ath11k's readings then, 5 dB
// low (B110), put right.
func slowUnder(sig int) (float64, bool) {
	switch {
	case sig >= struggleHeard:
		return struggleRate, true
	case sig < struggleFloor:
		return 0, false
	}
	return struggleRate - struggleSlope*float64(struggleHeard-sig), true
}

// durationWords says a whole number of hours, or of minutes.
func durationWords(d time.Duration) string {
	if d%time.Hour == 0 {
		if d == time.Hour {
			return "an hour"
		}
		return fmt.Sprintf("%d hours", int(d/time.Hour))
	}
	return fmt.Sprintf("%d minutes", int(d/time.Minute))
}

// betterPlace is where a client that struggles on an AP did better lately
// (0309), of its minutes in the window (now) and over struggleLookBack
// (lately): the AP other than that one, on the band it was busy on most
// there, whose busy minutes heard it struggleBetterDB stronger in the middle
// than its busy minutes there now,
// with at most half the share sent again, over struggleMinutes of them at
// least; the strongest heard. None where it was not busy there now.
func betterPlace(now, lately []conditions.ClientMinute, on hierarchy.NodeID, name func(hierarchy.NodeID) string) *alerts.BetterPlace {
	type place struct {
		ap   hierarchy.NodeID
		band string
	}
	type tally struct {
		sig             []int
		packets, resent int64
	}
	add := func(t *tally, m conditions.ClientMinute) {
		if m.Signal != nil {
			t.sig = append(t.sig, *m.Signal)
		}
		t.packets += m.TxPackets
		t.resent += m.TxRetries
	}
	middle := func(v []int) int {
		v = slices.Clone(v)
		slices.Sort(v)
		return v[len(v)/2]
	}
	// The band it was busy on most here: signals are only alike on one
	// band, and sticky moves a client on its band.
	bands := map[string]int{}
	for _, m := range now {
		if m.AP == on && m.TxBytes+m.RxBytes >= struggleBusy {
			bands[m.Band]++
		}
	}
	band := ""
	for b, n := range bands {
		if n > bands[band] || (n == bands[band] && b < band) {
			band = b
		}
	}
	var here tally
	for _, m := range now {
		if m.AP == on && m.Band == band && m.TxBytes+m.RxBytes >= struggleBusy {
			add(&here, m)
		}
	}
	if len(here.sig) == 0 || here.packets == 0 {
		return nil
	}
	sig, share := middle(here.sig), float64(here.resent)/float64(here.packets)
	there := map[place]*tally{}
	for _, m := range lately {
		if m.AP == on || m.Band != band || m.TxBytes+m.RxBytes < struggleBusy {
			continue
		}
		k := place{m.AP, m.Band}
		if there[k] == nil {
			there[k] = &tally{}
		}
		add(there[k], m)
	}
	var best *alerts.BetterPlace
	for k, t := range there {
		if len(t.sig) < struggleMinutes || t.packets == 0 {
			continue
		}
		s, r := middle(t.sig), float64(t.resent)/float64(t.packets)
		if s < sig+struggleBetterDB || r > share/2 {
			continue
		}
		if best == nil || s > best.Signal || (s == best.Signal && name(k.ap) < best.AP) {
			best = &alerts.BetterPlace{AP: name(k.ap), Band: k.band, Signal: s, Resent: r, Minutes: len(t.sig)}
		}
	}
	return best
}
