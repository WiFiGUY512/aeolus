package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The flight recorder (0207 part 3). Each AP keeps what each five seconds
// came to over the last half hour; when something goes wrong, or someone
// asks, it sends the ten minutes before and the ten after on its line,
// with the recording they belong to. The manager keeps them a month.

const (
	// recordingsKeep is how long recordings are kept.
	recordingsKeep = 30 * 24 * time.Hour
	// recordingLongest is the longest stretch one recording may cover.
	recordingLongest = 2 * time.Hour
	// recordingsListed is how many recordings a listing gives, the latest.
	recordingsListed = 200
)

// recordWhys is why a recording was made: on a radio, what was sent to its
// clients tried again or lost all at once, many of them gone at once, its
// channel pinned full; power control's clients took a step badly; or
// someone asked.
var recordWhys = map[string]bool{"retries": true, "clients-left": true, "busy": true, "distress": true, "asked": true,
	// Every radio down at once, and back: the Wi-Fi restarted, or its
	// driver loaded again (Griff, 2026-10-23).
	"radios-down": true, "radios-restart": true}

// apRecord is a recording going on, as an AP says it with its points: when
// what went wrong did, why, what it said of it, and the stretch it covers,
// in seconds since 1970.
type apRecord struct {
	At    int64  `json:"at"`
	Why   string `json:"why"`
	Said  string `json:"said"`
	From  int64  `json:"from"`
	Until int64  `json:"until"`
}

// liveRecord is what an AP is told to record now: the seconds before and
// after, and what was said of why.
type liveRecord struct {
	Said   string `json:"said,omitempty"`
	Before int    `json:"before"`
	After  int    `json:"after"`
}

// slimPoint is a point's radios and clients as they are sent and kept,
// without what the point says already; host names the clients, where
// given.
func slimPoint(p livePoint, host func(string) string) ([]liveRadio, []liveClient) {
	radios, clients := make([]liveRadio, 0, len(p.Radios)), make([]liveClient, 0, len(p.Clients))
	for _, m := range p.Radios {
		radios = append(radios, liveRadio{Radio: m.Radio, Band: m.Band, minutePoint: minutePoint{At: m.At, Channel: m.Channel, Width: m.Width, Power: m.Power,
			Busy: m.Busy, BusyMax: m.BusyMax, Rx: m.Rx, Tx: m.Tx, Noise: m.Noise, Clients: m.Clients, ClientsMax: m.ClientsMax, Counts: m.Counts}})
	}
	for _, m := range p.Clients {
		c := liveClient{MAC: m.MAC, Band: m.Band, SSID: m.SSID, Signal: m.Signal, SignalMin: m.SignalMin,
			TxRate: m.TxRate, RxRate: m.RxRate, Secs: m.Secs, Heard: m.Heard, Counts: m.Counts}
		if host != nil {
			c.Host = host(m.MAC)
		}
		clients = append(clients, c)
	}
	return radios, clients
}

// keepRecording keeps what an AP said of a recording going on, and those
// of its points that fall in one of its recordings.
func (s *Server) keepRecording(ap hierarchy.NodeID, rec *apRecord, pts []livePoint, now time.Time) {
	if rec != nil {
		at, from, until := time.Unix(rec.At, 0).UTC(), time.Unix(rec.From, 0).UTC(), time.Unix(rec.Until, 0).UTC()
		if !recordWhys[rec.Why] || len(rec.Said) > 200 || !printable(rec.Said) || from.After(at) || at.After(until) || until.Sub(from) > recordingLongest ||
			at.Before(now.Add(-recordingLongest)) || at.After(now.Add(minutesAhead)) {
			slog.Warn("a recording not well said", "ap", ap, "why", rec.Why)
		} else if err := s.conds.OpenRecording(ap, at, rec.Why, rec.Said, from, until); err != nil {
			slog.Warn("a recording not kept", "ap", ap, "err", err)
		}
	}
	if len(pts) == 0 {
		return
	}
	data := map[time.Time][]byte{}
	for _, p := range pts {
		radios, clients := slimPoint(p, nil)
		b, err := json.Marshal(map[string]any{"radios": radios, "clients": clients})
		if err == nil {
			data[p.At] = b
		}
	}
	if _, err := s.conds.RecordPoints(ap, data); err != nil {
		slog.Warn("a recording's points not kept", "ap", ap, "err", err)
	}
}

// recordingView is a recording as it is read, with its AP's name.
type recordingView struct {
	conditions.Recording
	Name string `json:"ap_name"`
}

