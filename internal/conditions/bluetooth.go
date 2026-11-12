package conditions

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What the APs' Bluetooth radios heard, kept (0158): each device that keeps
// its address, what it last said it is and when any AP first and last heard
// it; when each AP heard it, five minutes at a time, with the strongest
// signal in each; and how many rotating addresses of each kind each AP
// heard, five minutes at a time. Addresses that rotate are kept only as
// those counts.
const bluetoothTables = `
CREATE TABLE ble_devices (
	mac      TEXT PRIMARY KEY,
	first_at INTEGER NOT NULL,
	last_at  INTEGER NOT NULL,
	last_ap  TEXT NOT NULL,
	name     TEXT NOT NULL,
	kind     TEXT NOT NULL,
	what     TEXT NOT NULL,
	maker    TEXT NOT NULL,
	tracker  TEXT NOT NULL,
	type     TEXT NOT NULL
);
CREATE INDEX ble_devices_last ON ble_devices (last_at);
CREATE TABLE ble_seen (
	mac  TEXT NOT NULL,
	ap   TEXT NOT NULL,
	slot INTEGER NOT NULL,
	rssi INTEGER NOT NULL,
	PRIMARY KEY (mac, ap, slot)
) WITHOUT ROWID;
CREATE INDEX ble_seen_ap ON ble_seen (ap, slot);
CREATE INDEX ble_seen_slot ON ble_seen (slot);
CREATE TABLE ble_rotating (
	ap    TEXT NOT NULL,
	slot  INTEGER NOT NULL,
	what  TEXT NOT NULL,
	count INTEGER NOT NULL,
	PRIMARY KEY (ap, slot, what)
) WITHOUT ROWID;
CREATE INDEX ble_rotating_slot ON ble_rotating (slot);`

// BLESlot is how long each of a device's sightings stands for.
const BLESlot = 5 * time.Minute

func slotOf(t time.Time) int64 {
	s := int64(BLESlot / time.Second)
	return t.Unix() / s * s
}

// BLEDevice is a device the APs heard, as the store knows it: what it last
// said it is, and when any AP first and last heard it, and which last.
type BLEDevice struct {
	MAC     string           `json:"mac"`
	Name    string           `json:"name,omitempty"`
	Kind    string           `json:"kind,omitempty"`
	What    string           `json:"what,omitempty"`
	Maker   string           `json:"maker,omitempty"`
	Tracker string           `json:"tracker,omitempty"`
	Type    string           `json:"type,omitempty"`
	FirstAt time.Time        `json:"first_at"`
	LastAt  time.Time        `json:"last_at"`
	LastAP  hierarchy.NodeID `json:"last_ap"`
	// Named is the name a person gave it (0220), where one did.
	Named string `json:"named,omitempty"`
}

// BLEHeard is a device in one AP's report: when it was heard last, and
// first, how strongly (-127 where not said), and what it is.
type BLEHeard struct {
	MAC                                    string
	At, First                              time.Time
	RSSI                                   int
	Name, Kind, What, Maker, Tracker, Type string
}

// BLECount is how many rotating addresses of one kind were heard at once.
type BLECount struct {
	What  string    `json:"what"`
	At    time.Time `json:"at"`
	Count int       `json:"count"`
}

// BLESeen is when an AP heard a device: the start of the five minutes, and
// the strongest signal heard in them.
type BLESeen struct {
	AP   hierarchy.NodeID `json:"ap"`
	At   time.Time        `json:"at"`
	RSSI int              `json:"rssi"`
}

