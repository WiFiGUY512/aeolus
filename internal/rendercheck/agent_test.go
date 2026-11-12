package rendercheck

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/compose"
	"github.com/WiFiGUY512/aeolus/internal/uci"
)

// The contract between the agent and the manager (0040). Each case in
// agent/test/cases holds an intent, the AP config it starts from and, beside
// it, the UCI the agent renders (<case>.uci). That UCI must pass the render
// check and leave alone every section Aeolus does not own; and the agent must
// render exactly it, which is checked wherever ucode is installed (CI builds
// it).

const agentDir = "../../agent"

type agentCase struct {
	name    string
	Intent  map[string]any `json:"intent"`
	Current string         `json:"current"`
	Stale   []string       `json:"stale"`
	// Sections the AP has beyond the fixture, as package, then section.
	Extra map[string]map[string]map[string]any `json:"extra"`
	Kept  []string                             `json:"kept"` // package.section
	Gone  []string                             `json:"gone"`
	Facts struct {
		AP string `json:"ap"` // the AP it renders for, whose segment MACs are checked (0060)
		// The AP's clock, in Unix seconds, which a test network is on by
		// (0179); unset, it is not set, and none is.
		Now int64 `json:"now"`
	} `json:"facts"`
	// The renderer's errors, which make the agent refuse the config: with
	// "errors" in a case, exactly these, in order; without, they are not
	// checked.
	Errors *[]string `json:"errors"`
	// Refused: the renderer held back what the AP cannot run yet, and the
	// check must refuse what it renders, so the AP keeps what runs (0095).
	Refused bool `json:"refused"`
	golden  string
}

func agentCases(t *testing.T) []agentCase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(agentDir, "test", "cases", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no agent test cases: %v", err)
	}
	var out []agentCase
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		c := agentCase{name: strings.TrimSuffix(filepath.Base(p), ".json")}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		golden, err := os.ReadFile(strings.TrimSuffix(p, ".json") + ".uci")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		c.golden = string(golden)
		out = append(out, c)
	}
	return out
}

func TestAgentOutputPassesTheCheck(t *testing.T) {
	for _, c := range agentCases(t) {
		cfg, err := uci.Parse(c.golden)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		// A test network is checked for only while it is on, as the API
		// asks the check (0179).
		p := CheckAP(compose.Live(c.Intent, time.Unix(c.Facts.Now, 0)), cfg, c.Facts.AP)
		if c.Refused && len(p) == 0 {
			t.Errorf("%s: the check passes what the agent held back", c.name)
		}
		if !c.Refused && len(p) > 0 {
			t.Errorf("%s: the check refuses the agent's output:\n%s", c.name, strings.Join(p, "\n"))
		}
		untouched(t, c, cfg)
		for _, name := range c.Stale {
			if cfg.Package("wireless").Named(name) != nil {
				t.Errorf("%s: stale %s was kept", c.name, name)
			}
		}
		for _, ref := range c.Kept {
			if pkg, name, _ := strings.Cut(ref, "."); cfg.Package(pkg).Named(name) == nil {
				t.Errorf("%s: %s was removed", c.name, ref)
			}
		}
		for _, ref := range c.Gone {
			if pkg, name, _ := strings.Cut(ref, "."); cfg.Package(pkg).Named(name) != nil {
				t.Errorf("%s: %s was kept", c.name, ref)
			}
		}
	}
}

