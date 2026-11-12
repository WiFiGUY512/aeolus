package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/journey"
)

// Why clients leave (0255). Each AP says a client's leaving as it happens
// (0252), and since 0255 with what it knew of why: who sent it off, if
// anyone, by which frame and with which reason. The manager keeps each
// (conditions.ClientEvent, kind "left") and names it by leaveRules.

// leaveDetail is what an AP said of a leaving, as kept: by is aeolus (one
// of Aeolus's own daemons sent it off, saying why in aeolus), client, ap
// or none (no frame came near it); frame deauth or disassoc and its reason
// where one came; all where the AP's frame went to every station; signal
// how strongly the AP last heard the client, by its minutes; last that
// minute itself; and inactive_ms how long the client had sent nothing, by
// the AP's latest report, inactive_ago_s before the leaving.
type leaveDetail struct {
	By          string     `json:"by"`
	Frame       string     `json:"frame,omitempty"`
	Reason      *int       `json:"reason,omitempty"`
	All         bool       `json:"all,omitempty"`
	Aeolus      string     `json:"aeolus,omitempty"`
	Signal      *int       `json:"signal,omitempty"`
	Network     string     `json:"network,omitempty"`
	Last        *leaveLast `json:"last,omitempty"`
	InactiveMS  *int64     `json:"inactive_ms,omitempty"`
	InactiveAgo *int       `json:"inactive_ago_s,omitempty"`
}

// leaveLast is a client's last minute at the AP before it left (0255, 0207):
// ago_s from that minute's end to the leaving, how strongly the AP heard it
// (on average and at the least), and what went to it and came from it.
type leaveLast struct {
	AgoS      int   `json:"ago_s"`
	Signal    *int  `json:"signal,omitempty"`
	SignalMin *int  `json:"signal_min,omitempty"`
	TxPackets int64 `json:"tx_packets"`
	TxRetries int64 `json:"tx_retries"`
	TxFailed  int64 `json:"tx_failed"`
	RxPackets int64 `json:"rx_packets"`
}

var leaveWord = regexp.MustCompile(`^[a-z][a-z-]{0,23}$`)

// leaveOf is what a news item says of a leaving, where it says anything an
// older agent did not: nil from one before 0255.
func leaveOf(n clientNews) *leaveDetail {
	if n.By == "" {
		return nil
	}
	d := &leaveDetail{By: n.By, All: n.All}
	switch n.By {
	case "aeolus", "client", "ap", "none":
	default:
		return nil
	}
	if n.Frame == "deauth" || n.Frame == "disassoc" {
		d.Frame = n.Frame
		if n.Reason != nil && *n.Reason >= 0 && *n.Reason <= 65535 {
			d.Reason = n.Reason
		}
	}
	if leaveWord.MatchString(n.Aeolus) {
		d.Aeolus = n.Aeolus
	}
	if leaveWord.MatchString(n.Network) {
		d.Network = n.Network
	}
	return d
}

// reasonWords is what the reasons a deauthentication or disassociation
// gives mean (802.11's reason codes), those met most.
var reasonWords = map[int]string{
	1: "no reason given", 2: "its authentication no longer holds", 3: "leaving", 4: "idle too long",
	5: "the AP has too many clients", 6: "it is not authenticated", 7: "it is not associated", 8: "leaving the network",
	9: "it is not authenticated", 12: "moved on by a BSS transition", 14: "a key message failed its check", 15: "the key handshake timed out",
	16: "the group key handshake timed out", 23: "its sign-in failed", 34: "too many frames went unacknowledged",
}

// leaveRule names one kind of leaving: the first rule that matches a
// leaving names it. trouble is whether it is worth a look, not the client's
// own choice nor Aeolus's: the AP or the air sent it off, or it went with
// no word. The table is what to extend: a new kind is a row here, and
// nothing else changes.
// leaveRule names a kind of leaving: its words, whether it is worth a look
// (trouble), whether it counts as the client's dropping off (drop, 0259: not
// Aeolus's doing nor its network's), and which leavings are of it.
type leaveRule struct {
	kind, words   string
	trouble, drop bool
	match         func(d leaveDetail) bool
}

func reasonIn(d leaveDetail, codes ...int) bool {
	return d.Reason != nil && slices.Contains(codes, *d.Reason)
}

