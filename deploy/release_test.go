package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// releaseRig is a repository with two tags, one whose installer takes
// release files (0316) and one from before, and a go that only says what it
// was asked to build.
type releaseRig struct {
	t                *testing.T
	root, repo, fake string
	installer        string
}

func newReleaseRig(t *testing.T) *releaseRig {
	t.Helper()
	for _, tool := range []string{"sh", "git", "tar", "gzip", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	root := t.TempDir()
	r := &releaseRig{t: t, root: root, repo: filepath.Join(root, "repo"), fake: filepath.Join(root, "bin", "go")}
	installer, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	r.installer = string(installer)
	script, err := os.ReadFile("release.sh")
	if err != nil {
		t.Fatal(err)
	}
	// The stand-in go: GOVERSION, and a "manager" that says its version and
	// target, built only where go.mod is.
	r.write(r.fake, `#!/bin/sh
case $1 in
env) echo go1.27.0 ;;
build)
	out= v=
	while [ $# -gt 0 ]; do
		case $1 in
		-o) out=$2; shift ;;
		-ldflags) v=${2##*main.version=}; shift ;;
		esac
		shift
	done
	[ -f go.mod ] || exit 3
	printf '#!/bin/sh\necho %s %s/%s\n' "$v" "$GOOS" "$GOARCH" >"$out"
	;;
*) exit 2 ;;
esac
`)
	os.Chmod(r.fake, 0o755)
	r.git(root, "init", "-q", r.repo)
	r.git(r.repo, "config", "user.email", "t@example.com")
	r.git(r.repo, "config", "user.name", "t")
	r.write(filepath.Join(r.repo, "go.mod"), "module example.com/aeolus\n\ngo 1.27.0\n")
	r.write(filepath.Join(r.repo, "cmd", "aeolus", "main.go"), "package main\n\nfunc main() {}\n")
	r.write(filepath.Join(r.repo, "deploy", "release.sh"), string(script))
	r.write(filepath.Join(r.repo, "deploy", "aeolus.service"), "[Unit]\nDescription=Aeolus manager API\n")
	r.write(filepath.Join(r.repo, "deploy", "aeolus.sh"), "#!/bin/sh\nexec /opt/aeolus/bin/aeolus \"$@\"\n")
	r.write(filepath.Join(r.repo, "docs", "install.md"), "# Installing Aeolus\n")
	// The release from before: its installer builds from git alone.
	r.write(filepath.Join(r.repo, "deploy", "install.sh"), "#!/bin/sh\n# Stands up an Aeolus manager\n")
	r.git(r.repo, "add", "-A")
	r.git(r.repo, "commit", "-q", "-m", "before")
	r.git(r.repo, "tag", "-a", "v0.0.1", "-m", "v0.0.1")
	r.write(filepath.Join(r.repo, "deploy", "install.sh"), r.installer)
	r.git(r.repo, "add", "-A")
	r.git(r.repo, "commit", "-q", "-m", "release files")
	r.git(r.repo, "tag", "-a", "v1.2.3", "-m", "v1.2.3")
	// What the checkout has besides never goes in.
	r.write(filepath.Join(r.repo, "docs", "install.md"), "# not committed\n")
	return r
}

func (r *releaseRig) write(path, body string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *releaseRig) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// run runs the repository's release.sh with these arguments.
func (r *releaseRig) run(args ...string) (string, error) {
	cmd := exec.Command("sh", append([]string{"deploy/release.sh"}, args...)...)
	cmd.Dir = r.repo
	cmd.Env = append(os.Environ(), "AEOLUS_GO="+slash(r.fake))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// unpack reads a release file: each entry's name and contents.
func (r *releaseRig) unpack(path string) map[string]string {
	r.t.Helper()
	f, err := os.Open(path)
	if err != nil {
		r.t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		r.t.Fatalf("%s: %v", path, err)
	}
	tr := tar.NewReader(z)
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			r.t.Fatalf("%s: %v", path, err)
		}
		b, _ := io.ReadAll(tr)
		out[strings.TrimSuffix(h.Name, "/")] = string(b)
	}
}

