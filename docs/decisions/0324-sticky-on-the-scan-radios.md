# 0324. Sticky on the scan radios

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Griff, "do steering on real readings", of moving clients by what the scan radios hear once B156 is fixed. Written up by Claude
- Refines: 0289 (sticky by what the other APs hear), 0321 (wherever a client is heard best)

## Context

- **Sticky's "heard better" (0289) weighs a client by its probes, as each AP hears them,** and by what the client says it hears (802.11k). A client that holds on sends no probes, and a TV gives no reports: no other AP has a reading, and sticky cannot move it.
- **0321 set each AP's scan radio against its own radios,** by the clients it serves, and made what the scan radio hears a reading the APs compare. That was for clients with a rule of the nearest alone.
- **The C-360s' scan radios gave most frames no level** until porting's B156 fix: ath11k's full monitor cleared each PPDU's status for a station without a peer, so frames came at -95 dBm, which airscan drops. With the fix, 307 of 309 frames had a real level.

## Decision

- **`network.<id>.steering.sticky_scan`, a switch beside sticky's settings:** sticky also weighs what the scan radios hear of the network's clients. It takes effect with sticky on.
  - The renderer lists the networks it is on for: `list sticky_scan '<id>'`. The check holds the list.
- **Where a network on an AP weighs the scan radios, that AP's scan radio gives readings of every client it hears,** not only those with a rule of the nearest.
  - Each reading is set against the AP's own radios, and taken on the primary channel of the AP the client talks through, as 0321 has it.
  - They go to the other APs after the probe readings: a rule of the nearest's clients first, then the others, the most strongly heard first. A message that cannot hold them all leaves out the last, the clients this AP hears least, which are the least likely to belong here.
- **The AP a client is on weighs a scan reading as a probe reading, where its network weighs them:** heard better by sticky's margin, under sticky's floor, at two looks running. Elsewhere it is passed over.
- **The weak rule is as before:** a scan reading is not "heard by another AP" for it.

## Consequences

- **Quiet clients become visible to sticky** on networks that weigh the scan radios, once B156 is on the C-360s. Without it they still give few readings.
- **Calibration is per AP and band, from its own clients (0321).** An AP that serves fewer than three clients on a band gives no readings on that band.
- **It is off unless set.** It is to be turned on for Home_Wi-Fi_IoT once B156 is on the C-360s and their readings look right. Sticky's own guards hold: the margin, the floor, once in five minutes, rest after coming back, and no test clients.
- **Checked by agent/test/steer.uc with 25.12's ucode:**
  - With no network weighing them, only the rule's client is read. Where one does, every client heard is read, the rule's first, then the strongest.
  - A client at -72 that only a scan radio hears 9 dB better elsewhere is moved at the second look once lab weighs them, and not before.
  - A scan reading is not heard-elsewhere for the weak rule.
  - The render case lists `sticky_scan`, and the check refuses its absence.
