# 0301. A radio being set up again

- Status: Proposed
- Date: 2026-11-04
- Proposed by: Griff, from porting's B43, closed as by design on 2026-11-03: "decide per caller what a pending radio should mean". Written up by Claude
- Refines: 0039 (the state report), 0057 (the apply check), 0065 (the DHCP watch), 0073 (radio resource management), 0089 (guard intervals), 0118 (client connections), 0138 (steering), 0148 (captures), 0201 (off the air when lost), 0207 (telemetry by the minute), 0209 (the AP's address on its own Wi-Fi), 0224 (802.11r key holders), 0253 (facts wait for a radio's networks)

## Context

- **netifd forgets a running radio's interface names while it sets the radio up again** (porting's B43, `build/bugs/b43-b45-triage.md`).
  - A reconfigure that touches a radio runs wifi-scripts' `wdev_config_init()` (`wireless-device.uc`), which empties the handler data. That's where netifd keeps each interface's `ifname`.
  - Until the radio's handler names them again, `ubus call network.wireless status` lists each of the radio's interfaces without `ifname`.
  - Radios are set up one at a time, so on a three- or four-radio AP the last one goes seconds without names.
- **The interfaces never stop meanwhile.** hostapd reloads only what changed.
- **netifd says so.** The radio shows `pending: true` from the same step that forgets, so no look sees a name missing without `pending`. `netifd.wireless.done` marks the end.
  - It also shows `up: false` throughout (`up` is `state == "up"`), on stock 25.12 (Shed) and on the port alike.
  - `pending` is true while netifd takes a radio down, too.
- **It's by design, in stock 25.12 too.** Porting drafted a wifi-scripts change that keeps the names (`b43-wireless-keep-ifnames.diff`), but it isn't in the port, and stock APs (Shed) won't have it. Aeolus must cope by itself.
- **Every reader of that status took a missing name, or a radio not up, for gone.** On every apply that touched the Wi-Fi:
  - **steering** dropped the radio's BSSes from its view and its hostapd listeners, and took them up again a look later. Keep on 6 GHz (0300) and the other ways let clients go meanwhile;
  - **the journey daemon** closed the radio's sockets and forgot what its frames said, just as clients rejoin;
  - **radio resource management** forgot the radio, and its next look counted as a start;
  - **the minute's telemetry and the DHCP watch** lost the radio's clients for a look;
  - **the state report** could show the radio with no channel, no clients and its networks off the air, and leave its 802.11r key holders out;
  - **aeolus-gi**, run right after each apply, forgot the guard interval it held on a pending radio's interfaces, and set it again next time;
  - **a capture** started then went without the radio's interfaces;
  - **the apply check's "before"**, taken during another reconfigure, took a pending radio's networks for already down, so they went unchecked after the apply.

## Decision

- **One shared helper, `aeolus.wifinames`** (`agent/files/usr/share/ucode/aeolus/wifinames.uc`), not a copy in each reader. Every reader goes through it.
- **A daemon, which looks again and again, sees the status through a keeper of its own** (`wifinames.keeper()`, `view(status)`):
  - A radio that isn't pending is as netifd has it. The keeper remembers its `up`, and what it needs of each interface by section.
  - **A pending radio gets back what its last settled look had**, interface by interface, where netifd leaves it out and it is still so, and the `up` it had then. It is marked `held`. Its config and `pending` stay netifd's.
  - A pending radio netifd already names is held too, so its `up` comes back: netifd says `up: false` while pending, names or not.
  - **What is carried is a table, `CARRIED`**, by field, with what says the old value still holds. Today it is `ifname`, while the kernel still has an interface by that name (`/sys/class/net`). A field netifd comes to leave out goes there.
  - **Not held:** a radio never seen settled (as at boot: there is nothing to carry); one none of whose interfaces the kernel still has (taken down: it's going); one pending for more than `HOLD` (120 s: no setup of a running radio takes that long); a section that is new (it has no old name).
  - A status that is none (netifd's object gone) passes through, and what was kept stays.
- **A look made once waits instead** (`wifinames.settle(ask)`): it asks again every second while a radio is pending, for `SETTLE` (20 s) at most, then takes the last answer. A radio still pending then lists its interfaces without names, and the caller leaves what it holds of them as it is: neither there nor gone.
- **Each reader:**

| Reader | Runs | A pending radio |
|---|---|---|
| aeolus-agent `wireless_status()`: the state report (0039), 802.11r key holders (0224), facts' `coming_up` (0253), `on_air_state` (0209) | daemon | kept, through the agent's keeper. |
| aeolus-agent `wifi_down`, the apply check's "before" (0057) | daemon | kept: what was down before is what really was. |
| aeolus-agent `wifi_back`, the apply check after (0057) | daemon | not back yet: all its networks are listed as down, as before, so the check waits for netifd to finish, `WIFI_WAIT` (150 s) at most. |
| aeolus-steer `networks()` (0138, 0300) | daemon | kept: its BSSes stay steered, and listened to. |
| aeolus-journey `look()` (0118) | daemon | kept: its sockets stay open. A BSS hostapd made anew is still caught by its ifindex. |
| aeolus-rrm `look_radios()` (0073) | daemon | kept: the radio isn't forgotten, and no start is counted. |
| aeolus-live `radios_now()` (0207) | daemon | kept: its minute goes on. |
| aeolus-prober `ifnames_of()` (0201), `dhcp_scan()` (0065) | daemon | kept. What 0201 took off the air is still put back by its own list (`offair.json`) and `/sys/class/net`. |
| aeolus-gi, the run after each apply and every few minutes (0089) | once | waits. A radio still pending then is let be: no held guard interval is dropped while one is. |
| aeolus-gi `<interface>`, from the net hotplug (0089) | once | already waits, up to 90 s, for netifd to name the interface. |
| aeolus-capture `have()` (0148) | once | waits, so the capture has every interface. |

## Consequences

- **A reconfigure no longer looks like radios going away.** Steering, the journey, RRM, telemetry and the DHCP watch carry on through it, as the interfaces do.
- **The apply check is no weaker.** After an apply it still waits for each radio to settle. A radio pending for all of `WIFI_WAIT` still fails the apply.
- **A one-shot run during a reconfigure waits up to 20 s.** aeolus-gi runs in the background, and a capture starts that much later.
- **A radio that settles with an interface missing shows it missing at once.** The keeper only bridges `pending`.
- **Where the wifi-scripts change lands** (porting's drafted `b43-wireless-keep-ifnames.diff`, upstream or in the port), netifd lists the names itself while pending. The keeper then carries no names (a field present is never replaced), but still carries `up`, which the change leaves false. Nothing needs retiring: this follows netifd's design, not a fault.
- **Tests:** `agent/test/wifinames.uc` (`TestAgentWifiNames`) feeds a Living Room–shaped status with pending radios that have no ifnames. It checks:
  - held radios, with names and `up` back;
  - an interface gone from the kernel;
  - a section added mid-setup;
  - settling again;
  - netifd keeping the names itself while pending;
  - a radio taken down;
  - a first look;
  - netifd's object gone;
  - `HOLD`'s edge;
  - malformed statuses and an impossible name;
  - `settle`: asking again until no radio is pending, giving up after its time, and passing null through.
