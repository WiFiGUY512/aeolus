package change

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// enroll returns an enroll change for ap and the plain token it records.
func enroll(t *testing.T, ap hierarchy.NodeID) (Op, string) {
	t.Helper()
	plain, id, hash, err := access.NewAPToken()
	must(t, err)
	return Op{Kind: Enroll, Node: ap, Name: string(ap), TokenID: id, TokenHash: hash, Value: json.RawMessage(`{"mac":"02:00:00:00:00:01"}`)}, plain
}

func TestEnrollLandsInLandingZoneWithAToken(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	op, plain := enroll(t, "ap-020000000001")
	mustApply(t, s, op)

	if n, _ := s.Org.Locations.Node("ap-020000000001"); n.Parent != LandingZone || n.Kind != hierarchy.KindAP {
		t.Fatalf("enrolled AP = %+v", n)
	}
	if ap, err := s.Access.AuthenticateAP(plain); err != nil || ap != "ap-020000000001" {
		t.Fatalf("AuthenticateAP = %q, %v", ap, err)
	}
	if string(s.Facts["ap-020000000001"]) != `{"mac":"02:00:00:00:00:01"}` {
		t.Fatalf("facts = %s", s.Facts["ap-020000000001"])
	}
	if _, err := s.Access.Authenticate(plain); err == nil {
		t.Fatal("an AP token worked as an account token")
	}
}

// enrollAs returns an enroll change for ap with the facts given, and the
// plain token it records.
func enrollAs(t *testing.T, ap hierarchy.NodeID, facts string) (Op, string) {
	t.Helper()
	op, plain := enroll(t, ap)
	op.Value = json.RawMessage(facts)
	return op, plain
}

func TestAKnownAPEnrollsAgain(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	first, plain := enroll(t, "ap-1")
	mustApply(t, s, first)
	// Still in Landing Zone, it takes its entry again: nothing was given to
	// it yet (0223).
	again, plain2 := enrollAs(t, "ap-1", `{"mac":"02:00:00:00:00:01","openwrt":"again"}`)
	mustApply(t, s, again)
	if _, err := s.Access.AuthenticateAP(plain); !errors.Is(err, access.ErrBadToken) {
		t.Fatalf("the first token, after enrolling again in Landing Zone: %v", err)
	}
	if ap, err := s.Access.AuthenticateAP(plain2); err != nil || ap != "ap-1" {
		t.Fatalf("the new token in Landing Zone = %q, %v", ap, err)
	}
	// Adopted, with a setting of its own, it waits with the new token.
	mustApply(t, s, Op{Kind: Move, Tree: Locations, Node: "ap-1", Parent: "house"})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "ap-1", Path: "radio.5g.channel", Value: json.RawMessage(`"36"`)})
	reflashed, plain3 := enrollAs(t, "ap-1", `{"mac":"02:00:00:00:00:01","openwrt":"reflashed"}`)
	mustApply(t, s, reflashed)
	if ap, err := s.Access.AuthenticateAP(plain3); !errors.Is(err, access.ErrWaiting) || ap != "ap-1" {
		t.Fatalf("an adopted AP's new token = %q, %v, want ap-1 waiting", ap, err)
	}
	if _, err := s.Access.AuthenticateAP(plain2); err != nil {
		t.Fatalf("its token stopped working before a person said: %v", err)
	}
	if string(s.Facts["ap-1"]) != `{"mac":"02:00:00:00:00:01","openwrt":"again"}` || string(s.Again["ap-1"]) != `{"mac":"02:00:00:00:00:01","openwrt":"reflashed"}` {
		t.Fatalf("facts %s, again %s", s.Facts["ap-1"], s.Again["ap-1"])
	}
	// Enrolling again once more replaces the token it waits with.
	twice, plain4 := enrollAs(t, "ap-1", `{"mac":"02:00:00:00:00:01","openwrt":"twice"}`)
	mustApply(t, s, twice)
	if _, err := s.Access.AuthenticateAP(plain3); !errors.Is(err, access.ErrBadToken) {
		t.Fatalf("the token it waited with before: %v", err)
	}
	// Let back in, it is as it was, with the new token and what it said.
	if err := Authorize(s, "claude", Op{Kind: ReadmitAP, Node: "ap-1"}); err != nil {
		t.Fatalf("claude letting it back in: %v", err)
	}
	if err := Authorize(s, "office", Op{Kind: ReadmitAP, Node: "ap-1"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("office letting it back in: %v", err)
	}
	_, eff, err := Apply(s, Op{Kind: ReadmitAP, Node: "ap-1"})
	must(t, err)
	if after, _ := eff.After.(map[string]any); after["openwrt"] != "twice" {
		t.Fatalf("effect = %+v", eff)
	}
	if ap, err := s.Access.AuthenticateAP(plain4); err != nil || ap != "ap-1" {
		t.Fatalf("its new token, let back in = %q, %v", ap, err)
	}
	if _, err := s.Access.AuthenticateAP(plain2); !errors.Is(err, access.ErrBadToken) {
		t.Fatalf("its old token, once let back in: %v", err)
	}
	if n, _ := s.Org.Locations.Node("ap-1"); n.Parent != "house" || len(s.Again) != 0 || string(s.Facts["ap-1"]) != `{"mac":"02:00:00:00:00:01","openwrt":"twice"}` {
		t.Fatalf("let back in: %+v, again %v, facts %s", n, s.Again, s.Facts["ap-1"])
	}
	if v, ok := s.Org.Locations.Own("ap-1", "radio.5g.channel"); !ok || v != "36" {
		t.Fatalf("its own setting: %v %v", v, ok)
	}
	if _, _, err := Apply(s, Op{Kind: ReadmitAP, Node: "ap-1"}); !errors.Is(err, access.ErrNoWaiting) {
		t.Fatalf("letting back in one that waits for nothing: %v", err)
	}
}

