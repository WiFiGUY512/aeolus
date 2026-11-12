# 0167. No rebind after a test on DFS

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff, on the scan radio being bound again after client tests: "we have to eliminate that problem", "going deaf, aka rebinding". Written up by Claude
- Refines: 0121, 0126

## Context

- **On 2026-10-11 a C-360's scan radio went deaf after a client test on a DFS channel,** six times of six, and never off DFS. A test then stopped airscan, used the radio, and started airscan again.
- **So a test that used a DFS channel bound the radio's driver again before giving it back,** every time, deaf or not. Each time the radio's phy took a new name (phy3, phy4, phy5…), and hostapd logged that it could not open the old one.
- **Since airscan 0.2 a test borrows the radio** (lend and reclaim) and airscan is not stopped.
- **On 2026-10-15 the scan radio joined a network on DFS channel 60 four times** (once from C360-AP at -82 dBm, three times from the living-room C-360 to its own network), each followed by a full sweep of the spectrum: every look after heard 31 to 35 networks. A passive and an active scan on that channel did nothing either.

## Decision

- **A test on a DFS channel says so (`after_dfs`) and binds no driver again for it.**
- **A radio that hears nothing after any test is bound again, as before:** by airscan, which heals a deaf radio itself, or by the test where airscan cannot. A radio that heard nothing before a test is bound again first, as before.

## Consequences

- **The scan radio keeps its phy, and its neighbours their quiet,** through the assurance tests every 15 minutes on an AP whose 5 GHz is on a DFS channel.
- **Should the deafness come back,** it is found and healed by the look after the test, and the test's result says it was deaf.
