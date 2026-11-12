package change

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

var (
	ErrUnknownActor = errors.New("the actor has no account")
	ErrForbidden    = errors.New("not permitted")
)

// ForbiddenError says which right the actor lacked, and where (0177); or
// that it takes an owner (0176).
type ForbiddenError struct {
	Actor string
	Need  string
	Tree  TreeName
	Node  hierarchy.NodeID
	Owner bool
}

func (e *ForbiddenError) Error() string {
	if e.Owner {
		return fmt.Sprintf("%s: %v: needs an owner", e.Actor, ErrForbidden)
	}
	if e.Node == "" {
		return fmt.Sprintf("%s: %v: needs %s", e.Actor, ErrForbidden, e.Need)
	}
	return fmt.Sprintf("%s: %v: needs %s on %s in %s", e.Actor, ErrForbidden, e.Need, e.Node, e.Tree)
}

func (e *ForbiddenError) Unwrap() error { return ErrForbidden }

// Authorize decides whether actor may make op, given the current state (0025,
// 0030). The change log calls it under its lock, just before applying, so the
// decision and the change see the same state. A target that does not exist is
// left for Apply to report.
func Authorize(s *State, actor string, op Op) error {
	if op.Kind == CreateOrg {
		if string(op.Account) != actor {
			return fmt.Errorf("%w: create-org must name its caller as the first admin", ErrForbidden)
		}
		return nil
	}
	if s == nil {
		return ErrNoOrg
	}
	// A batch is each of its changes, each allowed as if made alone (0203).
	// One that makes a folder is allowed a change at a time, each on what
	// those before it leave, so a change in the new folder is judged where
	// the folder will be (0320). A change that cannot be made stops the
	// judging: Apply, made for real, says why.
	if op.Kind == Batch {
		if !makesNodes(op.Ops) {
			for _, sub := range op.Ops {
				if err := Authorize(s, actor, sub); err != nil {
					return err
				}
			}
			return nil
		}
		c := s.Clone()
		for _, sub := range op.Ops {
			if err := Authorize(c, actor, sub); err != nil {
				return err
			}
			next, _, err := Apply(c, sub)
			if err != nil {
				return nil
			}
			c = next
		}
		return nil
	}
	// An end is each of what it puts back, allowed as if made alone; the
	// manager makes one in its own name at the change's until (0345), what
	// the change log works out and nothing else.
	if op.Kind == EndChange {
		if actor == SystemActor {
			return nil
		}
		for _, sub := range op.Ops {
			if err := Authorize(s, actor, sub); err != nil {
				return err
			}
		}
		return nil
	}
	if actor == SystemActor {
		return authorizeSystem(s, op)
	}
	who := access.AccountID(actor)
	me, ok := s.Access.Account(who)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownActor, actor)
	}
	if !me.Active() {
		return fmt.Errorf("%w: %s: %v", ErrForbidden, actor, access.ErrInactive)
	}
	// owner: only an owner runs the Org's people (0176). self: an account
	// may always see to its own password, name and tokens.
	owner := func() error {
		if me.Owner {
			return nil
		}
		return &ForbiddenError{Actor: actor, Owner: true}
	}
	selfOrOwner := func(account access.AccountID) error {
		if account == who {
			return nil
		}
		return owner()
	}
	// chain is node's ancestry in tn, nil where either is unknown: Apply
	// reports those.
	chain := func(tn TreeName, node hierarchy.NodeID) []hierarchy.NodeID {
		t, err := tree(s.Org, tn)
		if err != nil {
			return nil
		}
		return t.Ancestry(node)
	}
	need := func(x access.Right, tn TreeName, node hierarchy.NodeID) error {
		c := chain(tn, node)
		if c == nil || s.Access.May(who, string(tn), c, x) {
			return nil
		}
		return &ForbiddenError{Actor: actor, Need: string(x), Tree: tn, Node: node}
	}
	// give: giving a role, or taking it, needs a role there that gives it
	// (0177).
	give := func(r access.Role, tn TreeName, node hierarchy.NodeID) error {
		c := chain(tn, node)
		if c == nil || s.Access.MayGive(who, string(tn), c, r) {
			return nil
		}
		return &ForbiddenError{Actor: actor, Need: "to give " + r.String(), Tree: tn, Node: node}
	}
	isAP := func(tn TreeName, node hierarchy.NodeID) bool {
		if tn != Locations {
			return false
		}
		n, ok := s.Org.Locations.Node(node)
		return ok && n.Kind == hierarchy.KindAP
	}

	switch op.Kind {
	case MoveEnd:
		// Whoever may make the change again may move its end: the change
		// log judges that, with the change (0345). Fields set with it are
		// allowed as a set of them.
		if len(op.Values) == 0 {
			return nil
		}
		return Authorize(s, actor, Op{Kind: Set, Tree: op.Tree, Node: op.Node, Values: op.Values})
	case AddFolder:
		return need(access.Configure, op.Tree, op.Parent)
	case AddAP:
		return need(access.APs, op.Tree, op.Parent)
	case Set, Unset:
		for _, x := range settingRights(op, isAP(op.Tree, op.Node)) {
			if err := need(x, op.Tree, op.Node); err != nil {
				return err
			}
		}
		return nil
	case Rename:
		if isAP(op.Tree, op.Node) {
			return need(access.APs, op.Tree, op.Node)
		}
		return need(access.Configure, op.Tree, op.Node)
	case Move:
		if t, err := tree(s.Org, op.Tree); err == nil && op.Tree == Locations {
			if n, ok := t.Node(op.Node); ok && n.Kind == hierarchy.KindAP {
				if t.InIsolated(n.Parent) && !t.InIsolated(op.Parent) {
					// Adoption out of Landing Zone (0032): the destination decides.
					if err := need(access.View, op.Tree, n.Parent); err != nil {
						return err
					}
					return need(access.APs, op.Tree, op.Parent)
				}
				if err := need(access.APs, op.Tree, op.Node); err != nil {
					return err
				}
				return need(access.APs, op.Tree, op.Parent)
			}
		}
		if err := need(access.Configure, op.Tree, op.Node); err != nil {
			return err
		}
		return need(access.Configure, op.Tree, op.Parent)
	case AssignServices:
		if err := need(access.Configure, Locations, op.Node); err != nil {
			return err
		}
		for _, f := range op.Services {
			if err := need(access.View, Services, f); err != nil {
				return err
			}
		}
		return nil
	case Lock, Unlock:
		return need(access.Administer, op.Tree, op.Node)
	case BreakHierarchy:
		if err := need(access.Administer, op.Tree, op.Node); err != nil {
			return err
		}
		t, err := tree(s.Org, op.Tree)
		if err != nil {
			return nil
		}
		for _, l := range t.LocksAbove(op.Node) {
			if err := need(access.Administer, op.Tree, l.Node); err != nil {
				return err
			}
		}
		return nil
	case AddBuiltins:
		root := s.Org.Locations.Root()
		if err := need(access.Administer, Locations, root); err != nil {
			return err
		}
		return need(access.Administer, Services, root)
	case AddKey, SetKey, RemoveKey:
		// A network's keys are its folder's: a leasing office's, say
		// (0070), or a building manager's (0177).
		return need(access.Keys, Services, op.Node)
	case SetConcentrator, RemoveConcentrator, SetVNI, RemoveVNI:
		// The library serves every network (0037).
		return need(access.Administer, Services, s.Org.Services.Root())
	case AddTemplate:
		// A template is its level's, as a setting there is (0085).
		return need(access.Configure, Locations, op.Parent)
	case EditTemplate, SetTemplate, UnsetTemplate, RemoveTemplate:
		if tm, ok := s.Library.Template(op.Template); ok {
			return need(access.Configure, Locations, tm.At)
		}
		return nil // Apply reports the unknown template
	case AddAccount, DisableAccount, EnableAccount, RemoveAccount, SetOwner:
		return owner()
	case SetPassword, SetAccount:
		return selfOrOwner(op.Account)
	case AddInvite:
		// An owner's, in its own name (0336).
		if op.Account != who {
			return fmt.Errorf("%w: an invite names its maker, the one making it", ErrForbidden)
		}
		return owner()
	case RevokeInvite:
		return owner()
	case UseInvite:
		return fmt.Errorf("%w: an invite is used by the one it is for, through POST /v1/register", ErrForbidden)
	case GrantRole, RevokeRole:
		// An admin of a folder gives any role there; a manager, a manager,
		// an installer or a viewer (0177). Apply reports a role that is no
		// such one.
		r, err := access.ParseRole(op.Role)
		if err != nil {
			return nil
		}
		return give(r, op.Tree, op.Node)
	case Enroll:
		return fmt.Errorf("%w: APs enroll themselves, through POST /v1/enroll", ErrForbidden)
	case Facts:
		return fmt.Errorf("%w: an AP says what it is itself, through POST /v1/ap/facts", ErrForbidden)
	case RemoveAP, ReadmitAP:
		return need(access.APs, Locations, op.Node)
	case ReplaceAP:
		// The AP whose place it is, and the one put there (0223).
		if err := need(access.APs, Locations, op.Node); err != nil {
			return err
		}
		return need(access.APs, Locations, op.With)
	case RemoveFolder:
		// As making one: configure where it hangs. Roles granted on it go
		// with it, which is for whoever may take them (0177).
		t, err := tree(s.Org, op.Tree)
		if err != nil {
			return nil // Apply reports the unknown tree
		}
		n, ok := t.Node(op.Node)
		if !ok {
			return nil // Apply reports the unknown node
		}
		if n.Parent == "" {
			return need(access.Administer, op.Tree, op.Node)
		}
		// Configure where it hangs, whatever is granted on the folder
		// itself: an admin of the folder alone does not decide what its
		// parent holds.
		if err := need(access.Configure, op.Tree, n.Parent); err != nil {
			return err
		}
		for _, g := range s.Access.Grants() {
			if g.Tree == string(op.Tree) && g.Node == op.Node {
				if err := give(g.Role, op.Tree, op.Node); err != nil {
					return err
				}
			}
		}
		return nil
	case IssueToken:
		return selfOrOwner(op.Account)
	case RevokeToken:
		if tok, ok := s.Access.Token(op.TokenID); ok && tok.Account == who {
			return nil
		}
		return owner()
	}
	return fmt.Errorf("%w: %q", ErrUnknownKind, op.Kind)
}

