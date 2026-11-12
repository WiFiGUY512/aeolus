package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

const (
	// apcKeep is how long power control's steps are kept (0206).
	apcKeep = 30 * 24 * time.Hour
	// powerSteps is how many steps are listed, the latest.
	powerSteps = 500
)

// ok says whether a step of power control is well said (0206): when, a
// band, from and to in dBm, why, and what its clients did.
func (e apcEvent) ok() bool {
	return e.At > 0 && bands[e.Band] && power(e.From) && power(e.To) && apcWhys[e.Why] &&
		(e.Scanning == nil || (*e.Scanning >= 0 && *e.Scanning <= 256)) && (e.Left == nil || (*e.Left >= 0 && *e.Left <= 4096)) && ratios(e.Ratio)
}

// fresh says whether a step is kept: one said to be from a time yet to
// come is of a clock not yet set, and one from before the steps are kept
// is let go.
func (e apcEvent) fresh(now time.Time) bool {
	at := time.Unix(e.At, 0)
	return !at.After(now.Add(steerClock)) && !at.Before(now.Add(-apcKeep))
}

// kept is a step as it is kept.
func (e apcEvent) kept() conditions.APCEvent {
	return conditions.APCEvent{At: time.Unix(e.At, 0), Band: e.Band, From: e.From, To: e.To, Why: e.Why, Scanning: e.Scanning, Left: e.Left, Ratio: e.Ratio}
}

// powerLonely is a lonely client (0206) as its AP says, with its name
// where the manager knows one.
type powerLonely struct {
	apcLonely
	Host string `json:"host,omitempty"`
}

// powerRadio is a radio under power control (0206), as its AP last said.
type powerRadio struct {
	rrmPower
	Lonely []powerLonely `json:"lonely"`
}

// powerAP is one AP's power control (0206): whether it is on, what it does
// for clients no other AP hears better (guard, ignore, or ask, the
// default), the signal such a client should hear it at, the hours a step
// back holds, and the radios it holds as of the AP's last report.
type powerAP struct {
	AP          hierarchy.NodeID `json:"ap"`
	Name        string           `json:"name"`
	On          bool             `json:"on"`
	Clients     string           `json:"clients"`
	ClientFloor int              `json:"client_floor"`
	Hold        int              `json:"hold"`
	Reported    *time.Time       `json:"reported,omitempty"`
	Radios      []powerRadio     `json:"radios"`
}

// powerStep is one of power control's steps, with its AP's name.
type powerStep struct {
	conditions.APCEvent
	APName string `json:"ap_name"`
}

// powerView is power control over the APs the caller may view: GET
// /v1/power.
type powerView struct {
	Hours int         `json:"hours"`
	APs   []powerAP   `json:"aps"`
	Steps []powerStep `json:"steps"`
}

// power is power control on the APs the caller may view (0077, 0206),
// below one Locations node where ?under= names it: each AP's settings and
// radios as it last said, and the steps of ?hours= (24 unless set, at most
// 720), the latest first.
func (s *Server) power(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	hours := 24
	if v := q.Get("hours"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 720 {
			return badRequest("hours: from 1 to 720")
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
	view := powerView{Hours: hours, APs: []powerAP{}, Steps: []powerStep{}}
	var macs []string
	for _, id := range aps {
		a := powerSettings(c.state, id)
		a.Name = name(id)
		st, err := s.conds.LatestState(id)
		if err != nil {
			return err
		}
		if st != nil {
			var rep struct {
				RRM *struct {
					APC []rrmPower `json:"apc"`
				} `json:"rrm"`
			}
			if json.Unmarshal(st.Report, &rep) == nil && rep.RRM != nil {
				at := st.At
				a.Reported = &at
				for _, p := range rep.RRM.APC {
					pr := powerRadio{rrmPower: p, Lonely: []powerLonely{}}
					for _, l := range p.Lonely {
						pr.Lonely = append(pr.Lonely, powerLonely{apcLonely: l})
						macs = append(macs, l.MAC)
					}
					pr.rrmPower.Lonely = nil
					a.Radios = append(a.Radios, pr)
				}
			}
		}
		view.APs = append(view.APs, a)
	}
	slices.Sort(macs)
	hosts, err := s.conds.Hosts(slices.Compact(macs), aps)
	if err != nil {
		return err
	}
	for i := range view.APs {
		for j := range view.APs[i].Radios {
			for k, l := range view.APs[i].Radios[j].Lonely {
				view.APs[i].Radios[j].Lonely[k].Host = hosts[l.MAC]
			}
		}
	}
	steps, err := s.conds.APCEvents(time.Now().Add(-time.Duration(hours)*time.Hour), aps, powerSteps)
	if err != nil {
		return err
	}
	for _, e := range steps {
		view.Steps = append(view.Steps, powerStep{APCEvent: e, APName: name(e.AP)})
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}

// powerSettings is an AP's power control settings, the defaults where it
// sets none.
func powerSettings(state *change.State, id hierarchy.NodeID) powerAP {
	a := powerAP{AP: id, Clients: "ask", ClientFloor: -80, Hold: 24, Radios: []powerRadio{}}
	cfg, err := state.Org.ResolveAP(id)
	if err != nil {
		return a
	}
	if r, ok := cfg.Location["apc.enabled"]; ok {
		a.On, _ = r.Value.(bool)
	}
	if r, ok := cfg.Location["apc.clients"]; ok {
		if v, _ := r.Value.(string); v != "" {
			a.Clients = v
		}
	}
	if r, ok := cfg.Location["apc.client_floor"]; ok {
		if v, ok := whole(r.Value); ok {
			a.ClientFloor = v
		}
	}
	if r, ok := cfg.Location["apc.hold"]; ok {
		if v, ok := whole(r.Value); ok {
			a.Hold = v
		}
	}
	return a
}
