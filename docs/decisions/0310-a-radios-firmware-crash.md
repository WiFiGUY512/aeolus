# 0310. A radio's firmware crash

- Status: Accepted
- Date: 2026-11-05
- Proposed by: the OpenWrt porting session, from its B17 test on the Garage and C360-AP. Written up by Claude
- Refines: 0216 (a radio with its networks not on the air)

## Context

- **Porting's B17, 2026-11-05:** a forced firmware assert on a C-360's QCN9074 (ath11k) is not recovered.
  - The firmware reloads, but mac80211's restart times out waiting for ath11k. It drops the radio ("Hardware became unavailable during restart"), and the radio stays down until its driver is bound again.
  - Touching the dead radio again oopsed the Garage's kernel, and it rebooted at 00:43. C360-AP hung in its reboot.
- **The kernel says each step:**
  - ath11k: `ath11k_pci 0001:01:00.0: firmware crashed: MHI_CB_EE_RDDM`.
  - ath10k: `<device>: firmware crashed!`.
  - mac80211, where the radio did not come back: `ieee80211 phyN: Hardware became unavailable during restart`.
- **Aeolus said none of it.** A radio left down shows only as its networks not on the air (0216), after 4 minutes, with no cause. A crash it came back from showed as nothing, though every client of the radio was dropped.

## Decision

- **The agent reads the kernel's log for those lines** (`aeolus.fwcrash`) and reports each crash while the log holds it (`fw_crashes`, the latest 8):
  - how long ago, by the kernel's own clock (its seconds since start), so the log's dates and the AP's clock don't matter;
  - the driver, the device, and why, where the driver says;
  - whether the radio came back: not where a phy of that device, or one no longer known, was lost within 60 seconds after the crash;
  - the radios of the device with their bands, from the wireless config's `path`. A SoC's second radio on one device is its path plus `+1`, as OpenWrt names it, so a crash of an IPQ8074's built-in radios names both.
- **Read on the real lines (added the same night, from porting's traces):**
  - The radio lost is `Hardware became unavailable during restart.` with no device of its own, inside a mac80211 warning. It is tied to the latest `ieee80211 phyN: Hardware restart was requested` before it.
  - The stock driver can say no crash at all: only `<driver> <device>: failed to send WMI_<X> cmd` every few seconds, and the hardware restart. So a hardware restart with no crash line near it is a crash, and so is a run of 5 or more failed commands, each within 30 s of the one before. A run near a crash keeps it from having come back while the run goes on, and for 2 minutes after.
  - A recovery can stall with nothing said. So **came back is seen, not assumed:** a serving radio's device has hostapd serving on it again a minute after the crash, or the scan radio has made a BSS scan, client survey or spectral sweep since (not Bluetooth's scans, which are another radio's).
  - The agent reads every such line the log holds, not a tail: a run of failed commands would push the crash itself out of one.
  - Tried on the Garage's real crash (01:19, its scan radio): read as `ath11k_pci 0001:01:00.0`, MHI_CB_EE_RDDM, scan radio, not back. **That was a false alarm.** airscan's heal bound its driver again at 01:19:59 ("the scan radio hears again, its driver bound again: 55 networks"), 50 s after the crash, while v0.254.0 still took mac80211's loss as final. So came back is what is seen now, whatever the log said. The agent reports the loss (`lost`) beside it, and a run of failed commands that goes on keeps a radio from having come back. Read again at 01:33: came back.
  - Why the driver's own recovery fails here (porting): ath11k's start holds conf_mutex while it waits 10 s for the restart work, which needs conf_mutex to tear spectral down, and spectral is built into our images. Porting is fixing that at base.
- **The manager raises `fw-crash` for the latest crash of each device:**
  - **critical** where the radio did not come back: "it stays down until its driver is bound again, which a restart of the AP does". It ends once a radio of the device serves again, or the AP restarts and its log no longer holds the crash.
  - **warning** for 6 hours where it came back: "its clients were dropped as it started again".
- **A scan radio's crash is a warning,** whether it came back or not. It serves no client, and its wireless section, which airscan holds (`airscan=1`, 0126), says so. Not back, it reads: "its surveys and client tests stop until a restart of the AP". That is the case porting's B17 makes.
- **A crash under 2 minutes old, not yet seen back and not said lost, is "starting again"** (a warning). A driver that recovers does so in seconds: 2.8 s with porting's 0085 on the Garage, 2026-11-05. The agent sees a serving radio back only a minute on, so without this the first minute of every recovered crash was critical. Said lost, it is critical at once.
- **The report's crashes are taken one by one** (0290): one not as the agent writes it is left out, not the report.
- **What the log said is kept past its ring, until the AP starts again** (added the same day). The Garage's crash at 01:46 rolled out of its 128 KB log by 03:33, and its alert ended 4 hours early. So the agent keeps what it read in `/var/run/aeolus/fwcrash.json` (tmpfs), and each poll merges it with what the log holds now (`fwcrash.merge`):
  - crashes, restarts and radios lost are kept by their kernel time, the latest 32 of each;
  - failed commands are kept as runs `{ up, last, n }`, so a run whose first lines rolled out still counts them all.
  
  tmpfs starts again with the kernel's clock. Anything timed after the clock now is from before a restart, and is dropped.

- **The SoC's own radios (AHB) crash on their remote processor** (added the same day, from porting's assert on the W-318 at 04:44 CST). The kernel says `remoteproc remoteproc0: crash detected in wcnss: type fatal error`, which names no Wi-Fi device. Failed WMI commands follow until the processor is up again. On builds before port -121 that took 170 s: the processor waits up to 5 minutes for its crash dump to be read (B151). Then each radio gets a hardware restart.
  - **The device:** the agent reads the line as the crash. Its device is the one whose failed commands begin within 30 s after it, else the AP's one device not on PCI.
  - **One crash, not three:** a hardware restart after the start of a run of failed commands, and within 30 s of its end, belongs to that run's crash. Such a restart also ends the run at once, so the crash is back as soon as the radios restart, not 120 s later.
  - **Back:** while the processor waits, hostapd still says the radios are on the air. So a radio counts as back by its networks on the air only where the crash was said lost, the case of a driver bound again by hand. Otherwise the agent's word, which counts the failed commands, is it.
  - **Wording:** not back after 2 minutes, it reads "has not started again: its clients have no service here meanwhile, whatever hostapd says. Its processor starts again by itself, in up to 5 minutes where the build waits for its crash dump to be read; if it does not, a restart of the AP brings it back".
  - **Before:** v0.258.0 read the W-318's assert as "a hardware restart was asked" at 04:47:26, the end of it.

## Consequences

- **A firmware crash is named with its cause,** and one left down is critical at once, not after the 4 minutes of 0216.
- **A crash that rebooted the AP leaves nothing to say:** its log, and what the agent kept of it, went with the reboot. A crash dump (porting's console) is where such a crash is found.
- **Tried with the agent's module on the C260 and the Living Room:** their devices map to their radios (the IPQ8074's `c000000.wifi` to its 5 and 2.4 GHz radios, the QCN9074s to the scan and 6 GHz radios). No AP's log held a crash to read.
