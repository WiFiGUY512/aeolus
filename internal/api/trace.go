package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Live Trace (0144). Someone asks that one client be traced, by its MAC, on
// or not. Every AP that holds its line to the manager (0145) is told at
// once, and one that does not at its next poll; from then each says every
// thing it sees of that client as it sees it, and how strongly it hears
// it. The manager puts what the APs say in one line, each time on its own
// clock, so that a client carried down a hallway is seen leaving one AP
// and coming to the next, a step at a time, and whoever watches is sent
// each step as it comes.
//
// It is kept in memory alone: a trace is for looking now, and is gone when
// the manager stops.

// TraceHeader tells an AP, on every config poll, which clients to trace
// and for how many seconds more: "aa:bb:cc:00:11:22=840,..." (0144). An AP
// that holds its line has heard already; this is for one that does not.
const TraceHeader = "Aeolus-Trace"

const (
	// traceFor is how long a trace lasts unless said, and traceMost
	// the longest that can be asked: APs that trace look at their clients
	// twice a second, and should not be left at it.
	traceFor  = 15 * time.Minute
	traceMost = 60 * time.Minute
	// traceAtOnce is how many clients are traced at once.
	traceAtOnce = 4
	// traceKept is how many things are kept of one client, the latest;
	// traceRead how many a look takes; traceBatch how many an AP says at
	// a time.
	traceKept  = 5000
	traceRead  = 500
	traceBatch = 200
	// traceAfter is how long a trace that ended can still be read, and
	// traceLate how long after its end an AP's word of it is still taken:
	// what it saw just before it was told to stop.
	traceAfter = 24 * time.Hour
	traceLate  = 30 * time.Second
)

// traceEvent is one thing an AP saw of a traced client: when, by the
// manager's clock, which the AP's own time is put on (0145); the AP; the
// network and band it was on or heard on; the stage, as an attempt's steps
// name theirs (0118), or heard, signal, done or said; what, in words;
// whether it went well, where it says; how strongly the AP heard it, in
// dBm, where it says; and how long after it happened the manager had it,
// in ms, where the AP's clock is known well enough to say.
type traceEvent struct {
	Seq     int64            `json:"seq"`
	At      time.Time        `json:"at"`
	AP      hierarchy.NodeID `json:"ap"`
	APName  string           `json:"ap_name,omitempty"`
	Network string           `json:"network,omitempty"`
	Band    string           `json:"band,omitempty"`
	Stage   string           `json:"stage"`
	What    string           `json:"what"`
	OK      *bool            `json:"ok,omitempty"`
	Signal  *int             `json:"signal,omitempty"`
	Lag     *float64         `json:"lag_ms,omitempty"`
	// When, by the AP's own clock, in ms since 1970, where it is to be
	// held to what the AP saw just before it (see run).
	raw int64
}

// run is the last thing one AP said of one client, and when the things it
// said close together began. What an AP sees within runGap of the thing
// before keeps the spacing the AP timed the two by, for as long as runLong:
// each sending is put on the manager's clock by its own reckoning, and two
// reckonings differ by a part of a millisecond, which would turn round two
// things the AP timed a millisecond apart. Over runLong the AP's clock may
// have moved against the manager's by more than that, and the newer
// reckoning is taken.
type run struct {
	raw   int64
	at    time.Time
	began time.Time
}

const (
	runGap  = 100 * time.Millisecond
	runLong = 2 * time.Second
)

// traced is one client's trace: who asked, from when, until when, and
// what the APs said of it.
type traced struct {
	by     access.AccountID
	since  time.Time
	until  time.Time
	events []traceEvent
	runs   map[hierarchy.NodeID]run
}

// traceBook is the clients traced, by MAC, and those whose trace ended
// lately.
type traceBook struct {
	mu  sync.Mutex
	seq int64
	of  map[string]*traced
	// Those who watch a client's trace as it comes, each woken when there
	// is more of it to read.
	watchers map[string]map[chan struct{}]struct{}
}

