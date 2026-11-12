package conditions

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Each radio's beacons are kept a report at a time (0339), and read back
// summed over the reports since a time, a radio each; and trimmed with the
// reports.
func TestBeacons(t *testing.T) {
	s, clock := open(t)
	start := *clock
	must(t, s.RecordBeacons("ap-1", []RadioBeacons{{Radio: "radio1", Band: "2g", Sent: 11800, Failed: 200, Over: 300}, {Radio: "radio0", Band: "5g", Sent: 12000, Over: 300}}))
	must(t, s.RecordBeacons("ap-2", []RadioBeacons{{Radio: "radio1", Band: "2g", Sent: 5000, Failed: 5000, Over: 300}}))
	must(t, s.RecordBeacons("ap-1", nil))
	*clock = start.Add(10 * time.Minute)
	must(t, s.RecordBeacons("ap-1", []RadioBeacons{{Radio: "radio1", Band: "2g", Sent: 11900, Failed: 100, Over: 300}}))

	got, err := s.BeaconsSince("ap-1", start)
	must(t, err)
	if len(got) != 2 || got[0] != (RadioBeacons{Radio: "radio0", Band: "5g", Sent: 12000, Over: 300}) ||
		got[1] != (RadioBeacons{Radio: "radio1", Band: "2g", Sent: 23700, Failed: 300, Over: 600}) {
		t.Fatalf("ap-1's half hour: %+v", got)
	}
	if got, err := s.BeaconsSince("ap-1", start.Add(time.Minute)); err != nil || len(got) != 1 || got[0].Failed != 100 {
		t.Errorf("ap-1's last: %+v %v", got, err)
	}
	if got, err := s.BeaconsSince("ap-3", start); err != nil || got != nil {
		t.Errorf("ap-3, none: %+v %v", got, err)
	}
	*clock = start.Add(5 * time.Minute).Add(time.Hour)
	if _, err := s.TrimStates(time.Hour); err != nil {
		t.Fatal(err)
	}
	if got, err := s.BeaconsSince("ap-1", start); err != nil || len(got) != 1 || got[0].Over != 300 {
		t.Errorf("after the trim: %+v %v", got, err)
	}
}

// Each radio's beacons are read back summed over each stretch of a span
// (0350), of the APs asked for, on one band where it is asked, by AP, radio
// and time; a stretch with no report has no row; every 0 is one stretch.
func TestBeaconsOver(t *testing.T) {
	s, clock := open(t)
	start := *clock
	report := func(after time.Duration, ap string, radios ...RadioBeacons) {
		t.Helper()
		*clock = start.Add(after)
		must(t, s.RecordBeacons(hierarchy.NodeID(ap), radios))
	}
	two := func(sent, failed int64) RadioBeacons {
		return RadioBeacons{Radio: "radio1", Band: "2g", Sent: sent, Failed: failed, Over: 300}
	}
	five := RadioBeacons{Radio: "radio0", Band: "5g", Sent: 12000, Over: 300}
	report(-time.Minute, "ap-1", two(1, 1)) // before the span
	report(0, "ap-1", two(11800, 200), five)
	report(5*time.Minute, "ap-1", two(11900, 100))
	report(20*time.Minute, "ap-1", two(12000, 0))
	report(5*time.Minute, "ap-2", two(5000, 5000))
	report(5*time.Minute, "ap-3", two(9, 9)) // not asked for

	got, err := s.BeaconsOver([]hierarchy.NodeID{"ap-1", "ap-2"}, "", start, 10*time.Minute, 100)
	must(t, err)
	var said []string
	for _, b := range got {
		said = append(said, fmt.Sprintf("%s %s %s +%v %d/%d/%d", b.AP, b.Radio, b.Band, b.At.Sub(start), b.Sent, b.Failed, b.Over))
	}
	want := []string{
		"ap-1 radio0 5g +0s 12000/0/300",
		"ap-1 radio1 2g +0s 23700/300/600",
		"ap-1 radio1 2g +20m0s 12000/0/300",
		"ap-2 radio1 2g +0s 5000/5000/300",
	}
	if strings.Join(said, "; ") != strings.Join(want, "; ") {
		t.Errorf("stretches:\n got %s\nwant %s", strings.Join(said, "; "), strings.Join(want, "; "))
	}
	// One band; and the rows read stop at most.
	if got, err := s.BeaconsOver([]hierarchy.NodeID{"ap-1", "ap-2"}, "5g", start, 10*time.Minute, 100); err != nil || len(got) != 1 || got[0].Radio != "radio0" {
		t.Errorf("5 GHz: %+v %v", got, err)
	}
	if got, err := s.BeaconsOver([]hierarchy.NodeID{"ap-1", "ap-2"}, "2g", start, 10*time.Minute, 2); err != nil || len(got) != 2 {
		t.Errorf("at most 2: %+v %v", got, err)
	}
	// One stretch since a time, as the beacons alert sums them.
	if got, err := s.BeaconsOver([]hierarchy.NodeID{"ap-1"}, "2g", start.Add(time.Minute), 0, 100); err != nil || len(got) != 1 ||
		got[0].RadioBeacons != (RadioBeacons{Radio: "radio1", Band: "2g", Sent: 23900, Failed: 100, Over: 600}) || !got[0].At.Equal(start.Add(time.Minute)) {
		t.Errorf("since a minute in: %+v %v", got, err)
	}
	// No AP, nothing.
	if got, err := s.BeaconsOver(nil, "", start, time.Minute, 100); err != nil || len(got) != 0 {
		t.Errorf("no AP: %+v %v", got, err)
	}
}

// The stretches are read by the AP and time index, the span's rows alone:
// the manager's store is busy, and keeps a month of them.
func TestBeaconsOverByIndex(t *testing.T) {
	s, _ := open(t)
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN `+beaconsOver, 0, 600000, `["ap-1","ap-2"]`, 0, "2g", "2g", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	all := strings.Join(plan, "; ")
	if !strings.Contains(all, "SEARCH beacons USING INDEX beacons_ap (ap=? AND at>?)") || strings.Contains(all, "SCAN beacons") {
		t.Fatalf("plan: %s", all)
	}
}
