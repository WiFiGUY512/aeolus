package journey

import (
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

var t0 = time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)

func dbm(v int) *int { return &v }

func sample(ap string, min int, band string, signal int, c Client) Sample {
	c.Band, c.Signal = band, dbm(signal)
	if c.Network == "" {
		c.Network, c.SSID = "lab", "Aeolus Lab"
	}
	return Sample{AP: hierarchy.NodeID("ap-" + ap), APName: ap, At: t0.Add(time.Duration(min) * time.Minute), C: c}
}

func TestSessionsAndRoams(t *testing.T) {
	j := Build("aa:bb:cc:00:11:22", []Sample{
		sample("office", 0, "5g", -55, Client{Connected: 120, TxRate: 400, Host: "griffs-phone"}),
		sample("office", 5, "5g", -58, Client{TxRate: 300}),
		// Roamed to the C-360 at 6 GHz.
		sample("c360", 10, "6g", -50, Client{TxRate: 1200}),
		sample("c360", 15, "6g", -52, Client{TxRate: 1100}),
		// Gone for an hour, then back on the office AP: a new session, not a roam.
		sample("office", 80, "5g", -60, Client{TxRate: 350}),
	})
	if len(j.Sessions) != 3 || j.Roams != 1 || j.Host != "griffs-phone" {
		t.Fatalf("journey = %+v", j)
	}
	s := j.Sessions[0]
	if !s.From.Equal(t0.Add(-2*time.Minute)) || !s.To.Equal(t0.Add(5*time.Minute)) || *s.SignalMin != -58 || *s.SignalMax != -55 || s.TxRate != 350 {
		t.Fatalf("first session = %+v", s)
	}
	if j.Sessions[1].From.Before(j.Sessions[0].To) {
		t.Fatalf("the second session starts before the first ended: %+v", j.Sessions[1])
	}
	if len(j.Issues) != 0 {
		t.Fatalf("issues = %+v", j.Issues)
	}
}

func TestIssues(t *testing.T) {
	var samples []Sample
	// Back and forth between two APs, each visit weak.
	for i := 0; i < 6; i++ {
		ap := "office"
		if i%2 == 1 {
			ap = "shed"
		}
		samples = append(samples, sample(ap, i*5, "5g", -80, Client{TxRate: 20, TxPackets: 1000, TxRetries: retried(400), DHCP: "none"}))
	}
	j := Build("aa:bb:cc:00:11:22", samples)
	// Busy minutes on the first visit, four in ten frames sent again.
	var ms []Minute
	for i := 0; i < 4; i++ {
		ms = append(ms, Minute{AP: "ap-office", Band: "5g", At: t0.Add(time.Duration(i) * time.Minute), TxPackets: 5000, TxRetries: 2000, Bytes: 4 << 20})
	}
	Resends(&j, ms)
	got := map[string]bool{}
	for _, is := range j.Issues {
		got[is.Kind] = true
	}
	for _, k := range []string{"weak-signal", "retries", "no-dhcp", "ping-pong"} {
		if !got[k] {
			t.Errorf("no %s issue: %+v", k, j.Issues)
		}
	}
	for _, is := range j.Issues {
		if is.Kind == "ping-pong" && !strings.Contains(is.Message, "office and shed") {
			t.Fatalf("ping-pong = %q", is.Message)
		}
	}
}

// TestWeakSignal: a session at -80 on average is weak, one at -79 is not
// (0293).
func TestWeakSignal(t *testing.T) {
	for _, c := range []struct {
		signal int
		weak   bool
	}{{-79, false}, {-80, true}, {-85, true}} {
		j := Build("aa:bb:cc:00:11:22", []Sample{sample("office", 0, "5g", c.signal, Client{TxRate: 350})})
		weak := false
		for _, is := range j.Issues {
			weak = weak || is.Kind == "weak-signal"
		}
		if weak != c.weak {
			t.Errorf("%d dBm: weak = %v, want %v (%+v)", c.signal, weak, c.weak, j.Issues)
		}
	}
}

func TestEmpty(t *testing.T) {
	if j := Build("aa:bb:cc:00:11:22", nil); len(j.Sessions) != 0 || len(j.Issues) != 0 {
		t.Fatalf("journey = %+v", j)
	}
}

