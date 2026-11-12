package agent

import (
	"io/fs"
	"regexp"
	"testing"
)

// makesMonitor is what making a monitor interface reads like in the
// agent's files: iw's "type monitor", as words or as a list's items, or
// nl80211's interface type.
var makesMonitor = regexp.MustCompile(`type['", ]+monitor|IFTYPE_MONITOR`)

// TestNoMonitorInterface keeps the agent from making a monitor interface on
// an AP's radios (0163): on the C-360, ath11k's monitor receive wrote past
// its buffers into the Ethernet driver's, and the AP restarted or stopped
// receiving several times a day until the monitor interfaces went.
func TestNoMonitorInterface(t *testing.T) {
	err := fs.WalkDir(Files, "files", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := Files.ReadFile(path)
		if err != nil {
			return err
		}
		if loc := makesMonitor.FindIndex(b); loc != nil {
			t.Errorf("%s makes a monitor interface: %q", path, b[loc[0]:loc[1]])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
