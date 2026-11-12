# 0308. A slow way is not a slow radio

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from the Living Room's 2.4 GHz slow-radio alerts of 2026-11-04
- Refines: 0242 (a radio that runs slowly)

## Context

- **The slow-radio alert (0242) blames a radio** where its last three client tests that measured speed each got under a third of their tester's usual from it, by any of its testers.
- **On 2026-11-04 it said the Living Room's 2.4 GHz "receives slowly", twice** (11:49-13:32 and from 23:12 CST). The facts:
  - **The C260's slow link:** since 11-04 00:14, every test the C260's scan radio made of the Living Room's 2.4 GHz uploaded 0.2-0.8 Mbit/s, where 11-14 was usual. It heard the Living Room at -68 to -70 dBm, and the C260's ath10k reads 8-12 dB high. Its uplink rate was MCS 0-4, while the Living Room's downlink to it was MCS 12-13.
  - **The Garage got its usual from the same radio:** 53-74 Mbit/s up at -58 to -61 dBm, through the same hours.
  - **The radio's own clients were fine:** the Living Room received from them at 55-67 Mbit/s, the same before and after, at 7-15% sent again.
  - **The C260 got its usual from every other AP's 2.4 GHz.**
- **So it was the C260's way to the Living Room, not the Living Room's radio.** Its last three tests were all the C260's, so the alert took them for the radio's.

## Decision

- **A run of slow tests says the radio is slow only where no other tester got its usual from it since the run's testers' tests began to be slow.**
  - A tester's slowness began at the oldest of its unbroken slow tests on the radio's channel, the newest first.
  - Another tester's latest test that is not slow, and is newer than that, says the radio is well. The run is then that tester's way, and there is no alert.
  - Another tester's latest from before, or slow too, leaves the alert standing, as before.
- **Unchanged:** the run of three, the usual (the upper quartile of a tester's tests at a like signal, 7 days), the third, and the 6-hour freshness.

- **Where the run is one tester's tests alone, the alert says so** (added the same night): "All are C260's tests, so it may be C260's way to it and not the radio: a test from another AP would tell". That case is a radio on a channel no other tester has tried yet. It came up at once on the Living Room's 5 GHz, moved to channel 60, while its own clients reached it at about 145 Mbit/s as before.

## Consequences

- **The Living Room's 2.4 GHz alert of 2026-11-04 would not have been raised:** the Garage got its usual from it at 17:00, within the C260's slow run that began at 00:14.
- **A radio gone slow for every client still alerts,** since every tester's tests since then are slow.
- **A radio whose fault hurts only far clients** (a weaker receiver, a lost chain) is no longer alerted while a near tester still gets its usual. Its own clients' rates (0207) show that. The C260 to Living Room link is left to look into apart: the C260's scan radio, as a client at range.
