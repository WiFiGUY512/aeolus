# 0207. Telemetry by the minute, and as it happens

- Status: Accepted (Griff, 2026-10-22: "I feel like we need some kind of near real time telemetry from the AP to give the APs more nimble options"; then "dont we need it to run constantly for infrence?"; then "yeap, do that")
- Date: 2026-10-22
- Proposed by: Griff; written up by Claude
- Refines: 0145, 0140, 0206, 0205, 0073

## Context

- **What the manager knows of the air comes once a minute, as a snapshot.** Each state report says each radio's power and each client's signal, rate and counters as they are the moment it is made. What happened between two reports is not in either.
- **Inference needs the shape of each minute, all the time.** The advisor (0205), and any model after it, learns from what a channel's load, a client's signal and its retries did over time, beside what happened: a step, a roam, a failure. A snapshot a minute misses the spikes and the dips, and nothing at all is kept of a minute no one looked at.
- **What changes is news before the next report.** A power step (0206) reaches the manager up to a minute late, in the next report.
- **Every AP already holds a line to the manager** (0145), on which `aeolus-live` sends what it sees of traced clients at once, with its clock reckoned against the manager's.
- **The APs' own decisions need none of this from the manager.** They read their own radios, and hear their neighbours in the RRM hellos every 10 s (0073).

## Decision

### Four parts, in this order

1. **Minute summaries, always on, and events as they happen.** Built first: what inference and the advisor feed on.
2. **A live view, on demand:** while someone watches, the APs they watch send every few seconds over the line.
3. **A flight recorder:** each AP keeps its last hour at full detail, and sends the minutes around anything that goes wrong.
4. **Load in the hellos:** each AP tells its neighbours how busy each band is, for power control and steering to use.

### 1. Minute summaries

- **`aeolus-live` samples every 5 seconds,** and sends what each minute came to, at the minute's end. It reads the radios with nl80211:
  - each radio's survey of its own channel;
  - its power, channel and width;
  - each client's station counters, signal and rates.
- **A radio's minute:**
  - its band, channel, width and power;
  - how busy its channel was: on average, and at its busiest 5 s;
  - how much of that was its own receiving and sending;
  - its noise floor;
  - its clients, on average and at most;
  - what was sent to them: packets, those tried again, those lost, and bytes each way.
- **A client's minute:**
  - its MAC, band and SSID;
  - its signal, average and weakest;
  - its rates each way;
  - what was sent to it and tried again or lost;
  - bytes each way;
  - for how much of the minute it was there.
- **Each minute is stamped by the AP's clock,** on the minute. NTP holds the APs within tens of ms, enough for minutes.
- **What cannot be sent waits:** up to 60 minutes, oldest dropped first. A manager that is down for a while misses nothing shorter.
- **The manager keeps:**
  - radios' minutes for 30 days;
  - clients' minutes for 7.

  `GET /v1/minutes` reads them, and the MCP tool `get_minutes` gives them to the advisor.

### Events as they happen

- **A daemon hands an event to `aeolus-live`,** on its socket, as `aeolus-journey` does, and it goes up at once.
- **Power control's steps (0206) are the first:** the manager keeps each as it keeps those of the report, once, whichever comes first.

### The page

- **Interfaces › Radios › Load,** one band at a time:
  - each AP's channel;
  - how busy it was in the last minute, on average and at its peak;
  - the last hour as a line;
  - its clients;
  - retries;
  - noise.

## Consequences

- **The manager has every minute of every radio, and of every client, whether anyone looked or not.**
- **About 6 MB a day for an AP with 40 clients** is sent and kept, a few hundred bytes a minute for a radio and some 150 for a client. At 1,000 APs, minutes are kept and 5-second samples are not.
- **A power step is on the manager within a second.**
- **Nothing rests on it.** Without `aeolus-live`, or without the line, the AP serves as before, and the state reports carry what they always did.
- **The manager is still in no control loop:** the APs decide; the minutes are for seeing and for inference.

## As built (part 1)

- **On the AP:**
  - `aeolus.minutes` (`agent/files/usr/share/ucode/aeolus/minutes.uc`) works out a minute from its samples, with tests (`agent/test/minutes.uc`);
  - `aeolus-live` samples on every fifth second, by the calendar's clock, so a sample on the minute ends one minute and begins the next;
  - `aeolus-live -m N` prints N minutes and ends, for trying it out beside the one procd runs.
- **The survey that counts is the one of the frequency the radio serves on:**
  - **The flag that says which is in use cannot be trusted.** ath10k (the R7800) and mt76 (the OpenWrt One) never set it. The C-360's 5 GHz radio and the C-260's 2.4 GHz radio set it on the channel RRM last looked at.
  - **ath11k's retries are counted in debugfs, as the prober reads them (0164),** and only at a minute's ends.
- **Tried by hand on four APs before release, 2026-10-22:**
  - **Every one said:** busy, noise, its clients, their signals, rates and counts.
  - **A minute's own numbers looked as they should.** C360-AP's 2.4 GHz was 27–37% busy, at most 78%, with 17 clients. The R7800's was 16%, the OpenWrt One's 15–16%.
- **The manager:**
  - **It keeps the minutes** in `radio_minutes` and `client_minutes` (migration 27).
  - **A minute is read alone,** so a field from a newer AP never has the whole sending refused, the traced clients' events with it.
