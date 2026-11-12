# 0220. A Bluetooth device you name

- Status: Accepted
- Date: 2026-10-24
- Proposed by: Claude, from 0158's "Not done" ("naming a device, as a client can be named"; "alerts, such as for a sensor not heard for a day"), on Griff's standing word to go on developing Aeolus. Written up by Claude
- Refines: 0153, 0157, 0158

## Context

The APs' Bluetooth radios hear the house's sensors: SwitchBot meters, thermometers, the things Home Assistant reads through the APs (0156). A device is listed by what it says of itself, often a short code or nothing, and its MAC.

A sensor whose battery runs down simply stops being heard, and nothing says so. Home Assistant shows a stale value; Aeolus shows a device that was heard yesterday.

## Decision

- **A person names a Bluetooth device,** as a Wi-Fi client is named (0153):
  - `PUT /v1/bluetooth/devices/{mac}/name {name}`, at most 64 characters on one line, its spaces tidied; empty takes the name away.
  - It is for a device an AP has heard, and needs the clients right on the AP that heard it last.
  - The name is kept in the conditions store (`ble_names`, migration 32→33). It is nothing told to an AP, and not in the change log.
- **The name shows** in the Bluetooth history (`named` on `GET /v1/bluetooth` and on a device's page), before what the device calls itself. The device's page has Name it… / Rename….
- **A named device not heard by any AP for a day is an alert,** `ble-silent`, info, on the AP that heard it last:
  - "Bluetooth device attic sensor (c1:…), which this AP heard last, has not been heard by any AP for a day or more: a sensor whose battery ran down, or one taken away".
  - It begins when the device was last heard. It ends when an AP hears the device again, when its name is taken away, or once it has been gone for 30 days.
  - **Naming a device is what asks for this:** a device nobody named is no one's to miss.

## Consequences

- **A sensor that matters gets a name,** and its dead battery shows in the alerts within a day.
- **Info alerts are not sent** unless a folder's `notify.severity` is `info` (0101). They are in the alerts list and the advisor reads them.
- **A device that rotates its address** cannot be named: it is a count, not a device (0158).
