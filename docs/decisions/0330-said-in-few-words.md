# 0330. Alerts said in a few words; a client's failed joins alone

- Status: Accepted (Griff, 2026-11-06)
- Date: 2026-11-06
- Proposed by: Griff. On the Overview's Needs attention: "lets make these much shorter and clickable for a long explanation". On a client's failed count in the clients list: "I was thinking if I clicked on the '14 Failed' it would show me just the failures (with the type of failure next to the collapsed version)". Written up by Claude
- Refines: 0099 (alerts), 0109 (alert history), 0118 (attempts to come online), 0284 (the Overview), 0222 (a client's attempts on its page)

## Context

- **An alert's message says everything:** what is wrong, why, and what to do. Some, such as a struggling client's, run to four lines in the Overview's narrow Needs attention column, so a few alerts fill it.
- **The clients list counts a client's failed joins** ("14 failed"), but the count led nowhere. A client's page lists every attempt, passed and failed, so a few failures among many joins take scrolling to find.

## Decision

### Each alert has a short

- **Every alert carries `short`:** what it is about and what is wrong, in a few words (at most 48 characters, enforced by the alert tests), with the AP shown beside it. Examples:
  - "a phone struggles"
  - "offline for 12 min"
  - "5 GHz radio firmware down"
  - "Guest fails its client test on 5 GHz"
  - "4 of 6 cannot get on Guest"
- **The manager writes the short for each kind of alert, next to the message.** The `add` helpers take it as an argument, so the compiler refuses a new kind without one.
- **A client in a short is called by its name alone.** In the message, it keeps its MAC after the name.
- **The alert log keeps the short too** (conditions schema 39, `alert_log.short`), so History says it the same way.
- **The UI shows the short, and the message one click down.** This applies to Needs attention, the Alerts tab and its History. A weak-signal line becomes the client and its signal; its AP, band and what weak means sit one click down.
- **An alert from an older manager, with no short, shows its message as before.**
- **Notifications put the short in their title:** "Aeolus: C360-AP: a phone struggles (warning)". The body is the message.

### Failed joins alone

- **The failed count in the clients list is a button.** It opens that client's failed attempts alone, over the page, latest first.
- **The window matches the count it was clicked from.** The On now list's count covers the last 24 hours below the folder. Everyone seen counts since the client was first seen.
- **At the top, how many stopped where**, the most common first. For example: "9 at Association: …", "5 at DHCP: no answer to its 3 DHCP requests".
- **Each collapsed line shows where it stopped and why first**, then where, band, network, signal and what it was. These stay in view however narrow the window. A line opens to its steps, as on the client's page. A link leads to every attempt on the client's page.
- **`GET /v1/clients/{mac}/connections` gains three filters.** `?outcome=` (online, failed, left or connected), `?since=` (a time) and `?under=` (a Locations node). The store's `ConnectionsWhere` takes a query struct, so later filters are fields, not new functions.

## Consequences

- **A list of alerts is a line each.** The long explanation is still there, one click away.
- **Every new alert kind needs a short:** the build and the alert tests fail without one.
- **A failed join is two clicks from the clients list, with nothing between it and the other failures.**
