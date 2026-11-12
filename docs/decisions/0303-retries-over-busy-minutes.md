# 0303. Retries over busy minutes

- Status: Accepted
- Date: 2026-11-04
- Proposed by: the OpenWrt porting session, in the Phone-B client matrix (finding F17): "count retries only over busy minutes, or weight by frames". Written up by Claude
- Refines: 0103 (a client's journey), 0233 (a client that struggles)

## Context

- **A session in a client's journey gave its retries as the reports' running count:** what was sent to it again over what was sent, as the AP counted them since it joined (0103, 0164).
- **That count mixes in the idle stretches.** A phone at rest gets a few hundred frames a minute, and many of them are rate control trying a faster rate. Those tries fail often, and say nothing of the link.
- **So healthy clients read badly.** In the Phone-B matrix (2026-11-03), the phone's sessions and the journey's "retries" issue said 35–55% on 6 GHz and 45–53% on 5 GHz. Porting found that, over the minutes with load (5,000 frames a minute or more), it was 5–16%.
- **a phone over a day (2026-11-04, from its minutes, 0207):**

  | Band | Minutes under 2,000 frames | Minutes moving 1 MB or more |
  |---|---|---|
  | 6 GHz | 41–77% sent again (213 minutes) | 12% (32 minutes) |
  | 5 GHz | 29–44% (539 minutes) | 16% (67 minutes) |
  | 2.4 GHz | 30–35% (35 minutes) | 27% (8 minutes) |

- **A megabyte a minute is already Aeolus's busy minute:** the struggling alert counts resends over those alone (0233). On the phone's minutes it falls where 2,000 frames a minute does.
- **The manager keeps each client's minutes 7 days** (0207), and the journey page shows a day or a week.

## Decision

- **A session's retries are over its busy minutes alone:** the minutes on its AP and band in which the client moved a megabyte or more, both ways (`journey.BusyBytes`, which the struggling alert now shares). The share is what was sent again over what was sent, as the Clients tab counts it.
- **A minute is the session's** from the minute the session began in to `Gap` (12 minutes) after its last report, but not into the session after it.
- **A session with no busy minute has no retries,** and neither has one whose minutes are gone. The reports' running count is no longer said anywhere in the journey: a figure that mixes idle in is worse than none.
- **The session says over how many busy minutes** (`busy_minutes`). The journey page shows it on the figure, and says why one is missing.
- **The "retries" issue needs over a fifth (0103's line) over 3 busy minutes or more,** so one burst does not say it. Its words say it is over the minutes the client was busy.
- **The Clients tab is unchanged:** it shows a client's running count now, beside its other counts.

## Consequences

- **Built on `journey.Resends`,** which sets a journey's retries from the client's minutes, as `Rejoins` and `Drops` add to it. `Build` no longer gives retries.
- **`GET /v1/clients/{mac}` reads the client's minutes** on the APs the caller may view, from its first session's start, and the MCP `get_client_journey` says so.
- **A client that streams and still reads high is the struggling kind:** the C260's alert on the Living Room TV (2026-11-04, 63% over two hours) holds. The TV moved 20,000–55,000 frames a minute at -75 dBm, with three in five sent again: it is busy, not idle.
- **On an ath11k radio, a client that joined before its retries were counted (0164)** has 0 retries in its minutes, as the agent says them, where its reports said null. Such a client is rare: the prober turns the counting on as it starts.
