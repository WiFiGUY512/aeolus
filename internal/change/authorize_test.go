package change

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// people builds an Org with a few accounts and roles:
//   - griff:  admin at both roots (from create-org)
//   - claude: operator at both roots
//   - office: operator on Services › Household, viewer at the Services root
//   - gatekeeper: admin on Locations › Main Gate only
func people(t *testing.T) *State {
	t.Helper()
	s, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "example", Name: "Example", Account: "griff"})
	must(t, err)
	L, S := Locations, Services
	for _, op := range []Op{
		{Kind: AddFolder, Tree: L, Node: "house", Name: "House", Parent: "example"},
		{Kind: AddFolder, Tree: L, Node: "gate", Name: "Main Gate", Parent: "example"},
		{Kind: AddAP, Tree: L, Node: "gate-ap", Name: "GateOpenWrt", Parent: "gate"},
		{Kind: AddFolder, Tree: S, Node: "household", Name: "Household", Parent: "example"},
		{Kind: AddFolder, Tree: S, Node: "guest", Name: "Guest", Parent: "example"},
		{Kind: Set, Tree: L, Node: "example", Path: "system.poll", Value: json.RawMessage(`60`)},
		{Kind: Lock, Tree: L, Node: "example", Path: "system.poll"},
		{Kind: AddAccount, Account: "claude", Name: "Claude"},
		{Kind: AddAccount, Account: "office", Name: "Leasing office"},
		{Kind: AddAccount, Account: "gatekeeper", Name: "Gate admin"},
		{Kind: GrantRole, Account: "claude", Tree: L, Node: "example", Role: "operator"},
		{Kind: GrantRole, Account: "claude", Tree: S, Node: "example", Role: "operator"},
		{Kind: GrantRole, Account: "office", Tree: S, Node: "household", Role: "operator"},
		{Kind: GrantRole, Account: "office", Tree: S, Node: "example", Role: "viewer"},
		{Kind: GrantRole, Account: "gatekeeper", Tree: L, Node: "gate", Role: "admin"},
	} {
		mustApply(t, s, op)
	}
	return s
}