// A radio another service owns, airscan's scan radio, comes out exactly as
// it went in: no radio settings and no network on it (0081).
func TestAgentLeavesReservedRadiosAlone(t *testing.T) {
	seen := 0
	for _, c := range agentCases(t) {
		raw, err := os.ReadFile(filepath.Join(agentDir, "test", c.Current))
		if err != nil {
			t.Fatal(err)
		}
		var current map[string]map[string]map[string]any
		if err := json.Unmarshal(raw, &current); err != nil {
			t.Fatal(err)
		}
		cfg, err := uci.Parse(c.golden)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		w := cfg.Package("wireless")
		for name, s := range current["wireless"] {
			if s[".type"] != "wifi-device" || s["airscan"] != "1" {
				continue
			}
			seen++
			got := w.Named(name)
			if got == nil {
				t.Errorf("%s: reserved radio %s was removed", c.name, name)
				continue
			}
			for k, v := range s {
				if strings.HasPrefix(k, ".") {
					continue
				}
				if have, _ := got.Option(k); have != v {
					t.Errorf("%s: reserved radio %s: %s is %q, want %v", c.name, name, k, have, v)
				}
			}
			for _, k := range got.Names() {
				if _, ok := s[k]; !ok {
					t.Errorf("%s: reserved radio %s gained %s", c.name, name, k)
				}
			}
			for _, iface := range w.OfType("wifi-iface") {
				if dev, _ := iface.Option("device"); dev == name && strings.HasPrefix(iface.Name, "aeolus_") {
					t.Errorf("%s: %s puts a network on reserved radio %s", c.name, iface.Name, name)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no test case has a reserved radio")
	}
}

// untouched checks that every named wifi-iface, interface and bridge-vlan
// Aeolus does not own comes out exactly as it went in (0040), but for the
// bridge-vlan entries of the ports the intent sets the VLANs of (0053), and
// for a Wi-Fi network of the AP's own that one of Aeolus's took the place
// of (0137): only where a network of the intent says it replaces the AP's
// own, and only one the case names as gone.
func untouched(t *testing.T, c agentCase, cfg *uci.Config) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(agentDir, "test", c.Current))
	if err != nil {
		t.Fatal(err)
	}
	var current map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	for pkg, sections := range c.Extra {
		for name, s := range sections {
			current[pkg][name] = s
		}
	}
	kept := map[string]bool{"wifi-iface": true, "interface": true, "bridge-vlan": true}
	replaced := map[string]bool{}
	nets, _ := c.Intent["network"].(map[string]any)
	for _, n := range nets {
		if n, _ := n.(map[string]any); n["replace_own"] == true {
			for _, ref := range c.Gone {
				replaced[ref] = true
			}
		}
	}
	ports, _ := c.Intent["ports"].(map[string]any)
	// A bond's members, which take its place when Aeolus takes it apart, and
	// give it back (0096): the bond the AP has, or the one Aeolus keeps.
	bondOf := map[string]string{}
	for _, s := range current["network"] {
		if s["type"] == "bonding" {
			for _, m := range anyList(s["ports"]) {
				bondOf[m] = s["name"].(string)
			}
		}
	}
	if s := current["aeolus"][Unbond]; s != nil {
		for _, m := range anyList(s["ports"]) {
			bondOf[m] = s["name"].(string)
		}
	}
	// A bridge as a fresh OpenWrt has it is made to filter VLANs (0152): an
	// interface that was on such a bridge is on its untagged VLAN after,
	// with the DHCP client id it sent on the bridge (0288), and is otherwise
	// as it was.
	moved := map[string]string{}
	duid := ""
	for _, s := range current["network"] {
		if d, ok := s["dhcp_default_duid"].(string); ok && s[".type"] == "globals" {
			duid = d
		}
	}
	for _, s := range current["network"] {
		name, _ := s["name"].(string)
		if s[".type"] != "device" || s["type"] != "bridge" || name == "" {
			continue
		}
		had := false
		for _, v := range current["network"] {
			had = had || (v[".type"] == "bridge-vlan" && v["device"] == name)
		}
		if had {
			continue
		}
		for _, v := range cfg.Package("network").OfType("bridge-vlan") {
			if value(v, "device") == name && value(v, "vlan") == "1" && !strings.HasPrefix(v.Name, "aeolus_") {
				moved[name] = name + ".1"
			}
		}
	}
	others := func(entries any) any {
		list, ok := entries.([]any)
		if !ok {
			return entries
		}
		out := []any{}
		seen := map[string]bool{}
		for _, e := range list {
			port, flags, has := strings.Cut(e.(string), ":")
			if VLANEnd.MatchString(port) {
				continue // a network's veth, which Aeolus puts in the bridge (0061)
			}
			if BridgedPort.MatchString(port) {
				continue // a VNI a tunnel carries into the AP's bridge (0335)
			}
			if b, ok := bondOf[port]; ok {
				port, e = b, b
				if has {
					e = b + ":" + flags
				}
			}
			if seen[e.(string)] {
				continue
			}
			seen[e.(string)] = true
			if set, _ := ports[port].(map[string]any); set["mode"] != "access" && set["mode"] != "trunk" && set["mode"] != "native" && set["mode"] != "tunnel" {
				out = append(out, e)
			}
		}
		return out
	}
	for pkg, sections := range current {
		for name, s := range sections {
			if strings.HasPrefix(name, "aeolus_") || s[".anonymous"] == true || !kept[s[".type"].(string)] {
				continue
			}
			got := cfg.Package(pkg).Named(name)
			if got == nil && s[".type"] == "wifi-iface" && replaced[pkg+"."+name] {
				continue
			}
			if got == nil {
				t.Errorf("%s: %s.%s was removed", c.name, pkg, name)
				continue
			}
			want := map[string]any{}
			for k, v := range s {
				if !strings.HasPrefix(k, ".") {
					want[k] = v
				}
			}
			have := map[string]any{}
			for _, k := range got.Names() {
				if v, ok := got.Option(k); ok {
					have[k] = v
				} else {
					list := []any{}
					for _, e := range got.List(k) {
						list = append(list, e)
					}
					have[k] = list
				}
			}
			if s[".type"] == "bridge-vlan" {
				have["ports"], want["ports"] = others(have["ports"]), others(want["ports"])
			}
			if d, _ := want["device"].(string); s[".type"] == "interface" && moved[d] != "" {
				want["device"] = moved[d]
				if id := clientID(d, duid); id != "" && want["proto"] == "dhcp" && want["clientid"] == nil &&
					(want["sendclientid"] == nil || want["sendclientid"] == "auto") {
					want["clientid"] = id
				}
			}
			if !reflect.DeepEqual(have, want) {
				t.Errorf("%s: %s.%s changed:\n got %v\nwant %v", c.name, pkg, name, have, want)
			}
		}
	}
}

// clientID is the DHCP client id OpenWrt sends on a device, where it has a
// DUID (proto_dhcp_get_default_clientid): ff, the first eight hex digits of
// the MD5 of the device's name, and the DUID.
func clientID(device, duid string) string {
	if duid == "" {
		return ""
	}
	sum := md5.Sum([]byte(device))
	return "ff" + hex.EncodeToString(sum[:])[:8] + strings.ToLower(duid)
}

// A bridge as a fresh OpenWrt has it, one network with no VLANs, is made to
// filter them (0152): its ports untagged in VLAN 1, and the AP's own
// interface on that VLAN of it, so the AP keeps its address, with the DHCP
// client id OpenWrt made from the bridge's name (0288); then the networks'
// VLANs go on the uplink as on any AP. One that already has an 802.1Q
// device on it is left as it is, and nothing is rendered on it.
func TestAgentMakesAFreshBridgeFilterVLANs(t *testing.T) {
	golden := func(name string) *uci.Package {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(agentDir, "test", "cases", name+".uci"))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := uci.Parse(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Package("network")
	}
	n := golden("fresh-bridge")
	v := n.Named("vlan1")
	if v == nil || value(v, "device") != "br-lan" || value(v, "vlan") != "1" || !reflect.DeepEqual(v.List("ports"), []string{"eth0:u*"}) {
		t.Errorf("the bridge's untagged VLAN: %v", v)
	}
	if lan := n.Named("lan"); lan == nil || value(lan, "device") != "br-lan.1" || value(lan, "proto") != "dhcp" {
		t.Errorf("the AP's own interface is not on the bridge's untagged VLAN: %v", lan)
	}
	// ff, the first eight hex digits of md5("br-lan"), and the fixture's DUID.
	if lan := n.Named("lan"); lan == nil || value(lan, "clientid") != "ffeb351e8c0004f27e36e651104bdf9f626122601aaa72" {
		t.Errorf("the AP's own interface does not keep the client id it sent on br-lan: %v", lan)
	}
	if lan := golden("fresh-bridge-hardware-id").Named("lan"); lan == nil || value(lan, "device") != "br-lan.1" || value(lan, "clientid") != "" {
		t.Errorf("an interface that sends its MAC was given a client id, or not moved: %v", lan)
	}
	if v := n.Named("aeolus_vlan20"); v == nil || !reflect.DeepEqual(v.List("ports"), []string{"eth0:t"}) {
		t.Errorf("the network's VLAN is not tagged on the uplink: %v", v)
	}
	if lab := n.Named("aeolus_lab"); lab == nil || value(lab, "device") != "br-lan.20" {
		t.Errorf("the network's interface: %v", lab)
	}

	n = golden("bridge-8021q")
	if len(n.OfType("bridge-vlan")) != 0 {
		t.Errorf("a bridge with an 802.1Q device on it was made to filter VLANs: %d bridge-vlan sections", len(n.OfType("bridge-vlan")))
	}
	if lan := n.Named("lan"); lan == nil || value(lan, "device") != "br-lan" {
		t.Errorf("its own interface was moved: %v", lan)
	}
	if guest := n.Named("guest"); guest == nil || value(guest, "device") != "br-lan.30" {
		t.Errorf("its 802.1Q interface was changed: %v", guest)
	}
}

// anyList is a UCI list as JSON has it, or one value as a list of one.
func anyList(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := []string{}
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// In ucode, a function can only name a top-level function declared above it:
// a call to one declared below compiles, and fails only when it runs, with
// "access to undeclared variable" (v0.28.0's agent, on every start). The
// agent's own code is not run in CI, so each call is checked against the
// order of declarations here instead.
func TestAgentCallsOnlyWhatIsDeclaredAbove(t *testing.T) {
	decl := regexp.MustCompile(`^(?:export\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	call := regexp.MustCompile(`(^|[^A-Za-z0-9_.$])([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	files := []string{"files/usr/sbin/aeolus-agent", "files/usr/sbin/aeolus-prober", "files/usr/sbin/aeolus-rrm", "files/usr/sbin/aeolus-journey", "files/usr/sbin/aeolus-steer", "files/usr/sbin/aeolus-live", "files/usr/sbin/aeolus-ble", "files/usr/sbin/aeolus-capture", "files/usr/sbin/aeolus-clienttest", "files/usr/libexec/aeolus-gi", "files/usr/libexec/aeolus-power"}
	mods, _ := filepath.Glob(filepath.Join(agentDir, "files", "usr", "share", "ucode", "aeolus", "*.uc"))
	for _, m := range mods {
		rel, _ := filepath.Rel(agentDir, m)
		files = append(files, filepath.ToSlash(rel))
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(agentDir, f))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(raw), "\n")
		at := map[string]int{} // a top-level function -> the line it is declared on
		for i, l := range lines {
			if m := decl.FindStringSubmatch(l); m != nil {
				at[m[1]] = i
			}
		}
		within := "" // the top-level function the line is in
		for i, l := range lines {
			if m := decl.FindStringSubmatch(l); m != nil {
				within = m[1]
			} else if !strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, "}") && strings.TrimSpace(l) != "" {
				within = "" // top-level code runs after the declarations above it
			}
			if within == "" {
				continue
			}
			code, _, _ := strings.Cut(l, "//")
			for _, m := range call.FindAllStringSubmatch(code, -1) {
				if d, ok := at[m[2]]; ok && d > i && m[2] != within {
					t.Errorf("%s:%d: %s calls %s, which is declared below it (line %d), so the call fails when it runs", f, i+1, within, m[2], d+1)
				}
			}
		}
	}
}

func TestAgentRendersItsOutput(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	// Absolute, because ucode resolves a relative search path from the
	// directory of the file doing the import, which breaks the agent's own
	// imports between its modules.
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range agentCases(t) {
		cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "render.uc"),
			filepath.Join(agentDir, "test", "cases", c.name+".json"))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, stderr.String())
		}
		if string(out) != c.golden {
			t.Errorf("%s: the agent renders something else than %s.uci:\n%s", c.name, c.name, out)
		}
		if c.Errors != nil {
			var got []string
			for _, line := range strings.Split(stderr.String(), "\n") {
				if e, ok := strings.CutPrefix(line, "render: "); ok {
					got = append(got, e)
				}
			}
			if strings.Join(got, "\n") != strings.Join(*c.Errors, "\n") {
				t.Errorf("%s: the renderer's errors are\n%s\nwant\n%s", c.name, strings.Join(got, "\n"), strings.Join(*c.Errors, "\n"))
			}
		}
		// The MSS clamp's file, made from the rendered tunnels (0054).
		want, err := os.ReadFile(filepath.Join(agentDir, "test", "cases", c.name+".nft"))
		if err != nil {
			continue
		}
		cmd = exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "render.uc"),
			filepath.Join(agentDir, "test", "cases", c.name+".json"), "clamp")
		if out, err = cmd.Output(); err != nil {
			t.Fatalf("%s clamp: %v", c.name, err)
		}
		if string(out) != string(want) {
			t.Errorf("%s: the agent makes another clamp than %s.nft:\n%s", c.name, c.name, out)
		}
	}
}

