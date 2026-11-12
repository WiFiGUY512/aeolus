package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/oui"
)

// airscanState is what an AP's scan radio found of other APs (0160): each
// BSS it hears that is not the AP's own, the strongest first, with what its
// wire check says; and the clients it hears talking through those the
// manager asked after.
type airscanState struct {
	BSS     []airscanBSS    `json:"bss"`
	Clients []airscanClient `json:"clients,omitempty"`
}

// airscanBSS is one BSS a scan radio hears: its BSSID and SSID ("" where it
// hides its name), band, channel and signal; whether frames sent on our wire
// came out of it (nil while that is not known yet), and on which VLANs; how
// many times the scan radio looked; and seconds since it was heard.
type airscanBSS struct {
	BSSID   string  `json:"bssid"`
	SSID    string  `json:"ssid"`
	Band    string  `json:"band"`
	Channel int     `json:"channel"`
	Signal  float64 `json:"signal"`
	Wired   *bool   `json:"wired"`
	VLANs   []int   `json:"vlans,omitempty"`
	Looks   int64   `json:"looks"`
	Ago     int64   `json:"ago"`
}

// airscanClient is one client a scan radio hears talking through a BSSID
// the manager asked after: how many frames, and seconds since.
type airscanClient struct {
	MAC    string `json:"mac"`
	BSSID  string `json:"bssid"`
	Frames int64  `json:"frames"`
	Ago    int64  `json:"ago"`
}

func (a *airscanState) check() error {
	if a == nil {
		return nil
	}
	if len(a.BSS) > 64 || len(a.Clients) > 64 {
		return badRequest("airscan: at most 64 networks and 64 clients")
	}
	for _, b := range a.BSS {
		if !macRE.MatchString(b.BSSID) || len(b.SSID) > 32 || !printable(b.SSID) || !bands[b.Band] || b.Channel < 1 || b.Channel > 233 ||
			b.Signal < -127 || b.Signal > 30 || len(b.VLANs) > 16 || b.Looks < 0 || b.Ago < 0 {
			return badRequest("airscan: a network is a BSSID, a printable SSID of at most 32 characters (none where hidden), a band, a channel from 1 to 233, a signal in dBm, whether it is on the wire, at most 16 VLANs, how many looks and seconds since")
		}
		for _, v := range b.VLANs {
			if v < 0 || v > 4094 {
				return badRequest("airscan: a VLAN is from 0 to 4094")
			}
		}
	}
	for _, c := range a.Clients {
		if !macRE.MatchString(c.MAC) || !macRE.MatchString(c.BSSID) || c.Frames < 0 || c.Ago < 0 {
			return badRequest("airscan: a client is a MAC, the BSSID it talks through, how many frames and seconds since")
		}
	}
	return nil
}

// heardThere is a network an AP's latest report says it hears, by RRM's
// scans (0105) or its scan radio's (0160), with what the wire check says.
type heardThere struct {
	BSSID, SSID, Band string
	Channel           int
	Signal            float64
	Ago               int64
	Wired             *bool
	VLANs             []int
	Looks             int64
}

