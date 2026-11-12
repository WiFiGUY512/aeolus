#!/bin/sh
# Stands up an Aeolus manager on Debian or Ubuntu, or brings one to a newer
# release (0170). As root, from a release file unpacked (0316):
#
#	tar -xzf aeolus-<tag>-linux-<arch>.tar.gz
#	sh aeolus-<tag>-linux-<arch>/deploy/install.sh [options]
#
# or with the file not unpacked, or its URL, from anywhere:
#
#	sh install.sh --from aeolus-<tag>-linux-<arch>.tar.gz [options]
#
# or from a checkout of this repository, which it builds:
#
#	sh deploy/install.sh [options]
#
# Once installed it is aeolus-install, and run again it upgrades in place.
#
#	--from <file|URL|dir> a release file (0316): the tarball, its URL, or the
#	                      tarball unpacked; else the release this script is in,
#	                      where it is in one. Nothing is built: no git, no Go
#	--sha256 <hex>        the SHA-256 that file must have; else it is checked
#	                      against the SHA256SUMS beside it, where there is one
#	--version <ref>       the release, a tag or a branch; else the newest stable
#	                      release (0282), or the newest v* tag where none is
#	--repo <url>          where the code comes from; else where it came from
#	                      before, or the checkout this script is in
#	--ssh-key <file>      a read-only deploy key, for a repository reached by SSH
#	--known-hosts <file>  that forge's host key; else it is trusted on first use
#	--hosts <a,b,...>     the names and addresses the certificate holds, on the
#	                      first install; else this host's name and address
#	--org <id>            the Org, made on the first install; asked for if not given
#	--org-name <name>     its display name; else its ID
#	--admin <account>     the first admin, who runs this; asked for if not given
#	--token               print the first admin an API token, for scripts,
#	                      in place of asking for their password
#	--password-stdin      read the first admin's password from the first line
#	                      of standard input, as deploy/proxmox.sh hands it on
#	--mcp-account <id>    an account for an MCP client too, whose token is
#	                      written to /etc/aeolus/mcp.token and never shown
#	--tftp                serve /srv/tftp by TFTP too, for moving APs to OpenWrt
#	--no-firewall         leave ufw and firewalld as they are
#
# Each part is done only where it is not done already: git and curl from
# apt; the Go toolchain go.mod asks for, from go.dev, checked against its
# published SHA-256, in /opt/aeolus/go; the release built in /opt/aeolus/bin,
# or from a release file, its manager put there and what it was kept in
# /etc/aeolus/release, with no git and no Go;
# the service's user, /etc/aeolus (root:aeolus 0770) and /var/lib/aeolus
# (aeolus 0700); the unit; on the first install, the Org and its first admin,
# its super admin, whose password is asked for twice on the terminal and
# never shown (0176): with no terminal, or --token, a token is printed
# instead, once, to the terminal only; the service started,
# which makes its TLS certificate; its ports let through ufw or firewalld
# where one of them is on. It ends with the line that joins an OpenWrt AP to
# this manager, with the certificate's fingerprint.
#
# Run again, it keeps /var/lib/aeolus, the secret key, the certificate APs
# pin and serve.env. The running build is replaced only by one that built
# and runs, and the service restarts only when its build or unit changed.
set -eu

SRC=/opt/aeolus/src
BIN=/opt/aeolus/bin
GODIR=/opt/aeolus/go
ETC=/etc/aeolus
STATE=/var/lib/aeolus
# Where the code comes from when nothing else says. Aeolus has no public home
# to fetch from (0316): a release file, a checkout or --repo says, unless
# AEOLUS_REPO names one, such as a fork's.
DEFAULT_REPO=${AEOLUS_REPO:-}

