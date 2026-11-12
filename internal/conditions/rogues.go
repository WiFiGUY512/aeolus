package conditions

import (
	"encoding/json"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Rogue clients (0160): each client a scan radio heard talking through a
// rogue AP or a suspect one, once for each rogue and each AP that heard it:
// when first and last, the most frames seen, and the rogue's SSID as last
// heard. Times are whole milliseconds since 1970. They go when the clients'
// attempts do (0118).
const rogueClientsTable = `
CREATE TABLE rogue_clients (
	bssid    TEXT NOT NULL,
	mac      TEXT NOT NULL,
	ap       TEXT NOT NULL,
	ssid     TEXT NOT NULL,
	first_at INTEGER NOT NULL,
	last_at  INTEGER NOT NULL,
	frames   INTEGER NOT NULL,
	PRIMARY KEY (bssid, mac, ap)
) WITHOUT ROWID;
CREATE INDEX rogue_clients_last ON rogue_clients (last_at);`

// RogueClient is a client heard on a rogue: through which BSSID and SSID, by
// which AP, when first and last, and the most frames seen.
type RogueClient struct {
	MAC     string
	BSSID   string
	SSID    string
	AP      hierarchy.NodeID
	FirstAt time.Time
	LastAt  time.Time
	Frames  int64
}

// RecordRogueClients keeps the clients one AP heard on rogues, each at the
// time it was heard, LastAt.
func (s *Store) RecordRogueClients(ap hierarchy.NodeID, list []RogueClient) error {
	if len(list) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range list {
		at := c.LastAt.UnixMilli()
		if _, err := tx.Exec(`INSERT INTO rogue_clients (bssid, mac, ap, ssid, first_at, last_at, frames) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (bssid, mac, ap) DO UPDATE SET ssid = excluded.ssid, first_at = min(first_at, excluded.first_at),
				last_at = max(last_at, excluded.last_at), frames = max(frames, excluded.frames)`,
			c.BSSID, c.MAC, string(ap), c.SSID, at, at, c.Frames); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RogueClients is every client one of aps heard on a rogue since a time,
// the last heard first, at most limit.
func (s *Store) RogueClients(since time.Time, aps []hierarchy.NodeID, limit int) ([]RogueClient, error) {
	if aps == nil {
		aps = []hierarchy.NodeID{}
	}
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT bssid, mac, ap, ssid, first_at, last_at, frames FROM rogue_clients
		WHERE last_at >= ? AND ap IN (SELECT value FROM json_each(?)) ORDER BY last_at DESC, mac, bssid, ap LIMIT ?`, since.UnixMilli(), string(ids), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RogueClient{}
	for rows.Next() {
		var c RogueClient
		var ap string
		var first, last int64
		if err := rows.Scan(&c.BSSID, &c.MAC, &ap, &c.SSID, &first, &last, &c.Frames); err != nil {
			return nil, err
		}
		c.AP, c.FirstAt, c.LastAt = hierarchy.NodeID(ap), time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}
