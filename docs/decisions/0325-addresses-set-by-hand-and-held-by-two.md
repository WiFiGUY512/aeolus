# 0325. Addresses set by hand, and held by two

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Griff, of a client's "To look at": a static address is "not really a problem provided all things are proper", and the APs know each VLAN's range, so "we should ignore it if it's in the proper range for that SSID", and flag it if it is not, "because it would tell us why that client is not working". Then: "the only other thing I can think of is if a client is creating an IP conflict". Written up by Claude
- Refines: 0103 (a client's journey), 0189 (client tests get their address by DHCP)

## Context

- **A client's journey notes each session in which it used an address without asking DHCP** (0103): "on Living-Room-C-360 it uses 192.168.20.218 without asking DHCP: a static address".
  - Most clients set by hand are set right. One client's page on 2026-11-05 had over thirty such notes, all for an address on its VLAN's subnet, and said nothing of whether it was right.
- **The APs' client tests take each network's address by DHCP** (0189), its mask with it: through the air on the AP they join, over the wire on the AP itself. They run on a schedule, so each network's subnet on each AP is known.
- **Each AP's report lists its clients with their addresses** (0066), every minute.
- **Rogue DHCP servers are already raised**, as `dhcp-servers` (0065): a server not among a network's `dhcp_servers`, or two answering where none are named.

## Decision

- **An address set by hand is weighed by the subnet its network has where it was:** the subnets the client tests of the last week were given on that AP for that network, or, where none were, on any AP.
  - On one of them: as it should be, and no note.
  - On none: a warning, `static-off-subnet`, naming the subnets. For example: "on C360-AP it uses 192.168.5.7 without asking DHCP, off Home Wi-Fi's subnet there (192.168.20.0/24): set by hand for another network, it reaches nothing".
  - No subnet known: the note as before.
- **An address another client holds now is a conflict:** a warning, `address-conflict`, on each client's page.
  - Only where the client is on now, by its latest session, and the other client is in the latest report of an AP, of the last three minutes, on the same network. Two clients that held one address an hour apart do not conflict.
  - It names the other client and its AP: "it holds 192.168.20.80, and so does printer (7e:2a:ea:04:b1:04) on C360-AP now: two clients with one address each lose traffic. One was set by hand, or a DHCP server gave out an address in use".
  - Only APs the caller may view are looked at.

## Consequences

- **Fewer notes:** a client set by hand to the right address shows none.
- **A client set up for another network**, as a device moved from one SSID to another with its old address, says why it reaches nothing.
- **A conflict is seen while it happens,** on either client's page. One that has ended is not shown; a conflict alert across the fleet is not built.
- **Costs:** the client tests of the week are read from the cache assurance keeps (0285). The conflict check reads the reports of the last three minutes that hold the address, and the latest report of each AP among them.
- **Checked:**
  - internal/journey: on the subnet the note goes; off it, a warning naming the subnet; no subnet known, the note stays; another holder, a warning naming it.
  - internal/api, through GET /v1/clients/{mac}: a client test given 192.168.20.108/24 on sweet. 192.168.20.218 set by hand: no note. 192.168.5.7: off the subnet. The phone and the printer on 192.168.20.80: a conflict on each page, gone once the phone is not in the AP's latest report.
