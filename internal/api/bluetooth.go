package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// label is how a group of rotating addresses is known over time (0158): the
// company their data is from, and what they are, where that says more.
func (g bleGroup) label() string {
	name := g.CompanyName
	if name == "" && g.Company != nil {
		name = fmt.Sprintf("company 0x%04x", *g.Company)
	}
	if name == "" {
		name = "unknown maker"
	}
	if g.What != "" && g.What != g.CompanyName {
		return name + " · " + g.What
	}
	return name
}

// heard is what a report says its radio heard, for the store (0158): each
// device when it was last heard and first, and each group of rotating
// addresses, as of now.
func (b *bleDevices) heard(now time.Time) ([]conditions.BLEHeard, []conditions.BLECount) {
	if b == nil {
		return nil, nil
	}
	ago := func(s int64) time.Time { return now.Add(-time.Duration(s) * time.Second) }
	var hs []conditions.BLEHeard
	for _, d := range b.Devices {
		rssi := -127
		if d.RSSI != nil {
			rssi = *d.RSSI
		}
		maker := d.Maker
		if maker == "" {
			maker = d.CompanyName
		}
		kind := d.Type
		if d.Random != "" {
			kind = d.Random + " " + d.Type
		}
		hs = append(hs, conditions.BLEHeard{MAC: d.MAC, At: ago(d.LastAgo), First: ago(d.FirstAgo), RSSI: rssi,
			Name: d.Name, Kind: d.Kind, What: d.What, Maker: maker, Tracker: d.Tracker, Type: kind})
	}
	var cs []conditions.BLECount
	for _, g := range b.Rotating {
		cs = append(cs, conditions.BLECount{What: g.Label, At: ago(g.LastAgo), Count: g.Count})
	}
	return hs, cs
}

// bluetoothHeard is every device the APs the caller may view under ?under=
// (a folder or an AP; all, unless set) heard in the last ?hours= (24
// unless set, at most 2160), the last heard first, at most 500: each with
// what it is, when any AP first and last heard it, and what each AP heard
// of it: in how many five minutes, its strongest signal, and when first and
// last; and the rotating addresses, how many of each kind were heard each
// five minutes, the most any one AP heard. GET /v1/bluetooth (0158).
func (s *Server) bluetoothHeard(w http.ResponseWriter, r *http.Request, c call) error {
	hours, err := hoursOf(r, 24, 2160)
	if err != nil {
		return err
	}
	aps := mayView(c, hierarchy.NodeID(r.URL.Query().Get("under")))
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	devices, err := s.conds.BluetoothHeard(aps, since, 500)
	if err != nil {
		return err
	}
	rotating, err := s.conds.BluetoothRotating(aps, since)
	if err != nil {
		return err
	}
	if devices == nil {
		devices = []conditions.BLEDeviceHeard{}
	}
	// Each called what a person called it (0220).
	if names, err := s.conds.BluetoothNames(); err == nil {
		for i := range devices {
			devices[i].Named = names[devices[i].MAC]
		}
	}
	if rotating == nil {
		rotating = []conditions.BLECount{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "devices": devices, "rotating": rotating})
	return nil
}

var bleMACRE = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

// bluetoothDevice is one device's history: what it is and when any AP
// first and last heard it, and each five minutes an AP the caller may view
// under ?under= heard it in the last ?hours= (168 unless set, at most
// 2160), the oldest first, at most 30,000: the AP, when, and the strongest
// signal. GET /v1/bluetooth/devices/{mac} (0158).
func (s *Server) bluetoothDevice(w http.ResponseWriter, r *http.Request, c call) error {
	mac := strings.ToLower(r.PathValue("mac"))
	if !bleMACRE.MatchString(mac) {
		return badRequest("a MAC such as c1:00:00:00:00:01")
	}
	hours, err := hoursOf(r, 168, 2160)
	if err != nil {
		return err
	}
	aps := mayView(c, hierarchy.NodeID(r.URL.Query().Get("under")))
	seen, err := s.conds.BluetoothSeen(mac, aps, time.Now().Add(-time.Duration(hours)*time.Hour), 30000)
	if err != nil {
		return err
	}
	d, ok, err := s.conds.BluetoothDeviceKnown(mac)
	if err != nil {
		return err
	}
	// Who may view none of the APs that heard it hears nothing of it.
	if !ok || (len(seen) == 0 && !slicesContain(aps, d.LastAP)) {
		return errNotFound
	}
	if seen == nil {
		seen = []conditions.BLESeen{}
	}
	if names, err := s.conds.BluetoothNames(); err == nil {
		d.Named = names[mac]
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "device": d, "seen": seen})
	return nil
}

// bluetoothRename gives a Bluetooth device a name (0220), as a client is
// given one (0153), or with an empty one takes it away: PUT
// /v1/bluetooth/devices/{mac}/name with {"name": "attic sensor"}. It is
// for a device an AP has heard, and needs the clients right on the AP that
// heard it last. A named device not heard for a day is an alert.
func (s *Server) bluetoothRename(w http.ResponseWriter, r *http.Request, c call) error {
	mac := strings.ToLower(r.PathValue("mac"))
	if !bleMACRE.MatchString(mac) {
		return badRequest("a MAC such as c1:00:00:00:00:01")
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	name := strings.Join(strings.Fields(body.Name), " ")
	if utf8.RuneCountInString(name) > clientNameMost {
		return badRequest("name: at most %d characters", clientNameMost)
	}
	for _, ch := range name {
		if unicode.IsControl(ch) || ch == utf8.RuneError {
			return badRequest("name: letters, digits, spaces and marks, on one line")
		}
	}
	d, ok, err := s.conds.BluetoothDeviceKnown(mac)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	if !ok || !may(c, change.Locations, t, d.LastAP, access.View) {
		return errNotFound
	}
	if !may(c, change.Locations, t, d.LastAP, access.Clients) {
		return &apiError{http.StatusForbidden, "naming a Bluetooth device needs the clients right on the AP that heard it last (0177)"}
	}
	if err := s.conds.NameBluetooth(mac, name, string(c.actor)); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{"mac": mac, "name": name})
	return nil
}

func slicesContain(aps []hierarchy.NodeID, ap hierarchy.NodeID) bool {
	for _, a := range aps {
		if a == ap {
			return true
		}
	}
	return false
}
