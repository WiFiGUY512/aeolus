package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The noise floor of each AP's radios, where the APs tab can show it under
// each band's channel (Griff, 2026-10-25: "the noise floor for each of the
// bands should be seen somewhere ... I want it to be representative"): what
// each radio measured on its own channel over its latest minutes (0207),
// the middle of them, so one loud minute says nothing alone.
const (
	// noiseMinutes is how many of the latest minutes a floor is the middle
	// of.
	noiseMinutes = 15
	// noiseThermal is thermal noise in 20 MHz, about -101 dBm: a floor
	// below it is no measurement, but what a driver says that has none, as
	// ath11k before port 2a240ea and the R7800's ath10k do.
	noiseThermal = -101
	// noiseLoud is a floor above which the channel is loud with what is no
	// Wi-Fi: above -85 dBm.
	noiseLoud = -85
)

// noiseFloor is one radio's floor: the middle of its minutes' readings, in
// dBm, the lowest and the highest of them, and how many there were.
// Unmeasured is a middle below thermal noise; Loud one above noiseLoud.
type noiseFloor struct {
	Band       string `json:"band"`
	DBm        int    `json:"dbm"`
	Low        int    `json:"low"`
	High       int    `json:"high"`
	Minutes    int    `json:"minutes"`
	Unmeasured bool   `json:"unmeasured,omitempty"`
	Loud       bool   `json:"loud,omitempty"`
}

// noise is the noise floor of each radio of the APs the caller may view,
// below one Locations node where ?under= names it: GET /v1/noise. By AP,
// then radio; a radio that said none in the time is not there.
func (s *Server) noise(w http.ResponseWriter, r *http.Request, c call) error {
	rows, err := s.conds.RadioMinutes(time.Now().Add(-noiseMinutes*time.Minute), mayView(c, hierarchy.NodeID(r.URL.Query().Get("under"))), "")
	if err != nil {
		return err
	}
	type key struct {
		ap    hierarchy.NodeID
		radio string
	}
	read, band := map[key][]int{}, map[key]string{}
	for _, m := range rows {
		if m.Noise == nil {
			continue
		}
		k := key{m.AP, m.Radio}
		read[k] = append(read[k], *m.Noise)
		band[k] = m.Band
	}
	out := map[hierarchy.NodeID]map[string]noiseFloor{}
	for k, v := range read {
		sort.Ints(v)
		f := noiseFloor{Band: band[k], DBm: v[len(v)/2], Low: v[0], High: v[len(v)-1], Minutes: len(v)}
		f.Unmeasured = f.DBm < noiseThermal
		f.Loud = !f.Unmeasured && f.DBm > noiseLoud
		if out[k.ap] == nil {
			out[k.ap] = map[string]noiseFloor{}
		}
		out[k.ap][k.radio] = f
	}
	writeJSON(w, http.StatusOK, map[string]any{"minutes": noiseMinutes, "thermal": noiseThermal, "loud_over": noiseLoud, "aps": out})
	return nil
}
