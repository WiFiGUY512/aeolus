# 0225. A test client that roams

- Status: Accepted
- Date: 2026-10-27
- Proposed by: Griff, wanting a before-and-after of roaming speed once the APs share keys (0224): "Can't you have a client roaming effect by simply having the radio join the other ap even if its signal isn't perfect. There are 4 aps in the office alone". Written up by Claude
- Refines: 0121 (client tests), 0123 (service assurance), 0136 (roaming)

## Context

- **Roaming was measured only from the clients' own roams** (0136). They come when people walk about, by the clients' own choice: whether they use 802.11r, and when. A day of them is a few dozen, and says little of one network on one band.
- **An AP with a scan radio already joins its networks as a client would** (0121), on itself and on the APs it hears well (0123). It joined as plain SAE or WPA2, even where the network has 802.11r, so it never tried it.
- **wpa_supplicant roams on asking:** `ROAM <bssid>` reassociates with a BSSID it has heard, whatever the signal, and whatever `bssid_accept` the network has. On C360-AP's scan radio, 2026-10-27, Home Wi-Fi on the office AP's 5 GHz:
  - It roamed to TestC360, the C-260 and Living Room, and back, all four by 802.11r (FT-SAE). Its gateway answered after each.
  - From its first authentication to being let on, by its own clock, it took 316 to 448 ms. `SME: Trying to authenticate` is said before the radio has gone to the other channel. On a C-360's scan radio that is about a third of a second, and on a DFS channel it waits to hear the AP first. The AP it goes to times the roam from the frames it gets, as the roaming page times every client's.
  - The first look, before the join, can miss a BSSID: Living Room on channel 165 was missed once, and `ROAM` refused it as unknown.
- **What wpa_supplicant says of a roam:**
  - "Key negotiation completed" comes after an 802.11r roam too (`wpa_sm_notify_assoc`), so it does not tell them apart.
  - A new key held for the AP (`PMKSA-CACHE-ADDED`) or an 802.1X sign-in (`CTRL-EVENT-EAP-STARTED`) does say a sign-in again.
  - `STATUS`'s `key_mgmt` says whether it went by 802.11r.

## Decision

- **A network with 802.11r is joined as a client that uses it:** `FT-SAE`, `FT-PSK` or `FT-EAP` offered before the rest. Every test of such a network goes by 802.11r, as a modern phone would.
- **`assurance.tests.roam`, on a Locations folder, has each test through the air roam once it is on and measured:**
  - The manager names the APs: each other AP its tester tests the same network on, on that band, in order, by its BSSIDs. At most eight.
  - The agent goes to each one's BSSID heard on that band, then back to where it began. Before each roam it makes sure wpa_supplicant has heard the BSSID, looking on its channel once if not.
  - Each roam records: whether it was let on, its time from the first authentication there and from the asking, how the client came by its keys, any refusal on the way (as of 802.11r where the AP has no key), and whether the gateway answered after.
  - A roam not let on does not fail the test. The roams after it go on only while the client is still on.
- **The assurance page says what the roams came to, one band at a time:** for each network, the roams, those not let on, and each kind's typical, least and most time.
  - Each roam is as the AP it went to recorded the test's client, matched by the client, the AP, the network and the time: the same scan radio tests one network after another, a minute or so apart. That is the same measure as the clients' own roams on the roaming page: 802.11r, a key held (OKC or PMKSA caching), the WPA2 key handshake, or a sign-in again.
  - A roam the AP did not time is counted, of the kind the test says, and kept out of the times: the test's own clock takes in the scan radio's channel change. On a network that signs clients in by 802.1X, an AP times no 802.11r roam, as it keeps its log to its own level there, which says nothing of the authentication (0133).
  - The test's line says its roams by its own clock.

## Consequences

- **A steady measure of each network's roams on each band**, on the schedule, whether or not anyone walks about. It is the before-and-after of 0224: a network with 802.11r against one without, and a test before the keys were shared against one after.
- **Each roam is a second or two more of the scan radio,** with airscan waiting. The setting is off unless set.
- **The tests' roams stay off the roaming page,** whose clients are people's (0136). They are on the assurance page alone.
- **A network that signs clients in is roamed with the test user's sign-in** (0124). With 802.11r or a key held, a roam needs no RADIUS round.
