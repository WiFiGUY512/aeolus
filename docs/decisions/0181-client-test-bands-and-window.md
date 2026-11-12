# 0181. The bands and the hours of the client tests on a schedule

- Status: Accepted (Griff, 2026-10-20: "do 2 and 4 from #54 too")
- Date: 2026-10-20
- Proposed by: Griff, issue #54, ways 2 and 4, after 0180 built 1, 3, 5 and 8. Written up by Claude
- Refines: 0123, 0180

## Context

- **0180's controls left two of #54's ways open** that Griff now wants:
  - **The band:** a test through the air joined on every band its network was on, such as 6 GHz where few clients use it.
  - **The hour:** tests were asked at any hour, taking airtime and addresses while people were there.

## Decision

- **Two more settings under `assurance.tests`** on a Locations folder or AP, the manager's own as 0180's are:
  - **`bands`:** the bands a test through the air joins on, of 2g, 5g and 6g; unset, every band its network is on. Tests over the wire use no band and are not narrowed.
  - **`window`:** when tests on a schedule are asked, `HH:MM-HH:MM` in the AP's local time, as RRM's own window is written (0073).
    - **When it runs:** it may run past midnight, so `18:00-08:00` keeps tests out of working hours. One that starts where it ends is the whole day; unset, any time.
    - **Out of it:** no test on a schedule is asked, and the schedule takes up again when it comes round, with what is longest due first.
- **The AP's local time is by its zone,** `system.tz`, or UTC where none is set, as its clock is. The manager carries the time zone database itself (`time/tzdata`), so the window needs nothing from the host.
- **Tests and rounds asked by hand take neither,** as with 0180's.
- **The page:** both are in the schedule editor's second section, and a tester's line says the hours and bands it is held to.

## Consequences

- **A site can keep tests to the night,** or out of working hours, and off a band it doesn't care to test.
- **An AP whose zone is set wrongly** is tested at the wrong hours by as much: the window is its local time as its folders say it is.

## As built (2026-10-20)

- **The schema:** `assurance.tests.bands` and `assurance.tests.window`, covered in the render check's list as not sent to APs.
- **`assurecontrols.go`:** `controls.onBand` and `controls.open`; the AP's zone read with its controls. `airCombos` keeps to the bands; the scheduler asks nothing on a schedule out of the window.
- **`/v1/assurance`:** a tester's `bands` and `window`.
- **The UI:** the editor's fields and labels, and each tester's line.
- **Tested:**
  - **Bands:** a band the tester is not on gives it nothing to test, and the band it is on gives what it had.
  - **The window in the scheduler:** out of it, nothing is asked; in it, the schedule goes on.
  - **The window by itself:** at 23:30 in Chicago, which is 04:30 UTC:
    - by the AP's zone or by UTC;
    - past midnight;
    - the whole day;
    - working hours kept out.

## Open

- #54's ways 6 (the scan radio's budget) and 7 (alert thresholds): 0182.
