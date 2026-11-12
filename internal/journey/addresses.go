package journey

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// Addresses set by hand, and addresses two clients hold (0325). Griff,
// 2026-11-05: a static address "is not really a problem provided all things
// are proper"; one that is not right for the VLAN "would tell us why that
// client is not working". And a client making an address conflict.

// Subnets says the subnets a network has on an AP, as the APs' client tests
// were given them by DHCP there, or on any AP where none was there; none
// where none is known.
type Subnets func(ap hierarchy.NodeID, network string) []netip.Prefix

// Holder is another client that holds an address now: its MAC, what it
// calls itself, and the AP it is on.
type Holder struct {
	MAC    string `json:"mac"`
	Host   string `json:"host,omitempty"`
	AP     string `json:"ap"`
	APName string `json:"ap_name"`
}

// Addresses weighs each address the client set itself (its "static" note):
// one on its network's subnet where it was is as it should be, and the note
// is let go; one off it is a warning, as it reaches nothing there; one where
// no subnet is known keeps its note. holders are the other clients that hold
// the address it holds now, held: each a warning, at now.
func Addresses(j *Journey, subnets Subnets, held string, holders []Holder, now time.Time) {
	if subnets != nil {
		out := j.Issues[:0]
		for _, is := range j.Issues {
			if is.Kind != "static" {
				out = append(out, is)
				continue
			}
			i := slices.IndexFunc(j.Sessions, func(s Session) bool { return s.From.Equal(is.At) && s.DHCP == "static" })
			if i < 0 {
				out = append(out, is)
				continue
			}
			s := j.Sessions[i]
			a, err := netip.ParseAddr(s.Address)
			nets := subnets(s.AP, s.Network)
			if err != nil || len(nets) == 0 {
				out = append(out, is)
				continue
			}
			if slices.ContainsFunc(nets, func(p netip.Prefix) bool { return p.Contains(a) }) {
				continue
			}
			each := make([]string, len(nets))
			for k, p := range nets {
				each[k] = p.String()
			}
			out = append(out, Issue{Severity: "warning", Kind: "static-off-subnet", At: is.At,
				Message: fmt.Sprintf("on %s it uses %s without asking DHCP, off %s's subnet there (%s): set by hand for another network, it reaches nothing",
					s.Name, s.Address, called(s), strings.Join(each, ", "))})
		}
		j.Issues = out
	}
	for _, h := range holders {
		who := h.MAC
		if h.Host != "" {
			who = fmt.Sprintf("%s (%s)", h.Host, h.MAC)
		}
		j.Issues = append(j.Issues, Issue{Severity: "warning", Kind: "address-conflict", At: now,
			Message: fmt.Sprintf("it holds %s, and so does %s on %s now: two clients with one address each lose traffic. One was set by hand, or a DHCP server gave out an address in use",
				held, who, h.APName)})
	}
}

// called is a session's network as people know it: its SSID, else its ID.
func called(s Session) string {
	if s.SSID != "" {
		return s.SSID
	}
	return s.Network
}
