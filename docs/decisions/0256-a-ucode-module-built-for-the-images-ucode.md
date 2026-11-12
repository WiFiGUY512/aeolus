# 0256. A ucode module built for the image's own ucode

- Status: Accepted (Griff, 2026-10-31: "do 1 2 and 3"; 3 was getting Aeolus ready for the ucode update, with its first run on the C-260)
- Date: 2026-10-31
- Proposed by: Claude, on the C-260's first boot on port -82
- Refines: 0229 (ucode modules from the module feed), 0248 (packages back before the network), 0171 (module feeds)

## Context

- **A ucode module built for another ucode installs and doesn't load** (faults entry 36): its libucode is another ABI than the image's.
- **apk takes the highest version a feed offers, whatever ucode it was built for.** It doesn't fall back to a lower version where the highest can't go in: it fails.
- **On 2026-10-28 (0229) that was OpenWrt's snapshot beside port images of an older ucode.**
  - The snapshot's uclient was the higher version and didn't load.
  - So restore took the ucode modules from the manager's module feed for the AP's kernel first, pinned there: the port's own builds, -r2, for the images' ucode (`libucode20230711`).
- **Port build -82 (2026-10-31) moved to the snapshot's ucode** (`libucode20261009`).
  - Its kernel is the same as every port build's since -36. The manager keys module feeds by kernel version (0171), so -82 is served the same module feed, of build -36: uclient and udebug -r2, built for the old ucode.
  - On the C-260, the first boot on -82:
    1. The kept copies didn't go back before the network: the old uclient's libucode wasn't there, and one copy that can't go in keeps every one out.
    2. Restore then put vxlan and the kernel modules in once the network was up, and netifd had to start anew to use vxlan (faults entry 6). The lab's DHCP server was down at that moment, so the C-260 was left with no address for 11 minutes.
    3. uclient and udebug never went in: apk chose -r2, and failed, `libucode20230711 (no such package)`. The agent couldn't reach the manager.
    - Installing the snapshot's builds (-r1, and udebug's) by hand mended it: every module loads, and the agent runs.

## Decision

- **A build of a ucode module fits where apk can put it in without putting in another libucode beside the image's.** apk is asked first, with `--simulate`. A build for another ucode either can't go in, where no feed offers its libucode, or would bring that libucode in.
- **Of a ucode module, the build apk would choose is taken where it fits, else the first other build in the feeds that does,** pinned to it, as one from the module feed was (0229). That holds in both directions: the port's -r2 on an image of the old ucode, the snapshot's -r1 on -82.
- **It's asked of the module feed alone first,** as 0229 does, and then of every feed: in restore, in each install, and in installing again a module that doesn't load.
- **Where the kept copies won't all go in before the network, ucode modules are left out first.** Next the kernel modules and vxlan, as before, then both. So vxlan and the kernel modules still go in before the network where only a ucode module is wrong, and the network isn't started anew for them. Restore fetches what was left out, the build that fits.

## Consequences

- **An AP that moves to an image of another ucode gets modules that load,** from whichever feed has them: the agent reaches its manager after the first restore, by itself.
- **Each ucode module asked for costs an apk simulation or two:** a second or two, in restore and in an install only.
- **The C-260's 11 minutes without an address came from a network started anew while DHCP was down.** That can still happen where vxlan itself can't go in before the network.

## Not done here

- **Module feeds kept by the build, not the kernel** (the porting session's suggestion, 2026-10-31): by the build's tag in `/etc/openwrt_release`, so builds of one kernel and another ucode don't share a feed. With this decision a shared feed does no harm; keying by build is the cleaner way, and the manager's change.
- **Keeping the address where the network has to start anew:** an AP whose DHCP server is down then has none until it answers.
