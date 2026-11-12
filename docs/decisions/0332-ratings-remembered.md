# 0332. Channel ratings remembered, stale, for a day

- Status: Accepted (Griff, 2026-11-06)
- Date: 2026-11-06
- Proposed by: Griff, on OfficeOpenWrt's Ratings tab, which showed one block while its 5 GHz radio sat on DFS channel 132: "lets keep what the AP remembers here, and mayby put a srtale timer on them, at some point they mey be refreshed by a borroweed radio but lets not do that quite yet". Written up by Claude
- Refines: 0073 (RRM's ratings), 0120 (DFS after a look), 0263 (radios that can't scan)

## Context

- **RRM drops a channel's rating 30 minutes after its last visit (`RATED_FOR`).**
- **A radio on a DFS channel can't scan** (0120), and a radio whose scans are refused can't either (0263). Its other channels then fall off the Ratings tab after half an hour.
- **The case:** OfficeOpenWrt, an OpenWrt One with no scan radio, moved 44 → 132 at about 01:00 CST on 2026-11-06. By morning its Ratings tab showed only 132–136, though the AP had rated every 5 GHz block before the move.

## Decision

- **A rating not visited for 30 minutes is now stale, not dropped.** RRM keeps it as it last was, for up to a day (`REMEMBER`, 24 h), then drops it.
- **The report marks it `stale: true`.** Its `ago` says how old it is.
- **RRM never moves a radio by a stale rating:**
  - Only fresh ratings reach `decide`.
  - Only fresh ones can be best.
  - "Every channel rated" counts only fresh ones.
  - A radio on DFS steps off to its known home as before (0120).
- **A visit to a stale channel starts its rating anew.** It isn't blended with the hours-old one.
- **Remembered ratings go into the report after the fresh ones,** so they never crowd the fresh ones out of the 64 reported.
- **Agent restarts keep them:** RRM restores remembered ratings from rrm.json up to the same age.
- **The manager's "RRM finds no channel" alert (0267) counts fresh ratings only.**
- **On the Ratings tab, a remembered block is faded and dashed, with its age under it** ("12 h").
  - The AP's line says "1 block rated · 5 remembered".
  - Hovering or clicking a block says how old it is and that RRM doesn't move by it.
  - The 20 MHz table marks each row "remembered".

## Consequences

- **An AP that can't scan for a while still shows what it last knew of its band,** and how old that is.
- **RRM's choices are unchanged:** they rest on fresh ratings alone.
- **Later, a borrowed scan radio could refresh them** (not built; Griff, 2026-11-06: "not yet").
