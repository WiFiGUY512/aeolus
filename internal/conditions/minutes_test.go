package conditions

import (
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// What each minute came to (0207): each minute is kept once however often
// an AP sends it; the radios' are read by AP, radio and time, on one band
// where asked; a client's by time; and the radios' and the clients' are
// forgotten each after their own time.
func TestMinutes(t *testing.T) {
	s, clock := open(t)
	now := clock.Truncate(time.Minute)
	busy := func(v float64) *float64 { return &v }
	r := func(ap, radio, band string, ago time.Duration, b float64) RadioMinute {
		return RadioMinute{AP: hierarchy.NodeID(ap), At: now.Add(-ago), Radio: radio, Band: band, Busy: busy(b), Counts: Counts{TxPackets: 10, TxRetries: 1}}
	}
	sig := -60
	c := func(ap string, ago time.Duration) ClientMinute {
		return ClientMinute{MAC: "aa:bb:cc:00:00:01", At: now.Add(-ago), AP: hierarchy.NodeID(ap), Band: "5g", SSID: "Lab", Signal: &sig, Secs: 60}
	}
	radios := []RadioMinute{r("ap-1", "radio0", "5g", 2*time.Minute, 20), r("ap-1", "radio0", "5g", time.Minute, 30), r("ap-1", "radio1", "2g", time.Minute, 60),
		r("ap-2", "radio0", "5g", 40*24*time.Hour, 5)}
	clients := []ClientMinute{c("ap-1", 2*time.Minute), c("ap-2", time.Minute), c("ap-2", 8*24*time.Hour)}
	// One said what it hears of the AP (802.11k).
	heard := -62
	clients[1].Heard = &heard
	must(t, s.RecordMinutes(radios, clients))
	must(t, s.RecordMinutes(radios[:2], clients[:1]))

	got, err := s.RadioMinutes(now.Add(-time.Hour), []hierarchy.NodeID{"ap-1", "ap-2"}, "")
	must(t, err)
	if len(got) != 3 || got[0].Radio != "radio0" || *got[0].Busy != 20 || *got[1].Busy != 30 || got[2].Band != "2g" || got[0].TxRetries != 1 {
		t.Fatalf("the hour's radios: %+v", got)
	}
	if got, _ := s.RadioMinutes(now.Add(-time.Hour), []hierarchy.NodeID{"ap-1"}, "2g"); len(got) != 1 || *got[0].Busy != 60 {
		t.Errorf("2.4 GHz: %+v", got)
	}
	cs, err := s.ClientMinutes("aa:bb:cc:00:00:01", now.Add(-time.Hour), []hierarchy.NodeID{"ap-1", "ap-2"})
	must(t, err)
	if len(cs) != 2 || cs[0].AP != "ap-1" || cs[1].AP != "ap-2" || *cs[0].Signal != -60 || cs[0].SSID != "Lab" || cs[0].Heard != nil || cs[1].Heard == nil || *cs[1].Heard != -62 {
		t.Errorf("the client's hour: %+v", cs)
	}
	rn, cn, err := s.PruneMinutes(now.Add(-30*24*time.Hour), now.Add(-7*24*time.Hour))
	must(t, err)
	if rn != 1 || cn != 1 {
		t.Errorf("forgot %d radios' and %d clients' minutes, want 1 and 1", rn, cn)
	}
}

// The newest minute of an AP's radios (0311): of each radio, over the time
// asked; of any, of all kept; and none of an AP with none.
func TestLastMinutes(t *testing.T) {
	s, clock := open(t)
	now := clock.Truncate(time.Minute)
	r := func(ap, radio string, ago time.Duration) RadioMinute {
		return RadioMinute{AP: hierarchy.NodeID(ap), At: now.Add(-ago), Radio: radio, Band: "5g"}
	}
	must(t, s.RecordMinutes([]RadioMinute{r("ap-1", "radio0", 3*time.Minute), r("ap-1", "radio0", 2*time.Minute), r("ap-1", "radio1", 30*time.Minute),
		r("ap-1", "radio2", 3*24*time.Hour), r("ap-2", "radio0", 5*24*time.Hour)}, nil))
	radios, last, err := s.LastMinutes("ap-1", now.Add(-24*time.Hour))
	must(t, err)
	if len(radios) != 2 || !radios["radio0"].Equal(now.Add(-2*time.Minute)) || !radios["radio1"].Equal(now.Add(-30*time.Minute)) || !last.Equal(now.Add(-2*time.Minute)) {
		t.Errorf("ap-1: %v, last %v", radios, last)
	}
	radios, last, err = s.LastMinutes("ap-2", now.Add(-24*time.Hour))
	must(t, err)
	if len(radios) != 0 || !last.Equal(now.Add(-5*24*time.Hour)) {
		t.Errorf("ap-2: %v, last %v", radios, last)
	}
	radios, last, err = s.LastMinutes("ap-3", now.Add(-24*time.Hour))
	must(t, err)
	if len(radios) != 0 || !last.IsZero() {
		t.Errorf("ap-3: %v, last %v", radios, last)
	}
}
