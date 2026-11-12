package oui

import "testing"

// An AP's maker by its BSSID: by its prefix; for a BSSID with the locally
// administered bit set, by a sibling heard with it, else by its prefix with
// that bit cleared; and Mojo, by the name it has now.
func TestBSSIDMaker(t *testing.T) {
	for _, c := range []struct {
		bssid, maker string
		derived      bool
	}{
		{"20:97:27:c9:41:b7", "TELTONIKA NETWORKS UAB", false},
		{"26:97:27:c9:41:b7", "TELTONIKA NETWORKS UAB", true},
		{"26:97:27:c9:41:b9", "TELTONIKA NETWORKS UAB", true},
		{"e4:d1:24:08:6d:11", "Mojo Networks (Arista)", false},
		{"3e:ff:4d:02:26:c0", "", false},
		{"22:97:27:c9:41:b7", "TELTONIKA NETWORKS UAB", true},
		{"not a mac", "", false},
	} {
		heard := []string{"20:97:27:c9:41:b7", "26:97:27:c9:41:b7", "e4:d1:24:08:6d:11", "3e:ff:4d:02:26:c0"}
		if c.bssid == "22:97:27:c9:41:b7" {
			heard = nil // no sibling: by the prefix with the bit cleared, 20:97:27
		}
		if m, d := BSSIDMaker(c.bssid, heard); m != c.maker || d != c.derived {
			t.Errorf("%s: %q %v, want %q %v", c.bssid, m, d, c.maker, c.derived)
		}
	}
}
