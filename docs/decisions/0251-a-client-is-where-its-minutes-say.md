# 0251. A client is where its minutes say

- Status: Accepted (Griff, 2026-10-30: "I noticed the aeolus lists for where the phone is are rather slow")
- Date: 2026-10-30
- Proposed by: Griff. Written up by Claude
- Refines: 0185 (a client is on one AP at a time), 0118 (every client seen)

## Context

- **What Griff saw:** on the morning of 2026-10-30, a phone went from ce7f to 57ff's 6 GHz. Aeolus still showed it on ce7f for minutes afterwards.
- **Why:** where a client is came from each AP's state report, which an AP sends every five minutes (`STATE_EVERY = 300`). Each place used it:
  - the Clients tab;
  - every count made from it (0185);
  - the clients list (0118);
  - `list_clients`.

  A client could be shown on the AP it had left for up to five minutes, and on the AP it had joined only after that AP's next report.
- **Each AP already says it every minute:** its minutes (0207), on the line it holds open (0145), name every client on each radio. With each client they give:
  - its signal and rates;
  - what was sent to it;
  - what it sent the AP (`rx_packets`);
  - how many seconds of the minute it was there.
- **A minute's `rx_packets` is what tells them apart:** a client that has roamed away and is still kept by hostapd is in the old AP's minutes too, but it sends that AP nothing.

## Decision

- **A client that sent an AP anything in a minute was active on it then,** at the minute's start plus the seconds it was there. The manager's record of where each client was last active (0185) now takes this from each AP's minutes as they come, as well as from each report.
  - **The AP it left names it `elsewhere` within a minute.** It is left out of that AP's counts, as 0185 has it.
  - **The AP it is on names it `arrived` in its condition** until its next report lists it. Each arrival is that AP's latest minute of it, as `GET /v1/minutes` gives a client's minute.
  - **A minute older than what is known of a client says nothing.**
  - **A report's own entry for a client** does not move it back over a later minute on the same AP.
- **The clients list (0118) follows the minutes too:** a minute puts a known client on that AP, from then, where it is on the network it was last seen on. Two cases wait for the AP's report:
  - a client seen for the first time;
  - a client that moved to another network.

  A report no longer puts back on its AP a client that was active on another AP since.
- **The page shows an arrival on the AP it is on:**
  - from its minute: band, network, signal, rates and retries;
  - from its latest report on any AP: who it is, and its Wi-Fi;
  - on the same network, its address and DHCP too;
  - its MCS, streams, data and how long it has been on wait for the AP's next report, and its Connected cell says "new here".

## Consequences

- **A client roaming, or joining, shows on its AP within about a minute.** That holds on:
  - the Clients tab and every count made from it (0185): a folder's tiles, busiest APs, the APs tab, and an AP's own page;
  - the clients list, `list_clients` and the MCP tools that read it.
- **For the minute it roamed in, both APs may have it.** The next minute puts it on the AP it is on.
- **What each AP reports, and when, is unchanged.** This is the manager alone, and the APs' agents are not touched.
- **Kept in memory for an hour, as 0185's record is.** For a minute after the manager starts, it knows only what the minutes and reports since have said.
- **A later step can make the APs say a join the moment it happens.** That would bring it within seconds, but it changes the agent, so it waits until the ce7f test (B113/B114) is over. Built as 0252 on 2026-10-31.

## As built (2026-10-30)

- **The manager:**
  - `internal/api/active.go`: `activity.minutes` and `activity.arrived`, and `note` keeping a later minute on the same AP;
  - `keepMinutes` in `minutes.go` feeds them;
  - `condition` in `report.go` gives `arrived`, and the report's sightings skip a client that is elsewhere;
  - `conditions.ClientsActive` (`moveClient`) for the clients list.
- **The page:**
  - `views/onnow.js`: `hereNow` and `onNow` take arrivals, and `gone` counts them back on their band;
  - `views/clients.js`: the "new here" cell, and `—` for data.
- **Tested:**
  - **The rule in Go:**
    - a minute of nothing on the AP left;
    - the minute on the AP joined, after which it is elsewhere on one and arrived on the other;
    - the report that lists it ends the arrival;
    - the AP it left reports it again, and it stays gone;
    - an older minute is passed over.
  - **Through the API:** a minute on the office moves the phone there:
    - the living room's condition names it elsewhere;
    - the office's condition gives its arrival with the minute's rates;
    - the clients list has it on the office, even after the living room reports it again;
    - the office's report then lists it, and the arrival is gone.
  - **The page's merge,** on a sample of the morning's roam: the phone shows on 57ff's 6 GHz with its name and address, and is not counted on ce7f.
