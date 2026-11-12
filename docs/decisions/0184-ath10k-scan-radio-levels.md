# 0184. What an ath10k scan radio hears, as it hears it

- Status: Accepted (Griff, 2026-10-20: "go ahead with the agent fix")
- Date: 2026-10-20
- Proposed by: Griff, on the neighbour map: "man something is weird here". Written up by Claude
- Refines: 0073, 0116, 0161

## Context

- **The C-260 heard every neighbour 11 to 21 dB louder than they heard it:** for example, the office AP at −23 against −39 back, and the living-room C-360 on 5 GHz at −56 against −82 back. The C-360s' scan radios (ath11k) agree with each other within a few dB.
- **The cause is its scan radio's driver, ath10k-ct** (QCA988x, `kmod-ath10k-ct` 2026.07.31, firmware 10.1-ct). Its module parameter `ofdm_peak_power_rssi` is unset by default, and unset turns it on. It then adds 8 dB on 2.4 GHz and 12 dB on 5 GHz to every management frame's signal, scan results included, to match OFDM peak power. No other radio here reports peak power.
- **It can be turned off while the radio runs,** in debugfs (`/sys/kernel/debug/ieee80211/<phy>/ath10k/ofdm_peak_power_rssi`), and goes back on when the driver starts again.
- **Turning it off on the C-260 fixed it.** Its next look heard the office AP at −34 on 2.4 GHz (−39 back) and −51 on 5 GHz (−50 back), and the shed at −83 (−84 back). The pairs left a few dB apart are where the scan radio and the serving radios have different antennas.
- **Upstream ath10k would not do better:** its levels are SNR plus a fixed −95 dBm, against this radio's measured floor of −102. And QCA988x's stock firmware has a history of crashing in monitor mode, which airscan uses.

## Decision

- **RRM keeps an ath10k-ct scan radio's `ofdm_peak_power_rssi` off.**
  - At each look it reads the setting for the phy airscan names, and turns it off where it is on. A reboot or a driver reload puts it back, so it is not set once and left.
  - It logs when it turns it off. If it can't, it says why.
- **A look taken before then is not taken in,** and each neighbour's level starts again from the next look, rather than being averaged with the levels from before.
- **Other radios are left alone:** ath11k scan radios, and APs without a scan radio.
- **The Arista port is told** (the "OpenWRT porting" session): an image could ship the setting off as a module option. The agent's own check stays either way, as it costs nothing where the setting is off already.

## Consequences

- **Neighbour levels from a C-260, and from the W-118 and C-130 once ported,** match what their neighbours hear back, within the antennas' difference. Channel planning and the neighbour map stop treating such an AP as nearer than it is.
- **Its client readings in monitor mode, which airscan reports, drop by the same 8 to 12 dB.** Those readings were already 17 to 28 dB under what the C-260's own serving radio hears from the same clients. That is a separate fault in the driver's data path, still open.

## As built (2026-10-20)

- **The code:** `flat()` and its call in `sweep()`, in `agent/files/usr/sbin/aeolus-rrm`, and `flat_at` and each neighbour's `fresh` level.
- **Tested:** on the C-260 itself, before the change: the setting turned off by hand, and the next look compared, as above.

## Serving radios too (2026-10-20)

- **Griff: "extend it to the shed".**
  - **The serving radios:** the shed's two QCA9984 radios run the same driver with the setting on. Its clients' levels and the neighbour levels from its own scans read up to 8 to 12 dB high.
  - **Data frames:** the driver adds to these as well (`htt_rx.c`, by spatial streams: 8, 4, 3 or 3 dB on 2.4 GHz; 12, 12, 10 or 10 on 5 GHz).
- **RRM now keeps the setting off on every ath10k radio the AP has:**
  - when: at each look, every 10 s, whether RRM is on or not;
  - the levels this covers: the scan radio's, a serving radio's scans, and its clients'.
- **What was heard before a switch is not taken in:**
  - a look of the scan radio's from before it;
  - a serving radio's scan begun before it.
  
  Each neighbour's level then starts again from the next.
- **Tested:**
  - on the shed's driver: the check turned off the setting on phy0 and phy1, and found nothing to do the second time;
  - `aeolus-rrm` compiles;
  - the agent's tests pass.
