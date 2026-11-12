package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A client two APs list is the one it was last active on (0185), as the
// reports come in either order; the AP it left keeps it no longer.
func TestActivity(t *testing.T) {
	var a activity
	t0 := time.Date(2026, 10, 21, 3, 8, 0, 0, time.UTC)
	phone := "92:8e:1b:c6:4c:2c"
	// The living room's report: on it 872 s, quiet 133 s.
	if gone := a.note("living", t0, []wifiClient{{MAC: phone, Connected: 872, InactiveMS: 132890}}); len(gone) != 0 {
		t.Fatalf("the first to say: %v", gone)
	}
	// The office's, 20 s later: on it 153 s, quiet under a second. The office has it.
	if gone := a.note("office", t0.Add(20*time.Second), []wifiClient{{MAC: phone, Connected: 153, InactiveMS: 880}}); len(gone) != 0 {
		t.Fatalf("the office: %v", gone)
	}
	// The living room again, a minute on, still quiet: it is the office's.
	if gone := a.note("living", t0.Add(time.Minute), []wifiClient{{MAC: phone, InactiveMS: 192890}, {MAC: "aa:00:00:00:00:01"}}); !gone[phone] || len(gone) != 1 {
		t.Fatalf("the living room again: %v", gone)
	}
	st := func(at time.Time, clients ...map[string]any) *conditions.State {
		r, _ := json.Marshal(map[string]any{"clients": clients})
		return &conditions.State{At: at, Report: r}
	}
	if e := a.elsewhere("living", st(t0.Add(time.Minute), map[string]any{"mac": phone, "inactive_ms": 192890}, map[string]any{"mac": "aa:00:00:00:00:01"})); len(e) != 1 || e[0] != phone {
		t.Fatalf("elsewhere, the living room: %v", e)
	}
	if e := a.elsewhere("office", st(t0.Add(20*time.Second), map[string]any{"mac": phone, "inactive_ms": 880})); e != nil {
		t.Fatalf("elsewhere, the office: %v", e)
	}
	// It comes back to the living room, busy there: the living room's again.
	if gone := a.note("living", t0.Add(3*time.Minute), []wifiClient{{MAC: phone, Connected: 5, InactiveMS: 10}}); len(gone) != 0 {
		t.Fatalf("back in the living room: %v", gone)
	}
	if gone := a.note("office", t0.Add(3*time.Minute+10*time.Second), []wifiClient{{MAC: phone, InactiveMS: 60000}}); !gone[phone] {
		t.Fatalf("the office, left behind: %v", gone)
	}
	// A client gone an hour is forgotten.
	a.note("office", t0.Add(2*time.Hour), nil)
	if _, ok := a.last[phone]; ok {
		t.Fatalf("kept past an hour: %v", a.last)
	}
	if a.elsewhere("x", nil) != nil {
		t.Fatal("elsewhere with no report")
	}
}

// Through the API (0185): the AP a client left counts it no longer in its
// clients over time, and its condition names it, for the page.
func TestClientOnOneAP(t *testing.T) {
	f := newFixture(t)
	f.change("griff", map[string]any{"kind": "add-builtins"})
	radios := []any{map[string]any{"band": "5g"}}
	post := func(apTok string, clients ...map[string]any) string {
		t.Helper()
		ap, token, _ := strings.Cut(apTok, "|")
		_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 60, "clients": clients}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
		return ap
	}
	client := func(mac string, inactive int) map[string]any {
		return map[string]any{"mac": mac, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "connected": 200, "inactive_ms": inactive}
	}
	living := f.enrollAt("living", "30:86:2d:bc:a5:ff", "sandbox", radios, map[string]int{"5g": 104})
	office := f.enrollAt("office", "20:05:b6:df:64:e0", "sandbox", radios, map[string]int{"5g": 157})
	phone := "92:8e:1b:c6:4c:2c"
	officeAP := post(office, client(phone, 880))
	livingAP := post(living, client(phone, 132890), client("aa:00:00:00:00:01", 10))

	_, cfg := f.do("GET", "/v1/aps/"+livingAP+"/config", "griff", nil)
	if e, _ := cfg["condition"].(map[string]any)["elsewhere"].([]any); len(e) != 1 || e[0] != phone {
		t.Fatalf("the living room's condition: %v", cfg["condition"].(map[string]any)["elsewhere"])
	}
	_, cfg = f.do("GET", "/v1/aps/"+officeAP+"/config", "griff", nil)
	if _, ok := cfg["condition"].(map[string]any)["elsewhere"]; ok {
		t.Fatalf("the office's condition names one elsewhere: %v", cfg["condition"])
	}
	got := map[hierarchy.NodeID]int{}
	if err := f.conds.Uses(time.Now().Add(-time.Hour), func(u conditions.Use) error { got[u.AP] = u.Clients; return nil }); err != nil {
		t.Fatal(err)
	}
	if got[hierarchy.NodeID(officeAP)] != 1 || got[hierarchy.NodeID(livingAP)] != 1 {
		t.Fatalf("clients counted: %v", got)
	}
}

