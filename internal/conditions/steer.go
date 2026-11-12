package conditions

import (
	"database/sql"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Steering (0139): each asking an AP's steering made of a client (0138),
// kept as the AP said it. The AP's own count of them starts again whenever
// its steering does, as at every update of its agent, and its report holds
// the last fifty: kept here, what steering came to over a day or a month
// can be said. An AP says an asking in each report until it has fifty
// newer, so each is kept once, by when its AP said it was, of whom, by
// which way and what came of it. said is that time, in seconds, by the AP's
// clock; at is when the manager takes it to have been, in ms, as with an
// attempt (connectionsAt). A clock not yet set says the same times again
// each time its AP starts, so an asking said by one is told by the run of
// its AP as well: run is when that AP started, in seconds, by the manager's
// clock, and 0 for an asking whose time is its AP's word.
const steerTable = `
CREATE TABLE steer_events (
	ap      TEXT NOT NULL,
	said    INTEGER NOT NULL,
	at      INTEGER NOT NULL,
	mac     TEXT NOT NULL,
	network TEXT NOT NULL,
	method  TEXT NOT NULL,
	band    TEXT NOT NULL,
	bands   TEXT NOT NULL,
	asked   TEXT NOT NULL,
	answer  TEXT NOT NULL,
	code    INTEGER,
	signal  INTEGER,
	outcome TEXT NOT NULL,
	took_ms INTEGER NOT NULL,
	run     INTEGER NOT NULL,
	UNIQUE (ap, said, mac, method, outcome, run)
);
CREATE INDEX steer_events_at ON steer_events (at);`

// steerWhy keeps why a sticky asking was made (0289): weak, or better,
// where another AP heard the client more strongly, which better_ap names
// with how strongly it heard it, in dBm. Askings kept before say none.
const steerWhy = `
ALTER TABLE steer_events ADD COLUMN why TEXT NOT NULL DEFAULT '';
ALTER TABLE steer_events ADD COLUMN better_ap TEXT NOT NULL DEFAULT '';
ALTER TABLE steer_events ADD COLUMN better_signal INTEGER;`

// SteerEvent is one asking, at its end: of which client, on which network,
// by which way (suggest, hold or sticky) and from which band; what was
// asked, what the client answered, what came of it, and after how long;
// for sticky, why, and the AP that heard the client better, with how
// strongly (0289). At
// is when it was, as the manager takes it; Said is when its AP said, in
// seconds since 1970, which with the rest tells it from any other. Run is
// when its AP started, in seconds since 1970, where Said is the word of a
// clock not yet set; else 0.
type SteerEvent struct {
	AP      hierarchy.NodeID
	At      time.Time
	Said    int64
	Run     int64
	MAC     string
	Network string
	Method  string
	From    string
	To      []string
	Asked   string
	Answer  string
	Code    *int
	Signal  *int
	Outcome string
	TookMS  int
	Why     string
	Better  string
	BetterS *int
}

// SteerCount is how many askings on an AP, of a network's clients by one
// way, came to one thing.
type SteerCount struct {
	AP      hierarchy.NodeID
	Network string
	Method  string
	Outcome string
	N       int
}

// SteerClient is what was asked of one client: how many times, how many of
// those it moved for, how many it said no to, and how many times it came
// back; and when it was last asked.
type SteerClient struct {
	MAC     string
	Asked   int
	Moved   int
	Refused int
	Back    int
	Last    time.Time
}

// An asking is each record but the one that says a client moved up by a
// suggestion or a hold came back down: that is what became of an asking
// already counted. A sticky client that came straight back is its asking's
// own end; one that came back within ten minutes, its asking having ended
// as left a minute on (0289), says so in a record of its own, asking
// nothing, which is no asking either.
const steerAsking = `NOT (outcome = 'back' AND (method != 'sticky' OR asked = 'none'))`

// steerRun is how far apart two reckonings of when an AP started may be
// and be of the same run: each is the manager's time less the uptime the AP
// said, and so is out by as long as its report took to come.
const steerRun = time.Minute

// AddSteer keeps the askings an AP reported that are not yet kept, and
// says how many those were. One is kept already where its AP said the same
// before of the same time, and both times were its AP's word, or one of
// them was, or both were said in the same run of the AP.
func (s *Store) AddSteer(ap hierarchy.NodeID, events []SteerEvent) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, e := range events {
		mac, near := strings.ToLower(e.MAC), int64(steerRun/time.Second)
		res, err := tx.Exec(`INSERT OR IGNORE INTO steer_events (ap, said, at, mac, network, method, band, bands, asked, answer, code, signal, outcome, took_ms, run, why, better_ap, better_signal)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (SELECT 1 FROM steer_events WHERE ap = ? AND said = ? AND mac = ? AND method = ? AND outcome = ?
				AND (run = 0 OR ? = 0 OR run BETWEEN ? AND ?))`,
			string(ap), e.Said, e.At.UnixMilli(), mac, e.Network, e.Method, e.From, strings.Join(e.To, ","), e.Asked, e.Answer, e.Code, e.Signal, e.Outcome, e.TookMS, e.Run,
			e.Why, e.Better, e.BetterS,
			string(ap), e.Said, mac, e.Method, e.Outcome, e.Run, e.Run-near, e.Run+near)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	return added, tx.Commit()
}

// steerWhere is the askings since a time on aps, but for those of the
// clients in not: found by their time, and then kept or not by their AP,
// as attempts are (ConnectionsSince).
const steerWhere = ` FROM steer_events WHERE at >= ? AND +ap IN (SELECT value FROM json_each(?)) AND mac NOT IN (SELECT value FROM json_each(?))`

func steerArgs(since time.Time, aps []hierarchy.NodeID, not []string) ([]any, error) {
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
	return []any{since.UnixMilli(), string(ids), string(macs)}, nil
}

// SteerSince lists the askings since a time on aps, but for those of the
// clients in not, the latest first, at most limit.
func (s *Store) SteerSince(since time.Time, aps []hierarchy.NodeID, not []string, limit int) ([]SteerEvent, error) {
	args, err := steerArgs(since, aps, not)
	if err != nil {
		return nil, err
	}
	return s.steers(`SELECT `+steerCols+steerWhere+`
		ORDER BY at DESC, rowid DESC LIMIT ?`, append(args, limit)...)
}

// steerCols are an asking's columns, as steers reads them.
const steerCols = `ap, said, at, mac, network, method, band, bands, asked, answer, code, signal, outcome, took_ms, run, why, better_ap, better_signal`

// steers reads askings, of steerCols.
func (s *Store) steers(q string, args ...any) ([]SteerEvent, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SteerEvent{}
	for rows.Next() {
		var e SteerEvent
		var ap, bands string
		var at int64
		if err := rows.Scan(&ap, &e.Said, &at, &e.MAC, &e.Network, &e.Method, &e.From, &bands, &e.Asked, &e.Answer, &e.Code, &e.Signal, &e.Outcome, &e.TookMS, &e.Run,
			&e.Why, &e.Better, &e.BetterS); err != nil {
			return nil, err
		}
		e.AP, e.At = hierarchy.NodeID(ap), time.UnixMilli(at).UTC()
		if bands != "" {
			e.To = strings.Split(bands, ",")
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SteerCounts counts the askings since a time on aps, but for those of the
// clients in not: for each AP, network and way, how many came to each
// thing. The count under the outcome "asked" is how many askings there
// were, as an AP's own count has it (0138).
func (s *Store) SteerCounts(since time.Time, aps []hierarchy.NodeID, not []string) ([]SteerCount, error) {
	args, err := steerArgs(since, aps, not)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT ap, network, method, outcome, count(*), sum(`+steerAsking+`)`+steerWhere+`
		GROUP BY ap, network, method, outcome ORDER BY ap, network, method, outcome`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SteerCount{}
	asked := map[[3]string]int{}
	var order [][3]string
	for rows.Next() {
		var c SteerCount
		var ap string
		var askings int
		if err := rows.Scan(&ap, &c.Network, &c.Method, &c.Outcome, &c.N, &askings); err != nil {
			return nil, err
		}
		c.AP = hierarchy.NodeID(ap)
		out = append(out, c)
		k := [3]string{ap, c.Network, c.Method}
		if _, ok := asked[k]; !ok {
			order = append(order, k)
		}
		asked[k] += askings
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, k := range order {
		out = append(out, SteerCount{AP: hierarchy.NodeID(k[0]), Network: k[1], Method: k[2], Outcome: "asked", N: asked[k]})
	}
	return out, nil
}

// SteerClients lists the clients asked since a time on aps, but for those
// in not: those asked most first, at most limit. One of whom the time holds
// only a coming back, its asking from before, was not asked in it and is
// not listed; and a coming back is no time a client was last asked.
func (s *Store) SteerClients(since time.Time, aps []hierarchy.NodeID, not []string, limit int) ([]SteerClient, error) {
	args, err := steerArgs(since, aps, not)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT mac, sum(`+steerAsking+`) AS askings, sum(outcome = 'moved'), sum(outcome = 'refused'), sum(outcome = 'back'), max(CASE WHEN `+steerAsking+` THEN at END) AS last`+steerWhere+`
		GROUP BY mac HAVING askings > 0 ORDER BY askings DESC, last DESC, mac LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SteerClient{}
	for rows.Next() {
		var c SteerClient
		var last int64
		if err := rows.Scan(&c.MAC, &c.Asked, &c.Moved, &c.Refused, &c.Back, &last); err != nil {
			return nil, err
		}
		c.Last = time.UnixMilli(last).UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

// Hosts is the host name each of the clients named last gave, of those
// last seen on one of aps and that gave one.
func (s *Store) Hosts(macs []string, aps []hierarchy.NodeID) (map[string]string, error) {
	out := map[string]string{}
	if len(macs) == 0 {
		return out, nil
	}
	of, err := json.Marshal(macs)
	if err != nil {
		return nil, err
	}
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT mac, host FROM clients WHERE host != '' AND mac IN (SELECT value FROM json_each(?)) AND ap IN (SELECT value FROM json_each(?))`, string(of), string(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var mac, host string
		if err := rows.Scan(&mac, &host); err != nil {
			return nil, err
		}
		out[mac] = host
	}
	return out, rows.Err()
}

// SteerAfter is where a client was let on next after an asking (0291):
// the AP and band of its first attempt begun after after and up to to at
// which it was let on (letOn), when, and how strongly that AP heard it: by
// the attempt's record, which says the signal it joined at, or else by its
// first minute there (0207) up to minuteLate after to. Signal is nil where
// neither says.
type SteerAfter struct {
	AP     hierarchy.NodeID
	Band   string
	At     time.Time
	Signal *int
}

// minuteLate is how long after an asking's look ends its client's first
// minute where it went may still come: an AP says its minutes a minute at
// a time, and the minute it joined in ends after it joined.
const minuteLate = 2 * time.Minute

// SteerAfter is where a client was let on next, after a time and up to
// another, and how strongly it was heard there; false where it was let on
// nowhere then. Each is one look by an index (connections by MAC and time,
// client_minutes by MAC and minute), and at most one row.
func (s *Store) SteerAfter(mac string, after, to time.Time) (SteerAfter, bool, error) {
	var a SteerAfter
	var ap string
	var at int64
	var sig sql.NullFloat64
	err := s.db.QueryRow(`SELECT ap, band, at, json_extract(record, '$.signal') FROM connections
		WHERE mac = ? AND at > ? AND at <= ? AND `+letOn+`
		ORDER BY at, id LIMIT 1`, mac, after.UnixMilli(), to.UnixMilli()).Scan(&ap, &a.Band, &at, &sig)
	if err == sql.ErrNoRows {
		return SteerAfter{}, false, nil
	}
	if err != nil {
		return SteerAfter{}, false, err
	}
	a.AP, a.At = hierarchy.NodeID(ap), time.UnixMilli(at).UTC()
	heard := func(v float64) *int {
		n := int(math.Round(v))
		if n < -127 || n >= 0 {
			return nil
		}
		return &n
	}
	if sig.Valid {
		a.Signal = heard(sig.Float64)
	}
	if a.Signal == nil {
		var m sql.NullFloat64
		err := s.db.QueryRow(`SELECT signal FROM client_minutes WHERE mac = ? AND at >= ? AND at <= ? AND ap = ? AND band = ? AND signal IS NOT NULL
			ORDER BY at LIMIT 1`, mac, stamp(a.At.Add(-time.Minute)), stamp(to.Add(minuteLate)), ap, a.Band).Scan(&m)
		if err != nil && err != sql.ErrNoRows {
			return SteerAfter{}, false, err
		}
		if m.Valid {
			a.Signal = heard(m.Float64)
		}
	}
	return a, true, nil
}
