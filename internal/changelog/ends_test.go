package changelog

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// window is last night's test window as one change that ends itself
// (0345): a services folder of its own below household, a network in it
// switched off, the AP taking it beside iot, maintenance and a channel pin
// on the AP, and the AP's 5 GHz width, set above, set at it.
func window(until time.Time) change.Op {
	L, S := change.Locations, change.Services
	return change.Op{Kind: change.Batch, Until: until, Ops: []change.Op{
		{Kind: change.AddFolder, Tree: S, Node: "w-window", Name: "Window", Parent: "household"},
		setOp(S, "w-window", "network.sweet.enabled", false),
		{Kind: change.AssignServices, Node: "office-ap", Services: []hierarchy.NodeID{"iot", "w-window"}},
		{Kind: change.Set, Tree: L, Node: "office-ap", Values: map[hierarchy.Path]json.RawMessage{
			"maintenance.on": js(true), "maintenance.until": js("2026-09-25T13:00Z"), "maintenance.hold": js(true),
		}},
		setOp(L, "office-ap", "radio.5g.channel", "140"),
		setOp(L, "office-ap", "radio.2g.channel", "6"),
	}}
}

// own is every field set at the APs and folders the window touches, and
// whether its folder is there.
func own(l *Log) map[string]any {
	s := l.Snapshot()
	out := map[string]any{}
	for _, p := range []hierarchy.Path{"services", "maintenance.on", "maintenance.until", "maintenance.hold", "radio.5g.channel", "radio.2g.channel"} {
		if v, ok := s.Org.Locations.Own("office-ap", p); ok {
			out[string(p)] = v
		}
	}
	_, out["folder"] = s.Org.Services.Node("w-window")
	return out
}

func TestAChangeThatEndsItselfPutsBackWhatItDid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aeolus.db")
	l := openAt(t, path)
	commitAll(t, l, scenario())
	before := own(l)
	until := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
	w := commit(t, l, window(until))

	ends := l.Ends()
	if len(ends) != 1 || ends[0].Seq != w.Seq || !ends[0].Until.Equal(until) {
		t.Fatalf("ends = %+v, want change %d until %s", ends, w.Seq, until)
	}
	if due := l.Due(until.Add(-time.Minute)); len(due) != 0 {
		t.Fatalf("due a minute early: %v", due)
	}
	if due := l.Due(until); !reflect.DeepEqual(due, []int64{w.Seq}) {
		t.Fatalf("due at until = %v", due)
	}

	// Replayed, the end to come is still there.
	l.Close()
	l = openAt(t, path)
	if ends := l.Ends(); len(ends) != 1 || ends[0].Seq != w.Seq {
		t.Fatalf("after replay, ends = %+v", ends)
	}

	e, left, err := l.End(change.SystemActor, "", w.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("left = %v, want nothing", left)
	}
	if e.Op.Kind != change.EndChange || e.Op.Ends != w.Seq || !strings.Contains(e.Reason, "at its until") {
		t.Errorf("end = %+v %q", e.Op, e.Reason)
	}
	if got := own(l); !reflect.DeepEqual(got, before) {
		t.Errorf("after the end:\n got %v\nwant %v", got, before)
	}
	if ends := l.Ends(); len(ends) != 0 {
		t.Errorf("ends after the end = %+v", ends)
	}
	if _, _, err := l.End(change.SystemActor, "", w.Seq); !errors.Is(err, change.ErrNoEnd) {
		t.Errorf("ended twice: %v", err)
	}

	// Replayed, it stays ended, and the Org as it was put back.
	l.Close()
	l = openAt(t, path)
	if ends := l.Ends(); len(ends) != 0 {
		t.Errorf("after replay, ends = %+v", ends)
	}
	if got := own(l); !reflect.DeepEqual(got, before) {
		t.Errorf("after replay:\n got %v\nwant %v", got, before)
	}
}