// recordingsList is the flight recorder's recordings of the APs the caller
// may view (0207 part 3), below one Locations node where ?under= names it,
// of the last ?hours= (168 unless set, at most 720): GET /v1/recordings,
// the latest first, at most 200.
func (s *Server) recordingsList(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	hours := 168
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours: from 1 to 720")
		}
		hours = n
	}
	aps := mayView(c, hierarchy.NodeID(q.Get("under")))
	recs, err := s.conds.Recordings(time.Now().Add(-time.Duration(hours)*time.Hour), aps, recordingsListed)
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	out := make([]recordingView, 0, len(recs))
	for _, rec := range recs {
		v := recordingView{Recording: rec, Name: string(rec.AP)}
		if n, ok := t.Node(rec.AP); ok {
			v.Name = n.Name
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "recordings": out})
	return nil
}

// recordingRead is one recording with its points, the oldest first, each
// {at, radios, clients} as GET /v1/live has them: GET /v1/recordings/{id}.
// It needs viewer on its AP.
func (s *Server) recordingRead(w http.ResponseWriter, r *http.Request, c call) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return badRequest("id: a recording's number")
	}
	rec, pts, err := s.conds.RecordingOf(id)
	if errors.Is(err, conditions.ErrNoRecording) {
		return &apiError{http.StatusNotFound, "no such recording"}
	}
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	if !may(c, change.Locations, t, rec.AP, access.View) {
		return &apiError{http.StatusNotFound, "no such recording"}
	}
	v := recordingView{Recording: rec, Name: string(rec.AP)}
	if n, ok := t.Node(rec.AP); ok {
		v.Name = n.Name
	}
	// The clients by the names they gave.
	macs := map[string]bool{}
	type point struct {
		At      time.Time    `json:"at"`
		Radios  []liveRadio  `json:"radios"`
		Clients []liveClient `json:"clients"`
	}
	points := make([]point, 0, len(pts))
	for _, p := range pts {
		var d point
		if json.Unmarshal(p.Data, &d) != nil {
			continue
		}
		d.At = p.At
		for _, cl := range d.Clients {
			macs[cl.MAC] = true
		}
		points = append(points, d)
	}
	list := make([]string, 0, len(macs))
	for m := range macs {
		list = append(list, m)
	}
	hosts, err := s.conds.Hosts(list, []hierarchy.NodeID{rec.AP})
	if err != nil {
		return err
	}
	for i := range points {
		for j := range points[i].Clients {
			points[i].Clients[j].Host = hosts[points[i].Clients[j].MAC]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"recording": v, "points": points})
	return nil
}

// recordNow asks the APs below a Locations node to record what is going on
// now (0207 part 3): POST /v1/recordings, {under, before?, after?, said?},
// the seconds before and after (600 each unless said, at most 1800). Each
// AP the caller may operate is told on its line; one without a line is
// not. The answer is each AP asked, {ap, ap_name, line}.
func (s *Server) recordNow(w http.ResponseWriter, r *http.Request, c call) error {
	var body struct {
		Under  string `json:"under"`
		Before int    `json:"before"`
		After  int    `json:"after"`
		Said   string `json:"said"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Before < 0 || body.Before > 1800 || body.After < 0 || body.After > 1800 || len(body.Said) > 120 || !printable(body.Said) {
		return badRequest("before and after: seconds, at most 1800; said: at most 120 printable characters")
	}
	if body.Before == 0 {
		body.Before = 600
	}
	if body.After == 0 {
		body.After = 600
	}
	if body.Said == "" {
		body.Said = "asked by " + string(c.actor)
	}
	t := c.state.Org.Locations
	b, _ := json.Marshal(liveSay{Now: msOf(time.Now()), Record: &liveRecord{Said: body.Said, Before: body.Before, After: body.After}})
	b = append(b, '\n')
	asked := []liveAP{}
	for _, ap := range mayView(c, hierarchy.NodeID(body.Under)) {
		if !may(c, change.Locations, t, ap, access.Clients) {
			continue
		}
		a := liveAP{AP: ap, Name: string(ap), Line: s.live.tellOne(ap, b)}
		if n, ok := t.Node(ap); ok {
			a.Name = n.Name
		}
		asked = append(asked, a)
	}
	if len(asked) == 0 {
		return &apiError{http.StatusForbidden, "recording needs the operator role on an AP there"}
	}
	writeJSON(w, http.StatusOK, map[string]any{"asked": asked})
	return nil
}
