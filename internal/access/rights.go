package access

import "github.com/WiFiGUY512/aeolus/internal/hierarchy"

// Right is one thing a role may do (0177). A role is the set of rights it
// holds, not a rung: an account may do on a node what any role it holds
// there or above allows.
type Right string

const (
	// View sees everything there but packets.
	View Right = "view"
	// Administer locks, unlocks and breaks hierarchy; sets the credentials
	// the manager signs in with (0168); runs the library's tunnels (0037)
	// and the built-in folders.
	Administer Right = "administer"
	// Configure changes settings, networks, radios, folders, services and
	// templates.
	Configure Right = "configure"
	// Keys runs residents' per-user keys (0070).
	Keys Right = "keys"
	// Clients names, traces, reconnects and blocks a client.
	Clients Right = "clients"
	// APs adopts, adds, names, moves, places and removes an AP.
	APs Right = "aps"
	// Ports sets an AP's own ports and uplink.
	Ports Right = "ports"
	// Act asks an AP to act: locate, restart Wi-Fi, reboot, a client test,
	// an assurance round.
	Act Right = "actions"
	// Capture takes packets: starts a capture, and reads what one caught.
	Capture Right = "capture"
	// Shell opens a terminal on an AP (0198): a shell there as root.
	Shell Right = "shell"
	// TestSSID switches on the temporary test network.
	TestSSID Right = "test-ssid"
	// SignIns reads the sign-in record of those holding roles there.
	SignIns Right = "sign-ins"
)

// roleRights is what each role may do, as 0177's table has it. An owner,
// the super admin, may do all of it everywhere, and the Org's people
// besides.
var roleRights = map[Role][]Right{
	Admin:     {View, Administer, Configure, Keys, Clients, APs, Ports, Act, Capture, Shell, TestSSID, SignIns},
	Operator:  {View, Configure, Keys, Clients, APs, Ports, Act, TestSSID},
	Manager:   {View, Keys, Clients, SignIns},
	Installer: {View, APs, Ports, Act, TestSSID},
	Viewer:    {View},
}

// roleGives is which roles each role may give and take where it holds it
// (0177). Only an owner makes an owner.
var roleGives = map[Role][]Role{
	Admin:   {Admin, Operator, Manager, Installer, Viewer},
	Manager: {Manager, Installer, Viewer},
}

// Has says whether the role holds a right.
func (r Role) Has(x Right) bool {
	for _, y := range roleRights[r] {
		if y == x {
			return true
		}
	}
	return false
}

// Gives says whether the role may give or take another.
func (r Role) Gives(o Role) bool {
	for _, y := range roleGives[r] {
		if y == o {
			return true
		}
	}
	return false
}

// Rights is each role's rights, by name, for the UI to judge its buttons
// by. The manager decides.
func Rights() map[string][]Right {
	out := make(map[string][]Right, len(roleRights))
	for r, xs := range roleRights {
		out[r.String()] = xs
	}
	return out
}

// RolesAt is the roles an account holds on a node, given the node's
// ancestry in a tree (root first), granted there or above: roles flow down,
// through breaks (0025). An account that can't act holds none (0176).
func (a *Access) RolesAt(account AccountID, tree string, ancestry []hierarchy.NodeID) []Role {
	if acc, ok := a.accounts[account]; !ok || !acc.Active() {
		return nil
	}
	held := map[Role]bool{}
	for g := range a.grants {
		if g.Account != account || g.Tree != tree {
			continue
		}
		for _, n := range ancestry {
			if n == g.Node {
				held[g.Role] = true
				break
			}
		}
	}
	var out []Role
	for _, r := range []Role{Admin, Operator, Manager, Installer, Viewer} {
		if held[r] {
			out = append(out, r)
		}
	}
	return out
}

// May says whether an account may do x on a node, given the node's ancestry
// in a tree (root first). An owner may do everything everywhere (0176).
func (a *Access) May(account AccountID, tree string, ancestry []hierarchy.NodeID, x Right) bool {
	if len(ancestry) == 0 {
		return false
	}
	if acc, ok := a.accounts[account]; ok && acc.Active() && acc.Owner {
		return true
	}
	for _, r := range a.RolesAt(account, tree, ancestry) {
		if r.Has(x) {
			return true
		}
	}
	return false
}

// RightsAt is every right an account holds on a node.
func (a *Access) RightsAt(account AccountID, tree string, ancestry []hierarchy.NodeID) []Right {
	out := []Right{}
	for _, x := range roleRights[Admin] {
		if a.May(account, tree, ancestry, x) {
			out = append(out, x)
		}
	}
	return out
}

// MayGive says whether an account may give or take role r on a node.
func (a *Access) MayGive(account AccountID, tree string, ancestry []hierarchy.NodeID, r Role) bool {
	if len(ancestry) == 0 {
		return false
	}
	if acc, ok := a.accounts[account]; ok && acc.Active() && acc.Owner {
		return true
	}
	for _, held := range a.RolesAt(account, tree, ancestry) {
		if held.Gives(r) {
			return true
		}
	}
	return false
}

// RightsAnywhere is every right an account holds on some node of either
// tree: what the UI shows tabs for, such as Captures.
func (a *Access) RightsAnywhere(account AccountID) []Right {
	acc, ok := a.accounts[account]
	if !ok || !acc.Active() {
		return []Right{}
	}
	held := map[Right]bool{}
	for g := range a.grants {
		if g.Account == account {
			for _, x := range roleRights[g.Role] {
				held[x] = true
			}
		}
	}
	out := []Right{}
	for _, x := range roleRights[Admin] {
		if acc.Owner || held[x] {
			out = append(out, x)
		}
	}
	return out
}

// Gives is which roles each role gives, by name, for the UI's grant form.
func Gives() map[string][]string {
	out := map[string][]string{}
	for r, os := range roleGives {
		for _, o := range os {
			out[r.String()] = append(out[r.String()], o.String())
		}
	}
	return out
}

// GivesAt is the roles an account may give and take on a node: any, for an
// owner; those its roles there give, for anyone else.
func (a *Access) GivesAt(account AccountID, tree string, ancestry []hierarchy.NodeID) []Role {
	var out []Role
	for _, r := range []Role{Admin, Operator, Manager, Installer, Viewer} {
		if a.MayGive(account, tree, ancestry, r) {
			out = append(out, r)
		}
	}
	return out
}
