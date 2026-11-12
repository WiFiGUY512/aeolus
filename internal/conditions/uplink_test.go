package conditions

import (
	"testing"
	"time"
)

// An AP's uplink ports' minutes are kept each once (0351), read back by
// port and time since a time, at most a number of them, and pruned.
func TestUplinkMinutes(t *testing.T) {
	s, clock := open(t)
	at := clock.Truncate(time.Minute)
	f := func(v float64) *float64 { return &v }
	speed, full := 2500, true
	m := func(port string, ago time.Duration, rx float64) UplinkMinute {
		return UplinkMinute{AP: "ap-1", At: at.Add(-ago), Port: port, Up: true, Speed: &speed, Full: &full, Secs: 60,
			Rx: f(rx), Tx: f(rx / 2), RxMax: f(rx * 2), TxMax: f(rx), RxBytes: int64(rx * 7500000), TxBytes: 1, Errors: 1, Changes: 0}
	}
	must(t, s.RecordUplinkMinutes([]UplinkMinute{m("lan1", 2*time.Minute, 100), m("lan1", time.Minute, 200), m("lan0", time.Minute, 50)}))
	// Sent again, a minute is kept once; another AP's is its own.
	must(t, s.RecordUplinkMinutes([]UplinkMinute{m("lan1", time.Minute, 999)}))
	other := m("lan1", time.Minute, 5)
	other.AP = "ap-2"
	must(t, s.RecordUplinkMinutes([]UplinkMinute{other}))
	must(t, s.RecordUplinkMinutes(nil))

	got, err := s.UplinkMinutes("ap-1", at.Add(-time.Hour), 100)
	must(t, err)
	if len(got) != 3 || got[0].Port != "lan0" || got[1].Port != "lan1" || *got[1].Rx != 100 || *got[2].Rx != 200 || !got[2].At.Equal(at.Add(-time.Minute)) ||
		*got[2].Speed != 2500 || !*got[2].Full || got[2].RxBytes != 1500000000 || got[2].Errors != 1 {
		t.Fatalf("ap-1's minutes: %+v", got)
	}
	if got, err := s.UplinkMinutes("ap-1", at.Add(-90*time.Second), 100); err != nil || len(got) != 2 {
		t.Errorf("since 90 s ago: %+v %v", got, err)
	}
	if got, err := s.UplinkMinutes("ap-1", at.Add(-time.Hour), 1); err != nil || len(got) != 1 {
		t.Errorf("at most one: %+v %v", got, err)
	}
	if n, err := s.PruneUplinkMinutes(at.Add(-90 * time.Second)); err != nil || n != 1 {
		t.Errorf("pruned %d %v", n, err)
	}
	if got, err := s.UplinkMinutes("ap-2", at.Add(-time.Hour), 100); err != nil || len(got) != 1 || *got[0].Rx != 5 {
		t.Errorf("ap-2's: %+v %v", got, err)
	}
}