// TestResends: a session's retries are over its busy minutes alone (0303,
// F17): its idle and light minutes, mostly rate control's tries, are left
// out; a session with no busy minute, or whose minutes are gone, has none,
// whatever the reports' running count said; a minute is the session's on
// its AP and band, and not the next session's; and the issue needs a share
// over a fifth over three busy minutes.
func TestResends(t *testing.T) {
	minute := func(ap, band string, at int, sent, again, bytes int64) Minute {
		return Minute{AP: hierarchy.NodeID("ap-" + ap), Band: band, At: t0.Add(time.Duration(at) * time.Minute), TxPackets: sent, TxRetries: again, Bytes: bytes}
	}
	build := func() Journey {
		return Build("aa:bb:cc:00:11:22", []Sample{
			sample("office", 0, "6g", -55, Client{Connected: 600, TxRate: 900, TxPackets: 9000, TxRetries: retried(6000)}),
			sample("office", 5, "6g", -55, Client{TxRate: 900}),
			sample("c360", 10, "5g", -60, Client{Connected: 60, TxRate: 600}),
			sample("c360", 15, "5g", -60, Client{TxRate: 600}),
		})
	}
	j := build()
	Resends(&j, []Minute{
		// The office session, from 10 minutes before its first report:
		// idle minutes, most of their frames sent again, and three busy.
		minute("office", "6g", -9, 600, 450, 200<<10), minute("office", "6g", -8, 800, 600, 300<<10),
		minute("office", "6g", -2, 20000, 2000, 30<<20), minute("office", "6g", 1, 30000, 3000, 40<<20), minute("office", "6g", 3, 10000, 1000, 15<<20),
		// The c360 session's minutes, idle only; and one on the office
		// AP while the client was on the c360: neither session's.
		minute("c360", "5g", 10, 900, 500, 100<<10), minute("office", "6g", 12, 50000, 40000, 60<<20),
	})
	s := j.Sessions[0]
	if s.Retries == nil || *s.Retries != 0.1 || s.BusyMinutes != 3 {
		t.Fatalf("office: retries %v over %d busy minutes, want 0.1 over 3", s.Retries, s.BusyMinutes)
	}
	if s := j.Sessions[1]; s.Retries != nil || s.BusyMinutes != 0 {
		t.Fatalf("c360, idle only: retries %v over %d busy minutes", s.Retries, s.BusyMinutes)
	}
	if len(j.Issues) != 0 {
		t.Fatalf("issues = %+v", j.Issues)
	}
	// Minutes gone: no retries, though the reports counted 6000 of 9000.
	j = build()
	Resends(&j, nil)
	if j.Sessions[0].Retries != nil {
		t.Fatalf("no minutes: retries %v", *j.Sessions[0].Retries)
	}
	// A fifth and more over three busy minutes is an issue; over two, not.
	for _, c := range []struct {
		busy  int
		issue bool
	}{{2, false}, {3, true}} {
		j = build()
		var ms []Minute
		for i := 0; i < c.busy; i++ {
			ms = append(ms, minute("office", "6g", i, 10000, 2500, 20<<20))
		}
		Resends(&j, ms)
		if got := len(j.Issues) > 0; got != c.issue {
			t.Errorf("%d busy minutes: issues %+v", c.busy, j.Issues)
		}
		want := "25% of frames to it on office were sent again, over 3 minutes it was busy: interference, or a signal too weak for its rate"
		if c.issue && (len(j.Issues) != 1 || j.Issues[0].Kind != "retries" || j.Issues[0].Message != want) {
			t.Errorf("the issue: %+v", j.Issues)
		}
	}
}

// From the review of #133: the AP's own SSIDs are kept apart; a session
// without a band is not slow on 5 GHz; and three roams are not yet back and
// forth.
func TestReviewFixes(t *testing.T) {
	own := func(ssid string, min int) Sample {
		s := sample("office", min, "5g", -60, Client{TxRate: 300})
		s.C.Network, s.C.SSID = "", ssid
		return s
	}
	if j := Build("aa:bb:cc:00:11:22", []Sample{own("home", 0), own("guest", 5)}); len(j.Sessions) != 2 {
		t.Fatalf("two of the AP's own SSIDs: %+v", j.Sessions)
	}
	j := Build("aa:bb:cc:00:11:22", []Sample{sample("office", 0, "", -60, Client{TxRate: 10}), sample("office", 5, "", -60, Client{TxRate: 10})})
	for _, is := range j.Issues {
		if is.Kind == "slow" {
			t.Fatalf("a session with no band is slow: %+v", is)
		}
	}
	var three []Sample
	for i, ap := range []string{"a", "b", "a", "b", "c"} {
		at := i * 5
		if ap == "c" {
			at = 300
		}
		three = append(three, sample(ap, at, "5g", -60, Client{TxRate: 300}))
	}
	for _, is := range Build("aa:bb:cc:00:11:22", three).Issues {
		if is.Kind == "ping-pong" {
			t.Fatalf("three roams called back and forth: %+v", is)
		}
	}
}