- **News:** power control's steps, from `aeolus-rrm` through aeolus-live's socket, kept as the report's are.
- **The page:** Interfaces › Radios › Load, by the band picker. Each AP gives:
  - its channel;
  - the last minute's busy, and its peak;
  - the last hour as a line;
  - clients;
  - the retried share of the last five minutes;
  - noise.

## As built (part 4: the APs act on it)

- **Each AP's own load, locally.** `aeolus-live` writes what each radio's last minute came to to `/var/run/aeolus/load.json`:
  - how busy the channel was, on average and at its busiest;
  - its clients;
  - the share of what was sent that was tried again or lost.

  `aeolus.minutes`' `load` reckons the share from the larger of the two counts, at most 1.
- **Load in the hellos.**
  - RRM says each radio's load in its hellos while that minute is fresh, three minutes (`rrm.load_said`).
  - It lists each neighbour's radio BSSID and load in its state (`rrm.load_heard` checks what a hello says).
  - The Neighbours page shows each neighbour's load under its channel.
- **Faster power control.**
  - A step down is watched every minute from two minutes on, not once at ten. At each look, the time since the step is set against as long before it, with the same `rrm.distress`. At ten minutes the watch ends.
  - Clients that scan, leave or need more tries at once say so within two minutes. Thin evidence still takes longer: a window must hold 200 packets before retries count.
  - The distress says how many seconds after the step it was seen (`after`), and the Power page says so.
- **Steering by load.**
  - A weak client asked to go is offered the network's other APs, the least busy first (`steer.by_load`). Each AP's busy figure comes from its hellos, matched by the middle four bytes of its BSSID and the band of the place's neighbour report: all of a C-360's radios share the middle (36:86:2d:ef:2f:c0 on 5 GHz, :d0 on 2.4 GHz), and matched by the middle alone its three radios took one load, whichever its hello said last (found and fixed 2026-11-02). Those not known come after, as hostapd lists them.
  - No client is asked up to a band whose channel was 80% busy or more in the last minute (`steer.FULL`).

## As built (part 2: the live view)

- **Opening it:** Radios › Load › **Watch live** opens `GET /v1/live` for the node. It is held open, as Live Trace's is.
- **Telling the APs:** each AP below is told on its line (0145) to send for two minutes, `{"watch": 120}`, and told again every minute while anyone watches. When no one does, it is told `{"watch": 0}`. An AP that opens its line while it is watched is told at once.
- **On the AP:**
  - `aeolus-live` already samples every five seconds for the minutes.
  - While watched, it works out each five seconds as a minute is reckoned, from the sample before to this one, and sends it at once as a `point`.
  - It reads ath11k's retries every time while watched, not only at a minute's ends.
  - It keeps two minutes of points while it cannot send them.
  - `aeolus-live -w N` prints N points and ends, for trying it out beside the one procd runs, as `-m` does minutes.
- **On the manager:**
  - **It keeps an AP's last quarter hour of points in memory, and none on disk,** each once by its time. Whoever watches is sent the last five minutes as they begin, and each new point as it comes.
  - **A viewer sees the APs they may view, and no others.**
- **The page:**
  - **What it shows,** one band at a time:
    - each radio's channel, busy now, the last five minutes as a line, clients, the retried share of the last 30 seconds, noise and power;
    - its clients, those moving the most data first, with signal, rates, retries and the data they move now.
  - **It keeps its answer open across the page's own redrawing,** and stops when Stop is pressed, when another radio view is opened, or ten seconds after its page is left.
- **Cost, only while watched:** about 100 bytes a radio and 150 a client every five seconds.

## As built (part 3: the flight recorder)

- **The AP keeps half an hour.** `aeolus-live` now works out what each five seconds came to all the time, not only while watched. It keeps the last half hour in memory, each point with its 24 busiest clients: some 360 points.
- **What goes wrong is judged against the radio's own last five minutes** (`aeolus.record`, tested in `agent/test/record.uc`):
  - **retries:** at least 40% of 200 packets or more tried again or lost in five seconds, where its usual is under 15%;
  - **clients-left:** at least three of its clients, and a third of them, gone in five seconds;
  - **busy:** its channel 95% busy or more for fifteen seconds, where its usual is under 60%.
- **Two more ways a recording begins:**
  - power control's clients taking a step down badly, its distress news (0206);
  - someone asking: Record now, `POST /v1/recordings`, told on the line.
- **The AP begins at most one recording itself each half hour.** What goes wrong again in one makes it longer, to an hour at the most.
- **Sending:** the points from ten minutes before go up at once, and each one after as it comes, to ten minutes after. They go with the recording's own `record` on the line, and up to 480 points wait while the manager cannot be reached.
- **The manager:**
  - keeps `recordings` and `recording_points` (migration 29) for a month, each point once;
  - lists and reads them by `GET /v1/recordings` and `GET /v1/recordings/{id}`;
  - gives the advisor `list_recordings` and `get_recording`.
- **The page:** Radios › Load has **Recordings** below the minutes, with Record now. One opened shows, on the band chosen:
  - each radio's busy and tried-again lines over the recording, what went wrong marked;
  - its clients: their signal before, at it and at the end, what they say they hear (0208), what was tried again before and after, and those gone.
