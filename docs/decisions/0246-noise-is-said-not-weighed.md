# 0246. Noise is said, not weighed

- Status: Accepted
- Date: 2026-10-29
- Proposed by: the OpenWRT porting session, from the port's facts on noise floors; built by Claude
- Refines: 0073 (radio resource management), 0116 (it listens through the scan radio), 0245 (RRM hears the spectrum)

## Context

- **A channel's cost added 2 for each dB of noise above -95 dBm** (`rrm.cost`), where the rating had a noise floor.
- **The readings a radio gives are not alike enough to weigh channels by:**
  - **On an AP that listens through its scan radio, only the channel in use has one.** The QCN9074 family's firmware (the C-360's scan radio and its 6 GHz radio) puts a fixed -107 dBm in every off-channel dwell, and the port leaves it out (port patch 0035). The channel in use alone paid for its noise: 8 to 14 points against 0, most of the 20 points' gain a move needs (`rrm.MARGIN`). Moves for small gains came cheap.
  - **Where both are read, they differ by kind.** The porting session measured one channel at one moment, on the bench unit's 2.4 GHz: in use -94, its own 150 ms dwell -86 to -87. The in-use figure is long-filtered across the operating width; a dwell is a 20 MHz instant. Mixed, the channel in use looks some 8 dB quieter than any other, and the radio stays put where it should not.
- **Energy that is no network counts as busy through the scan radio's spectral sweep (0245),** which is what met the attic AP.

## Decision

- **Noise counts for nothing in a channel's cost,** on every AP.
- **It is still reported** in each rating where the radio gives one, and the ratings window shows it.

## Consequences

- **A radio moves for busy time, the spectral sweep's energy, and the networks heard,** each read alike on every channel.
- **A channel noisier than the rest is not avoided for its noise alone,** on an AP without a scan radio to sweep it. Its busy time still counts.
- **Untested by a unit test:** the daemon's fold has none. Checked by compiling the agent on Shed, and live: each rating's cost against its busy after the release.
