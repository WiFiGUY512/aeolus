# 0222. A client's own page

- Status: Accepted
- Date: 2026-10-27
- Proposed by: Griff: a click on a client's name in Roaming, above all on one that went badly, should lead to the client itself. There he wants its actions in a densely populated list, each showing where it was (Home / the AP), to follow its journey. Written up by Claude
- Refines: 0103 (a client's journey), 0118 (a client's attempts to come online), 0136 (roaming)

## Context

- **A client's journey opened in place.** A click on its name, in Roaming, a folder's Clients, a network's steering, an alert or a finding, drew the journey (0103) in a panel under the list it was in.
- **Its attempts were folded lines.** Each attempt to come online (0118) was a line that opened to its steps, fifty at most. A line named the AP and the folders it is in, but not where a roam came from.
- **Following one client meant scrolling a panel** under whatever page it was opened from. Its roams were read in Roaming, and its attempts somewhere else.

## Decision

- **A client has a page of its own, `#/clients/<mac>`,** where its name leads from everywhere it is a link:
  - Roaming, its tables of roams, those that went badly and the clients that joined most;
  - a folder's or an AP's Clients, both the clients on now and everyone seen;
  - a network's steering;
  - a rejoins alert;
  - an advisor's finding;
  - a client test's own client.
- **A roam's link opens the page at that roam, `#/clients/<mac>/<attempt>`:** the attempt opened and marked, and brought into view the first time the page is drawn. One older than the attempts listed says so, and Older attempts reads further back.
- **At the top, who it is:**
  - its name, and Rename;
  - its MAC;
  - what it calls itself, where a person named it something else;
  - its maker, or that its address is private;
  - who it signed in as;
  - where it is now: the AP's folders and the AP, its network, band, signal and address, or where it was last seen;
  - Live Trace, Lookups, Path, Capture, and Ask the advisor, as each one's rights allow.
- **What it did: every attempt to come online, a line each, the latest first.** Each line has:
  - when, to the second;
  - where: the folders of its AP, the outermost first, the Org left out, then the AP (Home › C360-AP), linked to that AP's clients;
  - its band and network;
  - what it was: a join; a roam, from where (the folders and AP, and band) and of which kind; or a band switch, from which band. A roam's time to be let on comes with it;
  - its signal, how it went, and why where it failed or left, with its capture's link.
- **The table is dense:** small type, one line a row, the failed tinted. A line opens to its steps, as the folded line did. A hundred are read at a time, and Older attempts reads the next hundred. The page keeps them, and the lines opened, as it is drawn again each minute.
- **Where it stayed:** its sessions over a day or a week (0103), as dense, each with where its AP is.
- **The attempts the API gives say where a roam came from:** `roam` in `GET /v1/clients/{mac}/connections` gains `from_ap`, `from_ap_name` and `from_band`, as `GET /v1/roaming` has them. They are there only where the caller may view that AP.

## Consequences

- **A client is a page to go to, and to come back from:** the browser's Back returns to the list. Nothing opens in place any more, and the journey panel is gone.
- **One list holds everything a client did,** each line saying where. A roam that went badly is read with what came before and after it.
- **A session says where it stayed, not what it did.** The two stay apart: an attempt is an event, a session a stretch of time from the reports.
