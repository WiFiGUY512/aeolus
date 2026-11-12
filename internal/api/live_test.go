package api

import (
	"bufio"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// lineOf opens a held answer and gives each line of it as it comes, read
// as JSON; the second value ends it.
func (f *fixture) lineOf(path, bearer string) (<-chan map[string]any, func()) {
	f.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", f.url+path, nil)
	must(f.t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	must(f.t, err)
	if resp.StatusCode != 200 {
		cancel()
		f.t.Fatalf("%s: %d", path, resp.StatusCode)
	}
	out := make(chan map[string]any, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var v map[string]any
			if json.Unmarshal(sc.Bytes(), &v) == nil {
				out <- v
			}
		}
	}()
	return out, cancel
}

// next is the next line of a held answer that want takes, within a second;
// how long it was in coming is its second value.
func next(t *testing.T, lines <-chan map[string]any, what string, want func(map[string]any) bool) (map[string]any, time.Duration) {
	t.Helper()
	from := time.Now()
	wait := time.After(time.Second)
	for {
		select {
		case v, ok := <-lines:
			if !ok {
				t.Fatalf("%s: the line ended", what)
			}
			if want(v) {
				return v, time.Since(from)
			}
		case <-wait:
			t.Fatalf("%s: not said within a second", what)
		}
	}
}

// TestLiveLine: the line an AP holds open (0145). It is told the clients
// to trace as it joins, and at once when they change, without a poll; an
// AP has one line, and a newer one ends the older.
func TestLiveLine(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()
	const mac = "7e:2a:ea:04:b1:8f"

	lines, hang := f.lineOf("/v1/ap/live", token)
	defer hang()
	traced := func(n int) func(map[string]any) bool {
		return func(v map[string]any) bool {
			m, ok := v["trace"].(map[string]any)
			return ok && len(m) == n
		}
	}
	first, _ := next(t, lines, "as it joins", traced(0))
	if at, ok := first["now"].(float64); !ok || math.Abs(at-float64(time.Now().UnixMilli())) > 2000 {
		t.Errorf("the manager's clock, as it joins: %v", first["now"])
	}

	// Asked, the AP hears of it without a poll, and with the time it has.
	if code, v := f.do("POST", "/v1/clients/"+mac+"/trace?minutes=5", "griff", nil); code != 200 || v["aps"] != float64(2) || len(v["live"].([]any)) != 1 {
		t.Fatalf("asked: %d %v", code, v)
	}
	said, took := next(t, lines, "a client to trace", traced(1))
	if secs := said["trace"].(map[string]any)[mac]; secs != float64(300) && secs != float64(301) {
		t.Errorf("the time it has: %v", secs)
	}
	if took > 200*time.Millisecond {
		t.Errorf("the AP heard of it after %v", took)
	}
	// Stopped, the same.
	if code, v := f.do("DELETE", "/v1/clients/"+mac+"/trace", "griff", nil); code != 200 {
		t.Fatalf("stopped: %d %v", code, v)
	}
	next(t, lines, "none to trace", traced(0))

	// The AP asks again: the older line ends, and the newer is told.
	again, hang2 := f.lineOf("/v1/ap/live", token)
	defer hang2()
	next(t, again, "the newer line, as it joins", traced(0))
	gone := time.After(time.Second)
	for open := true; open; {
		select {
		case _, open = <-lines:
		case <-gone:
			t.Fatal("the older line was left open")
		}
	}
	if on := f.api.live.on(); len(on) != 1 {
		t.Errorf("lines held: %v", on)
	}
	if code, _, b := f.apDo("GET", "/v1/ap/live", "not a token", nil, nil); code != 401 {
		t.Errorf("no token: %d %v", code, b)
	}
	// The AP gone, its line is no longer counted.
	hang2()
	for end := time.Now().Add(time.Second); len(f.api.live.on()) != 0; {
		if time.Now().After(end) {
			t.Fatal("a line whose AP went is still held")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestLiveClock: what an AP sends up its line is put on the manager's
// clock (0145). The AP says when it sent, and is told when that came and
// when the answer left; from the two it reckons how far its clock is off,
// and says so with what it sends next.
func TestLiveClock(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	const mac = "7e:2a:ea:04:b1:8f"
	path := "/v1/clients/" + mac + "/trace"
	if code, v := f.do("POST", path, "griff", nil); code != 200 {
		t.Fatalf("asked: %d %v", code, v)
	}
	// This AP's clock is 5 s behind the manager's.
	const behind = 5000.0
	clock := func() float64 { return float64(time.Now().UnixMicro())/1000 - behind }
	send := func(body map[string]any) (got, sent float64) {
		t.Helper()
		body["sent"] = clock()
		code, _, b := f.apDo("POST", "/v1/ap/live", token, body, nil)
		if code != 200 {
			t.Fatalf("up the line: %d %v", code, b)
		}
		return b["got"].(float64), b["sent"].(float64)
	}
	event := func(what string) []any {
		return []any{map[string]any{"at": int64(clock()), "mac": mac, "stage": "auth", "what": what}}
	}
	at := func(what string) (time.Time, map[string]any) {
		t.Helper()
		_, v := f.do("GET", path, "griff", nil)
		for _, e := range v["events"].([]any) {
			e := e.(map[string]any)
			if e["what"] == what {
				when, err := time.Parse(time.RFC3339Nano, e["at"].(string))
				must(t, err)
				return when, e
			}
		}
		t.Fatalf("%s is not in the trace: %v", what, v["events"])
		return time.Time{}, nil
	}

	// It has said nothing of its clock yet: its word is taken to have come
	// the moment it was sent, which is right to the time it took.
	t1 := clock()
	got, sent := send(map[string]any{"events": event("before it knows")})
	t4 := clock()
	if when, e := at("before it knows"); math.Abs(float64(time.Since(when).Milliseconds())) > 500 || e["lag_ms"] != nil {
		t.Errorf("before it knows: %v ago, %v", time.Since(when), e)
	}
	if got > sent || math.Abs(got-float64(time.Now().UnixMilli())) > 2000 {
		t.Errorf("when it came and when the answer left: %v %v", got, sent)
	}
	// As NTP reckons it: half of what the way there says and the way back.
	offset := ((got - t1) + (sent - t4)) / 2
	rtt := (t4 - t1) - (sent - got)
	if math.Abs(offset-behind) > 50 || rtt < 0 {
		t.Fatalf("reckoned %v ms behind over %v ms, of %v", offset, rtt, behind)
	}
	// Said, every time it names is put on the manager's clock by it: right
	// to what the reckoning can say, half the way there and back, and the
	// millisecond the manager's own times are given in.
	slack := time.Duration((rtt/2 + 1) * float64(time.Millisecond))
	send(map[string]any{"offset": offset, "rtt": rtt, "events": event("once it knows")})
	when, e := at("once it knows")
	if d := time.Since(when); d < -slack || d > 500*time.Millisecond {
		t.Errorf("once it knows: %v ago", d)
	}
	if lag, ok := e["lag_ms"].(float64); !ok || lag < 0 || lag > 500 {
		t.Errorf("how long it took to come: %v", e["lag_ms"])
	}
	// And what it sends without saying so again goes by what it last said.
	send(map[string]any{"events": event("without saying again")})
	if when, _ := at("without saying again"); time.Since(when) > 500*time.Millisecond || time.Since(when) < -slack {
		t.Errorf("without saying again: %v ago", time.Since(when))
	}
	// Whoever reads the trace is shown the AP's line and its clock. It
	// holds none yet. (The office's AP, which never enrolled, is the other
	// of the two.)
	if _, v := f.do("GET", path, "griff", nil); v["aps"] != float64(2) || len(v["live"].([]any)) != 0 {
		t.Errorf("no line held: %v %v", v["aps"], v["live"])
	}
	lines, hang := f.lineOf("/v1/ap/live", token)
	defer hang()
	next(t, lines, "as it joins", func(map[string]any) bool { return true })
	_, v := f.do("GET", path, "griff", nil)
	live := v["live"].([]any)
	if len(live) != 1 {
		t.Fatalf("its line: %v", v["live"])
	}
	l := live[0].(map[string]any)
	if l["ap"] != ap || l["ap_name"] == nil || math.Abs(l["offset_ms"].(float64)-behind) > 50 || l["rtt_ms"] == nil {
		t.Errorf("its line, as read: %v", l)
	}
	// A clock said to be further off than can be is not believed.
	send(map[string]any{"offset": 1e15, "events": event("an offset beyond belief")})
	// Within a millisecond of now the other way too: an event's time is kept
	// in whole milliseconds, and a clock that moves in steps may be behind it.
	if when, _ := at("an offset beyond belief"); time.Since(when) > 500*time.Millisecond || time.Since(when) < -time.Millisecond {
		t.Errorf("an offset beyond belief: %v ago", time.Since(when))
	}
	// A reckoning that puts a thing after the manager had it is wrong by
	// that much: the thing took no time to come, and never less.
	send(map[string]any{"offset": offset + 5000, "rtt": rtt, "events": event("after it came")})
	if _, e := at("after it came"); e["lag_ms"] != 0.0 || math.Signbit(e["lag_ms"].(float64)) {
		t.Errorf("after it came, it took: %v", e["lag_ms"])
	}
}

// TestTraceLive: whoever watches a trace is sent each thing as it comes
// (0145), and its start and stop.
func TestTraceLive(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()
	const mac = "7e:2a:ea:04:b1:8f"
	path := "/v1/clients/" + mac + "/trace"

	lines, hang := f.lineOf(path+"/live", f.tokens["griff"])
	defer hang()
	first, _ := next(t, lines, "as it is first read", func(v map[string]any) bool { return v["mac"] == mac })
	if first["tracing"] != false || len(first["events"].([]any)) != 0 {
		t.Errorf("before it is traced: %v", first)
	}
	if code, v := f.do("POST", path, "griff", nil); code != 200 {
		t.Fatalf("asked: %d %v", code, v)
	}
	next(t, lines, "that it is traced", func(v map[string]any) bool { return v["tracing"] == true })

	say := func(what string) {
		t.Helper()
		now := float64(time.Now().UnixMicro()) / 1000
		code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"sent": now, "offset": 0, "rtt": 1,
			"events": []any{map[string]any{"at": int64(now), "mac": mac, "network": "sweet", "band": "5g", "stage": "auth", "what": what}}}, nil)
		if code != 200 || b["kept"] != float64(1) {
			t.Fatalf("up the line: %d %v", code, b)
		}
	}
	has := func(what string) func(map[string]any) bool {
		return func(v map[string]any) bool {
			events, _ := v["events"].([]any)
			return len(events) == 1 && events[0].(map[string]any)["what"] == what
		}
	}
	say("authenticated")
	_, took := next(t, lines, "the first step", has("authenticated"))
	if took > 200*time.Millisecond {
		t.Errorf("the watcher had it after %v", took)
	}
	// Each comes once, from where the last left off.
	say("associated")
	next(t, lines, "the second step", has("associated"))

	// An AP taking up its line is said too.
	ap, hangAP := f.lineOf("/v1/ap/live", token)
	defer hangAP()
	next(t, ap, "as the AP joins", func(map[string]any) bool { return true })
	next(t, lines, "that an AP holds its line", func(v map[string]any) bool {
		live, _ := v["live"].([]any)
		return len(live) == 1
	})
	if code, v := f.do("DELETE", path, "griff", nil); code != 200 {
		t.Fatalf("stopped: %d %v", code, v)
	}
	next(t, lines, "that it is stopped", func(v map[string]any) bool { return v["tracing"] == false })

	// One who watches from where a look left off is sent only what is new.
	later, hang2 := f.lineOf(path+"/live?after=1", f.tokens["griff"])
	defer hang2()
	if v, _ := next(t, later, "from where a look left off", func(v map[string]any) bool { return v["mac"] == mac }); len(v["events"].([]any)) != 1 {
		t.Errorf("after the first: %v", v["events"])
	}
	if code, b := f.do("GET", path+"/live?after=soon", "griff", nil); code != 400 {
		t.Errorf("after=soon: %d %v", code, b)
	}
	if code, b := f.do("GET", "/v1/clients/nonsense/trace/live", "griff", nil); code != 400 {
		t.Errorf("no MAC: %d %v", code, b)
	}
}

// TestTraceLiveOverHTTP2: a browser holds the answer over HTTP/2, where a
// stream is ended when the limit set for a write passes, written to or
// not. The limit is set past the manager's next word, so the answer lives
// through as many of them as it likes.
func TestTraceLiveOverHTTP2(t *testing.T) {
	beat, slow := liveBeat, liveSlow
	liveBeat, liveSlow = 150*time.Millisecond, 50*time.Millisecond
	defer func() { liveBeat, liveSlow = beat, slow }()

	f := newFixture(t)
	srv := httptest.NewUnstartedServer(f.api.Handler())
	srv.EnableHTTP2 = true
	// As the manager is served: limits that suit answers that end.
	srv.Config.ReadTimeout, srv.Config.WriteTimeout = 100*time.Millisecond, 100*time.Millisecond
	srv.StartTLS()
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/v1/clients/7e:2a:ea:04:b1:8f/trace/live", nil)
	must(t, err)
	req.Header.Set("Authorization", "Bearer "+f.tokens["griff"])
	resp, err := srv.Client().Do(req)
	must(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ProtoMajor != 2 {
		t.Fatalf("asked: %d over HTTP/%d", resp.StatusCode, resp.ProtoMajor)
	}
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	// Six of the manager's words, each a limit and more after the last.
	for i := 0; i < 6; i++ {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("the answer ended after %d lines", i)
			}
			if i > 0 && l != "{}" {
				t.Errorf("line %d: %s", i, l)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("nothing after %d lines", i)
		}
	}
}

// TestTraceKeepsAnAPsOwnSpacing: each sending is put on the manager's clock
// by its own reckoning, and two reckonings differ by a part of a
// millisecond. What one AP timed close together keeps the spacing the AP
// timed it by, so two things a millisecond apart are not turned round; what
// it saw a while later goes by the newer reckoning.
func TestTraceKeepsAnAPsOwnSpacing(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()
	const mac = "7e:2a:ea:04:b1:8f"
	path := "/v1/clients/" + mac + "/trace"
	if code, v := f.do("POST", path, "griff", nil); code != 200 {
		t.Fatalf("asked: %d %v", code, v)
	}
	now := time.Now().UnixMilli()
	send := func(offset float64, at int64, what string) {
		t.Helper()
		code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"sent": float64(now), "offset": offset, "rtt": 1,
			"events": []any{map[string]any{"at": at, "mac": mac, "stage": "key", "what": what}}}, nil)
		if code != 200 || b["kept"] != float64(1) {
			t.Fatalf("up the line: %d %v", code, b)
		}
	}
	// Message 2 and message 3, a millisecond apart by the AP's clock, sent
	// apart: the second with a reckoning 1.6 ms the other way.
	send(0.8, now-5000, "message 2 of 4 came from it")
	send(-0.8, now-4999, "message 3 of 4 sent to it")
	// The same millisecond, and one the AP timed a little before.
	send(0.5, now-4999, "message 4 of 4 came from it")
	send(-0.5, now-5003, "message 1 of 4 sent to it")
	// Three seconds on, the AP's clock has been reckoned 2 ms further off.
	send(2.8, now-2000, "key handshake completed")
	_, v := f.do("GET", path, "griff", nil)
	var at []time.Time
	for _, e := range v["events"].([]any) {
		when, err := time.Parse(time.RFC3339Nano, e.(map[string]any)["at"].(string))
		must(t, err)
		at = append(at, when)
	}
	if len(at) != 5 {
		t.Fatalf("kept %d of 5", len(at))
	}
	gap := func(i, j int) time.Duration { return at[j].Sub(at[i]) }
	if gap(0, 1) != time.Millisecond || gap(1, 2) != 0 || gap(0, 3) != -3*time.Millisecond {
		t.Errorf("as the AP timed them: %v, %v, %v", gap(0, 1), gap(1, 2), gap(0, 3))
	}
	// 3000 ms by the AP's clock, and the 2 ms the newer reckoning adds.
	if want := 3002 * time.Millisecond; gap(0, 4) != want {
		t.Errorf("a while later, by the newer reckoning: %v, want %v", gap(0, 4), want)
	}
}

