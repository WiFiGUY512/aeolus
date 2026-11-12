package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The stand-ins aeolus-packages runs against, as the real tools behave where
// it matters: an opkg whose index can be read or not (update fails where one
// feed is bad, yet reads the others), whose download of an unknown package
// says so and exits 0, as opkg's does; an apk whose add is one change, made
// or not; a uclient-fetch that, like the real one, says "HTTP error 404"
// only without -q and exits 8 on an HTTP error and 4 when it cannot connect;
// and the commands that restart the Wi-Fi, which say they ran. opkg's lock,
// held, refuses every command, as the real one's does. $ROOT is the test's directory, where the script's own paths are
// moved too.
const fakeOpkg = `#!/bin/sh
[ -f $ROOT/locked ] && { echo "Could not lock /var/lock/opkg.lock" >&2; exit 255; }
cmd=$1; shift
case $cmd in
update)
	[ -f $ROOT/net ] || exit 1
	touch $ROOT/indexed
	[ -f $ROOT/badfeed ] && exit 1
	exit 0 ;;
status) grep -qx "$1" $ROOT/installed && echo "Status: install user installed" ; exit 0 ;;
download)
	if [ -f $ROOT/indexed ] && [ -f $ROOT/net ] && grep -qx "$1" $ROOT/available; then
		f="$1_$(cat $ROOT/version-$1 2>/dev/null || echo 1.0)_all.ipk"
		if [ -f $ROOT/garbage-$1 ]; then
			echo "$1" >"$f"
		else
			b=$(mktemp -d)
			mkdir -p $b/data/usr/sbin
			head -c $(cat $ROOT/size-$1 2>/dev/null || echo 300000) /dev/zero >$b/data/usr/sbin/wpad
			tar -C $b/data -czf $b/data.tar.gz ./usr
			echo "Package: $1" >$b/control
			tar -C $b -czf $b/control.tar.gz ./control
			echo 2.0 >$b/debian-binary
			tar -C $b -czf "$f" ./debian-binary ./data.tar.gz ./control.tar.gz
			rm -rf $b
		fi
	else
		echo "Cannot find package $1." >&2
	fi
	exit 0 ;;
remove)
	[ -f $ROOT/remove-fails ] && exit 255
	grep -vx "$1" $ROOT/installed >$ROOT/installed.new; mv $ROOT/installed.new $ROOT/installed
	grep -q '^wpad' $ROOT/installed || rm -f $ROOT/usr/sbin/wpad $ROOT/usr/sbin/hostapd
	echo "remove $1" >>$ROOT/calls ;;
install)
	force=
	[ "$1" = --force-reinstall ] && { force=1; shift; }
	case $1 in
	*.ipk)
		[ -f "$1" ] || exit 1
		p=$(basename "$1"); p=${p%%_*} ;;
	*)
		[ -f $ROOT/indexed ] && [ -f $ROOT/net ] && grep -qx "$1" $ROOT/available || exit 1
		p=$1 ;;
	esac
	[ -f $ROOT/fail-$p ] && exit 1
	case $1 in *.ipk) [ -f $ROOT/fail-ipk-$p ] && exit 1 ;; esac
	if grep -qx "$p" $ROOT/installed; then
		# as opkg: a package installed is up to date, its files as they are
		[ -n "$force" ] || { echo "Package $p installed in root is up to date."; exit 0; }
		echo "reinstall $p" >>$ROOT/calls
	else
		echo $p >>$ROOT/installed
		echo "install $p" >>$ROOT/calls
	fi
	case $p in wpad*) touch $ROOT/usr/sbin/wpad $ROOT/usr/sbin/hostapd ;; esac ;;
*) exit 1 ;;
esac
`

const fakeApk = `#!/bin/sh
# -X with --repositories-file /dev/null: that feed alone, whose packages are
# in feed-has as name-version (0229). A build for another ucode (0256): a
# file other-ucode-<name> for the other feeds', feed-other-ucode-<name> for
# the feed's, and adding it with --simulate says it brings a libucode in.
feed=
while :; do case $1 in --repositories-file) shift 2 ;; -X) feed=$2; shift 2 ;; *) break ;; esac; done
cmd=$1; shift
[ "$cmd" = --print-arch ] && { echo aarch64; exit 0; }
# unmet lists, as apk does, what files need that is neither installed nor
# among them (0315): a package's needs are in deps-<name>, one a line.
unmet() {
	names=
	for a in "$@"; do case $a in *.apk) names="$names $(echo "${a##*/}" | sed 's/-[0-9].*//')" ;; esac; done
	bad=
	for a in "$@"; do
		case $a in *.apk) ;; *) continue ;; esac
		n=$(echo "${a##*/}" | sed 's/-[0-9].*//')
		for d in $(cat $ROOT/deps-$n 2>/dev/null); do
			grep -qx "$d" $ROOT/installed && continue
			echo " $names " | grep -q " $d " && continue
			bad="$bad\n  $d (no such package):\n    required by: ${a##*/}"
			bad="${bad%.apk}[$d]"
		done
	done
	[ -n "$bad" ] || return 1
	printf "ERROR: unable to select packages:$bad\n"
}
if [ "$cmd" = add ] && [ "$1" = --simulate ]; then
	shift
	while [ "$1" = --repositories-file ]; do shift 2; done
	if [ "$1" = --allow-untrusted ]; then
		shift
		unmet "$@" && exit 1
		echo "OK: 1 package"
		exit 0
	fi
	a=$1 n=${1%%=*}
	if [ -n "$feed" ]; then
		grep -qx "$n-${a#*=}" $ROOT/feed-has 2>/dev/null || exit 1
		[ -f $ROOT/feed-other-ucode-$n ] && echo "(1/2) Installing libucode20230711 (2026.07.09)"
	else
		grep -qx "$n" $ROOT/available || grep -qx "$n" $ROOT/installed || exit 1
		[ -f $ROOT/other-ucode-$n ] && echo "(1/2) Installing libucode20261009 (2026.10.28)"
		# The libraries a package's build would bring in, where
		# brings-<name> lists them (0318), for every build of it;
		# brings-<name>=<version>, which may be empty, for one build.
		b=$ROOT/brings-$a; [ -f "$b" ] || b=$ROOT/brings-$n
		[ -f "$b" ] && for l in $(cat "$b"); do echo "(1/2) Installing $l (1.0-r1)"; done
	fi
	echo "OK: 1 package"
	exit 0
fi
case $cmd in
verify)
	for f in "$@"; do [ -f $ROOT/unsigned-${f##*/} ] && { echo "$f: UNTRUSTED signature"; exit 1; }; echo "$f: OK"; done
	exit 0 ;;
search)
	[ -n "$feed" ] || exit 1
	[ "$1" = -x ] && shift
	# A build's own package feeds (0315) list theirs in arch-has (0343).
	has=feed-has
	case $feed in */aarch64_*) has=arch-has ;; esac
	grep "^$1-[0-9]" $ROOT/$has 2>/dev/null
	exit 0 ;;
update) [ -f $ROOT/net ] ;;
info)
	[ $# -eq 0 ] && { cat $ROOT/installed; exit 0; }
	[ "$1" = -e ] && shift; grep -qx "$1" $ROOT/installed ;;
list)
	# -I [name]: what is installed, as name-version and more; a build other
	# than 1.0-r1 where installed-version-<name> says so.
	if [ "$1" = -I ]; then
		for n in $(cat $ROOT/installed); do
			[ -n "$2" ] && [ "$2" != "$n" ] && continue
			echo "$n-$(cat $ROOT/installed-version-$n 2>/dev/null || echo 1.0-r1) aarch64_cortex-a53 {feeds} (ISC) [installed]"
		done
		exit 0
	fi
	# -a <name>: each build the feeds have, as name-version and more.
	if [ "$1" = -a ]; then
		[ -n "$feed" ] && grep "^$2-[0-9]" $ROOT/feed-has 2>/dev/null | sed 's/$/ aarch64_cortex-a53 {feeds} (ISC)/'
		[ -z "$feed" ] && grep -qx "$2" $ROOT/available && echo "$2-1.0-r1 aarch64_cortex-a53 {feeds} (ISC)"
	fi
	exit 0 ;;
fetch)
	echo "fetch ${feed:+-X $feed }$*" >>$ROOT/calls
	rec=
	[ "$1" = -R ] && { rec=1; shift; }
	[ "$1" = -o ] || exit 1
	dir=$2; shift 2
	[ -f $ROOT/net ] || exit 1
	if [ -n "$rec" ]; then
		set -- "$@" $(for n in "$@"; do cat $ROOT/deps-$n 2>/dev/null; done)
	fi
	# As apk 3.0.5: names alone, no versions. The highest build the feeds
	# have, another than 1.0-r1 where highest-<name> says so; with -X, the
	# build that feed has (feed-has).
	for n in "$@"; do
		case $n in *=*|*'<'*|*'>'*|*'~'*) echo "$n: unable to select package (or its dependencies)"; exit 1 ;; esac
		if [ -n "$feed" ]; then
			v=$(sed -n "s/^$n-\([0-9].*\)/\1/p" $ROOT/feed-has 2>/dev/null | head -1)
			[ -n "$v" ] || exit 1
		else
			grep -qx "$n" $ROOT/available || grep -qx "$n" $ROOT/installed || continue
			v=$(cat $ROOT/highest-$n 2>/dev/null || echo 1.0-r1)
		fi
		echo "$n" >"$dir/$n-$v.apk"
	done
	exit 0 ;;
policy)
	# Each build of $1, and where it is: the one installed, the feeds'
	# highest, and those of the -X feed (feed-has).
	n=$1
	echo "$n policy:"
	iv=; grep -qx "$n" $ROOT/installed && iv=$(cat $ROOT/installed-version-$n 2>/dev/null || echo 1.0-r1)
	hv=; grep -qx "$n" $ROOT/available && hv=$(cat $ROOT/highest-$n 2>/dev/null || echo 1.0-r1)
	fv=$(sed -n "s/^$n-\([0-9].*\)/\1/p" $ROOT/feed-has 2>/dev/null)
	for v in $(printf '%s\n' $iv $hv $fv | sort -u); do
		echo "  $v:"
		[ "$v" = "$iv" ] && echo "    lib/apk/db/installed"
		echo "$fv" | grep -qx "$v" && echo "    https://manager/feed/packages.adb"
		[ "$v" = "$hv" ] && echo "    https://manager/base/packages.adb"
	done
	exit 0 ;;
fix)
	for a in "$@"; do p=$a; done
	[ -f $ROOT/net ] && grep -qx "$p" $ROOT/installed || exit 1
	case $p in wpad*) touch $ROOT/usr/sbin/wpad $ROOT/usr/sbin/hostapd ;; esac
	echo "fix $p" >>$ROOT/calls ;;
add)
	while [ "$1" = --repositories-file ]; do shift 2; done
	if [ "$1" = --allow-untrusted ]; then
		shift
		files=
		for a in "$@"; do case $a in *.apk) [ -f $ROOT/unfit-file-${a##*/} ] && exit 1; files="$files ${a##*/}" ;; esac; done
		unmet "$@" && exit 1
		for f in $files; do
			n=$(echo "$f" | sed 's/-[0-9].*//')
			grep -qx "$n" $ROOT/installed || echo $n >>$ROOT/installed
			# A module's file built against the image's libraries, where
			# swaps-<module> says so: it loads once in (0343).
			case $n in ucode-mod-*) [ -f "$ROOT/swaps-${n#ucode-mod-}" ] && rm -f "$ROOT/unfit-${n#ucode-mod-}" ;; esac
		done
		echo "add-files$files" >>$ROOT/calls
		exit 0
	fi
	[ -f $ROOT/net ] || exit 1
	if [ -n "$feed" ]; then
		for a in "$@"; do
			grep -qx "${a%%=*}-${a#*=}" $ROOT/feed-has 2>/dev/null || exit 1
			grep -qx "${a%%=*}" $ROOT/installed || echo ${a%%=*} >>$ROOT/installed
			n=${a%%=*}; rm -f "$ROOT/unfit-${n#ucode-mod-}"
			echo "feed $a" >>$ROOT/calls
		done
		exit 0
	fi
	if [ "$1" = --upgrade ]; then
		shift
		for a in "$@"; do grep -qx "$a" $ROOT/available || exit 1; done
		for a in "$@"; do
			grep -qx "$a" $ROOT/installed || echo $a >>$ROOT/installed
			[ -f $ROOT/fits-$a ] && rm -f "$ROOT/unfit-${a#ucode-mod-}"
			[ -f $ROOT/newer-$a ] && touch "$ROOT/unfit-${a#ucode-mod-}"
			echo "upgrade $a" >>$ROOT/calls
		done
		exit 0
	fi
	# A build by name=version (0318) is the name's, as available has it.
	for a in "$@"; do case $a in !*) ;; *) grep -qx "${a%%=*}" $ROOT/available || exit 1 ;; esac; done
	new=
	for a in "$@"; do case $a in
		!*) grep -vx "${a#!}" $ROOT/installed >$ROOT/installed.new; mv $ROOT/installed.new $ROOT/installed ;;
		*) a=${a%%=*}; grep -qx "$a" $ROOT/installed || { echo $a >>$ROOT/installed; new="$new $a"; [ -f $ROOT/newer-$a ] && touch "$ROOT/unfit-${a#ucode-mod-}"; } ;;
	esac; done
	# as apk: a package installed already is left as it is, files and all
	if ! grep -q '^wpad' $ROOT/installed; then
		rm -f $ROOT/usr/sbin/wpad $ROOT/usr/sbin/hostapd
	else
		case "$new" in *wpad*) touch $ROOT/usr/sbin/wpad $ROOT/usr/sbin/hostapd ;; esac
	fi
	echo "add $*" >>$ROOT/calls
	[ -f $ROOT/script-fails ] && exit 1
	exit 0 ;;
*) exit 1 ;;
esac
`

