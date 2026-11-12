# 0281. The Bluetooth reader after hcidump

- Status: Accepted (Griff, 2026-11-01: hold the C-360s' -104 flash "until BLE works")
- Date: 2026-11-01
- Refines: 0156 (the Bluetooth proxy), 0157

## Context

- **aeolus-ble read the controller with BlueZ's hcidump** (`hcidump -i hci0 -R`): each packet from the controller as hex lines, and each command to it, so it saw LE Set Scan Enable.
- **BlueZ 5.87 has no hcidump.** Upstream took it out: there's no tools/hcidump.c, and it isn't built even with --enable-deprecated. Its place is btmon. Port -104 brought 5.87, so the C260 on -104 started no aeolus-ble: the init script wanted hcidump. The C-360s would have lost their Home Assistant Bluetooth proxies the same way. Griff held their flash.
- **btmon wasn't in -104 either.** The port's -105 is -104 with bluez-utils-btmon in every image.
- **`btmon -i hci0 -w <file>` writes btsnoop:** the Linux monitor's datalink, 2001. Each record is a 24-byte header, then the packet (a command is kind 2, an event kind 3), without the H4 byte hcidump showed. With `-w /dev/stdout` it writes its decoded text to stdout too, mixed in. With `-w /dev/fd/3 3>&1 1>/dev/null` the stream is btsnoop alone.

## Decision

- **The tool the controller is read with is a table, DUMPERS,** each entry with its program, its command, how it is played from a file with -r, and its reader:
  - hcidump's raw lines, as before;
  - btmon's btsnoop, read a record at a time, header and then data, each read for just its length, so no read waits on more than the controller has said.
  - The daemon takes the first the AP has, each time it opens the controller.
- **Both readers hand the daemon the same messages:** each event as hcidump wrote it (btsnoop's with its H4 byte, 04, put back), and each LE Set Scan Enable it is told. Everything after the reader is unchanged.
- **The btsnoop pieces are in the library** (aeolus.esphome: `btsnoop_stream`, `btsnoop_record`, `hci_text`), and the BLE test reads them.
- **The init script starts aeolus-ble with hcitool and either hcidump or btmon.**
- **A file played with -r is btsnoop where it starts as btsnoop does,** its head once and its records over and over; otherwise hcidump's lines.
- **A dumper left running by a daemon before is found by either name.**

## Consequences

- **An AP with BlueZ 5.87 and btmon (port -105 on) has its Bluetooth proxy again.** One with 5.86 (-95, stock 25.12) reads with hcidump as before.
- **Checked:**
  - On the C260, on -104 with bluez-utils-btmon added by hand, the new aeolus-ble listened on 6053 and read through btmon (`btmon -i hci0 -w /dev/fd/3`): 2870 advertisements from 10 devices in under a minute, while hcitool scanned.
  - It stopped cleanly after its two minutes, btmon and the scan with it.
  - The BLE test's new cases: a stream's head, a record's header, an event to hcidump's text, read back to the same advertisement as hcidump's.
  - It compiles on Shed's 25.12 ucode.
