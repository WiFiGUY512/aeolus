# 0351. The wire as a choke point

- Status: Accepted (Griff, 2026-11-10: "I like the flag for a wired congestion or choke point")
- Date: 2026-11-11
- Proposed by: Griff. Written up by Claude
- Refines: 0207 (telemetry by the minute), 0053 and 0093 (an AP's ports and their speeds), 0096 (the uplink's ports)

## Context

- **Aeolus judged the air, not the wire.** The alerts said a radio was busy, slow or missing beacons, but nothing said the AP's own uplink was what held its clients back.
- **The uplinks in the house are 1 to 5 Gbit/s** (2026-11-11): the C-360s, the C260, the W-318 and the Office at 2.5, C360-AP at 5, the Shed at 1. C360-AP carried 1.75 Gbit/s in porting's NSS lab (2026-11-10), so a 2.5 Gbit/s uplink can run full with a few busy clients on 5 and 6 GHz.
- **The state report every five minutes is too coarse for this.** It has each port's counters (`uplink_port`), but an uplink full for a minute is lost in five minutes' average.
- **aeolus-live already samples the radios every 5 seconds** and sends what each minute came to (0207).
- **The scheduled speed tests run over the air from the scan radio,** at 50 to 740 Mbit/s, and say nothing of the wire.
- **No alert said that a link had negotiated a slower speed than both ends offer, ran at half duplex, had errors, or kept losing its link.**

## Decision

### What the AP says

- **aeolus-live samples each of the uplink's Ethernet ports every 5 seconds,** beside the radios:
  - the ports are a bond's members, else the uplink itself, as the renderer names it (`aeolus.uplink`), looked up again each minute, at most 8;
  - from sysfs: bytes and errors each way, how often its link came or went (`carrier_changes`), its link, speed and duplex.
- **Each minute says, a port at a time, `uplink`:** `{port, up, speed, full, secs, rx, tx, rx_max, tx_max, rx_bytes, tx_bytes, errors, changes}`.
  - `rx` and `tx` are Mbit/s on average over the seconds counted (`secs`). `rx` is what came in to the AP from the switch, which its clients download; `tx` is what went out.
  - `rx_max` and `tx_max` are its busiest five seconds each way.
  - A 64-bit count grows by more in five seconds at 5 Gbit/s than a radio's counts may (`STEP_MOST`), so the ports' counts have their own rule (`wire_grew`): a 32-bit kernel's count that goes round once is counted. A stretch faster than its link can carry is a count that started again, and is passed over for its bytes; its errors and changes of link still count.
- **What it decides is `aeolus.minutes`** (`port_sample`, `uplink`, `wire_grew`), tested in `agent/test/minutes.uc`.

### What the manager keeps

- **`uplink_minutes`, a row a port a minute,** kept as long as the radios' minutes, 30 days, and pruned with them (conditions schema 42).
- **A manager takes a minute with no `uplink`**, an agent's from before, as before. A port not well said is left out, and the rest kept.

### The flag

Each a warning on the AP, by port:

- **`uplink-full`:** in 3 minutes or more of the last 15, the port ran full, either way: its busiest five seconds at 90% of its link or more, or its minute's average at 70%. "lan1, its 2.5 Gbit/s uplink, ran full in 4 minutes of the last 15, at 2.4 Gbit/s at its busiest, into the AP, to its clients: there the wire, not the Wi-Fi, held its clients back."
- **`uplink-errors`:** 10 errors or more in the last 15 minutes: a cable, a connector or the switch port.
- **`uplink-flaps`:** its link lost twice or more in the last hour. Two changes of carrier are one link lost.
- **`link-slow`:** from the AP's latest report, any of its Ethernet ports that links slower than both it and the switch offer (`speed` below the lesser of `max` and `partner_max`, 0093): "lan2 links at 1 Gbit/s, though it and the switch both offer 2.5 Gbit/s".
- **`link-half`:** any port at half duplex.

The thresholds are constants beside the rule (`UplinkFullPct`, `UplinkAvgPct`, `UplinkFullMinutes`, `UplinkBusyWindow`, `UplinkErrorsFewest`, `UplinkLossesFewest`, `UplinkLossWindow`).

### Where it shows

- **Wired › Uplink, a part of its own:**
  - on an AP, a line a port (its link, its busiest each way as a share of its link, its errors and links lost over the span) and a chart: each way on average, its busiest five seconds dotted, and its link's speed dashed, the line the others cannot pass;
  - on a folder, a row an AP and port with the same numbers, a busiest of 90% of its link or more marked.
  - An hour to 3 days. Over many hours a point stands for several minutes: their average weighed by the seconds each counted, and the busiest of them.
- **`GET /v1/aps/{ap}/uplink?hours=N`** gives it: 24 hours unless set, at most 72, at most 720 points a port.

## Consequences

- **A wire that holds clients back is flagged within minutes,** with which way and how hard. A cable that drops to 1 Gbit/s, a port at half duplex, and a link that comes and goes are said too.
- **It sees the AP's own uplink, not past it.** A full trunk between two switches, or a router that chokes, shows only where it fills an AP's uplink. The switches' own counters would say more (0168 reads Arista switches); not yet.
- **Each AP sends a row a port a minute more,** about 200 bytes. The manager keeps about 43,000 rows a port over 30 days.
- **An agent before this release says no uplink:** its APs show "nothing said of its uplink yet" until they take the new agent.
- **Checked by:** the agent's minutes test, and the manager's tests of the store, the parsing, the chart's points and each alert; after release, each AP's minutes carrying its uplink, and the Wired › Uplink part on the house APs.
