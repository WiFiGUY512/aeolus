package access

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestPasswords(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$600000$") || CheckPasswordHash(h) != nil {
		t.Fatalf("hash %q", h)
	}
	if !VerifyPassword(h, "correct horse battery") || VerifyPassword(h, "correct horse batterY") {
		t.Fatal("verify")
	}
	// The same password twice gets two hashes: each its own salt.
	if h2, _ := HashPassword("correct horse battery"); h2 == h {
		t.Fatal("no salt")
	}
	// A hash that can't be read checks nothing, and isn't taken.
	for _, bad := range []string{"", "plain", "pbkdf2-sha256$600000$c2FsdA$aGFzaA", "md5$1$a$b", "pbkdf2-sha256$10$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if VerifyPassword(bad, "anything") || CheckPasswordHash(bad) == nil {
			t.Errorf("%q taken for a hash", bad)
		}
	}
}

func TestPasswordRules(t *testing.T) {
	for p, ok := range map[string]bool{
		"short":                  false,
		"twelve chars":           true,
		"griff-the-1":            false, // 11
		"GRIFF":                  false,
		"griffgriffgr":           true,
		"tab\there and long":     false,
		strings.Repeat("x", 128): true,
		strings.Repeat("x", 129): false,
		"ünïcödé pässwörd":       true,
	} {
		if err := CheckPasswordRules("griff", p); (err == nil) != ok {
			t.Errorf("%q: %v, want ok=%v", p, err, ok)
		}
	}
	// Its own ID, in any case, is not a password, however long.
	if err := CheckPasswordRules("buildingmanager", "BuildingManager"); !errors.Is(err, ErrPasswordRules) {
		t.Errorf("its own ID: %v", err)
	}
}

func TestTemporaryPassword(t *testing.T) {
	re := regexp.MustCompile(`^[a-hjkmnp-z2-9]{4}(-[a-hjkmnp-z2-9]{4}){3}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := TemporaryPassword()
		if err != nil || !re.MatchString(p) || CheckPasswordRules("griff", p) != nil {
			t.Fatalf("%q %v", p, err)
		}
		if seen[p] {
			t.Fatalf("%q twice", p)
		}
		seen[p] = true
	}
}
