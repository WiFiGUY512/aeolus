package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// 802.11r's key holders (0224). Each BSS of a network with 802.11r is one,
// by its BSSID and a NAS-Identifier of its own, and says so in its AP's
// state report. The manager hands each AP every key holder of its networks
// on its site, with a key for each network that seals what they say to each
// other, beside the config, so the config versions nothing by it. hostapd
// pushes a client's keys to them over the wire as the client signs in, and
// asks the one it signed in with for them where none came.

// FTHeader names, in every poll's answer, the version of the key holders an
// AP is to have: where it is not the one it has, it asks for them.
const FTHeader = "Aeolus-FT"

// ftStale is how long an AP's key holders are kept after its last report
// says them: an AP gone that long is taken off the others' lists.
const ftStale = 24 * time.Hour

// ftBSS is a BSS of a network with 802.11r, as the AP running it says
// (0224): the network, its BSSID, and the NAS-Identifier it is a key holder
// by.
type ftBSS struct {
	Network string `json:"network"`
	BSSID   string `json:"bssid"`
	NASID   string `json:"nasid"`
}

var ftNASIDRE = regexp.MustCompile(`^[!-~]{1,48}$`)

func (b ftBSS) ok() bool {
	return networkIDRE.MatchString(b.Network) && macRE.MatchString(b.BSSID) && ftNASIDRE.MatchString(b.NASID)
}

// cleanFT keeps the BSSes that are well said, at most 64, each once.
func cleanFT(list []ftBSS) []ftBSS {
	list = slices.DeleteFunc(list, func(b ftBSS) bool { return !b.ok() })
	slices.SortFunc(list, func(a, b ftBSS) int { return strings.Compare(a.Network+" "+a.BSSID, b.Network+" "+b.BSSID) })
	list = slices.CompactFunc(list, func(a, b ftBSS) bool { return a.Network == b.Network && a.BSSID == b.BSSID })
	if len(list) > 64 {
		list = list[:64]
	}
	return list
}

// ftBook keeps each AP's key holders as its latest state report says them,
// so a poll need not read every AP's report. An AP not yet in it is read
// from its report once.
type ftBook struct {
	mu  sync.Mutex
	aps map[hierarchy.NodeID]ftSaid
}

type ftSaid struct {
	at  time.Time
	bss []ftBSS
}

// ftNote keeps what an AP's state report says of its key holders.
func (s *Server) ftNote(ap hierarchy.NodeID, at time.Time, bss []ftBSS) {
	s.ft.mu.Lock()
	defer s.ft.mu.Unlock()
	if s.ft.aps == nil {
		s.ft.aps = map[hierarchy.NodeID]ftSaid{}
	}
	s.ft.aps[ap] = ftSaid{at, bss}
}

// ftOf is what an AP last said of its key holders.
func (s *Server) ftOf(ap hierarchy.NodeID) ftSaid {
	s.ft.mu.Lock()
	said, ok := s.ft.aps[ap]
	s.ft.mu.Unlock()
	if ok {
		return said
	}
	if st, err := s.conds.LatestState(ap); err == nil && st != nil {
		var r struct {
			FT []ftBSS `json:"ft"`
		}
		if json.Unmarshal(st.Report, &r) == nil {
			said = ftSaid{st.At, cleanFT(r.FT)}
		}
	}
	s.ftNote(ap, said.at, said.bss)
	return said
}

// ftSite is the part of the locations tree an AP's key holders are of: the
// folder right below the Org's root that it is in. Clients roam within a
// site, and no further.
func ftSite(t *hierarchy.Tree, ap hierarchy.NodeID) hierarchy.NodeID {
	if a := t.Ancestry(ap); len(a) >= 2 {
		return a[1]
	}
	return ap
}

// ftNetwork is one network's key holders as an AP is given them: the key
// their messages are sealed with, and every BSS of the network on the site.
type ftNetwork struct {
	Key     string  `json:"key"`
	Holders []ftBSS `json:"holders"`
}

// ftHolders is, by network, the key holders of the AP's site, from the
// latest reports of its APs that are not in Landing Zone, and their
// version: a hash of them all.
func (s *Server) ftHolders(state *change.State, ap hierarchy.NodeID, now time.Time) (map[string]ftNetwork, string) {
	t := state.Org.Locations
	site := ftSite(t, ap)
	out := map[string]ftNetwork{}
	for _, id := range t.APs() {
		if ftSite(t, id) != site || t.InIsolated(id) {
			continue
		}
		said := s.ftOf(id)
		if now.Sub(said.at) > ftStale {
			continue
		}
		for _, b := range said.bss {
			n := out[b.Network]
			n.Holders = append(n.Holders, b)
			out[b.Network] = n
		}
	}
	for id, n := range out {
		slices.SortFunc(n.Holders, func(a, b ftBSS) int { return strings.Compare(a.BSSID, b.BSSID) })
		n.Key = hex.EncodeToString(s.box.Derive("ft/" + id))
		out[id] = n
	}
	raw, _ := json.Marshal(out) // a map's keys come sorted
	sum := sha256.Sum256(raw)
	return out, hex.EncodeToString(sum[:16])
}

// setFTHeader names, on a poll's answer, the version of the key holders
// the AP is to have (0224). An AP in Landing Zone has none.
func (s *Server) setFTHeader(w http.ResponseWriter, state *change.State, ap hierarchy.NodeID) {
	if unadopted(state, ap) != nil {
		return
	}
	if _, version := s.ftHolders(state, ap, time.Now()); version != "" {
		w.Header().Set(FTHeader, version)
	}
}

// apFT hands an AP its site's key holders (0224).
func (s *Server) apFT(w http.ResponseWriter, r *http.Request, c apCall) error {
	state := s.log.Snapshot()
	if err := unadopted(state, c.ap); err != nil {
		return err
	}
	nets, version := s.ftHolders(state, c.ap, time.Now())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"version": version, "networks": nets})
	return nil
}
