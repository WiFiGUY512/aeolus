package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Roaming (0136): a client that is on a network and moves to another AP of
// it, or another radio of the same AP. The AP it comes to records the
// attempt as it does any (0118), and with it how the client came: as one
// already on the network (a reassociation), by which authentication, with a
// key from before or none, and how long it took to be let on. The manager
// reads those back as roams: how many, of which kind, how long they
// typically took, and those that failed or were slow.

// The kinds of roam, by how much of coming online the client did again, the
// least first: a fast transition (802.11r), which brings its keys; a key
// made from its sign-in at another AP (OKC), or one this AP held from its
// last time here (PMKSA cache), or one of the two where it is not known
// which, each with the four key messages and no sign-in; a passphrase, with
// the four messages, as a network with one has nothing more to do; and
// everything again, the sign-in or WPA3's exchange.
const (
	roamFT     = "ft"
	roamOKC    = "okc"
	roamPMKSA  = "pmksa"
	roamCached = "cached"
	roamKeys   = "keys"
	roamFull   = "full"
)

var roamKinds = []string{roamFT, roamOKC, roamPMKSA, roamCached, roamKeys, roamFull}

const (
	// roamSlowMS is how long a roam may take at the AP it comes to, from
	// the first line its AP has of it to its being let on, before it is
	// called slow: a call in progress hears a gap of a quarter of a second.
	roamSlowMS = 250
	// roamingLatest is how many roams are listed, the latest; and as many
	// of those that failed or were slow.
	roamingLatest = 50
	// roamingFresh is how long before an attempt a client must have been
	// let on at the AP or radio it comes from for the attempt to be a
	// roam, where the client did not come as one already on the network;
	// or, within roamingReported, been reported by that AP as moving
	// something through it. An AP reports every stateEvery, at its next
	// poll after, a minute later at the most where it polls every minute,
	// as it does unless system.poll says otherwise: a report of a client
	// that was there until it moved is no older than that, and the time it
	// took to move. An AP that polls less often than every two minutes
	// reports less often than the window is long, and some roams of its
	// clients are not seen.
	roamingFresh    = 5 * time.Minute
	roamingReported = stateEvery + 2*time.Minute
	// roamingBack is how far before the first roam looked at a client's
	// attempts are read together, to know where each was before; one that
	// was nowhere in that time is looked for by itself, however far back.
	roamingBack = time.Hour
)

// roamingMost is how many attempts are read for the roams of a time, of
// the APs asked about; and roamingPast how many of those clients' attempts
// anywhere, to know where each was before.
var roamingMost, roamingPast = 5000, 20000

// connHow is how a client came, as its AP's record of the attempt has it
// (0136): by a reassociation; its authentication, as hostapd names it (ft
// for a fast transition, sae for WPA3, open); a key from before, from this
// AP's own cache (pmksa) or made from a sign-in at another AP (okc); how
// often the AP asked the client it still held to prove itself (SA Query);
// how long from the first line its AP has of it to its being let on; and,
// where that first line was its association, as where hostapd says nothing
// of an authentication (began: assoc), that the time is short of what it
// took.
//
// It is read by itself, whatever else it holds: a later agent's word here
// that this manager does not know costs that word, and a value it cannot
// take costs how the client came, and neither the attempt's record.
type connHow struct {
	Re    bool   `json:"re,omitempty"`
	Alg   string `json:"alg,omitempty"`
	PMK   string `json:"pmk,omitempty"`
	SAQ   int    `json:"saq,omitempty"`
	OnMS  *int   `json:"on_ms,omitempty"`
	Began string `json:"began,omitempty"`
	bad   bool
}

var howAlgRE = regexp.MustCompile(`^[a-z0-9]{1,12}$`)

func (h *connHow) UnmarshalJSON(b []byte) error {
	type plain connHow
	var p plain
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&p); err != nil {
		h.bad = true
		return nil
	}
	*h = connHow(p)
	return nil
}

