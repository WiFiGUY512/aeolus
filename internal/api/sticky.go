package api

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Sticky clients (0340): a client that held on to an AP and band where it
// was heard weakly, for half an hour or more over the hours asked about,
// while another AP of the fleet heard it far better on the same band. The
// list is for a person to judge whether a client needs steering of its own,
// to the nearest AP (0321), or pinning to one (0319); or whether no AP hears
// it better, and what it lacks is coverage. Told from the clients' minutes
// (0207), a minute at a time, each by the AP it was on.
const (
	// stickyWeak is the signal, in dBm, at which a minute is weak: -80 is
	// weak, -70 good (Griff), and -78 is the line between, with room for
	// a reading or two of jitter.
	stickyWeak = -78
	// stickyWeakMinutes is how many weak minutes on one AP and band a
	// client must have had in the hours for it to be listed.
	stickyWeakMinutes = 30
	// Another AP and band heard it better where, over stickyBetterMinutes
	// at least, its middle signal there was stickyBetterDB stronger than
	// its middle where it held on: a client heard at -84 by one AP and at
	// -50 by another a little later had somewhere better.
	stickyBetterDB      = 10
	stickyBetterMinutes = 10
	// A late move is one from where it held on to another AP on the same
	// band, its next minute there no more than stickyMoveGap after its
	// last here, at stickyMoveDB or more stronger: it was within reach of
	// the AP it came to while it held on to the one it left.
	stickyMoveDB  = 15
	stickyMoveGap = 5 * time.Minute
	// stickyNow is how long ago a client's last minute where it held on
	// may be for it to be there now.
	stickyNow = 10 * time.Minute
	// stickyMost is how many clients are listed, the most weak minutes
	// first.
	stickyMost = 200
)

// stickyView is the sticky list: the hours it is of, the weak signal, and
// each client and place it held on, the most weak minutes first, with how
// many of them are on each band.
type stickyView struct {
	Hours   int            `json:"hours"`
	Weak    int            `json:"weak"`
	Bands   map[string]int `json:"bands"`
	Clients []stickyClient `json:"clients"`
}

// stickyClient is a client, on one AP and band where it held on: how long it
// was on there, and how much of that weakly, at what middle signal; when it
// was last there, and whether it is there now; the best place it had
// elsewhere on the band, and its latest late move; the bands it was seen on;
// its rule of steering (0319, 0321), whether its network is steered there
// and sticky on (0289); what steering asked of it; and what of all that it
// is: sticky (somewhere better), weak (nowhere better seen) or pinned (its
// rule keeps it on the AP it held on to).
type stickyClient struct {
	MAC         string        `json:"mac"`
	Name        string        `json:"name,omitempty"`
	Host        string        `json:"host,omitempty"`
	AP          string        `json:"ap"`
	APName      string        `json:"ap_name"`
	Band        string        `json:"band"`
	SSID        string        `json:"ssid,omitempty"`
	Minutes     int           `json:"minutes"`
	WeakMinutes int           `json:"weak_minutes"`
	Signal      int           `json:"signal"`
	Recent      *int          `json:"recent"`
	Last        time.Time     `json:"last"`
	Now         bool          `json:"now"`
	Better      *stickyPlace  `json:"better"`
	Late        *stickyLate   `json:"late"`
	Bands       []string      `json:"bands"`
	Rule        *stickyRule   `json:"rule"`
	Steered     bool          `json:"steered"`
	Sticky      bool          `json:"sticky"`
	Asked       *stickyAsked  `json:"asked"`
	Class       string        `json:"class"`
	Help        string        `json:"help"`
	HelpAP      *stickyAPName `json:"help_ap,omitempty"`
}

// stickyPlace is an AP and band a client was heard better on: its middle
// signal there, over how many minutes.
type stickyPlace struct {
	AP      string `json:"ap"`
	APName  string `json:"ap_name"`
	Signal  int    `json:"signal"`
	Minutes int    `json:"minutes"`
}

// stickyLate is a client's late moves from where it held on: how many, and
// the latest, to which AP, from what signal to what, and when.
type stickyLate struct {
	Count  int       `json:"count"`
	AP     string    `json:"ap"`
	APName string    `json:"ap_name"`
	From   int       `json:"from"`
	To     int       `json:"to"`
	At     time.Time `json:"at"`
}

// stickyRule is a client's rule of where it belongs, as its AP has it: of
// the nearest, or pinned to its APs, by ID and name, on its bands.
type stickyRule struct {
	Nearest bool           `json:"nearest"`
	APs     []stickyAPName `json:"aps"`
	Bands   []string       `json:"bands"`
}

type stickyAPName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// stickyAsked is what steering asked of a client in the hours, on any AP:
// how many times, and how many of those it moved, refused or came back.
type stickyAsked struct {
	Asked   int `json:"asked"`
	Moved   int `json:"moved"`
	Refused int `json:"refused"`
	Back    int `json:"back"`
}

