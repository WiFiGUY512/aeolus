# 0263. A radio that can't scan

- Status: Accepted (Aeolus's own development, 2026-11-01, at Griff's word to keep making fixes and features overnight)
- Date: 2026-11-01
- Refines: 0073 (radio resource management), 0120 (DFS channels after a look)

## Context

- **A radio on a DFS channel can't scan** (0120). Linux keeps it there listening for radar and refuses every scan. RRM's ratings for its band lapse after half an hour, so RRM can't move it or see a neighbour come.
- **With `radio.5g.dfs` `allow`, hostapd may start a radio on one,** and nothing then looks.
- **RRM's report already lists such radios,** in `rrm.cannot_scan`, and the Neighbours page marks them "can't scan". Nothing else said so.
- **At Home:**
  - Shed sat on DFS 52 from at least 2026-10-31 17:28 CDT.
  - Office sat on 108 until look-first was set (change 601, 2026-11-01 07:42Z).
  - Neither could rate a channel. It came out only in their logs.
- **A radio may also refuse every scan off DFS** (`refused`). That is a driver's fault, and as blind.

## Decision

- **A warning, `cannot-scan`, keyed by band,** is shown for each radio in the AP's `rrm.cannot_scan` that has not scanned for half an hour or more:
  - **On a DFS channel:** it says so, that RRM rates no channel for it and moves it nowhere, and to set `radio.<band>.dfs` to `look-first` (0120).
  - **Refusing scans elsewhere:** it says so, and for how long.
- **Its since is when the radio stopped scanning,** from the report's `ago`.
- **One on a DFS channel it went to after a look** has where it steps off to (`home`), and rates its own channel by its own counts (0120). It is not said.
- **It ends when the radio scans again,** or leaves the channel.

## Consequences

- **A site on `allow`, with RRM on, hears when a radio has gone blind on DFS,** with what to set.
- **An AP with a scan radio never lists one,** as it rates its channels through that radio (0116).
- **Not checked live:** Home is on look-first now, so no radio there is in this state. The rules are checked by `TestCannotScan`.
