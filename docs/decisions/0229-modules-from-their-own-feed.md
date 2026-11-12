# 0229. A port image's ucode modules come from its own feed

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from the -49 flash of every Arista AP; the port session found the cause on the C-260's /rom. Written up by Claude
- Refines: 0069 (running without the internet), 0171 (an image that carries none of Aeolus), 0214 (a module that does not load)

## Context

- **After each sysupgrade to the port's -49 build, the agent's first restore took a ucode-mod-uclient that does not load.**
  - It happened on the C-260, C360-AP, Living Room and TestC360, 2026-10-28: `Error relocating /usr/lib/ucode/uclient.so: ucv_metamethod_lookup: symbol not found`.
  - The image carries no ucode-mod-uclient, -udebug or -math (B41, B78: they are the agent's, and the port does not carry them).
  - The manager's module feed for the image's kernel (0171) has builds of uclient and udebug for the image's ucode, 2026.07.09.
  - The image's own feeds are OpenWrt's snapshot, which has newer builds, for a newer ucode. apk takes the highest version, so the snapshot's came in.
- **The agent recovered a pass later,** as 0214 has it. That cost a minute or two after each flash with no agent, and a second network restart for vxlan, which dropped the AP's clients again.
- **restore counted a module it had just put in as back, whether it loaded or not.** Only the next pass found it broken.

## Decision

- **The ucode modules come from the module feed alone, where it has them.**
  - `required`, `add` and `restore` each install every ucode module they want from the manager's module feed for the AP's kernel by itself (`apk --repositories-file /dev/null -X <feed>`), at the version it has there.
  - That version stays pinned in apk's world, so a later install from all feeds keeps it.
  - A module the feed does not have, as ucode-mod-math, comes from the other feeds as before.
  - Under opkg, or with no module feed (an image of OpenWrt's own), nothing changes.
- **restore counts a module back only if it loads.** One that does not is said, as 0214's unfit, and restore is due again.

## Consequences

- **The first restore after a sysupgrade puts back a uclient that loads,** where the module feed has one. The agent reaches its manager on the first pass, and the network restarts once.
- **The module feed must hold every ucode module the agent needs that the snapshot builds for another ucode.** Today that is uclient and udebug. math from the snapshot loads on 2026.07.09, and it is put in the feed if a snapshot moves on past it.
- **A pinned module is not replaced by a newer build from the module feed until the next sysupgrade,** when the world is the image's again.
