# 0280. Copies for the next build

- Status: Accepted (Griff, 2026-11-01: "do the next-build copies thing")
- Date: 2026-11-01
- Refines: 0248 (copies put back before the network), 0260 (a module feed per build), 0276 (packages wait for the manager)

## Context

- **After a sysupgrade, aeolus-early puts back the packages beyond the image before the network,** from copies kept in /etc/aeolus/pkgs (0248). The network then starts once, with vxlan already in.
- **The copies' kernel modules fit only the kernel they were built for.** On an image with a new kernel, early leaves them out, and vxlan with them. restore puts them back once the network is up, and the network starts again for vxlan: one more restart of every AP's networks at each such flash.
- **The port's next build, -103 (2026-11-01), brings kernel 6.18.60** in place of -95's 6.18.55. The port won't carry vxlan in its images: it serves the devices, and Aeolus loads on top (Griff, 2026-10-24).
- **The manager can hold the next build's module feed before the flash** (0260). It's served by that build's kernel, which no AP runs yet, so it touches no AP's lines.
- **apk won't fetch a module built for a kernel other than the one running** ("unable to select package"). But the files are in the feed by name, and `apk verify` checks each against the keys the image trusts. -95 and -103 are signed with the port's one key.

## Decision

- **The manager lists the builds it has feeds for** at `/feeds/kmods/builds`: one a line, tag, kernel and target. The target is the feed's `feed-<target>` directory, or `-`.
- **`aeolus-packages keep` keeps the AP's kernel modules as its next build has them, in /etc/aeolus/pkgs-next.**
  - The next build is the newest build of the AP's own target that is newer than its own and has another kernel: a later line (v2026.10.17.1), or a higher number on the same one.
  - It reads the build feed's index first (apk update with that feed alone). apk searches only an index it holds: in v0.220.0, the C-360s, which had never read -103's, found none of its modules.
  - Each module beyond the image is fetched from that build's feed by its name there, and kept only where all of them come and verify. A stamp of build, kernel and names keeps it from fetching them again.
  - Where the manager has no newer build, what was kept goes.
  - Only a port image, which names its build, does any of this. An OpenWrt image's kernel modules come from OpenWrt's own feeds.
- **aeolus-early, on the kernel the kept modules are for, puts them back in place of the copies' own,** with the rest of the copies and before the network. Their sums must hold.
- **The sums are checked with `sha256sum -c`, output hidden:** `-s` is BusyBox's alone. The tests use GNU's, and the copies' check had never run under them.

## Consequences

- **A flash to a build with a new kernel starts the network once,** as one with the same kernel does, where its feed was on the manager an hour before the flash. The agent's hourly `keep` fetches them.
- **The next build's feed goes on the manager ahead of a flash, as soon as porting stages it.** One for another kernel than the APs run serves none of their lines meanwhile. One with the same kernel still waits for the flash: the kernel-only line serves the newest build's index.
- **The kept modules take about 100 KB of the overlay** (six modules on a C-360).
- **Checked:**
  - a -95 C260 fetched kmod-vxlan and kmod-veth from -103's feed by name, and `apk verify` passed them;
  - Go tests of the listing (by tag, kernel and target), of keep (the right build of the right target; not fetched again; none kept where one won't verify; taken away with no newer build), and of early (not used on the kernel before; used after; not where the sums don't hold);
  - the agent's whole package suite passes.
