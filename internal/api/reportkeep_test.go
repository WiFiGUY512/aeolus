package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A state report with a bad element in one of its lists is taken without it
// (0290), and an alert says so; one the manager cannot take at all is
// refused, and an alert says that.
func TestReportKeepsTheRest(t *testing.T) {
	f := newFixture(t)
	ap, token, version := f.adopted()
	good := map[string]any{"mac": "92:8e:1b:c6:4c:2c", "network": "sweet", "ssid": "Home Wi-Fi", "band": "5g", "signal": -60,
		"connected": 60, "inactive_ms": 10, "rx_bytes": 1, "tx_bytes": 1, "rx_packets": 1, "tx_packets": 1}
	alertOf := func(kind string) map[string]any {
		t.Helper()
		code, body := f.do("GET", "/v1/alerts", "griff", nil)
		if code != 200 {
			t.Fatalf("alerts: %d %v", code, body)
		}
		for _, a := range body["alerts"].([]any) {
			if m := a.(map[string]any); m["ap"] == ap && m["kind"] == kind {
				return m
			}
		}
		return nil
	}
	clientsKept := func() []any {
		t.Helper()
		_, cfg := f.do("GET", "/v1/aps/"+ap+"/config", "griff", nil)
		cl, _ := cfg["condition"].(map[string]any)["state"].(map[string]any)["report"].(map[string]any)["clients"].([]any)
		return cl
	}

	// v0.232.0's: one of its clients a steering reading, an array, beside
	// one as the prober writes them (0289).
	report := map[string]any{"version": version, "uptime": 60, "openwrt": "25.12.5",
		"clients": []any{[]any{"6c:ac:c2:cf:b6:3f", "5g", "86:2d:bc:a5", -60, 3}, good}}
	code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil)
	if code != 200 {
		t.Fatalf("a report with one bad client: %d %v", code, body)
	}
	// The answer says what was left out, for the agent.
	if lo, _ := body["left_out"].([]any); len(lo) != 1 || lo[0].(map[string]any)["field"] != "clients" || lo[0].(map[string]any)["dropped"] != float64(1) {
		t.Fatalf("left_out = %v", body["left_out"])
	}
	if cl := clientsKept(); len(cl) != 1 || cl[0].(map[string]any)["mac"] != "92:8e:1b:c6:4c:2c" {
		t.Fatalf("clients kept = %v", cl)
	}
	a := alertOf("report-trimmed")
	if a == nil || a["key"] != "report-trimmed:clients" || a["severity"] != "warning" {
		t.Fatalf("report-trimmed = %v", a)
	}
	if m, _ := a["message"].(string); !strings.Contains(m, "clients") || !strings.Contains(m, "1 of the 2 in its latest") ||
		!strings.Contains(m, "in 1 reports") || !strings.Contains(m, "cannot unmarshal array") {
		t.Fatalf("report-trimmed says %q", m)
	}
	if alertOf("reports-refused") != nil {
		t.Fatalf("a report taken in part is no report refused")
	}

	// A list that is no list is left out whole, and the rest taken.
	report["clients"] = map[string]any{"mac": "92:8e:1b:c6:4c:2c"}
	if code, _, body := f.apDo("POST", "/v1/ap/state", token, report, nil); code != 200 {
		t.Fatalf("clients that are no list: %d %v", code, body)
	}
	if cl := clientsKept(); len(cl) != 0 {
		t.Fatalf("clients kept = %v", cl)
	}
	if m, _ := alertOf("report-trimmed")["message"].(string); !strings.Contains(m, "no list at all") || !strings.Contains(m, "in 2 reports") {
		t.Fatalf("report-trimmed says %q", m)
	}

	// One it cannot take at all: a field it does not know.
	if code, _, _ := f.apDo("POST", "/v1/ap/state", token, map[string]any{"version": version, "uptime": 60, "temperature": 40}, nil); code != 400 {
		t.Fatalf("an unknown field: %d", code)
	}
	r := alertOf("reports-refused")
	if r == nil || r["severity"] != "warning" {
		t.Fatalf("reports-refused = %v", r)
	}
	if m, _ := r["message"].(string); !strings.Contains(m, "refused 1 of the 3") || !strings.Contains(m, "temperature") {
		t.Fatalf("reports-refused says %q", m)
	}
	// What was taken of the last report it took stays its latest.
	if cl := clientsKept(); len(cl) != 0 {
		t.Fatalf("clients kept = %v", cl)
	}
}

