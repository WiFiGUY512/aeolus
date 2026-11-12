# 0172. A wpad swap that cannot lose the Wi-Fi

- Status: Proposed
- Date: 2026-10-17
- Proposed by: Griff, "do 1-4", the fourth being the design question 0171 left under Not done. Written up by Claude
- Refines: 0050, 0057, 0171

## Context

- **The full wpad replaces the image's basic one, for 802.11v** (0050, 0057): band steering and BSS transition need it. Under apk (OpenWrt 25.12 and later) it is one change, made or not. Under opkg (24.10 and before) the basic one is removed first, then the full one installed.
- **0171 made trying the opkg swap safe** (both wpads fetched before the basic one goes; a failed remove changes nothing; the basic one back from its copy or the feeds; the Wi-Fi restarted only where a wpad went), and left three weak points, as they were before it:
  - **A swap cut off between its two steps** (a power cut, the AP restarted) leaves no wpad: the AP serves no Wi-Fi, and nothing puts one back.
  - **An overlay too full for either** leaves no wpad the same way. The basic wpad is the image's, in /rom: removing it writes whiteouts and frees nothing, so the full one needs room of its own, and the basic one, should it go back, as much again.
  - **A full wpad that will not install** (no room the check saw, a dependency the feeds cannot meet) is tried again each hour while restore is not done: each time the basic one is removed and put back and the Wi-Fi restarted, every client dropped.
- **Every wpad, and hostapd's own packages, give `/usr/sbin/hostapd`;** a wpad removed takes it away. The image's own package database, in /rom, says which TLS library its wpad was built with.
- **opkg's index gives an Installed-Size, but `opkg info` does not print it;** the size is that of the package's data.tar.gz unpacked, which a downloaded `.ipk` holds (an ipk is a gzipped tar of debian-binary, data.tar.gz and control.tar.gz). Measured with busybox on a C-360 for 24.10.0's wpad-basic-mbedtls: 1167360 bytes, the index's own figure.

## Decision

- **A wpad missing where there should be one is mended** (`repair_wpad`): where `/usr/sbin/hostapd` is gone and the image came with one (in /rom) or a swap of Aeolus's was under way, restore puts one back: a wpad still listed whose files are gone is installed again; else the basic one of the swap's or the image's TLS library (from /rom; mbedtls, OpenWrt's default since 23.05, where it cannot be read), else the full one; and the Wi-Fi comes up on it. The basic one first, as the smaller: full_wpad, with its room check, then replaces it as it would the image's. A device built without a hostapd (wired only, a Wi-Fi station) is left as it is.
- **Before a swap takes anything away it is written down** (`/etc/aeolus/wpad-swapping`, its TLS library, kept over a restart) **and restore made due** (`/usr/lib/aeolus/packages` removed): so a swap cut off between its steps is mended by the next restore, from the init script or the agent, which runs restore while it is due. The note goes once a wpad is there; an install or restore that leaves no wpad where one belongs is not done.
- **Under opkg nothing is removed without room for both wpads:** the free space of /overlay (or /, without one; `df -Pk`, whose lines do not wrap) must hold both downloaded packages unpacked, with 256 KiB to spare, compared in KiB. Each package and its data.tar.gz must read whole first; a size that cannot be read is no room: nothing is removed on a guess.
- **A full wpad that would not install is not tried again on that image:** its package's file name is written to `/usr/lib/aeolus/wpad-refused` however the basic one came back (from its copy or the feeds), and restore counts the wpad as settled. A sysupgrade drops the note, so each image tries once; one that goes in clears it. Where the AP has no hostapd the note does not settle anything.
- Under apk the swap stays one change; it too is written down and restore made due before it, and whether the basic one went is read from what is installed.

## Consequences

- An AP whose swap is cut off, or whose overlay filled, serves Wi-Fi again once restore has run with the network up: at the agent's start, or within the hour.
- An opkg AP with little room keeps its basic wpad: band steering and BSS transition are refused on it, as where the swap fails, and the AP says why. Small-flash opkg APs that swapped before may now keep the basic one after their next sysupgrade, since the check counts both packages unpacked.
- An opkg AP whose full wpad will not install drops its clients once an image, not each hour.
- Tried against stand-ins for opkg (with real `.ipk` files; a package installed already is left as it is, files and all, without `--force-reinstall`), apk (likewise without `apk fix`), df and /rom (`agent/packages_test.go`, 15 tests): no room, a package that cannot be read, a refused version not tried again and tried after a sysupgrade, the refused note however the basic one came back, a missing wpad mended (the basic one, then the full one, of the image's TLS library, under both), a swap that lost both wpads written down and mended on an image without a hostapd in /rom, a listed wpad whose files are gone installed again, a device built without a hostapd left alone, and an install without its wpad not done. On the hardware: the size read of 24.10.0's wpad-basic-mbedtls on a C-360's busybox is 1167360 bytes, the index's own Installed-Size. Not tried on an opkg AP: none of the lab's is reachable.

## Not done

- **An AP whose only uplink is its own Wi-Fi** (a mesh point) cannot fetch a wpad once it has none; its basic one is in /rom, but opkg cannot put back a package it removed from there without the package file.
