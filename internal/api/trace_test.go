package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Live Trace (0144): whoever may operate an AP asks; every AP is told on
// its poll, where it holds no line; what the APs say of the client is kept
// in one line and read from where a look before left off, by those who may
// view each AP; and it ends when asked or when its time is up.
func TestTraceAClient(t *testing.T) {
	f := newFixture(t)
	ap, token, _ := f.adopted()
	const mac = "7e:2a:ea:04:b1:8f"
	path := "/v1/clients/" + mac + "/trace"

	// Not traced: nothing to read, and no word of it to the APs.
	if code, v := f.do("GET", path, "griff", nil); code != 200 || v["tracing"] != false || len(v["events"].([]any)) != 0 {
		t.Fatalf("before: %d %v", code, v)
	}
	_, head, _ := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if got := head.Get(TraceHeader); got != "" {
		t.Fatalf("no client traced, and the AP is told %q", got)
	}
	// What an AP says of a client not traced is passed over.
	said := func(events ...any) float64 {
		t.Helper()
		code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"sent": float64(time.Now().UnixMilli()), "offset": 0, "events": events}, nil)
		if code != 200 {
			t.Fatalf("the AP's word: %d %v", code, b)
		}
		return b["kept"].(float64)
	}
	now := time.Now().UnixMilli()
	event := func(ago int64, stage, what string, more map[string]any) map[string]any {
		e := map[string]any{"at": now - ago, "mac": mac, "network": "sweet", "band": "5g", "stage": stage, "what": what}
		for k, v := range more {
			e[k] = v
		}
		return e
	}
	if n := said(event(0, "auth", "authenticated", nil)); n != 0 {
		t.Fatalf("a client not traced: kept %v", n)
	}

	// One with no role in Locations may not ask; an operator may.
	if code, b := f.do("POST", path, "office", nil); code != 403 {
		t.Fatalf("the office asks: %d %v", code, b)
	}
	for _, bad := range []string{"/v1/clients/nonsense/trace", path + "?minutes=0", path + "?minutes=61", path + "?minutes=soon"} {
		if code, b := f.do("POST", bad, "griff", nil); code != 400 {
			t.Errorf("%s: %d %v", bad, code, b)
		}
	}
	code, v := f.do("POST", path+"?minutes=10", "griff", nil)
	if code != 200 || v["tracing"] != true || v["by"] != "griff" || v["mac"] != mac {
		t.Fatalf("asked: %d %v", code, v)
	}
	until, err := time.Parse(time.RFC3339Nano, v["until"].(string))
	if err != nil || time.Until(until) < 9*time.Minute || time.Until(until) > 10*time.Minute+time.Second {
		t.Errorf("until: %v %v", v["until"], err)
	}
	// Every AP is told at its poll, with the seconds left.
	_, head, _ = f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if got := head.Get(TraceHeader); !strings.HasPrefix(got, mac+"=") || strings.Contains(got, ",") {
		t.Fatalf("the AP is told %q", got)
	}

	// What it says is kept, oldest first; what is not well said is passed
	// over, and the rest of what came with it kept.
	yes := true
	n := said(
		event(3000, "heard", "heard asking", map[string]any{"signal": -61}),
		event(2000, "auth", "authenticated", map[string]any{"ok": yes}),
		event(1900, "lunch", "a stage there is none of", nil),
		event(1800, "assoc", "associated\nand more", nil),
		map[string]any{"at": now, "mac": "not a mac", "stage": "auth", "what": "x"},
		event(1500, "signal", "heard at -48 dBm", map[string]any{"signal": 12}),
		event(1000, "done", "online in 0.4 s", map[string]any{"ok": yes, "signal": -48}),
		// A clock far from the manager's: taken to be of now.
		event(40*60*1000, "end", "left", nil),
	)
	if n != 4 {
		t.Fatalf("kept %v, want 4", n)
	}
	code, v = f.do("GET", path, "griff", nil)
	events := v["events"].([]any)
	var got []string
	for _, e := range events {
		e := e.(map[string]any)
		got = append(got, fmt.Sprintf("%v %v %v %v", e["stage"], e["what"], e["ok"], e["signal"]))
	}
	if g, want := strings.Join(got, "; "), "heard heard asking <nil> -61; auth authenticated true <nil>; done online in 0.4 s true -48; end left <nil> <nil>"; code != 200 || g != want {
		t.Fatalf("as read: %d\n got %s\nwant %s", code, g, want)
	}
	first := events[0].(map[string]any)
	if first["ap"] != ap || first["ap_name"] == nil || first["network"] != "sweet" || first["band"] != "5g" {
		t.Errorf("an event, as kept: %v", first)
	}
	if at, err := time.Parse(time.RFC3339Nano, first["at"].(string)); err != nil || at.UnixMilli() != now-3000 {
		t.Errorf("when it was, by the AP's clock: %v %v", first["at"], err)
	}
	last := events[3].(map[string]any)
	if at, err := time.Parse(time.RFC3339Nano, last["at"].(string)); err != nil || time.Since(at) > time.Minute {
		t.Errorf("a time far from the manager's is of now: %v %v", last["at"], err)
	}
	// From where a look left off: nothing more, then what came since.
	next := fmt.Sprint(int64(v["next"].(float64)))
	if _, again := f.do("GET", path+"?after="+next, "griff", nil); len(again["events"].([]any)) != 0 || fmt.Sprint(int64(again["next"].(float64))) != next {
		t.Errorf("nothing new: %v", again)
	}
	said(event(0, "signal", "heard at -70 dBm", map[string]any{"signal": -70}))
	if _, again := f.do("GET", path+"?after="+next, "griff", nil); len(again["events"].([]any)) != 1 {
		t.Errorf("one more since: %v", again["events"])
	}
	if code, b := f.do("GET", path+"?after=soon", "griff", nil); code != 400 {
		t.Errorf("after=soon: %d %v", code, b)
	}
	// One who may view no AP is told it is traced, and sees none of it,
	// though a look gets as far.
	if _, o := f.do("GET", path, "office", nil); o["tracing"] != true || len(o["events"].([]any)) != 0 || o["next"] == float64(0) {
		t.Errorf("the office's look: %v", o)
	}
	if code, b := f.do("GET", path, "", nil); code != 401 {
		t.Errorf("no token: %d %v", code, b)
	}

	// Four at once, and no fifth; one traced already goes on.
	for i := 1; i <= 3; i++ {
		if code, b := f.do("POST", fmt.Sprintf("/v1/clients/02:00:00:00:00:0%d/trace", i), "griff", nil); code != 200 {
			t.Fatalf("trace %d: %d %v", i, code, b)
		}
	}
	if code, b := f.do("POST", "/v1/clients/02:00:00:00:00:09/trace", "griff", nil); code != 409 {
		t.Errorf("a fifth: %d %v", code, b)
	}
	if code, b := f.do("POST", path, "griff", nil); code != 200 || len(b["events"].([]any)) != 5 {
		t.Errorf("one traced already, asked again: %d %v", code, b)
	}
	_, head, _ = f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if got := head.Get(TraceHeader); strings.Count(got, "=") != 4 {
		t.Errorf("the AP is told of four: %q", got)
	}

	// Stopped: the APs are told no more, what was said can still be read,
	// and what an AP saw just before is still taken.
	if code, b := f.do("DELETE", path, "office", nil); code != 403 {
		t.Errorf("the office stops it: %d %v", code, b)
	}
	if code, b := f.do("DELETE", path, "griff", nil); code != 200 || b["tracing"] != false {
		t.Fatalf("stopped: %d %v", code, b)
	}
	_, head, _ = f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if got := head.Get(TraceHeader); strings.Contains(got, mac) || strings.Count(got, "=") != 3 {
		t.Errorf("after the stop the AP is told %q", got)
	}
	if n := said(event(0, "end", "left", nil)); n != 1 {
		t.Errorf("a word just after the stop: kept %v", n)
	}
	if _, b := f.do("GET", path, "griff", nil); b["tracing"] != false || len(b["events"].([]any)) != 6 {
		t.Errorf("read after the stop: %v", b)
	}
	// More than an AP may say at a time.
	many := make([]any, traceBatch+1)
	for i := range many {
		many[i] = event(0, "auth", "authenticated", nil)
	}
	if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"sent": float64(time.Now().UnixMilli()), "events": many}, nil); code != 400 {
		t.Errorf("too many at a time: %d %v", code, b)
	}
}

