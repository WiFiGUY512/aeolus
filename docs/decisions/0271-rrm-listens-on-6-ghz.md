# 0271. RRM listens on 6 GHz

- Status: Accepted (Aeolus's own development, 2026-11-01; Griff: "how did this happen? everything landed on 5?")
- Date: 2026-11-01
- Refines: 0045 (RRM's channels on 2.4 GHz), 0087 (6 GHz channel sets)

## Context

- **On 2026-11-01 all three C-360s' 6 GHz radios were on channel 5,** at 80 MHz.
  - Each is set to an automatic channel, and a 6 GHz radio on one starts on 5.
  - Porting's -93 flashes (C360-AP 09:41 CST, ce7f 09:47) and 57ff's firmware test (B102, 09:49) each started one again.
  - 57ff and ce7f had been on 5 since 2026-10-31. C360-AP had been on 21.
- **RRM listened on no 6 GHz channel but the radio's own.** `visits` had lists for 2.4 and 5 GHz only. So each radio rated channel 5 alone, saw the other two there (`blotted_by`), and had nowhere rated to go: 5 stayed its best.
- **C360-AP reached 21 only from a rating it kept from an earlier spell on 21.** It moved there in the window ("shared", 2026-11-01 02:06 CST), and lost it when the flash started it again.

## Decision

- **On 6 GHz a radio listens on its set (0087), else on the preferred scanning channels,** 5 to 229 every 16th: those phones look on by themselves.
- **The set is each block's primary (0243).** At 80 MHz that is the preferred scanning channels, 5 to 213 on the C-360s. Each stands for its block, as on 2.4 GHz each channel is its own.
- **As on the other bands, its own channel is listened on wherever it is.**

## Consequences

- **A 6 GHz radio now rates the channels it may use,** from its own off-channel looks and the scan radio's sweep. A shared channel then has somewhere to go.
- **Moves for a shared channel stay in RRM's window** (02:00–05:00 at Home). On 2026-11-01 the three C-360s spread out in the next one.
- **A 6 GHz radio looks at up to 15 channels in turn, where it looked at one.** Each visit is the same short look off its channel that a 5 GHz radio makes over 25.
- **Checked:** `agent/test/rrm.uc` covers a set, the preferred scanning channels with no set, and the radio's own channel outside its set.
