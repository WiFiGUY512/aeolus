package alerts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
)

var now = time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)

// kinds is the alerts' severities and kinds. One with no few words to say
// it in (0330), or few words that are not few, says so in its place, so no
// test of it passes.
func kinds(as []Alert) string {
	var out []string
	for _, a := range as {
		k := a.Severity + ":" + a.Kind
		if a.Short == "" || len(a.Short) > shortest {
			k += ":SHORT(" + a.Short + ")"
		}
		out = append(out, k)
	}
	return strings.Join(out, " ")
}

// shortest is the most a short may be: one line of a narrow list.
const shortest = 48

func seen(ago time.Duration, running int64) *conditions.Seen {
	at := now.Add(-ago)
	return &conditions.Seen{At: at, Source: "192.168.20.7", Running: &running, RunningAt: &at}
}

func TestAHealthyAPHasNone(t *testing.T) {
	as := For(Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7,
		Latest: conditions.Latest{Seen: seen(30*time.Second, 7)}})
	if len(as) != 0 {
		t.Fatalf("alerts = %s", kinds(as))
	}
}

func TestOfflineAfterThreePollsAndNeverUnderFiveMinutes(t *testing.T) {
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(4*time.Minute, 7)
	if as := For(in); len(as) != 0 {
		t.Fatalf("4 minutes: %s", kinds(as))
	}
	in.Latest.Seen = seen(6*time.Minute, 7)
	as := For(in)
	if kinds(as) != "critical:offline" || !strings.Contains(as[0].Message, "6 minutes") {
		t.Fatalf("6 minutes: %v", as)
	}
	// A slow poll waits longer.
	in.Poll = 5 * time.Minute
	if as := For(in); len(as) != 0 {
		t.Fatalf("6 minutes at a 5 minute poll: %s", kinds(as))
	}
}

func TestHeldRefusedAndFailed(t *testing.T) {
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 108,
		Problems: []string{"uplink.bond: taking the bond apart needs spanning tree on", "another"}}
	in.Latest.Seen = seen(10*time.Second, 107)
	in.Latest.Check = &conditions.Check{At: now, Version: 108, Result: "refused", Problems: []string{"network bridge br-lan: stp is \"\""}}
	in.Latest.Apply = &conditions.Apply{At: now, Version: 108, OK: false, Error: "lost the manager within 90 seconds of applying; reverted"}
	as := For(in)
	if got := kinds(as); got != "critical:refused critical:apply-failed warning:held" {
		t.Fatalf("alerts = %s", got)
	}
	if !strings.Contains(as[2].Message, "(and 1 more)") {
		t.Fatalf("held = %q", as[2].Message)
	}
	// An earlier version's refusal is history, not an alert.
	in.Problems = nil
	in.Latest.Check.Version, in.Latest.Apply.Version = 107, 107
	in.Latest.Seen = seen(10*time.Second, 108)
	if as := For(in); len(as) != 0 {
		t.Fatalf("old records: %s", kinds(as))
	}
}

func TestBehindOnlyWhenNothingExplainsIt(t *testing.T) {
	// It reports every few minutes, which refreshes when it said what it
	// runs; behind is measured from when the current version was made.
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 9, Since: now.Add(-20 * time.Minute)}
	in.Latest.Seen = seen(20*time.Second, 8)
	as := For(in)
	if got := kinds(as); got != "warning:behind" || !as[0].Since.Equal(in.Since) {
		t.Fatalf("alerts = %s", got)
	}
	// A version made a minute ago is still on its way.
	in.Since = now.Add(-time.Minute)
	if got := kinds(For(in)); got != "" {
		t.Fatalf("new version: %s", got)
	}
}

func TestFromTheReport(t *testing.T) {
	report := map[string]any{
		"wireless_missing": true,
		"agent":            map[string]any{"hash": "aaaa", "update": map[string]any{"version": "v0.57.0", "hash": "bbbb", "state": "rolled-back", "why": "it did not confirm itself within 300 s", "ago": 3600}},
		"transports": map[string]any{
			"aeolus-50": map[string]any{"active": "none"},
			"lab":       map[string]any{"active": "fallback"},
		},
		"vxlan": map[string]any{
			"tunnels": []any{
				map[string]any{"vni": 50, "peer": "1.1.1.2", "probe": map[string]any{"verdict": "down"}},
				map[string]any{"vni": 60, "peer": "1.1.1.2", "standby": true, "probe": map[string]any{"verdict": "down"}},
				// Up to netifd, its device gone (0187).
				map[string]any{"vni": 70, "peer": "1.1.1.2", "gone": true, "probe": map[string]any{"verdict": "off"}},
			},
			"loops": []any{map[string]any{"port": "lan3", "vni": 30, "ago": 600}},
		},
		"uplink_vlans": []any{map[string]any{"vlan": 30, "verdict": "silent"}, map[string]any{"vlan": 20, "verdict": "present"}},
		"dhcp":         map[string]any{"guest": map[string]any{"answered": 0, "unanswered": 4}},
		"radius": []any{
			map[string]any{"network": "staff", "server": "192.0.2.10", "verdict": "silent"},
			map[string]any{"network": "lab", "server": "192.0.2.11", "verdict": "up"},
			map[string]any{"network": "guest", "server": "192.0.2.12", "verdict": "unverified"},
		},
		"time": map[string]any{"synced": false},
	}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7, WantsAgent: "bbbb"}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-2 * time.Minute), Version: 7, Report: raw}
	want := "critical:wireless-missing critical:no-transport critical:tunnel-gone critical:loop critical:radius-silent warning:agent-update warning:on-fallback warning:tunnel-down warning:vlan-silent warning:dhcp-silent info:clock"
	// One request alone unanswered is no silence: a client's renewal let
	// pass, answered at its next try.
	single, _ := json.Marshal(map[string]any{"dhcp": map[string]any{"guest": map[string]any{"answered": 0, "unanswered": 1}}})
	one := in
	one.Latest.State = &conditions.State{At: in.Latest.State.At, Version: 7, Report: single}
	if strings.Contains(kinds(For(one)), "dhcp-silent") {
		t.Errorf("one request alone is silence")
	}
	as := For(in)
	if got := kinds(as); got != want {
		t.Fatalf("alerts =\n%s\nwant\n%s", got, want)
	}
	// Since, from the report's own ago where it has one: the rollback an
	// hour before the report, the loop ten minutes before it.
	for _, a := range as {
		want := in.Latest.State.At
		switch a.Kind {
		case "agent-update":
			want = want.Add(-time.Hour)
		case "loop":
			want = want.Add(-10 * time.Minute)
		}
		if !a.Since.Equal(want) {
			t.Errorf("%s since %v, want %v", a.Kind, a.Since, want)
		}
	}
	// A failed update to a bundle the AP no longer should run is history.
	in.WantsAgent = "aaaa"
	if got := kinds(For(in)); strings.Contains(got, "agent-update") {
		t.Fatalf("an update no longer wanted: %s", got)
	}
	in.WantsAgent = "bbbb"
	// A report too old to speak for the AP now says nothing of what is in
	// it, only that the reports stopped while it calls (0315).
	in.Latest.State.At = now.Add(-time.Hour)
	if got := kinds(For(in)); got != "warning:reports-stopped" {
		t.Fatalf("old report: %s", got)
	}
}

