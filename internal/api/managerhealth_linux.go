//go:build linux

package api

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// cpuSample is the host's processor time, all of it and the idle part,
// and the manager's own, in clock ticks, as /proc counts them.
type cpuSample struct{ total, idle, own uint64 }

func cpuTimes() (cpuSample, bool) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSample{}, false
	}
	line, _, _ := strings.Cut(string(stat), "\n")
	f := strings.Fields(line)
	if len(f) < 6 || f[0] != "cpu" {
		return cpuSample{}, false
	}
	var s cpuSample
	for i, v := range f[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		s.total += n
		if i == 3 || i == 4 { // idle and iowait
			s.idle += n
		}
	}
	self, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return cpuSample{}, false
	}
	// After the command's closing bracket: the state, then on to utime and
	// stime, the 14th and 15th fields.
	rest := string(self)
	if i := strings.LastIndexByte(rest, ')'); i >= 0 && i+2 <= len(rest) {
		rest = rest[i+2:]
	}
	g := strings.Fields(rest)
	if len(g) < 13 {
		return cpuSample{}, false
	}
	ut, _ := strconv.ParseUint(g[11], 10, 64)
	st, _ := strconv.ParseUint(g[12], 10, 64)
	s.own = ut + st
	return s, true
}

// share is what part of all the processors, in percent, the host was busy
// and the manager used, from prev to now.
func (now cpuSample) share(prev cpuSample) (*float64, *float64) {
	if now.total <= prev.total {
		return nil, nil
	}
	dt := float64(now.total - prev.total)
	host := math.Round(1000*(1-float64(now.idle-prev.idle)/dt)) / 10
	own := math.Round(1000*float64(now.own-prev.own)/dt) / 10
	return &host, &own
}

// ownRSS is the manager's resident memory, in bytes.
func ownRSS() (uint64, bool) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			f := strings.Fields(v)
			if len(f) >= 1 {
				kb, err := strconv.ParseUint(f[0], 10, 64)
				return kb * 1024, err == nil
			}
		}
	}
	return 0, false
}

// hostStats is the host's load, and its memory in bytes: all of it, and
// what is available.
type hostStats struct {
	load                   []float64
	memTotal, memAvailable uint64
}

func hostNow() (hostStats, bool) {
	var h hostStats
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return h, false
	}
	for i, v := range strings.Fields(string(b)) {
		if i == 3 {
			break
		}
		x, _ := strconv.ParseFloat(v, 64)
		h.load = append(h.load, x)
	}
	m, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return h, false
	}
	for _, line := range strings.Split(string(m), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) < 1 {
			continue
		}
		kb, _ := strconv.ParseUint(f[0], 10, 64)
		switch k {
		case "MemTotal":
			h.memTotal = kb * 1024
		case "MemAvailable":
			h.memAvailable = kb * 1024
		}
	}
	return h, true
}

// diskOf is the size of the file system a file is on, and what is free
// there, in bytes; none for one that has no size, as /proc.
func diskOf(path string) (uint64, uint64, bool) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(filepath.Dir(path), &fs); err != nil || fs.Blocks == 0 {
		return 0, 0, false
	}
	return fs.Blocks * uint64(fs.Bsize), fs.Bavail * uint64(fs.Bsize), true
}
