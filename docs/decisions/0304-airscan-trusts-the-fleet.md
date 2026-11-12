# 0304. airscan trusts the fleet

- Status: Accepted
- Date: 2026-11-04
- Proposed by: the OpenWrt porting session, in the Phone-B client matrix (finding F3): "write every org SSID, test ones included, and every fleet BSSID as it is really sent into those lists". Written up by Claude
- Refines: 0105 (what the APs together call their own), 0106 (rogues.known), 0160 (rogues judged by the manager)

## Context

- **airscan is the survey the Arista APs' scan radio runs** (the port's package). It classifies every BSS it hears:
  - **trusted:** the AP's own BSSIDs, and those its config's `trusted_bssid` names (prefixes allowed).
  - **ours:** a BSS with one of the AP's own SSIDs, or one `trusted_ssid` names. Ours that relays airscan's probe over the wire is **verified**; ours that never relays it is an **evil twin**.
  - **wired rogue:** a BSS of any other SSID that relays the probe.
- **Left to itself, airscan knows no other AP's BSSIDs, nor the SSIDs only other APs serve.** It logged `wired_rogue` and `evil_twin` for the fleet's own BSSes: for the W-318's aeolus-s23 after the C360-AP rebooted, for the Shed AP's aeolus_50, and on 2026-11-04, a dozen lines a scan on the Living Room for the Garage, Office and C260.
- **The manager ignores airscan's verdicts** and judges against the fleet's BSSIDs itself (0160). So the noise was in airscan's own log, its events and its LuCI page, not in Aeolus's alerts. It is noise all the same, and it hides a real one.
- **airscan reads its config before each BSS scan** (`load_config` in `run_bss`), so new lists need no restart.

## Decision

- **The manager hands each AP what its airscan is to trust,** beside the config, as it hands the 802.11r key holders (0224). Nothing in it versions the config.
  - `trusted_bssid`: every BSSID an AP's latest report names as its own, as it is really sent, plus the BSSIDs the AP's folders know (`rogues.known`, 0106). All in lower case.
  - `trusted_ssid`: the SSID of every network, test ones included.
  - At most 512 of each. What the fleet calls its own is read at most once a minute, not on every poll.
- **Every poll's answer names its version** in `Aeolus-Trust`, a hash of the doc. An AP whose version differs fetches `GET /v1/ap/trust`. An AP in Landing Zone gets neither.
- **The agent writes the lists into airscan's config** (`airscan.main`), once for each version, where airscan is installed. An AP without it (the stock OpenWrt One and R7800) writes nothing.
- **What a person put in those lists stays.** The agent notes what it added (`/etc/aeolus/airscan-trust.json`) and only ever removes that. An entry a person had before the manager named it is the person's.
- **The module `aeolus.trust`** holds the cleaning and the merge, and `agent/test/trust.uc` tests them.

## Consequences

- **airscan calls the fleet's BSSes trusted,** so its `wired_rogue` and `evil_twin` lines and events are left for what is not ours.
- **It still finds an evil twin of our SSIDs:** a BSS with one of them and a BSSID not in the list is "ours", and it is an evil twin where it never relays the probe. Knowing the SSIDs makes it better at that than before.
- **A spoof of a fleet BSSID on another AP's channel is trusted by airscan.** The manager still judges against the fleet (0160), and an AP's own BSSID on a foreign channel is still `spoofed` to that AP.
- **A list written is a change to airscan's config file,** so the next `reload_config` restarts airscan once. That reload comes when the agent next applies a config version.
- **A new AP's BSSIDs reach the others within about a minute of its first report.**
- **Released with the agent:** an older agent ignores the header, and an older manager sends none.
