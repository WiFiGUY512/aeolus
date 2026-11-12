package api

import (
	"strings"
	"testing"
	"time"
)

// A test window (0344): maintenance with hold and until. The AP is told to
// hold still until then, in hold, with the end in Unix seconds; the manager
// names it no agent meanwhile, and its alerts say it is in one. Without
// until, or with hold off, it is plain maintenance; past until, the AP is
// named its agent again, and the hold it was sent stays as it was, which it
// is past.
func TestHeldWindow(t *testing.T) {
	f := newFixture(t)
	_, cur := f.agents()
	ap, token, version := f.adopted()
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 90, "wireless_missing": true}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	set := func(values map[string]any) {
		t.Helper()
		if code, b := f.change("griff", map[string]any{"kind": "set", "tree": "locations", "node": ap, "values": values}); code != 200 {
			t.Fatalf("set: %d %v", code, b)
		}
	}
	poll := func() (string, map[string]any) {
		t.Helper()
		code, h, body := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code != 200 || body["state"] != "ready" {
			t.Fatalf("poll: %d %v", code, body)
		}
		hold, _ := body["config"].(map[string]any)["hold"].(map[string]any)
		return h.Get(AgentHeader), hold
	}
	said := func() any {
		t.Helper()
		_, body := f.do("GET", "/v1/alerts", "griff", nil)
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["ap"] == ap && m["kind"] == "wireless-missing" {
				return m["maintenance"]
			}
		}
		t.Fatalf("no wireless-missing alert: %v", body)
		return nil
	}

	set(map[string]any{"maintenance.on": true, "maintenance.until": "2099-01-01T12:00Z", "maintenance.why": "porting's -136", "maintenance.hold": true})
	end := float64(time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC).Unix())
	if agent, hold := poll(); agent != "" || hold["until"] != end || hold["why"] != "porting's -136" {
		t.Fatalf("in a test window: agent %q, hold %v", agent, hold)
	}
	if m := said(); m != "in a test window, held still until 2099-01-01 12:00 UTC: porting's -136" {
		t.Fatalf("alert in a test window: %v", m)
	}

	// Hold off: plain maintenance, no hold, the agent named.
	set(map[string]any{"maintenance.hold": false})
	if agent, hold := poll(); agent != cur.Hash || hold != nil {
		t.Fatalf("hold off: agent %q, hold %v", agent, hold)
	}
	if m, _ := said().(string); !strings.HasPrefix(m, "in maintenance until") {
		t.Fatalf("plain maintenance: %v", m)
	}

	// Hold without an end holds nothing.
	if code, b := f.change("griff", map[string]any{"kind": "unset", "tree": "locations", "node": ap, "paths": []string{"maintenance.until"}}); code != 200 {
		t.Fatalf("unset until: %d %v", code, b)
	}
	set(map[string]any{"maintenance.hold": true})
	if agent, hold := poll(); agent != cur.Hash || hold != nil {
		t.Fatalf("hold without until: agent %q, hold %v", agent, hold)
	}

	// Past its end: the agent named again; the AP keeps the end it was
	// sent, which its clock is past.
	set(map[string]any{"maintenance.until": "2020-01-01"})
	if agent, hold := poll(); agent != cur.Hash || hold["until"] != float64(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()) {
		t.Fatalf("past its end: agent %q, hold %v", agent, hold)
	}
	if m := said(); m != nil {
		t.Fatalf("alert past the window: %v", m)
	}
}

// An AP with a scan radio in a test window (0344) is asked for no client
// test on a schedule; once it is over, it is.
func TestHeldWindowTests(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	serving := map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40", "HE80"}}
	spare := map[string]any{"radio": "radio2", "band": "5g", "htmodes": []string{"HE20"}, "reserved": true}
	scanner := strings.Split(f.enrollAt("Scanner", "02:00:00:00:00:4a", "office", []any{serving, spare}, map[string]int{"5g": 36}), "|")
	say := func() {
		t.Helper()
		_, _, cfg := f.apDo("GET", "/v1/ap/config", scanner[1], nil, nil)
		body := map[string]any{"version": cfg["version"], "uptime": 60, "bssids": []string{"02:00:00:00:00:4a"},
			"rrm": map[string]any{"address": "192.0.2.1", "advertised": []string{"5g"}, "neighbours": []any{}, "ratings": []any{}, "moves": []any{}}}
		if code, _, b := f.apDo("POST", "/v1/ap/state", scanner[1], body, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
	}
	change := func(op map[string]any) {
		t.Helper()
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	change(map[string]any{"kind": "set", "tree": "locations", "node": "office", "path": "assurance.tests.every", "value": 15})
	change(map[string]any{"kind": "set", "tree": "locations", "node": scanner[0], "values": map[string]any{
		"maintenance.on": true, "maintenance.until": "2099-01-01T12:00Z", "maintenance.hold": true}})
	say()
	if got := f.api.assure(f.now()); len(got) != 0 {
		t.Fatalf("asked of an AP in a test window: %v", got)
	}
	change(map[string]any{"kind": "unset", "tree": "locations", "node": scanner[0], "paths": []string{"maintenance.on", "maintenance.until", "maintenance.hold"}})
	say()
	if got := f.api.assure(f.now()); len(got) != 1 || got[0].AP != scanner[0] {
		t.Fatalf("after the window: %v", got)
	}
}
