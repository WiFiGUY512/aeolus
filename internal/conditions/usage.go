package conditions

import (
	"database/sql"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Usage (0108): how many Wi-Fi clients an AP had at each state report, and
// how much they moved since the report before, which the manager works out
// as the report comes. A row a report, so a day's chart reads a few
// hundred rows, not a day of reports.
const usageTable = `
CREATE TABLE usage (
	ap      TEXT NOT NULL,
	at      TEXT NOT NULL,
	clients INTEGER NOT NULL,
	down    INTEGER NOT NULL,
	up      INTEGER NOT NULL
);
CREATE INDEX usage_at ON usage (at);`

// usageBands keeps an AP's clients on each band too (0166): 2.4, 5 and
// 6 GHz, at each report and the most at a report in each hour. Rows from
// before have none.
const usageBands = `
ALTER TABLE usage ADD COLUMN c2g INTEGER;
ALTER TABLE usage ADD COLUMN c5g INTEGER;
ALTER TABLE usage ADD COLUMN c6g INTEGER;
ALTER TABLE usage_hours ADD COLUMN c2g INTEGER;
ALTER TABLE usage_hours ADD COLUMN c5g INTEGER;
ALTER TABLE usage_hours ADD COLUMN c6g INTEGER;`

// Bands are the bands an AP's clients are counted on (0166), in the order
// of their columns.
var Bands = []string{"2g", "5g", "6g"}

// bandCounts are a report's clients on each band, as columns take them:
// none where the report said nothing of bands.
func bandCounts(bands map[string]int) []any {
	out := make([]any, len(Bands))
	if bands == nil {
		return out
	}
	for i, b := range Bands {
		out[i] = bands[b]
	}
	return out
}

// readBands is a row's clients on each band, or nil where it has none.
func readBands(cols []sql.NullInt64) map[string]int {
	var out map[string]int
	for i, c := range cols {
		if c.Valid {
			if out == nil {
				out = map[string]int{}
			}
			out[Bands[i]] = int(c.Int64)
		}
	}
	return out
}

// clientUsageTable keeps what each client moved at each report (0110), a
// row for each that moved anything.
const clientUsageTable = `
CREATE TABLE client_usage (
	ap   TEXT NOT NULL,
	at   TEXT NOT NULL,
	mac  TEXT NOT NULL,
	host TEXT NOT NULL,
	down INTEGER NOT NULL,
	up   INTEGER NOT NULL
);
CREATE INDEX client_usage_at ON client_usage (at);`

// clientUsageByClient lets the host name a client last gave on an AP be
// found at once, not by reading back through the whole table.
const clientUsageByClient = `CREATE INDEX client_usage_client ON client_usage (ap, mac, at);`

// clientUsageNamed is the rows in which a client gave a host name, by
// client, so its last is found at once: by clientUsageByClient alone, one
// that never gave one was read back through every row it ever had.
const clientUsageNamed = `CREATE INDEX client_usage_named ON client_usage (ap, mac, at) WHERE host != '';`

// ClientUse is what one client moved since its AP's last report (0110),
// with the host name it gave in DHCP, if any.
type ClientUse struct {
	MAC  string
	Host string
	Down int64
	Up   int64
}

// ClientTotal is what one client moved on one AP since a time, with the
// host name it last gave there, when, and in which row: of two names given
// at one time, the later row's was given last. Totals by day keep no row,
// and their HostRow is 0.
type ClientTotal struct {
	AP      hierarchy.NodeID
	MAC     string
	Host    string
	HostAt  time.Time
	HostRow int64
	Down    int64
	Up      int64
}

// Moved says whether an AP reported, between two times, that a client moved
// anything through it (0110): the client was on that AP then. For whether
// its attempt at another was a roam (0136).
func (s *Store) Moved(ap hierarchy.NodeID, mac string, from, to time.Time) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM client_usage WHERE ap = ? AND mac = ? AND at >= ? AND at < ?)`,
		string(ap), mac, stamp(from), stamp(to)).Scan(&n)
	return n != 0, err
}

// Use is one AP's clients at a report, and what they moved since its last:
// Down is what the AP sent them, Up what it received from them, in bytes.
type Use struct {
	AP      hierarchy.NodeID
	At      time.Time
	Clients int
	Down    int64
	Up      int64
	// Bands is its clients on each band (0166), nil where the row has none.
	Bands map[string]int
}

// RecordUse records an AP's clients at a report and what each moved, in
// one transaction: the AP's row, its traffic the sum of theirs, and a row
// for each client (0110); and counts them into the AP's hour and each
// client's day (0140).
func (s *Store) RecordUse(ap hierarchy.NodeID, clients int, bands map[string]int, moved []ClientUse) error {
	now := s.now()
	at := stamp(now)
	var down, up int64
	for _, c := range moved {
		down += c.Down
		up += c.Up
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	by := bandCounts(bands)
	if _, err := tx.Exec(`INSERT INTO usage (ap, at, clients, down, up, c2g, c5g, c6g) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		append([]any{string(ap), at, clients, down, up}, by...)...); err != nil {
		return err
	}
	for _, c := range moved {
		if _, err := tx.Exec(`INSERT INTO client_usage (ap, at, mac, host, down, up) VALUES (?, ?, ?, ?, ?, ?)`, string(ap), at, c.MAC, c.Host, c.Down, c.Up); err != nil {
			return err
		}
	}
	if err := addUse(tx, ap, now, clients, by, down, up, moved); err != nil {
		return err
	}
	return tx.Commit()
}