// watch has the caller woken whenever there is more to read of a client's
// trace, or it starts or stops. The second value ends it.
func (b *traceBook) watch(mac string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.watchers == nil {
		b.watchers = map[string]map[chan struct{}]struct{}{}
	}
	if b.watchers[mac] == nil {
		b.watchers[mac] = map[chan struct{}]struct{}{}
	}
	b.watchers[mac][ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.watchers[mac], ch)
		if len(b.watchers[mac]) == 0 {
			delete(b.watchers, mac)
		}
	}
}

// wake wakes those who watch a client. The book is held by the caller.
func (b *traceBook) wake(mac string) {
	for ch := range b.watchers[mac] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// wakeAll wakes everyone who watches: which APs hold a line has changed.
func (b *traceBook) wakeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for mac := range b.watchers {
		b.wake(mac)
	}
}

// start traces a client until d from now, or goes on tracing it until
// then; it says no where enough are traced already.
func (b *traceBook) start(mac string, by access.AccountID, now time.Time, d time.Duration) (*traced, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.of == nil {
		b.of = map[string]*traced{}
	}
	on := 0
	for m, f := range b.of {
		switch {
		case now.Sub(f.until) > traceAfter:
			delete(b.of, m)
		case f.until.After(now) && m != mac:
			on++
		}
	}
	f := b.of[mac]
	if f == nil || !f.until.After(now) {
		if on >= traceAtOnce {
			return nil, false
		}
		// Traced anew: what was kept of the last time goes.
		f = &traced{since: now}
		b.of[mac] = f
	}
	f.by, f.until = by, now.Add(d)
	b.wake(mac)
	return f, true
}

// stop ends a client's trace now; what was said of it stays to be read.
func (b *traceBook) stop(mac string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	f := b.of[mac]
	if f == nil || !f.until.After(now) {
		return false
	}
	f.until = now
	b.wake(mac)
	return true
}

// active is the clients traced now, each with the seconds it has left.
func (b *traceBook) active(now time.Time) map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]int{}
	for m, f := range b.of {
		if f.until.After(now) {
			out[m] = int(f.until.Sub(now)/time.Second) + 1
		}
	}
	return out
}

// add takes what an AP said of traced clients; a word of one not
// traced, or long after its trace ended, is passed over. It says how
// many it kept.
func (b *traceBook) add(now time.Time, events []traceEvent, macs []string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := 0
	for i, e := range events {
		f := b.of[macs[i]]
		if f == nil || now.Sub(f.until) > traceLate {
			continue
		}
		if e.raw != 0 {
			r, ok := f.runs[e.AP]
			gap := time.Duration(e.raw-r.raw) * time.Millisecond
			if at := r.at.Add(gap); ok && gap > -runGap && gap < runGap && at.Sub(r.began) < runLong && r.began.Sub(at) < runLong {
				e.At = at
			} else {
				r.began = e.At
			}
			r.raw, r.at = e.raw, e.At
			if f.runs == nil {
				f.runs = map[hierarchy.NodeID]run{}
			}
			f.runs[e.AP] = r
		}
		b.seq++
		e.Seq = b.seq
		f.events = append(f.events, e)
		if len(f.events) > traceKept {
			f.events = slices.Clone(f.events[len(f.events)-traceKept:])
		}
		kept++
		b.wake(macs[i])
	}
	return kept
}

// read is a client's trace, and what was said of it after seq, at most
// traceRead, the oldest of them first.
func (b *traceBook) read(mac string, after int64) (f traced, events []traceEvent, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.of[mac]
	if p == nil {
		return traced{}, nil, false
	}
	i := sort.Search(len(p.events), func(i int) bool { return p.events[i].Seq > after })
	events = slices.Clone(p.events[i:min(len(p.events), i+traceRead)])
	return traced{by: p.by, since: p.since, until: p.until}, events, true
}

