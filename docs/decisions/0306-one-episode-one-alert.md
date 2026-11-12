# 0306. One episode, one alert

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from the alert history of 2026-11-03 to 05
- Refines: 0101 (sending alerts), 0109 (the alert log), 0182 (a failing test's alert), 0233 (a client that struggles)

## Context

The alert history over two days logged one episode as many rows, in two ways.

- **An alert whose cause flickers came back as a new row:**
  - The Shed's Aeolus-Enterprise 2.4 GHz test, tried from the Garage, failed three times between 08:58 and 09:30 CST on 11-04, and was not tried again.
  - Its alert stands for three of the tester's rounds plus an hour while the thing is in the tester's turn, and an hour when it is not (`verdictStands`).
  - Whether it is in the Garage's turn depends on whether the Garage's scan radio hears the Shed. So the alert ended, then came back hours later, still saying it began at 08:58.
  - The log opened a second row for it (2252, then 2264). A webhook or ntfy target past the 15-minute quiet time would have had it sent again.
- **A struggling alert moved from AP to AP with the client:**
  - It was put on the AP the client was on now, while its words named another ("moved between 5 GHz and 6 GHz on Living-Room-C-360").
  - One night's band flipping of a phone on the Living Room was logged on the Living Room, the C260, the Garage, C360-AP and the Office: twelve rows for one episode.

## Decision

- **An alert that comes back saying it began when an ended row of it began is that row taken up again:** its end is cleared, and no new row is made. An alert that says it began anew, or says nothing of when it began, gets a row of its own as before.
- **The notifier keeps, for a day after an alert sent ends, when it said it began.** The same key back within that day, saying it began then too, is the same episode and is not sent again. A new episode is sent, after the quiet time as before.
- **A struggling alert is on the AP the client struggles on, wherever it is now:**
  - the AP whose bands it moved between, where that is a reason;
  - else the AP it was busy on the most minutes, where the resends and the slowness were seen.
  - It still needs the client heard somewhere in the last 10 minutes.

## Consequences

- **A failing test's alert that flickers is one row** from its first failure to its last end, and is sent once.
- **A struggling alert stays put while the client roams,** so the alert, its row and anything sent are about the AP named.
- **A struggling client heard well elsewhere now** keeps its alert on the AP it struggled on, until the two-hour window no longer shows it there. That is the AP to look at.
- **Rows already logged twice stay as they are.**
