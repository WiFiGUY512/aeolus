# 0185. A client is on one AP at a time

- Status: Accepted (Griff, 2026-10-20: "this cannot be true at the same time", then "make sure all the statistics are updated with that as well")
- Date: 2026-10-20
- Proposed by: Griff, with a screenshot of one phone on two APs in the Clients list. Written up by Claude
- Refines: 0066, 0108, 0166
- Refined by: 0251, which takes where a client was last active from each AP's minutes too, so a roam shows within a minute

## Context

- **a phone was on two APs' lists at once:**

  | AP | Joined | Idle |
  |---|---|---|
  | OfficeOpenWrt | 153 s before | 0.9 s |
  | Living-Room-C-360 | 872 s before | 133 s |

  It had roamed to the office without telling the living room. hostapd keeps such a client until it has been quiet for five minutes.
- **What the APs report is right:** each AP lists the clients its hostapd has. The page showed every AP's list as it came, so the phone was shown, and counted, twice.
- **So was every count made from those lists:**
  - the Clients tab and its count;
  - a folder's Wi-Fi clients tile, its counts by band and network, and its busiest APs;
  - the APs tab's clients;
  - an AP's own clients tile and tab;
  - the clients over time, which the manager counts as each report comes.

## Decision

- **A client is on the AP it was last active on:** that AP's report's time, less how long the client had been idle then. Of two alike, it is on the one it joined last.
- **The manager keeps, for each client, the AP it was last active on and when,** from each report as it comes. It keeps them in memory, for an hour.
  - **A report's counts leave out** the clients it lists that were active on another AP since. That covers its clients and its clients on each band, which the clients over time are made from.
  - **An AP's condition names them as `elsewhere`,** so that a page showing one AP can leave them out too.
- **The page counts each client once,** with the AP it was last active on:
  - it leaves out what `elsewhere` names;
  - where two APs still list one client, it takes the one with the later activity;
  - a radio's clients, as its AP reports them, are less those on its band that are elsewhere.

## Consequences

- **A roaming client is counted once,** on the AP it is on, everywhere the page counts clients.
- **What the manager already recorded is not mended.** Earlier hours keep the double counts.
- **For a minute after the manager starts,** before every AP has reported once, the first report from an AP a client left can still count it. The next one doesn't.
- **What each AP reports is unchanged,** and so is what the APs do.

## As built (2026-10-20)

- **The manager:**
  - `internal/api/active.go`: `activity`, `note` at each report and `elsewhere` for the condition;
  - the report's counts in `report.go`.
- **The page:** `views/onnow.js` (`hereNow`, `onNow`, `gone`), used by:
  - the Clients tab;
  - the folder Overview: its tile, band and network counts, and busiest APs;
  - the APs tab;
  - an AP's own page: its clients tile and its clients over time.
- **Tested:**
  - **In Go:** the rule, with the phone's own numbers, in either order. The client returns, and an hour forgets it. Through the API, two APs report the phone: the one it left counts one client, not two, and its condition names the phone.
  - **On a local manager,** with the living room and the shed both listing the phone:
    - the folder's clients list and its tile say 2, with the phone on the shed;
    - busiest APs and the APs tab give the living room 1;
    - the living room's own page lists only the laptop.
