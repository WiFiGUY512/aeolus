# 0291. Where a steered client went

- Status: Proposed
- Date: 2026-11-03
- Proposed by: Griff, by his standing word to make Aeolus better on its own (2026-10-24): steering is to say whether it works (0138). Written up by Claude
- Refines: 0139, 0289

## Context

- **Steering's history (0139) says what each asking came to where its AP could see:** moved, said no, stayed, left, came back. What it cannot say is whether the client was then better off. "Left" is all an AP knows of a client it put off that joined another AP.
- **Sticky on Home_Wi-Fi_IoT, its first ten minutes (2026-11-03 00:26 CST):**
  - The Samsung Family Hub was put off C360-AP's 5 GHz at −77, for the Living Room AP, which heard it at −62.
  - The well switch at the Shed AP was put off at −93, with nowhere better; it came back after 5.5 minutes.
  - Both read "left". Only the attempts each made afterwards told them apart.
- **The manager already keeps what is needed:**
  - **Every attempt a client makes to come online (0118),** with its AP, band and time, and the signal its AP heard it join at (`record.signal`). These are indexed by client and time.
  - **Every client's minutes (0207),** with its signal, keyed by client and minute.

## Decision

- **Each asking that ended with its client moved, left, came back, or joined is followed to where its client was let on next:** its first attempt, on any AP, begun after the asking and up to five minutes after its end, at which it was let on (as 0136's `LastLetOn` tells one). That gives:
  - **The AP and band,** and when.
  - **The signal there:** what the attempt's record says it joined at, or else its first minute there.
  - **None of this for an asking that ended otherwise:** it stayed, said no, or was heard well again, so it went nowhere.
  - **None for a record that asks nothing:** 0138's "came back down" after a suggestion or a hold, or 0289's late come-back of a sticky client within ten minutes. Each says what became of an asking, and that asking's own where-it-went already finds the client back. Followed again, it was counted twice. The rule is the store's own (`steerAsking`).
- **What that came to (`result`) is the first of a table that is so:**
  - `back`: where it was asked from;
  - `band`: on another band, its signal not set against the one it left (bands apart);
  - `unknown`: a signal not known on either side;
  - `stronger` or `weaker`: heard 3 dB or more better or worse there, on the same band;
  - `alike`: anything else.

  A kind added to the table is counted and shown like the rest.
- **`GET /v1/steering`:**
  - each of the latest askings gains `after` `{ap, ap_name, band, signal, at}` and `result`, with the AP named where the caller may view it;
  - each network and AP gains `results`, the count of each result over the latest askings weighed, at most 300 (`weighed`);
  - the MCP tool says the same.
- **Bounded:** at most two indexed lookups of one row for each asking weighed: connections by MAC and time, then client_minutes by MAC and minute.
- **In the UI, on the Networks tab:**
  - each latest asking shows where its client went: "→ Living-Room-C-360 5 GHz −62 dBm (was −77)", or "(was −77 on 5 GHz)" where the band changed;
  - each network and AP has a chip, "Where they went: N heard better, M worse", with every result in its title.

## Consequences

- **Steering can be judged by its effect,** not only by what the AP saw: a sticky client put off and heard 15 dB better elsewhere is told from one that came back to where it was.
- **Signals are the APs' own readings of the client,** at join or over its first minute. They differ by each AP's receiver, as 0289 says of its margin.
- **A client that joined a network another AP has under another ID** is followed all the same: the next attempt is the client's, wherever.
- **Results are over the latest askings, not the whole time asked for:** a month of askings is not weighed one by one at every look.
- **Tested:**
  - **The store:** the first let-on attempt after an asking is found, with its record's signal, or its minute's where the record has none; an attempt too late, or before the asking, is not.
  - **The API:**
    - a client put off at −77 that joined another AP 40 s on, at −62, is `stronger`;
    - one that joined 2.4 GHz is `band`;
    - one never seen again has no `after`, and one that stayed has none though it joined later;
    - a weak client put off at −93 that came back 5.5 minutes on is `back` once, on its asking, and its late come-back record has no `after` of its own;
    - the AP is unnamed to a caller who may not view it;
    - the network's and the asking AP's `results` agree.
