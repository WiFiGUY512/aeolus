# 0285. The pages read less

- Status: Accepted (Aeolus's own development, 2026-11-02)
- Date: 2026-11-02
- Refines: 0099 (alerts), 0123 (service assurance), 0136 (roaming), 0242 (slow radios)

## Context

- **The pages people keep open were slow.** On the lab's manager, over the day before the screens redesign (0284):
  - `/v1/alerts` took 1.3 s on average;
  - `/v1/roaming` took 1.6 s;
  - `/v1/assurance` took 1.2 s.

  The Overview and the Clients tab ask for them every half minute while they are open: some 2,400 times a day each.
- **Profiled against a copy of the lab's store** (2.9 GB, its sealed values sealed again with a key of the copy's own), the three took 260 to 290 ms on a desktop. The manager's container is about five times slower.
- **Most of the time was two things:**
  1. **The client tests (28%).** Every reading took every test of up to a week, each with every step it took, and parsed it again:
     - 11,129 tests holding 19.8 MB in the lab;
     - read by Assurance, and by the alerts for failing tests and slow radios;
     - read through a full scan of the actions table, which had no index by kind.

     Each reading also expired the tests that waited too long, with an update that scanned the whole table too.
  2. **Join counts (19%).** Roaming's count of each client's joins over the day used the index by AP, and so read every attempt each AP ever made (28,670), to keep the day's 2,158.

## Decision

- **The actions table has an index by kind, state and time (`actions_kind`, conditions migration 35).** The client tests of a time, and those to expire, are found by it alone.
- **A client test that is over is read and parsed once.**
  - Each reading asks the store only which tests there are since its time, and how each stands (`ClientTestHeads`, from the index).
  - It reads in full (`ActionsByID`) only those not kept, or whose state changed.
  - The manager keeps those over, as read, for as far back as any alert reads (a week) and a day more. The Assurance tab's longer spans read older ones from the store as before.
- **Join counts read the day's attempts by time** (`+j.ap`, so the planner does not take the index by AP), as `ConnectionsSince` already did.
- **The Overview asks for assurance in brief** (`brief=1`). The answer has the testers and checks only:
  - each check without its runs or its way online, and with only its last test's outcome;
  - no tests listed, no joins, and no roams of the tests, which were most of the work.

## Consequences

- **On the copy:**

  | Page | Before | After |
  |---|---|---|
  | `/v1/alerts` | 261 ms | 38 ms |
  | `/v1/roaming` | 269 ms | 70 ms |
  | `/v1/assurance` | 288 ms | 183 ms |

  The CPU of a round of them fell from 8 s to 2 s.
- **The Overview's assurance, in brief:** 43 ms and 117 KB, where it was 288 ms and 1.3 MB.
- **What is kept is a week of tests, as parsed:** a few MB in the lab.
- **Left for later:**
  - Assurance's time is now mostly the roams of the tests' clients (`roamsAmong`).
  - The Assurance tab's own answer is still 1.3 MB. Measured later, most of it is the latest test of each thing tested, not the 50 tests it lists: 0287 gives them in brief.
