# 0272. A count below zero

- Status: Accepted (Griff, 2026-11-01: "Guard it")
- Date: 2026-11-01
- Refines: 0207 (what a minute came to), 0206 (a step's distress)

## Context

- **Office's mt76 sometimes reads a station's tx retries as 2^32 − k** (k 19–70; OpenWrt fault 63, B124). mt7915 adds `count - 1` for each frame it frees, and a frame with count 0 takes one off the station's 32-bit count. Fixed in mt76 748311fa, which openwrt-25.12 doesn't have yet.
- **Aeolus took the step as growth.** A count from 25 to 4,294,967,277 grew by about 4.29 billion in one look.
  - 44 minutes in about 10 days, all on Office's 5 GHz, all in the C-360 client tests' short stations (10–60 s).
  - In those minutes, Office's 5 GHz said 100% retried in its load (RRM's hellos, steering's choices) and in the manager's radio shares. The flight recorder's points and RRM's step windows read the same counts.
- **Griff's word, asked as an OpenWrt fault is: guard it.**

## Decision

- **A running count that steps by 2^31 or more between two looks didn't grow** (`minutes.grew_by`). No count of a client's grows that much in seconds; a 32-bit count that went below zero always does.
- **The look is passed over,** and the count grows on from the one before it. A count that went back, as when a client joins again, starts again from the new value, as before.
- **The minutes, and so the flight recorder's points, and RRM's step windows (0206) all take growth from `grew_by`.**

## Consequences

- **Office's 5 GHz load and minutes are right in a test station's short stay.** What its count did while below zero is lost: a few retries.
- **The 44 minutes already kept stay as they are.** They age out with the rest.
- **It is a stopgap for an OpenWrt fault:** listed as fault 63, to go once Office runs an mt76 with 748311fa.
- **Checked:** `agent/test/minutes.uc` covers a count below zero passed over and growing on, a count gone back, and growth. The rrm test is as before. aeolus-rrm and aeolus-live compile on Shed's 25.12 ucode.