func (h *connHow) ok() bool {
	return !h.bad && (h.Alg == "" || howAlgRE.MatchString(h.Alg)) && (h.PMK == "" || h.PMK == "pmksa" || h.PMK == "okc") &&
		h.SAQ >= 0 && h.SAQ <= 64 && (h.OnMS == nil || (*h.OnMS >= 0 && *h.OnMS <= 600000)) && (h.Began == "" || h.Began == "assoc")
}

// roam is one attempt that was a roam: who, when, to which AP and from
// where, where that is known, of which kind, how long it took to be let
// on, and how it went: ok, slow (let on, after more than roamSlowMS),
// failed (not let on), or left (gone before it was let on, with nothing
// refused, as a client that thought better of it). One whose AP has no
// line of its authentication is let on with no time: what there is, from
// its association, is not what it took.
type roam struct {
	ID       int64            `json:"id"`
	At       time.Time        `json:"at"`
	MAC      string           `json:"mac"`
	Host     string           `json:"host,omitempty"`
	AP       hierarchy.NodeID `json:"ap"`
	APName   string           `json:"ap_name"`
	Network  string           `json:"network"`
	SSID     string           `json:"ssid"`
	Band     string           `json:"band"`
	FromAP   hierarchy.NodeID `json:"from_ap,omitempty"`
	FromName string           `json:"from_ap_name,omitempty"`
	FromBand string           `json:"from_band,omitempty"`
	Kind     string           `json:"kind"`
	MS       *int             `json:"ms,omitempty"`
	Untimed  bool             `json:"untimed,omitempty"`
	// BandSwitch is set where the client came from another radio of the
	// same AP: a band switch, and counted apart from the roams, which are
	// from one AP to another.
	BandSwitch bool   `json:"band_switch,omitempty"`
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	SAQueries  int    `json:"sa_queries,omitempty"`
}

// roamRecord is what a roam is read from in an attempt's record.
type roamRecord struct {
	BSS    string     `json:"bss"`
	Host   string     `json:"host"`
	How    *connHow   `json:"how"`
	Events []connStep `json:"events"`
}

// roamKind is the kind of roam an attempt is, where it is one: how much
// the client did again. said is whether the client itself came as one
// already on the network, by a reassociation, or by a fast transition,
// which only such a client makes; many a client that moves says neither,
// and joins the next AP as if for the first time. known is whether its
// record says how it came at all: an agent before 0136 does not. signs
// says whether its network signs clients in by 802.1X, where a client that
// did not sign in again came with a key from before, though hostapd,
// logging less there (0133), does not say which; it is asked only where
// that decides.
func roamKind(rec roamRecord, signs func() bool) (kind string, said, known bool) {
	h := rec.How
	if h == nil || !h.ok() {
		return "", false, false
	}
	said = h.Re || h.Alg == "ft"
	signedIn := slices.ContainsFunc(rec.Events, func(e connStep) bool { return e.Stage == "signin" })
	switch {
	case h.Alg == "ft":
		return roamFT, said, true
	case h.PMK == "okc":
		return roamOKC, said, true
	case h.PMK == "pmksa":
		return roamPMKSA, said, true
	case signedIn || h.Alg == "sae":
		return roamFull, said, true
	case signs():
		return roamCached, said, true
	}
	return roamKeys, said, true
}

// letOn says whether an attempt was let on to its network: its record says
// when, or, of an agent that did not say, it came online, was connected,
// or failed at something that comes after.
func letOn(cn conditions.Connection, rec roamRecord) bool {
	if rec.How != nil && rec.How.ok() && rec.How.OnMS != nil {
		return true
	}
	return cn.Outcome == "online" || cn.Outcome == "connected" ||
		(cn.Outcome == "failed" && slices.Contains([]string{"dhcp", "gateway", "dns", "internet"}, cn.Stage)) ||
		(cn.Outcome == "left" && !strings.HasPrefix(cn.Reason, "before it was let on"))
}

