package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
)

// Accounts run from the manager (0176), part 1, through the API: whoami
// says who is an owner; a password is never sent as a change and never shown
// in the log; an owner disables, enables and removes accounts, and no one
// else does; the last owner stays.
func TestAccountsThroughTheAPI(t *testing.T) {
	f := newFixture(t)
	if _, me := f.do("GET", "/v1/whoami", "griff", nil); me["owner"] != true || me["must_change"] != false {
		t.Fatalf("griff's whoami: %v", me)
	}
	if _, me := f.do("GET", "/v1/whoami", "claude", nil); me["owner"] != false {
		t.Fatalf("claude's whoami: %v", me)
	}

	// A password is never a change sent in, hashed or not.
	h, err := access.HashPassword("a long enough passphrase")
	must(t, err)
	if code, b := f.change("griff", map[string]any{"kind": "set-password", "account": "office", "password_hash": h}); code != 400 {
		t.Fatalf("set-password through /v1/changes: %d %v", code, b)
	}
	// One set on the host is in the log, without its hash.
	if _, err := f.log.Commit("office", "test", change.Op{Kind: change.SetPassword, Account: "office", PasswordHash: h}); err != nil {
		t.Fatal(err)
	}
	_, log := f.do("GET", "/v1/changes", "griff", nil)
	raw, _ := json.Marshal(log)
	if strings.Contains(string(raw), "pbkdf2") || !strings.Contains(string(raw), "set-password") {
		t.Fatal("the log shows a password's hash, or not the change")
	}

	// Only an owner disables an account; its token then stops, until it is
	// enabled again.
	if code, b := f.change("claude", map[string]any{"kind": "disable-account", "account": "tenant"}); code != 403 {
		t.Fatalf("an operator disables an account: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "disable-account", "account": "tenant"}); code != 200 {
		t.Fatalf("the owner disables one: %d %v", code, b)
	}
	if code, _ := f.do("GET", "/v1/whoami", "tenant", nil); code != 401 {
		t.Fatalf("a disabled account's token: %d", code)
	}
	if code, _ := f.change("griff", map[string]any{"kind": "disable-account", "account": "tenant"}); code != 409 {
		t.Fatalf("disabled twice: %d", code)
	}
	if code, _ := f.change("griff", map[string]any{"kind": "enable-account", "account": "tenant"}); code != 200 {
		t.Fatalf("enabled: %d", code)
	}
	if code, _ := f.do("GET", "/v1/whoami", "tenant", nil); code != 200 {
		t.Fatalf("enabled again, its token: %d", code)
	}

	// The last owner stays one.
	if code, b := f.change("griff", map[string]any{"kind": "remove-account", "account": "griff"}); code != 409 {
		t.Fatalf("the last owner removed: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set-owner", "account": "griff", "owner": false}); code != 409 {
		t.Fatalf("the last owner unmade: %d %v", code, b)
	}
	if code, b := f.change("griff", map[string]any{"kind": "set-owner", "account": "griff"}); code != 400 {
		t.Fatalf("set-owner saying neither: %d %v", code, b)
	}

	// Removed: its token stops, and its ID is not taken again.
	if code, b := f.change("griff", map[string]any{"kind": "remove-account", "account": "tenant"}); code != 200 {
		t.Fatalf("removed: %d %v", code, b)
	}
	if code, _ := f.do("GET", "/v1/whoami", "tenant", nil); code != 401 {
		t.Fatalf("a removed account's token: %d", code)
	}
	if code, _ := f.change("griff", map[string]any{"kind": "add-account", "account": "tenant", "name": "Again"}); code != 409 {
		t.Fatalf("a removed ID taken again: %d", code)
	}
}