func TestReplaceAPKeepsItsPlace(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	old, oldToken := enrollAs(t, "ap-1", `{"mac":"02:00:00:00:00:01","model":"Arista C-260","board":"arista,c-260"}`)
	mustApply(t, s, old)
	mustApply(t, s, Op{Kind: Move, Tree: Locations, Node: "ap-1", Parent: "house"})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "ap-1", Path: "radio.5g.channel", Value: json.RawMessage(`"36"`)})
	waiting, _ := enrollAs(t, "ap-1", `{"mac":"02:00:00:00:00:01"}`)
	mustApply(t, s, waiting) // the old box, enrolled again: its wait ends too
	box, boxToken := enrollAs(t, "ap-2", `{"mac":"02:00:00:00:00:02","model":"Arista C-360","board":"arista,c-360"}`)
	mustApply(t, s, box)
	for _, c := range []struct {
		op   Op
		want error
	}{
		{Op{Kind: ReplaceAP, Node: "ap-1"}, ErrNoWith},
		{Op{Kind: ReplaceAP, Node: "ap-2", With: "ap-2"}, ErrReplaceItself},
		{Op{Kind: ReplaceAP, Node: "ap-1", With: "house"}, hierarchy.ErrNotFound},
		{Op{Kind: ReplaceAP, Node: "ap-2", With: "ap-1"}, ErrReplaceAdopted},
	} {
		if _, _, err := Apply(s, c.op); !errors.Is(err, c.want) {
			t.Errorf("%+v: %v, want %v", c.op, err, c.want)
		}
	}
	if err := Authorize(s, "office", Op{Kind: ReplaceAP, Node: "ap-1", With: "ap-2"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("office replacing: %v", err)
	}
	_, eff, err := Apply(s, Op{Kind: ReplaceAP, Node: "ap-1", With: "ap-2"})
	must(t, err)
	before, _ := eff.Before.(map[string]any)
	after, _ := eff.After.(map[string]any)
	if before["mac"] != "02:00:00:00:00:01" || before["model"] != "Arista C-260" || after["mac"] != "02:00:00:00:00:02" || after["from"] != "ap-2" {
		t.Fatalf("effect = %+v", eff)
	}
	if _, ok := s.Org.Locations.Node("ap-2"); ok {
		t.Fatal("the new AP's own entry stayed")
	}
	if n, _ := s.Org.Locations.Node("ap-1"); n.Parent != "house" || n.Name != "ap-1" {
		t.Fatalf("its place = %+v", n)
	}
	if v, ok := s.Org.Locations.Own("ap-1", "radio.5g.channel"); !ok || v != "36" {
		t.Fatalf("its own setting: %v %v", v, ok)
	}
	if ap, err := s.Access.AuthenticateAP(boxToken); err != nil || ap != "ap-1" {
		t.Fatalf("the new box's token = %q, %v, want ap-1", ap, err)
	}
	if _, err := s.Access.AuthenticateAP(oldToken); !errors.Is(err, access.ErrBadToken) {
		t.Fatalf("the old box's token: %v", err)
	}
	if string(s.Facts["ap-1"]) != `{"mac":"02:00:00:00:00:02","model":"Arista C-360","board":"arista,c-360"}` || len(s.Again) != 0 {
		t.Fatalf("facts %s, again %v", s.Facts["ap-1"], s.Again)
	}
	if _, ok := s.Facts["ap-2"]; ok {
		t.Fatal("the new AP's own facts stayed")
	}
	// One waiting in Landing Zone has no place to take.
	third, _ := enroll(t, "ap-3")
	mustApply(t, s, third)
	fourth, _ := enroll(t, "ap-4")
	mustApply(t, s, fourth)
	if _, _, err := Apply(s, Op{Kind: ReplaceAP, Node: "ap-3", With: "ap-4"}); !errors.Is(err, ErrReplaceWaiting) {
		t.Fatalf("replacing one in Landing Zone: %v", err)
	}
}

