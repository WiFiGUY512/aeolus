package api

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// GET /v1/usage (0108): what the APs' clients moved, worked out from each
// report and the one before, and the most clients each AP had.
func TestUsage(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	client := func(mac string, connected, rx, tx int) map[string]any {
		return map[string]any{"mac": mac, "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -60, "signal_avg": -60,
			"tx_rate": 400, "rx_rate": 300, "connected": connected, "inactive_ms": 10, "rx_bytes": rx, "tx_bytes": tx, "rx_packets": 1, "tx_packets": 1}
	}
	withHost := func(c map[string]any, host string) map[string]any { c["host"] = host; return c }
	for i, clients := range [][]any{
		// It joined in the last interval: all it moved counts.
		{withHost(client("7e:2a:ea:04:b1:8f", 30, 1000, 5000), "griffs-phone")},
		// It moved 4000 down and 500 up since; another, on for two hours,
		// was not in the last report and counts nothing yet.
		{client("7e:2a:ea:04:b1:8f", 330, 1500, 9000), client("84:0d:8e:73:9a:f7", 7200, 10, 10)},
		// It joined again: from zero.
		{client("7e:2a:ea:04:b1:8f", 20, 100, 200)},
	} {
		if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60 + i, "clients": clients}, nil); code != 200 {
			t.Fatalf("state %d: %d %v", i, code, body)
		}
	}
	code, body := f.do("GET", "/v1/usage?under=office&hours=24", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if body["down"] != 9200.0 || body["up"] != 1600.0 || len(body["buckets"].([]any)) != 48 || body["bucket"] != 1800.0 {
		t.Fatalf("usage = down %v up %v, %d buckets of %v s", body["down"], body["up"], len(body["buckets"].([]any)), body["bucket"])
	}
	aps := body["aps"].([]any)
	if a := aps[0].(map[string]any); a["ap"] != ap || a["peak"] != 2.0 || a["down"] != 9200.0 {
		t.Fatalf("aps = %v", aps)
	}
	// It reported in the last bucket, and in no other; office-ap never did.
	heard := func(a any) (n int, last bool) {
		h := a.(map[string]any)["heard"].([]any)
		for _, x := range h {
			if x == true {
				n++
			}
		}
		return n, h[len(h)-1] == true
	}
	if n, last := heard(aps[0]); n != 1 || !last {
		t.Fatalf("heard = %v", aps[0])
	}
	if n, _ := heard(aps[1]); n != 0 {
		t.Fatalf("office-ap heard = %v", aps[1])
	}
	// The client that moved it all is the top one (0110), by its host name;
	// the one that moved nothing is not there.
	top := body["clients"].([]any)
	if c := top[0].(map[string]any); len(top) != 1 || c["mac"] != "7e:2a:ea:04:b1:8f" || c["host"] != "griffs-phone" || c["down"] != 9200.0 || c["up"] != 1600.0 || c["aps"].([]any)[0] != "ShedAP" {
		t.Fatalf("top clients = %v", top)
	}
	if code, _ := f.do("GET", "/v1/usage?hours=0", "griff", nil); code != 400 {
		t.Fatalf("hours=0: %d", code)
	}
	// office has no role in Locations: no APs, nothing moved.
	if _, body := f.do("GET", "/v1/usage", "office", nil); len(body["aps"].([]any)) != 0 || body["down"] != 0.0 || len(body["clients"].([]any)) != 0 {
		t.Fatalf("office = %v", body)
	}
}

// After a gap longer than usageGap, a client that was there before counts
// nothing, as the gap's bytes would all fall in one bucket; one that joined
// in the last interval counts all it moved.
func TestUsageOfAGap(t *testing.T) {
	now := time.Now()
	prev, _ := json.Marshal(map[string]any{"clients": []map[string]any{{"mac": "7e:2a:ea:04:b1:8f", "connected": 1000, "rx_bytes": 1000, "tx_bytes": 1000}}})
	down, up := usageOf(&conditions.State{At: now.Add(-2 * time.Hour), Report: prev}, now, []wifiClient{
		{MAC: "7e:2a:ea:04:b1:8f", Connected: 8200, RxBytes: 9e9, TxBytes: 9e9},
		{MAC: "84:0d:8e:73:9a:f7", Connected: 120, RxBytes: 300, TxBytes: 700},
	})
	if down != 700 || up != 300 {
		t.Fatalf("down %d, up %d", down, up)
	}
}

// Never more than 48 buckets, for any span.
func TestUsageBuckets(t *testing.T) {
	f := newFixture(t)
	for _, hours := range []int{1, 5, 6, 7, 13, 24, 25, 48, 49, 168, 719, 720, 2000, 8760} {
		_, body := f.do("GET", "/v1/usage?hours="+fmt.Sprint(hours), "griff", nil)
		n, b := len(body["buckets"].([]any)), body["bucket"].(float64)
		// Over more than two days, whole hours: it is read by the hour (0140).
		if n > 48 || float64(n)*b < float64(hours*3600) || int(b)%60 != 0 || hours > 48 && int(b)%3600 != 0 {
			t.Fatalf("hours=%d: %d buckets of %v s", hours, n, b)
		}
	}
}

// A counter that went back without the client joining again, as a 32-bit
// one does when it wraps, counts nothing that time.
func TestUsageOfAWrap(t *testing.T) {
	now := time.Now()
	prev, _ := json.Marshal(map[string]any{"clients": []map[string]any{{"mac": "7e:2a:ea:04:b1:8f", "connected": 1000, "rx_bytes": 4294967000, "tx_bytes": 10}}})
	down, up := usageOf(&conditions.State{At: now.Add(-5 * time.Minute), Report: prev}, now,
		[]wifiClient{{MAC: "7e:2a:ea:04:b1:8f", Connected: 1300, RxBytes: 200, TxBytes: 50}})
	if down != 0 || up != 0 {
		t.Fatalf("down %d, up %d", down, up)
	}
}

