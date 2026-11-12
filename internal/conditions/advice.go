package conditions

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// The advisor's reviews and findings (0205). A review is Claude reading the
// manager's data, as a viewer, and saying what it found; a finding is one
// problem, with what it saw and what to do. A finding found again, by its
// key, is the same finding, so it is seen and counted, not repeated. They
// are observations for people, beside the change log and never in it.
const adviceTables = `
CREATE TABLE advice_runs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	started     TEXT NOT NULL,
	ended       TEXT NOT NULL DEFAULT '',
	why         TEXT NOT NULL,
	model       TEXT NOT NULL,
	requests    INTEGER NOT NULL DEFAULT 0,
	input       INTEGER NOT NULL DEFAULT 0,
	output      INTEGER NOT NULL DEFAULT 0,
	cache_read  INTEGER NOT NULL DEFAULT 0,
	cache_write INTEGER NOT NULL DEFAULT 0,
	searches    INTEGER NOT NULL DEFAULT 0,
	cost        REAL NOT NULL DEFAULT 0,
	outcome     TEXT NOT NULL DEFAULT 'running',
	summary     TEXT NOT NULL DEFAULT '',
	error       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX advice_runs_started ON advice_runs (started);
CREATE TABLE advice (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	key      TEXT NOT NULL,
	run      INTEGER NOT NULL,
	first_at TEXT NOT NULL,
	at       TEXT NOT NULL,
	times    INTEGER NOT NULL DEFAULT 1,
	kind     TEXT NOT NULL,
	severity TEXT NOT NULL,
	title    TEXT NOT NULL,
	saw      TEXT NOT NULL,
	todo     TEXT NOT NULL,
	subjects TEXT NOT NULL,
	state    TEXT NOT NULL DEFAULT 'open',
	state_by TEXT NOT NULL DEFAULT '',
	state_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX advice_key ON advice (key);
CREATE INDEX advice_at ON advice (at);`

// adviceScope gives each review and finding the folder whose advisor it is
// (0212): the Locations folder that holds the key. Those from before are
// the Org's, which AdoptAdvice names once the manager knows it.
const adviceScope = `
ALTER TABLE advice_runs ADD COLUMN scope TEXT NOT NULL DEFAULT '';
ALTER TABLE advice ADD COLUMN scope TEXT NOT NULL DEFAULT '';
CREATE INDEX advice_runs_scope ON advice_runs (scope, id);
CREATE INDEX advice_scope_key ON advice (scope, key);`

// adviceAsked keeps why a finding is in its state, as what the advisor saw
// when it found the problem gone, and the question a review answered
// (0215).
const adviceAsked = `
ALTER TABLE advice ADD COLUMN state_why TEXT NOT NULL DEFAULT '';
ALTER TABLE advice_runs ADD COLUMN question TEXT NOT NULL DEFAULT '';`

// AdviceRun is one review: the folder whose advisor made it (0212), when
// it began and ended, why it ran (the schedule, or who asked), the
// question it answered, where it was asked one (0215), the model,
// the requests it made and their tokens, the web searches, what it cost in
// US dollars, how it ended (running, done, budget, failed) and its summary
// or error.
type AdviceRun struct {
	ID         int64      `json:"id"`
	Scope      string     `json:"scope"`
	Started    time.Time  `json:"started"`
	Ended      *time.Time `json:"ended,omitempty"`
	Why        string     `json:"why"`
	Question   string     `json:"question,omitempty"`
	Model      string     `json:"model"`
	Requests   int        `json:"requests"`
	Input      int64      `json:"input"`
	Output     int64      `json:"output"`
	CacheRead  int64      `json:"cache_read"`
	CacheWrite int64      `json:"cache_write"`
	Searches   int64      `json:"searches"`
	Cost       float64    `json:"cost"`
	Outcome    string     `json:"outcome"`
	Summary    string     `json:"summary,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// Subjects are what a finding concerns: clients by MAC, APs by ID, and
// networks by ID.
type Subjects struct {
	Clients  []string `json:"clients,omitempty"`
	APs      []string `json:"aps,omitempty"`
	Networks []string `json:"networks,omitempty"`
}

// Finding is one problem a review found: the folder whose advisor found it
// (0212), its key, the review that last found it, when first and last, how
// many times, its kind and severity, its title, what it saw, what to do,
// what it concerns, and whether it is open, dismissed or resolved, by whom,
// when and why (0215).
type Finding struct {
	ID       int64      `json:"id"`
	Scope    string     `json:"scope"`
	Key      string     `json:"key"`
	Run      int64      `json:"run"`
	FirstAt  time.Time  `json:"first_at"`
	At       time.Time  `json:"at"`
	Times    int        `json:"times"`
	Kind     string     `json:"kind"`
	Severity string     `json:"severity"`
	Title    string     `json:"title"`
	Saw      string     `json:"saw"`
	Todo     string     `json:"todo"`
	Subjects Subjects   `json:"subjects"`
	State    string     `json:"state"`
	StateBy  string     `json:"state_by,omitempty"`
	StateAt  *time.Time `json:"state_at,omitempty"`
	StateWhy string     `json:"state_why,omitempty"`
}

// Resolver is who resolves a finding whose problem a review found gone
// (0215).
const Resolver = "the advisor"

// ErrNoFinding is a finding that is not there.
var ErrNoFinding = errors.New("no such finding")

// AdoptAdvice gives the reviews and findings kept before advisors were a
// folder's (0212) to the Org, root.
func (s *Store) AdoptAdvice(root string) error {
	if _, err := s.db.Exec(`UPDATE advice_runs SET scope = ? WHERE scope = ''`, root); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE advice SET scope = ? WHERE scope = ''`, root)
	return err
}