var leaveRules = []leaveRule{
	{"moved", "let go of: it joined another AP", false, false, func(d leaveDetail) bool { return d.By == "aeolus" && d.Aeolus == "moved" }},
	{"asked", "disconnected by a person", false, false, func(d leaveDetail) bool { return d.By == "aeolus" && d.Aeolus == "asked" }},
	{"steered", "put off by steering", false, false, func(d leaveDetail) bool {
		return d.By == "aeolus" && d.Aeolus == "steered" || d.By == "ap" && reasonIn(d, 12)
	}},
	{"off-air", "its network was taken off the air", false, false, func(d leaveDetail) bool { return d.By == "aeolus" && d.Aeolus == "off-air" }},
	{"aeolus", "sent off by Aeolus", false, false, func(d leaveDetail) bool { return d.By == "aeolus" }},
	{"network", "its network went down or started anew", true, false, func(d leaveDetail) bool { return d.All }},
	{"leaving", "said it was leaving", false, true, func(d leaveDetail) bool { return d.By == "client" && (d.Reason == nil || reasonIn(d, 1, 3, 8)) }},
	{"client-other", "left, saying something was wrong", true, true, func(d leaveDetail) bool { return d.By == "client" }},
	{"idle", "sent off by the AP: idle too long", true, true, func(d leaveDetail) bool { return d.By == "ap" && reasonIn(d, 4) }},
	{"unheard", "sent off by the AP: too many frames went unacknowledged", true, true, func(d leaveDetail) bool { return d.By == "ap" && reasonIn(d, 34) }},
	{"keys", "sent off by the AP: its keys failed", true, true, func(d leaveDetail) bool { return d.By == "ap" && reasonIn(d, 14, 15, 16) }},
	{"signin", "sent off by the AP: its sign-in failed", true, true, func(d leaveDetail) bool { return d.By == "ap" && reasonIn(d, 23) }},
	{"busy", "sent off by the AP: too many clients", true, true, func(d leaveDetail) bool { return d.By == "ap" && reasonIn(d, 5) }},
	{"state", "sent off by the AP: it no longer held it as joined", true, true, func(d leaveDetail) bool { return d.By == "ap" && reasonIn(d, 2, 6, 7, 9) }},
	{"sent-off", "sent off by the AP", true, true, func(d leaveDetail) bool { return d.By == "ap" }},
	{"silent", "gone with no word: no frame from it or the AP", true, true, func(d leaveDetail) bool { return true }},
}

// leaveKind is the rule that names a leaving.
func leaveKind(d leaveDetail) leaveRule {
	for _, r := range leaveRules {
		if r.match(d) {
			return r
		}
	}
	return leaveRules[len(leaveRules)-1]
}

// leavings is the client events of the leavings in moves, each with what its
// AP said of why, and how strongly the AP last heard the client: one from
// an agent before 0255, which says no why, is not kept.
func (s *Server) leavings(ap hierarchy.NodeID, moves []clientMove) []conditions.ClientEvent {
	var out []conditions.ClientEvent
	var inactive map[string]int64
	var reported time.Time
	for _, m := range moves {
		if !m.Left || m.Leave == nil {
			continue
		}
		d := *m.Leave
		if lm := s.lastMinute(ap, m.MAC, m.At); lm != nil {
			d.Signal = lm.Signal
			d.Last = &leaveLast{AgoS: max(0, int(m.At.Sub(lm.At.Add(time.Minute)).Seconds())), Signal: lm.Signal, SignalMin: lm.SignalMin,
				TxPackets: lm.TxPackets, TxRetries: lm.TxRetries, TxFailed: lm.TxFailed, RxPackets: lm.RxPackets}
		}
		// How long it had sent nothing, by the AP's latest report: read
		// once for all of an AP's leavings, as many may come at once.
		if inactive == nil {
			inactive, reported = s.inactiveOf(ap)
		}
		if ms, ok := inactive[m.MAC]; ok {
			ago := max(0, int(m.At.Sub(reported).Seconds()))
			d.InactiveMS, d.InactiveAgo = &ms, &ago
		}
		raw, err := json.Marshal(d)
		if err != nil {
			continue
		}
		out = append(out, conditions.ClientEvent{AP: ap, MAC: m.MAC, At: m.At, Kind: "left", Band: m.Band, SSID: m.SSID, Detail: raw})
	}
	return out
}

