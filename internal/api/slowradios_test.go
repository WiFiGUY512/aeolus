package api

import (
	"sort"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/conditions"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A radio runs slowly where its last three client tests that measured speed
// each got under a third of what their tester usually gets from it (0242),
// as the Living Room AP's 5 GHz did after a channel switch on the ath11k
// driver: 7 Mbit/s down from ce7f, where 84 was usual. One whose newest test
// is well, whose run is old, or whose testers have too few tests for a
// usual, does not.
func TestSlowRadios(t *testing.T) {
	now := time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC)
	names := map[hierarchy.NodeID]string{"ap-lr": "Living-Room-C-360", "ap-ce7f": "OpenWrtce7f", "ap-c360": "C360-AP"}
	name := func(id hierarchy.NodeID) (string, bool) { n, ok := names[id]; return n, ok }
	type speed struct {
		tester   string
		ago      time.Duration
		down, up float64
		sig      float64 // how strongly it heard the AP; 0 where it does not say
	}
	// The tests, the newest first, as the store gives them.
	tests := func(on, band string, ss ...speed) []testRecord {
		var out []testRecord
		for _, s := range ss {
			down, up, sig := s.down, s.up, s.sig
			d := &clientTest{Client: "02:00:00:00:00:01", Outcome: "passed", Speed: &testSpeed{Down: &down, Up: &up}}
			if sig != 0 {
				d.Link = &testLink{Signal: &sig}
			}
			out = append(out, testRecord{
				Action: conditions.Action{AP: s.tester, Kind: "client-test", At: now.Add(-s.ago), State: "done"},
				ask:    clientTestAsk{Network: "home-wi-fi", On: on, Band: band},
				detail: d,
			})
		}
		return out
	}
	usual := []speed{
		{"ap-ce7f", 20 * time.Hour, 84, 181, 0}, {"ap-ce7f", 30 * time.Hour, 91, 206, 0}, {"ap-ce7f", 40 * time.Hour, 76, 170, 0},
		{"ap-ce7f", 50 * time.Hour, 88, 167, 0}, {"ap-ce7f", 60 * time.Hour, 79, 160, 0}, {"ap-ce7f", 70 * time.Hour, 136, 190, 0},
	}
	slow := []speed{{"ap-ce7f", 10 * time.Minute, 7, 150, 0}, {"ap-ce7f", 20 * time.Minute, 7, 140, 0}, {"ap-ce7f", 30 * time.Minute, 6.9, 160, 0}}

	got := slowRadiosOf(tests("ap-lr", "5g", append(slow, usual...)...), now, name, nil)
	if len(got) != 1 || len(got["ap-lr"]) != 1 {
		t.Fatalf("slow radios: %+v", got)
	}
	r := got["ap-lr"][0]
	if r.Band != "5g" || len(r.Tests) != 3 || !r.Since.Equal(now.Add(-30*time.Minute)) {
		t.Fatalf("the radio: %+v", r)
	}
	// The usual is the upper quartile of the tester's tests: of nine, the
	// seventh, 88 down and 181 up.
	for _, st := range r.Tests {
		if st.Tester != "OpenWrtce7f" || !st.SlowDown || st.SlowUp || *st.UsualDown != 88 || *st.UsualUp != 181 {
			t.Errorf("a test: %+v (usual %v/%v)", st, *st.UsualDown, *st.UsualUp)
		}
	}

	// Well again in its newest test: nothing.
	well := append([]speed{{"ap-ce7f", 5 * time.Minute, 80, 170, 0}}, append(slow, usual...)...)
	if got := slowRadiosOf(tests("ap-lr", "5g", well...), now, name, nil); len(got) != 0 {
		t.Errorf("well again: %+v", got)
	}
	// Slow, but seven hours ago and not tested since: nothing.
	var old []speed
	for _, s := range slow {
		s.ago += 7 * time.Hour
		old = append(old, s)
	}
	if got := slowRadiosOf(tests("ap-lr", "5g", append(old, usual...)...), now, name, nil); len(got) != 0 {
		t.Errorf("an old run: %+v", got)
	}
	// Too few tests for a usual: nothing.
	if got := slowRadiosOf(tests("ap-lr", "5g", append(slow, usual[:1]...)...), now, name, nil); len(got) != 0 {
		t.Errorf("no usual: %+v", got)
	}
	// Two testers, each slow against its own usual, make a run of three.
	c360 := []speed{{"ap-c360", 15 * time.Minute, 60, 300, 0}}
	for i := 0; i < 5; i++ {
		c360 = append(c360, speed{"ap-c360", time.Duration(20+i) * time.Hour, 260, 350, 0})
	}
	mixed := tests("ap-lr", "5g", slow[0], c360[0], slow[1])
	mixed = append(mixed, tests("ap-lr", "5g", append(usual, c360[1:]...)...)...)
	got = slowRadiosOf(mixed, now, name, nil)
	if len(got["ap-lr"]) != 1 || got["ap-lr"][0].Tests[1].Tester != "C360-AP" || *got["ap-lr"][0].Tests[1].UsualDown != 260 {
		t.Errorf("two testers: %+v", got)
	}
	// A tester moved, as ce7f was into the Living Room AP's place: it hears
	// the radio 25 dB less, and its slow tests have no usual at a like
	// signal (0242). The same tests heard as before are slow.
	var near, moved, same []speed
	for i := 0; i < 6; i++ {
		near = append(near, speed{"ap-ce7f", time.Duration(20+i) * time.Hour, 260, 350, -45})
	}
	for _, s := range slow {
		s.down, s.up = 60, 80
		s.sig = -70
		moved = append(moved, s)
		s.sig = -46
		same = append(same, s)
	}
	if got := slowRadiosOf(tests("ap-lr", "5g", append(moved, near...)...), now, name, nil); len(got) != 0 {
		t.Errorf("a tester moved: %+v", got)
	}
	if got := slowRadiosOf(tests("ap-lr", "5g", append(same, near...)...), now, name, nil); len(got["ap-lr"]) != 1 || *got["ap-lr"][0].Tests[0].UsualDown != 260 {
		t.Errorf("heard as before: %+v", got)
	}
	// Tests over the wire, failed, or on an AP not in the tree say nothing.
	if got := slowRadiosOf(tests("ap-gone", "5g", append(slow, usual...)...), now, name, nil); len(got) != 0 {
		t.Errorf("an AP not in the tree: %+v", got)
	}
	if got := slowRadiosOf(tests("ap-lr", wireBand, append(slow, usual...)...), now, name, nil); len(got) != 0 {
		t.Errorf("over the wire: %+v", got)
	}
}

