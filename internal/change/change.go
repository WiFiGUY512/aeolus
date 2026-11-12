// Package change defines the changes that can be made to the manager's state
// and applies them. Every change the manager accepts is one Op, recorded in
// the change log (0009) and replayed from it; Apply is deterministic, so
// replaying the log rebuilds exactly the same state.
//
// The state is the Org's two trees plus its accounts, tokens and roles
// (0024): access changes go through the log like everything else.
package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/keys"
	"github.com/WiFiGUY512/aeolus/internal/library"
)

// Kind names an operation.
type Kind string

const (
	// The trees.
	CreateOrg      Kind = "create-org"
	AddFolder      Kind = "add-folder"
	AddAP          Kind = "add-ap"
	Rename         Kind = "rename"
	Move           Kind = "move"
	Set            Kind = "set"
	Unset          Kind = "unset"
	Lock           Kind = "lock"
	Unlock         Kind = "unlock"
	BreakHierarchy Kind = "break-hierarchy"
	AssignServices Kind = "assign-services"
	// Batch is several changes made as one, on any nodes of either tree:
	// all of them or none (0203). What it may hold is Batchable.
	Batch       Kind = "batch"
	AddBuiltins Kind = "add-builtins"
	// A change that ends itself (0345): end-change puts back what change
	// Ends did, its Ops worked out by the change log; move-end gives it
	// another Until, and may set fields of one node with it, as part of
	// that change (a test window's maintenance.until, one hour more).
	EndChange Kind = "end-change"
	MoveEnd   Kind = "move-end"

	// Access (0024, 0025).
	AddAccount  Kind = "add-account"
	IssueToken  Kind = "issue-token"
	RevokeToken Kind = "revoke-token"
	GrantRole   Kind = "grant"
	RevokeRole  Kind = "revoke"

	// Accounts run from the manager (0176): a password, kept as a hash; an
	// account disabled, enabled or removed; an owner made or unmade; its
	// display name.
	SetPassword    Kind = "set-password"
	DisableAccount Kind = "disable-account"
	EnableAccount  Kind = "enable-account"
	RemoveAccount  Kind = "remove-account"
	SetOwner       Kind = "set-owner"
	SetAccount     Kind = "set-account"

	// Invites (0336): an owner's link for someone to join by, revoked
	// before it is used; or used by the one it is for, through POST
	// /v1/register, which makes their account.
	AddInvite    Kind = "add-invite"
	RevokeInvite Kind = "revoke-invite"
	UseInvite    Kind = "use-invite"

	// The library's concentrators (0023), retired (0085): they replay from
	// an old log, and the API takes no new ones.
	SetConcentrator    Kind = "set-concentrator"
	RemoveConcentrator Kind = "remove-concentrator"
	SetVNI             Kind = "set-vni"
	RemoveVNI          Kind = "remove-vni"

	// The library's AP templates (0085).
	AddTemplate    Kind = "add-template"
	EditTemplate   Kind = "edit-template"
	SetTemplate    Kind = "set-template"
	UnsetTemplate  Kind = "unset-template"
	RemoveTemplate Kind = "remove-template"

	// APs arriving and leaving (0033, 0038).
	Enroll   Kind = "enroll"
	RemoveAP Kind = "remove-ap"
	// An AP back after its token was lost, let back in by a person; and an
	// AP from Landing Zone in another's place, which keeps its records (0223).
	ReadmitAP Kind = "readmit-ap"
	ReplaceAP Kind = "replace-ap"

	// An AP saying again what it is, where that has changed since it
	// enrolled or last said (0122).
	Facts Kind = "facts"

	// A folder that holds nothing, taken away (0125).
	RemoveFolder Kind = "remove-folder"

	// Per-user keys (0014, 0070).
	AddKey    Kind = "add-key"
	SetKey    Kind = "set-key"
	RemoveKey Kind = "remove-key"
)