// What may help a listed client, said in Help.
const (
	stickyHelpNearest  = "nearest"   // a rule of the nearest AP (0321)
	stickyHelpPin      = "pin"       // pinned to the AP it does best on (0319)
	stickyHelpSteerOff = "steer-off" // its network is not steered where it held on: no rule acts there
	stickyHelpRuleIdle = "rule-idle" // it has a rule, and held on all the same
	stickyHelpCoverage = "coverage"  // no AP heard it better: coverage, not steering
	stickyHelpPinned   = "pinned"    // pinned here by a person: as asked
	stickyHelpPinMoved = "pin-moved" // pinned here, and heard far weaker of late: moved? (pin-moved)
)

func (s *Server) sticky(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	hours := 24
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 168 {
			return badRequest("hours: from 1 to 168")
		}
		hours = n
	}
	now := time.Now()
	since := now.Add(-time.Duration(hours) * time.Hour)
	aps := mayView(c, hierarchy.NodeID(q.Get("under")))
	t := c.state.Org.Locations
	name := func(id hierarchy.NodeID) string {
		if n, ok := t.Node(id); ok {
			return n.Name
		}
		return string(id)
	}
	tests := testClients(c.state)
	view := stickyView{Hours: hours, Weak: stickyWeak, Bands: map[string]int{}, Clients: []stickyClient{}}
	var mac string
	var minutes []conditions.ClientMinute
	flush := func() {
		if _, test := tests[strings.ToLower(mac)]; len(minutes) > 0 && !test {
			view.Clients = append(view.Clients, stickyOf(minutes, now, name)...)
		}
		minutes = minutes[:0]
	}
	if err := s.conds.EachClientMinute(since, aps, func(m conditions.ClientMinute) error {
		if m.MAC != mac {
			flush()
			mac = m.MAC
		}
		minutes = append(minutes, m)
		return nil
	}); err != nil {
		return err
	}
	flush()
	sort.SliceStable(view.Clients, func(i, j int) bool {
		a, b := view.Clients[i], view.Clients[j]
		if a.Now != b.Now {
			return a.Now
		}
		if a.WeakMinutes != b.WeakMinutes {
			return a.WeakMinutes > b.WeakMinutes
		}
		return a.MAC < b.MAC
	})
	if len(view.Clients) > stickyMost {
		view.Clients = view.Clients[:stickyMost]
	}
	if len(view.Clients) == 0 {
		writeJSON(w, http.StatusOK, view)
		return nil
	}
	// Who each is, what steering asked of it, and its rule where it held
	// on, as that AP's config has it.
	hosts, _ := s.conds.ClientHosts(aps, since)
	asked := map[string]*stickyAsked{}
	if list, err := s.conds.SteerClients(since, aps, nil, 1000); err == nil {
		for _, a := range list {
			asked[strings.ToLower(a.MAC)] = &stickyAsked{Asked: a.Asked, Moved: a.Moved, Refused: a.Refused, Back: a.Back}
		}
	}
	docs := map[string]map[string]any{}
	for i := range view.Clients {
		sc := &view.Clients[i]
		low := strings.ToLower(sc.MAC)
		sc.Host, sc.Asked = hosts[low], asked[low]
		if n, ok, err := s.conds.ClientNamed(low); err == nil && ok {
			sc.Name = n.Name
		}
		doc, seen := docs[sc.AP]
		if !seen {
			if res, err := s.compose(c.state, hierarchy.NodeID(sc.AP), s.reveal); err == nil {
				doc = res.Doc
			}
			docs[sc.AP] = doc
		}
		sc.Rule, sc.Steered, sc.Sticky = stickyRuleOf(doc, low, sc.SSID, name)
		sc.Class, sc.Help, sc.HelpAP = stickyHelp(sc)
		view.Bands[sc.Band]++
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}

