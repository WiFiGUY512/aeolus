# 0165. An AP that goes is seen to at once

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff: "if an ap is down the bubble next to it, (green or orange, should go red", and "make it close to real time". Written up by Claude
- Refines: 0042, 0145

## Context

- **An AP's dot in the tree went red only once it had not been heard from for three minutes:** three of its polls. An AP that rebooted, or lost its power, kept a green or amber dot for the first minutes it was gone, and the page drew it again only every half minute.
- **Each AP holds a line open to the manager (0145),** and the manager says its clock on it every 10 seconds.
- **An AP that reboots closes its line;** one that loses its power or its cable does not, and the manager's kernel would go on taking the line to be open until TCP's own retries ran out, a quarter of an hour.

## Decision

- **An AP is down once its line went and it opened no other:** at once where the manager found it gone with the line, and 10 seconds after where the AP closed the line itself, the time an agent restarting takes to open another.
- **The manager finds an AP gone when what it wrote on the line goes unacknowledged for 15 seconds:** its kernel gives the connection up then (TCP_USER_TIMEOUT, on the line's connection alone). With its clock said every 10 seconds, the line of an AP without power is given up within 25.
- **GET /v1/aps says `down`, and GET /v1/aps/alive** says which APs are down, lightly, for a page to ask every few seconds. The tree asks it every 3 seconds while the page is in front, and turns a down AP's dot red, with when its line went on hover; the dot an AP that is back was drawn with comes back.
- **An AP whose line the manager has not seen go is never said to be down by it:** an AP with an agent older than the line, or any AP just after the manager starts. The three minutes stand for those.

## Consequences

- **An AP that reboots goes red about 10 seconds after,** and one that loses its power within about 35: the kernel giving the connection up ends the request as a close does, so the grace is waited too.
- **Tried, 2026-10-15, on the Shed AP,** its traffic to the manager dropped for 45 seconds: its line went 18 seconds in (v0.110.0; in v0.109.0 the give-up time was not set, the connection being the manager's quick-ACK wrapper's, and the line went only when the AP opened another), so it was down 10 seconds after; its line was open again 4 seconds after the drop ended.
- **The other pages, the AP's own among them, say Down at their next drawing.**
