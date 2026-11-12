# 0267. RRM with nowhere to go

- Status: Accepted (Aeolus's own development, 2026-11-01, at Griff's word to keep making fixes and features overnight)
- Date: 2026-11-01
- Refines: 0073 (radio resource management)
- Follows: 0266

## Context

- **For three days RRM moved no 5 GHz radio on any AP,** and nothing said so (0266).
  - Each AP's report rated every channel and marked none `best`.
  - "None best" is a state RRM can't be in where it works: a radio's own block is always one it may go to, so the best of them is marked even where the radio stays.
- **So "rated, and none best" means RRM finds no block it may use at all.** The set and width may leave it none, though the config check refuses most such sets (0075, 0120). Or RRM reads them wrong, as it did.

## Decision

- **A warning, `rrm-nowhere`, keyed by band,** is shown where an AP's RRM report rates four or more of a band's channels and marks none of them its best.
- **It says how many were rated, and that RRM moves the radio nowhere:** its channels and width may leave it no whole block, or RRM reads them wrong.
- **It ends when a channel on that band is marked best.**

## Consequences

- **A fault like 0266's is said within a report or two** of the radio being rated.
- **Fewer than four rated, as just after the daemon starts, is not said.**
- **Checked:** `TestRRMNowhere`. Live, 2026-11-01: with the manager on v0.213.0 and the agents not yet, it was said on all six APs' 5 GHz ("RRM rated 25 of its 5 GHz channels and finds none…"). Each ended once its agent was on v0.213.0, a few minutes later.
