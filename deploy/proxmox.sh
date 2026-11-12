#!/bin/sh
# Makes a Debian container on a Proxmox VE host and stands an Aeolus manager
# up in it with deploy/install.sh (0175), or upgrades the manager in a
# container it made. As root on the Proxmox host, from a release file
# unpacked (0316):
#
#	tar -xzf aeolus-<tag>-linux-amd64.tar.gz
#	sh aeolus-<tag>-linux-amd64/deploy/proxmox.sh [options] [-- installer options]
#
# or from a checkout of this repository, which the container builds:
#
#	sh deploy/proxmox.sh [options] [-- installer options]
#
#	--ctid <id>          the container; else the next free ID. One this script
#	                     made before is upgraded
#	--hostname <name>    aeolus
#	--cores <n>          2
#	--memory <MB>        2048
#	--swap <MB>          512
#	--disk <GB>          16
#	--storage <id>       its disk; else the roomiest that holds containers,
#	                     ZFS or LVM before a directory
#	--bridge <name>      vmbr0
#	--vlan <tag>         none
#	--ip <cidr>          a fixed address, with --gateway; else DHCP
#	--gateway <ip>
#	--root-key <file>    a public key root may sign in to the container with
#	--org <id>           the Org; asked for if not given
#	--admin <account>    its first admin, you; asked for if not given
#	--yes                make it without asking first
#
# On a first install, the first admin's password, to sign in with, is asked
# for twice on the host's terminal and never shown (0176), and handed to the
# installer in the container on its standard input. With --yes, with no
# terminal, or with the installer's --token, a token is printed instead.
#
# What follows "--" goes to the installer as it is: --from, --sha256,
# --version, --repo, --hosts, --org-name, --mcp-account, --tftp and the rest.
# A file it names is copied into the container, readable by root alone: a
# deploy key or host key (--ssh-key, --known-hosts), and a release file
# (--from, the tarball or the directory it unpacked to) with the SHA256SUMS
# beside it, where there is one. A release file named by its URL, the
# container fetches.
#
# Where the manager comes from: a release file (0316), the one given with
# --from, else the one this script is in, which the installer takes as it
# is; else the checkout this script is in, which goes into the container as
# a git bundle the installer builds from, unless --repo says another. Run
# again with --ctid from a newer release file or checkout, the container
# gets it and upgrades. A release file needs nothing from the internet in
# the container, unless its template has no curl. On its own, piped or a
# copy, the script fetches the installer from AEOLUS_RAW and the code from
# AEOLUS_REPO, such as a fork's: Aeolus has no public home to fetch from.
#
# The container is as Aeolus's own (CT 118) is: Debian 13, unprivileged,
# nesting and keyctl on, started at boot, on the host's time zone. No
# password is set: `pct enter <id>` opens a shell in it.
#
# AEOLUS_SYS_NET is for its tests: where the host's network devices are.
set -eu

BUNDLE=/opt/aeolus/aeolus.bundle
# Where the installer and the code come from for the script on its own: a
# fork's or a mirror's, as named; there is no public one (0316).
RAW=${AEOLUS_RAW:-}
PUBLISHED=${AEOLUS_REPO:-}
SYSNET=${AEOLUS_SYS_NET:-/sys/class/net}

