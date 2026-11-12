#!/bin/sh
# Updates the manager when it asks (0183). Installed as
# /usr/local/sbin/aeolus-updater and run as root by aeolus-updater.service,
# which aeolus-updater.path starts when the manager leaves a request in
# /var/lib/aeolus/update. The manager runs as aeolus, without root: it can
# only ask, and only for one of two things:
#
#	check            fetch the release tags, and say the newest, and the
#	                 newest on the stable channel (0282)
#	update vX.Y.Z    build and install that release, a tag the repository has
#
# Anything else is refused. What came of it goes in the same directory, for
# the manager to read: status (key=value lines), update.log (the last run's
# output) and notes (what each release newer than the one running says of
# itself, 0217). Both are written to a new file and renamed into place, so a
# link the aeolus user leaves there is replaced, never written through.
#
# A manager put up from a release file (0316) has no repository to ask: both
# are answered with how a newer release goes in, by hand, for now.
#
# AEOLUS_UPDATE_DIR, AEOLUS_SRC, AEOLUS_BIN, AEOLUS_INSTALL, AEOLUS_UPDATE and
# AEOLUS_RELEASE are for its tests: where the requests are, the checkout, the
# binaries, the two ways it installs a release, and the record of a release
# file.
set -u

DIR=${AEOLUS_UPDATE_DIR:-/var/lib/aeolus/update}
SRC=${AEOLUS_SRC:-/opt/aeolus/src}
BIN=${AEOLUS_BIN:-/opt/aeolus/bin}
INSTALL=${AEOLUS_INSTALL:-/usr/local/sbin/aeolus-install}
UPDATE=${AEOLUS_UPDATE:-/usr/local/sbin/aeolus-update}
RELEASE=${AEOLUS_RELEASE:-/etc/aeolus/release}
REQ=$DIR/request
LOG=/var/log/aeolus-updater.log
[ -n "${AEOLUS_UPDATE_DIR:-}" ] && LOG=$DIR/.log-root

# put writes stdin to a file in DIR through a new file and a rename.
put() {
	tmp=$(mktemp "$DIR/.$1.XXXXXX") || return 1
	cat >"$tmp"
	chmod 0644 "$tmp"
	chown aeolus:aeolus "$tmp" 2>/dev/null
	mv -f "$tmp" "$DIR/$1"
}

# key reads one value of the status as it stands.
key() {
	[ -f "$DIR/status" ] && [ ! -L "$DIR/status" ] && sed -n "s/^$1=//p" "$DIR/status" | head -n 1
}

# status writes the status, from these values and the ones it keeps.
running=$("$BIN/aeolus" version 2>/dev/null | head -n 1 | tr -cd 'A-Za-z0-9._+-')
newest=$(key newest) stable=$(key stable) checked=$(key checked) state=idle tag=$(key tag) at=$(key at) error=
status() {
	printf 'running=%s\nnewest=%s\nstable=%s\nchecked=%s\nstate=%s\ntag=%s\nat=%s\nerror=%s\n' \
		"$running" "$newest" "$stable" "$checked" "$state" "$tag" "$at" "$error" | put status
}