// authorizeSystem limits what the manager may do in its own name (0036):
// create the built-in folders, enroll APs into Landing Zone while it has
// room (0038), and make the template of a kind of AP no template is for, at
// the Org, picked there, with nothing in it (0085).
func authorizeSystem(s *State, op Op) error {
	switch op.Kind {
	case AddBuiltins:
		return nil
	case AddTemplate:
		if op.Parent != s.Org.Locations.Root() || !op.Default || len(op.Values) > 0 {
			return ErrDefaultTemplate
		}
		for _, tm := range s.Library.Templates() {
			for _, b := range op.Boards {
				if tm.ForBoard(b) {
					return ErrDefaultTemplate
				}
			}
		}
		return nil
	case Enroll:
		if len(s.Org.Locations.Descendants(LandingZone)) >= LandingZoneLimit {
			return ErrFull
		}
		return nil
	case Facts:
		return nil
	case UseInvite:
		// For one who registered with an invite's link, its secret checked
		// (0336); whether it is still good is the commit check's, by the
		// clock.
		if _, ok := s.Access.Invite(op.TokenID); !ok {
			return fmt.Errorf("%w: %s", access.ErrNoInvite, op.TokenID)
		}
		return nil
	}
	return fmt.Errorf("%w: the manager itself may only add built-in folders, enroll APs into Landing Zone, record what an AP says it is, make a new kind of AP's template, and make the account of one who registered with an invite", ErrForbidden)
}

