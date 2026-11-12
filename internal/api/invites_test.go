package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/change"
)

// invite makes an invite as an account and returns its code, or "" and the
// answer where it was refused.
func (f *fixture) invite(as string, body map[string]any) (string, int, map[string]any) {
	f.t.Helper()
	code, b := f.do("POST", "/v1/invites", as, body)
	if code != 200 {
		return "", code, b
	}
	return b["code"].(string), code, b
}

// registerWith registers with an invite's code, and returns the session's
// cookie, if one was set, with the answer.
func (f *fixture) registerWith(code, account, password string) (string, int, map[string]any) {
	f.t.Helper()
	resp, b := f.session("POST", "/v1/register", "", false, map[string]any{"code": code, "account": account, "name": "New Comer", "password": password})
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookie && c.Value != "" {
			return c.Value, resp.StatusCode, b
		}
	}
	return "", resp.StatusCode, b
}

const joinPassword = "a long enough passphrase"

// Invites (0336): an owner makes a link with the roles it gives; the one it
// is for reads what it gives without signing in, registers once with their
// own password, and is signed in holding those roles. No one else makes,
// revokes or uses one; the log never shows its secret or the password.
func TestInvites(t *testing.T) {
	f := newFixture(t)
	viewer := map[string]any{"roles": []any{map[string]any{"tree": "services", "node": "household", "role": "viewer"}}, "days": 3, "note": "for a newcomer"}

	// Only an owner makes one, and never as a change sent in.
	if _, code, b := f.invite("claude", viewer); code != 403 {
		t.Fatalf("an operator makes an invite: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "add-invite", "account": "griff", "token_id": "x", "roles": viewer["roles"], "expires": time.Now().Add(time.Hour)}); code != 400 {
		t.Fatalf("add-invite through /v1/changes: %d %v", code, b)
	}
	for name, bad := range map[string]map[string]any{
		"no role":        {"days": 3},
		"on an AP":       {"roles": []any{map[string]any{"tree": "locations", "node": "office-ap", "role": "viewer"}}},
		"no such folder": {"roles": []any{map[string]any{"tree": "services", "node": "nowhere", "role": "viewer"}}},
		"no such role":   {"roles": []any{map[string]any{"tree": "services", "node": "household", "role": "king"}}},
		"too long":       {"roles": viewer["roles"], "days": 31},
		"a long note":    {"roles": viewer["roles"], "note": strings.Repeat("n", change.NoteMax+1)},
	} {
		if _, code, b := f.invite("griff", bad); code != 400 && code != 404 {
			t.Errorf("an invite with %s: %d %v", name, code, b)
		}
	}
	code, status, made := f.invite("griff", viewer)
	if status != 200 || !strings.HasSuffix(made["link"].(string), RegisterPath+code) || made["path"] != RegisterPath+code {
		t.Fatalf("an owner's invite: %d %v", status, made)
	}
	if iv := made["invite"].(map[string]any); iv["state"] != "open" || iv["by"] != "griff" || iv["note"] != "for a newcomer" {
		t.Fatalf("the invite made: %v", iv)
	}

	// Read without signing in: what it gives, and who sent it.
	resp, read := f.session("GET", "/v1/register/"+code, "", false, nil)
	if resp.StatusCode != 200 || read["org"] != "Example" || read["by"] != "griff" {
		t.Fatalf("reading the invite: %d %v", resp.StatusCode, read)
	}
	if r := read["roles"].([]any)[0].(map[string]any); r["where"] != "Household" || r["role"] != "viewer" {
		t.Fatalf("what it gives: %v", r)
	}
	id, _, _ := strings.Cut(code, ".")
	if resp, b := f.session("GET", "/v1/register/"+id+".notthesecret", "", false, nil); resp.StatusCode != 404 {
		t.Fatalf("a wrong secret: %d %v", resp.StatusCode, b)
	}

	// The account, the name and the password are checked before it is used.
	for _, try := range []struct {
		account, password string
		want              int
	}{
		{"Not An Account!", joinPassword, 400},
		{"claude", joinPassword, 409},
		{"aeolus", joinPassword, 409},
		{"aeolus-advisor", joinPassword, 409},
		{"newcomer", "short", 400},
		{"newcomer", "newcomer", 400},
	} {
		if c, st, b := f.registerWith(code, try.account, try.password); st != try.want || c != "" {
			t.Errorf("registering %q: %d %v", try.account, st, b)
		}
	}

	// Used: an account with the roles, signed in, its password its own.
	cookie, st, b := f.registerWith(code, "Newcomer", joinPassword)
	if st != 200 || cookie == "" || b["account"] != "newcomer" || b["owner"] != false {
		t.Fatalf("registering: %d %v", st, b)
	}
	if resp, me := f.session("GET", "/v1/whoami", cookie, false, nil); resp.StatusCode != 200 || me["account"] != "newcomer" || me["must_change"] != false {
		t.Fatalf("whoami as newcomer: %d %v", resp.StatusCode, me)
	}
	_, acc := f.do("GET", "/v1/accounts/newcomer", "griff", nil)
	if a := acc["account"].(map[string]any); a["name"] != "New Comer" || grantsOf(a) != "services household viewer take=true" {
		t.Fatalf("newcomer's account: %v", a)
	}
	if c, _, _ := f.signIn("newcomer", joinPassword, false); c == "" {
		t.Fatal("newcomer signs in with the password chosen")
	}
	// Once only.
	if c, st, b := f.registerWith(code, "newcomer2", joinPassword); st != 410 || c != "" {
		t.Fatalf("used twice: %d %v", st, b)
	}
	if resp, b := f.session("GET", "/v1/register/"+code, "", false, nil); resp.StatusCode != 410 {
		t.Fatalf("reading a used invite: %d %v", resp.StatusCode, b)
	}
	// No one uses one by a change, not even an owner.
	if code, b := f.change("griff", map[string]any{"kind": "use-invite", "token_id": id, "account": "mallory", "password_hash": "x"}); code != 400 {
		t.Fatalf("use-invite through /v1/changes: %d %v", code, b)
	}
	if _, err := f.log.Commit("griff", "test", change.Op{Kind: change.UseInvite, TokenID: id, Account: "mallory", PasswordHash: "x"}); err == nil {
		t.Fatal("an owner commits use-invite")
	}

	// The log: made by griff, used by the manager for newcomer, with no secret.
	_, log := f.do("GET", "/v1/changes", "griff", nil)
	raw, _ := json.Marshal(log)
	if strings.Contains(string(raw), "pbkdf2") || strings.Contains(string(raw), "token_hash") || !strings.Contains(string(raw), "use-invite") {
		t.Fatal("the log shows a hash, or not the change")
	}
	_, list := f.do("GET", "/v1/invites", "griff", nil)
	ivs := list["invites"].([]any)
	if len(ivs) != 1 {
		t.Fatalf("the invites: %v", ivs)
	}
	if iv := ivs[0].(map[string]any); iv["state"] != "used" || iv["used_by"] != "newcomer" || iv["made"] == nil || iv["used_at"] == nil {
		t.Fatalf("the used invite: %v", iv)
	}
	if code, _ := f.do("GET", "/v1/invites", "claude", nil); code != 403 {
		t.Fatalf("an operator lists invites: %d", code)
	}

	// Revoked before it is used, it can't be; nor revoked twice.
	code2, _, made2 := f.invite("griff", viewer)
	id2 := made2["invite"].(map[string]any)["id"].(string)
	if code, _ := f.do("DELETE", "/v1/invites/"+id2, "claude", nil); code != 403 {
		t.Fatalf("an operator revokes: %d", code)
	}
	if code, b := f.do("DELETE", "/v1/invites/"+id2, "griff", nil); code != 200 {
		t.Fatalf("revoking: %d %v", code, b)
	}
	if code, _ := f.do("DELETE", "/v1/invites/"+id2, "griff", nil); code != 409 {
		t.Fatalf("revoked twice: %d", code)
	}
	if c, st, _ := f.registerWith(code2, "erin", joinPassword); st != 410 || c != "" {
		t.Fatalf("a revoked invite used: %d", st)
	}

	// One that makes an owner; and one whose maker is no longer an owner.
	code3, _, _ := f.invite("griff", map[string]any{"owner": true})
	if _, st, b := f.registerWith(code3, "frank", joinPassword); st != 200 || b["owner"] != true {
		t.Fatalf("an owner's invite: %d %v", st, b)
	}
	code4, _, _ := f.invite("griff", viewer)
	if _, err := f.log.Commit("frank", "test", change.Op{Kind: change.SetOwner, Account: "griff", Owner: new(bool)}); err != nil {
		t.Fatal(err)
	}
	if _, st, b := f.registerWith(code4, "gina", joinPassword); st != 410 {
		t.Fatalf("an invite from one no longer an owner: %d %v", st, b)
	}

	// Expired, by the commit check's clock.
	st5 := f.log.Snapshot()
	for _, iv := range st5.Access.Invites() {
		if iv.State(time.Now()) == "open" {
			if err := checkInvite(st5, change.Op{Kind: change.UseInvite, TokenID: iv.ID}, iv.Expires.Add(time.Second)); err == nil {
				t.Fatal("an expired invite passes the check")
			}
		}
	}

	// Guessing codes locks the address, as guessing passwords does.
	for i := 0; i < 6; i++ {
		f.session("GET", "/v1/register/"+id+".guess", "", false, nil)
	}
	if resp, _ := f.session("GET", "/v1/register/"+id+".guess", "", false, nil); resp.StatusCode != 429 {
		t.Fatalf("guessing: %d", resp.StatusCode)
	}
}
