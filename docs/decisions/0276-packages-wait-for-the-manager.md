# 0276. Packages wait for the manager

- Status: Accepted (Aeolus's own development, 2026-11-01)
- Date: 2026-11-01
- Refines: 0069 (packages through the manager), 0171 (a module feed per kernel), 0248 (copies kept for a sysupgrade)

## Context

- **The agent ran its package step as it started:**
  1. `aeolus-packages feeds`, which points the feeds at the manager and adds the line for the manager's module feed (0171, 0260);
  2. then `restore` or `keep`;
  3. then again every hour.
- **Every feed is the manager's** (0069).
- **A sysupgrade takes the module feed's line away** (`/etc/apk/repositories.d/aeolus-kmods.list` isn't kept). aeolus-early puts the packages back from the kept copies before the network (0248), so the agent starts early in the boot, before the AP can reach its manager.
- **So the first run reached nothing, on each AP's -95 flash (2026-11-01).**
  - The line wasn't added: the manager couldn't be asked, which leaves the line as it is, here none.
  - `keep` fetched 0 of 19 copies.
  - Both waited an hour. Meanwhile the AP had no module feed: nothing that needs a kernel module could be installed, and the copies weren't refreshed.

## Decision

- **The agent's first package run waits for the manager to answer a poll** (200 or 304), then runs every hour as before.

## Consequences

- **After a sysupgrade, the module feed's line is back within a poll of the manager being reached,** and the copies are kept from it.
- **An agent that never reaches its manager runs no package step,** which could fetch nothing anyway.
- **Restore after a sysupgrade, where the copies didn't cover it, waits for the manager too.** That's no loss: it fetches from the manager. The init script's boot run (0234), which retries until the modules the agent needs load, is unchanged.
