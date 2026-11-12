# 0157. What an AP's Bluetooth radio hears

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff: "Can you also do a ble clients tab?"; to a tab of what each AP hears nearby, the fixed devices listed, the rotating ones folded, the trackers called out, and a folder's view of which AP hears each device best: "Perfect. Lets do all that". Written up by Claude
- Refines: 0081, 0156

## Context

- **Nothing connects to an AP over Bluetooth:** what it has are devices it hears advertise. A "client" here is a device near the AP.
- **The C-360's own survey hears them already** (airscan, 0081): ten seconds every two minutes or so, asking each device for the rest of what it would say, and it reads what they say: names, makers, services, beacons, and the trackers it knows.
- **Lent to Home Assistant** (0156), the AP's radio hears every advertisement, all the while.
- **Most of what is heard changes its address every few minutes.** Phones, computers and watches keep theirs private; one heard on C-360 unit 1, 2026-10-13: of twelve devices, six were Windows computers' rotating addresses, probably one or two computers; two were Apple Find My; three were fixed devices with names. Listed by address, a phone is a new device every quarter of an hour.
- **The manager keeps every report for days.** What is added to it is added many times over.

## Decision

- **An AP reports what its Bluetooth radio heard in the last ten minutes** (`ble_devices`), from the daemon that lends the radio to Home Assistant while it runs, from the survey, or from both, taken together: one device for each address, with what each heard that the other did not, such as a name a device gives only when asked.
- **A device that keeps its address, and every tracker, is reported one by one,** the strongest first, sixty-four at the most: its address and what kind; its signal now, on average and at its strongest; how often, and since when, it was heard; and what it says of itself.
- **The addresses that rotate are counted,** by the company their data is from, its first byte and their first service: how many, the strongest of them, and when the last was heard. Twenty-four groups at the most.
  - A report gains a kilobyte or two: one and a half, on C-360 unit 1.
- **The manager names each device before the report is stored:** the maker of a public address by its OUI (0067); the company its data is from, whom its services are assigned to, and the category of its appearance, by the Bluetooth SIG's assigned numbers, taken when the manager is built (`internal/bluetooth`, from the SIG's public repository); and what it is, where what it sends says: a tracker, a beacon, a sensor, a phone or computer, and so on.
- **An AP whose report has them has a Bluetooth tab:** a line of counts; its trackers; its devices, each a row; and the rotating addresses, a row a group.
- **A folder has a Bluetooth tab** with every device its APs hear, once, the AP that hears it best first, with its signal there, then the others that hear it: about where it is.
- **The daemon keeps the table of what it hears** (`/var/run/aeolus/ble-devices.json`, every ten seconds), as the survey does; how an advertisement is read is the survey's (`aeolus/btad.uc`).

## Consequences

- **Where a sensor, a tracker or a beacon is can be read from a folder's tab:** the AP that hears it best.
- **An AirTag that stays near an AP is seen,** with when it was first heard.
- **A device's name, maker and kind are as good as what it says:** many say nothing but rotating addresses and data only their maker reads.
- **Tried, 2026-10-13:** on C-360 unit 1, the agent's part, run by hand against the radio lent to Home Assistant and the survey: six devices one by one, two of them Apple Find My, and four rotating Windows addresses counted as one group, in a kilobyte and a half. The tabs, in the harness, with made-up devices on two APs, one heard by both.

## Not done

- **A history of a device:** where it was heard, and when, over days.
- **Alerts,** such as for a tracker that stays.
- **The SIG's lists are a build's:** a device of a company assigned later is named by its number until the manager is built again.
