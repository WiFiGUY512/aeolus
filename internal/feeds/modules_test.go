package feeds

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// feedAt puts a build's feed in dir/rel: its index, its kernel's package and
// one module.
func feedAt(t *testing.T, dir, rel, kernel, index string) {
	t.Helper()
	d := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"packages.adb":                 index,
		"kernel-" + kernel + ".apk":    "kernel",
		"kmod-veth-6.12.112-r1.apk":    "veth of " + index,
		"kmod-nft-bridge-6.12.112.apk": "nft-bridge",
	} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func modulesGet(t *testing.T, m *Modules, method, p string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(method, p, nil))
	return rec.Code, rec.Body.String()
}

const kernel = "6.12.112~1439aebc94b35ebb6258531fbd56aa12-r1"

// An AP asks by its kernel's version for its build's feed, wherever under
// the directory it was copied, and gets the index and the packages beside
// it, and nothing else.
func TestModulesByKernel(t *testing.T) {
	dir := t.TempDir()
	feedAt(t, dir, "c360/c7fff51/feed/packages", kernel, "c360 index")
	feedAt(t, dir, "c260-07c5f98", "6.12.112~0123abcd-r1", "c260 index")
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	if k := m.Kernels(); len(k) != 2 || k[0] != "6.12.112~0123abcd-r1" || k[1] != kernel {
		t.Fatalf("kernels %v", k)
	}
	for _, c := range []struct {
		method, path string
		code         int
		body         string
	}{
		{"GET", "/feeds/kmods/" + kernel + "/packages.adb", 200, "c360 index"},
		{"GET", "/feeds/kmods/" + kernel + "/kmod-veth-6.12.112-r1.apk", 200, "veth of c360 index"},
		{"GET", "/feeds/kmods/6.12.112~0123abcd-r1/kmod-veth-6.12.112-r1.apk", 200, "veth of c260 index"},
		{"HEAD", "/feeds/kmods/" + kernel + "/packages.adb", 200, ""},
		{"GET", "/feeds/kmods/6.12.94~dbd7ac59-r1/packages.adb", 404, ""},
		{"GET", "/feeds/kmods/" + kernel + "/missing.apk", 404, ""},
		{"GET", "/feeds/kmods/" + kernel + "/../c260-07c5f98/packages.adb", 404, ""},
		{"GET", "/feeds/kmods/" + kernel + "/.hidden", 404, ""},
		{"GET", "/feeds/kmods/" + kernel, 404, ""},
		{"GET", "/feeds/kmods/" + kernel + "/", 404, ""},
		{"POST", "/feeds/kmods/" + kernel + "/packages.adb", 405, ""},
	} {
		code, body := modulesGet(t, m, c.method, c.path)
		if code != c.code || (c.code == 200 && body != c.body) {
			t.Errorf("%s %s: %d %q", c.method, c.path, code, body)
		}
	}
}

// A feed copied in while the manager runs is found when an AP first asks
// for its kernel, once the last look is a minute old; one deeper than four
// below the directory, or without its index, is no feed.
func TestModulesFoundLater(t *testing.T) {
	dir := t.TempDir()
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	m.now = func() time.Time { return clock }
	feedAt(t, dir, "c360-new", kernel, "new index")
	feedAt(t, dir, "a/b/c/d/e", "6.12.1~deep-r1", "deep")
	if err := os.MkdirAll(filepath.Join(dir, "noindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "noindex", "kernel-6.12.2~x-r1.apk"), []byte("kernel"), 0o644)
	if code, _ := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); code != 404 {
		t.Fatalf("found before a minute: %d", code)
	}
	clock = clock.Add(LookEvery)
	if code, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); code != 200 || body != "new index" {
		t.Fatalf("after a minute: %d %q", code, body)
	}
	if k := m.Kernels(); len(k) != 1 || k[0] != kernel {
		t.Fatalf("kernels %v", k)
	}
}

