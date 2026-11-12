package conditions

import (
	"database/sql"
	"time"
)

// suggestionsTable is what people ask Aeolus to do, from its pages (0323):
// each kept here first, with the page it was asked from, then sent on to
// where the manager is told to send them, tried again until it gets there.
// A time is milliseconds; sent_at 0 is not sent yet.
const suggestionsTable = `
CREATE TABLE suggestions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	at         INTEGER NOT NULL,
	account    TEXT NOT NULL,
	body       TEXT NOT NULL,
	contact    TEXT NOT NULL DEFAULT '',
	route      TEXT NOT NULL DEFAULT '',
	title      TEXT NOT NULL DEFAULT '',
	version    TEXT NOT NULL DEFAULT '',
	sent_at    INTEGER NOT NULL DEFAULT 0,
	sent_to    TEXT NOT NULL DEFAULT '',
	tries      INTEGER NOT NULL DEFAULT 0,
	next_try   INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX suggestions_due ON suggestions (sent_at, next_try);
CREATE INDEX suggestions_by ON suggestions (account, at);`

// Suggestion is one thing someone asked Aeolus to do (0323): what they
// wrote, how to reach them if they said, the page and release they asked
// from, and whether it has been sent on, where, or why not yet.
type Suggestion struct {
	ID        int64      `json:"id"`
	At        time.Time  `json:"at"`
	Account   string     `json:"account"`
	Text      string     `json:"text"`
	Contact   string     `json:"contact,omitempty"`
	Route     string     `json:"route,omitempty"`
	Title     string     `json:"title,omitempty"`
	Version   string     `json:"version,omitempty"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	SentTo    string     `json:"sent_to,omitempty"`
	Tries     int        `json:"tries,omitempty"`
	LastError string     `json:"last_error,omitempty"`
}

const suggestionColumns = `id, at, account, body, contact, route, title, version, sent_at, sent_to, tries, last_error`

func scanSuggestion(sc interface{ Scan(...any) error }) (Suggestion, error) {
	var x Suggestion
	var at, sent int64
	if err := sc.Scan(&x.ID, &at, &x.Account, &x.Text, &x.Contact, &x.Route, &x.Title, &x.Version, &sent, &x.SentTo, &x.Tries, &x.LastError); err != nil {
		return Suggestion{}, err
	}
	x.At = time.UnixMilli(at).UTC()
	if sent > 0 {
		t := time.UnixMilli(sent).UTC()
		x.SentAt = &t
	}
	return x, nil
}

func scanSuggestions(rows *sql.Rows, err error) ([]Suggestion, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Suggestion{}
	for rows.Next() {
		x, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// AddSuggestion keeps a suggestion, made now, and returns it as kept.
func (s *Store) AddSuggestion(x Suggestion) (Suggestion, error) {
	x.At = s.now().UTC()
	res, err := s.db.Exec(`INSERT INTO suggestions (at, account, body, contact, route, title, version) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		x.At.UnixMilli(), x.Account, x.Text, x.Contact, x.Route, x.Title, x.Version)
	if err != nil {
		return Suggestion{}, err
	}
	x.ID, err = res.LastInsertId()
	return x, err
}

// SuggestionsBy counts the suggestions an account made since a time.
func (s *Store) SuggestionsBy(account string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM suggestions WHERE account = ? AND at >= ?`, account, since.UnixMilli()).Scan(&n)
	return n, err
}

// Suggestions are the latest suggestions, newest first, at most limit.
func (s *Store) Suggestions(limit int) ([]Suggestion, error) {
	return scanSuggestions(s.db.Query(`SELECT `+suggestionColumns+` FROM suggestions ORDER BY id DESC LIMIT ?`, limit))
}

// SuggestionsDue are those not yet sent whose next try has come, oldest
// first, at most limit.
func (s *Store) SuggestionsDue(now time.Time, limit int) ([]Suggestion, error) {
	return scanSuggestions(s.db.Query(`SELECT `+suggestionColumns+` FROM suggestions WHERE sent_at = 0 AND next_try <= ? ORDER BY id LIMIT ?`, now.UnixMilli(), limit))
}

// SuggestionSent records a suggestion sent on, to where.
func (s *Store) SuggestionSent(id int64, to string) error {
	_, err := s.db.Exec(`UPDATE suggestions SET sent_at = ?, sent_to = ?, last_error = '' WHERE id = ?`, s.now().UnixMilli(), to, id)
	return err
}

// SuggestionFailed records a try that did not get a suggestion there, why,
// and when it is tried again.
func (s *Store) SuggestionFailed(id int64, why string, next time.Time) error {
	_, err := s.db.Exec(`UPDATE suggestions SET tries = tries + 1, last_error = ?, next_try = ? WHERE id = ?`, why, next.UnixMilli(), id)
	return err
}
