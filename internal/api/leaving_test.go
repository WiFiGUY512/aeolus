package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Each kind of leaving, by the first rule that matches (0255).
func TestLeaveKinds(t *testing.T) {
	n := func(v int) *int { return &v }
	for _, c := range []struct {
		d         leaveDetail
		kind      string
		trouble   bool
		wordsHave string
	}{
		{leaveDetail{By: "aeolus", Aeolus: "moved"}, "moved", false, "another AP"},
		{leaveDetail{By: "aeolus", Aeolus: "asked", Frame: "deauth", Reason: n(1)}, "asked", false, "a person"},
		{leaveDetail{By: "aeolus", Aeolus: "steered", Frame: "deauth", Reason: n(5)}, "steered", false, "steering"},
		{leaveDetail{By: "ap", Frame: "disassoc", Reason: n(12)}, "steered", false, "steering"},
		{leaveDetail{By: "aeolus", Aeolus: "off-air"}, "off-air", false, "off the air"},
		{leaveDetail{By: "aeolus", Aeolus: "something-new"}, "aeolus", false, "Aeolus"},
		{leaveDetail{By: "ap", Frame: "deauth", Reason: n(3), All: true}, "network", true, "went down"},
		{leaveDetail{By: "client", Frame: "deauth", Reason: n(3)}, "leaving", false, "leaving"},
		{leaveDetail{By: "client", Frame: "disassoc", Reason: n(8)}, "leaving", false, "leaving"},
		{leaveDetail{By: "client", Frame: "deauth", Reason: n(2)}, "client-other", true, "wrong"},
		{leaveDetail{By: "ap", Frame: "deauth", Reason: n(4)}, "idle", true, "idle"},
		{leaveDetail{By: "ap", Frame: "disassoc", Reason: n(34)}, "unheard", true, "unacknowledged"},
		{leaveDetail{By: "ap", Frame: "deauth", Reason: n(15)}, "keys", true, "keys"},
		{leaveDetail{By: "ap", Frame: "deauth", Reason: n(23)}, "signin", true, "sign-in"},
		{leaveDetail{By: "ap", Frame: "deauth", Reason: n(5)}, "busy", true, "clients"},
		{leaveDetail{By: "ap", Frame: "deauth", Reason: n(7)}, "state", true, "joined"},
		{leaveDetail{By: "ap", Frame: "deauth", Reason: n(99)}, "sent-off", true, "AP"},
		{leaveDetail{By: "none"}, "silent", true, "no word"},
	} {
		k := leaveKind(c.d)
		if k.kind != c.kind || k.trouble != c.trouble || !strings.Contains(k.words, c.wordsHave) {
			t.Errorf("%+v: %s %v %q, want %s %v (%q)", c.d, k.kind, k.trouble, k.words, c.kind, c.trouble, c.wordsHave)
		}
	}
}

// What a leaving's news says, as kept: an agent before 0255 says no why;
// what is out of its range is left out.
func TestLeaveOf(t *testing.T) {
	r := 34
	if d := leaveOf(clientNews{Kind: "left"}); d != nil {
		t.Errorf("no why: %+v", d)
	}
	if d := leaveOf(clientNews{Kind: "left", By: "someone"}); d != nil {
		t.Errorf("an unknown by: %+v", d)
	}
	d := leaveOf(clientNews{Kind: "left", By: "ap", Frame: "disassoc", Reason: &r, Network: "home-wi-fi"})
	if d == nil || d.By != "ap" || d.Frame != "disassoc" || d.Reason == nil || *d.Reason != 34 || d.Network != "home-wi-fi" {
		t.Errorf("the AP's: %+v", d)
	}
	big := 70000
	d = leaveOf(clientNews{Kind: "left", By: "client", Frame: "beacon", Reason: &big, Aeolus: "No Such Word!"})
	if d == nil || d.Frame != "" || d.Reason != nil || d.Aeolus != "" {
		t.Errorf("out of range: %+v", d)
	}
}