// roamKindsView is the roams of one kind: how many, how many of them
// failed and how many were slow, and of those let on and timed the middle,
// the least and the most of their times in ms.
type roamKindsView struct {
	Kind   string `json:"kind"`
	Roams  int    `json:"roams"`
	Failed int    `json:"failed"`
	Slow   int    `json:"slow"`
	MS     *int   `json:"typical_ms,omitempty"`
	Low    *int   `json:"low_ms,omitempty"`
	High   *int   `json:"high_ms,omitempty"`
}

// roamingView is the roams of the clients on the APs given since a time:
// in all and a kind at a time, the latest, and those that went badly. The
// tests' own clients are left out (0121): what an AP's scan radio does is
// no client's roaming.
type roamingView struct {
	Hours   int `json:"hours"`
	SlowMS  int `json:"slow_over_ms"`
	Roams   int `json:"roams"`
	Failed  int `json:"failed"`
	Slow    int `json:"slow"`
	Left    int `json:"left"`
	Untimed int `json:"untimed"`
	// Switches is how many moves were from one radio of an AP to another
	// of the same: band switches, in no other count here.
	Switches int             `json:"band_switches"`
	Clients  int             `json:"clients"`
	MS       *int            `json:"typical_ms,omitempty"`
	Kinds    []roamKindsView `json:"kinds"`
	Latest   []roam          `json:"latest"`
	Bad      []roam          `json:"bad"`
	// More is set where the APs asked about had more attempts in the time
	// than are read: the counts are then of the latest of them.
	More bool `json:"more,omitempty"`
	// Joins is the clients that came to the APs most often in the time, a
	// roam or a join from nothing alike, more than once: one that keeps
	// coming is one to look at.
	Joins []roamJoins `json:"joins"`
	// KeyWaits is how long each AP kept its clients waiting on each band
	// between their last key message and being let on (0231).
	KeyWaits []keyWait `json:"key_waits"`
}

// keyWait is how long one AP kept clients waiting, on one band, between
// the fourth key message and being let on (0231), over the joins of the
// time that had one: the radio's storing of the client's key. It was
// 30-60 ms where mac80211 waited out an RCU grace period for it (B94,
// 2026-10-28) and is 1 or 2 ms where it does not, so an AP whose typical
// wait is long has a kernel or driver that holds every join up. A join by
// 802.11r has no key messages, nor one where its AP keeps no log of them,
// as on a network that signs clients in (0133).
type keyWait struct {
	AP     hierarchy.NodeID `json:"ap"`
	APName string           `json:"ap_name"`
	Band   string           `json:"band"`
	Joins  int              `json:"joins"`
	MS     *int             `json:"typical_ms,omitempty"`
	High   *int             `json:"high_ms,omitempty"`
}

// keyWaitMost is the longest wait taken as one: past it, the client's own
// lateness or a lost frame, not the AP's storing of a key.
const keyWaitMost = 5000

// keyWaits is the waits of the attempts in list, an AP and a band at a
// time, by band and then the AP's name.
func keyWaits(t *hierarchy.Tree, list []conditions.Connection) []keyWait {
	type at struct {
		ap   hierarchy.NodeID
		band string
	}
	waits := map[at][]int{}
	for _, cn := range list {
		var rec roamRecord
		if json.Unmarshal(cn.Record, &rec) != nil || rec.How == nil || !rec.How.ok() || rec.How.OnMS == nil {
			continue
		}
		m4 := -1
		for _, e := range rec.Events {
			if e.K == "m4" {
				m4 = e.T
			}
		}
		if wait := *rec.How.OnMS - m4; m4 >= 0 && wait >= 0 && wait <= keyWaitMost {
			waits[at{cn.AP, cn.Band}] = append(waits[at{cn.AP, cn.Band}], wait)
		}
	}
	out := []keyWait{}
	for k, v := range waits {
		w := keyWait{AP: k.ap, APName: string(k.ap), Band: k.band, Joins: len(v)}
		if n, ok := t.Node(k.ap); ok {
			w.APName = n.Name
		}
		w.MS, _, w.High = middle(v)
		out = append(out, w)
	}
	band := map[string]int{"2g": 0, "5g": 1, "6g": 2}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Band != out[j].Band {
			bi, iok := band[out[i].Band]
			bj, jok := band[out[j].Band]
			if iok != jok {
				return iok
			}
			if bi != bj {
				return bi < bj
			}
			return out[i].Band < out[j].Band
		}
		return out[i].APName < out[j].APName
	})
	return out
}