func TestLandingZone(t *testing.T) {
	as := For(Input{AP: "ap-2", Name: "new", Now: now, Unassigned: true, Latest: conditions.Latest{Seen: seen(time.Hour, 0)}})
	if kinds(as) != "info:unassigned" {
		t.Fatalf("alerts = %s", kinds(as))
	}
}

// A network not one of the APs' that broadcasts one of Aeolus's SSIDs is a
// rogue (0105); one of the APs' own BSSIDs, or another SSID, is not.
func TestRogue(t *testing.T) {
	report := map[string]any{"rrm": map[string]any{"others": []any{
		map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "Aeolus Lab", "band": "2g", "channel": 11, "signal": -61, "ago": 120},
		map[string]any{"bssid": "02:11:22:33:44:55", "ssid": "Aeolus Lab", "band": "5g", "channel": 36, "signal": -50, "ago": 30},
		map[string]any{"bssid": "e4:d1:24:08:6d:12", "ssid": "Home Wi-Fi", "band": "2g", "channel": 6, "signal": -70, "ago": 30},
	}}}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7,
		Fleet: &Fleet{BSSIDs: map[string]bool{"02:11:22:33:44:55": true}, SSIDs: map[string]bool{"Aeolus Lab": true}}}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	as := For(in)
	if kinds(as) != "warning:rogue" || as[0].Key != "rogue:e4:d1:24:08:6d:11" || !strings.Contains(as[0].Message, `"Aeolus Lab" on 2.4 GHz channel 11`) {
		t.Fatalf("alerts = %+v", as)
	}
	if !as[0].Since.Equal(in.Latest.State.At.Add(-2 * time.Minute)) {
		t.Fatalf("since = %v", as[0].Since)
	}
}

// An SSID is matched as an AP hears it: a stranger's Café, heard as
// Caf??, is a rogue of Aeolus's Café.
func TestHeard(t *testing.T) {
	for in, want := range map[string]string{"Café": "Caf??", "Aeolus Lab": "Aeolus Lab", "???": "???", "abcdefghijklmnopqrstuvwxyz0123456789": "abcdefghijklmnopqrstuvwxyz012345"} {
		if got := Heard(in); got != want {
			t.Errorf("Heard(%q) = %q, want %q", in, got, want)
		}
	}
}

// A network not one of the APs' that the scan radio found on the wire is a
// rogue for certain, whatever its name, hidden or not (0160); one made
// from an AP's address, a BSSID it renumbered, is the AP's own.
func TestRogueOnTheWire(t *testing.T) {
	report := map[string]any{"airscan": map[string]any{"bss": []any{
		map[string]any{"bssid": "a8:42:a1:00:00:07", "ssid": "Free WiFi", "band": "2g", "channel": 1, "signal": -55, "wired": true, "vlans": []int{10, 20}, "ago": 5},
		map[string]any{"bssid": "a8:42:a1:00:00:08", "ssid": "", "band": "5g", "channel": 36, "signal": -60, "wired": true, "vlans": []int{10}, "ago": 5},
		map[string]any{"bssid": "3e:11:22:33:44:c0", "ssid": "Aeolus Lab", "band": "5g", "channel": 36, "signal": -15, "wired": true, "vlans": []int{20}, "ago": 5},
		map[string]any{"bssid": "e4:d1:24:08:6d:11", "ssid": "Aeolus Lab", "band": "2g", "channel": 11, "signal": -70, "wired": false, "ago": 5},
		map[string]any{"bssid": "70:4f:57:13:4e:f4", "ssid": "Smart Plug", "band": "2g", "channel": 6, "signal": -84, "wired": false, "ago": 5},
	}}, "dhcp": map[string]any{"lab": map[string]any{"servers": []any{
		map[string]any{"id": "192.168.20.254", "mac": "28:e7:1d:d2:d0:13"}, map[string]any{"id": "192.168.20.99", "mac": "f2:0d:e6:3f:00:32"}}}}}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7,
		Fleet: &Fleet{BSSIDs: map[string]bool{"02:11:22:33:44:55": true}, SSIDs: map[string]bool{"Aeolus Lab": true}, Addrs: map[string]bool{"11:22:33:44": true}}}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	as := For(in)
	if kinds(as) != "critical:rogue-wired critical:rogue-wired warning:dhcp-servers warning:rogue" {
		t.Fatalf("alerts = %s", kinds(as))
	}
	if !strings.Contains(as[0].Message, "is on your wire, on VLANs 10 and 20") || !strings.Contains(as[0].Message, `"Free WiFi"`) ||
		!strings.Contains(as[1].Message, "on VLAN 10") || !strings.Contains(as[1].Message, "a hidden name") ||
		!strings.Contains(as[2].Message, "192.168.20.99 (f2:0d:e6:3f:00:32)") || as[3].Key != "rogue:e4:d1:24:08:6d:11" {
		t.Fatalf("alerts = %+v", as)
	}
	// Each named by its maker, as the threats page names it.
	if !strings.HasPrefix(as[0].Message, "a8:42:a1:00:00:07 (TP-Link") || !strings.HasPrefix(as[3].Message, "e4:d1:24:08:6d:11 (Mojo Networks") {
		t.Fatalf("makers: %q / %q", as[0].Message, as[3].Message)
	}
	// Known, a rogue on the wire raises nothing.
	in.Known = map[string]bool{"a8:42:a1:00:00:07": true, "a8:42:a1:00:00:08": true}
	if kinds(For(in)) != "warning:dhcp-servers warning:rogue" {
		t.Fatalf("known: %s", kinds(For(in)))
	}
	// The network says the servers it has (0160): both, by address or MAC,
	// raise nothing; where it has one, the other is the alert, alone.
	in.DHCPServers = map[string][]string{"lab": {"192.168.20.254", "f2:0d:e6:3f:00:32"}}
	if kinds(For(in)) != "warning:rogue" {
		t.Fatalf("both the network's own: %s", kinds(For(in)))
	}
	in.DHCPServers = map[string][]string{"lab": {"28:e7:1d:d2:d0:13"}}
	as = For(in)
	if kinds(as) != "warning:dhcp-servers warning:rogue" || !strings.Contains(as[0].Message, "not one of network lab's answers there: 192.168.20.99 (f2:0d:e6:3f:00:32)") ||
		strings.Contains(as[0].Message, "192.168.20.254") {
		t.Fatalf("one the network's own: %+v", as[0])
	}
	if !DHCPKnown([]string{"28:E7:1D:D2:D0:13"}, "192.168.20.254", "28:e7:1d:d2:d0:13") {
		t.Fatal("a MAC in capitals is not known")
	}
}