func keys(release string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(release, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

// A release makes a file for each target, from the tag and nothing else,
// with the manager built as the installer builds it, a RELEASE that says
// what it is, the scripts that install it, and SHA256SUMS of them all
// (0316). Made again, the files are the same to the byte.
func TestRelease(t *testing.T) {
	r := newReleaseRig(t)
	out := filepath.Join(r.root, "out")
	if msg, err := r.run("--out", slash(out), "v1.2.3"); err != nil {
		t.Fatalf("release: %v\n%s", err, msg)
	}
	commit := r.git(r.repo, "rev-parse", "v1.2.3^{commit}")
	short := r.git(r.repo, "rev-parse", "--short", "v1.2.3^{commit}")
	dest := filepath.Join(out, "v1.2.3")
	var sums strings.Builder
	files := map[string][]byte{}
	for _, arch := range []string{"amd64", "arm64"} {
		name := "aeolus-v1.2.3-linux-" + arch + ".tar.gz"
		b, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = b
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(b), name)
		top := "aeolus-v1.2.3-linux-" + arch + "/"
		got := r.unpack(filepath.Join(dest, name))
		for entry := range got {
			if entry+"/" != top && !strings.HasPrefix(entry, top) {
				t.Errorf("%s: %s is outside %s", name, entry, top)
			}
		}
		rel := keys(got[top+"RELEASE"])
		want := map[string]string{"name": "aeolus", "version": "v1.2.3-" + short, "tag": "v1.2.3", "commit": commit, "os": "linux", "arch": arch, "go": "go1.27.0"}
		for k, v := range want {
			if rel[k] != v {
				t.Errorf("%s: RELEASE %s = %q, want %q", name, k, rel[k], v)
			}
		}
		if rel["time"] == "" {
			t.Errorf("%s: RELEASE has no time", name)
		}
		if m := got[top+"aeolus"]; !strings.Contains(m, "v1.2.3-"+short+" linux/"+arch) {
			t.Errorf("%s: the manager is %q", name, m)
		}
		if got[top+"deploy/install.sh"] != r.installer {
			t.Errorf("%s: deploy/install.sh is not the tag's", name)
		}
		if got[top+"docs/install.md"] != "# Installing Aeolus\n" {
			t.Errorf("%s: docs/install.md = %q, not the tag's", name, got[top+"docs/install.md"])
		}
		if _, ok := got[top+"deploy/aeolus.service"]; !ok {
			t.Errorf("%s: no deploy/aeolus.service", name)
		}
		if _, ok := got[top+"deploy/proxmox.sh"]; ok {
			t.Errorf("%s: a file the tag has not is in it", name)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "SHA256SUMS")); string(b) != sums.String() {
		t.Fatalf("SHA256SUMS = %q, want %q", b, sums.String())
	}
	// Made again: the same bytes.
	if msg, err := r.run("--out", slash(out), "v1.2.3"); err != nil {
		t.Fatalf("again: %v\n%s", err, msg)
	}
	for name, b := range files {
		again, _ := os.ReadFile(filepath.Join(dest, name))
		if !bytes.Equal(again, b) {
			t.Errorf("%s differs when made again", name)
		}
	}
	// One target: its file alone, and its sum.
	one := filepath.Join(r.root, "one")
	if msg, err := r.run("--out", slash(one), "--arch", "arm64", "v1.2.3"); err != nil {
		t.Fatalf("--arch arm64: %v\n%s", err, msg)
	}
	entries, _ := os.ReadDir(filepath.Join(one, "v1.2.3"))
	if len(entries) != 2 {
		t.Fatalf("--arch arm64 made %d files", len(entries))
	}
	if b, _ := os.ReadFile(filepath.Join(one, "v1.2.3", "SHA256SUMS")); !strings.HasSuffix(strings.TrimSpace(string(b)), "aeolus-v1.2.3-linux-arm64.tar.gz") || strings.Count(string(b), "\n") != 1 {
		t.Fatalf("--arch arm64: SHA256SUMS = %q", b)
	}
}

// What cannot be a release is refused before anything is made.
func TestReleaseRefuses(t *testing.T) {
	r := newReleaseRig(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"v0.0.1"}, "takes no release file"},
		{[]string{"v9.9.9"}, "no tag or commit v9.9.9"},
		{[]string{"--arch", "mips", "v1.2.3"}, "no target mips"},
		{[]string{}, "name the release"},
		{[]string{"../v1.2.3"}, "cannot name a file"},
		{[]string{"--sign", "v1.2.3"}, "no option --sign"},
	} {
		out := filepath.Join(r.root, "refused")
		msg, err := r.run(append([]string{"--out", slash(out)}, c.args...)...)
		if err == nil || !strings.Contains(msg, c.want) {
			t.Errorf("%v: %v, %q; want %q", c.args, err, msg, c.want)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Errorf("%v: something was made", c.args)
		}
	}
}
