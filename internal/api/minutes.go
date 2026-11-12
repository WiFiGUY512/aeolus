package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What each minute came to on the APs' radios (0207). aeolus-live samples
// each radio and its clients every five seconds, and at each minute's end
// sends what the minute came to on the AP's line (0145), with what it sees
// of traced clients. The manager keeps every minute: the radios' a month,
// the clients' a week.

const (
	// radioMinutesKeep and clientMinutesKeep are how long the radios' and
	// the clients' minutes are kept.
	radioMinutesKeep  = 30 * 24 * time.Hour
	clientMinutesKeep = 7 * 24 * time.Hour
	// minutesBatch is how many minutes an AP may send at once; minutesLate
	// how long after a minute it is still taken, an AP keeping an hour of
	// them while it cannot send; minutesAhead how far ahead of the
	// manager's clock an AP's may be.
	minutesBatch = 10
	minutesLate  = 26 * time.Hour
	minutesAhead = 5 * time.Minute
	// minutesHours is how many hours of radios' minutes one asking reads;
	// a client's, minutesClientHours.
	minutesHours       = 24
	minutesClientHours = 168
	// newsBatch is how much news an AP may send at once.
	newsBatch = 200
	// uplinkMinutesMost is how many of an AP's uplink minutes one asking
	// reads: an hour and a little more of 8 ports (0351).
	uplinkMinutesMost = 8 * 65
)

// apMinute is what a minute came to on an AP, as aeolus-live says it: its
// start, in seconds since 1970, and its radios and their clients.
type apMinute struct {
	At      int64           `json:"at"`
	Radios  []apRadioMinute `json:"radios"`
	Clients []apClientMin   `json:"clients"`
}

type apCounts struct {
	TxPackets int64 `json:"tx_packets"`
	TxRetries int64 `json:"tx_retries"`
	TxFailed  int64 `json:"tx_failed"`
	RxPackets int64 `json:"rx_packets"`
	TxBytes   int64 `json:"tx_bytes"`
	RxBytes   int64 `json:"rx_bytes"`
}

type apRadioMinute struct {
	Radio      string   `json:"radio"`
	Band       string   `json:"band"`
	Channel    *int     `json:"channel"`
	Width      *int     `json:"width"`
	Power      *float64 `json:"power"`
	Busy       *float64 `json:"busy"`
	BusyMax    *float64 `json:"busy_max"`
	Rx         *float64 `json:"rx"`
	Tx         *float64 `json:"tx"`
	Noise      *float64 `json:"noise"`
	Clients    *float64 `json:"clients"`
	ClientsMax *int     `json:"clients_max"`
	apCounts
}

type apClientMin struct {
	MAC       string   `json:"mac"`
	Band      string   `json:"band"`
	SSID      string   `json:"ssid"`
	Signal    *float64 `json:"signal"`
	SignalMin *float64 `json:"signal_min"`
	TxRate    *float64 `json:"tx_rate"`
	RxRate    *float64 `json:"rx_rate"`
	Secs      int      `json:"secs"`
	Heard     *float64 `json:"heard"`
	apCounts
}

// within says whether v, where said, is from lo to hi.
func within(v *float64, lo, hi float64) bool { return v == nil || (*v >= lo && *v <= hi) }

// wholeOf is v as a whole number, where said.
func wholeOf(v *float64) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	if *v < 0 {
		n = int(*v - 0.5)
	} else {
		n = int(*v + 0.5)
	}
	return &n
}

func (c apCounts) ok() bool {
	return c.TxPackets >= 0 && c.TxRetries >= 0 && c.TxFailed >= 0 && c.RxPackets >= 0 && c.TxBytes >= 0 && c.RxBytes >= 0
}

func (c apCounts) kept() conditions.Counts {
	return conditions.Counts{TxPackets: c.TxPackets, TxRetries: c.TxRetries, TxFailed: c.TxFailed, RxPackets: c.RxPackets, TxBytes: c.TxBytes, RxBytes: c.RxBytes}
}

