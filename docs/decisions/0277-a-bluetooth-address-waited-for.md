# 0277. A Bluetooth address waited for

- Status: Accepted (Aeolus's own development, 2026-11-01; porting's suggestion)
- Date: 2026-11-01
- Refines: 0157 (the Bluetooth proxy), 0273 (an address that changes)

## Context

- **At boot, aeolus-ble can start before the port's Bluetooth setup gives the controller its address.**
  - A C-360's `arista-c360-ble` (B68) writes it in the background once hci0 attaches, and takes about 10 s for its three power cycles.
  - A C-260's `arista-c260-bluetooth` loads the CSR chip's PS keys, the address among them.
- **On the -95 flashes (2026-11-01), aeolus-ble began each time on what the controller said first:**
  - C360-AP and ce7f: no address, so the AP's own MAC (:FF, :7F);
  - the C-260: the CSR default, 00:02:5B:00:A5:A5.
- **0273's look each minute took the real one 60 s on,** and Home Assistant connected again. But HA had seen another Bluetooth source for that minute, on every boot, and kept it besides the real one.
- **Porting can't close the gap at their end:** whatever its START number, the setup's work runs after hci0 attaches. They suggested aeolus-ble wait.

## Decision

- **A controller with no address of its own yet is given ADDR_WAIT (45 s) to have one** before aeolus-ble listens for Home Assistant. It says once that it waits.
- **"No address" is a table, NO_ADDRESS:** all zeros (a C-360 before B68) and the CSR default 00:02:5B:00:A5:A5 (a C-260 before its PS keys). Neither is taken as the controller's, at start or in 0273's look each minute.
- **Past the wait, it begins on the AP's own MAC,** as before, and 0273's look takes a real address that comes later.

## Consequences

- **A booting C-360 or C-260 gives HA its controller's real address from the first connection.** No other source is made.
- **A controller with a real address starts at once,** as on any restart after boot.
- **One that never gets one (57ff, on -93) starts 45 s later,** on the AP's MAC as before.
- **Checked:** with the new aeolus-ble bind-mounted and restarted:
  - ce7f listened at once on E4:D1:24:BE:3A:78;
  - 57ff waited from 13:35:16, then listened at 13:36:01 on 30:86:2D:BC:A5:FF.
  - It compiles on Shed's 25.12 ucode, and its replay test is unchanged.
- **The wait is timed by the monotonic clock (2026-11-01, v0.221.1).** Timed by the time of day, NTP's step at boot (about 4 minutes on) ended it at once: on C360-AP's -105 boot, a second before B68 wrote :F8. HA got :FF for 27 s, until 0273's look took :F8.