// What the manager keeps of an AP's reports holds for the window
// (alerts.ReportTroubleFor), and ends after.
func TestReportBookWindow(t *testing.T) {
	var b reportBook
	ap := hierarchy.NodeID("ap-a0046082275e")
	t0 := time.Date(2026, 11, 3, 6, 40, 0, 0, time.UTC)
	b.note(ap, t0, nil, "request body: json: cannot unmarshal array")
	b.note(ap, t0.Add(2*time.Minute), []reportTrim{{Field: "clients", Dropped: 1, Of: 9, Why: "json: cannot unmarshal array"}}, "")
	b.note(ap, t0.Add(4*time.Minute), nil, "")
	got := b.of(ap, t0.Add(10*time.Minute))
	if got == nil || got.Reports != 3 || got.Refused != 1 || !got.Since.Equal(t0) || len(got.Trimmed) != 1 {
		t.Fatalf("at 10 minutes: %+v", got)
	}
	if l := got.Trimmed[0]; l.Field != "clients" || l.Reports != 1 || l.Dropped != 1 || l.Of != 9 || !l.Since.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("clients trimmed: %+v", l)
	}
	// The refusal is 16 minutes old, the trim 14: one gone, the other held.
	got = b.of(ap, t0.Add(16*time.Minute))
	if got == nil || got.Refused != 0 || got.Reports != 2 || len(got.Trimmed) != 1 {
		t.Fatalf("at 16 minutes: %+v", got)
	}
	// A refusal after a quiet window starts a run of its own.
	b.note(ap, t0.Add(20*time.Minute), nil, "too large")
	if got = b.of(ap, t0.Add(21*time.Minute)); got == nil || got.Refused != 1 || !got.Since.Equal(t0.Add(20*time.Minute)) || got.Why != "too large" {
		t.Fatalf("at 21 minutes: %+v", got)
	}
	// Nothing in the window: nothing.
	if got = b.of(ap, t0.Add(40*time.Minute)); got != nil {
		t.Fatalf("at 40 minutes: %+v", got)
	}
	if got = b.of("ap-000000000000", t0); got != nil {
		t.Fatalf("an AP that sent none: %+v", got)
	}
}

// trimReport leaves out of each list it takes element by element what is
// not well said, inside an object too, and leaves the body as it was where
// nothing is.
func TestTrimReport(t *testing.T) {
	body := []byte(`{"version":3,"bssids":["02:11:22:33:44:55","nope"],"radios":[{"radio":"radio0","band":"5g"},{"radio":"radio1","band":"60g"}]}`)
	out, trims, err := trimReport(body)
	if err != nil || len(trims) != 2 {
		t.Fatalf("trims = %+v, %v", trims, err)
	}
	if string(out) != `{"bssids":["02:11:22:33:44:55"],"radios":[{"radio":"radio0","band":"5g"}],"version":3}` {
		t.Fatalf("out = %s", out)
	}
	if trims[0].Field != "radios" || trims[0].Dropped != 1 || trims[0].Of != 2 || !strings.Contains(trims[0].Why, "60g") {
		t.Fatalf("radios trimmed: %+v", trims[0])
	}
	clean := []byte(`{"version":3,"bssids":["02:11:22:33:44:55"]}`)
	if out, trims, err := trimReport(clean); err != nil || len(trims) != 0 || string(out) != string(clean) {
		t.Fatalf("a clean report: %s %+v %v", out, trims, err)
	}
	if _, _, err := trimReport([]byte(`[1,2]`)); err == nil {
		t.Fatalf("a body that is no object is taken")
	}
	// A list inside an object, as a later entry of reportLists may name.
	l := reportList{path: []string{"rrm", "moves"}, keep: element(func(m struct {
		Band string `json:"band"`
	}) error {
		if m.Band == "" {
			return badRequest("a move has a band")
		}
		return nil
	})}
	top := mustObject(t, `{"rrm":{"address":"192.168.1.2","moves":[{"band":"2g"},{"band":""}]}}`)
	tr := trimAt(top, l.path, l)
	if tr == nil || tr.Field != "rrm.moves" || tr.Dropped != 1 || tr.Of != 2 || string(top["rrm"]) != `{"address":"192.168.1.2","moves":[{"band":"2g"}]}` {
		t.Fatalf("rrm.moves: %+v %s", tr, top["rrm"])
	}
}

func mustObject(t *testing.T, s string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}