// retried is a count of retries, as a report gives one.
func retried(n int64) *int64 { return &n }

// Coming online at one AP again and again (0190): the most within any hour
// is counted, an AP at a time, and an issue said where that is fewest or
// more; "ping-pong" is taken back where it is.
func TestRejoins(t *testing.T) {
	at := time.Date(2026, 10, 21, 4, 0, 0, 0, time.UTC)
	var ons []Online
	// Living room: 25 times, 45 s apart; then, two hours on, 3 more.
	for i := 0; i < 25; i++ {
		ons = append(ons, Online{AP: "lr", Name: "Living Room", At: at.Add(time.Duration(i) * 45 * time.Second)})
	}
	for i := 0; i < 3; i++ {
		ons = append(ons, Online{AP: "lr", Name: "Living Room", At: at.Add(2*time.Hour + time.Duration(i)*time.Minute)})
	}
	// The office: 19 times, spread over the same hour.
	for i := 0; i < 19; i++ {
		ons = append(ons, Online{AP: "office", Name: "Office", At: at.Add(time.Duration(i) * 3 * time.Minute)})
	}
	j := Journey{Issues: []Issue{{Kind: "ping-pong"}, {Kind: "retries"}}}
	Rejoins(&j, ons, 20, at.Add(3*time.Hour))
	var kinds []string
	for _, i := range j.Issues {
		kinds = append(kinds, i.Kind)
	}
	if strings.Join(kinds, " ") != "retries rejoins" {
		t.Fatalf("issues %v", kinds)
	}
	// The office it left an hour on and tried nowhere again: it stayed.
	want := "it came online on Living Room 25 times within an hour, about every 45s: it drops its connection itself and joins again. On Office it stays, so something of that AP's radio does not suit it"
	if got := j.Issues[1].Message; got != want || !j.Issues[1].At.Equal(at) {
		t.Errorf("said %q at %s, want %q at %s", got, j.Issues[1].At, want, at)
	}
	// Fewer than fewest: nothing, and ping-pong stays.
	j = Journey{Issues: []Issue{{Kind: "ping-pong"}}}
	Rejoins(&j, ons[:19], 20, at.Add(3*time.Hour))
	if len(j.Issues) != 1 || j.Issues[0].Kind != "ping-pong" {
		t.Errorf("fewer than fewest: %v", j.Issues)
	}
}

