package api

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/compose"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// fleetOwn is what the APs together call their own (0105): every BSSID an
// AP's latest report names as its own, and every SSID of Aeolus's networks,
// as an AP hears it (alerts.Heard); and every address an AP said it has,
// and every BSSID, by its middle four bytes, which a BSSID made from it
// shares (0160).
func (s *Server) fleetOwn(state *change.State) *alerts.Fleet {
	f := &alerts.Fleet{BSSIDs: map[string]bool{}, SSIDs: map[string]bool{}, Addrs: map[string]bool{}}
	for b := range s.fleetBSSIDs(state) {
		f.BSSIDs[b] = true
		if m := alerts.Middle(b); m != "" {
			f.Addrs[m] = true
		}
	}
	for _, raw := range state.Facts {
		var facts struct {
			MAC  string   `json:"mac"`
			MACs []string `json:"macs"`
		}
		if json.Unmarshal(raw, &facts) != nil {
			continue
		}
		for _, m := range append(facts.MACs, facts.MAC) {
			if m := alerts.Middle(m); m != "" {
				f.Addrs[m] = true
			}
		}
	}
	state.Org.Services.EachSet(func(_ hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		if ssid, ok := v.(string); ok && strings.HasPrefix(string(p), "network.") && strings.HasSuffix(string(p), ".ssid") {
			f.SSIDs[alerts.Heard(ssid)] = true
		}
	})
	return f
}

// fleetBSSIDs is every BSSID an AP's latest report names as its own, and
// the AP's.
func (s *Server) fleetBSSIDs(state *change.State) map[string]hierarchy.NodeID {
	out := map[string]hierarchy.NodeID{}
	for _, id := range state.Org.Locations.APs() {
		st, err := s.conds.LatestState(id)
		if err != nil || st == nil {
			continue
		}
		var r struct {
			BSSIDs []string `json:"bssids"`
		}
		if json.Unmarshal(st.Report, &r) == nil {
			for _, b := range r.BSSIDs {
				out[b] = id
			}
		}
	}
	return out
}

// fleetWide is what the alerts of every AP are worked out from that is
// worked out across the APs at once: the networks failing their client
// tests, the joins that go nowhere, the clients that rejoin, and those
// heard on a rogue.
type fleetWide struct {
	tests     map[hierarchy.NodeID][]alerts.FailingTest
	joins     map[hierarchy.NodeID][]alerts.FailingJoins
	rejoins   map[hierarchy.NodeID][]alerts.Rejoining
	struggles map[hierarchy.NodeID][]alerts.Struggling
	slow      map[hierarchy.NodeID][]alerts.SlowRadio
	onRogue   map[hierarchy.NodeID][]alerts.OnRogue
	bleSilent map[hierarchy.NodeID][]alerts.BLESilent
}

func (s *Server) fleetWide(state *change.State, now time.Time) fleetWide {
	return fleetWide{tests: s.failingTests(state, now), joins: s.failingJoins(state, now), rejoins: s.rejoining(state, now),
		struggles: s.struggling(state, now), slow: s.slowRadios(state, now), onRogue: s.onRogues(state, now), bleSilent: s.bleSilent(now)}
}

// bleSilent is, by the AP that heard each last, the Bluetooth devices
// people named that no AP has heard for alerts.BLESilentAfter, and for no
// longer than alerts.BLESilentFor (0220).
func (s *Server) bleSilent(now time.Time) map[hierarchy.NodeID][]alerts.BLESilent {
	out := map[hierarchy.NodeID][]alerts.BLESilent{}
	named, err := s.conds.BluetoothNamed()
	if err != nil {
		return out
	}
	for _, n := range named {
		if quiet := now.Sub(n.LastAt); n.Heard && quiet >= alerts.BLESilentAfter && quiet < alerts.BLESilentFor {
			out[n.LastAP] = append(out[n.LastAP], alerts.BLESilent{MAC: n.MAC, Name: n.Name, LastAt: n.LastAt})
		}
	}
	return out
}

