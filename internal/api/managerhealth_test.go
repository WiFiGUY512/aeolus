package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The manager's own health (0219): an admin's at the Org, with its
// databases' sizes and the answers it was slow to give.
func TestManagerHealth(t *testing.T) {
	f := newFixture(t)
	db := filepath.Join(t.TempDir(), "aeolus.db")
	if err := os.WriteFile(db, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db+"-wal", make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	f.api.WithFiles(db)
	keepSlow(slowAnswer{At: time.Now(), Method: "GET", Path: "/v1/aps", Status: 200, MS: 1800})
	if code, _ := f.do("GET", "/v1/system/health", "claude", nil); code != 404 {
		t.Fatalf("not an admin at the Org: %d", code)
	}
	code, body := f.do("GET", "/v1/system/health", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	dbs := body["databases"].([]any)
	if len(dbs) != 1 || dbs[0].(map[string]any)["name"] != "aeolus.db" || dbs[0].(map[string]any)["bytes"] != 4096.0 || dbs[0].(map[string]any)["wal"] != 100.0 {
		t.Fatalf("databases = %v", dbs)
	}
	slow := body["slow"].(map[string]any)
	if slow["last_hour"].(float64) < 1 || slow["recent"].([]any)[0].(map[string]any)["path"] != "/v1/aps" {
		t.Fatalf("slow = %v", slow)
	}
	if body["host"].(map[string]any)["cpus"].(float64) < 1 || body["process"].(map[string]any)["goroutines"].(float64) < 1 {
		t.Fatalf("host and process = %v %v", body["host"], body["process"])
	}
}

// An answer asked to wait, as an AP's key agent waits for its keys, is not
// slow for having waited: none of it is kept.
func TestSlowNotWaited(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()
	_, _, got := f.apDo("GET", "/v1/ap/keys", token, nil, nil)
	version, _ := got["version"].(string)
	start := time.Now()
	if code, _, _ := f.apDo("GET", "/v1/ap/keys?wait=1", token, nil, map[string]string{"If-None-Match": `"` + version + `"`}); code != 304 || time.Since(start) < time.Second {
		t.Fatalf("held: %d after %v", code, time.Since(start))
	}
	_, recent := slowSince(start, slowKept)
	for _, a := range recent {
		if a.Path == "/v1/ap/keys" {
			t.Fatalf("kept as slow: %+v", a)
		}
	}
}

// At most slowKept slow answers are kept, the newest.
func TestSlowKept(t *testing.T) {
	for i := 0; i < slowKept+10; i++ {
		keepSlow(slowAnswer{At: time.Now(), Path: "/x", MS: int64(1000 + i)})
	}
	_, recent := slowSince(time.Now().Add(-time.Hour), 3)
	if len(recent) != 3 || recent[0].MS != int64(1000+slowKept+9) {
		t.Fatalf("recent = %+v", recent)
	}
	slowAnswers.Lock()
	n := len(slowAnswers.list)
	slowAnswers.Unlock()
	if n != slowKept {
		t.Fatalf("kept %d", n)
	}
}