func TestAuthorize(t *testing.T) {
	s := people(t)
	L, S := Locations, Services
	set := func(tree TreeName, node hierarchy.NodeID, path hierarchy.Path) Op {
		return Op{Kind: Set, Tree: tree, Node: node, Path: path, Value: json.RawMessage(`1`)}
	}
	cases := []struct {
		name  string
		actor string
		op    Op
		ok    bool
	}{
		{"operator sets below its grant", "claude", set(L, "house", "radio.2g.width"), true},
		{"no role in this tree", "office", set(L, "house", "radio.2g.width"), false},
		{"operator on its folder", "office", set(S, "household", "network.sweet.ssid"), true},
		{"viewer cannot set", "office", set(S, "guest", "network.guest.ssid"), false},
		{"operator cannot lock", "claude", Op{Kind: Lock, Tree: L, Node: "house", Path: "system.tz"}, false},
		{"admin locks", "griff", Op{Kind: Lock, Tree: L, Node: "house", Path: "system.tz"}, true},
		{"break needs admin where the escaped lock is", "gatekeeper", Op{Kind: BreakHierarchy, Tree: L, Node: "gate"}, false},
		{"org admin breaks", "griff", Op{Kind: BreakHierarchy, Tree: L, Node: "gate"}, true},
		{"add under a folder needs operator on it", "office", Op{Kind: AddFolder, Tree: S, Node: "b", Name: "B", Parent: "household"}, true},
		{"move needs operator on the new parent too", "office", Op{Kind: Move, Tree: S, Node: "household", Parent: "guest"}, false},
		{"assign services needs Locations operator", "office", Op{Kind: AssignServices, Node: "house", Services: []hierarchy.NodeID{"household"}}, false},
		{"assign services with both roles", "claude", Op{Kind: AssignServices, Node: "house", Services: []hierarchy.NodeID{"household"}}, true},
		{"an operator may not add accounts", "claude", Op{Kind: AddAccount, Account: "x", Name: "X"}, false},
		// Only an owner runs the Org's people (0176): griff, from create-org.
		{"a folder admin may not add accounts", "gatekeeper", Op{Kind: AddAccount, Account: "x", Name: "X"}, false},
		{"the owner adds accounts", "griff", Op{Kind: AddAccount, Account: "x", Name: "X"}, true},
		{"the owner disables one", "griff", Op{Kind: DisableAccount, Account: "office"}, true},
		{"a folder admin may not disable one", "gatekeeper", Op{Kind: DisableAccount, Account: "office"}, false},
		{"nor remove one", "gatekeeper", Op{Kind: RemoveAccount, Account: "office"}, false},
		{"nor make an owner", "gatekeeper", Op{Kind: SetOwner, Account: "gatekeeper", Owner: ptr(true)}, false},
		{"the owner makes one", "griff", Op{Kind: SetOwner, Account: "gatekeeper", Owner: ptr(true)}, true},
		{"anyone sets their own password", "office", Op{Kind: SetPassword, Account: "office", PasswordHash: "h"}, true},
		{"not someone else's", "gatekeeper", Op{Kind: SetPassword, Account: "office", PasswordHash: "h"}, false},
		{"the owner resets one", "griff", Op{Kind: SetPassword, Account: "office", PasswordHash: "h", MustChange: true}, true},
		{"anyone renames themselves", "office", Op{Kind: SetAccount, Account: "office", Name: "Office"}, true},
		{"not someone else", "claude", Op{Kind: SetAccount, Account: "office", Name: "Office"}, false},
		{"grant inside own admin folder", "gatekeeper", Op{Kind: GrantRole, Account: "office", Tree: L, Node: "gate", Role: "viewer"}, true},
		{"grant outside own admin folder", "gatekeeper", Op{Kind: GrantRole, Account: "office", Tree: L, Node: "house", Role: "viewer"}, false},
		{"own token", "claude", Op{Kind: IssueToken, Account: "claude", TokenID: "t1"}, true},
		{"token for someone else", "claude", Op{Kind: IssueToken, Account: "office", TokenID: "t2"}, false},
		{"the owner issues for someone else", "griff", Op{Kind: IssueToken, Account: "office", TokenID: "t3"}, true},
		{"a folder admin may not", "gatekeeper", Op{Kind: IssueToken, Account: "office", TokenID: "t4"}, false},
		{"unknown actor", "mallory", set(L, "house", "radio.2g.width"), false},
		// A batch is allowed only where each of its changes is (0203).
		{"a batch inside its grant", "office", Op{Kind: Batch, Ops: []Op{set(S, "household", "network.sweet.ssid"), {Kind: Unset, Tree: S, Node: "household", Path: "network.sweet.vlan"}}}, true},
		{"a batch reaching past it", "office", Op{Kind: Batch, Ops: []Op{set(S, "household", "network.sweet.ssid"), set(S, "guest", "network.guest.ssid")}}, false},
		{"create-org must name its caller", "claude", Op{Kind: CreateOrg, Node: "x", Name: "X", Account: "griff"}, false},
	}
	for _, c := range cases {
		err := Authorize(s, c.actor, c.op)
		if (err == nil) != c.ok {
			t.Errorf("%s: Authorize(%s, %s) = %v, want ok=%v", c.name, c.actor, c.op.Kind, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrUnknownActor) {
			t.Errorf("%s: unexpected error kind %v", c.name, err)
		}
	}
}

func TestBreakWithoutLocksAboveNeedsOnlyTheFolder(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: Unlock, Tree: Locations, Node: "example", Path: "system.poll"})
	if err := Authorize(s, "gatekeeper", Op{Kind: BreakHierarchy, Tree: Locations, Node: "gate"}); err != nil {
		t.Fatalf("break with no locks above: %v", err)
	}
}

func TestRevokingTokens(t *testing.T) {
	s := people(t)
	_, id, hash, err := access.NewToken()
	must(t, err)
	mustApply(t, s, Op{Kind: IssueToken, Account: "office", TokenID: id, TokenHash: hash})
	if err := Authorize(s, "office", Op{Kind: RevokeToken, TokenID: id}); err != nil {
		t.Fatalf("revoking own token: %v", err)
	}
	if err := Authorize(s, "claude", Op{Kind: RevokeToken, TokenID: id}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoking someone else's token: %v", err)
	}
	mustApply(t, s, Op{Kind: RevokeToken, TokenID: id})
	if _, _, err := Apply(s, Op{Kind: RevokeToken, TokenID: id}); !errors.Is(err, access.ErrRevoked) {
		t.Fatalf("second revoke: %v", err)
	}
}

