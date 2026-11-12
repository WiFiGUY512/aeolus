# 0315. A build's own packages

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from the port -121 flash run (2026-11-05)
- Refines: 0229 (ucode modules from the module feed), 0230 (feeds through the manager), 0248 (copies kept for after a sysupgrade), 0260 (a feed by build)

## Context

- **Packages beyond a port image** come back after each sysupgrade (0248): uclient and the other ucode modules, usteer, ustp, udebugd, snmpd, vxlan. They come from the copies kept before it, else from the manager's feeds once the network is up.
- **The manager's feeds are its module feed for the build (0260) and OpenWrt's snapshot (0230),** mirrored from upstream as it changes.
- **On 2026-11-03 the snapshot moved to a new libubox ABI (libubox20261103),** and built uclient, udebug, udebugd, usteer and ustp again under the same versions. The port images have libubox20260721.
- **On 2026-11-05, after the -121 flashes of the C260, C-200 and W-318:**
  - The copies kept could not go back before the network: the kept usteer and ustp needed libubox20261103, and early puts them back all or none.
  - So restore fetched them, and uclient, from the snapshot, with libubox20261103 beside the image's.
  - Each agent process then held two libubox, so two uloops. uclient's socket and timer were in the other one, and the agent waited on its first request for good: no state reports, no actions, for half an hour.
  - The APs' live daemons went on calling, so no AP was offline. With the reports old, nothing from them was said either.
- **The build's own package feeds** (`feed-<target>/<arch>/{base,packages}`, signed with the image's key) have every one of those packages, built against the image's libraries. The manager didn't serve them.

## Decision

- **The manager serves a build's own package feeds beside its module feed,** as the builder lays them out: `/feeds/kmods/<kernel>/<tag>/<arch>/<feed>/`. It does so only where the module feed is the builder's `feed-<target>/packages`, and never through a link. Each build's feeds are copied in with its module feed.
- **A port AP lists them** in `aeolus-kmods.list`, after its module feed, where the manager has them. The arch it asks for is OpenWrt's package arch, as `/etc/apk/arch` (or `DISTRIB_ARCH`) has it: `apk --print-arch` says only `aarch64`. v0.262.0 asked for that and found no feed; fixed in v0.262.1, whose keep rule (4) fetches every AP's copies again.
- **Restore installs what they have from them alone,** at their version (`from_feed`, before anything is asked for by name), not just the ucode modules.
- **Keep keeps their builds** (`keep_copies`), fetched from them alone. It doesn't take the snapshot's same version, nor fetch a copy again by the build installed.
- **Early, where the copies won't all go in,** leaves out only those apk names as needing what it cannot select, and puts the rest back before the network: vxlan and the kernel modules among them.
- **An alert, `reports-stopped`:** an AP that calls, and whose last state report is more than 15 minutes old.

## Consequences

- **What a port build has comes from it,** whatever OpenWrt's snapshot does under the same version.
- **A build's feeds must be copied in for this to hold.** Without them, an AP falls back to the snapshot as before.
- **Each AP's copies are fetched again once,** under keep's new rule (3).
- **Packages the build doesn't have** (ucode-mod-math) still come from the snapshot.
