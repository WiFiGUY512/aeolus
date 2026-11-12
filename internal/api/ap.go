package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The routes APs use (0033, 0038). An AP authenticates with its own token,
// which works only here; account tokens do not work here.

// facts is what an AP sends to enroll, and, with the address it came from,
// what the manager records for the person deciding whether to adopt it.
type facts struct {
	MAC      string          `json:"mac"`
	MACs     []string        `json:"macs,omitempty"`
	Hostname string          `json:"hostname,omitempty"`
	Model    string          `json:"model,omitempty"`
	Board    string          `json:"board,omitempty"`
	OpenWrt  string          `json:"openwrt,omitempty"`
	Radios   json.RawMessage `json:"radios,omitempty"`
	Source   string          `json:"source,omitempty"`
}

const (
	maxEnrollBody = 64 << 10
	maxMACs       = 32
	maxText       = 128
)

// apID derives an AP's ID from its MAC (0038): "ap-" and the 12 hex digits.
func apID(mac string) hierarchy.NodeID {
	return hierarchy.NodeID("ap-" + strings.ReplaceAll(mac, ":", ""))
}

// enrolledName is what a new AP is called: the hostname it sends, its ID
// where it sends none. OpenWrt's own default gets the last four of the MAC
// after it, so that APs fresh from the box tell apart (Griff, 2026-10-20:
// "like OpenWrt1dcf"); a hostname someone chose stays as it is.
func enrolledName(hostname, mac string) string {
	switch {
	case hostname == "":
		return string(apID(mac))
	case strings.EqualFold(hostname, "OpenWrt"):
		return hostname + strings.ReplaceAll(mac, ":", "")[8:]
	}
	return hostname
}

// normalMAC reads a unicast EUI-48 address and writes it as aa:bb:cc:dd:ee:ff.
func normalMAC(s string) (string, error) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return "", badRequest("%q is not a MAC address", s)
	}
	if hw[0]&1 == 1 || string(hw) == "\x00\x00\x00\x00\x00\x00" {
		return "", badRequest("%s is not an address a device can have", hw)
	}
	return hw.String(), nil
}

// plainText accepts a short single-line string.
func plainText(field, s string, max int) error {
	if len(s) > max {
		return badRequest("%s is longer than %d characters", field, max)
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return badRequest("%s holds a character that is not printable", field)
		}
	}
	return nil
}

func (f *facts) check() error {
	var err error
	if f.MAC == "" {
		return badRequest("mac is required: the MAC address the AP is known by")
	}
	if f.MAC, err = normalMAC(f.MAC); err != nil {
		return err
	}
	if len(f.MACs) > maxMACs {
		return badRequest("at most %d MAC addresses", maxMACs)
	}
	for i, m := range f.MACs {
		if f.MACs[i], err = normalMAC(m); err != nil {
			return err
		}
	}
	for _, t := range []struct {
		field, value string
		max          int
	}{{"hostname", f.Hostname, 63}, {"model", f.Model, maxText}, {"board", f.Board, maxText}, {"openwrt", f.OpenWrt, maxText}} {
		if err := plainText(t.field, t.value, t.max); err != nil {
			return err
		}
	}
	if len(f.Radios) > 0 {
		var radios []json.RawMessage
		if json.Unmarshal(f.Radios, &radios) != nil {
			return badRequest("radios must be a list")
		}
	}
	return nil
}

// enroll records a new AP in Landing Zone and hands it its token, once
// (0033). It needs no token. The manager makes the change in its own name
// (0036), with the address the request came from as the reason.
func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEnrollBody)
	var f facts
	err := readJSON(r, &f)
	if err == nil {
		err = f.check()
	}
	if err != nil {
		s.fail(w, r, "", err)
		return
	}
	f.Source, _, _ = net.SplitHostPort(r.RemoteAddr)
	state := s.log.Snapshot()
	if state == nil {
		s.fail(w, r, "", change.ErrNoOrg)
		return
	}
	id := knownAs(state, f.MAC)
	name := enrolledName(f.Hostname, f.MAC)
	// An AP known already keeps its name, enrolling again (0223).
	n, again := state.Org.Locations.Node(id)
	if again {
		name = n.Name
	}
	raw, err := json.Marshal(f)
	if err != nil {
		s.fail(w, r, "", err)
		return
	}
	plain, tokenID, hash, err := access.NewAPToken()
	if err != nil {
		s.fail(w, r, "", err)
		return
	}
	op := change.Op{Kind: change.Enroll, Node: id, Name: name, TokenID: tokenID, TokenHash: hash, Value: raw}
	reason := "enrolled from " + f.Source
	if again {
		reason = "enrolled again from " + f.Source
	}
	if _, err := s.log.Commit(change.SystemActor, reason, op); err != nil {
		switch {
		case errors.Is(err, hierarchy.ErrExists):
			err = &apiError{http.StatusConflict, fmt.Sprintf("%s enrolled at the same moment: try again", id)}
		case errors.Is(err, hierarchy.ErrNotFound):
			err = &apiError{http.StatusServiceUnavailable, "this manager has no Landing Zone yet"}
		}
		s.fail(w, r, "", err)
		return
	}
	slog.Info("enrolled", "ap", id, "name", name, "source", f.Source, "again", again)
	out := map[string]any{"ap": id, "name": name, "token": plain, "config": "/v1/ap/config"}
	// Adopted already, it waits for a person to let it back in (0223).
	if again && !state.Org.Locations.InIsolated(id) {
		out["waiting"] = true
	}
	writeJSON(w, http.StatusCreated, out)
}

