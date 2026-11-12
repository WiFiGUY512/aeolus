package api

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A capture read in the page (0147): each frame of it in a line, in words,
// so that a failure is looked into where it is listed, and Wireshark is for
// what the words leave out.

// capturedFrame is one frame of a capture: how long after its attempt
// began it came, in ms, before it where that is less than nothing; which
// side of the AP it crossed, the radio's or the wire's; who sent it and who
// it was for; what it is, in words; how strongly the AP heard it; whether
// it was one the AP sent and the client did not acknowledge; and what the
// AP noted of it.
type capturedFrame struct {
	AfterMS float64 `json:"after_ms"`
	Side    string  `json:"side"`
	From    string  `json:"from,omitempty"`
	To      string  `json:"to,omitempty"`
	What    string  `json:"what"`
	Signal  *int    `json:"signal,omitempty"`
	Failed  bool    `json:"failed,omitempty"`
	Note    string  `json:"note,omitempty"`
	Bytes   int     `json:"bytes"`
}

// pcapng's link types, as an agent's capture has them, and the option that
// is a frame's comment.
const (
	linkRadio   = 127
	linkWire    = 1
	pcapComment = 1
)

// readCapture is a capture's frames, each in words, timed from when its
// attempt began. The file is one the manager held to an agent's shape when
// it took it (pcapFrames): what does not read is passed over, and never
// refused here.
func readCapture(b []byte, started time.Time) []capturedFrame {
	out := []capturedFrame{}
	var links []uint16
	le := binary.LittleEndian
	for at := 0; len(b)-at >= 12; {
		kind, n := le.Uint32(b[at:]), int(le.Uint32(b[at+4:]))
		if n < 12 || n%4 != 0 || n > len(b)-at {
			break
		}
		body := b[at+8 : at+n-4]
		at += n
		switch kind {
		case pcapInterface:
			if len(body) >= 2 {
				links = append(links, le.Uint16(body))
			}
		case pcapFrame:
			if len(body) < 20 {
				continue
			}
			which, size := int(le.Uint32(body)), int(le.Uint32(body[12:]))
			if which >= len(links) || size > len(body)-20 {
				continue
			}
			us := int64(le.Uint32(body[4:]))<<32 | int64(le.Uint32(body[8:]))
			f := capturedFrame{AfterMS: float64(us-started.UnixMicro()) / 1000, Bytes: size}
			data := body[20 : 20+size]
			if links[which] == linkRadio {
				f.Side = "radio"
				readRadio(data, &f)
			} else {
				f.Side = "wire"
				readWire(data, &f)
			}
			// What follows the frame, made up to four bytes: its options.
			for o := body[20+(size+3)&^3:]; len(o) >= 4; {
				code, on := le.Uint16(o), int(le.Uint16(o[2:]))
				if code == 0 || on > len(o)-4 {
					break
				}
				if code == pcapComment {
					f.Note = shown(string(o[4:4+on]), 200)
				}
				o = o[4+(on+3)&^3:]
			}
			out = append(out, f)
		}
	}
	return out
}

