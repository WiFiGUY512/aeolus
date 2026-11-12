# 0294. Charts in Plotly, and the last few hours

- Status: Proposed
- Date: 2026-11-03
- Proposed by: Griff, on the Clients and Usage charts: "I love the graphs and what they do, though I think we need a pull down for 'hours' since fails happen, alternatively instead of these style graphs we can use plotly, and use its graph tools right there to modify it to the users needs on the fly", then "also I really like plotly" (2026-11-03, about 14:15 CST). Written up by Claude
- Refines: 0108, 0140, 0166
- Keeps: 0069 (nothing fetched from the internet), 0198 (libraries kept as their authors ship them)

## Context

- **The Clients chart (0166) and the Usage chart (0108) were drawn by hand in SVG,** over a day, a week, 30 days or a year (0140).
- **A day is 48 half-hour buckets,** so the failures to get on in one half-hour are one bar. To see when each one happened, a person needed a shorter span.
- **`GET /v1/usage` already takes any `hours` from 1,** in at most 48 buckets of at least 5 minutes, the APs' report interval.
- **The charts could not be zoomed, panned or saved,** and their series could not be hidden.

## Decision

- **An Hours pulldown ahead of the span's buttons:** an hour, 3, 6 or 12 hours. The manager is unchanged. An hour and 3 hours are 5-minute buckets, 6 hours 8-minute ones, and 12 hours 15-minute ones.
- **Both charts are drawn with plotly.js 4.1.2,** its basic bundle (`plotly.js-basic-dist-min`, MIT):
  - kept as its authors ship it in `internal/ui/static/vendor/plotly/`, with `third_party/plotly/`'s notes;
  - served by the manager and loaded the first time a page has a chart (1.2 MB, 390 KB gzipped). Nothing is fetched from the internet, and the offline test names each address in the bundle and why none is fetched.
- **What a person can do with a chart:**
  - hover for one box of every series at a time, the strips under a chart included;
  - drag to zoom, pan, and double-click to put it back;
  - click a name in its key to hide or show that series, and double-click one to show it alone;
  - save it as a picture.
- **What was zoomed and hidden is kept over the page's 30 s redraws:** the zoom by chart and span, what was hidden by chart. It is kept in the page, so a reload starts afresh.
- **`chart.js` is the one place a chart is made:** the page's colours and type, the key, the time axis, the hover, the toolbar, and what is kept. A view gives it its traces and axes.
- **One axis a plot (no second scale over the first):** the Usage chart's clients line moves from over the bars to a strip of its own under them, as the Clients chart's failures already were.
- **The Content-Security-Policy is unchanged (`style-src 'self'`):**
  - Plotly's own style rules are served as `vendor/plotly/plotly.css`, written from the bundle by `third_party/plotly/plotly_css.py`.
  - The rules Plotly would add for each chart are blocked; their colours (the toolbar's) are set in the layout instead, which Plotly sets on the elements themselves.
  - Plotly's warnings are off (`logging: 0`), since it would warn of the blocked rules on every draw.
- **Plotly is given only numbers, times and the page's own words, never a client's name.** It draws labels from a markup of its own, and uses `innerHTML` in places. `TestModules`, like ESLint, leaves `vendor/` alone.

## Consequences

- **A failure's time can be read to 5 minutes** over the last hour or 3, by hovering its bar.
- **Zooming into a day does not fetch finer buckets:** the Hours pulldown does. Finer data on zoom would need `/v1/usage` to take a start and an end.
- **The other charts stay in SVG for now:** an AP's processor and memory, a network's step times, Bluetooth, recordings. Each can move to `chart.js` the same way.
- **A tab in the background draws its charts when it is shown:** Plotly needs the box's width, and a page that is not shown is not laid out.
- **The manager's binary grows by about 1.2 MB.**
- **Tested in a browser against the UI harness:**
  - both charts draw on an AP's Overview and a folder's, after the 30 s redraw, and when Traffic is opened;
  - the Hours pulldown redraws with 5-minute buckets at 3 hours;
  - drag-zoom and a hidden series survive a redraw; a double-click puts the zoom back;
  - the hover covers the strips, and the title row fits at 1024 px.
