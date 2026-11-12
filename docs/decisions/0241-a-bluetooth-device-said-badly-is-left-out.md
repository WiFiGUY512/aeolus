# 0241. A Bluetooth device said badly is left out

- Status: Accepted
- Date: 2026-10-29
- Proposed by: Claude, from the Living Room's refused reports on the morning of 2026-10-29. Written up by Claude
- Refines: 0157 (what an AP's Bluetooth radio hears)

## Context

- **The manager refused an AP's whole state report** when one Bluetooth device in `ble_devices` broke a rule: HTTP 400, and nothing of the report kept.
- **On 2026-10-29 the Living Room's reports were refused from at least 08:49Z to 10:26Z.**
  - The cause: one device, the Samsung fridge (38:68:a4:3c:59:f1), carried `rssi_max` 99 dBm.
  - The manager's view of that AP went stale: no BSSIDs, radios or clients. Client tests on it were refused for want of BSSIDs.
  - Restarting aeolus-ble cleared it.
- **The 99 came from aeolus-ble's own table.** `btad.note` left out only 127 ("no signal"). Any other byte read where the RSSI is, as from an advertisement read wrong, became the device's signal, and its strongest for as long as it was heard. airscan's survey heard the same fridge at -69.
- **A Bluetooth signal runs from -127 to 20 dBm.** The manager already takes no other.

## Decision

- **The agent takes a signal only from -127 to 20 dBm** (`btad.dbm`): when it notes an advertisement, and when it reports a device or a group, so that one kept from before is reported without it.
- **The manager leaves out each device and group said badly, and keeps the report.**
  - A `ble_devices` that names no source of its own is left out whole, and the report is kept without it.
  - Past 128 devices or 32 groups, the rest are left out.
- **The rules a device or group must keep are as before.** Only what comes of breaking one changes.

## Consequences

- **One advertisement read wrong no longer stops all an AP says.**
- **A device said badly is missing from what the AP heard until it is said well again,** usually on the next report.
- **Tested:**
  - **The agent:** a fridge noted at -68 then 99 keeps -68 as its signal and its strongest; one kept at 99 is reported without `rssi_max`; `dbm` takes -127 to 20 and nothing else.
  - **The manager:** each kind of badly said device, beside a good one, leaves the good one kept and the report taken (200); a group of none is left out and another kept; with no source, the report is kept without `ble_devices`.
