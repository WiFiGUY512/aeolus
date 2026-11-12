# 0290. A state report taken as much as it can be

- Status: Proposed
- Date: 2026-11-03
- Proposed by: Griff, by his standing word to keep developing Aeolus on its own (2026-10-24), after the night of 2026-11-02. Written up by Claude
- Refines: 0039 (the state report), 0099 (alerts)

## Context

- **One bad element refused a whole report.** The manager read a state report strictly, all or nothing:
  - a field it did not know, an element of a list it could not read, or one that failed its check, and it answered 400 for the whole report and kept none of it;
  - nothing in Aeolus said so: the AP logged the 400, and the manager logged nothing an alert reads.
- **2026-11-02, 22:25 to 23:31 CST, on every AP.** v0.232.0's steering daemon wrote its probe readings to the prober's file (0289's v0.234.0 note).
  - About one report in five then carried a steering reading, an array, as its first client.
  - The manager refused each of those whole: `json: cannot unmarshal array into Go struct field stateReport.clients of type api.wifiClient`.
  - For an hour the manager's view of every AP was older than it showed, with its DHCP watch missing.
  - The porting session found it, in the APs' logs, an hour in.
- **A report is many things,** most of them lists, each from its own part of the agent: the prober's clients, the agent's radios and ports, the RADIUS servers, 802.11r's key holders. A fault in one need not lose the rest.

## Decision

- **Each of the report's lists is taken element by element.** Before the report is read, each list named in a table (`reportLists`) is read one element at a time:
  - the elements read as strictly as the report is, with no field the manager does not know;
  - each held to the check the report's check holds each of its elements to.
  - One that cannot be read, or does not pass, is left out, and the rest of the list and of the report taken.
  - A list that is no list at all is left out whole.
  - The lists: `radios`, `bssids`, `ports`, `vlan_probes`, `uplink_vlans`, `on_air`, `clients`, `others`, `radius` and `ft`.
- **The table is the place to grow.** An entry is a path, so a list inside an object of the report (`rrm.neighbours`) takes one line and the element's check. The checks each element is held to are the report check's own, drawn out of it (a radio's, a port's, an interface on the air's, a VLAN probe's, a watched VLAN's, a BSSID's), so the two cannot drift apart.
- **What stays the whole report's:**
  - what a list is held to as a whole, as how many it may have and no two ports, or watched VLANs, alike;
  - a field the manager does not know at the report's top (manager before APs, as always);
  - a body that is no JSON object;
  - and the checks of the report's objects, as `rrm`, `vxlan` and `time`.
  - Those are refused with 400, as before.
- **What was left out, and what was refused, is kept for each AP:** each report it sent in the last 15 minutes, which of them were refused and why the latest was, and for each list that had elements left out, in how many reports, how many of how many in the latest, and why the first of those was. It is kept in memory: a manager that started since says nothing of before, and the AP's next report says it again.
- **Two warnings say it** (0099):
  - `reports-refused`: the manager refused one or more of the AP's state reports whole in the last 15 minutes. It says how many of how many, and why the latest was, since the run began, and that what Aeolus shows of the AP may be out of date.
  - `report-trimmed`, keyed `report-trimmed:` and the list: the manager took one or more of them without elements of that list. It says how many of how many in the latest, or that it was no list at all, in how many reports, and why.
  - Each ends 15 minutes after the last such report: an AP whose reports are refused now and then, as on 2026-11-02, holds its alert while that goes on, and does not flap.

## Consequences

- **The night of 2026-11-02 would have read differently.** Every report would have been taken, without the one steering reading among its clients, and each AP would have said `report-trimmed:clients` within the minute: "1 of the N in its latest ... json: cannot unmarshal array into Go value of type api.wifiClient".
- **A bad element is no longer a 400.** The answer to the report says what was left out (`left_out`: each list, how many of how many, whether whole, and why), which today's agents do not read and a later one may log; the manager's alert is where it is said now. The agent's own guard (0289's v0.234.0 fix, leaving out what is not a client) stays.
- **Reports cost a little more to read:** the listed lists are read twice, once element by element and once as the report, and the body is written again where anything was left out. A report is at most 512 KB, once a minute or so.
- **Not done here:**
  - The lists inside the report's objects (`rrm.neighbours`, `vxlan.tunnels`, `dhcp`'s servers): each a line of the table once its element has a check of its own, drawn out of its object's.
  - A record of it that outlives the manager, and a history of it beside the alerts' own (0109).
- **Checked:**
  - the report tests, with the bad elements of 31 of their cases now left out and the rest taken, two alike still refused;
  - a report of v0.232.0's kind, one array among its clients: taken with the good client, `report-trimmed:clients` saying "1 of the 2";
  - a list that is no list, left out whole;
  - a field the manager does not know, refused, and `reports-refused` saying "1 of the 3";
  - the 15 minutes, with each time minutes away from the window's edge.
