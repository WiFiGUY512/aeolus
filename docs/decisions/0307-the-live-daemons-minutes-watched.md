# 0307. The live daemon's minutes, watched

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from the Living Room's 13-hour gap in its minutes (2026-11-04)
- Refines: 0207 (telemetry by the minute)

## Context

- **aeolus-live samples the AP's radios and clients every 5 seconds and sends what each minute came to** (0207). The flight recorder, power control, the struggling alert (0233), the journey's retries (0303) and the charts all read those minutes.
- **On 2026-11-04 the Living Room C-360 sent no radio's minute from 10:09 to 23:38 CST, on all three radios.** At 10:07 porting swapped its ath11k modules in RAM, which set up its radios again under new phy names. Its state reports went on. The minutes came back only when the v0.249.0 update restarted every instance.
  - Its client minutes stopped too, so 13 hours of its clients are missing from the journey and the struggling alert.
- **The Garage (12:21) and C360-AP (14:28) had the same swap and kept their minutes.** The Living Room's log from 10:09 is long gone, so whether its live daemon hung, or ran with no radio in view, is not known.
- **procd starts again a daemon that ends** (`respawn`). It does not restart one that runs and does nothing.
- **aeolus-live writes what each minute came to on each radio (`/var/run/aeolus/load.json`) as the minute ends,** with its radios by band, whether or not it has any.

## Decision

- **At each poll, the agent judges whether the live daemon's minutes stopped** (`aeolus.livewatch`):
  - `load.json` not written for 5 minutes; or
  - written with no radio for 5 minutes while hostapd serves networks.
- **It then has procd kill the `live` instance** (`service signal`, SIGKILL). procd starts it again as it would one that ended. The agent logs why, as a warning: `live: no minute for 400 s: started again`.
- **Not more than once every 30 minutes,** so a radio view that stays empty doesn't restart it again and again.
- **An AP whose live daemon never wrote `load.json` is left alone.**

### Why it ran and did nothing (added the same night)

- **Porting's account of the Living Room's morning:** three ath11k swaps in two hours, at 08:20, 09:52 and 10:05-10:07. Each was wifi down, then rmmod and insmod of all three modules, then wifi up. Each brought every phy back under a new index, and the hostapd ubus objects and debugfs paths with it (phy13-16 became phy17-20 at 10:07). The Garage and C360-AP had one swap each.
- **aeolus-live's minute tick set its next run as its last step.** An error anywhere in a tick, as in the bookkeeping of radios that came back on new phys, skipped that step and ended the ticks for good. The daemon itself ran on, its line to the manager open.
- **So the tick now does its work in a try:**
  - It sets the next tick whatever the work did.
  - It logs an error the first time and then once an hour (`minute: <error> (<n> so far)`), so the next one says where it is.
  - The print modes (`-m`, `-w`) still end where they did.
- **Tried on the C260:** `-w 2` ended after two points in 6 s, and `-m 1` after its minute with both radios.

## Consequences

- **A live daemon that stalls costs about 5 minutes of minutes,** not hours.
- **Tried on the C260 on 2026-11-05:** procd started `live` again within seconds (pid 20700, then 7755), and it reopened its line to the manager.
- **Hostapd serving with no radio in the minutes for 5 minutes,** as during a long DFS wait on a one-radio AP, restarts the sampler once. That is harmless.
- **Left for later:** a manager alert for an AP whose reports come and whose minutes don't, for an agent too old to watch, or a restart that doesn't help. Done in 0311.