// setTraceHeader says, on a config poll, which clients an AP is to trace.
func (s *Server) setTraceHeader(w http.ResponseWriter) {
	on := s.traces.active(time.Now())
	if len(on) == 0 {
		return
	}
	macs := make([]string, 0, len(on))
	for m := range on {
		macs = append(macs, m)
	}
	slices.Sort(macs)
	for i, m := range macs {
		macs[i] = m + "=" + strconv.Itoa(on[m])
	}
	w.Header().Set(TraceHeader, strings.Join(macs, ","))
}

// traceMAC is the client a trace is of, as its path names it.
func traceMAC(r *http.Request) (string, error) {
	mac := strings.ToLower(r.PathValue("mac"))
	if !macRE.MatchString(mac) {
		return "", badRequest("a client is named by its MAC, such as aa:bb:cc:00:11:22")
	}
	return mac, nil
}

// operates says whether the caller holds the clients right on some AP:
// who may ask the APs to trace a client, or to stop. A trace is as deep as
// an operator goes; a capture is an admin's (0177).
func operates(c call) bool {
	t := c.state.Org.Locations
	for _, id := range t.APs() {
		if may(c, change.Locations, t, id, access.Clients) {
			return true
		}
	}
	return false
}

// traceView is a client's trace as it is read.
type traceView struct {
	MAC     string       `json:"mac"`
	Tracing bool         `json:"tracing"`
	By      string       `json:"by,omitempty"`
	Since   *time.Time   `json:"since,omitempty"`
	Until   *time.Time   `json:"until,omitempty"`
	Next    int64        `json:"next"`
	Events  []traceEvent `json:"events"`
	// The APs the caller may view: how many, and those that hold their
	// line now, which hear of a trace at once and say what they see as
	// they see it (0145).
	APs  int         `json:"aps"`
	Live []traceLine `json:"live"`
}

func (s *Server) traceSaid(c call, mac string, after int64) traceView {
	v := traceView{MAC: mac, Next: after, Events: []traceEvent{}}
	t := c.state.Org.Locations
	seen := map[hierarchy.NodeID]bool{}
	may := func(ap hierarchy.NodeID) bool {
		ok, known := seen[ap]
		if !known {
			ok = may(c, change.Locations, t, ap, access.View)
			seen[ap] = ok
		}
		return ok
	}
	v.APs, v.Live = s.traceLines(c, may)
	f, events, ok := s.traces.read(mac, after)
	if !ok {
		return v
	}
	v.Tracing, v.By, v.Since, v.Until = f.until.After(time.Now()), string(f.by), &f.since, &f.until
	for _, e := range events {
		// How far a look got, whether or not the caller may see each.
		v.Next = e.Seq
		if !may(e.AP) {
			continue
		}
		if n, ok := t.Node(e.AP); ok {
			e.APName = n.Name
		}
		v.Events = append(v.Events, e)
	}
	return v
}

// traceStart asks every AP to trace a client, for ?minutes (15 unless
// said, at most 60), or to go on tracing it that long from now: POST
// /v1/clients/{mac}/trace. Whoever may operate an AP may ask. Four
// clients can be traced at once.
func (s *Server) traceStart(w http.ResponseWriter, r *http.Request, c call) error {
	mac, err := traceMAC(r)
	if err != nil {
		return err
	}
	if !operates(c) {
		return &apiError{http.StatusForbidden, "tracing a client needs the operator role on an AP"}
	}
	d := traceFor
	if q := r.URL.Query().Get("minutes"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || time.Duration(n)*time.Minute > traceMost {
			return badRequest("minutes: from 1 to %d", int(traceMost/time.Minute))
		}
		d = time.Duration(n) * time.Minute
	}
	if _, ok := s.traces.start(mac, c.actor, time.Now(), d); !ok {
		return &apiError{http.StatusConflict, "four clients are traced already: stop one first"}
	}
	s.tellTrace()
	writeJSON(w, http.StatusOK, s.traceSaid(c, mac, 0))
	return nil
}

// traceStop ends a client's trace: DELETE /v1/clients/{mac}/trace.
func (s *Server) traceStop(w http.ResponseWriter, r *http.Request, c call) error {
	mac, err := traceMAC(r)
	if err != nil {
		return err
	}
	if !operates(c) {
		return &apiError{http.StatusForbidden, "stopping a trace needs the operator role on an AP"}
	}
	if s.traces.stop(mac, time.Now()) {
		s.tellTrace()
	}
	writeJSON(w, http.StatusOK, s.traceSaid(c, mac, 0))
	return nil
}

