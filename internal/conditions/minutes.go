package conditions

import (
	"encoding/json"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What each minute came to on the APs' radios (0207), as aeolus-live sends
// it at each minute's end: a row a radio, and a row a client on a radio.
// The minute's start is its time. A minute an AP sends again is kept once.
const minutesTables = `
CREATE TABLE radio_minutes (
	ap          TEXT NOT NULL,
	at          TEXT NOT NULL,
	radio       TEXT NOT NULL,
	band        TEXT NOT NULL,
	channel     INTEGER,
	width       INTEGER,
	power       INTEGER,
	busy        REAL,
	busy_max    REAL,
	rx          REAL,
	tx          REAL,
	noise       INTEGER,
	clients     REAL,
	clients_max INTEGER,
	tx_packets  INTEGER NOT NULL DEFAULT 0,
	tx_retries  INTEGER NOT NULL DEFAULT 0,
	tx_failed   INTEGER NOT NULL DEFAULT 0,
	rx_packets  INTEGER NOT NULL DEFAULT 0,
	tx_bytes    INTEGER NOT NULL DEFAULT 0,
	rx_bytes    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (ap, at, radio)
) WITHOUT ROWID;
CREATE INDEX radio_minutes_at ON radio_minutes (at);
CREATE TABLE client_minutes (
	mac         TEXT NOT NULL,
	at          TEXT NOT NULL,
	ap          TEXT NOT NULL,
	band        TEXT NOT NULL,
	ssid        TEXT NOT NULL DEFAULT '',
	signal      INTEGER,
	signal_min  INTEGER,
	tx_rate     REAL,
	rx_rate     REAL,
	tx_packets  INTEGER NOT NULL DEFAULT 0,
	tx_retries  INTEGER NOT NULL DEFAULT 0,
	tx_failed   INTEGER NOT NULL DEFAULT 0,
	rx_packets  INTEGER NOT NULL DEFAULT 0,
	tx_bytes    INTEGER NOT NULL DEFAULT 0,
	rx_bytes    INTEGER NOT NULL DEFAULT 0,
	secs        INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (mac, at, ap, band)
) WITHOUT ROWID;
CREATE INDEX client_minutes_at ON client_minutes (at);
CREATE INDEX client_minutes_ap ON client_minutes (ap, at);`

// Counts are what was sent to a radio's clients over a minute, or to one
// client: packets, those tried again, those lost, and bytes each way.
type Counts struct {
	TxPackets int64 `json:"tx_packets"`
	TxRetries int64 `json:"tx_retries"`
	TxFailed  int64 `json:"tx_failed"`
	RxPackets int64 `json:"rx_packets"`
	TxBytes   int64 `json:"tx_bytes"`
	RxBytes   int64 `json:"rx_bytes"`
}

// RadioMinute is what a minute came to on one radio (0207): its channel,
// width and power; how busy its channel was, in percent, on average and at
// its busiest 5 s, and its own receiving and sending; its noise floor; its
// clients on average and at most; and what was sent to them.
type RadioMinute struct {
	AP         hierarchy.NodeID `json:"ap"`
	At         time.Time        `json:"at"`
	Radio      string           `json:"radio"`
	Band       string           `json:"band"`
	Channel    *int             `json:"channel"`
	Width      *int             `json:"width"`
	Power      *int             `json:"power"`
	Busy       *float64         `json:"busy"`
	BusyMax    *float64         `json:"busy_max"`
	Rx         *float64         `json:"rx"`
	Tx         *float64         `json:"tx"`
	Noise      *int             `json:"noise"`
	Clients    *float64         `json:"clients"`
	ClientsMax *int             `json:"clients_max"`
	Counts
}

// ClientMinute is what a minute came to for one client on one radio
// (0207): its signal, on average and at its weakest; its rates each way,
// in Mbit/s; what was sent to it; and for how many seconds of the minute
// it was there.
type ClientMinute struct {
	MAC       string           `json:"mac"`
	At        time.Time        `json:"at"`
	AP        hierarchy.NodeID `json:"ap"`
	Band      string           `json:"band"`
	SSID      string           `json:"ssid"`
	Signal    *int             `json:"signal"`
	SignalMin *int             `json:"signal_min"`
	TxRate    *float64         `json:"tx_rate"`
	RxRate    *float64         `json:"rx_rate"`
	Secs      int              `json:"secs"`
	// Heard is what it said it hears of the AP, in dBm (802.11k), where it
	// said it lately, at the AP's power then.
	Heard *int `json:"heard,omitempty"`
	Counts
}

// minutesHeard keeps what each client said it hears of the AP over a
// minute (802.11k), where it said it lately.
const minutesHeard = `ALTER TABLE client_minutes ADD COLUMN heard INTEGER;`

// RecordMinutes keeps what an AP's minutes came to, each once.
func (s *Store) RecordMinutes(radios []RadioMinute, clients []ClientMinute) error {
	if len(radios) == 0 && len(clients) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range radios {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO radio_minutes (ap, at, radio, band, channel, width, power, busy, busy_max, rx, tx, noise,
			clients, clients_max, tx_packets, tx_retries, tx_failed, rx_packets, tx_bytes, rx_bytes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(r.AP), stamp(r.At), r.Radio, r.Band, r.Channel, r.Width, r.Power, r.Busy, r.BusyMax, r.Rx, r.Tx, r.Noise,
			r.Clients, r.ClientsMax, r.TxPackets, r.TxRetries, r.TxFailed, r.RxPackets, r.TxBytes, r.RxBytes); err != nil {
			return err
		}
	}
	for _, c := range clients {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO client_minutes (mac, at, ap, band, ssid, signal, signal_min, tx_rate, rx_rate,
			tx_packets, tx_retries, tx_failed, rx_packets, tx_bytes, rx_bytes, secs, heard) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.MAC, stamp(c.At), string(c.AP), c.Band, c.SSID, c.Signal, c.SignalMin, c.TxRate, c.RxRate,
			c.TxPackets, c.TxRetries, c.TxFailed, c.RxPackets, c.TxBytes, c.RxBytes, c.Secs, c.Heard); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RadioMinutes is the radios' minutes on aps since since, on band where it
// is said, by AP, radio and time, the oldest first.
func (s *Store) RadioMinutes(since time.Time, aps []hierarchy.NodeID, band string) ([]RadioMinute, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT ap, at, radio, band, channel, width, power, busy, busy_max, rx, tx, noise, clients, clients_max,
		tx_packets, tx_retries, tx_failed, rx_packets, tx_bytes, rx_bytes FROM radio_minutes
		WHERE at >= ? AND ap IN (SELECT value FROM json_each(?)) AND (? = '' OR band = ?) ORDER BY ap, radio, at`, stamp(since), string(ids), band, band)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RadioMinute{}
	for rows.Next() {
		var r RadioMinute
		var ap, at string
		if err := rows.Scan(&ap, &at, &r.Radio, &r.Band, &r.Channel, &r.Width, &r.Power, &r.Busy, &r.BusyMax, &r.Rx, &r.Tx, &r.Noise,
			&r.Clients, &r.ClientsMax, &r.TxPackets, &r.TxRetries, &r.TxFailed, &r.RxPackets, &r.TxBytes, &r.RxBytes); err != nil {
			return nil, err
		}
		r.AP = hierarchy.NodeID(ap)
		if r.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LastMinutes is when the newest minute the manager keeps of an AP's
// radios began (0311): of each radio, over the minutes since since; and of
// any radio, of all it keeps. An AP it keeps none of has none and a zero
// time.
func (s *Store) LastMinutes(ap hierarchy.NodeID, since time.Time) (map[string]time.Time, time.Time, error) {
	radios := map[string]time.Time{}
	var last time.Time
	var at *string
	if err := s.db.QueryRow(`SELECT MAX(at) FROM radio_minutes WHERE ap = ?`, string(ap)).Scan(&at); err != nil {
		return nil, last, err
	}
	if at == nil {
		return radios, last, nil
	}
	var err error
	if last, err = time.Parse(time.RFC3339Nano, *at); err != nil {
		return nil, last, err
	}
	rows, err := s.db.Query(`SELECT radio, MAX(at) FROM radio_minutes WHERE ap = ? AND at >= ? GROUP BY radio`, string(ap), stamp(since))
	if err != nil {
		return nil, last, err
	}
	defer rows.Close()
	for rows.Next() {
		var radio, at string
		if err := rows.Scan(&radio, &at); err != nil {
			return nil, last, err
		}
		t, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, last, err
		}
		radios[radio] = t
	}
	return radios, last, rows.Err()
}

// ClientMinutes is one client's minutes on aps since since, the oldest
// first.
func (s *Store) ClientMinutes(mac string, since time.Time, aps []hierarchy.NodeID) ([]ClientMinute, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT mac, at, ap, band, ssid, signal, signal_min, tx_rate, rx_rate, tx_packets, tx_retries, tx_failed,
		rx_packets, tx_bytes, rx_bytes, secs, heard FROM client_minutes
		WHERE mac = ? AND at >= ? AND ap IN (SELECT value FROM json_each(?)) ORDER BY at, ap, band`, mac, stamp(since), string(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClientMinute{}
	for rows.Next() {
		var c ClientMinute
		var ap, at string
		if err := rows.Scan(&c.MAC, &at, &ap, &c.Band, &c.SSID, &c.Signal, &c.SignalMin, &c.TxRate, &c.RxRate,
			&c.TxPackets, &c.TxRetries, &c.TxFailed, &c.RxPackets, &c.TxBytes, &c.RxBytes, &c.Secs, &c.Heard); err != nil {
			return nil, err
		}
		c.AP = hierarchy.NodeID(ap)
		if c.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// EachClientMinute calls fn with every client's minutes since since on one
// of aps, a client at a time and each client's oldest first (0340), with
// their signal; minutes with none are left out. Only what the sticky list
// weighs is read: the client, the time, the AP, the band, the SSID and the
// signal.
func (s *Store) EachClientMinute(since time.Time, aps []hierarchy.NodeID, fn func(ClientMinute) error) error {
	ids, err := json.Marshal(aps)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT mac, at, ap, band, ssid, signal FROM client_minutes
		WHERE at >= ? AND signal IS NOT NULL AND ap IN (SELECT value FROM json_each(?)) ORDER BY mac, at, ap, band`, stamp(since), string(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c ClientMinute
		var ap, at string
		if err := rows.Scan(&c.MAC, &at, &ap, &c.Band, &c.SSID, &c.Signal); err != nil {
			return err
		}
		c.AP = hierarchy.NodeID(ap)
		if c.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return err
		}
		if err := fn(c); err != nil {
			return err
		}
	}
	return rows.Err()
}

// PruneMinutes forgets the radios' minutes before radios and the clients'
// before clients: how many of each.
func (s *Store) PruneMinutes(radios, clients time.Time) (int64, int64, error) {
	del := func(q string, before time.Time) (int64, error) {
		res, err := s.db.Exec(q, stamp(before))
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	r, err := del(`DELETE FROM radio_minutes WHERE at < ?`, radios)
	if err != nil {
		return 0, 0, err
	}
	c, err := del(`DELETE FROM client_minutes WHERE at < ?`, clients)
	return r, c, err
}
