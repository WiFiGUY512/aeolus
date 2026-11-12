package api

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/oui"
)

// How clients of one maker fare on each kind of AP (0193), so a device that
// does not get on with one kind of radio is seen as such, and not as one
// client's bad day: the IoT devices rejoined every 45 s on the C-360s'
// 2.4 GHz and held one session for days on the office AP.
const (
	// compatLooping is how many times an hour a client that rejoins comes
	// online at one AP for it to be looping there: as the rejoins alert
	// counts (0190).
	compatLooping = rejoinsFewest
	// compatSettled is how few times an hour a maker's clients come online
	// again where they stay: once in two hours.
	compatSettled = 0.5
	// compatWorst is how many of a group's clients are listed, the worst
	// first.
	compatWorst = 5
)

// compatClient is one client in a group: how often it came online again
// at an AP while it was there.
type compatClient struct {
	MAC     string           `json:"mac"`
	Host    string           `json:"host,omitempty"`
	AP      hierarchy.NodeID `json:"ap"`
	APName  string           `json:"ap_name"`
	Joins   int              `json:"joins"`
	PerHour float64          `json:"per_hour"`
}

// compatGroup is a maker's clients on one model of AP on one band.
// Trouble is where they loop here and not on other models; Everywhere,
// where they loop on other models too, which is no model's (0200).
type compatGroup struct {
	Maker      string         `json:"maker"`
	Private    bool           `json:"private,omitempty"`
	Model      string         `json:"model"`
	Band       string         `json:"band"`
	Clients    int            `json:"clients"`
	Attempts   int            `json:"attempts"`
	Online     int            `json:"online"`
	Failed     int            `json:"failed"`
	PerHour    float64        `json:"per_hour"` // the middle client's
	Looping    int            `json:"looping"`
	APs        []string       `json:"aps"`
	Worst      []compatClient `json:"worst"`
	Trouble    string         `json:"trouble,omitempty"`
	Everywhere string         `json:"everywhere,omitempty"`
	rates      []float64
}

// rejoinRate is how many times an hour a client came online again at an
// AP while it was there: none for once, else its joins after the first
// over the time from the first to the latest, at least a minute of it.
// Where it came online fewer times than a looping client must, as the
// rejoins alert counts (0190), that time is an hour at least: two joins a
// minute apart are one rejoin, not sixty an hour, and no loop (2026-10-24:
// a UPS monitor and a phone that rejoined once were said to loop).
func rejoinRate(j conditions.Joined) float64 {
	if j.Online < 2 {
		return 0
	}
	least := time.Minute
	if j.Online < compatLooping {
		least = time.Hour
	}
	span := max(j.Last.Sub(j.First), least)
	return float64(j.Online-1) / span.Hours()
}

