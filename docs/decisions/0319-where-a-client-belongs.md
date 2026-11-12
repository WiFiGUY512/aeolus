# 0319. Where a client belongs

- Status: Proposed
- Date: 2026-11-05
- Proposed by: Griff, of per-client steering: "per client steering sounds like a great feature". Written up by Claude
- Refines: 0138 (steering a network at a time), 0289 (sticky clients), 0300 (keep on 6 GHz)

## Context

- **Steering decides by what it measures:** band steering by how near a client is, sticky by how strongly each AP hears it (0289). A person sometimes knows better. Examples:
  - The Living Room TV sits on the C260 in the next room, though the Living Room C-360 is in the same room.
  - An ESP32 at the Shed joins and drops in a loop at an AP it hears poorly.
  - A device should stay on one AP, or one band, whatever its own roaming thinks.
- **Sticky's machinery already moves a client to another AP's place,** without restarting any Wi-Fi:
  - An 802.11v transition request names the places, then the client is put off if it stays.
  - It is kept off every band here for a hold down, by hostapd's own ban.
  - One that comes straight back is left alone longer each time (0138).
  - The places are the network's other BSSes, as hostapd's neighbour list names them on a steered network. RRM's hellos say which AP each radio is (`aps_near`, by the middle of its BSSID and its band).
- **A client's bands are known** from what it said as it joined, here or, with community sharing, elsewhere (`can`). A client never seen to use 5 GHz is not known to be able to.
- **The steering daemon reads its settings** from the agent's own UCI package every 30 seconds. A change there alone reloads no Wi-Fi: the agent applies only the packages that changed.

## Decision

- **`homes.<mac>`, a Locations group:** where a client belongs, set at a folder or the Org; every AP below takes it, field by field.
  - `aps`: the APs it belongs on, one to eight, by ID.
  - `bands`: optional; the bands it belongs on there, of 2g, 5g and 6g. Unset, any.
  - `hold`: optional; the seconds it is kept off an AP it does not belong on, once put off, 1 to 300. Unset, 30.
  - At most 256 clients. The MAC is lower case with colons.
- **The manager marks the APs a client belongs on** as it composes each AP's config: `homes.<mac>.here` is true on those, as no AP knows its own ID.
- **The renderer writes each rule into the steering daemon's section,** one entry a client, sorted by MAC: `list home '<mac> here aps=<id>,<id> bands=5g,6g hold=30'`, with `here` only where it is so and `bands` only where set. The section is there only where some network is steered, as before: a rule acts only on a steered network, where hostapd names the places. The check (0039) holds the entries to the intent exactly.
- **On a steered network, a client with a rule that is not where it belongs is moved there** (sticky's asking, `why: home`):
  - **Where it belongs:** on an AP marked `here`, on one of its `bands` (any, unset). Nothing is asked of it.
  - **Its places:**
    - On an AP it belongs on, but on a band it does not: the network's other bands here that it belongs on.
    - Otherwise, or where there are none here: the network's places elsewhere that hostapd lists, on an AP the rule names (by RRM's word on that radio), on a band it belongs on.
    - In both cases, only a band the client is known to use (`can`). A client only ever seen on 2.4 GHz is never sent to 5 or 6 GHz, and is left where it is when its home has no 2.4 GHz place: it is not stranded.
  - **With no place, nothing is done.** That covers a home AP that is down, not heard by RRM, or without the network or the band.
  - **How:** as sticky's `better`. A client that takes 802.11v is told to go there, disassociation imminent, and put off five seconds later if it stays. One that takes none is put off at once.
    - Put off for a place on another AP, it is kept off every band of the network here for `hold`.
    - For a band here, it is kept off the band it was on.
- **0138's guards hold:**
  - It is settled first: on ten seconds before it is asked.
  - Once in five minutes at most, shared with sticky.
  - One that comes back within ten minutes is left alone for half an hour, then twice as long each time, up to a day. So a rule naming an AP the client cannot reach costs it a few short drops, further apart each time.
  - Never an AP's own test client.
  - Hostapd never waits for the daemon. Nothing is acted on that is not known to be so now.
- **The other ways keep within a rule:**
  - Band steering's suggest asks a client up only to a band it belongs on.
  - Community sharing reaches another AP only where the client belongs.
  - Sticky moves it only to a place it belongs on, and not off an AP it belongs on to one it does not.
  - Keep on 6 GHz keeps no client there that does not belong on 6 GHz.
- **What came of each asking is in the AP's report,** as sticky's are: `why: home`, and the AP it was sent to.
  - The AP's signal of the client there is optional for `home`, as no AP may have heard it there; `better` still needs it.
  - The manager keeps and lists them as before (0139, 0289). The Networks page shows them as "Sent where it belongs".
- **Setting a rule takes the clients right,** as blocking a client does (0100): admin, operator or manager.

## Consequences

- **A rule acts only on a steered network.** Steering a network turns on 802.11k and 802.11v, which restarts its Wi-Fi once; that is still the person's choice (0138). The lab's Home Wi-Fi, Home_Wi-Fi_IoT and aeolus_50 are steered.
- **A rule names APs, so moving a client's home is one change at a folder.** It re-versions the APs below, and reloads no Wi-Fi.
- **A client with a private MAC has a MAC per network,** and may change it: a rule follows the MAC.
- **An agent from before this renders no `home` entries.** Where a rule reaches it, the check refuses its config until its agent is the manager's own, a minute later, as 0289 had it.
- **Not done here:**
  - A rule on a network that is not steered.
- **The client's page sets it** (where the person has the clients right at the Org):
  - "Where it belongs…" opens a short form: the APs ticked, and the bands there, none for any.
  - Preview makes one change at the Org; with no AP ticked, the rule is taken away.
  - A client with a rule shows it as a chip, "belongs on <APs> · <bands>".
  - `make_change` sets `homes.<mac>.aps` too, and a folder's page shows a rule as "Where <mac> belongs". A rule set below the Org is not shown on the client's page.
- **Checked:**
  - **The daemon's judgement,** by agent/test/steer.uc with 25.12's ucode:
    - A client on 5 GHz whose rule names another AP is told to go there once it has been on ten seconds, heard at -45 here all the same. It stays, so it is put off five seconds on and kept off both of the network's bands here for its rule's hold.
    - One that takes no request is put off at once. Back within the minute, it is left alone for half an hour, then sent again.
    - One on the AP it belongs on but the wrong band is sent to the band it belongs on here, and kept off only the band it was on. It moves; one already there is asked nothing.
    - Nothing is done for:
      - a client only ever seen on 2.4 GHz whose home has 5 GHz alone;
      - one whose rule names bands it was never seen to use;
      - one whose home AP RRM does not know;
      - an AP's own test client.
    - None of the other ways takes a client out of its rule:
      - suggest asks no client up off the 2.4 GHz it belongs on;
      - hold back holds it nowhere;
      - sticky sends no client away from the AP it belongs on;
      - keep on 6 GHz keeps none that belongs on 5 GHz.
    - The entries as the renderer writes them, and those not well said, which are none.
  - **The render:** steering.json renders both kinds of entry, and the check holds them, refusing a missing entry, a missing `here`, or another hold.
  - **The manager:**
    - compose marks the APs a client belongs on, and only those; the schema refuses an upper-case MAC, a field a rule has not, an AP not by its ID, nine APs, and a hold of 301.
    - The report keeps a home record without a signal, and drops a better record without one.
    - Setting a rule takes the clients right.
