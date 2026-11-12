// Package access holds accounts, API tokens and roles (0024, 0025). It is part
// of the state rebuilt from the change log: every account, token and grant
// arrives through a logged change.
//
// Tokens are stored only as a SHA-256 hash of their secret part. The secret is
// 32 random bytes, so a fast hash is enough; there is nothing to brute-force.
package access

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Role is what an account may do on a folder and below it (0025): the
// rights it holds there (0177, rights.go).
type Role int

const (
	None Role = iota
	Viewer
	Operator
	Admin
	Manager
	Installer
)

var roleNames = map[Role]string{None: "none", Viewer: "viewer", Operator: "operator", Admin: "admin", Manager: "manager", Installer: "installer"}

func (r Role) String() string { return roleNames[r] }

// ParseRole reads "viewer", "operator", "admin", "manager" or "installer".
func ParseRole(s string) (Role, error) {
	for r, n := range roleNames {
		if n == s && r != None {
			return r, nil
		}
	}
	return None, fmt.Errorf("%w: %q", ErrBadRole, s)
}

// AccountID identifies an account; it is the actor name in the change log.
type AccountID string

// Account is one actor: a person, an automation, or Claude.
type Account struct {
	ID   AccountID
	Name string
	// Owner says it runs the Org's people (0176): it adds, disables and
	// removes accounts, makes owners, resets passwords and manages others'
	// tokens, and it holds admin everywhere.
	Owner bool
	// A disabled account can't sign in, use its tokens or make a change
	// until it is enabled again. A removed one never can, and its ID is never
	// taken again: the change log names it.
	Disabled bool
	Removed  bool
	// Password is the kept hash (password.go), empty without one. MustChange
	// says someone else set it, and it is to be changed at the next sign-in.
	Password   string `json:"-"`
	MustChange bool
}

// Active says whether the account may sign in and act.
func (acc Account) Active() bool { return !acc.Disabled && !acc.Removed }

// Token is an API token as stored: its ID and the hash of its secret.
type Token struct {
	ID      string
	Account AccountID
	Hash    []byte
	Revoked bool
}

// Grant gives an account a role on a folder in one tree.
type Grant struct {
	Account AccountID
	Tree    string
	Node    hierarchy.NodeID
	Role    Role
}

var (
	ErrExists      = errors.New("already exists")
	ErrNoAccount   = errors.New("no such account")
	ErrNoToken     = errors.New("no such token")
	ErrNoGrant     = errors.New("no such grant")
	ErrBadRole     = errors.New("unknown role")
	ErrBadToken    = errors.New("invalid or revoked token")
	ErrBadTokenArg = errors.New("a token needs an ID and a 32-byte hash")
	ErrRevoked     = errors.New("token is already revoked")
	ErrInactive    = errors.New("the account is disabled or removed")
	ErrRemoved     = errors.New("the account is removed")
	ErrLastOwner   = errors.New("the Org keeps at least one owner who can sign in")
	ErrNoChange    = errors.New("it is that way already")
	// ErrWaiting is an AP's token that waits for a person to let the AP
	// back in (0223): it says which AP, and gets it nothing else.
	ErrWaiting   = errors.New("this AP enrolled again, and waits for a person to let it back in")
	ErrNoWaiting = errors.New("this AP has not enrolled again: nothing waits to be let back in")
)

// APToken is an AP's credential (0033), kept apart from account tokens: an AP
// token never works on the admin API, and an account token never works as an
// AP.
type APToken struct {
	ID      string
	AP      hierarchy.NodeID
	Hash    []byte
	Revoked bool
	// Waiting is the token an AP the manager knew enrolled again with
	// (0223), as after a reflash: it gets the AP nothing until a person lets
	// it back in.
	Waiting bool
}

// Access is the set of accounts, tokens and grants.
type Access struct {
	accounts map[AccountID]*Account
	tokens   map[string]*Token
	grants   map[Grant]bool
	apTokens map[string]*APToken
	invites  map[string]*Invite // 0336
}

// New returns an empty Access.
func New() *Access {
	return &Access{accounts: map[AccountID]*Account{}, tokens: map[string]*Token{}, grants: map[Grant]bool{}, apTokens: map[string]*APToken{}, invites: map[string]*Invite{}}
}

