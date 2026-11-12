# 0221. Beacons that do not go out

- Status: Accepted
- Date: 2026-10-26
- Proposed by: Claude, on Griff's standing word to go on developing Aeolus and to use what the OpenWrt porting sessions make ("use whatever comes from them"). Written up by Claude
- Refines: 0216 (a radio's networks on the air)
- Refined by: 0339, which judges the beacons over the last half hour, not a report at a time; 0350, which shows them on the Radios page
- Uses: port b4ecbba's ath11k fix for `beacon_stats` with more than one network on a radio (B85, port main 0025)

## Context

An AP announces each of its networks in beacons, about ten a second. A client counts the beacons it misses, and after a few in a row it takes the AP to be gone and leaves. A beacon fails to go out when the channel is too busy to send on at its time, or when the radio is stuck.

Aeolus saw none of this. 0216 says when a radio's networks are not on the air at all. A radio whose networks are up but whose beacons often fail looked healthy, while its clients dropped.

ath11k counts each network's beacons sent and failed (`fw_stats/beacon_stats` in debugfs). Until port b4ecbba, reading that file timed out on any radio with more than one network (B85), so nothing could use it. Since 2026-10-25 every Arista AP Aeolus manages runs b4ecbba.

C360-AP over its first 33 hours on b4ecbba: about 1.18 million beacons a network. It failed 0.03% on 5 and 6 GHz and 0.16% on 2.4 GHz.

## Decision

- **The agent reads each ath11k radio's beacon counts** at each state report, all its networks together:
  - It finds the radio's phy through iwinfo.
  - It sends `beacons {sent, failed, over}`: what came between this report and the last, over that many seconds.
  - It sends nothing on its first report, where the counts went back (the driver loaded again), or where the radio has no such file (ath10k, mt76).
  - A phy whose file does not read is not read again until the agent starts again. On an older image the read waits three seconds for an answer that never comes.
- **The manager checks the counts:** from 0, over 1 to 86400 seconds, at most 640 a second (64 networks at ten each).
- **A radio that failed one beacon in fifty or more in a report is a warning,** `beacons:<band>`, with at least 30 failed, so a short time's few say nothing:
  - "4.0% of its beacons on 2.4 GHz did not go out over the last 5 minutes (310 of 7800): its clients may lose it. The channel is too busy to send on, or the radio is stuck".
  - It lasts while the reports say so.

## Consequences

- **A radio that is up but cannot send** is seen, before its clients' drops are the only sign.
- **Only ath11k radios report beacons,** the C-360s' and C-260s'. The R7800 (ath10k) and the OpenWrt One (mt76) say nothing, and raise nothing.
- **The counts are per radio, not per network:** a radio's networks share its channel and fail alike. C360-AP's four networks on 2.4 GHz each failed 1,861 to 1,942.
- **Not done:** a history of beacons over time (the minutes, 0207, could carry them), and showing them on the Radios page beside each radio.
