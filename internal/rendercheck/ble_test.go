package rendercheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WiFiGUY512/aeolus/internal/uci"
)

// The AP's Bluetooth radio lent to Home Assistant (0156): the agent's ble
// section is there, and says it is on, exactly where system.ble_proxy is
// on. The check refuses a config that lends it unasked, and one that does
// not lend it where it is asked.
func TestBluetoothIsLentOnlyWhereAsked(t *testing.T) {
	var lent, plain agentCase
	for _, c := range agentCases(t) {
		switch c.name {
		case "ble":
			lent = c
		case "sandbox":
			plain = c
		}
	}
	if lent.name == "" || plain.name == "" {
		t.Fatal("the ble and sandbox cases are needed")
	}
	parse := func(text string) *uci.Config {
		t.Helper()
		cfg, err := uci.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	s := parse(lent.golden).Package("aeolus").Named("aeolus_ble")
	if s == nil || s.Type != "ble" || value(s, "enabled") != "1" {
		t.Fatalf("the agent's section where it is lent: %v", s)
	}
	if parse(plain.golden).Package("aeolus").Named("aeolus_ble") != nil {
		t.Error("the section is there where it is not lent")
	}
	// Each one's config held to the other's intent.
	if p := strings.Join(CheckAP(plain.Intent, parse(lent.golden), ""), "\n"); !strings.Contains(p, "aeolus.aeolus_ble: the Bluetooth radio is not lent, but the section is there") {
		t.Errorf("lent unasked: %s", p)
	}
	if p := strings.Join(CheckAP(lent.Intent, parse(plain.golden), ""), "\n"); !strings.Contains(p, "aeolus.aeolus_ble: the Bluetooth radio is lent, but the section is missing") {
		t.Errorf("not lent where asked: %s", p)
	}
	// Off, said outright, is as not said.
	off := map[string]any{}
	for k, v := range plain.Intent {
		off[k] = v
	}
	system := map[string]any{"ble_proxy": false}
	for k, v := range obj(plain.Intent, "system") {
		system[k] = v
	}
	off["system"] = system
	if p := CheckAP(off, parse(plain.golden), ""); len(p) > 0 {
		t.Errorf("set off outright: %s", strings.Join(p, "\n"))
	}
}

// The daemon that lends the radio is started by the agent's init script,
// kept across a firmware upgrade and made executable where the agent is
// installed by hand or as a package, as each of the agent's programs is: by
// the directory it is in.
func TestBluetoothDaemonIsInstalled(t *testing.T) {
	for file, want := range map[string]string{
		"files/etc/init.d/aeolus":         "procd_set_param command /usr/sbin/aeolus-ble",
		"files/lib/upgrade/keep.d/aeolus": "/usr/sbin/aeolus-ble\n",
		"install.sh":                      "find usr/sbin usr/libexec etc/init.d -type f",
		"openwrt/aeolus-agent/Makefile":   "find $(1)/usr/sbin $(1)/usr/libexec $(1)/etc/init.d -type f -exec chmod 0755",
		"files/usr/sbin/aeolus-agent":     "report.ble = bt;",
		"files/usr/sbin/aeolus-ble":       "import * as esp from 'aeolus.esphome';",
	} {
		raw, err := os.ReadFile(filepath.Join(agentDir, filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ReplaceAll(string(raw), "\r\n", "\n"), want) {
			t.Errorf("%s has no %q", file, want)
		}
	}
}
