// Package eapi reads an Arista switch by its eAPI (0168): what it knows of
// its neighbours, the MACs it has learned and where, its addresses, its way
// out and its ARP, all with show commands a read-only account may run. It
// sets nothing on a switch.
package eapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Commands are what Read asks a switch, in one request.
var Commands = []string{
	"show hostname",
	"show version",
	"show lldp neighbors",
	"show mac address-table",
	"show ip interface brief",
	"show ip route 0.0.0.0/0",
	"show ip arp",
	"show port-channel dense",
}

// ciphers are what the switch client offers: Go's own, and after them the
// RSA key exchange EOS's eAPI offers unless it is given an SSL profile of
// its own (homelab and tower-switch offer AES256-SHA alone), which Go no
// longer offers unless asked (0168).
var ciphers = func() []uint16 {
	var out []uint16
	for _, c := range tls.CipherSuites() {
		out = append(out, c.ID)
	}
	return append(out,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_RSA_WITH_AES_256_CBC_SHA)
}()

// NewHTTP is a client for switches on the site's own network, whose eAPI
// certificates are their own, signed by no one (0168).
func NewHTTP(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		// A switch that sends the request elsewhere is not followed: the
		// password would go with it.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, CipherSuites: ciphers}, //nolint:gosec // a switch's own certificate (0168)
		},
	}
}

// Run sends cmds to the switch at host (a name or address, with :port where
// it is not 443) and returns each command's answer, in order.
func Run(ctx context.Context, hc *http.Client, host, user, password string, cmds []string) ([]json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "runCmds", "id": "aeolus",
		"params": map[string]any{"version": 1, "cmds": cmds, "format": "json"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host+"/command-api", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s is no address a switch is asked at", host)
	}
	req.SetBasicAuth(user, password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		var ne interface{ Timeout() bool }
		var de *net.DNSError
		var re tls.RecordHeaderError
		switch {
		case errors.As(err, &ne) && ne.Timeout():
			return nil, fmt.Errorf("%s did not answer in time", host)
		case errors.As(err, &de):
			return nil, fmt.Errorf("%s is no name the manager can find", host)
		case errors.As(err, &re), strings.Contains(err.Error(), "tls:"):
			return nil, fmt.Errorf("%s answers, but no TLS could be agreed with its eAPI: %v", host, err)
		case errors.Is(err, syscall.ECONNREFUSED), strings.Contains(err.Error(), "refused"):
			return nil, fmt.Errorf("%s refuses the connection: is its eAPI on (management api http-commands)?", host)
		}
		return nil, fmt.Errorf("%s could not be asked: its eAPI does not answer", host)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: its answer broke off", host)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("%s refuses the user or password", host)
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s has no eAPI turned on (management api http-commands)", host)
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil, fmt.Errorf("%s sends eAPI elsewhere (%d): give its address as eAPI answers at it", host, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s answered %d", host, resp.StatusCode)
	}
	var out struct {
		Result []json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s: its answer is not eAPI's", host)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%s: %s", host, out.Error.Message)
	}
	if len(out.Result) != len(cmds) {
		return nil, fmt.Errorf("%s answered %d of %d commands", host, len(out.Result), len(cmds))
	}
	return out.Result, nil
}

// Neighbor is what a switch's LLDP says is on one of its ports.
type Neighbor struct {
	Port       string `json:"port"`
	Device     string `json:"device"`
	DevicePort string `json:"device_port"`
}

// Learned is a MAC a switch has learned, on a VLAN, on a port.
type Learned struct {
	VLAN int    `json:"vlan"`
	MAC  string `json:"mac"`
	Port string `json:"port"`
}

// Interface is one of a switch's addresses: a VLAN's (an SVI) or a routed
// port's.
type Interface struct {
	Name   string       `json:"name"`
	Prefix netip.Prefix `json:"prefix"`
	Up     bool         `json:"up"`
}

// NextHop is where a route goes: an address, by an interface.
type NextHop struct {
	Address   string `json:"address"`
	Interface string `json:"interface"`
}

// ARP is an address a switch has the MAC of: by its interface, and the
// port behind it where that interface is a VLAN.
type ARP struct {
	Address   string `json:"address"`
	MAC       string `json:"mac"`
	Interface string `json:"interface"`
	Port      string `json:"port,omitempty"`
}

// Switch is all Read learned of one switch.
type Switch struct {
	Address    string              `json:"address"`
	Hostname   string              `json:"hostname"`
	FQDN       string              `json:"fqdn"`
	MAC        string              `json:"mac"`
	Model      string              `json:"model"`
	Neighbors  []Neighbor          `json:"neighbors"`
	Learned    []Learned           `json:"learned"`
	Interfaces []Interface         `json:"interfaces"`
	Default    []NextHop           `json:"default"`
	ARP        []ARP               `json:"arp"`
	Channels   map[string][]string `json:"channels"`
	At         time.Time           `json:"at"`
}

// Read asks the switch at host all of Commands, and makes a Switch of it.
func Read(ctx context.Context, hc *http.Client, host, user, password string) (*Switch, error) {
	res, err := Run(ctx, hc, host, user, password, Commands)
	if err != nil {
		return nil, err
	}
	sw, err := Parse(host, res)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", host, err)
	}
	sw.At = time.Now()
	return sw, nil
}

