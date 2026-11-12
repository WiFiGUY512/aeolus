package conditions

import (
	"database/sql"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The names people give Bluetooth devices (0220), as clients are named
// (0153): one that says nothing of itself, or one a person keeps watch on,
// as a sensor, is called what they call it, by its MAC. A named device not
// heard for a day is an alert.
const bleNamesTable = `
CREATE TABLE ble_names (
	mac  TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	by   TEXT NOT NULL,
	at   INTEGER NOT NULL
);`

// NameBluetooth gives a Bluetooth device a name, as who; an empty one takes
// its name away.
func (s *Store) NameBluetooth(mac, name, by string) error {
	mac = strings.ToLower(mac)
	if name == "" {
		_, err := s.db.Exec(`DELETE FROM ble_names WHERE mac = ?`, mac)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO ble_names (mac, name, by, at) VALUES (?1, ?2, ?3, ?4)
		ON CONFLICT (mac) DO UPDATE SET name = ?2, by = ?3, at = ?4`, mac, name, by, s.now().UnixMilli())
	return err
}

// BluetoothNames is the names people gave Bluetooth devices, by MAC.
func (s *Store) BluetoothNames() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT mac, name FROM ble_names`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var mac, name string
		if err := rows.Scan(&mac, &name); err != nil {
			return nil, err
		}
		out[mac] = name
	}
	return out, rows.Err()
}

// BLENamed is a device a person named, and when and where it was last
// heard, where it ever was.
type BLENamed struct {
	MAC, Name string
	LastAt    time.Time
	LastAP    hierarchy.NodeID
	Heard     bool
}

// BluetoothNamed is every device people named, with when and by which AP
// it was last heard.
func (s *Store) BluetoothNamed() ([]BLENamed, error) {
	rows, err := s.db.Query(`SELECT n.mac, n.name, d.last_at, d.last_ap FROM ble_names n LEFT JOIN ble_devices d ON d.mac = n.mac ORDER BY n.mac`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BLENamed
	for rows.Next() {
		var n BLENamed
		var last sql.NullInt64
		var ap sql.NullString
		if err := rows.Scan(&n.MAC, &n.Name, &last, &ap); err != nil {
			return nil, err
		}
		if last.Valid {
			n.Heard, n.LastAt, n.LastAP = true, time.Unix(last.Int64, 0).UTC(), hierarchy.NodeID(ap.String)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
