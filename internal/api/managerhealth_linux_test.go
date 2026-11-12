//go:build linux

package api

import (
	"path/filepath"
	"testing"
)

// On Linux the host and the manager are read from /proc (0219).
func TestHostAndProcessRead(t *testing.T) {
	s, ok := cpuTimes()
	if !ok || s.total == 0 {
		t.Fatalf("cpu times = %+v, %v", s, ok)
	}
	if rss, ok := ownRSS(); !ok || rss == 0 {
		t.Fatalf("rss = %d, %v", rss, ok)
	}
	if h, ok := hostNow(); !ok || len(h.load) != 3 || h.memTotal == 0 {
		t.Fatalf("host = %+v, %v", h, ok)
	}
	if total, free, ok := diskOf(filepath.Join(t.TempDir(), "aeolus.db")); !ok || total == 0 || free > total {
		t.Fatalf("disk = %d, %d, %v", total, free, ok)
	}
	if _, _, ok := diskOf("/proc/self/stat"); ok {
		t.Fatal("/proc has a size")
	}
	host, own := cpuSample{total: 2000, idle: 1500, own: 50}.share(cpuSample{total: 1000, idle: 900, own: 10})
	if host == nil || *host != 40 || own == nil || *own != 4 {
		t.Fatalf("share = %v %v", host, own)
	}
}
