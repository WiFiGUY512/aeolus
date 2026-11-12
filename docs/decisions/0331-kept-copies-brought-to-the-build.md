# 0331. Kept copies brought up to the build's own

- Status: Accepted (the porting session asked, 2026-11-06; within Griff's standing go for autonomous development)
- Date: 2026-11-06
- Proposed by: the OpenWrt porting session, after -130 on the C-260: "Could the restore prefer the build's feed when the feed has the package, and use kept copies only for what the feed lacks?" Written up by Claude
- Refines: 0248 (copies put back before the network), 0315 (the build's own feeds)

## Context

- **After a sysupgrade, `early` puts back the copies of the packages beyond the image** from `/etc/aeolus/pkgs`, before the network comes up (0248).
- **Those copies were kept under the build before.** `restore` then finds nothing missing, so a package the new build's own feeds have in a newer build stays at the old one.
- **The case:** -130's feed has ucode-mod-math r5, and the C-260 kept r1 from -129. It's the same source and ABI, so it's harmless, but the AP is left mixed.

## Decision

- **`aeolus-packages keep`, which the agent runs every hour once restore is done, first brings packages up to the build.**
  - It covers each package beyond the image whose name starts with one in `AS_BUILT` (today `ucode-mod-`).
  - Where the build's own feeds (0315) hold another build of that package, keep puts that build in.
  - It does so only where the build fits: no other libucode, and no other ABI of a library the AP has (0256, 0318).
  - It checks that the module loads afterwards.
- **Kernel modules and the wpad are left out.**
  - Kernel modules go with the image's kernel.
  - The wpad's upgrade starts the radios anew.
- **Then keep keeps its copies as before.** They now match what is installed.
- **The running agent takes up the new build at its next start.** Keep does not restart it, so it never cuts a client test short (#348).

## Consequences

- **A flash leaves an AP on its build's own ucode modules within the hour.** Before, they stayed at whatever the last build had until the next one.
- **Tried on the C-260 (-130, 2026-11-06 13:34 CST):** ucode-mod-math went r1 → r5 and loads, and 19 copies were kept again.
