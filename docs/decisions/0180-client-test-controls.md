# 0180. Controls for the client tests on a schedule

- Status: Accepted (Griff, 2026-10-20: "do 1, 3, 5 and 8 from #54")
- Date: 2026-10-20
- Proposed by: Griff, issue #54: "we should probably put more controls into the automated testing of other APs using the multifunction radio as a client". He chose four of the eight ways the issue offered. Written up by Claude
- Refines: 0121, 0123, 0128, 0131

## Context

- **The tests on a schedule had four controls:** how often (`every`), through the air or over the wire (`over`), and the two speed measurements.
- **Everything else was fixed:**
  - a tester joined every network it could;
  - on itself, and on every AP it heard at −75 dBm or better;
  - whatever the load on the radios;
  - and the only way to stop the tests was to unset `every`, which loses the schedule.

## Decision

- **Eight new settings, all under `assurance.tests` on a Locations folder or AP,** inherited and locked as any setting. They are the manager's own and never reach an AP.
  1. **Networks:**
     - `networks`: only these, by ID;
     - `leave_out`: never these, such as a guest network behind a captive portal.
  2. **Who tests whom:**
     - `on` is `self` (only itself), `heard` (itself and the APs it hears at the floor or better, as before, and the default) or `listed`;
     - `listed` names the APs for `listed`, itself among them only where it names it;
     - `floor` is the weakest, in dBm, a tester may hear another AP to join a network on it: −75 unless set, from −95 to −40.
       - **Both ways** (2026-11-03): the AP must hear the tester at the floor or better too, as its neighbour list says (`their_signal`); where it has not said, the tester's reading alone. A test client's frames go to the AP as much as the AP's come to it. The Shed AP, across a yard, was heard by the Garage AP at −73 and heard it at −78; the Garage AP's tests of it got 4–6 Mbit/s and twice an offer of an address never confirmed, for the distance and not the service. No tester reaches it both ways at −75 now: its networks are tested over the wire where tests go that way (0128), and through the air only where a floor set lower takes a tester in.
       - **A test through the air that failed with the AP it joined heard below its tester's floor is no evidence for an alert** (2026-10-22). It failed for the distance. This happens where power control (0077) has just turned that AP down and the tester's smoothed reading of it has not caught up. The assurance page still shows it.
  3. **Load guards:**
     - **`max_clients`:** a test through the air waits while the tester's radio, or the joined AP's radio, on that band carries more clients than this;
     - **`max_busy`:** it waits while the channel it joins on is busier than this share, in per cent, as the joined AP last rated it (0073).
     - **How it waits:** the scheduler looks again a minute later, and the next thing in turn that isn't held back goes first.
     - **What they read:** each AP's latest state report. Where an AP has said nothing, nothing waits.
     - **Tests over the wire never wait:** they use no radio.
  4. **Pause:** `paused` true asks no test on a schedule here or below; `every` and the rest stay as they are, for when it is off again.
- **Tests asked by hand, and rounds (0129), take none of these.**
- **The page:** the Assurance tab's schedule editor has a second section, "What they join, and when they wait". A paused schedule says so, on the Schedule fold and on each tester's line.

## Consequences

- **A site keeps tests away** from a captive guest network, from far-off APs, and from radios at their busiest. It can stop them for an event and start them again unchanged.
- **Fewer tests through the air leave the scan radio more time** for airscan and rogue detection (0126, 0160). A budget of its own for that (#54's sixth way) is still open.

## As built (2026-10-20)

- **The schema:** `assurance.tests.paused`, `networks`, `leave_out`, `on`, `listed`, `floor`, `max_clients` and `max_busy`, each covered in the render check's list as not sent to APs.
- **`internal/api/assurecontrols.go`:**
  - `assureControls` reads them;
  - `controls.tests` and `joinsOn` narrow what a tester tests;
  - `putOff` says why a test through the air waits.
- **`assurance.go`:**
  - the scheduler's own combos take the controls, and a round's take none (`anyControls`);
  - the scheduler skips a paused AP, and passes over what the load holds back;
  - `/v1/assurance` says `paused` for each tester.
- **The UI:** `views/assurance.js` and the labels in `format.js`.
- **Tested** (`TestAssuranceControls`):
  - **What the scheduler tests:** with nothing set, every network on itself and the AP it hears well; a floor of −85 taking a far AP in; itself alone; listed APs without itself; one network only; that network left out.
  - **Pause and load:** paused, nothing is asked. The tester's radio over `max_clients` puts everything off, and a joined AP's radio over it puts its tests off. A channel busier than `max_busy` puts its tests off, and the test is asked once the load is light again.
  - **The test catches breakage:** taking out the pause, or the load guard, makes it fail.
- **The browser:** the editor's new section was looked at on a local manager.

## Open

- #54's other ways: the scan radio's budget (6) and alert thresholds (7); bands (2) and a time window (4) are 0181.
- Picking listed APs from a list, rather than typing their IDs.
