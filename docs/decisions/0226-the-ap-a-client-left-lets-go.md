# 0226. The AP a client left lets go of it

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from the roam tests (0225); Griff: "Yes, active client management is good. We will work any mac switch path problems if they happen". Written up by Claude
- Refines: 0073 (radio neighbours), 0136 (roaming), 0224 (keys handed ahead of a roam)

## Context

- **A client that roams back to an AP it just left was slow to get on there.** The roam tests of 2026-10-28, from C360-AP's scan radio, on Home Wi-Fi, timed by the AP gone to:

  | Joined by | To another AP | Back to the AP it began on |
  |---|---|---|
  | WPA2 or WPA3 with 802.11r | 10–24 ms | 650–770 ms |
  | the WPA2 handshake | 36–45 ms | 1.2–2.4 s |
  | WPA3 alone | 75–80 ms | 2.4 s, and once not at all |

  - **802.11r back there was refused** (status 55, about one roam in twenty, 0225), and the client got on with a key it held instead.
  - **The rest waited out 802.11w's SA Query:** the AP asks the client it remembers to prove itself before it takes the new association, up to a second.
- **The AP a client leaves does not know it left.**
  - A client that roams by 802.11r, or reassociates elsewhere, says nothing to the AP it leaves.
  - That AP holds it as on until it has been silent for minutes, and asks it in vain all that while.
  - hostapd lets go of a client that moves between the radios of one AP (its "Prune association"), but has no such word between APs. 802.11F's IAPP once gave one, and is not in this hostapd.
- **hostapd can let go of a client saying nothing in the air:** `DEAUTHENTICATE <mac> tx=0` on its control socket removes it from hostapd and the radio, and sends no frame.
- **The APs already talk over the wire,** to each other's management addresses, in messages signed with the radio neighbours' key (0073). Every Aeolus AP heard in the air is known with its address.

## Decision

- **An AP that lets a client on says so to the other Aeolus APs it knows of,** over the wire.
  - It sends `{t: "here", ap, at, seq, macs}`, signed as the hellos are, unicast to each AP's management address, on the hellos' port.
  - The journey sends it, as it sees hostapd let the client on, on any network. It goes at most once a second for a client.
  - The APs are those radio resource management knows: its neighbours and the APs it hears in the air.
- **An AP that still holds the client lets go of it,** on each of its networks that has it: `DEAUTHENTICATE <mac> tx=0`, logged.
  - aeolus-rrm takes the message, signed and newer than the last from that AP.
  - **A client on for less than two seconds is left be:** it came back after the other AP let it on.
- **On only where radio resource management is,** whose key signs the messages and whose neighbours are told.

## Consequences

- **A client that comes back finds the AP free of it.** It is let on as a client new there is, by 802.11r where it can. There is no SA Query wait, and no refusal of its fast transition by an AP still holding the old one.
- **A client that went to another AP without a word,** as one that sleeps and wakes elsewhere, is let go of at once too, not minutes later.
- **Nothing is sent in the air:** a client is never told to go. Only an AP it has already left forgets it.
- **Only the APs with the key can make an AP let go of a client,** and a message sent again is refused.
- **What switches have learned of a client's MAC is not touched.** The AP it went to sent its own frames, as at any roam. Griff takes up any MAC path trouble on the switches should it show.
- **It is in the APs alone:** the manager is neither asked nor told.
