//go:build !linux

package api

// A manager runs on Linux; elsewhere, as on a developer's machine, its
// host and process are not read (0219).

type cpuSample struct{}

func cpuTimes() (cpuSample, bool) { return cpuSample{}, false }

func (now cpuSample) share(prev cpuSample) (*float64, *float64) { return nil, nil }

func ownRSS() (uint64, bool) { return 0, false }

type hostStats struct {
	load                   []float64
	memTotal, memAvailable uint64
}

func hostNow() (hostStats, bool) { return hostStats{}, false }

func diskOf(string) (uint64, uint64, bool) { return 0, 0, false }
