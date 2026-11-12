package api

import (
	"context"
	"net/http"
	"sort"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// folderHold is what a folder's advisor reads (0212): node, a Locations folder,
// and what is below it, but the folders below with an advisor of their own
// (skip); and in Services, the folders the APs there use, with what is
// above and below them (services). A review of one customer's folder so
// reads nothing of another's, nor sends it under its key.
type folderHold struct {
	node     hierarchy.NodeID
	skip     map[hierarchy.NodeID]bool
	services map[hierarchy.NodeID]bool
}

// covers says whether the node at the end of chain, its ancestry from the
// root (hierarchy.Tree.Ancestry), is one w reads.
func (w *folderHold) covers(tree change.TreeName, chain []hierarchy.NodeID) bool {
	if len(chain) == 0 {
		return false
	}
	if tree == change.Services {
		return w.services[chain[len(chain)-1]]
	}
	in := false
	for _, n := range chain {
		if n == w.node {
			in = true
			continue
		}
		if in && w.skip[n] {
			return false
		}
	}
	return in
}

// holdKey is the context key of the folder a request the manager makes of
// itself is held to (0212).
type holdKey struct{}

// asHeld is As, held to w: the API answers it for what w covers alone.
func asHeld(h http.Handler, actor access.AccountID, w *folderHold) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), asKey{}, actor)
		h.ServeHTTP(rw, r.WithContext(context.WithValue(ctx, holdKey{}, w)))
	})
}

// heldOf is what a request the manager made of itself is held to, if
// anything.
func heldOf(r *http.Request) *folderHold {
	w, _ := r.Context().Value(holdKey{}).(*folderHold)
	return w
}

// advisorScopes are the folders with an advisor of their own (0212): each
// Locations node that sets system.advisor.key itself, the Org's root
// first, then the others by their place in the tree.
func advisorScopes(state *change.State) []hierarchy.NodeID {
	t := state.Org.Locations
	root := t.Root()
	var out []hierarchy.NodeID
	for _, n := range append([]hierarchy.NodeID{root}, t.Descendants(root)...) {
		if r, ok := t.Resolve(n, "system.advisor.key"); ok && r.From == n && (r.Origin == hierarchy.OriginSelf || r.Origin == hierarchy.OriginLocked) {
			out = append(out, n)
		}
	}
	return out
}

// scopeOf is the folder whose advisor reviews node (0212): the nearest at
// or above it with a key of its own, or "" where none is.
func scopeOf(state *change.State, node hierarchy.NodeID) hierarchy.NodeID {
	scopes := map[hierarchy.NodeID]bool{}
	for _, s := range advisorScopes(state) {
		scopes[s] = true
	}
	chain := state.Org.Locations.Ancestry(node)
	for i := len(chain) - 1; i >= 0; i-- {
		if scopes[chain[i]] {
			return chain[i]
		}
	}
	return ""
}

// holdOf is what the advisor of the folder node reads (0212): node
// and below, but the folders below with their own advisor; and the service
// folders the APs there are given, with their ancestors, whose settings
// they inherit, and what is below them.
func holdOf(state *change.State, node hierarchy.NodeID) *folderHold {
	L, S := state.Org.Locations, state.Org.Services
	w := &folderHold{node: node, skip: map[hierarchy.NodeID]bool{}, services: map[hierarchy.NodeID]bool{}}
	for _, s := range advisorScopes(state) {
		if s != node {
			w.skip[s] = true
		}
	}
	used := map[hierarchy.NodeID]bool{}
	for _, n := range append([]hierarchy.NodeID{node}, L.Descendants(node)...) {
		if !w.covers(change.Locations, L.Ancestry(n)) {
			continue
		}
		if r, ok := L.Resolve(n, hierarchy.ServicesPath); ok {
			if folders, ok := r.Value.([]hierarchy.NodeID); ok {
				for _, f := range folders {
					used[f] = true
				}
			}
		}
	}
	for f := range used {
		if _, ok := S.Node(f); !ok {
			continue
		}
		for _, n := range S.Ancestry(f) {
			w.services[n] = true
		}
		for _, n := range S.Descendants(f) {
			w.services[n] = true
		}
	}
	return w
}

// viewableScopes is the advisors' folders the caller may view, those
// higher in the tree first (0212).
func viewableScopes(c call) []hierarchy.NodeID {
	t := c.state.Org.Locations
	var out []hierarchy.NodeID
	for _, s := range advisorScopes(c.state) {
		if may(c, change.Locations, t, s, access.View) {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(t.Ancestry(out[i])) < len(t.Ancestry(out[j])) })
	return out
}
