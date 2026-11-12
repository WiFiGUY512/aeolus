# 0218. An AP in maintenance

- Status: Accepted
- Date: 2026-10-24
- Proposed by: Claude, on Griff's standing word to go on developing Aeolus: OpenWrt5b1f, off on purpose since 2026-10-20, has held a critical offline alert for four days. Written up by Claude
- Refines: 0099, 0101, 0109
- Refined by: 0344 (a test window: maintenance with hold, in which the AP holds still)

## Context

An AP that is off on purpose, or being worked on, alerts as one that failed:
- unplugged to be moved;
- a test unit set aside;
- a driver being tried by hand.

OpenWrt5b1f has been off since 2026-10-20, and its critical "offline" alert has stood since. It is the only alert, so the count of what needs attention says one when nothing does. The advisor's first review raised it as a finding.

The ways out today each lose something:
- Removing the AP loses its history and settings.
- Raising `notify.severity` silences every AP's warnings, not one AP's.

## Decision

- **A Locations folder or AP can be in maintenance.** The `maintenance` settings inherit as any do, so a folder's maintenance is its APs':
  - `on`;
  - `until`, in UTC: a day, as it begins (`2026-11-01`), or a day and a time (`2026-11-01T18:00Z`);
  - `why`.
- **An AP is in maintenance while `on` is true,** and, where `until` is set, until then. Past it, the AP is in service again without anyone changing anything.
- **Its alerts are still worked out, listed and kept,** each saying `maintenance` ("in maintenance until 2026-11-01 00:00 UTC: moving it").
  - They come last in the list, and are counted apart (`counts.maintenance`), not among what needs attention.
  - On the Alerts tab they are greyed, and the AP's page has a banner saying so.
- **None of its alerts is sent.**
  - An alert that outlasts the maintenance is sent once it ends, after the usual two minutes: the AP is still not right.
  - An alert sent before the maintenance began is still said to end, so whoever had it is not left waiting.
- **The settings are the manager's own,** never sent to APs.

## Consequences

- **OpenWrt5b1f can be put in maintenance** with a why, and the critical count is zero again, while its history stays.
- **A folder in maintenance silences every AP in it.** A site being rebuilt does not page anyone for a day.
- **The alert log keeps what happened,** maintenance or not. It is a record, not a pager.
- **An `until` written by hand that names no real day is refused** by the schema's pattern. A day such as 31 February passes the pattern, and is then no time at all: the AP is in maintenance until `on` is taken away.