// A leaving from an AP's live line is kept and counted by kind, a band at a
// time, with how strongly the AP last heard the client.
func TestLeavingFromNews(t *testing.T) {
	f := newFixture(t)
	f.change("griff", map[string]any{"kind": "add-builtins"})
	radios := []any{map[string]any{"band": "5g"}}
	office := f.enrollAt("office", "20:05:b6:df:64:e0", "sandbox", radios, map[string]int{"5g": 157})
	ap, token, _ := strings.Cut(office, "|")
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 60}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	phone, laptop, plug := "92:8e:1b:c6:4c:2c", "3c:22:fb:00:00:01", "a4:cf:12:00:00:02"
	at := time.Now().Add(time.Second).UnixMilli()
	left := func(mac string, at int64, why map[string]any) map[string]any {
		n := map[string]any{"t": "news", "kind": "left", "mac": mac, "at": at, "ssid": "Home Wi-Fi", "band": "5g", "network": "sweet"}
		for k, v := range why {
			n[k] = v
		}
		return n
	}
	news := []any{
		left(phone, at, map[string]any{"by": "client", "frame": "deauth", "reason": 3}),
		left(laptop, at+10, map[string]any{"by": "ap", "frame": "disassoc", "reason": 34}),
		left(plug, at+20, map[string]any{"by": "ap", "frame": "disassoc", "reason": 34}),
		left(phone, at+30, map[string]any{"by": "none"}),
		// From an agent before 0255: no why, not kept.
		left(plug, at+40, nil),
	}
	if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"news": news, "sent": float64(time.Now().UnixMilli())}, nil); code != 200 {
		t.Fatalf("live: %d %v", code, b)
	}
	code, v := f.do("GET", "/v1/leaving?under="+ap, "griff", nil)
	if code != 200 {
		t.Fatalf("leaving: %d %v", code, v)
	}
	raw, _ := json.Marshal(v)
	var got leavingView
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 4 || got.Trouble != 3 || len(got.Bands) != 1 || got.Bands[0].Band != "5g" {
		t.Fatalf("counts: %s", raw)
	}
	if k := got.Bands[0].Kinds; len(k) != 3 || k[0].Kind != "unheard" || k[0].Count != 2 {
		t.Fatalf("kinds, the most first: %+v", k)
	}
	if l := got.Latest; len(l) != 4 || l[0].MAC != phone || l[0].Kind != "silent" || l[1].ReasonWords != "too many frames went unacknowledged" || l[1].APName != "office" {
		t.Fatalf("latest: %+v", l)
	}
	// One client's, and a band with none.
	if _, v := f.do("GET", "/v1/leaving?under="+ap+"&mac="+laptop, "griff", nil); v["total"] != float64(1) {
		t.Fatalf("the laptop's: %v", v)
	}
	if _, v := f.do("GET", "/v1/leaving?under="+ap+"&band=2g", "griff", nil); v["total"] != float64(0) {
		t.Fatalf("2.4 GHz: %v", v)
	}
	if code, _ := f.do("GET", "/v1/leaving?hours=0", "griff", nil); code != 400 {
		t.Fatalf("hours 0: %d", code)
	}
}