// roamJoins is how often one client tried to come online in the time, how
// many of those failed, and at which APs, by name.
type roamJoins struct {
	MAC      string   `json:"mac"`
	Host     string   `json:"host,omitempty"`
	Attempts int      `json:"attempts"`
	Failed   int      `json:"failed"`
	APs      []string `json:"aps"`
}

// roamingJoins is how many of the clients that came most are listed.
const roamingJoins = 10

func middle(v []int) (mid, low, high *int) {
	if len(v) == 0 {
		return nil, nil, nil
	}
	sort.Ints(v)
	m, l, h := v[len(v)/2], v[0], v[len(v)-1]
	return &m, &l, &h
}

// signsIn says whether a network on an AP signs each client in by 802.1X,
// worked out once for each.
func (s *Server) signsIn(state *change.State) func(hierarchy.NodeID, string) bool {
	signs := map[string]bool{}
	return func(ap hierarchy.NodeID, network string) bool {
		k := string(ap) + " " + network
		if v, ok := signs[k]; ok {
			return v
		}
		v := false
		if res, err := s.compose(state, ap, s.reveal); err == nil && !res.Unassigned {
			nets, _ := res.Doc["network"].(map[string]any)
			n, _ := nets[network].(map[string]any)
			sec, _ := n["security"].(string)
			v = enterprise[sec]
		}
		signs[k] = v
		return v
	}
}

// roamSaid is what a client's own attempts say of one that was a roam: of
// which kind, how long it took to be let on, where that is known, and where
// it came from, where the caller may view that AP (0222).
type roamSaid struct {
	Kind       string           `json:"kind"`
	MS         *int             `json:"ms,omitempty"`
	BandSwitch bool             `json:"band_switch,omitempty"`
	FromAP     hierarchy.NodeID `json:"from_ap,omitempty"`
	FromName   string           `json:"from_ap_name,omitempty"`
	FromBand   string           `json:"from_band,omitempty"`
}

