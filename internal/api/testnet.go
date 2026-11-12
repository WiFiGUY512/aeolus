package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/compose"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// checkTestNetwork vets switching a test network on (0179), when the change
// is made: a network and an until together, the network one of the test
// networks the folders there offer, until ahead and no further than
// compose.TestNetworkMax. Switching one off is always allowed. A replay of
// the log checks nothing: an until long past is only a network off. Each of a
// batch's changes is vetted as one alone (0203).
func checkTestNetwork(state *change.State, op change.Op, now time.Time) error {
	for _, sub := range op.Ops {
		if err := checkTestNetwork(state, sub, now); err != nil {
			return err
		}
	}
	if op.Kind != change.Set || op.Tree != change.Locations {
		return nil
	}
	vals := map[string]json.RawMessage{}
	if op.Path != "" {
		vals[string(op.Path)] = op.Value
	}
	for p, v := range op.Values {
		vals[string(p)] = v
	}
	rawNet, hasNet := vals[compose.TestNetworkPath+".network"]
	rawUntil, hasUntil := vals[compose.TestNetworkPath+".until"]
	if !hasNet && !hasUntil {
		return nil
	}
	if !hasNet || !hasUntil {
		return badRequest("a test network is switched on with its network and its until together (0179)")
	}
	var id string
	var until int64
	if json.Unmarshal(rawNet, &id) != nil || json.Unmarshal(rawUntil, &until) != nil {
		return nil // the schema says what is wrong
	}
	if until <= now.Unix() || until > now.Add(compose.TestNetworkMax+time.Minute).Unix() {
		return badRequest("a test network is switched on until a time ahead, at most %s from now (0179)", compose.TestNetworkMax)
	}
	if !testNetworkOffered(state, op.Node, id) {
		return badRequest("%s is not a test network the folders here offer: one is a network with test set (0179)", id)
	}
	return nil
}

// testNetworkOffered says whether network id is a test network one of the
// Services folders a Locations node takes offers.
func testNetworkOffered(state *change.State, node hierarchy.NodeID, id string) bool {
	for _, n := range testNetworksAt(state, node) {
		if n.ID == id {
			return true
		}
	}
	return false
}

// testNet is a test network offered at a Locations node: its ID and SSID.
type testNet struct {
	ID   string `json:"id"`
	SSID string `json:"ssid"`
}

// testNetworksAt is the test networks the Services folders a Locations node
// takes offer (0179), by ID.
func testNetworksAt(state *change.State, node hierarchy.NodeID) []testNet {
	out := []testNet{}
	r, ok := state.Org.Locations.Resolve(node, "services")
	if !ok {
		return out
	}
	folders := serviceIDs(r.Value)
	seen := map[string]bool{}
	for _, f := range folders {
		for p, r := range state.Org.Services.ResolveAll(f) {
			id, ok := strings.CutSuffix(strings.TrimPrefix(string(p), "network."), ".test")
			if !strings.HasPrefix(string(p), "network.") || !ok || strings.Contains(id, ".") || seen[id] || !isTrue(r.Value) {
				continue
			}
			seen[id] = true
			ssid := id
			if s, ok := state.Org.Services.Resolve(f, hierarchy.Path(fmt.Sprintf("network.%s.ssid", id))); ok {
				if v, ok := s.Value.(string); ok {
					ssid = v
				}
			}
			out = append(out, testNet{ID: id, SSID: ssid})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// isTrue says whether a resolved value is true.
func isTrue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case json.RawMessage:
		var b bool
		return json.Unmarshal(x, &b) == nil && b
	}
	return false
}

// serviceIDs reads the service folders a Locations node's services field
// names, in whichever form the value is held.
func serviceIDs(v hierarchy.Value) []hierarchy.NodeID {
	folders := []hierarchy.NodeID{}
	switch v := v.(type) {
	case []hierarchy.NodeID:
		folders = append(folders, v...)
	case []any:
		for _, f := range v {
			if s, ok := f.(string); ok {
				folders = append(folders, hierarchy.NodeID(s))
			}
		}
	case []string:
		for _, s := range v {
			folders = append(folders, hierarchy.NodeID(s))
		}
	case json.RawMessage:
		var s []string
		_ = json.Unmarshal(v, &s)
		for _, x := range s {
			folders = append(folders, hierarchy.NodeID(x))
		}
	}
	return folders
}

// placement is a Locations folder as a Services folder's page sees it
// (0320): the service folders it takes, and the node that says so, its own
// or one above it; none where nothing does.
type placement struct {
	Node     hierarchy.NodeID   `json:"node"`
	Services []hierarchy.NodeID `json:"services"`
	From     hierarchy.NodeID   `json:"from,omitempty"`
}

// placements is every Locations folder the caller may view, the Org first,
// with the service folders each takes: where a Services folder's networks
// go, and where it could be added. APs and isolated folders take none of
// their own here.
func placements(c call) []placement {
	loc := c.state.Org.Locations
	out := []placement{}
	for _, id := range append([]hierarchy.NodeID{loc.Root()}, loc.Descendants(loc.Root())...) {
		n, ok := loc.Node(id)
		if !ok || n.Kind == hierarchy.KindAP || n.Isolated || loc.InIsolated(id) || !may(c, change.Locations, loc, id, access.View) {
			continue
		}
		p := placement{Node: id, Services: []hierarchy.NodeID{}}
		if r, ok := loc.Resolve(id, hierarchy.ServicesPath); ok {
			p.Services = serviceIDs(r.Value)
			p.From = r.From
		}
		out = append(out, p)
	}
	return out
}
