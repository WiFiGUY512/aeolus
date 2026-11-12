# 0201. Off the air when lost: an SSID that cannot reach its way on

- Status: Accepted (Griff, 2026-10-21: "we need an option to down the ssid if that ssid cant get back to its concentrator, or its vlan, that prevents SSID based black holes as well, but make it an option, not a rule")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0054, 0059, 0060, 0061

## Context

- **A black hole by SSID.** When an AP loses a network's VLAN, its trunk no longer carrying it, or its tunnel's concentrator, the SSID stays on the air:
  - Clients join it, get no address or no way out, and stay. The AP is the strongest they hear.
  - A network with a fallback switches (0061), but one without has nothing to switch to, and one whose fallback is down too sits on the dead one.
  - 0054 said so of a tunnel that cannot reach its concentrator: "the network is down at that AP until switching comes".
- **The prober already judges each way on:**
  - each VXLAN tunnel, by the concentrator's answer and the overlay's (0059);
  - and the VLANs of networks with a fallback, by a lease of its own and the gateway's answer (0060, 0061).
- **One SSID can be taken off the air without the radio.** hostapd's control socket for each network takes `STOP_AP`, which stops that network's beacons alone. `UPDATE_BEACON` starts them again, since nl80211 starts an AP whose beacons were stopped.
  - Tried on OpenWrt3c8f on 2026-10-21, on `aeolus_50`: it went off the air and came back on its channel.
  - The radio's three other networks stayed up throughout.
  - Nothing restarts, and the config is not touched.

## Decision

- **An option, not a rule:** `network.<id>.transport.down_when_lost`, on or off, set like any network setting. Off, the default, changes nothing.
- **What judges it**, on each AP, for each network with the option:
  - **With a fallback:** the transport it is on now, by its switch (0061).
  - **On a tunnel:** the tunnel's probe, by VNI.
  - **On a VLAN:** that VLAN's probe. A VLAN network with the option is probed as a fallback's VLAN is: a lease of its own on the VLAN, and the gateway's answer (0060).
- **How it decides**, every 2 seconds, with the switching:
  - **Down or unverified takes it off the air.**
    - Down is a way that answered and then stopped, for three of the probe's intervals, 90 seconds by default. A tunnel device that has been off that long is down too.
    - Unverified is a way that never answered in its first three intervals while the link under it is up: a VLAN the switch never carried, or a tunnel whose far end answers nothing past it.
    - As first built, unverified left the network on the air. The live test on 2026-10-21 found that a VLAN carried by no switch is exactly that verdict, so it is taken off too.
    - **A network with no DHCP server** gets no lease from its probe and answers nothing. It needs an address to ask set as its transport's `probe`, or this option takes it off the air.
  - **Up puts it back at once.**
  - **Unknown leaves it as it is.** That is the verdict for the first three intervals, so an AP never takes a network off before its probe has had time.
- **Off the air:**
  1. Each of the network's Wi-Fi interfaces on the AP is found by its section, from netifd.
  2. Its clients are deauthenticated, so each looks elsewhere at once.
  3. hostapd stops its beacons.
  4. An interface that a reload or restart put back on the air while the network is lost is taken off again.
- **Back on:** the interfaces it took off are started again, whether netifd lists them at that moment or not.
  - While a reload sets the network up again, netifd can leave an interface out for a moment. One forgotten then would stay off the air.
  - One that fails is tried again every 2 seconds. Only an interface that no longer exists is let go.
- **It never leaves an SSID off the air without a running prober judging it.** What it took off is kept in `/var/run/aeolus/offair.json`.
  - A prober that starts puts those back: through the network, where it still has the option, so a failed try is tried again; otherwise at once.
  - Then it judges afresh.
- **The AP reports it** in `off_air`: per network, whether it is off, since when, why, and the interfaces taken off. A network going off the air or back on is reported at once, as a tunnel going down is (0059), not with the next report up to five minutes later.
- **The manager warns**, as `off-air`: "network home-wi-fi-iot is off the air on this AP: its VLAN 10 does not answer, so its clients join another AP or network until it answers".
  - A client test through the air (0123) is not asked of a network on an AP that has it off the air. One that ran and could not find it there is no failing test: the off-air alert says why. Tests over the wire go on, as the way on is what they test.
- **The Networks tab** has the switch, "Off the air when lost", under Traffic. A network with it set says so.

## Consequences

- **Clients of an AP that lost the way on go to an AP that has it,** or to another network, instead of sitting on one that goes nowhere.
- **Where every AP has lost it, the SSID is gone from the air everywhere.** That is what Griff asked for: clearly gone, not quietly broken.
- **Each AP that runs a VLAN network with the option holds a lease on that VLAN,** as for a fallback (0060).
- **About 90 seconds pass between the loss and the SSID going off the air:** three of the probe's intervals. It comes back within 2 seconds of the probe's first answer.

## As built (2026-10-21)

- **Schema:** `transport.down_when_lost`.
- **Agent:**
  - `downs()` in `render.uc` writes a `down` section per network, `network`, `via` and its `iface` sections, and probes the VLAN.
  - In `aeolus-prober`: `keep_off()`, `take_off()`, `put_back()`, `beaconing()` (nl80211) and `ifnames_of()`; `off_air` in its results.
  - `probe.off_air()` in `probe.uc` makes the judgement.
  - `report.off_air` in `aeolus-agent`, and in its `probe_news()`, so a change is reported at once.
- **Manager:**
  - `offAirState` in `internal/api/report.go`, checked;
  - the `off-air` alert in `internal/alerts/alerts.go`;
  - and the render check's `downs()`, which checks the plan and the VLAN probe.
- **UI:** the switch in `views/networks.js`, and its label in `format.js`.
- **Tested:**
  - **Render case `down-when-lost`,** rendered with real ucode and passed by the check.
  - **`probe.off_air` in `agent/test/probe.uc`.**
  - **`TestOffAir`** (alerts) and `TestStateReports`, which take the report and refuse a bad one.
  - **`STOP_AP` and `UPDATE_BEACON` by hand on OpenWrt3c8f,** before this was built.
  - **Live on OpenWrt3c8f, 2026-10-21,** with a throwaway network on VLAN 99, which no switch carries:
    - It went off the air 90 s after the prober started, and the manager warned `off-air`.
    - That found two gaps, both fixed:
      - **Unverified was left on the air.** A VLAN that never answered while its uplink is up is unverified, not down; it now counts as lost.
      - **The warning waited for the next report.** A network going off or on is now probe news, reported at once.
    - Moved to VLAN 20, the network was judged back at once. But it stayed off the air: the network reload hid its interface from netifd for a moment, and the prober forgot it had taken it off. That is the fix above.
    - Moved back to VLAN 99, the AP's config was refused: its VLAN 20 probe was still there, though nothing probed VLAN 20 any more.
      - A VLAN's probe, `aeolus.aeolus_vlan<N>`, has the name of the VLAN's bridge-vlan in the network config. The render keeps that name while any network is on the VLAN, so the sweep kept the probe too.
      - This was so since 0060, for a fallback's VLAN too.
      - Now `probes()` removes every VLAN probe it did not make. Render case `probe-moved`.
