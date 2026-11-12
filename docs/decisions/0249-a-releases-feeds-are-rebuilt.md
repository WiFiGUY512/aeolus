# 0249. A release's feeds are rebuilt

- Status: Accepted (Claude, 2026-10-29, under Griff's standing word to develop Aeolus on its own)
- Date: 2026-10-29
- Proposed by: Claude
- Refines: 0069 (running without the internet), 0248 (packages back before the network)

## Context

- **The feed cache took a release's files never to change.** Only a release branch's snapshots were fetched again: their index within minutes, and the packages kept beside a changed index let go.
- **OpenWrt's buildbots build a release's package feeds (`packages/<arch>/<feed>`) again,** from the release's branch, under the same names:
  - 25.12.2's `vxlan-7.apk` is of 2026-10-28's build. The manager had kept the one it fetched on 10-04.
  - 25.12.2's `wpad-mbedtls` r5 is gone from the feed.
  - Only a release's targets, the image's own build and its kernel modules, never change.
- **apk refuses a package of another build than its index names ("ADB integrity error").** At v0.197.0, the first `restore` to keep copies of the packages (0248) could not fetch:
  - vxlan on the Office AP (25.12.2) and on Shed (25.12.5);
  - usteer on Shed;
  - wpad-mbedtls on the Office AP.
- **Each AP's own index was also days old:** restore reads it again only when it installs something.

## Decision

- **The feed cache treats a release's package feeds as it does a snapshot's:**
  - their index is fetched again after five minutes, not a day;
  - when it has changed, the packages kept beside it go, to be fetched again of the build it names.
- **A release's targets keep their day.**
- **restore reads the index again before it fetches the copies it keeps.** That happens only when what is installed has changed.

## Consequences

- **A package fetched through the manager is of the build the AP's index names,** unless OpenWrt rebuilds it in the minutes between the two.
- **Without the internet, nothing changes:** the cache serves what it keeps, and an AP's index matches it.
- **Packages let go when their index changes cannot be served while the internet is down.** They could not have been installed against that index anyway.
- **Tested:** the cache, with a release feed's index and package rebuilt upstream, serves the new pair after five minutes, and keeps a target's index and packages.
