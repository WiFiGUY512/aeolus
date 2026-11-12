# 0278. RRM moves on 6 GHz

- Status: Accepted (Aeolus's own development, 2026-11-01)
- Date: 2026-11-01
- Refines: 0271 (RRM listens on 6 GHz), 0117 (a move's arguments)

## Context

- **0271 had RRM rate the 6 GHz channels,** and it chose some: 133 for 57ff, 181 for C360-AP and ce7f.
- **It could not move a 6 GHz radio there.** switch_args, which builds hostapd's switch_chan arguments, knew 2.4 and 5 GHz only. For 6 GHz it gave nothing, so each planned move failed as "its width does not fit there". 57ff's start move, 5 to 133, after its -95 boot, 2026-11-01 15:01 CST, was the first seen.
- **6 GHz blocks are regular:** from channel 1, a block's width apart (40 MHz: 1–5, 9–13…; 80: 1–13, 17–29…; 160: 1–29, 33–61…). Unlike 5 GHz, they need no table.
- **A 6 GHz radio runs HE, or EHT, with no HT or VHT.**
- **OpenWrt's ubus switch_chan can't make a 6 GHz move of 40 MHz or wider yet** (B129, fault 66). It hands nl80211 the 40 MHz center for any wider width, and the kernel refuses the channel. The port fixes it in 0070, build -96. The control socket's CHAN_SWITCH, given the same center, does it: 57ff's 6 GHz went from 5 to 133 at 80 MHz that way, by hand, 15:09 CST.

## Decision

- **switch_args takes 6 GHz:** the block by `blocks6` (40, 80 and 160 MHz; 320 MHz, whose blocks overlap, is not taken), the center of its block, the secondary's side as on 5 GHz, and HE (EHT where the radio runs it) with no HT or VHT.
- **Moves go by switch_chan as on the other bands.** No workaround for B129: Griff's word, 2026-11-01, "Wait for porting's fix". Until an AP runs a build with it, a 6 GHz move there fails as hostapd refuses it, and RRM logs it and tries no other way.

## Consequences

- **Once the APs run -96, RRM spreads the C-360s' 6 GHz radios** off a shared channel, in its window or at a start.
- **Until then a 6 GHz move fails** with hostapd's own answer, "Operation not supported", not "its width does not fit there".
- **Checked:**
  - RRM's test has 6 GHz cases at 20, 80, 160 and 320 MHz, and the control command for 133 at 80;
  - the arguments for 133 at 80 MHz (6615, center 6625) are what hostapd's CHAN_SWITCH took on 57ff.
