# 0239. A client test finds an AP that moved

- Status: Accepted; refined by 0240 (where its AP is set to be)
- Date: 2026-10-29
- Proposed by: Claude, from the night's client tests on the Living Room. Written up by Claude
- Refines: 0121 (the client test), 0225 (it roams)

## Context

- **A test on a named AP looks only where its scan radio last heard it.**
  - The agent scans the frequencies its last airscan heard that AP's BSSIDs on.
  - It falls back to the whole band only when none was heard.
- **So an AP that moved channel since is not found.**
  - On 2026-10-29 the Living Room moved 149 → 157 and back while its 5 GHz fault was chased.
  - Tests asked right after each move failed at "find", though the AP was up and serving. Airscan had last heard it on the old channel.
  - They passed once airscan had caught up, minutes later.
- **The manager already knows where the AP is now.** Each AP's state report carries its radios, with each one's band and channel.

## Decision

- **The manager hands the test the channels the AP named in `on` is on now,** from its last state report: each radio with a channel, on the band asked, or every band when none is asked, at most 8.
- **The agent scans those too,** with the frequencies its scan radio heard the AP on, each once. A channel it cannot place on its band is left out.
- **Nothing else changes:** with neither heard nor said, the whole band, as before. A test with no AP named, or roaming, is as it was.

## Consequences

- **A test right after a channel change finds the AP** where it now is, without waiting for airscan.
- **Since v0.235.0 a move is reported at once** (2026-11-03): the AP's last report is where it is now only if it reports after it moves. A move of RRM's was news only at the next report, five minutes on; the C260 moved from 44 to 157 in the second its report said 44, and a test asked then was sent to 44. Its latest move, as it is announced and as it ends, is now news that sends a report on the agent's next turn (`probe_news`).
- **An old agent ignores `channels`,** and behaves as before. An old manager sends none, and a new agent behaves as before.
- **Tested:**
  - **The agent:** `wanted` takes the channels; a target moved on 5 GHz is looked for on both frequencies, one not heard where it says it is; `freq_of` places 2.4, 5 and 6 GHz channels and refuses the rest.
  - **The manager:** an ask on 5 GHz carries only the 5 GHz radio's channel, and none with no channel.
