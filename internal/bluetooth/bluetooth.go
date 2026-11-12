// Package bluetooth names what an AP's Bluetooth radio hears (0157): the
// company a device's manufacturer data is from, whom a 16-bit UUID is
// assigned to, and the category its appearance says, by the Bluetooth
// SIG's assigned numbers, taken when the manager was built: run
// `go run ./internal/bluetooth/gen` to take them again. And it says what
// kind of thing a device is, where what it sends says so.
package bluetooth

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed sig.txt.gz
var table []byte

var (
	once        sync.Once
	companies   map[int]string
	uuids       map[string]string
	appearances map[int]string
)

func load() {
	companies, uuids, appearances = map[int]string{}, map[string]string{}, map[int]string{}
	z, err := gzip.NewReader(bytes.NewReader(table))
	if err != nil {
		return
	}
	s := bufio.NewScanner(z)
	for s.Scan() {
		f := strings.SplitN(s.Text(), "\t", 3)
		if len(f) != 3 {
			continue
		}
		n, err := strconv.ParseUint(f[1], 16, 16)
		if err != nil {
			continue
		}
		switch f[0] {
		case "c":
			companies[int(n)] = f[2]
		case "u":
			uuids[strings.ToLower(f[1])] = f[2]
		case "a":
			appearances[int(n)] = f[2]
		}
	}
}

// Company names the company an identifier is assigned to, or "".
func Company(id int) string {
	once.Do(load)
	return companies[id]
}

// UUID names whom a 16-bit UUID ("fd3d") is assigned to: a member company,
// or the service it stands for. "" where it is not assigned.
func UUID(u string) string {
	once.Do(load)
	return uuids[strings.ToLower(u)]
}

// Appearance names the category of an appearance value: its top ten bits.
func Appearance(v int) string {
	once.Do(load)
	return appearances[v>>6]
}

// Size says how many companies, UUIDs and categories the table holds.
func Size() (int, int, int) {
	once.Do(load)
	return len(companies), len(uuids), len(appearances)
}

// Device is what is known of a device heard, for Kind.
type Device struct {
	Company    *int     // whose its manufacturer data is
	Mfr        string   // that data after the company, as hex, or its start
	UUID16     []string // the 16-bit UUIDs it names, as hex
	Appearance *int
	Tracker    string // what the AP took it to be, if a tracker
	Beacon     string // ibeacon or eddystone, if a beacon
}

// Companies whose manufacturer data says what a device is, by its first
// byte.
const (
	microsoft = 0x0006
	apple     = 0x004c
)

// The trackers the AP knows, by the word it gives each.
var trackers = map[string]string{
	"apple_find_my":         "Apple Find My",
	"samsung_smarttag":      "Samsung SmartTag",
	"tile":                  "Tile",
	"google_find_my_device": "Google Find My Device",
}

// What a 16-bit UUID says a device is, where it says: a kind and what.
var byUUID = map[string][2]string{
	"fcd2": {"sensor", "BTHome"},
	"181a": {"sensor", "environmental sensing"},
	"1809": {"sensor", "thermometer"},
	"181b": {"health", "body composition"},
	"181d": {"health", "weight scale"},
	"1810": {"health", "blood pressure"},
	"1808": {"health", "glucose"},
	"180d": {"fitness", "heart rate"},
	"1816": {"fitness", "cycling speed and cadence"},
	"1818": {"fitness", "cycling power"},
	"1826": {"fitness", "fitness machine"},
	"1812": {"input", "keyboard, mouse or controller"},
	"fd6f": {"personal", "exposure notification"},
	"fe2c": {"", "Google Fast Pair"},
	"fe95": {"", "Xiaomi MiBeacon"},
	"fd3d": {"", "SwitchBot"},
	"feaf": {"", "Nest"},
	"1827": {"", "Bluetooth mesh, unprovisioned"},
	"1828": {"", "Bluetooth mesh proxy"},
}

// What an appearance category says a device is.
var byAppearance = map[int]string{
	1: "personal", 2: "personal", 3: "personal", 15: "input", 12: "sensor",
	13: "fitness", 14: "health", 16: "health", 17: "fitness", 18: "fitness",
	49: "health", 50: "health", 10: "media",
}

// Kind says what a device is, where what it sends says so: kind is one of
// tracker, beacon, sensor, health, fitness, input, personal (a phone,
// computer or watch), audio or media, or "" where it is not known; what
// says more. Where neither is known what is the company its data is from,
// or whom its first UUID is assigned to.
func Kind(d Device) (kind, what string) {
	if t, ok := trackers[d.Tracker]; ok {
		return "tracker", t
	}
	if d.Tracker != "" {
		return "tracker", d.Tracker
	}
	switch d.Beacon {
	case "ibeacon":
		return "beacon", "iBeacon"
	case "eddystone":
		return "beacon", "Eddystone"
	}
	if d.Company != nil && len(d.Mfr) >= 2 {
		first := strings.ToLower(d.Mfr[:2])
		switch *d.Company {
		case apple:
			switch first {
			case "10":
				return "personal", "Apple iPhone, iPad, Mac or Watch"
			case "07":
				return "audio", "AirPods or Beats"
			case "09":
				return "media", "AirPlay"
			case "0c":
				return "personal", "Apple Handoff"
			}
		case microsoft:
			switch {
			case first == "01":
				return "personal", "Windows device"
			case strings.HasPrefix(strings.ToLower(d.Mfr), "030080"):
				return "input", "accessory waiting to pair (Swift Pair)"
			}
		}
	}
	for _, u := range d.UUID16 {
		if k, ok := byUUID[strings.ToLower(u)]; ok {
			return k[0], k[1]
		}
	}
	if d.Appearance != nil {
		if name := Appearance(*d.Appearance); name != "" && *d.Appearance>>6 != 0 {
			return byAppearance[*d.Appearance>>6], name
		}
	}
	if d.Company != nil {
		if name := Company(*d.Company); name != "" {
			return "", name
		}
	}
	for _, u := range d.UUID16 {
		if name := UUID(u); name != "" {
			return "", name
		}
	}
	return "", ""
}
