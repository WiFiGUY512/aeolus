package journey

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// TestAddresses: an address set by hand on its network's subnet there is
// let be; one off it is a warning naming the subnet; one where no subnet is
// known keeps its note; another client holding the address it holds now is
// a warning each (0325).
func TestAddresses(t *testing.T) {
	j := Build("aa:bb:cc:00:11:22", []Sample{
		// On the office AP: 192.168.20.218 on 192.168.20.0/24, as it should be.
		sample("office", 0, "5g", -55, Client{Address: "192.168.20.218", DHCP: "static"}),
		// Gone an hour, on the C-360: 192.168.5.7, off the subnet.
		sample("c360", 70, "5g", -55, Client{Address: "192.168.5.7", DHCP: "static"}),
		// Gone an hour, on the guest network nothing tested: no subnet known.
		sample("c360", 140, "2g", -55, Client{Address: "10.9.9.9", DHCP: "static", Network: "guest", SSID: "Guest"}),
	})
	static := 0
	for _, is := range j.Issues {
		if is.Kind == "static" {
			static++
		}
	}
	if static != 3 {
		t.Fatalf("before: %d static notes, want 3: %+v", static, j.Issues)
	}
	lab := []netip.Prefix{netip.MustParsePrefix("192.168.20.0/24")}
	subnets := func(ap hierarchy.NodeID, network string) []netip.Prefix {
		if network == "lab" {
			return lab
		}
		return nil
	}
	holders := []Holder{{MAC: "aa:bb:cc:00:11:99", Host: "printer", AP: "ap-office", APName: "office"}}
	Addresses(&j, subnets, "10.9.9.9", holders, t0.Add(150*time.Minute))
	kinds := map[string][]Issue{}
	for _, is := range j.Issues {
		kinds[is.Kind] = append(kinds[is.Kind], is)
	}
	if n := len(kinds["static"]); n != 1 || !strings.Contains(kinds["static"][0].Message, "10.9.9.9") {
		t.Fatalf("static notes = %+v, want the guest one alone", kinds["static"])
	}
	off := kinds["static-off-subnet"]
	if len(off) != 1 || off[0].Severity != "warning" || !strings.Contains(off[0].Message, "192.168.5.7") || !strings.Contains(off[0].Message, "192.168.20.0/24") ||
		!strings.Contains(off[0].Message, "Aeolus Lab") || !strings.Contains(off[0].Message, "on c360") {
		t.Fatalf("off the subnet = %+v", off)
	}
	conflict := kinds["address-conflict"]
	if len(conflict) != 1 || conflict[0].Severity != "warning" || !strings.Contains(conflict[0].Message, "printer (aa:bb:cc:00:11:99) on office") ||
		!strings.Contains(conflict[0].Message, "10.9.9.9") {
		t.Fatalf("conflict = %+v", conflict)
	}
	// With nothing known of subnets or holders, all is as Build had it.
	k := Build("aa:bb:cc:00:11:22", []Sample{sample("office", 0, "5g", -55, Client{Address: "192.168.20.218", DHCP: "static"})})
	before := len(k.Issues)
	Addresses(&k, nil, "", nil, t0)
	if len(k.Issues) != before {
		t.Fatalf("with nothing known: %+v", k.Issues)
	}
}