// Two feeds for one kernel: the one with the newest index is served,
// whichever comes first by name.
func TestModulesOneFeedAKernel(t *testing.T) {
	dir := t.TempDir()
	feedAt(t, dir, "b-newer", kernel, "newer")
	feedAt(t, dir, "a-older", kernel, "older")
	old, newer := time.Now().Add(-time.Hour), time.Now().Add(-time.Minute)
	if err := os.Chtimes(filepath.Join(dir, "a-older", "packages.adb"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "b-newer", "packages.adb"), newer, newer); err != nil {
		t.Fatal(err)
	}
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	if code, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); code != 200 || body != "newer" {
		t.Fatalf("%d %q", code, body)
	}
}

// Only the index and the packages are served, each a file in the feed's
// directory: a link there, to a file outside or inside, is not, nor is a
// file of another name; and a feed whose index is a link is no feed.
func TestModulesNoLinks(t *testing.T) {
	dir := t.TempDir()
	feedAt(t, dir, "c360", kernel, "index")
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(dir, "c360")
	if err := os.Symlink(outside, filepath.Join(d, "secret.apk")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := os.Symlink("kmod-veth-6.12.112-r1.apk", filepath.Join(d, "linked.apk")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "notes.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := "6.12.3~linked-r1"
	feedAt(t, dir, "linkedindex", other, "real")
	li := filepath.Join(dir, "linkedindex", "packages.adb")
	if err := os.Rename(li, li+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("packages.adb.real", li); err != nil {
		t.Fatal(err)
	}
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path string
		code int
	}{
		{"/feeds/kmods/" + kernel + "/packages.adb", 200},
		{"/feeds/kmods/" + kernel + "/kmod-veth-6.12.112-r1.apk", 200},
		{"/feeds/kmods/" + kernel + "/secret.apk", 404},
		{"/feeds/kmods/" + kernel + "/linked.apk", 404},
		{"/feeds/kmods/" + kernel + "/notes.txt", 404},
		{"/feeds/kmods/" + other + "/packages.adb", 404},
	} {
		if code, body := modulesGet(t, m, "GET", c.path); code != c.code || (code == 200 && body == "secret") {
			t.Errorf("%s: %d %q", c.path, code, body)
		}
	}
}

// A newer feed put beside one already served, for the same kernel, is served
// once the last look is a minute old.
func TestModulesNewerBeside(t *testing.T) {
	dir := t.TempDir()
	feedAt(t, dir, "older", kernel, "older")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "older", "packages.adb"), old, old); err != nil {
		t.Fatal(err)
	}
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	m.now = func() time.Time { return clock }
	feedAt(t, dir, "newer", kernel, "newer")
	if _, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); body != "older" {
		t.Fatalf("within the minute: %q", body)
	}
	clock = clock.Add(LookEvery)
	if code, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); code != 200 || body != "newer" {
		t.Fatalf("after a minute: %d %q", code, body)
	}
}

// A feed being copied in beside one served, its index just written, is not
// served until its index has settled.
func TestModulesHalfCopied(t *testing.T) {
	dir := t.TempDir()
	feedAt(t, dir, "served", kernel, "served")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "served", "packages.adb"), old, old); err != nil {
		t.Fatal(err)
	}
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now().Add(LookEvery)
	m.now = func() time.Time { return clock }
	feedAt(t, dir, "copying", kernel, "copying")
	just := clock.Add(-time.Second)
	if err := os.Chtimes(filepath.Join(dir, "copying", "packages.adb"), just, just); err != nil {
		t.Fatal(err)
	}
	if _, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); body != "served" {
		t.Fatalf("a feed still being copied was served: %q", body)
	}
	clock = clock.Add(LookEvery)
	if _, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); body != "copying" {
		t.Fatalf("once settled: %q", body)
	}
}

// A feed taken away, and another put in for the same kernel, is found again
// once the last look is a minute old, without the manager starting anew.
func TestModulesFeedReplaced(t *testing.T) {
	dir := t.TempDir()
	feedAt(t, dir, "first-build", kernel, "first")
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	m.now = func() time.Time { return clock }
	if code, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); code != 200 || body != "first" {
		t.Fatalf("before: %d %q", code, body)
	}
	if err := os.RemoveAll(filepath.Join(dir, "first-build")); err != nil {
		t.Fatal(err)
	}
	feedAt(t, dir, "second-build", kernel, "second")
	clock = clock.Add(LookEvery)
	if code, body := modulesGet(t, m, "GET", "/feeds/kmods/"+kernel+"/packages.adb"); code != 200 || body != "second" {
		t.Fatalf("after: %d %q", code, body)
	}
}