// lastMinute is the latest minute an AP said of a client before at (0207),
// in the ten minutes before it, from the minutes kept: one in which it sent
// nothing too, as a client asleep. Nil where there is none.
func (s *Server) lastMinute(ap hierarchy.NodeID, mac string, at time.Time) *conditions.ClientMinute {
	list, err := s.conds.ClientMinutes(mac, at.Add(-10*time.Minute), []hierarchy.NodeID{ap})
	if err != nil {
		return nil
	}
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].At.Before(at) {
			return &list[i]
		}
	}
	return nil
}

// inactiveOf is each client's inactive_ms in an AP's latest report, and when
// that report was made: empty where it has none.
func (s *Server) inactiveOf(ap hierarchy.NodeID) (map[string]int64, time.Time) {
	out := map[string]int64{}
	st, err := s.conds.LatestState(ap)
	if err != nil || st == nil {
		return out, time.Time{}
	}
	var r struct {
		Clients []wifiClient `json:"clients"`
	}
	if json.Unmarshal(st.Report, &r) != nil {
		return out, st.At
	}
	for _, c := range r.Clients {
		out[strings.ToLower(c.MAC)] = c.InactiveMS
	}
	return out, st.At
}

// leavingMost is how many leavings are read for a view, at the most, and
// leavingLatest how many it lists.
const (
	leavingMost   = 20000
	leavingLatest = 50
)

type leavingKindView struct {
	Kind    string `json:"kind"`
	Words   string `json:"words"`
	Trouble bool   `json:"trouble"`
	Count   int    `json:"count"`
}

type leavingBandView struct {
	Band  string            `json:"band"`
	Total int               `json:"total"`
	Kinds []leavingKindView `json:"kinds"`
}

type leavingRow struct {
	At          time.Time        `json:"at"`
	AP          hierarchy.NodeID `json:"ap"`
	APName      string           `json:"ap_name"`
	MAC         string           `json:"mac"`
	Band        string           `json:"band"`
	SSID        string           `json:"ssid"`
	Kind        string           `json:"kind"`
	Words       string           `json:"words"`
	Trouble     bool             `json:"trouble"`
	By          string           `json:"by"`
	Frame       string           `json:"frame,omitempty"`
	Reason      *int             `json:"reason,omitempty"`
	ReasonWords string           `json:"reason_words,omitempty"`
	Signal      *int             `json:"signal,omitempty"`
	Last        *leaveLast       `json:"last,omitempty"`
	InactiveMS  *int64           `json:"inactive_ms,omitempty"`
	InactiveAgo *int             `json:"inactive_ago_s,omitempty"`
	Next        *leaveNext       `json:"next,omitempty"`
}

// leaveNext is where a client went after it left (0258): its next attempt
// on any AP, how many seconds after it left that began, below 0 where it
// had begun on another AP already, and how it went. None where it made no
// attempt within nextWithin.
type leaveNext struct {
	AP      hierarchy.NodeID `json:"ap"`
	APName  string           `json:"ap_name"`
	AfterS  int              `json:"after_s"`
	Outcome string           `json:"outcome"`
}

// nextBefore and nextWithin are how long before a leaving an attempt on
// another AP is taken for the client's having moved already, and how long
// after it one is looked for.
const (
	nextBefore = 30 * time.Second
	nextWithin = 10 * time.Minute
)

// nextAfter is where a client went after it left an AP at a time: the first
// attempt from nextBefore before to nextWithin after, but for one on that
// AP begun before (the one it left). Nil where there is none.
func (s *Server) nextAfter(ap hierarchy.NodeID, mac string, at time.Time, t *hierarchy.Tree) *leaveNext {
	list, err := s.conds.ConnectionsBetween(mac, at.Add(-nextBefore), at.Add(nextWithin), 8)
	if err != nil {
		return nil
	}
	for _, c := range list {
		if c.AP == ap && !c.Started.After(at) {
			continue
		}
		n := &leaveNext{AP: c.AP, AfterS: int(c.Started.Sub(at).Round(time.Second) / time.Second), Outcome: c.Outcome}
		if node, ok := t.Node(c.AP); ok {
			n.APName = node.Name
		}
		return n
	}
	return nil
}

