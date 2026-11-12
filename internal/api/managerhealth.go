package api

import (
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

// The manager's own health (0219): how its host and its process fare,
// how big its databases are, and the answers it was slow to give, for an
// admin at the Org. On 2026-10-23 a query left running on the host took a
// processor core for 43 minutes and slowed every page, and nothing in the
// manager said so.

// slowKept is how many slow answers the manager keeps to show.
const slowKept = 50

// slowAnswer is one answer slowLog found slow.
type slowAnswer struct {
	At     time.Time `json:"at"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	MS     int64     `json:"ms"`
}

// slowAnswers keeps the latest slow answers, the newest last.
var slowAnswers struct {
	sync.Mutex
	list []slowAnswer
}

func keepSlow(a slowAnswer) {
	slowAnswers.Lock()
	defer slowAnswers.Unlock()
	slowAnswers.list = append(slowAnswers.list, a)
	if n := len(slowAnswers.list); n > slowKept {
		slowAnswers.list = append([]slowAnswer(nil), slowAnswers.list[n-slowKept:]...)
	}
}

// slowSince is how many slow answers came since t, and the latest of all,
// the newest first, at most n.
func slowSince(t time.Time, n int) (int, []slowAnswer) {
	slowAnswers.Lock()
	defer slowAnswers.Unlock()
	count := 0
	for _, a := range slowAnswers.list {
		if a.At.After(t) {
			count++
		}
	}
	out := make([]slowAnswer, 0, n)
	for i := len(slowAnswers.list) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, slowAnswers.list[i])
	}
	return count, out
}

// started is when the manager began.
var started = time.Now()

// cpuSampler keeps what share of the processors the host and the manager
// used over the last sample, taken every cpuEvery once anyone asks.
type cpuSampler struct {
	once      sync.Once
	mu        sync.Mutex
	host, own *float64
}

const cpuEvery = 30 * time.Second

var cpu cpuSampler

func (c *cpuSampler) start() {
	c.once.Do(func() {
		go func() {
			prev, ok := cpuTimes()
			for range time.Tick(cpuEvery) {
				now, nowOK := cpuTimes()
				if ok && nowOK {
					c.mu.Lock()
					c.host, c.own = now.share(prev)
					c.mu.Unlock()
				}
				prev, ok = now, nowOK
			}
		}()
	})
}

func (c *cpuSampler) shares() (host, own *float64) {
	c.start()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host, c.own
}

// WithFiles names the manager's database files, whose sizes its health
// says (0219).
func (s *Server) WithFiles(paths ...string) *Server {
	s.files = paths
	return s
}

// managerHealth is the manager's own health: GET /v1/system/health, an
// admin's at the Org (0219).
func (s *Server) managerHealth(w http.ResponseWriter, _ *http.Request, c call) error {
	if !administers(c) {
		return errNotFound
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	proc := map[string]any{"heap": ms.HeapAlloc, "goroutines": runtime.NumGoroutine()}
	if rss, ok := ownRSS(); ok {
		proc["rss"] = rss
	}
	hostCPU, ownCPU := cpu.shares()
	if ownCPU != nil {
		proc["cpu"] = *ownCPU
	}
	host := map[string]any{"cpus": runtime.NumCPU()}
	if h, ok := hostNow(); ok {
		host["load"], host["mem_total"], host["mem_available"] = h.load, h.memTotal, h.memAvailable
	}
	if hostCPU != nil {
		host["cpu"] = *hostCPU
	}
	if len(s.files) > 0 {
		if total, free, ok := diskOf(s.files[0]); ok {
			host["disk_total"], host["disk_free"] = total, free
		}
	}
	files := []map[string]any{}
	for _, p := range s.files {
		f := map[string]any{"name": baseName(p)}
		if fi, err := os.Stat(p); err == nil {
			f["bytes"] = fi.Size()
		}
		if fi, err := os.Stat(p + "-wal"); err == nil {
			f["wal"] = fi.Size()
		}
		files = append(files, f)
	}
	count, recent := slowSince(time.Now().Add(-time.Hour), 10)
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.version, "started": started, "uptime": int64(time.Since(started).Seconds()),
		"host": host, "process": proc, "databases": files,
		"slow": map[string]any{"after_ms": slowAfter.Milliseconds(), "last_hour": count, "recent": recent},
	})
	return nil
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
