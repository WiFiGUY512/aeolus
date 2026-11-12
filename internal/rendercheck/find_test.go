package rendercheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The finder (0151) is plain shell, so that an image may carry it with none
// of the agent. Its parts that only read are run here, where there is a
// shell to run them.
func finder(t *testing.T, stdin string, env []string, args ...string) (string, error) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh here to run the finder with")
	}
	script := filepath.ToSlash(filepath.Join(agentDir, "find", "files", "usr", "sbin", "aeolus-find"))
	cmd := exec.Command(sh, append([]string{script}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	return strings.ReplaceAll(string(out), "\r\n", "\n"), err
}

// What DHCP gave says which names to ask for and of whom: aeolus in each
// domain of the search list, as given and never its parents, then aeolus
// by itself; of the servers given, and not of this AP's own resolver.
func TestFinderAsksWhatDHCPGave(t *testing.T) {
	given := filepath.Join(t.TempDir(), "resolv.conf.auto")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(given, []byte("# Interface lan\nnameserver 192.168.1.254\nnameserver 127.0.0.1\nsearch Example.com  corp.example.\texample.com bad_name\n# Interface lan6\nnameserver fd00::1\nnameserver ::1\nnameserver 192.168.1.254\n"), 0o644))
	env := []string{"AEOLUS_FIND_RESOLV=" + filepath.ToSlash(given)}

	names, err := finder(t, "", env, "names")
	must(err)
	if want := "aeolus.example.com\naeolus.corp.example\naeolus\n"; names != want {
		t.Errorf("names:\n got %q\nwant %q", names, want)
	}
	servers, err := finder(t, "", env, "servers")
	must(err)
	if want := "192.168.1.254\nfd00::1\n"; servers != want {
		t.Errorf("servers:\n got %q\nwant %q", servers, want)
	}

	// No more domains than a handful are tried, however long the list.
	must(os.WriteFile(given, []byte("search a.example b.example c.example d.example e.example f.example g.example h.example\n"), 0o644))
	names, err = finder(t, "", env, "names")
	must(err)
	if got := strings.Count(names, "\n"); got != 7 || !strings.HasSuffix(names, "aeolus.f.example\naeolus\n") {
		t.Errorf("names from a long search list: %q", names)
	}
}

// Only a private address is taken for a manager: a name out on the
// internet never is, whatever a search list leads to.
func TestFinderTakesOnlyPrivateAddresses(t *testing.T) {
	for addr, want := range map[string]bool{
		"10.0.0.1":         true,
		"10.255.255.254":   true,
		"192.168.20.60":    true,
		"172.16.0.1":       true,
		"172.31.255.1":     true,
		"169.254.10.10":    true,
		"100.64.0.1":       true,
		"100.127.9.9":      true,
		"172.15.0.1":       false,
		"172.32.0.1":       false,
		"100.63.0.1":       false,
		"100.128.0.1":      false,
		"192.169.1.1":      false,
		"8.8.8.8":          false,
		"1.1.1.1":          false,
		"192.168.1":        false,
		"192.168.1.1.1":    false,
		"192.168.1.256":    false,
		"fd00::1":          false,
		"aeolus":           false,
		"10.0.0.1; reboot": false,
		"":                 false,
	} {
		_, err := finder(t, "", nil, "private", addr)
		if got := err == nil; got != want {
			t.Errorf("private %q: %v, want %v", addr, got, want)
		}
	}
}

// A system run from memory, as an installer is, joins nothing: what it
// joined as would be gone at its next boot.
func TestFinderKnowsASystemRunFromMemory(t *testing.T) {
	for what, c := range map[string]struct {
		mounts string
		want   bool
	}{
		"an image run from memory":  {"rootfs / rootfs rw 0 0\nproc /proc proc rw 0 0\ntmpfs /tmp tmpfs rw 0 0\n", true},
		"squashfs with its overlay": {"/dev/root /rom squashfs ro 0 0\nproc /proc proc rw 0 0\ntmpfs /tmp tmpfs rw 0 0\noverlayfs:/overlay / overlay rw 0 0\n", false},
		"a root of its own":         {"/dev/root / ext4 rw 0 0\nproc /proc proc rw 0 0\n", false},
		"a root in memory over one": {"rootfs / rootfs rw 0 0\ntmpfs / tmpfs rw 0 0\n", true},
	} {
		mounts := filepath.Join(t.TempDir(), "mounts")
		if err := os.WriteFile(mounts, []byte(c.mounts), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := finder(t, "", []string{"AEOLUS_FIND_MOUNTS=" + filepath.ToSlash(mounts)}, "from-memory")
		if got := err == nil; got != c.want {
			t.Errorf("%s: from memory %v, want %v", what, got, c.want)
		}
	}
}

// The addresses are read from what nslookup says, in both the forms
// BusyBox has had; the server's own address, which comes first, is not one.
func TestFinderReadsNslookup(t *testing.T) {
	for what, c := range map[string]struct{ said, want string }{
		"BusyBox 1.37": {
			"Server:\t\t192.168.1.254\nAddress:\t192.168.1.254:53\n\nName:\taeolus.example.com\nAddress: 192.168.20.60\n\n",
			"192.168.20.60\n",
		},
		"an older BusyBox": {
			"Server:    192.168.1.254\nAddress 1: 192.168.1.254\n\nName:      aeolus.example.com\nAddress 1: 192.168.20.60 aeolus.example.com\nAddress 2: 192.168.20.61\n",
			"192.168.20.60\n192.168.20.61\n",
		},
		"both kinds of address, one twice": {
			"Server:\t\t192.168.1.254\nAddress:\t192.168.1.254:53\n\nNon-authoritative answer:\nName:\taeolus.example\nAddress: 10.1.1.1\n\nNon-authoritative answer:\nName:\taeolus.example\nAddress: fd00::60\nAddress: 10.1.1.1\n",
			"10.1.1.1\n",
		},
		"no such name": {
			"Server:\t\t192.168.1.254\nAddress:\t192.168.1.254:53\n\n** server can't find aeolus: NXDOMAIN\n",
			"",
		},
		"no answer": {";; connection timed out; no servers could be reached\n", ""},
	} {
		got, err := finder(t, c.said, nil, "answers")
		if err != nil || got != c.want {
			t.Errorf("%s: got %q (%v), want %q", what, got, err, c.want)
		}
	}
}
