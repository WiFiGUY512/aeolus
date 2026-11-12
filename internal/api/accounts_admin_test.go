package api

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// ids is the accounts of an /v1/accounts answer, sorted.
func ids(body map[string]any) string {
	var out []string
	for _, a := range body["accounts"].([]any) {
		out = append(out, a.(map[string]any)["account"].(string))
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// grantsOf is one account's grants as an answer says them.
func grantsOf(acc map[string]any) string {
	var out []string
	for _, g := range acc["grants"].([]any) {
		g := g.(map[string]any)
		out = append(out, fmt.Sprintf("%s %s %s take=%v", g["tree"], g["node"], g["role"], g["may_take"]))
	}
	sort.Strings(out)
	return strings.Join(out, "; ")
}

// Who sees whom on the Admin page (0176, 0177): an owner everyone; an admin
// or a manager those holding roles where it may give them; anyone itself.
func TestPeople(t *testing.T) {
	f := newFixture(t)
	grant := func(account, tree, node, role string) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "grant", "account": account, "tree": tree, "node": node, "role": role}); code != 200 {
			t.Fatalf("grant: %d %v", code, b)
		}
	}
	grant("office", "locations", "house", "manager")
	grant("tenant", "locations", "office", "installer")

	_, all := f.do("GET", "/v1/accounts", "griff", nil)
	if got := ids(all); got != "claude griff office tenant" || all["may_add"] != true {
		t.Errorf("an owner sees %q, may add %v", got, all["may_add"])
	}
	_, mine := f.do("GET", "/v1/accounts", "claude", nil)
	if got := ids(mine); got != "claude" || mine["may_add"] != false {
		t.Errorf("an operator sees %q, may add %v", got, mine["may_add"])
	}
	// The manager of House sees the installer it may give and take there,
	// and of its other roles, those on folders it may view.
	_, managed := f.do("GET", "/v1/accounts", "office", nil)
	if got := ids(managed); got != "office tenant" {
		t.Errorf("a manager sees %q", got)
	}
	for _, a := range managed["accounts"].([]any) {
		if a := a.(map[string]any); a["account"] == "tenant" {
			if got, want := grantsOf(a), "locations office installer take=true; services household viewer take=false"; got != want {
				t.Errorf("tenant's roles, as its manager sees them: %q, want %q", got, want)
			}
		}
	}
	if code, _ := f.do("GET", "/v1/accounts/claude", "office", nil); code != 404 {
		t.Errorf("a manager reading an account it does not see: %d", code)
	}
	// Tokens and sessions are the account's own and an owner's.
	if _, b := f.do("GET", "/v1/accounts/tenant", "office", nil); b["tokens"] != nil || b["sessions"] != nil || b["may_run"] != false {
		t.Errorf("a manager reading its installer: %v", b)
	}
	if _, b := f.do("GET", "/v1/accounts/tenant", "griff", nil); b["tokens"] == nil || b["may_run"] != true {
		t.Errorf("an owner reading an account: %v", b)
	}
}

// An owner adds an account with a temporary password, and resets one;
// no one else may (0176).
func TestAddAndReset(t *testing.T) {
	f := newFixture(t)
	if code, b := f.do("POST", "/v1/accounts", "claude", map[string]any{"account": "staff1", "name": "Staff One"}); code != 403 {
		t.Errorf("an operator adding an account: %d %v", code, b)
	}
	code, b := f.do("POST", "/v1/accounts", "griff", map[string]any{"account": " Staff1 ", "name": "Staff One"})
	if code != 200 || b["account"] != "staff1" || len(b["password"].(string)) != 19 {
		t.Fatalf("an owner adding an account: %d %v", code, b)
	}
	first := b["password"].(string)
	cookie, _, who := f.signIn("staff1", first, false)
	if who["must_change"] != true {
		t.Errorf("a new account's first sign-in: %v", who)
	}
	_, read := f.do("GET", "/v1/accounts/staff1", "griff", nil)
	if acc := read["account"].(map[string]any); acc["must_change"] != true || acc["has_password"] != true || acc["state"] != "active" || acc["name"] != "Staff One" {
		t.Errorf("the new account: %v", acc)
	}
	if code, b := f.do("POST", "/v1/accounts", "griff", map[string]any{"account": "staff1", "name": "Again"}); code != 409 {
		t.Errorf("adding it again: %d %v", code, b)
	}

	// A reset ends its sessions, and the new temporary password is the one.
	code, b = f.do("POST", "/v1/accounts/staff1/password", "griff", nil)
	if code != 200 || b["password"] == first || b["sessions_ended"] != float64(1) {
		t.Fatalf("a reset: %d %v", code, b)
	}
	if resp, _ := f.session("GET", "/v1/whoami", cookie, false, nil); resp.StatusCode != 401 {
		t.Errorf("a session after its password was reset: %d", resp.StatusCode)
	}
	if _, resp, _ := f.signIn("staff1", b["password"].(string), false); resp.StatusCode != 200 {
		t.Errorf("signing in with the reset password: %d", resp.StatusCode)
	}
	if code, b := f.do("POST", "/v1/accounts/griff/password", "griff", nil); code != 400 {
		t.Errorf("resetting one's own: %d %v", code, b)
	}
	if code, b := f.do("POST", "/v1/accounts/tenant/password", "claude", nil); code != 404 {
		t.Errorf("an operator resetting another's: %d %v", code, b)
	}
}