// A DHCP server heard with no address of its own, 0.0.0.0 or
// 255.255.255.255, is the server of its MAC: the tower switch, heard as
// 192.168.1.254 and 255.255.255.255, is one server and no alert
// (2026-10-25). Two of one MAC that each gave an address, as behind a
// relay, stay two.
func TestDHCPNoAddress(t *testing.T) {
	alert := func(servers ...[2]string) string {
		var list []any
		for _, s := range servers {
			list = append(list, map[string]any{"id": s[0], "mac": s[1]})
		}
		raw, _ := json.Marshal(map[string]any{"dhcp": map[string]any{"lab": map[string]any{"servers": list}}})
		in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		for _, a := range For(in) {
			if a.Kind == "dhcp-servers" {
				return a.Message
			}
		}
		return ""
	}
	const tower, old = "fc:bd:67:a6:67:97", "aa:bb:cc:dd:ee:ff"
	if got := alert([2]string{"192.168.1.254", tower}, [2]string{"255.255.255.255", tower}); got != "" {
		t.Errorf("one server, with no address once: %s", got)
	}
	if got := alert([2]string{"0.0.0.0", tower}, [2]string{"255.255.255.255", tower}); got != "" {
		t.Errorf("one server, never with an address: %s", got)
	}
	if got := alert([2]string{"0.0.0.0", tower}, [2]string{"192.168.1.254", tower}, [2]string{"192.168.10.254", old}); !strings.HasPrefix(got, "2 DHCP servers answer on network lab: 192.168.1.254 (fc:bd:67:a6:67:97), 192.168.10.254 (aa:bb:cc:dd:ee:ff).") {
		t.Errorf("two servers, one with no address once: %s", got)
	}
	if got := alert([2]string{"192.168.1.254", tower}, [2]string{"192.168.10.254", tower}); !strings.HasPrefix(got, "2 DHCP servers") {
		t.Errorf("two behind one relay: %s", got)
	}
}

// TestOffAir (0201): a network the AP took off the air while it cannot reach
// its way on is a warning, since when it went, saying why; one that is on
// the air again, or never went, is none.
func TestOffAir(t *testing.T) {
	report := map[string]any{
		"off_air": map[string]any{
			"home-wi-fi-iot": map[string]any{"off": true, "ago": 120, "why": "its VLAN 10 does not answer", "stopped": []any{"phy0-ap1", "phy1-ap1"}},
			"home-wi-fi":     map[string]any{"off": false, "ago": 30},
			"guest":          map[string]any{"off": false},
		},
	}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	as := For(in)
	if got := kinds(as); got != "warning:off-air" {
		t.Fatalf("alerts = %s, want warning:off-air", got)
	}
	a := as[0]
	if a.Key != "off-air:home-wi-fi-iot" || !strings.Contains(a.Message, "network home-wi-fi-iot is off the air on this AP: its VLAN 10 does not answer") ||
		!a.Since.Equal(in.Latest.State.At.Add(-2*time.Minute)) {
		t.Errorf("the alert: %+v", a)
	}
}

// A radio with none of its networks on the air, past a restart, is
// critical; one with some, a warning naming them; neither before it has
// been so for RadioShortAfter, nor where all are on (0216).
func TestRadioDown(t *testing.T) {
	report := map[string]any{
		"radios": []any{
			map[string]any{"radio": "radio1", "band": "5g", "bss": 0, "bss_want": 4, "short_for": 600, "missing": []string{"aeolus_50", "Home Wi-Fi"}},
			map[string]any{"radio": "radio2", "band": "2g", "bss": 2, "bss_want": 4, "short_for": 300, "missing": []string{"Home Wi-Fi", "Home_Wi-Fi_IoT"}},
			map[string]any{"radio": "radio3", "band": "6g", "bss": 0, "bss_want": 1, "short_for": 60},
			map[string]any{"radio": "radio0", "band": "5g", "up": false},
			map[string]any{"radio": "radio4", "band": "6g", "bss": 1, "bss_want": 1},
		},
	}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "Living-Room-C-360", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	as := For(in)
	if got := kinds(as); got != "critical:radio-down warning:radio-short" {
		t.Fatalf("alerts = %s", got)
	}
	if a := as[0]; a.Key != "radio-down:5g" || a.Message != "its 5 GHz radio (radio1) has none of its 4 networks on the air: their clients have no 5 GHz service here" ||
		!a.Since.Equal(in.Latest.State.At.Add(-10*time.Minute)) {
		t.Errorf("down: %+v", a)
	}
	if a := as[1]; a.Key != "radio-short:2g" || a.Message != "2 of its 4 networks on 2.4 GHz are not on the air (Home Wi-Fi, Home_Wi-Fi_IoT): their clients have no 2.4 GHz service here" {
		t.Errorf("short: %+v", a)
	}
}

// A radio whose beacons do not go out warns, at one in fifty over the last
// half hour and at least BeaconsFewest of them (0221, 0339): not at a
// healthy 0.2%, nor at a burst of a few minutes the half hour hides, nor
// at a short time's few. Before the reports cover a quarter hour, only a
// radio that misses one in ten or more.
func TestBeaconsFailing(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"radios": []any{}})
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	in.Beacons = []conditions.RadioBeacons{
		// Half an hour at 2%: its clients may well lose it.
		{Radio: "radio1", Band: "2g", Sent: 70560, Failed: 1440, Over: 1800},
		// A burst of 5% over five minutes, in a healthy half hour: 1%.
		{Radio: "radio0", Band: "5g", Sent: 71280, Failed: 720, Over: 1800},
		// A healthy 0.2%.
		{Radio: "radio2", Band: "6g", Sent: 71856, Failed: 144, Over: 1800},
	}
	as := For(in)
	if got := kinds(as); got != "warning:beacons" {
		t.Fatalf("alerts = %s", got)
	}
	if a := as[0]; a.Key != "beacons:2g" || a.Message != "2.0% of its beacons on 2.4 GHz did not go out over the last 30 minutes (1440 of 72000): its clients may lose it. The channel is too busy to send on, or the radio is stuck" {
		t.Errorf("beacons: %+v", a)
	}

	// An AP just started: five minutes of a 4% burst say nothing yet, nor
	// a minute's few; a radio stuck from its start does.
	in.Beacons = []conditions.RadioBeacons{
		{Radio: "radio1", Band: "2g", Sent: 11520, Failed: 480, Over: 300},
		{Radio: "radio0", Band: "5g", Sent: 80, Failed: 20, Over: 10},
		{Radio: "radio2", Band: "6g", Sent: 2400, Failed: 9600, Over: 300},
	}
	as = For(in)
	if got := kinds(as); got != "warning:beacons" {
		t.Fatalf("alerts as it starts = %s", got)
	}
	if a := as[0]; a.Key != "beacons:6g" || a.Message != "80.0% of its beacons on 6 GHz did not go out over the last 5 minutes (9600 of 12000): its clients may lose it. The channel is too busy to send on, or the radio is stuck" {
		t.Errorf("stuck: %+v", a)
	}
}

