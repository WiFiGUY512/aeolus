# 0348. The clients-heard table taken out again

- Status: Accepted (Griff, 2026-11-10: "I guess we should just eliminate this list altogether")
- Date: 2026-11-10
- Proposed by: Griff. Written up by Claude
- Withdraws: 0347 (the clients an AP hears, and is told of)

## Context

**What Griff wanted from the list was keys ahead of a roam.** He asked that a hello from one AP bring a list of clients from the other, with a counter that goes up as it changes. What he meant: "I only thought the ap could pre cache clients nearby so they could roam more effectively."

**That is done already, without the list, and for every AP of the site** (0224):
- a client that signs in with 802.11r has its key pushed by its AP to every other AP of the site, over the wire, as it signs in;
- in the 24 hours to 2026-11-10 19:50 CST, 65 roams were 802.11r, 4 to 10 ms, none failed;
- with seven APs, holding every key on every AP costs nothing, so narrowing it to the APs that hear a client would make no roam faster.

**What cannot be held ahead, the list cannot help with either.** A WPA3 sign-in without 802.11r needs the client in the exchange. The laptops did 29 of those in the same day, typically 83 ms.

**What 0347 added is a view of the list, not a use of it:**
- each AP's table of the clients it heard or was told of, kept by aeolus-rrm;
- the agent's `POST /v1/ap/heard` each minute, and the manager's `GET /v1/heard`;
- an AP's Clients → Clients heard, and the neighbour map's lines to click.

Nothing acts on any of it.

## Decision

**0347 is taken out whole:** the table in aeolus-rrm and `aeolus.probed`, the agent's report, both routes, the AP's sub-tab, and the map's lines to click. The neighbour map is as 0346 left it.

**The list itself stays (0289).** Each AP still sends the others, each hello round, the clients it hears by their probes. It is what these go by:
- sticky steering's "heard better elsewhere" (on Home_Wi-Fi_IoT): a weak client is let go only where another AP hears it by the margin better;
- a client's nearest-AP rule (0321), as the Samsung TV's;
- whether to ask a client what it hears (802.11k): not where another AP heard it of late.

Without it, sticky steering would put off weak clients with no sign of anywhere better, which is what 2.4 GHz-only clients must be spared.

**No list counter** (0347's "next"): nothing would act on a change.

## Consequences

- **The AP page and the neighbour map are as before v0.319.0.**
- **An AP no longer says its table each minute.** An agent still on v0.319.0 meets no route and lets it be (it takes 404 quietly), until it takes the new agent from the manager.
- **Roaming and steering are unchanged.**
- **Checked by:** CI's tests, which are as before 0347; after release, every AP on the new agent, and none logging `heard:` or `state report: HTTP`.
