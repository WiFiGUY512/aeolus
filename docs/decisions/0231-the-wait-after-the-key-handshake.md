# 0231. The wait after the key handshake, per AP

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from B94; Griff: "Keep going on the improvements list". Written up by Claude
- Refines: 0133 (the key handshake's messages), 0136 (roaming)

## Context

- **On 2026-10-28 a kernel fault was found that held up every WPA2 join by 30–60 ms** (B94).
  - mac80211 waited out an RCU grace period as it stored a client's key, between the fourth key message and the client being let on. It did so on any network without management-frame protection, on any driver that does the protection in hardware, as mt76 does.
  - It took a debug wpad on one AP, its debug log, and twenty roams to see it.
  - The port fixed it (patch 0033) on the Arista APs. Shed has the fix in RAM until its next reboot. The Office AP does not have it yet.
- **Aeolus already records both moments of every join** whose AP logs the key messages: the fourth message (0133) and the client's being let on (`on_ms`, 0136).
  - On Shed, they showed 23–50 ms before the fix and 2 ms after. On the Office AP, 43 ms.
  - Nothing put them side by side, so an AP that loses the fix, or never had it, went unseen.

## Decision

- **The roaming view gives each AP's wait, a band at a time** (`key_waits`): from the last fourth key message to the client being let on.
  - It gives the typical and longest wait, over every attempt of its hours whose record has both moments: joins and roams alike, the tests' own clients left out.
  - A join by 802.11r has no key messages, and one whose AP keeps no log of them has none either, as on a network that signs clients in (0133). Neither counts.
  - A wait past 5 s is the client's or a lost frame's, not the AP's, and is left out.
- **The roaming panel says it a line a band,** each AP's typical wait, its count and longest on hover. 20 ms or more is marked: 1 or 2 ms is what storing a key takes.
- **MCP's `get_roaming` has it too,** so the advisor sees it.

## Consequences

- **An AP whose kernel or driver holds joins up shows at once,** on the band it does it on: the Office AP today, and Shed again if it reboots before a build of its own has the fix.
- **It needs the key messages in the AP's log,** which the journey asks hostapd for (0133). An AP that cannot give them shows nothing, not a wait of 0.
- **No alert yet.** Whether a long wait should raise one is left until the lab has run with it a while.