func TestAFailedEnrollmentLeavesNoAP(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	first, _ := enroll(t, "ap-1")
	mustApply(t, s, first)
	clash, _ := enroll(t, "ap-2")
	clash.TokenID = first.TokenID
	if _, _, err := Apply(s, clash); !errors.Is(err, access.ErrExists) {
		t.Fatalf("token ID clash: %v", err)
	}
	if _, ok := s.Org.Locations.Node("ap-2"); ok {
		t.Fatal("the AP stayed after its token was refused")
	}
}

func TestRemoveAPRevokesItsToken(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	op, plain := enroll(t, "ap-1")
	mustApply(t, s, op)
	mustApply(t, s, Op{Kind: Move, Tree: Locations, Node: "ap-1", Parent: "house"})
	mustApply(t, s, Op{Kind: Set, Tree: Locations, Node: "ap-1", Path: "radio.5g.channel", Value: json.RawMessage(`"36"`)})

	_, eff, err := Apply(s, Op{Kind: RemoveAP, Node: "ap-1"})
	must(t, err)
	if len(eff.Removed) != 1 || eff.Removed[0].Path != "radio.5g.channel" {
		t.Fatalf("effect = %+v", eff)
	}
	if _, ok := s.Org.Locations.Node("ap-1"); ok {
		t.Fatal("AP still in the tree")
	}
	if _, err := s.Access.AuthenticateAP(plain); !errors.Is(err, access.ErrBadToken) {
		t.Fatalf("token after removal: %v", err)
	}
	if _, ok := s.Facts["ap-1"]; ok {
		t.Fatal("facts kept after removal")
	}
	// Removed, it may enroll again, into Landing Zone.
	again, _ := enroll(t, "ap-1")
	mustApply(t, s, again)
	if _, _, err := Apply(s, Op{Kind: RemoveAP, Node: "house"}); !errors.Is(err, hierarchy.ErrNotAnAP) {
		t.Fatalf("removing a folder: %v", err)
	}
}

func TestWhoMayEnrollAndRemove(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	op, _ := enroll(t, "ap-1")
	if err := Authorize(s, "griff", op); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an account enrolling an AP: %v", err)
	}
	remove := Op{Kind: RemoveAP, Node: "gate-ap"}
	// gatekeeper is admin on Main Gate, where the AP is; office has no role
	// in Locations.
	for actor, ok := range map[string]bool{"gatekeeper": true, "office": false, "claude": true, "griff": true} {
		if err := Authorize(s, actor, remove); (err == nil) != ok {
			t.Errorf("%s removing gate-ap: %v, want ok=%v", actor, err, ok)
		}
	}
}

func TestLandingZoneHasALimit(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	for i := 0; i < LandingZoneLimit; i++ {
		mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: hierarchy.NodeID(fmt.Sprintf("ap-%d", i)), Parent: LandingZone})
	}
	op, _ := enroll(t, "ap-one-more")
	if err := Authorize(s, SystemActor, op); !errors.Is(err, ErrFull) {
		t.Fatalf("enrolling into a full Landing Zone: %v", err)
	}
	mustApply(t, s, Op{Kind: RemoveAP, Node: "ap-0"})
	if err := Authorize(s, SystemActor, op); err != nil {
		t.Fatalf("enrolling after one was removed: %v", err)
	}
}
