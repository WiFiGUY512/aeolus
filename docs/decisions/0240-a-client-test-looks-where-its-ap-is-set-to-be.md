# 0240. A client test looks where its AP is set to be

- Status: Accepted
- Date: 2026-10-29
- Proposed by: Claude, from the night's channel-switch tests on the Living Room. Written up by Claude
- Refines: 0239 (a client test finds an AP that moved)

## Context

- **0239 hands a test the channels the target AP's last state report gives.**
- **A report lags a move.** On 2026-10-29 the Living Room was moved 157 → 149 (change 500, 07:02:48). It was on 149 by 07:03:06. A test asked at 07:03:10 was handed 157, because the AP had not reported since. The tester's scan radio had last heard it on 157 too, so the test would not have found it, had its scan radio not been made to scan by hand.
- **The manager knows where a pinned radio is going:** its `radio.<band>.channel`, as set for it.

## Decision

- **The manager adds each band's channel as set for the target AP,** where it is a number (not auto), to the channels 0239 hands the test: on the band asked, each once, at most 8 in all.
- **The agent is unchanged:** it scans each channel it is handed, with the ones it heard.

## Consequences

- **A test asked right after a move of a set channel finds the AP** whether or not the AP has reported since.
- **A channel left to the AP (auto) is still known only from its report.** A move RRM makes is said in the next report, as before.
- **Tested:** an ask on 5 GHz carries the reported 157 and the set 36, each once.