// Parse makes a Switch of the answers to Commands.
func Parse(host string, res []json.RawMessage) (*Switch, error) {
	if len(res) != len(Commands) {
		return nil, fmt.Errorf("%d answers to %d commands", len(res), len(Commands))
	}
	sw := &Switch{Address: host, Channels: map[string][]string{}}
	var name struct {
		Hostname string `json:"hostname"`
		FQDN     string `json:"fqdn"`
	}
	var ver struct {
		Model string `json:"modelName"`
		MAC   string `json:"systemMacAddress"`
	}
	var lldp struct {
		Neighbors []struct {
			Port       string `json:"port"`
			Device     string `json:"neighborDevice"`
			DevicePort string `json:"neighborPort"`
		} `json:"lldpNeighbors"`
	}
	var macs struct {
		Unicast struct {
			Entries []struct {
				VLAN int    `json:"vlanId"`
				MAC  string `json:"macAddress"`
				Port string `json:"interface"`
			} `json:"tableEntries"`
		} `json:"unicastTable"`
	}
	var ifs struct {
		Interfaces map[string]struct {
			Name     string `json:"name"`
			Protocol string `json:"lineProtocolStatus"`
			Address  struct {
				IP struct {
					Address string `json:"address"`
					MaskLen int    `json:"maskLen"`
				} `json:"ipAddr"`
			} `json:"interfaceAddress"`
		} `json:"interfaces"`
	}
	var route struct {
		VRFs map[string]struct {
			Routes map[string]struct {
				Vias []struct {
					Address   string `json:"nexthopAddr"`
					Interface string `json:"interface"`
				} `json:"vias"`
			} `json:"routes"`
		} `json:"vrfs"`
	}
	var arp struct {
		Neighbors []struct {
			Address   string `json:"address"`
			MAC       string `json:"hwAddress"`
			Interface string `json:"interface"`
		} `json:"ipV4Neighbors"`
	}
	var pcs struct {
		Channels map[string]struct {
			Ports map[string]json.RawMessage `json:"ports"`
		} `json:"portChannels"`
	}
	for i, v := range []any{&name, &ver, &lldp, &macs, &ifs, &route, &arp, &pcs} {
		if err := json.Unmarshal(res[i], v); err != nil {
			return nil, fmt.Errorf("%q answered with what is not understood", Commands[i])
		}
	}
	sw.Hostname, sw.FQDN, sw.Model = name.Hostname, name.FQDN, ver.Model
	sw.MAC, _ = MAC(ver.MAC)
	for _, n := range lldp.Neighbors {
		sw.Neighbors = append(sw.Neighbors, Neighbor{Port: n.Port, Device: n.Device, DevicePort: n.DevicePort})
	}
	for _, e := range macs.Unicast.Entries {
		if m, ok := MAC(e.MAC); ok {
			sw.Learned = append(sw.Learned, Learned{VLAN: e.VLAN, MAC: m, Port: e.Port})
		}
	}
	for key, i := range ifs.Interfaces {
		a, err := netip.ParseAddr(i.Address.IP.Address)
		if err != nil || a.IsUnspecified() {
			continue
		}
		// The address itself, with its network's length: 192.168.20.1/24.
		p := netip.PrefixFrom(a, i.Address.IP.MaskLen)
		if !p.IsValid() {
			continue
		}
		n := i.Name
		if n == "" {
			n = key
		}
		sw.Interfaces = append(sw.Interfaces, Interface{Name: n, Prefix: p, Up: i.Protocol == "up"})
	}
	sort.Slice(sw.Interfaces, func(a, b int) bool { return sw.Interfaces[a].Name < sw.Interfaces[b].Name })
	if d, ok := route.VRFs["default"]; ok {
		for _, v := range d.Routes["0.0.0.0/0"].Vias {
			sw.Default = append(sw.Default, NextHop{Address: v.Address, Interface: v.Interface})
		}
	}
	for _, n := range arp.Neighbors {
		m, ok := MAC(n.MAC)
		if !ok {
			continue
		}
		// "Vlan1010, Ethernet28": the VLAN, and the port behind it.
		// "Vlan20, not learned" where the switch has not learned the MAC.
		in, port, _ := strings.Cut(n.Interface, ",")
		if port = strings.TrimSpace(port); port == "not learned" {
			port = ""
		}
		sw.ARP = append(sw.ARP, ARP{Address: n.Address, MAC: m, Interface: strings.TrimSpace(in), Port: port})
	}
	for pc, c := range pcs.Channels {
		for p := range c.Ports {
			sw.Channels[pc] = append(sw.Channels[pc], p)
		}
		sort.Strings(sw.Channels[pc])
	}
	return sw, nil
}

