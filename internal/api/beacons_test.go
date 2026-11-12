package api

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/alerts"
	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A span's stretch is the shortest that keeps it to beaconsPoints, and
// each divides the beacons alert's half hour, so the half hour ending with
// any stretch is whole stretches (0350).
func TestBeaconsStep(t *testing.T) {
	for _, st := range beaconsSteps {
		if alerts.BeaconsWindow%st != 0 {
			t.Errorf("%v does not divide the half hour", st)
		}
	}
	for hours, want := range map[int]time.Duration{1: 5 * time.Minute, 3: 5 * time.Minute, 12: 5 * time.Minute, 24: 10 * time.Minute, 36: 15 * time.Minute, 168: 30 * time.Minute} {
		if got := beaconsStep(hours); got != want {
			t.Errorf("%d hours: %v, want %v", hours, got, want)
		}
	}
}

// Each stretch is judged over the half hour that ends with it, the
// stretches before the span's first included, as the beacons alert judges
// its last half hour (0339): one in ten before the counts cover a quarter
// hour, one in fifty after; and only the span's stretches are given and
// summed.
func TestBeaconsLine(t *testing.T) {
	at := func(i int) time.Time { return time.Unix(int64(i)*900, 0) }
	b := func(sent, failed int64) conditions.RadioBeacons {
		return conditions.RadioBeacons{Sent: sent, Failed: failed, Over: 300}
	}
	pts, sum := beaconsLine(map[int]conditions.RadioBeacons{0: b(12000, 0), 1: b(6000, 6000), 3: b(12000, 0), 8: b(11800, 200), 9: b(1, 1)}, 1, 8, 2, at)
	var said []string
	for _, p := range pts {
		said = append(said, fmt.Sprintf("%d %d/%d %v", p.At.Unix()/900, p.Sent, p.Failed, p.Alert))
	}
	if got, want := strings.Join(said, "; "), "1 6000/6000 true; 3 12000/0 false; 8 11800/200 false"; got != want {
		t.Errorf("points:\n got %s\nwant %s", got, want)
	}
	if sum != (conditions.RadioBeacons{Sent: 29800, Failed: 6200, Over: 900}) {
		t.Errorf("sum: %+v", sum)
	}
	// A quarter hour of counts: one in fifty is an alert, a little less is
	// not.
	for failed, want := range map[int64]bool{300: true, 200: false} {
		pts, _ := beaconsLine(map[int]conditions.RadioBeacons{0: b(11700, failed), 1: b(11700, failed), 2: b(11700, failed)}, 2, 1, 3, at)
		if len(pts) != 1 || pts[0].Alert != want {
			t.Errorf("%d failed in each of three: %+v", failed, pts)
		}
	}
}

