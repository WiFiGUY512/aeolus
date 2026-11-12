# 0166. Clients first on the Overview

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff: "I feel like the main overview on both the folder and the AP should be a client connection overview, with a graph for 2.4 5 and 6ghz, Number on and number of problems, that seems more important than the total data usage". Written up by Claude
- Refines: 0102, 0108, 0140

## Context

- **A folder's Overview began with tiles,** among them Wi-Fi clients now by band, and lower down a chart of the day's traffic with the most clients at once as one line. An AP's had no chart of its clients but that one.
- **The manager kept, at each report, how many clients an AP had,** and the most at a report in each hour; not on which band.

## Decision

- **A Clients panel comes first** on a folder's Overview, after its tiles, and on an AP's, after its state: the clients on now, by band; the attempts to come online that failed over the span, and how many clients made them, opening the Assurance tab; and a chart, a line a band (2.4, 5 and 6 GHz) of the most clients the APs had at once, with the failures in a strip under it, on no scale of the clients'. Its span is chosen as Usage's is: a day, a week, 30 days or a year.
- **The manager keeps each report's clients on each band,** beside the total, and the most in each hour (conditions schema 25): rows from before have none, and the chart's lines begin where they do.
- **GET /v1/usage says them,** each bucket's `bands` and `failed`, and the span's `failed` and `failed_clients`.
- **Usage goes to the end** of a folder's Overview; on an AP's it stays after Actions.

## Consequences

- **The lines are the most at once in each span,** summed over the APs, as the clients' line of Usage is: a phone that moves from 5 to 2.4 GHz and back in one span counts on both.
- **The band colours are checked for colour blindness** (2.4 blue, 5 orange, 6 aqua); the 6 GHz aqua is light against the panel, so each span says its numbers on hover and the key names each line.
