package change

// Invites (0336): an owner makes one, with the roles it gives and until
// when; revokes it before it is used; or the one it is for uses it, through
// POST /v1/register, which makes their account with their own password and
// those roles, in one change the manager makes in its own name.

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// RoleOn is a role on a folder, as an invite gives it.
type RoleOn struct {
	Tree TreeName         `json:"tree"`
	Node hierarchy.NodeID `json:"node"`
	Role string           `json:"role"`
}

// NoteMax is the longest note an invite keeps.
const NoteMax = 200

var (
	ErrNoExpiry = errors.New("an invite says until when it is good (expires)")
	ErrBadNote  = fmt.Errorf("an invite's note is at most %d characters, none of them control characters", NoteMax)
)

func validateInvite(op Op) error {
	if op.TokenID == "" {
		return ErrNoTokenID
	}
	switch op.Kind {
	case AddInvite:
		if op.Account == "" {
			return ErrNoAccount
		}
		if op.Expires.IsZero() {
			return ErrNoExpiry
		}
		if len(op.Roles) == 0 && (op.Owner == nil || !*op.Owner) {
			return access.ErrInviteGives
		}
		if len(op.Roles) > access.MaxInviteRoles {
			return fmt.Errorf("%w: at most %d roles", access.ErrInviteGives, access.MaxInviteRoles)
		}
		if utf8.RuneCountInString(op.Note) > NoteMax || strings.IndexFunc(op.Note, unicode.IsControl) >= 0 {
			return ErrBadNote
		}
	case UseInvite:
		if op.Account == "" {
			return ErrNoAccount
		}
		if op.PasswordHash == "" {
			return ErrNoPassword
		}
		if op.Name != "" {
			return folderName(op.Name)
		}
	}
	return nil
}

// folderRole is a role on a folder, checked as a grant is: the tree, the
// folder in it, that it is no AP, and the role.
func folderRole(s *State, tn TreeName, node hierarchy.NodeID, role string) (access.Grant, error) {
	t, err := tree(s.Org, tn)
	if err != nil {
		return access.Grant{}, err
	}
	n, ok := t.Node(node)
	if !ok {
		return access.Grant{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, node)
	}
	if n.Kind == hierarchy.KindAP {
		return access.Grant{}, ErrNotAFolder
	}
	r, err := access.ParseRole(role)
	if err != nil {
		return access.Grant{}, err
	}
	return access.Grant{Tree: string(tn), Node: node, Role: r}, nil
}

// inviteSaid is an invite as an effect says it: never its hash.
func inviteSaid(iv access.Invite) map[string]any {
	roles := make([]map[string]string, 0, len(iv.Grants))
	for _, g := range iv.Grants {
		roles = append(roles, map[string]string{"tree": g.Tree, "node": string(g.Node), "role": g.Role.String()})
	}
	out := map[string]any{"invite": iv.ID, "by": string(iv.By), "roles": roles, "owner": iv.Owner, "expires": iv.Expires}
	if iv.Note != "" {
		out["note"] = iv.Note
	}
	return out
}

func applyInvite(s *State, op Op) (Effect, error) {
	a := s.Access
	switch op.Kind {
	case AddInvite:
		iv := access.Invite{ID: op.TokenID, Hash: op.TokenHash, By: op.Account, Owner: op.Owner != nil && *op.Owner, Expires: op.Expires.UTC(), Note: op.Note}
		for _, r := range op.Roles {
			g, err := folderRole(s, r.Tree, r.Node, r.Role)
			if err != nil {
				return Effect{}, err
			}
			iv.Grants = append(iv.Grants, g)
		}
		return Effect{After: inviteSaid(iv)}, a.AddInvite(iv)
	case RevokeInvite:
		iv, ok := a.Invite(op.TokenID)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", access.ErrNoInvite, op.TokenID)
		}
		return Effect{Before: inviteSaid(iv)}, a.RevokeInvite(op.TokenID)
	}

	// Used: the account, its password, its roles and whether it is an
	// owner, made on a copy that is kept only once all of it is made.
	iv, ok := a.Invite(op.TokenID)
	if !ok {
		return Effect{}, fmt.Errorf("%w: %s", access.ErrNoInvite, op.TokenID)
	}
	if op.Account == SystemActor {
		return Effect{}, ErrReserved
	}
	name := op.Name
	if name == "" {
		name = string(op.Account)
	}
	c := a.Clone()
	if err := c.UseInvite(iv.ID, op.Account); err != nil {
		return Effect{}, err
	}
	if err := c.AddAccount(op.Account, name); err != nil {
		return Effect{}, err
	}
	if err := c.SetPassword(op.Account, op.PasswordHash, false); err != nil {
		return Effect{}, err
	}
	for _, g := range iv.Grants {
		// The folder may have gone since the invite was made.
		if _, err := folderRole(s, TreeName(g.Tree), g.Node, g.Role.String()); err != nil {
			return Effect{}, fmt.Errorf("the invite's role %s on %s: %w", g.Role, g.Node, err)
		}
		g.Account = op.Account
		if err := c.Grant(g); err != nil {
			return Effect{}, err
		}
	}
	if iv.Owner {
		if err := c.SetOwner(op.Account, true); err != nil {
			return Effect{}, err
		}
	}
	*a = *c // the same Access, for whoever holds it
	after := inviteSaid(iv)
	after["account"], after["name"] = string(op.Account), name
	return Effect{After: after}, nil
}
