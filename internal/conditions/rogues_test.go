package conditions

import (
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A client heard on a rogue (0160) is kept once for each rogue and AP that
// heard it, from its first hearing to its last, with the most frames seen;
// it goes when the clients' attempts of its time do.
func TestRogueClients(t *testing.T) {
	s, clock := open(t)
	start := *clock
	heard := func(ap hierarchy.NodeID, mac string, frames int64) {
		t.Helper()
		must(t, s.RecordRogueClients(ap, []RogueClient{{MAC: mac, BSSID: "e4:d1:24:08:6d:11", SSID: "Home Wi-Fi", LastAt: *clock, Frames: frames}}))
	}
	heard("ap-1", "34:b7:da:64:a6:58", 100)
	*clock = clock.Add(time.Minute)
	heard("ap-1", "34:b7:da:64:a6:58", 90)
	heard("ap-2", "34:b7:da:64:a6:58", 95)
	*clock = clock.Add(time.Minute)
	heard("ap-1", "f0:ad:4e:46:a9:1f", 7)

	list, err := s.RogueClients(start, []hierarchy.NodeID{"ap-1"}, 10)
	must(t, err)
	if len(list) != 2 || list[0].MAC != "f0:ad:4e:46:a9:1f" {
		t.Fatalf("ap-1's = %+v", list)
	}
	if c := list[1]; c.MAC != "34:b7:da:64:a6:58" || !c.FirstAt.Equal(start) || !c.LastAt.Equal(start.Add(time.Minute)) || c.Frames != 100 || c.SSID != "Home Wi-Fi" {
		t.Fatalf("the plug = %+v", c)
	}
	if list, _ := s.RogueClients(start, []hierarchy.NodeID{"ap-1", "ap-2"}, 10); len(list) != 3 {
		t.Fatalf("both APs' = %+v", list)
	}
	if list, _ := s.RogueClients(start.Add(90*time.Second), []hierarchy.NodeID{"ap-1", "ap-2"}, 10); len(list) != 1 {
		t.Fatalf("since 90 s = %+v", list)
	}
	if list, _ := s.RogueClients(start, nil, 10); len(list) != 0 {
		t.Fatalf("no AP = %+v", list)
	}
	// A day on, kept a day: only what was heard in the last day stays.
	*clock = start.Add(24*time.Hour + 90*time.Second)
	_, err = s.TrimConnections(24 * time.Hour)
	must(t, err)
	if list, _ := s.RogueClients(start, []hierarchy.NodeID{"ap-1", "ap-2"}, 10); len(list) != 1 || list[0].MAC != "f0:ad:4e:46:a9:1f" {
		t.Fatalf("after the trim = %+v", list)
	}
}
