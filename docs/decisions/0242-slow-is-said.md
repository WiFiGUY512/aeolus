# 0242. Slow is said

- Status: Accepted (Griff, 2026-10-29: "do 0242 AND bring back whatever you shut down that you thought was false noise but was real")
- Date: 2026-10-29
- Proposed by: Claude, from the Living Room AP's 5 GHz fault (B101). Written up by Claude
- Refines: 0233 (a client that struggles), 0123 (client tests that fail); withdraws 0236 (resends are not struggling)
- Refined by: 0250, which moves the rate bar 5 dB higher once ath11k's readings were put right (B110)

## Context

- **B101:** on the ath11k driver, a channel switch left the radio sending at MCS 0–3 until it was restarted. On the Living Room AP that was 2 to 15 Mbit/s down, where 76 to 136 was usual.
- **Only one alert ever said it, and Aeolus then took that alert out.**
  - On 2026-10-28, 15:11 to 17:52Z, the struggling alert (0233) said of a phone on the Living Room AP: "71% of the frames sent to it were sent again".
  - 0236 took resends out as a reason that day. It held her trouble to be over and the alert to be noise. It was not over: her phone was on that AP's 5 GHz after RRM had moved it, which is the fault.
- **Overnight nothing said it.**
  - Her phone heard the AP at -70 to -76 dBm. The struggling alert's rate rule judges only a client heard at -65 or better.
  - The APs' own client tests got 7 Mbit/s down, but a test is an alert only when it fails, never when it is slow.
- **What the week's data shows:**
  - Named clients on the Living Room's faulty 5 GHz ran at 32 Mbit/s, heard at -66 to -70 dBm, the middle of their busy minutes.
  - On ce7f's healthy 5 GHz, clients heard at -81 to -85 ran at 34.
  - ce7f's tests of the Living Room got 2 to 15 Mbit/s down after a switch, and 76 to 136 fresh.
- **Scheduled client tests measured no speed:** `assurance.tests.speed` was unset at Home.

## Decision

### Resends are a reason again

- **The struggling alert counts resends as 0233 had it:** 30% or more of the frames sent to the client, over ten busy minutes or more.
- **0236 is withdrawn.**
  - Its own data had healthy phones at 19 to 37% on good links. So the threshold may raise a healthy phone; it is 0233's first guess, to be judged by what it raises.
  - The 71% the fault gave is far above it.

### The rate's bar slides with the signal

- **A client heard at -65 dBm or better is slow under 120 Mbit/s,** as before.
- **Heard less well, the bar falls 6.5 Mbit/s a dB:**
  - 87.5 at -70;
  - 55 at -75;
  - 22.5 at -80.
- **Heard less well than -80 dBm, no bar is set:** such a client runs slowly on any AP.
- **The alert says the bar:** "on 5 GHz it ran at 32 Mbit/s, heard at -70 dBm, where under 88 is slow".

### A radio that runs slowly: `slow-radio`

- **A client test through the air that measures speed is a client at a place that does not move,** so what each tester gets from each AP's radio has a usual.
- **The usual:** of the tester's tests of that radio over seven days that heard it within 6 dB of the test judged, five at the least, the upper quartile, each way.
  - That is what the radio does when it is well, and a long bad spell does not pull it down.
  - **A like signal** (added the same day): a tester that moves, as ce7f did into the Living Room AP's place, hears the radio otherwise and learns its usual anew. A radio gone slow is heard as before, so B101 and B109 are still said.
  - A test that says no signal is held to all the tester's tests.
- **A test is slow** where it got under a third of its tester's usual, down or up.
- **A radio runs slowly** where its last three tests that have a usual, by any of its testers, were each slow, and the newest is from the last six hours.
- **It is a warning on that AP,** keyed `slow-radio:<band>`, since the oldest of the three. It says:
  - each test, down/up;
  - each tester's usual;
  - which way: "it sends slowly", "it receives slowly", "it sends and receives slowly", or "its clients get little from it" where each was slow a different way.
- **It ends** with the first test that is not slow, or six hours after the last.
- **Tests asked by hand count,** as scheduled ones do. Tests over the wire, failed tests and tests on no AP named do not.

### Scheduled tests measure speed

- **`assurance.tests.speed` is on at Home** (change 520), against the local server: 32 MB each way, about a second each. The remote server stays off.

## Consequences

- **B101 would have been said:**
  - as `slow-radio` on the Living Room's 5 GHz within three of its tests;
  - as `struggling` on a phone, for its resends and, at -70, for its rate.
- **A radio that never ran well has no usual to fall from,** and is not said: C360-AP's tests of the Living Room got 3 to 14 Mbit/s all week.
- **A new tester, or a new AP, says nothing for its first five tests of each radio:** about fifteen hours at the lab's pace.
- **Resends may raise a healthy phone again.** If they do, the threshold is changed in code, by what it raised, and not taken out.
- **Tested:**
  - **The struggling alert:** resends at 40% and 60% are a reason; heard at -71 dBm, 60 Mbit/s is slow under 81 and 100 is not; heard at -84, 20 is not judged; `slowUnder` at -50, -65, -71, -80 and -81.
  - **`slowRadiosOf`:**
    - three slow tests make a radio slow, against the upper quartile;
    - a tester that moved, heard 25 dB less, says nothing; the same slow tests heard as before are slow;
    - a well newest test, a run seven hours old, or a tester with one test say nothing;
    - two testers' slow tests make a run, each against its own usual;
    - tests over the wire, and an AP not in the tree, say nothing.
  - **The alert's message:** each way, keyed by band.