# Run from a pipe, sh reads the script as it goes, so all of it is read
# into main before any of it runs: nothing it starts can eat the rest.
main() {
	die() {
		echo "aeolus-install: $*" >&2
		exit 1
	}
	say() {
		echo "aeolus-install: $*"
	}
	# sums <name>: the SHA-256 the SHA256SUMS on standard input gives that file.
	sums() {
		awk -v n="$1" '($2 == n || $2 == "*" n) && length($1) == 64 && $1 ~ /^[0-9a-f]+$/ { print $1; exit }'
	}

	ref= repo= key= known= hosts= org= orgname= admin= mcp= tftp= firewall=1 token= pwstdin= password= signin=
	from= sha= rel= relfrom= relsha=
	while [ $# -gt 0 ]; do
		case $1 in
		--from | --sha256 | --version | --repo | --ssh-key | --known-hosts | --hosts | --org | --org-name | --admin | --mcp-account)
			[ $# -ge 2 ] || die "$1 needs a value"
			case $1 in
			--from) from=$2 ;;
			--sha256) sha=$2 ;;
			--version) ref=$2 ;;
			--repo) repo=$2 ;;
			--ssh-key) key=$2 ;;
			--known-hosts) known=$2 ;;
			--hosts) hosts=$2 ;;
			--org) org=$2 ;;
			--org-name) orgname=$2 ;;
			--admin) admin=$2 ;;
			--mcp-account) mcp=$2 ;;
			esac
			shift 2
			;;
		--tftp)
			tftp=1
			shift
			;;
		--token)
			token=1
			shift
			;;
		--password-stdin)
			pwstdin=1
			shift
			;;
		--no-firewall)
			firewall=
			shift
			;;
		*) die "no option $1: the options are at the top of deploy/install.sh" ;;
		esac
	done

	[ "$(id -u)" = 0 ] || die "run it as root"
	command -v apt-get >/dev/null 2>&1 || die "this installer knows Debian and Ubuntu, with apt"
	[ -d /run/systemd/system ] || die "Aeolus runs as a systemd service, and this host is not running systemd"

	# What it installs: a release file (0316), as given, or the one this
	# script is in, unpacked, unless told to build from git; else a build
	# from git, of the checkout this script is in or the one --repo names.
	here=
	case $0 in
	*install.sh)
		d=$(dirname "$0")/..
		if [ -f "$d/RELEASE" ] && [ -f "$d/aeolus" ]; then
			[ -n "$from$repo$ref$key" ] || from=$(cd "$d" && pwd)
		elif [ -f "$d/go.mod" ] && [ -f "$d/cmd/aeolus/main.go" ]; then
			here=$(cd "$d" && pwd)
		fi
		;;
	esac
	[ -z "$from" ] || [ -z "$repo$ref$key$known" ] ||
		die "--repo, --version, --ssh-key and --known-hosts are for building from git; a release file is its own release"
	[ -n "$from" ] || [ -z "$sha" ] || die "--sha256 is for a release file, given with --from"
	sha=$(printf '%s' "$sha" | tr -d ' :' | tr A-F a-f)
	[ -z "$sha" ] || echo "$sha" | grep -qxE '[0-9a-f]{64}' || die "--sha256 takes a SHA-256: 64 hexadecimal digits"
	case $(uname -m) in
	x86_64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*)
		arch=$(uname -m)
		[ -n "$from" ] || die "Go is fetched for amd64 and arm64 only, and this is $arch"
		;;
	esac
	exec 9>/run/aeolus-install.lock
	flock -n 9 || die "another aeolus-install is running"
	umask 022
	# Plain C: a locale the caller brings (LANG over SSH or pct exec) may
	# not be installed here, and apt's Perl complains of it at length.
	unset LANGUAGE
	export LC_ALL=C
	tmp= rtmp=
	trap 'rm -rf ${tmp:+"$tmp"} ${rtmp:+"$rtmp"} "$BIN/aeolus.new"' EXIT

	# A key or host key named by a relative path is kept by its full one,
	# for the runs after this one.
	[ -z "$key" ] || key=$(readlink -f "$key")
	[ -z "$known" ] || known=$(readlink -f "$known")
	[ -z "$key" ] || [ -r "$key" ] || die "cannot read the key $key"
	[ -z "$known" ] || [ -r "$known" ] || die "cannot read the host key $known"

	# The first install makes the Org, so what it needs is asked for now,
	# not after the build.
	first=
	[ -f "$STATE/aeolus.db" ] || first=1
	if [ -n "$first" ]; then
		tty=
		if [ -t 0 ]; then
			tty=/dev/stdin
		elif (: </dev/tty) 2>/dev/null; then
			tty=/dev/tty
		fi
		if [ -z "$org" ] && [ -n "$tty" ]; then
			printf 'The Org, the top of both trees, as an ID (e.g. your company): ' >&2
			read -r org <"$tty"
		fi
		if [ -z "$admin" ] && [ -n "$tty" ]; then
			printf 'Your account, the first admin: ' >&2
			read -r admin <"$tty"
		fi
		[ -n "$org" ] && [ -n "$admin" ] || die "a first install makes the Org: give --org <id> and --admin <account>"
		for id in "$org" "$admin" ${mcp:+"$mcp"}; do
			echo "$id" | grep -qxE '[a-z0-9][a-z0-9-]{0,31}' ||
				die "\"$id\" is not an ID: 1 to 32 lowercase letters, digits and hyphens, starting with a letter or digit"
		done
		# The first admin's password (0176): handed on, or asked for twice
		# and not echoed. With no terminal, or --token, a token is printed
		# in its place.
		if [ -n "$pwstdin" ]; then
			IFS= read -r password || true
		elif [ -z "$token" ] && [ -n "$tty" ]; then
			hidden() {
				printf '%s' "$1" >&2
				trap 'stty echo <"$tty" 2>/dev/null; exit 130' INT TERM
				stty -echo <"$tty" 2>/dev/null || true
				IFS= read -r line <"$tty" || line=
				stty echo <"$tty" 2>/dev/null || true
				trap - INT TERM
				echo >&2
			}
			while :; do
				hidden "Your password, $admin, to sign in with (12 characters or more): "
				password=$line
				hidden 'The same again: '
				if [ "$password" != "$line" ]; then
					echo 'They differ. Once more.' >&2
				elif [ ${#password} -lt 12 ] || [ ${#password} -gt 128 ]; then
					echo 'A password is 12 to 128 characters; length is what counts. Once more.' >&2
				elif [ "$password" = "$admin" ]; then
					echo 'Not your account'"'"'s ID. Once more.' >&2
				else
					break
				fi
			done
			line=
		fi
		[ -n "$password" ] || token=1
	elif [ -n "$org$admin$orgname$mcp" ]; then
		say "this manager has its Org already: --org, --org-name, --admin and --mcp-account are for a first install"
	fi

	# Where the code comes from, for a build: as given; else the checkout's
	# own origin, set by the run before; else the checkout this script is
	# in. A manager put up from a release file is given a newer one, unless
	# told to build.
	if [ -z "$from" ] && [ -z "$repo" ]; then
		if [ -f "$ETC/release" ] && [ -z "$ref$here" ]; then
			die "this manager was put up from a release file ($(sed -n 's/^from=//p' "$ETC/release")): give it a newer one, aeolus-install --from <file or URL>"
		elif [ -d "$SRC/.git" ]; then
			repo=$(git -C "$SRC" remote get-url origin)
		elif [ -n "$here" ]; then
			repo=$here
		else
			repo=$DEFAULT_REPO
		fi
	fi
	[ -n "$from$repo" ] || die "say where Aeolus comes from: --from <release file or its URL> (0316), --repo <git URL>, or run it from a checkout"
	[ ! -d "$repo" ] || repo=$(cd "$repo" && pwd)

	# git only for a build: a release file needs none.
	packages= git=git
	[ -z "$from" ] || git=
	for p in ca-certificates curl $git ${tftp:+tftpd-hpa}; do
		dpkg-query -W -f '${Status}' "$p" 2>/dev/null | grep -q 'ok installed' || packages="$packages $p"
	done
	if [ -n "$packages" ]; then
		say "installing$packages"
		apt-get update -qq
		DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends $packages >/dev/null
	fi

	# The service's user and the places it keeps its settings and state,
	# made as on the first manager (0011); ones already there are left as
	# they are.
	getent passwd aeolus >/dev/null ||
		useradd --system --user-group --home-dir "$STATE" --no-create-home --shell /usr/sbin/nologin aeolus
	[ -d "$ETC" ] || install -d -o root -g aeolus -m 0770 "$ETC"
	[ -d "$STATE" ] || install -d -o aeolus -g aeolus -m 0700 "$STATE"
	install -d -m 0755 /opt/aeolus "$BIN"

	if [ -n "$from" ]; then
		# The release file (0316), by its path or its URL, or unpacked:
		# checked against the SHA-256 it should have, where that is known,
		# and its manager put beside the running one. Nothing is built.
		rtmp=$(mktemp -d /opt/aeolus/.release.XXXXXX)
		if [ -d "$from" ]; then
			rel=$(cd "$from" && pwd)
			relfrom=$rel
		else
			case $from in
			http://* | https://*)
				relfrom=$from
				name=${from%%[?#]*}
				name=${name##*/}
				[ -n "$name" ] || name=release.tar.gz
				file=$rtmp/$name
				# The SHA-256 it should have: as given, else as the
				# SHA256SUMS beside it says, where there is one.
				[ -n "$sha" ] || sha=$(curl -fsSL --retry 3 "${from%/*}/SHA256SUMS" 2>/dev/null | sums "$name") || sha=
				say "fetching $from"
				# A transfer cut goes on from where it stopped, and one
				# spoilt starts again, until the file is what it should
				# be, as Go's download does.
				# A file the server says is not there is not tried again.
				i=0
				while :; do
					if code=$(curl -fsSL -C - -o "$file" -w '%{http_code}' "$from"); then
						[ -n "$sha" ] || break
						[ "$(sha256sum <"$file" | cut -d' ' -f1)" = "$sha" ] && break
						rm -f "$file"
					else
						case $code in 401 | 403 | 404 | 410) die "$from: the server answers $code, so there is no such file to take: nothing changed" ;; esac
					fi
					i=$((i + 1))
					[ "$i" -lt 10 ] || die "$from did not come whole in 10 tries: nothing changed"
					sleep 1
				done
				;;
			*)
				[ -f "$from" ] || die "no release file $from"
				relfrom=$(readlink -f "$from")
				file=$relfrom
				name=${relfrom##*/}
				[ -n "$sha" ] || [ ! -f "${relfrom%/*}/SHA256SUMS" ] || sha=$(sums "$name" <"${relfrom%/*}/SHA256SUMS")
				;;
			esac
			relsha=$(sha256sum <"$file" | cut -d' ' -f1)
			if [ -n "$sha" ]; then
				[ "$relsha" = "$sha" ] || die "$name is not the file it should be: its SHA-256 is $relsha, not $sha. Nothing changed"
				say "$name has the SHA-256 it should: $relsha"
			else
				say "$name has the SHA-256 $relsha, with nothing to check it against (--sha256 says what it should be)"
			fi
			mkdir "$rtmp/x"
			tar -C "$rtmp/x" --no-same-owner -xzf "$file" || die "$name does not unpack, so it is not a release file: nothing changed"
			set -- "$rtmp/x"/*
			[ $# -eq 1 ] && [ -d "$1" ] || die "$name is not a release file, whose files are in one directory: nothing changed"
			rel=$1
		fi
		relkey() {
			sed -n "s/^$1=//p" "$rel/RELEASE" | head -n 1
		}
		[ -f "$rel/RELEASE" ] && [ "$(relkey name)" = aeolus ] || die "$relfrom is not an Aeolus release: it has no RELEASE that says so"
		[ -f "$rel/aeolus" ] && [ ! -L "$rel/aeolus" ] || die "$relfrom has no manager in it"
		[ -f "$rel/deploy/aeolus.service" ] && [ -f "$rel/deploy/aeolus.sh" ] || die "$relfrom has no deploy/ to install it with"
		[ "$(relkey os)" = linux ] && [ "$(relkey arch)" = "$arch" ] ||
			die "$relfrom is built for $(relkey os)/$(relkey arch), and this host is linux/$arch: take aeolus-$(relkey tag)-linux-$arch.tar.gz"
		version=$(relkey version | tr -cd 'A-Za-z0-9._+-')
		[ -n "$version" ] || die "$relfrom names no version"
		say "taking $version"
		install -m 0755 "$rel/aeolus" "$BIN/aeolus.new"
		deploy=$rel/deploy
	else
		# A repository reached by SSH, with a deploy key: the forge's host key
		# is the one given, or the one met first. The checkout keeps the
		# command, so a run without --ssh-key reaches it the same way.
		if [ -n "$key" ]; then
			check=accept-new
			if [ -n "$known" ]; then
				cmp -s "$known" "$ETC/known_hosts" || install -m 0644 "$known" "$ETC/known_hosts"
				check=yes
			fi
			export GIT_SSH_COMMAND="ssh -i $key -o IdentitiesOnly=yes -o UserKnownHostsFile=$ETC/known_hosts -o StrictHostKeyChecking=$check"
		fi
		if [ ! -d "$SRC/.git" ]; then
			say "fetching $repo"
			rm -rf "$SRC.new"
			tmp=$SRC.new
			git -c init.defaultBranch=main clone -q --no-checkout "$repo" "$SRC.new"
			mv "$SRC.new" "$SRC"
			tmp=
		fi
		git -C "$SRC" remote set-url origin "$repo"
		[ -z "$key" ] || git -C "$SRC" config core.sshCommand "$GIT_SSH_COMMAND"
		git -C "$SRC" fetch -q --tags --prune origin

		# The newest stable release (0282): one with a tag stable/vX.Y.Z on the
		# very commit vX.Y.Z is. Else the newest release.
		if [ -z "$ref" ]; then
			for s in $(git -C "$SRC" tag -l 'stable/v*' --sort=-v:refname); do
				t=${s#stable/}
				c=$(git -C "$SRC" rev-parse -q --verify "refs/tags/$t^{commit}" 2>/dev/null) || continue
				[ "$c" = "$(git -C "$SRC" rev-parse -q --verify "refs/tags/$s^{commit}" 2>/dev/null)" ] && { ref=$t; break; }
			done
		fi
		[ -n "$ref" ] || ref=$(git -C "$SRC" tag -l 'v*' --sort=-v:refname | head -n 1)
		[ -n "$ref" ] || die "$repo has no release tag: name a tag or branch with --version"
		rev=$ref
		git -C "$SRC" rev-parse -q --verify "refs/remotes/origin/$ref" >/dev/null && rev=origin/$ref
		git -C "$SRC" rev-parse -q --verify "$rev^{commit}" >/dev/null || die "$repo has no tag or branch $ref"
		git -C "$SRC" -c advice.detachedHead=false checkout -q --detach "$rev"
		version=$ref-$(git -C "$SRC" rev-parse --short HEAD)

		# The Go toolchain go.mod asks for, kept beside the code and used for
		# nothing else; a toolchain already there that is as new is kept. Its
		# caches and work are on disk: /tmp may be memory (Debian 13's is).
		export GOTOOLCHAIN=local GOPATH=/var/cache/aeolus/go GOCACHE=/var/cache/aeolus/go-build GOTMPDIR=/var/cache/aeolus/tmp GOFLAGS=-modcacherw CGO_ENABLED=0
		install -d -m 0700 "$GOTMPDIR"
		want=$(sed -n 's/^toolchain go\([0-9][0-9.]*\)$/\1/p' "$SRC/go.mod")
		[ -n "$want" ] || want=$(sed -n 's/^go \([0-9][0-9.]*\)$/\1/p' "$SRC/go.mod")
		[ -n "$want" ] || die "go.mod names no Go version"
		case $want in *.*.*) ;; *) want=$want.0 ;; esac
		have=$("$GODIR/bin/go" env GOVERSION 2>/dev/null || true)
		have=${have#go}
		if [ -z "$have" ] || [ "$(printf '%s\n%s\n' "$have" "$want" | sort -V | head -n 1)" != "$want" ]; then
			tgz=go$want.linux-$arch.tar.gz
			say "fetching Go $want"
			tmp=$(mktemp -d /opt/aeolus/.go.XXXXXX)
			# A transfer cut or spoilt on the way goes on from where it
			# stopped, until the file is what go.dev says it is. In the lab
			# (2026-10-18) half of all fast downloads broke off with a bad
			# record MAC: the internet router's hardware TCP segmentation
			# spoilt them (0175).
			sum=$(curl -fsSL --retry 5 --retry-all-errors "https://dl.google.com/go/$tgz.sha256")
			echo "$sum" | grep -qxE '[0-9a-f]{64}' || die "go.dev gave no SHA-256 for $tgz: nothing changed"
			i=0
			until echo "$sum  $tmp/$tgz" | sha256sum -c --status 2>/dev/null; do
				i=$((i + 1))
				[ "$i" -le 10 ] || die "$tgz did not come whole from go.dev in 10 tries: nothing changed"
				curl -fsSL -C - -o "$tmp/$tgz" "https://dl.google.com/go/$tgz" || sleep 1
			done
			tar -C "$tmp" -xzf "$tmp/$tgz"
			rm -rf "$GODIR.old"
			[ ! -d "$GODIR" ] || mv "$GODIR" "$GODIR.old"
			mv "$tmp/go" "$GODIR"
			rm -rf "$GODIR.old" "$tmp"
			tmp=
		fi

		# The modules, fetched first and again where a transfer breaks, so
		# that a build that fails is the code's fault.
		i=0
		until (cd "$SRC" && "$GODIR/bin/go" mod download); do
			i=$((i + 1))
			[ "$i" -lt 5 ] || die "Go's modules did not come in 5 tries: nothing changed"
			sleep 1
		done
		say "building $version"
		# One package at a time, and a tighter collector: the Anthropic SDK's
		# package (the advisor, 0205) takes some 1.7 GB to compile, more than a
		# 2 GB host has to spare with Go's defaults.
		(cd "$SRC" && GOGC=20 "$GODIR/bin/go" build -p 1 -trimpath -ldflags "-s -w -X main.version=$version" -o "$BIN/aeolus.new" ./cmd/aeolus)
		deploy=$SRC/deploy
	fi
	"$BIN/aeolus.new" version >/dev/null || die "the new manager does not run here: nothing changed"
	restart=
	if cmp -s "$BIN/aeolus.new" "$BIN/aeolus"; then
		rm -f "$BIN/aeolus.new"
	else
		mv -f "$BIN/aeolus.new" "$BIN/aeolus"
		restart=1
	fi
	cmp -s "$deploy/aeolus.service" /etc/systemd/system/aeolus.service || restart=1
	install -m 0644 "$deploy/aeolus.service" /etc/systemd/system/aeolus.service
	install -m 0755 "$deploy/aeolus.sh" /usr/local/bin/aeolus
	# A release from before this script has none to install.
	[ ! -f "$deploy/install.sh" ] || install -m 0755 "$deploy/install.sh" /usr/local/sbin/aeolus-install
	# The updater (0183): the manager, which runs without root, asks it to
	# look for a newer release or to install one. A release from before it
	# has none.
	if [ -f "$deploy/updater.sh" ]; then
		install -m 0755 "$deploy/updater.sh" /usr/local/sbin/aeolus-updater
		install -m 0644 "$deploy/aeolus-updater.service" "$deploy/aeolus-updater.path" /etc/systemd/system/
		[ -d "$STATE/update" ] || install -d -o aeolus -g aeolus -m 0700 "$STATE/update"
	fi
	# What this manager was put up from (0316): a release file's RELEASE,
	# with where the file was and its SHA-256, for the next run and the
	# updater. A build from git leaves none.
	if [ -n "$from" ]; then
		{
			grep -E '^[a-z][a-z0-9_]*=' "$rel/RELEASE" | grep -vE '^(from|sha256|installed)=' || true
			echo "from=$relfrom"
			echo "sha256=$relsha"
			echo "installed=$(date +%s)"
		} >"$ETC/release.new"
		chmod 0644 "$ETC/release.new"
		mv -f "$ETC/release.new" "$ETC/release"
	else
		rm -f "$ETC/release"
	fi

	# The names and addresses the certificate holds, made on the service's
	# first start: this host's name and the address its default route
	# leaves from, unless given.
	if [ ! -f "$ETC/serve.env" ]; then
		if [ -z "$hosts" ]; then
			name=$(hostname -f 2>/dev/null || hostname)
			[ "$name" != localhost ] || name=
			# Read from the default route itself: an address one might
			# ask the way to, such as 1.1.1.1, can be inside a network.
			route=$(ip -4 route show default 2>/dev/null | head -n 1)
			addr=$(echo "$route" | sed -n 's/.* src \([0-9.]*\).*/\1/p')
			dev=$(echo "$route" | sed -n 's/.* dev \([^ ]*\).*/\1/p')
			[ -n "$addr" ] || [ -z "$dev" ] ||
				addr=$(ip -4 -o addr show dev "$dev" scope global 2>/dev/null | awk '{ sub(/\/.*/, "", $4); print $4; exit }')
			[ -n "$addr" ] || addr=$(hostname -I 2>/dev/null | awk '{print $1}')
			hosts=$name${name:+${addr:+,}}$addr
		fi
		printf '# Settings for aeolus serve: see /etc/systemd/system/aeolus.service.\nAEOLUS_HOSTS=%s\n' "$hosts" >"$ETC/serve.env"
	elif [ -n "$hosts" ]; then
		say "--hosts left unused: $ETC/serve.env is kept, and with it the certificate APs pin"
	fi

	if [ -n "$first" ]; then
		say "making the Org $org, with $admin its first admin"
		set -- -org "$org" -admin "$admin"
		[ -z "$orgname" ] || set -- "$@" -org-name "$orgname"
		[ -z "$mcp" ] || set -- "$@" -mcp-account "$mcp"
		[ -z "$password" ] || set -- "$@" -password-stdin
		[ -z "$token" ] || set -- "$@" -token
		# What init made is taken away again if it failed: its token was
		# never shown. The password goes on its standard input, never on a
		# command line.
		if ! printf '%s\n' "$password" | runuser -u aeolus -- "$BIN/aeolus" init "$@"; then
			rm -f "$STATE/aeolus.db" "$STATE/aeolus.db-wal" "$STATE/aeolus.db-shm"
			die "the Org was not made, and nothing is running"
		fi
		[ -z "$password" ] || signin="Sign in as $admin with the password you chose."
		[ -z "$token" ] || signin="${signin:-Sign in with the token printed above.}${password:+ The token above is for scripts.}"
		password=
	fi

	# The ports the service listens on, let through only where a firewall
	# is on; nothing else is opened.
	if [ -n "$firewall" ]; then
		ports=$(
			. "$ETC/serve.env"
			relay=${AEOLUS_RELAY_LISTEN:-:67} knock=${AEOLUS_KNOCK_LISTEN:-:15002}
			printf 8443/tcp
			[ "$relay" = off ] || printf ' %s/udp' "${relay##*:}"
			[ "$knock" = off ] || printf ' %s/tcp' "${knock##*:}"
			[ "${AEOLUS_MDNS:-}" = off ] || printf ' 5353/udp'
		)
		[ -z "$tftp" ] || ports="$ports 69/udp"
		if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
			say "letting $ports through ufw"
			for p in $ports; do ufw allow "$p" >/dev/null; done
		elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
			say "letting $ports through firewalld"
			for p in $ports; do firewall-cmd -q --permanent --add-port="$p"; done
			firewall-cmd -q --reload
		fi
	fi

	systemctl daemon-reload
	[ ! -f /etc/systemd/system/aeolus-updater.path ] || systemctl enable -q --now aeolus-updater.path
	systemctl enable -q aeolus
	if [ -n "$restart" ] || ! systemctl is-active -q aeolus; then
		say "starting the service"
		systemctl restart aeolus
	fi
	# Up once it answers; it makes its certificate on its first start,
	# before it listens.
	i=0
	until curl -fsk -o /dev/null --max-time 3 https://127.0.0.1:8443/healthz; do
		i=$((i + 1))
		[ "$i" -lt 60 ] || die "the service does not answer on port 8443: journalctl -u aeolus says why"
		sleep 1
	done

	fp=$("$BIN/aeolus" fingerprint -cert "$ETC/tls.crt")
	# APs join by address: dnsmasq on an AP drops a name that points to a
	# private address (0083).
	set -- $(sed -n 's/^AEOLUS_HOSTS=//p' "$ETC/serve.env" | tr ',' ' ')
	at=
	for h in "$@"; do
		case $h in
		*[!0-9.]*) ;;
		?*)
			at=$h
			break
			;;
		esac
	done
	[ -n "$at" ] || at=${1:-$(hostname)}
	echo
	echo "Aeolus $("$BIN/aeolus" version) is running: https://$at:8443/"
	[ -z "$first" ] || echo "$signin"
	echo "Its certificate's SHA-256 (aeolus fingerprint says it again): $fp"
	echo
	echo "To join an OpenWrt AP, 24.10 or later, run as root on the AP (-u <port> names its uplink):"
	echo
	echo "  wget -qO- --no-check-certificate https://$at:8443/install | sh -s -- -f $fp"
	echo
	if [ -n "$from" ]; then
		echo "To upgrade: aeolus-install --from <a newer release file, or its URL>"
	else
		echo "To upgrade: aeolus-install [--version <tag>]"
	fi
}

main "$@"; exit