// Each AP's minutes say where a client is within a minute (0251): one that
// sent an AP anything in a minute is on it from then. The AP it left names
// it elsewhere, and the AP it is on names it as arrived until its report
// lists it. One hostapd keeps after it left, sending nothing, says nothing.
func TestActivityMinutes(t *testing.T) {
	var a activity
	t0 := time.Date(2026, 10, 30, 13, 20, 0, 0, time.UTC)
	phone := "6c:ac:c2:cf:b6:3f"
	st := func(at time.Time, clients ...map[string]any) *conditions.State {
		r, _ := json.Marshal(map[string]any{"clients": clients})
		return &conditions.State{At: at, Report: r}
	}
	minute := func(ap string, at time.Time, secs int, rx int64) conditions.ClientMinute {
		return conditions.ClientMinute{MAC: phone, At: at, AP: hierarchy.NodeID(ap), Band: "6g", SSID: "Home Wi-Fi", Secs: secs, Counts: conditions.Counts{RxPackets: rx}}
	}
	// ce7f's report: on it, quiet 2 s.
	a.note("ce7f", t0, []wifiClient{{MAC: phone, InactiveMS: 2000}})
	ce7f := st(t0, map[string]any{"mac": phone, "inactive_ms": 2000})
	// ce7f still has it a minute on, sending nothing: hostapd keeps it.
	a.minutes("ce7f", []conditions.ClientMinute{minute("ce7f", t0.Add(time.Minute), 60, 0)})
	if e := a.elsewhere("ce7f", ce7f); e != nil {
		t.Fatalf("elsewhere on a minute of nothing: %v", e)
	}
	// 57ff's minute: there 50 s of it, sending.
	a.minutes("57ff", []conditions.ClientMinute{minute("57ff", t0.Add(5*time.Minute), 50, 9)})
	if e := a.elsewhere("ce7f", ce7f); len(e) != 1 || e[0] != phone {
		t.Fatalf("elsewhere, ce7f: %v", e)
	}
	got := a.arrived("57ff", st(t0.Add(-time.Minute)))
	if len(got) != 1 || got[0].MAC != phone || got[0].Band != "6g" {
		t.Fatalf("arrived on 57ff: %v", got)
	}
	if a.arrived("ce7f", ce7f) != nil {
		t.Fatal("arrived on ce7f")
	}
	// 57ff's report lists it: not arrived any more, and its minutes keep it.
	a.note("57ff", t0.Add(6*time.Minute), []wifiClient{{MAC: phone, InactiveMS: 120000}})
	if got := a.arrived("57ff", st(t0.Add(6*time.Minute), map[string]any{"mac": phone})); got != nil {
		t.Fatalf("arrived once listed: %v", got)
	}
	if o := a.last[phone]; o.ap != "57ff" || !o.at.Equal(t0.Add(5*time.Minute+50*time.Second)) {
		t.Fatalf("its activity: %v", o)
	}
	// ce7f's report, still listing it quiet: it is 57ff's.
	if gone := a.note("ce7f", t0.Add(7*time.Minute), []wifiClient{{MAC: phone, InactiveMS: 400000}}); !gone[phone] {
		t.Fatalf("ce7f's report: %v", gone)
	}
	// A report on 57ff after its minutes, and no minute since: not arrived.
	if got := a.arrived("57ff", st(t0.Add(8*time.Minute))); got != nil {
		t.Fatalf("arrived before the report: %v", got)
	}
	// A minute that is older than what is known says nothing.
	a.minutes("ce7f", []conditions.ClientMinute{minute("ce7f", t0, 60, 5)})
	if a.last[phone].ap != "57ff" {
		t.Fatalf("an older minute moved it: %v", a.last[phone])
	}
}

