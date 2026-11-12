# 0243. The primary is the lowest

- Status: Accepted (Griff, 2026-10-29: "on channel selection we should always use the lower for the primary, that way its consistent everywhere", "except 6g, where we should always use the second at 80")
- Date: 2026-10-29
- Proposed by: Griff. Written up by Claude
- Refines: 0075 (channel sets), 0087 (6 GHz channels), 0073 (radio resource management)

## Context

- **At 40 MHz or more a radio uses a block of 20 MHz channels, and beacons on one of them, its primary.**
  - Beacons, acknowledgements and anything sent 20 MHz wide go on the primary.
  - The rest of the block is used only for wide data.
- **A channel set does not say which of the block is the primary.** OpenWrt hands hostapd the channel as set: 153 at 40 MHz is primary 153 with 149 below it (HT40-).
- **hostapd can change it as it starts.** Its scan for networks a 40 MHz channel overlaps swaps the primary and the secondary where another network's primary is on its secondary.
  - On 2026-10-29 the Living Room AP, set to 149 at 40 MHz, came up on primary 153 after its reflash.
  - Its next channel switch flipped both its block and its primary.
  - The C-260, set to 153, came up on 149, since the Living Room's primary was on its secondary.
- **So the same setting gave different primaries,** by AP and by start. An automatic channel was the same: hostapd's pick, and RRM's, could be either half of a block.
- **6 GHz clients look for networks by themselves only on the preferred scanning channels** (5, 21, 37, every 16th). Each is the second 20 MHz of its 80 MHz block.

## Decision

- **On 5 GHz the primary is the block's lowest channel.** A channel set at 40 MHz or more names its block, and the radio beacons on its lowest: 153 at 40 MHz is 149, and 161 at 80 MHz is 149.
- **On 6 GHz the primary is the block's second channel.**
  - At 80 MHz that is the block's preferred scanning channel: 5, 21, 37 and so on.
  - At 160 MHz it is the lower 80's: 21 at 160 MHz is 5.
  - At 40 MHz it is the block's upper half.
- **An automatic channel lists only each whole block's primary,** so hostapd's pick and RRM's moves land on it. RRM enters a block on its lowest channel, where before it entered on its best-rated.
- **Every 5 GHz radio has hostapd's overlap scan off** (`noscan`), so it keeps the primary it is given.
- **2.4 GHz is as before.** At 20 MHz it has no primary to choose. At 40 MHz OpenWrt joins 1 to 6 with the channel above and 7 up with the one below (0117).
- **The manager's render check expects the same:** the primary, and `noscan` on 5 GHz.

## Consequences

- **The same channel and width give the same primary on every AP, every start.**
- **Taking it restarts each AP's 5 GHz radio once:**
  - every AP, for `noscan`;
  - an automatic 5 GHz radio also for its shorter list.
  - A 6 GHz radio at 160 MHz with preferred scanning channels restarts too, where its list loses the upper 80's, such as 21.
  - Their clients drop and come back. 2.4 GHz is not touched.
- **An Aeolus AP no longer moves its primary to share a neighbour's.** Two 40 MHz networks on one block with their primaries apart still work; each defers to the other's traffic on both halves. Where a neighbour is a problem, RRM moves the radio to another block.
- **A channel set to a block's upper half is shown as set, and runs on the lowest.** The AP's report says the primary.
- **Tested:**
  - **`radio.Primary` and `Usable`:** 5 GHz at 20, 40, 80 and 160 MHz; 6 GHz at 40, 80, 160 and 320 MHz, the overlapping 320 MHz families among them.
  - **Every agent render case,** each checked by the render check: the lists hold only primaries, and 5 GHz has `noscan`.
  - **A new case, primary-channel:** 5 GHz set to 153 at 40 MHz renders 149, and 6 GHz set to 21 at 160 MHz renders 5.
  - **RRM:** a block rated better on 153 is entered on 149. Checked on the C-260's own ucode, which has `digest`.
- **Tried by hand on the C-260 before:** with `noscan` set and 153 at 40 MHz, hostapd kept primary 153, where without it it swapped to 149.