// TestLiveDown: an AP is down once its line went and it opened no other
// (0165): at once where it was found gone with it, after liveGrace where
// it closed the line itself; never where its line was not seen to go.
func TestLiveDown(t *testing.T) {
	var h liveHub
	const ap = hierarchy.NodeID("ap-1")
	t0 := time.Unix(1_800_000_000, 0)
	if _, down := h.down(ap, t0); down {
		t.Fatal("an AP whose line was never seen to go is down")
	}
	l := h.join(ap, t0)
	if _, down := h.down(ap, t0); down {
		t.Fatal("an AP holding its line is down")
	}
	h.leave(ap, l, t0, false)
	if _, down := h.down(ap, t0.Add(liveGrace-time.Second)); down {
		t.Fatal("an AP that closed its line is down within the grace")
	}
	if at, down := h.down(ap, t0.Add(liveGrace)); !down || !at.Equal(t0) {
		t.Fatalf("an AP that closed its line and opened no other: %v %v", at, down)
	}
	l = h.join(ap, t0.Add(20*time.Second))
	if _, down := h.down(ap, t0.Add(21*time.Second)); down {
		t.Fatal("an AP back on its line is down")
	}
	h.leave(ap, l, t0.Add(30*time.Second), true)
	if at, down := h.down(ap, t0.Add(30*time.Second)); !down || !at.Equal(t0.Add(30*time.Second)) {
		t.Fatalf("an AP found gone with its line: %v %v", at, down)
	}
	// A line another took the place of is not one gone.
	old := h.join(ap, t0.Add(40*time.Second))
	h.join(ap, t0.Add(41*time.Second))
	h.leave(ap, old, t0.Add(42*time.Second), true)
	if _, down := h.down(ap, t0.Add(43*time.Second)); down {
		t.Fatal("the end of a line another replaced made the AP down")
	}
}

// TestAPsAlive: GET /v1/aps/alive and GET /v1/aps say which APs are down,
// to whoever may view them (0165).
func TestAPsAlive(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	id := hierarchy.NodeID(ap)
	if code, v := f.do("GET", "/v1/aps/alive", "griff", nil); code != 200 || len(v["down"].(map[string]any)) != 0 {
		t.Fatalf("none down: %d %v", code, v)
	}
	f.api.live.leave(id, f.api.live.join(id, time.Now()), time.Now(), true)
	code, v := f.do("GET", "/v1/aps/alive", "griff", nil)
	if code != 200 || v["down"].(map[string]any)[ap] == nil {
		t.Fatalf("one down: %d %v", code, v)
	}
	_, body := f.do("GET", "/v1/aps", "griff", nil)
	for _, a := range body["aps"].([]any) {
		if m := a.(map[string]any); m["id"] == ap && m["down"] == nil {
			t.Fatalf("the AP list does not say it is down: %v", m)
		}
	}
	// office has no role in Locations, so it is told of none.
	if _, v := f.do("GET", "/v1/aps/alive", "office", nil); len(v["down"].(map[string]any)) != 0 {
		t.Fatalf("office is told of %v", v["down"])
	}
}