// RecordBluetooth keeps what one AP's report says its radio heard. A report
// says again what was heard in the ten minutes before it, which comes to
// the same five minutes, and is kept once, with the strongest signal.
func (s *Store) RecordBluetooth(ap hierarchy.NodeID, heard []BLEHeard, counts []BLECount) error {
	if len(heard) == 0 && len(counts) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, h := range heard {
		mac := strings.ToLower(h.MAC)
		if _, err := tx.Exec(`INSERT INTO ble_seen (mac, ap, slot, rssi) VALUES (?, ?, ?, ?)
			ON CONFLICT (mac, ap, slot) DO UPDATE SET rssi = MAX(rssi, excluded.rssi)`, mac, string(ap), slotOf(h.At), h.RSSI); err != nil {
			return err
		}
		first := h.First
		if first.IsZero() || first.After(h.At) {
			first = h.At
		}
		if _, err := tx.Exec(`INSERT INTO ble_devices (mac, first_at, last_at, last_ap, name, kind, what, maker, tracker, type)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (mac) DO UPDATE SET
				first_at = MIN(first_at, excluded.first_at),
				last_ap = CASE WHEN excluded.last_at >= last_at THEN excluded.last_ap ELSE last_ap END,
				last_at = MAX(last_at, excluded.last_at),
				name = CASE WHEN excluded.name != '' THEN excluded.name ELSE name END,
				kind = CASE WHEN excluded.kind != '' THEN excluded.kind ELSE kind END,
				what = CASE WHEN excluded.what != '' THEN excluded.what ELSE what END,
				maker = CASE WHEN excluded.maker != '' THEN excluded.maker ELSE maker END,
				tracker = CASE WHEN excluded.tracker != '' THEN excluded.tracker ELSE tracker END,
				type = excluded.type`,
			mac, first.Unix(), h.At.Unix(), string(ap), h.Name, h.Kind, h.What, h.Maker, h.Tracker, h.Type); err != nil {
			return err
		}
	}
	for _, c := range counts {
		if _, err := tx.Exec(`INSERT INTO ble_rotating (ap, slot, what, count) VALUES (?, ?, ?, ?)
			ON CONFLICT (ap, slot, what) DO UPDATE SET count = MAX(count, excluded.count)`, string(ap), slotOf(c.At), c.What, c.Count); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// in is a list of placeholders, and the APs to fill them, for an IN clause.
func in(aps []hierarchy.NodeID) (string, []any) {
	args := make([]any, len(aps))
	for i, a := range aps {
		args[i] = string(a)
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(aps)), ","), args
}

// BLEAPHeard is what one AP heard of a device in a time: in how many five
// minutes, its strongest signal, and when first and last.
type BLEAPHeard struct {
	AP    hierarchy.NodeID `json:"ap"`
	Slots int              `json:"slots"`
	Best  int              `json:"best"`
	First time.Time        `json:"first"`
	Last  time.Time        `json:"last"`
}

// BLEDeviceHeard is a device, and what each AP heard of it in a time.
type BLEDeviceHeard struct {
	BLEDevice
	APs []BLEAPHeard `json:"aps"`
}

// BluetoothHeard is every device the APs given heard since, the last heard
// first, most at the most.
func (s *Store) BluetoothHeard(aps []hierarchy.NodeID, since time.Time, most int) ([]BLEDeviceHeard, error) {
	if len(aps) == 0 {
		return nil, nil
	}
	ph, args := in(aps)
	rows, err := s.db.Query(`SELECT mac, ap, COUNT(*), MAX(rssi), MIN(slot), MAX(slot) FROM ble_seen
		WHERE ap IN (`+ph+`) AND slot >= ? GROUP BY mac, ap`, append(args, slotOf(since))...)
	if err != nil {
		return nil, err
	}
	by := map[string][]BLEAPHeard{}
	for rows.Next() {
		var mac, ap string
		var h BLEAPHeard
		var first, last int64
		if err := rows.Scan(&mac, &ap, &h.Slots, &h.Best, &first, &last); err != nil {
			rows.Close()
			return nil, err
		}
		h.AP, h.First, h.Last = hierarchy.NodeID(ap), time.Unix(first, 0).UTC(), time.Unix(last, 0).UTC()
		by[mac] = append(by[mac], h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []BLEDeviceHeard
	for mac, heard := range by {
		d, ok, err := s.BluetoothDeviceKnown(mac)
		if err != nil {
			return nil, err
		}
		if !ok {
			d = BLEDevice{MAC: mac}
		}
		out = append(out, BLEDeviceHeard{BLEDevice: d, APs: heard})
	}
	last := func(d BLEDeviceHeard) time.Time {
		var t time.Time
		for _, h := range d.APs {
			if h.Last.After(t) {
				t = h.Last
			}
		}
		return t
	}
	sort.Slice(out, func(i, j int) bool {
		if !last(out[i]).Equal(last(out[j])) {
			return last(out[i]).After(last(out[j]))
		}
		return out[i].MAC < out[j].MAC
	})
	if most > 0 && len(out) > most {
		out = out[:most]
	}
	return out, nil
}

// BluetoothDeviceKnown is what the store knows of one device.
func (s *Store) BluetoothDeviceKnown(mac string) (BLEDevice, bool, error) {
	var d BLEDevice
	var first, last int64
	var ap string
	err := s.db.QueryRow(`SELECT mac, first_at, last_at, last_ap, name, kind, what, maker, tracker, type FROM ble_devices WHERE mac = ?`, strings.ToLower(mac)).
		Scan(&d.MAC, &first, &last, &ap, &d.Name, &d.Kind, &d.What, &d.Maker, &d.Tracker, &d.Type)
	if err == sql.ErrNoRows {
		return BLEDevice{}, false, nil
	}
	if err != nil {
		return BLEDevice{}, false, err
	}
	d.FirstAt, d.LastAt, d.LastAP = time.Unix(first, 0).UTC(), time.Unix(last, 0).UTC(), hierarchy.NodeID(ap)
	return d, true, nil
}

// BluetoothSeen is every five minutes the APs given heard one device in
// since, the oldest first, most at the most: the latest kept.
func (s *Store) BluetoothSeen(mac string, aps []hierarchy.NodeID, since time.Time, most int) ([]BLESeen, error) {
	if len(aps) == 0 {
		return nil, nil
	}
	ph, args := in(aps)
	rows, err := s.db.Query(`SELECT ap, slot, rssi FROM ble_seen WHERE mac = ? AND ap IN (`+ph+`) AND slot >= ?
		ORDER BY slot DESC LIMIT ?`, append(append([]any{strings.ToLower(mac)}, args...), slotOf(since), most)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BLESeen
	for rows.Next() {
		var ap string
		var slot int64
		var x BLESeen
		if err := rows.Scan(&ap, &slot, &x.RSSI); err != nil {
			return nil, err
		}
		x.AP, x.At = hierarchy.NodeID(ap), time.Unix(slot, 0).UTC()
		out = append(out, x)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// BluetoothRotating is how many rotating addresses of each kind the APs
// given heard since, each five minutes: the most any one of them heard at
// once, as one phone near two APs is heard by both. The oldest first.
func (s *Store) BluetoothRotating(aps []hierarchy.NodeID, since time.Time) ([]BLECount, error) {
	if len(aps) == 0 {
		return nil, nil
	}
	ph, args := in(aps)
	rows, err := s.db.Query(`SELECT what, slot, MAX(count) FROM ble_rotating WHERE ap IN (`+ph+`) AND slot >= ?
		GROUP BY what, slot ORDER BY slot, what`, append(args, slotOf(since))...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BLECount
	for rows.Next() {
		var c BLECount
		var slot int64
		if err := rows.Scan(&c.What, &slot, &c.Count); err != nil {
			return nil, err
		}
		c.At = time.Unix(slot, 0).UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

// trimBluetooth deletes what was heard before cut, and the devices last
// heard before it.
func (s *Store) trimBluetooth(cut time.Time) error {
	for _, q := range []string{
		`DELETE FROM ble_seen WHERE slot < ?`,
		`DELETE FROM ble_rotating WHERE slot < ?`,
		`DELETE FROM ble_devices WHERE last_at < ?`,
	} {
		if _, err := s.db.Exec(q, cut.Unix()); err != nil {
			return err
		}
	}
	return nil
}
