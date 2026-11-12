package api

// The manager's own releases (0183). The manager runs without root, so it
// does not update itself: it asks the updater, which the installer sets up
// as root, by leaving a request in the update directory, and reads what the
// updater says in the same place. It asks the updater to look for a newer
// release once a day, and, where the Org says so, to install one in its
// window.

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
)

const (
	// updateLook is how often the manager asks the updater to look for a
	// newer release.
	updateLook = 24 * time.Hour
	// updateRetry is how long an update that failed waits to be asked
	// again by itself.
	updateRetry = 24 * time.Hour
	// updateStale is how long the updater may say it is checking or
	// updating: more than its unit lets it run (30 minutes).
	updateStale = 45 * time.Minute
)

var tagRE = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)`)

// WithUpdates gives the server the directory the updater reads its requests
// from and writes what came of them to (0183). Where it does not exist, the
// installer set no updater up, and there is nothing to ask.
func (s *Server) WithUpdates(dir string) *Server {
	s.updates = dir
	return s
}

// updateStatus is what the updater last said.
type updateStatus struct {
	Running string
	Newest  string
	Stable  string // the newest release on the stable channel (0282), or ""
	Checked time.Time
	State   string // idle, checking, updating, updated or failed
	Tag     string
	At      time.Time
	Error   string
	Written time.Time // when the updater last wrote it
}

// The channels a manager takes its releases from (0282). Every release is
// on testing as it is made; one that has run a while there is promoted to
// stable, by a second tag, stable/vX.Y.Z, on its commit.
const (
	channelStable  = "stable"
	channelTesting = "testing"
)

// target is the release the channel says to run: the newest on it, or ""
// where it has none yet.
func (st updateStatus) target(channel string) string {
	if channel == channelTesting {
		return st.Newest
	}
	return st.Stable
}

// settled is the status as it stands at now. One left checking or updating
// for longer than the updater may run was stopped before it could say how
// it went, as when the host ran out of memory building a release
// (2026-10-21): that is a failure, and holds nothing up.
func (st updateStatus) settled(now time.Time) updateStatus {
	if (st.State == "checking" || st.State == "updating") && !st.Written.IsZero() && now.Sub(st.Written) >= updateStale {
		st.State, st.Error = "failed", "the updater stopped before it said how it went: update.log may say why"
	}
	return st
}

// readUpdateStatus reads the updater's status: key=value lines.
func readUpdateStatus(dir string) updateStatus {
	var st updateStatus
	f, err := os.Open(filepath.Join(dir, "status"))
	if err != nil {
		return st
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil {
		st.Written = fi.ModTime()
	}
	unix := func(v string) time.Time {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return time.Unix(n, 0)
		}
		return time.Time{}
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), "=")
		switch k {
		case "running":
			st.Running = v
		case "newest":
			st.Newest = v
		case "stable":
			st.Stable = v
		case "checked":
			st.Checked = unix(v)
		case "state":
			st.State = v
		case "tag":
			st.Tag = v
		case "at":
			st.At = unix(v)
		case "error":
			st.Error = v
		}
	}
	return st
}

// releaseNote is what a release newer than the one running says of
// itself: its tag's message (0217).
type releaseNote struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

// readUpdateNotes reads what the updater wrote of the releases newer than
// the one it found running, those newer than running alone, the newest
// first: blocks each begun by a line "@@ vX.Y.Z".
func readUpdateNotes(dir, running string) []releaseNote {
	raw, err := os.ReadFile(filepath.Join(dir, "notes"))
	if err != nil || len(raw) > 64<<10 {
		return nil
	}
	var out []releaseNote
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "@@ "); ok && tagRE.MatchString(v) {
			out = append(out, releaseNote{Version: v})
			continue
		}
		if len(out) > 0 {
			n := &out[len(out)-1]
			n.Notes += line + "\n"
		}
	}
	kept := out[:0]
	for _, n := range out {
		if !newer(n.Version, running) {
			continue
		}
		n.Notes = strings.TrimSpace(n.Notes)
		if len(n.Notes) > 4096 {
			n.Notes = n.Notes[:4096]
		}
		kept = append(kept, n)
	}
	return kept
}

// newer says whether release a is newer than b, both as tags name them
// (vX.Y.Z, and anything after); false where either is not one.
func newer(a, b string) bool {
	x, y := tagRE.FindStringSubmatch(a), tagRE.FindStringSubmatch(b)
	if x == nil || y == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		p, _ := strconv.Atoi(x[i])
		q, _ := strconv.Atoi(y[i])
		if p != q {
			return p > q
		}
	}
	return false
}

// pending says whether a request waits for the updater.
func (s *Server) pending() bool {
	_, err := os.Lstat(filepath.Join(s.updates, "request"))
	return err == nil
}

// ask leaves a request for the updater: check, or update and a release. It
// is written whole, then renamed into place, so the updater never reads one
// half written.
func (s *Server) ask(req string) error {
	tmp, err := os.CreateTemp(s.updates, ".request.")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(req + "\n"); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(s.updates, "request"))
}

// updatable says whether the manager has an updater to ask.
func (s *Server) updatable() bool {
	if s.updates == "" {
		return false
	}
	fi, err := os.Stat(s.updates)
	return err == nil && fi.IsDir()
}

// updateSettings are the Org's: whether the manager installs a newer
// release by itself, when, in which zone, and from which channel (0282:
// stable unless set).
func updateSettings(state *change.State) (automatic bool, window, tz, channel string) {
	window, channel = "03:00-05:00", channelStable
	t := state.Org.Locations
	root := t.Root()
	if r, ok := t.Resolve(root, "system.updates.automatic"); ok {
		automatic, _ = r.Value.(bool)
	}
	if r, ok := t.Resolve(root, "system.updates.channel"); ok {
		if v, _ := r.Value.(string); v == channelTesting {
			channel = v
		}
	}
	if r, ok := t.Resolve(root, "system.updates.window"); ok {
		if v, _ := r.Value.(string); v != "" {
			window = v
		}
	}
	if r, ok := t.Resolve(root, "system.tz"); ok {
		tz, _ = r.Value.(string)
	}
	return automatic, window, tz, channel
}

// administers says whether the caller may run the manager's own releases:
// an admin at the Org, or an owner (0177).
func administers(c call) bool {
	t := c.state.Org.Locations
	return may(c, change.Locations, t, t.Root(), access.Administer)
}

// updateView is the manager's release, and the updater's word, for the
// page: GET /v1/system/update.
func (s *Server) updateView(w http.ResponseWriter, _ *http.Request, c call) error {
	if !administers(c) {
		return errNotFound
	}
	automatic, window, _, channel := updateSettings(c.state)
	out := map[string]any{"installed": s.updatable(), "running": s.version, "automatic": automatic, "window": window, "channel": channel}
	if s.updatable() {
		st := readUpdateStatus(s.updates).settled(time.Now())
		at := func(t time.Time) any {
			if t.IsZero() {
				return nil
			}
			return t
		}
		target := st.target(channel)
		out["newest"], out["stable"], out["target"], out["state"], out["tag"], out["error"] = st.Newest, st.Stable, target, st.State, st.Tag, st.Error
		out["checked"], out["at"] = at(st.Checked), at(st.At)
		out["newer"] = newer(target, s.version)
		// What each newer release says of itself (0217), up to the one the
		// channel names.
		notes := []releaseNote{}
		for _, n := range readUpdateNotes(s.updates, s.version) {
			if target != "" && !newer(n.Version, target) {
				notes = append(notes, n)
			}
		}
		out["notes"] = notes
		out["pending"] = s.pending()
		if log, err := os.ReadFile(filepath.Join(s.updates, "update.log")); err == nil {
			out["log"] = string(log)
		}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// updateAsk asks the updater: POST /v1/system/update {action}, check to look
// for a newer release now, update to install the newest it found.
func (s *Server) updateAsk(w http.ResponseWriter, r *http.Request, c call) error {
	if !administers(c) {
		return errNotFound
	}
	var b struct {
		Action string `json:"action"`
	}
	if err := readJSON(r, &b); err != nil {
		return err
	}
	if !s.updatable() {
		return &apiError{http.StatusConflict, "this manager has no updater: aeolus-install, run again on its host, sets one up"}
	}
	st := readUpdateStatus(s.updates).settled(time.Now())
	if s.pending() || st.State == "checking" || st.State == "updating" {
		return &apiError{http.StatusConflict, "the updater is busy: try again when it is done"}
	}
	var req string
	switch b.Action {
	case "check":
		req = "check"
	case "update":
		_, _, _, channel := updateSettings(c.state)
		target := st.target(channel)
		if !newer(target, s.version) {
			return &apiError{http.StatusConflict, "no newer release is known on the " + channel + " channel: check first"}
		}
		req = "update " + target
	default:
		return badRequest("action is check or update")
	}
	if err := s.ask(req); err != nil {
		return err
	}
	slog.Info("updates: asked", "request", req, "by", c.actor)
	writeJSON(w, http.StatusAccepted, map[string]any{"asked": req})
	return nil
}

// updateDue is what the manager asks the updater by itself at now, if
// anything: to look for a newer release once a day; with automatic on, to
// install the newest on its channel (0282) in the window, unless that one
// failed lately.
func updateDue(st updateStatus, running string, automatic bool, window, tz, channel string, now time.Time) string {
	st = st.settled(now)
	if st.State == "checking" || st.State == "updating" {
		return ""
	}
	if st.Checked.IsZero() || now.Sub(st.Checked) >= updateLook {
		return "check"
	}
	target := st.target(channel)
	if automatic && newer(target, running) && windowOpen(window, tz, now) &&
		!(st.State == "failed" && st.Tag == target && now.Sub(st.At) < updateRetry) {
		return "update " + target
	}
	return ""
}

// Updates asks the updater what is due, every interval until ctx ends
// (0183).
func (s *Server) Updates(ctx context.Context, every time.Duration) {
	if !s.updatable() {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		s.updateOnce(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// updateOnce asks what is due at now, if nothing waits already.
func (s *Server) updateOnce(now time.Time) string {
	state := s.log.Snapshot()
	if state == nil || !s.updatable() || s.pending() {
		return ""
	}
	automatic, window, tz, channel := updateSettings(state)
	req := updateDue(readUpdateStatus(s.updates), s.version, automatic, window, tz, channel, now)
	if req == "" {
		return ""
	}
	if err := s.ask(req); err != nil {
		slog.Warn("updates: cannot ask the updater", "err", err)
		return ""
	}
	slog.Info("updates: asked by itself", "request", req)
	return req
}
