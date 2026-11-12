#!/bin/sh
# Makes the release files of an Aeolus release (0316): for each target, a
# tarball of the manager built for it with the scripts that install, run and
# update it, and SHA256SUMS of them all. From a checkout, with Go:
#
#	sh deploy/release.sh [options] <tag>
#
#	--out <dir>       where they go, in a directory named for the tag; else
#	                  release/ in the checkout
#	--arch <a,b,...>  which of the targets below; else every one
#
# Each is built from the tag itself (git archive), never from what the
# checkout has besides, as the installer builds (0170): no C, -trimpath,
# -s -w, and the version <tag>-<commit>, so a manager put up from a release
# file says the version one built from git says. A tag is a release (vX.Y.Z);
# any other tag or commit is built too, for trying, and its files say so in
# their name. Unpacked, a release file is
#
#	aeolus-<tag>-linux-<arch>/
#		aeolus     the manager, with the agent in it (0079)
#		RELEASE    what it is: key=value, a line each
#		deploy/    install.sh, which installs it, proxmox.sh, the updater,
#		           the units and aeolus.sh, as the tag has them
#		docs/      getting-started.md, install.md, hardware.md and concepts.md
#
# and its own deploy/install.sh installs it. A tag whose installer takes no
# release file is refused.
#
# AEOLUS_GO names the go it builds with, for its tests.
set -eu

# The targets, one a line: the name in the file's, GOARCH, and GOARM where
# it has one ("-" where not). The installer takes a release whose name is
# the host's arch as it names it, so a new target is a line here and a line
# there.
TARGETS='
amd64 amd64 -
arm64 arm64 -
'

# What goes in beside the manager, from the tag, a path a line. One the tag
# has not is left out, but for the installer.
FILES='
deploy/install.sh
deploy/proxmox.sh
deploy/updater.sh
deploy/aeolus.service
deploy/aeolus-updater.service
deploy/aeolus-updater.path
deploy/aeolus.sh
docs/getting-started.md
docs/install.md
docs/hardware.md
docs/concepts.md
'

main() {
	die() {
		echo "aeolus-release: $*" >&2
		exit 1
	}
	say() {
		echo "aeolus-release: $*"
	}

	out= only= tag=
	while [ $# -gt 0 ]; do
		case $1 in
		--out | --arch)
			[ $# -ge 2 ] || die "$1 needs a value"
			case $1 in
			--out) out=$2 ;;
			--arch) only=$2 ;;
			esac
			shift 2
			;;
		-*) die "no option $1: the options are at the top of deploy/release.sh" ;;
		*)
			[ -z "$tag" ] || die "one tag at a time"
			tag=$1
			shift
			;;
		esac
	done
	[ -n "$tag" ] || die "name the release: sh deploy/release.sh <tag>"
	echo "$tag" | grep -qxE '[A-Za-z0-9][A-Za-z0-9._-]*' || die "\"$tag\" cannot name a file: a tag of letters, digits, dots, hyphens and underscores"

	repo=$(cd "$(dirname "$0")/.." && pwd)
	git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || die "run it from a checkout of Aeolus"
	commit=$(git -C "$repo" rev-parse -q --verify "$tag^{commit}") || die "no tag or commit $tag here"
	short=$(git -C "$repo" rev-parse --short "$commit")
	version=$tag-$short
	when=$(git -C "$repo" log -1 --format=%ct "$commit")
	echo "$tag" | grep -qxE 'v[0-9]+\.[0-9]+\.[0-9]+' || say "$tag is not a release tag (vX.Y.Z): these files are for trying"
	# The installer in the file must know release files, or the file
	# cannot be installed by itself.
	git -C "$repo" show "$commit:deploy/install.sh" 2>/dev/null | grep -q '^#	--from <file|URL|dir>' ||
		die "$tag's installer takes no release file (0316): build one from a newer tag"

	go=${AEOLUS_GO:-go}
	command -v "$go" >/dev/null 2>&1 || die "no Go here to build with"
	[ -n "$out" ] || out=$repo/release
	dest=$out/$tag

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	mkdir "$tmp/src" "$tmp/stage" "$tmp/out"
	# As committed, with no line ends turned to CRLF, as a Windows checkout
	# would have them: a script so changed does not run on the host.
	git -C "$repo" -c core.autocrlf=false -c core.eol=lf archive --format=tar "$commit" | tar -C "$tmp/src" -xf -
	gover=$(cd "$tmp/src" && "$go" env GOVERSION)

	# The same bytes from the same tag and toolchain, where tar can be
	# told to: names sorted, no owners, the commit's time.
	gnu=
	tar --version 2>/dev/null | grep -q 'GNU tar' && gnu=1

	n=0 names=
	while read -r name goarch goarm; do
		[ -n "$name" ] || continue
		names="$names $name"
		if [ -n "$only" ]; then
			case ,$only, in *,"$name",*) ;; *) continue ;; esac
		fi
		[ "$goarm" != - ] || goarm=
		base=aeolus-$tag-linux-$name
		stage=$tmp/stage/$base
		mkdir "$stage"
		say "building $version for linux/$name"
		(cd "$tmp/src" && CGO_ENABLED=0 GOOS=linux GOARCH=$goarch GOARM=$goarm \
			"$go" build -trimpath -ldflags "-s -w -X main.version=$version" -o "$stage/aeolus" ./cmd/aeolus)
		[ -s "$stage/aeolus" ] || die "the build for $name made nothing"
		for f in $FILES; do
			[ -f "$tmp/src/$f" ] || continue
			mkdir -p "$stage/${f%/*}"
			cp "$tmp/src/$f" "$stage/$f"
			case $f in
			*.sh) chmod 0755 "$stage/$f" ;;
			*) chmod 0644 "$stage/$f" ;;
			esac
		done
		chmod 0755 "$stage/aeolus"
		[ -x "$stage/aeolus" ] ||
			say "this file system keeps no file modes, as Windows': the manager in the file is not marked to run (the installer sets it). Make releases on Linux"
		# What the file is, a key a line, for the installer and the
		# updater to read; a reader passes over a key it does not know.
		printf '%s\n' name=aeolus "version=$version" "tag=$tag" "commit=$commit" \
			os=linux "arch=$name" "go=$gover" "time=$when" >"$stage/RELEASE"
		chmod 0644 "$stage/RELEASE"
		if [ -n "$gnu" ]; then
			(cd "$tmp/stage" && tar --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$when" -cf - "$base")
		else
			(cd "$tmp/stage" && tar -cf - "$base")
		fi | gzip -n -9 >"$tmp/out/$base.tar.gz"
		n=$((n + 1))
	done <<EOF
$TARGETS
EOF
	[ "$n" -gt 0 ] || die "no target $only: the targets are$names"

	mkdir -p "$dest"
	for f in "$tmp/out"/*.tar.gz; do
		mv -f "$f" "$dest/"
	done
	# SHA256SUMS covers every file of the tag here, a target built before
	# as well, in sha256sum's own form, so sha256sum -c reads it.
	(
		cd "$dest"
		for f in aeolus-"$tag"-linux-*.tar.gz; do
			printf '%s  %s\n' "$(sha256sum <"$f" | cut -d' ' -f1)" "$f"
		done >SHA256SUMS.new
		mv -f SHA256SUMS.new SHA256SUMS
	)
	say "$version is in $dest:"
	sed 's/^/  /' "$dest/SHA256SUMS"
}

main "$@"; exit
