package api

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
)

// A capture as aeolus-journey writes one (0146): its two interfaces, then
// frames, each with which interface it is of, its time in microseconds, its
// bytes, and a note where it has one.
type testFrame struct {
	link int
	us   int64
	data []byte
	note string
}

func pcapWith(frames ...testFrame) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	pad := func(p []byte) []byte {
		for len(p)%4 != 0 {
			p = append(p, 0)
		}
		return p
	}
	block := func(kind uint32, body []byte) {
		n := uint32(12 + len(body))
		binary.Write(&b, le, kind)
		binary.Write(&b, le, n)
		b.Write(body)
		binary.Write(&b, le, n)
	}
	u16 := func(v uint16) []byte { return le.AppendUint16(nil, v) }
	u32 := func(v uint32) []byte { return le.AppendUint32(nil, v) }
	block(pcapSection, append(append(u32(pcapOrder), 1, 0, 0, 0), bytes.Repeat([]byte{0xff}, 8)...))
	block(pcapInterface, append(append(u16(linkRadio), 0, 0), u32(2048)...))
	block(pcapInterface, append(append(u16(linkWire), 0, 0), u32(2048)...))
	for _, f := range frames {
		body := append(u32(uint32(f.link)), u32(uint32(f.us>>32))...)
		body = append(body, u32(uint32(f.us))...)
		body = append(body, u32(uint32(len(f.data)))...)
		body = append(body, u32(uint32(len(f.data)))...)
		body = append(body, pad(append([]byte{}, f.data...))...)
		if f.note != "" {
			body = append(body, u16(pcapComment)...)
			body = append(body, u16(uint16(len(f.note)))...)
			body = append(body, pad([]byte(f.note))...)
			body = append(body, 0, 0, 0, 0)
		}
		block(pcapFrame, body)
	}
	return b.Bytes()
}

