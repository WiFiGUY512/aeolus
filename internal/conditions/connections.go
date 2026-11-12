package conditions

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Clients and their connections (0118): every Wi-Fi client an AP has seen,
// kept for good, and each attempt one made to come online, passed or
// failed, as its AP recorded it step by step. Times here are whole
// milliseconds since 1970, so they sort as they are.
const connectionsTables = `
CREATE TABLE connections (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ap      TEXT NOT NULL,
	mac     TEXT NOT NULL,
	started INTEGER NOT NULL,
	network TEXT NOT NULL,
	ssid    TEXT NOT NULL,
	band    TEXT NOT NULL,
	outcome TEXT NOT NULL,
	stage   TEXT NOT NULL,
	reason  TEXT NOT NULL,
	took_ms INTEGER NOT NULL,
	record  TEXT NOT NULL,
	UNIQUE (ap, mac, started)
);
CREATE INDEX connections_mac ON connections (mac, started);
CREATE INDEX connections_started ON connections (started);
CREATE TABLE clients (
	mac          TEXT PRIMARY KEY,
	first_seen   INTEGER NOT NULL,
	last_seen    INTEGER NOT NULL,
	ap           TEXT NOT NULL,
	network      TEXT NOT NULL,
	ssid         TEXT NOT NULL,
	host         TEXT NOT NULL,
	user         TEXT NOT NULL,
	address      TEXT NOT NULL,
	attempts     INTEGER NOT NULL,
	failed       INTEGER NOT NULL,
	last_attempt INTEGER NOT NULL,
	last_outcome TEXT NOT NULL
);
CREATE INDEX clients_last ON clients (last_seen);`

// connectionsAt gives each attempt the time the manager takes it to have
// begun, at. started stays what its AP said, by its own clock: with its AP
// and its client, that is what tells one attempt from another, and it is
// the same however often the AP sends it. An AP whose clock is unset says a
// time long gone; given the time it came in as started, a report sent
// twice, as when the answer to the first was lost, was kept twice.
const connectionsAt = `
ALTER TABLE connections ADD COLUMN at INTEGER NOT NULL DEFAULT 0;
UPDATE connections SET at = started;
CREATE INDEX connections_at ON connections (mac, at);`

// connectionsByTime finds the attempts of a time by when the manager takes
// them to have begun, whoever made them: how the clients' joins went is
// counted over the last hour at every look for alerts (0127).
const connectionsByTime = `
CREATE INDEX connections_by_time ON connections (at);`

// Connection is one attempt a client made to come online on an AP. Record
// is what the AP sent of it: its steps, and what it learned of the client.
// Started is when it began, as the manager takes it; Said is when its AP
// said, in ms since 1970, which with the AP and the client tells it from
// any other.
type Connection struct {
	ID      int64            `json:"id"`
	AP      hierarchy.NodeID `json:"ap"`
	MAC     string           `json:"mac"`
	Started time.Time        `json:"started"`
	Network string           `json:"network"`
	SSID    string           `json:"ssid"`
	Band    string           `json:"band"`
	Outcome string           `json:"outcome"`
	Stage   string           `json:"stage"`
	Reason  string           `json:"reason"`
	TookMS  int64            `json:"took_ms"`
	Record  json.RawMessage  `json:"record"`
	// The capture kept of it, where one is (0135): an attempt that failed.
	Capture *CaptureKept `json:"capture,omitempty"`
	// What it showed of the client, for the client's own row.
	Host    string `json:"-"`
	Address string `json:"-"`
	Said    int64  `json:"-"`
}

// Signal is how strongly the AP heard the client as it came, in dBm, as
// the attempt's record says (0312); nil where it says none.
func (c Connection) Signal() *int {
	var r struct {
		Signal *int `json:"signal"`
	}
	if len(c.Record) == 0 || json.Unmarshal(c.Record, &r) != nil || r.Signal == nil || *r.Signal >= 0 || *r.Signal < -127 {
		return nil
	}
	return r.Signal
}