// Through the API (0251): a minute on the office moves the phone there at
// once. The living room's condition names it elsewhere, the office's as
// arrived with what its minute came to, and the clients list has it on the
// office; the office's report then lists it.
func TestClientWhereMinutes(t *testing.T) {
	f := newFixture(t)
	f.change("griff", map[string]any{"kind": "add-builtins"})
	radios := []any{map[string]any{"band": "5g"}}
	living := f.enrollAt("living", "30:86:2d:bc:a5:ff", "sandbox", radios, map[string]int{"5g": 104})
	office := f.enrollAt("office", "20:05:b6:df:64:e0", "sandbox", radios, map[string]int{"5g": 157})
	phone := "92:8e:1b:c6:4c:2c"
	client := map[string]any{"mac": phone, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "connected": 200, "inactive_ms": 500, "host": "Phone-B"}
	report := func(apTok string, clients ...map[string]any) string {
		t.Helper()
		ap, token, _ := strings.Cut(apTok, "|")
		_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 60, "clients": clients}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
		return ap
	}
	// The living room still lists it, quiet two minutes: it left without
	// saying so.
	livingAP := report(living, map[string]any{"mac": phone, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "connected": 200, "inactive_ms": 120000, "host": "Phone-B"})
	officeAP := report(office)
	_, token, _ := strings.Cut(office, "|")
	// An office minute after its report, with the phone sending in it: the
	// next minute, so as to be well clear of the report's time whatever the
	// clock (the manager takes minutes up to five ahead).
	at := (time.Now().Unix()/60 + 1) * 60
	said := map[string]any{"at": at, "radios": []any{map[string]any{"radio": "radio0", "band": "5g", "channel": 157, "width": 80}},
		"clients": []any{map[string]any{"mac": phone, "band": "5g", "ssid": "Home Wi-Fi", "signal": -54, "signal_min": -56, "tx_rate": 960.7, "rx_rate": 1200.9,
			"tx_packets": 100, "tx_retries": 3, "tx_failed": 0, "rx_packets": 90, "tx_bytes": 5000, "rx_bytes": 2000, "secs": 60}}}
	if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"minutes": []any{said}, "sent": float64(time.Now().UnixMilli())}, nil); code != 200 {
		t.Fatalf("live: %d %v", code, b)
	}
	cond := func(ap string) map[string]any {
		t.Helper()
		_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
		return cfg["condition"].(map[string]any)
	}
	if e, _ := cond(livingAP)["elsewhere"].([]any); len(e) != 1 || e[0] != phone {
		t.Fatalf("the living room's condition: %v", cond(livingAP))
	}
	arr, _ := cond(officeAP)["arrived"].([]any)
	if len(arr) != 1 || arr[0].(map[string]any)["mac"] != phone || arr[0].(map[string]any)["rx_rate"] != 1200.9 {
		t.Fatalf("the office's arrivals: %v", cond(officeAP))
	}
	_, list := f.do("GET", "/v1/clients?q=92:8e", "griff", nil)
	if cs, _ := list["clients"].([]any); len(cs) != 1 || cs[0].(map[string]any)["ap"] != officeAP {
		t.Fatalf("the clients list: %v", list)
	}
	// The living room reports it again, idle: still the office's, there too.
	report(living, map[string]any{"mac": phone, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "connected": 260, "inactive_ms": 65000})
	_, list = f.do("GET", "/v1/clients?q=92:8e", "griff", nil)
	if cs, _ := list["clients"].([]any); len(cs) != 1 || cs[0].(map[string]any)["ap"] != officeAP {
		t.Fatalf("the clients list after the living room's report: %v", list)
	}
	// The office's report lists it: no longer arrived.
	report(office, client)
	if _, ok := cond(officeAP)["arrived"]; ok {
		t.Fatalf("still arrived once listed: %v", cond(officeAP))
	}
}

