# 0164. Retries an ath11k radio counts

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff: "for some reason the C-360s dont seem to be able to see retries of clinets, all of them being 0% is pretty unusual". Written up by Claude
- Refines: 0066

## Context

- **A client's retries came from what nl80211 says of it** (`tx retries`), which mac80211 fills from the driver's transmit status.
- **ath11k never fills it:** every client of a C-360 reads 0 retries, while `tx failed` counts.
- **ath11k does count them** when its extended tx stats are on (`/sys/kernel/debug/ieee80211/<phy>/ath11k/ext_tx_stats`): each frame's tries past its first, in a `tx_stats` file in debugfs for each client, made when the client joins. Off by default, and back off when the driver starts again.

## Decision

- **On an ath11k radio that serves, the prober turns extended tx stats on,** and reads each client's retries from its `tx_stats`: the retry packets by channel width, summed (`probe.ath11k_retries`).
- **A client that joined before they were on has no file, and its retries are not known:** the report says null, and the Clients tab shows "—" until it joins again. They are not given as 0.
- **Every other radio's retries are as before.**

## Consequences

- **Tried, 2026-10-15, on C360-AP,** the changed prober run beside the real one with `-n`: of 29 clients, the 9 that had joined since the stats went on had their retries (a laptop on 6 GHz 11,993 of 23,214 frames); the rest were null.
- **The manager takes a report's retries as null or a count,** and a session in a client's journey has no retry share where they were not counted.
- **The stats cost the radio's driver some work for each frame sent.** The C-360 is the only ath11k AP in the lab.
