package feeds

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Modules serves the module feeds the manager keeps (0171): each a copy of
// a build's own package feed, for an image built with a kernel of its own,
// as the Arista ports are. OpenWrt's module feeds fit only OpenWrt's own
// kernels, so such an AP's kernel modules come from its build's feed or not
// at all. An AP asks for the one for its kernel by the kernel package's
// version:
//
//	/feeds/kmods/<kernel version>/packages.adb
//
// and then for the packages that index names, beside it. Every build of a
// port can have one kernel (the Arista ports' since -36), and one build's
// modules may not fit another's image, as its ucode modules a newer ucode
// (0256). So an AP asks first for its own build's, by its kernel and the
// build's tag (0260):
//
//	/feeds/kmods/<kernel version>/<build tag>/packages.adb
//
// A feed is of the build whose tag, as v2026.10.17.1-83-g439336f, is in
// its directory's path below the manager's, as the port's builder names its
// output; a feed with none is of no build, and is served by its kernel
// alone. Each feed is signed
// with its build's key, which that build's image trusts; the manager only
// hands the files on, as they were put there.
//
// The builds there are feeds for are listed, one a line, by tag, kernel and
// target, the target as the builder names a target's feed (feed-<target>),
// or "-" where its path names none (0280):
//
//	/feeds/kmods/builds
//
// An AP keeps copies of its kernel modules as its next build has them, from
// that build's feed, to put back before the network after the sysupgrade
// to it.
//
// A build's own package feeds, as its builder lays them out beside its
// module feed (feed-<target>/packages, and feed-<target>/<arch>/<feed>),
// are served by the build too (0315):
//
//	/feeds/kmods/<kernel version>/<build tag>/<arch>/<feed>/packages.adb
//
// An AP takes the packages its build has from them, and not from OpenWrt's
// snapshot, whose builds of the same versions may be against other
// libraries than the image's (2026-11-05: uclient, usteer and ustp against
// libubox20261103, while the image had libubox20260721).
//
// A feed is a directory, at most four below the one the manager is given,
// that holds a packages.adb and the kernel's own package,
// kernel-<version>.apk: a build's feed/packages/ copied in as it is (a
// directory linked in is not followed). One put there while the manager
// runs is found the first time an AP asks for its kernel, a minute after the
// last look at the most; one taken away, or replaced by another for the
// same kernel, likewise. Only the index and the packages are served, as
// files in the feed's directory: never a link, never another name.
type Modules struct {
	dir string
	now func() time.Time

	mu      sync.Mutex
	served  map[string]string // kernel version, or kernel/build tag (0260) -> the directory of its feed
	targets map[string]string // kernel/build tag -> the target its feed is of, or "-" (0280)
	looked  time.Time
	warned  map[string]string // the same -> the feeds last warned of
}

// LookEvery is how often, at the most, the directory is looked through
// again, when an AP asks.
const LookEvery = time.Minute

// Settle is how long a feed's index must have been left as it is before the
// feed is served in place of another for the same kernel: one being copied
// in is not taken halfway.
const Settle = 10 * time.Second

const deepest = 4

var kernelPackage = regexp.MustCompile(`^kernel-(.+)\.apk$`)

// buildTag is a port build's tag, as its builder names its output (0260).
var (
	buildTag      = regexp.MustCompile(`v[0-9]{4}\.[0-9]{2}\.[0-9]{2}(\.[0-9]+)?-[0-9]+-g[0-9a-f]{7,40}`)
	wholeBuildTag = regexp.MustCompile(`^` + buildTag.String() + `$`)
	// feedTarget is a target's feed, as the builder names it (0280).
	feedTarget = regexp.MustCompile(`^feed-([a-z0-9_]+)$`)
)

// targetOf is the target a feed's path names, or "-".
func targetOf(rel string) string {
	for _, s := range strings.Split(rel, "/") {
		if t := feedTarget.FindStringSubmatch(s); t != nil {
			return t[1]
		}
	}
	return "-"
}

// Builds is the builds there is a feed for, each "<tag> <kernel> <target>",
// sorted (0280).
func (m *Modules) Builds() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.now().Sub(m.looked) >= LookEvery {
		m.look()
	}
	out := []string{}
	for k := range m.served {
		if kernel, tag, ok := strings.Cut(k, "/"); ok {
			out = append(out, tag+" "+kernel+" "+m.targets[k])
		}
	}
	sort.Strings(out)
	return out
}

// NewModules opens the module feeds in dir, making it where it is missing.
func NewModules(dir string) (*Modules, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m := &Modules{dir: dir, now: time.Now}
	m.mu.Lock()
	m.look()
	m.mu.Unlock()
	return m, nil
}