// A client's join and leave are exact (0252): a join on 57ff has it there at
// once, and ce7f's minute begun before the join does not put it back; ce7f
// letting go of it after says nothing. Its leave from 57ff takes it off,
// and 57ff's minute begun before, or a report of it from before, does not
// put it back; its join again does.
func TestActivityJoins(t *testing.T) {
	var a activity
	t0 := time.Date(2026, 10, 31, 13, 20, 0, 0, time.UTC)
	phone := "6c:ac:c2:cf:b6:3f"
	st := func(at time.Time, clients ...map[string]any) *conditions.State {
		r, _ := json.Marshal(map[string]any{"clients": clients})
		return &conditions.State{At: at, Report: r}
	}
	minute := func(ap string, at time.Time, secs int, rx int64) conditions.ClientMinute {
		return conditions.ClientMinute{MAC: phone, At: at, AP: hierarchy.NodeID(ap), Band: "6g", SSID: "Home Wi-Fi", Secs: secs, Counts: conditions.Counts{RxPackets: rx}}
	}
	signal := -61
	join := func(ap string, at time.Time) clientMove {
		return clientMove{ClientMinute: conditions.ClientMinute{MAC: phone, At: at, AP: hierarchy.NodeID(ap), Band: "6g", SSID: "Home Wi-Fi", Signal: &signal}}
	}
	leave := func(ap string, at time.Time) clientMove {
		return clientMove{ClientMinute: conditions.ClientMinute{MAC: phone, At: at, AP: hierarchy.NodeID(ap), Band: "6g", SSID: "Home Wi-Fi"}, Left: true}
	}
	a.note("ce7f", t0, []wifiClient{{MAC: phone, InactiveMS: 2000}})
	ce7f := st(t0, map[string]any{"mac": phone, "inactive_ms": 2000})
	// It joins 57ff 30 s into the minute.
	a.moved("57ff", []clientMove{join("57ff", t0.Add(30*time.Second))})
	if e := a.elsewhere("ce7f", ce7f); len(e) != 1 || e[0] != phone {
		t.Fatalf("elsewhere, ce7f, on the join: %v", e)
	}
	got := a.arrived("57ff", st(t0.Add(-time.Minute)))
	if len(got) != 1 || got[0].MAC != phone || got[0].Signal == nil || *got[0].Signal != -61 || !got[0].At.Equal(t0.Add(30*time.Second)) {
		t.Fatalf("arrived on 57ff: %v", got)
	}
	// ce7f's minute: it sent ce7f something before it went, and hostapd
	// kept it the whole minute. Begun before the join: still 57ff's.
	a.minutes("ce7f", []conditions.ClientMinute{minute("ce7f", t0, 60, 9)})
	if o := a.last[phone]; o.ap != "57ff" {
		t.Fatalf("ce7f's minute took it back: %v", o)
	}
	// ce7f lets go of it, told by 57ff (0226): it is 57ff's still.
	a.moved("ce7f", []clientMove{leave("ce7f", t0.Add(31*time.Second))})
	if o := a.last[phone]; o.ap != "57ff" || o.left {
		t.Fatalf("ce7f's letting go: %v", o)
	}
	// 57ff's next minute keeps it there.
	a.minutes("57ff", []conditions.ClientMinute{minute("57ff", t0.Add(time.Minute), 60, 40)})
	if o := a.last[phone]; o.ap != "57ff" || o.exact || !o.at.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("57ff's minute: %v", o)
	}
	// A join news late, from before that minute, says nothing.
	a.moved("ce7f", []clientMove{join("ce7f", t0.Add(50*time.Second))})
	if o := a.last[phone]; o.ap != "57ff" {
		t.Fatalf("an old join moved it: %v", o)
	}
	// It leaves 57ff 30 s into the third minute.
	a.moved("57ff", []clientMove{leave("57ff", t0.Add(2*time.Minute+30*time.Second))})
	if got := a.arrived("57ff", st(t0.Add(-time.Minute))); got != nil {
		t.Fatalf("arrived once left: %v", got)
	}
	ff := st(t0.Add(2*time.Minute), map[string]any{"mac": phone, "inactive_ms": 500})
	if e := a.elsewhere("57ff", ff); len(e) != 1 || e[0] != phone {
		t.Fatalf("elsewhere, 57ff, once left: %v", e)
	}
	// Its minute there, begun before it left: it stays gone.
	a.minutes("57ff", []conditions.ClientMinute{minute("57ff", t0.Add(2*time.Minute), 35, 4)})
	if o := a.last[phone]; !o.left {
		t.Fatalf("57ff's minute put it back: %v", o)
	}
	// 57ff's report, from before it left: gone from 57ff.
	if gone := a.note("57ff", t0.Add(3*time.Minute), []wifiClient{{MAC: phone, InactiveMS: 40000}}); !gone[phone] {
		t.Fatalf("57ff's report after the leave: %v", gone)
	}
	// It joins 57ff again: back.
	a.moved("57ff", []clientMove{join("57ff", t0.Add(4*time.Minute))})
	if o := a.last[phone]; o.ap != "57ff" || o.left {
		t.Fatalf("joined again: %v", o)
	}
	if e := a.elsewhere("57ff", st(t0.Add(5*time.Minute), map[string]any{"mac": phone, "inactive_ms": 500})); e != nil {
		t.Fatalf("elsewhere once back: %v", e)
	}
}

