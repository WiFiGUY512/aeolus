package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Each radio's beacons over time, for the Radios page (0350), from what the
// manager keeps of each state report (0339).

// beaconsHours is the longest span GET /v1/beacons reads, a week; the
// reports' rows are kept longer (30 days unless set), but a week of
// half-hours is as many as a chart shows well.
const beaconsHours = 168

// beaconsSteps are the stretches a radio's beacons are summed over: the
// shortest of them that keeps a span to beaconsPoints, else the last. Each
// divides the beacons alert's half hour, so the half hour that ends with
// each stretch can be judged as the alert judges it. An hour to 12 are
// 5 minutes, the APs' report interval; a day 10; a week 30.
var beaconsSteps = []time.Duration{5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute}

const beaconsPoints = 144

// One asking reads the beacons of at most beaconsAPs APs, the rest left
// out and said to be (cut), and at most beaconsRows stretches: a week of
// half-hours is 342 a radio, with the half hour before the span.
const (
	beaconsAPs  = 50
	beaconsRows = 40000
)

// beaconsStep is the stretch a span of hours is summed over.
func beaconsStep(hours int) time.Duration {
	span := time.Duration(hours) * time.Hour
	for _, st := range beaconsSteps {
		if span/st <= beaconsPoints {
			return st
		}
	}
	return beaconsSteps[len(beaconsSteps)-1]
}

// beaconsPoint is a radio's beacons over one stretch: at its start, those
// sent and failed, over how many seconds; and alert where the half hour
// that ends with it is an alert, as the beacons alert judges one.
type beaconsPoint struct {
	At     time.Time `json:"at"`
	Sent   int64     `json:"sent"`
	Failed int64     `json:"failed"`
	Over   int       `json:"over"`
	Alert  bool      `json:"alert,omitempty"`
}

// beaconsHalf is a radio's last half hour, as the beacons alert reads it
// now: the share, in percent, it is an alert at, and whether it is.
type beaconsHalf struct {
	Sent    int64 `json:"sent"`
	Failed  int64 `json:"failed"`
	Over    int   `json:"over"`
	AlertAt int64 `json:"alert_at"`
	Alert   bool  `json:"alert"`
}

// beaconsRadio is one radio's beacons over the span: its totals, its last
// half hour (nil with no report in it) and its stretches, the oldest
// first, those with a report alone.
type beaconsRadio struct {
	AP     hierarchy.NodeID `json:"ap"`
	Name   string           `json:"ap_name"`
	Radio  string           `json:"radio"`
	Band   string           `json:"band"`
	Sent   int64            `json:"sent"`
	Failed int64            `json:"failed"`
	Over   int              `json:"over"`
	Half   *beaconsHalf     `json:"half"`
	Points []beaconsPoint   `json:"points"`
}

// beaconsLine is a radio's stretches by number (got), as the span shows
// them: those from first to first+count, each judged over the k stretches
// that end with it, the half hour; at is a stretch's start. It answers them
// and what they sum to.
func beaconsLine(got map[int]conditions.RadioBeacons, first, count, k int, at func(int) time.Time) ([]beaconsPoint, conditions.RadioBeacons) {
	out := []beaconsPoint{}
	var sum conditions.RadioBeacons
	for i := first; i < first+count; i++ {
		b, ok := got[i]
		if !ok {
			continue
		}
		var half conditions.RadioBeacons
		for j := i - k + 1; j <= i; j++ {
			h := got[j]
			half.Sent += h.Sent
			half.Failed += h.Failed
			half.Over += h.Over
		}
		_, alert := alerts.BeaconsJudged(half)
		out = append(out, beaconsPoint{At: at(i), Sent: b.Sent, Failed: b.Failed, Over: b.Over, Alert: alert})
		sum.Sent += b.Sent
		sum.Failed += b.Failed
		sum.Over += b.Over
	}
	return out, sum
}