// An address the AP holds on a network its own Wi-Fi serves warns (0209):
// its management, as C360-AP's on VLAN 20 was, or another, as Office's
// leftover wifi_iot on VLAN 10.
func TestSharedWithWiFi(t *testing.T) {
	report := map[string]any{
		"on_air": []any{
			map[string]any{"interface": "vlan20", "management": true, "vlan": 20, "ssids": []any{"Aeolus-Enterprise", "Home Wi-Fi"}},
			map[string]any{"interface": "wifi_iot", "management": false, "vlan": 10, "ssids": []any{"Home_Wi-Fi_IoT"}},
			map[string]any{"interface": "lan", "management": false, "vlan": nil, "ssids": []any{}},
		},
	}
	raw, _ := json.Marshal(report)
	in := Input{AP: "ap-1", Name: "C360-AP", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	as := For(in)
	if got := kinds(as); got != "warning:shared-with-wifi warning:shared-with-wifi" {
		t.Fatalf("alerts = %s", got)
	}
	if a := as[0]; a.Key != "shared-with-wifi:vlan20" ||
		!strings.Contains(a.Message, `managed on VLAN 20 (vlan20), which "Aeolus-Enterprise" and "Home Wi-Fi" are also on: what the AP itself broadcasts goes out over the air`) {
		t.Errorf("management: %+v", a)
	}
	if a := as[1]; a.Key != "shared-with-wifi:wifi_iot" ||
		!strings.Contains(a.Message, `holds an address on VLAN 10 (wifi_iot), which "Home_Wi-Fi_IoT" is also on`) ||
		!strings.Contains(a.Message, "Take the address off") {
		t.Errorf("another address: %+v", a)
	}
	if got := ssidsOn([]string{"a", "b", "c", "d", ""}); got != `"a", "b" and 3 more are` {
		t.Errorf("five: %s", got)
	}
}

// Power control that asks (0206) warns of a lonely client below the
// threshold, and of clients that took a step down badly in the last hour;
// where it guards or ignores, it has its answer and warns of neither.
func TestPowerControlAsks(t *testing.T) {
	at := now.Add(-time.Minute)
	radio := func(band, clients string, distressAt int64, acted bool) map[string]any {
		return map[string]any{
			"band": band, "power": 8, "clients": clients, "client_floor": -80,
			"lonely": []any{
				map[string]any{"mac": "aa:bb:cc:00:00:01", "est": -86},
				map[string]any{"mac": "aa:bb:cc:00:00:02", "est": -82},
				map[string]any{"mac": "aa:bb:cc:00:00:03", "est": -70},
			},
			"distress": map[string]any{"at": distressAt, "why": "retries", "scanning": 0, "left": 0, "ratio": []any{0.05, 0.31}, "acted": acted},
		}
	}
	in := Input{AP: "ap-1", Name: "OfficeOpenWrt", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	put := func(radios ...any) {
		raw, _ := json.Marshal(map[string]any{"rrm": map[string]any{"apc": radios}})
		in.Latest.State = &conditions.State{At: at, Version: 7, Report: raw}
	}
	put(radio("5g", "ask", at.Add(-20*time.Minute).Unix(), false))
	as := For(in)
	if got := kinds(as); got != "warning:apc-client warning:apc-distress" {
		t.Fatalf("alerts = %s", got)
	}
	if a := as[0]; a.Key != "apc-client:5g" || !strings.Contains(a.Message, "aa:bb:cc:00:00:01 and aa:bb:cc:00:00:02, which no other AP hears better, hears it at about -86 dBm, below -80") ||
		!strings.Contains(a.Message, "guard") || !strings.Contains(a.Message, "ignore") {
		t.Errorf("the lonely client: %+v", a)
	}
	if a := as[1]; a.Key != "apc-distress:5g" || !strings.Contains(a.Message, "31% of what was sent to them was tried again or lost, from 5%") ||
		!a.Since.Equal(at.Add(-20*time.Minute)) {
		t.Errorf("the distress: %+v", a)
	}
	// An older distress, or one it acted on, is no news.
	put(radio("5g", "", at.Add(-2*time.Hour).Unix(), false), radio("2g", "ask", at.Unix(), true))
	if got := kinds(For(in)); got != "warning:apc-client warning:apc-client" {
		t.Errorf("old or acted on: %s", got)
	}
	// The loneliest says itself what it hears (802.11k): no "about".
	measured := radio("5g", "ask", at.Add(-2*time.Hour).Unix(), false)
	measured["lonely"].([]any)[0].(map[string]any)["measured"] = true
	put(measured)
	if as := For(in); len(as) != 1 || !strings.Contains(as[0].Message, "hears it at -86 dBm, as it says itself, below -80") {
		t.Errorf("measured: %v", as)
	}
	for _, mode := range []string{"guard", "ignore"} {
		put(radio("5g", mode, at.Unix(), false))
		if as := For(in); len(as) != 0 {
			t.Errorf("%s: %s", mode, kinds(as))
		}
	}
}

// A radio whose last client tests ran under a third of the usual is a
// warning (0242), said which way it was slow, keyed by its band.
func TestSlowRadio(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	since := now.Add(-40 * time.Minute)
	in := Input{AP: "ap-1", Name: "Living-Room-C-360", Now: now, Poll: time.Minute, Version: 7,
		Latest: conditions.Latest{Seen: seen(30*time.Second, 7)},
		SlowRadios: []SlowRadio{{Band: "5g", Since: &since, Tests: []SlowTest{
			{Tester: "OpenWrtce7f", Down: f(7), Up: f(150), UsualDown: f(84), UsualUp: f(181.1), SlowDown: true},
			{Tester: "OpenWrtce7f", Down: f(7), Up: f(140), UsualDown: f(84), UsualUp: f(181.1), SlowDown: true},
			{Tester: "C360-AP", Down: f(6.94), Up: nil, UsualDown: f(260), SlowDown: true},
		}}}}
	as := For(in)
	if kinds(as) != "warning:slow-radio" {
		t.Fatalf("alerts = %s", kinds(as))
	}
	want := "its last 3 client tests on 5 GHz ran 7/150, 7/140 and 6.9/- Mbit/s down/up, each under a third of what its testers usually get here (84/181.1 from OpenWrtce7f, 260/- from C360-AP): it sends slowly"
	if a := as[0]; a.Message != want || a.Key != "slow-radio:5g" || a.Since == nil || !a.Since.Equal(since) {
		t.Errorf("the alert: %+v\nwant %q", a, want)
	}
	for i := range in.SlowRadios[0].Tests {
		in.SlowRadios[0].Tests[i].SlowDown, in.SlowRadios[0].Tests[i].SlowUp = i == 0, true
	}
	if m := For(in)[0].Message; !strings.HasSuffix(m, ": it receives slowly") {
		t.Errorf("slow up: %q", m)
	}
	in.SlowRadios[0].Tests[1].SlowUp = false
	in.SlowRadios[0].Tests[1].SlowDown = true
	if m := For(in)[0].Message; !strings.HasSuffix(m, ": its clients get little from it") {
		t.Errorf("slow each a way: %q", m)
	}
}

// What runs by local hours with no zone set runs by UTC's (0262): what the
// AP runs, where its own clock is on UTC; what the manager runs for it,
// whatever the AP's clock.
func TestClockZone(t *testing.T) {
	at := func(rule any, in Input) string {
		raw, _ := json.Marshal(map[string]any{"time": map[string]any{"synced": true, "rule": rule}})
		in.AP, in.Name, in.Now, in.Poll, in.Version = "ap-1", "C260", now, time.Minute, 7
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		for _, a := range For(in) {
			if a.Kind == "clock-zone" {
				return a.Severity + ": " + a.Message
			}
		}
		return ""
	}
	rrm := Input{OnClock: []string{"RRM's planned moves (02:00-05:00)"}}
	if got, want := at("GMT0", rrm), "warning: no time zone is set for it, so RRM's planned moves (02:00-05:00) happen at those hours in UTC. Set system.tz on its folder"; got != want {
		t.Errorf("on UTC:\n%s\nwant\n%s", got, want)
	}
	for _, rule := range []any{"CST6CDT,M3.2.0,M11.1.0", nil} {
		if got := at(rule, rrm); got != "" {
			t.Errorf("clock on %v: %s", rule, got)
		}
	}
	set := rrm
	set.Zone = "America/Chicago"
	if got := at("GMT0", set); got != "" {
		t.Errorf("a zone set, the agent not yet on it: %s", got)
	}
	both := Input{OnClock: rrm.OnClock, ByZone: []string{"the client tests on a schedule (01:00-05:00)"}}
	if got := at("UTC", both); !strings.Contains(got, "RRM's planned moves (02:00-05:00) and the client tests on a schedule (01:00-05:00) happen") {
		t.Errorf("both: %s", got)
	}
	// What the manager runs goes by system.tz alone.
	if got := at("CST6CDT,M3.2.0,M11.1.0", both); !strings.Contains(got, "so the client tests on a schedule (01:00-05:00) happen") {
		t.Errorf("by the manager: %s", got)
	}
	if got := at("GMT0", Input{}); got != "" {
		t.Errorf("nothing by local hours: %s", got)
	}
	for rule, utc := range map[string]bool{
		"": true, "UTC": true, "UTC0": true, "GMT0": true, "<+00>0": true, "UTC+00:00": true,
		"CST6CDT,M3.2.0,M11.1.0": false, "GMT0BST,M3.5.0/1,M10.5.0": false, "<+05>-5": false, "EST5": false, "IST-5:30": false,
	} {
		if utcRule.MatchString(rule) != utc {
			t.Errorf("%q is UTC: %v, want %v", rule, !utc, utc)
		}
	}
}

// A radio that can't scan past the half hour its ratings last is said
// (0263): on a DFS channel hostapd started it on, with how allow looks first said (0328);
// refusing scans elsewhere. One on a DFS channel it went to after a look,
// which knows where it steps off to, and one deaf for less, are not.
func TestCannotScan(t *testing.T) {
	at := func(deaf ...map[string]any) []Alert {
		raw, _ := json.Marshal(map[string]any{"rrm": map[string]any{"cannot_scan": deaf}})
		in := Input{AP: "ap-1", Name: "OfficeOpenWrt", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		var out []Alert
		for _, a := range For(in) {
			if a.Kind == "cannot-scan" {
				out = append(out, a)
			}
		}
		return out
	}
	as := at(map[string]any{"band": "5g", "channel": 108, "why": "dfs", "ago": 5400})
	if len(as) != 1 || as[0].Key != "cannot-scan:5g" || as[0].Severity != Warning ||
		as[0].Message != "its 5 GHz radio has been on DFS channel 108 for 90 minutes, where Linux keeps it listening for radar and refuses every scan, so RRM rates no channel for it and moves it nowhere. With radio.5g.dfs allow, a radio starts outside DFS and goes to one only after a look where RRM moves radios and its width and channels leave it a block outside DFS to start on: 160 MHz has none on 5 GHz (0120, 0328)" {
		t.Fatalf("on DFS: %+v", as)
	}
	if want := now.Add(-time.Minute - 5400*time.Second); !as[0].Since.Equal(want) {
		t.Errorf("since %v, want %v", as[0].Since, want)
	}
	if as := at(map[string]any{"band": "2g", "channel": 6, "why": "refused", "ago": 7200}); len(as) != 1 || !strings.HasPrefix(as[0].Message, "its 2.4 GHz radio has refused every scan for 2 hours, on channel 6") {
		t.Fatalf("refused: %+v", as)
	}
	if as := at(map[string]any{"band": "5g", "channel": 52, "why": "dfs", "ago": 5400, "home": 36},
		map[string]any{"band": "2g", "channel": 6, "why": "refused", "ago": 1200}); len(as) != 0 {
		t.Fatalf("gone after a look, or deaf for 20 minutes: %+v", as)
	}
}

// RRM with nowhere to go (0267): a band with its channels rated and none
// its best is said; one with a best, or too few rated, is not.
func TestRRMNowhere(t *testing.T) {
	at := func(ratings ...map[string]any) string {
		raw, _ := json.Marshal(map[string]any{"rrm": map[string]any{"ratings": ratings}})
		in := Input{AP: "ap-1", Name: "Shed-AP", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		var out []string
		for _, a := range For(in) {
			if a.Kind == "rrm-nowhere" {
				out = append(out, a.Key+": "+a.Message)
			}
		}
		return strings.Join(out, "\n")
	}
	r := func(band string, ch int, best bool) map[string]any {
		return map[string]any{"band": band, "channel": ch, "best": best}
	}
	five := []map[string]any{r("5g", 36, false), r("5g", 40, false), r("5g", 44, false), r("5g", 48, false), r("5g", 149, false)}
	if got, want := at(five...), "rrm-nowhere:5g: RRM rated 5 of its 5 GHz channels and finds none its radio may use, not even its own: it moves it nowhere. Its radio's channels and width may leave it no whole block, or RRM reads them wrong (0266)"; got != want {
		t.Errorf("none best:\n%s\nwant\n%s", got, want)
	}
	five[2] = r("5g", 44, true)
	if got := at(append(five, r("2g", 1, false), r("2g", 6, true), r("2g", 11, false))...); got != "" {
		t.Errorf("a best on each band: %s", got)
	}
	if got := at(r("6g", 5, false), r("6g", 21, false), r("6g", 37, false)); got != "" {
		t.Errorf("three rated: %s", got)
	}
	// Where the report says its radios, a band none of them has a network
	// on is not one RRM moves: its ratings are left from before.
	radios := func(rs ...map[string]any) string {
		raw, _ := json.Marshal(map[string]any{"radios": rs, "rrm": map[string]any{"ratings": five}})
		in := Input{AP: "ap-1", Name: "OpenWrt5b1f", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		var out []string
		for _, a := range For(in) {
			if a.Kind == "rrm-nowhere" {
				out = append(out, a.Key)
			}
		}
		return strings.Join(out, " ")
	}
	five[2] = r("5g", 44, false)
	if got := radios(map[string]any{"radio": "radio0", "band": "5g", "up": true}, map[string]any{"radio": "radio1", "band": "2g", "bss": 2, "bss_want": 2}); got != "" {
		t.Errorf("a 5 GHz radio with no network: %s", got)
	}
	if got := radios(map[string]any{"radio": "radio0", "band": "5g", "bss": 0, "bss_want": 2}); got != "rrm-nowhere:5g" {
		t.Errorf("a 5 GHz radio whose networks are wanted: %q", got)
	}
	if got := radios(map[string]any{"radio": "radio0", "band": "5g", "bss": 2, "bss_want": 2}); got != "rrm-nowhere:5g" {
		t.Errorf("a 5 GHz radio with its networks: %q", got)
	}
}

// A slow radio's run of one tester's tests alone says it may be that
// tester's way to it (0308), as the C260's to the Living Room was.
func TestSlowRadioOneTesterSaysSo(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	since := now.Add(-40 * time.Minute)
	in := Input{AP: "ap-1", Name: "Living-Room-C-360", Now: now, Poll: time.Minute, Version: 7,
		Latest: conditions.Latest{Seen: seen(30*time.Second, 7)},
		SlowRadios: []SlowRadio{{Band: "5g", Since: &since, Tests: []SlowTest{
			{Tester: "C260", Down: f(44.1), Up: f(3.3), UsualDown: f(91.5), UsualUp: f(52.8), SlowUp: true},
			{Tester: "C260", Down: f(19.6), Up: f(2), UsualDown: f(91.5), UsualUp: f(52.8), SlowDown: true, SlowUp: true},
			{Tester: "C260", Down: f(19.2), Up: f(10.5), UsualDown: f(91.5), UsualUp: f(52.8), SlowDown: true, SlowUp: true},
		}}}}
	want := "its last 3 client tests on 5 GHz ran 44.1/3.3, 19.6/2 and 19.2/10.5 Mbit/s down/up, each under a third of what its testers usually get here (91.5/52.8 from C260): it receives slowly. All are C260's tests, so it may be C260's way to it and not the radio: a test from another AP would tell"
	if m := For(in)[0].Message; m != want {
		t.Errorf("one tester:\n%q\nwant\n%q", m, want)
	}
}

// A struggling client that did better on another AP lately is said to be
// maybe stuck, and what would move it (0309).
func TestStrugglingBetterPlace(t *testing.T) {
	since := now.Add(-time.Hour)
	in := Input{AP: "ap-1", Name: "C260", Now: now, Poll: time.Minute, Version: 7, Latest: conditions.Latest{Seen: seen(30*time.Second, 7)},
		Struggles: []Struggling{{MAC: "28:07:08:7e:13:ea", Who: "Living Room Television (28:07:08:7e:13:ea)", Why: []string{"65% of the frames sent to it were sent again"}, Since: &since,
			Better: &BetterPlace{AP: "Living-Room-C-360", Band: "5g", Signal: -60, Resent: 0.04, Minutes: 388}}}}
	want := "Living Room Television (28:07:08:7e:13:ea) struggles: 65% of the frames sent to it were sent again. In the last two days it did better on Living-Room-C-360 (5 GHz): heard at -60 dBm, 4% sent again over 388 busy minutes. It may be stuck here: sticky steering on its network (0289) would move it"
	if m := For(in)[0].Message; m != want {
		t.Errorf("got\n%q\nwant\n%q", m, want)
	}
}

// A radio whose firmware crashed (0310): one that did not come back is
// critical, since the crash, until a radio of it serves again; one that
// came back is a warning for FWCrashFor; the latest crash of a device is
// the one said.
func TestFWCrash(t *testing.T) {
	crash := func(ago int, dev string, recovered bool, radios, bands []string) map[string]any {
		return map[string]any{"ago": ago, "driver": "ath11k_pci", "device": dev, "why": "MHI_CB_EE_RDDM", "recovered": recovered, "radios": radios, "bands": bands}
	}
	run := func(report map[string]any) []Alert {
		raw, _ := json.Marshal(report)
		in := Input{AP: "ap-1", Name: "Garage-C-360", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		return For(in)
	}
	as := run(map[string]any{"fw_crashes": []any{
		crash(9000, "c000000.wifi", true, []string{"radio0", "radio1"}, []string{"5g", "2g"}),
		crash(600, "c000000.wifi", true, []string{"radio0", "radio1"}, []string{"5g", "2g"}),
		crash(120, "0001:01:00.0", false, []string{"radio2"}, nil),
	}})
	if got := kinds(as); got != "critical:fw-crash warning:fw-crash" {
		t.Fatalf("alerts = %s", got)
	}
	if a := as[0]; a.Key != "fw-crash:0001:01:00.0" || a.Message != "the firmware of a radio (ath11k_pci 0001:01:00.0) crashed (MHI_CB_EE_RDDM) and it did not come back: it stays down until its driver is bound again, which a restart of the AP does" ||
		!a.Since.Equal(now.Add(-time.Minute).Add(-120*time.Second)) {
		t.Errorf("not back: %+v", a)
	}
	if a := as[1]; a.Key != "fw-crash:c000000.wifi" || a.Message != "the firmware of its 5 GHz and 2.4 GHz radios (radio0, radio1) crashed (MHI_CB_EE_RDDM) and came back: its clients were dropped as it started again" {
		t.Errorf("back, the latest: %+v", a)
	}
	// Long ago and back: nothing. Lost and not back, but serving again: its
	// driver was bound again by hand, back.
	lost := crash(300, "0001:01:00.0", false, []string{"radio2"}, []string{"6g"})
	lost["lost"] = true
	as = run(map[string]any{
		"radios":     []any{map[string]any{"radio": "radio2", "band": "6g", "bss": 1, "bss_want": 1}},
		"fw_crashes": []any{crash(int((7 * time.Hour).Seconds()), "c000000.wifi", true, nil, nil), lost},
	})
	if got := kinds(as); got != "warning:fw-crash" || !strings.HasPrefix(as[0].Message, "the firmware of its 6 GHz radio (radio2) crashed (MHI_CB_EE_RDDM) and came back") {
		t.Errorf("long ago, and serving again: %s %+v", kinds(as), as)
	}
	// Not lost and not back by the agent's word, though hostapd says its
	// networks are on the air: not back (0310, the W-318's stale ENABLED).
	as = run(map[string]any{
		"radios":     []any{map[string]any{"radio": "radio2", "band": "6g", "bss": 1, "bss_want": 1}},
		"fw_crashes": []any{crash(300, "0001:01:00.0", false, []string{"radio2"}, []string{"6g"})},
	})
	if got := kinds(as); got != "critical:fw-crash" {
		t.Errorf("not lost, hostapd says on the air: %s %+v", got, as)
	}
}

// A scan radio's firmware crash (0310) is a warning: it serves no client.
func TestFWCrashScanRadio(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"fw_crashes": []any{map[string]any{"ago": 300, "driver": "ath11k_pci", "device": "0001:01:00.0", "why": "MHI_CB_EE_RDDM",
		"recovered": false, "radios": []string{"radio2"}, "bands": []string{}, "scan": true}}})
	in := Input{AP: "ap-1", Name: "Garage-C-360", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	as := For(in)
	if kinds(as) != "warning:fw-crash" || as[0].Message != "the firmware of its scan radio (radio2) crashed (MHI_CB_EE_RDDM) and it did not come back: its surveys and client tests stop until a restart of the AP" {
		t.Errorf("scan radio: %s %+v", kinds(as), as)
	}
}

// A crash not seen to have come back yet, young and not said lost, is
// starting again: a warning, not critical (0310); said lost, critical at
// once; past FWCrashStarting, critical.
func TestFWCrashStarting(t *testing.T) {
	run := func(ago int, lost bool) []Alert {
		raw, _ := json.Marshal(map[string]any{"fw_crashes": []any{map[string]any{"ago": ago, "driver": "ath11k", "device": "c000000.wifi", "why": "MHI_CB_EE_RDDM",
			"recovered": false, "lost": lost, "radios": []string{"radio0", "radio1"}, "bands": []string{"5g", "2g"}}}})
		in := Input{AP: "ap-1", Name: "Garage-C-360", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		return For(in)
	}
	if as := run(30, false); kinds(as) != "warning:fw-crash" || !strings.HasSuffix(as[0].Message, "crashed (MHI_CB_EE_RDDM) and is starting again: its clients are dropped meanwhile") {
		t.Errorf("30 s, not lost: %s %+v", kinds(as), as)
	}
	if as := run(30, true); kinds(as) != "critical:fw-crash" {
		t.Errorf("30 s, lost: %s", kinds(as))
	}
	if as := run(300, false); kinds(as) != "critical:fw-crash" {
		t.Errorf("5 min, not back: %s", kinds(as))
	}
}

// An AP whose minutes stopped (0311): all its radios', where it sent none
// for ten minutes; else each serving radio's, where the others' come. Not
// of an AP up less than ten minutes, offline, serving nothing, or never
// sending minutes; nor of a radio its config just turned on.
func TestMinutesStopped(t *testing.T) {
	type radio struct {
		name, band string
		bss        int
	}
	run := func(up time.Duration, radios []radio, last map[string]time.Duration, mod func(*Input)) []Alert {
		var rs []any
		for _, r := range radios {
			rs = append(rs, map[string]any{"radio": r.name, "band": r.band, "bss": r.bss})
		}
		raw, _ := json.Marshal(map[string]any{"uptime": int64((up - time.Minute) / time.Second), "radios": rs})
		in := Input{AP: "ap-1", Name: "Living-Room-C-360", Now: now, Poll: time.Minute, Version: 7, Since: now.Add(-48 * time.Hour)}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		m := &Minutes{Radios: map[string]time.Time{}}
		for r, ago := range last {
			at := now.Add(-ago)
			m.Radios[r] = at
			if at.After(m.Last) {
				m.Last = at
			}
		}
		in.Minutes = m
		if mod != nil {
			mod(&in)
		}
		return For(in)
	}
	three := []radio{{"radio0", "5g", 4}, {"radio1", "2g", 4}, {"radio2", "6g", 0}, {"radio3", "6g", 1}}
	fresh := map[string]time.Duration{"radio0": time.Minute, "radio1": time.Minute, "radio3": time.Minute}
	if as := run(time.Hour, three, fresh, nil); len(as) != 0 {
		t.Errorf("minutes come: %s", kinds(as))
	}
	// All stopped 25 minutes ago: one alert, since the end of the last.
	stopped := map[string]time.Duration{"radio0": 25 * time.Minute, "radio1": 25 * time.Minute, "radio3": 26 * time.Minute}
	as := run(time.Hour, three, stopped, nil)
	if kinds(as) != "warning:minutes-stopped" || as[0].Key != "minutes-stopped" || !as[0].Since.Equal(now.Add(-24*time.Minute)) ||
		!strings.HasPrefix(as[0].Message, "has sent no minute of its radios for 24 minutes, though it calls and serves networks") {
		t.Errorf("all stopped: %s %+v", kinds(as), as)
	}
	// Up only 8 minutes, as after a flash: not yet.
	if as := run(8*time.Minute, three, stopped, nil); len(as) != 0 {
		t.Errorf("up 8 minutes: %s", kinds(as))
	}
	// Offline: that alert says it, not this one.
	if as := run(time.Hour, three, stopped, func(in *Input) { in.Latest.Seen = seen(20*time.Minute, 7) }); kinds(as) != "critical:offline" {
		t.Errorf("offline: %s", kinds(as))
	}
	// Serving nothing, or never sending minutes, or not looked up.
	if as := run(time.Hour, []radio{{"radio0", "5g", 0}}, stopped, nil); len(as) != 0 {
		t.Errorf("serving nothing: %s", kinds(as))
	}
	if as := run(time.Hour, three, nil, nil); len(as) != 0 {
		t.Errorf("never sent: %s", kinds(as))
	}
	if as := run(time.Hour, three, stopped, func(in *Input) { in.Minutes = nil }); len(as) != 0 {
		t.Errorf("not looked up: %s", kinds(as))
	}
	// One radio stopped, as after its phy came back under another name; a
	// scan radio's (radio2, no networks) are no matter.
	one := map[string]time.Duration{"radio0": time.Minute, "radio1": time.Minute, "radio3": 40 * time.Minute, "radio2": 3 * time.Hour}
	as = run(time.Hour, three, one, nil)
	if kinds(as) != "warning:minutes-stopped" || as[0].Key != "minutes-stopped:radio3" ||
		!strings.HasPrefix(as[0].Message, "its 6 GHz radio (radio3) has sent no minute for 39 minutes, though it serves a network and its other radios' minutes come") {
		t.Errorf("one radio: %s %+v", kinds(as), as)
	}
	// One never sent in the day: quiet since the AP started.
	none := map[string]time.Duration{"radio0": time.Minute, "radio1": time.Minute}
	as = run(time.Hour, three, none, nil)
	if kinds(as) != "warning:minutes-stopped" || as[0].Key != "minutes-stopped:radio3" || !as[0].Since.Equal(now.Add(-time.Hour)) {
		t.Errorf("one radio none: %s %+v", kinds(as), as)
	}
	// ... unless its config was just made, as one turning it on.
	if as := run(time.Hour, three, one, func(in *Input) { in.Since = now.Add(-3 * time.Minute) }); len(as) != 0 {
		t.Errorf("just turned on: %s", kinds(as))
	}
}

// The SoC's own radios (AHB) crashed on their remote processor (0310, the
// W-318, 2026-11-05): starting again at first; not back after two minutes,
// it says the processor starts again by itself, not that the driver must be
// bound again; back, as any.
func TestFWCrashRemoteproc(t *testing.T) {
	run := func(ago int, back bool) []Alert {
		raw, _ := json.Marshal(map[string]any{
			"fw_crashes": []any{map[string]any{"ago": ago, "driver": "remoteproc", "device": "c000000.wifi", "why": "wcnss: fatal error",
				"recovered": back, "lost": false, "radios": []string{"radio0", "radio1"}, "bands": []string{"5g", "2g"}}},
			"radios": []any{map[string]any{"radio": "radio0", "band": "5g", "bss": 2}, map[string]any{"radio": "radio1", "band": "2g", "bss": 2}}})
		in := Input{AP: "ap-1", Name: "OpenWrt5b1f", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(10*time.Second, 7)
		in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
		return For(in)
	}
	if as := run(30, false); kinds(as) != "warning:fw-crash" || !strings.Contains(as[0].Message, "crashed (wcnss: fatal error) and is starting again") {
		t.Errorf("30 s: %s %+v", kinds(as), as)
	}
	as := run(200, false)
	if kinds(as) != "critical:fw-crash" || !strings.Contains(as[0].Message, "has not started again: its clients have no service here meanwhile, whatever hostapd says. Its processor starts again by itself") ||
		strings.Contains(as[0].Message, "bound again") {
		t.Errorf("200 s, not back: %s %+v", kinds(as), as)
	}
	if as := run(400, true); kinds(as) != "warning:fw-crash" || !strings.HasSuffix(as[0].Message, "came back: its clients were dropped as it started again") {
		t.Errorf("back: %s %+v", kinds(as), as)
	}
}

// An AP that calls, and whose state reports stopped more than three of
// them ago, says so (0315): its agent hung, as three did after a
// sysupgrade on 2026-11-05 while their live daemons went on calling. Not
// one offline, nor one never reported, nor one reported lately.
func TestReportsStopped(t *testing.T) {
	run := func(seenAgo, stateAgo time.Duration) []Alert {
		in := Input{AP: "ap-1", Name: "C260", Now: now, Poll: time.Minute, Version: 7}
		in.Latest.Seen = seen(seenAgo, 7)
		if stateAgo > 0 {
			in.Latest.State = &conditions.State{At: now.Add(-stateAgo), Version: 7, Report: []byte(`{}`)}
		}
		return For(in)
	}
	as := run(10*time.Second, 26*time.Minute)
	if kinds(as) != "warning:reports-stopped" || !strings.HasPrefix(as[0].Message, "has sent no state report for 26 minutes, though it calls the manager") ||
		!as[0].Since.Equal(now.Add(-26*time.Minute)) {
		t.Errorf("stopped: %s %+v", kinds(as), as)
	}
	if as := run(10*time.Second, 10*time.Minute); len(as) != 0 {
		t.Errorf("reported lately: %s", kinds(as))
	}
	if as := run(20*time.Minute, 26*time.Minute); kinds(as) != "critical:offline" {
		t.Errorf("offline: %s", kinds(as))
	}
	if as := run(10*time.Second, 0); len(as) != 0 {
		t.Errorf("never reported: %s", kinds(as))
	}
}

// The wire as a choke point (0351): an uplink port that ran full, had
// errors or lost its link, by its minutes; a port that links slower than
// both ends offer, or at half duplex, by the report.
func TestUplinkChokes(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"radios": []any{}, "ports": []any{
		map[string]any{"name": "lan1", "up": true, "speed": "2500F", "max": 2500, "partner_max": 10000},
		map[string]any{"name": "lan2", "up": true, "speed": "1000F", "max": 2500, "partner_max": 2500},
		map[string]any{"name": "lan3", "up": true, "speed": "100H", "max": 1000, "partner_max": 100},
		map[string]any{"name": "lan4", "up": false, "speed": "", "max": 1000},
		map[string]any{"name": "lan5", "up": true, "speed": "1000F", "max": 1000},
	}})
	in := Input{AP: "ap-1", Name: "Garage-C-360", Now: now, Poll: time.Minute, Version: 7}
	in.Latest.Seen = seen(10*time.Second, 7)
	in.Latest.State = &conditions.State{At: now.Add(-time.Minute), Version: 7, Report: raw}
	f := func(v float64) *float64 { return &v }
	speed := 2500
	minute := func(ago time.Duration, rx, rxMax, tx, txMax float64, errors, changes int64) conditions.UplinkMinute {
		return conditions.UplinkMinute{AP: "ap-1", At: now.Add(-ago), Port: "lan1", Up: true, Speed: &speed, Secs: 60,
			Rx: f(rx), RxMax: f(rxMax), Tx: f(tx), TxMax: f(txMax), Errors: errors, Changes: changes}
	}
	in.Uplink = []conditions.UplinkMinute{
		// Long ago: its link went and came back twice, over the hour.
		minute(50*time.Minute, 10, 20, 5, 9, 0, 2),
		minute(40*time.Minute, 10, 20, 5, 9, 0, 2),
		// Its busiest five seconds at 90% or more, in three minutes; its
		// average at 70% in one more; and errors.
		minute(14*time.Minute, 900, 2310, 40, 80, 4, 0),
		minute(10*time.Minute, 1800, 2400, 40, 80, 3, 0),
		minute(6*time.Minute, 300, 1200, 600, 2260, 2, 0),
		minute(3*time.Minute, 10, 20, 5, 9, 1, 0),
	}
	as := For(in)
	got := map[string]string{}
	for _, a := range as {
		got[a.Key] = a.Message
	}
	want := map[string]string{
		"uplink-full:lan1":   "lan1, its 2.5 Gbit/s uplink, ran full in 3 minutes of the last 15, at 2.4 Gbit/s at its busiest, into the AP, to its clients: there the wire, not the Wi-Fi, held its clients back",
		"uplink-errors:lan1": "lan1, its uplink, had 10 errors in the last 15 minutes: a cable, a connector or the switch port is at fault, and what they spoil is sent again",
		"uplink-flaps:lan1":  "lan1, its uplink, lost its link 2 times in the last hour: each time, its clients reach nothing until it is back. A cable, a connector, the switch port, or its power",
		"link-slow:lan2":     "lan2 links at 1 Gbit/s, though it and the switch both offer 2.5 Gbit/s: a cable, a connector or the switch port holds it back",
		"link-half:lan3":     "lan3 runs at half duplex, at 100 Mbit/s: it and the switch disagree on how to talk, and their frames collide. Have both ends negotiate",
	}
	if len(got) != len(want) {
		t.Errorf("alerts = %s", kinds(as))
	}
	for k, m := range want {
		if got[k] != m {
			t.Errorf("%s:\n got %q\nwant %q", k, got[k], m)
		}
	}

	// Quiet: two full minutes, nine errors, a link lost once, none.
	in.Uplink = []conditions.UplinkMinute{
		minute(30*time.Minute, 10, 20, 5, 9, 0, 2),
		minute(9*time.Minute, 900, 2310, 40, 80, 4, 0),
		minute(5*time.Minute, 300, 1200, 600, 2260, 5, 0),
		minute(2*time.Minute, 1700, 2000, 40, 80, 0, 0),
	}
	in.Latest.State.Report, _ = json.Marshal(map[string]any{"radios": []any{}})
	if as := For(in); len(as) != 0 {
		t.Errorf("quiet uplink = %s", kinds(as))
	}
}
