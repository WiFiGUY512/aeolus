// Package sessions keeps the manager's sign-in sessions and its record of
// sign-ins (0176). They are events rather than intent, so they live beside
// the change log, not in it. A session is 32 random bytes a browser holds in
// a cookie; the store keeps only their SHA-256, so the file never holds what
// would open one.
package sessions

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const (
	// IdleTimeout ends a session after this long without a request;
	// KeepTimeout, one kept with "Keep me signed in".
	IdleTimeout = 8 * time.Hour
	KeepTimeout = 30 * 24 * time.Hour
	// SignInsKept is how long the sign-in record goes back.
	SignInsKept = 90 * 24 * time.Hour
	// A session's last use is written at most this often, not on every
	// request.
	touchEvery = time.Minute
	pruneEvery = time.Hour

	schemaVersion = 2
)

const schema = `
CREATE TABLE sessions (
	id        TEXT PRIMARY KEY,    -- hex SHA-256 of the cookie's secret
	account   TEXT NOT NULL,
	keep      INTEGER NOT NULL,
	created   INTEGER NOT NULL,    -- Unix seconds
	last_used INTEGER NOT NULL,
	source    TEXT NOT NULL,       -- the address it signed in from
	agent     TEXT NOT NULL,       -- the browser's User-Agent, cut short
	pwtag     TEXT NOT NULL        -- which password it signed in with
);
CREATE INDEX sessions_account ON sessions (account);
CREATE TABLE signins (
	at      INTEGER NOT NULL,
	account TEXT NOT NULL,
	source  TEXT NOT NULL,
	ok      INTEGER NOT NULL,
	why     TEXT NOT NULL
);
CREATE INDEX signins_at ON signins (at);
` + schema2

// schema2 adds when each API token was last used, and from where (0176).
const schema2 = `
CREATE TABLE IF NOT EXISTS tokens_used (
	id        TEXT PRIMARY KEY,    -- the token's ID, never its secret
	last_used INTEGER NOT NULL,
	source    TEXT NOT NULL
);
`

// Session is a signed-in browser.
type Session struct {
	ID       string // hex SHA-256 of its secret
	Account  string
	Keep     bool
	Created  time.Time
	LastUsed time.Time
	Source   string
	Agent    string
	PwTag    string
}

// Expires is when the session ends if nothing uses it.
func (s Session) Expires() time.Time {
	if s.Keep {
		return s.LastUsed.Add(KeepTimeout)
	}
	return s.LastUsed.Add(IdleTimeout)
}

// SignIn is one try at signing in: who, from where, and whether it worked
// and why not.
type SignIn struct {
	At      time.Time `json:"at"`
	Account string    `json:"account"`
	Source  string    `json:"source"`
	OK      bool      `json:"ok"`
	Why     string    `json:"why"`
}

// Why a sign-in went as it did.
const (
	OK          = "ok"
	Wrong       = "wrong account or password"
	NoPassword  = "the account has no password"
	Inactive    = "the account is disabled or removed"
	Locked      = "too many failed tries"
	MustChanged = "ok, the password must be changed"
	Registered  = "registered with an invite" // 0336
)

// Store keeps the sessions and the record.
type Store struct {
	db  *sql.DB
	now func() time.Time

	mu     sync.Mutex
	pruned time.Time
	// used is when each token's use was last written: at most once a
	// minute, not on every request.
	used map[string]time.Time
}