// onRogues is, by AP, the clients the APs serve that its scan radio heard
// sending to a rogue or a suspect in the last OnRogueFor (0160): not the
// APs' own BSSIDs, nor those its folders know, nor the APs' test clients.
// A client is one of the APs' where the manager has seen one of them
// serve it.
func (s *Server) onRogues(state *change.State, now time.Time) map[hierarchy.NodeID][]alerts.OnRogue {
	out := map[hierarchy.NodeID][]alerts.OnRogue{}
	rows, err := s.conds.RogueClients(now.Add(-alerts.OnRogueFor), state.Org.Locations.APs(), 500)
	if err != nil || len(rows) == 0 {
		return out
	}
	own := testClients(state)
	fleet := s.fleetBSSIDs(state)
	known := map[hierarchy.NodeID]map[string]bool{}
	who := map[string]string{} // by MAC: how a client of the APs' is called; "" for one that is not
	for _, row := range rows {
		mac, bssid := strings.ToLower(row.MAC), strings.ToLower(row.BSSID)
		if _, test := own[mac]; test {
			continue
		}
		if _, ours := fleet[bssid]; ours {
			continue
		}
		if known[row.AP] == nil {
			known[row.AP] = knownRogues(state, row.AP)
		}
		if known[row.AP][bssid] {
			continue
		}
		w, seen := who[mac]
		if !seen {
			if cl, ok, err := s.conds.ClientOf(mac); err == nil && ok {
				w = mac
				if n, named, err := s.conds.ClientNamed(mac); err == nil && named && n.Name != "" {
					w = n.Name + " (" + mac + ")"
				} else if cl.Host != "" {
					w = cl.Host + " (" + mac + ")"
				}
			}
			who[mac] = w
		}
		if w == "" {
			continue
		}
		first := row.FirstAt
		out[row.AP] = append(out[row.AP], alerts.OnRogue{MAC: mac, Who: w, BSSID: bssid, SSID: row.SSID, Since: &first})
	}
	return out
}

// dhcpServers is, by network, the DHCP servers each of an AP's networks
// says it has (0160), in lower case.
func dhcpServers(state *change.State, id hierarchy.NodeID) map[string][]string {
	out := map[string][]string{}
	cfg, err := state.Org.ResolveAP(id)
	if err != nil {
		return out
	}
	for net, n := range cfg.Networks {
		r, ok := n.Fields["dhcp_servers"]
		if !ok {
			continue
		}
		switch v := r.Value.(type) {
		case []any:
			for _, x := range v {
				if s, ok := x.(string); ok {
					out[net] = append(out[net], strings.ToLower(s))
				}
			}
		case []string:
			for _, s := range v {
				out[net] = append(out[net], strings.ToLower(s))
			}
		}
	}
	return out
}

// knownRogues is the BSSIDs an AP's folders say are no rogues, by
// rogues.known (0106), in lower case as the APs report them.
func knownRogues(state *change.State, id hierarchy.NodeID) map[string]bool {
	out := map[string]bool{}
	cfg, err := state.Org.ResolveAP(id)
	if err != nil {
		return out
	}
	r, ok := cfg.Location["rogues.known"]
	if !ok {
		return out
	}
	switch v := r.Value.(type) {
	case []any:
		for _, b := range v {
			if s, ok := b.(string); ok {
				out[strings.ToLower(s)] = true
			}
		}
	case []string:
		for _, b := range v {
			out[strings.ToLower(b)] = true
		}
	}
	return out
}

