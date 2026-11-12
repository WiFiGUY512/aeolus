# 0335. The untagged network: a tunnel's VNI on it, and a port on it

- Status: Accepted (Griff, 2026-11-06)
- Date: 2026-11-06
- Proposed by: Griff, adopting GateOpenWrt, an L3 island whose wire carries nothing tagged:
  - "I dont like the monitoring tunnel albatros, It should just be a tuinnel attached to vlan1 or untagged or whatever"
  - "Its an l3 island and we just want the untagged stuff on it. So we dont need vlan 1 anywhere there right?"
  - "Ok. Do those"
  Written up by Claude
- Supersedes: 0334 (monitoring tunnels)
- Refines: 0053 (port modes), 0055 (tunnels set in Locations), 0152 (a fresh bridge made to filter VLANs)

## Context

- **The gate's wire carries nothing tagged.** lan1 to the MikroTik, a camera on lan2, and the AP's own 10.3.0.8 are all untagged. Its networks go home over VXLAN.
- **Adopted, its bridge filters VLANs (0152).** What was on br-lan is untagged in VLAN 1, which is only the bridge's name for the untagged network.
- **Two things had no way to be said there:**
  - **Helios.** The gate carried its management LAN to Helios over a hand-made VXLAN, VNI 9003, bridged in by a hotplug script. 0334 made that a "monitoring tunnel", a separate concept, which Griff found heavy.
  - **lan4.** The gate's lan4 could not go back on its untagged network. `access` with untagged 1 asks the uplink to carry VLAN 1 tagged, which the gate's never does.

## Decision

### A tunnel carries a VNI into the AP's own bridge

- **The setting:** `concentrators.<name>.bridged.<where>.vni`, on the tunnel itself.
  - `where` is `untagged`, the network the uplink carries untagged, or a VLAN of the bridge by its number, for an AP whose uplink is a trunk.
  - The VNI has at most seven digits.
- **The renderer makes `aeolus_b<vni>`:**
  - A VXLAN interface to the tunnel's far end, bound to no interface and with no MTU of its own, so it takes the bridge's.
    - netifd gives a bridge's ports the bridge's MTU, and a VXLAN bound to an interface can take at most that interface's MTU less its headers.
    - On the gate, whose br-lan is at 2000, netifd took a bound one out of the bridge (v0.294.0, fixed in v0.295.0).
    - Unbound, it joins at the bridge's MTU, and the AP's own network keeps its MTU. Frames larger than the path go in fragments.
  - A port of the uplink's bridge, untagged in that VLAN: for `untagged`, the VLAN the uplink itself is untagged in.
  - A firewall rule, `aeolus_bridged_<vni>`, letting it in.
  - A monitoring server at the far end is on that network as if plugged into it.
- **Not probed.** It isn't one of the tunnels the prober probes (0060), because its segment is the AP's own network.
- **Its VNI is the AP's own.** Compose holds a config where a network, a port, or another such VNI uses it there, or where the tunnel's address isn't an IP address.
- **On the Tunnels page:** each tunnel's card lists what it carries into the bridge, and its form adds, changes or removes them.
- **`monitor.*` (0334) is taken out.** It was never set.

### A port on the untagged network

- **`ports.<p>.mode: native`** puts the port on the uplink's untagged network: in its bridge, untagged in the VLAN the uplink is untagged in, and in no other VLAN. It asks no VLAN of the uplink.
- **A port on a tunnel comes back with it**, as it does with access or trunk (0058).
- **The UI calls it "Untagged network".**

### The gate

- **Helios is `concentrators.helios`:** 192.168.1.44, port 4789, MTU 1450, carrying VNI 9003 on the untagged network. The hand-made `mon` and its hotplug script go.
- **The gate's lan4 is `native`.**
- **Home's `ports.lan4` tunnel moves to `boards.netgear,r7800.ports.lan4`,** so it applies to the R7800 alone.

## Consequences

- **What an L3 island needs is said without a VLAN number.** The house's VLANs stay available where an uplink is a trunk.
- **Helios's tunnel is part of the gate's config,** and survives a sysupgrade with nothing kept by hand.
