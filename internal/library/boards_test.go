package library

import (
	"reflect"
	"testing"
)

// A renamed board goes by its new name (0174); any other board as it is.
func TestBoardNames(t *testing.T) {
	for in, want := range map[string]string{
		"arista,c360":   "arista,c-360",
		"arista,c-360":  "arista,c-360",
		"arista,c260":   "arista,c-260",
		"arista,w318":   "arista,w-318",
		"netgear,r7800": "netgear,r7800",
		"":              "",
	} {
		if got := Board(in); got != want {
			t.Errorf("Board(%q) = %q, want %q", in, got, want)
		}
	}
	got := Boards([]string{"arista,c360", "netgear,r7800", "arista,c-360", "arista,w318"})
	if want := []string{"arista,c-360", "netgear,r7800", "arista,w-318"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Boards = %v, want %v", got, want)
	}
	if Boards(nil) != nil {
		t.Error("Boards(nil) is not nil")
	}
	// A board renamed twice goes by its newest name from either earlier one.
	renamed["x,a"], renamed["x,b"] = "x,b", "x,c"
	defer func() { delete(renamed, "x,a"); delete(renamed, "x,b") }()
	if Board("x,a") != "x,c" || Board("x,b") != "x,c" {
		t.Errorf("a board renamed twice: %q, %q", Board("x,a"), Board("x,b"))
	}
	tm := Template{Boards: []string{"arista,c360"}}
	if !tm.ForBoard("arista,c-360") || !tm.ForBoard("arista,c360") || tm.ForBoard("arista,c-260") {
		t.Error("ForBoard does not take a renamed board by either name")
	}
}
