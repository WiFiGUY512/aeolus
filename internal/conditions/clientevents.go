package conditions

import (
	"encoding/json"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What happened to a client at an AP, as the AP said it at once (0255): a
// row an event, its kind ("left", a client's leaving) and the rest as JSON,
// so that a new kind, or more said of one, needs no new table. Kept as long
// as the attempts to come online are (TrimConnections).
const clientEventsTable = `
CREATE TABLE client_events (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	ap     TEXT NOT NULL,
	mac    TEXT NOT NULL,
	at     INTEGER NOT NULL,
	kind   TEXT NOT NULL,
	band   TEXT NOT NULL,
	ssid   TEXT NOT NULL,
	detail TEXT NOT NULL DEFAULT '{}',
	UNIQUE (ap, mac, at, kind)
);
CREATE INDEX client_events_at ON client_events (kind, at);
CREATE INDEX client_events_mac ON client_events (mac, at);`

// ClientEvent is one thing that happened to a client at an AP (0255), at
// when the AP said, by its clock. Detail is what the kind says of it, as
// JSON.
type ClientEvent struct {
	AP     hierarchy.NodeID
	MAC    string
	At     time.Time
	Kind   string
	Band   string
	SSID   string
	Detail json.RawMessage
}

// RecordClientEvents keeps events, each once: the same said again by the AP
// is the same row.
func (s *Store) RecordClientEvents(list []ClientEvent) error {
	if len(list) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range list {
		detail := string(e.Detail)
		if detail == "" {
			detail = "{}"
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO client_events (ap, mac, at, kind, band, ssid, detail) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			string(e.AP), e.MAC, e.At.UnixMilli(), e.Kind, e.Band, e.SSID, detail); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClientEventQuery is which events to read: of a kind, at APs, of one
// client where MAC is set, on a band where Band is, from Since to Until
// (Until the zero time for now), newest first, at most Limit.
type ClientEventQuery struct {
	Kind         string
	APs          []hierarchy.NodeID
	MAC          string
	Band         string
	Since, Until time.Time
	Limit        int
}

// ClientEvents reads the events a query asks for.
func (s *Store) ClientEvents(q ClientEventQuery) ([]ClientEvent, error) {
	ids, err := json.Marshal(q.APs)
	if err != nil {
		return nil, err
	}
	until := q.Until
	if until.IsZero() {
		until = s.now().Add(time.Hour)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(`SELECT ap, mac, at, kind, band, ssid, detail FROM client_events
		WHERE kind = ? AND at >= ? AND at < ? AND ap IN (SELECT value FROM json_each(?))
		AND (? = '' OR mac = ?) AND (? = '' OR band = ?)
		ORDER BY at DESC, id DESC LIMIT ?`,
		q.Kind, q.Since.UnixMilli(), until.UnixMilli(), string(ids), q.MAC, q.MAC, q.Band, q.Band, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientEvent
	for rows.Next() {
		var e ClientEvent
		var ap, detail string
		var at int64
		if err := rows.Scan(&ap, &e.MAC, &at, &e.Kind, &e.Band, &e.SSID, &detail); err != nil {
			return nil, err
		}
		e.AP, e.At, e.Detail = hierarchy.NodeID(ap), time.UnixMilli(at).UTC(), json.RawMessage(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}
