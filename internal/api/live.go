package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// The line each AP holds open to the manager (0145). APs reach the manager
// and never the other way (0007), so until now the manager could say
// something to an AP only in its answer to a poll, which may be a minute
// off. An AP now also asks GET /v1/ap/live and the manager does not finish
// its answer: it writes a line on it whenever it has something to say, and
// the AP has it within the time the network takes.
//
// What the manager says there is small and whole each time: the clients to
// trace (0144), and its own clock. What an AP says back goes by POST
// /v1/ap/live on a connection it keeps warm: what it saw of the clients
// traced, as it sees it, and with each the time by its clock at which it
// sent. The manager answers with the times by its own clock at which that
// came and at which the answer left, which is all the AP needs to reckon
// how far its clock is from the manager's, as NTP does. It says so with
// what it sends next, and every time an AP names is put on the manager's
// clock: what two APs saw of one client is then in its true order to about
// a millisecond, whatever their own clocks say.
//
// Nothing rests on the line: an AP without one is told what it is to trace
// at its poll, as before.

var (
	// liveBeat is how often the manager says its clock on a line with
	// nothing else to say: an AP that hears nothing for two and a half of
	// them takes the line to be dead and asks again.
	liveBeat = 10 * time.Second
	// liveSlow is how long one write on a line may take.
	liveSlow = 10 * time.Second
	// liveDead is how long what the manager wrote on a line may go
	// unacknowledged before the kernel gives the connection up (0165): an
	// AP that loses its power or its cable never closes its line, and the
	// clock the manager says every liveBeat is what goes unacknowledged.
	liveDead = 15 * time.Second
	// liveGrace is how long an AP that closed its line itself has to open
	// another before it is down (0165): an agent restarting does so in a
	// second or two; an AP that reboots does not.
	liveGrace = 10 * time.Second
)

const (
	// liveWaits is how many things may wait to be written on one line. A
	// line that falls further behind is let go: the AP asks again, and is
	// told all of it afresh.
	liveWaits = 16
	// clockFar is the furthest an AP may say its clock is from the
	// manager's, in ms, and be believed; clockStale how long what it said
	// is shown, and used for what it sends without saying so again: a clock
	// that is being corrected moves half a millisecond a second, so not
	// long.
	clockFar   = 366 * 24 * 3600 * 1000.0
	clockStale = 5 * time.Second
)

// msOf is a time in ms since 1970, to the microsecond.
func msOf(t time.Time) float64 { return float64(t.UnixMicro()) / 1000 }

// held readies an answer that does not end: the server's own limits are
// for answers that do, and are taken off it. What it gives back writes one
// thing on the answer and sends it on its way. Each write may take
// liveSlow, and the limit set for it runs past the next thing the manager
// is sure to write: over HTTP/2 a limit that passes ends the answer even
// while nothing is being written.
func held(w http.ResponseWriter) (func([]byte) error, error) {
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return nil, err
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	return func(b []byte) error {
		if err := rc.SetWriteDeadline(time.Now().Add(liveBeat + liveSlow)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		return rc.Flush()
	}, nil
}

// liveLine is one AP's line: what waits to be written on it, and gone,
// which is closed when the line is let go.
type liveLine struct {
	say   chan []byte
	gone  chan struct{}
	since time.Time
}

// apClock is how far an AP says its clock is from the manager's: the ms to
// add to a time by the AP's clock to have it by the manager's; how long
// its asking and the answer took on the way, in ms, which bounds how wrong
// that can be; and when it said so.
type apClock struct {
	offset float64
	rtt    float64
	at     time.Time
}

// liveHub is the lines the APs hold open, and each AP's clock; and of
// each AP whose line went since the manager started, when and how (0165).
type liveHub struct {
	mu     sync.Mutex
	lines  map[hierarchy.NodeID]*liveLine
	clocks map[hierarchy.NodeID]apClock
	gone   map[hierarchy.NodeID]lineGone
}

// lineGone is when an AP's line went, and whether the AP was found gone
// with it: what the manager wrote on it was never taken, where an AP that
// closed its line itself may be about to open another.
type lineGone struct {
	at   time.Time
	sure bool
}

// join gives an AP a line. One it had before is let go: an AP has one.
func (h *liveHub) join(ap hierarchy.NodeID, now time.Time) *liveLine {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lines == nil {
		h.lines = map[hierarchy.NodeID]*liveLine{}
	}
	if old := h.lines[ap]; old != nil {
		close(old.gone)
	}
	l := &liveLine{say: make(chan []byte, liveWaits), gone: make(chan struct{}), since: now}
	h.lines[ap] = l
	delete(h.gone, ap)
	return l
}

// leave takes an AP's line away, if it is still the one it has, and notes
// when, and whether the AP was found gone with it.
func (h *liveHub) leave(ap hierarchy.NodeID, l *liveLine, now time.Time, sure bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lines[ap] == l {
		delete(h.lines, ap)
		if h.gone == nil {
			h.gone = map[hierarchy.NodeID]lineGone{}
		}
		h.gone[ap] = lineGone{at: now, sure: sure}
	}
}

// down says whether an AP is down (0165), and since when: its line went
// and it opened no other, at once where the AP was found gone with it, or
// after liveGrace where it closed the line itself. An AP whose line the
// manager has not seen go since it started is not said to be.
func (h *liveHub) down(ap hierarchy.NodeID, now time.Time) (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lines[ap] != nil {
		return time.Time{}, false
	}
	g, ok := h.gone[ap]
	if !ok || (!g.sure && now.Sub(g.at) < liveGrace) {
		return time.Time{}, false
	}
	return g.at, true
}

// tell writes b on every line. A line with too much waiting is let go.
func (h *liveHub) tell(b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ap, l := range h.lines {
		select {
		case l.say <- b:
		default:
			close(l.gone)
			delete(h.lines, ap)
		}
	}
}

// on is the APs that hold a line now, and since when.
func (h *liveHub) on() map[hierarchy.NodeID]time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[hierarchy.NodeID]time.Time, len(h.lines))
	for ap, l := range h.lines {
		out[ap] = l.since
	}
	return out
}

