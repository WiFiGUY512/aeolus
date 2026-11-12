# 0260. A module feed for each build

- Status: Accepted (Griff, 2026-10-31: "do 2", the second of the next things put to him; the porting session's suggestion)
- Date: 2026-10-31
- Refines: 0171 (module feeds by kernel), 0229 (modules from their own feed), 0256 (a ucode module built for the image's ucode)

## Context

- **The manager serves module feeds by kernel version** (0171), and of several feeds for one kernel, the newest.
- **Every Arista port build since -36 has one kernel** (6.18.55~ff43a0f…-r1 on ipq807x). The manager held -36's feed alone, so every later build was handed -36's packages.
- **That broke when -82 moved to a newer ucode (libucode20261009):** -36's ucode modules were built for the old one.
  - 0256 makes an AP leave a module that doesn't fit to the other feeds.
  - It also makes the copies kept for a sysupgrade the builds installed.
  - That steers around the wrong feed; it doesn't give the right one.
- **One build has a feed for each target** (ipq807x, ipq60xx), each with its own kernel. So the build alone can't name a feed either.

## Decision

- **A feed is also of a build:** the build whose tag (`v2026.10.17.1-83-g439336f`) is in its directory's path below the manager's module feed directory, as the port's builder names its output (`out/main/<tag>/feed-ipq807x/`). The manager serves it by kernel and build too:

      /feeds/kmods/<kernel version>/<build tag>/packages.adb

  Of several for one kernel and build, the newest settled one is served, as by kernel. By the kernel alone, nothing changes.
- **A port image asks for its own build's first:** `aeolus-packages` reads its tag from `/etc/os-release` (`OPENWRT_RELEASE`'s "arista v…"). Where the manager answers 404, it takes its kernel's, as before.
  - A feed of its build found later replaces the kernel's line, and restore is due again, as a newly found feed's is.
  - An image of OpenWrt's own has no tag and asks by its kernel alone.
- **Each build's feed has to be copied to the manager,** under a path with its tag, as -36's was. That is the port's to do with each build it puts on an AP, once.

## Consequences

- **An AP on a build whose feed the manager has gets that build's modules,** for its own ucode, before 0256's fallback is needed.
- **Until a build's feed is copied in, nothing changes:** the AP asks, gets a 404, and keeps its kernel's. Tried on the C260 (-83) against the manager before this: the build's URL answered 404, the kernel's answered, and its feed line stayed as it was.
- **An AP asks for the feed when its agent starts,** and each hour while restore is due. So one copied in later is taken at the AP's next agent start, a release or a sysupgrade.