// stickyOf is where one client held on, of its minutes, the oldest first:
// each AP and band it had stickyWeakMinutes weak minutes on, with the best
// place it had elsewhere on that band and its late moves from there.
func stickyOf(minutes []conditions.ClientMinute, now time.Time, name func(hierarchy.NodeID) string) []stickyClient {
	type place struct {
		ap   hierarchy.NodeID
		band string
	}
	type tally struct {
		sig, recent []int
		weak        int
		last        time.Time
		ssid        string
	}
	places := map[place]*tally{}
	bands := map[string]bool{}
	for _, m := range minutes {
		k := place{m.AP, m.Band}
		p := places[k]
		if p == nil {
			p = &tally{}
			places[k] = p
		}
		p.sig = append(p.sig, *m.Signal)
		if now.Sub(m.At) <= alerts.PinMovedRecent {
			p.recent = append(p.recent, *m.Signal)
		}
		if *m.Signal <= stickyWeak {
			p.weak++
		}
		p.last, p.ssid = m.At, m.SSID
		bands[m.Band] = true
	}
	middle := func(v []int) int {
		v = slices.Clone(v)
		slices.Sort(v)
		return v[len(v)/2]
	}
	seen := make([]string, 0, len(bands))
	for b := range bands {
		seen = append(seen, b)
	}
	sort.Strings(seen)
	var out []stickyClient
	for k, p := range places {
		if p.weak < stickyWeakMinutes {
			continue
		}
		sc := stickyClient{MAC: minutes[0].MAC, AP: string(k.ap), APName: name(k.ap), Band: k.band, SSID: p.ssid,
			Minutes: len(p.sig), WeakMinutes: p.weak, Signal: middle(p.sig), Last: p.last, Now: now.Sub(p.last) <= stickyNow, Bands: seen}
		// Its signal there over the last half hour, as pin-moved weighs it.
		if len(p.recent) >= alerts.PinMovedMinutes {
			r := middle(p.recent)
			sc.Recent = &r
		}
		for o, q := range places {
			if o.band != k.band || o.ap == k.ap || len(q.sig) < stickyBetterMinutes {
				continue
			}
			if m := middle(q.sig); m >= sc.Signal+stickyBetterDB && (sc.Better == nil || m > sc.Better.Signal || (m == sc.Better.Signal && name(o.ap) < sc.Better.APName)) {
				sc.Better = &stickyPlace{AP: string(o.ap), APName: name(o.ap), Signal: m, Minutes: len(q.sig)}
			}
		}
		// Its late moves: from a minute here to its next on another AP on
		// the band, soon after and far stronger.
		for i, m := range minutes {
			if m.AP != k.ap || m.Band != k.band {
				continue
			}
			for _, n := range minutes[i+1:] {
				if n.At.Sub(m.At) > stickyMoveGap {
					break
				}
				if n.AP == k.ap && n.Band == k.band {
					break
				}
				if n.Band != k.band {
					continue
				}
				if *n.Signal-*m.Signal >= stickyMoveDB {
					if sc.Late == nil {
						sc.Late = &stickyLate{}
					}
					sc.Late.Count++
					sc.Late.AP, sc.Late.APName, sc.Late.From, sc.Late.To, sc.Late.At = string(n.AP), name(n.AP), *m.Signal, *n.Signal, n.At
				}
				break
			}
		}
		out = append(out, sc)
	}
	return out
}

// stickyRuleOf reads, of an AP's composed config, a client's rule of where
// it belongs (0319, 0321), and whether the network of the SSID it held on to
// is steered there, and with sticky on (0289): a rule acts only on a steered
// network.
func stickyRuleOf(doc map[string]any, mac, ssid string, name func(hierarchy.NodeID) string) (*stickyRule, bool, bool) {
	var rule *stickyRule
	if homes, _ := doc["homes"].(map[string]any); homes != nil {
		if h, _ := homes[mac].(map[string]any); h != nil {
			r := stickyRule{APs: []stickyAPName{}, Bands: []string{}}
			r.Nearest, _ = h["nearest"].(bool)
			for _, a := range asList(h["aps"]) {
				r.APs = append(r.APs, stickyAPName{ID: a, Name: name(hierarchy.NodeID(a))})
			}
			r.Bands = append(r.Bands, asList(h["bands"])...)
			if r.Nearest || len(r.APs) > 0 {
				rule = &r
			}
		}
	}
	nets, _ := doc["network"].(map[string]any)
	for _, v := range nets {
		n, _ := v.(map[string]any)
		if n == nil || n["ssid"] != ssid || n["enabled"] == false {
			continue
		}
		st, _ := n["steering"].(map[string]any)
		suggest, set := st["suggest"].(bool)
		if !set {
			suggest, _ = n["band_steering"].(bool)
		}
		hold, _ := st["hold_back"].(bool)
		sticky, _ := st["sticky"].(bool)
		return rule, suggest || hold || sticky, sticky
	}
	return rule, false, false
}

// asList is a config list of strings.
func asList(v any) []string {
	var out []string
	switch l := v.(type) {
	case []any:
		for _, e := range l {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, l...)
	}
	return out
}

