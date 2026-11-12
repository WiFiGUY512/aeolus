# 0247. Each channel's floor

- Status: Accepted (Griff, 2026-10-29: "put the noise floor of each channel in the rating please", and of the ratings window, "this is too much")
- Date: 2026-10-29
- Proposed by: Griff. Written up by Claude
- Refines: 0245 (RRM hears the spectrum), 0246 (noise is said, not weighed)

## Context

- **Every channel but the one in use said "no noise floor measured".**
  - The C-360's scan radio's firmware gives a fixed -107 dBm in every off-channel dwell, and the port leaves it out (port patch 0035).
  - So the only floor read was the serving radio's own, on its own channel.
- **The scan radio's spectral sweep gives a noise level for each 20 MHz window,** the middle of its samples'.
  - It is on the scan radio's own scale, uncalibrated.
  - Its differences between channels are real. In ce7f's attic, 149 and 153 were 1 and 3 dB above the quiet channels, as the serving radio read them in use: -88 on 149, against -90 to -91 lower down.
- **The ratings window (0245) said too much,** and still said noise counted in a rating, which 0246 had ended.

## Decision

- **A channel's floor is the sweep's level, set against the noise floor the radio measures on its own channel:**
  - shift = the radio's own reading, less the sweep's level on that channel;
  - each channel's floor = its sweep level + shift.
  - With no reading of its own, or no sweep of the last 40 minutes, no floor.
- **The AP reports it as `floor`,** in dBm, in each rating that has one. The manager takes it, and refuses one outside -127 to 0.
- **The floor counts for nothing in the rating** (0246). It is there to be seen.
- **Radios › Ratings:**
  - each block shows its floor, the highest of its channels', under its rating;
  - the per-channel table's Noise column is Floor;
  - the window a block opens is one line of why (rated, worst channel, in use, best, used by, DFS), one table, and one line saying what the rating is made of.

## Consequences

- **Every channel a scan radio sweeps has a floor,** and a raised one shows where an interferer sits below the preamble level.
- **The floor is as good as the radio's own reading on its channel:**
  - It is taken across the operating width over time, where a sweep window is 20 MHz for a moment, so all of a band's floors may sit a few dB off.
  - Their differences hold.
  - On an ath11k radio the own reading is real; signals are what B110 skews, not the floor.
- **Tested:**
  - **`rrm.floor_of`** (agent/test/rrm.uc, run on the C-260's ucode): 20 MHz windows' levels by channel, others and out-of-range levels left out, a stale sweep none. The refactor through `windows_of` leaves `energy_of`'s results as they were.
  - **The manager** takes a rating with a floor and refuses one of 3.