// settingRights is what setting or unsetting op's fields takes (0177): the
// credentials the manager signs in with are an admin's (0168); an AP's own
// ports and uplink, and where it is on its map, an installer's as well as
// whoever configures; the clients a network blocks, and where a client
// belongs (0319), whoever runs clients;
// switching a test network on, whoever has the test SSID right (0179); the
// rest, configure.
func settingRights(op Op, ap bool) []access.Right {
	paths := append([]hierarchy.Path{op.Path}, op.Paths...)
	for p := range op.Values {
		paths = append(paths, p)
	}
	var out []access.Right
	add := func(x access.Right) {
		if !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	for _, p := range paths {
		switch s := string(p); {
		case s == "":
		case credential(p):
			add(access.Administer)
		case ap && (s == "ports" || s == "uplink" || strings.HasPrefix(s, "ports.") || strings.HasPrefix(s, "uplink.")):
			add(access.Ports)
		case ap && strings.HasPrefix(s, "system.place."):
			add(access.APs)
		case strings.HasPrefix(s, "system.updates."):
			// The manager's own releases (0183): an admin's, as the
			// credentials are.
			add(access.Administer)
		case strings.HasPrefix(s, "network.") && strings.HasSuffix(s, ".blocked"):
			add(access.Clients)
		case s == "homes" || strings.HasPrefix(s, "homes."):
			// Where a client belongs (0319), as what a network blocks.
			add(access.Clients)
		case s == "test_network" || strings.HasPrefix(s, "test_network."):
			add(access.TestSSID)
		default:
			add(access.Configure)
		}
	}
	if len(out) == 0 {
		add(access.Configure)
	}
	return out
}

// Credentials are the fields that name where the manager itself signs in,
// and with what: the DNS server whose log it asks (0154) and the switches
// it reads (0168), and the advisor's (0205), whose key is the Org's own and
// which says where the Org's data goes. Whoever sets where could send the
// secret set above it anywhere, so only an admin sets them (0168).
var Credentials = []string{"system.dns_log.", "system.switches.", "system.advisor."}

// credential says whether a field is one of Credentials.
func credential(p hierarchy.Path) bool {
	for _, c := range Credentials {
		if strings.HasPrefix(string(p), c) {
			return true
		}
	}
	return false
}

// managerOnly is the fields set in Locations that the manager itself uses,
// and that are no part of any AP's config, by what they start with: the DNS
// server whose log it asks (0154), the switches it reads (0168), where a
// folder or AP is on its map (0169), its own releases (0183) and its
// advisor (0205).
var managerOnly = []string{"system.dns_log.", "system.switches.", "system.place.", "system.updates.", "system.advisor."}

// ManagerOnly says whether a field is the manager's own: no AP is sent it,
// and a change to it gives no AP a new version (0169).
func ManagerOnly(p hierarchy.Path) bool {
	for _, m := range managerOnly {
		if strings.HasPrefix(string(p), m) {
			return true
		}
	}
	return false
}
