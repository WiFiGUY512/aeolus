# 0273. A Bluetooth address that changes

- Status: Accepted (Aeolus's own development, 2026-11-01; port B68, which Griff said yes to)
- Date: 2026-11-01
- Refines: 0157 (the Bluetooth proxy)

## Context

- **aeolus-ble tells Home Assistant the controller's address once,** read as it starts. HA takes it as the Bluetooth source's address (`bluetooth_mac_address`), and asks for the proxy's info only as it connects.
- **On a C-360 the controller had no address of its own** (hcitool: 00:00:00:00:00:00). aeolus-ble said the AP's Ethernet MAC instead: 30:86:2D:EF:2F:FF on C360-AP.
- **Port B68 gives it its public one** (30:86:2D:EF:2F:F8 on C360-AP), from the AP's ART.
  - From -94, `arista-c360-ble` writes it at boot: START=99, in the background, waiting up to 90 s for hci0. It takes about 10 s and power-cycles hci0 three times.
  - aeolus-ble can start before that, and kept the address it read until it started again. On C360-AP, B68 was run by hand at 10:09 CST, and aeolus-ble still said :FF an hour on.

## Decision

- **aeolus-ble reads the controller's address again each minute.**
- **Where it reads a real address that isn't the one it said,** it takes it, says so, and lets the HA clients go. They connect again and read the new address.
- **An address read as none changes nothing,** as on a C-360 before B68, or while hci0 is down mid-cycle. So nothing flaps back to the AP's own MAC.

## Consequences

- **A C-360 on -94 gives HA its public address within a minute of the boot's write,** whichever starts first.
- **On C360-AP, once this runs, HA gets a new Bluetooth source, 30:86:2D:EF:2F:F8.** The old one, :FF, lingers until HA drops it. That is the cost of B68 Griff took.
- **57ff and ce7f, on -93, say none and change nothing until -94.**
- **Checked:** aeolus-ble compiles on Shed's 25.12 ucode, and its replay test is as before.
