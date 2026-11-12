# 0257. A network may keep a client that stops acknowledging

- Status: Accepted (Griff, 2026-10-31, given in the porting session: "do both, and put -83 on everything", both being B122's stopgap here and its cause in the port)
- Date: 2026-10-31
- Proposed by: the porting session, for B122 [suspect]
- Refines: 0255 (why clients leave)

## Context

- **hostapd sends a client off the moment the radio says it stopped acknowledging** (`disassoc_low_ack`, on by default in OpenWrt): reason 34, "too many frames went unacknowledged".
- **On the C-360s that report can be wrong** (B122 [suspect], the porting session's). On 2026-10-31 a kettle on Home Wi-Fi IoT was sent off with reason 34 twice:
  - by ce7f at 16:30:43, heard at −62 dBm with 0 retries;
  - by 57ff at 16:37:32, at −59 dBm with 0 retries, 4½ minutes after it joined.
  - ath11k's firmware peer kickout is the likely source: it reports a client as gone, mac80211 takes that for low ACK, and hostapd sends it off.
- **The same reason is right for a client truly gone:** an ESP at −91 dBm with 190 of 423 frames lost was rightly sent off by Shed. So turning it off everywhere would keep dead links.
- **hostapd has a second check:** a client that sends nothing for `ap_max_inactivity` (300 s) is polled, and removed if it doesn't answer. That still removes a client truly gone, only later.
- **The porting session is finding the cause in ath11k** (B122): a debug driver logging the firmware's whole kickout event.

## Decision

- **A network may say whether a client the radio says stopped acknowledging is sent off at once:** `network.<id>.disassoc_low_ack`, a boolean.
  - Unset, it is left out, and OpenWrt's default holds: on.
  - Off, the AP's own inactivity check removes a client truly gone, after 300 s.
- **It is set off on Home Wi-Fi IoT alone,** at Griff's word, as B122's stopgap, until the port's fix lands. Then it is unset again (faults list). B122 turned out not to be a fault; see Afterwards.
- **The manager checks it as it checks the other network options:** set, the AP's wifi-iface has it; unset, none.

## Consequences

- **Home Wi-Fi IoT's sleeping devices are no longer sent off for a firmware's wrong word.** A device truly gone stays listed up to 300 s longer.
- **Setting it re-renders every AP that carries the network:** each reloads that network, and its clients join again within seconds.
- **Every other network is as it was.**

## Afterwards

- **B122 is not a fault** (closed 2026-10-31, faults list entry 62). Every client sent off for missing ACKs had really gone. The kettle runs its own loop of about 5 minutes on and 100 s silent. Drive-cooler was back on C360-AP the same second. The bedside lamp was switched off. The firmware's kickout event can't tell a wrongly kicked client from a gone one, but Aeolus's minutes and joins can.
- **So the setting only keeps a gone client listed** until hostapd's 300 s inactivity check, where on it ends a dead association sooner. Off, it also spends 2.4 GHz airtime on retries to the client that's gone.
- **Undone the same day, at Griff's word** via the porting session ("Do all of these things if you think they are valuable"). Change 597 (2026-10-31 22:13 CDT) unset it on Home Wi-Fi IoT, so OpenWrt's default (on) holds on every network again.
- **The setting stays:** a network whose clients really do sleep through the radio's retries may still want it.