// knownAs is the AP an enrolling MAC is (0223): the one whose facts say that
// MAC, as one an AP was put in the place of does; else the one with the ID
// the MAC makes, where that one's facts say no other MAC; else a new AP. A
// MAC whose ID another box holds now, put in its place, is a new AP by the
// next ID free: ap-<MAC>-2, -3.
func knownAs(state *change.State, mac string) hierarchy.NodeID {
	ids := make([]hierarchy.NodeID, 0, len(state.Facts))
	for ap := range state.Facts {
		ids = append(ids, ap)
	}
	slices.Sort(ids)
	for _, ap := range ids {
		if factsMAC(state, ap) == mac {
			return ap
		}
	}
	id := apID(mac)
	if other := factsMAC(state, id); other == "" || other == mac {
		return id
	}
	for i := 2; ; i++ {
		next := hierarchy.NodeID(fmt.Sprintf("%s-%d", id, i))
		if _, ok := state.Org.Locations.Node(next); !ok {
			return next
		}
	}
}

// factsMAC is the MAC an AP is known by, as its facts say: the one it
// enrolled with, or that of the AP put in its place (0223). Empty where it
// has no facts.
func factsMAC(state *change.State, ap hierarchy.NodeID) string {
	var f facts
	if json.Unmarshal(state.Facts[ap], &f) != nil {
		return ""
	}
	return f.MAC
}

// factsSaid is when each AP last changed what it says it is, so one whose
// facts keep changing, a radio coming and going, fills no log (0122).
type factsSaid struct {
	mu sync.Mutex
	at map[hierarchy.NodeID]time.Time
}

// factsEvery is how often an AP's facts may change.
const factsEvery = 10 * time.Minute

// soon says how long an AP must wait before its facts may change again:
// none where they may now, which it then notes as changed.
func (f *factsSaid) soon(ap hierarchy.NodeID, now time.Time) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	if left := factsEvery - now.Sub(f.at[ap]); left > 0 {
		return left
	}
	if f.at == nil {
		f.at = map[hierarchy.NodeID]time.Time{}
	}
	f.at[ap] = now
	return 0
}

// sameJSON says whether two JSON values are the same, whatever the order of
// their objects' keys.
func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// apFacts takes what an AP says of itself, again (0122): the same as it
// enrolled with, as it is now. A radio it has gained, a scan radio another
// service now owns, the modes a newer driver gives, a newer OpenWrt: the
// manager knew none of it, having asked once. Where it differs from what
// the manager has, the manager records it, in its own name as an
// enrollment is (0036). Where it came from stays where it enrolled from.
func (s *Server) apFacts(w http.ResponseWriter, r *http.Request, c apCall) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxEnrollBody)
	var f facts
	if err := readJSON(r, &f); err != nil {
		return err
	}
	if err := f.check(); err != nil {
		return err
	}
	state := s.log.Snapshot()
	if state == nil {
		return change.ErrNoOrg
	}
	// The MAC it is known by: the one it enrolled with, or that of the AP
	// put in its place (0223); its ID's, where its facts have none.
	if known := factsMAC(state, c.ap); known != f.MAC && (known != "" || apID(f.MAC) != c.ap) {
		return badRequest("mac: the one this AP enrolled with, which it is known by")
	}
	var old facts
	_ = json.Unmarshal(state.Facts[c.ap], &old)
	f.Source = old.Source
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if sameJSON(raw, state.Facts[c.ap]) {
		writeJSON(w, http.StatusOK, map[string]any{"changed": false})
		return nil
	}
	if left := s.said.soon(c.ap, time.Now()); left > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(left.Seconds())+1))
		return &apiError{http.StatusTooManyRequests, "what this AP is changed less than 10 minutes ago: say it again later"}
	}
	if _, err := s.log.Commit(change.SystemActor, "the AP said what it is again", change.Op{Kind: change.Facts, Node: c.ap, Value: raw}); err != nil {
		return err
	}
	slog.Info("facts changed", "ap", c.ap)
	NewKinds(s.log) // another board, perhaps: a kind of AP with no template yet (0085)
	writeJSON(w, http.StatusOK, map[string]any{"changed": true})
	return nil
}

// apCall is one request from an authenticated AP: the AP, and when its
// request came, before anything was done about it.
type apCall struct {
	ap hierarchy.NodeID
	at time.Time
}

type apHandler func(w http.ResponseWriter, r *http.Request, c apCall) error

