#!/bin/sh
# Installs the Aeolus agent on an OpenWrt AP by copying its files, for an AP
# without the aeolus-agent package (0040, 0080). Run it on the AP from the
# directory it was unpacked in:
#
#	sh install.sh <manager URL> <uplink port> <manager certificate>
#
# for example
#
#	sh install.sh https://192.168.20.60:8443 wan /tmp/manager.crt
#
# It puts the agent's files in place, inert, and hands over to aeolus-setup,
# which joins the AP to the manager and installs the packages Aeolus needs
# through the manager's cache of OpenWrt's feeds (0069). With the package
# installed (apk add aeolus-agent), run aeolus-setup with the same arguments
# instead. The agent's files and settings are kept over a sysupgrade, and
# after one the packages it needs are installed again from the manager.
set -eu

[ $# -eq 3 ] || { sed -n '2,10p' "$0" >&2; exit 2; }
here=$(dirname "$0")

cp -R "$here/files/." /
# The programs and the init scripts run, by where they are, as the manager's
# bundle gives them their modes: a list of names here fell behind the files.
(cd "$here/files" && find usr/sbin usr/libexec etc/init.d -type f) | while read -r f; do
	chmod 0755 "/$f"
done

exec /usr/sbin/aeolus-setup "$@"
