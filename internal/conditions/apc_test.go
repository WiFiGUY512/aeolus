package conditions

import (
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Power control's steps (0206): an AP says each in report after report,
// and it is kept once; they are read back the latest first, of the APs
// asked for and the time; and the old ones are forgotten.
func TestAPCEvents(t *testing.T) {
	s, clock := open(t)
	now := *clock
	dbm := func(v int) *int { return &v }
	half, tenth := 0.5, 0.1
	step := func(ago time.Duration, from, to int, why string) APCEvent {
		return APCEvent{At: now.Add(-ago).Truncate(time.Second), Band: "5g", From: dbm(from), To: dbm(to), Why: why}
	}
	said := []APCEvent{step(40*24*time.Hour, 23, 20, "above"), step(2*time.Hour, 20, 17, "above"), step(time.Hour, 17, 14, "above")}
	back := step(30*time.Minute, 14, 17, "distress")
	back.Scanning, back.Left, back.Ratio = dbm(3), dbm(0), []*float64{&tenth, &half}
	said = append(said, back)
	must(t, s.RecordAPCEvents("ap-1", said))
	must(t, s.RecordAPCEvents("ap-1", said))
	must(t, s.RecordAPCEvents("ap-2", []APCEvent{step(10*time.Minute, 20, 17, "above")}))

	got, err := s.APCEvents(now.Add(-24*time.Hour), []hierarchy.NodeID{"ap-1"}, 10)
	must(t, err)
	if len(got) != 3 || got[0].Why != "distress" || *got[0].From != 14 || *got[0].To != 17 || *got[0].Scanning != 3 || *got[0].Ratio[1] != 0.5 ||
		got[1].Scanning != nil || got[1].Ratio != nil || got[2].AP != "ap-1" || !got[2].At.Equal(now.Add(-2*time.Hour).Truncate(time.Second)) {
		t.Fatalf("ap-1's day: %+v", got)
	}
	if both, err := s.APCEvents(now.Add(-24*time.Hour), []hierarchy.NodeID{"ap-1", "ap-2"}, 2); err != nil || len(both) != 2 || both[0].AP != "ap-2" {
		t.Errorf("both, at most 2: %+v %v", both, err)
	}
	n, err := s.PruneAPCEvents(now.Add(-30 * 24 * time.Hour))
	must(t, err)
	if n != 1 {
		t.Errorf("forgot %d, want the month-old one", n)
	}
	if all, _ := s.APCEvents(time.Time{}, []hierarchy.NodeID{"ap-1"}, 10); len(all) != 3 {
		t.Errorf("after forgetting: %d", len(all))
	}
}
