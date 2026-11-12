package sessions

import (
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T, now *time.Time) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sessions.db"), func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessions(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	secret, ses, err := s.Create("griff", "tag1", false, "192.168.20.217", "Firefox")
	if err != nil || len(secret) < 40 || ses.ID == secret {
		t.Fatalf("create: %q %+v %v", secret, ses, err)
	}
	if got, err := s.Lookup(secret); err != nil || got.Account != "griff" || got.PwTag != "tag1" {
		t.Fatalf("lookup: %+v %v", got, err)
	}
	if _, err := s.Lookup(secret + "x"); err != ErrNone {
		t.Fatalf("a wrong secret: %v", err)
	}
	// Used within its idle time, it lives on; idle past it, it ends.
	now = now.Add(7 * time.Hour)
	if _, err := s.Lookup(secret); err != nil {
		t.Fatalf("after 7 h in use: %v", err)
	}
	now = now.Add(IdleTimeout)
	if _, err := s.Lookup(secret); err != ErrNone {
		t.Fatalf("idle past its time: %v", err)
	}
	// A kept one lasts 30 days idle.
	kept, _, _ := s.Create("griff", "tag1", true, "", "")
	now = now.Add(29 * 24 * time.Hour)
	if _, err := s.Lookup(kept); err != nil {
		t.Fatalf("kept, 29 days idle: %v", err)
	}
	// Ending all of an account's but one.
	a, aSes, _ := s.Create("office", "t", false, "", "")
	b, _, _ := s.Create("office", "t", false, "", "")
	if n, err := s.DeleteFor("office", aSes.ID); err != nil || n != 1 {
		t.Fatalf("delete for: %d %v", n, err)
	}
	if _, err := s.Lookup(a); err != nil {
		t.Fatal("the one kept was ended")
	}
	if _, err := s.Lookup(b); err != ErrNone {
		t.Fatal("the other lives on")
	}
	if err := s.Retag(aSes.ID, "t2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Lookup(a); got.PwTag != "t2" {
		t.Fatalf("retag: %q", got.PwTag)
	}
}

func TestSignInRecord(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	s := open(t, &now)
	for i, si := range []SignIn{
		{Account: "griff", Source: "a", OK: false, Why: Wrong},
		{Account: "griff", Source: "a", OK: true, Why: OK},
		{Account: "office", Source: "b", OK: false, Why: Locked},
	} {
		now = now.Add(time.Minute)
		if err := s.Record(si); err != nil {
			t.Fatal(i, err)
		}
	}
	all, _ := s.SignIns("", 0)
	if len(all) != 3 || all[0].Account != "office" || all[0].Why != Locked {
		t.Fatalf("all, newest first: %+v", all)
	}
	if mine, _ := s.SignIns("griff", 0); len(mine) != 2 || !mine[0].OK {
		t.Fatalf("griff's: %+v", mine)
	}
	// Past 90 days, the record forgets.
	now = now.Add(SignInsKept + 2*time.Hour)
	s.Record(SignIn{Account: "griff", OK: true, Why: OK})
	if all, _ := s.SignIns("", 0); len(all) != 1 {
		t.Fatalf("after 90 days: %+v", all)
	}
}

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	l := NewLimiter(func() time.Time { return now })
	k := "account:griff"
	for i := 0; i < FailuresBeforeLock-1; i++ {
		l.Fail(k)
	}
	if l.Locked(k) != 0 {
		t.Fatal("locked before the fifth failure")
	}
	l.Fail(k)
	if d := l.Locked(k, "address:x"); d != time.Minute {
		t.Fatalf("first lock: %v", d)
	}
	// Each lock doubles, to at most 15 minutes.
	want := []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	for _, w := range want {
		now = now.Add(time.Hour - time.Minute) // past the lock, within the hour it's remembered
		for i := 0; i < FailuresBeforeLock; i++ {
			l.Fail(k)
		}
		if d := l.Locked(k); d != w {
			t.Fatalf("lock: %v, want %v", d, w)
		}
	}
	// A success clears it.
	l.Succeed(k)
	if l.Locked(k) != 0 {
		t.Fatal("still locked after a success")
	}
}

// A token's use is written at most once a minute; a store made by the
// first version gains the table for it.
func TestTokenUses(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "sessions.db")
	s, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	// As the first version left it.
	if _, err := s.db.Exec(`DROP TABLE tokens_used; PRAGMA user_version = 1;`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s, err = Open(path, func() time.Time { return now }); err != nil {
		t.Fatalf("opening a first-version store: %v", err)
	}
	defer s.Close()
	if err := s.TokenUsed("tok1", "192.168.20.217"); err != nil {
		t.Fatal(err)
	}
	first := now
	now = now.Add(30 * time.Second)
	s.TokenUsed("tok1", "192.168.20.5")
	if u, _ := s.TokenUses([]string{"tok1", "tok2"}); len(u) != 1 || !u["tok1"].At.Equal(first) || u["tok1"].Source != "192.168.20.217" {
		t.Errorf("within the minute: %+v", u)
	}
	now = now.Add(time.Minute)
	s.TokenUsed("tok1", "192.168.20.5")
	if u, _ := s.TokenUses([]string{"tok1"}); !u["tok1"].At.Equal(now) || u["tok1"].Source != "192.168.20.5" {
		t.Errorf("after it: %+v", u)
	}
}
