# 0178. Channel ratings as blocks at the radio's width

- Status: Accepted (Griff, 2026-10-20: "do all those except going public")
- Date: 2026-10-20
- Proposed by: Griff, issue #45: "the APs should aggregate their channel rating into their own channel widths, in this case it should be 40s; we might even try the colour channel blocks similar to the channel selection under radios, with a green to red range for good to bad channels". Written up by Claude
- Refines: 0073, 0075

## Context

- **The Ratings tab listed every 20 MHz channel,** while the radios run 40 MHz. A channel can't be judged alone at 40 MHz: 48 is only usable with 44.
- **RRM already judges blocks** (`rrm.uc` `blocks`): on 5 GHz at 40 MHz or more, each block whose channels are all rated, as good as its worst channel, used where any of its channels is. The page didn't show that.
- **Reading 25 rows per AP** to see where a radio could go was slow.

## Decision

- **Each AP's ratings on a band are drawn as RRM judges them, at its radio's own width:**
  - on 5 GHz at 40, 80 or 160 MHz, a box a block (36–40, 44–48 …), rated as its worst channel; a block with a channel not yet rated is left out, as RRM leaves it out;
  - on 2.4 GHz, at 20 MHz, and on 6 GHz, a box a channel, as RRM judges them there.
- **Colour:** green for a clear channel, through yellow, to red at a rating of 80 and over. The scale is absolute, not relative to the AP: RRM's rating is 100 for a channel others keep busy all the time, so the same colour means the same thing on every AP.
- **Marks:** the block in use is ringed; the one that rates best, where the radio would move, has a ★; one a neighbour uses is dimmed; a DFS one is hatched, as on the channel map. Hovering gives the numbers: the rating and now, the worst channel's busy share, noise and networks, and who uses it.
- **Every 20 MHz channel's table** stays one click away, under the blocks.
- **RRM's decisions don't change.** This is how the page shows them.

## Consequences

- An AP's 5 GHz options read at a glance, in the radio's own terms.
- 6 GHz stays per channel, as RRM judges it; should RRM judge 6 GHz by blocks, the page follows it.

## As built (2026-10-20)

- `views/neighbours.js`: `judged` mirrors `rrm.uc` `blocks` with RRM's own 5 GHz block table; `hue` is the colour; `blocksRow` draws them above each AP's table, which moves into an "Each 20 MHz channel" fold.
- Checked against a local manager with an AP reporting ratings: at 40 MHz on 5 GHz, twelve blocks, the one in use ringed, the best starred, the colours by rating; on 2.4 GHz three channels. The block logic was checked by itself under node: a block as good as its worst channel, a block with an unrated channel left out.
