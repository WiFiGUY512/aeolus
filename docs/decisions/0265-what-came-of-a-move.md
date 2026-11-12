# 0265. What came of a move

- Status: Accepted (Aeolus's own development, 2026-11-01, at Griff's word to keep making fixes and features overnight)
- Date: 2026-11-01
- Refines: 0073 (radio resource management), 0120 (DFS channels after a look)

## Context

- **An RRM move is a channel switch.**
  - Outside DFS it is announced, and clients that follow keep their association.
  - To a DFS channel, the radio's networks start again after a minute's radar check, and every client has to join again.
- **Power control watches what its clients do after a step down (0206).** Nothing records what a move did to the radio's clients. Its samples are taken only while power control is on, which it is not at Home.
- **The question now:**
  - Six APs share 2.4 GHz's three channels, and moved there back and forth (0264).
  - Look-first (0120) can now move 5 GHz radios to DFS.
  - The IoT clients are on 2.4 GHz. Whether they follow a switch, or drop and join again, says what each move costs.

## Decision

- **At each move it makes, the RRM daemon reads the radio's clients from the driver:** each by MAC, with when it associated.
- **Two minutes after the move is made, it reads them again** (`FOLLOW_AFTER`, 120 s). After a radar check, the two minutes start once the check is done. A move that fails, or a check that says nothing for ten minutes, is not counted.
- **It says, of the clients the radio had (`rrm.followed`):**
  - `kept`: on it still on their own association, so they followed;
  - `back`: on it again on a new association, so they dropped and joined again;
  - `gone`: not on it.
- **The counts go on the move,** as `clients: {clients, kept, back, gone}`, in the report's `rrm.moves`. The daemon logs them too.
- **A driver that gives no time of association has every client that is there counted as kept.**
- **Shown in Neighbours › Moves,** beside what came of the move: "10 of 12 followed", amber where any are gone, with the rest on hover.

## Consequences

- **Each move says what it cost,** so a later rule can weigh it, such as fewer moves on a band whose clients do not follow.
- **The daemon reads the clients once at a move and once two minutes on.** It doesn't sample them all the time, as power control does.
- **Clients that left in those two minutes for their own reasons are counted as gone.**
- **Checked:** `agent/test/rrm.uc` (kept, back, gone, a driver with no times, nothing before), on talos with CI's ucode. The report's check takes the counts and refuses ones that do not add up.
- **Live, v0.213.0, 2026-11-01:**
  - Office's move from 149 to DFS 52 at 02:48 CST: 2 clients, 0 followed, 0 back, 2 gone, counted two minutes after its radar check ended.
  - The C260's move from 44 to 36 at 02:47: none.
