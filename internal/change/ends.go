package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A change that ends itself (0345). A set, unset, add-folder,
// assign-services or batch of them may carry Until: at that time the
// manager makes end-change, which puts back what it did, field by field,
// wherever nothing has changed it since, and takes away the folders it
// made. A person may end it sooner the same way, or move its end with
// move-end.

// MaxUntil is the furthest a change's end may be from when it is made.
const MaxUntil = 31 * 24 * time.Hour

var (
	// ErrBadUntil refuses an end that is not to come, or too far off.
	ErrBadUntil = errors.New("a change's until is a time to come, at most 31 days off")
	// ErrNoEnd refuses an end of a change that has none to come.
	ErrNoEnd = errors.New("no such change ends by itself: it has no until, or has ended")
	// ErrUseEnd refuses end-change and move-end made as any change: the
	// manager works out what an end puts back (0345).
	ErrUseEnd = errors.New("a change is ended or its end moved through POST /v1/ends/{seq}, which works out what it puts back")
)

// endable is what a change that ends itself may be, alone or in a batch.
var endable = []Kind{Set, Unset, AddFolder, AssignServices}

// endKinds is what an end makes: what puts back each of endable.
var endKinds = []Kind{Set, Unset, AssignServices, RemoveFolder}

// Endable says whether op may carry Until.
func Endable(op Op) bool {
	if op.Kind == Batch {
		for _, sub := range op.Ops {
			if !slices.Contains(endable, sub.Kind) || !sub.Until.IsZero() {
				return false
			}
		}
		return len(op.Ops) > 0
	}
	return slices.Contains(endable, op.Kind)
}

// CheckUntil checks a new change's Until against now: only on what Endable
// says, or a move-end, at a time to come and at most MaxUntil off. Not on
// replay: what is in the log stays.
func CheckUntil(op Op, now time.Time) error {
	if op.Until.IsZero() {
		return nil
	}
	if op.Kind != MoveEnd && !Endable(op) {
		return fmt.Errorf("%w: only a set, unset, add-folder, assign-services or a batch of them ends itself, not %s", ErrBadUntil, op.Kind)
	}
	if !op.Until.After(now) || op.Until.Sub(now) > MaxUntil {
		return ErrBadUntil
	}
	return nil
}

// Step is one thing a change that ends itself did, for its end: a field at
// a node, which held Was (where Had) and was left holding Now (where Has);
// or a folder it made.
type Step struct {
	Tree   TreeName         `json:"tree"`
	Node   hierarchy.NodeID `json:"node"`
	Path   hierarchy.Path   `json:"path,omitempty"`
	Was    hierarchy.Value  `json:"was,omitempty"`
	Had    bool             `json:"had,omitempty"`
	Now    hierarchy.Value  `json:"now,omitempty"`
	Has    bool             `json:"has,omitempty"`
	Folder bool             `json:"folder,omitempty"`
}

// Undo is what op did, between before and after, as the steps its end puts
// back: each field it set or unset once, as it was before the whole change
// and as it was left, then the folders it made, the last made first. A
// field in a folder it made goes with the folder.
func Undo(before, after *State, op Op) []Step {
	ops := []Op{op}
	if op.Kind == Batch {
		ops = op.Ops
	}
	made := map[TreeName][]hierarchy.NodeID{}
	for _, o := range ops {
		if o.Kind == AddFolder {
			made[o.Tree] = append(made[o.Tree], o.Node)
		}
	}
	var steps []Step
	seen := map[string]bool{}
	field := func(tn TreeName, node hierarchy.NodeID, p hierarchy.Path) {
		key := string(tn) + "\x00" + string(node) + "\x00" + string(p)
		if seen[key] || slices.Contains(made[tn], node) {
			return
		}
		seen[key] = true
		st := Step{Tree: tn, Node: node, Path: p}
		if t, err := tree(before.Org, tn); err == nil {
			st.Was, st.Had = t.Own(node, p)
		}
		if t, err := tree(after.Org, tn); err == nil {
			st.Now, st.Has = t.Own(node, p)
		}
		steps = append(steps, st)
	}
	for _, o := range ops {
		switch o.Kind {
		case Set:
			for _, f := range o.Fields() {
				field(o.Tree, o.Node, f.Path)
			}
		case Unset:
			for _, p := range o.Unsets() {
				field(o.Tree, o.Node, p)
			}
		case AssignServices:
			field(Locations, o.Node, hierarchy.ServicesPath)
		}
	}
	for _, tn := range []TreeName{Locations, Services} {
		for i := len(made[tn]) - 1; i >= 0; i-- {
			steps = append(steps, Step{Tree: tn, Node: made[tn][i], Folder: true})
		}
	}
	return steps
}