const fakeFetch = `#!/bin/sh
quiet=
for a in "$@"; do [ "$a" = -q ] && quiet=1; done
[ -f $ROOT/net ] || { echo "Failed to send request: Operation not permitted" >&2; exit 4; }
# The builds the manager has feeds for (0280), where builds is; a package
# fetched with -O from a build's feed, where feed-files names it.
out= url=
while [ $# -gt 0 ]; do case $1 in -O) out=$2; shift 2 ;; -*) shift ;; *) url=$1; shift ;; esac; done
case $url in
*/feeds/kmods/builds)
	[ -f $ROOT/builds ] || { [ -n "$quiet" ] || echo "HTTP error 404" >&2; exit 8; }
	cat $ROOT/builds; exit 0 ;;
*/feeds/kmods/*/v[0-9]*/*/*/packages.adb)
	# A build's own package feeds beside its module feed (0315), where
	# build-arch-feed is.
	[ -f $ROOT/build-arch-feed ] || { [ -n "$quiet" ] || echo "HTTP error 404" >&2; exit 8; }
	exit 0 ;;
*/feeds/kmods/*/v[0-9]*/*.apk)
	f=${url##*/}
	grep -qx "$f" $ROOT/feed-files 2>/dev/null || { [ -n "$quiet" ] || echo "HTTP error 404" >&2; exit 8; }
	echo "$f" >"$out"; exit 0 ;;
esac
set -- $url
# A build's own module feed (0260) is there only where build-feed is.
for a in "$@"; do
	case $a in */feeds/kmods/*/v[0-9]*/packages.adb) [ -f $ROOT/build-feed ] || { [ -n "$quiet" ] || echo "HTTP error 404" >&2; exit 8; } ;; esac
done
[ -f $ROOT/feed ] && exit 0
[ -n "$quiet" ] || echo "HTTP error 404" >&2
exit 8
`

// fakeUcode loads a module unless it is marked unfit-<module>: one installed
// for another ucode than the image's (2026-10-24).
const fakeUcode = `#!/bin/sh
m=$(echo "$2" | sed -n "s/.*require('\\([a-z0-9_]*\\)').*/\\1/p")
[ -n "$m" ] && [ -f $ROOT/unfit-$m ] && { echo "Unable to dlopen file '/usr/lib/ucode/$m.so': symbol not found" >&2; exit 1; }
exit 0
`

const fakeDf = `#!/bin/sh
echo "Filesystem 1024-blocks Used Available Capacity Mounted on"
echo "overlayfs:/overlay 40000 1000 $(cat $ROOT/free 2>/dev/null || echo 30000) 3% /overlay"
`

const fakeCalls = `#!/bin/sh
echo "$(basename $0) $*" >>$ROOT/calls
`

// agentRestart is how the script has the agent started again on an AP:
// through rpcd's rc object, which runs the init script outside the agent's
// service (OpenWrt main kills a service's processes when it stops it).
const agentRestart = `ubus call rc init {"name":"aeolus","action":"restart"}`

// agentStart starts what of the agent is not running, the same way (0237).
const agentStart = `ubus call rc init {"name":"aeolus","action":"start"}`

const fakeUci = `#!/bin/sh
[ "$*" = "-q get aeolus.agent.url" ] && echo https://manager:8443
exit 0
`

// packagesRig makes the test's directory: the script with its paths moved
// in, the stand-ins on PATH, and opkg's or apk's state.
func packagesRig(t *testing.T, pm string, installed, available []string) (root string, run func(args ...string) (string, error)) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	root = t.TempDir()
	script, err := Files.ReadFile("files/usr/libexec/aeolus-packages")
	if err != nil {
		t.Fatal(err)
	}
	s := string(script)
	// One pass, /rom/ first, so that /rom/lib/apk/ is the image's and not
	// moved twice. With forward slashes: sh would take a Windows path's
	// backslashes for escapes.
	var moves []string
	for _, d := range []string{"/rom/", "/etc/os-release", "/etc/openwrt_release", "/etc/init.d/", "/etc/aeolus/", "/usr/lib/aeolus/", "/var/run/", "/etc/apk/", "/etc/opkg/", "/lib/apk/", "/etc/ssl/certs/", "/tmp/aeolus-wpad", "/tmp/aeolus-copies", "/tmp/aeolus-next", "/tmp/aeolus-rom", "/usr/sbin/wpad", "/usr/sbin/hostapd"} {
		moves = append(moves, d, filepath.ToSlash(root)+d)
	}
	s = strings.NewReplacer(moves...).Replace(s)
	bin := filepath.Join(root, "bin")
	for _, d := range []string{bin, root + "/etc/init.d", root + "/etc/aeolus", root + "/usr/lib/aeolus", root + "/var/run", root + "/etc/apk/repositories.d", root + "/lib/apk/db", root + "/tmp", root + "/usr/sbin", root + "/rom/usr/lib/opkg/info", root + "/rom/lib/apk/db", root + "/rom/usr/sbin"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		root + "/aeolus-packages": s,
		bin + "/uci":              fakeUci,
		bin + "/wifi":             fakeCalls,
		bin + "/ubus":             fakeCalls,
		bin + "/logger":           "#!/bin/sh\n[ \"$1\" = -t ] && shift 2\necho \"$*\" >>\"$ROOT/log\"\nexit 0\n",
		bin + "/uclient-fetch":    fakeFetch,
		bin + "/df":               fakeDf,
		bin + "/ucode":            fakeUcode,
		root + "/etc/init.d/wpad": fakeCalls, root + "/etc/init.d/aeolus": fakeCalls, root + "/etc/init.d/network": fakeCalls,
	}
	if pm == "apk" {
		files[bin+"/apk"] = fakeApk
	} else {
		files[bin+"/opkg"] = fakeOpkg
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(root+"/installed", []byte(strings.Join(installed, "\n")+"\n"), 0o644)
	for _, p := range installed {
		if strings.HasPrefix(p, "wpad") {
			touch(t, root+"/usr/sbin/wpad")
			touch(t, root+"/usr/sbin/hostapd")
		}
	}
	os.WriteFile(root+"/available", []byte(strings.Join(available, "\n")+"\n"), 0o644)
	os.WriteFile(root+"/calls", nil, 0o644)
	touch(t, root+"/rom/usr/sbin/hostapd") // the image came with one
	return root, func(args ...string) (string, error) {
		cmd := exec.Command("sh", append([]string{root + "/aeolus-packages"}, args...)...)
		cmd.Env = []string{"ROOT=" + root, "PATH=" + shellPath(sh, bin)}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
}

// shellPath is the PATH a script run with sh gets: bin first, then the
// shell's own tools. On Windows, Git's sh takes a Windows list, which it
// makes its own, and its tools are in its usrin, beside it or a level up.
func shellPath(sh, bin string) string {
	if runtime.GOOS != "windows" {
		return bin + ":/usr/bin:/bin"
	}
	dir := filepath.Dir(sh)
	return strings.Join([]string{bin, dir, filepath.Join(dir, "..", "usr", "bin")}, ";")
}

func lines(p string) []string {
	b, _ := os.ReadFile(p)
	return strings.Fields(string(b))
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func calls(root string) string {
	b, _ := os.ReadFile(root + "/calls")
	return string(b)
}

// required is what the agent cannot start without, all installed here, so
// that restore goes on to the rest.
var required = strings.Fields("ucode-mod-fs ucode-mod-uci ucode-mod-ubus ucode-mod-uloop ucode-mod-uclient ucode-mod-math ucode-mod-log ucode-mod-digest")

var wpads = []string{"wpad-mbedtls", "wpad-basic-mbedtls"}

// After a sysupgrade under opkg, before the network is up, there is no
// package index: restore leaves the image's wpad-basic alone and the Wi-Fi
// as it is, and tries again later. With the network up, it replaces it.
func TestPackagesRestoreKeepsWpadWithoutIndex(t *testing.T) {
	root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), append(wpads, "usteer"))
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)

	if out, err := run("restore"); err == nil {
		t.Errorf("restore without an index said done: %s", out)
	}
	if got := lines(root + "/installed"); !contains(got, "wpad-basic-mbedtls") || contains(got, "wpad-mbedtls") {
		t.Fatalf("without an index the wpad changed: installed %v", got)
	}
	if c := calls(root); c != "" {
		t.Errorf("without an index, ran: %s", c)
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err == nil {
		t.Errorf("marked done without an index")
	}

	touch(t, root+"/net")
	if out, err := run("restore"); err != nil {
		t.Fatalf("restore with the network up: %v\n%s", err, out)
	}
	got := lines(root + "/installed")
	if contains(got, "wpad-basic-mbedtls") || !contains(got, "wpad-mbedtls") || !contains(got, "usteer") {
		t.Fatalf("with an index: installed %v", got)
	}
	if !strings.Contains(calls(root), "wifi") {
		t.Errorf("the Wi-Fi was not brought up on the full wpad: %s", calls(root))
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err != nil {
		t.Errorf("not marked done")
	}
}

// One feed that cannot be read makes opkg update fail, yet the others were
// read: the wpad is replaced all the same, and restore is done.
func TestPackagesOneBadFeed(t *testing.T) {
	root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
	os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
	touch(t, root+"/net")
	touch(t, root+"/badfeed")
	if out, err := run("restore"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := lines(root + "/installed"); !contains(got, "wpad-mbedtls") {
		t.Fatalf("installed %v", got)
	}
}

// Under opkg nothing is taken away where it cannot be put back: no full wpad
// to be had, opkg's lock held by another, or the basic one that will not be
// removed. The Wi-Fi is left alone each time.
func TestPackagesFullWpadNothingTakenAway(t *testing.T) {
	for name, c := range map[string]struct {
		available []string
		file      string
		body      string
	}{
		"no full wpad":  {[]string{"wpad-basic-mbedtls"}, "", ""},
		"lock held":     {wpads, "locked", ""},
		"remove failed": {wpads, "remove-fails", ""},
	} {
		root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), c.available)
		os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
		touch(t, root+"/net")
		if c.file != "" {
			os.WriteFile(root+"/"+c.file, []byte(c.body), 0o644)
		}
		run("restore")
		if got := lines(root + "/installed"); !contains(got, "wpad-basic-mbedtls") {
			t.Errorf("%s: the basic wpad went: %v", name, got)
		}
		if c := calls(root); strings.Contains(c, "wifi") || strings.Contains(c, "wpad") {
			t.Errorf("%s: nothing changed, yet ran: %s", name, c)
		}
	}
}