// What a client that comes online at an AP again and again blames (0200):
// where it did the same on another AP and stayed on none, the client or
// what is beyond the APs, not the radio; where it stayed on another, the
// radio; where both, what the APs it does it on share; where it was on no
// other long enough to tell, nothing yet. An attempt that did not come
// online says it had left where it was.
func TestRejoinsBlame(t *testing.T) {
	at := time.Date(2026, 10, 21, 4, 0, 0, 0, time.UTC)
	every45 := func(ap, name string, from time.Duration, n int) []Online {
		var out []Online
		for i := 0; i < n; i++ {
			out = append(out, Online{AP: hierarchy.NodeID(ap), Name: name, At: at.Add(from + time.Duration(i)*45*time.Second)})
		}
		return out
	}
	once := func(ap, name string, from time.Duration) []Online { return every45(ap, name, from, 1) }
	tried := every45("lr", "Living Room", 19*time.Minute+45*time.Second, 20)
	for i := range tried {
		tried[i].Tried = true
	}
	loop := every45("lr", "Living Room", 0, 25)
	// As heard (0312): the loop at -79 dBm, the other at -62 or -70.
	heard := func(ons []Online, s int) []Online {
		out := append([]Online(nil), ons...)
		for i := range out {
			v := s
			out[i].Signal = &v
		}
		return out
	}
	for _, c := range []struct {
		name   string
		ons    []Online
		issues int
		want   string
	}{
		{"on two", append(every45("office", "Office", 20*time.Minute, 25), loop...), 2,
			", and did the same on Office: it drops its connection itself on every AP it joins, so it is not that AP's radio. It is the client itself, or something beyond the APs: the network or internet path its own traffic takes, as when its cloud connection stalls. Look there, not at the radio"},
		{"gone again from the other", append(append(once("office", "Office", 19*time.Minute), tried...), loop...), 1,
			", and did the same on Office: it drops its connection itself on every AP it joins"},
		{"stayed on the other", append(once("office", "Office", 19*time.Minute), loop...), 1,
			": it drops its connection itself and joins again. On Office it stays, so something of that AP's radio does not suit it"},
		{"too lately on the other", append(once("office", "Office", 55*time.Minute), loop...), 1,
			": it drops its connection itself and joins again. Nothing on another AP tells yet whether that AP's radio is the cause, or the client itself or something beyond the APs: see whether it does the same on another AP"},
		{"weaker here", append(heard(once("office", "Office", 19*time.Minute), -62), heard(loop, -79)...), 1,
			": it drops its connection itself and joins again. On Office it stays, heard at -62 dBm there against -79 dBm here: a link 17 dB weaker is the likelier cause, not that AP's radio. It comes back here as its own choice"},
		{"stayed on the other, as well heard", append(heard(once("office", "Office", 19*time.Minute), -70), heard(loop, -79)...), 1,
			": it drops its connection itself and joins again. On Office it stays, so something of that AP's radio does not suit it. It is heard at -79 dBm here"},
		{"on two, stayed on a third", append(append(once("ph", "Shed", 40*time.Minute), every45("office", "Office", 20*time.Minute, 25)...), loop...), 2,
			", and did the same on Office: it drops its connection itself and joins again. On Shed it stays: look at what that AP and Office share, unlike Shed: their kind of radio, or the path beyond them"},
	} {
		j := Journey{}
		Rejoins(&j, c.ons, 20, at.Add(time.Hour))
		var lr string
		for _, i := range j.Issues {
			if strings.HasPrefix(i.Message, "it came online on Living Room 25 times") {
				lr = i.Message
			}
		}
		if len(j.Issues) != c.issues || !strings.Contains(lr, c.want) {
			t.Errorf("%s: %d issues, Living Room's %q; want %d, with %q", c.name, len(j.Issues), lr, c.issues, c.want)
		}
		if radio := strings.Contains(lr, "radio does not suit"); radio != strings.HasPrefix(c.name, "stayed on the other") {
			t.Errorf("%s: blames the radio %v: %q", c.name, radio, lr)
		}
	}
}

// A client that drops off and comes back fewest times within an hour, on
// whichever APs, keeps dropping (0259): where it left several, the client or
// what is beyond the APs; where one, see another first. Fewer, or the same
// spread over more than an hour, is not it.
func TestDrops(t *testing.T) {
	base := time.Date(2026, 10, 31, 21, 16, 0, 0, time.UTC)
	drop := func(ap string, min int, away time.Duration) Drop {
		return Drop{AP: hierarchy.NodeID("ap-" + ap), Name: ap, At: base.Add(time.Duration(min) * time.Minute), Away: away}
	}
	kettle := []Drop{drop("57ff", 0, 100*time.Second), drop("c360", 7, 98*time.Second), drop("57ff", 14, 110*time.Second),
		drop("ce7f", 21, 97*time.Second), drop("57ff", 28, 120*time.Second)}
	var j Journey
	Drops(&j, kettle, 4)
	if len(j.Issues) != 1 || j.Issues[0].Kind != "drops" || !j.Issues[0].At.Equal(base) {
		t.Fatalf("issues: %+v", j.Issues)
	}
	m := j.Issues[0].Message
	for _, want := range []string{"5 times within an hour", "about every 7m0s", "away about 1m40s", "leaving 57ff, c360, ce7f", "look at the client"} {
		if !strings.Contains(m, want) {
			t.Errorf("message %q lacks %q", m, want)
		}
	}
	d, ok := KeepsDropping(kettle, 4)
	if !ok || d.Times != 5 || d.APs[0] != "57ff" {
		t.Errorf("dropping: %+v %v", d, ok)
	}
	j = Journey{}
	Drops(&j, []Drop{drop("57ff", 0, time.Minute), drop("57ff", 9, time.Minute), drop("57ff", 18, time.Minute), drop("57ff", 27, time.Minute)}, 4)
	if len(j.Issues) != 1 || !strings.Contains(j.Issues[0].Message, "each time from 57ff") {
		t.Errorf("one AP: %+v", j.Issues)
	}
	j = Journey{}
	Drops(&j, kettle[:3], 4)
	Drops(&j, []Drop{drop("57ff", 0, time.Minute), drop("57ff", 25, time.Minute), drop("57ff", 50, time.Minute), drop("57ff", 75, time.Minute)}, 4)
	if len(j.Issues) != 0 {
		t.Errorf("three, or four over more than an hour: %+v", j.Issues)
	}
}