// Client is a Wi-Fi client as last seen: where, and what it said of itself;
// how many times it tried to come online, and how many of those failed.
type Client struct {
	MAC       string           `json:"mac"`
	FirstSeen time.Time        `json:"first_seen"`
	LastSeen  time.Time        `json:"last_seen"`
	AP        hierarchy.NodeID `json:"ap"`
	Network   string           `json:"network"`
	SSID      string           `json:"ssid"`
	Host      string           `json:"host"`
	// The name a person gave it (0153), where one did.
	Name        string `json:"name,omitempty"`
	User        string `json:"user"`
	Address     string `json:"address"`
	Attempts    int    `json:"attempts"`
	Failed      int    `json:"failed"`
	LastOutcome string `json:"last_outcome"`
}

// ClientSeen is a client on an AP at a state report (0066).
type ClientSeen struct {
	MAC, Network, SSID, Host, User, Address string
}

// seeClient is one statement for both ways a client is seen: where it is
// newer than what is kept, the place it was seen; what it said of itself
// wherever it said anything; the count of its attempts; and how its latest
// attempt went, which a state report, seeing it later, leaves as it is.
const seeClient = `
INSERT INTO clients (mac, first_seen, last_seen, ap, network, ssid, host, user, address, attempts, failed, last_attempt, last_outcome)
VALUES (?1, ?2, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
ON CONFLICT (mac) DO UPDATE SET
	first_seen = min(first_seen, excluded.first_seen),
	last_seen = max(last_seen, excluded.last_seen),
	ap = CASE WHEN excluded.last_seen >= last_seen THEN excluded.ap ELSE ap END,
	network = CASE WHEN excluded.last_seen >= last_seen THEN excluded.network ELSE network END,
	ssid = CASE WHEN excluded.last_seen >= last_seen THEN excluded.ssid ELSE ssid END,
	host = CASE WHEN excluded.host != '' THEN excluded.host ELSE host END,
	user = CASE WHEN excluded.user != '' THEN excluded.user ELSE user END,
	address = CASE WHEN excluded.address != '' THEN excluded.address ELSE address END,
	attempts = attempts + excluded.attempts,
	failed = failed + excluded.failed,
	last_attempt = max(last_attempt, excluded.last_attempt),
	last_outcome = CASE WHEN excluded.last_attempt > 0 AND excluded.last_attempt >= last_attempt THEN excluded.last_outcome ELSE last_outcome END`

