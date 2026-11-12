# 0309. Where a struggling client did better

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from the Living Room TV's struggling alert of 2026-11-04
- Refines: 0233 (a client that struggles), 0306 (the AP it struggles on)

## Context

- **The struggling alert said the Living Room TV struggled on the C260** from 2026-11-04 14:16 CST: 50-78% of the frames sent to it sent again.
- **Its journey said why:**
  - Until 08:13 CST it was on the Living Room AP's 5 GHz, heard at -60 dBm, 4% sent again, about 120 Mbit/s.
  - At 08:13 the Living Room AP restarted with its -118 flash. The TV went to the C260, heard at -74 dBm, 65% sent again, about 26 Mbit/s, and stayed there.
  - It was stuck, not struggling where it had to. Sticky steering (0289) is what moves such a client, and it is off on Home_Wi-Fi_IoT.
- **The alert said none of that.** Finding it took the client's journey, read by hand.

## Decision

- **A struggling client's alert says where it did better in the last two days, where it did:**
  - another AP, on the band it was busy on most where it struggles;
  - whose busy minutes (a megabyte or more, 0303) heard it 6 dB or more stronger in the middle;
  - and sent at most half as much of what it sent it again;
  - over 10 busy minutes or more. The strongest heard of those is named.
- **It reads:** "In the last two days it did better on Living-Room-C-360 (5 GHz): heard at -60 dBm, 4% sent again over 388 busy minutes. It may be stuck here: sticky steering on its network (0289) would move it."
- **Nothing is moved.** Turning sticky on for a network, or putting a client off, is a person's call, as it was.

## Consequences

- **The manager reads the client's minutes over 48 hours,** only for a client that struggles. Client minutes are kept 7 days.
- **Signals are compared only within a band:** 2.4 GHz is heard more strongly than 5 GHz from the same place, and sticky moves a client on its band.
- **A client that never did better elsewhere** gets the alert as before, with nothing added.
