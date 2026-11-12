package api

import (
	"cmp"
	"encoding/json"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/journey"
)

// Addresses set by hand and addresses two clients hold, in a client's
// journey (0325).

// subnetsFor is how far back the client tests are read for each network's
// subnets: a week, as a network's addressing seldom changes.
const subnetsFor = 7 * 24 * time.Hour

// holdersFresh is how recent a report is that says who holds an address
// now: an AP reports every minute.
const holdersFresh = 3 * time.Minute

// subnetsOf is each network's subnets as the APs' client tests since a time
// were given them by DHCP (0189): by the AP the test joined, for a wired
// test the AP itself, and by the network on any AP. A mask under 8 bits is
// taken for no mask.
func (s *Server) subnetsOf(since time.Time) (journey.Subnets, error) {
	tests, err := s.tests(since)
	if err != nil {
		return nil, err
	}
	type key struct {
		ap      hierarchy.NodeID
		network string
	}
	byAP, byNet := map[key][]netip.Prefix{}, map[string][]netip.Prefix{}
	add := func(list []netip.Prefix, p netip.Prefix) []netip.Prefix {
		if slices.Contains(list, p) {
			return list
		}
		return append(list, p)
	}
	for _, r := range tests {
		d := r.detail
		if d == nil || d.DHCP == nil || r.ask.Network == "" || d.DHCP.Mask < 8 || d.DHCP.Mask > 32 {
			continue
		}
		a, err := netip.ParseAddr(d.DHCP.Address)
		if err != nil || !a.Is4() {
			continue
		}
		p, err := a.Prefix(d.DHCP.Mask)
		if err != nil {
			continue
		}
		on := hierarchy.NodeID(r.ask.On)
		if on == "" {
			on = hierarchy.NodeID(r.AP)
		}
		k := key{on, r.ask.Network}
		byAP[k] = add(byAP[k], p)
		byNet[r.ask.Network] = add(byNet[r.ask.Network], p)
	}
	return func(ap hierarchy.NodeID, network string) []netip.Prefix {
		if l := byAP[key{ap, network}]; len(l) > 0 {
			return l
		}
		return byNet[network]
	}, nil
}

// holdersOf is the address a client holds now, by its latest session where
// that is current, and the other clients on its network that hold it now
// too: each in the latest report of an AP the caller may view, of the last
// few minutes. An AP's report before its latest says nothing of now.
func (s *Server) holdersOf(c call, mac string, j journey.Journey, now time.Time) (string, []journey.Holder, error) {
	if len(j.Sessions) == 0 {
		return "", nil, nil
	}
	last := j.Sessions[len(j.Sessions)-1]
	if _, err := netip.ParseAddr(last.Address); err != nil || now.Sub(last.To) > holdersFresh {
		return "", nil, nil
	}
	latest := map[hierarchy.NodeID]conditions.State{}
	if err := s.conds.StatesWith(now.Add(-holdersFresh), last.Address, func(ap hierarchy.NodeID, st conditions.State) error {
		latest[ap] = st
		return nil
	}); err != nil {
		return "", nil, err
	}
	t := c.state.Org.Locations
	var out []journey.Holder
	for _, ap := range slices.Sorted(maps.Keys(latest)) {
		if !may(c, change.Locations, t, ap, access.View) {
			continue
		}
		st := latest[ap]
		if newest, err := s.conds.LatestState(ap); err != nil {
			return "", nil, err
		} else if newest == nil || !newest.At.Equal(st.At) {
			continue
		}
		var rep struct {
			Clients []journey.Client `json:"clients"`
		}
		if json.Unmarshal(st.Report, &rep) != nil {
			continue
		}
		name := string(ap)
		if n, ok := t.Node(ap); ok {
			name = n.Name
		}
		for _, cl := range rep.Clients {
			if cl.Address != last.Address || strings.EqualFold(cl.MAC, mac) || (last.Network != "" && cl.Network != "" && cl.Network != last.Network) {
				continue
			}
			out = append(out, journey.Holder{MAC: strings.ToLower(cl.MAC), Host: cl.Host, AP: string(ap), APName: name})
		}
	}
	slices.SortFunc(out, func(a, b journey.Holder) int {
		return cmp.Or(cmp.Compare(a.APName, b.APName), cmp.Compare(a.MAC, b.MAC))
	})
	return last.Address, out, nil
}
