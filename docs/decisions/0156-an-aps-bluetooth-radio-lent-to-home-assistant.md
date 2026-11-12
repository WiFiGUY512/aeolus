# 0156. An AP's Bluetooth radio, lent to Home Assistant

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff: "how can we attach Home Assistant (hestia) to the BLE radio in these APs"; and, to an ESPHome-compatible Bluetooth proxy on the AP, "perfect!! please start the process". Written up by Claude
- Refines: 0081, 0119, 0126

## Context

- **Some APs have a Bluetooth LE radio.** The Arista C-360 has a Nordic nRF52833 on a serial port, with Zephyr's controller in it: LE only, with no address of its own. Linux has it as `hci0`.
- **Home Assistant hears Bluetooth through the radios it is given,** and takes one that is far away by ESPHome's native API: a TCP connection, port 6053, on which a device says what it is and hands on each advertisement its radio hears, as it heard it and how strongly. An ESP32 set up so is a Bluetooth proxy. Home Assistant then knows its sensors, its beacons and where a device is from that place too.
- **The APs are where the house is covered already,** powered and on the network. Griff's Home Assistant runs as a virtual machine, with no Bluetooth radio of its own, and hears only through such proxies.
- **The AP's own survey uses the same radio** (airscan, 0081): about ten seconds every two minutes, through the kernel, each device told once.
- **What the AP has to read its controller with:** BlueZ's `hcitool` and `hcidump`. ucode's socket module opens a Bluetooth socket and cannot bind one to a controller.

## Decision

- **An AP can lend its Bluetooth radio to Home Assistant:** `system.ble_proxy`, in Locations under System, off unless set on. It is set on a folder or on one AP, as any system setting.
- **Lent, the AP is a Bluetooth proxy as ESPHome's is,** on port 6053: a new daemon of the agent's, `aeolus-ble`. In Home Assistant the AP is added under ESPHome, by its address.
  - It says it is `<hostname>-ble`, by the MAC of the AP's first Ethernet port, and that it offers nothing but a scan handed on raw, which it says the state of and makes the way it is asked.
  - It has no switches and no sensors, and makes no connections: asked how many it has free, it has none of none.
