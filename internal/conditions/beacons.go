package conditions

import (
	"encoding/json"
	"math"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Each radio's beacons at each state report (0339): those its networks
// sent and those that did not go out since the agent's last report, over
// that many seconds (0221), a row a radio a report. The beacons alert reads
// the last half hour of them, so a burst of a minute or two, which ath11k's
// radios have every hour or so, says nothing alone. Kept as long as the
// reports are. Times are whole milliseconds since 1970.
const beaconsTable = `
CREATE TABLE beacons (
	ap     TEXT NOT NULL,
	at     INTEGER NOT NULL,
	radio  TEXT NOT NULL,
	band   TEXT NOT NULL,
	sent   INTEGER NOT NULL,
	failed INTEGER NOT NULL,
	over   INTEGER NOT NULL
);
CREATE INDEX beacons_ap ON beacons (ap, at);
CREATE INDEX beacons_at ON beacons (at);`

// RadioBeacons is a radio's beacons over a time (0221, 0339): those its
// networks sent, all of them together, those that did not go out, and over
// how many seconds.
type RadioBeacons struct {
	Radio  string `json:"radio"`
	Band   string `json:"band"`
	Sent   int64  `json:"sent"`
	Failed int64  `json:"failed"`
	Over   int    `json:"over"`
}

// RecordBeacons keeps an AP's radios' beacons at a state report (0339).
func (s *Store) RecordBeacons(ap hierarchy.NodeID, radios []RadioBeacons) error {
	if len(radios) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := s.now().UnixMilli()
	for _, b := range radios {
		if _, err := tx.Exec(`INSERT INTO beacons (ap, at, radio, band, sent, failed, over) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			string(ap), at, b.Radio, b.Band, b.Sent, b.Failed, b.Over); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BeaconsSince is what each of an AP's radios' beacons came to over the
// reports since since (0339), a radio on a band each, by radio.
func (s *Store) BeaconsSince(ap hierarchy.NodeID, since time.Time) ([]RadioBeacons, error) {
	rows, err := s.db.Query(`SELECT radio, band, sum(sent), sum(failed), sum(over) FROM beacons
		WHERE ap = ? AND at >= ? GROUP BY radio, band ORDER BY radio, band`, string(ap), since.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RadioBeacons
	for rows.Next() {
		var b RadioBeacons
		if err := rows.Scan(&b.Radio, &b.Band, &b.Sent, &b.Failed, &b.Over); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BeaconsStretch is what a radio's beacons came to over the reports of one
// stretch of time (0350), At its start.
type BeaconsStretch struct {
	AP hierarchy.NodeID
	At time.Time
	RadioBeacons
}

// beaconsOver is BeaconsOver's query: each radio's beacons summed over the
// reports in each stretch, read by the AP and time index (beacons_ap), the
// span's rows alone.
const beaconsOver = `SELECT ap, radio, band, (at - ?) / ? AS i, sum(sent), sum(failed), sum(over) FROM beacons
	WHERE ap IN (SELECT value FROM json_each(?)) AND at >= ? AND (? = '' OR band = ?)
	GROUP BY ap, radio, band, i ORDER BY ap, radio, band, i LIMIT ?`

// BeaconsOver is what the radios of the APs aps sent and failed to send
// (0350), summed over the reports in each stretch of every from since on,
// by AP, radio, band and time, the oldest first: a stretch with no report
// has no row. every 0 or less is one stretch, since since. band, where it
// is set, is that band's alone. It reads at most most rows.
func (s *Store) BeaconsOver(aps []hierarchy.NodeID, band string, since time.Time, every time.Duration, most int) ([]BeaconsStretch, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	start, step := since.UnixMilli(), every.Milliseconds()
	if step <= 0 {
		step = math.MaxInt64
	}
	rows, err := s.db.Query(beaconsOver, start, step, string(ids), start, band, band, most)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BeaconsStretch{}
	for rows.Next() {
		var b BeaconsStretch
		var ap string
		var i int64
		if err := rows.Scan(&ap, &b.Radio, &b.Band, &i, &b.Sent, &b.Failed, &b.Over); err != nil {
			return nil, err
		}
		b.AP = hierarchy.NodeID(ap)
		b.At = since
		if every > 0 {
			b.At = since.Add(time.Duration(i) * every)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
