# 0347. The clients an AP hears, and is told of

- Status: Withdrawn by 0348 (Griff, 2026-11-10: "I guess we should just eliminate this list altogether"). Released in v0.319.0; v0.320.0 takes it out
- Date: 2026-11-10
- Proposed by: Griff, 2026-11-10: "is there a place in aeolus where we can go to see the client table thats on each AP and weither it heard it directly or was client inffor shared by another AP", "also it should go fifo based on client activity", and of the neighbour map's hellos, "why not have a hello from one AP result in a list of clients from the other?". Built under the standing word (autonomous development). Accepted with the merge
- Refines: 0289 (sticky clients heard better elsewhere), 0346 (the neighbour map)

## Context

**Each AP already shares the clients it hears** (0289):
- aeolus-steer keeps, each look, how strongly the AP hears the clients it does not serve, by their probes, for the last 30 seconds.
- aeolus-rrm sends that list, signed, to every other Aeolus AP it knows, each hello round of ten seconds. It goes over the wire, in a packet of its own beside the hello.
- What the others send, aeolus-rrm keeps for aeolus-steer, which sends a client to the AP that hears it best.

**None of it reached the manager.** Both lists lived only in /var/run on the AP, 30 seconds deep, so nobody could see what an AP heard or was told, or by whom.

**The state report is not the place for it.** Every report is kept, and searched by a client's MAC (0103). A table of 128 clients in each would grow the history by megabytes a day, and every client's search would find the APs that only heard of it.

## Decision

**aeolus-rrm keeps a table of the clients heard, first in, first out by activity:**
- one row for each client, band and teller (this AP, or the AP that told of it), holding the latest reading: signal, when, and whether a scan radio heard it (0321);
- its own readings taken each hello round, whether or not there is an AP to tell; another AP's, as each message comes;
- the 128 heard latest kept, the latest first. As it fills, the one heard longest ago goes, and none stays unheard for a day.

The logic is `aeolus.probed`'s `heard` and `heard_list`, tested with the module.

**The agent says the table to the manager every minute** (`POST /v1/ap/heard`), while RRM is on and its results are recent. Ages are as of the send. **The manager keeps the latest each AP said, in memory.** It keeps no history and stores none of it with the reports, so a manager started afresh has none until each AP next says. `GET /v1/heard?under=` gives each table the caller may view, with ages as of now.

**In the UI:**
- **An AP's Clients → Clients heard** is its table on the page's band, the latest first.
  - Each row says how strongly, how long ago, and who heard it: "Here", "Here, by its scan radio", or "Told by" the AP that told of it, linked to that AP's own table.
  - All, Heard here or Told by others picks which rows show.
  - The heading counts both, and the APs that told.
- **The neighbour map: click a line** (or Enter on it) for what its two APs told each other on the map's band: each way's clients, the latest first.
  - The line is drawn thicker while picked.
  - What it shows is kept as the page is drawn again, and closed with Close.
  - A hello and its client list are what go between the two over the wire each round.

**Not done here:** a list counter. Griff asked whether a hello could carry a count that goes up when the client list changes.
- **Not on the beacon in the air:** that would rewrite every radio's beacon every few seconds.
- **Not for traffic:** the list goes whole every round and fits one frame, so nothing is saved by fetching only on change.
- **Next, on the wire:** a counter on the list itself, which goes up only when its set of clients changes, so an AP and the map can tell a changed list from the same one again.

## Consequences

- **Anyone who may view an AP can see what it hears and what it is told, and by whom.** What steering goes by (0289) is no longer only on the AP.
- **The table reaches back further than the 30 seconds steering acts on:** up to 128 clients, the latest heard first, each with how long ago.
- **The state reports, their history and searches by MAC are unchanged.**
- **An older agent says nothing, and its AP shows none.** The manager must be the newer one before the agents, which this release is: the agents take the new agent from it.
- **Checked by:**
  - `agent/test/probed.uc`:
    - its own readings and another AP's taken as rows;
    - a newer reading kept over an older one;
    - 128 kept of 133, the latest first, the oldest five gone;
    - all gone after a day.
  - The API's `TestClientsHeard`:
    - rows kept, the bad left out, at most 128;
    - said again in place of the old;
    - a field it does not know refused, as is an AP it does not know;
    - shown to an operator and not to one who may view no AP.
  - The UI on a local manager with seven APs that tell each other: the AP's table, and a line's two ways under the map.
  - `aeolus-rrm` and `aeolus-agent` compile on ShedAP's stock 25.12 ucode.
- **Not proven yet: the table from real APs.** It is checked after release, on each AP and in the manager.
