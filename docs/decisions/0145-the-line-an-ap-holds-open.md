# 0145. The line an AP holds open, and one clock for what APs see

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff, of Live Trace (0144) as it first went out, where an AP learned at its next poll that a client was to be traced: "we have to create some form of high speed streaming to do this much faster than a minute and at every poll. I want ms sync". Written up by Claude
- Refines: 0007, 0144, 0118, 0033

## Context

- **APs poll, and the manager never pushes** (0007), so that no AP needs a way in to it, behind CGNAT or anywhere. What the manager has to say to an AP waits for the AP's next poll, a minute where that is the setting.
- **A minute is too long to wait for a trace to begin,** when someone stands with a client in their hand.
- **What an AP saw went up slowly too:** `aeolus-journey` wrote it to a file, and the agent read the file four times a second and made a new connection for each sending.
- **Each AP timed what it saw by its own clock.** NTP holds two APs within some tens of ms of each other, and the steps of a roam are a few ms apart. On the lab's APs, measured: one 4 ms from the manager's clock, one 69 ms. What the two said of one client would have been put in the wrong order.
- **A request from an AP took 40 ms longer than the network needed,** every time, measured from the C-360: 41 to 43 ms each on the LAN, the first on a connection excepted. An AP's HTTP client, uclient, writes a request as several small pieces. TCP on the AP sends the first and holds the rest until it is acknowledged, and TCP on the manager waits 40 ms before it acknowledges a piece it has nothing to answer yet.

## Decision

- **Each AP holds one line open to the manager.** It asks `GET /v1/ap/live`, and the manager does not finish its answer: it writes a line of JSON on it when it has something to say, and the AP has it in the time the network takes. The AP opened it, so 0007's reason stands: nothing reaches in to an AP.
- **What the manager says on it is small and whole each time:** the clients to trace, each with the seconds it has left, as the AP joins and whenever they change; and its clock every ten seconds, so that a dead line is known.
- **The config is not sent on it,** nor is an AP told there to fetch one. Versions, applies and their checks stay with the poll (0007).
- **A new daemon holds it, `aeolus-live`.** Told whom to trace, it writes the file `aeolus-journey` reads and tells `aeolus-journey` to look at once. It asks again when the line ends or goes quiet, sooner where the line had held and later each time it does not, a little apart from the other APs.
- **What an AP sees goes up as it is seen.** `aeolus-journey` hands each thing to `aeolus-live` on a socket, and `aeolus-live` sends it by `POST /v1/ap/live` on a connection it keeps open, a few together where they came together. The agent sends none of it now.
- **Every AP's time is put on the manager's clock.** With each sending the AP says when it sent, by its own clock; the manager answers when that came and when the answer left, by its own. From the four the AP reckons how far its clock is off, as NTP does, and says it with what it sends next. The manager adds it to every time that AP names.
  - As a trace begins the AP reckons four times running, and what it sees waits up to a second for the first. While it traces it reckons every second.
  - **A clock that is being corrected moves.** The lab's test manager slid 0.45 ms a second against both APs while its own clock was being set right, so a reckoning sixteen seconds old was 7 ms wrong. The AP goes by the newest reckoning that travelled about as quickly as any of its last eight, none older than ten seconds, moved on by how fast the two clocks are parting, which it reckons from the same eight.
  - One that lands more than 3 ms from where the others put the clock says the clock was set: the others are let go.
  - An AP that has said nothing of its clock in the last five seconds is taken at its word, as if what it sent came the moment it was sent.
  - **What one AP saw close together keeps the spacing that AP timed it by:** within 100 ms of the thing before, for two seconds at the most. Two reckonings differ by a part of a millisecond, and without this two key messages the office's AP timed a millisecond apart came out the wrong way round.
- **The manager acknowledges at once what an AP sends** (`TCP_QUICKACK`, set again after every read and write, on Linux), which takes the 40 ms away: 0.5 to 2 ms a request, measured.
- **Whoever watches a trace is sent each thing as the manager has it:** `GET /v1/clients/{mac}/trace/live`, an answer that does not end either. The UI draws from it, and asks every two seconds instead where it cannot be held.
- **Nothing rests on the line.** An AP without one learns at its poll whom to trace, by the `Aeolus-Trace` header, as before, and still sends what it sees. An AP without `aeolus-live` serves as it did and is only not traced.

## Consequences

- **A trace begins in milliseconds.** Measured on the C-360 against a manager on the lab's LAN: its first word of the client timed 3 ms after the asking, and with the manager 5 to 6 ms after it happened.
- **What two APs say of one client is in its true order** to about a millisecond. Each reckoning is as wrong as half the difference between the way there and the way back; on the lab's LAN each landed within half a millisecond of where the ones before had put the clock. Over a WAN it is as good as the two ways are alike.
- **What a step's time is worth is the AP's own noting of it besides:** a step from hostapd's log is timed by when the log took the line in, to the millisecond, and one from a frame by when the kernel took the frame in, where it had been by when `aeolus-journey` got to reading it.
- **The manager holds a connection for every AP,** idle but for ten bytes every ten seconds. A manager of thousands of APs holds thousands.
- **One more daemon runs on each AP,** asleep unless the manager speaks or a client is traced.
- **A trace is shown with how well it is being heard:** how many of the APs hold their line, each one's clock against the manager's, and how long after it happened the manager had each thing.
- **An AP whose clock is far out is still placed right,** so long as it holds its line. One without `aeolus-live` is placed by its own clock, as before.
- **Other things the manager would say sooner now have a way:** an action asked of an AP (0104) waits for its poll today, and so does a new config.

## Not decided

- **Saying on the line that an action waits, or that the config has changed,** so an AP polls at once. A config that reached every AP of a site in the same instant would restart their Wi-Fi together, which the poll's spread does not.
- **The key agent's long poll (0070) on the same line.**
- **The manager behind a proxy** that holds back an answer until it ends. The line would open and say nothing; APs would fall back to the poll.
- **Sending on the line what is not a trace's:** a client's joining as it happens, for everyone, always.