// A leaving keeps the client's last minute at the AP and how long it had sent
// nothing by the AP's latest report (0255), for telling a sleeping client
// sent off from one out of reach.
func TestLeavingKeepsLastMinute(t *testing.T) {
	f := newFixture(t)
	f.change("griff", map[string]any{"kind": "add-builtins"})
	radios := []any{map[string]any{"band": "2g"}}
	ce7f := f.enrollAt("ce7f", "e4:d1:24:be:3a:7f", "sandbox", radios, map[string]int{"2g": 6})
	ap, token, _ := strings.Cut(ce7f, "|")
	kettle := "bc:ff:4d:34:90:17"
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 60,
		"clients": []any{map[string]any{"mac": kettle, "network": "iot", "ssid": "IoT", "band": "2g", "connected": 300, "inactive_ms": 41000}}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	// Its minute, the next one so as to be clear of the report's time.
	at := (time.Now().Unix()/60 + 1) * 60
	said := map[string]any{"at": at, "radios": []any{map[string]any{"radio": "radio1", "band": "2g", "channel": 6, "width": 20}},
		"clients": []any{map[string]any{"mac": kettle, "band": "2g", "ssid": "IoT", "signal": -62, "signal_min": -64, "tx_packets": 233, "tx_retries": 0,
			"tx_failed": 5, "rx_packets": 0, "secs": 60}}}
	if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"minutes": []any{said}, "sent": float64(time.Now().UnixMilli())}, nil); code != 200 {
		t.Fatalf("minute: %d %v", code, b)
	}
	left := map[string]any{"t": "news", "kind": "left", "mac": kettle, "at": (at + 70) * 1000, "ssid": "IoT", "band": "2g", "network": "iot",
		"by": "ap", "frame": "disassoc", "reason": 34}
	if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"news": []any{left}, "sent": float64(time.Now().UnixMilli())}, nil); code != 200 {
		t.Fatalf("news: %d %v", code, b)
	}
	_, v := f.do("GET", "/v1/leaving?under="+ap, "griff", nil)
	raw, _ := json.Marshal(v)
	var got leavingView
	if err := json.Unmarshal(raw, &got); err != nil || len(got.Latest) != 1 {
		t.Fatalf("leaving: %s", raw)
	}
	l := got.Latest[0]
	if l.Kind != "unheard" || l.Signal == nil || *l.Signal != -62 || l.Last == nil || l.Last.TxFailed != 5 || l.Last.RxPackets != 0 || l.Last.AgoS != 10 {
		t.Fatalf("its last minute: %s", raw)
	}
	if l.InactiveMS == nil || *l.InactiveMS != 41000 || l.InactiveAgo == nil {
		t.Fatalf("its inactive time: %s", raw)
	}
}

