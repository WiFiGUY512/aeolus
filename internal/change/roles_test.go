package change

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The five folder roles (0177), each granted on Locations › Main Gate and
// Services › Household: gatekeeper the admin there (from people), and
// opgate, manager, installer and viewer.
func roled(t *testing.T) *State {
	t.Helper()
	s := keyed(t)
	L, S := Locations, Services
	for _, op := range []Op{
		{Kind: AddFolder, Tree: L, Node: "gatehouse", Name: "Gatehouse", Parent: "gate"},
		{Kind: GrantRole, Account: "gatekeeper", Tree: S, Node: "household", Role: "admin"},
	} {
		mustApply(t, s, op)
	}
	for _, who := range []string{"opgate", "manager", "installer", "viewer"} {
		role := who
		if who == "opgate" {
			role = "operator"
		}
		id := access.AccountID(who)
		mustApply(t, s, Op{Kind: AddAccount, Account: id, Name: who})
		mustApply(t, s, Op{Kind: GrantRole, Account: id, Tree: L, Node: "gate", Role: role})
		mustApply(t, s, Op{Kind: GrantRole, Account: id, Tree: S, Node: "household", Role: role})
	}
	return s
}

func TestWhatEachRoleMayDo(t *testing.T) {
	s := roled(t)
	L, S := Locations, Services
	set := func(node hierarchy.NodeID, values map[hierarchy.Path]string) Op {
		op := Op{Kind: Set, Tree: L, Node: node, Values: map[hierarchy.Path]json.RawMessage{}}
		for p, v := range values {
			op.Values[p] = json.RawMessage(v)
		}
		return op
	}
	give := func(role string) Op {
		return Op{Kind: GrantRole, Account: "claude", Tree: L, Node: "gate", Role: role}
	}
	addKey := keyOp(AddKey, "household", "sweet", "unit-101", `{"name":"Unit 101","passphrase":`+sealedPass+`}`)
	// Who of admin (gatekeeper), operator, manager, installer and viewer
	// may make each.
	cases := []struct {
		name string
		op   Op
		may  string
	}{
		{"a radio setting on an AP", set("gate-ap", map[hierarchy.Path]string{"radio.2g.width": `20`}), "admin operator"},
		{"an AP's own ports", set("gate-ap", map[hierarchy.Path]string{"ports.lan1.enabled": `false`}), "admin operator installer"},
		{"an AP's uplink", set("gate-ap", map[hierarchy.Path]string{"uplink.bond": `false`}), "admin operator installer"},
		{"where an AP is on the map", set("gate-ap", map[hierarchy.Path]string{"system.place.x": `0.5`}), "admin operator installer"},
		{"ports and a radio at once", set("gate-ap", map[hierarchy.Path]string{"ports.lan1.enabled": `false`, "radio.2g.width": `20`}), "admin operator"},
		{"a folder's ports", set("gate", map[hierarchy.Path]string{"ports.lan1.enabled": `false`}), "admin operator"},
		{"the manager's own updates (0183)", set("gate", map[hierarchy.Path]string{"system.updates.automatic": `true`}), "admin"},
		{"the credentials the manager signs in with", set("gate", map[hierarchy.Path]string{"system.dns_log.server": `"10.0.0.1"`}), "admin"},
		{"switching a test network on", set("gate", map[hierarchy.Path]string{"test_network.network": `"trial"`, "test_network.until": `1792533600`}), "admin operator installer"},
		{"naming an AP", Op{Kind: Rename, Tree: L, Node: "gate-ap", Name: "Gate"}, "admin operator installer"},
		{"naming a folder", Op{Kind: Rename, Tree: L, Node: "gatehouse", Name: "Lodge"}, "admin operator"},
		{"adding an AP", Op{Kind: AddAP, Tree: L, Node: "gate-ap-2", Name: "Gate2", Parent: "gate"}, "admin operator installer"},
		{"adding a folder", Op{Kind: AddFolder, Tree: L, Node: "lodge", Name: "Lodge", Parent: "gate"}, "admin operator"},
		{"moving an AP", Op{Kind: Move, Tree: L, Node: "gate-ap", Parent: "gatehouse"}, "admin operator installer"},
		{"removing an AP", Op{Kind: RemoveAP, Tree: L, Node: "gate-ap"}, "admin operator installer"},
		{"a lock", Op{Kind: Lock, Tree: L, Node: "gate", Path: "radio.2g.width"}, "admin"},
		{"a resident's key", addKey, "admin operator manager"},
		{"blocking a client", Op{Kind: Set, Tree: S, Node: "household", Path: "network.sweet.blocked", Value: json.RawMessage(`["aa:bb:cc:00:11:22"]`)}, "admin operator manager"},
		{"where a client belongs (0319)", set("gate", map[hierarchy.Path]string{"homes.aa:bb:cc:00:11:22.aps": `["gate-ap"]`}), "admin operator manager"},
		{"a network setting", Op{Kind: Set, Tree: S, Node: "household", Path: "network.sweet.ssid", Value: json.RawMessage(`"Sweet"`)}, "admin operator"},
		{"giving admin", give("admin"), "admin"},
		{"giving operator", give("operator"), "admin"},
		{"giving manager", give("manager"), "admin manager"},
		{"giving installer", give("installer"), "admin manager"},
		{"giving viewer", give("viewer"), "admin manager"},
		{"adding an account", Op{Kind: AddAccount, Account: "newbie", Name: "New"}, ""},
	}
	actors := map[string]string{"admin": "gatekeeper", "operator": "opgate", "manager": "manager", "installer": "installer", "viewer": "viewer"}
	for _, c := range cases {
		for role, actor := range actors {
			want := false
			for _, r := range strings.Fields(c.may) {
				want = want || r == role
			}
			if err := Authorize(s, actor, c.op); (err == nil) != want {
				t.Errorf("%s by %s: %v, want allowed %v", c.name, role, err, want)
			}
		}
	}
}

// A manager takes away what it may give, and no more.
func TestManagersTakeWhatTheyGive(t *testing.T) {
	s := roled(t)
	for role, ok := range map[string]bool{"installer": true, "viewer": true, "manager": true, "operator": false} {
		mustApply(t, s, Op{Kind: GrantRole, Account: "claude", Tree: Locations, Node: "gatehouse", Role: role})
		if err := Authorize(s, "manager", Op{Kind: RevokeRole, Account: "claude", Tree: Locations, Node: "gatehouse", Role: role}); (err == nil) != ok {
			t.Errorf("a manager taking %s: %v", role, err)
		}
	}
	// Removing a folder takes the roles on it, so it needs the leave to.
	if err := Authorize(s, "opgate", Op{Kind: RemoveFolder, Tree: Locations, Node: "gatehouse"}); err == nil {
		t.Error("an operator removes a folder an operator role is granted on")
	}
}