// TestTraceAsTheAgentSaysIt: the agent names every field of an event, and
// those it has nothing for are null. They are taken as not said.
func TestTraceAsTheAgentSaysIt(t *testing.T) {
	f := newFixture(t)
	_, token, _ := f.adopted()
	const mac = "30:86:2d:ef:2f:f0"
	path := "/v1/clients/" + mac + "/trace"
	if code, v := f.do("POST", path, "griff", nil); code != 200 {
		t.Fatalf("asked: %d %v", code, v)
	}
	now := time.Now().UnixMilli()
	code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"sent": float64(now), "offset": 0, "rtt": nil, "events": []any{
		map[string]any{"at": now - 5, "mac": mac, "network": "lab", "band": "5g", "stage": "signal", "what": "on, heard at -41 dBm", "ok": nil, "signal": -41},
		map[string]any{"at": now, "mac": mac, "network": nil, "band": nil, "stage": "end", "what": "disassociated", "ok": nil, "signal": nil},
	}}, nil)
	if code != 200 || b["kept"] != float64(2) {
		t.Fatalf("the agent's word: %d %v", code, b)
	}
	_, v := f.do("GET", path, "griff", nil)
	events := v["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("read: %v", v)
	}
	last := events[1].(map[string]any)
	for _, k := range []string{"network", "band", "ok", "signal"} {
		if _, there := last[k]; there {
			t.Errorf("%s was not said, and is read as %v", k, last[k])
		}
	}
}
