# 0295. Every chart in Plotly

- Status: Proposed
- Date: 2026-11-03
- Proposed by: Griff: "move the health and other charts to plotly too" (2026-11-03, about 14:35 CST), once 0294 had moved Clients and Usage. Written up by Claude
- Refines: 0119, 0141, 0158, 0207, 0294

## Context

- **0294 drew the Clients and Usage charts with Plotly,** through `chart.js`, with an Hours pulldown.
- **Four charts were still hand-made SVG:**
  - an AP's processor and memory (Health, 0119);
  - a folder's response times (Wired › Network Paths, 0141), as a small line in each step's tile;
  - where and when a Bluetooth device was heard (0158);
  - a recording's channel load and retries (0207).
- **Some lines are not charts:** the sparklines in table rows (a radio's last hour under Radios › Load, the live watch's five minutes), the Bluetooth table's, and an AP's calling-in strip. Each is a word-sized glyph, one a row, some redrawn every five seconds.

## Decision

- **The four charts are drawn by `chart.js`:**
  - **Health:** processor and memory from 0 to 100%. Where the AP reports one, its temperature goes in a strip of its own under them.
  - **Response times:** one chart under the step tiles, a line a step on one logarithmic scale of time, since an address takes seconds where a name takes milliseconds. A step keeps its colour whichever others are drawn. The tiles keep their numbers and lose their small lines.
  - **Bluetooth, where and when:** a lane an AP, a mark each five minutes, shaded on a one-hue scale from -100 to -40 dBm, with that scale beside it. The lanes are named by the APs, never by the device, whose name is its own to choose.
  - **A recording:** channel busy, and dashed, what was tried again or lost, 0 to 100% every five seconds. The line breaks where a point is missing, and a dotted line marks what went wrong.
- **Each has the shared span picker where it has a span:** the Hours pulldown and its buttons (Bluetooth's go to 90 days, as long as it keeps them).
- **A chart's colours are the page's tokens:**
  - `--series-1` to `--series-8`, the reference categorical order checked for colour blindness, whose first three are the bands' (0166);
  - `--seq-low` and `--seq-high` for a one-hue scale.
  - A dark page (to come) redefines them in one place.
- **A chart's own layout wins over `chart.js`'s,** field by field (Bluetooth's hover is the mark under the pointer, not the time).
- **The sparklines stay SVG,** for the reasons above.
- **Ticks read by the clock up to half a day apart, and by the date from a day apart.**
- **Two words the page wrote by mistake are gone:**
  - The Health tab wrote its tiles as "[object HTMLDivElement],false,false" (since 0119's process lists).
  - A Bluetooth device with no name of a person's own showed "null" under its name.

## Consequences

- **All of the page's charts zoom, pan, hide a series and save a picture,** and keep their zoom and hidden series over a redraw.
- **The Network Paths tiles are shorter,** with one chart under them.
- **Tested in a browser against the UI harness:**
  - Health, with and without a temperature;
  - Network Paths, with three steps on a log scale;
  - Bluetooth and a recording, from answers made up in the page.