// dropsFewest is how many drops within an hour make a client one that keeps
// dropping (0259); dropAwayLeast is the least time away, in seconds, for a
// leaving to be a drop: a client moving between rooms is back at once.
const (
	dropsFewest   = 4
	dropAwayLeast = 10
	// dropsRead is how many of a client's leavings are read for its drops;
	// droppingMost how many clients a leaving view looks into for them.
	dropsRead    = 500
	droppingMost = 20
)

// dropOf is a leaving as a drop (0259), where it is one: of a kind that
// counts as one, and followed within nextWithin by another try to join,
// dropAwayLeast seconds or more after it left.
func (s *Server) dropOf(e conditions.ClientEvent, t *hierarchy.Tree) (journey.Drop, bool) {
	var d leaveDetail
	if json.Unmarshal(e.Detail, &d) != nil || !leaveKind(d).drop {
		return journey.Drop{}, false
	}
	n := s.nextAfter(e.AP, e.MAC, e.At, t)
	if n == nil || n.AfterS < dropAwayLeast {
		return journey.Drop{}, false
	}
	name := string(e.AP)
	if node, ok := t.Node(e.AP); ok {
		name = node.Name
	}
	return journey.Drop{AP: e.AP, Name: name, At: e.At, Away: time.Duration(n.AfterS) * time.Second}, true
}

// dropsOf are a client's drops since a time, on the APs named (0259).
func (s *Server) dropsOf(aps []hierarchy.NodeID, mac string, since time.Time, t *hierarchy.Tree) ([]journey.Drop, error) {
	list, err := s.conds.ClientEvents(conditions.ClientEventQuery{Kind: "left", APs: aps, MAC: mac, Since: since, Limit: dropsRead})
	if err != nil {
		return nil, err
	}
	var out []journey.Drop
	for _, e := range list {
		if d, ok := s.dropOf(e, t); ok {
			out = append(out, d)
		}
	}
	return out, nil
}

// leavingDropping is a client that kept dropping (0259), as a leaving view
// says it: who, how many times within an hour, about how often in seconds,
// how long away in the middle of them, and the APs it left, the most first.
type leavingDropping struct {
	MAC    string    `json:"mac"`
	Times  int       `json:"times"`
	First  time.Time `json:"first"`
	EveryS int       `json:"every_s"`
	AwayS  int       `json:"away_s"`
	APs    []string  `json:"aps"`
}

// dropping are the clients among a view's leavings that kept dropping
// (0259), the most times first. Only those with dropsFewest leavings of a
// kind that counts within an hour are looked into, at most droppingMost.
func (s *Server) dropping(list []conditions.ClientEvent, tests map[string]hierarchy.NodeID, t *hierarchy.Tree) []leavingDropping {
	byMAC := map[string][]conditions.ClientEvent{}
	var macs []string
	for _, e := range list {
		if _, ok := tests[e.MAC]; ok {
			continue
		}
		var d leaveDetail
		if json.Unmarshal(e.Detail, &d) != nil || !leaveKind(d).drop {
			continue
		}
		if byMAC[e.MAC] == nil {
			macs = append(macs, e.MAC)
		}
		byMAC[e.MAC] = append(byMAC[e.MAC], e)
	}
	out := []leavingDropping{}
	looked := 0
	for _, mac := range macs {
		evs := byMAC[mac]
		slices.SortFunc(evs, func(a, b conditions.ClientEvent) int { return a.At.Compare(b.At) })
		if n, _ := journey.MostIn(evs, func(e conditions.ClientEvent) time.Time { return e.At }, journey.DropsWithin); n < dropsFewest || looked >= droppingMost {
			continue
		}
		looked++
		var drops []journey.Drop
		for _, e := range evs {
			if d, ok := s.dropOf(e, t); ok {
				drops = append(drops, d)
			}
		}
		if d, ok := journey.KeepsDropping(drops, dropsFewest); ok {
			out = append(out, leavingDropping{MAC: mac, Times: d.Times, First: d.First, EveryS: int(d.Every / time.Second), AwayS: int(d.Away / time.Second), APs: d.APs})
		}
	}
	slices.SortFunc(out, func(a, b leavingDropping) int {
		if a.Times != b.Times {
			return b.Times - a.Times
		}
		return strings.Compare(a.MAC, b.MAC)
	})
	return out
}