// An account's tokens say who issued them and when they were last used;
// its sessions can be ended, all but this one or one at a time.
func TestTokensAndSessions(t *testing.T) {
	f := newFixture(t)
	f.do("GET", "/v1/whoami", "claude", nil)
	_, b := f.do("GET", "/v1/accounts/claude", "griff", nil)
	toks := b["tokens"].([]any)
	if len(toks) != 1 {
		t.Fatalf("claude's tokens: %v", toks)
	}
	if tok := toks[0].(map[string]any); tok["issued_by"] != "griff" || tok["issued_at"] == nil || tok["last_used"] == nil || tok["source"] == "" {
		t.Errorf("claude's token: %v", tok)
	}

	f.setPassword("tenant", "tenant password one", false)
	one, _, _ := f.signIn("tenant", "tenant password one", false)
	two, _, _ := f.signIn("tenant", "tenant password one", true)
	resp, b := f.session("GET", "/v1/accounts/tenant", one, false, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("reading one's own account: %d %v", resp.StatusCode, b)
	}
	list := b["sessions"].([]any)
	current := 0
	for _, s := range list {
		if s.(map[string]any)["current"] == true {
			current++
		}
	}
	if len(list) != 2 || current != 1 {
		t.Errorf("its sessions: %v", list)
	}
	if resp, b := f.session("DELETE", "/v1/accounts/tenant/sessions", one, true, nil); resp.StatusCode != 200 || b["ended"] != float64(1) {
		t.Errorf("ending the others: %d %v", resp.StatusCode, b)
	}
	if resp, _ := f.session("GET", "/v1/whoami", two, false, nil); resp.StatusCode != 401 {
		t.Errorf("the other session after: %d", resp.StatusCode)
	}
	if resp, _ := f.session("GET", "/v1/whoami", one, false, nil); resp.StatusCode != 200 {
		t.Errorf("this session after: %d", resp.StatusCode)
	}
	// One at a time, by an owner; not by another.
	_, b = f.do("GET", "/v1/accounts/tenant", "griff", nil)
	id := b["sessions"].([]any)[0].(map[string]any)["id"].(string)
	if code, b := f.do("DELETE", "/v1/accounts/tenant/sessions/"+id, "claude", nil); code != 404 {
		t.Errorf("an operator ending another's session: %d %v", code, b)
	}
	if code, b := f.do("DELETE", "/v1/accounts/tenant/sessions/"+id, "griff", nil); code != 200 || b["ended"] != float64(1) {
		t.Errorf("an owner ending it: %d %v", code, b)
	}
	if resp, _ := f.session("GET", "/v1/whoami", one, false, nil); resp.StatusCode != 401 {
		t.Errorf("the session an owner ended: %d", resp.StatusCode)
	}
}

// The sign-in record: an owner reads it all; a manager, of those holding
// roles where it holds the sign-ins right; anyone, its own.
func TestSignInRecord(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "grant", "account": "office", "tree": "locations", "node": "house", "role": "manager"}); code != 200 {
		t.Fatalf("grant: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "grant", "account": "tenant", "tree": "locations", "node": "office", "role": "installer"}); code != 200 {
		t.Fatalf("grant: %d %v", code, b)
	}
	f.setPassword("tenant", "tenant password one", false)
	f.signIn("tenant", "tenant password one", false)
	f.signIn("tenant", "not the password", false)
	f.signIn("claude", "claude has none", false)
	f.signIn("nobody", "no such account", false)

	said := func(as, query string) string {
		t.Helper()
		code, b := f.do("GET", "/v1/signins"+query, as, nil)
		if code != 200 {
			t.Fatalf("%s reading the record: %d %v", as, code, b)
		}
		var out []string
		for _, s := range b["signins"].([]any) {
			s := s.(map[string]any)
			out = append(out, fmt.Sprintf("%s:%v", s["account"], s["ok"]))
		}
		return strings.Join(out, " ")
	}
	if got, want := said("griff", ""), "nobody:false claude:false tenant:false tenant:true"; got != want {
		t.Errorf("an owner reads %q, want %q", got, want)
	}
	if got, want := said("griff", "?account=tenant&ok=false"), "tenant:false"; got != want {
		t.Errorf("an owner reads %q, want %q", got, want)
	}
	if got, want := said("office", ""), "tenant:false tenant:true"; got != want {
		t.Errorf("the manager of House reads %q, want %q", got, want)
	}
	if got, want := said("claude", ""), "claude:false"; got != want {
		t.Errorf("an operator reads %q, want %q", got, want)
	}
	if code, _ := f.do("GET", "/v1/signins?limit=0", "griff", nil); code != 400 {
		t.Errorf("a limit of 0: %d", code)
	}
}
