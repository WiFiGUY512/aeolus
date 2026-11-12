package mdns

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

// ask is a question for name's records of a kind, as a resolver sends it.
func ask(id uint16, name string, kind, class uint16) []byte {
	m := make([]byte, 12)
	binary.BigEndian.PutUint16(m, id)
	binary.BigEndian.PutUint16(m[4:], 1)
	m = append(m, wireName(name)...)
	m = binary.BigEndian.AppendUint16(m, kind)
	return binary.BigEndian.AppendUint16(m, class)
}

func TestLocal(t *testing.T) {
	for in, want := range map[string]string{
		"aeolus":        "aeolus.local",
		"Aeolus":        "aeolus.local",
		"aeolus.local":  "aeolus.local",
		"aeolus.local.": "aeolus.local",
		"wifi-1":        "wifi-1.local",
		"":              "",
		"a.b":           "",
		"a b":           "",
		"a_b":           "",
	} {
		if got := Local(in); got != want {
			t.Errorf("Local(%q) = %q, want %q", in, got, want)
		}
	}
}

// A multicast resolver's question is answered to the group, with the bit
// that replaces what is cached, no question and no ID.
func TestAnswersAMulticastResolver(t *testing.T) {
	a, ok := question(ask(0, "Aeolus.Local", typeA, classIN), "aeolus.local", false)
	if !ok || a.direct {
		t.Fatalf("question: ok %v, direct %v", ok, a.direct)
	}
	m := answer(a, "aeolus.local", net.IPv4(192, 168, 20, 60), false)
	want := append(append([]byte{0, 0, 0x84, 0, 0, 0, 0, 1, 0, 0, 0, 0}, wireName("aeolus.local")...),
		0, typeA, 0x80, 1, 0, 0, 0, ttlAsked, 0, 4, 192, 168, 20, 60)
	if !bytes.Equal(m, want) {
		t.Errorf("answer:\n got %x\nwant %x", m, want)
	}
}

// The bit that asks for the answer straight back is kept to.
func TestAnswersStraightBackWhereAsked(t *testing.T) {
	a, ok := question(ask(0, "aeolus.local", typeA, classIN|topBit), "aeolus.local", false)
	if !ok || !a.direct {
		t.Fatalf("question: ok %v, direct %v", ok, a.direct)
	}
}

// A resolver that is not a multicast one (the AP's finder asks from a port
// of its own) gets a plain DNS answer: its ID, its question, a short life.
func TestAnswersAPlainResolver(t *testing.T) {
	a, ok := question(ask(0x1234, "aeolus.local", typeA, classIN|topBit), "aeolus.local", true)
	if !ok || !a.direct {
		t.Fatalf("question: ok %v, direct %v", ok, a.direct)
	}
	m := answer(a, "aeolus.local", net.IPv4(10, 0, 0, 7), true)
	name := wireName("aeolus.local")
	want := []byte{0x12, 0x34, 0x84, 0, 0, 1, 0, 1, 0, 0, 0, 0}
	want = append(append(want, name...), 0, typeA, 0, 1)
	want = append(append(want, name...), 0, typeA, 0, 1, 0, 0, 0, ttlOne, 0, 4, 10, 0, 0, 7)
	if !bytes.Equal(m, want) {
		t.Errorf("answer:\n got %x\nwant %x", m, want)
	}
	// And the finder's own reading of it: the address, from a record whose
	// name is the one asked for.
	n, next, ok := readName(m, 12)
	if !ok || n != "aeolus.local" {
		t.Fatalf("the question's name: %q %v", n, ok)
	}
	n, next, ok = readName(m, next+4)
	if !ok || n != "aeolus.local" || !bytes.Equal(m[next+10:next+14], []byte{10, 0, 0, 7}) {
		t.Errorf("the answer's name %q, address %v", n, m[next+10:])
	}
}

func TestLeavesOtherQuestionsAlone(t *testing.T) {
	for what, m := range map[string][]byte{
		"another name":     ask(0, "printer.local", typeA, classIN),
		"its IPv6 address": ask(0, "aeolus.local", 28, classIN),
		"another class":    ask(0, "aeolus.local", typeA, 3),
		"a name it ends":   ask(0, "notaeolus.local", typeA, classIN),
		"too short":        {0, 0, 0, 0, 0, 1},
		"a name cut off":   ask(0, "aeolus.local", typeA, classIN)[:20],
		"an answer, not a question": func() []byte {
			m := ask(0, "aeolus.local", typeA, classIN)
			m[2] = 0x84
			return m
		}(),
	} {
		if _, ok := question(m, "aeolus.local", false); ok {
			t.Errorf("answered %s", what)
		}
	}
	// Anything of the name's is its address too.
	if _, ok := question(ask(0, "aeolus.local", typeANY, classIN), "aeolus.local", false); !ok {
		t.Error("did not answer a question for anything of the name's")
	}
	// A second question in one message is read.
	m := ask(0, "printer.local", typeA, classIN)
	binary.BigEndian.PutUint16(m[4:], 2)
	m = append(m, wireName("aeolus.local")...)
	m = append(m, 0, typeA, 0, classIN)
	if _, ok := question(m, "aeolus.local", false); !ok {
		t.Error("did not answer the second question of two")
	}
}

// A name may point back into the message; one that points in circles is
// given up on.
func TestReadsPointedNames(t *testing.T) {
	m := append(make([]byte, 12), wireName("aeolus.local")...) // at 12; "local" at 19
	at := len(m)
	m = append(m, 3, 'w', 'w', 'w', 0xc0, 19)
	n, next, ok := readName(m, at)
	if !ok || n != "www.local" || next != at+6 {
		t.Errorf("a pointed name: %q, next %d, ok %v", n, next, ok)
	}
	loop := append(make([]byte, 12), 0xc0, 12)
	if _, _, ok := readName(loop, 12); ok {
		t.Error("read a name that points at itself")
	}
}

// The address answered is this host's on the asker's network.
func TestAnswersWithTheAddressOnTheAskersNetwork(t *testing.T) {
	ifaces := []net.Interface{{Index: 1, Name: "eth0"}, {Index: 2, Name: "eth0.20"}}
	addrs := func(i net.Interface) ([]net.Addr, error) {
		if i.Index == 1 {
			return []net.Addr{&net.IPNet{IP: net.IPv4(192, 168, 1, 60), Mask: net.CIDRMask(24, 32)}}, nil
		}
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.IPv4(192, 168, 20, 60), Mask: net.CIDRMask(24, 32)},
		}, nil
	}
	ip, ifi := local(ifaces, addrs, net.IPv4(192, 168, 20, 7))
	if ip.String() != "192.168.20.60" || ifi.Name != "eth0.20" {
		t.Errorf("for an asker on the second network: %v on %v", ip, ifi)
	}
	if ip, _ := local(ifaces, addrs, net.IPv4(10, 9, 9, 9)); ip != nil {
		t.Errorf("answered an asker on none of its networks with %v", ip)
	}
}
