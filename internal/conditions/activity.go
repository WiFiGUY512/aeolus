package conditions

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// An AP's Activity (0338) is a page of what it did and what was done to
// it, newest first, from before a time, read back a page at a time as it
// is scrolled. Each of these reads one of its sources: of one AP, before
// before, the newest first, at most limit. A time at or after before is
// left to the page that read it.

// EventsBefore is what happened to an AP before before, of the kinds
// given, or of every kind where none are.
func (s *Store) EventsBefore(ap hierarchy.NodeID, before time.Time, kinds []string, limit int) ([]APEvent, error) {
	q := `SELECT id, at, kind, text, detail FROM ap_events WHERE ap = ? AND at < ?`
	args := []any{string(ap), before.UnixMilli()}
	if len(kinds) > 0 {
		raw, err := json.Marshal(kinds)
		if err != nil {
			return nil, err
		}
		q += ` AND kind IN (SELECT value FROM json_each(?))`
		args = append(args, string(raw))
	}
	rows, err := s.db.Query(q+` ORDER BY at DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APEvent{}
	for rows.Next() {
		var e APEvent
		var at int64
		var detail string
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.Text, &detail); err != nil {
			return nil, err
		}
		e.At = time.UnixMilli(at).UTC()
		if detail != "" {
			e.Detail = json.RawMessage(detail)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AppliedBefore is an AP's applies before before, with their IDs. An apply
// is kept as recorded, so its time is compared as stamped.
func (s *Store) AppliedBefore(ap hierarchy.NodeID, before time.Time, limit int) ([]int64, []Apply, error) {
	rows, err := s.db.Query(`SELECT id, at, version, hash, ok, error, checked FROM applies WHERE ap = ? AND `+stamped("at")+` < ? ORDER BY id DESC LIMIT ?`, string(ap), stamp(before), limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ids []int64
	out := []Apply{}
	for rows.Next() {
		var a Apply
		var id int64
		var at string
		if err := rows.Scan(&id, &at, &a.Version, &a.Hash, &a.OK, &a.Error, &a.Checked); err != nil {
			return nil, nil, err
		}
		if a.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		out = append(out, a)
	}
	return ids, out, rows.Err()
}

// AlertsBegunBefore is an AP's alerts that began before before; and
// AlertsEndedBefore, those that ended before it, by when they ended.
func (s *Store) AlertsBegunBefore(ap hierarchy.NodeID, before time.Time, limit int) ([]LoggedAlert, error) {
	return s.loggedAlerts(`SELECT id, ap, key, kind, severity, short, message, began, ended FROM alert_log
		WHERE ap = ? AND began < ? ORDER BY began DESC, id DESC LIMIT ?`, string(ap), stamp(before), limit)
}

func (s *Store) AlertsEndedBefore(ap hierarchy.NodeID, before time.Time, limit int) ([]LoggedAlert, error) {
	return s.loggedAlerts(`SELECT id, ap, key, kind, severity, short, message, began, ended FROM alert_log
		WHERE ap = ? AND ended IS NOT NULL AND ended < ? ORDER BY ended DESC, id DESC LIMIT ?`, string(ap), stamp(before), limit)
}

// ActionsBefore is what an AP was asked to do before before, of the kinds
// given, or of every kind but those in not.
func (s *Store) ActionsBefore(ap hierarchy.NodeID, before time.Time, kinds, not []string, limit int) ([]Action, error) {
	in, err := json.Marshal(kinds)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(not)
	if err != nil {
		return nil, err
	}
	return s.actions(`SELECT `+actionCols+` FROM actions WHERE ap = ? AND at < ?
		AND (json_array_length(?) = 0 OR kind IN (SELECT value FROM json_each(?)))
		AND kind NOT IN (SELECT value FROM json_each(?))
		ORDER BY at DESC, id DESC LIMIT ?`, string(ap), stamp(before), string(in), string(in), string(out), limit)
}

// TestsOfBefore is the client tests made of an AP from another, before
// before.
func (s *Store) TestsOfBefore(ap hierarchy.NodeID, before time.Time, limit int) ([]Action, error) {
	return s.actions(`SELECT `+actionCols+` FROM actions WHERE kind = 'client-test' AND `+testOn+` = ? AND at < ?
		ORDER BY at DESC, id DESC LIMIT ?`, string(ap), stamp(before), limit)
}

// APCBefore is an AP's power steps before before, with their IDs.
func (s *Store) APCBefore(ap hierarchy.NodeID, before time.Time, limit int) ([]int64, []APCEvent, error) {
	rows, err := s.db.Query(`SELECT id, at, band, from_dbm, to_dbm, why, scanning, left_n, ratio FROM apc_events
		WHERE ap = ? AND at < ? ORDER BY at DESC, id DESC LIMIT ?`, string(ap), stamp(before), limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ids []int64
	out := []APCEvent{}
	for rows.Next() {
		var e APCEvent
		var id int64
		var at, ratio string
		if err := rows.Scan(&id, &at, &e.Band, &e.From, &e.To, &e.Why, &e.Scanning, &e.Left, &ratio); err != nil {
			return nil, nil, err
		}
		e.AP = ap
		if e.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, nil, err
		}
		if ratio != "" {
			json.Unmarshal([]byte(ratio), &e.Ratio)
		}
		ids = append(ids, id)
		out = append(out, e)
	}
	return ids, out, rows.Err()
}

// SteerBefore is what an AP's steering asked of its clients before before.
func (s *Store) SteerBefore(ap hierarchy.NodeID, before time.Time, limit int) ([]SteerEvent, error) {
	return s.steers(`SELECT `+steerCols+` FROM steer_events WHERE ap = ? AND at < ? ORDER BY at DESC, rowid DESC LIMIT ?`,
		string(ap), before.UnixMilli(), limit)
}

// HourHealth is an AP's hour that began at hour: the worst of its reports
// in it, how many it made, and when the first came; false where it made
// none.
func (s *Store) HourHealth(ap hierarchy.NodeID, hour time.Time) (HealthPoint, int, time.Time, bool, error) {
	pts, err := s.HealthByHour(ap, hour)
	if err != nil || len(pts) == 0 || !pts[0].At.Equal(hour.Truncate(time.Hour)) {
		return HealthPoint{}, 0, time.Time{}, false, err
	}
	var n int
	var first sql.NullInt64
	if err := s.db.QueryRow(`SELECT reports FROM health_hours WHERE ap = ? AND hour = ?`, string(ap), hourOf(hour)).Scan(&n); err != nil {
		return HealthPoint{}, 0, time.Time{}, false, err
	}
	if err := s.db.QueryRow(`SELECT MIN(at) FROM health WHERE ap = ? AND at >= ? AND at < ?`, string(ap), hourOf(hour)*1000, (hourOf(hour)+3600)*1000).Scan(&first); err != nil {
		return HealthPoint{}, 0, time.Time{}, false, err
	}
	at := hour.Truncate(time.Hour)
	if first.Valid {
		at = time.UnixMilli(first.Int64).UTC()
	}
	return pts[0], n, at, true, nil
}
