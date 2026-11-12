# 0236. Resends are not struggling

- Status: Withdrawn by 0242 (slow is said): the resends it took out were the one sign of the Living Room AP's radio fault (B101), not noise
- Date: 2026-10-28
- Proposed by: Claude, from the struggling alert's first day on a phone. Written up by Claude
- Refines: 0233 (a client that struggles)

## Context

- **0233 counted a client as struggling where 30% or more of the frames sent to it, over its busy minutes, were sent again.** That was a first guess, to be changed after seeing what it raised.
- **On its first day it raised only that, on a phone, for hours,** after the phone's real trouble (channel 169, 0232) was gone.
  - The phone was on the Living Room AP's 5 GHz on channel 48, at 80 Mbit/s, heard at -70 to -75 dBm: further from that AP than in the morning.
  - No other AP had heard it, so there was nowhere better to send it.
- **The phone's minutes over five days show resends do not tell its good times from its bad:**
  - 25 to 37% sent again on the Living Room AP's 6 GHz at 1.2 to 1.5 Gbit/s, its best link in the house.
  - 34% on C360-AP's 2.4 GHz at 181 Mbit/s.
  - Where it heard the AP well (-65 dBm or better), 19% on the Living Room's 5 GHz on channel 169, its worst link, against 24% on that AP's 6 GHz.
  - a phone, the same model, had 23% on the Living Room's 5 GHz.
  - Another client on channel 48 at the same time had 2 to 3%. A phone's power saving and its many small frames make its share high on any link.
- **What told channel 169 apart was the rate:**
  - On the Living Room's 5 GHz on channel 169, where the phone heard the AP well: 105 Mbit/s.
  - On C360-AP's 5 GHz: 272 Mbit/s.
  - That is 0233's slow-for-its-signal reason, which stays.

## Decision

- **Frames sent again are not a reason a client struggles.** The alert has two reasons left, as 0233 has them:
  - It moved between one AP's bands four times or more.
  - On 5 or 6 GHz it ran under 120 Mbit/s, heard at -65 dBm or better, over ten busy minutes.
- **A client's journey (0103) still says many retries,** where a person looks at one client and weighs them beside its signal and rate.

## Consequences

- **A phone that runs well is not an alert for its resends,** and the struggling alert names only band flips and a slow link the client heard well.
- **A client far from every AP is not said to struggle.** That is a weak signal, which its journey says. A closer AP, where there is one, is steering's to find (0138).
- **a phone's alert ends with this release,** since alerts are worked out on each call.
