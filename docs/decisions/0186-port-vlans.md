# 0186. A port's VLANs, and what the AP is on each

- Status: Accepted (Griff, 2026-10-20, issue #60)
- Date: 2026-10-20
- Proposed by: Griff:

  > "Carries, should be port, and we need another line under that that says Access or Trunk and if Trunk it needs every vlan seen on that and the mac address its showing on that vlan and if it has an ip each IP it picked up"

  Then:

  > "1 mac the AP uses to talk to that vlan 2 all of the vlans visible coming from the switch 3 no on folders, that was not a good idea 4 dont worry about the tunnels yet"
  >
  > "If the ap is participating in are injection is will pull an ip"

  Written up by Claude.
- Refines: 0064, 0061, 0063

## Context

- **A port's details are spread out.** Selecting a port showed its Mode ("Uplink", or "Access", "Trunk", "Tunnel"). Carries said what Aeolus sets on it; for the uplink, that Aeolus leaves it alone, with chips for the VLANs it watches.
- **What the AP is on each VLAN was nowhere,** except under Info: one MAC for the whole port, and only tunnel starts and probe leases as its addresses.
- **On the APs:**
  - each VLAN on the uplink has a VLAN device on the uplink, or on the bridge it is in (`br-lan.20`, …), as `/proc/net/vlan/config` lists them;
  - those devices take the bridge's MAC;
  - an address is held only where an interface on that device has one, mostly the management VLAN;
  - a probe of a VLAN transport (0061) takes a DHCP lease from a MAC of its own.

## Decision

- **"Carries" is "Port"** on every page.
  - On a folder, the row that said what the port can do (also "Port") is "Hardware".
- **On an AP's page, an uplink, access or trunk port has a line under Port,** labelled "Access" or "Trunk", in place of its Mode. It lists each VLAN with:
  - whether it is untagged;
  - the MAC the AP talks with there;
  - every address it holds there, of three kinds:
    - its own, as netifd has them;
    - its probes' leases, with the probe's MAC (Griff: "if the AP is participating in … injection it will pull an IP");
    - where its tunnels start (0063).
  - A VLAN the AP doesn't carry says "not carried here".
- **Which VLANs:**
  - **The uplink:** every VLAN the switch sends it, as LLDP says, with those the AP carries or watches. It is a trunk where any of its VLANs is tagged or the switch sends more than one.
  - **An access or trunk port:** the VLANs set on it.
- **Not on folders,** and not for tunnel ports yet.
- **The agent says, for each VLAN in `uplink_port.vlans`,** its `mac` and `addresses`, prefix length and all. The prober's leases say the `mac` they are held with. The manager checks both.

## Consequences

- **What the AP is on each VLAN shows on its port,** where a person looks for it. "Not carried here" shows where the switch sends a VLAN the AP doesn't take.
- **The report gains fields,** so the manager must be updated before the agents. A release does that by itself: the manager restarts first, then the APs take the agent from it.

## As built (2026-10-20)

- **The agent:** `on_vlans()` in `aeolus-agent`, called from `uplink_port()`; the lease's `mac` in `aeolus-prober`.
- **The manager:** `carriedVLAN.MAC` and `.Addresses`, and `lease.MAC`, with their checks.
- **The page:** `onVlans()` and `uplinkVids()` in `views/interfaces.js`, and the port detail's rows.
- **Tested:**
  - **`on_vlans`** run read-only on the shed (`wan`) and the C-260 (`lan1`). Both gave VLAN 1 untagged with the management address and the bridge's MAC, and VLANs 10 and 20 tagged with that MAC and no address.
  - **The report test:** the new fields are kept as sent, and a bad MAC, an address without its prefix length, and a bad lease MAC are refused.
  - **On a local manager:** the shed's uplink reads Trunk, with:
    - VLAN 1 untagged, its MAC and 192.168.1.38/24;
    - VLAN 10, with no address;
    - VLAN 20, with the bridge's MAC and the probe's, and 192.168.20.80 (probe);
    - VLANs 30 and 50 not carried here.

    The folder's view of the same port has Hardware, Speed, Mode and Port, and no table.
