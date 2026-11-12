package api

import (
	"regexp"
	"strings"

	"github.com/WiFiGUY512/aeolus/internal/bluetooth"
	"github.com/WiFiGUY512/aeolus/internal/oui"
)

// bleDevices is what an AP's Bluetooth radio hears, device by device
// (0157): where the list comes from (proxy, the daemon that lends the radio
// to Home Assistant, which hears every advertisement; survey, the AP's own,
// every few minutes; or both), how long ago the survey last looked, the
// devices that keep their address and the trackers, and the addresses that
// rotate, counted by what they are from. Times are seconds ago.
type bleDevices struct {
	Source      string      `json:"source"`
	SurveyedAgo *int64      `json:"surveyed_ago,omitempty"`
	Devices     []bleDevice `json:"devices"`
	Rotating    []bleGroup  `json:"rotating"`
}

// bleDevice is one device heard: its address, public or random and what
// kind of random; its signal now, on average and at its strongest; how
// often and since when it was heard; and what its advertisements say.
type bleDevice struct {
	MAC        string     `json:"mac"`
	Type       string     `json:"type"`
	Random     string     `json:"random,omitempty"`
	RSSI       *int       `json:"rssi,omitempty"`
	RSSIAvg    *int       `json:"rssi_avg,omitempty"`
	RSSIMax    *int       `json:"rssi_max,omitempty"`
	Seen       int64      `json:"seen"`
	FirstAgo   int64      `json:"first_ago"`
	LastAgo    int64      `json:"last_ago"`
	Name       string     `json:"name,omitempty"`
	Flags      *int       `json:"flags,omitempty"`
	TxPower    *int       `json:"tx_power,omitempty"`
	Appearance *int       `json:"appearance,omitempty"`
	Company    *int       `json:"company,omitempty"`
	Mfr        string     `json:"mfr,omitempty"`
	UUID16     []string   `json:"uuid16,omitempty"`
	UUID128    []string   `json:"uuid128,omitempty"`
	Beacon     *bleBeacon `json:"beacon,omitempty"`
	Tracker    string     `json:"tracker,omitempty"`
	// What the manager adds before the report is stored: the maker of a
	// public address, by its OUI; the company its manufacturer data is
	// from; whom its 16-bit UUIDs are assigned to; and what it is.
	Maker       string   `json:"maker,omitempty"`
	CompanyName string   `json:"company_name,omitempty"`
	Services    []string `json:"services,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	What        string   `json:"what,omitempty"`
}

// bleBeacon is a beacon a device is: an iBeacon, with its UUID, major,
// minor and power at a metre; or Eddystone, with its frame.
type bleBeacon struct {
	Type  string `json:"type"`
	Frame string `json:"frame,omitempty"`
	UUID  string `json:"uuid,omitempty"`
	Major *int   `json:"major,omitempty"`
	Minor *int   `json:"minor,omitempty"`
	TX1m  *int   `json:"tx_1m,omitempty"`
}

// bleGroup is the addresses that rotate that came from the same company,
// with the same first bytes of data and first service UUID: how many were
// heard, the strongest of them, and how long ago the last.
type bleGroup struct {
	Company     *int   `json:"company,omitempty"`
	Mfr         string `json:"mfr,omitempty"`
	UUID16      string `json:"uuid16,omitempty"`
	Count       int    `json:"count"`
	RSSIMax     *int   `json:"rssi_max,omitempty"`
	LastAgo     int64  `json:"last_ago"`
	CompanyName string `json:"company_name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	What        string `json:"what,omitempty"`
	// How the group is known over time (0158).
	Label string `json:"label,omitempty"`
}

var (
	bleSources = map[string]bool{"proxy": true, "survey": true, "both": true}
	bleRandoms = map[string]bool{"": true, "static": true, "resolvable": true, "non-resolvable": true, "reserved": true}
	uuid16RE   = regexp.MustCompile(`^[0-9a-f]{4}$`)
	uuid128RE  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	bleHexRE   = regexp.MustCompile(`^([0-9a-f]{2}){0,32}$`)
	trackerRE  = regexp.MustCompile(`^[a-z0-9_]{0,32}$`)
)