// BeginAdviceRun records a review by scope's advisor that starts now, one
// that answers question where that is set (0215), and returns its ID.
func (s *Store) BeginAdviceRun(scope, why, question, model string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO advice_runs (scope, started, why, question, model) VALUES (?, ?, ?, ?, ?)`, scope, stamp(s.now()), why, question, model)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAdviceRun records how a review is going, or how it ended: an
// outcome other than running ends it now.
func (s *Store) UpdateAdviceRun(r AdviceRun) error {
	ended := ""
	if r.Outcome != "running" {
		ended = stamp(s.now())
	}
	_, err := s.db.Exec(`UPDATE advice_runs SET ended = ?, requests = ?, input = ?, output = ?, cache_read = ?, cache_write = ?,
		searches = ?, cost = ?, outcome = ?, summary = ?, error = ? WHERE id = ?`,
		ended, r.Requests, r.Input, r.Output, r.CacheRead, r.CacheWrite, r.Searches, r.Cost, r.Outcome, r.Summary, r.Error, r.ID)
	return err
}

// AdviceRuns is scope's advisor's latest reviews, newest first, at most
// limit.
func (s *Store) AdviceRuns(scope string, limit int) ([]AdviceRun, error) {
	rows, err := s.db.Query(`SELECT id, scope, started, ended, why, question, model, requests, input, output, cache_read, cache_write, searches, cost,
		outcome, summary, error FROM advice_runs WHERE scope = ? ORDER BY id DESC LIMIT ?`, scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdviceRun{}
	for rows.Next() {
		var r AdviceRun
		var started, ended string
		if err := rows.Scan(&r.ID, &r.Scope, &started, &ended, &r.Why, &r.Question, &r.Model, &r.Requests, &r.Input, &r.Output, &r.CacheRead, &r.CacheWrite,
			&r.Searches, &r.Cost, &r.Outcome, &r.Summary, &r.Error); err != nil {
			return nil, err
		}
		if r.Started, err = time.Parse(time.RFC3339Nano, started); err != nil {
			return nil, err
		}
		if ended != "" {
			t, err := time.Parse(time.RFC3339Nano, ended)
			if err != nil {
				return nil, err
			}
			r.Ended = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// EndRunningAdvice ends, as failed, every review left running, as by a
// manager that stopped in the middle of one.
func (s *Store) EndRunningAdvice(why string) error {
	_, err := s.db.Exec(`UPDATE advice_runs SET ended = ?, outcome = 'failed', error = ? WHERE outcome = 'running'`, stamp(s.now()), why)
	return err
}

// RecordFinding records a finding of review run, by f.Scope's advisor. One
// with the key of that advisor's finding not dismissed is that finding found
// again: what it says now replaces what it said, and it is counted; a
// resolved one is open again, its problem back (0215). One with the key of
// a dismissed finding stays dismissed, and is counted, unless it is worse
// than it was, when it is open again. It returns the finding's ID, and
// whether it is new.
func (s *Store) RecordFinding(run int64, f Finding) (int64, bool, error) {
	subj, err := json.Marshal(f.Subjects)
	if err != nil {
		return 0, false, err
	}
	now := stamp(s.now())
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var id int64
	var state, severity string
	err = tx.QueryRow(`SELECT id, state, severity FROM advice WHERE scope = ? AND key = ? ORDER BY id DESC LIMIT 1`, f.Scope, f.Key).Scan(&id, &state, &severity)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := tx.Exec(`INSERT INTO advice (scope, key, run, first_at, at, kind, severity, title, saw, todo, subjects)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, f.Scope, f.Key, run, now, now, f.Kind, f.Severity, f.Title, f.Saw, f.Todo, string(subj))
		if err != nil {
			return 0, false, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, false, err
		}
		return id, true, tx.Commit()
	case err != nil:
		return 0, false, err
	}
	if state == "dismissed" && severityRank(f.Severity) <= severityRank(severity) {
		_, err = tx.Exec(`UPDATE advice SET run = ?, at = ?, times = times + 1 WHERE id = ?`, run, now, id)
	} else {
		_, err = tx.Exec(`UPDATE advice SET run = ?, at = ?, times = times + 1, kind = ?, severity = ?, title = ?, saw = ?, todo = ?,
			subjects = ?, state = 'open', state_by = CASE WHEN state = 'open' THEN state_by ELSE '' END,
			state_at = CASE WHEN state = 'open' THEN state_at ELSE '' END,
			state_why = CASE WHEN state = 'open' THEN state_why ELSE '' END WHERE id = ?`,
			run, now, f.Kind, f.Severity, f.Title, f.Saw, f.Todo, string(subj), id)
	}
	if err != nil {
		return 0, false, err
	}
	return id, false, tx.Commit()
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "warning":
		return 2
	}
	return 1
}

