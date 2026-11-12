# 0321. Wherever a client is heard best

- Status: Proposed
- Date: 2026-11-05
- Proposed by: Griff, of the TV's rule (0319): "it needs to be dynamic, i want the tv steered to whatever AP is RF nearest, not a particular AP", and "the APs should be able to see the TV and compare between them". Written up by Claude
- Refines: 0289 (sticky by what the other APs hear), 0319 (where a client belongs)

## Context

- **A rule that names an AP (0319) is fixed.** Moved to the Living Room C-360 at 10:38 CST on 2026-11-05, the TV went from -80 dBm on the C260 to -58. A client that moves, or an AP that is moved or replaced, would need the rule changed.
- **Sticky's "heard better" (0289) compares how each AP hears a client, but only by its probes** (and what it says, 802.11k). A client that holds on sends no probes. The TV gives no beacon reports either. No AP but its own had a reading of it.
- **Every AP with a scan radio hears it all the same.** airscan lists each client it hears, with the BSSID it talks through, the frequency and the signal (`/tmp/airscan/clients.json`). At 10:44 CST, of the TV on the Living Room's 5 GHz:
  - the C260 heard it at -81;
  - C360-AP at -90;
  - the Garage at -93;
  - the Living Room's own scan radio at -92, where its serving radio heard -58.
- **A scan radio's reading is not a serving radio's:**
  - The antennas differ, and a C-260's ath10k reads high (+8 to +12).
  - On a 40 MHz channel, a scan radio on the secondary 20 MHz hears the client far weaker. The Living Room's -92 was at 5805 MHz; the TV's primary is 5785.
  - Raw, the readings above would make the C260 the nearest.

## Decision

- **`homes.<mac>.nearest`, a switch of the rule:** the client belongs on whichever AP hears it best. With `aps`, only among those; `bands` as before.
  - The renderer writes `nearest` in the client's entry: `list home '<mac> nearest [aps=…] [bands=…] hold=…'`. The check holds it.
  - A rule with `nearest` needs no `aps`.
- **Each AP sets its scan radio against its own radios,** a band at a time, by the clients it serves:
  - A sample is what the radio a client is on hears of it, less what the scan radio hears of it.
  - Only where the scan radio heard it on that radio's primary channel, within the last 30 seconds. (10 at first: airscan writes its clients every 12 s or so, so no AP calibrated, and no reading was made of the TV on 2026-11-05.)
  - The latest sample of each client is kept, for two hours, at most 64 clients a band.
  - The offset is the middle of them, once three clients or more have one.
- **What the scan radio hears of a client with a rule of the nearest, served by another AP, is a reading for the APs to compare.**
  - It counts only where it was heard on the primary channel of the AP it talks through, by RRM's word on that AP's radio.
  - With the offset, it is how this AP's radio on that band would hear it.
  - It is told to the other APs with the probe readings (0289), marked `s`. An AP from before 0321 drops marked readings, as its check of a reading wants five fields.
  - Readings hold 30 seconds, as probe readings do.
- **The AP a client with a rule of the nearest is on sends it to another AP that hears it 5 dB or more better** than this AP's radio does:
  - by probe or scan reading;
  - on the client's band;
  - where hostapd lists that AP's place for the network;
  - an AP and band the rule allows;
  - at two looks running.
  - It is sent as sticky sends a client heard better: told to go (802.11v), then put off five seconds later if it stays, and kept off every band here for the rule's hold.
  - The record says `why: nearest`, and the AP with its reading.
- **Where no other AP hears it, it chooses again.** A client with a rule of the nearest that no other AP has heard on its band for two minutes, by probe or scan radio, and that is heard here under -75 dBm, is put off, every band here kept off for the rule's hold, with the network's places elsewhere named. The APs cannot compare, and the client picks what it hears best.
  - A C-360's scan radio is seldom on 2.4 GHz, and hears clients there with no signal.
  - After the Living Room's flash to -122 (11:48 CST, 2026-11-05), the TV rejoined the C260's 2.4 GHz at -76 and stayed. The Living Room had no reading of it there: its scan radio had heard the TV once, at -90, a minute before, and its own 2.4 GHz clients with no signal at all.
  - One heard at -75 or better stays, as -70 is good where it is.
- **The guards of 0319 hold:**
  - It is settled first: on ten seconds before it is asked.
  - Once in five minutes at most.
  - One that comes back is left alone longer each time.
  - Never an AP's own test client.
- **Back twice, it is left there** (Griff, 2026-11-05: "stop the nearest rule after two"). A client that comes back to an AP the second time after that AP's rule of the nearest put it off, by the comparison or where no other AP hears it, is left there: the rule stops on that AP for a week, or until the rule changes. A rule that names an AP keeps trying, left alone longer each time.
  - The garage door opener (f0:ad:4e:46:a9:1f), an ESP, came back to the C260 31 s after each of four puttings off between 13:44 and 14:56 CST, as the rule's 30 s hold ran out. It never tried the Garage C-360, which heard it at -49 against the C260's -60. When the C260 was down it chose the Living Room. A client that joins the first AP it finds cannot be sent by putting it off, only taken off the air each time.
  - The AP says so in its log and its record of that coming back (`stopped`), and keeps it across a restart of its daemon. The manager takes the word in the report; it is not yet shown.
  - Checked by agent/test/steer.uc: such a client is put off at 0 and at 30 minutes and not again in six hours; it stays stopped across a restart; changed to hold=20, the rule puts it off again at once; a rule naming an AP puts its client off 4 times in the six hours.
- **Scan readings are taken only for clients with a rule of the nearest.** Sticky's heard-better still uses probes and 802.11k alone, so no other client is moved on a scan radio's word until this has been watched.

## Consequences

- **Only APs with a scan radio give scan readings:** the C-360s and the C260 in the lab. A client on an AP without one, or near one, is weighed by what probes it sends.
- **The serving AP's own reading is its radio's,** what hostapd says of the client, and needs no offset.
- **Calibration is per AP and band,** and is redone as its clients come and go. An AP that serves fewer than three clients on a band says nothing on that band.
- **Live Trace says each steering as it happens** (Griff: "in a live trace we should also see steering initiated whenever it happens"):
  - The steering daemon writes a line of the stage `steer` on aeolus-live's socket, for a client that is traced, the moment it asks hostapd.
  - "Steering initiated (client steering: to the nearest AP): asked to leave 5 GHz to ap-… (heard there at -62 dBm), heard here at -70 dBm".
  - Likewise for its putting off, a hold back, keep on 6 GHz, and band steering's asking up.
  - The frames themselves (the BSS transition request and response) were said already, where the rings are read.
- **The client's page sets it:** "Whichever AP hears it best", with the APs ticked becoming "only among".
- **Checked:**
  - By agent/test/steer.uc with 25.12's ucode:
    - Two served clients set nothing. With three, the offset is the middle of what each says (10 dB); one heard on another channel is left out.
    - A client with a rule of the nearest heard at -72 on its AP's primary channel is a reading of -62, marked `s`. None is made for a client with no rule, or on the other channel; none holds after 30 seconds.
    - On the AP it is on, at -70, a client another AP hears at -62 is told to go there at the second look. It stays, so it is put off five seconds on, kept off both bands for 30 seconds.
    - Nothing is done for a client heard 4 dB better, for a reading 31 seconds old, for an AP its rule leaves out, or for a client with no rule of the nearest.
  - By agent/test/probed.uc: a marked reading is taken, carried and read as `kind: scan`; any other mark is no reading.
  - The render, the check, the report (`why: nearest` with its AP and signal) and the schema, by their tests.
