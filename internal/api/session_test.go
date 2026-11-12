package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/sessions"
)

// raw sends a request with a session cookie (or none) and SessionHeader if
// asked, and returns the response and its decoded body.
func (f *fixture) session(method, path, cookie string, header bool, body any) (*http.Response, map[string]any) {
	f.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		must(f.t, err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.url+path, rd)
	must(f.t, err)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	}
	if header {
		req.Header.Set(SessionHeader, "1")
	}
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// setPassword gives an account a password, as an owner would.
func (f *fixture) setPassword(account, plain string, temporary bool) {
	f.t.Helper()
	h, err := access.HashPassword(plain)
	must(f.t, err)
	if _, err := f.log.Commit("griff", "test", change.Op{Kind: change.SetPassword, Account: access.AccountID(account), PasswordHash: h, MustChange: temporary}); err != nil {
		f.t.Fatal(err)
	}
}

// signIn signs in and returns the session's cookie.
func (f *fixture) signIn(account, password string, keep bool) (string, *http.Response, map[string]any) {
	f.t.Helper()
	resp, body := f.session("POST", "/v1/session", "", false, map[string]any{"account": account, "password": password, "keep": keep})
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookie && c.Value != "" {
			return c.Value, resp, body
		}
	}
	return "", resp, body
}

// Signing in with a password (0176), part 2.
func TestSignIn(t *testing.T) {
	f := newFixture(t)
	f.setPassword("office", "the office's passphrase", false)

	// Wrong password, unknown account, and an account with no password all
	// get the same answer.
	for _, try := range [][2]string{{"office", "not the passphrase"}, {"mallory", "the office's passphrase"}, {"claude", "anything at all here"}} {
		if c, resp, b := f.signIn(try[0], try[1], false); c != "" || resp.StatusCode != 401 || b["error"] != "wrong account or password" {
			t.Fatalf("%s: %d %v", try[0], resp.StatusCode, b)
		}
	}

	// Signed in: the cookie is HttpOnly, Secure, SameSite=Strict, and lasts
	// the browser's session unless kept.
	cookie, resp, b := f.signIn("Office", "the office's passphrase", false)
	if cookie == "" || resp.StatusCode != 200 || b["account"] != "office" || b["must_change"] != false {
		t.Fatalf("sign in: %d %v", resp.StatusCode, b)
	}
	set := resp.Header.Get("Set-Cookie")
	for _, want := range []string{"HttpOnly", "Secure", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(set, want) {
			t.Errorf("cookie lacks %s: %s", want, set)
		}
	}
	if strings.Contains(set, "Max-Age") {
		t.Errorf("an unkept session's cookie outlives the browser: %s", set)
	}
	if _, resp, _ := f.signIn("office", "the office's passphrase", true); !strings.Contains(resp.Header.Get("Set-Cookie"), "Max-Age=2592000") {
		t.Errorf("a kept session's cookie: %s", resp.Header.Get("Set-Cookie"))
	}
	if resp, me := f.session("GET", "/v1/whoami", cookie, false, nil); resp.StatusCode != 200 || me["account"] != "office" {
		t.Fatalf("whoami with the cookie: %d %v", resp.StatusCode, me)
	}

	// A change made with the cookie needs the header.
	op := map[string]any{"op": map[string]any{"kind": "set", "tree": "services", "node": "household", "path": "network.sweet.ssid", "value": "Sweet"}, "reason": "test"}
	if resp, b := f.session("POST", "/v1/changes", cookie, false, op); resp.StatusCode != 403 || !strings.Contains(b["error"].(string), SessionHeader) {
		t.Fatalf("a change without the header: %d %v", resp.StatusCode, b)
	}
	if resp, b := f.session("POST", "/v1/changes", cookie, true, op); resp.StatusCode != 200 {
		t.Fatalf("a change with it: %d %v", resp.StatusCode, b)
	}

	// Reset to a temporary password, the session ends; signed in with the
	// temporary one, only the password can be changed.
	f.setPassword("office", "temp-orar-ypas-sword", true)
	if resp, _ := f.session("GET", "/v1/whoami", cookie, false, nil); resp.StatusCode != 401 {
		t.Fatalf("after a reset, the old session: %d", resp.StatusCode)
	}
	cookie, _, b = f.signIn("office", "temp-orar-ypas-sword", false)
	if b["must_change"] != true {
		t.Fatalf("signed in with a temporary password: %v", b)
	}
	if resp, b := f.session("GET", "/v1/trees/locations", cookie, false, nil); resp.StatusCode != 403 || b["must_change"] != true {
		t.Fatalf("before changing it: %d %v", resp.StatusCode, b)
	}
	if resp, _ := f.session("GET", "/v1/whoami", cookie, false, nil); resp.StatusCode != 200 {
		t.Fatalf("whoami before changing it: %d", resp.StatusCode)
	}
	other, _, _ := f.signIn("office", "temp-orar-ypas-sword", false)
	change := func(cur, nw string) (*http.Response, map[string]any) {
		return f.session("POST", "/v1/session/password", cookie, true, map[string]any{"current": cur, "new": nw})
	}
	for _, c := range []struct {
		cur, nw string
		code    int
	}{
		{"temp-orar-ypas-sword", "temp-orar-ypas-sword", 400}, // not the temporary one again
		{"temp-orar-ypas-sword", "short", 400},
		{"not it", "a new passphrase of mine", 403},
	} {
		if resp, b := change(c.cur, c.nw); resp.StatusCode != c.code {
			t.Fatalf("change %q to %q: %d %v", c.cur, c.nw, resp.StatusCode, b)
		}
	}
	// Ended: the other one, and the kept one from before the reset, which
	// nothing had looked at since.
	if resp, b := change("temp-orar-ypas-sword", "a new passphrase of mine"); resp.StatusCode != 200 || b["other_sessions_ended"] != 2.0 {
		t.Fatalf("changed: %d %v", resp.StatusCode, b)
	}
	// This session goes on, and may do all again; the other one ended.
	if resp, _ := f.session("GET", "/v1/trees/locations", cookie, false, nil); resp.StatusCode != 200 {
		t.Fatalf("after changing it: %d", resp.StatusCode)
	}
	if resp, _ := f.session("GET", "/v1/whoami", other, false, nil); resp.StatusCode != 401 {
		t.Fatalf("the other session: %d", resp.StatusCode)
	}
	if acc, _ := f.log.Snapshot().Access.Account("office"); acc.MustChange || !access.VerifyPassword(acc.Password, "a new passphrase of mine") {
		t.Fatal("the change isn't in force")
	}

	// Disabled, its session ends at once.
	if code, b := f.change("griff", map[string]any{"kind": "disable-account", "account": "office"}); code != 200 {
		t.Fatalf("disable: %d %v", code, b)
	}
	if resp, _ := f.session("GET", "/v1/whoami", cookie, false, nil); resp.StatusCode != 401 {
		t.Fatalf("a disabled account's session: %d", resp.StatusCode)
	}
	if c, resp, _ := f.signIn("office", "a new passphrase of mine", false); c != "" || resp.StatusCode != 401 {
		t.Fatalf("a disabled account signs in: %d", resp.StatusCode)
	}

	// Signing out ends it, and clears the cookie.
	f.setPassword("tenant", "the tenant's passphrase", false)
	cookie, _, _ = f.signIn("tenant", "the tenant's passphrase", false)
	resp, _ = f.session("DELETE", "/v1/session", cookie, false, nil)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("sign out: %d %s", resp.StatusCode, resp.Header.Get("Set-Cookie"))
	}
	if resp, _ := f.session("GET", "/v1/whoami", cookie, false, nil); resp.StatusCode != 401 {
		t.Fatalf("after signing out: %d", resp.StatusCode)
	}

	// Five failures lock the account: the right password too, for a while.
	for i := 0; i < sessions.FailuresBeforeLock; i++ {
		f.signIn("tenant", "wrong", false)
	}
	if c, resp, _ := f.signIn("tenant", "the tenant's passphrase", false); c != "" || resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("locked: %d", resp.StatusCode)
	}

	// Every try is recorded, never with a password.
	tries, err := f.api.sessions.SignIns("", 0)
	must(t, err)
	whys := map[string]bool{}
	for _, si := range tries {
		whys[si.Why] = true
		if strings.Contains(si.Account, "passphrase") {
			t.Fatalf("a password in the record: %+v", si)
		}
	}
	for _, want := range []string{sessions.OK, sessions.Wrong, sessions.NoPassword, sessions.MustChanged, sessions.Inactive, sessions.Locked} {
		if !whys[want] {
			t.Errorf("the record has no %q: %v", want, whys)
		}
	}
}

