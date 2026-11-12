# 0268. Slow on a channel it left

- Status: Accepted (Aeolus's own development, 2026-11-01, at Griff's word to keep making fixes and features overnight)
- Date: 2026-11-01
- Refines: 0242 (a radio that runs slowly)

## Context

- **`slow-radio` (0242) stands while a radio's last three client tests that measured speed were each slow.** It ends with its next test that is not.
- **A test knows the channel it joined on** (`channel`, the BSS's primary). Nothing compared it with where the radio is now.
- **At Home, 2026-11-01:**
  - OfficeOpenWrt's 5 GHz tests were slow from 00:32 CDT, on DFS 108, where `allow` had started it and it could not scan.
  - At 01:42 CST look-first started it on 149, and at 02:48 RRM took it to 52.
  - The warning stood, on tests of a channel it had left, until a tester's turn came round to it.

## Decision

- **The run of slow tests ends at a test on another channel than the radio is on now.** That is the radio's channel on the band, from its AP's latest report: the one with networks on the air, where it has two there, as a C-260 has with its scan radio.
- **So slow tests on a channel the radio has left say nothing of it,** and newer tests on its channel count as before.
- **Where the channel now, or a test's, isn't known,** as with an older agent or report, the tests count as before.

## Consequences

- **A radio that moves off a channel it was slow on loses the warning at its next report,** and gains it again only from three slow tests where it is.
- **0242's own case still stands:** a radio slow after a channel switch on ath11k is slow on the channel it switched to, where it now is.
- **Checked:** `TestSlowRadioMovedOn` covers moved on, still there, and the channel not known. 0242's tests are as before.
