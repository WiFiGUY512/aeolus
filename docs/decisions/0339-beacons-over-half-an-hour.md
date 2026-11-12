# 0339. Beacons over half an hour

- Status: Accepted
- Date: 2026-11-08
- Proposed by: the OpenWrt porting session's finding in B169, which asked that the beacons alert judge a sustained rate. Taken up by Claude on Griff's standing word to go on developing Aeolus, and accepted with the merge
- Refines: 0221 (beacons that do not go out)
- Refined by: 0350, which shows the table on the Radios page

## Context

0221 warns of a radio whose beacons do not go out, `beacons:<band>`. It judged each state report alone: one beacon in fifty or more, and at least 30, over the minutes since the agent's last report.

That was an alert 63 times in the week to 2026-11-08, almost all on 2.4 GHz. Each lasted about one report, around 5 minutes, then ended. Most were on C260 (30) and Garage-C-360 (15).

The porting session looked into it (B169). They read the five ath11k APs' firmware counts for six hours on 2026-11-07:
- **There is a steady background:** 2.4 GHz misses 0.19 to 0.35% of its beacons, 5 GHz 0.04 to 0.15%, 6 GHz 0.02 to 0.03%. That follows how crowded each band is.
- **There are bursts:** 17 bursts of 1.5% or more over 30 seconds in the six hours. The worst minute was 4.9% (Garage, 19:45 CST).
- **The bursts are each AP's own.** Two APs burst together no more often than chance. They don't follow temperature, and only slightly the client tests.
- **No fault was found** in the radios, the firmware or the house.

A client takes an AP to be gone only after a run of beacons in a row it doesn't hear. So scattered misses at one in fifty for a few minutes are no harm to its clients. A radio that is stuck, or a channel kept busy for a long time, is harm. Judging one report at a time could not tell those apart.

Checked against two days of the stored reports of the same five APs:

| Rule | Alerts in 48 hours |
|---|---|
| A report at a time, 2% (0221) | 52 |
| Over the last 30 minutes, 1% (porting's suggestion) | 7 |
| Over the last 30 minutes, 2% | 0 |

The worst half hour on any band was 1.65% (W-318, 2.4 GHz).

## Decision

- **The manager keeps each radio's beacon counts at each state report,** in a table of their own: `beacons`, one row per radio per report, of what the agent sent (sent, failed, over). It keeps them as long as the reports, and drops them with them.
- **The beacons alert reads the last half hour** (`BeaconsWindow`) of those rows: each radio's beacons summed over the reports of the last 30 minutes.
- **Once those cover a quarter hour,** `BeaconsSpan`, it is still one in fifty, `BeaconsFailingPct`, and at least 30. A burst of a few minutes is diluted across the half hour around it. A rate held for most of it, or a radio stuck for five minutes or more, is not diluted.
- **Before the counts cover a quarter hour,** as an AP starts, only one in ten, `BeaconsStuckPct`, is an alert. That still catches a radio stuck from its start.
- **The message says over how long the counts ran:** "2.0% of its beacons on 2.4 GHz did not go out over the last 30 minutes (1440 of 72000): its clients may lose it. …". The key is unchanged, `beacons:<band>`, so the alert's history carries on.
- **The agent is unchanged:** it sends what came since its last report, as 0221 has it. This change is the manager's alone.

## Consequences

- **The ordinary bursts no longer warn.** In the last two days the new rule would have warned of nothing.
- **A real problem warns as quickly or nearly so.**
  - A radio stuck after running well is past 2% within one report: five minutes of 100% is 16% of a half hour.
  - A channel busy enough to fail 2% for most of a half hour warns once the reports show it.
- **The alert ends more slowly** once a radio recovers. The bad minutes stay in the half hour for up to 30 minutes, as they would in what its clients saw.
- **The thresholds are constants beside the rule.** A band-relative baseline, if one is wanted later, reads the same table.
- **The table is a history of beacons over time,** which 0221 left undone. A chart on the Radios page can read it with no new collection.
