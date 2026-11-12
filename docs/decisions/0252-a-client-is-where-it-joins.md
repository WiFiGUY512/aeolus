# 0252. A client is where it joins

- Status: Accepted (Griff, 2026-10-31: "whats the next feature"; 0251's later step, which waited for the ce7f test)
- Date: 2026-10-31
- Proposed by: Claude
- Refines: 0251 (a client is where its minutes say), 0185 (a client is on one AP at a time)

## Context

- **Since 0251, a client shows on the AP it joined within about a minute,** at the AP's next minute (0207).
- **0251 left one step for later:** the APs saying a join the moment it happens. It changes the agent, so it waited until the ce7f test was over. B114 is fixed in the port's -63, on all four ath11k APs since 2026-10-30, so the test is over.
- **Each AP already hears every join and every leave as it happens:**
  - aeolus-journey (0118) reads hostapd's lines;
  - a client let on, by its keys or by a fast transition, is the moment its attempt is `on`;
  - `AP-STA-DISCONNECTED` is hostapd letting go of a client it held as on.
- **aeolus-live (0145) sends news at once:** it already did so for power control's steps (0207).
- **A minute is coarse:** what a client sent in a minute may have been at any time in it.
  - The AP a client roamed from often keeps it the whole minute it roamed in, so that minute says it was there 60 s, sending.
  - By 0251's rule, a minute counts as at its start plus its seconds. The minute the client roamed in would then put it back on the AP it left, for a minute.

## Decision

- **Each AP says, at once, that a client joined it or left it.**
  - aeolus-journey hands aeolus-live news, which aeolus-live sends to the manager on its next turn:
    - `joined`: once per attempt, the moment the client was let on;
    - `left`: as hostapd lets go of it.
  - Each news item gives the client's MAC, the time by the AP's clock in ms, and the network's SSID, band and name. A join also gives how strongly the AP hears the client, where it knows.
- **The manager takes a join as 0251 takes a minute,** but at the exact moment:
  - the client is on that AP from then;
  - the AP's condition names it `arrived`, with its band, network and signal, until a minute or the AP's report says more;
  - the clients list (0118) has it on that AP.
- **A leave takes the client off the AP it was on, onto none.**
  - The AP's condition names it `elsewhere`, and the manager's counts leave it out, until the AP's next report, which no longer lists it.
  - A leave from an AP the client is no longer on says nothing. For example, the AP it roamed from lets go of it when the new AP says so (0226).
- **A join or a leave is exact; a minute is not.**
  - A minute begun before a client joined another AP, or left this one, does not put it back.
  - Against a record a minute made, a join or a leave counts from that minute's start.
  - Anything older than what is known says nothing, as before.

## Consequences

- **A client roaming, joining or leaving shows within seconds** wherever 0251 showed it within a minute:
  - the Clients tab and every count made from it;
  - the clients list;
  - `list_clients` and the MCP tools that read it.
- **The roam minute no longer puts a client back on the AP it left.** 0251 allowed that for a minute.
- **A client that leaves goes from its AP's list at once,** not at the AP's next report.
- **The agent changes:** aeolus-journey and aeolus-live. An older manager passes over the new kinds of news. An older agent sends none, and its APs go by their minutes, as with 0251.
- **What each AP reports, and when, is otherwise unchanged.** Still kept in memory for an hour, as 0185's record is.

## As built (2026-10-31)

- **The agent:**
  - `aeolus/journey.uc`: `moved`, the news item for a join or a leave;
  - `aeolus-journey`: `news`, and `on_log` saying a leave on `AP-STA-DISCONNECTED` and a join as an attempt is first `on`;
  - `aeolus-live`: no change to how news is sent.
- **The manager:**
  - `internal/api/minutes.go`: `newsOf` reads `joined` and `left`, the same time bounds as a minute;
  - `internal/api/active.go`: `activity.moved`, `activeOn.exact`, `left` and `from`; `minutes`, `note` and `elsewhere` follow them;
  - `internal/conditions/connections.go`: `ClientsJoined` moves a client in the clients list.
- **The page:** the "new here" cell's title says a join too.
- **Tested:**
  - **The agent:** `agent/test/journey.uc` prints `moved` for a join, a leave, a join with no signal, and two it refuses.
  - **The rule in Go,** `TestActivityJoins`:
    - a join moves the phone at once;
    - the old AP's minute begun before the join does not put it back;
    - the old AP letting go of it says nothing;
    - an old join is passed over;
    - a leave takes it off, and neither a minute begun before the leave nor a report from before it puts it back;
    - a join again does.
  - **Through the API,** `TestClientWhereJoins`:
    - the office's join makes the living room's condition name the phone elsewhere;
    - the office's condition has its arrival, with its signal;
    - the clients list has it on the office;
    - the office's leave ends the arrival;
    - a join news item with no MAC is passed over.
