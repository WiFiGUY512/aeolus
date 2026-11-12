package deploy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The Proxmox VE tools, stood in for: each container is a directory under
// $PCT_STATE, with its config, a root that pct push copies into, and what
// the installer was run with. Every call is logged.
var pveStandIns = map[string]string{
	"pct": `#!/bin/sh
S=$PCT_STATE
echo "pct $*" >>"$S/log"
cmd=$1
shift
case $cmd in
config) [ -f "$S/ct$1/config" ] && cat "$S/ct$1/config" ;;
status) echo "status: running" ;;
start) : ;;
create)
	id=$1
	mkdir -p "$S/ct$id/root"
	tags=
	while [ $# -gt 0 ]; do
		[ "$1" != --tags ] || tags=$2
		shift
	done
	printf 'tags: %s\n' "$tags" >"$S/ct$id/config"
	;;
push)
	mkdir -p "$S/ct$1/root$(dirname "$3")"
	cp "$2" "$S/ct$1/root$3"
	;;
exec)
	id=$1
	R=$S/ct$id/root
	shift
	[ "$1" != -- ] || shift
	case $1 in
	getent | install) : ;;
	test) [ -f "$R$3" ] ;;
	git) exit 1 ;;
	find) rm -f "$R"/root/.aeolus/aeolus-*.tar.gz ;;
	rm) rm -f "$R$3" ;;
	/usr/local/sbin/aeolus-install)
		shift
		printf '%s\n' "$@" >"$S/ct$id/install-args"
		cat >"$S/ct$id/install-stdin"
		;;
	*) echo "pct exec: no stand-in for $1" >&2; exit 9 ;;
	esac
	;;
*) echo "pct: no stand-in for $cmd" >&2; exit 9 ;;
esac
`,
	"pveam": `#!/bin/sh
echo "pveam $*" >>"$PCT_STATE/log"
[ "$1" != list ] || printf 'NAME SIZE\nlocal:vztmpl/debian-13-standard_13.1-2_amd64.tar.zst 120.00MB\n'
`,
	"pvesm": `#!/bin/sh
echo "Name Type Status Total Used Available %"
case $* in
*vztmpl*) echo "local dir active 100 10 90 10%" ;;
*rootdir*) printf 'local dir active 100 10 90 10%%\nlocal-zfs zfspool active 1000 100 900 10%%\n' ;;
esac
`,
	"pvesh": "#!/bin/sh\necho 123\n",
	"id":    "#!/bin/sh\necho 0\n",
}

// proxmoxRig is a Proxmox VE host stood in for, with a release made by
// release.sh: its files in a directory, the amd64 one unpacked beside them.
type proxmoxRig struct {
	t                      *testing.T
	state, bin, net, files string
	unpacked, amd64, arm64 string
	installer              []byte
	checkout               *releaseRig
}

