// Package mdns answers multicast DNS for one name under .local with this
// host's own address (0151), so that an AP with no DNS to ask finds the
// manager as aeolus.local on its network.
//
// It is the least of a responder (RFC 6762): it answers a question for the
// name's IPv4 address, or for anything of the name's, with the address this
// host has on the network the question came from. It does not first ask
// whether another host has the name, announces nothing unasked, and holds
// no other records.
package mdns

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"strings"
	"time"
)

const (
	port     = 5353
	typeA    = 1
	typeANY  = 255
	classIN  = 1
	topBit   = 0x8000 // of a question's class: answer straight back; of a record's: replace what is cached
	ttlAsked = 120    // seconds an answer to the group may be kept
	ttlOne   = 10     // seconds one sent straight back to a resolver that is not a multicast one may be
)

var group = net.IPv4(224, 0, 0, 251)

// Local is the name under .local that a label makes: aeolus is
// aeolus.local. It is empty where the label cannot be one.
func Local(label string) string {
	label = strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(label, "."), ".local"))
	if label == "" || len(label) > 63 || strings.ContainsAny(label, ". \t") {
		return ""
	}
	for _, c := range label {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return ""
		}
	}
	return label + ".local"
}

// asked is what a question for the name asks of a responder.
type asked struct {
	id     uint16
	direct bool   // the answer is wanted straight back, and not to the group
	echo   []byte // the question itself, to send back with the answer
}

// readName reads the name at a place in a message, lower case, and where
// what follows it begins. A name may end in a pointer back into the
// message.
func readName(m []byte, at int) (string, int, bool) {
	var labels []string
	next, hops := -1, 0
	for at < len(m) {
		n := int(m[at])
		switch {
		case n == 0:
			if next < 0 {
				next = at + 1
			}
			return strings.ToLower(strings.Join(labels, ".")), next, true
		case n&0xc0 == 0xc0:
			if at+1 >= len(m) || hops > 16 {
				return "", 0, false
			}
			hops++
			if next < 0 {
				next = at + 2
			}
			at = (n&0x3f)<<8 | int(m[at+1])
		case n > 63 || at+1+n > len(m):
			return "", 0, false
		default:
			labels = append(labels, string(m[at+1:at+1+n]))
			at += 1 + n
		}
	}
	return "", 0, false
}

// wireName is a name as a message holds it.
func wireName(name string) []byte {
	var out []byte
	for _, l := range strings.Split(name, ".") {
		if l == "" {
			continue
		}
		out = append(out, byte(len(l)))
		out = append(out, l...)
	}
	return append(out, 0)
}

// question reads a message for a question this responder answers: one for
// name's IPv4 address, or for anything of name's. legacy says the asker is
// not a multicast resolver (it asked from a port that is not 5353), which
// wants its question sent back and its answer straight to it.
func question(m []byte, name string, legacy bool) (asked, bool) {
	if len(m) < 12 || m[2]&0x80 != 0 || m[2]&0x78 != 0 {
		return asked{}, false // an answer, or not a plain question
	}
	count := int(binary.BigEndian.Uint16(m[4:]))
	at := 12
	for i := 0; i < count; i++ {
		n, next, ok := readName(m, at)
		if !ok || next+4 > len(m) {
			return asked{}, false
		}
		kind := binary.BigEndian.Uint16(m[next:])
		class := binary.BigEndian.Uint16(m[next+2:])
		if n == name && (kind == typeA || kind == typeANY) && class&^topBit == classIN {
			a := asked{id: binary.BigEndian.Uint16(m), direct: legacy || class&topBit != 0}
			if legacy {
				a.echo = append(append(wireName(name), 0, byte(kind)), 0, classIN)
			}
			return a, true
		}
		at = next + 4
	}
	return asked{}, false
}

// answer is the message that says name is at ip.
func answer(a asked, name string, ip net.IP, legacy bool) []byte {
	m := make([]byte, 12)
	class, ttl := uint16(classIN|topBit), uint32(ttlAsked)
	if legacy {
		// As a plain DNS answer: the asker's ID and question, no bit a
		// plain resolver would not know, and not to be kept long.
		binary.BigEndian.PutUint16(m, a.id)
		binary.BigEndian.PutUint16(m[4:], 1)
		class, ttl = classIN, ttlOne
	}
	binary.BigEndian.PutUint16(m[2:], 0x8400) // an answer, and this host's to give
	binary.BigEndian.PutUint16(m[6:], 1)
	m = append(m, a.echo...)
	m = append(m, wireName(name)...)
	m = binary.BigEndian.AppendUint16(m, typeA)
	m = binary.BigEndian.AppendUint16(m, class)
	m = binary.BigEndian.AppendUint32(m, ttl)
	m = binary.BigEndian.AppendUint16(m, 4)
	return append(m, ip.To4()...)
}

// local is this host's address on the network an asker is on, and the
// interface it is on; nil where the asker is on none of this host's
// networks, which multicast DNS does not answer.
func local(ifaces []net.Interface, addrs func(net.Interface) ([]net.Addr, error), from net.IP) (net.IP, *net.Interface) {
	for i := range ifaces {
		list, err := addrs(ifaces[i])
		if err != nil {
			continue
		}
		for _, a := range list {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			if n.Contains(from) {
				return n.IP.To4(), &ifaces[i]
			}
		}
	}
	return nil, nil
}

// Serve answers for name (a name under .local, as Local gives) until ctx
// ends. It returns at once where it cannot listen; a network that comes
// later is joined as it is found.
func Serve(ctx context.Context, log *slog.Logger, name string) error {
	if Local(name) != name {
		return errors.New("mdns: not a name under .local: " + name)
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, &net.UDPAddr{IP: group, Port: port})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// The group is joined on every network this host is on, and again as
	// they come and go: a minute is soon enough for one that is new.
	joined := map[string]bool{}
	join := func() {
		ifaces, err := net.Interfaces()
		if err != nil {
			return
		}
		for _, ifi := range ifaces {
			if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := ifi.Addrs()
			for _, a := range addrs {
				n, ok := a.(*net.IPNet)
				if !ok || n.IP.To4() == nil {
					continue
				}
				key := ifi.Name + "/" + n.IP.String()
				if joined[key] {
					continue
				}
				if err := joinGroup(conn, group, n.IP.To4()); err == nil {
					joined[key] = true
					log.Info("mdns: answering on a network", "name", name, "interface", ifi.Name, "address", n.IP.String())
				}
			}
		}
	}
	join()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				join()
			}
		}
	}()

	go func() {
		buf := make([]byte, 9000)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			legacy := from.Port != port
			a, ok := question(buf[:n], name, legacy)
			if !ok {
				continue
			}
			ifaces, err := net.Interfaces()
			if err != nil {
				continue
			}
			ip, ifi := local(ifaces, func(i net.Interface) ([]net.Addr, error) { return i.Addrs() }, from.IP)
			if ip == nil {
				continue
			}
			out := answer(a, name, ip, legacy)
			if a.direct {
				conn.WriteToUDP(out, from)
				continue
			}
			// To the group, out by the network it was asked on.
			if err := sendFrom(conn, ip); err != nil {
				log.Debug("mdns: cannot choose the network to answer on", "interface", ifi.Name, "err", err)
			}
			conn.WriteToUDP(out, &net.UDPAddr{IP: group, Port: port})
		}
	}()
	return nil
}
