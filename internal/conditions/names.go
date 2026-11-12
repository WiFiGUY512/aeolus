package conditions

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// clientNamesTable is the names people give clients (0153): a client that
// says no name of its own, or one that tells nobody what it is, is called
// what a person calls it, by its MAC. folded is the name in lower case,
// which a search is made in.
const clientNamesTable = `
CREATE TABLE client_names (
	mac    TEXT PRIMARY KEY,
	name   TEXT NOT NULL,
	folded TEXT NOT NULL,
	by     TEXT NOT NULL,
	at     INTEGER NOT NULL
);`

// ClientName is the name a person gave a client, who gave it and when.
type ClientName struct {
	MAC  string    `json:"mac"`
	Name string    `json:"name"`
	By   string    `json:"by"`
	At   time.Time `json:"at"`
}

// NameClient gives a client a name, in place of any it had; an empty name
// takes the name away.
func (s *Store) NameClient(mac, name, by string) error {
	if name == "" {
		_, err := s.db.Exec(`DELETE FROM client_names WHERE mac = ?`, mac)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO client_names (mac, name, folded, by, at) VALUES (?1, ?2, ?3, ?4, ?5)
		ON CONFLICT (mac) DO UPDATE SET name = excluded.name, folded = excluded.folded, by = excluded.by, at = excluded.at`,
		mac, name, strings.ToLower(name), by, s.now().UnixMilli())
	return err
}

// ClientNamed is the name a client was given, and whether it has one.
func (s *Store) ClientNamed(mac string) (ClientName, bool, error) {
	n := ClientName{MAC: mac}
	var at int64
	err := s.db.QueryRow(`SELECT name, by, at FROM client_names WHERE mac = ?`, mac).Scan(&n.Name, &n.By, &at)
	if err == sql.ErrNoRows {
		return ClientName{}, false, nil
	}
	if err != nil {
		return ClientName{}, false, err
	}
	n.At = time.UnixMilli(at).UTC()
	return n, true, nil
}

// ClientHosts is the name each client last gave of itself, by MAC, of the
// clients last seen on one of aps since a time: kept from when it said it,
// where its AP's report no longer does (0153).
func (s *Store) ClientHosts(aps []hierarchy.NodeID, since time.Time) (map[string]string, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT mac, host FROM clients WHERE host != '' AND last_seen >= ?2
		AND ap IN (SELECT value FROM json_each(?1))`, string(ids), since.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var mac, host string
		if err := rows.Scan(&mac, &host); err != nil {
			return nil, err
		}
		out[mac] = host
	}
	return out, rows.Err()
}

// ClientNames is the names given to the clients last seen on one of aps, by
// MAC.
func (s *Store) ClientNames(aps []hierarchy.NodeID) (map[string]string, error) {
	ids, err := json.Marshal(aps)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT n.mac, n.name FROM client_names n JOIN clients c ON c.mac = n.mac
		WHERE c.ap IN (SELECT value FROM json_each(?))`, string(ids))
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