// minutesOf is what an AP's minutes came to, as they are kept: each is
// read alone, so that a field a newer AP adds is passed over and not
// refused, and one not well said is left out, as is a radio or a client of
// one not well said. A minute's time is its start by the AP's clock, which
// NTP holds well enough for minutes; one from too long ago or yet to come
// is left out.
func minutesOf(ap hierarchy.NodeID, said []json.RawMessage, now time.Time) (radios []conditions.RadioMinute, clients []conditions.ClientMinute) {
	for _, raw := range said {
		_, rs, cs, ok := readStretch(ap, raw, 60, now, minutesLate)
		if ok {
			radios = append(radios, rs...)
			clients = append(clients, cs...)
		}
	}
	return radios, clients
}

// apUplinkMinute is what a minute came to on one of an AP's uplink ports,
// as aeolus-live says it (0351).
type apUplinkMinute struct {
	Port    string   `json:"port"`
	Up      bool     `json:"up"`
	Speed   *float64 `json:"speed"`
	Full    *bool    `json:"full"`
	Secs    int      `json:"secs"`
	Rx      *float64 `json:"rx"`
	Tx      *float64 `json:"tx"`
	RxMax   *float64 `json:"rx_max"`
	TxMax   *float64 `json:"tx_max"`
	RxBytes int64    `json:"rx_bytes"`
	TxBytes int64    `json:"tx_bytes"`
	Errors  int64    `json:"errors"`
	Changes int64    `json:"changes"`
}

// uplinkPortRE is an Ethernet port's name as Linux has it.
var uplinkPortRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

// uplinkMinutesOf is what an AP's minutes came to on its uplink ports
// (0351), as they are kept: a minute's time as minutesOf takes it, at most
// 8 ports a minute, and a port not well said left out. A minute from an
// agent before 0351 says none.
func uplinkMinutesOf(ap hierarchy.NodeID, said []json.RawMessage, now time.Time) []conditions.UplinkMinute {
	var out []conditions.UplinkMinute
	for _, raw := range said {
		var m struct {
			At     int64            `json:"at"`
			Uplink []apUplinkMinute `json:"uplink"`
		}
		if json.Unmarshal(raw, &m) != nil || m.At <= 0 || m.At%60 != 0 || len(m.Uplink) > 8 {
			continue
		}
		at := time.Unix(m.At, 0).UTC()
		if at.Before(now.Add(-minutesLate)) || at.After(now.Add(minutesAhead)) {
			continue
		}
		for _, u := range m.Uplink {
			// Up to 100 Gbit/s, over a minute and a little more.
			if !uplinkPortRE.MatchString(u.Port) || u.Secs < 0 || u.Secs > 70 || !within(u.Speed, 1, 100000) ||
				!within(u.Rx, 0, 100000) || !within(u.Tx, 0, 100000) || !within(u.RxMax, 0, 105000) || !within(u.TxMax, 0, 105000) ||
				u.RxBytes < 0 || u.TxBytes < 0 || u.Errors < 0 || u.Changes < 0 || u.Changes > 100000 {
				continue
			}
			out = append(out, conditions.UplinkMinute{
				AP: ap, At: at, Port: u.Port, Up: u.Up, Speed: wholeOf(u.Speed), Full: u.Full, Secs: u.Secs,
				Rx: u.Rx, Tx: u.Tx, RxMax: u.RxMax, TxMax: u.TxMax, RxBytes: u.RxBytes, TxBytes: u.TxBytes,
				Errors: u.Errors, Changes: u.Changes,
			})
		}
	}
	return out
}

