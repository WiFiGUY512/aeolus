# 0208. What a client hears of the AP, as it says itself

- Status: Accepted (Griff, 2026-10-23: "yes build the 802.11k beacon reports AND if the clinet supports k then use it for its actual "hears it" number i.e. get rid if the ~~")
- Date: 2026-10-23
- Proposed by: Griff; written up by Claude
- Refines: 0206, 0207
- Refined by: 0289, where steering asks a weak client no other AP hears what it hears of the network's other places, and reads hostapd's beacon-report notification

## Context

- **Power control judges a lonely client by what it hears of the AP (0206).** The AP cannot hear that. It hears the client's signal on the way in, so the way out has been reckoned: the AP's power, less the path, taking the client to send at 15 dBm. The page marks the result ≈, and it can be some 5 dB off: an ESP32 sends at up to 20 dBm, some small IoT radios at 10.
- **A client that supports 802.11k can be asked.** A beacon request has it listen for the AP's beacons and report how strongly it heard them (RCPI). hostapd sends the request (`rrm_beacon_req`), and says the report on its control socket (`BEACON-RESP-RX`).
- **In the house on 2026-10-23, 12 of 57 clients said they take beacon requests,** 4 of the 46 on 2.4 GHz. Living Room's 28:07:08:7e:13:ea, one of power control's lonely clients, is among them.

## Decision

- **The AP asks.**
  - Once a minute, `aeolus-live` looks at the clients on each of the AP's networks. Of those that take beacon requests (their RM Enabled Capabilities), it asks each every five minutes what it hears of the BSS it is on. It asks passively where the client can, else actively, else from the client's table; at most 8 a minute, the longest unasked first.
  - It reads the reports on hostapd's control socket, attached only for the 15 seconds the answers take. hostapd never waits for it, unlike a ubus listener.
- **What a client says is kept for 15 minutes,** with the AP's power when it was asked. A change of power since moves what it hears by as much.
- **Power control goes by it.** A lonely client that said what it hears is judged by that, marked measured, and not by the reckoning. The power it needs is worked out from it the same way.
- **The pages say which.**
  - **The Power page** shows a measured figure plainly, and a reckoned one with ≈; each says which on hover.
  - **The alert** says "as it says itself" where it is measured, and "about" where it is not.
  - **The live view** has a column, Hears the AP, for those that said.
- **The minutes keep it:** each client's minute says what it said it hears, `heard`, for inference (0207).

## As built

- `aeolus.measure` holds the judgement, tested in `agent/test/measure.uc` with reports two clients made:
  - how a client is asked, and the operating class of its channel;
  - reading a report;
  - who is due;
  - what a client hears after the AP's power changed.
- **Some clients write their signal into RCPI as a signed dBm.** A Samsung phone and another said 0xc2, which as RCPI is −13 dBm while the AP heard them at −57 and −63. A reading stronger than −20 dBm is read as a signed dBm, −62 here, and one that still says no signal is passed over.
- **Tried by hand on C360-AP and Living-Room-C-360 before release.** Eight clients answered within the first minute.
- **The stored data:**
  - `heard.json` in `/var/run/aeolus` carries what each client said, from `aeolus-live` to `aeolus-rrm`;
  - `client_minutes.heard` keeps it per minute (migration 28).

## Consequences

- **For a client that can say, "It hears the AP" is a measurement,** and power control's floor for it is exact.
- **Most IoT devices cannot say.** For them it stays a reckoning, with its ≈.
- **Each client that can say is asked every five minutes.** That is a request frame and a report of tens of bytes, with no scan off its channel when it is asked passively of its own BSS.