// stickyHelp says what a listed client is, and what may help it: a client
// pinned to the AP it held on to is as a person asked; one with somewhere
// better is sticky, and one seen only on one band whose better place is one
// AP is pinned there, any other given a rule of the nearest, which follows
// it; where its network is not steered there, no rule acts; one with a rule
// held on all the same; one with nowhere better seen lacks coverage, and no
// steering helps it.
func stickyHelp(sc *stickyClient) (string, string, *stickyAPName) {
	if r := sc.Rule; r != nil && !r.Nearest && slices.ContainsFunc(r.APs, func(a stickyAPName) bool { return a.ID == sc.AP }) {
		// Pinned here, and heard far weaker of late than over the hours:
		// moved, perhaps, from where it was pinned for (0340).
		if sc.Recent != nil && *sc.Recent <= alerts.PinMovedWeak && sc.Signal-*sc.Recent >= alerts.PinMovedDB {
			return "pinned", stickyHelpPinMoved, nil
		}
		return "pinned", stickyHelpPinned, nil
	}
	if sc.Better == nil && sc.Late == nil {
		return "weak", stickyHelpCoverage, nil
	}
	switch {
	case !sc.Steered:
		return "sticky", stickyHelpSteerOff, nil
	case sc.Rule != nil:
		return "sticky", stickyHelpRuleIdle, nil
	}
	best := sc.Better
	if best == nil {
		best = &stickyPlace{AP: sc.Late.AP, APName: sc.Late.APName}
	}
	if len(sc.Bands) == 1 && (sc.Late == nil || sc.Late.AP == best.AP) {
		return "sticky", stickyHelpPin, &stickyAPName{ID: best.AP, Name: best.APName}
	}
	return "sticky", stickyHelpNearest, nil
}

// pinsMoved are the clients pinned to an AP (0319), by its composed config,
// doc, that it has heard far weaker of late than it usually does (0340):
// pinned, a client that is moved is kept on an AP that may no longer be its
// best (Griff, 2026-11-08). A pin is a rule of APs that names this one, and
// not of the nearest. Where it did better in the last day, on another AP,
// is said with it.
func (s *Server) pinsMoved(state *change.State, ap hierarchy.NodeID, doc map[string]any, now time.Time) []alerts.PinMoved {
	homes, _ := doc["homes"].(map[string]any)
	var macs []string
	for mac, v := range homes {
		h, _ := v.(map[string]any)
		if h == nil || h["nearest"] == true || !slices.Contains(asList(h["aps"]), string(ap)) {
			continue
		}
		macs = append(macs, mac)
	}
	if len(macs) == 0 {
		return nil
	}
	sort.Strings(macs)
	t := state.Org.Locations
	name := func(id hierarchy.NodeID) string {
		if n, ok := t.Node(id); ok {
			return n.Name
		}
		return string(id)
	}
	middle := func(v []int) int {
		v = slices.Clone(v)
		slices.Sort(v)
		return v[len(v)/2]
	}
	var out []alerts.PinMoved
	for _, mac := range macs {
		minutes, err := s.conds.ClientMinutes(mac, now.Add(-alerts.PinMovedUsual), []hierarchy.NodeID{ap})
		if err != nil || len(minutes) == 0 {
			continue
		}
		// Its band of late: the band of its last minute here.
		band := minutes[len(minutes)-1].Band
		var recent, usual []int
		var since *time.Time
		for _, m := range minutes {
			if m.Signal == nil || m.Band != band {
				continue
			}
			switch age := now.Sub(m.At); {
			case age <= alerts.PinMovedRecent:
				recent = append(recent, *m.Signal)
				if since == nil && *m.Signal <= alerts.PinMovedWeak {
					at := m.At
					since = &at
				}
			case age > alerts.PinMovedGap:
				usual = append(usual, *m.Signal)
			}
		}
		if len(recent) < alerts.PinMovedMinutes || len(usual) < alerts.PinMovedUsualMinutes {
			continue
		}
		got, was := middle(recent), middle(usual)
		if got > alerts.PinMovedWeak || was-got < alerts.PinMovedDB {
			continue
		}
		who := mac
		if n, ok, err := s.conds.ClientNamed(mac); err == nil && ok {
			who = n.Name + " (" + mac + ")"
		}
		pm := alerts.PinMoved{MAC: mac, Who: who, Band: band, Now: got, Usual: was, Since: since}
		// Where it did better in the last day, of the APs, on its band.
		if lately, err := s.conds.ClientMinutes(mac, now.Add(-24*time.Hour), t.APs()); err == nil {
			by := map[hierarchy.NodeID][]int{}
			for _, m := range lately {
				if m.AP != ap && m.Band == band && m.Signal != nil {
					by[m.AP] = append(by[m.AP], *m.Signal)
				}
			}
			for other, sig := range by {
				if len(sig) < stickyBetterMinutes {
					continue
				}
				if m := middle(sig); m >= got+stickyBetterDB && (pm.Better == nil || m > pm.Better.Signal || (m == pm.Better.Signal && name(other) < pm.Better.AP)) {
					pm.Better = &alerts.BetterPlace{AP: name(other), Band: band, Signal: m, Minutes: len(sig)}
				}
			}
		}
		out = append(out, pm)
	}
	return out
}