// MAC writes a MAC as Linux does, aa:bb:cc:00:11:22, from that or from
// Cisco's and Arista's aabb.cc00.1122.
func MAC(s string) (string, bool) {
	h := strings.ToLower(strings.NewReplacer(":", "", ".", "", "-", "").Replace(s))
	if len(h) != 12 || strings.Trim(h, "0123456789abcdef") != "" {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(h[i : i+2])
	}
	return b.String(), true
}

// Names says whether name is this switch's: its host name or its full
// name, as LLDP or a person writes it.
func (sw *Switch) Names(name string) bool {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if name == "" {
		return false
	}
	return name == strings.ToLower(sw.Hostname) || name == strings.ToLower(sw.FQDN) || name == strings.ToLower(sw.Address)
}

// Owns says whether addr is one of the switch's own addresses.
func (sw *Switch) Owns(addr string) bool {
	a, err := netip.ParseAddr(addr)
	if err != nil {
		return false
	}
	for _, i := range sw.Interfaces {
		if i.Prefix.Addr() == a {
			return true
		}
	}
	return false
}

// Gateway is the switch's interface whose network addr is on: the
// gateway of a client with that address, where the switch routes for it.
func (sw *Switch) Gateway(addr string) *Interface {
	a, err := netip.ParseAddr(addr)
	if err != nil {
		return nil
	}
	var best *Interface
	for i := range sw.Interfaces {
		x := &sw.Interfaces[i]
		if x.Prefix.Masked().Contains(a) && (best == nil || x.Prefix.Bits() > best.Prefix.Bits()) {
			best = x
		}
	}
	return best
}

// Where is the port the switch learned mac on, on vlan (any VLAN where
// vlan is 0).
func (sw *Switch) Where(mac string, vlan int) (Learned, bool) {
	for _, l := range sw.Learned {
		if l.MAC == mac && (vlan == 0 || l.VLAN == vlan) {
			return l, true
		}
	}
	return Learned{}, false
}

// On is what LLDP says is on port: on it, or, for a port channel, on the
// first of its members that says.
func (sw *Switch) On(port string) (Neighbor, bool) {
	for _, n := range sw.Neighbors {
		if n.Port == port {
			return n, true
		}
	}
	for _, m := range sw.Channels[port] {
		for _, n := range sw.Neighbors {
			if n.Port == m {
				return n, true
			}
		}
	}
	return Neighbor{}, false
}

// Channel is the port channel port is a member of, if any.
func (sw *Switch) Channel(port string) string {
	for pc, ms := range sw.Channels {
		for _, m := range ms {
			if m == port {
				return pc
			}
		}
	}
	return ""
}

// ARPOf is what the switch's ARP has of addr.
func (sw *Switch) ARPOf(addr string) (ARP, bool) {
	for _, a := range sw.ARP {
		if a.Address == addr {
			return a, true
		}
	}
	return ARP{}, false
}