func TestAnEndLeavesWhatChangedSince(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	w := commit(t, l, window(time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)))
	commit(t, l, setOp(change.Locations, "office-ap", "radio.5g.channel", "149"))

	_, left, err := l.End("griff", "done early", w.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || !strings.Contains(left[0], "radio.5g.channel at office-ap: changed since") {
		t.Errorf("left = %v", left)
	}
	s := l.Snapshot()
	if v, _ := s.Org.Locations.Own("office-ap", "radio.5g.channel"); v != "149" {
		t.Errorf("5g channel = %v, want 149 kept", v)
	}
	if _, ok := s.Org.Locations.Own("office-ap", "radio.2g.channel"); ok {
		t.Error("2g channel still pinned")
	}
	if _, ok := s.Org.Services.Node("w-window"); ok {
		t.Error("the window's folder is still there")
	}
}

func TestAnEndCanBeMovedAndIsMadeOnlyByTheLog(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	until := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
	w := commit(t, l, window(until))

	later := until.Add(time.Hour)
	if _, err := l.MoveEnd("griff", "", w.Seq, later, nil); err != nil {
		t.Fatal(err)
	}
	if ends := l.Ends(); len(ends) != 1 || !ends[0].Until.Equal(later) {
		t.Fatalf("ends = %+v, want until %s", ends, later)
	}
	if _, err := l.MoveEnd("griff", "", w.Seq, time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC), nil); !errors.Is(err, change.ErrBadUntil) {
		t.Errorf("moved into the past: %v", err)
	}
	if _, err := l.MoveEnd("griff", "", w.Seq+1, later, nil); !errors.Is(err, change.ErrNoEnd) {
		t.Errorf("moved a change with no end: %v", err)
	}
	for _, op := range []change.Op{
		{Kind: change.EndChange, Ends: w.Seq},
		{Kind: change.MoveEnd, Ends: w.Seq, Until: later},
	} {
		if _, err := l.Commit("griff", "", op); !errors.Is(err, change.ErrUseEnd) {
			t.Errorf("%s committed as a change: %v", op.Kind, err)
		}
	}
}

func TestUntilIsCheckedWhenMade(t *testing.T) {
	l := open(t)
	commitAll(t, l, scenario())
	set := setOp(change.Locations, "office-ap", "radio.5g.channel", "140")
	for name, op := range map[string]change.Op{
		"past":      func() change.Op { o := set; o.Until = time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC); return o }(),
		"too far":   func() change.Op { o := set; o.Until = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC); return o }(),
		"lock":      {Kind: change.Lock, Tree: change.Locations, Node: "office-ap", Path: "radio.5g.width", Until: time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)},
		"own until": {Kind: change.Batch, Ops: []change.Op{func() change.Op { o := set; o.Until = time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC); return o }()}},
	} {
		if _, err := l.Commit("claude", "", op); !errors.Is(err, change.ErrBadUntil) {
			t.Errorf("%s: %v, want ErrBadUntil", name, err)
		}
	}
	if len(l.Ends()) != 0 {
		t.Errorf("ends = %+v", l.Ends())
	}
}

// One hour more for a test window: its end moved, and maintenance.until
// with it, part of the change, so both go at its end; and a field the
// window had not set, set with the move, goes too.
func TestAMovedEndTakesWhatItSetWithIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aeolus.db")
	l := openAt(t, path)
	commitAll(t, l, scenario())
	before := own(l)
	until := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
	w := commit(t, l, window(until))
	set := &change.Op{Tree: change.Locations, Node: "office-ap", Values: map[hierarchy.Path]json.RawMessage{
		"maintenance.until": js("2026-09-25T14:00Z"), "maintenance.why": js("one hour more"),
	}}
	if _, err := l.MoveEnd("griff", "", w.Seq, until.Add(time.Hour), set); err != nil {
		t.Fatal(err)
	}
	if v, _ := l.Snapshot().Org.Locations.Own("office-ap", "maintenance.until"); v != "2026-09-25T14:00Z" {
		t.Fatalf("maintenance.until = %v", v)
	}
	l.Close()
	l = openAt(t, path) // the move replays with what it set
	_, left, err := l.End(change.SystemActor, "", w.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("left = %v", left)
	}
	if got := own(l); !reflect.DeepEqual(got, before) {
		t.Errorf("after the end:\n got %v\nwant %v", got, before)
	}
	if _, ok := l.Snapshot().Org.Locations.Own("office-ap", "maintenance.why"); ok {
		t.Error("maintenance.why, set with the move, is still set")
	}
}
