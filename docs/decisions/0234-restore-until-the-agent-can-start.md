# 0234. Restore tries again until the agent can start

- Status: Accepted; refined by 0237 (start what can start)
- Date: 2026-10-28
- Proposed by: Claude, from the C-260's flash to the port's -54 build. Written up by Claude
- Refines: 0080 (the agent as a package), 0083 (installing from the manager), 0229 (modules from their own feed)

## Context

- **The agent's files outlive a sysupgrade, and its packages do not.**
  - The Arista port images carry no ucode-mod-math and no ucode-mod-uclient (B41, B78), and the agent imports math.
  - Without math the agent dies as it starts: `Syntax error: Unable to resolve path for module 'math'`.
  - So the init script, which is plain shell, starts `aeolus-packages restore` itself when one of those modules is missing (0080).
- **It started restore once.** That try comes as the AP boots, when it may have no address yet:
  - The C-260's LAN takes its address by DHCP, behind a bridge.
  - A restore then reaches nothing, and nothing tried again. The restore said "the agent tries again in an hour", but the agent could not start to try.
  - The agent's own retry (stranded, 0080) covers uclient alone. The agent starts without uclient; without math it cannot start at all.
- **On 2026-10-28 the C-260 came up on -54 with no agent.**
  - procd started it again every five seconds, and each start failed the same way. Those failures filled the log, so the log no longer held the boot-time restore's own lines.
  - The agent stayed down until `aeolus-packages required` was run by hand ten minutes later. That run took a few seconds and installed both modules.
  - Earlier flashes to -49 had the same images without math. Their boot-time restore got through, by luck of timing.

## Decision

- **A new mode, `aeolus-packages boot`, is restore until the modules the agent cannot start without all load.**
  - It runs restore, then checks those modules.
  - If any is missing or does not load, it waits and runs restore again: half a minute, then twice as long each time, up to ten minutes apart.
  - It says once what the agent is waiting for.
- **The init script starts `boot` where it started `restore`:** when a required module is missing or does not load.
- **One boot run at a time, by a lock of its own,** taken over where its run is gone, as restore's is.
  - Each restore it runs takes restore's lock as before.
  - So a restore the agent starts, or one run by hand, is waited for, not run beside.
- **The boot run ends once they load.** The agent is then up, and anything else restore could not put back is the agent's to retry, hourly, as before.

## Consequences

- **An AP flashed to an image without the agent's modules gets its agent back within about half a minute of having an address,** however late that comes.
- **Where the manager cannot be reached for a long while,** the AP asks again each ten minutes until it can, with no one needing to run anything.
- **A restore that runs while the network is down still says the agent tries again in an hour.** Where the agent cannot start, the boot run's own line says what it is waiting for.
- **The port images still lack math and uclient,** so each flash depends on this, and on the manager having them in its feeds. Both are in the manager's feeds today (0229).
