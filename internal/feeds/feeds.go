// Package feeds is the manager's cache of OpenWrt's package feeds (0069).
// APs fetch their packages through it: the first AP to want a file brings it
// from OpenWrt's server once, and every AP after gets it from the manager. A
// site keeps installing what its cache holds when its internet is gone.
//
// apk checks OpenWrt's signature on each index and each package's hash
// against its index, so files served from here are trusted as much as files
// from OpenWrt's own server. The cache only answers paths in OpenWrt's
// release tree, so it cannot be used to reach anything else.
//
// A release branch's snapshots (25.12-SNAPSHOT) are built anew under the
// same names, and so are a release's package feeds (packages/<arch>/...),
// which OpenWrt's buildbots build again from the release's branch: only a
// release's targets, the image's own build, never change (0249). apk refuses
// a package from another build than its index names ("ADB integrity
// error"). So an index of what is built anew is fetched again within
// minutes, and when it has changed the packages kept beside it go, to be
// fetched again of the build it names.
//
// OpenWrt's main snapshots, which an image built from main names as its
// feeds (the Arista ports' -49 and later), are a tree of their own beside
// the releases: a second cache answers it, under /feeds/snapshots/, and
// everything in it is a snapshot's (0230).
package feeds

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Upstream is OpenWrt's release tree; a path under /feeds/ is the same path
// under it.
const Upstream = "https://downloads.openwrt.org/releases/"

// SnapshotsUpstream is OpenWrt's main snapshots; a path under
// /feeds/snapshots/ is the same path under it (0230).
const SnapshotsUpstream = "https://downloads.openwrt.org/snapshots/"

const (
	IndexAge = 24 * time.Hour // an index is fetched again when older, if the upstream answers
	// The index of what is built anew is fetched again sooner: an AP that
	// asks for it is about to ask for packages of the build the upstream
	// has now.
	SnapshotIndexAge = 5 * time.Minute
	FetchLimit       = 10 * time.Minute
	// Without the internet, a connection often goes unanswered rather than
	// refused, so these say soon that the upstream is not there; FetchLimit
	// leaves a large file the time it needs once it is coming.
	ConnectLimit = 10 * time.Second
	AnswerLimit  = 30 * time.Second
	maxFile      = 256 << 20 // a firmware image fits; nothing in the tree is bigger
)