// A capture's frames are read into words (0147): what each is, who it was
// from and for, how strongly it was heard, whether the client acknowledged
// one the AP sent, and how long after the attempt began it came.
func TestCaptureReadIntoWords(t *testing.T) {
	ap, sta, all := []byte{0x32, 0x86, 0x2d, 0xef, 0x2f, 0xc0}, []byte{0x30, 0x86, 0x2d, 0xef, 0x2f, 0xf0}, bytes.Repeat([]byte{0xff}, 6)
	cat := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	// The headers aeolus-journey makes: for a frame heard, the channel and
	// its strength; for one sent, the channel and whether it failed.
	heard := func(dbm int8) []byte { return []byte{0, 0, 15, 0, 0x2a, 0, 0, 0, 0, 0, 0x9e, 0x09, 0x80, 0, byte(dbm)} }
	sent := func(failed byte) []byte {
		return []byte{0, 0, 16, 0, 0x0a, 0x80, 0, 0, 0, 0, 0x9e, 0x09, 0x80, 0, failed, 0}
	}
	// A key message heard has the channel alone.
	plain := []byte{0, 0, 14, 0, 0x0a, 0, 0, 0, 0, 0, 0x9e, 0x09, 0x80, 0}
	// One a monitor interface made (0135): a time, flags, a rate, the
	// channel, the signal, an antenna, and more of them in a second word.
	old := []byte{0, 0, 26, 0, 0x2f, 0x48, 0, 0x80, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0x0c, 0x64, 0x14, 0x40, 0x01}
	old = append(old[:8], append([]byte{0, 0, 0, 0}, old[8:]...)...)
	old[2] = byte(len(old) + 1)
	old = append(old, 0xc8)
	dot11 := func(fc0, fc1 byte, a1, a2, a3 []byte) []byte {
		return cat([]byte{fc0, fc1, 0, 0}, a1, a2, a3, []byte{0, 0})
	}
	key := func(hi, lo byte) []byte { return cat([]byte{2, 3, 0, 95, 2, hi, lo, 0, 16}, make([]byte, 90)) }
	llc := []byte{0xaa, 0xaa, 0x03, 0, 0, 0, 0x88, 0x8e}
	eth := func(dst, src []byte, kind uint16, p []byte) []byte {
		return cat(dst, src, []byte{byte(kind >> 8), byte(kind)}, p)
	}
	ip := func(proto byte, src, dst [4]byte, p []byte) []byte {
		return cat([]byte{0x45, 0, 0, 0, 0, 0, 0, 0, 64, proto, 0, 0}, src[:], dst[:], p)
	}
	udp := func(sp, dp uint16, p []byte) []byte {
		return cat([]byte{byte(sp >> 8), byte(sp), byte(dp >> 8), byte(dp), 0, 0, 0, 0}, p)
	}
	dhcp := func(kind byte, yours [4]byte) []byte {
		m := make([]byte, 240)
		copy(m[16:], yours[:])
		return append(m, 53, 1, kind, 255)
	}
	dns := func(answer bool, code byte, answers byte) []byte {
		flags := []byte{0x01, 0}
		if answer {
			flags = []byte{0x81, 0x80 | code}
		}
		return cat([]byte{0x12, 0x34}, flags, []byte{0, 1, 0, answers, 0, 0, 0, 0, 7}, []byte("example"), []byte{3}, []byte("com"), []byte{0, 0, 1, 0, 1})
	}
	gw, me, none := [4]byte{192, 168, 10, 1}, [4]byte{192, 168, 10, 50}, [4]byte{}
	everyone := [4]byte{255, 255, 255, 255}
	arp := func(op byte, shw []byte, sip [4]byte, tip [4]byte) []byte {
		return cat([]byte{0, 1, 8, 0, 6, 4, 0, op}, shw, sip[:], make([]byte, 6), tip[:])
	}
	start := time.UnixMilli(1791744505000)
	at := func(ms float64) int64 { return start.UnixMicro() + int64(ms*1000) }
	file := pcapWith(
		testFrame{0, at(-4200), cat(heard(-61), dot11(0x40, 0, all, sta, all), []byte{0, 5}, []byte("Sweet"), []byte{1, 0}), ""},
		testFrame{0, at(-4199), cat(heard(-61), dot11(0x40, 0, all, sta, all), []byte{0, 0}), ""},
		testFrame{0, at(0), cat(heard(-48), dot11(0xb0, 0, ap, sta, ap), []byte{0, 0, 1, 0, 0, 0}), ""},
		testFrame{0, at(1.5), cat(sent(0), dot11(0xb0, 0, sta, ap, ap), []byte{0, 0, 2, 0, 0, 0}), ""},
		testFrame{0, at(7), cat(heard(-49), dot11(0x20, 0, ap, sta, ap), []byte{0x31, 4, 10, 0}, []byte{0x72, 0xbb, 0x66, 0x9f, 0x76, 0x94}), ""},
		testFrame{0, at(10), cat(sent(0), dot11(0x30, 0, sta, ap, ap), []byte{0x31, 4, 0, 0, 5, 0xc0}), ""},
		testFrame{0, at(110), cat(sent(0), dot11(0x08, 0x02, sta, ap, ap), llc, key(0x00, 0x8a)), ""},
		testFrame{0, at(114), cat(plain, dot11(0x08, 0x01, ap, sta, ap), llc, key(0x01, 0x0a)), ""},
		testFrame{0, at(114.5), cat(sent(1), dot11(0x08, 0x02, sta, ap, ap), llc, key(0x13, 0xca)), "The client did not acknowledge this."},
		testFrame{0, at(1114.5), cat(sent(0), dot11(0x88, 0x42, sta, ap, ap), []byte{0, 0}, make([]byte, 40)), ""},
		testFrame{0, at(4000), cat(sent(1), dot11(0xc0, 0, sta, ap, ap), []byte{15, 0}), "The client did not acknowledge this."},
		testFrame{0, at(4001), cat(sent(0), dot11(0x10, 0, sta, ap, ap), []byte{0x31, 4, 17, 0, 0, 0}), ""},
		testFrame{0, at(4002), cat(old, dot11(0xa0, 0, ap, sta, ap), []byte{8, 0}), ""},
		testFrame{0, at(4003), cat(heard(-50), dot11(0xb0, 0, ap, sta, ap), []byte{3, 0, 1, 0, 0, 0}, make([]byte, 96)), ""},
		testFrame{0, at(4004), cat(heard(-50), dot11(0xd0, 0, ap, sta, ap), []byte{10, 8, 1, 0}), ""},
		testFrame{0, at(4005), []byte{0, 0, 9}, ""},
		testFrame{1, at(300), eth(all, sta, 0x0800, ip(17, none, everyone, udp(68, 67, dhcp(1, none)))), ""},
		testFrame{1, at(302), eth(sta, ap, 0x0800, ip(17, gw, me, udp(67, 68, dhcp(2, me)))), ""},
		testFrame{1, at(305), eth(sta, ap, 0x0800, ip(17, gw, me, udp(67, 68, dhcp(5, me)))), ""},
		testFrame{1, at(900), eth(all, sta, 0x0806, arp(1, sta, me, gw)), ""},
		testFrame{1, at(901), eth(sta, ap, 0x0806, arp(2, ap, gw, me)), ""},
		testFrame{1, at(902), eth(all, sta, 0x0806, arp(1, sta, me, me)), ""},
		testFrame{1, at(950), eth(ap, sta, 0x0800, ip(17, me, gw, udp(40000, 53, dns(false, 0, 0)))), ""},
		testFrame{1, at(953), eth(sta, ap, 0x0800, ip(17, gw, me, udp(53, 40000, dns(true, 0, 1)))), ""},
		testFrame{1, at(954), eth(sta, ap, 0x0800, ip(17, gw, me, udp(53, 40000, dns(true, 3, 0)))), ""},
		testFrame{1, at(960), eth(ap, sta, 0x0800, ip(6, me, [4]byte{1, 1, 1, 1}, []byte{0x9c, 0x40, 0x01, 0xbb, 0, 0, 0, 0, 0, 0, 0, 0, 0x50, 0x02, 0, 0})), ""},
		testFrame{1, at(970), eth(ap, sta, 0x0800, ip(1, me, gw, []byte{8, 0, 0, 0})), ""},
		testFrame{1, at(980), eth(ap, sta, 0x8100, cat([]byte{0, 10, 0x08, 0x06}, arp(1, sta, me, gw))), ""},
		testFrame{1, at(990), eth(ap, sta, 0x86dd, cat([]byte{0x60, 0, 0, 0, 0, 8, 58, 255}, make([]byte, 32), []byte{133, 0, 0, 0})), ""},
		testFrame{1, at(995), eth(ap, sta, 0x88cc, nil), ""},
	)
	if n, err := pcapFrames(file); err != nil || n != 30 {
		t.Fatalf("the file, as an agent's is held to: %d frames, %v", n, err)
	}
	var got []string
	for _, f := range readCapture(file, start) {
		s := fmt.Sprintf("%+.1f %s %s", f.AfterMS, f.Side, f.What)
		if f.Signal != nil {
			s += fmt.Sprintf(" [%d dBm]", *f.Signal)
		}
		if f.Failed {
			s += " [failed]"
		}
		if f.Note != "" {
			s += " [" + f.Note + "]"
		}
		got = append(got, s)
	}
	want := []string{
		`-4200.0 radio Probe request for "Sweet" [-61 dBm]`,
		`-4199.0 radio Probe request for any network [-61 dBm]`,
		`+0.0 radio Authentication request [-48 dBm]`,
		`+1.5 radio Authentication response: ok`,
		`+7.0 radio Reassociation request, coming from 72:bb:66:9f:76:94 [-49 dBm]`,
		`+10.0 radio Reassociation response: ok, AID 5`,
		`+110.0 radio Key message 1 of 4`,
		`+114.0 radio Key message 2 of 4`,
		`+114.5 radio Key message 3 of 4 [failed] [The client did not acknowledge this.]`,
		`+1114.5 radio Data frame, encrypted`,
		`+4000.0 radio Deauthentication, reason 15 (the key handshake timed out) [failed] [The client did not acknowledge this.]`,
		`+4001.0 radio Association response: refused, status 17`,
		`+4002.0 radio Disassociation, reason 8 (leaving the network) [-56 dBm]`,
		`+4003.0 radio Authentication, SAE commit [-50 dBm]`,
		`+4004.0 radio Action, category 10, action 8 [-50 dBm]`,
		`+4005.0 radio a frame too short to read`,
		`+300.0 wire DHCP Discover`,
		`+302.0 wire DHCP Offer: 192.168.10.50`,
		`+305.0 wire DHCP ACK: 192.168.10.50`,
		`+900.0 wire ARP: who has 192.168.10.1? asked by 192.168.10.50`,
		`+901.0 wire ARP: 192.168.10.1 is at 32:86:2d:ef:2f:c0`,
		`+902.0 wire ARP: is anyone at 192.168.10.50?`,
		`+950.0 wire DNS: 192.168.10.50 asks 192.168.10.1 for example.com`,
		`+953.0 wire DNS: 192.168.10.1 answers for example.com`,
		`+954.0 wire DNS: 192.168.10.1 says there is no example.com`,
		`+960.0 wire TCP connection asked for (SYN), 192.168.10.50:40000 to 1.1.1.1:443`,
		`+970.0 wire Ping, 192.168.10.50 to 192.168.10.1`,
		`+980.0 wire ARP: who has 192.168.10.1? asked by 192.168.10.50, VLAN 10`,
		`+990.0 wire Router solicitation (IPv6), :: to ::`,
		`+995.0 wire Ethernet frame, type 0x88cc`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the frames, read:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Who each was from and for.
	if f := readCapture(file, start)[2]; f.From != "30:86:2d:ef:2f:f0" || f.To != "32:86:2d:ef:2f:c0" || f.Bytes != 45 {
		t.Errorf("the authentication request: %+v", f)
	}
	// A file cut short gives what it holds whole, and never fails.
	for cut := 0; cut < len(file); cut += 7 {
		readCapture(file[:cut], start)
	}
}

// Where an AP is in Locations: the folders it is in, the outermost first,
// the Org itself left out.
func TestPlaceOfAnAP(t *testing.T) {
	tree := hierarchy.NewTree("org", "Org", true)
	must(t, tree.AddFolder("house", "Home", "org"))
	must(t, tree.AddFolder("office", "Office", "house"))
	must(t, tree.AddAP("ap-1", "OfficeAP", "office"))
	must(t, tree.AddAP("ap-2", "Loose", "org"))
	said := func(ap hierarchy.NodeID) string {
		var names []string
		for _, p := range placeOf(tree, ap) {
			names = append(names, string(p.ID)+"="+p.Name)
		}
		return strings.Join(names, " > ")
	}
	if got := said("ap-1"); got != "house=Home > office=Office" {
		t.Errorf("an AP two folders down: %q", got)
	}
	if got := said("ap-2"); got != "" {
		t.Errorf("an AP in the Org itself: %q", got)
	}
	if got := placeOf(tree, "none"); got == nil || len(got) != 0 {
		t.Errorf("no such AP: %v", got)
	}
}
