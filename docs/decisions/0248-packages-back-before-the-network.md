# 0248. Packages back before the network

- Status: Accepted (Griff, 2026-10-29: "make the package restore not restart the radios"; and, of putting them in the port's images instead, "do not include aeolus only packages into openwrt")
- Date: 2026-10-29
- Proposed by: Griff. Written up by Claude
- Refines: 0069 (packages through the manager), 0080 and 0234 (restore after a sysupgrade), 0172 (the wpad swap)

## Context

- **A sysupgrade keeps the agent's files (keep.d), not the packages Aeolus installed:**
  - vxlan and its kernel modules;
  - ustp, usteer, snmpd-ssl;
  - udebugd;
  - ucode-mod-socket, and on the Arista ports ucode-mod-uclient and ucode-mod-math;
  - the full wpad where the image's is basic.
- **restore put them back through the manager once the network was up.** Two of them restart every radio there:
  - the full wpad starts hostapd anew;
  - vxlan wants netifd restarted, which loads protocols only as it starts.
- **On 2026-10-29's flashes to the port's -62, each AP's radios started twice.** A 5 GHz radio on a DFS channel listened a minute for radar twice: the C-260 on 140, ce7f on 100.
- **Putting the packages in the port's images is not the port's to do:** they are Aeolus's.

## Decision

- **restore keeps a copy of each package installed beyond the image's own** (the installed set less /rom's) in `/etc/aeolus/pkgs`:
  - That folder is under /etc/aeolus, which a sysupgrade keeps.
  - It is fetched through the manager, again only when what is installed changed.
  - It is kept only whole, and where the overlay has room for it with 1 MiB to spare.
- **apk vouches for a package by its feed's signed index, not by the file:**
  - so the SHA-256 of each copy is written beside it (`.sums`) as it is fetched;
  - and only copies that still match are put back, with apk told to take them unsigned.
- **A new init script, aeolus-early, runs at the start of boot (S11).** That is after `boot`, before udebugd (S11), wpad (S19) and the network (S20).
  - **After a sysupgrade** (restore's done mark gone with the old image) it runs `aeolus-packages early`, which:
    - installs the copies in one apk change with no repository, the full wpad replacing the basic one in the same change;
    - where that fails (kernel modules for an older kernel, after a new image's new kernel), installs all but the kernel modules and vxlan;
    - unpins what apk took from the files, in its world, to plain names, as restore would have put them;
    - starts udebugd, ustpd and usteer where they came back and are enabled, since boot read its list of init scripts before they were there.
  - **On any other boot it does nothing.**
- **restore still runs as before once the network is up.** It finds nothing missing and does nothing, or puts back what early could not, restarting as before.
- **Logging:** early also writes to the kernel's log, since logd is not up yet at S11.
- **The init script `aeolus` enables aeolus-early** where a fleet update brought it. keep.d keeps the script and its link.

## Consequences

- **After a flash, the radios start once, with what they need, and a DFS channel's radar wait is one minute, not two.** The agent also starts at once on an image that lacks the ucode modules it needs.
- **About 3 MB of the overlay holds the copies on an Arista port** (libopenssl3 for snmpd-ssl is the most). Without room, none are kept, and restore works as before.
- **The web installer (0083) had to learn its place.** Its list of the places Aeolus's files have named `/etc/init.d/aeolus` alone. So from v0.198.0 it refused every new AP's install, before putting anything in place (found 2026-10-30, re-adopting ce7f).
  - It takes `/etc/init.d/aeolus-*` now.
  - `install.sh` and the package's Makefile give the modes by where a file is, as the bundle does, not by a list of names. The package left aeolus-early 0644, so its init script never enabled it.
  - A test holds the installer to every file the agent and LuCI's page ship (`TestInstallerTakesEveryFile`).
- **Copies were kept only by restore, which runs only after a sysupgrade.** install and add kept none, so an AP set up afresh had no copies at its first sysupgrade (found 2026-10-31).
  - ce7f had been flashed clean and reinstalled. On its flash to the port's -63, restore put vxlan back after the network and restarted the network, so every radio started twice.
  - The other APs had their copies from the restore after their flashes to -62.
  - Now the agent runs `aeolus-packages keep` as it starts and each hour, once restore is done. It fetches copies again only where what is installed changed, the same check restore's keeping makes. A package a setting asks for is in the next sysupgrade's copies too.
  - `TestPackagesKeep` holds it: copies of what is beyond the image only, none fetched again unchanged, and an added package kept.
- **apk only.** Under opkg (OpenWrt before 25.12) nothing is kept, and restore works as before.
- **A new kernel means new kernel modules,** which the copies cannot be: those, and vxlan, still go in after the network, with its restart.
- **Tested on the C-260 (-62):**
  - restore kept 20 copies;
  - tcpdump-mini and libpcap removed, `early` put them back offline in under 4 s;
  - vxlan and kmod-vxlan removed, the done mark taken away, and the AP rebooted, as after a flash: aeolus_50 came up on vxlan at once, nothing restarted the network, each radio started once, and 140 listened for radar once;
  - restore then found nothing to do;
  - apk's world holds no pins.