// GET /v1/beacons (0350): each radio's beacons in stretches over the hours
// asked, the stretches before the span read only to judge the half hours of
// its first; its last half hour as the beacons alert reads it; by AP name
// and radio, on one band where asked, of the APs the caller may view.
func TestBeacons(t *testing.T) {
	f := newFixture(t)
	for _, op := range []map[string]any{
		{"kind": "add-folder", "tree": "locations", "node": "annex", "name": "Annex", "parent": "house"},
		{"kind": "add-ap", "tree": "locations", "node": "annex-ap", "name": "AnnexAP", "parent": "annex"},
		{"kind": "grant", "account": "tenant", "tree": "locations", "node": "annex", "role": "viewer"},
	} {
		if code, b := f.change("griff", op); code != 200 {
			t.Fatalf("%v: %d %v", op, code, b)
		}
	}
	report := func(ago time.Duration, ap, radio, band string, sent, failed int64) {
		t.Helper()
		*f.skew = -ago
		must(t, f.conds.RecordBeacons(hierarchy.NodeID(ap), []conditions.RadioBeacons{{Radio: radio, Band: band, Sent: sent, Failed: failed, Over: 300}}))
	}
	report(3*time.Hour, "office-ap", "radio1", "2g", 1, 1)
	report(62*time.Minute, "office-ap", "radio1", "2g", 12000, 0)
	report(40*time.Minute, "office-ap", "radio1", "2g", 6000, 6000)
	report(12*time.Minute, "annex-ap", "radio1", "2g", 5000, 5000)
	report(10*time.Minute, "office-ap", "radio1", "2g", 12000, 0)
	report(10*time.Minute, "office-ap", "radio0", "5g", 12000, 0)
	report(5*time.Minute, "office-ap", "radio1", "2g", 12000, 0)
	*f.skew = 0

	get := func(as, query string) map[string]any {
		t.Helper()
		code, v := f.do("GET", "/v1/beacons"+query, as, nil)
		if code != 200 {
			t.Fatalf("beacons%s as %s: %d %v", query, as, code, v)
		}
		return v
	}
	radios := func(v map[string]any) string {
		var out []string
		for _, r := range v["radios"].([]any) {
			r := r.(map[string]any)
			out = append(out, fmt.Sprintf("%s %s %s", r["ap_name"], r["radio"], r["band"]))
		}
		return strings.Join(out, "; ")
	}
	v := get("griff", "?hours=1")
	if v["hours"] != float64(1) || v["step"] != float64(300) {
		t.Errorf("an hour: %v hours in %v s", v["hours"], v["step"])
	}
	if r := v["rule"].(map[string]any); r["pct"] != float64(2) || r["stuck_pct"] != float64(10) || r["fewest"] != float64(30) || r["window"] != float64(1800) || r["span"] != float64(900) {
		t.Errorf("the rule: %v", r)
	}
	from, err := time.Parse(time.RFC3339Nano, v["from"].(string))
	if err != nil || time.Since(from) < 55*time.Minute || time.Since(from) > time.Hour+time.Minute || from.Unix()%300 != 0 {
		t.Errorf("from: %v %v", v["from"], err)
	}
	if got, want := radios(v), "AnnexAP radio1 2g; OfficeOpenWrt radio0 5g; OfficeOpenWrt radio1 2g"; got != want {
		t.Errorf("radios:\n got %s\nwant %s", got, want)
	}
	if _, ok := v["cut"]; ok {
		t.Errorf("cut: %v", v["cut"])
	}
	// The office's 2.4 GHz radio: the three reports of the hour, not the
	// one before it; the burst at 40 minutes ago an alert over the half
	// hour that ends with it, the one before the span counted in it, and
	// the half hours since not.
	office := v["radios"].([]any)[2].(map[string]any)
	var pts []string
	for _, p := range office["points"].([]any) {
		p := p.(map[string]any)
		at, _ := time.Parse(time.RFC3339Nano, p["at"].(string))
		if at.Before(from) {
			t.Errorf("a point before the span: %v", p)
		}
		pts = append(pts, fmt.Sprintf("%v/%v %v", p["sent"], p["failed"], p["alert"] == true))
	}
	if got, want := strings.Join(pts, "; "), "6000/6000 true; 12000/0 false; 12000/0 false"; got != want {
		t.Errorf("points:\n got %s\nwant %s", got, want)
	}
	if office["sent"] != float64(30000) || office["failed"] != float64(6000) || office["over"] != float64(900) {
		t.Errorf("over the hour: %v", office)
	}
	// Its last half hour as the alert reads it: two reports, under a
	// quarter hour of counts, so one in ten; none failed.
	if h := office["half"].(map[string]any); h["sent"] != float64(24000) || h["failed"] != float64(0) || h["over"] != float64(600) || h["alert_at"] != float64(10) || h["alert"] != false {
		t.Errorf("its half hour: %v", h)
	}
	// The annex's: half its beacons failed, an alert.
	if h := v["radios"].([]any)[0].(map[string]any)["half"].(map[string]any); h["failed"] != float64(5000) || h["alert"] != true {
		t.Errorf("the annex's half hour: %v", h)
	}
	// One band; below a folder; the tenant's annex alone.
	if got := radios(get("griff", "?hours=1&band=2g")); got != "AnnexAP radio1 2g; OfficeOpenWrt radio1 2g" {
		t.Errorf("2.4 GHz: %s", got)
	}
	if got := radios(get("griff", "?hours=1&under=office")); got != "OfficeOpenWrt radio0 5g; OfficeOpenWrt radio1 2g" {
		t.Errorf("the office: %s", got)
	}
	if got := radios(get("tenant", "?hours=1")); got != "AnnexAP radio1 2g" {
		t.Errorf("the tenant's: %s", got)
	}
	// A day, in 10-minute stretches, has the report of three hours ago.
	d := get("griff", "?under=office&band=2g")
	if d["hours"] != float64(24) || d["step"] != float64(600) {
		t.Errorf("a day: %v hours in %v s", d["hours"], d["step"])
	}
	if r := d["radios"].([]any)[0].(map[string]any); r["sent"] != float64(42001) || r["failed"] != float64(6001) {
		t.Errorf("over the day: %v", r)
	}
	if w := get("griff", "?hours=168"); w["step"] != float64(1800) {
		t.Errorf("a week: %v s", w["step"])
	}
	for _, q := range []string{"?hours=0", "?hours=169", "?hours=x", "?band=4g"} {
		if code, _ := f.do("GET", "/v1/beacons"+q, "griff", nil); code != 400 {
			t.Errorf("%s: %d", q, code)
		}
	}
}