// compatibility is how each maker's clients fared on each model of AP, a
// band at a time, at the APs the caller may view, ?under= a Locations
// node, over ?hours= (24 unless set, at most 720): GET /v1/compatibility
// (0193).
func (s *Server) compatibility(w http.ResponseWriter, r *http.Request, c call) error {
	hours, err := hoursOf(r, 24, 720)
	if err != nil {
		return err
	}
	aps := mayView(c, hierarchy.NodeID(r.URL.Query().Get("under")))
	list, err := s.conds.JoinedSince(time.Now().Add(-time.Duration(hours) * time.Hour))
	if err != nil {
		return err
	}
	t := c.state.Org.Locations
	own := testClients(c.state)
	type key struct{ maker, model, band string }
	groups := map[key]*compatGroup{}
	for _, j := range list {
		mac := strings.ToLower(j.MAC)
		if _, mine := own[mac]; mine || !slices.Contains(aps, j.AP) {
			continue
		}
		maker, private := oui.Lookup(mac)
		if maker == "" {
			maker = "unknown"
			if private {
				maker = "a private address"
			}
		}
		model := c.state.Model(j.AP)
		if model == "" {
			model = "unknown"
		}
		k := key{maker, model, j.Band}
		g := groups[k]
		if g == nil {
			g = &compatGroup{Maker: maker, Private: private, Model: model, Band: j.Band, APs: []string{}, Worst: []compatClient{}}
			groups[k] = g
		}
		g.Clients++
		g.Attempts += j.Attempts
		g.Online += j.Online
		g.Failed += j.Failed
		rate := rejoinRate(j)
		g.rates = append(g.rates, rate)
		if rate >= compatLooping {
			g.Looping++
		}
		name := string(j.AP)
		if n, ok := t.Node(j.AP); ok {
			name = n.Name
		}
		if !slices.Contains(g.APs, name) {
			g.APs = append(g.APs, name)
		}
		g.Worst = append(g.Worst, compatClient{MAC: mac, AP: j.AP, APName: name, Joins: j.Online, PerHour: rate})
	}
	out := []*compatGroup{}
	for _, g := range groups {
		sort.Float64s(g.rates)
		g.PerHour = g.rates[len(g.rates)/2]
		sort.Strings(g.APs)
		sort.SliceStable(g.Worst, func(a, b int) bool { return g.Worst[a].PerHour > g.Worst[b].PerHour })
		g.Worst = g.Worst[:min(len(g.Worst), compatWorst)]
		for i := range g.Worst {
			if cl, ok, err := s.conds.ClientOf(g.Worst[i].MAC); err == nil && ok {
				g.Worst[i].Host = cl.Host
			}
		}
		out = append(out, g)
	}
	// Trouble: a maker's clients loop on one kind of AP and band, and on
	// another kind they settle. Where they loop on another kind too, in the
	// same hours, the looping is not this kind's (0200): they loop wherever
	// they are, as the IoT devices did with their cloud connection
	// stalling beyond the APs, which is said, and is no trouble. A private
	// address is no one maker's.
	for _, g := range out {
		if g.Private || g.Maker == "unknown" || g.Looping == 0 {
			continue
		}
		var settled, looping []string
		for _, o := range out {
			if o.Maker != g.Maker || o.Model == g.Model {
				continue
			}
			if on := fmt.Sprintf("%s (%s)", o.Model, bandWords(o.Band)); o.Looping > 0 {
				looping = append(looping, on)
			} else if o.PerHour <= compatSettled {
				settled = append(settled, on)
			}
		}
		said := fmt.Sprintf("%d of its %d %s clients here came online again %s, as one that drops the connection itself", g.Looping, g.Clients, g.Maker, everyWords(g.Worst[0].PerHour))
		if len(looping) > 0 {
			sort.Strings(looping)
			g.Everywhere = said + ", and they do so on " + strings.Join(slices.Compact(looping), " and ") +
				" too: the looping is not this model's. They loop wherever they are, which points at the clients themselves or the path beyond the APs, such as their cloud connection stalling"
			continue
		}
		g.Trouble = said
		if len(settled) > 0 {
			sort.Strings(settled)
			g.Trouble += "; on " + strings.Join(slices.Compact(settled), " and ") + " they stay"
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if (out[a].Trouble != "") != (out[b].Trouble != "") {
			return out[a].Trouble != ""
		}
		if (out[a].Everywhere != "") != (out[b].Everywhere != "") {
			return out[a].Everywhere != ""
		}
		if out[a].Maker != out[b].Maker {
			return out[a].Maker < out[b].Maker
		}
		if out[a].Model != out[b].Model {
			return out[a].Model < out[b].Model
		}
		return out[a].Band < out[b].Band
	})
	writeJSON(w, http.StatusOK, map[string]any{"hours": hours, "groups": out})
	return nil
}

// everyWords says a rate an hour as how often: "about every 45 s".
func everyWords(perHour float64) string {
	if perHour <= 0 {
		return "once"
	}
	every := time.Duration(float64(time.Hour) / perHour).Round(time.Second)
	return "about every " + every.String()
}

// bandWords is a band as a person says it.
func bandWords(b string) string {
	switch b {
	case "2g":
		return "2.4 GHz"
	case "5g":
		return "5 GHz"
	case "6g":
		return "6 GHz"
	}
	return b
}
