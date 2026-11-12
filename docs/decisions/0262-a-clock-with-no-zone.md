# 0262. A clock with no zone

- Status: Accepted (Aeolus's own development, 2026-11-01, at Griff's word to keep making fixes and features overnight)
- Date: 2026-11-01
- Refines: 0073 (radio resource management), 0074 (time zones from a list), 0181 (tests on a schedule)

## Context

- **Some windows run by local hours:**
  - RRM's planned moves, `rrm.window`, default 02:00-05:00, by the AP's own clock (0073). Since look-first (0120), these include a move to a DFS channel with a minute's silence for radar.
  - The client tests on a schedule, `assurance.tests.window` and `windows` (0181). The manager asks them by `system.tz`, and UTC where it is unset.
- **With no `system.tz` set, Aeolus leaves the AP's zone as it is (0074).** An image's own zone is UTC.
- **At Home on 2026-11-01:**
  - Three APs (Office, Shed, C360-AP) had America/Chicago, set by hand.
  - Three (the C260, Living-Room-C-360, ce7f) kept UTC. Their 02:00-05:00 was 8-11 PM Central, while people use the Wi-Fi.
  - Nothing said so. It came out only when their log times were read against the others'.

## Decision

- **The agent reports its clock's zone,** in its report's `time`:
  - `zone`, UCI's name;
  - `rule`, the POSIX rule in effect, from `/tmp/TZ`, or UCI's where that is not there yet.
- **A warning, `clock-zone`, is shown where no `system.tz` is set for an AP and something its config runs by local hours runs by UTC's:**
  - RRM's planned moves, where RRM moves channels (`rrm.enabled`, with `rrm.moves` not off) and the AP's own clock keeps UTC. Its window is `rrm.window`, or the field's default where unset.
  - The client tests on a schedule, where they run (`every` above 0, not `paused`) with a window. These go by `system.tz` alone, whatever the AP's clock.
  - A window that starts where it ends is the whole day, and needs no zone.
- **A clock keeps UTC** when its rule has no summer time and an offset of zero or none: `UTC0`, `GMT0`, `UTC`, `<+00>0`, or no rule.
- **It says what runs, with its hours, and to set `system.tz` on the AP's folder.** It ends when a zone is set, or when nothing runs by local hours. An agent that does not report `rule` gets no warning for what the AP runs by its own clock.
- **The windows are a table** (`localWindows`): another window by local hours is one more row.
- **The AP's Time row on its way-out panel shows the zone.**

## Consequences

- **A site with no zone set hears of it** while its windows are in the wrong hours, and not otherwise.
- **An AP whose zone was set by hand** gets no warning for its own clock. The scheduled tests, which go by `system.tz`, still do.
- **At Home:** `system.tz` America/Chicago was set at the folder (change 602, 2026-11-01 07:49Z), before this was built. So the three UTC APs keep Central time too.

## Not done here

- **A zone from the place:** `system.place` has a latitude and longitude, which could pick the zone where none is set.
- **The manager's own update window (0183)** goes by `system.tz` at the Org. It is the manager's, not an AP's, so no AP's alert says so.