// alertsOf is what needs attention on one AP now (0099).
func (s *Server) alertsOf(state *change.State, id hierarchy.NodeID, now time.Time, fleet *alerts.Fleet, w fleetWide) ([]alerts.Alert, error) {
	n, _ := state.Org.Locations.Node(id)
	version, _ := s.log.Version(id)
	res, err := s.compose(state, id, s.reveal)
	if err != nil {
		return nil, err
	}
	l, err := s.conds.Latest(id)
	if err != nil {
		return nil, err
	}
	poll := time.Minute
	if sys, _ := res.Doc["system"].(map[string]any); sys != nil {
		if p, ok := sys["poll"].(float64); ok && p > 0 {
			poll = time.Duration(p) * time.Second
		}
	}
	// The agent bundle it should run, as the fleet view has it (0079).
	wants := ""
	if s.agents != nil {
		if b, _, ok := s.agentFor(state, id); ok {
			wants = b.Hash
		}
	}
	in := alerts.Input{
		AP: id, Name: n.Name, Now: now, Poll: poll, Unassigned: res.Unassigned,
		Version: version, Since: s.log.VersionSince(id), WantsAgent: wants, Problems: res.Problems, Latest: l, Fleet: fleet,
		Known: knownRogues(state, id), Tests: w.tests[id], Joins: w.joins[id], Rejoins: w.rejoins[id], Struggles: w.struggles[id], SlowRadios: w.slow[id], OnRogue: w.onRogue[id], BLESilent: w.bleSilent[id],
		DHCPServers: dhcpServers(state, id), Again: againFrom(state, id), Reports: s.reports.of(id, now),
	}
	in.Zone, in.OnClock, in.ByZone = s.localTimes(state, id)
	// When its radios' newest minutes began (0311).
	if radios, last, err := s.conds.LastMinutes(id, now.Add(-alerts.MinutesLooked)); err != nil {
		slog.Warn("alerts: minutes not looked up", "ap", id, "err", err)
	} else {
		in.Minutes = &alerts.Minutes{Radios: radios, Last: last}
	}
	// The clients pinned here that seem to have moved (0340).
	in.PinsMoved = s.pinsMoved(state, id, res.Doc, now)
	// What its radios' beacons came to over the last half hour (0339).
	if b, err := s.conds.BeaconsSince(id, now.Add(-alerts.BeaconsWindow)); err != nil {
		slog.Warn("alerts: beacons not looked up", "ap", id, "err", err)
	} else {
		in.Beacons = b
	}
	// What its uplink ports' minutes came to over the last hour (0351).
	if u, err := s.conds.UplinkMinutes(id, now.Add(-alerts.UplinkLossWindow), uplinkMinutesMost); err != nil {
		slog.Warn("alerts: uplink minutes not looked up", "ap", id, "err", err)
	} else {
		in.Uplink = u
	}
	as := alerts.For(in)
	s.nameClients(as)
	if said, on := maintenanceOf(state, id, now); on {
		for i := range as {
			as[i].Maintenance = said
		}
	}
	return as, nil
}

// localWindow is a window an AP's config may run by local hours (0262):
// what it is called; whether it is on; its window's field, its blocks by day
// where it has them, and whether an unset window is the field's default or
// no window at all; and whether the AP's own clock keeps it, or the manager
// does by system.tz.
type localWindow struct {
	what            string
	on              func(get func(hierarchy.Path) any) bool
	window, windows hierarchy.Path
	byDefault       bool
	onClock         bool
}

var localWindows = []localWindow{
	{what: "RRM's planned moves", window: "rrm.window", byDefault: true, onClock: true,
		on: func(get func(hierarchy.Path) any) bool {
			return get("rrm.enabled") == true && get("rrm.moves") != false
		}},
	{what: "the client tests on a schedule", window: "assurance.tests.window", windows: "assurance.tests.windows",
		on: func(get func(hierarchy.Path) any) bool {
			n := number(get("assurance.tests.every"))
			return n != nil && *n > 0 && get("assurance.tests.paused") != true
		}},
}

// localTimes is an AP's zone, system.tz, and what its config has run by
// local hours (0262): by the AP's own clock, and by the manager for it.
func (s *Server) localTimes(state *change.State, id hierarchy.NodeID) (zone string, onClock, byZone []string) {
	cfg, err := state.Org.ResolveAP(id)
	if err != nil || cfg.Unassigned {
		return "", nil, nil
	}
	get := func(p hierarchy.Path) any {
		if r, ok := cfg.Location[p]; ok {
			return r.Value
		}
		return nil
	}
	zone, _ = get("system.tz").(string)
	for _, w := range localWindows {
		if !w.on(get) {
			continue
		}
		hours := textList(get(w.windows))
		if v, _ := get(w.window).(string); len(v) == 11 && v[:5] == v[6:] {
			// One that starts where it ends is the whole day.
		} else if v != "" {
			hours = append([]string{v}, hours...)
		} else if w.byDefault && len(hours) == 0 && s.schema != nil {
			if d, ok := s.schema.Default(w.window); ok {
				if v, _ := d.(string); v != "" {
					hours = []string{v}
				}
			}
		}
		if len(hours) == 0 {
			continue
		}
		said := w.what + " (" + strings.Join(hours, ", ") + ")"
		if w.onClock {
			onClock = append(onClock, said)
		} else {
			byZone = append(byZone, said)
		}
	}
	return zone, onClock, byZone
}

