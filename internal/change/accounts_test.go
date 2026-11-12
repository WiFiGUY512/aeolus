package change

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/access"
)

func ptr[T any](v T) *T { return &v }

// Accounts run from the manager (0176): the Org's first account is its
// owner; an owner is admin everywhere; a disabled account can't act, and
// its tokens stop until it is enabled; a removed one loses its tokens and
// grants, and its ID is never taken again; the last owner who can sign in
// stays one.
func TestAccounts(t *testing.T) {
	s := people(t)
	a := s.Access
	if g, _ := a.Account("griff"); !g.Owner || a.ActiveOwners() != 1 {
		t.Fatalf("the first account is not the owner: %+v", g)
	}

	// An owner is admin everywhere, whatever its grants.
	mustApply(t, s, Op{Kind: SetOwner, Account: "office", Owner: ptr(true)})
	if !a.May("office", string(Locations), s.Org.Locations.Ancestry("house"), access.Administer) {
		t.Fatal("an owner may not administer a folder it has no grant on")
	}
	mustApply(t, s, Op{Kind: SetOwner, Account: "office", Owner: ptr(false)})
	if a.May("office", string(Locations), s.Org.Locations.Ancestry("house"), access.View) {
		t.Fatal("no longer an owner, it still sees there")
	}

	// Disabled: no role, no token, no change; enabled again, all back.
	plain, id, hash, err := access.NewToken()
	must(t, err)
	mustApply(t, s, Op{Kind: IssueToken, Account: "claude", TokenID: id, TokenHash: hash})
	mustApply(t, s, Op{Kind: DisableAccount, Account: "claude"})
	if _, err := a.Authenticate(plain); err == nil {
		t.Fatal("a disabled account's token still works")
	}
	if r := a.RolesAt("claude", string(Locations), s.Org.Locations.Ancestry("house")); len(r) != 0 {
		t.Fatalf("a disabled account's roles: %v", r)
	}
	if err := Authorize(s, "claude", Op{Kind: Set, Tree: Locations, Node: "house", Path: "radio.2g.width", Value: json.RawMessage(`20`)}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a disabled account makes a change: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: DisableAccount, Account: "claude"}); !errors.Is(err, access.ErrNoChange) {
		t.Fatalf("disabled twice: %v", err)
	}
	mustApply(t, s, Op{Kind: EnableAccount, Account: "claude"})
	if who, err := a.Authenticate(plain); err != nil || who != "claude" {
		t.Fatalf("enabled again, its token: %v %v", who, err)
	}

	// The last owner who can sign in stays one.
	for _, op := range []Op{
		{Kind: DisableAccount, Account: "griff"},
		{Kind: RemoveAccount, Account: "griff"},
		{Kind: SetOwner, Account: "griff", Owner: ptr(false)},
	} {
		if _, _, err := Apply(s, op); !errors.Is(err, access.ErrLastOwner) {
			t.Fatalf("%s of the last owner: %v", op.Kind, err)
		}
	}
	if g, _ := a.Account("griff"); !g.Owner || !g.Active() {
		t.Fatalf("the refused changes changed the owner: %+v", g)
	}
	// With a second owner, the first may go.
	mustApply(t, s, Op{Kind: SetOwner, Account: "gatekeeper", Owner: ptr(true)})
	mustApply(t, s, Op{Kind: DisableAccount, Account: "griff"})
	mustApply(t, s, Op{Kind: EnableAccount, Account: "griff"})

	// Removed: its tokens revoked, its grants gone, its ID never again.
	_, eff, err := Apply(s, Op{Kind: RemoveAccount, Account: "claude"})
	must(t, err)
	if b := eff.Before.(map[string]any); b["tokens"] != 1 || b["grants"] != 2 {
		t.Fatalf("removal's effect: %v", eff.Before)
	}
	if _, err := a.Authenticate(plain); err == nil {
		t.Fatal("a removed account's token works")
	}
	for _, g := range a.Grants() {
		if g.Account == "claude" {
			t.Fatalf("a removed account keeps a grant: %+v", g)
		}
	}
	for _, op := range []Op{
		{Kind: AddAccount, Account: "claude", Name: "Again"},
		{Kind: EnableAccount, Account: "claude"},
		{Kind: GrantRole, Account: "claude", Tree: Locations, Node: "house", Role: "viewer"},
	} {
		if _, _, err := Apply(s, op); err == nil {
			t.Fatalf("%s on a removed account went through", op.Kind)
		}
	}

	// A password: kept as its hash, and the effect never shows it.
	h, err := access.HashPassword("a long enough passphrase")
	must(t, err)
	_, eff, err = Apply(s, Op{Kind: SetPassword, Account: "office", PasswordHash: h, MustChange: true})
	must(t, err)
	if raw, _ := json.Marshal(eff); strings.Contains(string(raw), "pbkdf2") {
		t.Fatalf("the effect shows the hash: %s", raw)
	}
	if o, _ := a.Account("office"); !access.VerifyPassword(o.Password, "a long enough passphrase") || !o.MustChange {
		t.Fatalf("the password as kept: must_change=%v", o.MustChange)
	}
	if _, _, err := Apply(s, Op{Kind: SetPassword, Account: "office", PasswordHash: "plain text"}); !errors.Is(err, access.ErrBadPasswordArg) {
		t.Fatalf("a password that is not a hash: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: SetAccount, Account: "office", Name: ""}); !errors.Is(err, ErrBadName) {
		t.Fatalf("an empty display name: %v", err)
	}
}

// An Org made before owners gets its owner as its log replays: create-org's
// account.
func TestReplayMakesTheOwner(t *testing.T) {
	var s *State
	for _, op := range []Op{
		{Kind: CreateOrg, Node: "example", Name: "Example", Account: "griff"},
		{Kind: AddAccount, Account: "claude", Name: "Claude"},
	} {
		var err error
		s, _, err = Apply(s, op)
		must(t, err)
	}
	if g, _ := s.Access.Account("griff"); !g.Owner {
		t.Fatal("replayed, griff is not the owner")
	}
	if c, _ := s.Access.Account("claude"); c.Owner {
		t.Fatal("replayed, claude is an owner")
	}
}
