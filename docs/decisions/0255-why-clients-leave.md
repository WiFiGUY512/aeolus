# 0255. Why clients leave

- Status: Accepted (Griff, 2026-10-31: "do why clients leave"; and "make sure all of your coding is extensible, so we can always do things to speed things up and not have to refactor the entire code base")
- Date: 2026-10-31
- Proposed by: Claude
- Refines: 0252 (a client is where it joins), 0254 (an attempt from hostapd's frames)

## Context

- **Each AP says a client's leaving the moment hostapd lets go of it** (0252, "left" news), but not why. hostapd's `AP-STA-DISCONNECTED` gives no reason.
- **The frames do, for every leaving, not only in an attempt** (0254: hostapd's rings, read on every AP):
  - **the client's own deauthentication or disassociation**, with its reason: "leaving" (3), "leaving the network" (8), or one saying something was wrong;
  - **the AP's, sending it off:** idle too long (4); too many frames went unacknowledged (34), a client out of reach; its keys failed (14, 15, 16); its sign-in failed (23); too many clients (5); a BSS transition (12);
  - **the AP's to every station:** its network going down, or starting anew.
- **Some leavings have no frame:**
  - the client simply went;
  - radio resource management let go of a client that joined another AP (0226), with `tx=0`, so that nothing goes to a client no longer there.
- **Aeolus sends clients off itself:** a person's disconnect (0107), steering (0138), a network taken off the air (0201). Each is a frame with a reason that hostapd might as well have sent.
- **So a leaving said today is one thing:** the phone that walked out the door and the plug that keeps falling off 2.4 GHz look the same.

## Decision

- **Each AP says why, with the leaving's news.** The agent says it raw:
  - **`by`:** `client`, `ap`, `aeolus` or `none`;
  - **`frame` and `reason`:** the leaving frame within 5 s before hostapd let go;
  - **`all`:** where the AP's frame went to every station;
  - **`aeolus`:** the word of the Aeolus daemon that sent it off: `moved`, `asked`, `steered` or `off-air`.
- **Aeolus's daemons say what they send off in a registry** (`aeolus/sentoff.uc`). Each writes a file of its own under `/var/run/aeolus/sent-off/`, kept a minute, and the journey daemon reads them as hostapd lets go. A new daemon that sends clients off writes there with a word of its own, and nothing else changes.
- **The manager keeps each leaving as a client event** (`client_events`, kind `left`), with what was said, and how strongly the AP last heard the client, from its minutes (0251). It is kept as long as the attempts are.
- **The manager names each leaving by a table of rules** (`leaveRules`), the first that matches:
  - `moved`, `asked`, `steered` (also the AP's reason 12), `off-air`, `aeolus`;
  - `network` (to every station);
  - `leaving` (the client said so) and `client-other`;
  - `idle`, `unheard`, `keys`, `signin`, `busy`, `state` (the AP no longer held it), `sent-off`;
  - `silent` (no word).
  - Each has its words, and whether it is **worth a look** (`trouble`): not the client's choice nor Aeolus's.
- **`GET /v1/leaving`** has them for the APs the caller may view, by Locations node, client or band, over up to 90 days:
  - each band's leavings counted by kind, bands never added together;
  - the latest 50.
  - The APs' own test clients are left out. The MCP tool is `get_leaving`.
- **The page:** a folded panel, "Why clients left", on an AP's and a folder's Overview, and "Left" on a client's page.
  - A line a band, each kind a chip, those worth a look marked;
  - the latest, with who sent each off and why, and how strongly the AP last heard it.

## Where it extends

- **A new kind of leaving** is a row in `leaveRules`, and its word in the page's `SHORT`. A word the page doesn't know shows in the manager's words.
- **A new event of a client's** is another `kind` in `client_events`, with what it says as JSON: no new table, and the same query (`ClientEventQuery`).
- **More said of a leaving** is a field in the news and in `leaveDetail`. An older manager passes it over; a new one keeps what it knows.
- **Faster views:** `client_events` is indexed by kind and time and by client and time. A view that is slow over 90 days can keep its counts by the hour, as the trends do (0140), without the rest changing.

## Consequences

- **Each leaving says why,** where it said nothing: a client out of reach (`unheard`), one that sleeps without a word (`idle`, `silent`), the AP's network going down, one Aeolus moved on.
- **A leaving from an agent before v0.206.0 says no why, and isn't kept.** Its news still moves the client (0252).
- **An AP without rings (0146) has no frames:** its leavings are `none`, but for those Aeolus's own daemons said.
- **The agent changes:** `aeolus-journey`, `aeolus/journey.uc`, the new `aeolus/sentoff.uc`, and the four daemons that send clients off. **The manager changes:** a migration, the news, `GET /v1/leaving`, the MCP tool and the page. The manager goes first, as always.

## Not done here

- **An alert on a band whose leavings are mostly worth a look.**
- **Leavings in the assurance rounds** (0123).

## Since: the client's last minute and its silence (2026-10-31, v0.206.1)

- **Each leaving keeps the client's last minute at the AP and how long it had sent nothing,** for telling a client asleep and sent off, at a good signal, from one out of reach.
  - The minute is from the minutes kept (0207), in the ten before it left: signal, packets sent, retries, failures, packets come from it, and how long before.
  - The silence is `inactive_ms` from the AP's latest report.
- **Asked by the porting session for B122 [suspect]:** ath11k's firmware sent a kettle off for missing ACKs while the AP heard it at −62 dBm with nothing tried again.