// A leaving says where the client went next (0258): its first attempt on any
// AP after it left, or on another AP up to nextBefore before, where it had
// moved already; none where it made no attempt within nextWithin. The attempt
// it left, on the same AP before, is not it.
func TestLeavingSaysWhereNext(t *testing.T) {
	f := newFixture(t)
	f.change("griff", map[string]any{"kind": "add-builtins"})
	radios := []any{map[string]any{"band": "2g"}}
	office := f.enrollAt("office", "20:05:b6:df:64:e0", "sandbox", radios, map[string]int{"2g": 6})
	c360 := f.enrollAt("c360", "30:86:2d:ef:2f:ff", "sandbox", radios, map[string]int{"2g": 1})
	ap, token, _ := strings.Cut(office, "|")
	other, _, _ := strings.Cut(c360, "|")
	_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
	if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 60}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, b)
	}
	kettle, cooler, lamp, plug := "bc:ff:4d:34:90:17", "2c:f4:32:9b:ca:67", "d8:3b:da:70:d5:0c", "a4:cf:12:00:00:02"
	at := time.Now().Add(time.Second).Truncate(time.Second)
	tried := func(mac string, off time.Duration) conditions.Connection {
		return conditions.Connection{MAC: mac, Started: at.Add(off), Network: "iot", SSID: "IoT", Band: "2g", Outcome: "online",
			Record: json.RawMessage(`{"events":[]}`)}
	}
	if _, err := f.conds.RecordConnections(hierarchy.NodeID(ap), []conditions.Connection{
		tried(kettle, -5*time.Minute), tried(plug, 20*time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.conds.RecordConnections(hierarchy.NodeID(other), []conditions.Connection{
		tried(kettle, 97*time.Second), tried(cooler, -time.Second), tried(lamp, nextWithin+time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var news []any
	for _, mac := range []string{kettle, cooler, lamp, plug} {
		news = append(news, map[string]any{"t": "news", "kind": "left", "mac": mac, "at": at.UnixMilli(), "ssid": "IoT", "band": "2g", "network": "iot",
			"by": "ap", "frame": "disassoc", "reason": 34})
	}
	if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"news": news, "sent": float64(time.Now().UnixMilli())}, nil); code != 200 {
		t.Fatalf("news: %d %v", code, b)
	}
	_, v := f.do("GET", "/v1/leaving?under="+ap, "griff", nil)
	raw, _ := json.Marshal(v)
	var got leavingView
	if err := json.Unmarshal(raw, &got); err != nil || len(got.Latest) != 4 {
		t.Fatalf("leaving: %s", raw)
	}
	next := map[string]*leaveNext{}
	for _, l := range got.Latest {
		next[l.MAC] = l.Next
	}
	if n := next[kettle]; n == nil || string(n.AP) != other || n.APName != "c360" || n.AfterS != 97 || n.Outcome != "online" {
		t.Errorf("the kettle, back on c360 97 s on: %+v", n)
	}
	if n := next[cooler]; n == nil || string(n.AP) != other || n.AfterS != -1 {
		t.Errorf("the cooler, joining c360 a second before: %+v", n)
	}
	if n := next[plug]; n == nil || string(n.AP) != ap || n.AfterS != 20 {
		t.Errorf("the plug, back on the office 20 s on: %+v", n)
	}
	if n := next[lamp]; n != nil {
		t.Errorf("the lamp, not back within %v: %+v", nextWithin, n)
	}
}

// A client that drops off and comes back again and again, away a while each
// time, keeps dropping (0259): the leaving view says so, and so does its
// journey. A phone that moves between APs, back at once each time, does not.
func TestLeavingSaysWhoKeepsDropping(t *testing.T) {
	f := newFixture(t)
	f.change("griff", map[string]any{"kind": "add-builtins"})
	radios := []any{map[string]any{"band": "2g"}}
	office := f.enrollAt("office", "20:05:b6:df:64:e0", "sandbox", radios, map[string]int{"2g": 6})
	c360 := f.enrollAt("c360", "30:86:2d:ef:2f:ff", "sandbox", radios, map[string]int{"2g": 1})
	ap, _, _ := strings.Cut(office, "|")
	other, _, _ := strings.Cut(c360, "|")
	kettle, phone := "bc:ff:4d:34:90:17", "6c:ac:c2:cf:b6:3f"
	base := time.Now().Add(-40 * time.Minute).Truncate(time.Second)
	var events []conditions.ClientEvent
	tries := map[string][]conditions.Connection{}
	for i := 0; i < 5; i++ {
		from, to := ap, other
		if i%2 == 1 {
			from, to = other, ap
		}
		at := base.Add(time.Duration(i) * 7 * time.Minute)
		events = append(events,
			conditions.ClientEvent{AP: hierarchy.NodeID(from), MAC: kettle, At: at, Kind: "left", Band: "2g", SSID: "IoT",
				Detail: json.RawMessage(`{"by":"ap","frame":"disassoc","reason":34}`)},
			conditions.ClientEvent{AP: hierarchy.NodeID(from), MAC: phone, At: at.Add(time.Minute), Kind: "left", Band: "2g", SSID: "IoT",
				Detail: json.RawMessage(`{"by":"client","frame":"disassoc","reason":8}`)})
		tries[to] = append(tries[to],
			conditions.Connection{MAC: kettle, Started: at.Add(100 * time.Second), Network: "iot", SSID: "IoT", Band: "2g", Outcome: "online", Record: json.RawMessage(`{"events":[]}`)},
			conditions.Connection{MAC: phone, Started: at.Add(time.Minute), Network: "iot", SSID: "IoT", Band: "2g", Outcome: "online", Record: json.RawMessage(`{"events":[]}`)})
	}
	if err := f.conds.RecordClientEvents(events); err != nil {
		t.Fatal(err)
	}
	for id, list := range tries {
		if _, err := f.conds.RecordConnections(hierarchy.NodeID(id), list); err != nil {
			t.Fatal(err)
		}
	}
	_, v := f.do("GET", "/v1/leaving?under=sandbox", "griff", nil)
	raw, _ := json.Marshal(v)
	var got leavingView
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Dropping) != 1 {
		t.Fatalf("dropping: %s", raw)
	}
	d := got.Dropping[0]
	if d.MAC != kettle || d.Times != 5 || d.EveryS != 420 || d.AwayS != 100 || len(d.APs) != 2 || d.APs[0] != "office" {
		t.Fatalf("the kettle: %+v", d)
	}
	_, j := f.do("GET", "/v1/clients/"+kettle, "griff", nil)
	if !strings.Contains(fmt.Sprint(j["issues"]), "dropped off and came back 5 times") {
		t.Errorf("the kettle's journey: %v", j["issues"])
	}
	_, j = f.do("GET", "/v1/clients/"+phone, "griff", nil)
	if strings.Contains(fmt.Sprint(j["issues"]), "dropped off") {
		t.Errorf("the phone's journey: %v", j["issues"])
	}
}