// setClock notes what an AP says of its clock.
func (h *liveHub) setClock(ap hierarchy.NodeID, c apClock) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clocks == nil {
		h.clocks = map[hierarchy.NodeID]apClock{}
	}
	h.clocks[ap] = c
}

// clock is what an AP last said of its clock, if it said so lately.
func (h *liveHub) clock(ap hierarchy.NodeID, now time.Time) (apClock, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.clocks[ap]
	if ok && now.Sub(c.at) > clockStale {
		delete(h.clocks, ap)
		return apClock{}, false
	}
	return c, ok
}

// liveSay is one line the manager writes to an AP: its clock, in ms since
// 1970; and, where it is there, the clients to trace, each with the
// seconds it has left, all of them every time. Poll says that something
// waits for the AP, an action someone asked of it (0104): it fetches what
// waits now, and not at its next poll. Capture is a capture someone
// started (0148), to begin at once: whoever asked is about to do what they
// want captured. Watch is how many seconds to send what each 5 seconds
// came to on its radios, someone watching it live (0207 part 2), or 0 to
// stop. Record has it record what is going on now (0207 part 3).
type liveSay struct {
	Now     float64         `json:"now"`
	Trace   *map[string]int `json:"trace,omitempty"`
	Poll    bool            `json:"poll,omitempty"`
	Capture *liveCapture    `json:"capture,omitempty"`
	Shell   *liveShell      `json:"shell,omitempty"`
	Watch   *int            `json:"watch,omitempty"`
	Record  *liveRecord     `json:"record,omitempty"`
}

// liveCapture is a capture to begin: its action's ID, and what to take.
type liveCapture struct {
	ID     int64           `json:"id"`
	Params json.RawMessage `json:"params"`
}

// tellOne says something on one AP's line, and whether it holds one: an AP
// without is told nothing, and comes to it at its poll.
func (h *liveHub) tellOne(ap hierarchy.NodeID, b []byte) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	l := h.lines[ap]
	if l == nil {
		return false
	}
	select {
	case l.say <- b:
		return true
	default:
		close(l.gone)
		delete(h.lines, ap)
		return false
	}
}

// tellWaits says on an AP's line that something waits for it, and whether
// the AP holds a line to hear it on.
func (s *Server) tellWaits(ap hierarchy.NodeID) bool {
	b, _ := json.Marshal(liveSay{Now: msOf(time.Now()), Poll: true})
	return s.live.tellOne(ap, append(b, '\n'))
}

// liveTrace is what every line is told of the clients traced now.
func (s *Server) liveTrace(now time.Time) []byte {
	on := s.traces.active(now)
	b, _ := json.Marshal(liveSay{Now: msOf(now), Trace: &on})
	return append(b, '\n')
}

