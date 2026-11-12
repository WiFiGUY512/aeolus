# 0342. aeolus-packages lets go of the init script's lock

- Status: Accepted
- Date: 2026-11-08
- Proposed by: the OpenWrt porting session, whose sysupgrade of C360-AP hung on it. Written up by Claude, and accepted with the merge
- Refines: 0234 (packages put back at boot, until they load), 0237

## Context

**The init script starts the reinstall at boot.** After a sysupgrade, the agent's packages are gone. Where the image lacks a module the agent needs, `/etc/init.d/aeolus` starts `aeolus-packages boot` in the background, and it tries again until they load (0234):
- half a minute after its first try;
- then twice as long each time;
- then every ten minutes.

**The background run took the script's lock with it.**
- OpenWrt's `rc.common` holds each init script's lock, `/var/lock/procd_<name>.lock`, on fd 1000 while the script runs.
- The background run inherited fd 1000 and kept the lock for as long as it ran.
- Every other call of `/etc/init.d/aeolus` waited on it meanwhile: `enabled`, `running`, `restart`.
- So did `sysupgrade -b`, which asks each init script whether it is enabled as it backs up the config, and with it any sysupgrade that keeps settings.

**C360-AP, 2026-11-08, on port -133.**
- The packages it needs could not be put back on that boot: `usteer` and `ustp` only with a second libubox (0318), and `ucode-mod-uclient` and `ucode-mod-udebug` built for another ucode. So the run kept trying.
- Porting's `sysupgrade -b` hung from 14:52 to 15:05 CST, its backup empty, until the run was killed by hand.

## Decision

- **`aeolus-packages boot` closes fd 1000 as its first act, where it is open:** `[ -e /proc/$$/fd/1000 ] && eval 'exec 1000>&-'`.
  - It is done in the run, not where the init script starts it. That way it holds for an AP still running the init script it booted with after the fleet updates its agent, and for any other way the run is started.
  - The test is there because busybox ash, the APs' shell, takes fd 1000, and dash, which runs the agent's tests, takes no fd past 9.
- **The run keeps its own lock, as before** (`/var/run/aeolus-packages.boot`), so one runs at a time.

## Consequences

- **A sysupgrade, a config backup, and the init script's other calls no longer wait on a reinstall,** however long it tries.
- **Checked on TestW318's busybox ash:** a child that runs `exec 1000>&-` has no fd 1000 after it, while its parent holds the lock.
- **Why C360-AP's packages could not be put back on its second boot on -133, when they could on its first (13:47 CST),** was the manager's copy of -133's feed, not this script. It held the build's modules alone, without the build's own arch feeds (`aarch64_cortex-a53/{base,packages,luci}`). So the agent kept the snapshot mirror's `ucode-mod-uclient` (built against libubox 20261103), and the next sysupgrade put that back. The arch feeds were copied in at 15:1x CST, and C360-AP was given the build's own `uclient` by hand.
