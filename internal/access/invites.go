package access

// Invites (0336): a link an owner makes for someone to join by. It says the
// roles it gives and where, whether it makes an owner, and until when it is
// good. Only a hash of its secret is kept, as a token's; it is used once, by
// the one who registers with it, and stops when it expires or is revoked.

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Invite is an invite as stored.
type Invite struct {
	ID   string
	Hash []byte
	// By is the owner who made it. It is good only while By is an owner
	// who can sign in: an invite speaks for its maker.
	By AccountID
	// Grants are the roles it gives, each with no account yet; Owner, that
	// it makes an owner.
	Grants  []Grant
	Owner   bool
	Expires time.Time
	Note    string
	// UsedBy is the account made with it, once it is used.
	UsedBy  AccountID
	Revoked bool
}

// State is what an invite is at a time: open, used, revoked or
// expired.
func (iv Invite) State(now time.Time) string {
	switch {
	case iv.UsedBy != "":
		return "used"
	case iv.Revoked:
		return "revoked"
	case !now.Before(iv.Expires):
		return "expired"
	}
	return "open"
}

var (
	ErrNoInvite      = errors.New("no such invite")
	ErrBadInvite     = errors.New("this invite link is not one the manager made: check it was copied whole")
	ErrInviteUsed    = errors.New("this invite has been used")
	ErrInviteRevoked = errors.New("this invite was revoked")
	ErrInviteExpired = errors.New("this invite has expired")
	ErrInviteMaker   = errors.New("the owner who made this invite is no longer an owner who can sign in")
	ErrInviteGives   = errors.New("an invite gives at least one role, or makes an owner")
)

// MaxInviteRoles is the most roles one invite gives.
const MaxInviteRoles = 16

func cloneInvite(iv *Invite) *Invite {
	cp := *iv
	cp.Hash = append([]byte(nil), iv.Hash...)
	cp.Grants = append([]Grant(nil), iv.Grants...)
	return &cp
}

// AddInvite records an invite. Its maker is an owner who can sign in; the
// caller checks each grant's folder, as for Grant.
func (a *Access) AddInvite(iv Invite) error {
	if iv.ID == "" || len(iv.Hash) != sha256.Size {
		return ErrBadTokenArg
	}
	if _, ok := a.invites[iv.ID]; ok {
		return fmt.Errorf("invite %s %w", iv.ID, ErrExists)
	}
	if len(iv.Grants) == 0 && !iv.Owner {
		return ErrInviteGives
	}
	if len(iv.Grants) > MaxInviteRoles {
		return fmt.Errorf("%w: at most %d roles", ErrInviteGives, MaxInviteRoles)
	}
	for _, g := range iv.Grants {
		if g.Role == None || roleNames[g.Role] == "" {
			return ErrBadRole
		}
	}
	if err := a.maker(iv.By); err != nil {
		return err
	}
	cp := cloneInvite(&iv)
	cp.UsedBy, cp.Revoked = "", false
	for i := range cp.Grants {
		cp.Grants[i].Account = ""
	}
	a.invites[iv.ID] = cp
	return nil
}

// maker says whether an account may stand behind an invite: an owner who
// can sign in.
func (a *Access) maker(id AccountID) error {
	acc, ok := a.accounts[id]
	if !ok || !acc.Owner || !acc.Active() {
		return ErrInviteMaker
	}
	return nil
}

// Invite returns a stored invite.
func (a *Access) Invite(id string) (Invite, bool) {
	iv, ok := a.invites[id]
	if !ok {
		return Invite{}, false
	}
	return *cloneInvite(iv), true
}

// Invites lists every invite, the latest to expire first.
func (a *Access) Invites() []Invite {
	out := make([]Invite, 0, len(a.invites))
	for _, iv := range a.invites {
		out = append(out, *cloneInvite(iv))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Expires.Equal(out[j].Expires) {
			return out[i].Expires.After(out[j].Expires)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// RevokeInvite stops an invite that has not been used.
func (a *Access) RevokeInvite(id string) error {
	iv, ok := a.invites[id]
	switch {
	case !ok:
		return fmt.Errorf("%w: %s", ErrNoInvite, id)
	case iv.UsedBy != "":
		return ErrInviteUsed
	case iv.Revoked:
		return ErrInviteRevoked
	}
	iv.Revoked = true
	return nil
}

// UsableInvite says why an invite can't be used now, or nil: it is used,
// revoked or expired, or its maker is no longer an owner.
func (a *Access) UsableInvite(id string, now time.Time) error {
	iv, ok := a.invites[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoInvite, id)
	}
	switch iv.State(now) {
	case "used":
		return ErrInviteUsed
	case "revoked":
		return ErrInviteRevoked
	case "expired":
		return ErrInviteExpired
	}
	return a.maker(iv.By)
}

// UseInvite marks an invite used by the account made with it. Whether it
// has expired is the caller's to check, against its own clock, so the
// change log replays the same whenever it is read.
func (a *Access) UseInvite(id string, account AccountID) error {
	iv, ok := a.invites[id]
	switch {
	case !ok:
		return fmt.Errorf("%w: %s", ErrNoInvite, id)
	case iv.UsedBy != "":
		return ErrInviteUsed
	case iv.Revoked:
		return ErrInviteRevoked
	}
	iv.UsedBy = account
	return nil
}

// NewInvite makes an invite's code, the part of its link after
// #/register/, to hand out once; and the ID and hash to record. It is a
// token's ID and secret, without the token's prefix.
func NewInvite() (code, id string, hash []byte, err error) {
	plain, id, hash, err := NewToken()
	if err != nil {
		return "", "", nil, err
	}
	return strings.TrimPrefix(plain, tokenPrefix+"."), id, hash, nil
}

// FindInvite returns the invite a code is for, its secret checked; not
// whether it can still be used, which UsableInvite says.
func (a *Access) FindInvite(code string) (Invite, error) {
	id, secret, ok := strings.Cut(strings.TrimSpace(code), ".")
	if !ok || id == "" || secret == "" {
		return Invite{}, ErrBadInvite
	}
	iv, found := a.invites[id]
	if !found {
		return Invite{}, ErrBadInvite
	}
	sum := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(sum[:], iv.Hash) != 1 {
		return Invite{}, ErrBadInvite
	}
	return *cloneInvite(iv), nil
}
