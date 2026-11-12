package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What replace-ap refuses (0223).
var (
	ErrReplaceItself  = errors.New("an AP cannot take its own place")
	ErrReplaceAdopted = errors.New("only an AP waiting in Landing Zone can take another's place: one adopted has records of its own")
	ErrReplaceWaiting = errors.New("an AP waiting in Landing Zone has no place to take: adopt the new one instead")
)

// applyAP handles an AP arriving and leaving (0033, 0038).
//
// enroll places a new AP in Landing Zone with its token and the facts it sent.
// An AP already known enrolls again (0223), as after a reflash: see
// enrollAgain. Nobody takes over an adopted AP's identity by enrolling in its
// name: its new token gets it nothing until a person lets it back in.
//
// remove-ap takes an AP out of the Locations tree, with whatever was set on
// it, and revokes its tokens.
//
// facts replaces what an AP said of itself with what it says now (0122).
// Nothing else of it changes: its place, its name and its token stay.
//
// readmit-ap lets an AP that enrolled again back in (0223): the token it
// waited with is its own, and what it said then its facts. replace-ap puts
// an AP waiting in Landing Zone in another's place: the other keeps its ID,
// name, place, settings and records, and takes the new AP's token and facts;
// the new AP's own entry goes.
func applyAP(s *State, op Op) (Effect, error) {
	t := s.Org.Locations
	switch op.Kind {
	case Enroll:
		if n, ok := t.Node(op.Node); ok && n.Kind == hierarchy.KindAP {
			return enrollAgain(s, op)
		}
		if err := t.AddAP(op.Node, op.Name, LandingZone); err != nil {
			return Effect{}, err
		}
		if err := s.Access.AddAPToken(op.TokenID, op.Node, op.TokenHash); err != nil {
			t.RemoveAP(op.Node) // leave the state as it was
			return Effect{}, err
		}
		if len(bytes.TrimSpace(op.Value)) > 0 {
			s.Facts[op.Node] = append([]byte(nil), op.Value...)
		}
		return Effect{After: map[string]string{"name": op.Name, "parent": string(LandingZone), "token": op.TokenID}}, nil
	case Facts:
		if n, ok := t.Node(op.Node); !ok || n.Kind != hierarchy.KindAP {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
		}
		was := s.Facts[op.Node]
		s.Facts[op.Node] = append([]byte(nil), op.Value...)
		return factsEffect(was, op.Value), nil
	case RemoveAP:
		n, ok := t.Node(op.Node)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
		}
		removed, err := t.RemoveAP(op.Node)
		if err != nil {
			return Effect{}, err
		}
		s.Access.RevokeAPTokens(op.Node)
		delete(s.Facts, op.Node)
		delete(s.Again, op.Node)
		return Effect{Before: map[string]string{"name": n.Name, "parent": string(n.Parent)}, Removed: removed}, nil
	case ReadmitAP:
		if n, ok := t.Node(op.Node); !ok || n.Kind != hierarchy.KindAP {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
		}
		again, ok := s.Again[op.Node]
		if !ok {
			return Effect{}, access.ErrNoWaiting
		}
		if err := s.Access.LetAPBackIn(op.Node); err != nil {
			return Effect{}, err
		}
		was := s.Facts[op.Node]
		s.Facts[op.Node] = again
		delete(s.Again, op.Node)
		return factsEffect(was, again), nil
	case ReplaceAP:
		return replaceAP(s, op)
	}
	return Effect{}, fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

// enrollAgain is an AP the manager knows enrolling again (0223): it lost its
// token, as to a reflash or a reset. One still waiting in Landing Zone takes
// its own entry again, the new token in place of the old: nothing was given
// to it, and adopting it is a person's call already. One adopted keeps its
// place and its token, and waits with the new one, and with what it said of
// itself, until a person lets it back in (readmit-ap).
func enrollAgain(s *State, op Op) (Effect, error) {
	if s.Org.Locations.InIsolated(op.Node) {
		s.Access.RevokeAPTokens(op.Node)
		if err := s.Access.AddAPToken(op.TokenID, op.Node, op.TokenHash); err != nil {
			return Effect{}, err
		}
		if len(bytes.TrimSpace(op.Value)) > 0 {
			s.Facts[op.Node] = append([]byte(nil), op.Value...)
		}
		return Effect{After: map[string]string{"again": "in Landing Zone", "token": op.TokenID}}, nil
	}
	if err := s.Access.AddWaitingAPToken(op.TokenID, op.Node, op.TokenHash); err != nil {
		return Effect{}, err
	}
	if s.Again == nil {
		s.Again = map[hierarchy.NodeID]json.RawMessage{}
	}
	s.Again[op.Node] = append([]byte(nil), op.Value...)
	return Effect{After: map[string]string{"again": "waits to be let back in", "token": op.TokenID}}, nil
}