// readStretch reads what a stretch of an AP's radios came to: a minute
// (0207), or the 5 seconds of a live view (0207 part 2), every seconds
// long and starting on one; at, its start; a radio or a client not well
// said is left out, and the stretch itself where it is not well said, or
// is from more than late ago or more than minutesAhead to come. Each is
// read alone, so that a field a newer AP adds is passed over and not
// refused.
func readStretch(ap hierarchy.NodeID, raw json.RawMessage, every int64, now time.Time, late time.Duration) (at time.Time, radios []conditions.RadioMinute, clients []conditions.ClientMinute, ok bool) {
	var m apMinute
	if json.Unmarshal(raw, &m) != nil || m.At <= 0 || m.At%every != 0 || len(m.Radios) > 8 || len(m.Clients) > 256 {
		return at, nil, nil, false
	}
	at = time.Unix(m.At, 0).UTC()
	if at.Before(now.Add(-late)) || at.After(now.Add(minutesAhead)) {
		return at, nil, nil, false
	}
	for _, r := range m.Radios {
		if !radioRE.MatchString(r.Radio) || !bands[r.Band] || !r.apCounts.ok() ||
			(r.Channel != nil && (*r.Channel < 0 || *r.Channel > 233)) || (r.Width != nil && (*r.Width < 5 || *r.Width > 320)) ||
			!within(r.Power, -10, 40) || !within(r.Busy, 0, 100) || !within(r.BusyMax, 0, 100) || !within(r.Rx, 0, 100) || !within(r.Tx, 0, 100) ||
			!within(r.Noise, -127, 0) || !within(r.Clients, 0, 1024) || (r.ClientsMax != nil && (*r.ClientsMax < 0 || *r.ClientsMax > 1024)) {
			continue
		}
		radios = append(radios, conditions.RadioMinute{
			AP: ap, At: at, Radio: r.Radio, Band: r.Band, Channel: r.Channel, Width: r.Width, Power: wholeOf(r.Power),
			Busy: r.Busy, BusyMax: r.BusyMax, Rx: r.Rx, Tx: r.Tx, Noise: wholeOf(r.Noise), Clients: r.Clients, ClientsMax: r.ClientsMax,
			Counts: r.apCounts.kept(),
		})
	}
	for _, c := range m.Clients {
		mac := strings.ToLower(c.MAC)
		if !macRE.MatchString(mac) || !bands[c.Band] || !c.apCounts.ok() || c.Secs < 0 || c.Secs > 60 ||
			!within(c.Signal, -127, 0) || !within(c.SignalMin, -127, 0) || !within(c.TxRate, 0, 100000) || !within(c.RxRate, 0, 100000) ||
			!within(c.Heard, -127, 0) {
			continue
		}
		ssid := c.SSID
		if len(ssid) > 32 || !printable(ssid) {
			ssid = ""
		}
		clients = append(clients, conditions.ClientMinute{
			MAC: mac, At: at, AP: ap, Band: c.Band, SSID: ssid, Signal: wholeOf(c.Signal), SignalMin: wholeOf(c.SignalMin),
			TxRate: c.TxRate, RxRate: c.RxRate, Secs: c.Secs, Heard: wholeOf(c.Heard), Counts: c.apCounts.kept(),
		})
	}
	return at, radios, clients, true
}

// clientNews is a client's coming or going as an AP's aeolus-journey heard
// it from hostapd (0252): joined, let on to one of the AP's networks, or
// left, let go of by hostapd; at when, by the AP's clock in ms; signal how
// strongly the AP heard one that joined, where it knew. Of a leaving, why
// (0255): by whom, the frame and its reason, all where the frame went to
// every station, and aeolus, the word of the daemon of Aeolus's own that
// sent it off (leaveOf).
type clientNews struct {
	Kind    string `json:"kind"`
	MAC     string `json:"mac"`
	At      int64  `json:"at"`
	SSID    string `json:"ssid"`
	Band    string `json:"band"`
	Network string `json:"network"`
	Signal  *int   `json:"signal"`
	By      string `json:"by"`
	Frame   string `json:"frame"`
	Reason  *int   `json:"reason"`
	All     bool   `json:"all"`
	Aeolus  string `json:"aeolus"`
}

