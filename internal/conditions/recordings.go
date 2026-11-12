package conditions

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The flight recorder's recordings (0207 part 3). When something goes wrong
// on an AP, or someone asks, it sends what each five seconds came to from
// ten minutes before to ten minutes after. A recording is the AP, when
// what went wrong did and why, and the stretch it covers; its points are
// kept as the AP sent them, each once.
const recordingsTables = `
CREATE TABLE recordings (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	ap       TEXT NOT NULL,
	at       TEXT NOT NULL,
	why      TEXT NOT NULL,
	said     TEXT NOT NULL DEFAULT '',
	from_at  TEXT NOT NULL,
	until_at TEXT NOT NULL,
	UNIQUE (ap, at)
);
CREATE INDEX recordings_at ON recordings (at);
CREATE TABLE recording_points (
	recording INTEGER NOT NULL,
	at        TEXT NOT NULL,
	data      TEXT NOT NULL,
	PRIMARY KEY (recording, at)
) WITHOUT ROWID;`

// Recording is one of the flight recorder's recordings: Points is how many
// five-second points it has.
type Recording struct {
	ID     int64            `json:"id"`
	AP     hierarchy.NodeID `json:"ap"`
	At     time.Time        `json:"at"`
	Why    string           `json:"why"`
	Said   string           `json:"said,omitempty"`
	From   time.Time        `json:"from"`
	Until  time.Time        `json:"until"`
	Points int              `json:"points"`
}

// RecordedPoint is one five-second point of a recording, as the AP sent it.
type RecordedPoint struct {
	At   time.Time       `json:"at"`
	Data json.RawMessage `json:"data"`
}

// ErrNoRecording is a recording that is not kept.
var ErrNoRecording = errors.New("no such recording")

// OpenRecording keeps that an AP is recording what went wrong at at: a new
// recording, or one it has, made longer to until.
func (s *Store) OpenRecording(ap hierarchy.NodeID, at time.Time, why, said string, from, until time.Time) error {
	_, err := s.db.Exec(`INSERT INTO recordings (ap, at, why, said, from_at, until_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (ap, at) DO UPDATE SET until_at = MAX(until_at, excluded.until_at)`,
		string(ap), stamp(at), why, said, stamp(from), stamp(until))
	return err
}

// RecordPoints keeps the points of an AP that fall in one of its
// recordings, each once, and says how many it kept. data is each point as
// it is kept, by its time.
func (s *Store) RecordPoints(ap hierarchy.NodeID, points map[time.Time][]byte) (int, error) {
	if len(points) == 0 {
		return 0, nil
	}
	first := time.Time{}
	for at := range points {
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}
	rows, err := s.db.Query(`SELECT id, from_at, until_at FROM recordings WHERE ap = ? AND until_at >= ?`, string(ap), stamp(first))
	if err != nil {
		return 0, err
	}
	type span struct {
		id       int64
		from, to time.Time
	}
	var spans []span
	for rows.Next() {
		var sp span
		var from, to string
		if err := rows.Scan(&sp.id, &from, &to); err != nil {
			rows.Close()
			return 0, err
		}
		sp.from, _ = time.Parse(time.RFC3339Nano, from)
		sp.to, _ = time.Parse(time.RFC3339Nano, to)
		spans = append(spans, sp)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(spans) == 0 {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	kept := 0
	for at, data := range points {
		for _, sp := range spans {
			if at.Before(sp.from) || at.After(sp.to) {
				continue
			}
			res, err := tx.Exec(`INSERT OR IGNORE INTO recording_points (recording, at, data) VALUES (?, ?, ?)`, sp.id, stamp(at), string(data))
			if err != nil {
				return 0, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				kept++
			}
		}
	}
	return kept, tx.Commit()
}

// Recordings is the recordings of aps that went wrong since since, the
// latest first, at most limit.
func (s *Store) Recordings(since time.Time, aps []hierarchy.NodeID, limit int) ([]Recording, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT r.id, r.ap, r.at, r.why, r.said, r.from_at, r.until_at,
		(SELECT COUNT(*) FROM recording_points p WHERE p.recording = r.id)
		FROM recordings r WHERE r.at >= ? AND r.ap IN (SELECT value FROM json_each(?)) ORDER BY r.at DESC LIMIT ?`, stamp(since), string(ids), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recording{}
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanRecording(row interface{ Scan(...any) error }) (Recording, error) {
	var r Recording
	var ap, at, from, until string
	if err := row.Scan(&r.ID, &ap, &at, &r.Why, &r.Said, &from, &until, &r.Points); err != nil {
		return r, err
	}
	r.AP = hierarchy.NodeID(ap)
	r.At, _ = time.Parse(time.RFC3339Nano, at)
	r.From, _ = time.Parse(time.RFC3339Nano, from)
	r.Until, _ = time.Parse(time.RFC3339Nano, until)
	return r, nil
}

// RecordingOf is one recording, with its points, the oldest first.
func (s *Store) RecordingOf(id int64) (Recording, []RecordedPoint, error) {
	r, err := scanRecording(s.db.QueryRow(`SELECT r.id, r.ap, r.at, r.why, r.said, r.from_at, r.until_at,
		(SELECT COUNT(*) FROM recording_points p WHERE p.recording = r.id) FROM recordings r WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil, ErrNoRecording
	}
	if err != nil {
		return r, nil, err
	}
	rows, err := s.db.Query(`SELECT at, data FROM recording_points WHERE recording = ? ORDER BY at`, id)
	if err != nil {
		return r, nil, err
	}
	defer rows.Close()
	pts := []RecordedPoint{}
	for rows.Next() {
		var at, data string
		if err := rows.Scan(&at, &data); err != nil {
			return r, nil, err
		}
		t, _ := time.Parse(time.RFC3339Nano, at)
		pts = append(pts, RecordedPoint{At: t, Data: json.RawMessage(data)})
	}
	return r, pts, rows.Err()
}

// PruneRecordings forgets the recordings of what went wrong before before,
// and their points.
func (s *Store) PruneRecordings(before time.Time) (int64, error) {
	if _, err := s.db.Exec(`DELETE FROM recording_points WHERE recording IN (SELECT id FROM recordings WHERE at < ?)`, stamp(before)); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM recordings WHERE at < ?`, stamp(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
