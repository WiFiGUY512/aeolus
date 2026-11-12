# 0297. WPA3 clients kept on WPA3

- Status: Accepted (Griff, 2026-11-03, about 18:45 CST: "do all of that and make it so on home wi-fi")
- Date: 2026-11-03
- Proposed by: Claude, from the Phone-B client matrix (finding F5). Written up by Claude
- Refines: 0086 (WPA3 or OWE on 6 GHz), 0224 (802.11r's key holders), 0227 (protected management frames)

## Context

- **A network in WPA2/WPA3 transition lets every client choose.** Home Wi-Fi is one: three of its devices sign in with WPA2-PSK alone (a Bambu printer, a Canon printer, an ESP32 switch), and the rest with SAE.
- **A client that can do WPA3 may still be talked down to WPA2.**
  - An evil twin offering the SSID as WPA2 alone is joined with the passphrase.
  - A profile saved as WPA2 joins with PSK first. The Phone-B did (T6), until it had once joined with SAE (T8).
- **WPA3 has an answer: Transition Disable** (WPA3 specification §13).
  - In the 4-way handshake the AP tells the client never again to use the older way on this SSID.
  - The client keeps that in its profile. Android shows it as the profile's WPA2 side switched off.
- **OpenWrt has it as a list option, `transition_disable`,** since April 2026 (main, Hauke Mehrtens). The Arista images have it. 25.12's releases do not: the Office AP (OpenWrt One, 25.12.2) and Shed (R7800, 25.12.5) would ignore it.
- **hostapd has had it since 2.10,** as `transition_disable=<bitmap>` in a BSS's own configuration, which every AP's OpenWrt passes through (`hostapd_bss_options`, as 802.11r's key holders already are, 0224).

## Decision

- **`network.<id>.transition_disable`, true or unset.** Set, each client that signs in the newer way is told to keep to it on this SSID.
  - WPA2/WPA3 and WPA3 alone: WPA3-Personal, bit 0x01.
  - WPA2/WPA3 Enterprise and WPA3 Enterprise: WPA3-Enterprise, 0x04.
  - OWE: Enhanced Open, 0x08.
  - Not on WPA2 alone, WPA2 Enterprise or open, which have no newer way; the schema refuses it there.
- **It is written as hostapd's own option, `transition_disable=0x01`,** in the BSS's `hostapd_bss_options`, after 802.11r's where the network has both. So every AP sends it, the 25.12 ones too, not only those with OpenWrt's new option.
- **The manager's render check expects exactly that list:** 802.11r's two options with 802.11r, and Transition Disable where it is set, each alone otherwise.

## Consequences

- **A client that can only do WPA2 is not affected.** It ignores what it does not know, and still joins a network in transition.
- **The setting cannot really be taken back.** A client that was told keeps to it.
  - A network that goes back to WPA2 alone, or open, loses such clients until each forgets the network.
  - Unsetting it stops new clients being told, and nothing more.
- **802.11r roams have no 4-way handshake,** so a client is told when it first signs in at an AP, not on each roam.
- **Home Wi-Fi keeps WPA2/WPA3:** its three WPA2-only devices would be shut out of WPA3 alone. It gets Transition Disable, so its phones and laptops keep to WPA3.
- **Once 25.12's successor carries OpenWrt's own option on every AP,** the renderer could write that instead. It changes nothing on the air.