// shown is text from a frame as a page may show it: what prints, and no
// more than n of it.
func shown(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= n {
			break
		}
		if r < ' ' || r > '~' {
			r = '?'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func macOf(b []byte) string { return net.HardwareAddr(b).String() }

// radiotapParts is how long each of a radiotap header's first sixteen parts
// is, and the boundary it sits on: its time, flags, rate, channel, hopping,
// the signal and the noise in dBm, and the rest as far as what is said of a
// frame sent.
var radiotapParts = [16][2]int{{8, 8}, {1, 1}, {1, 1}, {4, 2}, {2, 2}, {1, 1}, {1, 1}, {2, 2}, {2, 2}, {2, 2}, {1, 1}, {1, 1}, {1, 1}, {1, 1}, {2, 2}, {2, 2}}

// Reasons a client left or was told to, by number: those met most.
var leaveReasons = map[uint16]string{
	1: "no reason given", 2: "its authentication no longer holds", 3: "leaving", 4: "idle too long",
	5: "the AP has too many clients", 6: "it is not authenticated", 7: "it is not associated", 8: "leaving the network",
	14: "a key message failed its check", 15: "the key handshake timed out", 16: "the group key handshake timed out",
	23: "its sign-in failed", 34: "too many frames went unacknowledged",
}

var mgmtNames = map[byte]string{
	0: "Association request", 1: "Association response", 2: "Reassociation request", 3: "Reassociation response",
	4: "Probe request", 5: "Probe response", 8: "Beacon", 10: "Disassociation", 11: "Authentication", 12: "Deauthentication", 13: "Action",
}

// readRadio reads a frame from the radio's side: its radiotap header, for
// how strongly it was heard and whether it failed, then the 802.11 frame.
func readRadio(d []byte, f *capturedFrame) {
	le := binary.LittleEndian
	f.What = "a frame too short to read"
	if len(d) < 8 {
		return
	}
	rt := int(le.Uint16(d[2:]))
	if rt < 8 || rt > len(d) {
		return
	}
	// What the header has is said in a word, or several; its parts follow
	// in their order, each on its boundary.
	has, at := le.Uint32(d[4:]), 8
	for w := has; w&(1<<31) != 0 && at+4 <= rt; at += 4 {
		w = le.Uint32(d[at:])
	}
	for bit := 0; bit < 16; bit++ {
		if has&(1<<bit) == 0 {
			continue
		}
		size, edge := radiotapParts[bit][0], radiotapParts[bit][1]
		at = (at + edge - 1) / edge * edge
		if at+size > rt {
			break
		}
		switch bit {
		case 5:
			s := int(int8(d[at]))
			if s <= 0 && s >= -127 {
				f.Signal = &s
			}
		case 15:
			f.Failed = le.Uint16(d[at:])&1 != 0
		}
		at += size
	}
	m := d[rt:]
	if len(m) < 16 {
		return
	}
	f.To, f.From = macOf(m[4:10]), macOf(m[10:16])
	kind, sub := m[0]>>2&3, m[0]>>4
	body := []byte{}
	if len(m) >= 24 {
		body = m[24:]
	}
	u16 := func(at int) uint16 {
		if len(body) < at+2 {
			return 0
		}
		return le.Uint16(body[at:])
	}
	status := func(code uint16) string {
		if code == 0 {
			return "ok"
		}
		return "refused, status " + strconv.Itoa(int(code))
	}
	switch kind {
	case 0:
		name, ok := mgmtNames[sub]
		if !ok {
			name = fmt.Sprintf("Management frame, subtype %d", sub)
		}
		switch sub {
		case 4:
			if len(body) >= 2 && body[0] == 0 && int(body[1]) <= len(body)-2 && body[1] > 0 {
				name += ` for "` + shown(string(body[2:2+body[1]]), 32) + `"`
			} else {
				name += " for any network"
			}
		case 11:
			alg, seq := u16(0), u16(2)
			switch {
			case alg == 3:
				name = "Authentication, SAE " + map[bool]string{true: "commit", false: "confirm"}[seq == 1]
				if st := u16(4); st != 0 && st != 76 && st != 126 {
					name += ": " + status(st)
				}
			case seq%2 == 1:
				name = "Authentication request"
				if alg == 2 {
					name += ", by a fast transition (802.11r)"
				}
			default:
				name = "Authentication response: " + status(u16(4))
			}
		case 1, 3:
			name += ": " + status(u16(2))
			if u16(2) == 0 {
				name += fmt.Sprintf(", AID %d", u16(4)&0x3fff)
			}
		case 2:
			if len(body) >= 10 {
				name += ", coming from " + macOf(body[4:10])
			}
		case 10, 12:
			r := u16(0)
			name += fmt.Sprintf(", reason %d", r)
			if w := leaveReasons[r]; w != "" {
				name += " (" + w + ")"
			}
		case 13:
			if len(body) >= 2 {
				name += fmt.Sprintf(", category %d, action %d", body[0], body[1])
			}
		}
		f.What = name
	case 2:
		// A data frame: one that carries a key message or a sign-in's, in
		// the clear, says which.
		hdr := 24
		if m[0]&0x80 != 0 {
			hdr += 2
		}
		if len(m) > 1 && m[1]&3 == 3 {
			hdr += 6
		}
		f.What = "Data frame"
		if len(m) > 1 && m[1]&0x40 != 0 {
			f.What = "Data frame, encrypted"
			return
		}
		if len(m) >= hdr+8 && string(m[hdr:hdr+6]) == "\xaa\xaa\x03\x00\x00\x00" && m[hdr+6] == 0x88 && m[hdr+7] == 0x8e {
			f.What = eapolWords(m[hdr+8:])
		}
	default:
		f.What = fmt.Sprintf("Frame of type %d, subtype %d", kind, sub)
	}
}

// eapolWords is a key message or a sign-in's, in words.
func eapolWords(e []byte) string {
	if len(e) < 4 {
		return "EAPOL, too short to read"
	}
	switch e[1] {
	case 3:
		if len(e) < 7 {
			return "Key message, too short to read"
		}
		info := binary.BigEndian.Uint16(e[5:])
		first, mic, secure := info&0x80 != 0, info&0x100 != 0, info&0x200 != 0
		if info&0x08 == 0 {
			return "Group key message " + map[bool]string{true: "1", false: "2"}[first] + " of 2"
		}
		n := 2
		switch {
		case first && !mic:
			n = 1
		case first:
			n = 3
		case secure:
			n = 4
		}
		return fmt.Sprintf("Key message %d of 4", n)
	case 0:
		if len(e) < 5 {
			return "EAP message"
		}
		names := []string{"EAP message", "EAP request", "EAP response", "EAP success", "EAP failure"}
		if int(e[4]) < len(names) {
			return names[e[4]]
		}
		return "EAP message"
	case 1:
		return "EAPOL-Start: the client asks to sign in"
	}
	return fmt.Sprintf("EAPOL, type %d", e[1])
}

var dhcpNames = map[byte]string{1: "Discover", 2: "Offer", 3: "Request", 4: "Decline", 5: "ACK", 6: "NAK", 7: "Release", 8: "Inform"}

// readWire reads a frame from the wire side: Ethernet, and of what it
// carries the things a client's coming online is made of: ARP, DHCP, DNS,
// the opening of a connection.
func readWire(d []byte, f *capturedFrame) {
	be := binary.BigEndian
	f.What = "a frame too short to read"
	if len(d) < 14 {
		return
	}
	f.To, f.From = macOf(d[0:6]), macOf(d[6:12])
	kind, p, vlan := be.Uint16(d[12:]), d[14:], ""
	if kind == 0x8100 && len(p) >= 4 {
		vlan = fmt.Sprintf(", VLAN %d", be.Uint16(p)&0x0fff)
		kind, p = be.Uint16(p[2:]), p[4:]
	}
	defer func() { f.What += vlan }()
	switch kind {
	case 0x0806:
		f.What = "ARP"
		if len(p) >= 28 {
			who, told := net.IP(p[14:18]), net.IP(p[24:28])
			switch be.Uint16(p[6:]) {
			case 1:
				f.What = fmt.Sprintf("ARP: who has %s? asked by %s", told, who)
				if who.Equal(told) || who.IsUnspecified() {
					f.What = fmt.Sprintf("ARP: is anyone at %s?", told)
				}
			case 2:
				f.What = fmt.Sprintf("ARP: %s is at %s", who, macOf(p[8:14]))
			}
		}
	case 0x0800:
		if len(p) < 20 {
			f.What = "IPv4, too short to read"
			return
		}
		ihl := int(p[0]&15) * 4
		src, dst := net.IP(p[12:16]), net.IP(p[16:20])
		if ihl < 20 || len(p) < ihl {
			f.What = "IPv4, too short to read"
			return
		}
		f.What = ipWords(p[9], p[ihl:], src.String(), dst.String(), be.Uint16(p[6:])&0x1fff != 0)
	case 0x86dd:
		if len(p) < 40 {
			f.What = "IPv6, too short to read"
			return
		}
		f.What = ipWords(p[6], p[40:], net.IP(p[8:24]).String(), net.IP(p[24:40]).String(), false)
	case 0x888e:
		f.What = eapolWords(p)
	default:
		f.What = fmt.Sprintf("Ethernet frame, type 0x%04x", kind)
	}
}

// ipWords is what an IP packet carries, in words: its protocol, t the
// bytes after its header, and where it is from and for.
func ipWords(proto byte, t []byte, src, dst string, later bool) string {
	be := binary.BigEndian
	if later {
		return fmt.Sprintf("A later piece of a packet, %s to %s", src, dst)
	}
	switch proto {
	case 17:
		if len(t) < 8 {
			break
		}
		sp, dp, u := be.Uint16(t), be.Uint16(t[2:]), t[8:]
		switch {
		case (sp == 67 || sp == 68) && (dp == 67 || dp == 68):
			return dhcpWords(u)
		case sp == 53 || dp == 53:
			return dnsWords(u, sp == 53, src, dst)
		case sp == 546 || sp == 547:
			return fmt.Sprintf("DHCPv6, %s to %s", src, dst)
		}
		return fmt.Sprintf("UDP %s:%d to %s:%d", src, sp, dst, dp)
	case 6:
		if len(t) < 14 {
			break
		}
		sp, dp, flags := be.Uint16(t), be.Uint16(t[2:]), t[13]
		what := "TCP"
		switch {
		case flags&0x04 != 0:
			what = "TCP reset"
		case flags&0x12 == 0x12:
			what = "TCP connection accepted (SYN, ACK)"
		case flags&0x02 != 0:
			what = "TCP connection asked for (SYN)"
		case flags&0x01 != 0:
			what = "TCP connection closed (FIN)"
		}
		return fmt.Sprintf("%s, %s:%d to %s:%d", what, src, sp, dst, dp)
	case 1:
		if len(t) >= 1 {
			switch t[0] {
			case 8:
				return fmt.Sprintf("Ping, %s to %s", src, dst)
			case 0:
				return fmt.Sprintf("Ping answered, %s to %s", src, dst)
			case 3:
				return fmt.Sprintf("ICMP: cannot be reached, said by %s to %s", src, dst)
			}
		}
		return fmt.Sprintf("ICMP, %s to %s", src, dst)
	case 58:
		if len(t) >= 1 {
			names := map[byte]string{128: "Ping", 129: "Ping answered", 133: "Router solicitation", 134: "Router advertisement", 135: "Neighbour solicitation", 136: "Neighbour advertisement"}
			if n := names[t[0]]; n != "" {
				return fmt.Sprintf("%s (IPv6), %s to %s", n, src, dst)
			}
		}
		return fmt.Sprintf("ICMPv6, %s to %s", src, dst)
	}
	return fmt.Sprintf("IP protocol %d, %s to %s", proto, src, dst)
}

// dhcpWords is a DHCP message: which of them, and the address it offers or
// gives.
func dhcpWords(u []byte) string {
	if len(u) < 240 {
		return "DHCP"
	}
	what, yours := "DHCP", net.IP(u[16:20])
	for o := u[240:]; len(o) >= 2 && o[0] != 255; {
		if o[0] == 0 {
			o = o[1:]
			continue
		}
		n := int(o[1])
		if n > len(o)-2 {
			break
		}
		if o[0] == 53 && n == 1 {
			if name := dhcpNames[o[2]]; name != "" {
				what = "DHCP " + name
				if (o[2] == 2 || o[2] == 5) && !yours.IsUnspecified() {
					what += ": " + yours.String()
				}
			}
		}
		o = o[2+n:]
	}
	return what
}

// dnsWords is a DNS message: the name asked for, and for an answer whether
// it had one.
func dnsWords(u []byte, answer bool, src, dst string) string {
	if len(u) < 12 {
		return "DNS"
	}
	var labels []string
	for at := 12; at < len(u) && u[at] != 0 && len(labels) < 32; {
		n := int(u[at])
		if n > 63 || at+1+n > len(u) {
			break
		}
		labels = append(labels, shown(string(u[at+1:at+1+n]), 63))
		at += 1 + n
	}
	name := strings.Join(labels, ".")
	if name == "" {
		name = "a name"
	}
	if !answer {
		return fmt.Sprintf("DNS: %s asks %s for %s", src, dst, name)
	}
	switch code := u[3] & 15; {
	case code == 3:
		return fmt.Sprintf("DNS: %s says there is no %s", src, name)
	case code != 0:
		return fmt.Sprintf("DNS: %s could not answer for %s (code %d)", src, name, code)
	case binary.BigEndian.Uint16(u[6:]) == 0:
		return fmt.Sprintf("DNS: %s answers for %s, with nothing", src, name)
	}
	return fmt.Sprintf("DNS: %s answers for %s", src, name)
}

// connectionFrames is the capture kept of one attempt (0135), read for the
// page (0147): GET /v1/connections/{id}/capture/frames, each frame of it in
// words, timed from the attempt's start. Whoever may view the attempt's AP
// may read it, as they may the file.
func (s *Server) connectionFrames(w http.ResponseWriter, r *http.Request, c call) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return errNotFound
	}
	cn, k, ok, err := s.conds.CaptureOf(id)
	if err != nil {
		return err
	}
	if !ok || !may(c, change.Locations, c.state.Org.Locations, cn.AP, access.Capture) {
		return errNotFound
	}
	if k == nil {
		return &apiError{http.StatusNotFound, "no capture is kept of this attempt"}
	}
	// Timed from the attempt's start as its AP said it: a capture's times
	// are the AP's own.
	frames := readCapture(k.File, time.UnixMilli(cn.Said))
	writeJSON(w, http.StatusOK, map[string]any{
		"id": cn.ID, "mac": cn.MAC, "started": cn.Started, "radio": k.Radio, "wire": k.Wire, "bytes": len(k.File), "frames": frames,
	})
	return nil
}

// placeStep is one folder on the way down to an AP.
type placeStep struct {
	ID   hierarchy.NodeID `json:"id"`
	Name string           `json:"name"`
}

// placeOf is where an AP is in a tree: the folders it is in, the outermost
// first, with the tree's root left out, as it is the same for every AP.
func placeOf(t *hierarchy.Tree, ap hierarchy.NodeID) []placeStep {
	out := []placeStep{}
	n, ok := t.Node(ap)
	for ok && n.Parent != "" && n.Parent != t.Root() {
		if n, ok = t.Node(n.Parent); ok {
			out = append([]placeStep{{ID: n.ID, Name: n.Name}}, out...)
		}
	}
	return out
}
