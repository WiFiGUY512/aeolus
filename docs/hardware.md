# Hardware

Aeolus is for any AP that runs **OpenWrt 24.10 or later**. Nothing about it is tied to one vendor. What an AP can do under Aeolus depends on its radios.

## What every AP does

- Every network setting Aeolus has: SSIDs, WPA2/WPA3 Personal and Enterprise, per-user keys, VLANs and VXLAN tunnels, fast roaming (802.11r) and band steering.
- Channels: Aeolus plans them by listening on the serving radios between beacons. Power too, where automatic power is turned on.
- The client view: who is on, their signal and rates, how each joined, why each left.
- Tests over the wire: from its uplink, on each network's VLAN or tunnel, an AP asks for an address by DHCP, its gateway by ARP and a name by DNS.
- Alerts, the change log, and agent updates from the manager.

## What an AP with a scan radio adds

Some APs have a third radio that serves no clients: a **scan radio**. On those, Aeolus also:

- **Tests as a client would:** the scan radio joins a network on another AP, through the air, and times each step: finding it, the keys, DHCP, the gateway, DNS, speed, and roams between APs.
- **Surveys the air** all the time: neighbours, rogues and spectrum, without taking a serving radio off its channel.

## Tried

| AP | OpenWrt | Radios | Scan radio | 6 GHz | Bluetooth |
|---|---|---|---|---|---|
| OpenWrt One | 25.12 (stock) | 2.4, 5 GHz (MediaTek MT7981) | no | no | no |
| Netgear R7800 | 25.12 (stock) | 2.4, 5 GHz (Qualcomm IPQ8065, ath10k) | no | no | no |
| Arista C-360 | the Arista OpenWrt port | 2.4, 5, 6 GHz (IPQ8074 + QCN9074) | yes, a second QCN9074 | yes | yes, as a Home Assistant proxy |
| Arista C-260 | the Arista OpenWrt port | 2.4, 5 GHz (IPQ8074) | yes, QCA9882 (ath10k) | no | yes, as a Home Assistant proxy |

The Arista units run an OpenWrt port built for them. Its fixes go to OpenWrt upstream.

## Faults found along the way

Bugs found in OpenWrt itself while building Aeolus are listed in [openwrt-faults.md](openwrt-faults.md): each with what it does to Aeolus, any stopgap, and where it was fixed.