// replaceAP puts op.With, an AP waiting in Landing Zone, in op.Node's place
// (0223). op.Node keeps its ID and everything kept under it; it takes op.With's
// token and what op.With said of itself, its own tokens stop working, and
// op.With's entry goes, with anything set on it.
func replaceAP(s *State, op Op) (Effect, error) {
	t := s.Org.Locations
	n, ok := t.Node(op.Node)
	if !ok || n.Kind != hierarchy.KindAP {
		return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
	}
	w, ok := t.Node(op.With)
	if !ok || w.Kind != hierarchy.KindAP {
		return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.With)
	}
	switch {
	case op.With == op.Node:
		return Effect{}, ErrReplaceItself
	case !t.InIsolated(op.With):
		return Effect{}, fmt.Errorf("%w: %s is adopted already", ErrReplaceAdopted, w.Name)
	case t.InIsolated(op.Node):
		return Effect{}, fmt.Errorf("%w: %s waits in Landing Zone itself", ErrReplaceWaiting, n.Name)
	}
	removed, err := t.RemoveAP(op.With)
	if err != nil {
		return Effect{}, err
	}
	s.Access.HandAPTokens(op.With, op.Node)
	before := hardware(s.Facts[op.Node])
	after := hardware(s.Facts[op.With])
	after["from"], after["from_name"] = string(op.With), w.Name
	s.Facts[op.Node] = s.Facts[op.With]
	delete(s.Facts, op.With)
	delete(s.Again, op.Node)
	delete(s.Again, op.With)
	return Effect{Before: before, After: after, Removed: removed}, nil
}

// hardware is what an AP's facts say of the box: its MAC, model and board,
// those it has.
func hardware(facts json.RawMessage) map[string]any {
	var f struct{ MAC, Model, Board string }
	json.Unmarshal(facts, &f)
	out := map[string]any{}
	for k, v := range map[string]string{"mac": f.MAC, "model": f.Model, "board": f.Board} {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// removeFolder takes away a folder that holds nothing (0125): no folder and
// no AP below it, which the tree sees to, and nothing that still leans on
// it. A Services folder some Locations node offers stays, and one with
// per-user keys (0070); a Locations folder AP templates are made at stays
// (0085); and Landing Zone stays (0032, 0333). What was set at it goes with
// it, and the roles granted on it.
func removeFolder(s *State, op Op) (Effect, error) {
	t, err := tree(s.Org, op.Tree)
	if err != nil {
		return Effect{}, err
	}
	n, ok := t.Node(op.Node)
	if !ok {
		return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
	}
	// Landing Zone is built in, in Locations alone, so a Services folder
	// that happens to share its ID is a folder like any; and Sandbox is
	// built in no more (0333).
	if op.Tree == Locations && op.Node == LandingZone {
		return Effect{}, fmt.Errorf("%w: %s is a built-in folder", hierarchy.ErrNotAFolder, op.Node)
	}
	var by []string
	if op.Tree == Services {
		offers := func(at hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
			if list, ok := v.([]hierarchy.NodeID); ok && p == hierarchy.ServicesPath && slices.Contains(list, op.Node) && !slices.Contains(by, string(at)) {
				by = append(by, string(at))
			}
		}
		// Offered where it is set, or kept from before a break (0005).
		s.Org.Locations.EachSet(offers)
		s.Org.Locations.EachBase(offers)
		for _, k := range s.Keys.All() {
			if k.Folder == op.Node {
				by = append(by, "per-user key "+k.Name)
			}
		}
	} else {
		for _, tm := range s.Library.Templates() {
			if tm.At == op.Node {
				by = append(by, "template "+tm.Name)
			}
		}
	}
	if len(by) > 0 {
		sort.Strings(by)
		return Effect{}, &InUseError{What: "folder " + n.Name, By: by}
	}
	removed, err := t.RemoveFolder(op.Node)
	if err != nil {
		return Effect{}, err
	}
	for _, g := range s.Access.Grants() {
		if g.Tree == string(op.Tree) && g.Node == op.Node {
			s.Access.Revoke(g)
		}
	}
	return Effect{Before: map[string]string{"name": n.Name, "parent": string(n.Parent)}, Removed: removed}, nil
}

// factsEffect is what an AP said of itself that changed (0122): each of its
// facts, as hostname, openwrt or radios, that is not what it was, before
// and after; one it no longer says is in before alone, a new one in after.
// Nothing, where neither reads as an object.
func factsEffect(was, now []byte) Effect {
	var a, b map[string]json.RawMessage
	if json.Unmarshal(was, &a) != nil || json.Unmarshal(now, &b) != nil || b == nil {
		return Effect{}
	}
	before, after := map[string]any{}, map[string]any{}
	for k, v := range a {
		if w, ok := b[k]; !ok || !sameJSON(v, w) {
			before[k] = decodeRaw(v)
		}
	}
	for k, w := range b {
		if v, ok := a[k]; !ok || !sameJSON(v, w) {
			after[k] = decodeRaw(w)
		}
	}
	var e Effect
	if len(before) > 0 {
		e.Before = before
	}
	if len(after) > 0 {
		e.After = after
	}
	return e
}

// sameJSON says whether two JSON values are the same, however spaced.
func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	p, _ := json.Marshal(x)
	q, _ := json.Marshal(y)
	return bytes.Equal(p, q)
}

func decodeRaw(v json.RawMessage) any {
	var x any
	json.Unmarshal(v, &x)
	return x
}
