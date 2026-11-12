# 0293. A journey's weak signal is −80

- Status: Proposed
- Date: 2026-11-03
- Proposed by: Griff: "yes move the journey note to -80 too" (2026-11-03, about 14:10 CST), after 0292 left it at −75. Written up by Claude
- Refines: 0103, 0292

## Context

- **A client's journey (0103) notes a weak signal** for each session whose mean signal is under −75 dBm.
- **Griff's scale: −70 to −72 is good, and only −80 or −82 is weak.** 0292 moved sticky's weak line to −80, and listed this note as one of the −75 lines it left alone.
- **The Overview's attention list already calls a client weak at −80 or weaker** (`WEAK` in overview.js). A client could be on that list with no note in its journey, or noted in its journey at −76 with no mention on the Overview.

## Decision

- **A session is noted as weak where its mean signal is −80 dBm or weaker** (`journey.WeakSignal`), the Overview's line.
- **The note's words are unchanged.**

## Consequences

- **A session from −76 to −79 on average is no longer noted as weak.** Retries and slow rates are still noted on their own.
- **Sticky's weak rule stays at under −80 (0292):** it acts, so it waits a dB longer than a note does.
- **Assurance's floor (0123) and the struggling bar (0233, 0250) stay** as they are.
- **Tested:** a session at −79 is not noted, one at −80 or −85 is.
