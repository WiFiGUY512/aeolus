package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What airscan is to trust (0304, F3). airscan, the survey the Arista APs'
// scan radio runs, trusts a BSS that is the AP's own, or one its config's
// trusted_bssid names. A BSS with one of the AP's own SSIDs, or one
// trusted_ssid names, is "ours": where it relays airscan's probe over the
// wire it is verified, and where it never does, an evil twin. A BSS of any
// other SSID that relays it is a wired rogue. Left to itself, airscan
// knows no other AP's BSSIDs, nor the SSIDs only other APs serve, so it
// called the fleet's own BSSes rogues. The manager hands each AP what the
// fleet calls its own, beside the config, so the config versions nothing
// by it: every BSSID an AP's latest report names as its own, those the
// AP's folders know (rogues.known), and the SSID of every network.

// TrustHeader names, in every poll's answer, the version of what the AP's
// airscan is to trust: where it is not the one it has, it asks for it.
const TrustHeader = "Aeolus-Trust"

// trustFresh is how long what the fleet calls its own is kept before it is
// read from the APs' reports again: a poll need not read every AP's.
const trustFresh = time.Minute

// trustMost is the most of each list an AP is given.
const trustMost = 512

// trustDoc is what an AP's airscan is to trust.
type trustDoc struct {
	SSIDs  []string `json:"ssids"`
	BSSIDs []string `json:"bssids"`
}

// trustFleet keeps what the fleet calls its own, read at most every
// trustFresh, for the state it was read of.
type trustFleet struct {
	mu    sync.Mutex
	at    time.Time
	of    *change.State
	ssids []string
	bss   []string
}

// fleetTrust is what the fleet calls its own: the SSID of every network,
// and every BSSID an AP's latest report names as its own, each sorted.
func (s *Server) fleetTrust(state *change.State, now time.Time) (ssids, bssids []string) {
	s.trust.mu.Lock()
	defer s.trust.mu.Unlock()
	if s.trust.of == state && now.Sub(s.trust.at) < trustFresh {
		return s.trust.ssids, s.trust.bss
	}
	seen := map[string]bool{}
	state.Org.Services.EachSet(func(_ hierarchy.NodeID, p hierarchy.Path, v hierarchy.Value) {
		if ssid, ok := v.(string); ok && ssid != "" && strings.HasPrefix(string(p), "network.") && strings.HasSuffix(string(p), ".ssid") && !seen[ssid] {
			seen[ssid] = true
			ssids = append(ssids, ssid)
		}
	})
	for b := range s.fleetBSSIDs(state) {
		if b = strings.ToLower(b); macRE.MatchString(b) {
			bssids = append(bssids, b)
		}
	}
	slices.Sort(ssids)
	slices.Sort(bssids)
	bssids = slices.Compact(bssids)
	s.trust.at, s.trust.of, s.trust.ssids, s.trust.bss = now, state, ssids, bssids
	return ssids, bssids
}

// trustOf is what an AP's airscan is to trust, and its version: a hash of
// it. An AP's folders' known BSSIDs are its own to add.
func (s *Server) trustOf(state *change.State, ap hierarchy.NodeID, now time.Time) (trustDoc, string) {
	ssids, fleet := s.fleetTrust(state, now)
	bssids := append([]string{}, fleet...)
	for b := range knownRogues(state, ap) {
		if macRE.MatchString(b) {
			bssids = append(bssids, b)
		}
	}
	slices.Sort(bssids)
	bssids = slices.Compact(bssids)
	doc := trustDoc{SSIDs: append([]string{}, ssids...), BSSIDs: bssids}
	if len(doc.SSIDs) > trustMost {
		doc.SSIDs = doc.SSIDs[:trustMost]
	}
	if len(doc.BSSIDs) > trustMost {
		doc.BSSIDs = doc.BSSIDs[:trustMost]
	}
	raw, _ := json.Marshal(doc)
	sum := sha256.Sum256(raw)
	return doc, hex.EncodeToString(sum[:16])
}

// setTrustHeader names, on a poll's answer, the version of what the AP's
// airscan is to trust (0304). An AP in Landing Zone is given nothing.
func (s *Server) setTrustHeader(w http.ResponseWriter, state *change.State, ap hierarchy.NodeID) {
	if unadopted(state, ap) != nil {
		return
	}
	_, version := s.trustOf(state, ap, time.Now())
	w.Header().Set(TrustHeader, version)
}

// apTrust hands an AP what its airscan is to trust (0304).
func (s *Server) apTrust(w http.ResponseWriter, r *http.Request, c apCall) error {
	state := s.log.Snapshot()
	if err := unadopted(state, c.ap); err != nil {
		return err
	}
	doc, version := s.trustOf(state, c.ap, time.Now())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"version": version, "ssids": doc.SSIDs, "bssids": doc.BSSIDs})
	return nil
}