// RecordConnections keeps the attempts an AP reports, and counts each on
// its client. One it has reported before, by its client and start, is not
// kept twice. It returns how many were new.
func (s *Store) RecordConnections(ap hierarchy.NodeID, list []Connection) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, c := range list {
		at := c.Started.UnixMilli()
		said := c.Said
		if said == 0 {
			said = at
		}
		res, err := tx.Exec(`INSERT OR IGNORE INTO connections (ap, mac, started, at, network, ssid, band, outcome, stage, reason, took_ms, record)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(ap), c.MAC, said, at, c.Network, c.SSID, c.Band, c.Outcome, c.Stage, c.Reason, c.TookMS, string(c.Record))
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		added++
		failed := 0
		if c.Outcome == "failed" {
			failed = 1
		}
		if _, err := tx.Exec(seeClient, c.MAC, at, string(ap), c.Network, c.SSID, c.Host, "", c.Address, 1, failed, at, c.Outcome); err != nil {
			return 0, err
		}
	}
	return added, tx.Commit()
}

// SeeClients notes the clients on an AP at a state report: where each is,
// and what it says of itself. A client seen this way and never trying to
// come online, as one already on when recording began, is kept too.
func (s *Store) SeeClients(ap hierarchy.NodeID, list []ClientSeen) error {
	if len(list) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := s.now().UnixMilli()
	for _, c := range list {
		if _, err := tx.Exec(seeClient, c.MAC, at, string(ap), c.Network, c.SSID, c.Host, c.User, c.Address, 0, 0, 0, ""); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// moveClient puts a client on the AP whose minute says it sent it something
// (0251), from then, where it is on the network it was last seen on. One
// seen for the first time, or on another network, waits for its AP's
// report, which says the rest of it.
const moveClient = `
UPDATE clients SET
	ap = CASE WHEN ?2 >= last_seen THEN ?3 ELSE ap END,
	last_seen = max(last_seen, ?2)
WHERE mac = ?1 AND ssid = ?4 AND ?4 != ''`

// ClientsActive takes in an AP's clients' minutes (0251): each that sent the
// AP anything in its minute was there then, at the minute's start and as
// many seconds as it was there.
func (s *Store) ClientsActive(ap hierarchy.NodeID, list []ClientMinute) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range list {
		if c.RxPackets <= 0 {
			continue
		}
		at := c.At.Add(time.Duration(c.Secs) * time.Second).UnixMilli()
		if _, err := tx.Exec(moveClient, c.MAC, at, string(ap), c.SSID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClientsJoined takes in the clients an AP said it let on (0252), each at
// the moment it was.
func (s *Store) ClientsJoined(ap hierarchy.NodeID, list []ClientMinute) error {
	if len(list) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range list {
		if _, err := tx.Exec(moveClient, c.MAC, c.At.UnixMilli(), string(ap), c.SSID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Clients lists the clients last seen on one of aps, the latest first, that
// match find where it is not empty: part of a MAC, a host name, the name a
// person gave it (0153), a user or an address. It returns at most limit
// from offset, and how many there are in all.
func (s *Store) Clients(aps []hierarchy.NodeID, find string, limit, offset int) ([]Client, int, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, 0, err
	}
	const where = ` FROM clients c LEFT JOIN client_names n ON n.mac = c.mac WHERE c.ap IN (SELECT value FROM json_each(?1))
		AND (?2 = '' OR instr(c.mac, ?2) > 0 OR instr(lower(c.host), ?2) > 0 OR instr(n.folded, ?2) > 0 OR instr(lower(c.user), ?2) > 0 OR instr(c.address, ?2) > 0)`
	var total int
	if err := s.db.QueryRow(`SELECT count(*)`+where, string(ids), find).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT c.mac, c.first_seen, c.last_seen, c.ap, c.network, c.ssid, c.host, coalesce(n.name, ''), c.user, c.address, c.attempts, c.failed, c.last_outcome`+where+
		` ORDER BY c.last_seen DESC, c.mac LIMIT ?3 OFFSET ?4`, string(ids), find, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		var c Client
		var first, last int64
		var ap string
		if err := rows.Scan(&c.MAC, &first, &last, &ap, &c.Network, &c.SSID, &c.Host, &c.Name, &c.User, &c.Address, &c.Attempts, &c.Failed, &c.LastOutcome); err != nil {
			return nil, 0, err
		}
		c.AP, c.FirstSeen, c.LastSeen = hierarchy.NodeID(ap), time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// ClientOf is one client's row, and whether there is one.
func (s *Store) ClientOf(mac string) (Client, bool, error) {
	var c Client
	var first, last int64
	var ap string
	err := s.db.QueryRow(`SELECT c.mac, c.first_seen, c.last_seen, c.ap, c.network, c.ssid, c.host, coalesce(n.name, ''), c.user, c.address, c.attempts, c.failed, c.last_outcome
		FROM clients c LEFT JOIN client_names n ON n.mac = c.mac WHERE c.mac = ?`, mac).
		Scan(&c.MAC, &first, &last, &ap, &c.Network, &c.SSID, &c.Host, &c.Name, &c.User, &c.Address, &c.Attempts, &c.Failed, &c.LastOutcome)
	if err == sql.ErrNoRows {
		return Client{}, false, nil
	}
	if err != nil {
		return Client{}, false, err
	}
	c.AP, c.FirstSeen, c.LastSeen = hierarchy.NodeID(ap), time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
	return c, true, nil
}

// Connections lists a client's attempts on aps, the latest first: at most
// limit, and only those begun before before, where it is not zero.
func (s *Store) Connections(mac string, aps []hierarchy.NodeID, limit int, before time.Time) ([]Connection, error) {
	return s.ConnectionsWhere(ConnectionQuery{MAC: mac, APs: aps, Limit: limit, Before: before})
}

// ConnectionQuery picks one client's attempts: on aps, at most Limit,
// before Before and from Since where they are set, and of one Outcome where
// it is (0330: its failures alone).
type ConnectionQuery struct {
	MAC     string
	APs     []hierarchy.NodeID
	Limit   int
	Before  time.Time
	Since   time.Time
	Outcome string
}

// ConnectionsWhere is the attempts q picks, the latest first, each with
// the capture kept of it, where one is.
func (s *Store) ConnectionsWhere(q ConnectionQuery) ([]Connection, error) {
	ids, err := json.Marshal(q.APs)
	if err != nil {
		return nil, err
	}
	cut := int64(1) << 62
	if !q.Before.IsZero() {
		cut = q.Before.UnixMilli()
	}
	var from int64
	if !q.Since.IsZero() {
		from = q.Since.UnixMilli()
	}
	rows, err := s.db.Query(`SELECT c.id, c.ap, c.mac, c.at, c.network, c.ssid, c.band, c.outcome, c.stage, c.reason, c.took_ms, c.record,
			k.frames, length(k.file)
		FROM connections c LEFT JOIN captures k ON k.ap = c.ap AND k.mac = c.mac AND k.started = c.started
		WHERE c.mac = ? AND c.at < ? AND c.at >= ? AND (? = '' OR c.outcome = ?) AND c.ap IN (SELECT value FROM json_each(?))
		ORDER BY c.at DESC, c.id DESC LIMIT ?`, q.MAC, cut, from, q.Outcome, q.Outcome, string(ids), q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		var ap, record string
		var started int64
		var frames, size sql.NullInt64
		if err := rows.Scan(&c.ID, &ap, &c.MAC, &started, &c.Network, &c.SSID, &c.Band, &c.Outcome, &c.Stage, &c.Reason, &c.TookMS, &record, &frames, &size); err != nil {
			return nil, err
		}
		c.AP, c.Started, c.Record = hierarchy.NodeID(ap), time.UnixMilli(started).UTC(), json.RawMessage(record)
		if frames.Valid {
			c.Capture = &CaptureKept{Frames: int(frames.Int64), Bytes: int(size.Int64)}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// connectionRows reads the attempts a query found, of id, ap, mac, at,
// network, ssid, band, outcome, stage, reason, took_ms and record in that
// order, and closes its rows.
func connectionRows(rows *sql.Rows) ([]Connection, error) {
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		var ap, record string
		var started int64
		if err := rows.Scan(&c.ID, &ap, &c.MAC, &started, &c.Network, &c.SSID, &c.Band, &c.Outcome, &c.Stage, &c.Reason, &c.TookMS, &record); err != nil {
			return nil, err
		}
		c.AP, c.Started, c.Record = hierarchy.NodeID(ap), time.UnixMilli(started).UTC(), json.RawMessage(record)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ConnectionsOfClients lists the attempts of the clients named, on any AP,
// begun since a time, the latest first, at most limit: where each was
// before it roamed (0136).
func (s *Store) ConnectionsOfClients(macs []string, since time.Time, limit int) ([]Connection, error) {
	if macs == nil {
		macs = []string{}
	}
	list, err := json.Marshal(macs)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, ap, mac, at, network, ssid, band, outcome, stage, reason, took_ms, record FROM connections
		WHERE mac IN (SELECT value FROM json_each(?)) AND at >= ? ORDER BY at DESC, id DESC LIMIT ?`, string(list), since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	return connectionRows(rows)
}

// ConnectionsBetween lists a client's attempts, on any AP, begun from a time
// up to another, the earliest first, at most limit: what it did around a
// moment, as where it went after it left (0258).
func (s *Store) ConnectionsBetween(mac string, from, to time.Time, limit int) ([]Connection, error) {
	rows, err := s.db.Query(`SELECT id, ap, mac, at, network, ssid, band, outcome, stage, reason, took_ms, record FROM connections
		WHERE mac = ? AND at >= ? AND at <= ? ORDER BY at, id LIMIT ?`, mac, from.UnixMilli(), to.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	return connectionRows(rows)
}

// letOn is an attempt at which its client was let on to its network: its
// record says when, or it came online, was connected, failed at something
// that comes after, or left, and not before it was let on.
const letOn = `(outcome IN ('online', 'connected') OR (outcome = 'failed' AND stage IN ('dhcp', 'gateway', 'dns', 'internet'))
	OR (outcome = 'left' AND reason NOT LIKE 'before it was let on%')
	OR json_extract(record, '$.how.on_ms') IS NOT NULL)`

// LastLetOn is the latest attempt of a client, on any AP, begun before a
// time, at which it was let on to its network (letOn).
// Where a client was before it roamed (0136), however long before.
func (s *Store) LastLetOn(mac string, before time.Time) (Connection, bool, error) {
	var c Connection
	var ap, record string
	var started int64
	err := s.db.QueryRow(`SELECT id, ap, mac, at, network, ssid, band, outcome, stage, reason, took_ms, record FROM connections
		WHERE mac = ? AND at < ? AND `+letOn+`
		ORDER BY at DESC, id DESC LIMIT 1`, mac, before.UnixMilli()).
		Scan(&c.ID, &ap, &c.MAC, &started, &c.Network, &c.SSID, &c.Band, &c.Outcome, &c.Stage, &c.Reason, &c.TookMS, &record)
	if err == sql.ErrNoRows {
		return Connection{}, false, nil
	}
	if err != nil {
		return Connection{}, false, err
	}
	c.AP, c.Started, c.Record = hierarchy.NodeID(ap), time.UnixMilli(started).UTC(), json.RawMessage(record)
	return c, true, nil
}

// ConnectionsSince lists the attempts begun since a time on aps, but for
// those of the clients not names, the latest first, at most limit: for what
// they come to together, a step at a time (0130). They are found by their
// time, and then kept or not by their AP (the + keeps SQLite from finding
// them by AP, every one an AP has kept, to sort them after).
func (s *Store) ConnectionsSince(since time.Time, aps []hierarchy.NodeID, not []string, limit int) ([]Connection, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	if not == nil {
		not = []string{}
	}
	macs, err := json.Marshal(not)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, ap, mac, at, network, ssid, band, outcome, stage, reason, took_ms, record FROM connections
		WHERE at >= ? AND +ap IN (SELECT value FROM json_each(?)) AND mac NOT IN (SELECT value FROM json_each(?))
		ORDER BY at DESC, id DESC LIMIT ?`, since.UnixMilli(), string(ids), string(macs), limit)
	if err != nil {
		return nil, err
	}
	return connectionRows(rows)
}

// Join is how one client's attempts to come online on one network of one
// AP went since a time (0127): how many it made; how many got it on, online
// or let on and seen no further; how many of those were seen online, and
// how long they took to be, in ms, added up; how many failed; and when the
// latest of each began, the zero time where there was none: the latest that
// got it on, the latest that failed, and the latest of any.
type Join struct {
	AP       hierarchy.NodeID
	Network  string
	SSID     string
	MAC      string
	Attempts int
	Online   int
	Seen     int
	SeenMS   int64
	Failed   int
	// The first and the latest of its attempts that came online, or were
	// let on (0190).
	FirstOn    time.Time
	LastOn     time.Time
	LastFailed time.Time
	Last       time.Time
}

// JoinStop is where one client's failed attempts on one network of one AP
// stopped since a time, and why, as its AP put it: how many did, and when
// the first and the latest of them began.
type JoinStop struct {
	AP       hierarchy.NodeID
	Network  string
	SSID     string
	MAC      string
	Stage    string
	Reason   string
	Attempts int
	First    time.Time
	Last     time.Time
}

// Joins is how the clients' attempts to come online went since a time: a
// row a client, network and AP; and where those that failed stopped. An
// attempt a client left of its own accord (0118) is counted as an attempt
// and as nothing more.
func (s *Store) Joins(since time.Time) ([]Join, []JoinStop, error) {
	rows, err := s.db.Query(`SELECT ap, network, ssid, mac, COUNT(*),
			SUM(outcome IN ('online', 'connected')), SUM(outcome = 'online'),
			SUM(CASE WHEN outcome = 'online' THEN took_ms ELSE 0 END), SUM(outcome = 'failed'),
			COALESCE(MIN(CASE WHEN outcome IN ('online', 'connected') THEN at END), 0),
			MAX(CASE WHEN outcome IN ('online', 'connected') THEN at ELSE 0 END),
			MAX(CASE WHEN outcome = 'failed' THEN at ELSE 0 END), MAX(at)
		FROM connections WHERE at >= ? GROUP BY ap, network, ssid, mac ORDER BY ap, network, ssid, mac`, since.UnixMilli())
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	joins := []Join{}
	for rows.Next() {
		var j Join
		var ap string
		var firstOn, on, failed, last int64
		if err := rows.Scan(&ap, &j.Network, &j.SSID, &j.MAC, &j.Attempts, &j.Online, &j.Seen, &j.SeenMS, &j.Failed, &firstOn, &on, &failed, &last); err != nil {
			return nil, nil, err
		}
		j.AP = hierarchy.NodeID(ap)
		when := func(ms int64) time.Time {
			if ms <= 0 {
				return time.Time{}
			}
			return time.UnixMilli(ms).UTC()
		}
		j.FirstOn, j.LastOn, j.LastFailed, j.Last = when(firstOn), when(on), when(failed), when(last)
		joins = append(joins, j)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	failed, err := s.db.Query(`SELECT ap, network, ssid, mac, stage, reason, COUNT(*), MIN(at), MAX(at)
		FROM connections WHERE at >= ? AND outcome = 'failed'
		GROUP BY ap, network, ssid, mac, stage, reason ORDER BY ap, network, ssid, mac, stage, reason`, since.UnixMilli())
	if err != nil {
		return nil, nil, err
	}
	defer failed.Close()
	stops := []JoinStop{}
	for failed.Next() {
		var j JoinStop
		var ap string
		var first, last int64
		if err := failed.Scan(&ap, &j.Network, &j.SSID, &j.MAC, &j.Stage, &j.Reason, &j.Attempts, &first, &last); err != nil {
			return nil, nil, err
		}
		j.AP, j.First, j.Last = hierarchy.NodeID(ap), time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
		stops = append(stops, j)
	}
	return joins, stops, failed.Err()
}

// JoinCount is how many times a client tried to come online on the APs
// asked of since a time, and how many of those failed: its joins over a
// time, not since it was first seen (0118); with the host name its row
// has, and the APs it tried.
type JoinCount struct {
	MAC      string
	Host     string
	Attempts int
	Failed   int
	APs      []hierarchy.NodeID
}

// JoinCounts is how often clients tried to come online on aps since a
// time: those in macs, or where macs is nil, those that tried most; at
// most limit of them, the most first. An attempt a client left of its own
// accord is counted, as in its row.
func (s *Store) JoinCounts(aps []hierarchy.NodeID, macs []string, since time.Time, limit int) ([]JoinCount, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	var only any
	if macs != nil {
		b, err := json.Marshal(macs)
		if err != nil {
			return nil, err
		}
		only = string(b)
	}
	rows, err := s.db.Query(`SELECT j.mac, coalesce(c.host, ''), COUNT(*), SUM(j.outcome = 'failed'), json_group_array(DISTINCT j.ap)
		FROM connections j LEFT JOIN clients c ON c.mac = j.mac
		WHERE j.at >= ?1 AND +j.ap IN (SELECT value FROM json_each(?2)) AND (?3 IS NULL OR j.mac IN (SELECT value FROM json_each(?3)))
		GROUP BY j.mac ORDER BY COUNT(*) DESC, j.mac LIMIT ?4`, since.UnixMilli(), string(ids), only, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JoinCount{}
	for rows.Next() {
		var j JoinCount
		var tried string
		if err := rows.Scan(&j.MAC, &j.Host, &j.Attempts, &j.Failed, &tried); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(tried), &j.APs); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Failure is one attempt to come online that failed (0166): on which AP,
// when it began, as the manager takes it, and whose.
type Failure struct {
	AP  hierarchy.NodeID
	At  time.Time
	MAC string
}

// Failures calls fn with each attempt to come online that failed since a
// time, oldest first: for the Clients overview's problems (0166).
func (s *Store) Failures(since time.Time, fn func(Failure) error) error {
	rows, err := s.db.Query(`SELECT ap, at, mac FROM connections WHERE at >= ? AND outcome = 'failed' ORDER BY at`, since.UnixMilli())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var f Failure
		var ap string
		var at int64
		if err := rows.Scan(&ap, &at, &f.MAC); err != nil {
			return err
		}
		f.AP, f.At = hierarchy.NodeID(ap), time.UnixMilli(at).UTC()
		if err := fn(f); err != nil {
			return err
		}
	}
	return rows.Err()
}

// TrimConnections deletes the attempts begun more than keep ago, and
// returns how many; and what happened to the APs themselves that long ago
// (0119), which is kept as long; and the captures that came in that long
// ago (0135), their attempts gone; and what the APs' steering asked of
// clients that long ago (0139). The clients themselves are kept.
func (s *Store) TrimConnections(keep time.Duration) (int64, error) {
	if _, err := s.db.Exec(`DELETE FROM steer_events WHERE at < ?`, s.now().Add(-keep).UnixMilli()); err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(`DELETE FROM ap_events WHERE at < ?`, s.now().Add(-keep).UnixMilli()); err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(`DELETE FROM captures WHERE at < ?`, s.now().Add(-keep).UnixMilli()); err != nil {
		return 0, err
	}
	// What the APs' Bluetooth radios heard (0158), kept as long.
	if err := s.trimBluetooth(s.now().Add(-keep)); err != nil {
		return 0, err
	}
	// The clients heard on rogues (0160), kept as long.
	if _, err := s.db.Exec(`DELETE FROM rogue_clients WHERE last_at < ?`, s.now().Add(-keep).UnixMilli()); err != nil {
		return 0, err
	}
	// What happened to clients at the APs (0255), kept as long.
	if _, err := s.db.Exec(`DELETE FROM client_events WHERE at < ?`, s.now().Add(-keep).UnixMilli()); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM connections WHERE at < ?`, s.now().Add(-keep).UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Joined is one client's attempts to come online at one AP on one band
// since a time (0193): how many, how many came online or were let on, how
// many failed, and the first and latest of them.
type Joined struct {
	AP       hierarchy.NodeID
	Band     string
	MAC      string
	Attempts int
	Online   int
	Failed   int
	First    time.Time
	Last     time.Time
}

// JoinedSince is each client's attempts at each AP on each band since a
// time (0193), for how clients of one maker fare on each kind of AP.
func (s *Store) JoinedSince(since time.Time) ([]Joined, error) {
	rows, err := s.db.Query(`SELECT ap, band, mac, COUNT(*), SUM(outcome IN ('online', 'connected')), SUM(outcome = 'failed'), MIN(at), MAX(at)
		FROM connections WHERE at >= ? GROUP BY ap, band, mac ORDER BY ap, band, mac`, since.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Joined{}
	for rows.Next() {
		var j Joined
		var ap string
		var first, last int64
		if err := rows.Scan(&ap, &j.Band, &j.MAC, &j.Attempts, &j.Online, &j.Failed, &first, &last); err != nil {
			return nil, err
		}
		j.AP, j.First, j.Last = hierarchy.NodeID(ap), time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
		out = append(out, j)
	}
	return out, rows.Err()
}
