# 0334. Monitoring tunnels: the management network, whole, to a monitoring server

- Status: Superseded by 0335 (Griff, 2026-11-06: "I dont like the monitoring tunnel albatros, It should just be a tuinnel attached to vlan1 or untagged or whatever"). Released in v0.292.0 and never set; v0.294.0 takes `monitor.*` out
- Date: 2026-11-06
- Proposed by: Griff, bringing GateOpenWrt into Aeolus: "I think we should make an aeolus tunnel for monitoring. I have it on the gate and did have it on the rutm50". The gate's tunnel "is used for the monitoring server … it needs to be in l2". Written up by Claude
- Refines: 0054 (VXLAN transports), 0055 (tunnels set in Locations), 0058 (tunnel ports)

## Context

- **The gate carries its management network to Helios over a VXLAN tunnel, by hand.**
  - The tunnel is `network.mon`: VXLAN VNI 9003 to Helios at 192.168.1.44, MTU 1450.
  - A hotplug script, /etc/hotplug.d/iface/99-mon-bridge, puts it in br-lan once it is up.
  - /etc/sysupgrade.conf keeps that script over a sysupgrade.
  - Helios is then on the gate's management LAN at layer 2: the MikroTik, the camera on lan2, and the AP itself.
  - The RUTM50 had one too.
- **Aeolus had no way to say this.** Its tunnels carry networks (0054) or wired ports (0058), not the management network. An Aeolus-managed AP would carry a hand-made tunnel only as long as nothing touched it.

## Decision

- **A Locations setting, `monitor.<name>`, with `tunnel` and `vni`:**
  - `tunnel` names a tunnel set in `concentrators` (0055), for the far end's address, port and MTU.
  - `vni` is the tunnel's own VNI, at most seven digits.
  - Like any Locations setting, it can be set on a folder or an AP and inherits down.
- **The renderer makes it `aeolus_m<vni>`:**
  - A VXLAN interface to the tunnel's address, started from the management interface.
  - A port of the management interface's bridge, untagged in its VLAN where the bridge has VLANs, as after adoption (0152).
  - A firewall rule, `aeolus_monitor_<vni>`, letting the tunnel in from the far end.
  - The name fits Linux's 15 characters with a seven-digit VNI.
  - It is not one of the tunnels the prober probes (`aeolus_<vni>`, 0060). Its segment is the management network, whose own DHCP server and gateway would answer whatever the tunnel did.
  - The agent's tunnel mending (0187) covers it as it covers every VXLAN interface.
  - When one is no longer wanted, the renderer takes it out of every bridge and VLAN.
- **Compose keeps the named tunnel, and holds the AP's config where:**
  - the tunnel is not set at the AP, or its address is not an IP address;
  - a network, a port or another monitoring tunnel at the AP uses the same VNI. Two tunnels on one VNI and port cannot both start.
- **The render check checks:**
  - the interface: its address, VNI, port and MTU, started from the management interface;
  - its place in the management bridge and VLAN;
  - its firewall rule.
- **Wired › Tunnels lists the monitoring tunnels:** each one's tunnel, VNI and where it is set. Each tunnel's card lists it among what travels over it. One set on the node can be added, changed or removed there, as a draft of the page's save.

## Consequences

- **A monitoring server can be on any Aeolus AP's management network,** set once on a folder, and it survives sysupgrades without anything kept by hand.
- **The gate's `mon` and its hotplug script give way to `monitor.helios`** when the gate is adopted, with VNI 9003 to Helios.
- **The management bridge takes the tunnel's MTU** (1450 by default), as the gate's does today.