// A token that fails, again and again, locks the address it comes from.
func TestTokenGuessingIsSlowed(t *testing.T) {
	f := newFixture(t)
	f.tokens["forger"] = "aeolus1.0000000000000000.nope"
	for i := 0; i < sessions.FailuresBeforeLock; i++ {
		if code, _ := f.do("GET", "/v1/whoami", "forger", nil); code != 401 {
			t.Fatalf("try %d: %d", i, code)
		}
	}
	if code, b := f.do("GET", "/v1/whoami", "claude", nil); code != 429 {
		t.Fatalf("after five, from the same address: %d %v", code, b)
	}
}

// Changing one's own password with a token, as one who has none yet: no
// current password is asked for.
func TestFirstPasswordWithAToken(t *testing.T) {
	f := newFixture(t)
	code, b := f.do("POST", "/v1/session/password", "griff", map[string]any{"new": "griff's own passphrase"})
	if code != 200 {
		t.Fatalf("first password: %d %v", code, b)
	}
	if c, resp, b := f.signIn("griff", "griff's own passphrase", false); c == "" || b["owner"] != true {
		t.Fatalf("sign in with it: %d %v", resp.StatusCode, b)
	}
	// Now there is one, the current one is asked for.
	if code, _ := f.do("POST", "/v1/session/password", "griff", map[string]any{"new": "another passphrase"}); code != 403 {
		t.Fatalf("without the current one: %d", code)
	}
}