// FindingScope is the folder whose advisor made finding id (0212).
func (s *Store) FindingScope(id int64) (string, error) {
	var scope string
	err := s.db.QueryRow(`SELECT scope FROM advice WHERE id = ?`, id).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoFinding
	}
	return scope, err
}

// SetFindingState opens, dismisses or resolves a finding, as who, for
// why, which may be unset.
func (s *Store) SetFindingState(id int64, state, who, why string) error {
	res, err := s.db.Exec(`UPDATE advice SET state = ?, state_by = ?, state_at = ?, state_why = ? WHERE id = ?`, state, who, stamp(s.now()), why, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoFinding
	}
	return nil
}

// ResolveFinding resolves scope's advisor's open finding by key, as review
// run found its problem gone, for why: what it saw instead (0215). It
// returns the finding's ID, or ErrNoFinding where none by that key is open.
func (s *Store) ResolveFinding(scope, key string, run int64, why string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM advice WHERE scope = ? AND key = ? AND state = 'open' ORDER BY id DESC LIMIT 1`, scope, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoFinding
	}
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`UPDATE advice SET run = ?, state = 'resolved', state_by = ?, state_at = ?, state_why = ? WHERE id = ?`,
		run, Resolver, stamp(s.now()), why, id)
	return id, err
}

// Findings is scope's advisor's findings, the open first, each the latest
// found first, at most limit.
func (s *Store) Findings(scope string, limit int) ([]Finding, error) {
	rows, err := s.db.Query(`SELECT id, scope, key, run, first_at, at, times, kind, severity, title, saw, todo, subjects, state, state_by, state_at, state_why
		FROM advice WHERE scope = ? ORDER BY state = 'open' DESC, at DESC, id DESC LIMIT ?`, scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Finding{}
	for rows.Next() {
		var f Finding
		var first, at, subj, stateAt string
		if err := rows.Scan(&f.ID, &f.Scope, &f.Key, &f.Run, &first, &at, &f.Times, &f.Kind, &f.Severity, &f.Title, &f.Saw, &f.Todo, &subj,
			&f.State, &f.StateBy, &stateAt, &f.StateWhy); err != nil {
			return nil, err
		}
		if f.FirstAt, err = time.Parse(time.RFC3339Nano, first); err != nil {
			return nil, err
		}
		if f.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		if stateAt != "" {
			t, err := time.Parse(time.RFC3339Nano, stateAt)
			if err != nil {
				return nil, err
			}
			f.StateAt = &t
		}
		if err := json.Unmarshal([]byte(subj), &f.Subjects); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