// Open opens the store at path, making it if need be.
func Open(path string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	p := (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath()
	db, err := sql.Open("sqlite3", "file:"+p+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(wal)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		db.Close()
		return nil, err
	}
	switch {
	case v > schemaVersion:
		db.Close()
		return nil, fmt.Errorf("sessions schema version %d is newer than this manager (%d)", v, schemaVersion)
	case v == 0:
		if _, err := db.Exec(schema + fmt.Sprintf("PRAGMA user_version = %d;", schemaVersion)); err != nil {
			db.Close()
			return nil, err
		}
	case v == 1:
		if _, err := db.Exec(schema2 + fmt.Sprintf("PRAGMA user_version = %d;", schemaVersion)); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db, now: now, used: map[string]time.Time{}}, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// idOf is a secret's ID: the hex of its SHA-256.
func idOf(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// PwTag names a password hash without being one, so a session can tell
// whether the password it signed in with is still the account's.
func PwTag(hash string) string {
	sum := sha256.Sum256([]byte("aeolus-session-pwtag\x00" + hash))
	return hex.EncodeToString(sum[:8])
}

// Create starts a session and returns the secret for its cookie.
func (s *Store) Create(account, pwtag string, keep bool, source, agent string) (string, Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", Session{}, err
	}
	secret := base64.RawURLEncoding.EncodeToString(b)
	if len(agent) > 200 {
		agent = agent[:200]
	}
	now := s.now()
	ses := Session{ID: idOf(secret), Account: account, Keep: keep, Created: now, LastUsed: now, Source: source, Agent: agent, PwTag: pwtag}
	_, err := s.db.Exec(`INSERT INTO sessions (id, account, keep, created, last_used, source, agent, pwtag) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ses.ID, account, keep, now.Unix(), now.Unix(), source, agent, pwtag)
	if err != nil {
		return "", Session{}, err
	}
	s.prune()
	return secret, ses, nil
}

// ErrNone is a secret that names no live session.
var ErrNone = errors.New("no such session, or it ended")

// Lookup finds the session a cookie's secret names, if it hasn't ended, and
// notes that it was used. An ended one is taken away.
func (s *Store) Lookup(secret string) (Session, error) {
	if secret == "" {
		return Session{}, ErrNone
	}
	var ses Session
	var keep int
	var created, used int64
	err := s.db.QueryRow(`SELECT id, account, keep, created, last_used, source, agent, pwtag FROM sessions WHERE id = ?`, idOf(secret)).
		Scan(&ses.ID, &ses.Account, &keep, &created, &used, &ses.Source, &ses.Agent, &ses.PwTag)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNone
	}
	if err != nil {
		return Session{}, err
	}
	ses.Keep, ses.Created, ses.LastUsed = keep != 0, time.Unix(created, 0), time.Unix(used, 0)
	now := s.now()
	if !now.Before(ses.Expires()) {
		s.Delete(ses.ID)
		return Session{}, ErrNone
	}
	if now.Sub(ses.LastUsed) >= touchEvery {
		if _, err := s.db.Exec(`UPDATE sessions SET last_used = ? WHERE id = ?`, now.Unix(), ses.ID); err != nil {
			return Session{}, err
		}
		ses.LastUsed = now
	}
	return ses, nil
}

// Retag notes a session's new password, as when its own account changed it.
func (s *Store) Retag(id, pwtag string) error {
	_, err := s.db.Exec(`UPDATE sessions SET pwtag = ? WHERE id = ?`, pwtag, id)
	return err
}

// Delete ends a session.
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteFor ends an account's sessions but one (except, which may be empty),
// and says how many it ended.
func (s *Store) DeleteFor(account, except string) (int, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE account = ? AND id != ?`, account, except)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// Of lists an account's live sessions, newest first.
func (s *Store) Of(account string) ([]Session, error) {
	rows, err := s.db.Query(`SELECT id, account, keep, created, last_used, source, agent, pwtag FROM sessions WHERE account = ? ORDER BY last_used DESC`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	now := s.now()
	for rows.Next() {
		var ses Session
		var keep int
		var created, used int64
		if err := rows.Scan(&ses.ID, &ses.Account, &keep, &created, &used, &ses.Source, &ses.Agent, &ses.PwTag); err != nil {
			return nil, err
		}
		ses.Keep, ses.Created, ses.LastUsed = keep != 0, time.Unix(created, 0), time.Unix(used, 0)
		if now.Before(ses.Expires()) {
			out = append(out, ses)
		}
	}
	return out, rows.Err()
}

// TokenUse is when an API token was last used, and from where.
type TokenUse struct {
	At     time.Time `json:"at"`
	Source string    `json:"source"`
}

// TokenUsed notes that a token was used, from source.
func (s *Store) TokenUsed(id, source string) error {
	now := s.now()
	s.mu.Lock()
	if now.Sub(s.used[id]) < touchEvery {
		s.mu.Unlock()
		return nil
	}
	s.used[id] = now
	s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO tokens_used (id, last_used, source) VALUES (?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET last_used = excluded.last_used, source = excluded.source`, id, now.Unix(), source)
	return err
}

// TokenUses is when each of the tokens with those IDs was last used, of
// those that ever were.
func (s *Store) TokenUses(ids []string) (map[string]TokenUse, error) {
	out := map[string]TokenUse{}
	for _, id := range ids {
		var at int64
		var u TokenUse
		err := s.db.QueryRow(`SELECT last_used, source FROM tokens_used WHERE id = ?`, id).Scan(&at, &u.Source)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		u.At = time.Unix(at, 0)
		out[id] = u
	}
	return out, nil
}

// LastSignIns is when each account last signed in, of those that did in
// the record's time.
func (s *Store) LastSignIns() (map[string]time.Time, error) {
	rows, err := s.db.Query(`SELECT account, MAX(at) FROM signins WHERE ok = 1 GROUP BY account`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var acc string
		var at int64
		if err := rows.Scan(&acc, &at); err != nil {
			return nil, err
		}
		out[acc] = time.Unix(at, 0)
	}
	return out, rows.Err()
}

// Record notes a try at signing in.
func (s *Store) Record(si SignIn) error {
	if si.At.IsZero() {
		si.At = s.now()
	}
	_, err := s.db.Exec(`INSERT INTO signins (at, account, source, ok, why) VALUES (?, ?, ?, ?, ?)`, si.At.Unix(), si.Account, si.Source, si.OK, si.Why)
	s.prune()
	return err
}

// SignIns lists the record, newest first, at most limit of them, for one
// account or all ("").
func (s *Store) SignIns(account string, limit int) ([]SignIn, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	q, args := `SELECT at, account, source, ok, why FROM signins`, []any{}
	if account != "" {
		q += ` WHERE account = ?`
		args = append(args, account)
	}
	q += ` ORDER BY at DESC, rowid DESC LIMIT ?`
	rows, err := s.db.Query(q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SignIn{}
	for rows.Next() {
		var si SignIn
		var at int64
		var ok int
		if err := rows.Scan(&at, &si.Account, &si.Source, &ok, &si.Why); err != nil {
			return nil, err
		}
		si.At, si.OK = time.Unix(at, 0), ok != 0
		out = append(out, si)
	}
	return out, rows.Err()
}

// prune takes away, at most once an hour, the sessions that ended and the
// sign-ins older than SignInsKept.
func (s *Store) prune() {
	now := s.now()
	s.mu.Lock()
	if now.Sub(s.pruned) < pruneEvery {
		s.mu.Unlock()
		return
	}
	s.pruned = now
	s.mu.Unlock()
	s.db.Exec(`DELETE FROM sessions WHERE (keep = 0 AND last_used <= ?) OR (keep != 0 AND last_used <= ?)`,
		now.Add(-IdleTimeout).Unix(), now.Add(-KeepTimeout).Unix())
	s.db.Exec(`DELETE FROM signins WHERE at < ?`, now.Add(-SignInsKept).Unix())
}
