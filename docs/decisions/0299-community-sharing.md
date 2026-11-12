# 0299. Community sharing

- Status: Accepted (Griff, 2026-11-03, about 20:05 CST: "That should be an option on top called community sharing", "Selectable"; then "instead of it being near neighbors shouldnt it be in the band steering section?")
- Date: 2026-11-03
- Proposed by: Griff ("if the living room learns with a hard connect it should push that info to the neighbors"). Written up by Claude
- Refines: 0050 and 0138 (band steering), 0073 (radio resource management), 0289 (the APs tell one another how they hear clients)

## Context

- **Band steering's "suggest" (0050) asks a client up a band on the AP it is on.** With it on for Home Wi-Fi (change 686), a phone was asked from the Living Room's 5 GHz to its 6 GHz at 19:52 CST and moved in 1 s.
- **Under the Garage's C-360 it never got there.**
  - The phone held on to the C260's 5 GHz, which the C260 heard at -67, and the C260 has no 6 GHz radio.
  - Android does not roam while its link is good enough, so the Garage never had it to ask.
- **Each AP knows what a client can use only from the client's own association request there** (its Supported Operating Classes, read with get_sta_ies).
  - A hard connect at the Living Room teaches the Living Room alone.
  - An AP the client has not yet joined knows nothing of it.
- **The APs already tell one another how they hear clients by probe (0289),** signed, over RRM's link, one message a hello, and keep what they are told in files for the steering daemon.

## Decision

- **`network.<id>.steering.community`, a band-steering switch of the network's:** "Community sharing", off unless set.
  - It sits beside "Suggest a higher band", "Hold back 2.4 GHz" and "Move sticky clients".
  - First it was a Locations switch beside Neighbours; Griff put it with band steering, where the rest of a network's steering is.
  - Like sticky, it needs radio resource management on, as the APs tell one another over RRM's link.
  - The steering daemon's section lists the networks it is on for (`list community '<network>'`).
- **The APs tell one another what their clients said they can use.**
  - Each AP's steering daemon writes, each look, the clients on it, on a network with it on, that said as they joined there that they can use a band above 2.4 GHz, with the bands they said, the latest joined first.
  - aeolus-rrm sends that, signed, to every other Aeolus AP it knows, as a `community` message beside the probe message (`aeolus.community`).
  - The receiving AP's steering daemon takes each band in as known from when the client said it.
  - Only what a client said to an AP is told on, never what that AP was told, so no word goes round.
  - Up to 32 clients a message, fitted to one frame.
- **Band steering reaches other APs where the client has no higher band near enough on its own AP.**
  - This covers a client on a band below one it can use, where its AP has that band for the network not near enough for it, or not at all (the C260 and 6 GHz).
  - It is sent by 802.11v to the network's place on that band at another AP, the higher band first and then the best heard.
  - It must say (802.11k) that it hears that place at -60 dBm or better on 6 GHz, or -62 on 5 GHz. These are as the client hears the AP, which sends louder than the client does, so they sit above suggest's own thresholds.
  - With nothing fresh said, a client that gives beacon reports is asked what it hears on those places' channels: three times at most while it stays joined, two and then ten minutes apart, as suggest's own askings are.
  - A client that gives no beacon reports, or that never said it can use the band, is left where it is.
- **The asking is a suggest asking,** with the AP it was sent to named (`elsewhere`). It ends as `left`, with where the client went next (0291).

## Consequences

- **An AP knows a phone can use 6 GHz the moment it hears of it from a neighbour,** not only after the phone has joined it.
- **The C260 can now hand a 6 GHz-capable client to the Garage's or the Living Room's 6 GHz,** where the client says it hears it well.
- **Nothing restarts when it is turned on or off.** Only the steering daemon's section changes.
- **What the APs learn of clients stays on the APs** and among them, signed with the fleet's RRM key. The manager is not involved.
- **More can be shared under the same switch later,** as its own kind of reading.
