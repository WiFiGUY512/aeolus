// Package journey is one Wi-Fi client's history across the fleet (0103):
// the sessions it had, each on one AP, band and network, from the state
// reports the APs sent while it was on them; where it roamed; and what
// looks wrong, such as a weak signal, no DHCP, or moving back and forth.
// It is the first part of the client journey Griff wants: from what the
// manager keeps already, with nothing new on the APs.
package journey

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Client is what one state report says of a client (0066, 0067).
type Client struct {
	MAC       string  `json:"mac"`
	Network   string  `json:"network"`
	SSID      string  `json:"ssid"`
	Band      string  `json:"band"`
	Signal    *int    `json:"signal"`
	TxRate    float64 `json:"tx_rate"`
	RxRate    float64 `json:"rx_rate"`
	TxPackets int64   `json:"tx_packets"`
	TxRetries *int64  `json:"tx_retries"`
	TxFailed  int64   `json:"tx_failed"`
	Connected int64   `json:"connected"`
	Address   string  `json:"address"`
	Host      string  `json:"host"`
	DHCP      string  `json:"dhcp"`
	Gen       string  `json:"gen"`
	VLAN      int     `json:"vlan"`
}

// Sample is a client as one AP's report said, at the report's time.
type Sample struct {
	AP     hierarchy.NodeID
	APName string
	At     time.Time
	C      Client
}

// Session is a client's time on one AP, band and network, unbroken.
type Session struct {
	AP        hierarchy.NodeID `json:"ap"`
	Name      string           `json:"name"`
	Network   string           `json:"network,omitempty"`
	SSID      string           `json:"ssid"`
	Band      string           `json:"band"`
	From      time.Time        `json:"from"`
	To        time.Time        `json:"to"`
	Reports   int              `json:"reports"`
	SignalMin *int             `json:"signal_min,omitempty"`
	SignalMax *int             `json:"signal_max,omitempty"`
	SignalAvg *int             `json:"signal_avg,omitempty"`
	TxRate    float64          `json:"tx_rate"` // Mbit/s, the mean of the reports
	// Retries is the share of what was sent to it that was sent again, over
	// its busy minutes alone, and BusyMinutes how many (0303); set by
	// Resends, not by Build.
	Retries     *float64 `json:"retries,omitempty"`
	BusyMinutes int      `json:"busy_minutes,omitempty"`
	Address     string   `json:"address,omitempty"`
	DHCP        string   `json:"dhcp,omitempty"`
	Gen         string   `json:"gen,omitempty"`
}

