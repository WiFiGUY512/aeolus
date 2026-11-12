# 0266. A block by its primary, in RRM too

- Status: Accepted (Aeolus's own development, 2026-11-01, at Griff's word to keep making fixes and features overnight)
- Date: 2026-11-01
- Fixes: a fault of 0243's, in 0073's daemon
- Refines: 0075 (a radio's set of channels), 0120 (DFS channels after a look), 0243 (the primary is the lowest)

## Context

- **Since 0243 (2026-10-29),** the renderer writes a radio's `channels` as each whole block's primary at its width. At 40 MHz that is `36 44 52 … 149 157`, not `36 40 44 48 …`.
- **The port took that in its own check** (B121, port 0055), whose DFS-free start had wanted every channel of a block.
- **RRM's daemon did not.** It read `channels` as the channels the radio may use (`r.allowed`). It went to a block only where every channel of the block was in it. At 40 MHz or more on 5 GHz, no block had all its channels in it:
  - no planned move on 5 GHz: no `shared`, `better` or `start`;
  - no move to a DFS block after a look (0120), so look-first went nowhere;
  - no channel marked `best` in the report.
- **What it looked like, 2026-11-01 02:20 CST, on all six APs:**
  - every 5 GHz channel rated, none best;
  - Office and Living-Room-C-360 on 149 at 40 MHz, hearing each other at -58/-51 dBm;
  - the C260 and C360-AP on 44, at -46/-36;
  - none moved.
  - The last 5 GHz move on record was on 2026-10-27.

## Decision

- **RRM reads the set as the renderer writes it** (`rrm.allowed_of`): each listed channel stands for every channel of its block at the radio's width, as `rrm.covers` has them.
- **At 20 MHz, on 2.4 GHz, and on 6 GHz (which RRM rates channel by channel), a channel stands for itself,** as before.
- **The rest uses it unchanged:**
  - the blocks a radio may move to (`targets`);
  - the DFS blocks it may go to after a look (`far_targets`);
  - where it steps off a DFS channel to when it doesn't know (`rrm.fallback`);
  - the channels it visits.

## Consequences

- **5 GHz RRM moves again,** in the window and after a start, by its own rules, now with 0264's no going back.
- **Look-first (0120) can take a radio to a DFS block** that is clearly the best, with its minute's radar check.
- **The first night after the release,** the radios that share a channel with a neighbour they hear are likely to move. 0265 records what came of each move for its clients.
- **Live, v0.213.0 (02:35-02:39 CST, 2026-11-01):**
  - Every AP marked a best 5 GHz block again, and 0267's warning ended on each.
  - Within ten minutes the two pairs that shared a channel split: the C260 went 44 to 36, and Office went 149 to DFS 52 (0120).
- **Checked:** `agent/test/rrm.uc` covers the set at 40, 80 and 20 MHz, on 2.4 and 6 GHz, and none. At 40 MHz, the blocks a radio may go to from that set are 36, 44, 149 and 157, and the best is chosen. The daemon compiles on Shed's 25.12 ucode.