// A client that roamed between APs goes by the host name it gave last, on
// whichever AP (0110).
func TestTopClientsNameLast(t *testing.T) {
	f := newFixture(t)
	ap, _, _ := f.adopted()
	mac := "7e:2a:ea:04:b1:8f"
	if err := f.conds.RecordUse("office-ap", 1, nil, []conditions.ClientUse{{MAC: mac, Host: "old-name", Down: 10, Up: 1}}); err != nil {
		t.Fatal(err)
	}
	*f.skew = time.Minute // a later time by any clock, however coarse
	if err := f.conds.RecordUse(hierarchy.NodeID(ap), 1, nil, []conditions.ClientUse{{MAC: mac, Host: "new-name", Down: 20, Up: 2}}); err != nil {
		t.Fatal(err)
	}
	top, err := f.api.topClients(time.Now().Add(-time.Hour), map[hierarchy.NodeID]*usageAP{
		hierarchy.NodeID(ap): {AP: hierarchy.NodeID(ap), Name: "ShedAP"}, "office-ap": {AP: "office-ap", Name: "OfficeOpenWrt"},
	}, false)
	if err != nil || len(top) != 1 || top[0].Host != "new-name" || top[0].Down != 30 || len(top[0].APs) != 2 {
		t.Fatalf("top = %+v, %v", top, err)
	}
}

// Of two host names given at one time, as two reports within one tick of a
// coarse clock are, the one recorded last is the client's: on two APs
// whichever's rows come first, and on one (0110).
func TestTopClientsNameAtOneTime(t *testing.T) {
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	conds, err := conditions.Open(filepath.Join(t.TempDir(), "conditions.db"), func() time.Time { return now })
	must(t, err)
	t.Cleanup(func() { conds.Close() })
	for i, aps := range [][2]hierarchy.NodeID{{"ap-1", "ap-2"}, {"ap-2", "ap-1"}, {"ap-1", "ap-1"}} {
		mac := fmt.Sprintf("7e:2a:ea:04:b1:%02x", i)
		must(t, conds.RecordUse(aps[0], 1, nil, []conditions.ClientUse{{MAC: mac, Host: "old-name", Down: 10, Up: 1}}))
		must(t, conds.RecordUse(aps[1], 1, nil, []conditions.ClientUse{{MAC: mac, Host: "new-name", Down: 20, Up: 2}}))
	}
	top, err := (&Server{conds: conds}).topClients(now.Add(-time.Hour), map[hierarchy.NodeID]*usageAP{
		"ap-1": {AP: "ap-1", Name: "One"}, "ap-2": {AP: "ap-2", Name: "Two"},
	}, false)
	if err != nil || len(top) != 3 {
		t.Fatalf("top = %+v, %v", top, err)
	}
	for _, c := range top {
		if c.Host != "new-name" || c.Down != 30 {
			t.Fatalf("top = %+v", top)
		}
	}
}

// GET /v1/usage says the clients on each band in each bucket, and the
// attempts to come online that failed, for the Clients overview (0166).
func TestUsageBandsAndFailures(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	client := func(mac, band string) map[string]any {
		return map[string]any{"mac": mac, "network": "sweet", "ssid": "Home Wi-Fi", "band": band, "signal": -60, "signal_avg": -60,
			"tx_rate": 400, "rx_rate": 300, "connected": 30, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1}
	}
	clients := []any{client("7e:2a:ea:04:b1:8f", "2g"), client("84:0d:8e:73:9a:f7", "6g"), client("84:0d:8e:73:9a:f8", "6g")}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "clients": clients}, nil); code != 200 {
		t.Fatalf("state: %d %v", code, body)
	}
	failed := func(mac string, ago time.Duration) conditions.Connection {
		return conditions.Connection{MAC: mac, Started: time.Now().Add(-ago), Network: "sweet", SSID: "Home Wi-Fi",
			Band: "5g", Outcome: "failed", Stage: "key", Record: json.RawMessage(`{"events":[]}`)}
	}
	if _, err := f.conds.RecordConnections(hierarchy.NodeID(ap), []conditions.Connection{
		failed("aa:00:00:00:00:01", time.Minute), failed("aa:00:00:00:00:01", 2*time.Minute), failed("aa:00:00:00:00:02", time.Minute)}); err != nil {
		t.Fatal(err)
	}
	code, body := f.do("GET", "/v1/usage?under=office&hours=24", "griff", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	if body["failed"] != 3.0 || body["failed_clients"] != 2.0 {
		t.Fatalf("failed %v by %v clients", body["failed"], body["failed_clients"])
	}
	buckets := body["buckets"].([]any)
	last := buckets[len(buckets)-1].(map[string]any)
	if b, ok := last["bands"].(map[string]any); !ok || b["2g"] != 1.0 || b["5g"] != 0.0 || b["6g"] != 2.0 || last["clients"] != 3.0 {
		t.Fatalf("last bucket = %v", last)
	}
	n := 0
	for _, x := range buckets {
		n += int(x.(map[string]any)["failed"].(float64))
	}
	if n != 3 {
		t.Fatalf("failed across the buckets = %d", n)
	}
	// Over a week it is read by the hour, bands and all.
	_, body = f.do("GET", "/v1/usage?under=office&hours=168", "griff", nil)
	buckets = body["buckets"].([]any)
	last = buckets[len(buckets)-1].(map[string]any)
	if b, ok := last["bands"].(map[string]any); !ok || b["6g"] != 2.0 {
		t.Fatalf("by the hour, last bucket = %v", last)
	}
}