// Ending is what puts back steps in s, now: for each field still as the
// change left it, a set of what it held, or an unset where it held
// nothing; for each folder it made, its removal. What has changed since,
// or cannot be put back, is left as it is and said in left. Each is tried
// on a copy of s in turn, so ops can be made, in order, as one.
func Ending(s *State, steps []Step) (ops []Op, left []string) {
	c := s.Clone()
	try := func(op Op, what string) {
		next, _, err := Apply(c, op)
		if err != nil {
			left = append(left, fmt.Sprintf("%s: %v", what, err))
			return
		}
		c = next
		ops = append(ops, op)
	}
	for _, st := range steps {
		where := fmt.Sprintf("%s at %s", st.Path, st.Node)
		if st.Folder {
			where = fmt.Sprintf("folder %s", st.Node)
		}
		t, err := tree(c.Org, st.Tree)
		if err != nil {
			left = append(left, fmt.Sprintf("%s: %v", where, err))
			continue
		}
		if _, ok := t.Node(st.Node); !ok {
			if !st.Folder {
				left = append(left, where+": gone")
			}
			continue
		}
		if st.Folder {
			try(Op{Kind: RemoveFolder, Tree: st.Tree, Node: st.Node}, where)
			continue
		}
		cur, has := t.Own(st.Node, st.Path)
		if has != st.Has || (has && !same(cur, st.Now)) {
			left = append(left, where+": changed since, so left as it is")
			continue
		}
		if has == st.Had && (!has || same(cur, st.Was)) {
			continue // nothing to put back
		}
		services := st.Tree == Locations && st.Path == hierarchy.ServicesPath
		switch {
		case !st.Had:
			try(Op{Kind: Unset, Tree: st.Tree, Node: st.Node, Path: st.Path}, where)
		case services:
			var list []hierarchy.NodeID
			raw, _ := json.Marshal(st.Was)
			if err := json.Unmarshal(raw, &list); err != nil {
				left = append(left, fmt.Sprintf("%s: %v", where, err))
				continue
			}
			try(Op{Kind: AssignServices, Tree: Locations, Node: st.Node, Services: list}, where)
		default:
			raw, err := json.Marshal(st.Was)
			if err != nil {
				left = append(left, fmt.Sprintf("%s: %v", where, err))
				continue
			}
			try(Op{Kind: Set, Tree: st.Tree, Node: st.Node, Path: st.Path, Value: raw}, where)
		}
	}
	return ops, left
}

// Moved is steps once move-end has set fields with it: each field it set
// is left holding what it holds in after, part of the change, and put
// back at its end as it was before the change; a field the change had not
// set is put back as it was before the move.
func Moved(steps []Step, before, after *State, op Op) []Step {
	out := slices.Clone(steps)
	t, err := tree(after.Org, op.Tree)
	if err != nil {
		return out
	}
	for _, f := range op.Fields() {
		now, has := t.Own(op.Node, f.Path)
		i := slices.IndexFunc(out, func(st Step) bool {
			return !st.Folder && st.Tree == op.Tree && st.Node == op.Node && st.Path == f.Path
		})
		if i >= 0 {
			out[i].Now, out[i].Has = now, has
			continue
		}
		st := Step{Tree: op.Tree, Node: op.Node, Path: f.Path, Now: now, Has: has}
		if b, err := tree(before.Org, op.Tree); err == nil {
			st.Was, st.Had = b.Own(op.Node, f.Path)
		}
		// Before the folders, which go last.
		j := slices.IndexFunc(out, func(st Step) bool { return st.Folder })
		if j < 0 {
			j = len(out)
		}
		out = slices.Insert(out, j, st)
	}
	return out
}

// same says whether two values are one, as the log would write them.
func same(a, b hierarchy.Value) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

// validateEnd checks end-change and move-end.
func validateEnd(op Op) error {
	if op.Ends <= 0 {
		return ErrNoEnd
	}
	if op.Kind == MoveEnd {
		if op.Until.IsZero() {
			return ErrBadUntil
		}
		if len(op.Values) > 0 && op.Node == "" {
			return ErrNoNode
		}
		return nil
	}
	if len(op.Ops) > MaxBatch {
		return fmt.Errorf("%w: an end makes at most %d changes", ErrBadBatch, MaxBatch)
	}
	for i, sub := range op.Ops {
		if !slices.Contains(endKinds, sub.Kind) || !sub.Until.IsZero() {
			return fmt.Errorf("%w: change %d of the end is %s", ErrBadBatch, i+1, sub.Kind)
		}
	}
	return nil
}