// The built-in folders' IDs (0032), and the manager's own actor name for
// the changes it makes itself (0036). Sandbox is built in no more (0333):
// an Org made before has one in each tree, an ordinary folder now, often
// renamed and in use, so its ID stays what it was.
const (
	LandingZone hierarchy.NodeID = "landing-zone"
	Sandbox     hierarchy.NodeID = "sandbox"
	SystemActor                  = "aeolus"

	// MaxBatch is the most changes one batch makes (0203).
	MaxBatch = 256
	// LandingZoneLimit caps how many APs may wait in Landing Zone (0038).
	// Enrollment needs no token, so this bounds what anonymous callers can
	// add to the log until a person adopts or removes some.
	LandingZoneLimit = 250
)

// TreeName picks one of the Org's two trees (0013).
type TreeName string

const (
	Locations TreeName = "locations"
	Services  TreeName = "services"
)

// Op is one change. Only the fields its Kind uses are set.
type Op struct {
	Kind     Kind               `json:"kind"`
	Tree     TreeName           `json:"tree,omitempty"`
	Node     hierarchy.NodeID   `json:"node,omitempty"`
	Parent   hierarchy.NodeID   `json:"parent,omitempty"`
	Name     string             `json:"name,omitempty"`
	Path     hierarchy.Path     `json:"path,omitempty"`
	Value    json.RawMessage    `json:"value,omitempty"`
	Services []hierarchy.NodeID `json:"services,omitempty"`
	// Values sets several fields of one node together, in place of Path and
	// Value: all of them or none (0045).
	Values map[hierarchy.Path]json.RawMessage `json:"values,omitempty"`
	// Paths unsets several fields of one node together, in place of Path
	// (0046).
	Paths []hierarchy.Path `json:"paths,omitempty"`
	// Ops are a batch's changes, each of a Batchable kind, made in order as
	// one (0203, 0320); or an end's (0345).
	Ops []Op `json:"ops,omitempty"`
	// Until is when a change that ends itself ends (0345), or, for
	// move-end, its new end; Ends is the change an end or move-end is of.
	Until time.Time `json:"until,omitzero"`
	Ends  int64     `json:"ends,omitempty"`

	Account   access.AccountID `json:"account,omitempty"`
	Role      string           `json:"role,omitempty"`
	TokenID   string           `json:"token_id,omitempty"`
	TokenHash []byte           `json:"token_hash,omitempty"`
	// A password's hash, never the password, and whether it is to be
	// changed at the next sign-in (0176); and whether set-owner makes or
	// unmakes an owner.
	PasswordHash string `json:"password_hash,omitempty"`
	MustChange   bool   `json:"must_change,omitempty"`
	Owner        *bool  `json:"owner,omitempty"`

	// An invite (0336): its ID and hash in TokenID and TokenHash; for
	// add-invite, its maker in Account, the roles it gives, whether it
	// makes an owner (Owner), until when it is good and a note; for
	// use-invite, the account it makes, its Name and PasswordHash.
	Roles   []RoleOn  `json:"roles,omitempty"`
	Expires time.Time `json:"expires,omitzero"`
	Note    string    `json:"note,omitempty"`

	Concentrator string `json:"concentrator,omitempty"`
	VNI          int    `json:"vni,omitempty"`

	// Per-user keys (0070): the network, in the Services folder Node, and
	// the key's ID. Value is its definition, its passphrase sealed.
	Network string `json:"network,omitempty"`
	Key     string `json:"key,omitempty"`

	// AP templates (0085): the template's ID; for add-template its level
	// (Parent), name and boards, Default to pick it there for them, and
	// Values to make it with its settings, as an import does (0090); for
	// edit-template its name and boards; for set-template and
	// unset-template its fields, as for set and unset.
	Template string   `json:"template,omitempty"`
	Boards   []string `json:"boards,omitempty"`
	Default  bool     `json:"default,omitempty"`

	// With is the AP waiting in Landing Zone that replace-ap puts in Node's
	// place (0223).
	With hierarchy.NodeID `json:"with,omitempty"`

	// Builtins is which set of built-in folders add-builtins makes, an index
	// into builtinSets: none, as every entry before 0333 was, is 0032's.
	// The manager makes today's, BuiltinsNow; an older one is still taken,
	// and makes no more than a few folders an Org admin could.
	Builtins int `json:"builtins,omitempty"`
}