// Kernels is the kernel versions there is a feed for, sorted.
func (m *Modules) Kernels() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.served))
	for k := range m.served {
		if !strings.Contains(k, "/") {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// look finds the feeds; m.mu is held. Each is served by its kernel, and by
// its kernel and its build's tag where its path has one (0260). Where
// several hold the same kernel, or the same kernel and build, the one with
// the newest index is served, of those whose index has settled: the latest
// build's, whose key a newly built image trusts.
func (m *Modules) look() {
	type feed struct {
		dir    string
		index  time.Time
		target string
	}
	all := map[string][]feed{}
	filepath.WalkDir(m.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(m.dir, p)
		if d.IsDir() {
			if rel != "." && len(strings.Split(filepath.ToSlash(rel), "/")) > deepest {
				return fs.SkipDir
			}
			return nil
		}
		k := kernelPackage.FindStringSubmatch(d.Name())
		if k == nil || !segment.MatchString(k[1]) {
			return nil
		}
		dir := filepath.Dir(p)
		info, err := os.Lstat(filepath.Join(dir, "packages.adb"))
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		t := targetOf(filepath.ToSlash(rel))
		all[k[1]] = append(all[k[1]], feed{dir, info.ModTime(), t})
		if b := buildTag.FindString(filepath.ToSlash(rel)); b != "" {
			all[k[1]+"/"+b] = append(all[k[1]+"/"+b], feed{dir, info.ModTime(), t})
		}
		return nil
	})
	now := m.now()
	m.served = make(map[string]string, len(all))
	m.targets = make(map[string]string, len(all))
	for k, feeds := range all {
		var best *feed
		for i := range feeds {
			f := &feeds[i]
			if now.Sub(f.index) >= Settle && (best == nil || f.index.After(best.index)) {
				best = f
			}
		}
		if best == nil { // none settled yet: the newest
			best = &feeds[0]
			for i := range feeds {
				if feeds[i].index.After(best.index) {
					best = &feeds[i]
				}
			}
		}
		m.served[k] = best.dir
		m.targets[k] = best.target
		if len(feeds) > 1 {
			dirs := make([]string, len(feeds))
			for i, f := range feeds {
				dirs[i] = f.dir
			}
			sort.Strings(dirs)
			set := best.dir + "\x00" + strings.Join(dirs, "\x00")
			if m.warned[k] != set {
				slog.Warn("module feeds: several feeds for one kernel; the newest index is served", "kernel", k, "served", best.dir, "feeds", dirs)
				if m.warned == nil {
					m.warned = map[string]string{}
				}
				m.warned[k] = set
			}
		}
	}
	m.looked = now
}

// feed is the directory of the feed for a kernel, or a kernel and a build
// (kernel/tag), or "". The directory is looked through again when the last
// look is a minute old, so a feed put beside one already served, or in
// place of it, is found; until then, one whose index has gone is no feed.
func (m *Modules) feed(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.now().Sub(m.looked) >= LookEvery {
		m.look()
	} else if d, ok := m.served[key]; ok {
		if info, err := os.Lstat(filepath.Join(d, "packages.adb")); err != nil || !info.Mode().IsRegular() {
			delete(m.served, key)
		}
	}
	return m.served[key]
}

// besideFeed is the directory of a build's package feed for arch, beside its
// module feed dir, as the builder lays them out: dir is feed-<target>/packages
// and the feed feed-<target>/<arch>/<feed> (0315); "" where it is not so, or
// where a part of it is a link, which is not followed.
func besideFeed(dir, arch, feed string) string {
	if filepath.Base(dir) != "packages" {
		return ""
	}
	base := filepath.Dir(dir)
	for _, p := range []string{filepath.Join(base, arch), filepath.Join(base, arch, feed)} {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() {
			return ""
		}
	}
	return filepath.Join(base, arch, feed)
}

func (m *Modules) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "the module feeds are read-only", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/feeds/kmods/builds" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, b := range m.Builds() {
			w.Write([]byte(b + "\n"))
		}
		return
	}
	// <kernel>/<file>, or <kernel>/<build tag>/<file> (0260), or
	// <kernel>/<build tag>/<arch>/<feed>/<file> (0315).
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/feeds/kmods/"), "/")
	var key, file string
	var beside []string
	switch {
	case len(parts) == 2 && segment.MatchString(parts[0]):
		key, file = parts[0], parts[1]
	case len(parts) == 3 && segment.MatchString(parts[0]) && wholeBuildTag.MatchString(parts[1]):
		key, file = parts[0]+"/"+parts[1], parts[2]
	case len(parts) == 5 && segment.MatchString(parts[0]) && wholeBuildTag.MatchString(parts[1]) && segment.MatchString(parts[2]) && segment.MatchString(parts[3]):
		key, beside, file = parts[0]+"/"+parts[1], parts[2:4], parts[4]
	}
	if key == "" || !segment.MatchString(file) || (file != "packages.adb" && !strings.HasSuffix(file, ".apk")) {
		http.NotFound(w, r)
		return
	}
	dir := m.feed(key)
	if dir == "" {
		http.NotFound(w, r)
		return
	}
	if beside != nil {
		if dir = besideFeed(dir, beside[0], beside[1]); dir == "" {
			http.NotFound(w, r)
			return
		}
	}
	// Opened within the feed's directory, and only as a file of its own:
	// a link there, to anything, is not followed.
	root, err := os.OpenRoot(dir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	if info, err := root.Lstat(file); err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, err := root.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, file, info.ModTime(), f)
}
