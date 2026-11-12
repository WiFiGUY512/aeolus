# 0253. Facts wait for a radio's networks

- Status: Accepted (Claude, 2026-10-31, under Griff's standing word to develop Aeolus on its own, 2026-10-24)
- Date: 2026-10-31
- Proposed by: Claude, on ce7f's new version after its flash to port -77
- Refines: 0122 (an AP says again what it is)

## Context

- **The agent says again what the AP is** (0122): its MACs, board, OpenWrt and radios, where two looks running agree and differ from what it last said. Each change gives the AP's config a new version.
- **After it starts, it looks every minute** until it has said once, then every five.
- **A radio on a DFS channel listens for radar before it beacons:** a minute, ten on weather radar channels. Until then hostapd makes only the radio's first network; its others come after the check.
- **On 2026-10-31, ce7f came up after its flash in a 60 s radar check on channel 140.** The agent's first two looks were a minute apart, and both fell in the check.
  - They agreed on MACs without the three 5 GHz networks still to be made, e2, e6 and ee:d1:24:be:3a:40.
  - So the manager recorded a change of facts (593 in the change log), and ce7f a new version.
  - Nothing was applied, and Wi-Fi was untouched: the config came out the same. Five minutes on, the networks were back in its MACs, which made another change and another version.
- **netifd names an interface before hostapd makes it:** wifi-scripts tells netifd each interface's name as it sets the radio up, and that the radio is up once hostapd has started, before any radar check. aeolus-prober's lines during ce7f's check asked for phy0-ap1 to phy0-ap3 by those names: "No such device".

## Decision

- **A look while a radio netifd has up has a network not there yet is no look.** That is an interface `network.wireless` names whose device the kernel doesn't have.
- **For up to 15 minutes of the same networks missing.** Then the look is taken as it is, so a network that never comes, as one hostapd left out, holds the AP's facts back for 15 minutes at most. That is longer than a weather-radar check.

## Consequences

- **An AP that starts during a radar check says what it is once its networks are up:** no needless change of facts, and no needless versions.
- **A radio that restarts holds a look back while it does,** as two looks already did.
- **The agent changes:** `aeolus-agent`'s `facts_again`, and its new `coming_up`. The manager is unchanged.