// Through the API (0252): the office says the phone joined it, and at once
// the living room's condition names it elsewhere, the office's has it
// arrived, and the clients list has it on the office. The office says it
// left: its condition has it no longer.
func TestClientWhereJoins(t *testing.T) {
	f := newFixture(t)
	f.change("griff", map[string]any{"kind": "add-builtins"})
	radios := []any{map[string]any{"band": "5g"}}
	living := f.enrollAt("living", "30:86:2d:bc:a5:ff", "sandbox", radios, map[string]int{"5g": 104})
	office := f.enrollAt("office", "20:05:b6:df:64:e0", "sandbox", radios, map[string]int{"5g": 157})
	phone := "92:8e:1b:c6:4c:2c"
	report := func(apTok string, clients ...map[string]any) string {
		t.Helper()
		ap, token, _ := strings.Cut(apTok, "|")
		_, _, cfg := f.apDo("GET", "/v1/ap/config", token, nil, nil)
		if code, _, b := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": cfg["version"], "uptime": 60, "clients": clients}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, b)
		}
		return ap
	}
	livingAP := report(living, map[string]any{"mac": phone, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "connected": 200, "inactive_ms": 120000, "host": "Phone-B"})
	officeAP := report(office)
	_, token, _ := strings.Cut(office, "|")
	// A second on, so as to be clear of the reports' times whatever the clock.
	at := time.Now().Add(time.Second).UnixMilli()
	news := func(kind string, at int64) {
		t.Helper()
		n := map[string]any{"t": "news", "kind": kind, "mac": strings.ToUpper(phone), "at": at, "ssid": "Home Wi-Fi", "band": "5g", "network": "sweet"}
		if kind == "joined" {
			n["signal"] = -57
		}
		if code, _, b := f.apDo("POST", "/v1/ap/live", token, map[string]any{"news": []any{n, map[string]any{"t": "news", "kind": "joined", "mac": "nonsense", "at": at, "band": "5g"}},
			"sent": float64(time.Now().UnixMilli())}, nil); code != 200 {
			t.Fatalf("live: %d %v", code, b)
		}
	}
	news("joined", at)
	cond := func(ap string) map[string]any {
		t.Helper()
		_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
		return cfg["condition"].(map[string]any)
	}
	if e, _ := cond(livingAP)["elsewhere"].([]any); len(e) != 1 || e[0] != phone {
		t.Fatalf("the living room's condition: %v", cond(livingAP))
	}
	arr, _ := cond(officeAP)["arrived"].([]any)
	if len(arr) != 1 || arr[0].(map[string]any)["mac"] != phone || arr[0].(map[string]any)["signal"] != float64(-57) {
		t.Fatalf("the office's arrivals: %v", cond(officeAP))
	}
	_, list := f.do("GET", "/v1/clients?q=92:8e", "griff", nil)
	if cs, _ := list["clients"].([]any); len(cs) != 1 || cs[0].(map[string]any)["ap"] != officeAP {
		t.Fatalf("the clients list: %v", list)
	}
	// It leaves the office.
	news("left", at+5000)
	if _, ok := cond(officeAP)["arrived"]; ok {
		t.Fatalf("still arrived once left: %v", cond(officeAP))
	}
}
