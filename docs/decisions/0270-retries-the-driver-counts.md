# 0270. Retries the driver counts

- Status: Accepted (Aeolus's own development, 2026-11-01; the OpenWrt fix is the porting session's, flashed at Griff's word)
- Date: 2026-11-01
- Refines: 0164 (retries an ath11k radio counts)

## Context

- **ath11k filled no retries in what nl80211 says of a client** (OpenWrt fault 27, B62). So 0164 has the prober turn the radio's extended tx stats on, and the prober and aeolus-live read each client's retries from its `tx_stats` in debugfs.
- **That has two costs:**
  - A client that joined before the stats were on has no file, and its retries read "—".
  - The stats cost the driver work for each frame sent.
- **Port 0062 fixes it** (in -93, 68fb6e8): ath11k adds each frame's tries past its first, from its completion, to the client's `tx retries`. That is what debugfs counts.
- **On C360-AP on -93,** 2026-11-01 09:50 CST, 12 clients' nl80211 retries summed to 415, as their debugfs counts did. Client by client they differ, by up to 25.

## Decision

- **A radio where any client reads retries in nl80211 counts them itself** (`probe.counts_retries`). From then on, the prober and aeolus-live read its clients' retries there, as on every other radio.
- **Each daemon keeps its own record by phy, for as long as it runs.** It decides from each station dump before it reads the clients.
- **The prober turns extended tx stats on only on an ath11k radio not yet known to count retries itself.**
- **A driver without the fix reads 0 for every client, ever,** so it stays on debugfs as before. No list of builds or versions.

## Consequences

- **On -93, every client has its retries from when it joined,** with no "—" for clients that joined before the stats were on.
- **A radio switches to nl80211's count once a client shows a retry.** That is within a sample or two of a client sending. In that minute, a client's retries may grow by the difference between the two counts, once.
- **The stats the prober turned on, before the radio was known to count, stay on until the driver starts again,** where they go off. Nothing turns them off: Aeolus has never written 0 there.
- **Checked:** `agent/test/probe.uc` covers none read, some, kept after, another phy, and no phy.