// traceRead is a client's trace: whether it is traced, by whom and
// until when, and what the APs the caller may view said of it after
// ?after=, at most 500, oldest first; next is what to ask after the next
// time: GET /v1/clients/{mac}/trace.
func (s *Server) traceRead(w http.ResponseWriter, r *http.Request, c call) error {
	mac, err := traceMAC(r)
	if err != nil {
		return err
	}
	var after int64
	if q := r.URL.Query().Get("after"); q != "" {
		if after, err = strconv.ParseInt(q, 10, 64); err != nil || after < 0 {
			return badRequest("after: a number a look before gave as next")
		}
	}
	writeJSON(w, http.StatusOK, s.traceSaid(c, mac, after))
	return nil
}

// apTraceEvent is one thing an AP says of a traced client, on its way up
// the line (0145): at is when, in ms since 1970 by its clock.
type apTraceEvent struct {
	At      int64  `json:"at"`
	MAC     string `json:"mac"`
	Network string `json:"network,omitempty"`
	Band    string `json:"band,omitempty"`
	Stage   string `json:"stage"`
	What    string `json:"what"`
	OK      *bool  `json:"ok,omitempty"`
	Signal  *int   `json:"signal,omitempty"`
}

// The stages an AP may say: an attempt's own (0118), and those of a
// trace: heard (not on, and heard asking), signal (on, and how strongly),
// done (what an attempt came to) and end (it left).
var traceStages = map[string]bool{
	"find": true, "auth": true, "assoc": true, "signin": true, "key": true, "dhcp": true, "gateway": true, "dns": true,
	"internet": true, "online": true, "end": true, "heard": true, "signal": true, "done": true, "steer": true,
	// said: a word of hostapd's that belongs to no stage.
	"said": true,
}

// traceLive is a client's trace as it comes: GET
// /v1/clients/{mac}/trace/live. The answer does not end while the caller
// stays: each line of it is the trace as GET .../trace gives it, with what
// came since the line before, written when there is something new, an
// event or a start or a stop or an AP that took up or lost its line; and
// an empty object every ten seconds between, so that a caller knows the
// line lives. ?after= is where a look before left off.
func (s *Server) traceLive(w http.ResponseWriter, r *http.Request, c call) error {
	mac, err := traceMAC(r)
	if err != nil {
		return err
	}
	var after int64
	if q := r.URL.Query().Get("after"); q != "" {
		if after, err = strconv.ParseInt(q, 10, 64); err != nil || after < 0 {
			return badRequest("after: a number a look before gave as next")
		}
	}
	put, err := held(w)
	if err != nil {
		return err
	}
	write := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return put(append(b, '\n'))
	}
	woken, done := s.traces.watch(mac)
	defer done()
	beat := time.NewTicker(liveBeat)
	defer beat.Stop()
	// A caller's standing is as it was when it asked: it is not left to
	// watch on it for ever.
	end := time.NewTimer(traceMost + 10*time.Minute)
	defer end.Stop()
	was := ""
	for first := true; ; first = false {
		v := s.traceSaid(c, mac, after)
		after = v.Next
		// What a caller is shown besides the events: whether it is traced
		// and until when, and which APs hold their line.
		now := fmt.Sprint(v.Tracing, v.By, v.Until, v.APs, len(v.Live))
		for _, l := range v.Live {
			now += " " + string(l.AP)
		}
		if first || len(v.Events) > 0 || now != was {
			if write(v) != nil {
				return nil
			}
			was = now
		}
		if len(v.Events) >= traceRead {
			continue
		}
		select {
		case <-r.Context().Done():
			return nil
		case <-end.C:
			return nil
		case <-woken:
		case <-beat.C:
			if write(struct{}{}) != nil {
				return nil
			}
		}
	}
}
