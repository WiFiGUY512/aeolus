# 0160. Threats: rogue APs and their clients, and rogue DHCP servers

- Status: Proposed
- Date: 2026-10-14
- Proposed by: Griff: "we need a rogues area specifically, or a threats section"; then "Rogues AP, Rogue Clients (clients that were connected to rogues) wires check on the rogue is required to 100% identify the rogue, Rogue DHCP servers should be in the network tab under threats". Written up by Claude
- Refines: 0065, 0081, 0105, 0106

## Context

- **A rogue today is a network heard with one of Aeolus's SSIDs from a BSSID no AP names as its own** (0105): an alert on each AP that hears it, and a row in Radios › Networks heard (0106). That is an evil twin, a neighbour's AP of the same name, or one of ours not under Aeolus. Nothing says which.
- **The C-360's scan radio checks for this already** (airscan, 0081). It sends frames on each VLAN its AP carries, and listens for them coming out of other APs' radios. A BSS they come out of is on our wire, on those VLANs. On 2026-10-14:
  - the other C-360's networks and the office's were on the wire, on VLANs 10 and 20;
  - the unconverted C-360 (e4:d1:24:08:6d:xx), broadcasting the old SSIDs, was not, after some 10,000 looks by each C-360.
- **airscan's own verdict can't be taken as is.** It trusts any BSS on the wire that broadcasts one of its own AP's SSIDs (`verified`): one plugged into our network with our SSID would be trusted. Only the manager knows the fleet's BSSIDs.
- **airscan also hears clients, each with the BSSID it talks through.** On 2026-10-14 it heard 153 for the unconverted C-360's four BSSIDs, one of them the client Home Wi-Fi blocks.
- **An AP's BSSIDs are not fixed.** A C-360 hands them out anew in another order when a radio starts again, so a neighbour that heard the old ones took them for a stranger's until they aged out of its list (2026-10-14, 30 minutes). With a wire check, our own AP would be a rogue on our wire.
- **A second DHCP server on a network is a rogue DHCP server, or a misconfigured one.** Each AP says which servers answered its clients (0065). Nothing showed it but the AP's own report.

## Decision

- **A Locations folder has a Threats tab,** after Alerts, in two parts:
  - **Wireless:** rogue APs, and rogue clients.
  - **Network:** rogue DHCP servers.
  - `GET /v1/threats?under=&hours=` gives all three. The MCP adapter offers `list_threats`.
- **A rogue AP is a BSSID that is none of the APs' own, heard by an AP, that broadcasts one of Aeolus's SSIDs or is on our wire.** The wire check says how sure:
  - `rogue`: on our wire. A scan radio saw frames sent on one of our VLANs come out of it. It is in our network, ours to find and unplug. Alert: critical, `rogue-wired`.
  - `suspect`: one of our SSIDs, and not seen on the wire. Either a scan radio looked and never saw it there, or none hears it. It is an evil twin, a neighbour's AP of the same name, or one of ours on a part of the network the scan radios can't reach. Alert: warning, `rogue`, as before.
  - `known`: marked known by `rogues.known` (0106). It raises no alert, wired or not, and is listed apart with what the wire check says.
- **A BSSID is an AP's own** where its latest report names it, or where it differs from one of the AP's own addresses only in its first and last bytes. Radios make their BSSIDs that way, so a C-360's that changed is still its own.
- **The wire check is airscan's, on APs with a scan radio.** An AP's state report gains `airscan.bss`: every BSS its scan radio hears that is not its own, the strongest first, 64 at most. Each has its BSSID, SSID, band, channel, signal, whether it is on the wire (or not yet known), on which VLANs, how many times airscan looked, and seconds since it was heard. These count toward the rogue alert, as RRM's `others` do.
- **Rogue clients are the clients a scan radio hears talking through a rogue or suspect AP.**
  - The manager answers each state report with the BSSIDs it takes for rogues and suspects among those the AP heard (`watch`, 64 at most).
  - The AP's next report lists the clients airscan hears with them (`airscan.clients`): MAC, BSSID, frames, seconds since. 64 at most, those with the most frames first, each with 10 frames or more. Fewer are as often a client's MAC garbled in the air.
  - The manager keeps each pair of client and rogue (`rogue_clients`, conditions migration 23): when first and last heard, by which AP, and the most frames seen. Times are whole milliseconds. It is trimmed with the clients' attempts (0118).
  - The list says whether each client is one of ours, one the APs have served (0118), with its name; and which networks block it.
- **Rogue DHCP servers are on any network where more than one DHCP server answered,** as the APs' latest reports say (each remembers a server for an hour, 0065). The list gives each server's address, MAC, answers, when it last answered, and the APs that saw it. Alert: warning, `dhcp-servers`.

## Consequences

- **Only APs with a scan radio check the wire and hear clients:** today, the C-360s. Elsewhere a rogue is a suspect at most, as before.
- **airscan checks only the VLANs its AP carries.** A rogue on another VLAN is not seen on the wire.
- **A rogue's clients are found one report after the rogue.**
- **Which of two DHCP servers is the rogue is for a person to say.** Both are listed.

## Not done

- Marking a DHCP server as expected. Done since (2026-10-24): a network's `dhcp_servers` lists the DHCP servers it has, by address or MAC (a failover pair, say). Where it is set, only a server not among them raises `dhcp-servers`, even one answering alone, which nothing caught before. Where it is unset, two or more answering do, as before. The Threats tab marks each server as the network's own or not, and lists a network only where a stranger answers.
- An alert for one of our clients heard on a rogue. Done since (2026-10-24): `client-on-rogue`, a warning on the AP whose scan radio heard a client the APs have served sending to a rogue or a suspect in the last 15 minutes. It isn't raised for a BSSID that is the APs' own, one its folders' `rogues.known` names, or the APs' test clients. It names the client as people named it, else by its own name, and says to mark the AP known if it is yours. It ends 15 minutes after the client was last heard there.
- Taking clients off a rogue (containment).
