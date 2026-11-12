# 0245. RRM hears the spectrum

- Status: Accepted (Griff, 2026-10-29: "yes absolutly do 0245"; and "add to the channel blocks in radios>ratings a window pop when you click on a channel whayt all the stats of that channel are, aka why it picketd the rating")
- Date: 2026-10-29
- Proposed by: Claude, from the attic AP's 5 GHz (B109). Written up by Claude
- Refines: 0073 (radio resource management), 0116 (it listens through the scan radio)

## Context

- **The Living Room AP's 5 GHz heard its clients 8 to 10 dB worse than they heard it,** on 149/153 in the attic.
  - ce7f, a good unit, put in the same spot, did the same on 149.
  - On 60 it heard the same test client 6 dB better than the client heard it.
  - The spot is the cause, not the unit: on 149 ce7f's noise floor reads -88 dBm, against -91 lower down.
- **ce7f's scan radio's spectral sweep shows why.** Share of the time a channel held energy at or above -82 dBm, the preamble level (airscan's `busy_pct`):

| Channels | In the attic | At C360-AP |
|---|---|---|
| 36–64 | 2–6% | 4–8% |
| 112–136 | 40–73% | 3–8% |
| 140–161 | 15–35% | 0.5–8% |

  - It is no network: the survey's busy time on 149–161 was 0 to 5%. An outdoor link or a video sender near the attic is the likeliest.
- **RRM could not see it, and picked it.**
  - Its ratings take how busy a channel was from the scan radio's survey, which counts what the radio's carrier sense counts.
  - It takes a noise floor where the driver gives one, which the scan radio gives for none. Every channel but the one in use had no noise in its rating.
  - So 149–161 looked like the quietest channels. ce7f's start move went to 153 at 15:31Z, and it rated 157 best after.

## Decision

- **A channel's busy share is the busier of the survey's and the scan radio's last spectral sweep's,** a 20 MHz window at a time (`rrm.energy_of`).
- **What counts as a sweep:**
  - One from the last 40 minutes. airscan sweeps every 15.
  - Windows of 20 MHz, on the band, with a share from 0 to 100%. Others are left out.
- **Not on the radio's own channels,** where the sweep hears the AP's own sending.
- **The cost is as before** (`rrm.cost`): busy counts most, 100 for a channel kept busy all the time.
- **The AP reports the sweep's share as `energy`** in each rating that has one, in percent, beside `busy`. The manager takes it and refuses one outside 0 to 100.
- **Radios › Ratings:** a block, clicked, opens a window with:
  - why it rates as it does: its worst channel, in use, best, used by a neighbour, DFS;
  - each of its 20 MHz channels' numbers: rating, now, busy, spectrum, noise, networks, visits, last visit;
  - what each part of a visit's cost came to, and how the cost is made.
  - The per-channel table, and each block's hover, say the spectrum too.

## Consequences

- **The attic AP rates 112–161 as busy as the sweep finds them,** and moves to the quiet low channels by itself.
- **The same holds for every AP with a scan radio that sweeps** (the C-360s, the C-260). An AP without one rates as before.
- **The sweep counts neighbours' and Aeolus's own other APs' networks as well.** The survey counts those too, so the busier of the two is no double count.
- **A ratings page shows why a channel rates as it does,** where before it showed only a number.
- **Tested:**
  - **`rrm.energy_of`** (agent/test/rrm.uc, run on the C-260's ucode): a fresh sweep by channel; 40 MHz windows, another band's and out-of-range shares left out; a sweep too old, from ahead, or none gives none.
  - **The manager** takes a rating with `energy` and refuses one of 101.
