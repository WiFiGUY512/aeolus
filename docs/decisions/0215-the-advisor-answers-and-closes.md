# 0215. The advisor answers a question, and closes what it no longer sees

- Status: Accepted
- Date: 2026-10-24
- Proposed by: Claude, from 0205's "Not yet"; built on Griff's standing word to go on developing Aeolus ("I absolutly give you permission to continue to develop this thing autonomously"). Written up by Claude
- Refines: 0205, 0212

## Context

The advisor's first review of Home made six findings (0213). Within a day, most of them were dealt with:
- power control set to guard;
- Living Room's drops explained as a driver being tested;
- the lone DHCP request no longer an alert.

All six still read "open", and would stay open until a person dismissed each one. But dismissing says "not worth acting on", which is not what happened: they were fixed.

0205 also left asking the advisor about one thing for later. Today a person who wonders why the kettle keeps dropping reads the client's journey themselves, or waits for a whole review that may not look at the kettle.

## Decision

- **A finding can be resolved: its problem is gone.**
  - A third state beside open and dismissed. It records who resolved it, when and why (`state_why`).
  - A review resolves an open finding with a new tool, `resolve_finding {key, why}`. It may do so only where it looked and saw the problem gone, and `why` says what it saw (the tool, the numbers, the time).
  - A person who fixed the problem says so with **Fixed**. The API takes an optional reason.
  - A resolved finding that a later review records again is open again: its problem is back. A dismissed finding still stays dismissed unless it is worse.
- **Each review looks again at what is open.**
  - Its first message gives each open finding what it concerns (clients, APs, networks) and the first 400 characters of what it saw.
  - It is asked to look again at each one before it ends: resolve it where it is gone, record it again where it is still there, and leave it where it cannot tell.
- **The advisor answers a question.**
  - An admin of the folder asks with `POST /v1/advisor/run {scope, question}`, the question 1 to 2000 characters. The run keeps the question, and its summary is the answer.
  - It is a review of its own: the same reads, budget, one-at-a-time rule and bill, the folder's key. It is told to look into what was asked, as far as it needs and no further, and to end with the answer.
  - It records a finding only for a problem the question turned up that people should act on. It resolves an open finding it looked at and found gone.
- **On the advisor page:**
  - An Ask box, with the latest three answers under it. The advisor's text is drawn as paragraphs and lists, its **bold** bold, built of elements and never as HTML.
  - Open findings have **Fixed** and **Dismiss**. Resolved findings have a section of their own, which says by whom and why.
  - The reviews table says what each run was asked.
- **Ask the advisor buttons**, on an AP's page and in a client's journey, open the Ask box with the question begun ("About the AP OpenWrtnight (ap-…): "). They show only to an admin, somewhere; test clients have none.

## Consequences

- **The open findings stay true without anyone tending them.** A scheduled review closes what was fixed and says what it saw. The count of open findings again means work to do.
- **A question costs what its looking takes, up to the folder's budget.** A narrow one costs cents to a few tens of cents. It runs one at a time with reviews, and Review now and Ask wait for each other.
- **conditions migration 31→32** adds `advice.state_why` and `advice_runs.question`.
- **The MCP adapter's `get_advice`** shows resolved findings and the runs' questions.
- **`set_finding` in the MCP adapter** (added 2026-10-24) resolves, dismisses or reopens a finding as the caller, an operator at its folder: a Claude session that checked a finding against the network can say so, with why.

## Not yet

- A finding resolved by the manager itself, without a review, for kinds whose evidence it can check (an AP back online, an alert gone). For now, a review does it.
- A question asked through the MCP adapter. A Claude session there can read for itself.
