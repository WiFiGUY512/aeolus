package feeds

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// upstream is a stand-in for OpenWrt's release tree, counting what it is
// asked for.
type upstream struct {
	srv   *httptest.Server
	hits  sync.Map // path -> *int32
	files map[string]string
	down  atomic.Bool
}

func newUpstream(t *testing.T, files map[string]string) *upstream {
	u := &upstream{files: files}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		p := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/releases/"), "/snapshots/")
		n, _ := u.hits.LoadOrStore(p, new(int32))
		atomic.AddInt32(n.(*int32), 1)
		body, ok := u.files[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		time.Sleep(20 * time.Millisecond)
		io.WriteString(w, body)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) count(p string) int32 {
	n, ok := u.hits.Load(p)
	if !ok {
		return 0
	}
	return atomic.LoadInt32(n.(*int32))
}

const (
	idx = "25.12.5/targets/ipq806x/generic/packages/packages.adb"
	pkg = "25.12.5/packages/arm_cortex-a15_neon-vfpv4/base/usteer-2025.05.31~1.apk"
)

func get(t *testing.T, c *Cache, method, p string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	c.ServeHTTP(rec, httptest.NewRequest(method, p, nil))
	return rec.Code, rec.Body.String()
}

func open(t *testing.T, u *upstream, max int64) (*Cache, *time.Time) {
	t.Helper()
	c, err := New(t.TempDir(), u.srv.URL+"/releases/", max)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	c.now = func() time.Time { return clock }
	return c, &clock
}

func TestFetchedOnceThenServed(t *testing.T) {
	u := newUpstream(t, map[string]string{idx: "index v1", pkg: "usteer"})
	c, _ := open(t, u, 1<<20)
	for i := 0; i < 3; i++ {
		if code, body := get(t, c, "GET", "/feeds/"+pkg); code != 200 || body != "usteer" {
			t.Fatalf("get %d: %d %q", i, code, body)
		}
	}
	if n := u.count(pkg); n != 1 {
		t.Fatalf("upstream asked %d times", n)
	}
	if code, body := get(t, c, "HEAD", "/feeds/"+pkg); code != 200 || body != "" {
		t.Fatalf("head: %d %q", code, body)
	}
	if files, bytes := c.Stats(); files != 1 || bytes != int64(len("usteer")) {
		t.Fatalf("stats %d %d", files, bytes)
	}
}

func TestManyAskAtOnce(t *testing.T) {
	u := newUpstream(t, map[string]string{pkg: "usteer"})
	c, _ := open(t, u, 1<<20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, body := get(t, c, "GET", "/feeds/"+pkg); code != 200 || body != "usteer" {
				t.Errorf("%d %q", code, body)
			}
		}()
	}
	wg.Wait()
	if n := u.count(pkg); n != 1 {
		t.Fatalf("upstream asked %d times", n)
	}
}

func TestIndexesAreFetchedAgainButKeptWhenOffline(t *testing.T) {
	u := newUpstream(t, map[string]string{idx: "index v1"})
	c, clock := open(t, u, 1<<20)
	get(t, c, "GET", "/feeds/"+idx)
	u.files[idx] = "index v2"
	*clock = clock.Add(time.Hour)
	if _, body := get(t, c, "GET", "/feeds/"+idx); body != "index v1" {
		t.Fatalf("an hour on: %q", body)
	}
	*clock = clock.Add(IndexAge)
	if _, body := get(t, c, "GET", "/feeds/"+idx); body != "index v2" {
		t.Fatalf("a day on: %q", body)
	}
	// With the internet gone, the kept index is served, and what was never
	// fetched cannot be.
	u.down.Store(true)
	*clock = clock.Add(2 * IndexAge)
	if code, body := get(t, c, "GET", "/feeds/"+idx); code != 200 || body != "index v2" {
		t.Fatalf("offline: %d %q", code, body)
	}
	if code, _ := get(t, c, "GET", "/feeds/"+pkg); code != http.StatusBadGateway {
		t.Fatalf("offline, never fetched: %d", code)
	}
}

