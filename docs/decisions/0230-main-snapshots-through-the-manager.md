# 0230. OpenWrt's main snapshots through the manager

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from the -49 flash, where the restores' feed lines named OpenWrt's server. Written up by Claude
- Refines: 0069 (running without the internet), 0171 (an image that carries none of Aeolus)

## Context

- **APs fetch OpenWrt's packages through the manager (0069).**
  - The agent rewrites the AP's feed lines from OpenWrt's release tree, `https://downloads.openwrt.org/releases/`, to the manager's `/feeds/`.
  - The manager keeps a cache of that tree, so a site keeps installing what its cache holds when its internet is gone.
- **The port's -49 build is built from main.** Its feed lines name OpenWrt's main snapshots, `https://downloads.openwrt.org/snapshots/`. The agent did not rewrite those lines, and the manager's cache does not answer that tree.
  - After the flash of 2026-10-28, every Arista AP's restore went to OpenWrt's server itself, as Living Room's feed lines showed.
  - Without the internet, those restores would have had nothing but the module feed (0171).
- **Everything in the main snapshots is built anew under the same names,** as a release branch's snapshots are (0069). An index older than minutes names packages of another build.

## Decision

- **The manager keeps a second cache, of OpenWrt's main snapshots, and answers it under `/feeds/snapshots/`.**
  - It sits beside the release cache's directory (`feeds-snapshots`), with the same size limit.
  - Every index in it is a snapshot's: it is fetched again after five minutes, and when it has changed, the packages kept beside it go.
  - It answers only its own tree, as the release cache does.
- **The agent rewrites the main snapshots' feed lines to the manager's `/feeds/snapshots/`,** beside the release tree's and those of a manager the AP knew before.

## Consequences

- **An AP on an image built from main fetches its packages through the manager,** and a site whose internet is gone keeps installing what the cache holds.
- **The manager may hold a second copy of up to the same size,** 2 GB unless set. In the lab, the release cache holds 12 MB.
- **An AP's feed lines change on its next restore or feeds run** once its agent has this. Until then it keeps fetching from OpenWrt's server, as before.