// Under opkg, a full wpad that will not install: the basic one goes back
// from its copy and the Wi-Fi comes up on it. Its copy that will not
// install either: the basic one from the feeds, at once. Neither at all:
// the AP says so, and restore is not done.
func TestPackagesWpadPutBack(t *testing.T) {
	for name, c := range map[string]struct {
		fail  []string
		basic bool
	}{
		"from its copy":  {[]string{"fail-wpad-mbedtls"}, true},
		"from the feeds": {[]string{"fail-wpad-mbedtls", "fail-ipk-wpad-basic-mbedtls"}, true},
		"not at all":     {[]string{"fail-wpad-mbedtls", "fail-wpad-basic-mbedtls"}, false},
	} {
		root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
		os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
		touch(t, root+"/net")
		for _, f := range c.fail {
			touch(t, root+"/"+f)
		}
		out, err := run("restore")
		got := lines(root + "/installed")
		if contains(got, "wpad-mbedtls") || contains(got, "wpad-basic-mbedtls") != c.basic {
			t.Errorf("%s: installed %v", name, got)
		}
		if !strings.Contains(calls(root), "wifi") {
			t.Errorf("%s: the basic wpad went, but the Wi-Fi was not brought up again: %s", name, calls(root))
		}
		if err == nil {
			t.Errorf("%s: restore said done without the full wpad", name)
		}
		if !c.basic && !strings.Contains(out, "the Wi-Fi is down") {
			t.Errorf("%s: no wpad, and not said: %s", name, out)
		}
		if _, err := os.Stat(root + "/usr/lib/aeolus/wpad-refused"); (err == nil) != c.basic {
			t.Errorf("%s: refused written %v, the basic one back %v", name, err == nil, c.basic)
		}
	}
}

// Under apk the swap is one change: refused, nothing changed and the Wi-Fi
// is left alone.
func TestPackagesApkSwapRefused(t *testing.T) {
	root, run := packagesRig(t, "apk", append([]string{"wpad-basic-mbedtls"}, required...), []string{"wpad-basic-mbedtls"})
	touch(t, root+"/net")
	os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
	run("restore")
	if got := lines(root + "/installed"); !contains(got, "wpad-basic-mbedtls") {
		t.Fatalf("installed %v", got)
	}
	if c := calls(root); c != "" {
		t.Errorf("nothing changed, yet ran: %s", c)
	}
}