func TestAccessOpsApply(t *testing.T) {
	s := people(t)
	if _, _, err := Apply(s, Op{Kind: GrantRole, Account: "office", Tree: Locations, Node: "gate-ap", Role: "viewer"}); !errors.Is(err, ErrNotAFolder) {
		t.Fatalf("grant on an AP: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: GrantRole, Account: "office", Tree: Locations, Node: "gate", Role: "owner"}); !errors.Is(err, access.ErrBadRole) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: AddAccount, Account: "claude", Name: "again"}); !errors.Is(err, access.ErrExists) {
		t.Fatalf("duplicate account: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: IssueToken, Account: "ghost", TokenID: "t", TokenHash: make([]byte, 32)}); !errors.Is(err, access.ErrNoAccount) {
		t.Fatalf("token for unknown account: %v", err)
	}
	mustApply(t, s, Op{Kind: RevokeRole, Account: "office", Tree: Services, Node: "household", Role: "operator"})
	if err := Authorize(s, "office", Op{Kind: Set, Tree: Services, Node: "household", Path: "network.sweet.ssid", Value: json.RawMessage(`"x"`)}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("set after revoke: %v", err)
	}
}

func TestCreateOrgMakesItsCallerAdminOfBothTrees(t *testing.T) {
	s, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "example", Name: "Example", Account: "griff"})
	must(t, err)
	for _, tree := range []string{"locations", "services"} {
		if r := s.Access.RolesAt("griff", tree, []hierarchy.NodeID{"example"}); !slices.Equal(r, []access.Role{access.Admin}) {
			t.Errorf("griff in %s = %v, want admin", tree, r)
		}
	}
	if _, _, err := Apply(nil, Op{Kind: CreateOrg, Node: "example", Name: "Example"}); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("create-org without an admin: %v", err)
	}
}

// Today the manager makes Landing Zone alone (0333): no Sandbox, in either
// tree, and a set there is none of is refused.
func TestBuiltins(t *testing.T) {
	s := people(t)
	if _, _, err := Apply(s, Op{Kind: AddBuiltins, Builtins: len(builtinSets)}); !errors.Is(err, ErrBadBuiltins) {
		t.Fatalf("a set there is none of: %v", err)
	}
	_, eff, err := Apply(s, Op{Kind: AddBuiltins, Builtins: BuiltinsNow})
	must(t, err)
	if !reflect.DeepEqual(eff.After, []string{"locations/landing-zone"}) {
		t.Fatalf("created %v", eff.After)
	}
	if n, _ := s.Org.Locations.Node(LandingZone); !n.Isolated || n.Parent != s.Org.Locations.Root() {
		t.Fatalf("Landing Zone = %+v", n)
	}
	for _, tr := range []*hierarchy.Tree{s.Org.Locations, s.Org.Services} {
		if _, ok := tr.Node(Sandbox); ok {
			t.Fatal("a Sandbox was made")
		}
	}
	if _, _, err := Apply(s, Op{Kind: AddBuiltins, Builtins: BuiltinsNow}); !errors.Is(err, ErrBuiltins) {
		t.Fatalf("second add-builtins: %v", err)
	}
}

// An entry from before 0333 names no set, and replays as it was made: with
// Sandbox in both trees, so what the log did there later replays too. The
// manager's own add-builtins then finds nothing to make.
func TestBuiltinsReplayAsMade(t *testing.T) {
	s := people(t)
	var old Op
	must(t, json.Unmarshal([]byte(`{"kind":"add-builtins"}`), &old))
	_, eff, err := Apply(s, old)
	must(t, err)
	if !reflect.DeepEqual(eff.After, []string{"locations/landing-zone", "locations/sandbox", "services/sandbox"}) {
		t.Fatalf("created %v", eff.After)
	}
	if n, _ := s.Org.Locations.Node(LandingZone); !n.Isolated {
		t.Fatal("Landing Zone is not isolated")
	}
	if n, _ := s.Org.Locations.Node(Sandbox); n.Isolated {
		t.Fatal("Sandbox is isolated")
	}
	mustApply(t, s, Op{Kind: Rename, Tree: Locations, Node: Sandbox, Name: "Home"})
	if _, _, err := Apply(s, Op{Kind: AddBuiltins, Builtins: BuiltinsNow}); !errors.Is(err, ErrBuiltins) {
		t.Fatalf("today's add-builtins after an old one: %v", err)
	}
	if n, _ := s.Org.Locations.Node(Sandbox); n.Name != "Home" {
		t.Fatalf("the old Sandbox = %+v", n)
	}
}

func TestBuiltinsRefuseALookalike(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddFolder, Tree: Services, Node: Sandbox, Name: "My sandbox", Parent: "household"})
	if _, _, err := Apply(s, Op{Kind: AddBuiltins}); !errors.Is(err, hierarchy.ErrExists) {
		t.Fatalf("add-builtins over a lookalike: %v", err)
	}
}