- **Left to itself it only listens.** The controller scans passively: it asks nothing of what it hears, and sends nothing on the air. Every advertisement is handed on, not each device once, a tenth of a second's worth at a time.
- **Home Assistant can ask it to scan actively,** as it does of any proxy: for a window of some seconds when one of its devices needs it (its *auto*), or for good (*active*). Then the controller asks each device it hears for its scan response, where some keep their name or their readings, and those are handed on too.
  - Home Assistant chooses, for each proxy: the device's *Bluetooth scanning mode*, under ESPHome. A proxy at *passive* it warns of: a device that needs asking may show nothing where this proxy hears it best.
  - The proxy says it is set up to scan actively (ESPHome's *configured mode*), as an ESPHome proxy set `active: true` does: that is, it may be asked to. Home Assistant takes a proxy that says passive, or says nothing, for one that must only listen (see *Since* below).
  - It scans actively while any Home Assistant connected asks for that, and goes back to listening when the last one that did asks for that or goes.
  - The change takes well under a second: the scan is stopped and started the other way.
  - Asking, the controller gives its random address. The kernel has set that where the AP's own survey has run; where the controller has none and refuses, the daemon gives it one made of the AP's MAC, once, and where it still refuses, only listens for five minutes.
- **The radio is shared with the AP's own survey.** The daemon sees the kernel take the controller for it, lets it finish, hands on what is heard meanwhile, and takes the radio back a second after it ends, scanning the way it was. Scanning that stops for any other reason, or a controller that has said nothing for half a minute, is started again.
- **The port is not encrypted and asks no password.** Whoever can reach it hears what the radio hears, which is what anyone near the AP hears. ESPHome's encryption needs ciphers ucode has not got.
- **Up to four Home Assistants at once.** One not heard from for a minute is asked whether it is there, as an ESPHome device asks, and let go where it has not answered in half a minute.
  - Home Assistant says nothing of itself to a device that keeps sending to it, so silence alone says nothing.
- **The daemon's connections do not pass to the programs it starts.** One held by `hcitool` stayed open after the daemon had closed it, and Home Assistant did not know it had been let go.
- **The AP says how it does in its report** (`ble`): whether it has such a radio; whether the daemon runs; whether the radio scans, and whether actively; who is connected; how many advertisements it heard and handed on; how many devices it heard in the last five minutes. The AP's Health tab has it as a tile, Bluetooth.
- **The System tab has the setting in a panel of its own,** on a folder and on an AP, there whether or not it is set: its slider, set right there with the same review as any change; and on an AP that has it on, how the lending does and the address Home Assistant is given. Griff could not find the slider where it was only in the editor: "where is that slider though".
- **It is rendered whether or not the AP has such a radio:** an AP without one says so in its report, and nothing else comes of it.
- **Where the tools are not installed, the daemon is not started,** and the report says that they are missing. Nothing installs them: an AP with such a radio has them in its image.

## Consequences

- **Home Assistant has one more place it hears from for each AP lent,** with nothing to buy or power.
- **During the AP's own survey, about ten seconds in every two minutes, Home Assistant is told each device once** and not each advertisement.
- **Anything on the AP's management network can read the port.**
- **Tried, 2026-10-13, on a C-360, the daemon run by hand:** the radio heard about 22 advertisements a second from 7 devices; the client library Home Assistant itself uses for ESPHome (aioesphomeapi 46.10.0) connected, read what the proxy is and can do, and was handed 256 advertisements in 12 seconds, as a second client beside it was. The AP's survey took the radio and gave it back as described, and found its ten devices. Stopped, the daemon left no process and the controller not scanning.
- **Live, 2026-10-13:** the setting put on for that C-360, the agent's daemon listened within 45 seconds; Griff added it to his Home Assistant under ESPHome, which connected, registered it as a Bluetooth scanner, and takes about 21 advertisements a second from it. Home Assistant had it at *passive* and logged the warning above, which is why it now scans actively when asked.
- **Active scanning tried the same evening, on that C-360,** asked for by the same client library beside Home Assistant: the scan went active the moment it was asked and back when asked, and in eight seconds of it four of seven devices said more than one thing, against one of them in the eight seconds before and after, and three names were heard against two.
- **Two faults found live and mended the same evening:** the first build took Home Assistant's silence for its absence and let it go after a minute and a half, and the connection, held open by `hcitool`, did not close, so that Home Assistant came back only when its own pings went unanswered, a minute and a half later. Mended, a client that answers when asked stayed, and one that does not was closed, on an AP with a made-up controller; on the C-360, Home Assistant came back within two seconds of each restart.
- **Home Assistant's own asking, seen 2026-10-13:** its option set to *auto* by Griff at 21:04, it asked the C-360 for an active scan at 21:11:12 and for listening again at 21:11:27, its fifteen-second sweep; the proxy did both and kept the connection. Before that it had held one connection for 37 minutes, until an update of the agent restarted the daemon.
- **Not yet tried:** over days.

## Not done

- **Connections to devices** (ESPHome's active proxy): locks, toothbrushes and the like that must be connected to. The controller says it can; whether its firmware holds a connection is not known.
- **Encryption of the port,** and a password.
- **Home Assistant finding the AP by itself** (mDNS, `_esphomelib._tcp`).
- **The controller's own address:** the kernel here does not say it, so the proxy gives the Ethernet port's MAC for it too.
- **Other radios than `hci0`,** and installing the tools where they are missing.
- **An alert where a radio that is lent stops scanning.**

## Since: it says it may be asked to scan actively (2026-11-07)

- **Home Assistant 2026.11 set two C-360s to passive for good.** Its ESPHome integration now keeps each proxy's scanning mode as a setting. Where a proxy has none yet, it takes one from the configured mode in the proxy's scanner state, and sets *passive* where that says passive. The proxy did not say it, and a field left out reads as passive. So Living-Room-C-360 and Garage-C-360, which had no setting, were set to *passive*; C360-AP, set to *auto* by Griff on 2026-10-13, kept that. Home Assistant never asked the two to scan actively, and warned that they would not ("is in passive-only mode but active scans have been requested") as its integrations asked for active scans of their devices.
- **It was not the controller, nor the AP's own survey.** Neither proxy had been asked for an active scan at all. C360-AP, at *auto*, was asked for about 3,900 windows of ten seconds in its week, and its controller refused two, each followed by five minutes of listening.
- **Now the proxy says it is set up to scan actively,** in every scanner state, so Home Assistant gives a proxy it has no setting for its default, *auto*. A setting it already has stays: the two C-360s were set to *auto* by hand, on Griff's word, "allow them to active scan, they are plugs so not an issue".