# stable_of is the newest release on the stable channel (0282): one with a
# tag stable/vX.Y.Z on the very commit vX.Y.Z is, as a release is promoted
# once it has run a while on testing. Empty where none is.
stable_of() {
	git -C "$SRC" tag -l 'stable/v*' --sort=-v:refname | while read -r s; do
		t=${s#stable/}
		echo "$t" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$' || continue
		c=$(git -C "$SRC" rev-parse -q --verify "refs/tags/$t^{commit}" 2>/dev/null) || continue
		[ "$c" = "$(git -C "$SRC" rev-parse -q --verify "refs/tags/$s^{commit}" 2>/dev/null)" ] && { echo "$t"; break; }
	done | head -n 1 | tr -cd 'A-Za-z0-9._+-'
}

# notes writes what each release newer than the one running says of itself,
# its tag's message, the newest first and at most ten, for the manager's
# banner (0217): each a line "@@ vX.Y.Z", then its message. Printable text
# alone, at most 4 KB a release and 32 KB in all.
notes() {
	r=${running%%-*}
	git -C "$SRC" tag -l 'v*' --sort=-v:refname | tr -cd 'A-Za-z0-9._+\n-' | head -n 10 | while read -r t; do
		[ "$t" = "$r" ] && break
		printf '@@ %s\n' "$t"
		git -C "$SRC" tag -l --format='%(contents)' "$t" | tr -d '\000-\010\013-\037\177' | head -c 4096
		printf '\n'
	done | head -c 32768 | put notes
}

# fetch brings the release tags from where the code comes from, by the
# checkout's own way in; on a manager set up by aeolus-update, its deploy
# key (0162).
fetch() {
	if [ -z "$(git -C "$SRC" config core.sshCommand 2>/dev/null)" ] && [ -f /etc/aeolus/deploy_ed25519 ]; then
		export GIT_SSH_COMMAND="ssh -i /etc/aeolus/deploy_ed25519 -o IdentitiesOnly=yes -o UserKnownHostsFile=/etc/aeolus/known_hosts -o StrictHostKeyChecking=yes"
	fi
	git -C "$SRC" fetch -q --tags --prune origin
}

[ -d "$DIR" ] || exit 0
[ -f "$REQ" ] && [ ! -L "$REQ" ] || { rm -f "$REQ"; exit 0; }
ask=$(head -c 64 "$REQ" | head -n 1)
rm -f "$REQ"

# Put up from a release file (0316): a newer one is given to aeolus-install
# by hand. Neither a check nor an update fetches anything.
if [ -f "$RELEASE" ] && [ ! -L "$RELEASE" ]; then
	error="this manager was put up from a release file: a newer one goes in with aeolus-install --from <file or URL>"
	case $ask in
	check)
		checked=$(date +%s)
		status
		exit 0
		;;
	update\ v*)
		state=failed at=$(date +%s)
		status
		exit 1
		;;
	esac
fi

case $ask in
check)
	state=checking
	status
	state=idle
	if out=$(fetch 2>&1); then
		newest=$(git -C "$SRC" tag -l 'v*' --sort=-v:refname | head -n 1 | tr -cd 'A-Za-z0-9._+-')
		stable=$(stable_of)
		error=
		notes
	else
		error=$(printf '%s' "$out" | tr '\n' ' ' | tr -cd '[:print:]' | cut -c1-200)
		error="cannot fetch the release tags: $error"
	fi
	checked=$(date +%s)
	status
	;;
update\ v*)
	want=${ask#update }
	case $want in
	*[!A-Za-z0-9._-]*) error="refused: not a release" state=failed; status; exit 1 ;;
	esac
	echo "$want" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$' || { error="refused: not a release" state=failed; status; exit 1; }
	if ! fetch >/dev/null 2>&1 || ! git -C "$SRC" rev-parse -q --verify "refs/tags/$want" >/dev/null; then
		error="refused: the repository has no release $want" state=failed tag=$want at=$(date +%s)
		status
		exit 1
	fi
	# The tags were fetched just now: say the newest, as a check would, so a
	# release installed is never newer than the newest one said.
	newest=$(git -C "$SRC" tag -l 'v*' --sort=-v:refname | head -n 1 | tr -cd 'A-Za-z0-9._+-') checked=$(date +%s)
	stable=$(stable_of)
	tag=$want state=updating at=$(date +%s)
	status
	# The way this manager was put up: aeolus-update where it has its deploy
	# key, else the installer, which upgrades in place.
	if [ -f /etc/aeolus/deploy_ed25519 ] && [ -x "$UPDATE" ]; then
		set -- "$UPDATE" "$want"
	elif [ -x "$INSTALL" ]; then
		set -- "$INSTALL" --version "$want"
	else
		error="no aeolus-install or aeolus-update here" state=failed
		status
		exit 1
	fi
	if "$@" >"$LOG" 2>&1 </dev/null; then
		state=updated error=
	else
		state=failed error="$(basename "$1") failed: update.log says why"
	fi
	running=$("$BIN/aeolus" version 2>/dev/null | head -n 1 | tr -cd 'A-Za-z0-9._+-')
	at=$(date +%s)
	tail -n 200 "$LOG" | put update.log
	status
	notes
	[ "$state" = updated ]
	;;
*)
	error="refused: a request is check, or update and a release" state=failed
	status
	exit 1
	;;
esac