func TestTheManagerMayDoOnlyTwoThings(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	allowed := []Op{
		{Kind: AddBuiltins},
		{Kind: Enroll, Node: "new-ap", Name: "NewAP", TokenID: "t1", TokenHash: make([]byte, 32)},
	}
	for _, op := range allowed {
		if err := Authorize(s, SystemActor, op); err != nil {
			t.Errorf("%s: %v", op.Kind, err)
		}
	}
	refused := []Op{
		{Kind: AddAP, Tree: Locations, Node: "x", Name: "X", Parent: LandingZone},
		{Kind: AddAP, Tree: Locations, Node: "x", Name: "X", Parent: "house"},
		{Kind: RemoveAP, Node: "gate-ap"},
		{Kind: Set, Tree: Locations, Node: "house", Path: "system.tz", Value: json.RawMessage(`"UTC"`)},
		{Kind: AddAccount, Account: "x", Name: "X"},
		{Kind: GrantRole, Account: "claude", Tree: Locations, Node: "example", Role: "admin"},
	}
	for _, op := range refused {
		if err := Authorize(s, SystemActor, op); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s: got %v, want ErrForbidden", op.Kind, err)
		}
	}
	if _, _, err := Apply(s, Op{Kind: AddAccount, Account: SystemActor, Name: "impostor"}); !errors.Is(err, ErrReserved) {
		t.Fatalf("an account named aeolus: %v", err)
	}
	if err := Authorize(s, "claude", Op{Kind: AddBuiltins}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("operator adding built-ins: %v", err)
	}
}

func TestAdoptionIsDecidedByTheDestination(t *testing.T) {
	s := people(t)
	mustApply(t, s, Op{Kind: AddBuiltins})
	mustApply(t, s, Op{Kind: AddAP, Tree: Locations, Node: "new-ap", Name: "NewAP", Parent: LandingZone})
	for _, op := range []Op{
		{Kind: AddAccount, Account: "house-op", Name: "House operator"},
		{Kind: GrantRole, Account: "house-op", Tree: Locations, Node: "house", Role: "operator"},
		{Kind: AddAccount, Account: "house-adopter", Name: "House adopter"},
		{Kind: GrantRole, Account: "house-adopter", Tree: Locations, Node: "house", Role: "operator"},
		{Kind: GrantRole, Account: "house-adopter", Tree: Locations, Node: LandingZone, Role: "viewer"},
	} {
		mustApply(t, s, op)
	}
	adopt := func(dest hierarchy.NodeID) Op {
		return Op{Kind: Move, Tree: Locations, Node: "new-ap", Parent: dest}
	}
	cases := []struct {
		actor string
		op    Op
		ok    bool
	}{
		{"house-op", adopt("house"), false},     // cannot see Landing Zone
		{"house-adopter", adopt("house"), true}, // sees it, operates the destination
		{"house-adopter", adopt("gate"), false}, // no role on the destination
		{"claude", adopt("gate"), true},         // operator at the root
		{"gatekeeper", adopt("gate"), false},    // admin on gate, but cannot see Landing Zone
	}
	for _, c := range cases {
		err := Authorize(s, c.actor, c.op)
		if (err == nil) != c.ok {
			t.Errorf("%s adopting into %s: %v, want ok=%v", c.actor, c.op.Parent, err, c.ok)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCredentialsAreNoTemplates(t *testing.T) {
	for _, p := range []hierarchy.Path{"system.switches.password", "system.switches.addresses", "system.dns_log.token"} {
		if TemplateFieldOK(p) {
			t.Errorf("%s in a template", p)
		}
	}
	if !TemplateFieldOK("system.tz") {
		t.Error("system.tz refused")
	}
}

// A batch that makes a folder is allowed a change at a time, each on what
// those before it leave (0320): a network set in the new folder is judged
// where the folder is made, and taking it needs the location's right too.
func TestBatchWithANewFolderIsJudgedAStepAtATime(t *testing.T) {
	s := people(t)
	S := Services
	newFolder := func(parent hierarchy.NodeID) Op {
		return Op{Kind: AddFolder, Tree: S, Node: "extra", Name: "Extra", Parent: parent}
	}
	inIt := Op{Kind: Set, Tree: S, Node: "extra", Path: "network.extra.ssid", Value: json.RawMessage(`"Extra"`)}
	take := Op{Kind: AssignServices, Node: "house", Services: []hierarchy.NodeID{"household", "extra"}}
	for _, c := range []struct {
		name, actor string
		ops         []Op
		ok          bool
	}{
		{"a folder, taken, a network in it", "claude", []Op{newFolder("example"), take, inIt}, true},
		{"in a folder of one's own, a network", "office", []Op{newFolder("household"), inIt}, true},
		{"a folder where one only views", "office", []Op{newFolder("example"), inIt}, false},
		{"taken where one has no right", "office", []Op{newFolder("household"), take}, false},
		{"a location that takes it, no right in Services", "gatekeeper", []Op{newFolder("household"), {Kind: AssignServices, Node: "gate", Services: []hierarchy.NodeID{"extra"}}}, false},
	} {
		err := Authorize(s, c.actor, Op{Kind: Batch, Ops: c.ops})
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// Judging changes nothing.
	if _, ok := s.Org.Services.Node("extra"); ok {
		t.Fatal("judging the batch made its folder")
	}
}
