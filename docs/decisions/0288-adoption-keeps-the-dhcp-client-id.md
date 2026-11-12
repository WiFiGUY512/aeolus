# 0288. Adoption keeps the DHCP client id

- Status: Accepted (Aeolus's own development, 2026-11-02; found when the C-200 was adopted)
- Date: 2026-11-02
- Refines: 0152 (a fresh bridge is made to filter VLANs), 0283 (the uplink keeps its MAC)

## Context

- **The C-200 lost its reservation when it was adopted.** It had 192.168.1.28, reserved for it in the DHCP server. Adopted into `test` (change 613), it came back at .29, and the reservation had to be moved by hand to the client id it sent after.
- **Why:** OpenWrt 25.12's DHCP client sends an RFC 4361 client id, which `proto_dhcp_get_default_clientid` makes from the device the interface is on:
  - `ff`;
  - an IAID, the first eight hex digits of the MD5 of the device's name;
  - the AP's DUID, `network.globals.dhcp_default_duid`.
- **0152 moves the AP's own interface from `br-lan` to `br-lan.1`,** where the bridge is made to filter VLANs. The name changes, so the IAID does: `eb351e8c` on `br-lan`, `e5640d66` on `br-lan.1`. The MAC stays (0283), but to the DHCP server the AP is another client.
  - The lease for .28 was the old client id's, so the server gave the new one another address.
  - A server that keys its reservations by client id stops matching altogether.
- **It happens once, at adoption,** and only on an AP whose bridge didn't filter VLANs before: an AP as OpenWrt installs it. A person who reserves an address for a new AP does so before adopting it, so it's the reservation that's lost.

## Decision

- **Where the renderer moves an interface from the bridge to its VLAN (0152), the interface keeps the client id it sent on the bridge.** It sets `clientid` to the one OpenWrt made from the bridge's name: `ff`, the IAID of `br-lan`, the DUID.
- **Only where OpenWrt makes that id itself:**
  - the interface's proto is `dhcp`;
  - it has no `clientid` of its own;
  - `sendclientid` is unset or `auto` (with `hardware` it sends its MAC, which doesn't change; with `none` it sends nothing; `global` ignores `clientid`);
  - the DUID is there and reads as OpenWrt reads it (`hexdump_2hex`).
- **The MD5 is from ucode's digest module,** which the agent already needs. Without it, the renderer still loads and sets nothing.

## Consequences

- **An AP adopted with a reservation keeps its address,** as it keeps its MAC (0283).
- **The APs adopted already change nothing:** their bridges filter VLANs, so 0152 doesn't run again, and their reservations are for the id they send now.
- **The option is the AP's own from then on,** like the bridge's VLAN section.
- **The install guide says an AP keeps its address at adoption.**
- **Not done:** DHCPv6 (`lan6`). OpenWrt's script gives its client the DUID alone, with no IAID from the device's name; whether the address it's given changes with the move was not checked.
- **Owed:** a live check, on the next fresh install (VM 123's stable test).
- **Checked on talos with CI's ucode:**
  - every render case comes out as before, except `fresh-bridge`, which gains `clientid 'ffeb351e8c…'`, the id the C-360 sent on `br-lan`;
  - a new case, `fresh-bridge-hardware-id`, moves an interface that sends its MAC and sets nothing;
  - the contract test works the id out again in Go, from the fixture's DUID, and wants it on each interface 0152 moved.