// Clone returns an independent copy.
func (a *Access) Clone() *Access {
	c := New()
	for id, acc := range a.accounts {
		cp := *acc
		c.accounts[id] = &cp
	}
	for id, tok := range a.tokens {
		cp := *tok
		cp.Hash = append([]byte(nil), tok.Hash...)
		c.tokens[id] = &cp
	}
	for g := range a.grants {
		c.grants[g] = true
	}
	for id, tok := range a.apTokens {
		cp := *tok
		cp.Hash = append([]byte(nil), tok.Hash...)
		c.apTokens[id] = &cp
	}
	for id, iv := range a.invites {
		c.invites[id] = cloneInvite(iv)
	}
	return c
}

// AddAccount creates an account.
func (a *Access) AddAccount(id AccountID, name string) error {
	if id == "" {
		return fmt.Errorf("%w: empty account ID", ErrNoAccount)
	}
	if _, ok := a.accounts[id]; ok {
		return fmt.Errorf("account %s %w", id, ErrExists)
	}
	a.accounts[id] = &Account{ID: id, Name: name}
	return nil
}

// Account returns an account.
func (a *Access) Account(id AccountID) (Account, bool) {
	acc, ok := a.accounts[id]
	if !ok {
		return Account{}, false
	}
	return *acc, true
}

