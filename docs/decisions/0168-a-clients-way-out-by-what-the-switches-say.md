# 0168. A client's way out, by what the switches say

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff, who asked for "a traceroute that is build via ping scans, ARP, LLDP, device tables cam, mac whatever", to "start with homelab and tower switch". Written up by Claude
- Refines: 0064, 0142, 0154

## Context

- **When a client cannot get out, the first thing asked is where its way breaks,** and the manager could follow it only as far as the AP. The AP's Path tab (0142) shows the AP's own way out, not a client's.
- **A traceroute does not see a switch.** Most of a client's way at a site is layer 2: the AP's uplink, a switch or two, the switch that routes for its VLAN. Only the switches know that part: their LLDP neighbours, the MACs they have learned and on which port, their ARP and their routes.
- **The AP already says where it is plugged in:** its uplink's LLDP names the switch and the port (0064).
- **Griff's switches are Arista** (homelab, the gateway for every lab VLAN, and tower-switch past it). EOS answers show commands as JSON over eAPI, to a read-only account.

## Decision

- **The manager reads the switches set for a folder, `system.switches`:**
  - `addresses`, up to 16, each a name or address, with `:port` where eAPI is not on 443;
  - `user` and `password`, one read-only account that every switch there has (role `network-operator`). The password is sealed.
  - It is the manager's own, as the DNS server's log is (0154): no AP is sent any of it.
  - **Only an admin sets them,** and the DNS server's log too (0154): whoever may say where the manager signs in could send it the password set above.
  - **The three are used together only where they were set together:** on one node, and each set there by a person, or each copied there by Break Hierarchy. Addresses set below where the password is set, or where it was copied to, are not sent it. No template holds them.
- **Each switch is asked eight show commands in one eAPI request:** `show hostname`, `show version`, `show lldp neighbors`, `show mac address-table`, `show ip interface brief`, `show ip route 0.0.0.0/0`, `show ip arp` and `show port-channel dense`. Nothing is set on a switch.
  - **It is asked when someone opens a client's Path, at most once a minute** (once in 15 seconds where it could not be read), and what it said is kept no longer than that. A read under way is waited for, not made again, and a Path closed before a switch answers does not leave it said to be silent.
  - **A switch that sends eAPI elsewhere is not followed,** as the password would go with it.
  - **The switch client also offers TLS 1.2's RSA key exchange,** after Go's own ciphers. EOS's eAPI offers AES256-SHA alone unless the switch is given an SSL profile of its own, and Go no longer offers it unless asked. Only the switch client does; a failed handshake is said as that.
  - **Its certificate is not checked.** A switch's eAPI certificate is its own, signed by no one, and it is on the site's own network.
- **A client's way is walked from what the switches say:**
  1. **Client and AP,** from the AP's latest report: the client's address, band and signal; the AP's uplink.
  2. **The AP's switch,** the one its uplink's LLDP names (by chassis MAC, name, or management address), and the port it is on. The switch's MAC table says whether it learned the client there.
  3. **Along the client's VLAN,** to its gateway: of the switches with an address on the client's network, the one whose ARP has the client, then the one whose interface there is the client's VLAN. A switch with an address there only to be managed by is passed through. At each switch on the way, the port it learned that switch's MAC on, and what LLDP says is on that port.
     - Where the AP does not say the client's VLAN, the gateway's interface does: Vlan20 is VLAN 20.
  4. **By each router's default route:** its next hop, the ARP for it, the port behind it, and what LLDP says is there.
     - A switch read that has the next hop's address is the next router.
     - One that only carries the way to it is followed by the next hop's MAC, as along the client's VLAN.
     - Anything else is the last thing named, and the internet is past it.
  - **It stops, and says why,** where the AP's uplink hears no LLDP, a switch is not one the manager reads, a switch has not learned where the next is, there is no default route or no ARP for it, or the way loops. A way runs 16 hops at most.
- **`GET /v1/clients/{mac}/path`** gives the hops, for whoever may view the client's AP. A client's history and its Live Trace have a **Path** button that opens it in a window:
  - the way drawn (Griff: "can you represent these with pictures?"): each thing a picture of what it is (a phone, an AP, a switch, a switch that routes, a router, the world), joined by its links, the port each leaves by over the link and the port it comes in by under it; the client's own link dashed, with its band and signal; each coloured by how it is. A thing named by nothing but its address is named by who made it, by its MAC;
  - a table of each hop: its address, MAC, the ports in and out, the VLAN, and how each was known (the AP's report, LLDP, MAC table, ARP, route);
  - the switches read, and any that could not be.

## Consequences

- **Where a client's way breaks at a site is seen without a laptop on a switch.** So is a client a switch learned on the wrong port, and a port channel that has fallen back to one member.
- **Only Arista switches are read.** Another make's switch ends the way, named by the LLDP before it.
- **Only the default VRF's route and ARP are read.** A client whose network is routed in another VRF is followed by the default VRF's way out.
- **The tables are as fresh as the switches keep them.** A quiet client may have aged out of a MAC table: it is "not seen" there, and the way is still followed.
- **To come:**
  - names for the things past the last switch, from Helios (NetAlertX);
  - the APs refreshing the switches' tables, by ARP and ping on their VLANs, before a trace. Griff: the APs in a network are its "perfect watchdogs";
  - SNMP for other makes.
