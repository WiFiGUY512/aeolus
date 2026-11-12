# 0279. Enterprise says it has no SAE

- Status: Accepted (Griff, 2026-11-01: "Write sae_pwe=0")
- Date: 2026-11-01
- Refines: 0224 (802.11r keys over the wire)

## Context

- **No 802.11r roam to Office worked on Aeolus-Enterprise** (0 of 11 in the client tests, 2026-11-01). Office is the OpenWrt One on stock 25.12.2. The same roams between the Arista APs worked, and so did Home Wi-Fi's to Office.
- **Aeolus's part held:** the same FT settings were rendered on every AP, and the first AP pushed the PMK-R1 to Office's BSSIDs, which Office took (captured on br-lan.20, ethertype 0x88b7).
- **Porting's capture (B128) found it:**
  - Office answered the Reassociation Request with status 0, but its FTE said "RSNXE used" with no RSNXE in the frame or its beacons.
  - wpa_supplicant refuses that, and deauthenticated 57 ms on with reason 13 (invalid IE).
  - The client then signed in again in full, PEAP and all, at the AP it came from.
- **The cause:** hostapd sets that flag wherever `sae_pwe` is 1 or 2, whatever the AKM. OpenWrt 25.12's wifi-scripts set `sae_pwe=2` on `wpa3-mixed`, `wpa3` and the other WPA3 types, Enterprise among them, where there is no SAE.
  - Main's scripts set it only for SAE, which is why the Arista APs never hit it.
  - Porting fixes hostapd for upstream and in port 0071. Stock 25.12 users get it only in a point release.
- **This hits every Aeolus Enterprise network with 802.11r on stock OpenWrt 25.12,** and stock hardware is what most people run.

## Decision

- **A WPA Enterprise network with 802.11r is rendered with `sae_pwe '0'`,** in its wifi-iface.
  - The scripts set their default only where none is set, so 0 holds.
  - It means nothing on a network with no SAE: no SAE, no password element.
- **The manager's render check wants it there, and nowhere else.**
- **A stopgap** (OpenWrt fault 67), to go once 25.12 carries the fix.

## Consequences

- **802.11r roams on WPA Enterprise work to stock 25.12 APs,** with no full sign-in each time.
- **Releasing it restarts each AP's Enterprise network once,** where the option is new. Griff took that.
- **Checked:**
  - the agent's renders on talos (only the ft case changes: sae_pwe '0' on its three Enterprise interfaces);
  - the manager's render check;
  - Office's own scripts: `set_default` sets only a null, and the wireless schema takes sae_pwe as 0, 1 or 2.