// heardIn is the networks a report says its AP hears, once each by BSSID:
// RRM's first, then those only the scan radio heard, what the scan radio
// says of the wire kept for each.
func heardIn(raw json.RawMessage) []heardThere {
	var r struct {
		RRM *struct {
			Others []rrmOther `json:"others"`
		} `json:"rrm"`
		Airscan *airscanState `json:"airscan"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return nil
	}
	var out []heardThere
	at := map[string]int{}
	if r.RRM != nil {
		for _, o := range r.RRM.Others {
			at[o.BSSID] = len(out)
			out = append(out, heardThere{BSSID: o.BSSID, SSID: o.SSID, Band: o.Band, Channel: o.Channel, Signal: o.Signal, Ago: o.Ago})
		}
	}
	if r.Airscan != nil {
		for _, b := range r.Airscan.BSS {
			if i, ok := at[b.BSSID]; ok {
				out[i].Wired, out[i].VLANs, out[i].Looks = b.Wired, b.VLANs, b.Looks
				continue
			}
			at[b.BSSID] = len(out)
			out = append(out, heardThere{BSSID: b.BSSID, SSID: b.SSID, Band: b.Band, Channel: b.Channel, Signal: b.Signal, Ago: b.Ago,
				Wired: b.Wired, VLANs: b.VLANs, Looks: b.Looks})
		}
	}
	return out
}

// threatOf says what a network one AP hears is to it (0160): rogue (on our
// wire, and none of the APs'), suspect (one of Aeolus's SSIDs, not seen on
// our wire), known (either, marked known by rogues.known), or "" (one of the
// APs', or another network off our wire).
func threatOf(h heardThere, fleet *alerts.Fleet, known map[string]bool) string {
	wired := h.Wired != nil && *h.Wired
	switch {
	case fleet.Own(h.BSSID) || (!wired && !fleet.SSIDs[h.SSID]):
		return ""
	case known[h.BSSID]:
		return "known"
	case wired:
		return "rogue"
	}
	return "suspect"
}

// rogueWatch is the BSSIDs an AP's report says it hears that are rogues or
// suspects to it (0160), 64 at most, by BSSID: the AP lists the clients
// its scan radio hears with them in its next report. With them, what each
// network is.
func (s *Server) rogueWatch(state *change.State, ap hierarchy.NodeID, raw json.RawMessage) ([]string, map[string]heardThere) {
	fleet, known := s.fleetOwn(state), knownRogues(state, ap)
	watch := []string{}
	of := map[string]heardThere{}
	for _, h := range heardIn(raw) {
		if k := threatOf(h, fleet, known); k == "rogue" || k == "suspect" {
			watch = append(watch, h.BSSID)
			of[h.BSSID] = h
		}
	}
	sort.Strings(watch)
	if len(watch) > 64 {
		watch = watch[:64]
	}
	return watch, of
}

// rogueClients is the clients a report says its scan radio heard on the
// rogues and suspects in it, as the store keeps them, each when it was
// heard.
func rogueClients(report *stateReport, of map[string]heardThere, now time.Time) []conditions.RogueClient {
	if report.Airscan == nil {
		return nil
	}
	var out []conditions.RogueClient
	for _, c := range report.Airscan.Clients {
		h, ok := of[c.BSSID]
		if !ok || c.Frames < 1 {
			continue
		}
		out = append(out, conditions.RogueClient{MAC: c.MAC, BSSID: c.BSSID, SSID: h.SSID, LastAt: now.Add(-time.Duration(c.Ago) * time.Second), Frames: c.Frames})
	}
	return out
}

// threatWire is what the scan radios found of a rogue's wire: on (frames
// sent on our wire came out of it), off (they looked and never saw them) or
// unchecked (no scan radio hears it, or none has finished looking); on
// which VLANs; the most times one looked; and the APs whose scan radios
// looked.
type threatWire struct {
	State string   `json:"state"`
	VLANs []int    `json:"vlans,omitempty"`
	Looks int64    `json:"looks,omitempty"`
	By    []apName `json:"by,omitempty"`
}

type apName struct {
	AP   hierarchy.NodeID `json:"ap"`
	Name string           `json:"name"`
}

// threatAP is a rogue AP, a suspect or a known one (0160), by BSSID: its
// maker by that, where the registry knows it, and whether by the BSSID with
// its locally administered bit cleared; what it broadcasts, where, what the
// wire check found, the APs that hear it, the strongest first, and how many
// clients were heard on it over the time.
type threatAP struct {
	BSSID        string     `json:"bssid"`
	Maker        string     `json:"maker,omitempty"`
	MakerDerived bool       `json:"maker_derived,omitempty"`
	SSID         string     `json:"ssid"`
	Band         string     `json:"band"`
	Channel      int        `json:"channel"`
	Kind         string     `json:"kind"`
	Wire         threatWire `json:"wire"`
	HeardBy      []hearer   `json:"heard_by"`
	Clients      int        `json:"clients"`
}

// threatClient is a client heard on a rogue AP or a suspect (0160): through
// which BSSID and SSID and what that is now (its kind, or "" where no AP
// hears it any more); when first and last, the most frames, and the APs
// that heard it; whether it is one the APs have served, with its name; and
// the networks that block it.
type threatClient struct {
	MAC     string    `json:"mac"`
	BSSID   string    `json:"bssid"`
	SSID    string    `json:"ssid"`
	Kind    string    `json:"kind"`
	FirstAt time.Time `json:"first_at"`
	LastAt  time.Time `json:"last_at"`
	Frames  int64     `json:"frames"`
	HeardBy []apName  `json:"heard_by"`
	Ours    bool      `json:"ours"`
	Host    string    `json:"host,omitempty"`
	Blocked []string  `json:"blocked,omitempty"`
}

// dhcpRogue is a network on which more than one DHCP server answered, or,
// where the network says the servers it has, one it does not (0160), as the
// APs' latest reports say: each server, the most answers an AP counted from
// it, when it last answered, the APs that saw it, and whether it is one the
// network says it has (known).
type dhcpRogue struct {
	Network string          `json:"network"`
	SSID    string          `json:"ssid,omitempty"`
	Servers []dhcpRogueSeen `json:"servers"`
	byKey   map[string]int  // where each server, by address and MAC, is in Servers
	listed  bool            // whether the network says the servers it has
}

type dhcpRogueSeen struct {
	ID      string    `json:"id"`
	MAC     string    `json:"mac"`
	Answers int       `json:"answers"`
	Seen    time.Time `json:"seen"`
	APs     []apName  `json:"aps"`
	Known   bool      `json:"known,omitempty"`
}

// threatsList is the threats on the APs the caller may view (0160), below
// one Locations node where ?under= names it: GET /v1/threats. rogues are
// the rogue APs, suspects and known ones the APs hear now, rogues first;
// clients, those heard on one in the last ?hours= (24 unless set, at most
// 2160), the last heard first; dhcp, the networks on which more than one
// DHCP server answers.
func (s *Server) threatsList(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	hours := 24
	if q := r.URL.Query().Get("hours"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 2160 {
			return badRequest("hours: from 1 to 2160")
		}
		hours = n
	}
	fleet := s.fleetOwn(c.state)
	ssids := networkSSIDs(c.state)
	var aps []hierarchy.NodeID
	by := map[string]*threatAP{}
	verdicts := map[string]map[string]bool{} // by BSSID: whether each AP that hears it has it known
	dhcp := map[string]*dhcpRogue{}
	for _, id := range t.APs() {
		if !may(c, change.Locations, t, id, access.View) {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		aps = append(aps, id)
		st, err := s.conds.LatestState(id)
		if err != nil {
			return err
		}
		// An AP that has not reported for three reports' time hears
		// nothing now: what it last heard is not shown, as its alerts are
		// not (alerts.StateEvery; Griff, 2026-10-23, of an AP off three days
		// whose suspects stayed).
		if st == nil || time.Since(st.At) > 3*alerts.StateEvery {
			continue
		}
		n, _ := t.Node(id)
		known := knownRogues(c.state, id)
		for _, h := range heardIn(st.Report) {
			kind := threatOf(h, fleet, known)
			if kind == "" {
				continue
			}
			x := by[h.BSSID]
			if x == nil {
				x = &threatAP{BSSID: h.BSSID, SSID: h.SSID, Band: h.Band, Channel: h.Channel, Wire: threatWire{State: "unchecked"}}
				by[h.BSSID] = x
				verdicts[h.BSSID] = map[string]bool{}
			}
			verdicts[h.BSSID][string(id)] = kind == "known"
			if h.Wired != nil {
				x.Wire.By = append(x.Wire.By, apName{AP: id, Name: n.Name})
				x.Wire.Looks = max(x.Wire.Looks, h.Looks)
				if *h.Wired {
					x.Wire.State = "on"
					for _, v := range h.VLANs {
						if !slices.Contains(x.Wire.VLANs, v) {
							x.Wire.VLANs = append(x.Wire.VLANs, v)
						}
					}
				} else if x.Wire.State == "unchecked" {
					x.Wire.State = "off"
				}
			}
			x.HeardBy = append(x.HeardBy, hearer{AP: id, Name: n.Name, Signal: h.Signal, Seen: st.At.Add(-time.Duration(h.Ago) * time.Second)})
		}
		var rep struct {
			DHCP map[string]dhcpState `json:"dhcp"`
		}
		if json.Unmarshal(st.Report, &rep) != nil {
			continue
		}
		expected := dhcpServers(c.state, id)
		for net, d := range rep.DHCP {
			for _, sv := range d.Servers {
				x := dhcp[net]
				if x == nil {
					x = &dhcpRogue{Network: net, SSID: ssids[net], byKey: map[string]int{}}
					dhcp[net] = x
				}
				key := sv.ID + "|" + sv.MAC
				i, ok := x.byKey[key]
				if !ok {
					i = len(x.Servers)
					x.byKey[key] = i
					x.Servers = append(x.Servers, dhcpRogueSeen{ID: sv.ID, MAC: sv.MAC})
				}
				if len(expected[net]) > 0 {
					x.listed = true
					x.Servers[i].Known = x.Servers[i].Known || alerts.DHCPKnown(expected[net], sv.ID, sv.MAC)
				}
				seen := st.At.Add(-time.Duration(sv.Ago) * time.Second)
				x.Servers[i].Answers = max(x.Servers[i].Answers, sv.Answers)
				if seen.After(x.Servers[i].Seen) {
					x.Servers[i].Seen = seen
				}
				x.Servers[i].APs = append(x.Servers[i].APs, apName{AP: id, Name: n.Name})
			}
		}
	}

	// A network that is a rogue or a suspect to any AP that hears it is one,
	// though another AP's folders know it (0106).
	rank := map[string]int{"rogue": 0, "suspect": 1, "known": 2}
	rogues := make([]*threatAP, 0, len(by))
	// The BSSIDs listed, in order, for a sibling's maker.
	heardBSSIDs := make([]string, 0, len(by))
	for b := range by {
		heardBSSIDs = append(heardBSSIDs, b)
	}
	sort.Strings(heardBSSIDs)
	for b, x := range by {
		x.Kind = "known"
		for _, k := range verdicts[b] {
			if !k {
				x.Kind = "suspect"
				break
			}
		}
		if x.Kind == "suspect" && x.Wire.State == "on" {
			x.Kind = "rogue"
		}
		sort.SliceStable(x.HeardBy, func(i, j int) bool { return x.HeardBy[i].Signal > x.HeardBy[j].Signal })
		// Who made it, by its BSSID or a sibling's heard with it.
		x.Maker, x.MakerDerived = oui.BSSIDMaker(x.BSSID, heardBSSIDs)
		rogues = append(rogues, x)
	}

	// The clients heard on them, each client and rogue once.
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := s.conds.RogueClients(since, aps, 2000)
	if err != nil {
		return err
	}
	blocked := blockedBy(c.state)
	clients := []*threatClient{}
	pair := map[string]*threatClient{}
	for _, row := range rows {
		k := row.MAC + "|" + row.BSSID
		x := pair[k]
		if x == nil {
			x = &threatClient{MAC: row.MAC, BSSID: row.BSSID, SSID: row.SSID, FirstAt: row.FirstAt, LastAt: row.LastAt, Blocked: blocked[row.MAC]}
			if r := by[row.BSSID]; r != nil {
				x.Kind = r.Kind
			}
			if cl, ok, err := s.conds.ClientOf(row.MAC); err != nil {
				return err
			} else if ok {
				x.Ours, x.Host = true, cl.Host
			}
			pair[k] = x
			clients = append(clients, x)
		}
		if row.FirstAt.Before(x.FirstAt) {
			x.FirstAt = row.FirstAt
		}
		if row.LastAt.After(x.LastAt) {
			x.LastAt = row.LastAt
		}
		x.Frames = max(x.Frames, row.Frames)
		n, _ := t.Node(row.AP)
		x.HeardBy = append(x.HeardBy, apName{AP: row.AP, Name: n.Name})
	}
	for _, x := range clients {
		if r := by[x.BSSID]; r != nil {
			r.Clients++
		}
	}
	sort.SliceStable(clients, func(i, j int) bool { return clients[i].LastAt.After(clients[j].LastAt) })
	if len(clients) > 500 {
		clients = clients[:500]
	}
	sort.Slice(rogues, func(i, j int) bool {
		if a, b := rank[rogues[i].Kind], rank[rogues[j].Kind]; a != b {
			return a < b
		}
		if a, b := rogues[i].HeardBy[0].Signal, rogues[j].HeardBy[0].Signal; a != b {
			return a > b
		}
		return rogues[i].BSSID < rogues[j].BSSID
	})

	// The networks on which more than one server answered.
	servers := []*dhcpRogue{}
	for _, x := range dhcp {
		// A server heard with no address of its own, by any AP, is the one
		// of its MAC.
		x.Servers = alerts.FoldDHCP(x.Servers, func(s dhcpRogueSeen) string { return s.ID }, func(s dhcpRogueSeen) string { return s.MAC },
			func(into *dhcpRogueSeen, from dhcpRogueSeen) {
				into.Answers += from.Answers
				into.Known = into.Known || from.Known
				if from.Seen.After(into.Seen) {
					into.Seen = from.Seen
				}
				for _, a := range from.APs {
					if !slices.Contains(into.APs, a) {
						into.APs = append(into.APs, a)
					}
				}
			})
		stranger := slices.ContainsFunc(x.Servers, func(s dhcpRogueSeen) bool { return !s.Known })
		if x.listed && stranger || !x.listed && len(x.Servers) > 1 {
			sort.Slice(x.Servers, func(i, j int) bool { return x.Servers[i].Answers > x.Servers[j].Answers })
			servers = append(servers, x)
		}
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Network < servers[j].Network })

	counts := map[string]int{"rogue": 0, "suspect": 0, "known": 0, "clients": len(clients), "dhcp": len(servers)}
	for _, x := range rogues {
		counts[x.Kind]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "rogues": rogues, "clients": clients, "dhcp": servers, "counts": counts})
	return nil
}

// networkSSIDs is each Aeolus network's SSID, by its ID, as Services sets
// it nearest the root.
func networkSSIDs(state *change.State) map[string]string {
	out := map[string]string{}
	state.Org.Services.EachSet(func(_ hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		ssid, ok := v.(string)
		rest, found := strings.CutPrefix(string(p), "network.")
		if !ok || !found {
			return
		}
		if id, f := strings.CutSuffix(rest, ".ssid"); f && !strings.Contains(id, ".") {
			if _, have := out[id]; !have {
				out[id] = ssid
			}
		}
	})
	return out
}

// blockedBy is, for each client MAC a network blocks, the networks' IDs.
func blockedBy(state *change.State) map[string][]string {
	out := map[string][]string{}
	state.Org.Services.EachSet(func(_ hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		rest, found := strings.CutPrefix(string(p), "network.")
		if !found {
			return
		}
		id, f := strings.CutSuffix(rest, ".blocked")
		if !f || strings.Contains(id, ".") {
			return
		}
		var macs []string
		switch x := v.(type) {
		case []any:
			for _, m := range x {
				if s, ok := m.(string); ok {
					macs = append(macs, s)
				}
			}
		case []string:
			macs = x
		}
		for _, m := range macs {
			m = strings.ToLower(m)
			if !slices.Contains(out[m], id) {
				out[m] = append(out[m], id)
			}
		}
	})
	return out
}