// clientMove is a client's coming (a minute of it, with no counts, from
// the moment it was let on) or its going (left), with why, where the AP
// said (0255).
type clientMove struct {
	conditions.ClientMinute
	Left  bool
	Leave *leaveDetail
}

// newsOf is the news an AP's daemons handed aeolus-live (0207), each read
// alone by its kind: power control's steps (0206), as the state report
// says them, and clients coming and going (0252). A kind the manager does
// not know is passed over.
func newsOf(ap hierarchy.NodeID, said []json.RawMessage, now time.Time) (apc []conditions.APCEvent, moves []clientMove) {
	for _, raw := range said {
		var k struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(raw, &k) != nil {
			continue
		}
		switch k.Kind {
		case "apc":
			var e apcEvent
			if json.Unmarshal(raw, &e) != nil || !e.ok() || !e.fresh(now) {
				continue
			}
			apc = append(apc, e.kept())
		case "joined", "left":
			var n clientNews
			if json.Unmarshal(raw, &n) != nil {
				continue
			}
			mac, at := strings.ToLower(n.MAC), time.UnixMilli(n.At).UTC()
			if !macRE.MatchString(mac) || !bands[n.Band] || n.At <= 0 || at.Before(now.Add(-minutesLate)) || at.After(now.Add(minutesAhead)) ||
				(n.Signal != nil && (*n.Signal < -127 || *n.Signal > 0)) {
				continue
			}
			ssid := n.SSID
			if len(ssid) > 32 || !printable(ssid) {
				ssid = ""
			}
			m := clientMove{ClientMinute: conditions.ClientMinute{MAC: mac, At: at, AP: ap, Band: n.Band, SSID: ssid}, Left: k.Kind == "left"}
			if !m.Left {
				m.Signal = n.Signal
			} else {
				m.Leave = leaveOf(n)
			}
			moves = append(moves, m)
		}
	}
	return apc, moves
}

// keepMinutes keeps what an AP said of its minutes and its news, and how
// many minutes it kept.
func (s *Server) keepMinutes(ap hierarchy.NodeID, minutes, news []json.RawMessage, now time.Time) int {
	n := 0
	if len(minutes) > 0 {
		radios, clients := minutesOf(ap, minutes, now)
		if err := s.conds.RecordMinutes(radios, clients); err != nil {
			slog.Warn("minutes not kept", "ap", ap, "err", err)
		} else {
			n = len(radios)
		}
		if err := s.conds.RecordUplinkMinutes(uplinkMinutesOf(ap, minutes, now)); err != nil {
			slog.Warn("uplink minutes not kept", "ap", ap, "err", err)
		}
		// Where each client is, within a minute (0251), and not at the
		// AP's next report.
		s.active.minutes(ap, clients)
		if err := s.conds.ClientsActive(ap, clients); err != nil {
			slog.Warn("clients' minutes not kept as seen", "ap", ap, "err", err)
		}
	}
	evs, moves := newsOf(ap, news, now)
	if len(evs) > 0 {
		if err := s.conds.RecordAPCEvents(ap, evs); err != nil {
			slog.Warn("power control's steps not kept", "ap", ap, "err", err)
		}
	}
	// Where each client is, the moment it joins or leaves (0252); and why
	// each left (0255), with how strongly the AP last heard it, before its
	// leaving is taken in.
	if len(moves) > 0 {
		if err := s.conds.RecordClientEvents(s.leavings(ap, moves)); err != nil {
			slog.Warn("clients' leavings not kept", "ap", ap, "err", err)
		}
		s.active.moved(ap, moves)
		var joins []conditions.ClientMinute
		for _, m := range moves {
			if !m.Left {
				joins = append(joins, m.ClientMinute)
			}
		}
		if err := s.conds.ClientsJoined(ap, joins); err != nil {
			slog.Warn("clients' joins not kept as seen", "ap", ap, "err", err)
		}
	}
	return n
}