// The prober's frames, filters and verdicts (0059), as the agent's own ucode
// builds and reads them: agent/test/probe.uc prints them, and probe.out is
// what it must print. Its checksums were also worked out apart from ucode.
func TestAgentProbeFrames(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "probe.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "probe.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("the prober's frames are not what probe.out says:\n%s", out)
	}
}

// Radio resource management's pure parts (0073), as the agent's own ucode
// works them out: agent/test/rrm.uc prints them, and rrm.out is what it must
// print. Its HMACs are RFC 4231's, or worked out with Python's hmac.
func TestAgentRRM(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "rrm.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "rrm.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("radio resource management works out other than rrm.out says:\n%s", out)
	}
}

// The prober's RADIUS status request, how its answers read, and how hostapd's
// counts are read (0114), as the agent's own ucode works them out:
// agent/test/radius.uc prints them, and radius.out is what it must print.
// Its HMACs are RFC 2202's, and the request's was worked out with Python's
// hmac too.
func TestAgentRadius(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "radius.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "radius.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("the prober's RADIUS status works out other than radius.out says:\n%s", out)
	}
}

// The check catches a bond taken apart badly (0096): a VLAN the second port
// does not carry, which the AP would lose when that port is the path out,
// and a bridge without spanning tree.
func TestUnbondCheckCatchesALostVLAN(t *testing.T) {
	for _, c := range agentCases(t) {
		if c.name != "unbond" {
			continue
		}
		for bad, want := range map[string]string{
			strings.Replace(c.golden, "\tlist ports 'eth0:t'\n\tlist ports 'eth1:t'\n", "\tlist ports 'eth0:t'\n", 1): "VLAN 20 is not on eth1 as on eth0",
			strings.Replace(c.golden, "\toption stp '1'\n", "", 1):                                                    "spanning tree is off",
		} {
			cfg, err := uci.Parse(bad)
			if err != nil {
				t.Fatal(err)
			}
			if p := strings.Join(CheckAP(c.Intent, cfg, c.Facts.AP), "\n"); !strings.Contains(p, want) {
				t.Errorf("the check missed %q:\n%s", want, p)
			}
		}
		return
	}
	t.Fatal("no unbond case")
}