// beaconsView is each radio's beacons over time (0350), on the APs the
// caller may view, below one Locations node where ?under= names it, on
// ?band= alone where it is set, over the last ?hours= (24 unless set, at
// most 168): GET /v1/beacons. The span is in stretches of step seconds on
// the clock, from from, the last not over yet; and each radio has its last
// half hour as the beacons alert reads it now, and the rule it is read by.
func (s *Server) beaconsView(w http.ResponseWriter, r *http.Request, c call) error {
	q := r.URL.Query()
	band := q.Get("band")
	if band != "" && !bands[band] {
		return badRequest("band: 2g, 5g or 6g")
	}
	hours, err := hoursOf(r, 24, beaconsHours)
	if err != nil {
		return err
	}
	aps := mayView(c, hierarchy.NodeID(q.Get("under")))
	cut := len(aps) > beaconsAPs
	if cut {
		aps = aps[:beaconsAPs]
	}
	step := beaconsStep(hours)
	now := time.Now()
	end := now.Truncate(step).Add(step)
	count := int(time.Duration(hours) * time.Hour / step)
	from := end.Add(-time.Duration(count) * step)
	// The stretches before from that the half hours of the first end with.
	k := int(alerts.BeaconsWindow / step)
	since := from.Add(-time.Duration(k-1) * step)
	rows, err := s.conds.BeaconsOver(aps, band, since, step, beaconsRows)
	if err != nil {
		return err
	}
	// Read to the most: the last radio may be cut short, and is left out.
	if len(rows) == beaconsRows {
		cut = true
		last := rows[len(rows)-1]
		for len(rows) > 0 && rows[len(rows)-1].AP == last.AP && rows[len(rows)-1].Radio == last.Radio && rows[len(rows)-1].Band == last.Band {
			rows = rows[:len(rows)-1]
		}
	}
	// The last half hour, summed as the beacons alert sums it (0339).
	halves, err := s.conds.BeaconsOver(aps, band, now.Add(-alerts.BeaconsWindow), 0, beaconsRows)
	if err != nil {
		return err
	}
	type key struct{ ap, radio, band string }
	half := map[key]conditions.RadioBeacons{}
	for _, b := range halves {
		half[key{string(b.AP), b.Radio, b.Band}] = b.RadioBeacons
	}
	t := c.state.Org.Locations
	name := func(ap hierarchy.NodeID) string {
		if n, ok := t.Node(ap); ok {
			return n.Name
		}
		return string(ap)
	}
	at := func(i int) time.Time { return since.Add(time.Duration(i) * step).UTC() }
	radios := []beaconsRadio{}
	for i := 0; i < len(rows); {
		b := rows[i]
		got := map[int]conditions.RadioBeacons{}
		for ; i < len(rows) && rows[i].AP == b.AP && rows[i].Radio == b.Radio && rows[i].Band == b.Band; i++ {
			got[int(rows[i].At.Sub(since)/step)] = rows[i].RadioBeacons
		}
		pts, sum := beaconsLine(got, k-1, count, k, at)
		if len(pts) == 0 {
			continue
		}
		rd := beaconsRadio{AP: b.AP, Name: name(b.AP), Radio: b.Radio, Band: b.Band, Sent: sum.Sent, Failed: sum.Failed, Over: sum.Over, Points: pts}
		if h, ok := half[key{string(b.AP), b.Radio, b.Band}]; ok {
			pct, alert := alerts.BeaconsJudged(h)
			rd.Half = &beaconsHalf{Sent: h.Sent, Failed: h.Failed, Over: h.Over, AlertAt: pct, Alert: alert}
		}
		radios = append(radios, rd)
	}
	sort.SliceStable(radios, func(i, j int) bool {
		a, b := radios[i], radios[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Radio != b.Radio {
			return a.Radio < b.Radio
		}
		return a.Band < b.Band
	})
	out := map[string]any{
		"hours": hours, "from": from.UTC(), "step": int(step / time.Second), "radios": radios,
		"rule": map[string]any{
			"pct": alerts.BeaconsFailingPct, "stuck_pct": alerts.BeaconsStuckPct, "fewest": alerts.BeaconsFewest,
			"window": int(alerts.BeaconsWindow / time.Second), "span": int(alerts.BeaconsSpan / time.Second),
		},
	}
	if cut {
		out["cut"] = true
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
