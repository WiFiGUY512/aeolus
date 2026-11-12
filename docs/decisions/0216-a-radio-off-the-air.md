# 0216. A radio whose networks are not on the air

- Status: Accepted
- Date: 2026-10-24
- Proposed by: Claude, from 0122's "Not yet" ("an alert when an AP loses a radio"), on Griff's standing word to go on developing Aeolus. Written up by Claude
- Refines: 0099, 0122, 0201, 0213

## Context

An AP whose radio stays down left nothing on the manager:
- a firmware crash;
- a driver that did not come back;
- hostapd failing every network on a radio, as ath11k's missing AP/VLAN mode once made it (0082).

The report's `up` comes from netifd's account of the radio. A radio can be up there with none of its networks on the air. On 2026-10-24, OpenWrt3c8f's 5 GHz radio read up with 0 of its 4 networks on the air: it was listening for radar on channel 112 after a restart.

A client test (0121) finds a network missing only where a scan radio looks for it. 0213's recordings say when every radio went down at once, but not that one stayed down. The alerts (0099) had nothing for it, though its clients there have no service.

## Decision

- **The agent counts, for each radio, its networks on the air** (`bss`), of those it has (`bss_want`).
  - It has the AP interfaces its config gives it, but those turned off and those their network took off the air on purpose (0201).
  - A network is on the air where hostapd has its BSS enabled.
- **It keeps when a radio first had fewer on the air,** and reports for how long (`short_for`, seconds) and the SSIDs not on it (`missing`).
  - A radio on a DFS channel listening for radar before it may send is coming up, not short. hostapd has no BSS to ask then, so the agent reads the channel's DFS state: usable, not yet available, for no longer than the longest listen (ten minutes on weather radar's channels).
  - A radio turned off, or kept for scanning (disabled in its config), has nothing to count.
- **The manager raises an alert once a radio has been short for four minutes:** past an apply, a restart of the Wi-Fi or a driver loaded again.
  - **`radio-down`** (critical) where none of its networks are on the air: "its 5 GHz radio (radio1) has none of its 4 networks on the air: their clients have no 5 GHz service here".
  - **`radio-short`** (warning) where some are not, naming them.
  - Either begins when the report says the radio first fell short, and ends with the report that finds it whole.
- **The AP page's Radios tile** says "5 GHz 2 of 4 on air", amber, for a radio short of networks.

## Consequences

- **A radio that stays down is an alert within a report or two** (state reports come every five minutes), and the advisor sees it among the alerts.
- **A radio restart, a config applied or a radar listen raises nothing:** each is over before the next report.
- **An agent restart starts the count afresh,** so a radio already short is said a report later.
- **A radio whose channel stays usable for more than eleven minutes** is short, as a radio stuck in its listen would be.
- **What makes a radio fail is OpenWrt's to fix:** a driver, firmware or hostapd. Aeolus says so, and the fault goes to the porting sessions.