// againFrom is where an AP that enrolled again, and waits to be let back in,
// enrolled from (0223); "" for one that does not wait.
func againFrom(state *change.State, id hierarchy.NodeID) string {
	raw, ok := state.Again[id]
	if !ok {
		return ""
	}
	var f facts
	json.Unmarshal(raw, &f)
	return cmp.Or(f.Source, "an address not known")
}

// maintenanceOf says whether an AP is in maintenance now (0218), as its
// folders' maintenance.* fields say, and how: in a test window (0344) or
// not, why, and until when.
func maintenanceOf(state *change.State, id hierarchy.NodeID, now time.Time) (string, bool) {
	m, ok := apMaintenance(state, id)
	if !ok || !m.In(now) {
		return "", false
	}
	said := "in maintenance"
	if m.Window() {
		said = "in a test window, held still"
	}
	if !m.Until.IsZero() {
		said += " until " + m.Until.Format("2006-01-02 15:04") + " UTC"
	}
	if m.Why != "" {
		said += ": " + m.Why
	}
	return said, true
}

// apMaintenance is an AP's maintenance as its folders say (0218, 0344).
func apMaintenance(state *change.State, id hierarchy.NodeID) (compose.Maintenance, bool) {
	cfg, err := state.Org.ResolveAP(id)
	if err != nil || cfg.Unassigned {
		return compose.Maintenance{}, false
	}
	return compose.MaintenanceOf(func(p string) any {
		if r, ok := cfg.Location[hierarchy.Path(p)]; ok {
			return r.Value
		}
		return nil
	}), true
}

// heldAt says whether an AP holds still at now, in a test window (0344):
// the manager offers it no other agent and asks it for no client test on a
// schedule, nor any other AP for one of its networks, until it ends.
func heldAt(state *change.State, id hierarchy.NodeID, now time.Time) bool {
	m, ok := apMaintenance(state, id)
	return ok && m.Held(now)
}

// macIn is a MAC in an alert's message.
var macIn = regexp.MustCompile(`[0-9a-f]{2}(?::[0-9a-f]{2}){5}`)

// nameClients calls each client an alert says by its MAC alone by the name
// a person gave it, with its MAC after, as the rest of the manager does
// (0153): in the list, the history and what is sent. A MAC already in
// brackets after a name, or part of a longer run of hex, is left; so is one
// no one named, as a rogue's BSSID.
func (s *Server) nameClients(as []alerts.Alert) {
	named := map[string]string{}
	hex := func(c byte) bool { return c == ':' || c >= '0' && c <= '9' || c >= 'a' && c <= 'f' }
	for i := range as {
		msg := as[i].Message
		var b strings.Builder
		last := 0
		for _, at := range macIn.FindAllStringIndex(msg, -1) {
			from, to := at[0], at[1]
			if from > 0 && (msg[from-1] == '(' || hex(msg[from-1])) || to < len(msg) && hex(msg[to]) {
				continue
			}
			mac := msg[from:to]
			name, ok := named[mac]
			if !ok {
				if n, has, err := s.conds.ClientNamed(mac); err == nil && has {
					name = n.Name
				}
				named[mac] = name
			}
			if name == "" {
				continue
			}
			b.WriteString(msg[last:from])
			b.WriteString(name + " (" + mac + ")")
			last = to
		}
		if last > 0 {
			b.WriteString(msg[last:])
			as[i].Message = b.String()
		}
		// In its few words (0330), the name alone.
		if mac := macIn.FindString(as[i].Short); mac != "" {
			name, ok := named[mac]
			if !ok {
				if n, has, err := s.conds.ClientNamed(mac); err == nil && has {
					name = n.Name
				}
				named[mac] = name
			}
			if name != "" {
				as[i].Short = strings.Replace(as[i].Short, mac, name, 1)
			}
		}
	}
}