type leavingView struct {
	Hours   int               `json:"hours"`
	Total   int               `json:"total"`
	Trouble int               `json:"trouble"`
	Bands   []leavingBandView `json:"bands"`
	Latest  []leavingRow      `json:"latest"`
	More    bool              `json:"more,omitempty"`
	// The clients that kept dropping (0259), the most times first.
	Dropping []leavingDropping `json:"dropping"`
}

// leaving is GET /v1/leaving (0255): why clients left the APs the caller may
// view, over the last ?hours= (24 unless set, at most 2160), a band at a
// time (bands are not added together); ?under= keeps it to a Locations
// node's APs, ?mac= to one client, ?band= to one band. Each band has its
// leavings counted by kind, the most first; latest is the latest 50, each
// with its AP, kind and words, who sent it off, the frame and its reason,
// and how strongly the AP last heard it. The APs' own test clients (0121)
// are left out.
func (s *Server) leaving(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	hours := 24
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 2160 {
			return badRequest("hours: from 1 to 2160")
		}
		hours = n
	}
	band := q.Get("band")
	if band != "" && !bands[band] {
		return badRequest("band: 2g, 5g or 6g")
	}
	mac := q.Get("mac")
	if mac != "" && !macRE.MatchString(mac) {
		return badRequest("mac: aa:bb:cc:dd:ee:ff")
	}
	list, err := s.conds.ClientEvents(conditions.ClientEventQuery{Kind: "left", APs: mayView(c, hierarchy.NodeID(q.Get("under"))),
		MAC: mac, Band: band, Since: time.Now().Add(-time.Duration(hours) * time.Hour), Limit: leavingMost + 1})
	if err != nil {
		return err
	}
	view := leavingView{Hours: hours, Bands: []leavingBandView{}, Latest: []leavingRow{}}
	if len(list) > leavingMost {
		list, view.More = list[:leavingMost], true
	}
	tests, t := testClients(c.state), c.state.Org.Locations
	byBand := map[string]map[string]*leavingKindView{}
	totals := map[string]int{}
	for _, e := range list {
		if _, ok := tests[e.MAC]; ok {
			continue
		}
		var d leaveDetail
		if json.Unmarshal(e.Detail, &d) != nil {
			continue
		}
		k := leaveKind(d)
		view.Total++
		if k.trouble {
			view.Trouble++
		}
		totals[e.Band]++
		kinds := byBand[e.Band]
		if kinds == nil {
			kinds = map[string]*leavingKindView{}
			byBand[e.Band] = kinds
		}
		kv := kinds[k.kind]
		if kv == nil {
			kv = &leavingKindView{Kind: k.kind, Words: k.words, Trouble: k.trouble}
			kinds[k.kind] = kv
		}
		kv.Count++
		if len(view.Latest) < leavingLatest {
			row := leavingRow{At: e.At, AP: e.AP, MAC: e.MAC, Band: e.Band, SSID: e.SSID, Kind: k.kind, Words: k.words, Trouble: k.trouble,
				By: d.By, Frame: d.Frame, Reason: d.Reason, Signal: d.Signal, Last: d.Last, InactiveMS: d.InactiveMS, InactiveAgo: d.InactiveAgo}
			if n, ok := t.Node(e.AP); ok {
				row.APName = n.Name
			}
			row.Next = s.nextAfter(e.AP, e.MAC, e.At, t)
			if d.Reason != nil {
				row.ReasonWords = reasonWords[*d.Reason]
				if row.ReasonWords == "" {
					row.ReasonWords = fmt.Sprintf("reason %d", *d.Reason)
				}
			}
			view.Latest = append(view.Latest, row)
		}
	}
	view.Dropping = s.dropping(list, tests, t)
	for _, b := range []string{"2g", "5g", "6g"} {
		kinds := byBand[b]
		if len(kinds) == 0 {
			continue
		}
		bv := leavingBandView{Band: b, Total: totals[b]}
		for _, kv := range kinds {
			bv.Kinds = append(bv.Kinds, *kv)
		}
		slices.SortFunc(bv.Kinds, func(a, b leavingKindView) int {
			if a.Count != b.Count {
				return b.Count - a.Count
			}
			return strings.Compare(a.Kind, b.Kind)
		})
		view.Bands = append(view.Bands, bv)
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}
