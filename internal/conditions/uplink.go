package conditions

import (
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What each minute came to on the APs' uplinks (0351), as aeolus-live sends
// it with the radios' (0207): a row an Ethernet port under the uplink. Kept
// as long as the radios' minutes, and pruned with them.
const uplinkMinutesTable = `
CREATE TABLE uplink_minutes (
	ap       TEXT NOT NULL,
	at       TEXT NOT NULL,
	port     TEXT NOT NULL,
	up       INTEGER NOT NULL,
	speed    INTEGER,
	full     INTEGER,
	secs     INTEGER NOT NULL,
	rx       REAL,
	tx       REAL,
	rx_max   REAL,
	tx_max   REAL,
	rx_bytes INTEGER NOT NULL,
	tx_bytes INTEGER NOT NULL,
	errors   INTEGER NOT NULL,
	changes  INTEGER NOT NULL,
	PRIMARY KEY (ap, at, port)
) WITHOUT ROWID;
CREATE INDEX uplink_minutes_at ON uplink_minutes (at);`

// UplinkMinute is what a minute came to on one of an AP's uplink ports
// (0351): whether it had a link, its speed in Mbit/s and its duplex as last
// sampled; over how many seconds its bytes were counted; its Mbit/s each
// way on average and over its busiest five seconds, rx being what came in
// to the AP; its bytes each way; its errors each way together; and how
// often its link came or went.
type UplinkMinute struct {
	AP      hierarchy.NodeID `json:"ap"`
	At      time.Time        `json:"at"`
	Port    string           `json:"port"`
	Up      bool             `json:"up"`
	Speed   *int             `json:"speed"`
	Full    *bool            `json:"full"`
	Secs    int              `json:"secs"`
	Rx      *float64         `json:"rx"`
	Tx      *float64         `json:"tx"`
	RxMax   *float64         `json:"rx_max"`
	TxMax   *float64         `json:"tx_max"`
	RxBytes int64            `json:"rx_bytes"`
	TxBytes int64            `json:"tx_bytes"`
	Errors  int64            `json:"errors"`
	Changes int64            `json:"changes"`
}

// RecordUplinkMinutes keeps what an AP's uplink ports' minutes came to,
// each once.
func (s *Store) RecordUplinkMinutes(ms []UplinkMinute) error {
	if len(ms) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, m := range ms {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO uplink_minutes (ap, at, port, up, speed, full, secs, rx, tx, rx_max, tx_max,
			rx_bytes, tx_bytes, errors, changes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(m.AP), stamp(m.At), m.Port, m.Up, m.Speed, m.Full, m.Secs, m.Rx, m.Tx, m.RxMax, m.TxMax,
			m.RxBytes, m.TxBytes, m.Errors, m.Changes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UplinkMinutes is an AP's uplink ports' minutes since since, by port and
// time, the oldest first, at most limit.
func (s *Store) UplinkMinutes(ap hierarchy.NodeID, since time.Time, limit int) ([]UplinkMinute, error) {
	rows, err := s.db.Query(`SELECT ap, at, port, up, speed, full, secs, rx, tx, rx_max, tx_max, rx_bytes, tx_bytes, errors, changes
		FROM uplink_minutes WHERE ap = ? AND at >= ? ORDER BY port, at LIMIT ?`, string(ap), stamp(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UplinkMinute{}
	for rows.Next() {
		var m UplinkMinute
		var ap, at string
		if err := rows.Scan(&ap, &at, &m.Port, &m.Up, &m.Speed, &m.Full, &m.Secs, &m.Rx, &m.Tx, &m.RxMax, &m.TxMax,
			&m.RxBytes, &m.TxBytes, &m.Errors, &m.Changes); err != nil {
			return nil, err
		}
		m.AP = hierarchy.NodeID(ap)
		if m.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PruneUplinkMinutes forgets the uplinks' minutes before before: how many.
func (s *Store) PruneUplinkMinutes(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM uplink_minutes WHERE at < ?`, stamp(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
