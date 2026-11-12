# 0209. No AP address on a network its own Wi-Fi is on

- Status: Accepted
- Date: 2026-10-23
- Proposed by: Griff: "also double check to make sure any AP to AP communications possible go through the wire, not the airspace"; then, of moving C360-AP's management off VLAN 20 and a warning for it, "yes move C360-AP to VLAN 1 and add the warning". Written up by Claude

## Context

- **What the APs say to each other goes over the wire.**
  - Hellos (0073) go to each neighbour's management address, unicast.
  - usteer broadcasts on its network, `lan`.
  - 802.11r makes its keys on each AP (`ft_psk_generate_local`) and goes over the air only between a client and an AP (`ft_over_ds=0`), so no AP-to-AP exchange. Since 0224, the keys of WPA3 and WPA Enterprise go AP to AP, over the wire, unicast.
  - Tunnels go from each AP to its concentrator.
  - Of what is AP-to-AP, only the beacon advert (0161), 19 bytes on one network's beacons per radio, rides the air, and it says where the AP is on the wire.
- **What makes that so is where the AP's address is.** Five of the six lab APs were managed on VLAN 1, which only their uplinks carry. Twenty seconds' capture on every Wi-Fi port of two of them found no hello, no usteer and no ARP for an AP leaving by the air.
- **C360-AP was managed on VLAN 20,** with Home Wi-Fi and Aeolus-Enterprise on five of its BSSes.
  - What it broadcast there, an ARP for its gateway or anything else, went out over the air to their clients, and they could reach its management address.
  - Its usteer, on `lan`, which had no address there, spoke with no one; pointed at VLAN 20, its broadcasts would have gone out over the air.
- **OfficeOpenWrt still holds a DHCP address on VLAN 10,** `wifi_iot`, from before Aeolus, where Home_Wi-Fi_IoT is (and one on VLAN 50, which no Wi-Fi of its is on).

## Decision

- **C360-AP is managed on VLAN 1, as the others are** (by hand, 2026-10-23).
  - VLAN 1 is untagged on its uplink, `vlan1` with `lan1:u*`.
  - `lan` is a DHCP client on `br-lan.1`.
  - `vlan20` is gone.
  - Its `lan` DHCP server, DHCPv6 and router adverts, OpenWrt's defaults, are turned off before `lan` comes up there, as on the others.
  - Done by a script that put it all back unless told to keep it within five minutes.
- **The agent says where the AP holds an address on a network its own Wi-Fi is on.**
  - For each interface with an IPv4 address that is a bridge's VLAN, or a bridge, it reads the Wi-Fi interfaces among that VLAN's ports (or the bridge's members, where it filters no VLANs). netifd lists them as it lists them in the VLAN.
  - It reports them, `on_air`, by interface, VLAN and SSID, and whether the interface is the management one.
- **The manager warns of each,** `shared-with-wifi`, a warning:
  - for the management interface, to manage the AP on a VLAN no Wi-Fi network is on;
  - for another address, to take it off or move it.

## Consequences

- **C360-AP is now on VLAN 1, at 192.168.1.186, by DHCP.**
  - Its 19 clients stayed on.
  - Its usteer now speaks with the others on VLAN 1, and its hellos come from the new address.
  - Its agent made its aeolus_50 tunnel start from `lan` once it was restarted. The agent renders again only for a new config, or when it starts: a management interface changed by hand is taken up then.
- **Home Assistant's link to its Bluetooth proxy (0156) was by address,** 192.168.20.7. It was set to the new one, and took the advertisements again at once.
  - The proxies are reached over the router, and Home Assistant finds none by mDNS across VLANs. Each is added by its address, best kept by a DHCP reservation.
- **OfficeOpenWrt will warn of `wifi_iot` on VLAN 10** until the address is taken off; it is in no firewall zone, so the default `REJECT` keeps its clients from its services.
- **The manager must have this before the APs report it:** it refuses a report with a field it does not know. One release carries both, the manager updated first.
- **airscan's wired probes (0160) still go out over the air on purpose,** to find which networks are bridged to the wire: an ARP to 169.254.255.\<VLAN\> on each VLAN, from each AP with a scan radio, about 3.6 a second on each VLAN in the lab. airscan is the port's, not Aeolus's.