# Run from a pipe, sh reads the script as it goes, so all of it is read
# into main before any of it runs: nothing it starts can eat the rest.
main() {
	die() {
		echo "aeolus-proxmox: $*" >&2
		exit 1
	}
	say() {
		echo "aeolus-proxmox: $*"
	}

	ctid= hostname=aeolus cores=2 memory=2048 swap=512 disk=16 storage= bridge=vmbr0 vlan= ip= gateway= rootkey= org= admin= yes= token= password=
	while [ $# -gt 0 ]; do
		case $1 in
		--ctid | --hostname | --cores | --memory | --swap | --disk | --storage | --bridge | --vlan | --ip | --gateway | --root-key | --org | --admin)
			[ $# -ge 2 ] || die "$1 needs a value"
			case $1 in
			--ctid) ctid=$2 ;;
			--hostname) hostname=$2 ;;
			--cores) cores=$2 ;;
			--memory) memory=$2 ;;
			--swap) swap=$2 ;;
			--disk) disk=$2 ;;
			--storage) storage=$2 ;;
			--bridge) bridge=$2 ;;
			--vlan) vlan=$2 ;;
			--ip) ip=$2 ;;
			--gateway) gateway=$2 ;;
			--root-key) rootkey=$2 ;;
			--org) org=$2 ;;
			--admin) admin=$2 ;;
			esac
			shift 2
			;;
		--yes)
			yes=1
			shift
			;;
		--)
			shift
			break
			;;
		*) die "no option $1: the options are at the top of deploy/proxmox.sh; the installer's go after --" ;;
		esac
	done

	[ "$(id -u)" = 0 ] || die "run it as root"
	command -v pct >/dev/null 2>&1 && command -v pveam >/dev/null 2>&1 || die "run it on a Proxmox VE host"
	for n in "$cores" "$memory" "$swap" "$disk" ${ctid:+"$ctid"} ${vlan:+"$vlan"}; do
		case $n in '' | *[!0-9]*) die "\"$n\" is not a number" ;; esac
	done
	[ -z "$ip" ] || [ -n "$gateway" ] || die "a fixed --ip needs its --gateway"
	[ -z "$rootkey" ] || [ -r "$rootkey" ] || die "cannot read the key $rootkey"

	# What this script is in, whose installer the container runs: a release
	# file unpacked (0316), or a checkout; neither when it was piped, or is a
	# copy on its own.
	here= release=
	case $0 in
	*proxmox.sh)
		d=$(dirname "$0")/..
		if [ -f "$d/RELEASE" ] && [ -f "$d/aeolus" ] && [ -f "$d/deploy/install.sh" ]; then
			release=$(cd "$d" && pwd)
		elif [ -f "$d/go.mod" ] && [ -f "$d/deploy/install.sh" ]; then
			here=$(cd "$d" && pwd)
		fi
		;;
	esac
	# A container runs as its host does, so a release file is for the
	# host's arch, as the installer names it.
	case $(uname -m) in
	x86_64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) arch=$(uname -m) ;;
	esac

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	# What goes into the container's /root/.aeolus, for the installer.
	mkdir "$tmp/push"

	# On its own: the installer comes from where AEOLUS_RAW says, fetched
	# before anything is made.
	installer=$release$here/deploy/install.sh
	if [ -z "$release$here" ]; then
		[ -n "$RAW" ] && [ -n "$PUBLISHED" ] ||
			die "run it from a release file unpacked (0316) or from a checkout: on its own it has nothing to install, unless AEOLUS_RAW and AEOLUS_REPO name where Aeolus is"
		command -v curl >/dev/null 2>&1 || die "curl fetches the installer: apt install curl"
		curl -fsSL --retry 3 -o "$tmp/install.sh" "$RAW/deploy/install.sh" ||
			die "cannot fetch the installer from $RAW/deploy/install.sh"
		grep -q '^# Stands up an Aeolus manager' "$tmp/install.sh" ||
			die "$RAW/deploy/install.sh is not Aeolus's installer"
		installer=$tmp/install.sh
	fi

	# The installer's options, with the files they name made the
	# container's own; an Org or admin given there is taken as given here.
	repo= from= ref= key= sums=
	n=$#
	while [ "$n" -gt 0 ]; do
		a=$1
		shift
		n=$((n - 1))
		case $a in
		--ssh-key | --known-hosts)
			[ "$n" -gt 0 ] || die "$a needs a value"
			f=$(readlink -f "$1")
			shift
			n=$((n - 1))
			[ -r "$f" ] || die "cannot read $f"
			cp "$f" "$tmp/push/${a#--}"
			[ "$a" != --ssh-key ] || key=1
			set -- "$@" "$a" "/root/.aeolus/${a#--}"
			;;
		--from)
			# Given to the installer below, as the container has it.
			[ "$n" -gt 0 ] || die "$a needs a value"
			from=$1
			shift
			n=$((n - 1))
			;;
		--repo | --version)
			[ "$n" -gt 0 ] || die "$a needs a value"
			if [ "$a" = --repo ]; then repo=$1; else ref=$1; fi
			set -- "$@" "$a"
			;;
		--org | --admin)
			[ "$n" -gt 0 ] || die "$a needs a value"
			if [ "$a" = --org ]; then org=$1; else admin=$1; fi
			shift
			n=$((n - 1))
			;;
		--token)
			token=1
			set -- "$@" "$a"
			;;
		--password-stdin) die "--password-stdin is how this script hands the installer the password it asks for" ;;
		*) set -- "$@" "$a" ;;
		esac
	done
	# A release file: as given, else the one this script is in, unless the
	# installer is told to build from git. One on this host is copied into
	# the container, from the tarball it was unpacked from where that sits
	# beside it, else packed again; one by its URL the container fetches.
	[ -n "$from$repo$ref$key" ] || from=$release
	if [ -n "$from" ]; then
		[ -z "$repo$ref$key" ] || die "--repo, --version and --ssh-key are for a build from git; a release file is its own release"
		case $from in
		http://* | https://*) set -- "$@" --from "$from" ;;
		*)
			[ -e "$from" ] || die "no release file $from"
			f=$(readlink -f "$from") sums=
			if [ -d "$f" ]; then
				[ -f "$f/RELEASE" ] || die "$from is not a release file unpacked: it has no RELEASE"
				if [ -f "$f.tar.gz" ]; then
					f=$f.tar.gz
				else
					name=${f##*/}.tar.gz
					tar -C "${f%/*}" -czf "$tmp/push/$name" "${f##*/}"
					f=
				fi
			fi
			if [ -n "$f" ]; then
				name=${f##*/}
				cp "$f" "$tmp/push/$name"
				[ ! -f "${f%/*}/SHA256SUMS" ] || { cp "${f%/*}/SHA256SUMS" "$tmp/push/SHA256SUMS"; sums=1; }
			fi
			# Built for this host, or nothing is made.
			rarch=$(tar -xzOf "$tmp/push/$name" --wildcards '*/RELEASE' 2>/dev/null | sed -n 's/^arch=//p' | head -n 1)
			[ -n "$rarch" ] || die "$from is not an Aeolus release file: it has no RELEASE"
			[ "$rarch" = "$arch" ] || die "$from is built for $rarch, and this host is $arch: take aeolus-<tag>-linux-$arch.tar.gz"
			set -- "$@" --from "/root/.aeolus/$name"
			;;
		esac
	elif [ -z "$repo" ] && [ -n "$here" ]; then
		command -v git >/dev/null 2>&1 || die "git makes the bundle from this checkout: apt install git, or give -- --repo <git URL>"
	elif [ -z "$repo" ] && [ -n "$release" ]; then
		die "--version and --ssh-key build from git, which a release file is not: give -- --repo <git URL> too, or leave them out to install this release"
	fi

	# A container this script made before is upgraded; any other is left
	# alone.
	new=1
	if [ -n "$ctid" ] && pct config "$ctid" >/dev/null 2>&1; then
		pct config "$ctid" | grep -qE '^tags:.*\baeolus\b' ||
			die "CT $ctid was not made by this script (it has no tag aeolus): it is left alone"
		new=
	fi
	[ -n "$ctid" ] || ctid=$(pvesh get /cluster/nextid)
	[ -z "$new" ] || [ -d "$SYSNET/$bridge/bridge" ] || die "no bridge $bridge here: --bridge names one"

	# The first install makes the Org: what it needs is asked for here, on
	# the host's terminal, not in the container; with --yes, nothing is.
	first=1
	if [ -z "$new" ]; then
		[ "$(pct status "$ctid")" = "status: running" ] || pct start "$ctid"
		if pct exec "$ctid" -- test -f /var/lib/aeolus/aeolus.db; then
			first=
		fi
	fi
	if [ -n "$first" ]; then
		tty=
		if [ -n "$yes" ]; then
			:
		elif [ -t 0 ]; then
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
		for id in "$org" "$admin"; do
			echo "$id" | grep -qxE '[a-z0-9][a-z0-9-]{0,31}' ||
				die "\"$id\" is not an ID: 1 to 32 lowercase letters, digits and hyphens, starting with a letter or digit"
		done
		# The first admin's password, asked twice and not echoed, as the
		# installer would ask it (0176).
		if [ -z "$token" ] && [ -n "$tty" ]; then
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
		# Without one, the installer prints a token, and asks nothing in
		# the container.
		[ -n "$password" ] || [ -n "$token" ] || set -- "$@" --token
	fi
	[ -z "$org" ] || set -- "$@" --org "$org"
	[ -z "$admin" ] || set -- "$@" --admin "$admin"

	if [ -n "$new" ]; then
		# The newest Debian 13 template the host has, else the newest it
		# is offered.
		tstore=$(pvesm status --content vztmpl 2>/dev/null | awk 'NR > 1 && $3 == "active" { print $1; exit }')
		[ -n "$tstore" ] || die "no storage here holds container templates"
		template=$(pveam list "$tstore" 2>/dev/null | awk '{ sub(/^[^\/]*\//, "", $1) } $1 ~ /^debian-13-standard_/ { print $1 }' | sort -V | tail -n 1)
		fetch=
		if [ -z "$template" ]; then
			pveam update >/dev/null 2>&1 || true
			template=$(pveam available --section system 2>/dev/null | awk '$2 ~ /^debian-13-standard_/ { print $2 }' | sort -V | tail -n 1)
			[ -n "$template" ] || die "no Debian 13 template is offered to this host (pveam available)"
			fetch=1
		fi
		# Its disk: on ZFS, LVM or the like before a directory, the
		# roomiest first.
		if [ -z "$storage" ]; then
			storage=$(pvesm status --content rootdir 2>/dev/null | awk 'NR > 1 && $3 == "active" {
				print ($2 == "dir" || $2 == "nfs" || $2 == "cifs") ? 0 : 1, $6, $1 }' | sort -k1,1nr -k2,2nr | awk 'NR == 1 { print $3 }')
			[ -n "$storage" ] || die "no storage here holds containers"
		fi
		net=name=eth0,bridge=$bridge${vlan:+,tag=$vlan},ip=${ip:-dhcp}${gateway:+,gw=$gateway}

		echo "A new container for an Aeolus manager:"
		echo "  CT $ctid $hostname: $template, $cores cores, $memory MB, $swap MB swap, $disk GB on $storage"
		echo "  network: $bridge${vlan:+, VLAN $vlan}, ${ip:-DHCP}${gateway:+ via $gateway}"
		echo "  Org $org, first admin $admin"
		if [ -z "$yes" ]; then
			ok=
			if [ -t 0 ]; then
				printf 'Make it? [Y/n] ' >&2
				read -r ok
			elif (: </dev/tty) 2>/dev/null; then
				printf 'Make it? [Y/n] ' >&2
				read -r ok </dev/tty
			else
				die "nothing was made: say --yes to make it without asking"
			fi
			case $ok in '' | [Yy]*) ;; *) die "nothing was made" ;; esac
		fi

		if [ -n "$fetch" ]; then
			say "fetching $template"
			pveam download "$tstore" "$template" >/dev/null
		fi
		say "making CT $ctid"
		pct create "$ctid" "$tstore:vztmpl/$template" \
			--hostname "$hostname" --cores "$cores" --memory "$memory" --swap "$swap" \
			--rootfs "$storage:$disk" --net0 "$net" \
			--unprivileged 1 --features nesting=1,keyctl=1 --onboot 1 --ostype debian \
			--timezone host --tags aeolus --description "Aeolus manager, made by deploy/proxmox.sh (0175)" \
			${rootkey:+--ssh-public-keys "$rootkey"} >/dev/null
		pct start "$ctid"
	fi

	# Its network, before anything is fetched. A release file copied in
	# needs none, unless the template has no curl: so without the internet,
	# it goes on (0069).
	i=0
	until pct exec "$ctid" -- getent hosts deb.debian.org >/dev/null 2>&1; do
		i=$((i + 1))
		if [ "$i" -ge 60 ]; then
			case $from in
			'' | http://* | https://*) die "CT $ctid has no network that reaches deb.debian.org: pct enter $ctid" ;;
			esac
			say "CT $ctid does not reach deb.debian.org; going on, as the release file needs nothing from it unless curl is missing"
			break
		fi
		sleep 1
	done

	# The installer, and the code: a release file, copied in below or
	# fetched by its URL; else from a checkout, a bundle of it where the
	# container builds from one; else where AEOLUS_REPO says, for a
	# container that has no code yet or had it from a bundle. One given
	# another place by --repo before keeps it.
	pct exec "$ctid" -- install -d -m 0755 /opt/aeolus /usr/local/sbin
	pct push "$ctid" "$installer" /usr/local/sbin/aeolus-install --perms 0755
	if [ -z "$repo$from" ]; then
		origin=$(pct exec "$ctid" -- git -C /opt/aeolus/src remote get-url origin 2>/dev/null || true)
		if [ -n "$origin" ] && [ "$origin" != "$BUNDLE" ]; then
			:
		elif [ -n "$here" ]; then
			say "bundling $here"
			git -C "$here" bundle create --quiet "$tmp/aeolus.bundle" HEAD --branches --tags
			pct push "$ctid" "$tmp/aeolus.bundle" "$BUNDLE" --perms 0644
			[ -n "$origin" ] || set -- "$@" --repo "$BUNDLE"
		else
			set -- "$@" --repo "$PUBLISHED"
		fi
	fi
	# The files the installer is given, readable by root alone. A release
	# file copied in before is taken away first, and the SHA256SUMS that
	# came with it where the new one has none.
	if [ -n "$(ls "$tmp/push")" ]; then
		pct exec "$ctid" -- install -d -m 0700 /root/.aeolus
		case $from in
		'' | http://* | https://*) ;;
		*)
			pct exec "$ctid" -- find /root/.aeolus -maxdepth 1 -name 'aeolus-*.tar.gz' -delete
			[ -n "$sums" ] || pct exec "$ctid" -- rm -f /root/.aeolus/SHA256SUMS
			;;
		esac
		for f in "$tmp/push"/*; do
			pct push "$ctid" "$f" "/root/.aeolus/${f##*/}" --perms 0600
		done
	fi

	# The password goes on the installer's standard input, never on a
	# command line.
	if [ -n "$password" ]; then
		printf '%s\n' "$password" | pct exec "$ctid" -- /usr/local/sbin/aeolus-install --password-stdin "$@"
		password=
	else
		pct exec "$ctid" -- /usr/local/sbin/aeolus-install "$@"
	fi
	echo
	if [ -n "$from" ]; then
		echo "CT $ctid: a shell in it is pct enter $ctid; to upgrade it, from a newer release file unpacked: sh aeolus-<tag>-linux-$arch/deploy/proxmox.sh --ctid $ctid"
	elif [ -n "$here" ]; then
		echo "CT $ctid: a shell in it is pct enter $ctid; to upgrade it, from a newer checkout: sh deploy/proxmox.sh --ctid $ctid"
	else
		echo "CT $ctid: a shell in it is pct enter $ctid; to upgrade it: curl -fsSL $RAW/deploy/proxmox.sh | sh -s -- --ctid $ctid"
	fi
}

main "$@"; exit
