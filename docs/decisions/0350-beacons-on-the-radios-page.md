# 0350. Beacons on the Radios page

- Status: Accepted (Griff, 2026-11-10: "I would also like Beacon details")
- Date: 2026-11-10
- Proposed by: Griff. Written up by Claude
- Refines: 0339 (beacons over half an hour), 0221 (beacons that do not go out)

## Context

- 0339 keeps each radio's beacon counts at every state report, in `beacons`: sent, failed, and over how many seconds.
- The beacons alert reads only its last half hour. Nothing showed the rest: a radio's background, its bursts, or how close it came to the alert.
- 0221 left showing them on the Radios page undone.

## Decision

- **A Beacons part on the Radios tab,** of an AP and of a folder, after Load. One band at a time, the page's band picker's.
- **A chart, drawn with `chart.js` (0294):**
  - a line a radio of the share of its beacons that did not go out, in each stretch;
  - in a strip under it, how many went out a second, about ten for each network on the air;
  - a ring where the half hour that ends with a stretch is an alert, as the beacons alert judges one.
- **Over the Hours pulldown, a day or a week.** A day is the default. The stretches are 5 minutes up to 12 hours, 10 over a day, 30 over a week.
- **A row a radio under it:**
  - its last half hour as the alert reads it now, green or amber, with its counts;
  - the span's share and counts;
  - its worst stretch, and when.
- **The alert's rule is said over the chart,** from the manager: 2% over 30 minutes, at least 30; 10% before a quarter hour of counts.
- **`GET /v1/beacons`** gives it. The alert and the page judge a half hour with the same function, `alerts.BeaconsJudged`.
- **Bounded:** a week at most, 50 APs and 40,000 stretches an asking. It is read by `beacons_ap` (AP, time), and summed in the store.
- **No new collection, and no agent change.**

## Consequences

- **The half hour beside each radio is the alert's own,** summed over the same reports.
- **A ring over the chart is where the alert would have been on,** give or take a stretch; the history of alerts says when it was.
- **Bursts show** at 5 minutes over the last few hours, and are averaged away over a week.
- **Only ath11k radios count their beacons:** the R7800 and the OpenWrt One have no line.
- **The part is redrawn every 30 seconds,** as Load is. A new report comes every five minutes.
- **Tested in a browser on a week of made-up reports,** served from the route's own answers: an hour, a day and a week; a folder and an AP; a band with none; light and dark.