func (s *Server) apAuth(h apHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at := time.Now()
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		state := s.log.Snapshot()
		if !ok || state == nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid AP token")
			return
		}
		ap, err := state.Access.AuthenticateAP(strings.TrimSpace(token))
		if errors.Is(err, access.ErrWaiting) {
			waiting(w, r, ap)
			return
		}
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid AP token")
			return
		}
		// Every AP request counts as being seen (0039). Failing to record it
		// does not stop the AP.
		source, _, _ := net.SplitHostPort(r.RemoteAddr)
		if err := s.conds.Seen(ap, source); err != nil {
			slog.Error("recording last seen", "ap", ap, "err", err)
		}
		if err := h(w, r, apCall{ap: ap, at: at}); err != nil {
			s.fail(w, r, string(ap), err)
		}
	})
}

// apKnown is apAuth without its note that the AP was seen: for what an
// AP says many times a second on its line (0145), where a write to the
// store each time would cost more than the saying, and would be counted
// in the time the AP reckons its clock by. Its polls say it is there.
func (s *Server) apKnown(h apHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at := time.Now()
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		state := s.log.Snapshot()
		if !ok || state == nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid AP token")
			return
		}
		ap, err := state.Access.AuthenticateAP(strings.TrimSpace(token))
		if errors.Is(err, access.ErrWaiting) {
			waiting(w, r, ap)
			return
		}
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid AP token")
			return
		}
		if err := h(w, r, apCall{ap: ap, at: at}); err != nil {
			s.fail(w, r, string(ap), err)
		}
	})
}

// waiting answers an AP that enrolled again and waits for a person to let it
// back in (0223): its poll as Landing Zone's are, unassigned, and anything
// else 409, so it keeps the token it waits with. It is not counted as seen:
// until it is let back in, the manager hears nothing from it as the AP.
func waiting(w http.ResponseWriter, r *http.Request, ap hierarchy.NodeID) {
	if r.Method == http.MethodGet && r.URL.Path == "/v1/ap/config" {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"ap": ap, "state": "unassigned", "waiting": true})
		return
	}
	writeError(w, http.StatusConflict, access.ErrWaiting.Error())
}

// apPoll answers an AP's poll (0007, 0029, 0032). The AP sends the version it
// runs in If-None-Match:
//   - 304: that is still its version.
//   - "unassigned": it is in Landing Zone and gets nothing.
//   - "held": its config breaks rules; it keeps running what it has.
//   - "ready": its config, with secrets opened (0027), and the version as the
//     ETag. Only a ready config carries an ETag, so an AP only ever holds the
//     version of a config it was sent. With failed, where the AP said last
//     that this version did not apply (0235).
func (s *Server) apPoll(w http.ResponseWriter, r *http.Request, c apCall) error {
	w.Header().Set("Cache-Control", "no-store")
	// The agent it should run (0079), on every answer: an unchanged, held
	// or unassigned config still carries it.
	s.setAgentHeader(w, c.ap)
	// And the actions that wait for it (0104).
	s.setActionsHeader(w, c.ap)
	// And the clients it is to trace (0144).
	s.setTraceHeader(w)
	// And the version of its 802.11r key holders (0224).
	s.setFTHeader(w, s.log.Snapshot(), c.ap)
	// And the version of what its airscan is to trust (0304).
	s.setTrustHeader(w, s.log.Snapshot(), c.ap)
	// The version is read before the state, so the state is never older than
	// the version it is labeled with. If a change lands in between, the AP
	// gets the newer config under the older version and fetches it once more.
	version, _ := s.log.Version(c.ap)
	etag := `"` + strconv.FormatInt(version, 10) + `"`
	if matches(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	res, err := s.compose(s.log.Snapshot(), c.ap, s.reveal)
	if err != nil {
		return err
	}
	out := map[string]any{"ap": c.ap, "version": version}
	switch {
	case res.Unassigned:
		out["state"] = "unassigned"
	case len(res.Problems) > 0:
		out["state"], out["problems"] = "held", res.Problems
	default:
		out["state"], out["config"] = "ready", res.Doc
		// The AP said this version did not apply, though it may run it from
		// before, as after a sysupgrade onto an image without a package the
		// config needs (0235): an AP that runs it checks what renders to
		// what runs, and says it applied, where it would say nothing.
		if a, err := s.conds.LatestApply(c.ap); err == nil && a != nil && a.Version == version && !a.OK {
			out["failed"] = true
		}
		// The key the APs sign their hellos to each other with, while radio
		// resource management is on (0073). It is the same for every AP, and
		// comes beside the config, so it versions nothing.
		if rrmOn(res.Doc) {
			out["rrm"] = map[string]any{"key": hex.EncodeToString(s.box.Derive("rrm"))}
		}
		// The manager's own SSH key, which every AP lets in beside those
		// of system.ssh_keys (0197). It comes beside the config too.
		out["ssh"] = map[string]any{"manager": s.managerSSHPublic()}
		w.Header().Set("ETag", etag)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// rrmOn says whether a config turns radio resource management on (0073).
func rrmOn(doc map[string]any) bool {
	r, _ := doc["rrm"].(map[string]any)
	return r["enabled"] == true
}

// matches reports whether an If-None-Match header names the ETag.
func matches(header, etag string) bool {
	for _, t := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(t), "W/") == etag {
			return true
		}
	}
	return false
}
