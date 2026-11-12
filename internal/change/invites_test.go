package change

import (
	"errors"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
)

// Invites (0336), in the change package: an owner makes one in its own name;
// only the manager uses one, and using it makes the account, its password,
// its roles and whether it is an owner, or nothing at all.
func TestInviteChanges(t *testing.T) {
	s := people(t)
	_, id, hash, err := access.NewInvite()
	must(t, err)
	until := time.Now().Add(72 * time.Hour)
	add := Op{Kind: AddInvite, Account: "griff", TokenID: id, TokenHash: hash, Expires: until, Note: "for the first user",
		Roles: []RoleOn{{Tree: Services, Node: "household", Role: "viewer"}, {Tree: Locations, Node: "house", Role: "installer"}}}

	// Who may make, revoke and use one.
	for _, c := range []struct {
		name  string
		actor string
		op    Op
		ok    bool
	}{
		{"an owner, in its own name", "griff", add, true},
		{"an owner, in another's", "griff", func() Op { o := add; o.Account = "claude"; return o }(), false},
		{"not an owner", "claude", func() Op { o := add; o.Account = "claude"; return o }(), false},
		{"an admin of a folder", "gatekeeper", func() Op { o := add; o.Account = "gatekeeper"; return o }(), false},
		{"revoked by an owner", "griff", Op{Kind: RevokeInvite, TokenID: id}, true},
		{"revoked by another", "claude", Op{Kind: RevokeInvite, TokenID: id}, false},
		{"used by an owner", "griff", Op{Kind: UseInvite, TokenID: id, Account: "first-user", PasswordHash: "x"}, false},
		{"used by the manager, before it is made", SystemActor, Op{Kind: UseInvite, TokenID: id, Account: "first-user", PasswordHash: "x"}, false},
	} {
		if err := Authorize(s, c.actor, c.op); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	// What an invite may give.
	for name, op := range map[string]Op{
		"nothing":               {Kind: AddInvite, Account: "griff", TokenID: "a", TokenHash: hash, Expires: until},
		"no expiry":             {Kind: AddInvite, Account: "griff", TokenID: "b", TokenHash: hash, Roles: add.Roles},
		"on an AP":              {Kind: AddInvite, Account: "griff", TokenID: "c", TokenHash: hash, Expires: until, Roles: []RoleOn{{Tree: Locations, Node: "gate-ap", Role: "viewer"}}},
		"a bad role":            {Kind: AddInvite, Account: "griff", TokenID: "d", TokenHash: hash, Expires: until, Roles: []RoleOn{{Tree: Locations, Node: "house", Role: "king"}}},
		"from one not an owner": {Kind: AddInvite, Account: "claude", TokenID: "e", TokenHash: hash, Expires: until, Roles: add.Roles},
	} {
		if _, _, err := Apply(s, op); err == nil {
			t.Errorf("an invite giving %s was made", name)
		}
	}
	mustApply(t, s, add)

	// A use that fails makes nothing: an account taken leaves the invite
	// open and no account made.
	pw, err := access.HashPassword("a long enough passphrase")
	must(t, err)
	use := Op{Kind: UseInvite, TokenID: id, Account: "claude", Name: "the first user", PasswordHash: pw}
	if err := Authorize(s, SystemActor, use); err != nil {
		t.Fatalf("the manager uses an invite: %v", err)
	}
	if _, _, err := Apply(s, use); !errors.Is(err, access.ErrExists) {
		t.Fatalf("an invite used for an account that is taken: %v", err)
	}
	if iv, _ := s.Access.Invite(id); iv.UsedBy != "" {
		t.Fatalf("a failed use marked it used: %+v", iv)
	}
	if _, _, err := Apply(s, Op{Kind: UseInvite, TokenID: id, Account: SystemActor, PasswordHash: pw}); !errors.Is(err, ErrReserved) {
		t.Fatalf("an invite used for the manager's own name: %v", err)
	}

	// Used: the account, its password, its roles, and the invite used.
	held := s.Access
	use.Account = "first-user"
	mustApply(t, s, use)
	if s.Access != held {
		t.Fatal("using an invite swapped the state's Access")
	}
	acc, ok := s.Access.Account("first-user")
	if !ok || acc.Name != "the first user" || acc.Password != pw || acc.MustChange || acc.Owner {
		t.Fatalf("the account made: %+v", acc)
	}
	if !s.Access.May("first-user", string(Services), s.Org.Services.Ancestry("household"), access.View) ||
		!s.Access.May("first-user", string(Locations), s.Org.Locations.Ancestry("house"), access.Ports) {
		t.Fatal("the account has not the roles the invite gives")
	}
	if iv, _ := s.Access.Invite(id); iv.UsedBy != "first-user" {
		t.Fatalf("the invite after: %+v", iv)
	}
	if _, _, err := Apply(s, Op{Kind: UseInvite, TokenID: id, Account: "erin", PasswordHash: pw}); !errors.Is(err, access.ErrInviteUsed) {
		t.Fatalf("used twice: %v", err)
	}
	if _, _, err := Apply(s, Op{Kind: RevokeInvite, TokenID: id}); !errors.Is(err, access.ErrInviteUsed) {
		t.Fatalf("a used invite revoked: %v", err)
	}

	// One that makes an owner; and one whose folder went before it was
	// used, which makes nothing.
	_, id2, hash2, _ := access.NewInvite()
	mustApply(t, s, Op{Kind: AddInvite, Account: "griff", TokenID: id2, TokenHash: hash2, Expires: until, Owner: ptr(true)})
	mustApply(t, s, Op{Kind: UseInvite, TokenID: id2, Account: "frank", PasswordHash: pw})
	if f, _ := s.Access.Account("frank"); !f.Owner {
		t.Fatal("an owner's invite made no owner")
	}
	_, id3, hash3, _ := access.NewInvite()
	mustApply(t, s, Op{Kind: AddInvite, Account: "griff", TokenID: id3, TokenHash: hash3, Expires: until, Roles: []RoleOn{{Tree: Services, Node: "guest", Role: "viewer"}}})
	mustApply(t, s, Op{Kind: RemoveFolder, Tree: Services, Node: "guest"})
	if _, _, err := Apply(s, Op{Kind: UseInvite, TokenID: id3, Account: "gina", PasswordHash: pw}); err == nil {
		t.Fatal("an invite to a folder that is gone was used")
	}
	if _, ok := s.Access.Account("gina"); ok {
		t.Fatal("a failed use made the account")
	}

	// A copy is its own.
	c := s.Clone()
	mustApply(t, c, Op{Kind: RevokeInvite, TokenID: id3})
	if iv, _ := s.Access.Invite(id3); iv.Revoked {
		t.Fatal("revoked in a copy, revoked in the original")
	}
}