func newProxmoxRig(t *testing.T) *proxmoxRig {
	t.Helper()
	rr := newReleaseRig(t)
	files := filepath.Join(rr.root, "files")
	if msg, err := rr.run("--out", slash(files), "v1.2.3"); err != nil {
		t.Fatalf("release: %v\n%s", err, msg)
	}
	r := &proxmoxRig{t: t, state: filepath.Join(rr.root, "pve"), bin: filepath.Join(rr.root, "pvebin"), net: filepath.Join(rr.root, "net"),
		files: filepath.Join(files, "v1.2.3"), checkout: rr}
	r.amd64 = filepath.Join(r.files, "aeolus-v1.2.3-linux-amd64.tar.gz")
	r.arm64 = filepath.Join(r.files, "aeolus-v1.2.3-linux-arm64.tar.gz")
	r.unpacked = filepath.Join(r.files, "aeolus-v1.2.3-linux-amd64")
	cmd := exec.Command("tar", "-xzf", filepath.Base(r.amd64))
	cmd.Dir = r.files // a path with C: in it is a remote host to tar
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unpack: %v\n%s", err, out)
	}
	var err error
	if r.installer, err = os.ReadFile(filepath.Join(r.unpacked, "deploy", "install.sh")); err != nil {
		t.Fatal(err)
	}
	// The release rig's repository has no proxmox.sh: this one is put in,
	// as a release has it.
	helper, err := os.ReadFile("proxmox.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.unpacked, "deploy", "proxmox.sh"), helper, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{r.state, r.bin, filepath.Join(r.net, "vmbr0", "bridge")} {
		os.MkdirAll(d, 0o755)
	}
	for name, body := range pveStandIns {
		if err := os.WriteFile(filepath.Join(r.bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// run runs a proxmox.sh, as root on the host stood in for.
func (r *proxmoxRig) run(script string, args ...string) (string, error) {
	r.t.Helper()
	os.WriteFile(filepath.Join(r.state, "log"), nil, 0o644)
	cmd := exec.Command("sh", append([]string{slash(script)}, args...)...)
	cmd.Dir = r.state
	cmd.Env = append(os.Environ(), "PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PCT_STATE="+slash(r.state), "AEOLUS_SYS_NET="+slash(r.net), "AEOLUS_RAW=", "AEOLUS_REPO=")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *proxmoxRig) script() string { return filepath.Join(r.unpacked, "deploy", "proxmox.sh") }

func (r *proxmoxRig) log() string {
	b, _ := os.ReadFile(filepath.Join(r.state, "log"))
	return string(b)
}

// ct reads a file of container id: its root's, or the stand-in's own.
func (r *proxmoxRig) ct(id, path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.state, "ct"+id, filepath.FromSlash(path)))
}

func (r *proxmoxRig) args(id string) []string {
	r.t.Helper()
	b, err := r.ct(id, "install-args")
	if err != nil {
		r.t.Fatalf("the installer did not run in CT %s: %v", id, err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func has(args []string, want ...string) bool {
	for i := range args {
		if i+len(want) <= len(args) && strings.Join(args[i:i+len(want)], "\x00") == strings.Join(want, "\x00") {
			return true
		}
	}
	return false
}

// Run from a release file unpacked (0316), the helper makes the container
// and gives its installer that release: the tarball it was unpacked from,
// and the SHA256SUMS beside it, into /root/.aeolus, and the release's own
// installer. Nothing is bundled and no repository named.
func TestProxmoxFromRelease(t *testing.T) {
	r := newProxmoxRig(t)
	out, err := r.run(r.script(), "--yes", "--org", "distro", "--admin", "first-user")
	if err != nil {
		t.Fatalf("proxmox.sh: %v\n%s", err, out)
	}
	if log := r.log(); !strings.Contains(log, "pct create 123 local:vztmpl/debian-13-standard_13.1-2_amd64.tar.zst") || !strings.Contains(log, "--tags aeolus") {
		t.Fatalf("no container made as Aeolus's: %s", log)
	}
	if b, _ := r.ct("123", "root/usr/local/sbin/aeolus-install"); !bytes.Equal(b, r.installer) {
		t.Error("the installer pushed is not the release's own")
	}
	want, _ := os.ReadFile(r.amd64)
	if b, _ := r.ct("123", "root/root/.aeolus/aeolus-v1.2.3-linux-amd64.tar.gz"); !bytes.Equal(b, want) {
		t.Error("the release file pushed is not the one it was unpacked from")
	}
	sums, _ := os.ReadFile(filepath.Join(r.files, "SHA256SUMS"))
	if b, _ := r.ct("123", "root/root/.aeolus/SHA256SUMS"); !bytes.Equal(b, sums) {
		t.Error("the SHA256SUMS beside it was not pushed")
	}
	args := r.args("123")
	if !has(args, "--from", "/root/.aeolus/aeolus-v1.2.3-linux-amd64.tar.gz") || !has(args, "--org", "distro") || !has(args, "--admin", "first-user") || !has(args, "--token") {
		t.Errorf("the installer was given %q", args)
	}
	if has(args, "--repo") {
		t.Errorf("a repository was named: %q", args)
	}
	if _, err := r.ct("123", "root/opt/aeolus/aeolus.bundle"); err == nil {
		t.Error("a bundle was pushed")
	}
	if !strings.Contains(out, "from a newer release file unpacked") {
		t.Errorf("the last line does not say how to upgrade:\n%s", out)
	}
}

// Unpacked with no tarball beside it, the release is packed again; named by
// its URL, the container fetches it and nothing is copied in.
func TestProxmoxReleaseElsewhere(t *testing.T) {
	r := newProxmoxRig(t)
	moved := filepath.Join(filepath.Dir(r.files), "alone", "aeolus-v1.2.3-linux-amd64")
	os.MkdirAll(filepath.Dir(moved), 0o755)
	if err := os.Rename(r.unpacked, moved); err != nil {
		t.Fatal(err)
	}
	r.unpacked = moved
	if out, err := r.run(r.script(), "--yes", "--org", "distro", "--admin", "first-user"); err != nil {
		t.Fatalf("proxmox.sh: %v\n%s", err, out)
	}
	packed := filepath.Join(r.state, "ct123", "root", "root", ".aeolus", "aeolus-v1.2.3-linux-amd64.tar.gz")
	tl := exec.Command("tar", "-tzf", filepath.Base(packed))
	tl.Dir = filepath.Dir(packed)
	list, err := tl.CombinedOutput()
	if err != nil || !strings.Contains(string(list), "aeolus-v1.2.3-linux-amd64/RELEASE") {
		t.Fatalf("packed again: %v\n%s", err, list)
	}
	if _, err := r.ct("123", "root/root/.aeolus/SHA256SUMS"); err == nil {
		t.Error("a SHA256SUMS was pushed with a file packed again")
	}

	os.RemoveAll(filepath.Join(r.state, "ct123"))
	url := "https://releases.example/aeolus/v1.2.3/aeolus-v1.2.3-linux-amd64.tar.gz"
	if out, err := r.run(r.script(), "--yes", "--org", "distro", "--admin", "first-user", "--", "--from", url); err != nil {
		t.Fatalf("by URL: %v\n%s", err, out)
	}
	if args := r.args("123"); !has(args, "--from", url) {
		t.Errorf("by URL, the installer was given %q", args)
	}
	if _, err := os.Stat(filepath.Join(r.state, "ct123", "root", "root", ".aeolus")); err == nil {
		t.Error("by URL, something was copied in")
	}
}

// Run again with --ctid, a container it made is upgraded: no container
// made, no Org asked for, the older release file taken away and the new
// one given.
func TestProxmoxUpgradeFromRelease(t *testing.T) {
	r := newProxmoxRig(t)
	ct := filepath.Join(r.state, "ct200")
	for path, body := range map[string]string{
		"config":                        "tags: aeolus\n",
		"root/var/lib/aeolus/aeolus.db": "",
		"root/root/.aeolus/aeolus-v1.2.2-linux-amd64.tar.gz": "older",
	} {
		p := filepath.Join(ct, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	if out, err := r.run(r.script(), "--ctid", "200"); err != nil {
		t.Fatalf("proxmox.sh --ctid 200: %v\n%s", err, out)
	}
	if strings.Contains(r.log(), "pct create") {
		t.Error("a container was made")
	}
	args := r.args("200")
	if !has(args, "--from", "/root/.aeolus/aeolus-v1.2.3-linux-amd64.tar.gz") || has(args, "--token") || has(args, "--org") {
		t.Errorf("the installer was given %q", args)
	}
	if _, err := r.ct("200", "root/root/.aeolus/aeolus-v1.2.2-linux-amd64.tar.gz"); err == nil {
		t.Error("the older release file was left")
	}
}

// What cannot be installed is refused before anything is made or copied.
func TestProxmoxRefuses(t *testing.T) {
	r := newProxmoxRig(t)
	os.MkdirAll(filepath.Join(r.state, "ct300"), 0o755)
	os.WriteFile(filepath.Join(r.state, "ct300", "config"), []byte("tags: other\n"), 0o644)
	alone := filepath.Join(filepath.Dir(r.files), "proxmox.sh")
	b, _ := os.ReadFile(r.script())
	os.WriteFile(alone, b, 0o755)
	for _, c := range []struct {
		script string
		args   []string
		want   string
	}{
		{r.script(), []string{"--yes", "--org", "distro", "--admin", "first-user", "--", "--from", slash(r.arm64)}, "built for arm64, and this host is amd64"},
		{r.script(), []string{"--yes", "--org", "distro", "--admin", "first-user", "--", "--version", "v1.2.3"}, "build from git, which a release file is not"},
		{r.script(), []string{"--yes", "--org", "distro", "--admin", "first-user", "--", "--from", slash(r.amd64), "--version", "v1.2.3"}, "for a build from git"},
		{r.script(), []string{"--yes", "--org", "distro", "--admin", "first-user", "--", "--from", "/nowhere/aeolus.tar.gz"}, "no release file /nowhere/aeolus.tar.gz"},
		{r.script(), []string{"--ctid", "300"}, "was not made by this script"},
		{alone, []string{"--yes", "--org", "distro", "--admin", "first-user"}, "run it from a release file unpacked"},
	} {
		out, err := r.run(c.script, c.args...)
		if err == nil || !strings.Contains(out, c.want) {
			t.Errorf("%v: %v, %q; want %q", c.args, err, out, c.want)
		}
		if log := r.log(); strings.Contains(log, "pct create") || strings.Contains(log, "pct push") {
			t.Errorf("%v: something was made or copied:\n%s", c.args, log)
		}
	}
}

// From a checkout, as before: the checkout goes in as a git bundle the
// installer builds from; with --repo, a deploy key and host key go in
// instead, readable by root alone, and no bundle.
func TestProxmoxFromCheckout(t *testing.T) {
	r := newProxmoxRig(t)
	repo := r.checkout.repo
	helper, _ := os.ReadFile("proxmox.sh")
	os.WriteFile(filepath.Join(repo, "deploy", "proxmox.sh"), helper, 0o755)
	script := filepath.Join(repo, "deploy", "proxmox.sh")
	if out, err := r.run(script, "--yes", "--org", "distro", "--admin", "first-user"); err != nil {
		t.Fatalf("proxmox.sh: %v\n%s", err, out)
	}
	if _, err := r.ct("123", "root/opt/aeolus/aeolus.bundle"); err != nil {
		t.Errorf("no bundle pushed: %v", err)
	}
	if args := r.args("123"); !has(args, "--repo", "/opt/aeolus/aeolus.bundle") || has(args, "--from") {
		t.Errorf("the installer was given %q", args)
	}

	os.RemoveAll(filepath.Join(r.state, "ct123"))
	key := filepath.Join(r.state, "deploy_ed25519")
	known := filepath.Join(r.state, "known_hosts")
	os.WriteFile(key, []byte("a key"), 0o600)
	os.WriteFile(known, []byte("a host key"), 0o644)
	out, err := r.run(script, "--yes", "--org", "distro", "--admin", "first-user", "--", "--repo", "ssh://git@forge.example/aeolus.git", "--ssh-key", slash(key), "--known-hosts", slash(known))
	if err != nil {
		t.Fatalf("with --repo: %v\n%s", err, out)
	}
	args := r.args("123")
	if !has(args, "--repo", "ssh://git@forge.example/aeolus.git") || !has(args, "--ssh-key", "/root/.aeolus/ssh-key") || !has(args, "--known-hosts", "/root/.aeolus/known-hosts") {
		t.Errorf("with --repo, the installer was given %q", args)
	}
	if b, _ := r.ct("123", "root/root/.aeolus/ssh-key"); string(b) != "a key" {
		t.Error("the deploy key was not copied in")
	}
	if !strings.Contains(r.log(), "pct push 123 ") || !strings.Contains(r.log(), "/root/.aeolus/ssh-key --perms 0600") {
		t.Errorf("the key was not pushed readable by root alone:\n%s", r.log())
	}
	if _, err := r.ct("123", "root/opt/aeolus/aeolus.bundle"); err == nil {
		t.Error("with --repo, a bundle was pushed")
	}
}
