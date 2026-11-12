package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// GET /v1/clients/{mac} (0103): a client's sessions from the state reports
// of the APs the caller may view.
func TestClientJourney(t *testing.T) {
	f := newFixture(t)
	_, token, version := f.adopted()
	client := map[string]any{"mac": "7e:2a:ea:04:b1:8f", "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -61,
		"tx_rate": 400, "rx_rate": 300, "connected": 60, "host": "griffs-phone", "address": "192.168.20.80", "dhcp": "ok"}
	for i := 0; i < 2; i++ {
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60 + i, "clients": []any{client}}, nil); code != 200 {
			t.Fatalf("state: %d %v", code, body)
		}
	}
	code, body := f.do("GET", "/v1/clients/7E:2A:EA:04:B1:8F", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	sessions, _ := body["sessions"].([]any)
	if len(sessions) != 1 || body["host"] != "griffs-phone" {
		t.Fatalf("journey = %v", body)
	}
	if s := sessions[0].(map[string]any); s["name"] != "ShedAP" || s["reports"] != 2.0 || s["signal_avg"] != -61.0 {
		t.Fatalf("session = %v", s)
	}
	// office may view no AP, so it sees no session.
	if _, body := f.do("GET", "/v1/clients/7e:2a:ea:04:b1:8f", "office", nil); len(body["sessions"].([]any)) != 0 {
		t.Fatalf("office sees %v", body["sessions"])
	}
	if code, _ := f.do("GET", "/v1/clients/not-a-mac", "griff", nil); code != 400 {
		t.Fatalf("a bad MAC: %d", code)
	}
}

// A session's retries are over its busy minutes alone (0303, F17): what
// the reports' running count says is not used, nor are its idle minutes,
// where rate control's tries are most of what is sent.
func TestClientJourneyResends(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	const mac = "7e:2a:ea:04:b1:8f"
	client := map[string]any{"mac": mac, "network": "sweet", "ssid": "Home Wi-Fi", "band": "6g", "signal": -55,
		"tx_rate": 900, "connected": 600, "tx_packets": 9000, "tx_retries": 6000, "tx_failed": 0}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "clients": []any{client}}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	// Five idle minutes, most of their frames sent again, and three busy
	// ones, a tenth sent again.
	now := time.Now().Truncate(time.Minute)
	var ms []conditions.ClientMinute
	for i := 1; i <= 8; i++ {
		c := conditions.Counts{TxPackets: 800, TxRetries: 600, TxBytes: 300 << 10}
		if i <= 3 {
			c = conditions.Counts{TxPackets: 20000, TxRetries: 2000, TxBytes: 30 << 20, RxBytes: 1 << 20}
		}
		ms = append(ms, conditions.ClientMinute{MAC: mac, At: now.Add(-time.Duration(i) * time.Minute), AP: hierarchy.NodeID(ap), Band: "6g", SSID: "Home Wi-Fi", Secs: 60, Counts: c})
	}
	if err := f.api.conds.RecordMinutes(nil, ms); err != nil {
		t.Fatal(err)
	}
	code, body := f.do("GET", "/v1/clients/"+mac, "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	sessions, _ := body["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("journey = %v", body)
	}
	if s := sessions[0].(map[string]any); s["retries"] != 0.1 || s["busy_minutes"] != 3.0 {
		t.Fatalf("session = %v", s)
	}
	if issues, _ := body["issues"].([]any); len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
}

// An address set by hand is weighed by the subnet its network has there, as
// the APs' client tests were given it by DHCP; and another client holding
// the address it holds now is a conflict (0325).
func TestClientJourneyAddresses(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	raw := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	// A client test on sweet was given 192.168.20.108/24.
	a, err := f.api.conds.AddActionWith(hierarchy.NodeID(ap), "client-test", "", "system", raw(map[string]any{"network": "sweet", "ssid": "Home Wi-Fi", "band": "5g"}))
	if err != nil {
		t.Fatal(err)
	}
	detail := map[string]any{"v": 1, "ssid": "Home Wi-Fi", "band": "5g", "client": "02:00:00:00:00:f0", "outcome": "passed", "stage": "done", "reason": "", "steps": []any{},
		"dhcp": map[string]any{"ms": 284, "address": "192.168.20.108", "mask": 24, "router": "192.168.20.1", "dns": []string{"192.168.20.254"}, "server": "192.168.20.254", "lease": 86400}}
	if _, err := f.api.conds.FinishActionWith(hierarchy.NodeID(ap), a.ID, true, "passed", raw(detail)); err != nil {
		t.Fatal(err)
	}
	client := func(mac, address, dhcp, host string) map[string]any {
		return map[string]any{"mac": mac, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -55, "tx_rate": 400, "connected": 60, "host": host, "address": address, "dhcp": dhcp}
	}
	issues := func(mac string) map[string]string {
		code, body := f.do("GET", "/v1/clients/"+mac, "griff", nil)
		if code != 200 {
			t.Fatalf("%d %v", code, body)
		}
		out := map[string]string{}
		for _, x := range body["issues"].([]any) {
			is := x.(map[string]any)
			out[is["kind"].(string)] = is["message"].(string)
		}
		return out
	}
	// On the subnet: no note. Off it: a warning naming the subnet. And the
	// printer holds the phone's address, given it by DHCP: a conflict each.
	report := []any{client("7e:2a:ea:04:b1:01", "192.168.20.218", "static", "on-subnet"), client("7e:2a:ea:04:b1:02", "192.168.5.7", "static", "off-subnet"),
		client("7e:2a:ea:04:b1:03", "192.168.20.80", "static", "phone"), client("7e:2a:ea:04:b1:04", "192.168.20.80", "ok", "printer")}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "clients": report}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	if got := issues("7e:2a:ea:04:b1:01"); got["static"] != "" || got["static-off-subnet"] != "" || got["address-conflict"] != "" {
		t.Fatalf("on the subnet: %v", got)
	}
	if got := issues("7e:2a:ea:04:b1:02")["static-off-subnet"]; !strings.Contains(got, "192.168.5.7") || !strings.Contains(got, "192.168.20.0/24") {
		t.Fatalf("off the subnet: %q", got)
	}
	if got := issues("7e:2a:ea:04:b1:03")["address-conflict"]; !strings.Contains(got, "printer (7e:2a:ea:04:b1:04)") || !strings.Contains(got, "192.168.20.80") {
		t.Fatalf("the phone: %q", got)
	}
	if got := issues("7e:2a:ea:04:b1:04")["address-conflict"]; !strings.Contains(got, "phone (7e:2a:ea:04:b1:03)") {
		t.Fatalf("the printer: %q", got)
	}
	// The phone gone from the latest report: no conflict now, though an
	// earlier report had both.
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 61, "clients": report[3:]}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	if got := issues("7e:2a:ea:04:b1:04")["address-conflict"]; got != "" {
		t.Fatalf("after the phone left: %q", got)
	}
}