// Builds of a port that share a kernel each have their own feed, asked for
// by the kernel and the build's tag, which is in the feed's path (0260). By
// the kernel alone the newest is served, as before; a build the manager has
// no feed of, or a tag given for another kernel, is none.
func TestModulesByBuild(t *testing.T) {
	dir := t.TempDir()
	const k = "6.18.55~ff43a0f89c8d9bdaefbce22af8970550-r1"
	old, now := "v2026.10.17.1-36-g06e07b2", "v2026.10.17.1-83-g439336f"
	feedAt(t, dir, "main-"+old+"-ucode/packages", k, "-36 index")
	feedAt(t, dir, "out/"+now+"/feed-ipq807x", k, "-83 index")
	feedAt(t, dir, "out/"+now+"/feed-ipq60xx", "6.18.55~1de4b4e0-r1", "-83 ipq60xx index")
	feedAt(t, dir, "c260-ace586d0", "6.12.112~ace586d0-r1", "no build")
	// Each index settled, -83's the newer.
	for rel, ago := range map[string]time.Duration{"main-" + old + "-ucode/packages": 2 * time.Hour, "out/" + now + "/feed-ipq807x": time.Hour,
		"out/" + now + "/feed-ipq60xx": time.Hour, "c260-ace586d0": time.Hour} {
		at := time.Now().Add(-ago)
		os.Chtimes(filepath.Join(dir, filepath.FromSlash(rel), "packages.adb"), at, at)
	}
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ks := m.Kernels(); len(ks) != 3 {
		t.Fatalf("kernels, without the builds: %v", ks)
	}
	for _, c := range []struct {
		path string
		code int
		body string
	}{
		{"/feeds/kmods/" + k + "/" + old + "/packages.adb", 200, "-36 index"},
		{"/feeds/kmods/" + k + "/" + now + "/packages.adb", 200, "-83 index"},
		{"/feeds/kmods/" + k + "/" + now + "/kmod-veth-6.12.112-r1.apk", 200, "veth of -83 index"},
		{"/feeds/kmods/" + k + "/packages.adb", 200, "-83 index"},
		{"/feeds/kmods/6.18.55~1de4b4e0-r1/" + now + "/packages.adb", 200, "-83 ipq60xx index"},
		{"/feeds/kmods/" + k + "/v2026.10.17.1-99-gaaaaaaa/packages.adb", 404, ""},
		{"/feeds/kmods/6.12.112~ace586d0-r1/" + now + "/packages.adb", 404, ""},
		{"/feeds/kmods/6.12.112~ace586d0-r1/packages.adb", 200, "no build"},
		{"/feeds/kmods/" + k + "/not-a-tag/packages.adb", 404, ""},
		{"/feeds/kmods/" + k + "/" + now + "/../packages.adb", 404, ""},
	} {
		code, body := modulesGet(t, m, "GET", c.path)
		if code != c.code || (c.code == 200 && body != c.body) {
			t.Errorf("%s: %d %q", c.path, code, body)
		}
	}
}

