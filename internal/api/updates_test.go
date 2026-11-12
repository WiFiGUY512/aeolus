package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v0.130.0", "v0.125.0-74aa57a", true},
		{"v0.125.0", "v0.125.0-74aa57a", false},
		{"v1.0.0", "v0.999.9", true},
		{"v0.9.0", "v0.10.0", false},
		{"v0.130.0", "dev", false},
		{"", "v0.1.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// What the manager asks the updater by itself (0183): a look once a day;
// with automatic on, the newest release in the window, unless it failed
// lately; nothing while the updater is at work.
func TestUpdateDue(t *testing.T) {
	now := time.Date(2026, 10, 21, 3, 30, 0, 0, time.UTC)
	fresh := updateStatus{Newest: "v0.130.0", Checked: now.Add(-time.Hour), State: "idle"}
	for _, c := range []struct {
		name      string
		st        updateStatus
		automatic bool
		window    string
		want      string
	}{
		{"never looked", updateStatus{}, false, "03:00-05:00", "check"},
		{"looked a day ago", updateStatus{Newest: "v0.130.0", Checked: now.Add(-25 * time.Hour)}, true, "03:00-05:00", "check"},
		{"newer, but not automatic", fresh, false, "03:00-05:00", ""},
		{"automatic, in the window", fresh, true, "03:00-05:00", "update v0.130.0"},
		{"automatic, out of the window", fresh, true, "01:00-02:00", ""},
		{"that release failed an hour ago", updateStatus{Newest: "v0.130.0", Checked: now.Add(-time.Hour), State: "failed", Tag: "v0.130.0", At: now.Add(-time.Hour)}, true, "03:00-05:00", ""},
		{"that release failed two days ago", updateStatus{Newest: "v0.130.0", Checked: now.Add(-time.Hour), State: "failed", Tag: "v0.130.0", At: now.Add(-48 * time.Hour)}, true, "03:00-05:00", "update v0.130.0"},
		{"the updater at work", updateStatus{Newest: "v0.130.0", State: "updating"}, true, "03:00-05:00", ""},
		// Stopped before it could say how it went, as when the host ran out
		// of memory: a failure, tried again by itself after a day.
		{"the updater at work for 5 minutes", updateStatus{Newest: "v0.130.0", Checked: now.Add(-time.Hour), State: "updating", Tag: "v0.130.0", At: now.Add(-5 * time.Minute), Written: now.Add(-5 * time.Minute)}, true, "03:00-05:00", ""},
		{"an update stopped an hour ago", updateStatus{Newest: "v0.130.0", Checked: now.Add(-time.Hour), State: "updating", Tag: "v0.130.0", At: now.Add(-time.Hour), Written: now.Add(-time.Hour)}, true, "03:00-05:00", ""},
		{"an update stopped two days ago", updateStatus{Newest: "v0.130.0", Checked: now.Add(-time.Hour), State: "updating", Tag: "v0.130.0", At: now.Add(-48 * time.Hour), Written: now.Add(-48 * time.Hour)}, true, "03:00-05:00", "update v0.130.0"},
		{"a check stopped, a day on", updateStatus{Newest: "v0.125.0", Checked: now.Add(-25 * time.Hour), State: "checking", Written: now.Add(-25 * time.Hour)}, false, "03:00-05:00", "check"},
		{"nothing newer", updateStatus{Newest: "v0.125.0", Checked: now.Add(-time.Hour)}, true, "03:00-05:00", ""},
	} {
		if got := updateDue(c.st, "v0.125.0-74aa57a", c.automatic, c.window, "", channelTesting, now); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// On the stable channel (0282), the newest release promoted to it, not
	// the newest made; none while none is.
	promoted := updateStatus{Newest: "v0.130.0", Stable: "v0.128.0", Checked: now.Add(-time.Hour), State: "idle"}
	for _, c := range []struct {
		name string
		st   updateStatus
		want string
	}{
		{"stable, in the window", promoted, "update v0.128.0"},
		{"none promoted yet", fresh, ""},
		{"the stable one runs", updateStatus{Newest: "v0.130.0", Stable: "v0.125.0", Checked: now.Add(-time.Hour), State: "idle"}, ""},
		{"the stable one failed an hour ago", updateStatus{Newest: "v0.130.0", Stable: "v0.128.0", Checked: now.Add(-time.Hour), State: "failed", Tag: "v0.128.0", At: now.Add(-time.Hour)}, ""},
	} {
		if got := updateDue(c.st, "v0.125.0-74aa57a", true, "03:00-05:00", "", channelStable, now); got != c.want {
			t.Errorf("stable: %s: %q, want %q", c.name, got, c.want)
		}
	}
}

// The routes (0183): an admin at the Org reads the updater's word and asks
// it to look or to update; no one else sees them.
func TestUpdateRoutes(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	write := func(name, s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Asked in this goroutine: the server is not changed under a request
	// in flight.
	f.api.WithVersion("v0.125.0-74aa57a").WithUpdates(dir)
	do := func(method, as string, body any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/v1/system/update", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+f.tokens[as])
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		f.api.Handler().ServeHTTP(rec, req)
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	request := func() string {
		b, _ := os.ReadFile(filepath.Join(dir, "request"))
		return string(b)
	}
	write("status", fmt.Sprintf("running=v0.125.0-74aa57a\nnewest=v0.130.0\nstable=v0.128.0\nchecked=%d\nstate=idle\ntag=\nat=\nerror=\n", time.Now().Unix()))
	write("update.log", "installed v0.124.0\n")

	if code, _ := do("GET", "claude", nil); code != 404 {
		t.Errorf("an operator reading: %d", code)
	}
	code, b := do("GET", "griff", nil)
	if code != 200 || b["installed"] != true || b["newest"] != "v0.130.0" || b["channel"] != "stable" || b["target"] != "v0.128.0" || b["newer"] != true || b["log"] != "installed v0.124.0\n" || b["automatic"] != false {
		t.Fatalf("an owner reading: %d %v", code, b)
	}
	if code, _ := do("POST", "claude", map[string]any{"action": "check"}); code != 404 {
		t.Errorf("an operator asking: %d", code)
	}
	if code, b := do("POST", "griff", map[string]any{"action": "check"}); code != 202 || request() != "check\n" {
		t.Errorf("check: %d %v, request %q", code, b, request())
	}
	if code, b := do("POST", "griff", map[string]any{"action": "update"}); code != 409 {
		t.Errorf("update while a request waits: %d %v", code, b)
	}
	os.Remove(filepath.Join(dir, "request"))
	if code, b := do("POST", "griff", map[string]any{"action": "update"}); code != 202 || request() != "update v0.128.0\n" {
		t.Errorf("update: %d %v, request %q", code, b, request())
	}
	os.Remove(filepath.Join(dir, "request"))
	if code, _ := do("POST", "griff", map[string]any{"action": "reboot"}); code != 400 {
		t.Errorf("another action: %d", code)
	}
	// Nothing newer: no update to ask for.
	write("status", "running=v0.130.0\nnewest=v0.125.0\nstate=idle\n")
	if code, _ := do("POST", "griff", map[string]any{"action": "update"}); code != 409 {
		t.Errorf("update with nothing newer: %d", code)
	}

	// By itself: a look a day old is asked for again.
	write("status", fmt.Sprintf("running=v0.125.0\nnewest=v0.125.0\nchecked=%d\nstate=idle\n", time.Now().Add(-25*time.Hour).Unix()))
	if got := f.api.updateOnce(time.Now()); got != "check" || request() != "check\n" {
		t.Errorf("by itself: %q, request %q", got, request())
	}
	if got := f.api.updateOnce(time.Now()); got != "" {
		t.Errorf("with a request waiting, it asked %q", got)
	}

	// A manager with no updater says so.
	f.api.WithUpdates(filepath.Join(dir, "none"))
	if code, b := do("GET", "griff", nil); code != 200 || b["installed"] != false {
		t.Errorf("no updater: %d %v", code, b)
	}
	if code, _ := do("POST", "griff", map[string]any{"action": "check"}); code != 409 {
		t.Errorf("asking with no updater: %d", code)
	}
}

// What each newer release says of itself (0217): the blocks the updater
// wrote, those newer than the one running alone, the newest first, trimmed.
func TestReadUpdateNotes(t *testing.T) {
	dir := t.TempDir()
	notes := "@@ v0.168.0\nThe advisor's schedule\n\n- one\n- two\n\n@@ v0.167.0\nRadios off the air\n@@ v0.166.0\nRunning now\n@@ not-a-tag\nx\n"
	if err := os.WriteFile(filepath.Join(dir, "notes"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readUpdateNotes(dir, "v0.166.0-8d497f4")
	if len(got) != 2 || got[0].Version != "v0.168.0" || got[0].Notes != "The advisor's schedule\n\n- one\n- two" || got[1].Version != "v0.167.0" || got[1].Notes != "Radios off the air" {
		t.Fatalf("notes = %+v", got)
	}
	if got := readUpdateNotes(t.TempDir(), "v0.166.0"); got != nil {
		t.Fatalf("no notes file: %+v", got)
	}
}
