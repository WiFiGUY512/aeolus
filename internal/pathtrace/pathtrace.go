// Package pathtrace finds a client's way out (0168), a hop at a time:
// from the client to its AP, by the switch port the AP's own LLDP names, to
// the switch that routes for the client's network, and from there by each
// router's default route to the last thing the switches name before the
// internet. What it walks is what the switches themselves know: their LLDP
// neighbours, the MACs they have learned and on which port, their ARP and
// their routes. It asks nothing on the way, and sends nothing.
package pathtrace

import (
	"fmt"
	"strings"

	"github.com/WiFiGUY512/aeolus/internal/eapi"
)

// MaxHops is how many hops a way is followed for at most: a loop in what
// the switches say ends there.
const MaxHops = 16

// Hop is one thing on the way. Kind is client, ap, switch, device or
// internet. Vendor is who made it, by its MAC, where that says. A client's
// SSID, band and signal are its radio link's. In and Out are the ports the way comes in and leaves by, as
// the thing names them; Gateway, where this is the client's gateway, its
// interface there and address. By says how each fact was known: report
// (the AP's), lldp, mac (a switch's MAC table), arp, route.
type Hop struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Address string   `json:"address,omitempty"`
	MAC     string   `json:"mac,omitempty"`
	Vendor  string   `json:"vendor,omitempty"`
	Model   string   `json:"model,omitempty"`
	In      string   `json:"in,omitempty"`
	Out     string   `json:"out,omitempty"`
	VLAN    int      `json:"vlan,omitempty"`
	Gateway string   `json:"gateway,omitempty"`
	Seen    *bool    `json:"seen,omitempty"`
	SSID    string   `json:"ssid,omitempty"`
	Band    string   `json:"band,omitempty"`
	Signal  *int     `json:"signal,omitempty"`
	By      []string `json:"by,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// Client is the client, as its AP last reported it.
type Client struct {
	MAC     string
	Name    string
	Address string
	VLAN    int
	Band    string
	SSID    string
	Signal  *int
}

// AP is the AP the client is on, and what its uplink's LLDP says of the
// switch port it is on.
type AP struct {
	Name    string
	Address string
	Uplink  string // its own uplink port, as it names it
	Switch  string // the switch's LLDP system name
	Chassis string // the switch's chassis MAC, where its LLDP says one
	Port    string // the switch port
	Manage  []string
}

// Trace is a client's way out: its hops, in order, and why it stops where
// it does, where that is short of the internet.
type Trace struct {
	Hops []Hop  `json:"hops"`
	Ends string `json:"ends,omitempty"`
}

// Walk finds the way of c, on ap, over the switches read.
func Walk(c Client, ap AP, switches []*eapi.Switch) Trace {
	t := Trace{}
	add := func(h Hop) *Hop {
		t.Hops = append(t.Hops, h)
		return &t.Hops[len(t.Hops)-1]
	}
	// The switch that routes for the client's network. Where the AP did not
	// say the client's VLAN, as it says only one a key put it in (0070),
	// that switch's interface there says it: Vlan20 is VLAN 20.
	gw := gatewayFor(c, switches)
	if gw != nil && c.VLAN == 0 {
		c.VLAN = vlanOf(gw.Gateway(c.Address).Name)
	}
	add(Hop{Kind: "client", Name: c.Name, Address: c.Address, MAC: c.MAC, VLAN: c.VLAN, By: []string{"report"},
		SSID: c.SSID, Band: c.Band, Signal: c.Signal,
		Note: strings.TrimSpace(strings.Join(nonEmpty(c.SSID, bandName(c.Band), signal(c.Signal)), " · "))})
	add(Hop{Kind: "ap", Name: ap.Name, Address: ap.Address, Out: ap.Uplink, VLAN: c.VLAN, By: []string{"report"}})

	if ap.Switch == "" && ap.Chassis == "" && ap.Port == "" {
		t.Ends = "the AP's uplink hears no LLDP, so the switch it is on is not known"
		return t
	}
	sw := find(switches, ap.Switch, ap.Chassis, ap.Manage...)
	if sw == nil {
		add(Hop{Kind: "device", Name: orDash(ap.Switch, ap.Chassis), MAC: ap.Chassis, In: ap.Port, By: []string{"lldp"}})
		t.Ends = notRead(ap.Switch)
		return t
	}
	in := ap.Port
	// The switches the way has crossed on the client's VLAN, and those it
	// has been routed through: one switch can be both.
	visited, routed := map[string]bool{}, map[string]bool{}

	// Along the client's VLAN, to the switch that routes for it.
	var h *Hop
	for {
		if len(t.Hops) >= MaxHops {
			t.Ends = "the way runs past 16 hops: the switches' tables may loop"
			return t
		}
		visited[sw.Address] = true
		h = add(switchHop(sw, in, c.VLAN))
		if l, ok := sw.Where(c.MAC, c.VLAN); ok {
			seen := samePort(sw, l.Port, in)
			h.Seen = &seen
			by(h, "mac")
			if !seen {
				h.Note = fmt.Sprintf("it learned the client on %s", l.Port)
			}
		} else if c.MAC != "" {
			seen := false
			h.Seen = &seen
		}
		if gw != nil && (sw == gw || gw.MAC != "" && sw.MAC == gw.MAC) {
			i := sw.Gateway(c.Address)
			h.Gateway = fmt.Sprintf("%s %s", i.Name, i.Prefix.Addr())
			break
		}
		if gw == nil {
			t.Ends = fmt.Sprintf("no switch read has an address on the client's network (%s)", orDash(c.Address, ""))
			return t
		}
		l, ok := sw.Where(gw.MAC, c.VLAN)
		if !ok {
			t.Ends = fmt.Sprintf("%s has not learned where %s is on VLAN %d", sw.Hostname, gw.Hostname, c.VLAN)
			return t
		}
		h.Out = withChannel(sw, l.Port)
		by(h, "mac")
		next, nb, heard := across(sw, l.Port, switches)
		switch {
		case !heard:
			t.Ends = fmt.Sprintf("%s's LLDP names nothing on %s", sw.Hostname, l.Port)
			return t
		case next == nil:
			add(Hop{Kind: "device", Name: nb.Device, In: nb.DevicePort, VLAN: c.VLAN, By: []string{"lldp"}})
			t.Ends = notRead(nb.Device)
			return t
		case visited[next.Address]:
			t.Ends = fmt.Sprintf("the way comes back to %s: the switches' tables loop", next.Hostname)
			return t
		}
		sw, in = next, nb.DevicePort
	}

	// By each router's default route, to what lies beyond the last.
	routed[sw.Address] = true
	for {
		if len(t.Hops) >= MaxHops {
			t.Ends = "the way runs past 16 hops: the routes may loop"
			return t
		}
		if len(sw.Default) == 0 {
			t.Ends = fmt.Sprintf("%s has no default route", sw.Hostname)
			return t
		}
		via := sw.Default[0]
		by(h, "route")
		a, ok := sw.ARPOf(via.Address)
		port := a.Port
		if ok && port == "" {
			if l, ok := sw.Where(a.MAC, vlanOf(a.Interface)); ok {
				port = l.Port
			}
		}
		h.Out = strings.TrimSpace(strings.Join(nonEmpty(withChannel(sw, port), via.Interface), " · "))
		if h.Note == "" {
			h.Note = "default route via " + via.Address
		} else {
			h.Note += "; default route via " + via.Address
		}
		if !ok {
			add(Hop{Kind: "device", Name: via.Address, Address: via.Address, By: []string{"route"}})
			t.Ends = fmt.Sprintf("%s has no ARP for %s, its next hop", sw.Hostname, via.Address)
			return t
		}
		by(h, "arp")
		next, nb, heard := across(sw, port, switches)
		if next == nil {
			for _, s := range switches {
				if s.Owns(via.Address) {
					// Found by its address, not by LLDP: what LLDP said of
					// the port is another's, and no port of this one.
					next, nb = s, eapi.Neighbor{}
				}
			}
		}
		// A switch read that only carries the way to the next hop, on its
		// VLAN: followed by the next hop's MAC to the one that has it.
		vlan, carried := vlanOf(via.Interface), map[string]bool{}
		for next != nil && !next.Owns(via.Address) {
			if len(t.Hops) >= MaxHops || carried[next.Address] {
				t.Ends = fmt.Sprintf("the way to %s loops", via.Address)
				return t
			}
			carried[next.Address] = true
			x := add(switchHop(next, nb.DevicePort, vlan))
			l, ok := next.Where(a.MAC, vlan)
			if !ok {
				t.Ends = fmt.Sprintf("%s has not learned where %s is", next.Hostname, via.Address)
				return t
			}
			x.Out = withChannel(next, l.Port)
			by(x, "mac")
			from := next
			next, nb, heard = across(from, l.Port, switches)
			if !heard {
				nb = eapi.Neighbor{}
			}
		}
		if next != nil && routed[next.Address] {
			t.Ends = fmt.Sprintf("the way comes back to %s: the routes loop", next.Hostname)
			return t
		}
		if next == nil {
			// The last thing the switches name: past it, the internet,
			// which nothing here reads.
			last := add(Hop{Kind: "device", Name: orDash(nb.Device, via.Address), Address: via.Address, MAC: a.MAC, In: nb.DevicePort,
				By: nonEmpty("route", "arp", ifThen(nb.Device != "", "lldp"))})
			add(Hop{Kind: "internet", Name: "internet", Note: "past " + last.Name})
			return t
		}
		routed[next.Address] = true
		h = add(switchHop(next, nb.DevicePort, 0))
		h.Address = via.Address
		sw = next
	}
}

// gatewayFor is the switch that routes for c's network: of those read
// with an address on it, the one whose ARP has the client; then the one
// whose interface there is the client's VLAN's. A switch with an address
// of its own on the network, to be managed by, is not taken for it.
func gatewayFor(c Client, switches []*eapi.Switch) *eapi.Switch {
	var best *eapi.Switch
	score := -1
	for _, s := range switches {
		i := s.Gateway(c.Address)
		if i == nil {
			continue
		}
		n := 0
		if a, ok := s.ARPOf(c.Address); ok && (c.MAC == "" || a.MAC == c.MAC) {
			n += 2
		}
		if c.VLAN != 0 && strings.EqualFold(i.Name, vlanIf(c.VLAN)) {
			n++
		}
		if n > score {
			best, score = s, n
		}
	}
	return best
}

// by adds how a hop was known, once.
func by(h *Hop, how string) {
	for _, b := range h.By {
		if b == how {
			return
		}
	}
	h.By = append(h.By, how)
}

// across is what LLDP says is on the far end of port, whether it says
// anything, and the switch read it names, if any.
func across(sw *eapi.Switch, port string, switches []*eapi.Switch) (*eapi.Switch, eapi.Neighbor, bool) {
	nb, ok := sw.On(port)
	if !ok {
		return nil, nb, false
	}
	return find(switches, nb.Device, ""), nb, true
}

func switchHop(sw *eapi.Switch, in string, vlan int) Hop {
	return Hop{Kind: "switch", Name: orDash(sw.Hostname, sw.Address), Address: sw.Address, MAC: sw.MAC, Model: sw.Model,
		In: withChannel(sw, in), VLAN: vlan, By: []string{"lldp"}}
}

// find is the switch read that LLDP's name or chassis MAC, or one of the
// addresses its management TLVs say, is.
func find(switches []*eapi.Switch, name, chassis string, addrs ...string) *eapi.Switch {
	if m, ok := eapi.MAC(chassis); ok {
		for _, s := range switches {
			if s.MAC == m {
				return s
			}
		}
	}
	for _, s := range switches {
		if s.Names(name) {
			return s
		}
	}
	for _, a := range addrs {
		for _, s := range switches {
			if s.Address == a || s.Owns(a) {
				return s
			}
		}
	}
	return nil
}

// samePort says whether a switch learned something on port where it came in
// on in: the same port, or a port channel in is a member of.
func samePort(sw *eapi.Switch, port, in string) bool {
	return port == in || port != "" && sw.Channel(in) == port
}

// withChannel writes a port, with the port channel it is a member of.
func withChannel(sw *eapi.Switch, port string) string {
	if pc := sw.Channel(port); pc != "" {
		return port + " (" + pc + ")"
	}
	return port
}

func vlanIf(v int) string { return fmt.Sprintf("Vlan%d", v) }

func vlanOf(name string) int {
	var v int
	if _, err := fmt.Sscanf(name, "Vlan%d", &v); err == nil {
		return v
	}
	return 0
}

func notRead(name string) string {
	if name == "" {
		return "the switch the way goes to is not one the manager reads"
	}
	return fmt.Sprintf("%s is not one the manager reads: add its address in System, Switches the manager reads", name)
}

func bandName(b string) string {
	switch b {
	case "2g":
		return "2.4 GHz"
	case "5g":
		return "5 GHz"
	case "6g":
		return "6 GHz"
	}
	return b
}

func signal(s *int) string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("%d dBm", *s)
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func ifThen(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}

func orDash(a, b string) string {
	if a != "" {
		return a
	}
	if b != "" {
		return b
	}
	return "—"
}