// Issue is something that looks wrong in the journey.
type Issue struct {
	Severity string    `json:"severity"` // warning or info
	Kind     string    `json:"kind"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

// Journey is a client's history.
type Journey struct {
	MAC  string `json:"mac"`
	Host string `json:"host,omitempty"`
	// The name a person gave it (0153), who and when; set by who asks, not
	// by Build.
	Name     string     `json:"name,omitempty"`
	NamedBy  string     `json:"named_by,omitempty"`
	NamedAt  *time.Time `json:"named_at,omitempty"`
	First    time.Time  `json:"first"`
	Last     time.Time  `json:"last"`
	Sessions []Session  `json:"sessions"`
	Roams    int        `json:"roams"`
	Issues   []Issue    `json:"issues"`
	// The AP whose scan radio it is, where it is one making client tests
	// (0121) and no device of anyone's; set by who asks, not by Build.
	Tester string `json:"tester,omitempty"`
}

// Gap is how long a client may be missing from reports and still be in the
// same session: a little over two report intervals (0040).
const Gap = 12 * time.Minute

// WeakSignal is the mean signal, in dBm, at or under which a session is
// noted as weak (0293): -80, the line the Overview's weak-signal list uses.
const WeakSignal = -80

// Build makes the journey from the samples, in any order.
func Build(mac string, samples []Sample) Journey {
	j := Journey{MAC: mac, Sessions: []Session{}, Issues: []Issue{}}
	if len(samples) == 0 {
		return j
	}
	sort.SliceStable(samples, func(a, b int) bool { return samples[a].At.Before(samples[b].At) })
	j.First, j.Last = samples[0].At, samples[len(samples)-1].At
	type acc struct {
		s       Session
		signals []int
		rates   []float64
		last    Client
	}
	var cur *acc
	flush := func() {
		if cur == nil {
			return
		}
		s := cur.s
		if len(cur.signals) > 0 {
			lo, hi, sum := cur.signals[0], cur.signals[0], 0
			for _, v := range cur.signals {
				lo, hi, sum = min(lo, v), max(hi, v), sum+v
			}
			avg := (sum - len(cur.signals)/2) / len(cur.signals)
			if sum >= 0 {
				avg = (sum + len(cur.signals)/2) / len(cur.signals)
			}
			s.SignalMin, s.SignalMax, s.SignalAvg = &lo, &hi, &avg
		}
		for _, r := range cur.rates {
			s.TxRate += r / float64(len(cur.rates))
		}
		s.Address, s.DHCP, s.Gen = cur.last.Address, cur.last.DHCP, cur.last.Gen
		j.Sessions = append(j.Sessions, s)
		cur = nil
	}
	for _, x := range samples {
		if x.C.Host != "" {
			j.Host = x.C.Host
		}
		// The AP's own networks have no network ID: by SSID, then.
		sameNet := cur != nil && cur.s.Network == x.C.Network && (x.C.Network != "" || cur.s.SSID == x.C.SSID)
		same := sameNet && cur.s.AP == x.AP && cur.s.Band == x.C.Band && x.At.Sub(cur.s.To) <= Gap
		if !same {
			flush()
			cur = &acc{s: Session{AP: x.AP, Name: x.APName, Network: x.C.Network, SSID: x.C.SSID, Band: x.C.Band, From: x.At.Add(-time.Duration(x.C.Connected) * time.Second), To: x.At}}
			// A session cannot start before the one it follows ended.
			if n := len(j.Sessions); n > 0 && cur.s.From.Before(j.Sessions[n-1].To) {
				cur.s.From = j.Sessions[n-1].To
			}
		}
		cur.s.To = x.At
		cur.s.Reports++
		if x.C.Signal != nil {
			cur.signals = append(cur.signals, *x.C.Signal)
		}
		if x.C.TxRate > 0 {
			cur.rates = append(cur.rates, x.C.TxRate)
		}
		cur.last = x.C
	}
	flush()
	for i := 1; i < len(j.Sessions); i++ {
		if j.Sessions[i].AP != j.Sessions[i-1].AP && j.Sessions[i].From.Sub(j.Sessions[i-1].To) <= Gap {
			j.Roams++
		}
	}
	j.Issues = issues(j)
	return j
}

// issues is what looks wrong: a weak signal, few rates, DHCP that the client
// did not use, and moving back and forth between two APs. Retries are
// Resends's (0303).
func issues(j Journey) []Issue {
	out := []Issue{}
	add := func(sev, kind, msg string, at time.Time) {
		out = append(out, Issue{Severity: sev, Kind: kind, Message: msg, At: at})
	}
	for _, s := range j.Sessions {
		if s.SignalAvg != nil && *s.SignalAvg <= WeakSignal {
			add("warning", "weak-signal", fmt.Sprintf("a weak signal on %s (%s), %d dBm on average: it may need an AP nearer, or to roam sooner", s.Name, bandName(s.Band), *s.SignalAvg), s.From)
		}
		if (s.Band == "5g" || s.Band == "6g") && s.TxRate > 0 && s.TxRate < 30 && s.Reports >= 2 {
			add("info", "slow", fmt.Sprintf("on %s (%s) it ran at %.0f Mbit/s on average", s.Name, bandName(s.Band), s.TxRate), s.From)
		}
		switch s.DHCP {
		case "none":
			add("warning", "no-dhcp", fmt.Sprintf("on %s it asked for no address by DHCP, and shows none", s.Name), s.From)
		case "static":
			add("info", "static", fmt.Sprintf("on %s it uses %s without asking DHCP: a static address", s.Name, s.Address), s.From)
		}
	}
	// Back and forth: four roams or more between the same two APs, A B A B
	// A, within half an hour.
	for i := 0; i+4 < len(j.Sessions); i++ {
		a, b := j.Sessions[i].AP, j.Sessions[i+1].AP
		if a == b {
			continue
		}
		roams := 0
		for k := i + 1; k < len(j.Sessions) && roams < 4; k++ {
			want := b
			if roams%2 == 1 {
				want = a
			}
			if j.Sessions[k].AP != want || j.Sessions[k].From.Sub(j.Sessions[i].To) > 30*time.Minute {
				break
			}
			roams++
		}
		if roams >= 4 {
			add("warning", "ping-pong", fmt.Sprintf("it moved back and forth between %s and %s: their coverage overlaps where it sits; a lower power or a minimum signal settles it", j.Sessions[i].Name, j.Sessions[i+1].Name), j.Sessions[i].To)
			break
		}
	}
	return out
}

// BusyBytes is what a client moves in a minute, both ways, for the minute
// to be a busy one (0233, 0303). Over a busy minute, what is sent to it
// again is the air's and the radio's doing; over an idle or a light one, it
// is mostly rate control trying a faster rate now and then, much of which
// is sent again and says nothing of the link. a phone, a day on 6 GHz:
// 41–77% of its light minutes' frames sent again, 12% of its busy minutes'.
const BusyBytes = 1 << 20

// ResendsMost is the share sent again, over a session's busy minutes, past
// which it is an issue (0103), over ResendsFew busy minutes or more, so
// that one burst does not say it.
const (
	ResendsMost = 0.2
	ResendsFew  = 3
)

// Minute is what one minute came to for a client on one AP and band
// (0207): what was sent to it, sent again and lost, and the bytes it moved
// both ways.
type Minute struct {
	AP        hierarchy.NodeID
	Band      string
	At        time.Time // its start
	TxPackets int64
	TxRetries int64
	TxFailed  int64
	Bytes     int64
}

// Resends sets each session's retries from the client's minutes, in any
// order (0303): what was sent again over what was sent, as the Clients tab
// counts them (0164), in the busy minutes on the session's AP and band, and
// how many there were. A session with no busy minute has none, as has one
// whose minutes are gone (kept 7 days): the reports' running count, which
// mixed the idle stretches in, is no longer said. A share over ResendsMost
// over ResendsFew busy minutes or more is an issue.
func Resends(j *Journey, minutes []Minute) {
	type tally struct {
		sent, again int64
		busy        int
	}
	got := make([]tally, len(j.Sessions))
	for _, m := range minutes {
		if m.Bytes < BusyBytes {
			continue
		}
		if i := sessionOf(j.Sessions, m); i >= 0 {
			got[i].sent += m.TxPackets + m.TxFailed
			got[i].again += m.TxRetries
			got[i].busy++
		}
	}
	for i := range j.Sessions {
		s, t := &j.Sessions[i], got[i]
		s.Retries, s.BusyMinutes = nil, t.busy
		if t.busy == 0 || t.sent <= 0 {
			continue
		}
		r := float64(t.again) / float64(t.sent)
		s.Retries = &r
		if r > ResendsMost && t.busy >= ResendsFew {
			j.Issues = append(j.Issues, Issue{Severity: "warning", Kind: "retries", At: s.From,
				Message: fmt.Sprintf("%.0f%% of frames to it on %s were sent again, over %d minutes it was busy: interference, or a signal too weak for its rate", r*100, s.Name, t.busy)})
		}
	}
}

// sessionOf is the session a minute was in, by its index, or -1: the one on
// its AP and band from the minute it began in to Gap after its last report,
// and not into the session after it.
func sessionOf(sessions []Session, m Minute) int {
	for i, s := range sessions {
		if s.AP != m.AP || s.Band != m.Band {
			continue
		}
		end := s.To.Add(Gap)
		if i+1 < len(sessions) && sessions[i+1].From.Before(end) {
			end = sessions[i+1].From
		}
		if !m.At.Before(s.From.Truncate(time.Minute)) && m.At.Before(end) {
			return i
		}
	}
	return -1
}

func bandName(b string) string {
	switch b {
	case "2g":
		return "2.4 GHz"
	case "5g":
		return "5 GHz"
	case "6g":
		return "6 GHz"
	}
	return b
}

// Online is one time a client came online at an AP, as the AP's record of
// its attempt says (0118, 0190): what the state reports, a few minutes
// apart, cannot see. Tried is an attempt that did not come online: it is
// not counted, but says the client had left where it was by then (0200).
type Online struct {
	AP    hierarchy.NodeID
	Name  string
	At    time.Time
	Tried bool
	// Signal is how strongly the AP heard it as it came, in dBm, where its
	// record says (0312).
	Signal *int
}

// WeakerBy and WeakHere say when a client that comes back again and again
// to an AP, and stays on another, points at the link and not the radio
// (0312): the other heard it WeakerBy dB or more more strongly, and this
// one at WeakHere or under.
const (
	WeakerBy = 10
	WeakHere = -75
)

// Signals is how strongly the APs heard a client as it came online (0312):
// the AP an issue is of, here, and each other, by name.
type Signals struct {
	Here *int
	At   map[string]int
}

// Heard is how strongly each AP heard a client as it came online (0312):
// the middle of its times online there that say, by AP.
func Heard(ons []Online) map[hierarchy.NodeID]int {
	by := map[hierarchy.NodeID][]int{}
	for _, o := range ons {
		if !o.Tried && o.Signal != nil {
			by[o.AP] = append(by[o.AP], *o.Signal)
		}
	}
	out := map[hierarchy.NodeID]int{}
	for ap, s := range by {
		sort.Ints(s)
		out[ap] = s[len(s)/2]
	}
	return out
}

// SignalsOf is Heard as an issue of ap says it: here, and the others by
// name.
func SignalsOf(heard map[hierarchy.NodeID]int, ap hierarchy.NodeID, name func(hierarchy.NodeID) string) Signals {
	out := Signals{At: map[string]int{}}
	for id, s := range heard {
		if id == ap {
			v := s
			out.Here = &v
		} else {
			out.At[name(id)] = s
		}
	}
	return out
}

// RejoinsWithin is the time in which a client coming online at one AP
// fewest times is one that comes back again and again (0190).
const RejoinsWithin = time.Hour

// RejoinsStay is how long a client must try nowhere again, once it came
// online at an AP, for it to have stayed there (0200): over three times
// the three minutes one that comes back 20 times an hour is gone.
const RejoinsStay = 10 * time.Minute

// How a client fared at an AP it came online at (0200).
const (
	Looped = "looped" // it came online there fewest times or more within an hour
	Left   = "left"   // each time, it tried again somewhere sooner than RejoinsStay
	Stayed = "stayed" // once at least, it tried nowhere again for RejoinsStay or more
)

// mostWithin is the most of one AP's times coming online, in order, within
// RejoinsWithin of the first of them, and where they begin.
func mostWithin(list []Online) (best, from int) {
	return MostIn(list, func(o Online) time.Time { return o.At }, RejoinsWithin)
}

// MostIn is the most of a list in time order whose times, by at, lie within
// a span of the first of them, and where they begin.
func MostIn[T any](list []T, at func(T) time.Time, within time.Duration) (best, from int) {
	for i, k := 0, 0; i < len(list); i++ {
		for at(list[i]).Sub(at(list[k])) > within {
			k++
		}
		if n := i - k + 1; n > best {
			best, from = n, k
		}
	}
	return best, from
}

// Drop is a client's dropping off an AP and coming back (0259): the AP it
// left, by ID and name, when, and how long it was away before it tried to
// join again, on that AP or another.
type Drop struct {
	AP   hierarchy.NodeID
	Name string
	At   time.Time
	Away time.Duration
}

// DropsWithin is the time in which a client dropping off fewest times is one
// that keeps dropping (0259).
const DropsWithin = time.Hour

// Dropping is how a client kept dropping (0259): the most times within
// DropsWithin, from when, about how often, how long it was away in the
// middle of them, and the APs it left, the most left first.
type Dropping struct {
	Times int
	First time.Time
	Every time.Duration
	Away  time.Duration
	APs   []string
}

// KeepsDropping is how a client kept dropping, from its drops in any order:
// false where it dropped fewer than fewest times within DropsWithin.
func KeepsDropping(drops []Drop, fewest int) (Dropping, bool) {
	list := slices.Clone(drops)
	sort.SliceStable(list, func(a, b int) bool { return list[a].At.Before(list[b].At) })
	best, from := MostIn(list, func(d Drop) time.Time { return d.At }, DropsWithin)
	if best < fewest || best < 2 {
		return Dropping{}, false
	}
	list = list[from : from+best]
	aways := make([]time.Duration, 0, best)
	left := map[string]int{}
	var aps []string
	for _, d := range list {
		aways = append(aways, d.Away)
		if left[d.Name] == 0 {
			aps = append(aps, d.Name)
		}
		left[d.Name]++
	}
	slices.Sort(aways)
	sort.SliceStable(aps, func(a, b int) bool { return left[aps[a]] > left[aps[b]] })
	return Dropping{Times: best, First: list[0].At, Every: (list[best-1].At.Sub(list[0].At) / time.Duration(best-1)).Round(time.Second),
		Away: aways[len(aways)/2].Round(time.Second), APs: aps}, true
}

// Drops adds an issue where a client kept dropping (0259): fewest times or
// more within an hour, away a while each time, on whichever APs. A client
// walking between rooms is back at once; one coming back to one AP every
// few minutes or faster is Rejoins's (0190). Where it left several APs so,
// it is the client, or something beyond the APs, not one AP's radio.
func Drops(j *Journey, drops []Drop, fewest int) {
	d, ok := KeepsDropping(drops, fewest)
	if !ok {
		return
	}
	msg := fmt.Sprintf("it dropped off and came back %d times within an hour, about every %s, away about %s each time", d.Times, d.Every, d.Away)
	if len(d.APs) > 1 {
		msg += ", leaving " + strings.Join(d.APs, ", ") + ": it does so wherever it is, so look at the client, its power saving or what its own traffic goes through beyond the APs, before any one radio"
	} else {
		msg += ", each time from " + d.APs[0] + ": see whether it does the same on another AP before blaming that one's radio"
	}
	j.Issues = append(j.Issues, Issue{Severity: "warning", Kind: "drops", At: d.First, Message: msg})
}

// Fared is how a client fared at each AP it came online at (0200), from
// its attempts in any order, the latest of them up to until: looped where
// it came online fewest times within an hour; else stayed where it once
// tried nowhere again for RejoinsStay; else left. An AP it came online at
// once, so lately that it cannot have stayed yet, is not said.
func Fared(ons []Online, fewest int, until time.Time) map[hierarchy.NodeID]string {
	all := slices.Clone(ons)
	sort.SliceStable(all, func(a, b int) bool { return all[a].At.Before(all[b].At) })
	out := map[hierarchy.NodeID]string{}
	byAP := map[hierarchy.NodeID][]Online{}
	for i, o := range all {
		if o.Tried {
			continue
		}
		byAP[o.AP] = append(byAP[o.AP], o)
		switch {
		case i+1 == len(all):
			if until.Sub(o.At) >= RejoinsStay {
				out[o.AP] = Stayed
			}
		case all[i+1].At.Sub(o.At) >= RejoinsStay:
			out[o.AP] = Stayed
		case out[o.AP] == "":
			out[o.AP] = Left
		}
	}
	for ap, list := range byAP {
		if best, _ := mostWithin(list); best >= fewest {
			out[ap] = Looped
		}
	}
	return out
}

// Elsewhere is, of the APs but ap that a client came online at, by name,
// those where it did the same, coming back or gone again within minutes,
// and those where it stayed (0200), each in order.
func Elsewhere(fared map[hierarchy.NodeID]string, ap hierarchy.NodeID, name func(hierarchy.NodeID) string) (same, stays []string) {
	for id, f := range fared {
		switch {
		case id == ap:
		case f == Stayed:
			stays = append(stays, name(id))
		default:
			same = append(same, name(id))
		}
	}
	sort.Strings(same)
	sort.Strings(stays)
	return same, stays
}

// Blame is what a client that comes online at an AP again and again points
// at, said after how often it did (0200), the AP called ap, as "this AP".
// Where it did the same on other APs and stayed on none, it drops the
// connection itself wherever it is: the client, or what is beyond the APs,
// such as its cloud connection stalling, as the IoT devices' did. Where
// it stayed on another, the AP's radio; where both, what the APs it does it
// on share; where neither, nothing yet. Where it stayed on another that
// heard it WeakerBy dB more strongly, and ap at WeakHere or under, the
// weaker link is the likelier cause, not ap's radio (0312): as an ESP32
// at -79 dBm at the Shed that held at -62 on the Garage (2026-11-05).
func Blame(ap string, same, stays []string, heard Signals) string {
	did, stayed := strings.Join(same, " and "), strings.Join(stays, " and ")
	if did == "" && stayed != "" && heard.Here != nil && *heard.Here <= WeakHere {
		best, at := 0, ""
		for _, n := range stays {
			if s, ok := heard.At[n]; ok && (at == "" || s > best) {
				best, at = s, n
			}
		}
		if at != "" && best-*heard.Here >= WeakerBy {
			return fmt.Sprintf(": it drops its connection itself and joins again. On %s it stays, heard at %d dBm there against %d dBm here: a link %d dB weaker is the likelier cause, not %s's radio. It comes back here as its own choice",
				stayed, best, *heard.Here, best-*heard.Here, ap)
		}
	}
	switch {
	case did != "" && stayed == "":
		return ", and did the same on " + did + ": it drops its connection itself on every AP it joins, so it is not " + ap +
			"'s radio. It is the client itself, or something beyond the APs: the network or internet path its own traffic takes, as when its cloud connection stalls. Look there, not at the radio"
	case did != "":
		return ", and did the same on " + did + ": it drops its connection itself and joins again. On " + stayed + " it stays: look at what " + ap + " and " + did +
			" share, unlike " + stayed + ": their kind of radio, or the path beyond them"
	case stayed != "":
		return ": it drops its connection itself and joins again. On " + stayed + " it stays, so something of " + ap + "'s radio does not suit it" + heardHere(heard)
	}
	return ": it drops its connection itself and joins again. Nothing on another AP tells yet whether " + ap +
		"'s radio is the cause, or the client itself or something beyond the APs: see whether it does the same on another AP" + heardHere(heard)
}

// heardHere says how strongly the AP heard the client, where known (0312).
func heardHere(heard Signals) string {
	if heard.Here == nil {
		return ""
	}
	return fmt.Sprintf(". It is heard at %d dBm here", *heard.Here)
}

// Rejoins adds an issue where a client came online at one AP fewest times
// or more within an hour (0190): it got on each time, and dropped the
// connection itself a minute or so in. Its moves between APs are then its
// coming back, not coverage that overlaps, so "ping-pong" is taken back.
// What the issue blames is where else it did so, or stayed, up to now
// (0200): that AP's radio only where it stayed on another.
func Rejoins(j *Journey, ons []Online, fewest int, now time.Time) {
	byAP := map[hierarchy.NodeID][]Online{}
	names := map[hierarchy.NodeID]string{}
	var aps []hierarchy.NodeID
	for _, o := range ons {
		if o.Tried {
			continue
		}
		if byAP[o.AP] == nil {
			aps = append(aps, o.AP)
		}
		byAP[o.AP] = append(byAP[o.AP], o)
		names[o.AP] = o.Name
	}
	fared := Fared(ons, fewest, now)
	heard := Heard(ons)
	found := false
	for _, ap := range aps {
		list := byAP[ap]
		sort.Slice(list, func(a, b int) bool { return list[a].At.Before(list[b].At) })
		// The most within an hour of the first of them.
		best, from := mostWithin(list)
		if best < fewest {
			continue
		}
		first, last := list[from].At, list[from+best-1].At
		every := (last.Sub(first) / time.Duration(best-1)).Round(time.Second)
		name := func(id hierarchy.NodeID) string { return names[id] }
		same, stays := Elsewhere(fared, ap, name)
		j.Issues = append(j.Issues, Issue{Severity: "warning", Kind: "rejoins", At: first,
			Message: fmt.Sprintf("it came online on %s %d times within an hour, about every %s", list[from].Name, best, every) +
				Blame("that AP", same, stays, SignalsOf(heard, ap, name))})
		found = true
	}
	if found {
		j.Issues = slices.DeleteFunc(j.Issues, func(i Issue) bool { return i.Kind == "ping-pong" })
	}
}