// keep is what an AP's Bluetooth radio heard, less each device and group
// it says badly (0241), and nothing where it names no source of its own:
// the rest of the AP's report is kept either way, so that one advertisement
// read wrong does not stop all it says. How many it left out.
func (b *bleDevices) keep() (*bleDevices, int) {
	if b == nil {
		return nil, 0
	}
	if !bleSources[b.Source] || (b.SurveyedAgo != nil && *b.SurveyedAgo < 0) {
		return nil, len(b.Devices) + len(b.Rotating)
	}
	out, left := *b, 0
	out.Devices, out.Rotating = nil, nil
	for _, d := range b.Devices {
		if len(out.Devices) < 128 && d.good() {
			out.Devices = append(out.Devices, d)
		} else {
			left++
		}
	}
	for _, g := range b.Rotating {
		if len(out.Rotating) < 32 && g.good() {
			out.Rotating = append(out.Rotating, g)
		} else {
			left++
		}
	}
	return &out, left
}

// inRange says whether a number said is within lo and hi, where it is said.
func inRange(p *int, lo, hi int) bool { return p == nil || (*p >= lo && *p <= hi) }

// good says whether a device is well said: a MAC, public or random; signals
// from -127 to 20 dBm; counts and times not negative; a printable name of at
// most 64 bytes; numbers in their ranges; data in hex of at most 32 bytes;
// at most 8 16-bit and 4 128-bit UUIDs; an iBeacon or Eddystone beacon; and
// a tracker word.
func (d bleDevice) good() bool {
	if !macRE.MatchString(d.MAC) || (d.Type != "public" && d.Type != "random") || !bleRandoms[d.Random] ||
		!inRange(d.RSSI, -127, 20) || !inRange(d.RSSIAvg, -127, 20) || !inRange(d.RSSIMax, -127, 20) || d.Seen < 0 || d.FirstAgo < 0 || d.LastAgo < 0 ||
		len(d.Name) > 64 || strings.ContainsFunc(d.Name, func(r rune) bool { return r < 0x20 || r == 0x7f }) ||
		!inRange(d.Flags, 0, 255) || !inRange(d.TxPower, -127, 127) || !inRange(d.Appearance, 0, 65535) || !inRange(d.Company, 0, 65535) ||
		!bleHexRE.MatchString(d.Mfr) || len(d.UUID16) > 8 || len(d.UUID128) > 4 || !trackerRE.MatchString(d.Tracker) {
		return false
	}
	for _, u := range d.UUID16 {
		if !uuid16RE.MatchString(u) {
			return false
		}
	}
	for _, u := range d.UUID128 {
		if !uuid128RE.MatchString(u) {
			return false
		}
	}
	if e := d.Beacon; e != nil && !(e.Type == "ibeacon" && (e.UUID == "" || uuid128RE.MatchString(e.UUID)) && inRange(e.Major, 0, 65535) && inRange(e.Minor, 0, 65535) && inRange(e.TX1m, -127, 127)) &&
		!(e.Type == "eddystone" && len(e.Frame) <= 8) {
		return false
	}
	return true
}

// good says whether a group of rotating addresses is well said.
func (g bleGroup) good() bool {
	return inRange(g.Company, 0, 65535) && bleHexRE.MatchString(g.Mfr) && (g.UUID16 == "" || uuid16RE.MatchString(g.UUID16)) &&
		g.Count >= 1 && g.Count <= 100_000 && inRange(g.RSSIMax, -127, 20) && g.LastAgo >= 0
}

// name adds what the manager knows of each device heard: the maker of a
// public address, by its OUI; the company its data is from; whom its
// 16-bit UUIDs are assigned to; and what it is, where what it sends says.
func (b *bleDevices) name() {
	if b == nil {
		return
	}
	for i := range b.Devices {
		d := &b.Devices[i]
		d.Maker, d.CompanyName, d.Services = "", "", nil
		if d.Type == "public" {
			d.Maker, _ = oui.Lookup(d.MAC)
		}
		if d.Company != nil {
			d.CompanyName = bluetooth.Company(*d.Company)
		}
		for _, u := range d.UUID16 {
			if n := bluetooth.UUID(u); n != "" && len(d.Services) < 4 {
				d.Services = append(d.Services, n)
			}
		}
		beacon := ""
		if d.Beacon != nil {
			beacon = d.Beacon.Type
		}
		d.Kind, d.What = bluetooth.Kind(bluetooth.Device{Company: d.Company, Mfr: d.Mfr, UUID16: d.UUID16, Appearance: d.Appearance, Tracker: d.Tracker, Beacon: beacon})
	}
	for i := range b.Rotating {
		g := &b.Rotating[i]
		g.CompanyName = ""
		if g.Company != nil {
			g.CompanyName = bluetooth.Company(*g.Company)
		}
		var u []string
		if g.UUID16 != "" {
			u = []string{g.UUID16}
		}
		g.Kind, g.What = bluetooth.Kind(bluetooth.Device{Company: g.Company, Mfr: g.Mfr, UUID16: u})
		g.Label = g.label()
	}
}
