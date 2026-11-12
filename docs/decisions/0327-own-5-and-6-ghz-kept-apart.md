# 0327. An AP's own 5 GHz and 6 GHz kept apart

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Griff, of the Living Room C-360's 5 GHz after port -126: "probably want to figure out what's wrong with it", then "so minimum channel separation rule?". Found with the OpenWrt porting session. Written up by Claude
- Refines: 0073 (radio resource management), 0075 (an automatic channel's set), 0087 (6 GHz channels), 0298 (6 GHz blocks)

## Context

- **After port -126, no client joined the Living Room C-360's 5 or 6 GHz.**
  - Before the flash its 5 GHz had 9 clients. The TV, steered there by its rule of the nearest, refused the move (BTM status 7, no suitable candidates) and went back to the C260.
  - The radio's survey showed 3 ms received in 330 s, though its beacons went out and neighbours heard them.
- **An Aeolus client test from C360-AP onto it, 5 GHz ch157/40, signal -65, linked at HE-MCS 0, 17.2 Mbit/s,** and moved 32 Mbit/s up. On -124 the same test, at -63 to -68, ran HE-MCS 6, 309.7 Mbit/s, and 168–178 Mbit/s up.
- **Its 6 GHz had come up on ch5/160 (5945–6105 MHz),** 130 MHz above its 5 GHz (5775–5815). hostapd picked that channel at start. Port -125/-126's 0097 (B11) had lifted the C-360's 6 GHz LPI limit on transmit chains, about +5 dB at clients.
- **The A/B, with porting, on the Living Room:**

  | Living Room 6 GHz | 5 GHz link (C360-AP onto ch157) |
  | --- | --- |
  | off | -68 dBm, HE-MCS 6, 309.7 Mbit/s |
  | ch5/160, the limit back on (old power) | -72 dBm, HE-MCS 3, 137.6 Mbit/s, 60 up |
  | ch5/160, limit lifted (-126) | -65 dBm, HE-MCS 0, 17.2 Mbit/s, 32 up |
  | ch165/160 (6755–6915), limit lifted | -65 dBm, HE-MCS 6, 309.7 Mbit/s |

  - The 6 GHz radio, sending, deafens its own AP's 5 GHz receiver when they are close: a little at the old power, nearly wholly at the new.
- **The Garage C-360, on -126 too, was fine:** 5 GHz on 36, 6 GHz far above, and HE-MCS 7, 344 Mbit/s, onto its 5 GHz.

## Decision

- **`rrm.apart`: how far apart, in MHz edge to edge, each AP keeps its own 5 GHz and 6 GHz.** 250 unless set; 0 is no rule.
  - Porting agrees 250 for the C-360 with B11. It keeps ch5/160, and ch7 and ch23 at 80, off while 5 GHz may be in U-NII-3, and allows ch37 and up.
  - It is a Locations setting, so an AP template can set it per board.
- **At start, by the config:** an automatic 6 GHz radio's channel list drops every channel whose block's lower edge is under the top of what its own 5 GHz may use, plus `apart`.
  - "What its own 5 GHz may use" is the block of its set channel, or the highest block of its automatic list.
  - hostapd picks among what is left. If none is left, the list stays as it was.
  - A 6 GHz radio with no channel list (any channel) is left to the next point.
  - The config check holds the same.
- **By radio resource management:**
  - Neither radio moves to a block nearer than `apart` to the other.
  - A 6 GHz radio found too near, as after a start, moves at once to the best block that keeps apart, in or out of the window, with the move's reason `own`. The 5 GHz radio is not moved for it.

## Consequences

- **The C-360s' 6 GHz lists lose ch5/160** while their 5 GHz may use U-NII-3: 37, 69, 101, 133, 165 and 197 are left.
- **Other boards keep the 250 MHz until an AP template sets theirs.** That costs at most the lowest 6 GHz block while 5 GHz is high.
- **The manager takes `own` as a move's reason; the Neighbours page says it as "too near its own 5 GHz".**
- **Checked:**
  - agent/test/rrm.uc with CI's ucode: the edges of 157/40, 5/160 and 37/160. 5/160 is too near 157/40 at 250, and so is 157/40 to 5/160. 37/160 is not, nor 36/40 to 5/160. Never at 0, at 100, on one band, or on 2.4 GHz.
  - The render case six-ghz-psc: 5 GHz up to 149/80 (5815), so 6 GHz at 160 MHz loses 5 and keeps 37 to 197. The check holds it (internal/rendercheck's agent cases).
  - internal/radio: Edges.
  - aeolus-rrm compiles with that ucode.
