# 0298. A 6 GHz radio takes its whole block

- Status: Accepted (Griff, 2026-11-03, about 18:45 CST: "do all of that")
- Date: 2026-11-03
- Proposed by: Claude, from porting's B108 and the Phone-B client matrix (finding F16). Written up by Claude
- Refines: 0073 (radio resource management), 0243 (the primary), 0266 (a channel stands for its block), 0271 (6 GHz visits), 0278 (6 GHz moves)

## Context

- **On 5 GHz radio resource management knows a wide radio takes up a block** of 20 MHz channels: a neighbour on 36 at 80 MHz uses 36 to 48, and a radio rates, and moves to, whole blocks.
- **On 6 GHz it knew only the primary.**
  - A neighbour on 5 at 160 MHz blotted out channel 5 alone.
  - So 21 at 80 MHz, inside the same 160 MHz, looked free, and a radio could move there.
- **Overlapping 6 GHz is costly.** Porting's B108: while the C360-AP and the Garage shared 37 at 160 MHz, the Phone-B's 6 GHz frames were retried 24–49% of the time. With each AP on its own 80 MHz block (change 554), busy minutes ran under 12%.
- **A 6 GHz radio listens on few channels,** the preferred scanning channels of its set (0271), one in each 80 MHz. So most of a block's channels are never rated.

## Decision

- **A 6 GHz radio at 40 MHz or more takes up its whole block,** as on 5 GHz. The blocks are 0278's: from channel 1, a block's width apart.
  - Its own block's channels are its own.
  - A neighbour's blot out every channel of its block, and so does where it claims to move.
- **A block is blotted out by whoever uses any of its channels, rated or not.** The radio's planner is given every neighbour's channels, not only those of the channels it rated.
- **On 6 GHz a block is rated by those of its channels the radio listens on, at least one.** On 5 GHz every channel of a block must still be rated.
- **On 6 GHz a radio listens on each of its blocks' primaries and on the preferred scanning channels in its set.** So at 160 MHz it hears both 80 MHz halves of each block: 5 and 21, 37 and 53.
- **A block is still entered on its primary (0243):** on 6 GHz its second channel, at 80 and 160 MHz a preferred scanning channel.

## Consequences

- **Two Aeolus APs that hear each other are never put on overlapping 6 GHz channels** by radio resource management, whatever their widths, while there are free blocks: seven at 160 MHz in the US, fourteen at 80.
- **A 160 MHz radio listens on twice the channels it did,** one in each 80 MHz.
- **A neighbour at 20 or 40 MHz on a channel no one listens on is still seen,** by its own hellos.
- **A set channel is left alone, as before:** a pinned radio is never moved.
- **The Phone-B matrix's F6 was not a fault.** Its "21 at 160 MHz runs on 5" is 0243: 6 GHz beacons on its block's second channel.
