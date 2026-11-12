# 0152. A fresh bridge is made to filter VLANs

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff, of a second C-360 that joined by itself and, adopted, would not take its config ("this AP cannot render it: bridge br-lan does not filter VLANs; 802.1Q devices on the uplink are not rendered yet"): "please fix that". Written up by Claude
- Refines: 0040, 0053, 0151

## Context

- **Aeolus's networks are VLANs of the uplink's bridge** (0040): a network on VLAN 20 is the bridge's VLAN 20, tagged on the uplink, with the network's Wi-Fi in it. That needs a bridge that filters VLANs, one with `bridge-vlan` sections.
- **A fresh OpenWrt's bridge does not.** It is one network with no VLANs: `br-lan` with its ports, and the AP's own interface on the bridge itself. The renderer refused such an AP, and nothing of its config was applied.
- **The lab's APs were changed by hand before they were adopted:** ShedAP has `bridge-vlan` sections of its own making, and the first C-360 was changed with a script on 2026-10-09. The second C-360, 2026-10-13, was the first adopted as installed, and it was refused.
- **An AP that finds its manager by itself (0151) is of no use if it then needs a shell.**
- **What the hand-made layout is, on ShedAP:** every port of the bridge untagged in VLAN 1, and the AP's own interface on `br-lan.1`, where it takes its address by DHCP as before.
- **A bridge is itself in VLAN 1 unless told otherwise,** so what the AP sends on the bridge still goes out untagged where VLAN 1 is the untagged one.
- **The agent applies a config with a way back** (0040): if the manager cannot be reached within 90 seconds after, the AP returns to what it had.

## Decision

- **The renderer has the uplink's bridge filter VLANs, where it does not.** It is part of the config the AP renders, checked by the manager and applied with the way back, as any other.
- **What the bridge was stays as it was:**
  - each of its ports is untagged in VLAN 1, in a `bridge-vlan` section named `vlan1`;
  - each interface that was on the bridge is on that VLAN of it (`br-lan.1`), and is otherwise unchanged.
- **The section is the AP's own from then on,** not one of Aeolus's: nothing takes it away again, and a later render finds a bridge that filters VLANs and leaves it.
- **A network on VLAN 1 is then the untagged one,** as on an AP laid out by hand.
- **It is not done, and the AP is refused as before, where:**
  - something is already on an 802.1Q device of the bridge (`br-lan.30`), which would carry nothing once the bridge filtered VLANs;
  - a port of the bridge is itself a VLAN of something (`eth0.1`), as on a switch without DSA.

## Consequences

- **An AP as OpenWrt installs it can be adopted.** With 0151, a C-360 with the finder in its image is plugged in, appears in Landing Zone, and takes its folder's networks when a person moves it there.
- **Adopting an AP changes its network once,** for some seconds, as its own interface moves to the bridge's VLAN and takes its address again.
- **What is plugged into the AP's other ports stays on its network.**
- **Checked, 2026-10-13:** rendered with ucode on ShedAP, a fresh C-360's network came out as ShedAP's own is laid out, and the manager's check passed it; the other 41 cases came out as before. Not yet applied on an AP.

## Not done

- **The untagged VLAN's number is 1,** whatever the switch calls the VLAN it sends untagged. `system.management.vlan` is still not rendered.
- **An 802.1Q device on the uplink is still not rendered.**
