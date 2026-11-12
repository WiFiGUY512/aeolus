# 0188. Blocks of time, a run for a while, and a round of a few

- Status: Accepted (Griff, 2026-10-21: "all of those but also test groups or singles, maybe I dont need to test everything, but instead a single troublesome group", under his standing word to merge what is ready)
- Date: 2026-10-21
- Proposed by: Claude, from Griff's ask for timed blocks, continuous runs and tests of a few
- Refines: 0123, 0129, 0131, 0180, 0181

## Context

- **Client tests already ran two ways:**
  - **On a schedule,** every so many minutes, all day or within one window of hours (0181), paused or not (0180);
  - **By hand,** as a round of everything below a folder (0129).
- **Griff asked for more of both:**
  - more than one block of time a day, and different blocks on different days;
  - running for so many hours, then stopping by itself;
  - a round of a troublesome few: one network, one band, the things joined on one AP, not everything.
- **One window can't say "weekdays 08:00-17:00 and weekends 10:00-12:00".** A pause has to be lifted, and put back, by hand.

## Decision

- **Blocks of time,** `assurance.tests.windows`: up to 8, each `HH:MM-HH:MM` in the AP's local time, after days where it names any:
  - `mon-fri 08:00-17:00`, `sat,sun 10:00-12:00`, `mon,wed,fri 01:00-02:00`, a range past Saturday such as `fri-mon`;
  - with no days, every day;
  - one that runs past midnight belongs to the day it starts on: `fri 22:00-02:00` runs into Saturday morning;
  - `00:00-00:00` is the whole of its days.

  Where set, they take the place of `window`. A test on a schedule is asked only in one of them.
- **A run that ends,** `assurance.tests.until`: Unix seconds. After it, no test on a schedule is asked, as if paused, until it is unset or moved on. Its blocks, where set, still apply within it.
- **A round narrowed to a few:** `POST /v1/assurance/round` takes `networks`, `bands` and `on` beside `under`:
  - **`networks`:** only those networks;
  - **`bands`:** only tests through the air that join on those bands. A test over the wire has no band, so it is kept;
  - **`on`:** only the things joined on those APs. Each must be an AP the caller may view.

  Each, where empty, is any. A round so narrowed with nothing left to test is refused. The round shows what it was narrowed to (`filter`), and its progress counts only those.
- **The Assurance tab:**
  - **The schedule in a line:**
    - none;
    - paused;
    - running around the clock or in its blocks;
    - running until when;
    - stopped, and when its run ended.
  - **Run for 1, 2, 4, 8 or 24 hours.** It sets `until` and lifts a pause; with no schedule it sets one every 15 minutes.
  - **Stop** pauses it. **Run on** takes the end off, and the pause.
  - **Three choices before "Test everything now":** every network or one, every band or one, every AP or one. With one chosen, the button says "Test these now".
- **Testers in `GET /v1/assurance`** carry `windows`, `until`, and `running`: whether a test on a schedule may be asked of it now.
- **None of it is sent to the APs.** The manager decides when to ask, as it did (0123).

## Consequences

- **A schedule can follow the working week, and a run can be left to end by itself,** with nothing to remember to turn off.
- **A troublesome network, band or AP can be tested on its own,** in a round as short as it needs, without the rest of the folder's tests ahead of it.
- **`window` stays,** for folders that set it; `windows` wins where both are set.

## As built (2026-10-21)

- **Schedule:** `controls.windows`/`until` and `blockOpen()` in `internal/api/assurecontrols.go`; `open()` checks `until`, then the blocks, else the window.
- **Rounds:** `roundFilter` in `internal/api/round.go`, kept per kind of test in a round of both; `roundKeeps()` used by the pending loop and the progress in `assurance.go`.
- **Schema:** `assurance.tests.windows` (a pattern of days and times) and `assurance.tests.until` (an integer, 0 or more); rendercheck lists both as not sent to APs.
- **MCP:** `ask_assurance_round` takes `networks`, `bands` and `on`.
- **UI:** `scheduleNow()` and the three choices in `views/assurance.js`; labels and values in `format.js`.
- **Tested in Go:**
  - `TestBlockOpen`: days, ranges past Saturday, past midnight, the whole day;
  - `TestScheduleBlocksAndUntil`;
  - `TestScheduleBlocksSchema`: good blocks taken, bad ones refused;
  - `TestAssuranceRoundNarrowed`: one AP, one band, none left, and refusals.
- **Seen in the local harness:** the line, Run for, Stop, Run on, and the three choices.
