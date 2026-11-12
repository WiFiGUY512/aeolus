// Package oui names the maker of a MAC from its first three bytes (0067),
// by the IEEE's registry of MA-L assignments, taken when the manager was
// built: run `go run ./internal/oui/gen` to take it again.
package oui

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

//go:embed oui.txt.gz
var table []byte

var (
	once   sync.Once
	makers map[uint32]string
)

func load() {
	makers = map[uint32]string{}
	z, err := gzip.NewReader(bytes.NewReader(table))
	if err != nil {
		return
	}
	s := bufio.NewScanner(z)
	for s.Scan() {
		p, name, ok := strings.Cut(s.Text(), "\t")
		if n, err := strconv.ParseUint(p, 16, 32); ok && err == nil {
			makers[uint32(n)] = name
		}
	}
}

// Lookup names the maker of mac ("aa:bb:cc:dd:ee:ff"), or "" when the
// registry doesn't know its prefix. private is true for a locally
// administered MAC, such as a phone's private one, which names no maker.
func Lookup(mac string) (maker string, private bool) {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return "", false
	}
	var p uint32
	for _, x := range parts[:3] {
		b, err := strconv.ParseUint(x, 16, 8)
		if err != nil {
			return "", false
		}
		p = p<<8 | uint32(b)
	}
	if p>>16&0x02 != 0 {
		return "", true
	}
	once.Do(load)
	return makers[p], false
}

// owners name the makers the registry knows by an old name, by the name
// people know them by now.
var owners = map[string]string{
	"Mojo Networks, Inc.":     "Mojo Networks (Arista)",
	"AirTight Networks, Inc.": "AirTight Networks (Arista)",
}

// BSSIDMaker names the maker of an AP by one of its BSSIDs (Griff,
// 2026-10-23: "identify these rogue APs by mac"), or "" where the registry
// doesn't know it. An AP makes its other networks' BSSIDs from its own MAC,
// setting the locally administered bit of the first byte and often others
// beside it, as the RUTM50's 26:97:27:c9:41:b7 beside its 20:97:27:c9:41:b7:
// such a BSSID has no maker of its own. Its maker is then a sibling's among
// heard, the BSSIDs heard with it: one with the same five last bytes, or
// the same four and a last byte near, that is not locally administered;
// else its own with that bit cleared. derived says it was found so.
func BSSIDMaker(bssid string, heard []string) (maker string, derived bool) {
	bssid = strings.ToLower(bssid)
	m, private := Lookup(bssid)
	if private && len(bssid) == 17 {
		near := func(other string, exact bool) bool {
			if len(other) != 17 || other == bssid || other[3:14] != bssid[3:14] {
				return false
			}
			if exact {
				return other[14:] == bssid[14:]
			}
			a, errA := strconv.ParseUint(other[15:], 16, 8)
			b, errB := strconv.ParseUint(bssid[15:], 16, 8)
			return errA == nil && errB == nil && (a-b < 16 || b-a < 16)
		}
		for _, exact := range []bool{true, false} {
			for _, o := range heard {
				o = strings.ToLower(o)
				if near(o, exact) {
					if om, op := Lookup(o); om != "" && !op {
						m, derived = om, true
						break
					}
				}
			}
			if derived {
				break
			}
		}
		if !derived {
			if b, err := strconv.ParseUint(bssid[:2], 16, 8); err == nil {
				m, _ = Lookup(fmt.Sprintf("%02x", b&^0x02) + bssid[2:])
				derived = m != ""
			}
		}
	}
	if o, ok := owners[m]; ok {
		m = o
	}
	return m, derived
}

// Size says how many prefixes the table holds.
func Size() int {
	once.Do(load)
	return len(makers)
}
