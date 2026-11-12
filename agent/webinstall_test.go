package agent

import (
	"io/fs"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// placesCase is the installer's case of the places it puts files: the
// patterns it lets through, up to the one that refuses the rest.
var placesCase = regexp.MustCompile(`(?s)case \$p in\n(.*?)\t\t\*\) die "the manifest names \$p, not a place of Aeolus's"`)

// The installer puts every file the manager hands it in place (0083): each
// of the agent's files, and LuCI's page. It refuses a manifest naming a
// place it doesn't know, before it puts anything in place, so a file the
// agent gained and the installer didn't stops every new AP from joining
// (2026-10-30: aeolus-early, 0248, stopped ce7f's install). It still refuses
// a place that is not Aeolus's.
func TestInstallerTakesEveryFile(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	m := placesCase.FindSubmatch(WebInstall)
	if m == nil {
		t.Fatal("the installer has no case of the places it puts files")
	}
	var places []string
	for _, tree := range []struct {
		fsys fs.FS
		root string
	}{{Files, "files"}, {LuCI, "luci"}} {
		err := fs.WalkDir(tree.fsys, tree.root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				places = append(places, strings.TrimPrefix(p, tree.root))
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(places) < 20 {
		t.Fatalf("only %d files: %v", len(places), places)
	}
	check := "for p; do\n\tcase $p in\n" + string(m[1]) + "\t\t*) echo \"$p\" ;;\n\tesac\ndone\n"
	out, err := exec.Command(sh, append([]string{"-c", check, "check"}, places...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if refused := strings.Fields(string(out)); len(refused) > 0 {
		t.Errorf("the installer refuses %v", refused)
	}
	out, err = exec.Command(sh, "-c", check, "check", "/etc/shadow", "/etc/init.d/network", "/usr/sbin/hostapd").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := strings.Join(strings.Fields(string(out)), " "); got != "/etc/shadow /etc/init.d/network /usr/sbin/hostapd" {
		t.Errorf("the installer lets through what is not Aeolus's: refused only %q", got)
	}
}