// minuteRadio is one radio's minutes, as GET /v1/minutes gives them.
type minuteRadio struct {
	AP      hierarchy.NodeID `json:"ap"`
	Name    string           `json:"ap_name"`
	Radio   string           `json:"radio"`
	Band    string           `json:"band"`
	Minutes []minutePoint    `json:"minutes"`
}

// minutePoint is a radio's minute without what its radio says.
type minutePoint struct {
	At         time.Time `json:"at"`
	Channel    *int      `json:"channel"`
	Width      *int      `json:"width"`
	Power      *int      `json:"power"`
	Busy       *float64  `json:"busy"`
	BusyMax    *float64  `json:"busy_max"`
	Rx         *float64  `json:"rx"`
	Tx         *float64  `json:"tx"`
	Noise      *int      `json:"noise"`
	Clients    *float64  `json:"clients"`
	ClientsMax *int      `json:"clients_max"`
	conditions.Counts
}

// clientPoint is a client's minute, with its AP's name.
type clientPoint struct {
	conditions.ClientMinute
	APName string `json:"ap_name"`
}

// minutes is what each minute came to (0207) on the APs the caller may
// view, below one Locations node where ?under= names it: GET /v1/minutes.
// Each radio's minutes over ?hours= (1 unless set, at most 24), on ?band=
// alone where it is set, by AP and radio, the oldest first; and with
// ?mac=, that client's too, over as many as 168 hours, on any AP the
// caller may view.
func (s *Server) minutes(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	mac := strings.ToLower(q.Get("mac"))
	if mac != "" && !macRE.MatchString(mac) {
		return badRequest("mac: a client's MAC")
	}
	band := q.Get("band")
	if band != "" && !bands[band] {
		return badRequest("band: 2g, 5g or 6g")
	}
	most := minutesHours
	if mac != "" {
		most = minutesClientHours
	}
	hours := 1
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > most {
			return badRequest("hours: from 1 to %d", most)
		}
		hours = n
	}
	aps := mayView(c, hierarchy.NodeID(q.Get("under")))
	t := c.state.Org.Locations
	name := func(ap hierarchy.NodeID) string {
		if n, ok := t.Node(ap); ok {
			return n.Name
		}
		return string(ap)
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	out := map[string]any{"hours": hours}
	if mac == "" || q.Get("radios") != "" {
		rows, err := s.conds.RadioMinutes(since, aps, band)
		if err != nil {
			return err
		}
		radios := []minuteRadio{}
		for _, m := range rows {
			if n := len(radios); n == 0 || radios[n-1].AP != m.AP || radios[n-1].Radio != m.Radio {
				radios = append(radios, minuteRadio{AP: m.AP, Name: name(m.AP), Radio: m.Radio, Band: m.Band, Minutes: []minutePoint{}})
			}
			p := &radios[len(radios)-1]
			p.Band = m.Band
			p.Minutes = append(p.Minutes, minutePoint{At: m.At, Channel: m.Channel, Width: m.Width, Power: m.Power, Busy: m.Busy, BusyMax: m.BusyMax,
				Rx: m.Rx, Tx: m.Tx, Noise: m.Noise, Clients: m.Clients, ClientsMax: m.ClientsMax, Counts: m.Counts})
		}
		out["radios"] = radios
	}
	if mac != "" {
		rows, err := s.conds.ClientMinutes(mac, since, aps)
		if err != nil {
			return err
		}
		pts := make([]clientPoint, 0, len(rows))
		for _, m := range rows {
			pts = append(pts, clientPoint{ClientMinute: m, APName: name(m.AP)})
		}
		hosts, err := s.conds.Hosts([]string{mac}, aps)
		if err != nil {
			return err
		}
		cl := map[string]any{"mac": mac, "minutes": pts}
		if h := hosts[mac]; h != "" {
			cl["host"] = h
		}
		out["client"] = cl
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
