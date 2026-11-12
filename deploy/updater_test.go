// Package deploy holds the scripts that put a manager up and keep it
// current; its tests run them, as their hosts do, against stand-ins.
package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// updaterRig is the updater (0183) with a repository of releases to fetch
// from, a manager whose version a stand-in binary says, and an installer
// that "installs" a release by changing what that binary says.
type updaterRig struct {
	t                             *testing.T
	dir, src, bin, local, release string
}

func newUpdaterRig(t *testing.T, running string, tags ...string) *updaterRig {
	t.Helper()
	for _, tool := range []string{"sh", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	root := t.TempDir()
	r := &updaterRig{t: t, dir: filepath.Join(root, "update"), src: filepath.Join(root, "src"), bin: filepath.Join(root, "bin"), local: filepath.Join(root, "local"), release: filepath.Join(root, "release")}
	for _, d := range []string{r.dir, r.bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	origin := filepath.Join(root, "origin.git")
	r.git(root, "init", "-q", "--bare", origin)
	r.git(root, "clone", "-q", origin, r.local)
	r.git(r.local, "config", "user.email", "t@example.com")
	r.git(r.local, "config", "user.name", "t")
	for _, tag := range tags {
		name, msg, _ := strings.Cut(tag, "|")
		if err := os.WriteFile(filepath.Join(r.local, "f"), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		r.git(r.local, "add", "f")
		r.git(r.local, "commit", "-q", "-m", "release "+name)
		if msg == "" {
			msg = name
		}
		r.git(r.local, "tag", "-a", name, "-m", msg)
	}
	r.git(r.local, "push", "-q", "origin", "HEAD", "--tags")
	r.git(root, "clone", "-q", origin, r.src)
	r.setRunning(running)
	// The installer: says what it was asked, and puts that release in.
	install := "#!/bin/sh\necho \"installing $2\"\nprintf '#!/bin/sh\\necho %s-abc1234\\n' \"$2\" >" + slash(r.bin) + "/aeolus\nchmod +x " + slash(r.bin) + "/aeolus\n"
	if err := os.WriteFile(filepath.Join(root, "install"), []byte(install), 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

func slash(p string) string { return filepath.ToSlash(p) }

func (r *updaterRig) git(dir string, args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *updaterRig) setRunning(version string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.bin, "aeolus"), []byte("#!/bin/sh\necho "+version+"\n"), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

// ask leaves a request, as the manager does, and runs the updater.
func (r *updaterRig) ask(req string) (string, error) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, "request"), []byte(req+"\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	return r.run()
}

func (r *updaterRig) run() (string, error) {
	cmd := exec.Command("sh", "updater.sh")
	cmd.Env = append(os.Environ(),
		"AEOLUS_UPDATE_DIR="+slash(r.dir), "AEOLUS_SRC="+slash(r.src), "AEOLUS_BIN="+slash(r.bin),
		"AEOLUS_INSTALL="+slash(filepath.Join(filepath.Dir(r.dir), "install")), "AEOLUS_UPDATE=/nonexistent",
		"AEOLUS_RELEASE="+slash(r.release))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *updaterRig) read(name string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, name))
	if err != nil {
		r.t.Fatalf("%s: %v", name, err)
	}
	return string(b)
}

func (r *updaterRig) status() map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(r.read("status"), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

// A check fetches the release tags, says the newest, and writes what each
// release newer than the one running says of itself, the newest first,
// stopping at the one running (0217).
func TestUpdaterCheck(t *testing.T) {
	r := newUpdaterRig(t, "v0.166.0-8d497f4", "v0.166.0|Running now", "v0.167.0|Radios off the air\n\n- radio-down", "v0.168.0|Maintenance")
	if out, err := r.ask("check"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	st := r.status()
	if st["newest"] != "v0.168.0" || st["state"] != "idle" || st["error"] != "" || st["running"] != "v0.166.0-8d497f4" || st["checked"] == "" {
		t.Fatalf("status = %v", st)
	}
	// Each release's block, the newest first; git's blank lines after a
	// message are its own, and the manager trims them.
	notes := r.read("notes")
	newer := strings.Index(notes, "@@ v0.168.0\nMaintenance\n")
	older := strings.Index(notes, "@@ v0.167.0\nRadios off the air\n\n- radio-down\n")
	if newer != 0 || older <= newer || strings.Contains(notes, "v0.166.0") {
		t.Fatalf("notes = %q", notes)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "request")); !os.IsNotExist(err) {
		t.Fatalf("the request was left: %v", err)
	}
	// Nothing newer: no notes.
	r.setRunning("v0.168.0-1111111")
	r.ask("check")
	if notes := r.read("notes"); notes != "" {
		t.Fatalf("nothing newer, notes = %q", notes)
	}
}

// A check says the newest release on the stable channel too (0282): one
// promoted by a tag stable/vX.Y.Z on the very commit vX.Y.Z is; a stable
// tag on another commit promotes nothing, and none at all says none.
func TestUpdaterStable(t *testing.T) {
	r := newUpdaterRig(t, "v0.166.0-8d497f4", "v0.166.0", "v0.167.0", "v0.168.0")
	r.ask("check")
	if st := r.status(); st["newest"] != "v0.168.0" || st["stable"] != "" {
		t.Fatalf("none promoted: status = %v", st)
	}
	r.git(r.local, "tag", "stable/v0.167.0", "v0.167.0^{commit}")
	r.git(r.local, "tag", "stable/v0.168.0", "v0.167.0^{commit}") // a mistake: on another commit
	r.git(r.local, "push", "-q", "origin", "--tags")
	if out, err := r.ask("check"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if st := r.status(); st["newest"] != "v0.168.0" || st["stable"] != "v0.167.0" {
		t.Fatalf("promoted: status = %v", st)
	}
	// An update says it too.
	r.ask("update v0.167.0")
	if st := r.status(); st["state"] != "updated" || st["stable"] != "v0.167.0" {
		t.Fatalf("after an update: status = %v", st)
	}
}

// An update installs the release asked for, says how it went, and writes
// the notes anew from what now runs.
func TestUpdaterUpdate(t *testing.T) {
	r := newUpdaterRig(t, "v0.166.0-8d497f4", "v0.166.0", "v0.167.0|Radios off the air")
	if out, err := r.ask("update v0.167.0"); err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	st := r.status()
	if st["state"] != "updated" || st["tag"] != "v0.167.0" || st["running"] != "v0.167.0-abc1234" || st["error"] != "" || st["newest"] != "v0.167.0" {
		t.Fatalf("status = %v", st)
	}
	if log := r.read("update.log"); !strings.Contains(log, "installing v0.167.0") {
		t.Fatalf("update.log = %q", log)
	}
	if notes := r.read("notes"); notes != "" {
		t.Fatalf("after the update, notes = %q", notes)
	}
}

// What is not a check or an update to a release the repository has is
// refused, and a request that is a link is not read.
func TestUpdaterRefuses(t *testing.T) {
	r := newUpdaterRig(t, "v0.166.0-8d497f4", "v0.166.0")
	for req, want := range map[string]string{
		"update v0.999.0":         "refused: the repository has no release v0.999.0",
		"update ../../etc/x":      "refused: a request is check, or update and a release",
		"update v1.2.3; reboot":   "refused: not a release",
		"update main":             "refused: a request is check, or update and a release",
		"rm -rf /":                "refused: a request is check, or update and a release",
		"update v0.166.0-rc1":     "refused: not a release",
		"update v0.166.0\nreboot": "",
	} {
		r.ask(req)
		st := r.status()
		if want == "" {
			// Only the first line is read: an update to v0.166.0, the one running.
			if st["error"] != "" || st["tag"] != "v0.166.0" {
				t.Errorf("%q: status = %v", req, st)
			}
			continue
		}
		if st["state"] != "failed" || st["error"] != want {
			t.Errorf("%q: state %q, error %q, want %q", req, st["state"], st["error"], want)
		}
	}
	// A request that is a link is taken away, not followed.
	target := filepath.Join(t.TempDir(), "elsewhere")
	os.WriteFile(target, []byte("update v0.166.0\n"), 0o644)
	if err := os.Symlink(target, filepath.Join(r.dir, "request")); err != nil {
		t.Skip("no symlinks here")
	}
	before := r.read("status")
	r.run()
	if _, err := os.Lstat(filepath.Join(r.dir, "request")); !os.IsNotExist(err) {
		t.Fatalf("the link was left: %v", err)
	}
	if r.read("status") != before {
		t.Fatal("a linked request was acted on")
	}
}

// A manager put up from a release file (0316) has no repository to ask: a
// check says how a newer release goes in, and an update is refused with the
// same, neither fetching anything nor running the installer.
func TestUpdaterReleaseFile(t *testing.T) {
	r := newUpdaterRig(t, "v0.166.0-8d497f4", "v0.166.0", "v0.167.0")
	if err := os.WriteFile(r.release, []byte("name=aeolus\nversion=v0.166.0-8d497f4\nfrom=/root/aeolus-v0.166.0-linux-amd64.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const want = "this manager was put up from a release file: a newer one goes in with aeolus-install --from <file or URL>"
	if out, err := r.ask("check"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if st := r.status(); st["state"] != "idle" || st["error"] != want || st["newest"] != "" || st["checked"] == "" {
		t.Fatalf("check: status = %v", st)
	}
	if out, err := r.ask("update v0.167.0"); err == nil {
		t.Fatalf("update: no failure\n%s", out)
	}
	st := r.status()
	if st["state"] != "failed" || st["error"] != want || st["running"] != "v0.166.0-8d497f4" {
		t.Fatalf("update: status = %v", st)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "update.log")); !os.IsNotExist(err) {
		t.Fatalf("the installer ran: %v", err)
	}
	// Anything else is refused as before.
	r.ask("rm -rf /")
	if st := r.status(); st["error"] != "refused: a request is check, or update and a release" {
		t.Fatalf("a request that is neither: status = %v", st)
	}
	// Built from git again, the record is gone and the updater fetches.
	os.Remove(r.release)
	r.ask("check")
	if st := r.status(); st["newest"] != "v0.167.0" || st["error"] != "" {
		t.Fatalf("from git again: status = %v", st)
	}
}
