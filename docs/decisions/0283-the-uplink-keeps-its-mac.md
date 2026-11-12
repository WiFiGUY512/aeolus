# 0283. The uplink keeps its MAC

- Status: Accepted (Aeolus's own development, 2026-11-01; found by the install test Griff asked for)
- Date: 2026-11-01
- Refines: 0061 (a network with a fallback), 0121 (the client test), 0274

## Context

**The install test.** A stock OpenWrt 25.12.6 x86-64 VM on powermox (VM 123, VLAN 1 untagged, 10 and 20 tagged) was set up as a newcomer would: the one-line `/install`, then adopted into Home's `test` folder (change 606). Three things came of it.

1. **The manager's name didn't resolve on the AP.** dnsmasq's rebind protection, on by default, refused `aeolus.example.com`, which answers with 192.168.20.60, a private address. The one-liner works by IP; given the name, it fails before anything of Aeolus runs.
2. **After the first apply, the AP took another address and was refused.**
   - `test`'s network `keytest-vx` has a VLAN fallback, so the renderer put the VLAN end of a veth pair in br-lan (0061).
   - The kernel gives a veth a random MAC. A bridge takes the lowest MAC of its ports unless it's given one, and the veth's (ae:e2:c3:…) was lower than eth0's (bc:24:11:e4:08:bd).
   - So br-lan, and br-lan.1 with it, changed MAC. DHCP gave the AP another address (.17 to .20), and its facts went up with a MAC the manager doesn't know it by: "HTTP 400 mac: the one this AP enrolled with".
   - A real AP can do the same, wherever a network with a VLAN fallback joins its uplink and the veth's random MAC is lower than its port's.
3. **Every client test stopped before it began.** Since 0274 the client test imported ucode's rtnl, and the VM, with no Wi-Fi scripts, has no rtnl.so. "Unable to resolve path for module 'rtnl'" stopped even the tests over the wire, which never need it.

Everything else went as it should:
- the certificate's fingerprint printed and pinned;
- the agent installed, the uplink (eth0) found by itself, the feeds pointed at the manager;
- the one module stock 25.12.6 lacks (ucode-mod-digest) installed;
- the network restarted once for vxlan;
- the AP waited in Landing Zone, was adopted, and applied its version with its tunnel up.

## Decision

- **Where the renderer puts a veth's end in the uplink's bridge, the bridge keeps the AP's own MAC,** the one it enrolled with, read from its ID (`ap-bc2411e408bd`). It sets `macaddr` on the bridge's device, unless something gave it one already.
- **The client test loads rtnl only for a test through the air,** for the client's routes (0274). Without it, such a test stops at its address, saying ucode-mod-rtnl isn't installed. Tests over the wire don't need it.
- **The install guide says to use the manager's IP address,** and why.

## Consequences

- **An AP keeps its management MAC and address when a network with a VLAN fallback comes,** and its facts are taken.
- **The real APs change nothing now:** none has a veth in its uplink.
- **An AP without Wi-Fi scripts runs its tests over the wire.**
- **Known, not changed:** a test over the wire finds its network through the AP's Wi-Fi interface for it, so an AP with no radio says "this AP does not offer that network". Only a VM has no radio.
- **Checked:**
  - On the VM: the new renderer applied version 606 with br-lan's macaddr bc:24:11:e4:08:bd, br-lan took it back, and the facts errors stopped.
  - The new client test loaded with no rtnl.so and ran its tests over the wire to their own verdict.
  - The agent's renders on talos: only the two cases with a VLAN fallback (rrm, vxlan) gain the bridge's macaddr.
  - The manager's render check passes, and it compiles on Shed's 25.12 ucode.
