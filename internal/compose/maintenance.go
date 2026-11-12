package compose

import "time"

// Maintenance (0218) and its test window (0344), as an AP's folders say.
const (
	MaintenanceOnPath    = "maintenance.on"
	MaintenanceUntilPath = "maintenance.until"
	MaintenanceWhyPath   = "maintenance.why"
	MaintenanceHoldPath  = "maintenance.hold"
)

// Maintenance is an AP's maintenance as its folders say (0218): whether it
// is on, when it ends (zero: when on is taken away), why, and whether the
// AP holds still meanwhile (0344).
type Maintenance struct {
	On    bool
	Until time.Time
	Why   string
	Hold  bool
}

// MaintenanceOf reads an AP's maintenance from its settings: value gives a
// field's value by its path, nil where it is not set.
func MaintenanceOf(value func(path string) any) Maintenance {
	var m Maintenance
	m.On, _ = value(MaintenanceOnPath).(bool)
	m.Hold, _ = value(MaintenanceHoldPath).(bool)
	m.Why, _ = value(MaintenanceWhyPath).(string)
	if s, _ := value(MaintenanceUntilPath).(string); s != "" {
		m.Until, _ = ParseUntil(s)
	}
	return m
}

// ParseUntil reads maintenance.until: a day and a time in UTC
// (2026-11-01T18:00Z), or a day alone, as it begins.
func ParseUntil(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02T15:04Z", s)
	if err != nil {
		t, err = time.Parse("2006-01-02", s)
	}
	return t.UTC(), err == nil
}

// In says whether the AP is in maintenance at now: on, and its end not come.
func (m Maintenance) In(now time.Time) bool {
	return m.On && (m.Until.IsZero() || now.Before(m.Until))
}

// Window says whether the maintenance is a test window (0344): on, with
// hold, and an end. A test window always ends by itself, so hold without an
// end holds nothing.
func (m Maintenance) Window() bool { return m.On && m.Hold && !m.Until.IsZero() }

// Held says whether the AP holds still at now (0344): in a test window
// whose end has not come.
func (m Maintenance) Held(now time.Time) bool { return m.Window() && m.In(now) }

// markHold tells an AP of its test window (0344): hold, with when it ends
// in Unix seconds and why. The AP holds still until then by its own clock,
// whether it reaches the manager or not, so the end is sent as it is, come
// or not: the AP leaves the hold once it is past. It is the manager's word,
// not a setting, so it is added after the schema's check, which knows no
// such field.
func markHold(doc map[string]any, m Maintenance) {
	if !m.Window() {
		return
	}
	hold := map[string]any{"until": float64(m.Until.Unix())}
	if m.Why != "" {
		hold["why"] = m.Why
	}
	doc["hold"] = hold
}
