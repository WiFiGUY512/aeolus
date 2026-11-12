package pathtrace

import (
	"encoding/json"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/eapi"
)

func read(t *testing.T, name, addr string) *eapi.Switch {
	t.Helper()
	raw, err := os.ReadFile("../eapi/testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var res []json.RawMessage
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	sw, err := eapi.Parse(addr, res)
	if err != nil {
		t.Fatal(err)
	}
	return sw
}

// said writes a way as hops kind:name[in>out], to compare.
func said(tr Trace) string {
	var parts []string
	for _, h := range tr.Hops {
		s := h.Kind + ":" + h.Name
		if h.In != "" || h.Out != "" {
			s += "[" + h.In + ">" + h.Out + "]"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

var (
	phone = Client{MAC: "aa:bb:cc:00:11:22", Name: "a phone", Address: "192.168.20.151", VLAN: 20, Band: "5g", SSID: "Home_Wi-Fi", Signal: ptr(-52)}
	unit1 = AP{Name: "C360-AP", Address: "192.168.20.7", Uplink: "eth1", Switch: "homelab.example.com", Chassis: "28:e7:1d:d2:d0:13", Port: "Ethernet13"}
)

func ptr(n int) *int { return &n }

func TestWalkToTheInternet(t *testing.T) {
	homelab, tower := read(t, "homelab", "192.168.1.1"), read(t, "tower-switch", "10.0.1.1")
	tr := Walk(phone, unit1, []*eapi.Switch{tower, homelab})
	want := "client:a phone ap:C360-AP[>eth1] switch:homelab[Ethernet13 (Port-Channel2)>Ethernet28 · Vlan1010] " +
		"switch:tower-switch[Ethernet18>Ethernet1 · Vlan2] device:firewall[igb1>] internet:internet"
	if got := said(tr); got != want || tr.Ends != "" {
		t.Fatalf("way =\n %s\nwant\n %s\nends %q", got, want, tr.Ends)
	}
	c, gw, next, last := tr.Hops[0], tr.Hops[2], tr.Hops[3], tr.Hops[4]
	if c.Note != "Home_Wi-Fi · 5 GHz · -52 dBm" || c.Address != "192.168.20.151" || c.VLAN != 20 {
		t.Errorf("client = %+v", c)
	}
	if gw.Gateway != "Vlan20 192.168.20.1" || gw.Seen == nil || !*gw.Seen || gw.VLAN != 20 || gw.Note != "default route via 10.0.1.250" {
		t.Errorf("gateway = %+v", gw)
	}
	if strings.Join(gw.By, ",") != "lldp,mac,route,arp" {
		t.Errorf("gateway known by %v", gw.By)
	}
	if next.Address != "10.0.1.250" || next.Gateway != "" || next.Note != "default route via 10.0.0.1" {
		t.Errorf("next router = %+v", next)
	}
	if last.Address != "10.0.0.1" || last.MAC != "00:0d:b9:aa:bb:cc" {
		t.Errorf("last = %+v", last)
	}
	if tr.Hops[5].Note != "past firewall" {
		t.Errorf("internet = %+v", tr.Hops[5])
	}
}

func TestWalkToASwitchNotRead(t *testing.T) {
	homelab := read(t, "homelab", "192.168.1.1")
	tr := Walk(phone, unit1, []*eapi.Switch{homelab})
	want := "client:a phone ap:C360-AP[>eth1] switch:homelab[Ethernet13 (Port-Channel2)>Ethernet28 · Vlan1010] device:tower-switch.example.com[Ethernet18>] internet:internet"
	if got := said(tr); got != want {
		t.Fatalf("way =\n %s\nwant\n %s", got, want)
	}

	// None read: the AP's switch, as its LLDP names it, and why it stops.
	tr = Walk(phone, unit1, nil)
	if got := said(tr); got != "client:a phone ap:C360-AP[>eth1] device:homelab.example.com[Ethernet13>]" ||
		!strings.Contains(tr.Ends, "homelab.example.com is not one the manager reads") {
		t.Fatalf("none read: %s, %q", got, tr.Ends)
	}

	// An uplink that hears no LLDP.
	tr = Walk(phone, AP{Name: "C360-AP", Uplink: "eth1"}, []*eapi.Switch{homelab})
	if len(tr.Hops) != 2 || !strings.Contains(tr.Ends, "hears no LLDP") {
		t.Fatalf("no LLDP: %s, %q", said(tr), tr.Ends)
	}
}

func TestWalkFindsTheSwitchByName(t *testing.T) {
	homelab := read(t, "homelab", "192.168.1.1")
	ap := unit1
	ap.Chassis = ""
	ap.Switch = "homelab"
	if tr := Walk(phone, ap, []*eapi.Switch{homelab}); len(tr.Hops) < 3 || tr.Hops[2].Name != "homelab" {
		t.Errorf("by name: %s", said(tr))
	}
	// Or by the management address its LLDP says.
	ap.Switch = "unnamed"
	ap.Manage = []string{"192.168.1.1"}
	if tr := Walk(phone, ap, []*eapi.Switch{homelab}); len(tr.Hops) < 3 || tr.Hops[2].Name != "homelab" {
		t.Errorf("by address: %s", said(tr))
	}
}

func TestWalkSaysWhereTheClientWasLearned(t *testing.T) {
	homelab := read(t, "homelab", "192.168.1.1")
	// The living-room unit's client, on VLAN 10, but the switch learned it
	// on Ethernet17 while the AP says Ethernet13.
	c := Client{MAC: "aa:bb:cc:00:33:44", Address: "192.168.10.40", VLAN: 10}
	tr := Walk(c, unit1, []*eapi.Switch{homelab})
	h := tr.Hops[2]
	if h.Seen == nil || *h.Seen || h.Note != "it learned the client on Ethernet17; default route via 10.0.1.250" || h.Gateway != "Vlan10 192.168.10.1" {
		t.Errorf("hop = %+v", h)
	}
	// One it has not learned at all.
	c.MAC = "aa:bb:cc:99:99:99"
	if h := Walk(c, unit1, []*eapi.Switch{homelab}).Hops[2]; h.Seen == nil || *h.Seen {
		t.Errorf("not learned: %+v", h)
	}
}

func TestWalkAlongAVLANToItsGateway(t *testing.T) {
	homelab, tower := read(t, "homelab", "192.168.1.1"), read(t, "tower-switch", "10.0.1.1")
	// An AP on the tower switch, whose client's VLAN 20 is routed by
	// homelab: the tower switch learned homelab on Ethernet18.
	tower.Learned = append(tower.Learned, eapi.Learned{VLAN: 20, MAC: homelab.MAC, Port: "Ethernet18"}, eapi.Learned{VLAN: 20, MAC: phone.MAC, Port: "Ethernet5"})
	ap := AP{Name: "Shed", Uplink: "lan1", Switch: "tower-switch.example.com", Port: "Ethernet5"}
	tr := Walk(phone, ap, []*eapi.Switch{homelab, tower})
	want := "client:a phone ap:Shed[>lan1] switch:tower-switch[Ethernet5>Ethernet18] switch:homelab[Ethernet28>Ethernet28 · Vlan1010]"
	if got := said(tr); !strings.HasPrefix(got, want) {
		t.Fatalf("way =\n %s\nwant\n %s…", got, want)
	}
	if tr.Hops[3].Gateway != "Vlan20 192.168.20.1" || tr.Hops[2].Gateway != "" {
		t.Errorf("gateway on %+v", tr.Hops[3])
	}
	// Past homelab its default route goes back to the tower switch: crossed
	// on VLAN 20, and now routed through, on to the firewall.
	if got := said(tr); !strings.HasSuffix(got, "switch:tower-switch[Ethernet18>Ethernet1 · Vlan2] device:firewall[igb1>] internet:internet") {
		t.Errorf("way %s", got)
	}

	// Where the tower switch has not learned homelab on the VLAN.
	tower.Learned = tower.Learned[:len(tower.Learned)-2]
	tr = Walk(phone, ap, []*eapi.Switch{homelab, tower})
	if !strings.Contains(tr.Ends, "tower-switch has not learned where homelab is on VLAN 20") {
		t.Errorf("ends %q", tr.Ends)
	}
}

func TestWalkStopsWhereNoSwitchRoutes(t *testing.T) {
	homelab := read(t, "homelab", "192.168.1.1")
	c := phone
	c.Address = "172.16.5.5"
	tr := Walk(c, unit1, []*eapi.Switch{homelab})
	if !strings.Contains(tr.Ends, "no switch read has an address on the client's network (172.16.5.5)") || len(tr.Hops) != 3 {
		t.Errorf("%s, %q", said(tr), tr.Ends)
	}
	homelab.Default = nil
	if tr := Walk(phone, unit1, []*eapi.Switch{homelab}); tr.Ends != "homelab has no default route" {
		t.Errorf("no default route: %q", tr.Ends)
	}
	homelab = read(t, "homelab", "192.168.1.1")
	homelab.ARP = nil
	if tr := Walk(phone, unit1, []*eapi.Switch{homelab}); tr.Ends != "homelab has no ARP for 10.0.1.250, its next hop" {
		t.Errorf("no ARP: %q", tr.Ends)
	}
}

func TestWalkEndsOnALoop(t *testing.T) {
	a, b := read(t, "homelab", "192.168.1.1"), read(t, "tower-switch", "10.0.1.1")
	// The tower switch routes back to homelab, and homelab to it.
	b.Default = []eapi.NextHop{{Address: "10.0.1.249", Interface: "Vlan1010"}}
	tr := Walk(phone, unit1, []*eapi.Switch{a, b})
	if tr.Ends != "the way comes back to homelab: the routes loop" || tr.Hops[len(tr.Hops)-1].Name != "tower-switch" {
		t.Errorf("a loop: %s, %q", said(tr), tr.Ends)
	}
}

func TestWalkTakesTheVLANFromTheGateway(t *testing.T) {
	homelab := read(t, "homelab", "192.168.1.1")
	c := phone
	c.VLAN = 0
	tr := Walk(c, unit1, []*eapi.Switch{homelab})
	if h := tr.Hops[2]; tr.Hops[0].VLAN != 20 || h.VLAN != 20 || h.Seen == nil || !*h.Seen {
		t.Errorf("hops = %+v", tr.Hops[:3])
	}
}

func TestWalkPassesASwitchManagedOnTheClientsNetwork(t *testing.T) {
	homelab, tower := read(t, "homelab", "192.168.1.1"), read(t, "tower-switch", "10.0.1.1")
	// The tower switch has an address of its own on VLAN 20, to be managed
	// by; homelab routes for it, and its ARP has the phone.
	tower.Interfaces = append(tower.Interfaces, eapi.Interface{Name: "Vlan20", Prefix: netip.MustParsePrefix("192.168.20.5/24"), Up: true})
	tower.Learned = append(tower.Learned, eapi.Learned{VLAN: 20, MAC: homelab.MAC, Port: "Ethernet18"}, eapi.Learned{VLAN: 20, MAC: phone.MAC, Port: "Ethernet5"})
	ap := AP{Name: "Shed", Uplink: "lan1", Switch: "tower-switch.example.com", Port: "Ethernet5"}
	tr := Walk(phone, ap, []*eapi.Switch{tower, homelab})
	if tr.Hops[2].Name != "tower-switch" || tr.Hops[2].Gateway != "" || tr.Hops[3].Name != "homelab" || tr.Hops[3].Gateway != "Vlan20 192.168.20.1" {
		t.Errorf("way %s; gateways %q %q", said(tr), tr.Hops[2].Gateway, tr.Hops[3].Gateway)
	}
}

func TestWalkCarriedToTheNextHop(t *testing.T) {
	homelab, tower := read(t, "homelab", "192.168.1.1"), read(t, "tower-switch", "10.0.1.1")
	// homelab's next hop is the firewall itself, on VLAN 1010, which the
	// tower switch only carries.
	homelab.ARP[0].MAC = "00:0d:b9:aa:bb:cc"
	tower.Interfaces = tower.Interfaces[1:]
	tower.Learned = append(tower.Learned, eapi.Learned{VLAN: 1010, MAC: "00:0d:b9:aa:bb:cc", Port: "Ethernet1"})
	tr := Walk(phone, unit1, []*eapi.Switch{homelab, tower})
	want := "switch:homelab[Ethernet13 (Port-Channel2)>Ethernet28 · Vlan1010] switch:tower-switch[Ethernet18>Ethernet1] device:firewall[igb1>] internet:internet"
	if got := said(tr); !strings.HasSuffix(got, want) {
		t.Fatalf("way =\n %s\nwant …\n %s", got, want)
	}
	if h := tr.Hops[3]; h.VLAN != 1010 || h.Address != "10.0.1.1" || strings.Join(h.By, ",") != "lldp,mac" {
		t.Errorf("carrying hop = %+v", h)
	}
	if d := tr.Hops[4]; d.Address != "10.0.1.250" || d.MAC != "00:0d:b9:aa:bb:cc" {
		t.Errorf("next hop = %+v", d)
	}
	// One that has not learned it.
	tower.Learned = tower.Learned[:len(tower.Learned)-1]
	if tr := Walk(phone, unit1, []*eapi.Switch{homelab, tower}); tr.Ends != "tower-switch has not learned where 10.0.1.250 is" {
		t.Errorf("ends %q", tr.Ends)
	}
}

func TestWalkFindsARouterByItsAddress(t *testing.T) {
	homelab, tower := read(t, "homelab", "192.168.1.1"), read(t, "tower-switch", "10.0.1.1")
	// A media converter between them, which LLDP names and nothing reads:
	// the tower switch is found by owning the next hop, and no port of the
	// converter's is said to be its.
	for i, n := range homelab.Neighbors {
		if n.Port == "Ethernet28" {
			homelab.Neighbors[i] = eapi.Neighbor{Port: "Ethernet28", Device: "converter", DevicePort: "fiber2"}
		}
	}
	tr := Walk(phone, unit1, []*eapi.Switch{homelab, tower})
	if h := tr.Hops[3]; h.Name != "tower-switch" || h.In != "" || h.Address != "10.0.1.250" {
		t.Errorf("found by its address: %+v (%s)", h, said(tr))
	}
	if c := tr.Hops[0]; c.Band != "5g" || c.SSID != "Home_Wi-Fi" || c.Signal == nil || *c.Signal != -52 {
		t.Errorf("the client's radio link: %+v", c)
	}
}
