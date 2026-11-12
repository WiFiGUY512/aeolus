# 0264. No going back within a day

- Status: Accepted (Aeolus's own development, 2026-11-01, at Griff's word to keep making fixes and features overnight)
- Date: 2026-11-01
- Refines: 0073 (radio resource management)

## Context

- **RRM's planned moves (0073):**
  - `shared`: a neighbour uses the radio's channel, and the best is free, or its nearest user is 6 dB further away.
  - `better`: the best rates better by the margin, and is no more shared.
  - Each waits ten minutes for its reason, and goes only in the window. A radio stays put 15 minutes after any move.
- **Where neighbours use every channel,** as six APs do 2.4 GHz's three, each move changes what the others see. A radio can then go back and forth.
- **Living-Room-C-360's 2.4 GHz on 2026-10-31,** in its window (by UTC; see 0262):
  - 21:05 CDT: 6 to 1, `shared`. ce7f, on 1, then moved 1 to 6, `shared`, at 21:16.
  - 21:43: 1 to 6, `better`.
  - 22:46: 6 to 1, `shared`.
  - 23:21: 1 to 6, `better`.
- **Shed's did the same with 6 and 11** on 2026-10-31 (02:28 `better`, 03:02 `shared`), and on other nights.
- **Each move is a channel switch.** Clients follow it, but 2.4 GHz is where the IoT clients are, and not all of them follow well.
- **A DFS block a radio left is already left alone for a day** (0120). Nothing like it holds for other channels.

## Decision

- **A planned move, `shared` or `better`, does not go back to a block the radio left within a day** (`rrm.BACK_FOR`, 86400 s). It goes to the best of the others where that is worth a move by the same rules, or nowhere.
- **"Left" is a move the AP made from one of the block's channels,** `moved` or `checking`, as its moves say (`rrm.left_lately`). A move that was claimed and withdrawn left nothing. The moves are kept across the daemon's restarts already, so no new state is needed.
- **Not held back:**
  - a move for `interference`, which must leave at once;
  - a move just after the radio starts (`start`);
  - a radio stepping off a DFS channel to where it came from (0120).
- **The rule is a test function that `rrm.wanted` takes,** so another rule about where a move may go can be a different test.

## Consequences

- **A radio settles where it went for a day,** unless interference moves it or its radio restarts.
- **A band whose channels are all shared moves less.** A radio may stay on a shared channel that a move back would have improved by 6 dB. The next night it may go.
- **Checked:** `agent/test/rrm.uc` covers back where it left, for `better` and `shared`, with another worth going to, a channel not left, `interference`, `start`, and no rule. The daemon compiles on Shed's 25.12 ucode, and the module ran under CI's.
- **Not checked live yet.**
