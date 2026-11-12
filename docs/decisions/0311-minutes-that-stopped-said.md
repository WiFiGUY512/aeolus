# 0311. Minutes that stopped, said

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, as what 0307 left for later
- Refines: 0207 (telemetry by the minute), 0307 (the live daemon's minutes, watched), 0099 (alerts)

## Context

- **The charts, power control's history, client journeys (0303) and the struggling alert (0233) all read the radios' minutes** (0207), which aeolus-live sends each minute.
- **On 2026-11-04 the Living Room C-360 sent no minute for 13 hours** while its reports went on. Nothing said so: it was found by looking at a chart.
- **0307 has the agent start the sampler again** after 5 minutes without a minute, at most once every 30 minutes. That mends a sampler that hung. It doesn't help where:
  - the sampler starts again and stalls again;
  - its way to the manager is what fails;
  - the manager doesn't keep what comes;
  - the agent is older than 0307.
- **The sampler may also lose sight of one radio** while the others' minutes come, as when a radio comes back under another phy name (0307).

## Decision

- **A warning, `minutes-stopped`, when an AP's minutes don't come:**
  - **All its radios:** no minute for 10 minutes (`MinutesQuiet`, twice 0307's 5), since the end of the newest one kept. Key `minutes-stopped`.
  - **One radio that serves networks,** while the others' minutes come: no minute for 10 minutes. Key `minutes-stopped:<radio>`.
- **Only for an AP that:**
  - calls (one offline has its own alert);
  - has a state report within the last 15 minutes;
  - has been up 10 minutes, so a flash or a restart isn't one;
  - serves networks on some radio;
  - sent minutes before, so an AP whose agent never sent any isn't one.
- **A radio is quiet from the latest of these:**
  - its newest minute's end;
  - the AP's start;
  - when its current config version was made, for a radio that config may have just turned on;
  - 24 hours ago (`MinutesLooked`).
  
  A scan radio, or any radio with no networks on the air, is never one.
- **The manager looks up each AP's newest minutes when it works out the alerts** (`conditions.LastMinutes`):
  - the newest of any radio, by the table's key, at once;
  - each radio's newest over the last day.
  
  It stores nothing new.

## Consequences

- **A day like 2026-11-04's would have been a warning after about 10 minutes,** and sent two minutes later, as every alert is.
- **An AP whose minutes stopped more than 30 days ago** has none kept (`radioMinutesKeep`), and is then no longer said to be quiet.
- **A config change made after a radio's minutes stopped** starts that radio's quiet time again. The all-radios alert doesn't depend on the version.