// A run of slow tests on a channel the radio has since left speaks for
// that channel (0268): OfficeOpenWrt's 5 GHz was slow on DFS 108, and went
// to 149. On the channel it is on, they stand.
func TestSlowRadioMovedOn(t *testing.T) {
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	name := func(id hierarchy.NodeID) (string, bool) { return string(id), true }
	var list []testRecord
	add := func(ago time.Duration, down float64, ch int) {
		up := 300.0
		list = append(list, testRecord{
			Action: conditions.Action{AP: "ap-lr", Kind: "client-test", At: now.Add(-ago), State: "done"},
			ask:    clientTestAsk{Network: "home-wi-fi", On: "ap-office", Band: "5g"},
			detail: &clientTest{Client: "02:00:00:00:00:01", Outcome: "passed", Channel: ch, Speed: &testSpeed{Down: &down, Up: &up}},
		})
	}
	for i, d := range []float64{0.4, 23.6, 2.1} {
		add(time.Duration(10+10*i)*time.Minute, d, 108)
	}
	for i := 0; i < 6; i++ {
		add(time.Duration(20+10*i)*time.Hour, 136, 108)
	}
	at := func(ch int) func(hierarchy.NodeID, string) int {
		return func(hierarchy.NodeID, string) int { return ch }
	}
	if got := slowRadiosOf(list, now, name, at(149)); len(got) != 0 {
		t.Fatalf("moved on to 149: %+v", got)
	}
	if got := slowRadiosOf(list, now, name, at(108)); len(got["ap-office"]) != 1 {
		t.Fatalf("still on 108: %+v", got)
	}
	if got := slowRadiosOf(list, now, name, at(0)); len(got["ap-office"]) != 1 {
		t.Fatalf("its channel not known: %+v", got)
	}
}