func TestOnlyOpenWrtsTree(t *testing.T) {
	u := newUpstream(t, map[string]string{})
	c, _ := open(t, u, 1<<20)
	for _, p := range []string{"/feeds/", "/feeds/../etc/passwd", "/feeds/25.12.5/../../x", "/feeds/.hidden/x", "/feeds/a//b",
		"/feeds/25.12.5/packages/", "/feeds/http:/evil.example/x", "/feeds/a%20b", "/feeds/%2e%2e/x"} {
		if code, _ := get(t, c, "GET", p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	if code, _ := get(t, c, "GET", "/feeds/25.12.5/nothing.apk"); code != 404 || u.count("25.12.5/nothing.apk") != 1 {
		t.Errorf("missing upstream: %d", code)
	}
	if code, _ := get(t, c, "POST", "/feeds/"+pkg); code != http.StatusMethodNotAllowed {
		t.Errorf("post: %d", code)
	}
}

func TestLeastRecentlyUsedGoesFirst(t *testing.T) {
	files := map[string]string{"a.apk": "aaaa", "b.apk": "bbbb", "c.apk": "cccc"}
	u := newUpstream(t, files)
	c, clock := open(t, u, 9)
	get(t, c, "GET", "/feeds/a.apk")
	*clock = clock.Add(time.Second)
	get(t, c, "GET", "/feeds/b.apk")
	*clock = clock.Add(time.Second)
	get(t, c, "GET", "/feeds/a.apk") // a is used again
	*clock = clock.Add(time.Second)
	get(t, c, "GET", "/feeds/c.apk") // over 9 bytes: b goes
	if files, bytes := c.Stats(); files != 2 || bytes != 8 {
		t.Fatalf("stats %d %d", files, bytes)
	}
	get(t, c, "GET", "/feeds/a.apk")
	get(t, c, "GET", "/feeds/b.apk")
	if u.count("a.apk") != 1 || u.count("b.apk") != 2 {
		t.Fatalf("a %d, b %d", u.count("a.apk"), u.count("b.apk"))
	}
}

func TestReopened(t *testing.T) {
	u := newUpstream(t, map[string]string{pkg: "usteer"})
	dir := t.TempDir()
	c, _ := New(dir, u.srv.URL+"/releases/", 1<<20)
	get(t, c, "GET", "/feeds/"+pkg)
	c2, err := New(dir, u.srv.URL+"/releases/", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if files, _ := c2.Stats(); files != 1 {
		t.Fatalf("reopened with %d files", files)
	}
	if _, body := get(t, c2, "GET", "/feeds/"+pkg); body != "usteer" || u.count(pkg) != 1 {
		t.Fatalf("reopened: %q, upstream %d", body, u.count(pkg))
	}
}

// An upstream that takes the connection and never answers is given up on
// soon, and a kept copy is served (0069): offline, connections often go
// unanswered rather than refused.
func TestAnUpstreamThatNeverAnswers(t *testing.T) {
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(hang)
	c, err := New(t.TempDir(), srv.URL+"/releases/", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	c.client, c.answerBy = client(time.Second, 200*time.Millisecond), time.Second
	start := time.Now()
	if code, _ := get(t, c, "GET", "/feeds/"+pkg); code != http.StatusBadGateway {
		t.Fatalf("never fetched: %d", code)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

// A proxy that takes the connection and never answers the CONNECT is given
// up on as soon: the transport's own limits don't cover the tunnel.
func TestAProxyThatNeverAnswers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close() // held open, never answered
		}
	}()
	c, err := New(t.TempDir(), "https://downloads.invalid/releases/", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	c.client, c.answerBy = client(time.Second, time.Second), 300*time.Millisecond
	c.client.Transport.(*http.Transport).Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: ln.Addr().String()})
	start := time.Now()
	if code, _ := get(t, c, "GET", "/feeds/"+pkg); code != http.StatusBadGateway {
		t.Fatalf("through a silent proxy: %d", code)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

// A release branch's snapshots are built anew under the same names: their
// index is fetched again within minutes, and a new build's index lets go of
// the packages kept beside it, so that an AP never gets a package of
// another build than its index names (apk's "ADB integrity error", seen on
// the manager for vxlan and libopenssl3 on 2026-10-16). An index that has
// not changed keeps them, and a release's index waits its day.
func TestSnapshotsKeepIndexAndPackagesOfOneBuild(t *testing.T) {
	const (
		snapIdx = "25.12-SNAPSHOT/packages/aarch64_cortex-a53/base/packages.adb"
		snapPkg = "25.12-SNAPSHOT/packages/aarch64_cortex-a53/base/vxlan-7.apk"
		other   = "25.12-SNAPSHOT/packages/aarch64_cortex-a53/luci/luci-base-26.1.apk"
	)
	u := newUpstream(t, map[string]string{snapIdx: "build 1", snapPkg: "vxlan of build 1", other: "luci", idx: "index v1"})
	c, clock := open(t, u, 1<<20)
	get(t, c, "GET", "/feeds/"+snapIdx)
	get(t, c, "GET", "/feeds/"+snapPkg)
	get(t, c, "GET", "/feeds/"+other)
	get(t, c, "GET", "/feeds/"+idx)

	// Rebuilt upstream: within the five minutes, the build kept is served
	// whole.
	u.files[snapIdx], u.files[snapPkg], u.files[idx] = "build 2", "vxlan of build 2", "index v2"
	*clock = clock.Add(SnapshotIndexAge - time.Second)
	if _, body := get(t, c, "GET", "/feeds/"+snapIdx); body != "build 1" {
		t.Fatalf("index within minutes: %q", body)
	}
	if _, body := get(t, c, "GET", "/feeds/"+snapPkg); body != "vxlan of build 1" {
		t.Fatalf("package within minutes: %q", body)
	}

	// After them, the new build's index, and its packages with it.
	*clock = clock.Add(2 * time.Second)
	if _, body := get(t, c, "GET", "/feeds/"+snapIdx); body != "build 2" {
		t.Fatalf("index after: %q", body)
	}
	if _, body := get(t, c, "GET", "/feeds/"+snapPkg); body != "vxlan of build 2" {
		t.Fatalf("package after: %q", body)
	}
	if n := u.count(snapPkg); n != 2 {
		t.Fatalf("package fetched %d times", n)
	}
	// Another directory's packages are not that index's.
	get(t, c, "GET", "/feeds/"+other)
	if n := u.count(other); n != 1 {
		t.Fatalf("another directory's package fetched %d times", n)
	}
	// A release's index is a day's.
	if _, body := get(t, c, "GET", "/feeds/"+idx); body != "index v1" {
		t.Fatalf("release index: %q", body)
	}

	// Fetched again and the same: the packages stay.
	*clock = clock.Add(SnapshotIndexAge)
	get(t, c, "GET", "/feeds/"+snapIdx)
	get(t, c, "GET", "/feeds/"+snapPkg)
	if n, m := u.count(snapIdx), u.count(snapPkg); n != 3 || m != 2 {
		t.Fatalf("index fetched %d times, package %d", n, m)
	}
	files, bytes := c.Stats()
	if want := len("build 2") + len("vxlan of build 2") + len("luci") + len("index v1"); files != 4 || bytes != int64(want) {
		t.Fatalf("stats %d %d, want 4 %d", files, bytes, want)
	}
}

// A release's package feeds are built anew under the same names too, from
// the release's branch: 25.12.2's vxlan-7 was of 2026-10-28's build, and the
// manager served the one it had kept since 10-04, which apk refused (0249).
// Their index goes after minutes, as a snapshot's, and its packages with it;
// the release's targets, the image's own build, keep theirs a day.
func TestAReleasesPackageFeedsAreRebuilt(t *testing.T) {
	const (
		feedIdx = "25.12.2/packages/aarch64_cortex-a53/base/packages.adb"
		feedPkg = "25.12.2/packages/aarch64_cortex-a53/base/vxlan-7.apk"
		kmod    = "25.12.5/targets/ipq806x/generic/packages/kmod-vxlan-6.12.94-r1.apk"
	)
	u := newUpstream(t, map[string]string{feedIdx: "build 1", feedPkg: "vxlan of build 1", idx: "index v1", kmod: "kmod"})
	c, clock := open(t, u, 1<<20)
	for _, p := range []string{feedIdx, feedPkg, idx, kmod} {
		get(t, c, "GET", "/feeds/"+p)
	}

	u.files[feedIdx], u.files[feedPkg], u.files[idx] = "build 2", "vxlan of build 2", "index v2"
	*clock = clock.Add(SnapshotIndexAge + time.Second)
	if _, body := get(t, c, "GET", "/feeds/"+feedIdx); body != "build 2" {
		t.Fatalf("feed index after minutes: %q", body)
	}
	if _, body := get(t, c, "GET", "/feeds/"+feedPkg); body != "vxlan of build 2" {
		t.Fatalf("feed package after: %q", body)
	}
	if _, body := get(t, c, "GET", "/feeds/"+idx); body != "index v1" {
		t.Fatalf("target index after minutes: %q", body)
	}
	get(t, c, "GET", "/feeds/"+kmod)
	if n := u.count(kmod); n != 1 {
		t.Fatalf("a target's package fetched %d times", n)
	}
}

// OpenWrt's main snapshots are a tree of their own, which an image built
// from main names as its feeds: a cache of its own answers it under
// /feeds/snapshots/, and every index there is a snapshot's, fetched again
// within minutes, its packages with it when it changed (0230).
func TestMainSnapshots(t *testing.T) {
	const (
		tIdx = "targets/qualcommax/ipq807x/packages/packages.adb"
		tPkg = "targets/qualcommax/ipq807x/packages/kmod-veth-6.18.55-r1.apk"
		bIdx = "packages/aarch64_cortex-a53/base/packages.adb"
	)
	u := newUpstream(t, map[string]string{tIdx: "build 1", tPkg: "veth of build 1", bIdx: "base 1"})
	c, err := NewSnapshots(t.TempDir(), u.srv.URL+"/snapshots/", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	c.now = func() time.Time { return clock }
	for p, want := range map[string]string{tIdx: "build 1", tPkg: "veth of build 1", bIdx: "base 1"} {
		if code, body := get(t, c, "GET", "/feeds/snapshots/"+p); code != 200 || body != want {
			t.Fatalf("%s: %d %q", p, code, body)
		}
	}
	// Only its own tree.
	if code, _ := get(t, c, "GET", "/feeds/"+idx); code != 404 {
		t.Fatalf("a release's path: %d", code)
	}
	// Rebuilt upstream: the next build's index after minutes, not a day,
	// and its packages with it.
	u.files[tIdx], u.files[tPkg] = "build 2", "veth of build 2"
	clock = clock.Add(SnapshotIndexAge + time.Second)
	if _, body := get(t, c, "GET", "/feeds/snapshots/"+tIdx); body != "build 2" {
		t.Fatalf("index after minutes: %q", body)
	}
	if _, body := get(t, c, "GET", "/feeds/snapshots/"+tPkg); body != "veth of build 2" {
		t.Fatalf("package after the index changed: %q", body)
	}
}
