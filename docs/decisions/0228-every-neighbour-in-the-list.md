# 0228. Every neighbour in the list

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from the roaming readout's list of what would speed roams (0225, 0226); Griff: "Continue the list of improvements". Written up by Claude
- Refines: 0050 (band steering through usteer), 0138 (steering a network at a time)

## Context

- **A network's 802.11k neighbour list is usteer's.** hostapd gives a client the list when it asks, and Aeolus's steering daemon names its targets from it (0138).
  - usteer sets each BSS's list every second, from the BSSes of the same SSID it knows: its own AP's and those other APs' usteer tells it of.
  - It names at most `max_neighbor_reports` of them, 8 unless set. It ranks them by how often clients roamed between, then by BSSID.
- **In the lab every list was cut to 8.** On 2026-10-28, every one of ShedAP's eight BSSes listed exactly 8.
  - Home Wi-Fi is on 15 BSSes across six APs, so more than half were left out.
  - Shed's lists lacked Living Room, the C-260 and the C-360's 6 GHz. A client there that asked where to go was never told of them, and the daemon could not send it to them.
  - Those left out are the ones clients have not roamed to yet, and often the ones a client would be better off on: a 6 GHz BSS, or a new AP.
- **A longer list costs little.** A client scans the channels it names, and many BSSes share a few channels. A neighbour report response holds far more than 24.

## Decision

- **Aeolus owns a third of usteer's options: `max_neighbor_reports`, 24.**
  - That is every BSS of a network on a site of eight APs on three bands.
  - usteer still trades neighbour reports between the APs, and still ranks the list.
  - The other two options stand: `band_steering_interval` 0, and `ssid_list` empty (0050, 0138).
- **The render check holds the option to 24,** as it holds the other two. An AP without usteer is none the worse.

## Consequences

- **Each network's list names every BSS of it that usteer knows,** up to 24, on all bands. That includes the 6 GHz BSSes an AP with no 6 GHz radio would never have named.
- **A site of more than eight tri-band APs still has its lists cut,** to the 24 neighbours clients roam to most. Lists made from the radio neighbours Aeolus measures, rather than from usteer's, would be the next step. That needs usteer to stop setting them.
- **The render changes on every AP with usteer.** usteer reloads its config, and nothing in the air is restarted.
