package changelog

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/change"
)

// Pending is a change that ends itself (0345), not ended yet: when, and
// what its end puts back.
type Pending struct {
	Seq    int64         `json:"seq"`
	At     time.Time     `json:"at"`
	Actor  string        `json:"actor"`
	Reason string        `json:"reason,omitempty"`
	Until  time.Time     `json:"until"`
	Steps  []change.Step `json:"steps"`
	op     change.Op
}

// track keeps the ends to come as each change is made or replayed: a
// change with until is one, until an end of it is made; a move-end moves
// it. before is the state the change was made on, after the one it left.
func (l *Log) track(e Entry, before, after *change.State) {
	switch {
	case e.Op.Kind == change.EndChange:
		delete(l.ends, e.Op.Ends)
	case e.Op.Kind == change.MoveEnd:
		if p := l.ends[e.Op.Ends]; p != nil {
			p.Until = e.Op.Until
			if len(e.Op.Values) > 0 && before != nil {
				p.Steps = change.Moved(p.Steps, before, after, e.Op)
			}
		}
	case !e.Op.Until.IsZero() && before != nil:
		op := e.Op
		op.Until = time.Time{}
		l.ends[e.Seq] = &Pending{Seq: e.Seq, At: e.At, Actor: e.Actor, Reason: e.Reason, Until: e.Op.Until, Steps: change.Undo(before, after, e.Op), op: op}
	}
}

// Ends lists the changes that end themselves and have not ended yet, the
// soonest first.
func (l *Log) Ends() []Pending {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Pending, 0, len(l.ends))
	for _, p := range l.ends {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Until.Equal(out[j].Until) {
			return out[i].Until.Before(out[j].Until)
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

// Due lists the changes whose end has come by now, the soonest first.
func (l *Log) Due(now time.Time) []int64 {
	var due []int64
	for _, p := range l.Ends() {
		if !p.Until.After(now) {
			due = append(due, p.Seq)
		}
	}
	return due
}

// End ends change seq now: it puts back what the change did wherever
// nothing has changed it since, and takes away the folders it made, as one
// change, end-change, in actor's name. What it leaves is said in the
// reason, and returned. A person must be allowed the change itself again,
// and what its end makes; the manager ends a change at its until.
func (l *Log) End(actor, reason string, seq int64) (Entry, []string, error) {
	if actor == "" {
		return Entry{}, nil, ErrNoActor
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.ends[seq]
	if p == nil {
		return Entry{}, nil, fmt.Errorf("%w: %d", change.ErrNoEnd, seq)
	}
	if err := l.mayRedo(actor, p); err != nil {
		return Entry{}, nil, err
	}
	ops, left := change.Ending(l.state, p.Steps)
	if reason == "" {
		reason = fmt.Sprintf("the end of change %d", seq)
		if actor == change.SystemActor {
			reason = fmt.Sprintf("the end of change %d, at its until", seq)
		}
	}
	if len(left) > 0 {
		reason += "; left as they are: " + strings.Join(left, "; ")
	}
	e, err := l.commit(actor, reason, change.Op{Kind: change.EndChange, Ends: seq, Ops: ops})
	return e, left, err
}

// MoveEnd gives change seq another end, until, in actor's name: one who
// may make the change again. With it, set may set fields of one node, as
// part of the change: put back at its end as they were before it.
func (l *Log) MoveEnd(actor, reason string, seq int64, until time.Time, set *change.Op) (Entry, error) {
	if actor == "" {
		return Entry{}, ErrNoActor
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.ends[seq]
	if p == nil {
		return Entry{}, fmt.Errorf("%w: %d", change.ErrNoEnd, seq)
	}
	op := change.Op{Kind: change.MoveEnd, Ends: seq, Until: until.UTC()}
	if err := change.CheckUntil(op, l.now()); err != nil {
		return Entry{}, err
	}
	if err := l.mayRedo(actor, p); err != nil {
		return Entry{}, err
	}
	if set != nil {
		s := change.Op{Kind: change.Set, Tree: set.Tree, Node: set.Node, Values: set.Values}
		if l.check != nil {
			// As a set of them: rights, schema and all.
			if err := l.check(l.state, actor, s); err != nil {
				return Entry{}, err
			}
		}
		op.Tree, op.Node, op.Values = s.Tree, s.Node, s.Values
	}
	if reason == "" {
		reason = fmt.Sprintf("change %d now ends %s", seq, op.Until.Format(time.RFC3339))
	}
	return l.commit(actor, reason, op)
}

// mayRedo says whether a person may make p's change again, as its end or a
// new end of it takes. The manager may end what is due.
func (l *Log) mayRedo(actor string, p *Pending) error {
	if actor == change.SystemActor || l.check == nil {
		return nil
	}
	return change.Authorize(l.state, actor, p.op)
}
