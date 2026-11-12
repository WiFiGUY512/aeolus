# 0190. A client that comes online again and again

- Status: Accepted (Griff, 2026-10-21: "Keep working hard and deploying all night", under his standing word to merge what is ready)
- Date: 2026-10-21
- Proposed by: Claude, from two IoT devices found rejoining every 44 s overnight
- Refines: 0099, 0103, 0118, 0127

## Context

- **Two IoT device 4s (Espressif) came online about every 44 s, all night,** once the converted C-360s were back on 2026-10-20 at 22:48:
  - on Living-Room-C-360 and OpenWrtce7f, about 80 times an hour each;
  - one made 2,925 attempts in 8 days;
  - every one got online, and each time the device dropped the connection itself, with no deauthentication. It sent a probe and authenticated afresh, and the AP ended the old session only then;
  - on the office AP, from 2026-10-17 to 2026-10-21, one session held for 4 days.
- **Nothing in Aeolus said so:**
  - no alert: every attempt came online, so `joins-failing` (0127) does not count them;
  - the journey (0103), built from state reports minutes apart, saw the moves between the two C-360s and called them "ping-pong". It advised a lower power or a minimum signal, which is the wrong cause and the wrong cure;
  - it was found only by a DHCP capture: the robots asked for an address every 45 s.

## Decision

- **An alert, `rejoins`, a warning on the AP:** where one client came online there 20 times or more in the last hour.
  - It names the client by the name a person gave it (0153), else the name it gives itself, beside its MAC.
  - It says how many times, about how often, and that the client drops its connection itself: something of that AP's radio does not suit it.
  - It is keyed `rejoins:` and the MAC, so each such client is its own alert.
  - The APs' own scan radios making client tests (0121) are left out, as from `joins-failing`.
- **An issue in the client's journey, `rejoins`:** where it came online at one AP 20 times or more within any hour of the time asked. It is read from the APs' records of its attempts (0118), which see what reports minutes apart cannot.
- **Where `rejoins` is said, `ping-pong` is not:** the moves are the client coming back, not coverage that overlaps.
- **20 an hour is one every three minutes.** No client that stays comes back so often. One walking between rooms comes back to an AP a few times an hour.

## Consequences

- **A device in a loop like the robots' is an alert within the hour,** on the AP it loops on, by name.
- **The journey points at the AP, not at power or coverage.**
- **Why the robots loop on the C-360s is not known yet.** Both the C-360s and the office AP run 802.11ax on 2.4 GHz with the same HE settings, so 11ax alone is not the difference. What differs:
  - **the chip and its driver:** Qualcomm IPQ8074 on ath11k on the C-360, MediaTek MT7981 on mt76 in the office AP (an OpenWrt One);
  - **TWT:** the C-360 answers it (`he_twt_responder`), the office AP does not;
  - **HT capabilities:** the C-360 has `DSSS_CCK-40` and not `MAX-AMSDU-7935`.

  Any experiment with the radio's settings restarts it, so it waits for Griff.

## As built (2026-10-21)

- **The alert:** `rejoining()` in `internal/api/joins.go` tallies the hour's attempts that came online by AP and client, from `conditions.Join`, which now has `FirstOn`. It is passed through `alertsOf` to `alerts.Input.Rejoins`.
- **The journey issue:** `journey.Rejoins()` takes the client's attempts that came online, an AP at a time; `clientJourney` reads them, the latest 1,500.
- **The descriptions:** the MCP tools `list_alerts` and `get_client_journey`, and `docs/api.md`.
- **Tested:**
  - `TestRejoins` in `internal/api`: the alert by the client's own name, the journey's issue, and none for a client that came back five times;
  - `TestRejoins` in `internal/journey`: the most within any hour, an AP at a time, fewer than 20, and `ping-pong` taken back.
