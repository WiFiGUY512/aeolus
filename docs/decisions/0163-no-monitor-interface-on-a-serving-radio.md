# 0163. No monitor interface on a radio that serves

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff, choosing "leave the AP alone and fix the cause" ("do 1") over flashing the C-360 with a diagnostic Ethernet driver. Written up by Claude
- Refines: 0135, 0146

## Context

- **From 2026-10-12 the first C-360 restarted, or stopped receiving on Ethernet, several times a day.** The Ethernet driver (qca-nss-dp, EDMA v1) found the first word of its receive buffers written over: it read outside its store and the kernel stopped, or it took the wrong buffer and receive died until a reboot.
- **A kernel trace named the writer:** an skb that ath11k's monitor receive path had just built, its shared info full of frame payload. ath11k's monitor receive on this build wrote past the end of its buffers into memory beside them.
- **The monitor interfaces were the agent's,** one on each radio that serves, for failure captures (0135).
- **Failure captures have come from hostapd's own rings since 0146,** and the agent makes no monitor interface: it deletes any an older release left. The first C-360 also had captures turned off on 2026-10-13 (change 209).
- **Since then it has stayed up:** 53 hours by 2026-10-15, with no Ethernet warning, where before the longest run was 7.6 hours. The second C-360 has been up 46 hours.

## Decision

- **The agent makes no monitor interface on an AP's radios.** A Go test reads every agent file and fails on one that does.
- **The C-360 image carries the Ethernet driver hardened** (the port's qca-nss-dp patch 900), so that a stray write into a receive buffer drops a packet and says so, and does not stop the AP or its port.
- **airscan's monitor interface on the C-360's scan radio stays.** It is the port's, not the agent's, on the radio that serves no one, and it was there through both long runs.

## Consequences

- **Change 209 can be undone:** captures on the first C-360 would now come from hostapd's rings, with no monitor interface. That is Griff's to choose.
- **A capture of the air itself** (planned: RF capture on the multifunction radio) goes on the scan radio, never on a radio that serves.
- **The diagnostic driver was never loaded:** a live swap leaves the AP with no Ethernet until a reboot, and with the monitor interfaces gone it would have nothing to report.