// roamsAmong reads the roams among attempts, which are the caller's to
// see, and gives them the latest first. An attempt is a roam where the
// client came to an AP or a radio other than the one it was last let on
// at, and either came as one already on the network (roamKind), or was on
// that other, on the same network, moments before: let on there within
// roamingFresh, or reported by that AP within roamingReported. One that asked again on
// the radio it was on is none. Where a client was before is read from its own
// attempts on every AP, as the attempts given may leave out the APs it
// came from: together back to an hour before the first of the roams, and
// one by one further back for a client not seen in that time. It is named
// only where it is an AP the caller may view.
func (s *Server) roamsAmong(c call, list []conditions.Connection) ([]roam, error) {
	t := c.state.Org.Locations
	signsIn := s.signsIn(c.state)
	type cand struct {
		cn   conditions.Connection
		rec  roamRecord
		kind string
		said bool
	}
	cands := map[int64]cand{}
	macs := map[string]bool{}
	var first time.Time
	for _, cn := range list {
		var rec roamRecord
		if json.Unmarshal(cn.Record, &rec) != nil {
			continue
		}
		kind, said, known := roamKind(rec, func() bool { return signsIn(cn.AP, cn.Network) })
		if !known {
			continue
		}
		cands[cn.ID] = cand{cn, rec, kind, said}
		macs[cn.MAC] = true
		if first.IsZero() || cn.Started.Before(first) {
			first = cn.Started
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	who := make([]string, 0, len(macs))
	for mac := range macs {
		who = append(who, mac)
	}
	past, err := s.conds.ConnectionsOfClients(who, first.Add(-roamingBack), roamingPast)
	if err != nil {
		return nil, err
	}
	// place is where a client was let on, on which network, and when.
	type place struct {
		ap              hierarchy.NodeID
		bss, band, ssid string
		at              time.Time
	}
	last := map[string]place{}
	var out []roam
	seen := map[int64]bool{}
	emit := func(k cand, before place, known bool) {
		cn, rec := k.cn, k.rec
		r := roam{ID: cn.ID, At: cn.Started, MAC: cn.MAC, Host: rec.Host, AP: cn.AP, Network: cn.Network, SSID: cn.SSID, Band: cn.Band,
			Kind: k.kind, SAQueries: rec.How.SAQ}
		if n, ok := t.Node(cn.AP); ok {
			r.APName = n.Name
		}
		if n, ok := t.Node(before.ap); known && ok && may(c, change.Locations, t, before.ap, access.View) {
			r.FromAP, r.FromName, r.FromBand = before.ap, n.Name, before.band
		}
		// From another radio of the AP it came to: a band switch.
		r.BandSwitch = known && before.ap == cn.AP
		switch {
		case rec.How.OnMS != nil && rec.How.Began == "assoc":
			r.State, r.Untimed = "ok", true
		case rec.How.OnMS != nil:
			r.MS = rec.How.OnMS
			r.State = "ok"
			if *r.MS > roamSlowMS {
				r.State = "slow"
			}
		case letOn(cn, rec):
			// Let on, as its outcome says, by a record that has no time
			// for it: it went well, and is not timed.
			r.State, r.Untimed = "ok", true
		case cn.Outcome == "failed" && cn.Reason == "it was never let on" && (cn.Stage == "auth" || cn.Stage == "assoc"):
			// It authenticated, or began to, and went no further, and
			// nothing was refused: a client that looked at this AP and
			// thought better of it, or was not heard. Given up, and no
			// failure of the network's.
			r.State, r.Reason = "left", "it authenticated and went no further"
		case cn.Outcome == "failed":
			r.State, r.Reason = "failed", cn.Reason
		default:
			r.State, r.Reason = "left", cn.Reason
		}
		out = append(out, r)
	}
	// earlier is where a client was let on before an attempt, however long
	// before.
	var failed error
	earlier := func(cn conditions.Connection) (place, bool) {
		prev, ok, err := s.conds.LastLetOn(cn.MAC, cn.Started)
		if err != nil {
			failed = err
		}
		var rec roamRecord
		if !ok || json.Unmarshal(prev.Record, &rec) != nil {
			return place{}, false
		}
		return place{prev.AP, rec.BSS, prev.Band, prev.SSID, prev.Started}, true
	}
	// roamed says whether an attempt, with where its client was before,
	// is a roam: to another AP or radio; and by the client's own word, or
	// from where it was, on the same network, moments before.
	roamed := func(k cand, before place, known bool) bool {
		cn := k.cn
		if known && before.ap == cn.AP && before.bss == k.rec.BSS {
			return false
		}
		if k.said {
			return true
		}
		if !known || before.ssid != cn.SSID {
			return false
		}
		if gap := cn.Started.Sub(before.at); gap >= 0 && gap <= roamingFresh {
			return true
		}
		was, err := s.conds.Moved(before.ap, cn.MAC, cn.Started.Add(-roamingReported), cn.Started)
		if err != nil {
			failed = err
		}
		return was
	}
	// The oldest first: each client's place as it goes.
	for i := len(past) - 1; i >= 0; i-- {
		cn := past[i]
		var rec roamRecord
		if json.Unmarshal(cn.Record, &rec) != nil {
			continue
		}
		here := place{cn.AP, rec.BSS, cn.Band, cn.SSID, cn.Started}
		before, known := last[cn.MAC]
		if letOn(cn, rec) {
			last[cn.MAC] = here
		}
		k, is := cands[cn.ID]
		if !is {
			continue
		}
		seen[cn.ID] = true
		if !known {
			before, known = earlier(cn)
		}
		if roamed(k, before, known) {
			emit(k, before, known)
		}
	}
	// One its client's attempts did not reach back to, as where there were
	// more of them than are read.
	for id, k := range cands {
		if seen[id] {
			continue
		}
		before, known := earlier(k.cn)
		if roamed(k, before, known) {
			emit(k, before, known)
		}
	}
	if failed != nil {
		return nil, failed
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// roaming is the roams of the clients on the APs the caller may view,
// below one Locations node where ?under= names it, over ?hours= (24 unless
// set, at most 720): GET /v1/roaming.
func (s *Server) roaming(w http.ResponseWriter, r *http.Request, c call) error {
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
	view := roamingView{Hours: hours, SlowMS: roamSlowMS, Kinds: []roamKindsView{}, Latest: []roam{}, Bad: []roam{}, Joins: []roamJoins{}, KeyWaits: []keyWait{}}
	since, aps := time.Now().Add(-time.Duration(hours)*time.Hour), mayView(c, hierarchy.NodeID(q.Get("under")))
	list, err := s.conds.ConnectionsSince(since, aps, tests, roamingMost+1)
	if err != nil {
		return err
	}
	counts, err := s.conds.JoinCounts(aps, nil, since, roamingJoins+len(tests))
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	for _, j := range counts {
		if j.Attempts < 2 || len(view.Joins) == roamingJoins || slices.Contains(tests, j.MAC) {
			continue
		}
		one := roamJoins{MAC: j.MAC, Host: j.Host, Attempts: j.Attempts, Failed: j.Failed, APs: []string{}}
		for _, ap := range j.APs {
			if n, ok := t.Node(ap); ok {
				one.APs = append(one.APs, n.Name)
			}
		}
		sort.Strings(one.APs)
		view.Joins = append(view.Joins, one)
	}
	if len(list) > roamingMost {
		list, view.More = list[:roamingMost], true
	}
	view.KeyWaits = keyWaits(t, list)
	roams, err := s.roamsAmong(c, list)
	if err != nil {
		return err
	}
	// A band switch the client gave up is nothing: a client that looked
	// at the other radio of the AP it is on, and stayed where it was. Not
	// counted and not listed: there may be many, and they would crowd the
	// roams that went badly out of their list.
	roams = slices.DeleteFunc(roams, func(m roam) bool { return m.BandSwitch && m.State == "left" })
	kinds := map[string]*roamKindsView{}
	times := map[string][]int{}
	var all []int
	who := map[string]bool{}
	for _, m := range roams {
		// A band switch is no roam: counted by itself, however it went,
		// and listed with the roams below.
		if m.BandSwitch {
			view.Switches++
			continue
		}
		who[m.MAC] = true
		if m.State == "left" {
			view.Left++
			continue
		}
		k := kinds[m.Kind]
		if k == nil {
			k = &roamKindsView{Kind: m.Kind}
			kinds[m.Kind] = k
		}
		view.Roams++
		k.Roams++
		switch m.State {
		case "failed":
			view.Failed++
			k.Failed++
		case "slow":
			view.Slow++
			k.Slow++
		}
		if m.Untimed {
			view.Untimed++
		}
		if m.MS != nil {
			all = append(all, *m.MS)
			times[m.Kind] = append(times[m.Kind], *m.MS)
		}
	}
	view.Clients = len(who)
	view.MS, _, _ = middle(all)
	for _, kind := range roamKinds {
		if k := kinds[kind]; k != nil {
			k.MS, k.Low, k.High = middle(times[kind])
			view.Kinds = append(view.Kinds, *k)
		}
	}
	for _, m := range roams {
		if len(view.Latest) < roamingLatest {
			view.Latest = append(view.Latest, m)
		}
		if m.State != "ok" && len(view.Bad) < roamingLatest {
			view.Bad = append(view.Bad, m)
		}
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}
