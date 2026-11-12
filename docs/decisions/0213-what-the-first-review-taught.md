# 0213. What the advisor's first review taught

- Status: Accepted
- Date: 2026-10-23
- Proposed by: Claude, checking the advisor's first review of Home against the APs; Griff: "set guard and build the three improvements". Written up by Claude
- Refines: 0099, 0160, 0205, 0207

## Context

The advisor's first review (0205, 0212) made six findings. Each was real, but three led a reader astray:

- **"Living-Room-C-360 drops every client whenever a channel changes."**
  - What happened: another session was loading test builds of the ath11k driver on it, at 23:42:59, 23:56:08 and 00:29:34.
  - Each load made every radio anew, so the interfaces went from phy14 to phy18, phy19, phy20 and phy21, and every client was dropped.
  - The flight recorder said "5g: 5 of its 5 clients gone"; the advisor saw the 6 GHz radio come back on channel 5 and took a fresh start for a move of RRM's.
- **A DHCP request on Home_Wi-Fi_IoT goes unanswered every hour or so** (about 25 alerts in three days).
  - Each was one request in ten minutes from one of two ESP32s that stayed online: a renewal let pass, answered at its next try.
- **Four suspect BSSIDs stayed on the Threats tab for three days.**
  - Only OpenWrt5b1f had heard them, and it was off. Its alerts had gone, since alerts are worked out from a report only while it is fresh (three reports' time), but the Threats tab showed its last report whatever its age.

## Decision

- **Radios restarting are said as what they are:**
  - **The recorder:** every radio losing its channel at once, where some had one in the point before, starts a recording, `radios-down`, said before anything else, so that the clients it drops are not taken for the cause.
  - **The live daemon** (`radios_watch`) keeps each radio's phy while it is up.
    - When they come back, a radio on another phy had its driver loaded again: its wiphy was made anew, which no restart of the Wi-Fi alone does. That is said as `driver`; where they were all down on the same phys, as `wifi`.
    - It sends this as news (`radios-restart`: when they went down, when they came back, how long, how, the radios and their phys before and after) and records it as `radios-restart`, from before they went down.
    - Radios that come back one after another within a minute are one restart.
  - **The advisor** is told that every radio of an AP going down at once is a restart or a driver reload, not a channel move. The channels its radios come back on follow from it, and it should look for what restarted it (a config applied, or a person at work on the AP) before calling it a fault.
- **A network is DHCP-silent only with two of its clients' requests unanswered** in ten minutes and none answered. One alone is not.
- **The Threats tab passes over an AP that has not reported for three reports' time,** as the alerts do: what a silent AP last heard is no rogue heard now. It shows again once the AP reports.

## Consequences

- **A driver reload by hand, as Living Room's, is said as one,** in its recording, its news and the advisor's review.
- **A lone DHCP request no longer raises an alert;** a server that answers no one still does, at its second request.
- **A rogue that only an AP since gone heard leaves the Threats tab with it.**
- **Set alongside, by Griff's word:** `apc.clients` is `guard` at Home (change 440). Power control now raises a radio for its lonely clients, as the review advised for C360-AP's 2.4 GHz at 8 dBm.
