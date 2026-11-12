package access

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Passwords (0176) are kept only as a PBKDF2-HMAC-SHA256 hash, with a salt of
// their own, written scheme$iterations$salt$hash so that the scheme can
// change and an old hash still checks. Unlike a token's 32 random bytes, a
// password is chosen by a person, so the hash is slow on purpose.
const (
	passwordScheme     = "pbkdf2-sha256"
	passwordIterations = 600000
	passwordSaltLen    = 16
	passwordKeyLen     = 32

	PasswordMin = 12
	PasswordMax = 128
)

var (
	ErrPasswordRules  = fmt.Errorf("a password is %d to %d characters, none of them control characters, and not the account's ID", PasswordMin, PasswordMax)
	ErrBadPasswordArg = errors.New("a password hash is pbkdf2-sha256$<iterations>$<salt>$<hash>")
)

// CheckPasswordRules says whether plain may be account's password: long
// enough, not too long, and not its ID. Length is what counts (NIST
// 800-63B); there are no rules of composition.
func CheckPasswordRules(account AccountID, plain string) error {
	n := utf8.RuneCountInString(plain)
	if n < PasswordMin || n > PasswordMax || !utf8.ValidString(plain) || strings.IndexFunc(plain, unicode.IsControl) >= 0 ||
		strings.EqualFold(plain, string(account)) {
		return ErrPasswordRules
	}
	return nil
}

// HashPassword hashes a password for keeping, with a new salt.
func HashPassword(plain string) (string, error) {
	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, plain, salt, passwordIterations, passwordKeyLen)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", passwordScheme, passwordIterations, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// parsedHash is a kept hash, read.
type parsedHash struct {
	iter int
	salt []byte
	key  []byte
}

func parseHash(encoded string) (parsedHash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordScheme {
		return parsedHash{}, ErrBadPasswordArg
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 100000 || iter > 10000000 {
		return parsedHash{}, ErrBadPasswordArg
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[2])
	if err != nil || len(salt) < 16 {
		return parsedHash{}, ErrBadPasswordArg
	}
	key, err := b64.DecodeString(parts[3])
	if err != nil || len(key) != passwordKeyLen {
		return parsedHash{}, ErrBadPasswordArg
	}
	return parsedHash{iter, salt, key}, nil
}

// CheckPasswordHash says whether encoded is a hash this package can check.
func CheckPasswordHash(encoded string) error {
	_, err := parseHash(encoded)
	return err
}

// VerifyPassword says whether plain is the password encoded was made from.
// It takes as long for a hash it can't read as for one it can, against a
// dummy, so a wrong account and a wrong password look alike.
func VerifyPassword(encoded, plain string) bool {
	h, err := parseHash(encoded)
	ok := err == nil
	if !ok {
		h = parsedHash{passwordIterations, make([]byte, passwordSaltLen), make([]byte, passwordKeyLen)}
	}
	key, kerr := pbkdf2.Key(sha256.New, plain, h.salt, h.iter, passwordKeyLen)
	return ok && kerr == nil && subtle.ConstantTimeCompare(key, h.key) == 1
}

// tempAlphabet is what a temporary password is made of: lowercase letters
// and digits a person reading it out can't mistake for others.
const tempAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// TemporaryPassword makes a password to hand someone once, who changes it at
// their first sign-in (0176): four groups of four, as abcd-efgh-jkmn-pqrs.
func TemporaryPassword() (string, error) {
	// Bytes at or past the last whole multiple of the alphabet's length are
	// drawn again, so each character is as likely as the next.
	limit := 256 - 256%len(tempAlphabet)
	var sb strings.Builder
	buf := make([]byte, 32)
	for n := 0; n < 16; {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, c := range buf {
			if int(c) >= limit || n == 16 {
				continue
			}
			if n > 0 && n%4 == 0 {
				sb.WriteByte('-')
			}
			sb.WriteByte(tempAlphabet[int(c)%len(tempAlphabet)])
			n++
		}
	}
	return sb.String(), nil
}
