package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeInitUcode is ucode as the init script asks it: whether a module loads
// (-e "require('m')"), where its .so is there; and whether a program
// compiles (-c -o /dev/null <program>), where every native module it
// imports is there.
const fakeInitUcode = `#!/bin/sh
case $1 in
-e)
	m=$(echo "$2" | sed -n "s/.*require('\\([a-z0-9_]*\\)').*/\\1/p")
	[ -e $ROOT/usr/lib/ucode/$m.so ] ;;
-c)
	echo "compile $(basename $4)" >>$ROOT/calls
	for m in $(grep -o "^import .* from '[a-z0-9_]*'" "$4" | sed "s/.* from '\\([a-z0-9_]*\\)'/\\1/"); do
		[ -e $ROOT/usr/lib/ucode/$m.so ] || exit 1
	done ;;
esac
`

// The init script starts what can start (0237). While a module the agent
// needs is missing, a program that imports it is not started, to fail and be
// started anew each five seconds, filling the log: the agent, the prober and
// the line to the manager, without math. Steering, radio resource
// management and the record of clients coming online start, and the boot run
// is started to bring the module back (0234). With nothing missing,
// everything starts, and nothing is compiled first.
func TestInitStartsWhatCanStart(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	script, err := Files.ReadFile("files/etc/init.d/aeolus")
	if err != nil {
		t.Fatal(err)
	}
	daemons := []string{"aeolus-agent", "aeolus-prober", "aeolus-rrm", "aeolus-journey", "aeolus-steer", "aeolus-live", "aeolus-ble"}
	run := func(missing ...string) (string, error) {
		root := t.TempDir()
		r := filepath.ToSlash(root)
		s := strings.NewReplacer("/usr/lib/ucode/", r+"/usr/lib/ucode/", "/usr/sbin/", r+"/usr/sbin/",
			"/usr/libexec/", r+"/usr/libexec/", "/sbin/logread", r+"/sbin/logread", "/usr/bin/hci", r+"/usr/bin/hci").Replace(string(script))
		bin := filepath.Join(root, "bin")
		for _, d := range []string{bin, root + "/usr/lib/ucode", root + "/usr/sbin", root + "/sbin"} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		files := map[string]string{
			root + "/aeolus":       s,
			root + "/run.sh":       "procd_open_instance() { echo \"open ${1:-agent}\" >>$ROOT/calls; }\nprocd_set_param() { :; }\nprocd_close_instance() { :; }\n. \"$1\"\nstart_service\nwait\n",
			root + "/sbin/logread": "#!/bin/sh\n",
			bin + "/ucode":         fakeInitUcode,
			bin + "/uci":           fakeUci,
			bin + "/setsid":        "#!/bin/sh\necho \"setsid $(basename $1) $2\" >>$ROOT/calls\n",
		}
		for _, d := range daemons {
			b, err := Files.ReadFile("files/usr/sbin/" + d)
			if err != nil {
				t.Fatal(err)
			}
			files[root+"/usr/sbin/"+d] = string(b)
		}
		for _, m := range []string{"fs", "uci", "ubus", "uloop", "uclient", "math", "log", "digest", "socket", "nl80211"} {
			if !contains(missing, m) {
				files[root+"/usr/lib/ucode/"+m+".so"] = ""
			}
		}
		for p, body := range files {
			if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command("sh", root+"/run.sh", root+"/aeolus")
		cmd.Env = []string{"ROOT=" + root, "PATH=" + shellPath(sh, bin)}
		out, err := cmd.CombinedOutput()
		b, _ := os.ReadFile(root + "/calls")
		if err != nil {
			t.Logf("%s", out)
		}
		return string(b), err
	}

	c, err := run("math")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"setsid aeolus-packages boot", "open rrm", "open journey", "open steer"} {
		if !strings.Contains(c, want+"\n") {
			t.Errorf("without math, missing %q:\n%s", want, c)
		}
	}
	for _, not := range []string{"open agent", "open keys", "open prober", "open live"} {
		if strings.Contains(c, not+"\n") {
			t.Errorf("without math, %q, which would fail as it starts:\n%s", not, c)
		}
	}

	c, err = run()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"open agent", "open keys", "open prober", "open rrm", "open journey", "open steer", "open live"} {
		if !strings.Contains(c, want+"\n") {
			t.Errorf("with every module, missing %q:\n%s", want, c)
		}
	}
	if strings.Contains(c, "compile") || strings.Contains(c, "setsid") {
		t.Errorf("with every module, compiled first or started the boot run:\n%s", c)
	}
}
