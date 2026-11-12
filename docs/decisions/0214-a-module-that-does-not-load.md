# 0214. A module that is there and does not load

- Status: Accepted; refined by 0229 (a port image's ucode modules come from its own feed)
- Date: 2026-10-24
- Proposed by: Claude, from the port session's report on Living Room and OpenWrt3c8f; Griff: "how close are you to fixing uclient". Written up by Claude
- Refines: 0069, 0080, 0171

## Context

Living-Room-C-360 and OpenWrt3c8f were sysupgraded to an openwrt-arista image whose ucode is 2026.07.09. After the upgrade, the agent's restore (0069) put back the packages the AP had before. It took ucode-mod-uclient and ucode-mod-udebug from OpenWrt's snapshot feed, and those were built for a newer ucode. Both installed, and neither loads: `ucv_metamethod_lookup: symbol not found`.

Three things then went wrong:

- **The agent took a module that was there for one that works.**
  - The init script checked only that `/usr/lib/ucode/uclient.so` exists, so it started the agent.
  - The agent found it could not load uclient and waited for it (`stranded`, 0080).
  - In ucode, a second `require()` of a module whose load failed returns null in the same process, without throwing. `stranded` read that as uclient back, and started the agent anew.
  - It did this every five seconds, hundreds of times, and the agent never reached its manager.
- **restore did not see anything to do.** Every package was installed, so restore was done, and it never installed anything again.
- **aeolus-live started without its module,** for the same reason as the agent.

## Decision

- **A ucode module counts as there only if it loads.**
  - The init script tests each required module with `ucode -e "require(...)"`, as well as checking that its file is there. It starts aeolus-live only if socket and uclient load.
  - aeolus-packages tests ucode-mod-* packages the same way. A package that is installed and does not load is *broken*. restore and required treat a broken package as one to install again, as they do a missing one.
- **A broken package is installed again in place:** `apk add --upgrade`, or `opkg install --force-reinstall`. Where a feed has a build that fits, as the AP's own build's feed (0171) may, that build replaces it.
  - Where none fits, the agent says so: "…is installed but does not load on this image's ucode: it was built for another, and no feed has one that fits".
  - restore is then due again, so the hourly retry keeps trying. The same holds for a package that could not be installed at all.
- **The agent tests what it loaded, not only whether loading threw.** A uclient without `new` is no uclient, and the agent keeps why loading failed.
  - While it waits for uclient (`stranded`), it tests the module in a ucode of its own, since this process's `require` would say nothing.
  - It says whether uclient is missing, or there and unloadable.
  - It backs off as before, from a minute up to an hour.

## Consequences

- **An AP whose modules were built for another ucode waits quietly for one that fits.** It no longer loops, and it says why.
- **Living Room and OpenWrt3c8f come back without a hand install,** once a feed the manager serves has uclient and udebug built for their ucode. The port's build feed is that feed (0171).
  - apk and opkg take the highest version, so a build that fits must have a higher version than the one installed: a higher release than the snapshot's.
- **Each start of the agent and of aeolus-live costs a ucode run per module checked.** That is a few milliseconds.
