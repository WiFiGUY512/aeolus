# 0318. A package of another library ABI

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from the port -121 flash run (2026-11-05)
- Refines: 0256 (a ucode module's build that fits), 0315 (a build's own packages)
- See also: 0317 (two builds of a library in one process)

## Context

- **OpenWrt names a library's package by its ABI,** a number its name ends in: libubox20260721, libubox20261103. A new ABI is a new package, and apk installs it beside the old one rather than in its place.
- **On 2026-11-03 OpenWrt's snapshot moved to libubox20261103** and built uclient, usteer, ustp, udebug and udebugd again under the same versions. The port images had libubox20260721.
- **Packages that came from the snapshot put the new libubox in beside the image's,** and any process that loaded both held two uloops. Three agents waited on a socket in the other loop for good (0315), and ustpd and udebugd on the C260, C-200 and W-318 held both (0317).
- **0315 takes a build's packages from its own feeds,** where the manager has them. Where it does not (a build whose feeds were not copied in, a stock image, a package the build does not have such as ucode-mod-math), restore and install still asked the snapshot by name, and apk took its highest build, whatever libraries it brought.
- **0256 already kept a ucode module out where its build brings another libucode;** nothing did so for other packages or other libraries.

## Decision

- **Before a package goes in, apk simulates it** (`apk add --simulate`). Each library it would install whose name is a library the AP has, with another ABI number (`abi_beside`), is a library beside the AP's own.
- **Where the build apk chose brings none, it goes in as asked.**
- **Where it does, another build in the feeds that brings none goes in in its place,** pinned by name=version (`fitting`), as 0256 does for ucode modules.
- **Where no build fits, the package is left out,** and the log says why: "usteer is left out: the feeds' builds of it would put in libubox20261103 beside the AP's own build of that library…". Restore tries again on its next pass, as for any package that cannot be had.
- **A ucode module is checked for both** another libucode (0256) and another ABI of any library.
- **A library the AP does not have is no concern:** a package may bring libblobmsg-json20261103 where the image has no libblobmsg-json, and goes in.
- **A package apk cannot simulate at all is asked for as it is,** so apk says why it fails.

## Consequences

- **What a package needs that only the snapshot has, at another ABI, waits** for a build that fits: the port's feeds copied in, or the snapshot's ABI reaching the image. A package missing is said, and plain; a process holding two libraries stops without a word.
- **Every package costs one more simulated install** at restore and install time. Packages go in after a sysupgrade or a new setting, not each pass.
- **0317 still says it where two builds get in another way** (by hand, or an image built so).
