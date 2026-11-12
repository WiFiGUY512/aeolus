# 0200. A loop everywhere is not the AP's

- Status: Accepted (Griff, 2026-10-21: "yes, do that!!")
- Date: 2026-10-21
- Proposed by: Claude, from the IoT devices' loop turning out to be the WAN's
- Refines: 0190, 0193

## Context

- **The IoT devices' loop (0190) was not the C-360s'.** Two IoT device 4s (Espressif ESP32) came online about every 43 s. The cause was beyond the APs:
  - the T-Mobile 464XLAT path drops IPv4 packets sent without DF over about 1,408 bytes;
  - the ESP32 sends full 1,416-byte TCP segments without DF, so the robot's AWS IoT session (MQTT over TLS) stalled. The robot dropped Wi-Fi, and joined again about 40 s later;
  - it did so on every AP it was on, the C-360s and the R7800 alike. It only looked like the C-360s' because that is where the robots mostly sat;
  - a TCP MSS clamp on the WAN router fixed it.
- **Aeolus pointed at the wrong place, twice:**
  - the `rejoins` alert (0190) said, on each AP: "Something of this AP's radio does not suit it", whatever the client did on the others;
  - the compatibility watch (0193) called the C-360 trouble, and said "on <model> (<band>) they stay" from the maker's clients on another model, even where those looped there too.

## Decision

- **The `rejoins` alert says what the client did on the other APs in the same hour,** and blames by that:
  - **it did the same on every other AP it came online at, and stayed on none:** it drops its connection itself on every AP it joins, so it is not this AP's radio. It is the client itself, or something beyond the APs: the network or internet path its own traffic takes, as when its cloud connection stalls. Look there, not at the radio;
  - **it stayed on another:** this AP's radio, as before: "On Office it stays, so something of this AP's radio does not suit it";
  - **both:** not this AP's radio alone. Look at what this AP and those it loops on share, unlike where it stays: their kind of radio, or the path beyond them;
  - **it was on no other AP long enough to tell:** nothing yet. See whether it does the same on another AP.
- **What "did the same" and "stayed" are,** from the client's attempts at every AP (0118), all of them, in the window:
  - **looped:** it came online there 20 times or more within an hour;
  - **stayed:** it came online there, and tried nowhere again for 10 minutes or more, counting to now for its latest. Ten minutes is over three times the three minutes one that comes back 20 times an hour is gone;
  - **left:** it came online there, and tried again somewhere sooner, each time. That is the same as looping there: it did not stay;
  - **an attempt that did not come online counts as trying again:** one that failed for a quarter of an hour after it came online did not stay;
  - **an AP it came online at once, too lately to have stayed yet,** counts neither way.
- **The journey's `rejoins` issue (0190) says the same,** of "that AP", over the time asked.
- **Still one alert on each AP the client loops on, keyed `rejoins:` and its MAC.** One alert a client was weighed, and not chosen:
  - alerts are an AP's: who may see them, and where its folders send them (0101), go by the AP; the alert log keys them by AP and key (0109);
  - one alert moved to whichever AP the client loops on most would end and begin again as the hour's counts shift, and notify each time;
  - so each AP's alert names the others instead. The keys are as they were, so open alerts carry on.
- **The compatibility watch (0193): a maker's clients that loop on another model too are no model's trouble.**
  - Where, in the same hours, the maker's clients loop on another model as well, the group has no `trouble`. It has `everywhere`: "1 of its 1 Espressif Inc. clients here came online again about every 45s, as one that drops the connection itself, and they do so on OpenWrt One (2.4 GHz) too: the looping is not this model's. They loop wherever they are, which points at the clients themselves or the path beyond the APs, such as their cloud connection stalling".
  - "they stay" is said only where none of the maker's clients loop on another model, and there they come back once in two hours or less, as before.
  - Where they are on no other model, it is trouble as before, with nowhere they stay.
  - The troubled come first, then those with `everywhere`.
  - The Assurance tab's "Devices and APs" panel shows `everywhere` once a maker, below the troubled and plainly, not as trouble.

## Consequences

- **A client that loops on more than one AP is no longer blamed on a radio.** Where a robot looped on two APs in the hour, each AP's alert would have said to look beyond the APs; where it sat on one, nothing yet, and no longer the radio. The compatibility watch would have said the Espressif looping was no model's, as it looped on the C-360s and the R7800 alike.
- **A client seen on one AP alone gets no verdict.** That is what is known: one AP cannot tell its radio from the client or the path. Moving the device, or its roaming, tells.
- **A stay is measured from attempts, not sessions.** A client that came online and then went off altogether is counted as having stayed. That is rare in a client that loops; the state reports could tell later.
- **A maker is still a coarse group.** Clients of one maker that differ, a robot and a smart plug both Espressif, are counted together, and "they stay" may be the plug's. `everywhere` takes the claim away where any of the maker's clients loop on another model.
- **The alerts read the hour's attempts of the clients that loop** on each look, the latest 1,500 at most: only of clients that loop, so as a rule none.

## As built (2026-10-21)

- **The verdict:** in `internal/journey/journey.go`, `Fared()` (looped, stayed or left, an AP at a time), `Elsewhere()` (the other APs, by name) and `Blame()` (the words); `RejoinsStay` is 10 minutes. `journey.Online` has `Tried`, for an attempt that did not come online.
- **The alert:** `rejoining()` in `internal/api/joins.go` reads the hour's attempts of the clients that loop with `ConnectionsOfClients`, and passes `Same` and `Stays` in `alerts.Rejoining`; `alerts.For` says `journey.Blame("this AP", …)`.
- **The journey:** `journey.Rejoins()` takes `now`; `clientJourney` passes every attempt, those that did not come online as `Tried`.
- **Compatibility:** `compatibility()` in `internal/api/compat.go`, and `compatGroup.Everywhere`.
- **UI:** `compatPanel()` in `views/assurance.js`.
- **The descriptions:** the MCP tools `list_alerts`, `get_client_journey` and `get_compatibility`, and `docs/api.md`.
- **Tested:**
  - `TestRejoinsElsewhere` in `internal/api`: a client that loops on two APs, each AP's alert and its journey pointing beyond the radio; one that loops on one and then stayed on the other, the first one's radio;
  - `TestRejoins` in `internal/api`: on one AP alone, nothing yet;
  - `TestRejoinsBlame` in `internal/journey`: on two APs; gone again from the other within minutes, by attempts that did not come online; stayed on the other; too lately on the other to tell; on two, and stayed on a third;
  - `TestRejoins` in `internal/journey`: the office it left and tried nowhere again after, the radio;
  - `TestCompatibility`: once the robot loops on the OpenWrt One too, no group is trouble, and both Espressif groups say `everywhere`.
