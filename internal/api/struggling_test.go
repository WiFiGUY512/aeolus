package api

import (
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A client a person named that struggles is an alert on the AP it is on
// (0233): over two hours it moved between that AP's bands again and again,
// what was sent to it was sent again and again, and it ran slowly on 5 GHz
// where it heard the AP well, as a phone did on the Living Room AP's
// channel 169 (2026-10-28). One no one named is not, nor one that has gone.
// Resends are a reason again (0242), and the rate's bar slides with the
// signal.
func TestStrugglingClient(t *testing.T) {
	f := newFixture(t)
	if code, b := f.change("griff", map[string]any{"kind": "add-builtins"}); code != 200 {
		t.Fatalf("%d %v", code, b)
	}
	radios := []any{map[string]any{"radio": "radio0", "band": "5g", "htmodes": []string{"HE20", "HE40"}}, map[string]any{"radio": "radio3", "band": "6g", "htmodes": []string{"HE20", "HE160"}}}
	living := strings.Split(f.enrollAt("Living", "02:00:00:00:00:6a", "office", radios, map[string]int{"5g": 36, "6g": 5}), "|")
	ap := hierarchy.NodeID(living[0])
	const phone, other, gone = "fe:8f:ee:d3:60:e3", "fe:8f:ee:d3:60:e4", "fe:8f:ee:d3:60:e5"
	for mac, name := range map[string]string{phone: "a phone", gone: "Away phone"} {
		if err := f.api.conds.NameClient(mac, name, "griff"); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	// Twelve busy minutes on 5 GHz each, heard at -55 dBm, at 97 Mbit/s,
	// two in five frames sent again; the one gone was last heard 30 minutes
	// ago.
	var minutes []conditions.ClientMinute
	for _, mac := range []string{phone, other, gone} {
		end := now.Add(-time.Minute)
		if mac == gone {
			end = now.Add(-30 * time.Minute)
		}
		for i := 11; i >= 0; i-- {
			sig, rate := -55, 97.0
			minutes = append(minutes, conditions.ClientMinute{MAC: mac, At: end.Add(-time.Duration(i) * time.Minute).Truncate(time.Minute), AP: ap, Band: "5g", SSID: "Home Wi-Fi",
				Signal: &sig, TxRate: &rate, Secs: 60, Counts: conditions.Counts{TxPackets: 1000, TxRetries: 400, TxBytes: 3 << 20, RxBytes: 1 << 20}})
		}
	}
	if err := f.api.conds.RecordMinutes(nil, minutes); err != nil {
		t.Fatal(err)
	}
	// Back and forth between the AP's 5 and 6 GHz, five times let on: four
	// moves between its bands.
	for i, band := range []string{"5g", "6g", "5g", "6g", "5g"} {
		for _, mac := range []string{phone, other} {
			rec := map[string]any{"mac": mac, "bss": "phy0-ap2", "network": "sweet", "ssid": "Home Wi-Fi", "band": band,
				"started": now.Add(-time.Duration(100-20*i) * time.Minute).UnixMilli(), "took_ms": 20, "outcome": "connected", "stage": "key", "reason": "", "events": []any{},
				"how": map[string]any{"re": true, "alg": "ft", "on_ms": 10}}
			if code, _, b := f.apDo("POST", "/v1/ap/connections", living[1], map[string]any{"connections": []any{rec}}, nil); code != 200 || b["recorded"] != 1.0 {
				t.Fatalf("an attempt: %d %v", code, b)
			}
		}
	}
	_, view := f.do("GET", "/v1/alerts", "griff", nil)
	list, _ := view["alerts"].([]any)
	var found []map[string]any
	for _, a := range list {
		if m := a.(map[string]any); m["kind"] == "struggling" {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("struggling alerts: %v (all: %v)", found, list)
	}
	a := found[0]
	msg, _ := a["message"].(string)
	for _, want := range []string{"a phone (" + phone + ") struggles: ", "it moved between 5 GHz and 6 GHz on Living 4 times in 2 hours",
		"40% of the frames sent to it were sent again", "on 5 GHz it ran at 97 Mbit/s, heard at -55 dBm, where under 120 is slow"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in %q", want, msg)
		}
	}
	if a["ap"] != living[0] || a["severity"] != "warning" || a["key"] != "struggling:"+phone {
		t.Errorf("the alert: %v", a)
	}

	// Steady: no moves, few resends, fast. Nothing struggles.
	var steady []conditions.ClientMinute
	for i := 0; i < 12; i++ {
		sig, rate := -55, 310.0
		steady = append(steady, conditions.ClientMinute{MAC: "fe:8f:ee:d3:60:e6", At: now.Add(-time.Duration(i+1) * time.Minute).Truncate(time.Minute), AP: ap, Band: "5g", SSID: "Home Wi-Fi",
			Signal: &sig, TxRate: &rate, Secs: 60, Counts: conditions.Counts{TxPackets: 1000, TxRetries: 50, TxBytes: 3 << 20}})
	}
	why, _, _ := struggles(steady[len(steady)-1:], nil, func(id hierarchy.NodeID) string { return string(id) })
	if len(why) != 0 {
		t.Errorf("one minute says nothing: %v", why)
	}
	for i, j := 0, len(steady)-1; i < j; i, j = i+1, j-1 {
		steady[i], steady[j] = steady[j], steady[i]
	}
	if why, _, _ := struggles(steady, nil, func(id hierarchy.NodeID) string { return string(id) }); len(why) != 0 {
		t.Errorf("a steady client struggles: %v", why)
	}
	name := func(id hierarchy.NodeID) string { return string(id) }
	// Fast, with three in five frames sent again: the resends are a reason
	// of their own (0242), as on the Living Room AP's faulty radio.
	for i := range steady {
		steady[i].TxRetries = 600
	}
	if why, _, _ := struggles(steady, nil, name); len(why) != 1 || why[0] != "60% of the frames sent to it were sent again" {
		t.Errorf("a client with resends: %v", why)
	}
	// Heard at -66 dBm, the bar is 81 Mbit/s (0242, 0250): at 60 it is
	// slow, at 100 not; heard at -78, there is none, and 20 is not slow.
	for _, c := range []struct {
		sig  int
		rate float64
		want string
	}{{-66, 60, "on 5 GHz it ran at 60 Mbit/s, heard at -66 dBm, where under 81 is slow"}, {-66, 100, ""}, {-78, 20, ""}} {
		for i := range steady {
			sig, rate := c.sig, c.rate
			steady[i].Signal, steady[i].TxRate, steady[i].TxRetries = &sig, &rate, 50
		}
		why, _, _ := struggles(steady, nil, name)
		if got := strings.Join(why, "; "); got != c.want {
			t.Errorf("heard at %d, at %.0f: %q, want %q", c.sig, c.rate, got, c.want)
		}
	}
	for _, c := range []struct {
		sig  int
		bar  float64
		some bool
	}{{-50, 120, true}, {-60, 120, true}, {-66, 81, true}, {-75, 22.5, true}, {-76, 0, false}} {
		if bar, ok := slowUnder(c.sig); bar != c.bar || ok != c.some {
			t.Errorf("slowUnder(%d) = %v %v, want %v %v", c.sig, bar, ok, c.bar, c.some)
		}
	}
	if got := durationWords(2 * time.Hour); got != "2 hours" {
		t.Errorf("durationWords = %q", got)
	}
}

// The alert is on the AP a client struggles on (0306), not the one it is
// on now: the AP whose bands it moved between, or else the one it was busy
// on the most minutes; so it does not move from AP to AP with the client.
func TestStrugglesWhere(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	name := func(id hierarchy.NodeID) string { return string(id) }
	minute := func(ap string, ago int, retries int64) conditions.ClientMinute {
		sig, rate := -55, 310.0
		return conditions.ClientMinute{MAC: "fe:8f:ee:d3:60:e3", At: now.Add(-time.Duration(ago) * time.Minute), AP: hierarchy.NodeID(ap), Band: "5g",
			Signal: &sig, TxRate: &rate, Secs: 60, Counts: conditions.Counts{TxPackets: 1000, TxRetries: retries, TxBytes: 3 << 20}}
	}
	// Twelve busy minutes, a third of them sent again: eight on b, four on a.
	var ms []conditions.ClientMinute
	for i := 12; i >= 1; i-- {
		ap := "b"
		if i <= 4 {
			ap = "a"
		}
		ms = append(ms, minute(ap, i, 400))
	}
	if why, _, where := struggles(ms, nil, name); len(why) != 1 || where != "b" {
		t.Fatalf("resends: %v on %q, want on b", why, where)
	}
	// Back and forth between a's bands, though it is busier on b: on a.
	var tried []conditions.Connection
	for i, band := range []string{"5g", "6g", "5g", "6g", "5g"} {
		tried = append([]conditions.Connection{{AP: "a", Band: band, Outcome: "connected", Started: now.Add(-time.Duration(100-20*i) * time.Minute)}}, tried...)
	}
	if why, _, where := struggles(ms, tried, name); len(why) != 2 || where != "a" {
		t.Fatalf("moves on a: %v on %q, want on a", why, where)
	}
	// Nothing to say: no AP.
	for i := range ms {
		ms[i].TxRetries = 10
	}
	if why, _, where := struggles(ms, nil, name); len(why) != 0 || where != "" {
		t.Fatalf("steady: %v on %q", why, where)
	}
}

// A client that struggles where it did better on another AP lately says so
// (0309): the Living Room TV, on the C260's 5 GHz at -74 dBm with 65% sent
// again since the Living Room AP restarted, had been heard there at -60
// with 4%. Not another band, not too few minutes, not a place no better.
func TestBetterPlace(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	name := func(id hierarchy.NodeID) string {
		return map[hierarchy.NodeID]string{"c260": "C260", "lr": "Living-Room-C-360", "garage": "Garage"}[id]
	}
	minute := func(ap, band string, ago, sig int, sent, again int64) conditions.ClientMinute {
		return conditions.ClientMinute{MAC: "28:07:08:7e:13:ea", At: now.Add(-time.Duration(ago) * time.Minute), AP: hierarchy.NodeID(ap), Band: band,
			Signal: &sig, Secs: 60, Counts: conditions.Counts{TxPackets: sent, TxRetries: again, TxBytes: 30 << 20}}
	}
	var here, lately []conditions.ClientMinute
	for i := 1; i <= 20; i++ {
		here = append(here, minute("c260", "5g", i, -74, 20000, 13000))
	}
	for i := 0; i < 30; i++ {
		lately = append(lately,
			minute("lr", "5g", 1000+i, -60, 20000, 800), // 30 busy minutes, 4%: better
			minute("lr", "2g", 1100+i, -46, 5000, 100),  // another band: not compared
			minute("garage", "5g", 1200+i, -70, 20000, 9000))
	}
	lately = append(lately, minute("garage", "5g", 1300, -50, 20000, 100)) // no better in the middle
	b := betterPlace(here, append(lately, here...), "c260", name)
	if b == nil || b.AP != "Living-Room-C-360" || b.Band != "5g" || b.Signal != -60 || b.Minutes != 30 || b.Resent != 0.04 {
		t.Fatalf("better place: %+v", b)
	}
	// Ten busy minutes at least.
	if b := betterPlace(here, lately[:15], "c260", name); b != nil {
		t.Fatalf("five busy minutes on the LR: %+v", b)
	}
	// Not busy here: nothing to compare.
	for i := range here {
		here[i].TxBytes = 1 << 10
	}
	if b := betterPlace(here, lately, "c260", name); b != nil {
		t.Fatalf("idle here: %+v", b)
	}
}