// alertsList is what needs attention on the APs the caller may view (0099),
// below one Locations node where ?under= names it: GET /v1/alerts. It is
// worked out on each call from what the manager has, so an alert ends when
// its cause does.
func (s *Server) alertsList(w http.ResponseWriter, r *http.Request, c call) error {
	t := c.state.Org.Locations
	under := hierarchy.NodeID(r.URL.Query().Get("under"))
	now := time.Now()
	fleet := s.fleetOwn(c.state)
	wide := s.fleetWide(c.state, now)
	out := []alerts.Alert{}
	for _, id := range t.APs() {
		if !may(c, change.Locations, t, id, access.View) {
			continue
		}
		if under != "" && under != id && !slices.Contains(t.Ancestry(id), under) {
			continue
		}
		as, err := s.alertsOf(c.state, id, now, fleet, wide)
		if err != nil {
			return err
		}
		out = append(out, as...)
	}
	// Since is when the alert began, where the alert log has it earlier:
	// what a report says of some causes is when they were last seen, as a
	// rogue heard at the latest scan, not when they began (0109).
	if open, err := s.conds.OpenAlerts(); err == nil {
		began := map[string]time.Time{}
		for _, a := range open {
			began[string(a.AP)+"|"+a.Key] = a.Began
		}
		for i, a := range out {
			if b, ok := began[string(a.AP)+"|"+a.Key]; ok && (a.Since == nil || b.Before(*a.Since)) {
				out[i].Since = &b
			}
		}
	}
	rank := map[string]int{alerts.Critical: 0, alerts.Warning: 1, alerts.Info: 2}
	// Those of APs in maintenance come last, and are counted apart (0218):
	// they need no one's attention.
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := out[i].Maintenance != "", out[j].Maintenance != ""; a != b {
			return b
		}
		if a, b := rank[out[i].Severity], rank[out[j].Severity]; a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	counts := map[string]int{alerts.Critical: 0, alerts.Warning: 0, alerts.Info: 0, "maintenance": 0}
	for _, a := range out {
		if a.Maintenance != "" {
			counts["maintenance"]++
			continue
		}
		counts[a.Severity]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out, "counts": counts})
	return nil
}

// notifyTarget is where an AP's alerts go, as the notify.* fields of its
// folders say, nearest first (0101), the sealed URLs opened.
func (s *Server) notifyTarget(state *change.State, id hierarchy.NodeID) alerts.Target {
	cfg, err := state.Org.ResolveAP(id)
	if err != nil {
		return alerts.Target{}
	}
	// The URLs are sealed (0027): opened here, to send, and nowhere else.
	str := func(path string) string {
		r, ok := cfg.Location[hierarchy.Path(path)]
		if !ok {
			return ""
		}
		v, err := s.reveal(path, r.Value)
		if err != nil {
			slog.Warn("alerts: a notify setting cannot be opened", "path", path, "err", err)
			return ""
		}
		out, _ := v.(string)
		return out
	}
	_, held := maintenanceOf(state, id, time.Now())
	t := alerts.Target{Ntfy: str("notify.ntfy"), Webhook: str("notify.webhook"), Resolved: true, Maintenance: held}
	if r, ok := cfg.Location["notify.severity"]; ok {
		t.Least, _ = r.Value.(string)
	}
	if r, ok := cfg.Location["notify.resolved"]; ok {
		if b, ok := r.Value.(bool); ok {
			t.Resolved = b
		}
	}
	return t
}

// Notify sends alerts where each AP's folders say (0101), and keeps the
// alert log (0109), looking every interval until ctx ends. A send that
// fails is logged and not tried again.
func (s *Server) Notify(ctx context.Context, every time.Duration) {
	n := alerts.NewNotifier()
	rec := newAlertRecorder()
	client := &http.Client{Timeout: 10 * time.Second}
	look := func() {
		now := time.Now()
		state := s.log.Snapshot()
		var all []alerts.Alert
		targets := map[hierarchy.NodeID]alerts.Target{}
		fleet := s.fleetOwn(state)
		wide := s.fleetWide(state, now)
		for _, id := range state.Org.Locations.APs() {
			as, err := s.alertsOf(state, id, now, fleet, wide)
			if err != nil {
				slog.Warn("alerts: cannot work out an AP's", "ap", id, "err", err)
				continue
			}
			all = append(all, as...)
			targets[id] = s.notifyTarget(state, id)
		}
		s.recordAlerts(rec, now, all)
		for _, e := range n.Step(now, all, func(a alerts.Alert) alerts.Target { return targets[a.AP] }) {
			if err := alerts.Send(ctx, client, e); err != nil {
				slog.Warn("alerts: not sent", "ap", e.Alert.Name, "event", e.Kind, "key", e.Alert.Key, "err", err)
			} else {
				slog.Info("alerts: sent", "ap", e.Alert.Name, "event", e.Kind, "key", e.Alert.Key)
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