// Under apk, the swap made: the full wpad in the basic one's place, and the
// Wi-Fi brought up on it; an add that failed after the basic one went (a
// package's script) still brings the Wi-Fi up on what is there.
func TestPackagesApkSwap(t *testing.T) {
	for name, fails := range map[string]bool{"made": false, "script failed": true} {
		root, run := packagesRig(t, "apk", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
		touch(t, root+"/net")
		if fails {
			touch(t, root+"/script-fails")
		}
		os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
		run("restore")
		if got := lines(root + "/installed"); !contains(got, "wpad-mbedtls") || contains(got, "wpad-basic-mbedtls") {
			t.Errorf("%s: installed %v", name, got)
		}
		if !strings.Contains(calls(root), "wifi") {
			t.Errorf("%s: the Wi-Fi was not brought up on the full wpad: %s", name, calls(root))
		}
	}
}

// A module feed the manager has only after the AP's packages went in: once
// found, the modules that could not be had go on the list (and nothing
// that would need configuring) and restore is due again. The manager
// unreachable leaves a feed line alone; its 404 takes it away.
func TestPackagesKmodsFoundLater(t *testing.T) {
	for _, done := range []bool{true, false} { // installed long ago, or just sysupgraded
		root, run := packagesRig(t, "apk", append([]string{"usteer"}, required...), nil)
		os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.12.112~abc-r1\n\n"), 0o644)
		os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
		if done {
			touch(t, root+"/usr/lib/aeolus/packages")
		}
		touch(t, root+"/net")
		touch(t, root+"/feed")
		run("feeds")
		list := lines(root + "/etc/aeolus/packages")
		if !contains(list, "kmod-veth") || !contains(list, "vxlan") || !contains(list, "kmod-nft-bridge") || contains(list, "snmpd-ssl") {
			t.Errorf("done %v: the list %v", done, list)
		}
		if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err == nil {
			t.Errorf("done %v: restore not due", done)
		}
		if line, _ := os.ReadFile(root + "/etc/apk/repositories.d/aeolus-kmods.list"); !strings.Contains(string(line), "6.12.112~abc-r1") {
			t.Errorf("done %v: feed line %q", done, line)
		}
	}
	root, run := packagesRig(t, "apk", append([]string{"usteer"}, required...), nil)
	os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.12.112~abc-r1\n\n"), 0o644)
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
	touch(t, root+"/usr/lib/aeolus/packages")
	touch(t, root+"/net")
	touch(t, root+"/feed")
	if out, err := run("feeds"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	kmods := root + "/etc/apk/repositories.d/aeolus-kmods.list"
	if line, _ := os.ReadFile(kmods); !strings.Contains(string(line), "/feeds/kmods/6.12.112~abc-r1/packages.adb") {
		t.Fatalf("feed line %q", line)
	}
	list := lines(root + "/etc/aeolus/packages")
	if !contains(list, "kmod-veth") || !contains(list, "vxlan") || !contains(list, "kmod-nft-bridge") {
		t.Errorf("the modules a feed brings were not put on the list: %v", list)
	}
	if contains(list, "snmpd-ssl") {
		t.Errorf("snmpd-ssl, which needs configuring, was put on the list: %v", list)
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err == nil {
		t.Errorf("restore not due again")
	}

	os.WriteFile(kmods, []byte("https://manager:8443/feeds/kmods/6.12.0~old-r1/packages.adb\n"), 0o644)
	os.Remove(root + "/feed")
	os.Remove(root + "/net")
	run("feeds") // the manager cannot be asked
	if _, err := os.Stat(kmods); err != nil {
		t.Errorf("an unreachable manager took the feed line away")
	}
	touch(t, root+"/net")
	run("feeds") // the manager answers 404
	if _, err := os.Stat(kmods); err == nil {
		t.Errorf("the manager's 404 left the feed line")
	}
}

// A port image asks first for its own build's module feed, by its kernel
// and its build's tag (0260), and takes the kernel's where the manager has
// none of its build; one found later is taken in place of the kernel's. An
// image of OpenWrt's own, with no build tag, asks by its kernel alone.
func TestPackagesKmodsByBuild(t *testing.T) {
	root, run := packagesRig(t, "apk", append([]string{"usteer"}, required...), nil)
	os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.18.55~ff43a0f8-r1\n\n"), 0o644)
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
	os.WriteFile(root+"/etc/os-release", []byte("NAME=\"OpenWrt\"\nOPENWRT_RELEASE=\"OpenWrt SNAPSHOT r0+589-bcb16b63472 (arista v2026.10.17.1-83-g439336f)\"\n"), 0o644)
	touch(t, root+"/net")
	touch(t, root+"/feed")
	kmods := root + "/etc/apk/repositories.d/aeolus-kmods.list"
	byKernel := "/feeds/kmods/6.18.55~ff43a0f8-r1/packages.adb"
	byBuild := "/feeds/kmods/6.18.55~ff43a0f8-r1/v2026.10.17.1-83-g439336f/packages.adb"
	run("feeds") // the manager has no feed of this build
	if line, _ := os.ReadFile(kmods); !strings.Contains(string(line), byKernel) {
		t.Fatalf("no build feed: %q", line)
	}
	touch(t, root+"/build-feed")
	touch(t, root+"/usr/lib/aeolus/packages")
	if out, err := run("feeds"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if line, _ := os.ReadFile(kmods); !strings.Contains(string(line), byBuild) {
		t.Fatalf("its build's feed, found later: %q", line)
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err == nil {
		t.Errorf("restore not due again with the build's feed")
	}
	// An image of OpenWrt's own.
	os.WriteFile(root+"/etc/os-release", []byte("OPENWRT_RELEASE=\"OpenWrt 25.12.2 r32802-f505120278\"\n"), 0o644)
	os.Remove(kmods)
	run("feeds")
	if line, _ := os.ReadFile(kmods); !strings.Contains(string(line), byKernel) {
		t.Errorf("no build tag: %q", line)
	}
}

// restore after a sysupgrade, when the manager's module feed is first found:
// the modules it brings are installed in the same run, and the network
// restarted for vxlan.
func TestPackagesRestoreFeedFirst(t *testing.T) {
	inst := append([]string{"usteer", "wpad-mbedtls"}, required[:len(required)-1]...) // ucode-mod-digest missing
	root, run := packagesRig(t, "apk", inst, []string{"ucode-mod-digest", "vxlan", "kmod-nft-bridge", "kmod-veth"})
	os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.12.112~abc-r1\n\n"), 0o644)
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
	touch(t, root+"/net")
	touch(t, root+"/feed")
	if out, err := run("restore"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	got := lines(root + "/installed")
	for _, p := range []string{"ucode-mod-digest", "vxlan", "kmod-nft-bridge", "kmod-veth"} {
		if !contains(got, p) {
			t.Errorf("%s not installed: %v", p, got)
		}
	}
	c := calls(root)
	if !strings.Contains(c, "network restart") {
		t.Errorf("the network was not restarted for vxlan: %s", c)
	}
	// The agent held its config for vxlan and does not render the same
	// config twice in a run: it starts again once the network has vxlan.
	n, a := strings.Index(c, "network restart"), strings.LastIndex(c, agentRestart)
	if a < 0 || a < n {
		t.Errorf("the agent was not started again after the network: %s", c)
	}
}

// A module installed for another ucode than the image's does not load
// (2026-10-24: OpenWrt's snapshot uclient on a port image of an older
// ucode). restore and required install it again, a fitting build where a
// feed has one, as the AP's own build's may; where none fits, they say so,
// and restore is due again.
func TestPackagesUnfitModule(t *testing.T) {
	setup := func(fits bool) (string, func(args ...string) (string, error)) {
		inst := append([]string{"usteer", "wpad-mbedtls"}, required...)
		root, run := packagesRig(t, "apk", inst, append([]string{"usteer"}, required...))
		os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
		touch(t, root+"/usr/lib/aeolus/packages") // restored before, as after the sysupgrade
		touch(t, root+"/net")
		touch(t, root+"/unfit-uclient")
		if fits {
			touch(t, root+"/fits-ucode-mod-uclient")
		}
		return root, run
	}

	root, run := setup(true)
	if out, err := run("restore"); err != nil {
		t.Fatalf("restore: %v\n%s", err, out)
	}
	if c := calls(root); !strings.Contains(c, "upgrade ucode-mod-uclient") || !strings.Contains(c, agentRestart) {
		t.Errorf("the unfit uclient was not installed again, and the agent started: %s", c)
	}
	if _, err := os.Stat(root + "/unfit-uclient"); err == nil {
		t.Errorf("uclient still does not load")
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err != nil {
		t.Errorf("restore is not done: %v", err)
	}

	root, run = setup(false)
	out, err := run("restore")
	if err == nil || !strings.Contains(out, "ucode-mod-uclient is installed but does not load on this image's ucode") {
		t.Errorf("none fits, and restore said: %v\n%s", err, out)
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err == nil {
		t.Errorf("restore is done, with uclient unfit")
	}

	root, run = setup(true)
	os.Remove(root + "/usr/lib/aeolus/packages")
	if out, err := run("required"); err != nil || !strings.Contains(out, "installed what the agent needs: ucode-mod-uclient") {
		t.Errorf("required: %v\n%s", err, out)
	}
	_, run = setup(false)
	if out, err := run("required"); err == nil || !strings.Contains(out, "the agent cannot start without it") {
		t.Errorf("required, none fits: %v\n%s", err, out)
	}
}

// The ucode modules come from the manager's module feed for the AP's
// kernel alone, at the version it has there, where it has them (0229): the
// other feeds' builds, OpenWrt's snapshot's, are newer and for a newer
// ucode than a port image's, and apk takes the highest version (-49,
// 2026-10-28). One the feed does not have comes from the other feeds.
func TestPackagesModulesFromTheirFeed(t *testing.T) {
	const pinned = "ucode-mod-uclient=2026.06.07~daad21fa-r2"
	without := func(l []string, s string) []string {
		var out []string
		for _, x := range l {
			if x != s {
				out = append(out, x)
			}
		}
		return out
	}
	rig := func(installed []string, has bool) (string, func(args ...string) (string, error)) {
		root, run := packagesRig(t, "apk", append([]string{"usteer", "wpad-mbedtls"}, installed...), []string{"usteer", "ucode-mod-uclient", "vxlan", "kmod-nft-bridge", "kmod-veth"})
		os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.18.55~ff43a0f8-r1\n\n"), 0o644)
		os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
		touch(t, root+"/net")
		touch(t, root+"/feed")
		touch(t, root+"/newer-ucode-mod-uclient")
		if has {
			os.WriteFile(root+"/feed-has", []byte("ucode-mod-uclient-2026.06.07~daad21fa-r2\n"), 0o644)
		}
		return root, run
	}

	// After a sysupgrade, uclient missing: the feed's, and only it.
	root, run := rig(without(required, "ucode-mod-uclient"), true)
	if out, err := run("restore"); err != nil {
		t.Fatalf("restore: %v\n%s", err, out)
	}
	c := calls(root)
	if !strings.Contains(c, "feed "+pinned) || strings.Contains(c, "add ucode-mod-uclient") || strings.Contains(c, "upgrade ucode-mod-uclient") {
		t.Errorf("uclient not from its feed alone: %s", c)
	}
	if _, err := os.Stat(root + "/unfit-uclient"); err == nil {
		t.Errorf("uclient does not load")
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err != nil {
		t.Errorf("restore is not done: %v", err)
	}

	// The snapshot's there already, and not loading: the feed's in its place.
	root, run = rig(required, true)
	touch(t, root+"/unfit-uclient")
	touch(t, root+"/usr/lib/aeolus/packages")
	if out, err := run("restore"); err != nil {
		t.Fatalf("restore, unfit: %v\n%s", err, out)
	}
	if c := calls(root); !strings.Contains(c, "feed "+pinned) || strings.Contains(c, "upgrade ucode-mod-uclient") {
		t.Errorf("unfit uclient not put right from its feed: %s", c)
	}

	// required, as the agent starts: the same.
	root, run = rig(without(required, "ucode-mod-uclient"), true)
	if out, err := run("required"); err != nil || !strings.Contains(out, "installed what the agent needs: ucode-mod-uclient") {
		t.Errorf("required: %v\n%s", err, out)
	}
	if c := calls(root); !strings.Contains(c, "feed "+pinned) || strings.Contains(c, "add ucode-mod-uclient") {
		t.Errorf("required: uclient not from its feed: %s", c)
	}

	// The feed's build is for another ucode than the image's (0256): passed
	// over, and the other feeds' taken, which fits.
	root, run = rig(without(required, "ucode-mod-uclient"), true)
	touch(t, root+"/feed-other-ucode-ucode-mod-uclient")
	os.Remove(root + "/newer-ucode-mod-uclient")
	if out, err := run("restore"); err != nil {
		t.Fatalf("restore, the feed's for another ucode: %v\n%s", err, out)
	}
	if c := calls(root); strings.Contains(c, "feed "+pinned) || !strings.Contains(c, "add ucode-mod-uclient") {
		t.Errorf("the feed's build for another ucode was not passed over: %s", c)
	}

	// A feed without it: the other feeds', which here does not load, as
	// before.
	root, run = rig(without(required, "ucode-mod-uclient"), false)
	out, err := run("restore")
	if err == nil || !strings.Contains(out, "ucode-mod-uclient is installed but does not load") {
		t.Errorf("no module in the feed: %v\n%s", err, out)
	}
	if c := calls(root); strings.Contains(c, "feed ") {
		t.Errorf("installed from a feed that has none: %s", c)
	}
}

// restore that puts back only a package the config needs, vxlan here: the
// agent held that config and does not render it twice in a run, so it
// starts again, after the network (a C-260, 2026-10-17).
func TestPackagesRestoreStartsAgentForConfig(t *testing.T) {
	root, run := packagesRig(t, "apk", append([]string{"usteer", "wpad-mbedtls"}, required...), []string{"vxlan"})
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\nvxlan\n"), 0o644)
	touch(t, root+"/net")
	if out, err := run("restore"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !contains(lines(root+"/installed"), "vxlan") {
		t.Fatalf("vxlan not installed")
	}
	c := calls(root)
	n, a := strings.Index(c, "network restart"), strings.LastIndex(c, agentRestart)
	if n < 0 || a < n {
		t.Errorf("the agent was not started again after the network: %s", c)
	}
}

// restore that could put nothing back, vxlan missing from the feeds, starts
// nothing again and says it is not done.
func TestPackagesRestoreNothingBack(t *testing.T) {
	root, run := packagesRig(t, "apk", append([]string{"usteer", "wpad-mbedtls"}, required...), nil)
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\nvxlan\n"), 0o644)
	touch(t, root+"/net")
	if out, err := run("restore"); err == nil {
		t.Errorf("restore without vxlan to be had said done: %s", out)
	}
	if c := calls(root); strings.Contains(c, agentRestart) || strings.Contains(c, "aeolus restart") || strings.Contains(c, "network restart") {
		t.Errorf("with nothing put back, ran: %s", c)
	}
}

// The init script's boot run tries restore again until the modules the agent
// cannot start without load (0234): at boot the AP may have no address yet
// (a C-260 on the port's -54, 2026-10-28, its agent down until restore was
// run by hand). Here the network is up after the third wait, each twice the
// one before. Once they load, it starts what of the agent the init script
// could not (0237); where they all load already, it does only that.
func TestPackagesBootTriesAgain(t *testing.T) {
	var have []string
	for _, p := range required {
		if p != "ucode-mod-math" && p != "ucode-mod-uclient" {
			have = append(have, p)
		}
	}
	root, run := packagesRig(t, "apk", append([]string{"usteer", "wpad-mbedtls"}, have...), []string{"ucode-mod-math", "ucode-mod-uclient", "vxlan"})
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\nvxlan\n"), 0o644)
	os.WriteFile(root+"/bin/sleep", []byte("#!/bin/sh\necho \"sleep $*\" >>$ROOT/calls\n[ $(grep -c '^sleep' $ROOT/calls) -ge 3 ] && touch $ROOT/net\nexit 0\n"), 0o755)
	out, err := run("boot")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	got := lines(root + "/installed")
	for _, p := range []string{"ucode-mod-math", "ucode-mod-uclient", "vxlan"} {
		if !contains(got, p) {
			t.Errorf("%s not installed: %v", p, got)
		}
	}
	var waits []string
	for _, l := range strings.Split(calls(root), "\n") {
		if strings.HasPrefix(l, "sleep ") {
			waits = append(waits, l)
		}
	}
	if strings.Join(waits, ",") != "sleep 30,sleep 60,sleep 120" {
		t.Errorf("the waits: %v", waits)
	}
	if strings.Count(out, "the agent cannot start without ucode-mod-uclient ucode-mod-math; restore tries again") != 1 {
		t.Errorf("not said once what the agent waits for:\n%s", out)
	}
	if _, err := os.Stat(root + "/var/run/aeolus-packages.boot"); err == nil {
		t.Errorf("the boot run's lock is still there")
	}
	if c := calls(root); !strings.HasSuffix(strings.TrimSpace(c), agentStart) {
		t.Errorf("the agent was not started at the end: %s", c)
	}
	os.WriteFile(root+"/calls", nil, 0o644)
	if out, err := run("boot"); err != nil || strings.TrimSpace(calls(root)) != agentStart {
		t.Errorf("with every module there, boot ran %q: %v\n%s", calls(root), err, out)
	}
}

// Where rpcd's rc object cannot be asked (ubus fails), the agent is
// restarted by its init script, after the ask.
func TestPackagesRestartWithoutRc(t *testing.T) {
	root, run := packagesRig(t, "apk", append([]string{"usteer", "wpad-mbedtls"}, required...), []string{"vxlan"})
	os.WriteFile(root+"/bin/ubus", []byte("#!/bin/sh\necho \"ubus $*\" >>$ROOT/calls\nexit 1\n"), 0o755)
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\nvxlan\n"), 0o644)
	touch(t, root+"/net")
	if out, err := run("restore"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	c := calls(root)
	u, i := strings.Index(c, agentRestart), strings.Index(c, "aeolus restart")
	if u < 0 || i < u {
		t.Errorf("rc was not asked first, then the init script: %s", c)
	}
}

// A lock whose run is alive keeps a second run out; one its run left behind,
// the process gone, is taken over (a W-318, 2026-10-18: every run said
// "already running" until a reboot).
func TestPackagesStaleLock(t *testing.T) {
	// The script reads the lock holder's /proc/<pid>/cmdline. On Windows, Go
	// sees Windows process IDs, which Git's sh has no /proc entry for.
	if runtime.GOOS == "windows" {
		t.Skip("needs the lock holder's /proc entry, which Windows doesn't give sh")
	}
	root, run := packagesRig(t, "apk", append([]string{"usteer", "wpad-mbedtls"}, required...), []string{"vxlan"})
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\nvxlan\n"), 0o644)
	touch(t, root+"/net")
	lock := root + "/var/run/aeolus-packages.lock"
	os.MkdirAll(lock, 0o755)
	// a live run: a process whose command line names the script
	live := exec.Command("sh", "-c", "sleep 30; :", "aeolus-packages")
	if err := live.Start(); err != nil {
		t.Fatal(err)
	}
	defer live.Process.Kill()
	os.WriteFile(lock+"/pid", []byte(strconv.Itoa(live.Process.Pid)+"\n"), 0o644)
	if out, _ := run("restore"); !strings.Contains(out, "already running") || contains(lines(root+"/installed"), "vxlan") {
		t.Fatalf("a live run's lock did not keep this one out: %s", out)
	}
	// its pid now another program's: this test's own
	os.WriteFile(lock+"/pid", []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
	if out, err := run("restore"); err != nil || !contains(lines(root+"/installed"), "vxlan") {
		t.Fatalf("a lock whose pid is another program's was not taken over: %v\n%s", err, out)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Errorf("the lock is still there after the run")
	}
}

// The lock an older script left, with no pid in it (the W-318's), is taken
// over after the second a run just starting is given.
func TestPackagesLockWithoutPid(t *testing.T) {
	root, run := packagesRig(t, "apk", append([]string{"usteer", "wpad-mbedtls"}, required...), []string{"vxlan"})
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\nvxlan\n"), 0o644)
	touch(t, root+"/net")
	os.MkdirAll(root+"/var/run/aeolus-packages.lock", 0o755)
	if out, err := run("restore"); err != nil || !contains(lines(root+"/installed"), "vxlan") {
		t.Fatalf("a lock with no pid was not taken over: %v\n%s", err, out)
	}
}

// Under opkg, an overlay without room for both wpads, or a package whose
// size cannot be read: the basic one stays and the Wi-Fi is left alone
// (0172).
func TestPackagesWpadNoRoom(t *testing.T) {
	for name, knob := range map[string][2]string{
		"no room":      {"free", "500\n"},
		"unmeasurable": {"garbage-wpad-mbedtls", ""},
	} {
		root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
		os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
		touch(t, root+"/net")
		os.WriteFile(root+"/"+knob[0], []byte(knob[1]), 0o644)
		run("restore")
		if got := lines(root + "/installed"); !contains(got, "wpad-basic-mbedtls") || contains(got, "wpad-mbedtls") {
			t.Errorf("%s: installed %v", name, got)
		}
		if c := calls(root); c != "" {
			t.Errorf("%s: nothing changed, yet ran: %s", name, c)
		}
	}
	root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
	os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
	touch(t, root+"/net")
	os.WriteFile(root+"/free", []byte("2000\n"), 0o644) // room enough
	run("restore")
	if got := lines(root + "/installed"); !contains(got, "wpad-mbedtls") {
		t.Errorf("with room: installed %v", got)
	}
}

// Under opkg, a full wpad that would not install is not tried again, so
// the Wi-Fi is not restarted each hour: restore is settled; a new version
// of it is tried after the next sysupgrade, and replaces the basic one.
func TestPackagesWpadRefused(t *testing.T) {
	root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
	os.RemoveAll(root + "/usr/lib/aeolus") // just sysupgraded: not there
	os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
	touch(t, root+"/net")
	touch(t, root+"/fail-wpad-mbedtls")
	run("restore")
	if b, _ := os.ReadFile(root + "/usr/lib/aeolus/wpad-refused"); strings.TrimSpace(string(b)) != "wpad-mbedtls_1.0_all.ipk" {
		t.Fatalf("refused %q", b)
	}
	os.WriteFile(root+"/calls", nil, 0o644)
	if out, err := run("restore"); err != nil {
		t.Errorf("a refused wpad kept restore from being done: %v\n%s", err, out)
	}
	if c := calls(root); c != "" {
		t.Errorf("tried again, and ran: %s", c)
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err != nil {
		t.Errorf("not done")
	}

	os.RemoveAll(root + "/usr/lib/aeolus") // a sysupgrade, which drops /usr/lib/aeolus
	os.Remove(root + "/fail-wpad-mbedtls")
	os.WriteFile(root+"/version-wpad-mbedtls", []byte("1.1\n"), 0o644)
	if out, err := run("restore"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := lines(root + "/installed"); !contains(got, "wpad-mbedtls") || contains(got, "wpad-basic-mbedtls") {
		t.Errorf("the new version was not tried: %v", got)
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/wpad-refused"); err == nil {
		t.Errorf("still written down as refused")
	}
}

// A swap cut off between its steps (a power cut) leaves no wpad: restore
// mends it even where it had been done, with the image's own TLS library,
// the full wpad where it can be had, else the basic one, and brings the
// Wi-Fi up on it.
func TestPackagesWpadRepaired(t *testing.T) {
	for _, pm := range []string{"opkg", "apk"} {
		for name, avail := range map[string][]string{
			"full":  {"wpad-openssl", "wpad-basic-openssl"},
			"basic": {"wpad-basic-openssl"},
		} {
			root, run := packagesRig(t, pm, required, avail)
			if pm == "opkg" {
				touch(t, root+"/rom/usr/lib/opkg/info/wpad-basic-openssl.control")
			} else {
				os.WriteFile(root+"/rom/lib/apk/db/installed", []byte("P:wpad-basic-openssl\nV:1\n\n"), 0o644)
			}
			os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
			touch(t, root+"/usr/lib/aeolus/packages")
			touch(t, root+"/net")
			// With only the basic one to be had, restore is not done: the full
			// one is tried again later, as on any AP with the basic one.
			if out, err := run("restore"); err != nil && name == "full" {
				t.Errorf("%s %s: %v\n%s", pm, name, err, out)
			}
			got := lines(root + "/installed")
			want := "wpad-openssl"
			if name == "basic" {
				want = "wpad-basic-openssl"
			}
			if !contains(got, want) {
				t.Errorf("%s %s: installed %v", pm, name, got)
			}
			if !strings.Contains(calls(root), "wifi") {
				t.Errorf("%s %s: the Wi-Fi was not brought up: %s", pm, name, calls(root))
			}
		}
	}
}

// An install that leaves no wpad is not done, so restore comes back to it.
func TestPackagesInstallNotDoneWithoutWpad(t *testing.T) {
	root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
	touch(t, root+"/net")
	touch(t, root+"/fail-wpad-mbedtls")
	touch(t, root+"/fail-wpad-basic-mbedtls")
	run("install")
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err == nil {
		t.Errorf("done without a wpad")
	}
}

// A device built without a hostapd (wired only, a station): nothing is put
// on it, and restore is done (0172).
func TestPackagesNoWpadImage(t *testing.T) {
	root, run := packagesRig(t, "opkg", required, wpads)
	os.Remove(root + "/rom/usr/sbin/hostapd")
	os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
	touch(t, root+"/net")
	if out, err := run("restore"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := lines(root + "/installed"); contains(got, "wpad-mbedtls") || contains(got, "wpad-basic-mbedtls") {
		t.Errorf("a wpad put on a device without one: %v", got)
	}
	if c := calls(root); c != "" {
		t.Errorf("ran: %s", c)
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err != nil {
		t.Errorf("not done")
	}
}

// A swap that lost both wpads (here every install fails; a power cut
// between its steps leaves the same): restore is due and the swap written
// down, so the agent's next restore puts the basic wpad back and then the
// full one, even on an image without a hostapd in /rom.
func TestPackagesSwapCut(t *testing.T) {
	root, run := packagesRig(t, "opkg", append([]string{"wpad-basic-mbedtls"}, required...), wpads)
	os.Remove(root + "/rom/usr/sbin/hostapd")
	os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
	touch(t, root+"/usr/lib/aeolus/packages")
	touch(t, root+"/net")
	touch(t, root+"/fail-wpad-mbedtls")
	touch(t, root+"/fail-wpad-basic-mbedtls")
	run("install")
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err == nil {
		t.Fatalf("done with no wpad")
	}
	if b, _ := os.ReadFile(root + "/etc/aeolus/wpad-swapping"); strings.TrimSpace(string(b)) != "mbedtls" {
		t.Fatalf("swap not written down: %q", b)
	}
	os.Remove(root + "/fail-wpad-mbedtls")
	os.Remove(root + "/fail-wpad-basic-mbedtls")
	os.Remove(root + "/usr/lib/aeolus/wpad-refused")
	os.WriteFile(root+"/calls", nil, 0o644)
	if out, err := run("restore"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := lines(root + "/installed"); !contains(got, "wpad-mbedtls") {
		t.Errorf("installed %v", got)
	}
	if !strings.Contains(calls(root), "wifi") {
		t.Errorf("the Wi-Fi was not brought up: %s", calls(root))
	}
	if _, err := os.Stat(root + "/etc/aeolus/wpad-swapping"); err == nil {
		t.Errorf("the swap still written down")
	}
	if _, err := os.Stat(root + "/usr/lib/aeolus/packages"); err != nil {
		t.Errorf("not done")
	}
}

// A wpad still listed whose files are gone, its full one refused before:
// installed again, not settled without a hostapd.
func TestPackagesWpadFilesGone(t *testing.T) {
	for _, pm := range []string{"opkg", "apk"} {
		root, run := packagesRig(t, pm, append([]string{"wpad-basic-mbedtls"}, required...), wpads)
		os.Remove(root + "/usr/sbin/hostapd")
		os.Remove(root + "/usr/sbin/wpad")
		os.WriteFile(root+"/usr/lib/aeolus/wpad-refused", []byte("wpad-mbedtls_1.0_all.ipk\n"), 0o644)
		os.WriteFile(root+"/etc/aeolus/packages", nil, 0o644)
		touch(t, root+"/net")
		run("restore")
		if _, err := os.Stat(root + "/usr/sbin/hostapd"); err != nil {
			t.Errorf("%s: no hostapd: %s", pm, calls(root))
		}
		want := map[string]string{"opkg": "reinstall wpad-basic-mbedtls", "apk": "fix wpad-basic-mbedtls"}[pm]
		if !strings.Contains(calls(root), want) {
			t.Errorf("%s: not reinstalled (%q): %s", pm, want, calls(root))
		}
		if !strings.Contains(calls(root), "wifi") {
			t.Errorf("%s: the Wi-Fi was not brought up: %s", pm, calls(root))
		}
	}
}

// feeds points the AP's feeds at the manager: OpenWrt's releases at its
// /feeds/, OpenWrt's main snapshots, which an image built from main names,
// at its /feeds/snapshots/ (0230), and another manager's at this one's.
func TestPackagesFeedsThroughTheManager(t *testing.T) {
	root, run := packagesRig(t, "apk", append([]string{"usteer"}, required...), nil)
	list := root + "/etc/apk/repositories.d/distfeeds.list"
	os.WriteFile(list, []byte(strings.Join([]string{
		"https://downloads.openwrt.org/releases/25.12.5/targets/ipq806x/generic/packages/packages.adb",
		"https://downloads.openwrt.org/snapshots/targets/qualcommax/ipq807x/packages/packages.adb",
		"https://downloads.openwrt.org/snapshots/packages/aarch64_cortex-a53/base/packages.adb",
		"https://old-manager:8443/feeds/snapshots/packages/aarch64_cortex-a53/luci/packages.adb",
		"# a comment",
	}, "\n")+"\n"), 0o644)
	if out, err := run("feeds"); err != nil {
		t.Fatalf("feeds: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(list)
	want := strings.Join([]string{
		"https://manager:8443/feeds/25.12.5/targets/ipq806x/generic/packages/packages.adb",
		"https://manager:8443/feeds/snapshots/targets/qualcommax/ipq807x/packages/packages.adb",
		"https://manager:8443/feeds/snapshots/packages/aarch64_cortex-a53/base/packages.adb",
		"https://manager:8443/feeds/snapshots/packages/aarch64_cortex-a53/luci/packages.adb",
		"# a comment",
	}, "\n") + "\n"
	if string(got) != want {
		t.Errorf("feeds:\n%s\nwant:\n%s", got, want)
	}
}

// keep keeps a copy of each package beyond the image's, for early to put
// back after a sysupgrade (0248), and fetches them again only where what is
// installed changed. The agent runs it once restore is done: install and add
// keep none, so an AP set up afresh had none at its first sysupgrade, and
// restore put vxlan back after the network, restarting it (ce7f, 2026-10-31).
func TestPackagesKeep(t *testing.T) {
	image := append([]string{"wpad-basic-mbedtls"}, required...)
	extra := []string{"vxlan", "usteer"}
	root, run := packagesRig(t, "apk", append(image, extra...), append(extra, "tcpdump-mini"))
	var db strings.Builder
	for _, p := range image {
		db.WriteString("P:" + p + "\nV:1\n\n")
	}
	os.WriteFile(root+"/rom/lib/apk/db/installed", []byte(db.String()), 0o644)
	touch(t, root+"/net")
	copies := root + "/etc/aeolus/pkgs/"
	kept := func(when string, want []string) {
		t.Helper()
		if out, err := run("keep"); err != nil {
			t.Fatalf("%s: keep: %v\n%s", when, err, out)
		}
		sums, _ := os.ReadFile(copies + ".sums")
		for _, p := range want {
			f := p + "-1.0-r1.apk"
			if _, err := os.Stat(copies + f); err != nil || !strings.Contains(string(sums), f) {
				t.Errorf("%s: no copy of %s, or not summed: %v\n%s", when, p, err, sums)
			}
		}
		for _, p := range image {
			if _, err := os.Stat(copies + p + "-1.0-r1.apk"); err == nil {
				t.Errorf("%s: a copy of the image's own %s", when, p)
			}
		}
	}
	kept("first", extra)
	fetched := strings.Count(calls(root), "fetch ")
	kept("unchanged", extra)
	if n := strings.Count(calls(root), "fetch "); n != fetched {
		t.Errorf("fetched again with nothing installed changed: %s", calls(root))
	}
	// A package a setting asked for: fetched again, with it.
	f, _ := os.OpenFile(root+"/installed", os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("tcpdump-mini\n")
	f.Close()
	kept("added", append(extra, "tcpdump-mini"))
	// Copies kept by an older rule, under its stamp, are fetched again. apk
	// fetches the highest build the feeds have, which may be one for another
	// ucode (0256: the module feed's uclient on -82), so one of another
	// build than the one installed is fetched again from a feed that has the
	// one installed. One whose installed build no feed has is kept as the
	// highest, and not fetched again and again.
	os.WriteFile(root+"/highest-vxlan", []byte("1.0-r2\n"), 0o644)
	os.WriteFile(root+"/feed-has", []byte("vxlan-1.0-r1\n"), 0o644)
	os.WriteFile(root+"/highest-usteer", []byte("1.0-r3\n"), 0o644)
	stamp, _ := os.ReadFile(copies + ".stamp")
	if f := strings.Fields(string(stamp)); len(f) == 2 {
		os.WriteFile(copies+".stamp", []byte(f[1]+"\n"), 0o644)
	} else {
		t.Fatalf("the stamp is not a rule and a sum: %q", stamp)
	}
	kept("another build", []string{"vxlan", "tcpdump-mini"})
	if _, err := os.Stat(copies + "vxlan-1.0-r2.apk"); err == nil {
		t.Errorf("the copy of another build than the one installed was kept")
	}
	if !strings.Contains(calls(root), "fetch -X https://manager/feed/packages.adb -o") {
		t.Errorf("not fetched from the feed with the build installed: %s", calls(root))
	}
	if _, err := os.Stat(copies + "usteer-1.0-r3.apk"); err != nil {
		t.Errorf("no copy of usteer, whose installed build no feed has: %v", err)
	}
	fetched = strings.Count(calls(root), "fetch ")
	kept("another build, again", []string{"vxlan", "tcpdump-mini"})
	if n := strings.Count(calls(root), "fetch "); n != fetched {
		t.Errorf("fetched again with nothing installed changed: %s", calls(root))
	}
}

// nextRig is an apk port AP on build -95 with kmod-vxlan and kmod-veth beyond
// its image, whose manager lists the builds it has feeds for (0280).
func nextRig(t *testing.T) (root string, run func(args ...string) (string, error)) {
	t.Helper()
	image := append([]string{"wpad-basic-mbedtls", "kernel"}, required...)
	extra := []string{"vxlan", "kmod-vxlan", "kmod-veth", "usteer"}
	root, run = packagesRig(t, "apk", append(image, extra...), extra)
	var db strings.Builder
	for _, p := range image {
		db.WriteString("P:" + p + "\nV:1\n\n")
	}
	os.WriteFile(root+"/rom/lib/apk/db/installed", []byte(db.String()), 0o644)
	os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.18.55~ff43-r1\n\n"), 0o644)
	os.WriteFile(root+"/etc/os-release", []byte("OPENWRT_RELEASE=\"OpenWrt SNAPSHOT r0+589-bcb16b63472 (arista v2026.10.17.1-95-g9c8b99c)\"\n"), 0o644)
	os.WriteFile(root+"/etc/openwrt_release", []byte("DISTRIB_ID='OpenWrt'\nDISTRIB_TARGET='qualcommax/ipq807x'\n"), 0o644)
	touch(t, root+"/net")
	return root, run
}

// keep keeps the AP's kernel modules as its next build has them: the newest
// build of its target newer than its own, with another kernel, fetched from
// that build's feed by name and verified against the image's keys (0280).
// It fetches them again only where what it would keep changed, keeps none
// where one would not verify, and takes them away where the manager has no
// newer build.
func TestPackagesKeepNext(t *testing.T) {
	root, run := nextRig(t)
	next := root + "/etc/aeolus/pkgs-next/"
	os.WriteFile(root+"/builds", []byte(strings.Join([]string{
		"v2026.10.17.1-103-g4e32d7d 6.18.60~7908-r1 ipq807x",
		"v2026.10.17.1-103-g4e32d7d 6.18.60~1de4-r1 ipq60xx",
		"v2026.10.17.1-99-gaaaaaaa 6.18.58~aaaa-r1 ipq807x",
		"v2026.10.17.1-95-g9c8b99c 6.18.55~ff43-r1 ipq807x",
		"v2026.10.17.1-36-g06e07b2 6.18.55~ff43-r1 -",
		"v2026.10.17.1-110-gbbbbbbb 6.18.55~ff43-r1 ipq807x",
	}, "\n")+"\n"), 0o644)
	os.WriteFile(root+"/feed-has", []byte("kmod-vxlan-6.18.60-r1\nkmod-veth-6.18.60-r1\n"), 0o644)
	os.WriteFile(root+"/feed-files", []byte("kmod-vxlan-6.18.60-r1.apk\nkmod-veth-6.18.60-r1.apk\n"), 0o644)
	if out, err := run("keep"); err != nil {
		t.Fatalf("keep: %v\n%s", err, out)
	}
	sums, _ := os.ReadFile(next + ".sums")
	for _, f := range []string{"kmod-vxlan-6.18.60-r1.apk", "kmod-veth-6.18.60-r1.apk"} {
		if _, err := os.Stat(next + f); err != nil || !strings.Contains(string(sums), f) {
			t.Errorf("no copy of %s as -103 has it, or not summed: %v\n%s", f, err, sums)
		}
	}
	if k, _ := os.ReadFile(next + ".kernel"); strings.TrimSpace(string(k)) != "6.18.60~7908-r1" {
		t.Errorf("kept for kernel %q, want -103's ipq807x one", k)
	}
	// Kept as they were: not fetched again.
	os.Remove(root + "/feed-files")
	if out, err := run("keep"); err != nil || !fileHas(next+".sums", "kmod-vxlan-6.18.60-r1.apk") {
		t.Errorf("keep again with nothing changed lost the copies: %v\n%s", err, out)
	}
	// Another module beyond the image, which one of the feed's would not verify: none kept for it.
	os.WriteFile(root+"/feed-files", []byte("kmod-vxlan-6.18.60-r1.apk\nkmod-veth-6.18.60-r1.apk\nkmod-iptunnel-6.18.60-r1.apk\n"), 0o644)
	os.WriteFile(root+"/feed-has", []byte("kmod-vxlan-6.18.60-r1\nkmod-veth-6.18.60-r1\nkmod-iptunnel-6.18.60-r1\n"), 0o644)
	touch(t, root+"/unsigned-kmod-iptunnel-6.18.60-r1.apk")
	for _, l := range []string{"/installed", "/available"} {
		f, _ := os.OpenFile(root+l, os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString("kmod-iptunnel\n")
		f.Close()
	}
	if out, _ := run("keep"); !strings.Contains(out, "could not all be had") {
		t.Errorf("a module that would not verify was not said: %s", out)
	}
	if fileHas(next+".sums", "kmod-iptunnel") {
		t.Errorf("a module that would not verify was kept")
	}
	// No newer build: what was kept goes.
	os.WriteFile(root+"/builds", []byte("v2026.10.17.1-95-g9c8b99c 6.18.55~ff43-r1 ipq807x\n"), 0o644)
	if out, err := run("keep"); err != nil {
		t.Fatalf("keep: %v\n%s", err, out)
	}
	if _, err := os.Stat(next); err == nil {
		t.Errorf("kept for a next build the manager no longer has")
	}
}

// After the sysupgrade to the build they were kept for, early puts back the
// copies with those kernel modules in place of the copies' own, which fit
// only the kernel before (0280); before it, they are not used.
func TestPackagesEarlyNext(t *testing.T) {
	root, run := nextRig(t)
	copies, next := root+"/etc/aeolus/pkgs/", root+"/etc/aeolus/pkgs-next/"
	os.MkdirAll(copies, 0o755)
	os.MkdirAll(next, 0o755)
	for _, f := range []string{"vxlan-7.apk", "usteer-1.0-r1.apk", "kmod-vxlan-6.18.55-r1.apk", "kmod-veth-6.18.55-r1.apk"} {
		os.WriteFile(copies+f, []byte(f), 0o644)
	}
	for _, f := range []string{"kmod-vxlan-6.18.60-r1.apk", "kmod-veth-6.18.60-r1.apk"} {
		os.WriteFile(next+f, []byte(f), 0o644)
	}
	sum := func(dir string) {
		cmd := exec.Command("sh", "-c", "cd '"+filepath.ToSlash(dir)+"' && sha256sum *.apk >.sums")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sums: %v %s", err, out)
		}
	}
	sum(copies)
	sum(next)
	os.WriteFile(next+".kernel", []byte("6.18.60~7908-r1\n"), 0o644)
	touch(t, root+"/unfit-file-kmod-vxlan-6.18.55-r1.apk")
	touch(t, root+"/unfit-file-kmod-veth-6.18.55-r1.apk")
	// Still on the kernel before: the next build's are not used, and the
	// copies' own go in. As after a sysupgrade, only the image's are in.
	image := append([]string{"wpad-basic-mbedtls", "kernel"}, required...)
	os.WriteFile(root+"/installed", []byte(strings.Join(image, "\n")+"\n"), 0o644)
	os.Remove(root + "/unfit-file-kmod-vxlan-6.18.55-r1.apk")
	os.Remove(root + "/unfit-file-kmod-veth-6.18.55-r1.apk")
	if out, err := run("early"); err != nil {
		t.Fatalf("early on the kernel before: %v\n%s", err, out)
	}
	if c := calls(root); strings.Contains(c, "6.18.60") || !strings.Contains(c, "kmod-vxlan-6.18.55-r1.apk") {
		t.Errorf("on the kernel before, early used the next build's modules: %s", c)
	}
	// After the sysupgrade: -103's kernel, whose modules the copies' do not fit.
	os.WriteFile(root+"/calls", nil, 0o644)
	os.WriteFile(root+"/installed", []byte(strings.Join(append([]string{"wpad-basic-mbedtls", "kernel"}, required...), "\n")+"\n"), 0o644)
	os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.18.60~7908-r1\n\n"), 0o644)
	touch(t, root+"/unfit-file-kmod-vxlan-6.18.55-r1.apk")
	touch(t, root+"/unfit-file-kmod-veth-6.18.55-r1.apk")
	out, err := run("early")
	if err != nil {
		t.Fatalf("early after the sysupgrade: %v\n%s", err, out)
	}
	c := calls(root)
	for _, f := range []string{"vxlan-7.apk", "usteer-1.0-r1.apk", "kmod-vxlan-6.18.60-r1.apk", "kmod-veth-6.18.60-r1.apk"} {
		if !strings.Contains(c, f) {
			t.Errorf("early did not put back %s: %s", f, c)
		}
	}
	if strings.Contains(c, "6.18.55") {
		t.Errorf("early put back the old kernel's modules too: %s", c)
	}
	// One whose sums do not hold is not used.
	os.WriteFile(root+"/calls", nil, 0o644)
	os.WriteFile(root+"/installed", []byte(strings.Join(append([]string{"wpad-basic-mbedtls", "kernel"}, required...), "\n")+"\n"), 0o644)
	os.WriteFile(next+"kmod-veth-6.18.60-r1.apk", []byte("changed"), 0o644)
	run("early")
	if strings.Contains(calls(root), "6.18.60") {
		t.Errorf("early used next-build modules whose sums do not hold: %s", calls(root))
	}
}

// keep keeps, as the next build has them, its other packages beyond the
// image too, where its module feed or its own package feeds have them (0343):
// -135 built uclient, usteer and ustp anew against libubox20261103, under
// -132's very versions, which apk takes as installed and does not swap. Its
// kernel modules come from its module feed alone; one of the rest it does
// not have goes back from its copy.
func TestPackagesKeepNextBuiltAnew(t *testing.T) {
	root, run := nextRig(t)
	next := root + "/etc/aeolus/pkgs-next/"
	// OpenWrt's package arch, as apk keeps it, names the build's package feeds.
	os.WriteFile(root+"/etc/apk/arch", []byte("aarch64_cortex-a53\n"), 0o644)
	os.WriteFile(root+"/builds", []byte("v2026.10.17.1-103-g4e32d7d 6.18.60~7908-r1 ipq807x\n"), 0o644)
	os.WriteFile(root+"/feed-has", []byte("kmod-vxlan-6.18.60-r1\nkmod-veth-6.18.60-r1\n"), 0o644)
	os.WriteFile(root+"/arch-has", []byte("usteer-1.0-r1\nkmod-veth-6.18.99-r1\n"), 0o644)
	os.WriteFile(root+"/feed-files", []byte("kmod-vxlan-6.18.60-r1.apk\nkmod-veth-6.18.60-r1.apk\nusteer-1.0-r1.apk\nkmod-veth-6.18.99-r1.apk\n"), 0o644)
	out, err := run("keep")
	if err != nil {
		t.Fatalf("keep: %v\n%s", err, out)
	}
	for _, f := range []string{"kmod-vxlan-6.18.60-r1.apk", "kmod-veth-6.18.60-r1.apk", "usteer-1.0-r1.apk"} {
		if !fileHas(next+".sums", f) {
			t.Errorf("no copy of %s as -103 has it: %s", f, out)
		}
	}
	if fileHas(next+".sums", "6.18.99") {
		t.Errorf("a kernel module taken from a package feed: %s", out)
	}
	if !strings.Contains(out, "kept 3 of the 4 packages beyond the image as v2026.10.17.1-103-g4e32d7d has them, 2 of them kernel modules") {
		t.Errorf("not said what was kept:\n%s", out)
	}
}

// After the sysupgrade, early puts back the next build's own build of a
// package in place of the copy of the same name (0343).
func TestPackagesEarlyNextBuiltAnew(t *testing.T) {
	root, run := nextRig(t)
	copies, next := root+"/etc/aeolus/pkgs/", root+"/etc/aeolus/pkgs-next/"
	os.MkdirAll(copies, 0o755)
	os.MkdirAll(next, 0o755)
	for _, f := range []string{"vxlan-7.apk", "usteer-1.0-r1.apk", "kmod-vxlan-6.18.55-r1.apk"} {
		os.WriteFile(copies+f, []byte(f), 0o644)
	}
	for _, f := range []string{"kmod-vxlan-6.18.60-r1.apk", "usteer-1.0-r2.apk"} {
		os.WriteFile(next+f, []byte(f), 0o644)
	}
	for _, dir := range []string{copies, next} {
		cmd := exec.Command("sh", "-c", "cd '"+filepath.ToSlash(dir)+"' && sha256sum *.apk >.sums")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sums: %v %s", err, out)
		}
	}
	os.WriteFile(next+".kernel", []byte("6.18.60~7908-r1\n"), 0o644)
	os.WriteFile(root+"/installed", []byte(strings.Join(append([]string{"wpad-basic-mbedtls", "kernel"}, required...), "\n")+"\n"), 0o644)
	os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.18.60~7908-r1\n\n"), 0o644)
	if out, err := run("early"); err != nil {
		t.Fatalf("early: %v\n%s", err, out)
	}
	c := calls(root)
	if !strings.Contains(c, "usteer-1.0-r2.apk") || strings.Contains(c, "usteer-1.0-r1.apk") {
		t.Errorf("early did not put back the next build's usteer in place of the copy's: %s", c)
	}
	if !strings.Contains(c, "vxlan-7.apk") || !strings.Contains(c, "kmod-vxlan-6.18.60-r1.apk") {
		t.Errorf("early left out the rest: %s", c)
	}
}

// A module that does not load, where the feeds have it at the version
// installed, built anew against the image's libraries: restore puts in the
// feed's build from its file, which apk swaps, as it does not by name (0343).
func TestPackagesSwapBuild(t *testing.T) {
	inst := append([]string{"usteer", "wpad-mbedtls"}, required...)
	root, run := packagesRig(t, "apk", inst, append([]string{"usteer"}, required...))
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
	touch(t, root+"/usr/lib/aeolus/packages")
	touch(t, root+"/net")
	touch(t, root+"/unfit-uclient")
	touch(t, root+"/swaps-uclient")
	out, err := run("restore")
	if err != nil {
		t.Fatalf("restore: %v\n%s", err, out)
	}
	if c := calls(root); !strings.Contains(c, "add-files ucode-mod-uclient-1.0-r1.apk") {
		t.Errorf("the feed's build was not put in from its file: %s", c)
	}
	if _, err := os.Stat(root + "/unfit-uclient"); err == nil {
		t.Errorf("uclient still does not load:\n%s", out)
	}
	if !strings.Contains(out, "installed again: ucode-mod-uclient") {
		t.Errorf("not said:\n%s", out)
	}
}

func fileHas(p, s string) bool {
	b, err := os.ReadFile(p)
	return err == nil && strings.Contains(string(b), s)
}

// early puts back what apk can select, and leaves to restore those it names
// as needing what is neither in the image nor among the copies (0315), so
// vxlan and the kernel modules still go in before the network.
func TestPackagesEarlyLeavesUnselectable(t *testing.T) {
	image := append([]string{"wpad-basic-mbedtls"}, required...)
	root, run := packagesRig(t, "apk", image, nil)
	var db strings.Builder
	for _, p := range image {
		db.WriteString("P:" + p + "\nV:1\n\n")
	}
	os.WriteFile(root+"/rom/lib/apk/db/installed", []byte(db.String()), 0o644)
	copies := root + "/etc/aeolus/pkgs/"
	os.MkdirAll(copies, 0o755)
	for _, f := range []string{"vxlan-7-r8.apk", "kmod-vxlan-6.18.60-r1.apk", "usteer-2026.05.19~7532e125-r1.apk", "ustp-2026.10.28~1be05eb1-r1.apk"} {
		os.WriteFile(copies+f, []byte(f), 0o644)
	}
	cmd := exec.Command("sh", "-c", "cd '"+filepath.ToSlash(copies)+"' && sha256sum *.apk >.sums")
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sums: %v %s", err, o)
	}
	os.WriteFile(root+"/deps-usteer", []byte("libubox20261103\nlibblobmsg-json20261103\n"), 0o644)
	os.WriteFile(root+"/deps-ustp", []byte("libubox20261103\n"), 0o644)
	out, err := run("early")
	if err != nil {
		t.Fatalf("early: %v\n%s", err, out)
	}
	c := calls(root)
	if !strings.Contains(c, "vxlan-7-r8.apk") || !strings.Contains(c, "kmod-vxlan-6.18.60-r1.apk") {
		t.Errorf("vxlan and its module did not go in before the network: %s", c)
	}
	for _, l := range strings.Split(c, "\n") {
		if strings.HasPrefix(l, "add-files") && (strings.Contains(l, "usteer") || strings.Contains(l, "ustp")) {
			t.Errorf("one apk can select nothing for went in: %s", l)
		}
	}
	if !strings.Contains(out, "these could not go in before the network, apk finding nothing for what they need: usteer ustp") {
		t.Errorf("not said which were left: %s", out)
	}
}

// A port image takes what its build has from the build's own package feeds,
// beside its module feed, and not from OpenWrt's snapshot (0315): the
// snapshot built uclient, usteer and ustp again under the same versions
// against another libubox than the image's, and the agent hung. The feeds
// are listed where the manager has them; restore installs from them alone
// what they have, pinned; keep keeps their builds, not the snapshot's.
func TestPackagesBuildFeeds(t *testing.T) {
	image := append([]string{"wpad-mbedtls"}, required...)
	root, run := packagesRig(t, "apk", image, []string{"usteer"})
	os.WriteFile(root+"/lib/apk/db/installed", []byte("P:kernel\nV:6.18.60~7908-r1\n\n"), 0o644)
	os.WriteFile(root+"/etc/os-release", []byte("OPENWRT_RELEASE=\"OpenWrt SNAPSHOT r0+589-bcb16b63472 (arista v2026.10.17.1-128-g2554100)\"\n"), 0o644)
	touch(t, root+"/net")
	touch(t, root+"/feed")
	touch(t, root+"/build-feed")
	// OpenWrt's package arch, as apk keeps it: apk --print-arch says only
	// aarch64.
	os.WriteFile(root+"/etc/apk/arch", []byte("aarch64_cortex-a53\n"), 0o644)
	kmods := root + "/etc/apk/repositories.d/aeolus-kmods.list"
	build := "/feeds/kmods/6.18.60~7908-r1/v2026.10.17.1-128-g2554100/"
	run("feeds")
	if l, _ := os.ReadFile(kmods); strings.Count(string(l), "\n") != 1 || !strings.Contains(string(l), build+"packages.adb") {
		t.Fatalf("no arch feeds served: only the module feed: %q", l)
	}
	touch(t, root+"/build-arch-feed")
	if out, err := run("feeds"); err != nil || !strings.Contains(out, "come from the manager's feeds of the build") {
		t.Fatalf("feeds: %v %s", err, out)
	}
	l, _ := os.ReadFile(kmods)
	for _, f := range []string{"packages.adb", "aarch64_cortex-a53/base/packages.adb", "aarch64_cortex-a53/packages/packages.adb"} {
		if !strings.Contains(string(l), build+f) {
			t.Errorf("no %s in %q", f, l)
		}
	}
	// restore: usteer from the build's feeds, at their version, and not by
	// name from all of them, where the snapshot has the same version.
	os.WriteFile(root+"/feed-has", []byte("usteer-2026.05.19~7532e125-r1\n"), 0o644)
	os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\n"), 0o644)
	if out, err := run("restore"); err != nil {
		t.Fatalf("restore: %v %s", err, out)
	}
	c := calls(root)
	if !strings.Contains(c, "feed usteer=2026.05.19~7532e125-r1") {
		t.Errorf("usteer not from the build's feeds: %s", c)
	}
	if strings.Contains(c, "add usteer") {
		t.Errorf("usteer asked for by name from all the feeds too: %s", c)
	}
	// keep: the build's copy, though the feeds' highest is another build,
	// and the one installed another still.
	os.WriteFile(root+"/feed-has", []byte("usteer-2026.05.19~7532e125-r2\n"), 0o644)
	os.WriteFile(root+"/highest-usteer", []byte("2026.05.19~7532e125-r1\n"), 0o644)
	os.WriteFile(root+"/installed-version-usteer", []byte("2026.05.19~7532e125-r1\n"), 0o644)
	var db strings.Builder
	for _, p := range image {
		db.WriteString("P:" + p + "\nV:1\n\n")
	}
	os.WriteFile(root+"/rom/lib/apk/db/installed", []byte(db.String()), 0o644)
	if out, err := run("keep"); err != nil {
		t.Fatalf("keep: %v %s", err, out)
	}
	copies := root + "/etc/aeolus/pkgs/"
	if _, err := os.Stat(copies + "usteer-2026.05.19~7532e125-r2.apk"); err != nil {
		t.Errorf("no copy of the build's usteer: %v", err)
	}
	if _, err := os.Stat(copies + "usteer-2026.05.19~7532e125-r1.apk"); err == nil {
		t.Errorf("the feeds' other build was kept too")
	}
}

// A package goes in at a build that brings no other ABI of a library the AP
// has (0318): the snapshot's usteer, built against libubox20261103 beside
// the image's libubox20260721, is left out and why said, where no build of
// it fits; another of its builds that fits goes in in its place.
func TestPackagesABIGuard(t *testing.T) {
	image := append([]string{"wpad-mbedtls", "libubox20260721"}, required...)
	rig := func() (string, func(args ...string) (string, error)) {
		root, run := packagesRig(t, "apk", image, []string{"usteer", "tcpdump-mini"})
		os.WriteFile(root+"/etc/aeolus/packages", []byte("usteer\ntcpdump-mini\n"), 0o644)
		touch(t, root+"/net")
		os.WriteFile(root+"/brings-usteer", []byte("libubox20261103\nlibblobmsg-json20261103\n"), 0o644)
		return root, run
	}
	root, run := rig()
	out, _ := run("restore")
	c := calls(root)
	if strings.Contains(c, "usteer") && strings.Contains(c, "add") && fileHas(root+"/installed", "usteer") {
		t.Errorf("usteer of another libubox went in: %s", c)
	}
	// Said to the log: restore's own output of an install is not kept.
	if log, _ := os.ReadFile(root + "/log"); !strings.Contains(string(log), "usteer is left out: the feeds' builds of it would put in libubox20261103 beside the AP's own build of that library") {
		t.Errorf("not said why usteer was left out: %s%s", out, log)
	}
	if !fileHas(root+"/installed", "tcpdump-mini") {
		t.Errorf("tcpdump-mini, which brings nothing, did not go in: %s", c)
	}
	// A build of it that fits, by its version, goes in in its place.
	root, run = rig()
	os.WriteFile(root+"/brings-usteer=1.0-r1", nil, 0o644)
	out, _ = run("restore")
	if !fileHas(root+"/installed", "usteer") {
		t.Errorf("the build of usteer that fits did not go in: %s%s", out, calls(root))
	}
	if log, _ := os.ReadFile(root + "/log"); strings.Contains(string(log), "usteer is left out") {
		t.Errorf("usteer said left out, though a build of it fits: %s", log)
	}
}
