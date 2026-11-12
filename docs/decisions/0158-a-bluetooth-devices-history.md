# 0158. A Bluetooth device's history

- Status: Proposed
- Date: 2026-10-14
- Proposed by: Griff, to "a device's history over days: where it was heard, when": "Lets do history now". Written up by Claude
- Refines: 0157, 0118, 0140

## Context

- **What an AP's Bluetooth radio heard in the last ten minutes is in its report** (0157), and a report says again what the one before said. Read from the reports kept, history would mean reading every report of every AP.
- **A device that keeps its address is the same device tomorrow;** one whose address rotates is not, and is reported only as a count.
- **Clients' attempts to come online are kept 90 days, unless the manager is told otherwise** (0118).

## Decision

- **The manager keeps what each report says was heard,** as it takes the report, in three tables of its conditions store:
  - **each device:** what it last said it is (its name, kind, what, maker, tracker), when any AP first and last heard it, and the AP that heard it last;
  - **when each AP heard each device, five minutes at a time,** with the strongest signal in them: a report repeats the ten minutes before it, and that comes to the same five minutes, kept once;
  - **how many rotating addresses of each kind each AP heard, five minutes at a time,** the most at once. A kind is the company their data is from and what they are, as the manager names them (`label`).
- **Kept as long as clients' attempts,** and let go with them: a device last heard before then goes too.
- **`GET /v1/bluetooth`** is what the APs under a folder or AP heard over hours: each device, and what each AP heard of it (in how many five minutes, its strongest signal, first and last); and the rotating addresses, each kind a series, the most any one AP heard each five minutes, as one phone near two APs is heard by both.
- **`GET /v1/bluetooth/devices/{mac}`** is one device: what it is, and each five minutes an AP heard it, with the signal.
- **Who may view an AP sees what it heard,** and no more: a device heard only by APs one may not view is not there for them.
- **A device has its own page,** reached from its name or address wherever it is listed: what it is; when it was first and last heard, and by which AP; where and when, a lane for each AP and a mark for each five minutes, the darker the stronger, over a day, a week, 30 or 90 days; and its stays, each AP's sightings joined where they are no more than ten minutes apart.
- **The Bluetooth tabs say more:** the devices heard earlier in the day that are not heard now, when and by which AP they were last heard, and for how long; and each kind of rotating address, how many there were through the day, a small chart in its row.

## Consequences

- **Where a sensor or a tracker has been can be read,** and when it came and went.
- **How many phones and computers were near an AP through a day can be read,** from the rotating addresses, as a measure of how busy the place was, not a count of people.
- **The store grows by a row for each device each AP hears, each five minutes it hears it:** twenty devices all day near two APs is about 11,500 rows a day, about a million in 90 days, a few tens of megabytes.
- **Tried, 2026-10-14:** the store and the two calls by tests; the pages in the harness, with a made-up week: a sensor heard all week but each night at three, a tracker each evening, a garage door sensor last heard three hours before.

## Not done

- **Alerts,** such as for a tracker that stays, or a sensor not heard for a day. The second is done since, in 0220, for a device a person named.
- **A device's history of signals as a chart,** besides the marks' shades.
- **Naming a device,** as a client can be named (0153). Done since, in 0220.
