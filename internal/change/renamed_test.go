package change

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A renamed board goes by its new name (0174): what was set under its old
// name, a template's boards, templates.<board> and boards.<board>., is the
// new name's, and an AP that reports either name takes it all. No second
// template is made for the new name.
func TestRenamedBoards(t *testing.T) {
	s := templateOrg(t) // its C-360 reports the old name, arista,c360
	mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: "c360new", Name: "C360 renamed", Parent: "barn"})
	s.Facts["c360new"] = json.RawMessage(`{"board":"arista,c-360","model":"Arista C-360"}`)

	// set under the old name, as a manager's log holds it
	mustApply(t, s, Op{Kind: AddTemplate, Template: "c360", Name: "C-360", Parent: "example", Boards: []string{"arista,c360"}, Default: true})
	mustApply(t, s, Op{Kind: SetTemplate, Template: "c360", Path: "radio.6g.width", Value: raw(`80`)})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "house", Path: BoardPath("arista,c360", "ports.eth1.mode"), Value: raw(`"trunk"`)})

	if tm, _ := s.Library.Template("c360"); !reflect.DeepEqual(tm.Boards, []string{"arista,c-360"}) {
		t.Errorf("the template's boards: %v", tm.Boards)
	}
	if _, set := s.Org.Locations.Own("example", "templates.arista,c-360"); !set {
		t.Error("the pick is not under the new name")
	}
	for _, old := range []hierarchy.Path{"templates.arista,c360", BoardPath("arista,c360", "ports.eth1.mode")} {
		for _, n := range []hierarchy.NodeID{"example", "house"} {
			if _, set := s.Org.Locations.Own(n, old); set {
				t.Errorf("%s is left set on %s", old, n)
			}
		}
	}
	for _, ap := range []hierarchy.NodeID{"c360", "c360new"} {
		if got := s.Board(ap); got != "arista,c-360" {
			t.Errorf("%s's board: %q", ap, got)
		}
		if tm, at, ok := s.TemplateFor(ap); !ok || tm.ID != "c360" || at != "example" {
			t.Errorf("%s takes %+v at %s", ap, tm, at)
		}
		if got := resolved(t, s, ap, "radio.6g.width").Value; got != float64(80) {
			t.Errorf("%s's template value: %v", ap, got)
		}
		if got := resolved(t, s, ap, "ports.eth1.mode").Value; got != "trunk" {
			t.Errorf("%s's own kind's setting: %v", ap, got)
		}
	}
	if ops := NewKinds(s); len(ops) != 0 {
		t.Errorf("a template made for the new name: %+v", ops)
	}

	// A change under either name lands on the same field.
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "house", Path: "templates.arista,c-360", Value: raw(`"c360"`)})
	if _, set := s.Org.Locations.Own("house", "templates.arista,c-360"); !set {
		t.Fatal("the pick under the new name is not set")
	}
	mustApply(t, s, Op{Kind: Unset, Tree: Locations, Node: "house", Path: "templates.arista,c360"})
	if _, set := s.Org.Locations.Own("house", "templates.arista,c-360"); set {
		t.Error("unset under the old name leaves the new name's set")
	}
}

// The renaming takes a change's paths and boards, and leaves the change
// itself as it was said: the log keeps that.
func TestCurrentBoards(t *testing.T) {
	for in, want := range map[hierarchy.Path]hierarchy.Path{
		"templates.arista,c360":                "templates.arista,c-360",
		"boards.arista,c360.ports.eth1.mode":   "boards.arista,c-360.ports.eth1.mode",
		"boards.arista,w318.ports.lan1.mode":   "boards.arista,w-318.ports.lan1.mode",
		"templates.arista,c-360":               "templates.arista,c-360",
		"boards.netgear,r7800.ports.lan4.mode": "boards.netgear,r7800.ports.lan4.mode",
		"radio.6g.width":                       "radio.6g.width",
		"system.templates":                     "system.templates",
	} {
		if got := currentBoardPath(in); got != want {
			t.Errorf("currentBoardPath(%s) = %s, want %s", in, got, want)
		}
	}

	vals := map[hierarchy.Path]json.RawMessage{
		"templates.arista,c360":  raw(`"old"`),
		"templates.arista,c-360": raw(`"new"`),
		"templates.arista,c260":  raw(`"c260"`),
		"radio.5g.width":         raw(`40`),
	}
	paths := []hierarchy.Path{"boards.arista,w318.ports.lan1.mode", "boards.arista,w-318.ports.lan1.mode", "radio.5g.width"}
	boards := []string{"arista,c260", "arista,c-260", "netgear,r7800"}
	op := Op{Kind: Set, Values: vals, Paths: paths, Boards: boards, Path: "templates.arista,w318"}
	got := currentBoards(op)
	if got.Path != "templates.arista,w-318" {
		t.Errorf("path: %s", got.Path)
	}
	if want := map[hierarchy.Path]json.RawMessage{
		"templates.arista,c-360": raw(`"new"`), // the new name's value, set beside the old's
		"templates.arista,c-260": raw(`"c260"`),
		"radio.5g.width":         raw(`40`),
	}; !reflect.DeepEqual(got.Values, want) {
		t.Errorf("values: %v", got.Values)
	}
	if want := []hierarchy.Path{"boards.arista,w-318.ports.lan1.mode", "radio.5g.width"}; !reflect.DeepEqual(got.Paths, want) {
		t.Errorf("paths: %v", got.Paths)
	}
	if want := []string{"arista,c-260", "netgear,r7800"}; !reflect.DeepEqual(got.Boards, want) {
		t.Errorf("boards: %v", got.Boards)
	}
	if len(vals) != 4 || len(paths) != 3 || paths[0] != "boards.arista,w318.ports.lan1.mode" || boards[0] != "arista,c260" || op.Path != "templates.arista,w318" {
		t.Error("the change as said was altered")
	}
}