// A top-level function named as one of ucode's own hides it for the whole
// file: a call meant for ucode's then runs the file's, with no error. In
// aeolus/journey.uc a packet filter named filter made record count the
// filter's steps for the client's sign-in steps, and a client that left
// while associating was said to have stopped at its sign-in (v0.69.0).
func TestAgentHidesNoBuiltin(t *testing.T) {
	builtins := map[string]bool{}
	for _, b := range strings.Fields(`print printf sprintf length index rindex substr split join keys values map filter sort reverse
		push pop shift unshift splice slice uniq exists type int die ord chr hex uc lc trim ltrim rtrim replace match json include render
		warn system trace proto sleep assert regexp wildcard sourcepath min max b64dec b64enc uchr time localtime gmtime timelocal timegm
		clock hexdec hexenc gc loadstring loadfile call signal require iptoarr arrtoip getenv exit`) {
		builtins[b] = true
	}
	decl := regexp.MustCompile(`^(?:export\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	var files []string
	for _, pattern := range []string{"files/usr/sbin/aeolus-*", "files/usr/libexec/aeolus-*", "files/usr/share/ucode/aeolus/*.uc"} {
		m, _ := filepath.Glob(filepath.Join(agentDir, filepath.FromSlash(pattern)))
		files = append(files, m...)
	}
	if len(files) < 8 {
		t.Fatalf("only %d agent files found", len(files))
	}
	// The renderer's system and render are the settings it renders and the
	// module's own entry: it runs no command and expands no template, so it
	// calls neither of ucode's.
	known := map[string]bool{"render.uc system": true, "render.uc render": true}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(raw), "\n") {
			if m := decl.FindStringSubmatch(l); m != nil && builtins[m[1]] && !known[filepath.Base(f)+" "+m[1]] {
				t.Errorf("%s:%d: function %s hides ucode's own %s in this file", filepath.Base(f), i+1, m[1], m[1])
			}
		}
	}
}

// A client coming online, as the AP records it (0118): what hostapd's log
// lines mean, what the packet filter keeps and how each frame reads, and
// the record made of an attempt. agent/test/journey.uc prints them, with
// frames it makes itself and a small interpreter of the kernel's filter;
// journey.out is what it must print.
func TestAgentJourney(t *testing.T) {
	agentPrints(t, "journey", "a client coming online is recorded")
}

// TestAgentCapture runs the agent's test of the captures it keeps of
// attempts that fail (0135, 0146): what is read of hostapd's rings, the
// frames made of it, a ring of frames, and the file Wireshark opens;
// capture.out is what it must print.
func TestAgentCapture(t *testing.T) {
	agentPrints(t, "capture", "a failed attempt is captured")
}

// TestAgentPcap runs the agent's test of the captures people start (0148):
// which interfaces one takes, with what tcpdump is told to keep, and the
// script that takes it; pcap.out is what it must print.
func TestAgentPcap(t *testing.T) {
	agentPrints(t, "pcap", "a capture someone starts is planned")
}

// TestAgentSteer runs the agent's test of its steering of clients (0138):
// what is asked of a client, when, and what is made of where it then is,
// for each of the three ways and for a client that does not stay moved.
// agent/test/steer.uc plays the daemon's looks and hostapd's notifications
// to aeolus.steer; steer.out is what it must print.
func TestAgentSteer(t *testing.T) {
	agentPrints(t, "steer", "clients are steered")
}

// TestAgentSentoff runs the agent's test of what its daemons keep a client
// off (Keep on 6 GHz, 0300): each keeping, as it is renewed and lifted, and
// when a frame hostapd sent the client is one of it, which aeolus-journey
// leaves out of the trace; sentoff.out is what it must print.
func TestAgentSentoff(t *testing.T) {
	agentPrints(t, "sentoff", "what is kept off is said")
}

// TestAgentProbed runs the agent's test of what the APs tell one another of
// the clients they hear by probe and do not serve (0289): what this AP has
// to say from its steering daemon's file, its message fitted to one frame
// and read back, another AP's checked, and what the steering daemon reads;
// probed.out is what it must print.
func TestAgentProbed(t *testing.T) {
	agentPrints(t, "probed", "what the APs hear by probe is said")
}

// TestAgentCommunity runs the agent's test of what the APs tell one another
// of what their clients said they can use, where community sharing is on
// (0299): what this AP has to say from its steering daemon's file, its
// message fitted to one frame, another AP's checked, and what the steering
// daemon reads; community.out is what it must print.
func TestAgentCommunity(t *testing.T) {
	agentPrints(t, "community", "what the APs say their clients can use is said")
}

// TestAgentTrust runs the agent's test of what airscan is to trust
// (0304): the manager's doc as airscan's lists, and each list merged with
// what a person put there, the agent's own taken away when the manager
// drops them; trust.out is what it must print.
func TestAgentTrust(t *testing.T) {
	agentPrints(t, "trust", "what airscan trusts is worked out")
}

// TestAgentLiveWatch runs the agent's test of its watch on the live
// daemon's minutes (0307): when they stopped, or have had no radio while
// hostapd serves, and not again too soon; livewatch.out is what it must
// print.
func TestAgentLiveWatch(t *testing.T) {
	agentPrints(t, "livewatch", "the live daemon's minutes are watched")
}

// TestAgentFWCrash runs the agent's test of what it reads of radios whose
// firmware crashed (0310): the kernel's lines, each crash timed by the
// kernel's clock, and whether its radio came back; fwcrash.out is what it
// must print.
func TestAgentFWCrash(t *testing.T) {
	agentPrints(t, "fwcrash", "a radio's firmware crash is read")
}

// TestAgentMixedLibs runs the agent's test of the libraries a process holds
// two builds of at once (0317), from the maps of each; mixedlibs.out is what
// it must print.
func TestAgentMixedLibs(t *testing.T) {
	agentPrints(t, "mixedlibs", "two builds of a library in one process are found")
}

// TestAgentMinutes runs the agent's test of what a minute came to (0207):
// how busy a radio's channel was, from its survey's running counts, and a
// client's signal, rates and what it was sent, from its station's, across
// counts that went back and a client that joined again; and a minute on an
// AP; minutes.out is what it must print.
func TestAgentMinutes(t *testing.T) {
	agentPrints(t, "minutes", "a minute comes to")
}

// TestAgentMeasure runs the agent's test of what a client says it hears of
// the AP (802.11k): how it is asked, the operating class of its channel,
// hostapd's beacon reports as two clients made them, which clients are
// asked when, and what one hears after the AP's power changed;
// measure.out is what it must print.
func TestAgentMeasure(t *testing.T) {
	agentPrints(t, "measure", "a client's report is read")
}

// TestAgentRecord runs the agent's test of the flight recorder's judgement
// (0207 part 3): when what five seconds came to says something went wrong
// against a radio's usual, and when it does not; what a kept point keeps;
// and a recording's window; record.out is what it must print.
func TestAgentRecord(t *testing.T) {
	agentPrints(t, "record", "the flight recorder judges")
}

// TestAgentUplink runs the agent's test of which of the AP's own Wi-Fi
// networks are on a network it holds an address on (0209): a bridge VLAN's
// ports as netifd lists them, a bridge's members, and a hidden network; and
// of when its management interface moved under its config (0210);
// uplink.out is what it must print.
func TestAgentUplink(t *testing.T) {
	agentPrints(t, "uplink", "the Wi-Fi on a network is")
}

// TestAgentBLE runs the agent's test of what it says to Home Assistant for
// the AP's Bluetooth radio (0156): the numbers and messages of ESPHome's
// API, what is read of the controller's packets as hcidump writes them, and
// the advertisements handed on; ble.out is what it must print.
func TestAgentBLE(t *testing.T) {
	agentPrints(t, "ble", "the Bluetooth radio is lent")
}

// TestAgentBTAD runs the agent's test of what it makes of the devices the
// AP's Bluetooth radio hears (0157): what each advertisement says, the
// table aeolus-ble keeps, the survey's list taken with it, and what is
// reported; btad.out is what it must print.
func TestAgentBTAD(t *testing.T) {
	agentPrints(t, "btad", "the devices heard are reported")
}

// TestAgentBeacons runs the agent's test of how aeolus-rrm reads the
// adverts in the beacons the scan radio hears (0161): the radiotap header's
// Flags, and frames that failed their FCS check passed over; beacons.out is
// what it must print.
func TestAgentBeacons(t *testing.T) {
	agentPrints(t, "beacons", "the scan radio's beacons are read")
}

// TestAgentWifiNames runs the agent's test of how its daemons see netifd's
// Wi-Fi status while a radio is being set up again (0301): a pending radio
// whose interfaces netifd lists without names is seen as it last was,
// where the kernel still has its interfaces, and not where it is taken
// down, never seen settled, or pending too long; and a look made once
// waits for netifd to finish. wifinames.out is what it must print.
func TestAgentWifiNames(t *testing.T) {
	agentPrints(t, "wifinames", "a radio being set up again is seen")
}

// TestAgentPower runs the agent's test of the transmit power aeolus-power
// holds each radio to with iw (0349): what it reads of the agent's package
// and of what it held, and what it does for each radio, a power set,
// changed, let go, a radio set up again by netifd, one not there yet, and a
// wiphy two radios share. power.out is what it must print.
func TestAgentPower(t *testing.T) {
	agentPrints(t, "power", "the power held")
}

// agentPrints runs agent/test/<name>.uc and holds what it prints to
// <name>.out beside it.
func agentPrints(t *testing.T, name, what string) {
	t.Helper()
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", name+".out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", name+".uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("%s other than %s.out says:\n%s", what, name, out)
	}
}

// The check catches passphrases from the RADIUS server rendered badly
// (0115): ppsk left off, so hostapd would take the network's own passphrase
// from anyone; a key written beside it; and ppsk on a network that takes
// none from a server, where it would refuse every device.
func TestPassphrasesFromRADIUSCheck(t *testing.T) {
	seen := 0
	for _, c := range agentCases(t) {
		var bad map[string]string
		switch c.name {
		case "radius-passphrases":
			bad = map[string]string{
				strings.Replace(c.golden, "\toption ppsk '1'\n", "\toption key 'fixture-passphrase-1'\n", 1): "ppsk is \"\", want 1",
				strings.Replace(c.golden, "\toption ppsk '1'\n", "\toption ppsk '1'\n\toption key 'x'\n", 1): "a key is set, but each device's passphrase is the RADIUS server's",
			}
		case "mac-auth":
			bad = map[string]string{
				strings.Replace(c.golden, "\toption key 'fixture-passphrase-1'\n", "\toption key 'fixture-passphrase-1'\n\toption ppsk '1'\n", 1): "the network takes no passphrases from a RADIUS server",
			}
		default:
			continue
		}
		seen++
		for uciText, want := range bad {
			if uciText == c.golden {
				t.Fatalf("%s: nothing was changed for %q", c.name, want)
			}
			cfg, err := uci.Parse(uciText)
			if err != nil {
				t.Fatal(err)
			}
			if p := strings.Join(CheckAP(c.Intent, cfg, c.Facts.AP), "\n"); !strings.Contains(p, want) {
				t.Errorf("%s: the check missed %q:\n%s", c.name, want, p)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("%d of the 2 cases found", seen)
	}
}

// A module imported whole (import * as name) is hidden, in a function, by a
// local of the same name: a call through it compiles, and fails only when it
// runs, with "left-hand side expression is not an array or object" (v0.57.0's
// agent, on every state report, where ports_state had a local uplink).
func TestAgentHidesNoModule(t *testing.T) {
	imp := regexp.MustCompile(`^import \* as ([A-Za-z_][A-Za-z0-9_]*) from`)
	files := []string{"files/usr/sbin/aeolus-agent", "files/usr/sbin/aeolus-prober", "files/usr/sbin/aeolus-rrm", "files/usr/sbin/aeolus-journey", "files/usr/sbin/aeolus-steer", "files/usr/sbin/aeolus-live", "files/usr/sbin/aeolus-ble", "files/usr/sbin/aeolus-capture", "files/usr/sbin/aeolus-clienttest", "files/usr/libexec/aeolus-gi", "files/usr/libexec/aeolus-power"}
	mods, _ := filepath.Glob(filepath.Join(agentDir, "files", "usr", "share", "ucode", "aeolus", "*.uc"))
	for _, m := range mods {
		rel, _ := filepath.Rel(agentDir, m)
		files = append(files, filepath.ToSlash(rel))
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(agentDir, f))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(raw), "\n")
		for _, l := range lines {
			m := imp.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			local := regexp.MustCompile(`(?:\blet|\bconst|\bfor\s*\(\s*let)\s+(?:[A-Za-z_][A-Za-z0-9_]*\s*,\s*)?` + m[1] + `\b|function\s*[A-Za-z0-9_]*\s*\([^)]*\b` + m[1] + `\b[^)]*\)|\(\s*` + m[1] + `\s*\)\s*=>|\b` + m[1] + `\s*=>`)
			for i, l := range lines {
				if code, _, _ := strings.Cut(l, "//"); local.MatchString(code) {
					t.Errorf("%s:%d: a local %s hides the module imported as %s", f, i+1, m[1], m[1])
				}
			}
		}
	}
}

// The client test (0121): what wpa_supplicant is told for each way a
// network is secured, what its events mean, a DNS question and its answer,
// and what a test's findings come to. agent/test/clienttest.uc prints them;
// clienttest.out is what it must print.
func TestAgentClientTest(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "clienttest.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "clienttest.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("the client test's parts print other than clienttest.out says:\n%s", out)
	}
}

// A client test over the wire (0128): the MAC it is made from, the filter
// its socket keeps to, its DHCP, ARP and DNS frames and how their answers
// read, and which wire a network's frames take. agent/test/wiretest.uc
// prints them; wiretest.out is what it must print.
func TestAgentWireTest(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "wiretest.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "wiretest.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("the wire test's parts print other than wiretest.out says:\n%s", out)
	}
}

// TestAgentLetGo runs the agent's test of active client management's pure
// parts (0226): what an AP says of the clients it let on, what another
// reads of it, and whether to let go of a client hostapd holds; letgo.out
// is what it must print.
func TestAgentLetGo(t *testing.T) {
	agentPrints(t, "letgo", "active client management's parts print")
}

// TestAgentSSHKeys runs the agent's test of the SSH keys it puts on an AP
// (0197): those set in Aeolus and the manager's own go in after any put there
// by hand, and only Aeolus's own are ever taken away; sshkeys.out is what it
// must print.
func TestAgentSSHKeys(t *testing.T) {
	agentPrints(t, "sshkeys", "the SSH keys are merged")
}

// An AP's own Wi-Fi networks, that are not Aeolus's (0137): what its agent
// says of each, with nothing of its key; which section one is, where it is
// asked for; and the section as it is kept before it is taken out.
// agent/test/others.uc prints them; others.out is what it must print.
func TestAgentOthers(t *testing.T) {
	ucode, err := exec.LookPath("ucode")
	if err != nil {
		t.Skip("ucode is not installed; CI builds it (0040)")
	}
	modules, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "share", "ucode", "*.uc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(agentDir, "test", "others.out"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ucode, "-L", modules, filepath.Join(agentDir, "test", "others.uc"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if string(out) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("an AP's own networks print other than others.out says:\n%s", out)
	}
}

// A test network's SSIDs must carry when they go off (0179): without it,
// the agent would never turn them off.
func TestTestNetworkGoesOffByItsTime(t *testing.T) {
	var c agentCase
	raw, err := os.ReadFile(filepath.Join(agentDir, "test", "cases", "test-network-on.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile(filepath.Join(agentDir, "test", "cases", "test-network-on.uci"))
	if err != nil {
		t.Fatal(err)
	}
	doc := compose.Live(c.Intent, time.Unix(c.Facts.Now, 0))
	without := regexp.MustCompile(`(?m)^\toption aeolus_until .*\n`).ReplaceAllString(string(golden), "")
	cfg, err := uci.Parse(without)
	if err != nil {
		t.Fatal(err)
	}
	if p := strings.Join(CheckAP(doc, cfg, ""), "\n"); !strings.Contains(p, "aeolus_until") {
		t.Errorf("a test network's SSIDs without their time pass the check: %q", p)
	}
}
