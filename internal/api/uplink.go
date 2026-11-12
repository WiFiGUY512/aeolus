package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// An AP's uplink, a minute at a time (0351): what each of its uplink's
// Ethernet ports carried each way, on average and at its busiest five
// seconds, against its link's speed, with its errors and its link lost and
// back. GET /v1/aps/{ap}/uplink?hours=N, 24 unless set, at most
// uplinkHours; over many hours, a point stands for several minutes, at most
// uplinkPoints a port.

const (
	uplinkHours  = 72
	uplinkPoints = 720
)

// uplinkPoint is one point of a port's chart: the minute it starts at, and
// over the minutes it stands for, its link as last said, its average Mbit/s
// each way, its busiest five seconds each way, and its errors and changes
// of link together.
type uplinkPoint struct {
	At      time.Time `json:"at"`
	Minutes int       `json:"minutes"`
	Up      bool      `json:"up"`
	Speed   *int      `json:"speed"`
	Full    *bool     `json:"full"`
	Rx      *float64  `json:"rx"`
	Tx      *float64  `json:"tx"`
	RxMax   *float64  `json:"rx_max"`
	TxMax   *float64  `json:"tx_max"`
	Errors  int64     `json:"errors"`
	Changes int64     `json:"changes"`
}

type uplinkChart struct {
	Port   string        `json:"port"`
	Points []uplinkPoint `json:"points"`
}

// uplinkPointsOf is a port's minutes, the oldest first, as at most most
// points: each step minutes, in order. The averages are over the seconds
// each minute counted; the busiest is the busiest of any.
func uplinkPointsOf(ms []conditions.UplinkMinute, most int) []uplinkPoint {
	step := max((len(ms)+most-1)/max(most, 1), 1)
	out := []uplinkPoint{}
	for i := 0; i < len(ms); i += step {
		group := ms[i:min(i+step, len(ms))]
		last := group[len(group)-1]
		p := uplinkPoint{At: group[0].At, Minutes: len(group), Up: last.Up, Speed: last.Speed, Full: last.Full}
		var secs int
		var rx, tx float64
		haveRx, haveTx := false, false
		for _, m := range group {
			if m.Rx != nil && m.Secs > 0 {
				rx += *m.Rx * float64(m.Secs)
				haveRx = true
			}
			if m.Tx != nil && m.Secs > 0 {
				tx += *m.Tx * float64(m.Secs)
				haveTx = true
			}
			if m.Rx != nil || m.Tx != nil {
				secs += m.Secs
			}
			for _, w := range []struct{ from, to **float64 }{{&m.RxMax, &p.RxMax}, {&m.TxMax, &p.TxMax}} {
				if *w.from != nil && (*w.to == nil || **w.from > **w.to) {
					v := **w.from
					*w.to = &v
				}
			}
			p.Errors += m.Errors
			p.Changes += m.Changes
		}
		if secs > 0 {
			if haveRx {
				v := float64(int(rx/float64(secs)*10+0.5)) / 10
				p.Rx = &v
			}
			if haveTx {
				v := float64(int(tx/float64(secs)*10+0.5)) / 10
				p.Tx = &v
			}
		}
		out = append(out, p)
	}
	return out
}

func (s *Server) apUplink(w http.ResponseWriter, r *http.Request, c call) error {
	id := hierarchy.NodeID(r.PathValue("ap"))
	if err := apNode(c, id); err != nil {
		return err
	}
	hours, err := hoursOf(r, 24, uplinkHours)
	if err != nil {
		return err
	}
	ms, err := s.conds.UplinkMinutes(id, time.Now().Add(-time.Duration(hours)*time.Hour), hours*60*8+64)
	if err != nil {
		return err
	}
	ports := []uplinkChart{}
	for i := 0; i < len(ms); {
		j := i
		for j < len(ms) && ms[j].Port == ms[i].Port {
			j++
		}
		ports = append(ports, uplinkChart{Port: ms[i].Port, Points: uplinkPointsOf(ms[i:j], uplinkPoints)})
		i = j
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(map[string]any{"ap": id, "hours": hours, "ports": ports})
}
