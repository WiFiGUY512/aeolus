# 0338. An AP's Activity

- Status: Accepted (Griff, 2026-11-07)
- Date: 2026-11-07
- Proposed by: Griff:
  - "you know where we have the Activity when we go to the client? I think we should have that on the AP as well and there we should see the neighbor exchanges and such between all the APs as well as all the verifications that show the AP is up and healthy"
  - "oh and its ACS and RRM evaluations and moves? or should that be on the Radios page?"
  Written up by Claude
- Refines: 0119 (an AP's health and history), 0337 (a client's Activity)

## Context

- **What happened to an AP was in pieces.** Its Health tab had a History, from 0119:
  - what changed between its reports: a restart, a silence, an agent or firmware update, a radio down or up, a channel move, an uplink on another switch port;
  - the configs it applied;
  - its alerts, and what it was asked to do.
- **Much was not kept at all.** Each report carries its RRM's state, its neighbours, its tunnels and its RADIUS servers. Only the latest report's were kept:
  - its neighbours' hellos beginning and stopping;
  - RRM's claims, and what came of them: moved, yielded to a neighbour, withdrawn or failed;
  - a radio that can't scan;
  - a tunnel, VLAN or RADIUS server that stopped answering.
  The AP's own log said them, and was gone in a day.
- **Other things were kept elsewhere.** Power control's steps (0206), steering's askings of its clients (0139) and the client tests (0121) each had a page of their own, in another part of the UI.
- **Nothing said an AP was well.** A quiet hour and a lost hour looked the same.

## Decision

- **An AP has an Activity tab, after Clients.** It works as a client's (0337):
  - a line each, newest first;
  - in a window about twenty lines tall, its heads staying in view;
  - reading further back as it is scrolled to its end, or by Older lines;
  - a line with more to it opens to it;
  - where it was scrolled, what was read back and what was opened stay over the page's redraws.
- **Its parts are groups**, so one can be read alone:

  | Group | Its lines |
  |---|---|
  | All | every group's, together |
  | Health | its first report, restarts, silences, agent and firmware updates, uplink moves, and an hour's account of itself |
  | Radios | a radio down or up, a channel or width changed, its scan radio; RRM's claims, radar checks, moves, yields, withdrawals and failures, and what its clients did two minutes after a move; a radio that can't scan, or scans again; power control's steps |
  | Neighbours | hellos with other APs beginning, going both ways again, going one way, or stopping |
  | Network | a tunnel's, a VLAN probe's or a RADIUS server's verdict changing |
  | Config | each config applied or not, and what it was asked to do |
  | Alerts | each alert beginning and ending |
  | Clients | what its steering asked of a client, and the client tests, those it made and those another AP made of it |

- **An hour's account of itself.** The first report after an hour ends keeps a line for that hour:
  - how many reports it made, of those due from its first in the hour (one every five minutes): fewer is a warning;
  - its hellos: with how many neighbours they go both ways, and how many not;
  - how many of its tunnels and RADIUS servers answer;
  - its busiest processor, most memory in use, hottest, and most clients;
  - the config it ran.
  A day of them is how an AP is shown up and well. Each opens to the neighbours by name.
- **Each change is said once.**
  - A channel RRM moved to is said by RRM, which says why. A channel that moved otherwise is the radio's own line.
  - Neighbours whose hellos went the same way in one report share a line.
  - A move that comes into view from before the last report, as when RRM starts again, is not news. Nor are hellos stopping, or a transport starting, just after the AP or its agent started.
- **The Radios tab keeps what is so now; Activity keeps what happened.** Radios has the ratings, the neighbours, the power control's state and the last sixteen moves. An RRM or power line on Activity leads there.
- **Health keeps its charts.** Its History panel goes; a line points to Activity's Health part instead. `GET /v1/aps/{ap}/timeline` stays, for the tools that read it.

## How

- `ap_events` (0119) gains `detail`, a line's JSON; migration 39. `actions` gains an index on the AP a client test was made of.
- What the manager compares between reports now also compares RRM's moves, neighbours and radios that can't scan, and the tunnels', VLAN probes' and RADIUS servers' verdicts.
- `GET /v1/aps/{ap}/activity?before=&limit=&only=` reads every source from before a time and merges them, newest first. The sources:
  - ap_events;
  - applies;
  - actions, but client tests;
  - the alert log, its beginnings and endings;
  - apc_events;
  - steer_events;
  - client tests made by the AP and of it.
  The sources are a table: one more is one more row. Lines at the same time as a page's last come with it, so the next page, before that time, leaves none out.

## RRM's evaluations

Before this, while a planned move's reason held for ten minutes, RRM said nothing outside the AP. Now:

- **The agent reports its plans.** `rrm.plans` lists each move a radio would make, at most 12. Each gives its band, the channel it is on and the one it would go to, why, whether it costs a radar check or widens, and how many seconds its reason has held. An older manager ignores the field; an older agent leaves it out.
- **Activity gets a Radios line when a plan begins**, saying what it waits for: "channel 149 rates better than 36; it moves there once that has held 10 minutes, in its move window".
  - A plan that turns to another channel is one line, "in place of channel 149".
  - A plan given up before it was claimed gets a line too.
  - A plan that became a claim is said by the claim's line.
  - A plan held from report to report is said once.
- **A radio that stays where it is because nothing rates better says nothing.** Its channel's rating, and the others', are on the Radios tab.

## RRM starting afresh (v0.304.0, refined)

v0.304.0's own agent update showed it: RRM started again on every AP, forgot its neighbours, its radios that can't scan and its plans, and found them again. Each AP said "hellos with 7 APs stopped", then "began"; a DFS radio said "scans again", then "can't scan". None of it was news.

- **Hellos that stop leave a neighbour down for a quarter of an hour.** One that goes from both ways to only heard in the air, or off the list, was forgotten. No line.
- **The agent says how long RRM has run,** `rrm.up`, in seconds.
  - Where RRM started since the last report, or the last report was in its first three minutes, neighbours, radios that can't scan and plans given up are not compared.
  - Where it started again with no agent update or restart to say so, one line says so: "RRM started again, 90 s ago: it finds its neighbours afresh".
- **The agent passes them on (v0.306.0).** Its state report copies rrm.json a field at a time, and v0.304.0 and v0.305.0 left `plans` and `up` behind. It had left 0173's `listens` and `scan_out` behind since 0173, so Radios never said how an AP listens. It copies all four now.