// Accounts lists every account by ID, removed ones included.
func (a *Access) Accounts() []Account {
	out := make([]Account, 0, len(a.accounts))
	for _, acc := range a.accounts {
		out = append(out, *acc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// live returns an account that is not removed.
func (a *Access) live(id AccountID) (*Account, error) {
	acc, ok := a.accounts[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoAccount, id)
	}
	if acc.Removed {
		return nil, fmt.Errorf("%w: %s", ErrRemoved, id)
	}
	return acc, nil
}

// ActiveOwners counts the owners who can sign in.
func (a *Access) ActiveOwners() int {
	n := 0
	for _, acc := range a.accounts {
		if acc.Owner && acc.Active() {
			n++
		}
	}
	return n
}

// keepsOwner refuses a change that would leave an Org that had an owner who
// could sign in without one (0176). An Org from before owners has none, and
// nothing to keep.
func (a *Access) keepsOwner(before int) error {
	if before > 0 && a.ActiveOwners() == 0 {
		return ErrLastOwner
	}
	return nil
}

// SetName changes an account's display name.
func (a *Access) SetName(id AccountID, name string) error {
	acc, err := a.live(id)
	if err != nil {
		return err
	}
	acc.Name = name
	return nil
}

// SetPassword keeps an account's password hash, and whether it is to be
// changed at the next sign-in.
func (a *Access) SetPassword(id AccountID, encoded string, mustChange bool) error {
	if err := CheckPasswordHash(encoded); err != nil {
		return err
	}
	acc, err := a.live(id)
	if err != nil {
		return err
	}
	acc.Password, acc.MustChange = encoded, mustChange
	return nil
}

// SetDisabled disables or enables an account.
func (a *Access) SetDisabled(id AccountID, disabled bool) error {
	acc, err := a.live(id)
	if err != nil {
		return err
	}
	if acc.Disabled == disabled {
		return ErrNoChange
	}
	before := a.ActiveOwners()
	acc.Disabled = disabled
	if err := a.keepsOwner(before); err != nil {
		acc.Disabled = !disabled
		return err
	}
	return nil
}

// SetOwner makes an account an owner, or no longer one.
func (a *Access) SetOwner(id AccountID, owner bool) error {
	acc, err := a.live(id)
	if err != nil {
		return err
	}
	if acc.Owner == owner {
		return ErrNoChange
	}
	before := a.ActiveOwners()
	acc.Owner = owner
	if err := a.keepsOwner(before); err != nil {
		acc.Owner = !owner
		return err
	}
	return nil
}

// RemoveAccount removes an account for good: its tokens are revoked, its
// grants dropped, its password gone. It reports how many of each it had.
func (a *Access) RemoveAccount(id AccountID) (tokens, grants int, err error) {
	acc, err := a.live(id)
	if err != nil {
		return 0, 0, err
	}
	before := a.ActiveOwners()
	acc.Removed = true
	if err := a.keepsOwner(before); err != nil {
		acc.Removed = false
		return 0, 0, err
	}
	acc.Owner, acc.Password, acc.MustChange = false, "", false
	for _, tok := range a.tokens {
		if tok.Account == id && !tok.Revoked {
			tok.Revoked = true
			tokens++
		}
	}
	for g := range a.grants {
		if g.Account == id {
			delete(a.grants, g)
			grants++
		}
	}
	return tokens, grants, nil
}

// AddToken records a token's ID and hash for an account.
func (a *Access) AddToken(id string, account AccountID, hash []byte) error {
	if id == "" || len(hash) != sha256.Size {
		return ErrBadTokenArg
	}
	if acc, ok := a.accounts[account]; !ok {
		return fmt.Errorf("%w: %s", ErrNoAccount, account)
	} else if !acc.Active() {
		return fmt.Errorf("%w: %s", ErrInactive, account)
	}
	if _, ok := a.tokens[id]; ok {
		return fmt.Errorf("token %s %w", id, ErrExists)
	}
	a.tokens[id] = &Token{ID: id, Account: account, Hash: append([]byte(nil), hash...)}
	return nil
}

// Token returns a stored token.
func (a *Access) Token(id string) (Token, bool) {
	tok, ok := a.tokens[id]
	if !ok {
		return Token{}, false
	}
	return *tok, true
}

// TokensOf lists an account's tokens, revoked ones included, by ID.
func (a *Access) TokensOf(account AccountID) []Token {
	var out []Token
	for _, tok := range a.tokens {
		if tok.Account == account {
			cp := *tok
			cp.Hash = append([]byte(nil), tok.Hash...)
			out = append(out, cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// RevokeToken revokes a token for good.
func (a *Access) RevokeToken(id string) error {
	tok, ok := a.tokens[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoToken, id)
	}
	if tok.Revoked {
		return ErrRevoked
	}
	tok.Revoked = true
	return nil
}

// Grant gives a role. The caller checks that the folder exists in the tree.
func (a *Access) Grant(g Grant) error {
	if g.Role == None || roleNames[g.Role] == "" {
		return ErrBadRole
	}
	if _, err := a.live(g.Account); err != nil {
		return err
	}
	a.grants[g] = true
	return nil
}

// Revoke removes a grant.
func (a *Access) Revoke(g Grant) error {
	if !a.grants[g] {
		return ErrNoGrant
	}
	delete(a.grants, g)
	return nil
}

// Grants lists every grant, sorted.
func (a *Access) Grants() []Grant {
	out := make([]Grant, 0, len(a.grants))
	for g := range a.grants {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.Account != y.Account {
			return x.Account < y.Account
		}
		if x.Tree != y.Tree {
			return x.Tree < y.Tree
		}
		if x.Node != y.Node {
			return x.Node < y.Node
		}
		return x.Role < y.Role
	})
	return out
}

// AddAPToken records an AP's token ID and hash.
func (a *Access) AddAPToken(id string, ap hierarchy.NodeID, hash []byte) error {
	if id == "" || ap == "" || len(hash) != sha256.Size {
		return ErrBadTokenArg
	}
	if _, ok := a.apTokens[id]; ok {
		return fmt.Errorf("token %s %w", id, ErrExists)
	}
	a.apTokens[id] = &APToken{ID: id, AP: ap, Hash: append([]byte(nil), hash...)}
	return nil
}

// AddWaitingAPToken records the token an AP the manager knew enrolled again
// with (0223), in place of one it waited with before. Its other tokens stay
// as they were until a person lets it back in.
func (a *Access) AddWaitingAPToken(id string, ap hierarchy.NodeID, hash []byte) error {
	if id == "" || ap == "" || len(hash) != sha256.Size {
		return ErrBadTokenArg
	}
	if _, ok := a.apTokens[id]; ok {
		return fmt.Errorf("token %s %w", id, ErrExists)
	}
	for _, tok := range a.apTokens {
		if tok.AP == ap && tok.Waiting {
			tok.Revoked = true
		}
	}
	a.apTokens[id] = &APToken{ID: id, AP: ap, Hash: append([]byte(nil), hash...), Waiting: true}
	return nil
}

// WaitingAP says whether an AP enrolled again and waits to be let back in.
func (a *Access) WaitingAP(ap hierarchy.NodeID) bool {
	for _, tok := range a.apTokens {
		if tok.AP == ap && tok.Waiting && !tok.Revoked {
			return true
		}
	}
	return false
}

// LetAPBackIn makes the token an AP waits with its own (0223), and stops
// every other it holds.
func (a *Access) LetAPBackIn(ap hierarchy.NodeID) error {
	if !a.WaitingAP(ap) {
		return ErrNoWaiting
	}
	for _, tok := range a.apTokens {
		if tok.AP != ap || tok.Revoked {
			continue
		}
		if tok.Waiting {
			tok.Waiting = false
		} else {
			tok.Revoked = true
		}
	}
	return nil
}

// HandAPTokens gives the tokens one AP holds to another, whose place it
// takes (0223): the other's own stop working, and any either waits with.
// It returns how many were handed.
func (a *Access) HandAPTokens(from, to hierarchy.NodeID) int {
	n := 0
	for _, tok := range a.apTokens {
		switch {
		case tok.Revoked:
		case tok.AP == to || tok.Waiting && tok.AP == from:
			tok.Revoked = true
		case tok.AP == from:
			tok.AP = to
			n++
		}
	}
	return n
}

// RevokeAPTokens revokes every token an AP holds and returns how many.
func (a *Access) RevokeAPTokens(ap hierarchy.NodeID) int {
	n := 0
	for _, tok := range a.apTokens {
		if tok.AP == ap && !tok.Revoked {
			tok.Revoked = true
			n++
		}
	}
	return n
}

const (
	tokenPrefix   = "aeolus1"
	apTokenPrefix = "aeolusap1"
)

// NewToken makes a token: the plain text to hand out once, and the ID and hash
// to record.
func NewToken() (plain, id string, hash []byte, err error) {
	idb := make([]byte, 8)
	sec := make([]byte, 32)
	if _, err = rand.Read(idb); err != nil {
		return "", "", nil, err
	}
	if _, err = rand.Read(sec); err != nil {
		return "", "", nil, err
	}
	id = hex.EncodeToString(idb)
	secret := base64.RawURLEncoding.EncodeToString(sec)
	sum := sha256.Sum256([]byte(secret))
	return tokenPrefix + "." + id + "." + secret, id, sum[:], nil
}

// NewAPToken makes an AP token, in the same shape as NewToken but with its
// own prefix.
func NewAPToken() (plain, id string, hash []byte, err error) {
	plain, id, hash, err = NewToken()
	if err != nil {
		return "", "", nil, err
	}
	return apTokenPrefix + strings.TrimPrefix(plain, tokenPrefix), id, hash, nil
}

// AuthenticateAP returns the AP a plain AP token belongs to: with
// ErrWaiting, where the AP enrolled again and waits to be let back in.
func (a *Access) AuthenticateAP(plain string) (hierarchy.NodeID, error) {
	parts := strings.SplitN(plain, ".", 3)
	if len(parts) != 3 || parts[0] != apTokenPrefix {
		return "", ErrBadToken
	}
	tok, ok := a.apTokens[parts[1]]
	if !ok || tok.Revoked {
		return "", ErrBadToken
	}
	sum := sha256.Sum256([]byte(parts[2]))
	if subtle.ConstantTimeCompare(sum[:], tok.Hash) != 1 {
		return "", ErrBadToken
	}
	// A waiting token says whose it is, and that it waits (0223).
	if tok.Waiting {
		return tok.AP, ErrWaiting
	}
	return tok.AP, nil
}

// Authenticate returns the account a plain token belongs to.
func (a *Access) Authenticate(plain string) (AccountID, error) {
	parts := strings.SplitN(plain, ".", 3)
	if len(parts) != 3 || parts[0] != tokenPrefix {
		return "", ErrBadToken
	}
	tok, ok := a.tokens[parts[1]]
	if !ok || tok.Revoked {
		return "", ErrBadToken
	}
	sum := sha256.Sum256([]byte(parts[2]))
	if subtle.ConstantTimeCompare(sum[:], tok.Hash) != 1 {
		return "", ErrBadToken
	}
	// A disabled account's tokens stop working, and start again if it is
	// enabled (0176).
	if acc, ok := a.accounts[tok.Account]; !ok || !acc.Active() {
		return "", ErrBadToken
	}
	return tok.Account, nil
}
