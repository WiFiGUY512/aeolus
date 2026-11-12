# 0144. Live Trace: a client followed from AP to AP

- Status: Proposed
- Date: 2026-10-12
- Proposed by: Griff: "I want something on a client, connected or not, that follows the client and watches every event that happens to it, roams, auths, IPs, VLANs, whatever, AP to AP; you could put a client in a chair and sling it down the hallways and watch it roam, and go through all the processes in sequence from one AP to the next"; of where it lives, "this function would centralize on Aeolus"; and of its name, first built as "following a client", "I like Live Trace". Written up by Claude
- Refines: 0118, 0133, 0136, 0103, 0079
- Refined by: 0145, 0146

## Context

- **Each AP records every client's coming** (0118): from its authentication to its first traffic, a step at a time, the messages of its keys too (0133). The record goes to the manager when the attempt is over, at the AP's next poll, which may be a minute off.
- **So a move is read afterwards,** an attempt at a time, on the client's page; and what happens between attempts is not said at all: that an AP hears the client asking, how strongly it holds it, that it left.
- **Walking a site with a client is how roaming is judged.** Griff walked his on 2026-10-11 and the manager was read from its database after. He wants to watch it as it goes.

## Decision

- **A client is traced by its MAC, on or not, known or not,** at the asking of whoever may operate an AP: for 15 minutes unless said, an hour at most, and until stopped. Four clients can be traced at once.
- **Every AP is told,** which clients and for how many seconds more: at once on the line it holds open to the manager (0145), and in a header of its poll's answer where it holds none.
- **An AP that traces says each thing it sees of the client as it sees it:**
  - every step of an attempt, as it is worded and timed for the attempt's record;
  - what hostapd says of it outside an attempt: that it left, and why;
  - what an attempt came to, as it ends;
  - **how strongly it is heard,** looked at twice a second and said when it changes: on one of the AP's networks, with its signal; or not on, and heard asking, as usteer has it; or no longer heard.
- **The manager keeps what the APs say in one line,** each time on its own clock (0145), the latest 5,000 of a client, in memory: a trace is for looking now, and is gone when the manager stops. One that ended can be read for a day.
- **`GET /v1/clients/{mac}/trace` reads it** from where a look before left off, of the APs the caller may view; `POST` starts or extends it; `DELETE` stops it; `GET .../trace/live` sends it as it comes (0145).
- **Claude and other clients trace through `trace_client`,** the MCP adapter's tool: it reads a trace, and starts or stops one.
- **The UI calls it Live Trace,** on a client's page, and for any MAC typed on the Clients tab. Where each AP hears the client now, the one it is on first; then the line, the newest last, with how long each step came after the one before, and a mark where the AP changes. How strongly it is heard can be hidden, to read the steps alone.
- **It is the manager's, and the APs lose nothing by it:** an AP that hears nothing of tracing serves as before.

## Consequences

- **A roam is watched as it happens:** the old AP's signal falling, the new one hearing it, authentication, association, the keys, the address, and that it is on, with the time of each and the gap between them. Each step of a join also says how long it came after the join's authentication, which reads 0.000 (Griff, 2026-10-15: "start authenticated as 000"); another authentication begins another join where it is on another AP or more than 30 s later, and the client leaving ends one.
- **A client that is not on is traced too,** by where it is heard asking: a client that will not join shows which APs hear it, and nothing after.
- **An AP that traces asks hostapd of its clients twice a second:** for minutes, at someone's asking, and it stops by itself.
- **What is not seen is not said:** frames a client sends that no AP hears, and its own choice of when to roam.

## Not decided

- **A trace kept,** to be read back later or set beside another.
- **What steering asked of the client** (0138), in the same line.
- **A capture of its frames while it is traced** (0135 keeps one of an attempt that failed).
- **A map,** where the APs' places are known.