// tellTrace says on every line which clients are traced, when that
// changes; and wakes whoever watches one, since which APs heard it at once
// is part of what they are shown.
func (s *Server) tellTrace() {
	s.live.tell(s.liveTrace(time.Now()))
}

// apLive is the line an AP holds open: GET /v1/ap/live. The answer does
// not end. Each thing the manager says is one line of JSON: {now, trace?,
// poll?, capture?}, the clients to trace at once and whenever they change,
// that something waits for the AP, a capture to begin now, and its clock
// every ten seconds between. It ends when the AP goes, when the AP asks
// again, or when the manager stops.
func (s *Server) apLive(w http.ResponseWriter, r *http.Request, c apCall) error {
	write, err := held(w)
	if err != nil {
		return err
	}
	// An AP that goes without closing its line is found gone once what was
	// written on it goes unacknowledged for liveDead (0165).
	deadAfter(r.Context(), liveDead)
	line := s.live.join(c.ap, time.Now())
	// Whether the AP was found gone with its line: a write that failed.
	sure := false
	defer func() {
		s.live.leave(c.ap, line, time.Now(), sure)
		slog.Info("live line gone", "ap", c.ap, "found_gone", sure)
		s.traces.wakeAll()
	}()
	s.traces.wakeAll()
	if write(s.liveTrace(time.Now())) != nil {
		sure = true
		return nil
	}
	// One watched live is told so at once (0207 part 2).
	if s.watching.watched(c.ap) {
		secs := watchFor
		b, _ := json.Marshal(liveSay{Now: msOf(time.Now()), Watch: &secs})
		if write(append(b, '\n')) != nil {
			sure = true
			return nil
		}
	}
	beat := time.NewTicker(liveBeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-line.gone:
			return nil
		case b := <-line.say:
			if write(b) != nil {
				sure = true
				return nil
			}
		case t := <-beat.C:
			b, _ := json.Marshal(liveSay{Now: msOf(t)})
			if write(append(b, '\n')) != nil {
				sure = true
				return nil
			}
		}
	}
}

