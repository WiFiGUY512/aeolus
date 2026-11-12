# 0259. A client that keeps dropping, wherever it comes back

- Status: Accepted (Griff, 2026-10-31: "do 1", the first of the next things put to him)
- Date: 2026-10-31
- Refines: 0190 (a client that keeps rejoining), 0255 (why clients leave), 0258 (where it went next)

## Context

- **0190 catches a client that comes back to one AP every three minutes or faster:** 20 times within an hour. That was the IoT devices' 43 s loop, which turned out to be the WAN.
- **It misses a slower loop spread over APs.** On 2026-10-31 the kettle kept going: about 5 minutes on, about 100 s silent, then a join on another AP, every ~7 minutes for more than an hour. No AP saw it 20 times, so nothing was raised. It was found by hand, from the leavings (0255) and where it went next (0258).
- **Counting comings back across APs alone would catch phones walking between rooms.** What tells them apart is the time away. A phone that roams is on the next AP at once, or already joining it. The kettle was gone 100 s each time.

## Decision

- **A drop** is a leaving whose kind counts as one, after which the client tried to join again, on any AP, 10 s to 10 minutes later (0258's `next`).
  - The kinds that count are marked in the table of leave kinds (`drop`): every kind but Aeolus's own doing (moved, asked, steered, off-air, aeolus) and its network going down. A kind added later says itself whether it counts.
- **A client keeps dropping** where it dropped 4 times or more within an hour, on whichever APs. It's said:
  - in its journey, as the warning `drops`: how many times, about how often, how long away, and the APs it left. Where it left several, it points at the client or what is beyond the APs; where only one, at seeing whether it does the same on another first.
  - in the leaving view, as `dropping`, and in the panel as a "keeps dropping" line per client, on the AP and folder overviews and its own page;
  - in the MCP tools' words for both.
- **No alert yet.** An alert is an AP's (0190): who may see it, and where its folders send it. A client dropping across APs is the client's, and an alert moved from AP to AP would end and begin each time. If one is wanted, it would go on each AP the client left twice or more in the hour, as 0190's goes on each AP it comes back to.
- **The window code is shared:** journey's `MostIn` is the most of any list within a span, used by 0190's count and this one.

## Consequences

- **The kettle's loop is said by the manager itself:** "it dropped off and came back 8 times within an hour, about every 7m0s, away about 1m40s each time, leaving 57ff, C360-AP, ce7f…".
- **A phone walking between rooms isn't one,** nor is a client Aeolus moved or steered, nor one whose network restarted.
- **A leaving view looks into at most 20 clients for it,** each with up to 4 leavings within an hour, a few small queries each.
- **Leavings are kept from v0.206.0's agent on,** so there is nothing to find before 2026-10-31.
