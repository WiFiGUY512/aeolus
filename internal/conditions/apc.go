package conditions

import (
	"encoding/json"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Power control's steps (0206), as the APs report them: when, the band,
// from and to in dBm, why, and what the clients did where they had a say.
// An AP reports its latest steps with every report, so a step is kept once.
const apcTable = `
CREATE TABLE apc_events (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	ap       TEXT NOT NULL,
	at       TEXT NOT NULL,
	band     TEXT NOT NULL,
	from_dbm INTEGER,
	to_dbm   INTEGER,
	why      TEXT NOT NULL,
	scanning INTEGER,
	left_n   INTEGER,
	ratio    TEXT NOT NULL DEFAULT '',
	UNIQUE (ap, at, band, why)
);
CREATE INDEX apc_events_at ON apc_events (at);`

// APCEvent is one of power control's steps on an AP (0206). Ratio is the
// share of what was sent to its clients that was tried again or lost,
// before and after, where it was said.
type APCEvent struct {
	AP       hierarchy.NodeID `json:"ap"`
	At       time.Time        `json:"at"`
	Band     string           `json:"band"`
	From     *int             `json:"from"`
	To       *int             `json:"to"`
	Why      string           `json:"why"`
	Scanning *int             `json:"scanning,omitempty"`
	Left     *int             `json:"left,omitempty"`
	Ratio    []*float64       `json:"ratio,omitempty"`
}

// RecordAPCEvents keeps an AP's steps, each once.
func (s *Store) RecordAPCEvents(ap hierarchy.NodeID, events []APCEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range events {
		ratio := ""
		if len(e.Ratio) > 0 {
			raw, _ := json.Marshal(e.Ratio)
			ratio = string(raw)
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO apc_events (ap, at, band, from_dbm, to_dbm, why, scanning, left_n, ratio)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, string(ap), stamp(e.At), e.Band, e.From, e.To, e.Why, e.Scanning, e.Left, ratio); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// APCEvents is the steps on aps since since, newest first, at most limit.
func (s *Store) APCEvents(since time.Time, aps []hierarchy.NodeID, limit int) ([]APCEvent, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT ap, at, band, from_dbm, to_dbm, why, scanning, left_n, ratio FROM apc_events
		WHERE at >= ? AND ap IN (SELECT value FROM json_each(?)) ORDER BY at DESC, id DESC LIMIT ?`, stamp(since), string(ids), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APCEvent{}
	for rows.Next() {
		var e APCEvent
		var ap, at, ratio string
		if err := rows.Scan(&ap, &at, &e.Band, &e.From, &e.To, &e.Why, &e.Scanning, &e.Left, &ratio); err != nil {
			return nil, err
		}
		e.AP = hierarchy.NodeID(ap)
		if e.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		if ratio != "" {
			json.Unmarshal([]byte(ratio), &e.Ratio)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneAPCEvents forgets the steps before before.
func (s *Store) PruneAPCEvents(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM apc_events WHERE at < ?`, stamp(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