// segment is one part of a path the cache answers: OpenWrt's tree uses
// nothing else.
var segment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+~-]*$`)

// rebuilt says whether p is built anew under the same names: in a release
// branch's snapshots, or in a release's package feeds (0249). A release's
// targets never change.
func rebuilt(p string) bool {
	first, rest, _ := strings.Cut(p, "/")
	return strings.HasSuffix(first, "-SNAPSHOT") || strings.HasPrefix(rest, "packages/")
}

// rebuilt says whether p, in this cache, is built anew under the same names:
// everything is, in the main snapshots' (0230).
func (c *Cache) rebuilt(p string) bool { return c.all || rebuilt(p) }

// indexAge is how long an index at p is served before it is fetched again.
func (c *Cache) indexAge(p string) time.Duration {
	if c.rebuilt(p) {
		return SnapshotIndexAge
	}
	return IndexAge
}

// Cache is the feed cache. It is safe for concurrent use.
type Cache struct {
	dir      string
	upstream string
	prefix   string // the request paths it answers are under this
	all      bool   // every file is a snapshot's: the main snapshots (0230)
	max      int64
	client   *http.Client
	answerBy time.Duration // from asking to the answer's first line, a proxy's tunnel included
	now      func() time.Time

	mu       sync.Mutex
	used     map[string]time.Time // last served, by path
	sizes    map[string]int64
	total    int64
	inflight map[string]*fetch
}

type fetch struct {
	done chan struct{}
	err  error
}

// New opens the cache in dir, keeping at most max bytes, the least recently
// used going first. upstream is Upstream but in tests.
func New(dir, upstream string, max int64) (*Cache, error) {
	return openCache(dir, upstream, "/feeds/", false, max)
}

// NewSnapshots opens the cache of OpenWrt's main snapshots in dir (0230),
// answering under /feeds/snapshots/: upstream is SnapshotsUpstream but in
// tests.
func NewSnapshots(dir, upstream string, max int64) (*Cache, error) {
	return openCache(dir, upstream, "/feeds/snapshots/", true, max)
}

func openCache(dir, upstream, prefix string, all bool, max int64) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	c := &Cache{dir: dir, upstream: strings.TrimSuffix(upstream, "/") + "/", prefix: prefix, all: all, max: max,
		client: client(ConnectLimit, AnswerLimit), answerBy: ConnectLimit + AnswerLimit, now: time.Now,
		used: map[string]time.Time{}, sizes: map[string]int64{}, inflight: map[string]*fetch{}}
	os.RemoveAll(filepath.Join(dir, ".tmp"))
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		c.used[rel], c.sizes[rel] = info.ModTime(), info.Size()
		c.total += info.Size()
		return nil
	})
	return c, err
}

func client(connect, answer time.Duration) *http.Client {
	return &http.Client{
		Timeout: FetchLimit,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: connect, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   connect,
			ResponseHeaderTimeout: answer,
			MaxIdleConns:          8,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

// clean is the cache path a request names, or false if it isn't one the
// cache answers.
func (c *Cache) clean(p string) (string, bool) {
	if !strings.HasPrefix(p, c.prefix) {
		return "", false
	}
	p = strings.TrimPrefix(p, c.prefix)
	if p == "" || len(p) > 512 || strings.HasSuffix(p, "/") {
		return "", false
	}
	parts := strings.Split(p, "/")
	if len(parts) > 16 {
		return "", false
	}
	for _, s := range parts {
		if !segment.MatchString(s) {
			return "", false
		}
	}
	return path.Join(parts...), true
}

// index says whether p names an index or a list, which changes, rather than
// a package or image, which never does.
func index(p string) bool {
	b := path.Base(p)
	return strings.HasSuffix(b, ".adb") || strings.HasPrefix(b, "Packages") || strings.HasPrefix(b, "sha256sums") ||
		strings.HasSuffix(b, ".json") || strings.HasSuffix(b, ".sig") || strings.HasSuffix(b, ".asc")
}

func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "the feed cache is read-only", http.StatusMethodNotAllowed)
		return
	}
	p, ok := c.clean(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	local := filepath.Join(c.dir, filepath.FromSlash(p))
	info, err := os.Stat(local)
	fresh := err == nil && (!index(p) || c.now().Sub(info.ModTime()) < c.indexAge(p))
	if !fresh {
		ferr := c.fetch(r.Context(), p)
		var nf notFound
		switch {
		case ferr == nil:
		case errors.As(ferr, &nf):
			http.NotFound(w, r)
			return
		case err == nil:
			// The upstream didn't answer: what is kept will do.
			slog.Warn("feed cache: serving a kept copy", "path", p, "err", ferr)
		default:
			http.Error(w, "not in the cache, and OpenWrt's server could not be reached", http.StatusBadGateway)
			slog.Warn("feed cache: cannot fetch", "path", p, "err", ferr)
			return
		}
	}
	f, err := os.Open(local)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		http.Error(w, "cannot read the cache", http.StatusInternalServerError)
		return
	}
	c.mu.Lock()
	c.used[p] = c.now()
	c.mu.Unlock()
	http.ServeContent(w, r, path.Base(p), info.ModTime(), f)
}

type notFound struct{ status int }

func (e notFound) Error() string { return fmt.Sprintf("upstream answered %d", e.status) }

// fetch brings p from the upstream into the cache, once however many ask at
// the same time.
func (c *Cache) fetch(ctx context.Context, p string) error {
	c.mu.Lock()
	if f := c.inflight[p]; f != nil {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f := &fetch{done: make(chan struct{})}
	c.inflight[p] = f
	c.mu.Unlock()
	// The fetch outlives the request that started it, so the others waiting
	// on it, and the cache, still get the file.
	f.err = c.get(p)
	c.mu.Lock()
	delete(c.inflight, p)
	c.mu.Unlock()
	close(f.done)
	return f.err
}

func (c *Cache) get(p string) error {
	// The transport's limits don't cover a proxy's CONNECT, so a deadline
	// covers everything up to the answer's first line; FetchLimit, the body.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	late := time.AfterFunc(c.answerBy, cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.upstream+p, nil)
	if err != nil {
		late.Stop()
		return err
	}
	resp, err := c.client.Do(req)
	if !late.Stop() && err == nil {
		resp.Body.Close()
		err = fmt.Errorf("no answer within %v", c.answerBy)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone:
		return notFound{resp.StatusCode}
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("upstream answered %d", resp.StatusCode)
	}
	tmpDir := filepath.Join(c.dir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(tmpDir, "fetch-*")
	if err != nil {
		return err
	}
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxFile+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxFile {
		err = fmt.Errorf("larger than %d bytes", maxFile)
	}
	if err == nil && resp.ContentLength >= 0 && n != resp.ContentLength {
		err = fmt.Errorf("got %d of %d bytes", n, resp.ContentLength)
	}
	local := filepath.Join(c.dir, filepath.FromSlash(p))
	// An index of another build than the one kept.
	newBuild := err == nil && c.rebuilt(p) && index(p) && !same(local, tmp.Name())
	if err == nil {
		err = os.MkdirAll(filepath.Dir(local), 0o755)
	}
	if err == nil {
		now := c.now()
		os.Chtimes(tmp.Name(), now, now)
		err = os.Rename(tmp.Name(), local)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	slog.Info("feed cache: fetched", "path", p, "bytes", n)
	c.mu.Lock()
	c.total += n - c.sizes[p]
	c.sizes[p], c.used[p] = n, c.now()
	c.mu.Unlock()
	if newBuild {
		c.forget(p)
	}
	c.evict(p)
	return nil
}

// same says whether two files hold the same bytes.
func same(a, b string) bool {
	x, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b)
	return err == nil && bytes.Equal(x, y)
}

// forget lets go of the packages kept beside an index that has changed:
// they are of the build before, and are fetched again, of the build the
// index names now, when next asked for.
func (c *Cache) forget(idx string) {
	dir := path.Dir(idx)
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for p := range c.sizes {
		if path.Dir(p) != dir || index(p) {
			continue
		}
		os.Remove(filepath.Join(c.dir, filepath.FromSlash(p)))
		c.total -= c.sizes[p]
		delete(c.sizes, p)
		delete(c.used, p)
		n++
	}
	if n > 0 {
		slog.Info("feed cache: an index is of a new build; the packages kept beside it go", "dir", dir, "files", n)
	}
}

// evict removes the least recently used files until the cache fits, never
// the one just fetched.
func (c *Cache) evict(keep string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total <= c.max {
		return
	}
	paths := make([]string, 0, len(c.used))
	for p := range c.used {
		if p != keep {
			paths = append(paths, p)
		}
	}
	sort.Slice(paths, func(i, j int) bool { return c.used[paths[i]].Before(c.used[paths[j]]) })
	for _, p := range paths {
		if c.total <= c.max {
			break
		}
		os.Remove(filepath.Join(c.dir, filepath.FromSlash(p)))
		c.total -= c.sizes[p]
		delete(c.sizes, p)
		delete(c.used, p)
	}
}

// Stats is the cache's size: files and bytes.
func (c *Cache) Stats() (files int, bytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sizes), c.total
}