// The builds there are feeds for are listed by tag, kernel and target, the
// target as the builder names a target's feed, or "-" (0280): an AP finds
// its next build's there, of its own target, to keep its kernel modules as
// that build has them.
func TestModulesBuilds(t *testing.T) {
	dir := t.TempDir()
	const k, k60 = "6.18.55~ff43a0f89c8d9bdaefbce22af8970550-r1", "6.18.60~79086d879a82ae4f26c0df85800d8348-r1"
	feedAt(t, dir, "v2026.10.17.1-95-g9c8b99c/feed-ipq807x/packages", k, "-95")
	feedAt(t, dir, "v2026.10.17.1-103-g4e32d7d/feed-ipq807x/packages", k60, "-103")
	feedAt(t, dir, "v2026.10.17.1-103-g4e32d7d/feed-ipq60xx/packages", "6.18.60~1de4b4e0-r1", "-103 ipq60xx")
	feedAt(t, dir, "main-v2026.10.17.1-36-g06e07b2-ucode/packages", k, "-36")
	feedAt(t, dir, "c260-ace586d0", "6.12.112~ace586d0-r1", "no build")
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"v2026.10.17.1-103-g4e32d7d 6.18.60~1de4b4e0-r1 ipq60xx",
		"v2026.10.17.1-103-g4e32d7d " + k60 + " ipq807x",
		"v2026.10.17.1-36-g06e07b2 " + k + " -",
		"v2026.10.17.1-95-g9c8b99c " + k + " ipq807x",
	}
	if got := m.Builds(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("builds:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if code, body := modulesGet(t, m, "GET", "/feeds/kmods/builds"); code != 200 || body != strings.Join(want, "\n")+"\n" {
		t.Errorf("GET builds: %d %q", code, body)
	}
}

// A build's own package feeds, beside its module feed as the builder lays
// them out, are served by the build (0315): the AP takes what its build has
// from them. Not where the module feed is not the builder's packages/, nor
// through a link, nor by kernel alone.
func TestModulesBuildFeedsBeside(t *testing.T) {
	dir := t.TempDir()
	const k = "6.18.60~79086d879a82ae4f26c0df85800d8348-r1"
	const tag = "v2026.10.17.1-128-g2554100"
	feedAt(t, dir, tag+"/feed-ipq807x/packages", k, "-121 modules")
	base := filepath.Join(dir, tag, "feed-ipq807x", "aarch64_cortex-a53", "base")
	os.MkdirAll(base, 0o755)
	os.WriteFile(filepath.Join(base, "packages.adb"), []byte("-121 base"), 0o644)
	os.WriteFile(filepath.Join(base, "usteer-2026.05.19~7532e125-r1.apk"), []byte("usteer"), 0o644)
	// One of no builder's layout, and a link.
	feedAt(t, dir, "v2026.10.17.1-126-gd1693ca/flat", "6.18.60~e182f9e2-r1", "flat")
	os.MkdirAll(filepath.Join(dir, "v2026.10.17.1-126-gd1693ca", "aarch64_cortex-a53", "base"), 0o755)
	os.WriteFile(filepath.Join(dir, "v2026.10.17.1-126-gd1693ca", "aarch64_cortex-a53", "base", "packages.adb"), []byte("not beside"), 0o644)
	linked := os.Symlink(base, filepath.Join(dir, tag, "feed-ipq807x", "aarch64_cortex-a53", "linked")) == nil
	m, err := NewModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path string
		code int
		body string
	}{
		{"/feeds/kmods/" + k + "/" + tag + "/aarch64_cortex-a53/base/packages.adb", 200, "-121 base"},
		{"/feeds/kmods/" + k + "/" + tag + "/aarch64_cortex-a53/base/usteer-2026.05.19~7532e125-r1.apk", 200, "usteer"},
		{"/feeds/kmods/" + k + "/" + tag + "/packages.adb", 200, "-121 modules"},
		{"/feeds/kmods/" + k + "/" + tag + "/aarch64_cortex-a53/packages/packages.adb", 404, ""},
		{"/feeds/kmods/" + k + "/aarch64_cortex-a53/base/packages.adb", 404, ""},
		{"/feeds/kmods/" + k + "/" + tag + "/aarch64_cortex-a53/../packages.adb", 404, ""},
		{"/feeds/kmods/6.18.60~e182f9e2-r1/v2026.10.17.1-126-gd1693ca/aarch64_cortex-a53/base/packages.adb", 404, ""},
	} {
		code, body := modulesGet(t, m, "GET", c.path)
		if code != c.code || (c.code == 200 && body != c.body) {
			t.Errorf("%s: %d %q", c.path, code, body)
		}
	}
	if linked {
		if code, _ := modulesGet(t, m, "GET", "/feeds/kmods/"+k+"/"+tag+"/aarch64_cortex-a53/linked/packages.adb"); code != 404 {
			t.Errorf("served through a link: %d", code)
		}
	}
}