// Field is one field a set changes.
type Field struct {
	Path  hierarchy.Path
	Value json.RawMessage
}

// Fields lists the fields a set changes: its path and value, or its values
// in path order.
func (op Op) Fields() []Field {
	if len(op.Values) == 0 {
		return []Field{{op.Path, op.Value}}
	}
	out := make([]Field, 0, len(op.Values))
	for p, v := range op.Values {
		out = append(out, Field{p, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Unsets lists the fields an unset removes: its path, or its paths.
func (op Op) Unsets() []hierarchy.Path {
	if len(op.Paths) == 0 {
		return []hierarchy.Path{op.Path}
	}
	return op.Paths
}

// Effect records what a change did, for the change log: the state before and
// after, and any overrides it removed (0017).
type Effect struct {
	Before  any                  `json:"before,omitempty"`
	After   any                  `json:"after,omitempty"`
	Removed []hierarchy.Override `json:"removed,omitempty"`
}

// State is everything the change log rebuilds.
type State struct {
	Org     *hierarchy.Org
	Access  *access.Access
	Library *library.Library
	// Facts holds what each AP last said of itself: when it enrolled
	// (0033), for the person deciding whether to adopt it, and again
	// whenever that changed (0122).
	Facts map[hierarchy.NodeID]json.RawMessage
	// Keys are the per-user keys (0070).
	Keys *keys.Store
	// Again holds what each AP that enrolled again said of itself then
	// (0223), until a person lets it back in.
	Again map[hierarchy.NodeID]json.RawMessage
}

// Clone returns an independent copy, or nil for nil.
func (s *State) Clone() *State {
	if s == nil {
		return nil
	}
	facts := make(map[hierarchy.NodeID]json.RawMessage, len(s.Facts))
	for ap, f := range s.Facts {
		facts[ap] = f // never modified in place
	}
	again := make(map[hierarchy.NodeID]json.RawMessage, len(s.Again))
	for ap, f := range s.Again {
		again[ap] = f
	}
	return &State{Org: s.Org.Clone(), Access: s.Access.Clone(), Library: s.Library.Clone(), Facts: facts, Keys: s.Keys.Clone(), Again: again}
}

var (
	ErrNoOrg       = errors.New("the Org does not exist yet; the first change must be create-org")
	ErrOrgExists   = errors.New("the Org already exists")
	ErrUnknownKind = errors.New("unknown change kind")
	ErrUnknownTree = errors.New("unknown tree")
	ErrNoValue     = errors.New("set needs a value")
	ErrNoNode      = errors.New("change needs a node")
	ErrNoPath      = errors.New("change needs a field path")
	ErrTwoForms    = errors.New("a set or unset takes one field (path) or several (values, paths), not both")
	ErrBadBatch    = errors.New("a bad batch")
	ErrNoAccount   = errors.New("change needs an account")
	ErrNoTokenID   = errors.New("change needs a token ID")
	ErrNoFacts     = errors.New("facts needs what the AP said of itself")
	ErrNoWith      = errors.New("replace-ap needs the AP waiting in Landing Zone to put in its place (with)")
	ErrUseAssign   = errors.New("service folders are set with assign-services")
	ErrNotAFolder  = errors.New("roles are granted on folders, not APs")
	ErrBuiltins    = errors.New("the built-in folders already exist")
	ErrBadBuiltins = errors.New("no such set of built-in folders")
	ErrReserved    = errors.New("that account name is reserved for the manager itself")
	ErrNoPassword  = errors.New("set-password needs the password's hash")
	ErrNoOwner     = errors.New("set-owner says whether the account is an owner (owner: true or false)")
	ErrNoConcID    = errors.New("change needs a concentrator")
	ErrBadName     = errors.New("a folder's name is 1 to 64 characters, none of them control characters")
	ErrBadHostname = errors.New("an AP's name is its hostname (0076): letters, digits and hyphens, 1 to 63 of them, starting and ending with a letter or digit")
	ErrNameTaken   = errors.New("another AP has that name")
	ErrInUse       = errors.New("still in use")
	ErrFull        = fmt.Errorf("Landing Zone holds %d APs, its limit: adopt or remove some before more can enroll", LandingZoneLimit)
)

// InUseError lists what still refers to a concentrator or VNI.
type InUseError struct {
	What string
	By   []string
}

func (e *InUseError) Error() string {
	return fmt.Sprintf("%s is %v by %v", e.What, ErrInUse, e.By)
}

func (e *InUseError) Unwrap() error { return ErrInUse }

// Apply applies op to s and reports its effect. It returns the state to use
// from then on: a new one for create-org, otherwise s itself. On error, s is
// unchanged. A renamed board is taken by its new name, whichever op says
// (0174).
func Apply(s *State, op Op) (*State, Effect, error) {
	op = currentBoards(op)
	if err := validate(op); err != nil {
		return s, Effect{}, err
	}
	if op.Kind == CreateOrg {
		if s != nil {
			return s, Effect{}, ErrOrgExists
		}
		return createOrg(op)
	}
	if s == nil {
		return nil, Effect{}, ErrNoOrg
	}
	// A batch makes its changes in order on a copy, which becomes s only
	// once every one is made: one that fails leaves s as it was. Its effect
	// is each one's, in order (0203).
	if op.Kind == Batch || op.Kind == EndChange {
		c := s.Clone()
		effs := make([]Effect, 0, len(op.Ops))
		for i, sub := range op.Ops {
			var e Effect
			var err error
			if c, e, err = Apply(c, sub); err != nil {
				return s, Effect{}, fmt.Errorf("change %d of %d (%s %s %s): %w", i+1, len(op.Ops), sub.Kind, sub.Tree, sub.Node, err)
			}
			effs = append(effs, e)
		}
		*s = *c
		return s, Effect{After: effs}, nil
	}
	var eff Effect
	var err error
	switch op.Kind {
	case MoveEnd:
		// The log keeps when each change ends; fields set with it are set.
		eff = Effect{After: op.Until}
		if len(op.Values) > 0 {
			var t *hierarchy.Tree
			if t, err = tree(s.Org, op.Tree); err == nil {
				eff, err = set(t, Op{Kind: Set, Tree: op.Tree, Node: op.Node, Values: op.Values})
			}
		}
	case AddBuiltins:
		eff, err = addBuiltins(s.Org, builtinSets[op.Builtins])
	case SetConcentrator, RemoveConcentrator, SetVNI, RemoveVNI:
		eff, err = applyLibrary(s, op)
	case AddTemplate, EditTemplate, SetTemplate, UnsetTemplate, RemoveTemplate:
		eff, err = applyTemplate(s, op)
		if err == nil {
			err = checkTemplates(s)
		}
	case AddAccount, IssueToken, RevokeToken, GrantRole, RevokeRole,
		SetPassword, DisableAccount, EnableAccount, RemoveAccount, SetOwner, SetAccount:
		eff, err = applyAccess(s, op)
	case AddInvite, RevokeInvite, UseInvite:
		eff, err = applyInvite(s, op)
	case Enroll, RemoveAP, Facts, ReadmitAP, ReplaceAP:
		eff, err = applyAP(s, op)
	case RemoveFolder:
		eff, err = removeFolder(s, op)
	case AddKey, SetKey, RemoveKey:
		eff, err = applyKey(s, op)
	default:
		eff, err = apply(s.Org, op)
		if err == nil {
			err = checkKeys(s)
		}
		if err == nil {
			err = checkTemplates(s)
		}
	}
	return s, eff, err
}

func validate(op Op) error {
	if !op.Until.IsZero() && op.Kind != MoveEnd && !Endable(op) {
		return fmt.Errorf("%w: only a set, unset, add-folder, assign-services or a batch of them ends itself, not %s", ErrBadUntil, op.Kind)
	}
	switch op.Kind {
	case EndChange, MoveEnd:
		return validateEnd(op)
	case AddBuiltins:
		if op.Builtins < 0 || op.Builtins >= len(builtinSets) {
			return fmt.Errorf("%w: built-in folders %d; there are sets 0 to %d", ErrBadBuiltins, op.Builtins, len(builtinSets)-1)
		}
		return nil
	case Batch:
		if len(op.Ops) == 0 || len(op.Ops) > MaxBatch {
			return fmt.Errorf("%w: a batch makes 1 to %d changes", ErrBadBatch, MaxBatch)
		}
		for i, sub := range op.Ops {
			if !Batchable(sub.Kind) {
				return fmt.Errorf("%w: change %d is %s; a batch makes %s alone", ErrBadBatch, i+1, sub.Kind, batchKinds())
			}
			if !sub.Until.IsZero() {
				return fmt.Errorf("%w: change %d has its own until; a batch ends as one, by the batch's (0345)", ErrBadUntil, i+1)
			}
		}
		return nil
	case SetConcentrator, RemoveConcentrator, SetVNI, RemoveVNI:
		if op.Concentrator == "" {
			return ErrNoConcID
		}
		return nil
	case AddTemplate, EditTemplate, SetTemplate, UnsetTemplate, RemoveTemplate:
		return validateTemplate(op)
	case CreateOrg:
		if op.Account == "" {
			return ErrNoAccount
		}
	case Rename:
		if op.Name == "" {
			return ErrBadName
		}
	case AddFolder, AddAP, Move, BreakHierarchy, AssignServices:
	case Set:
		if len(op.Values) > 0 && (op.Path != "" || len(op.Value) > 0) {
			return ErrTwoForms
		}
		for _, f := range op.Fields() {
			if f.Path == "" {
				return ErrNoPath
			}
		}
	case Unset:
		if len(op.Paths) > 0 && op.Path != "" {
			return ErrTwoForms
		}
		for _, p := range op.Unsets() {
			if p == "" {
				return ErrNoPath
			}
		}
	case Lock, Unlock:
		if op.Path == "" {
			return ErrNoPath
		}
	case AddAccount, IssueToken:
		if op.Account == "" {
			return ErrNoAccount
		}
		if op.Kind == IssueToken && op.TokenID == "" {
			return ErrNoTokenID
		}
		return nil
	case RevokeToken:
		if op.TokenID == "" {
			return ErrNoTokenID
		}
		return nil
	case AddInvite, RevokeInvite, UseInvite:
		return validateInvite(op)
	case SetPassword, DisableAccount, EnableAccount, RemoveAccount, SetOwner, SetAccount:
		if op.Account == "" {
			return ErrNoAccount
		}
		switch {
		case op.Kind == SetPassword && op.PasswordHash == "":
			return ErrNoPassword
		case op.Kind == SetOwner && op.Owner == nil:
			return ErrNoOwner
		case op.Kind == SetAccount:
			return folderName(op.Name)
		}
		return nil
	case Enroll:
		if op.TokenID == "" {
			return ErrNoTokenID
		}
	case Facts:
		if len(op.Value) == 0 {
			return ErrNoFacts
		}
	case RemoveAP, RemoveFolder, ReadmitAP:
	case ReplaceAP:
		if op.With == "" {
			return ErrNoWith
		}
	case AddKey, SetKey, RemoveKey:
		return validateKey(op)
	case GrantRole, RevokeRole:
		if op.Account == "" {
			return ErrNoAccount
		}
	default:
		return fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
	}
	if op.Node == "" {
		return ErrNoNode
	}
	return nil
}

// createOrg starts the state. The Org's first account is its admin at the
// root of both trees, and its owner (0176), so there is never a moment
// without one. An Org made before owners gets its owner the same way, as the
// log replays.
func createOrg(op Op) (*State, Effect, error) {
	s := &State{Org: hierarchy.NewOrg(op.Node, op.Name), Access: access.New(), Library: library.New(), Facts: map[hierarchy.NodeID]json.RawMessage{}, Keys: keys.New(), Again: map[hierarchy.NodeID]json.RawMessage{}}
	if err := s.Access.AddAccount(op.Account, string(op.Account)); err != nil {
		return nil, Effect{}, err
	}
	if err := s.Access.SetOwner(op.Account, true); err != nil {
		return nil, Effect{}, err
	}
	for _, t := range []TreeName{Locations, Services} {
		if err := s.Access.Grant(access.Grant{Account: op.Account, Tree: string(t), Node: op.Node, Role: access.Admin}); err != nil {
			return nil, Effect{}, err
		}
	}
	return s, Effect{After: map[string]string{"node": string(op.Node), "name": op.Name, "admin": string(op.Account)}}, nil
}

// builtin is one built-in folder: the tree it is in, its ID and name, and
// whether it is isolated, outside inheritance (0032).
type builtin struct {
	tree     TreeName
	id       hierarchy.NodeID
	name     string
	isolated bool
}

// builtinSets are the sets of built-in folders, oldest first. An entry
// replays with the set it was made with, so what it made is made again; a
// new one takes the newest, BuiltinsNow.
var builtinSets = [][]builtin{
	// 0032: Landing Zone in Locations, and Sandbox in both trees.
	{
		{Locations, LandingZone, "Landing Zone", true},
		{Locations, Sandbox, "Sandbox", false},
		{Services, Sandbox, "Sandbox", false},
	},
	// 0333: Landing Zone alone. A Sandbox made before stays, an ordinary
	// folder.
	{
		{Locations, LandingZone, "Landing Zone", true},
	},
}

// BuiltinsNow is the set of built-in folders the manager makes today (0333).
var BuiltinsNow = len(builtinSets) - 1

// addBuiltins creates whichever of a set's built-in folders are missing.
func addBuiltins(o *hierarchy.Org, set []builtin) (Effect, error) {
	var created []string
	for _, b := range set {
		t, err := tree(o, b.tree)
		if err != nil {
			return Effect{}, err
		}
		if n, ok := t.Node(b.id); ok {
			if n.Isolated != b.isolated || n.Parent != t.Root() {
				return Effect{}, fmt.Errorf("%w: %s/%s exists but is not the built-in folder", hierarchy.ErrExists, b.tree, b.id)
			}
			continue
		}
		if b.isolated {
			err = t.AddIsolated(b.id, b.name, t.Root())
		} else {
			err = t.AddFolder(b.id, b.name, t.Root())
		}
		if err != nil {
			return Effect{}, err
		}
		created = append(created, string(b.tree)+"/"+string(b.id))
	}
	if len(created) == 0 {
		return Effect{}, ErrBuiltins
	}
	return Effect{After: created}, nil
}

func apply(o *hierarchy.Org, op Op) (Effect, error) {
	if op.Kind == AssignServices {
		before, _ := o.Locations.Own(op.Node, hierarchy.ServicesPath)
		if err := o.AssignServices(op.Node, op.Services); err != nil {
			return Effect{}, err
		}
		return Effect{Before: before, After: op.Services}, nil
	}

	t, err := tree(o, op.Tree)
	if err != nil {
		return Effect{}, err
	}
	switch op.Kind {
	case AddFolder:
		return Effect{After: placement(op)}, t.AddFolder(op.Node, op.Name, op.Parent)
	case AddAP:
		return Effect{After: placement(op)}, t.AddAP(op.Node, op.Name, op.Parent)
	case Rename:
		return rename(t, op)
	case Move:
		n, ok := t.Node(op.Node)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
		}
		removed, err := t.Move(op.Node, op.Parent)
		return Effect{Before: n.Parent, After: op.Parent, Removed: removed}, err
	case Set:
		return set(t, op)
	case Unset:
		return unset(t, op)
	case Lock:
		removed, err := t.Lock(op.Node, op.Path)
		return Effect{Before: false, After: true, Removed: removed}, err
	case Unlock:
		return Effect{Before: true, After: false}, t.Unlock(op.Node, op.Path)
	case BreakHierarchy:
		return Effect{Before: false, After: true}, t.BreakHierarchy(op.Node)
	}
	return Effect{}, fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

// folderIDRE is what a new folder's ID must be: it goes into addresses.
var folderIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ErrBadFolderID refuses a new folder's ID that isn't one.
var ErrBadFolderID = errors.New("a folder's ID is 1 to 32 lowercase letters, digits and hyphens, starting with a letter or digit")

// Guard checks what only a new change must meet, not one replayed from the
// log, which may predate the rule: a new folder's ID and name (0076).
func Guard(op Op) error {
	if op.Kind != AddFolder {
		return nil
	}
	if !folderIDRE.MatchString(string(op.Node)) {
		return ErrBadFolderID
	}
	return folderName(op.Name)
}

// folderName checks a folder's name: 1 to 64 characters, none of them
// control characters.
func folderName(name string) error {
	if name == "" || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return ErrBadName
	}
	return nil
}

// hostnameRE is a hostname label (RFC 1123): what an AP's name must be (0076).
var hostnameRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// rename renames a folder or an AP (0076). An AP's name is its hostname, so
// it must be one, and no other AP's.
func rename(t *hierarchy.Tree, op Op) (Effect, error) {
	n, ok := t.Node(op.Node)
	if !ok {
		return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotFound, op.Node)
	}
	if n.Kind == hierarchy.KindAP {
		if !hostnameRE.MatchString(op.Name) {
			return Effect{}, ErrBadHostname
		}
		for _, ap := range t.APs() {
			if other, _ := t.Node(ap); ap != op.Node && strings.EqualFold(other.Name, op.Name) {
				return Effect{}, fmt.Errorf("%w: %s", ErrNameTaken, ap)
			}
		}
	} else if err := folderName(op.Name); err != nil {
		return Effect{}, err
	}
	old, err := t.Rename(op.Node, op.Name)
	return Effect{Before: old, After: op.Name}, err
}

// set sets one field, or several together (0045). If any of them cannot be
// set, those already set are put back, so none of them change.
func set(t *hierarchy.Tree, op Op) (Effect, error) {
	fields := op.Fields()
	values := make([]hierarchy.Value, len(fields))
	for i, f := range fields {
		if op.Tree == Locations && f.Path == hierarchy.ServicesPath {
			return Effect{}, ErrUseAssign
		}
		v, err := decode(f.Value)
		if err != nil {
			return Effect{}, err
		}
		values[i] = v
	}
	if len(op.Values) == 0 {
		before, _ := t.Own(op.Node, op.Path)
		return Effect{Before: before, After: values[0]}, t.Set(op.Node, op.Path, values[0])
	}
	before, after := map[string]any{}, map[string]any{} // string keys, as read back from the log
	for i, f := range fields {
		if v, ok := t.Own(op.Node, f.Path); ok {
			before[string(f.Path)] = v
		}
		if err := t.Set(op.Node, f.Path, values[i]); err != nil {
			for _, done := range fields[:i] {
				if v, ok := before[string(done.Path)]; ok {
					t.Set(op.Node, done.Path, v)
				} else {
					t.Unset(op.Node, done.Path)
				}
			}
			return Effect{}, err
		}
		after[string(f.Path)] = values[i]
	}
	if len(before) == 0 {
		return Effect{After: after}, nil
	}
	return Effect{Before: before, After: after}, nil
}

// unset unsets one field, or several together (0046). Each must be set at
// the node; if one is not, none is unset.
func unset(t *hierarchy.Tree, op Op) (Effect, error) {
	if len(op.Paths) == 0 {
		before, _ := t.Own(op.Node, op.Path)
		return Effect{Before: before}, t.Unset(op.Node, op.Path)
	}
	before := map[string]any{} // string keys, as read back from the log
	for _, p := range op.Paths {
		v, ok := t.Own(op.Node, p)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", hierarchy.ErrNotSetHere, p)
		}
		before[string(p)] = v
	}
	for _, p := range op.Paths {
		t.Unset(op.Node, p) // each is set here, so none fails
	}
	return Effect{Before: before}, nil
}

func applyAccess(s *State, op Op) (Effect, error) {
	a := s.Access
	switch op.Kind {
	case AddAccount:
		if op.Account == SystemActor {
			return Effect{}, ErrReserved
		}
		return Effect{After: map[string]string{"account": string(op.Account), "name": op.Name}}, a.AddAccount(op.Account, op.Name)
	case IssueToken:
		return Effect{After: map[string]string{"account": string(op.Account), "token": op.TokenID}}, a.AddToken(op.TokenID, op.Account, op.TokenHash)
	case RevokeToken:
		tok, ok := a.Token(op.TokenID)
		if !ok {
			return Effect{}, fmt.Errorf("%w: %s", access.ErrNoToken, op.TokenID)
		}
		return Effect{Before: map[string]string{"account": string(tok.Account), "token": tok.ID}}, a.RevokeToken(op.TokenID)
	case SetAccount:
		acc, _ := a.Account(op.Account)
		return Effect{Before: map[string]string{"account": string(op.Account), "name": acc.Name}, After: map[string]string{"account": string(op.Account), "name": op.Name}},
			a.SetName(op.Account, op.Name)
	case SetPassword:
		// The effect says a password was set, never what it is.
		return Effect{After: map[string]any{"account": string(op.Account), "must_change": op.MustChange}}, a.SetPassword(op.Account, op.PasswordHash, op.MustChange)
	case DisableAccount, EnableAccount:
		off := op.Kind == DisableAccount
		return Effect{Before: map[string]any{"account": string(op.Account), "disabled": !off}, After: map[string]any{"account": string(op.Account), "disabled": off}},
			a.SetDisabled(op.Account, off)
	case SetOwner:
		return Effect{Before: map[string]any{"account": string(op.Account), "owner": !*op.Owner}, After: map[string]any{"account": string(op.Account), "owner": *op.Owner}},
			a.SetOwner(op.Account, *op.Owner)
	case RemoveAccount:
		tokens, grants, err := a.RemoveAccount(op.Account)
		return Effect{Before: map[string]any{"account": string(op.Account), "tokens": tokens, "grants": grants}}, err
	}

	// Grant or revoke a role on a folder.
	g, err := folderRole(s, op.Tree, op.Node, op.Role)
	if err != nil {
		return Effect{}, err
	}
	g.Account = op.Account
	view := map[string]string{"account": string(g.Account), "tree": g.Tree, "node": string(g.Node), "role": g.Role.String()}
	if op.Kind == GrantRole {
		return Effect{After: view}, a.Grant(g)
	}
	return Effect{Before: view}, a.Revoke(g)
}

func tree(o *hierarchy.Org, name TreeName) (*hierarchy.Tree, error) {
	switch name {
	case Locations:
		return o.Locations, nil
	case Services:
		return o.Services, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownTree, name)
}

func placement(op Op) map[string]string {
	return map[string]string{"name": op.Name, "parent": string(op.Parent)}
}

// decode turns a JSON value into the plain Go value stored in the tree. Values
// always pass through JSON, live and on replay, so both see the same types.
func decode(raw json.RawMessage) (hierarchy.Value, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, ErrNoValue
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("value: %w", err)
	}
	return v, nil
}

// batchable is what a batch may hold (0203): the sets and unsets a page's
// one save makes, and since 0320 a new folder and the service folders a
// location takes, so a page can make a services folder, have a location
// take it and put a network in it as one change. A kind joins here once it
// changes the Org alone, as these do, and is made on the batch's copy.
var batchable = []Kind{Set, Unset, AddFolder, AssignServices}

// Batchable says whether a batch may hold a change of kind k.
func Batchable(k Kind) bool {
	for _, b := range batchable {
		if b == k {
			return true
		}
	}
	return false
}

// makesNodes says whether a batch's changes make a node a later one of them
// may name, so each is allowed on what those before it leave (0320).
func makesNodes(ops []Op) bool {
	for _, o := range ops {
		if o.Kind == AddFolder {
			return true
		}
	}
	return false
}

func batchKinds() string {
	s := make([]string, len(batchable))
	for i, k := range batchable {
		s[i] = string(k)
	}
	return strings.Join(s, ", ")
}
