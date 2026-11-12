# 0182. The scan radio's budget, and how many failures make an alert

- Status: Accepted (Griff, 2026-10-20: "do 6 and 7 from #54")
- Date: 2026-10-20
- Proposed by: Griff, issue #54, ways 6 and 7, the last two; 0180 and 0181 built the others. Written up by Claude
- Refines: 0099, 0123, 0126, 0160, 0180

## Context

- **Every test through the air takes a tester's scan radio** away from airscan and rogue detection (0126, 0160). Nothing bounded how often, beyond `every` and the quick retry after a first failure.
- **An alert came of two failures running, always.** A flaky guest network alerted as soon as a vital one did. And one that must never fail unseen waited for a second try.

## Decision

- **6, the scan radio's budget:** `assurance.tests.per_hour` on a Locations folder or AP is the most tests through the air a tester makes in an hour, from 1 to 60; unset, as many as its schedule asks.
  - **What counts:** every test through the air in the hour before, those asked by hand and in rounds too, as they took the radio as well.
  - **What waits:** only tests on a schedule. Once the budget is spent, they wait until the hour has room again. Tests over the wire use no radio and never wait.
- **7, how many failures make an alert:**
  - **`assurance.tests.alert_after`** on a Locations folder or AP: how many failures running, of one network on one AP and band, make an alert. From 1 to 10; 2 unless set, as before.
  - **`network.<id>.alert_after`**, on the network in its Services folder, comes first wherever the network is tested. A flaky guest network can wait for four; a vital one can alert on the first.
  - **Kept off the APs:** both are the manager's own. The network's is taken out of what an AP is sent, as the client test's password is (0124).
  - **The quick retry and the turn order follow the number:**
    - a thing that has failed fewer times than it takes is tried again within minutes;
    - one that has reached it is "failing", and is asked every other time as before.
- **The page:**
  - the schedule editor's second section has both of the tester's;
  - a network's editor has its own under **Client tests**;
  - 0179's test flag moves into its first section, beside Broadcast.

## Consequences

- **A busy site can give airscan and rogue detection most of the scan radio's time,** and a test still comes round, only less often.
- **Alerts follow what a network is worth:** at once for the one that must work, after a few for the one that often fails for reasons of its own.

## As built (2026-10-20)

- **The schema:** `assurance.tests.per_hour`, `assurance.tests.alert_after`, `network.*.alert_after`, each in the render check's list as not sent to APs.
- **`change.AlertAfter`:** taken out of an AP's networks in `State.ResolveAP`.
- **`assurecontrols.go`:**
  - `controls.spent` and `failsAfter`;
  - `thresholds`, read once a look.
- **`assureVerdicts`:** takes each tester's number. The scheduler, the alerts and the Assurance page all use it.
- **The UI:** the editor fields and labels.
- **Tested:**
  - **The budget:** a budget of one, spent by the test just asked, holds the next turn until the hour is up, and then it goes on.
  - **The numbers:** two unless set; the folder's; the network's first, when the network is set in Services.
  - **What the APs get:** the network's number is not in the AP's config.
  - **The verdicts:** three failures running are failing at two and at one, not at four.

## Open

- Nothing more of #54.