// A run of one tester's slow tests is its way to the radio, not the radio,
// where another tester got its usual from the radio since that tester's
// tests began to be slow (0308): the C260's scan radio uploaded 0.2-0.8
// Mbit/s to the Living Room's 2.4 GHz from 11-04 00:14, where 11-14 was
// usual, while the Garage got its usual from it at 17:00. Where the other
// tester's latest is from before, or is slow too, the radio is slow.
func TestSlowRadioOneTestersWay(t *testing.T) {
	now := time.Date(2026, 11, 4, 23, 45, 0, 0, time.UTC)
	names := map[hierarchy.NodeID]string{"ap-lr": "Living-Room-C-360", "ap-c260": "C260", "ap-garage": "Garage-C-360"}
	name := func(id hierarchy.NodeID) (string, bool) { n, ok := names[id]; return n, ok }
	type speed struct {
		tester   string
		ago      time.Duration
		down, up float64
	}
	tests := func(ss ...speed) []testRecord {
		sort.Slice(ss, func(i, j int) bool { return ss[i].ago < ss[j].ago })
		var out []testRecord
		for _, s := range ss {
			down, up := s.down, s.up
			out = append(out, testRecord{
				Action: conditions.Action{AP: s.tester, Kind: "client-test", At: now.Add(-s.ago), State: "done"},
				ask:    clientTestAsk{Network: "home-wi-fi", On: "ap-lr", Band: "2g"},
				detail: &clientTest{Client: "02:00:00:00:00:01", Outcome: "passed", Speed: &testSpeed{Down: &down, Up: &up}},
			})
		}
		return out
	}
	var base []speed
	for i := 0; i < 6; i++ {
		ago := time.Duration(30+10*i) * time.Hour
		base = append(base, speed{"ap-c260", ago, 45, 12}, speed{"ap-garage", ago + time.Hour, 62, 55})
	}
	// The C260's slow streak, from a day ago.
	streak := []speed{{"ap-c260", 15 * time.Minute, 13.2, 0.5}, {"ap-c260", 30 * time.Minute, 1.3, 0.2}, {"ap-c260", time.Hour, 13.6, 0.5},
		{"ap-c260", 12 * time.Hour, 7.2, 0.6}, {"ap-c260", 24 * time.Hour, 11, 0.5}}
	all := func(more ...speed) []testRecord {
		return tests(append(append(append([]speed{}, base...), streak...), more...)...)
	}

	if got := slowRadiosOf(all(speed{"ap-garage", 7 * time.Hour, 67.4, 53.2}), now, name, nil); len(got) != 0 {
		t.Fatalf("the Garage got its usual 7 h ago, within the C260's streak: %+v", got)
	}
	if got := slowRadiosOf(all(), now, name, nil); len(got["ap-lr"]) != 1 {
		t.Fatalf("the Garage's latest is from before the streak: %+v", got)
	}
	if got := slowRadiosOf(all(speed{"ap-garage", 20 * time.Minute, 60, 4}), now, name, nil); len(got["ap-lr"]) != 1 || len(got["ap-lr"][0].Tests) != 3 {
		t.Fatalf("the Garage slow too: %+v", got)
	}
}