// clientNamed finds the row in which a client last gave a host name on an
// AP: the newest, and of those of one time the last recorded, as two
// reports within one tick of a coarse clock have the same time.
const clientNamed = ` FROM client_usage h WHERE h.ap = c.ap AND h.mac = c.mac AND h.host != '' ORDER BY h.at DESC, h.rowid DESC LIMIT 1`

// clientTotals is the SQL of ClientTotals. Its rows are read by time, the
// span's alone: by client, as the grouping would have it, every row kept is
// read.
const clientTotals = `SELECT c.ap, c.mac, SUM(c.down), SUM(c.up),
	(SELECT h.host` + clientNamed + `), (SELECT h.at` + clientNamed + `), (SELECT h.rowid` + clientNamed + `)
	FROM client_usage c INDEXED BY client_usage_at WHERE c.at >= ? GROUP BY c.ap, c.mac`

// ClientTotals is what each client moved on each AP since since (0110),
// with the last host name it gave there.
func (s *Store) ClientTotals(since time.Time) ([]ClientTotal, error) {
	rows, err := s.db.Query(clientTotals, stamp(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientTotal
	for rows.Next() {
		var t ClientTotal
		var ap string
		var host, at sql.NullString
		var row sql.NullInt64
		if err := rows.Scan(&ap, &t.MAC, &t.Down, &t.Up, &host, &at, &row); err != nil {
			return nil, err
		}
		t.AP, t.Host, t.HostRow = hierarchy.NodeID(ap), host.String, row.Int64
		if at.Valid {
			if t.HostAt, err = time.Parse(time.RFC3339Nano, at.String); err != nil {
				return nil, err
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UsageFrom is when the oldest usage row kept was recorded, and false when
// there is none: before it, nothing says whether an AP reported.
func (s *Store) UsageFrom() (time.Time, bool, error) {
	var at sql.NullString
	if err := s.db.QueryRow(`SELECT MIN(at) FROM usage`).Scan(&at); err != nil || !at.Valid {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339Nano, at.String)
	return t, err == nil, err
}

// Uses calls fn with every AP's use since since, oldest first.
func (s *Store) Uses(since time.Time, fn func(Use) error) error {
	rows, err := s.db.Query(`SELECT ap, at, clients, down, up, c2g, c5g, c6g FROM usage WHERE at >= ? ORDER BY at`, stamp(since))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var u Use
		var ap, at string
		by := make([]sql.NullInt64, len(Bands))
		if err := rows.Scan(&ap, &at, &u.Clients, &u.Down, &u.Up, &by[0], &by[1], &by[2]); err != nil {
			return err
		}
		u.Bands = readBands(by)
		if u.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return err
		}
		u.AP = hierarchy.NodeID(ap)
		if err := fn(u); err != nil {
			return err
		}
	}
	return rows.Err()
}
