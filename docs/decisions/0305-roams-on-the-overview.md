# 0305. Roams on the Overview

- Status: Accepted
- Date: 2026-11-04
- Proposed by: Griff ("where did the roaming health/dashboard go to?"). Written up by Claude
- Refines: 0136 (roaming), 0284 (the screens)

## Context

- **0284's Overview moved roaming to the Clients tab:** the Roaming panel sits folded under the client list, beside why clients left (0255).
- **The Overview's strip says how things stand, each cell leading to the tab that says more:** APs, clients, attention, joins, client tests and traffic. It had nothing of roaming, so roaming health was two clicks away and easy to lose.

## Decision

- **The strip has a "Roams, 24 h" cell:**
  - Its figure is the roams between APs and how many clients made them.
  - Under it: how long a roam typically took to be let on, and how many failed or were slow.
  - It is in the warning colour where one failed.
  - With no roam, it says "none", or the band switches where there were some.
- **The cell leads to the Clients tab with the Roaming panel opened** and brought into view.
- **An older manager without roaming has no cell.**

## Consequences

- **The Overview reads `GET /v1/roaming?under=&hours=24`,** which the Clients tab reads already.
- **The strip has seven cells.** It fits them on one row on a wide screen and wraps on a narrow one, as before.