// apLiveSaid takes what an AP says on its way up: POST /v1/ap/live,
// {sent, offset?, rtt?, events?, minutes?, news?}. sent is when it sent this, by its clock,
// in ms since 1970; offset what it reckons must be added to its clock to
// have the manager's, in ms, and rtt how long the asking it reckoned that
// from took there and back; events what it saw of the clients traced, at
// most 200, each as POST /v1/ap/trace takes them. Every event's time is
// put on the manager's clock by that offset, or by the last the AP gave,
// or, where it has given none, by taking this to have arrived the moment
// it was sent. The answer is {got, sent, kept}: when this came and when the
// answer left, by the manager's clock, in ms; and how many events were
// kept. points, at most 24, are what its last 5-second stretches came to,
// sent while someone watches it live (0207 part 2), each as a minute is;
// and record, a recording going on that they belong to (0207 part 3).
// minutes, at most 10, are what its minutes came to (0207), and news
// what its daemons handed aeolus-live, at most 200, such as clients joining
// and leaving (0252): minutes says how many of its radios' minutes were
// kept.
func (s *Server) apLiveSaid(w http.ResponseWriter, r *http.Request, c apCall) error {
	var body struct {
		Sent    float64           `json:"sent"`
		Offset  *float64          `json:"offset"`
		RTT     *float64          `json:"rtt"`
		Events  []apTraceEvent    `json:"events"`
		Minutes []json.RawMessage `json:"minutes"`
		News    []json.RawMessage `json:"news"`
		Points  []json.RawMessage `json:"points"`
		Record  *apRecord         `json:"record"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if len(body.Events) > traceBatch {
		return badRequest("events: at most %d at a time", traceBatch)
	}
	if len(body.Minutes) > minutesBatch || len(body.News) > newsBatch || len(body.Points) > pointsBatch {
		return badRequest("at most %d minutes, %d news and %d points at a time", minutesBatch, newsBatch, pointsBatch)
	}
	if math.IsNaN(body.Sent) || math.IsInf(body.Sent, 0) {
		return badRequest("sent: when this was sent, in ms since 1970")
	}
	got := c.at
	offset, known := 0.0, false
	if o := body.Offset; o != nil && !math.IsNaN(*o) && math.Abs(*o) < clockFar {
		rtt := 0.0
		if body.RTT != nil && *body.RTT >= 0 && *body.RTT < 60000 {
			rtt = *body.RTT
		}
		offset, known = *o, true
		s.live.setClock(c.ap, apClock{offset: offset, rtt: rtt, at: got})
	} else if last, ok := s.live.clock(c.ap, got); ok {
		offset, known = last.offset, true
	} else if body.Sent > 0 {
		// Nothing better: as if it came the moment it was sent, which is
		// wrong by the time it took on the way.
		offset = msOf(got) - body.Sent
	}
	events, macs := s.traceEvents(c.ap, body.Events, got, offset, known)
	kept := s.traces.add(got, events, macs)
	// What its minutes came to, and its news (0207).
	minutes := s.keepMinutes(c.ap, body.Minutes, body.News, got)
	// What its last 5-second stretches came to, while it is watched (0207
	// part 2): kept in memory, for whoever watches.
	pts := pointsOf(c.ap, body.Points, got)
	s.watching.add(c.ap, pts, got)
	// A recording going on, and its points (0207 part 3).
	s.keepRecording(c.ap, body.Record, pts, got)
	writeJSON(w, http.StatusOK, map[string]any{"got": msOf(got), "sent": msOf(time.Now()), "kept": kept, "minutes": minutes})
	return nil
}

// traceEvents is what an AP said of traced clients, as it is kept: one
// not well said is passed over; each time is put on the manager's clock by
// offset, in ms, and one still more than ten minutes from the manager's
// is taken to be of now, as an attempt's is (0118). timed says the offset
// is the AP's own reckoning, so how long each took to come is worth
// keeping.
func (s *Server) traceEvents(ap hierarchy.NodeID, said []apTraceEvent, now time.Time, offset float64, timed bool) (events []traceEvent, macs []string) {
	for _, e := range said {
		mac := strings.ToLower(e.MAC)
		if !macRE.MatchString(mac) || !traceStages[e.Stage] || len(e.What) > 200 || !printable(e.What) ||
			len(e.Network) > 64 || !printable(e.Network) || (e.Band != "" && !bands[e.Band]) ||
			(e.Signal != nil && (*e.Signal > 0 || *e.Signal < -127)) {
			continue
		}
		at := time.UnixMicro(int64(math.Round((float64(e.At) + offset) * 1000))).UTC()
		k := traceEvent{AP: ap, Network: e.Network, Band: e.Band, Stage: e.Stage, What: e.What, OK: e.OK, Signal: e.Signal}
		if d := at.Sub(now); d > 10*time.Minute || d < -10*time.Minute {
			at = now.UTC()
		} else {
			k.raw = e.At
			if timed {
				// The reckoning is as wrong as half the way there and back
				// (0145), so what came at once can be put a part of a
				// millisecond after it came. Nothing comes before it
				// happens: that is no time at all. Its time is left as
				// reckoned, which keeps it in order with the other APs'.
				lag := math.Max(0, math.Round(float64(now.Sub(at).Microseconds())/100)/10)
				k.Lag = &lag
			}
		}
		k.At = at
		events = append(events, k)
		macs = append(macs, mac)
	}
	return events, macs
}

// traceLine is an AP that holds its line now, as a trace's reader is
// shown it: the AP; and, where it has said, how far its clock is from the
// manager's and how long its asking took, in ms.
type traceLine struct {
	AP     hierarchy.NodeID `json:"ap"`
	Name   string           `json:"ap_name,omitempty"`
	Offset *float64         `json:"offset_ms,omitempty"`
	RTT    *float64         `json:"rtt_ms,omitempty"`
}

// traceLines is the APs the caller may view: how many there are, and
// those of them that hold a line now, by name.
func (s *Server) traceLines(c call, may func(hierarchy.NodeID) bool) (aps int, lines []traceLine) {
	t := c.state.Org.Locations
	on := s.live.on()
	now := time.Now()
	lines = []traceLine{}
	for _, id := range t.APs() {
		if !may(id) {
			continue
		}
		aps++
		if _, ok := on[id]; !ok {
			continue
		}
		l := traceLine{AP: id}
		if n, ok := t.Node(id); ok {
			l.Name = n.Name
		}
		if k, ok := s.live.clock(id, now); ok {
			off, rtt := math.Round(k.offset*10)/10, math.Round(k.rtt*10)/10
			l.Offset, l.RTT = &off, &rtt
		}
		lines = append(lines, l)
	}
	slices.SortFunc(lines, func(a, b traceLine) int { return strings.Compare(a.Name+" "+string(a.AP), b.Name+" "+string(b.AP)) })
	return aps, lines
}
