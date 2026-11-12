# OpenWrt faults Aeolus copes with

Aeolus doesn't work around faults in OpenWrt. They go to the OpenWrt porting sessions, which fix them at the base and send the fixes upstream. Their ledger is `Arista APs/BUGS.md`, the B numbers below.

Where Aeolus already copes with one, the stopgap is listed here, so it can be taken out once the fix is in a build the APs run.

**Paths** are relative to the repo root:
- `sbin/` is `agent/files/usr/sbin/`;
- `uc/` is `agent/files/usr/share/ucode/aeolus/`;
- `libexec/` is `agent/files/usr/libexec/`;
- `init` is `agent/files/etc/init.d/aeolus`;
- `NNNN` is a decision.

**Category:** *works around* (Aeolus code exists because of the fault), *detects* (Aeolus only says so), or *unclear* (fault or design is not settled).

**Status:** every entry was reported to the porting sessions on 2026-10-24. When the porting sessions say which build carries a fix:
1. Retire the stopgap.
2. Change the entry's status to "fixed in <build>; stopgap retired in vX".

Gathered on 2026-10-24 by reading decisions 0040–0214 and the agent.

## netifd and the kernel

1. **netifd loses a VXLAN device but still reads it up** (B35).
   - The kernel has no `aeolus_50`, while netifd shows it up and in `br-vx50`. `ifup` does nothing.
   - Taking the bridge's interface down remakes it, but restarts the radios.
   - Seen 2026-10-20 on OfficeOpenWrt (25.12.2) and Shed (R7800). Clients got no DHCP for a day. (0187)
   - Stopgap: `keep_tunnels` remakes the device over rtnl (sbin/aeolus-prober); the `tunnel-gone` alert. *Works around, and detects.*
   - **Fixed in port 0072** (netifd 002 + 003; upstream openwrt/netifd#105), in -104 and on: netifd remakes a device it made that the kernel took with its lower device, and the bridge takes it back, in 1.25–1.33 s. The stopgap looks first (two looks 10 s apart), so it does nothing there. Stock 25.12 (Office, Shed) keeps it.
2. **A tunnel is deleted with the VLAN device it starts from.**
   - Taking a bond apart rebuilt br-lan.20, and the kernel deleted the VXLAN on it. netifd kept its record, so a reload or ifup didn't remake it. Seen on a C-360, 2026-10-10. (0096)
   - Stopgap: `heal_tunnels` restarts the network after an apply (sbin/aeolus-agent). *Works around*; probably the same root as 1.
   - **The same root as 1, fixed in port 0072** (-104 on). `heal_tunnels` restarts only where a tunnel is still gone from the kernel 16 s after a reload, so it does nothing there.
3. **Routes leave an ip4table when a shared address goes** (B39).
   - Two DHCP interfaces on one device lease the same address. When one restarts, every table loses that address's routes, while netifd still lists them.
   - Seen on OfficeOpenWrt, 2026-10-06. (0063)
   - Stopgap: `keep_routes` (sbin/aeolus-prober, uc/probe.uc), `from_shared`. *Works around.*
4. **`network.wireless` vanishes from ubus** (B36).
   - 25.12's wireless.uc publishes it once, at netifd start. After a ubus reconnect it doesn't come back until the network restarts.
   - Seen on OfficeOpenWrt, 2026-10-06: a 4,096-key status was the likely trigger. (0073, 0111)
   - Stopgap: `wireless_missing`, a critical alert, "Radios unseen"; the apply check counts networks with iw. *Detects.*
   - B84, its cause on the ucode side: ucode's ubus module doesn't reconnect after a ubusd restart, so airscan and the aeolus-* daemons lose ubus silently until restarted (seen on TestC260, 2026-10-24). **Fixed in port b4ecbba** (v2026.10.17.1-41-gb4ecbba, ucode-mod-ubus r2, main 0026), which all four Arista APs run since 2026-10-25: the module reconnects and registers its objects and listeners again. The daemons connect once and had no stopgap, so nothing was retired. The C daemons followed in port ca8aa83 (main 0027/0028): rpcd, dnsmasq (and its ACL), odhcpd and odhcp6c reconnect, ujail binds the ubus directory, and hostapd drops a phy that is gone. **Fixed in port 704bd6a** (build -46), on all four Arista APs since 2026-10-27: the C260 from its reflash at 10:13 CDT, the C-360s from 11:58 to 12:48. usteerd still doesn't reconnect. Nothing was retired: there was no stopgap.
5. **A Wi-Fi interface is briefly missing from netifd's listing during a reload** (B43). Seen on OpenWrt3c8f. (0201)
   - Stopgap: offair.json is kept across that.
   - **Not a fault: closed by porting as by design, 2026-11-03.** On a reconfigure, wifi-scripts' `wdev_config_init()` forgets a running radio's interface names until its handler names them again; the radio shows `pending: true` (and `up: false`) throughout, and the interfaces never stop. In stock 25.12 too.
   - **Aeolus honours `pending` (0301):** `uc/wifinames.uc`. The daemons see a pending radio as it last was while the kernel still has its interfaces; one-shots wait for it to settle; the apply check still waits it out. Nothing to retire: it follows netifd's design. Porting's optional wifi-scripts change (`build/bugs/b43-wireless-keep-ifnames.diff`) would make no difference to it.
6. **netifd loads protocol handlers only when it starts,** so a vxlan installed later is unusable. Seen on OpenWrtnight, 2026-10-03. (0057)
   - Stopgap: `get_proto_handlers`, and a network restart after installs. *Unclear.*
   - **Fixed in port 0063 (B44), build -90 (d9b6090):** on each `ubus call network reload`, netifd reads the proto scripts it hasn't seen, and sets up again an interface left on the "none" handler. Tested by porting from RAM on the W-118, 2026-11-01. In -93 (68fb6e8). **Stopgap retired where netifd has the fix, in v0.215.0 (0269):** after installing vxlan, aeolus-packages reloads the network and restarts it only if netifd still doesn't list vxlan, as a stock netifd doesn't. The restart after kmod-vxlan comes back, after a new kernel, stays: it isn't this fault.
7. **netifd `add_device` with the default `link-ext` never attaches an existing device.** Seen 2026-10-04. (0061)
   - Stopgap: passes `link-ext: false`. *Unclear.*
8. **ustp in the 25.12 feed, with a 1 s hello: false root claims.**
   - ustp before upstream 17c36be forgets a neighbour after three of its own hellos.
   - Seen on a C-360 with an Arista switch, 2026-10-11: about 60 times in 7 h. (0095)
   - Stopgap: renders `hello_time '2'` (uc/render.uc). *Works around*; the fix is a newer ustp in the feed.
9. **ustp ignores the bridge `priority`:** every bridge is 32768. (0095)
   - Stopgap: a UI warning. *Unclear.*

## Wi-Fi scripts, hostapd and wpad

10. **The first per-user key restarts the phy** (B37).
    - Seen once, OfficeOpenWrt 25.12.2, 2026-10-06. Not seen in five tries on the W-318 snapshot. Adding or removing a `wifi-vlan` restarts the radios too. (0070)
    - Stopgap: none. *Unclear.*
11. **`ubus call hostapd.<bss> reload` drops the whole phy's clients** (B38). OfficeOpenWrt 25.12, 2026-10-06.
    - Stopgap: Aeolus uses `ubus call network reload` instead. *Works around.*
12. **With wpad-basic, one `bss_transition` line takes every network down:** hostapd refuses the whole config. Seen on OpenWrtnight, 2026-10-03. (0057)
    - Stopgap: checks for `bss_transition_request` before rendering it, and installs the full wpad. *Works around.*
13. **A wpad package swap leaves hostapd stopped.**
    - Removing wpad-basic stops it, and installing the full wpad doesn't start it. Seen on OfficeOpenWrt, 2026-10-09: about 6 min down.
    - opkg also keeps its index in RAM only, frees nothing when a /rom package is removed, and gives no Installed-Size. (0088, 0172)
    - Stopgap: `full_wpad`, `repair_wpad` and a swap journal (libexec/aeolus-packages). *Works around.*
14. **ath11k has no AP/VLAN mode, and hostapd then fails every BSS on the radio.** Seen on a C-360, 2026-10-09 and 2026-10-10. (0082, 0111)
    - Stopgap: the agent reports `ap_vlan`, and the manager refuses such configs. *Works around.*
15. **A MAC deny list cancels RADIUS MAC authentication:** ap.uc writes `macaddr_acl=0` after `=2`. The DAS secret is passed only for WPA Enterprise. (0112)
    - Stopgap: the manager refuses the combination. *Works around.*
    - **Fixed in port 0050** (B51): the deny list is kept with RADIUS MAC authentication, and the DAS client line is written for every kind of authentication. Before, a non-EAP BSS with `radius_das_client` was refused by hostapd.
      - On all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e).
      - Upstream: [openwrt/openwrt#26343](https://github.com/openwrt/openwrt/pull/26343).
      - Verified 2026-10-31 by running -76's wifi-scripts generator: the DAS line carries its secret, and a deny list keeps `macaddr_acl=2`.
      - The stopgap stays while the stock OpenWrt APs lack the fix.
16. **hostapd ubus `switch_chan` with force re-runs ACS** on an automatic channel. Seen on a C-360, 2026-10-10. (0120)
    - Stopgap: `CHAN_SWITCH` on the control socket (sbin/aeolus-rrm). *Works around.*
    - **Fixed in port 0051** (45744d5, B52), on all four ath11k APs since 2026-10-31 in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e). Upstream: [openwrt/openwrt#26342](https://github.com/openwrt/openwrt/pull/26342). The ubus fallback now sets `iface->is_ch_switch_dfs`, as hostapd's own `CHAN_SWITCH` does, so the restart keeps the channel asked for.
      - Tested 2026-10-31 on the C-260, with the -76 wpad run from RAM: `switch_chan` with force from 5700 to 5500 went DISABLED, COUNTRY_UPDATE, then DFS with no ACS, ran a 60 s CAC on 100, and was ENABLED on 5500.
      - The stock wpad went to ACS and channel 149 (06:41Z).
      - The stopgap works on every AP, so it stays until the stock OpenWrt APs have the fix too.
17. **Probe responses come from a stale template after `set_vendor_elements`.** Seen on mt76 and ath10k, 2026-10-05. (0073)
    - Stopgap: RRM reads beacons only. *Works around*; hostapd or the driver.
    - **Beside it, B53, fixed in port 0065 (build -93):** `set_vendor_elements` with "" left the cleared element in the beacons for 2–5 s. RRM clears its element when it is turned off (sbin/aeolus-rrm), so beacons said it was an Aeolus AP a few seconds longer; no stopgap. In no image on Aeolus's APs yet.
18. **hostapd ACS checks only the primary channel against `channels`.** Seen on the R7800, 2026-10-05. (0075)
    - Stopgap: renders whole blocks only. *Works around.*
19. **hostapd and udebug** (B55).
    - hostapd looks for udebugd only at start, forgets `rx_frame` at each start, and ignores ring config after udebugd restarts. Seen 2026-10-13. (0146)
    - Stopgap: installs udebugd before wpad, and re-sets `rx_frame` (sbin/aeolus-journey). *Works around.*
    - **Cause found by porting, 2026-11-04, and fixed upstream in [openwrt/openwrt#26469](https://github.com/openwrt/openwrt/pull/26469):**
      - hostapd and wpa_supplicant get their udebug config through a libubus auto-subscriber that registers for the `ubus.object.add` event.
      - They run as `network`, and `wpad_acl.json` has no `listen` entry, so ubusd refuses the registration and libubus ignores the error. They find udebugd only by the lookup at their own start, so a late udebugd, or one restarted, is never seen.
      - The fix is `"listen": [ "ubus.object.add" ]` in `wpad_acl.json`, verified on the W-118. It's in no image on Aeolus's APs yet: the port holds it during the -112 soak, to ~2026-11-17.
      - Once an AP runs an image with it, installing udebugd before wpad can go for that AP. Stock 25.12 APs keep the stopgap until a 25.12 point release carries it.
    - **`rx_frame` resetting at each hostapd start is by design:** it is a reader's flag on one ring, and a restarted hostapd makes a new ring with a new id. Re-setting it every 10 s stays.
20. **usteer makes hostapd wait on every ubus listener,** and a ucode listener that answers nothing refuses the client. Seen on a C-360, 2026-10-11. (0118, 0138)
    - Stopgap: `notify_response 0`; the listener always answers. *Works around.*
    - **Fixed in port 0064 (B56), after -90:** a listener's NO_DATA (5) or METHOD_NOT_FOUND (3) counts as no opinion, and the first real refusal stands; the same for BTM queries. In no image on Aeolus's APs yet. Checked 2026-11-01: aeolus-steer, Aeolus's only hostapd subscriber, returns 0 on every path, so nothing it says becomes a refusal. The stopgap can go once every AP runs a hostapd with it, stock OpenWrt too.
21. **hostapd logs too little at info,** and the level resets on reload. (0133)
    - Stopgap: `logger_syslog_level 1` every 10 s. *Unclear.*
    - **Retired where hostapd's rings are read** (0254, v0.205.0). An attempt's steps come from hostapd's frames there, and hostapd stays at info. The stopgap stays only for an AP whose hostapd has no rings until it starts anew (0146).
22. **A broadcast probe reaches hostapd once per BSS.** (0146)
    - Stopgap: one copy kept per 50 ms. *Unclear.*

## Drivers

23. **ath10k-ct `ofdm_peak_power_rssi` is on by default,** adding 8/12 dB to frame signals (B3). It comes back with every driver reload. (0184)
    - Stopgap: `flat()` turns it off on every ath10k phy (sbin/aeolus-rrm). *Works around.*
24. **ath10k gives a 0 dBm noise floor on its own 5 GHz channel.** Seen on the R7800, 2026-10-05. (0073)
    - Stopgap: noise outside -127..-20 dBm counts as unknown. *Works around.*
25. **The nl80211 survey "in use" flag is wrong:** never set on ath10k and mt76, and set on the last-scanned channel on the C-360 and C-260. (0207)
    - Stopgap: picks the survey entry by frequency. *Works around.*
    - On ath11k, B60's fix (port 2a240ea, in b4ecbba on all four Arista APs since 2026-10-25) flags only the new channel after a channel switch. The stopgap stays: ath10k and mt76 never set the flag.
26. **mt76 reports one count as both `tx_retries` and `tx_failed`.** Seen on the OpenWrt One, 2026-10-22.
    - Stopgap: takes the larger of the two. *Works around.*
27. **ath11k never fills nl80211 tx retries;** `ext_tx_stats` is off by default and resets with the driver. Seen on C360-AP, 2026-10-15. (0164)
    - Stopgap: turns it on and reads it. *Works around.*
    - **Fixed in port 0062 (B62), in -93 (68fb6e8):** ath11k adds each frame's transmit count past its first, from the TQM completion, to the client's `tx retries`. On C360-AP on -93, 2026-11-01 09:50 CST, 12 clients' nl80211 retries summed to 415, as their debugfs counts did, though client by client they differ. **Stopgap retired where the driver fills them, in v0.215.0 (0270):** a radio where any client reads retries in nl80211 is read there, and the prober doesn't turn its stats on.
28. **ath11k monitor receive corrupts qca-nss-dp memory** (B4). (0146, 0163)
    - Stopgap: no monitor interfaces on a C-360. *Works around.*
29. **The ath11k scan radio goes deaf after being a client on a DFS channel,** and a rebind renames its phy. Seen on a C-360 QCN9072, 2026-10-11 and 2026-10-15. (0121, 0126, 0167)
    - Stopgap: rebinds the PCI driver (sbin/aeolus-clienttest), plus a health check and alert. *Works around, and detects.*
    - **Re-checked 2026-10-31, for the porting session (their B100).** Aeolus's check hasn't fired since port -62:
      - 57ff: 0 of 113 air tests;
      - ce7f: 0 of 115;
      - C360-AP: 1 of 128, a radio that was deaf at the start of a test (10-30 02:26Z).
      - The last deaf-then-rebind after a test was on 10-29, before -62.
    - The radio still goes deaf, though, and now at airscan's next sweep, after the test's own look. airscan's heal re-probes the QCN9072 31–46 s after the test ends:
      - airscan logs "no beacon heard in a whole round of the channels", then the kernel re-probes `0001:01:00.0`;
      - on -63, up to 06:23Z on 10-31: 13 re-probes after 67 air tests on the three C-360s.
    - The trigger is now 2.4 GHz channel 6, not DFS:
      - channel 6: 11 of 14 tests, to 57ff (ath11k) and the office (mt76) alike;
      - channels 1 and 11: none of 19;
      - 5 GHz DFS channels: 2 of 15.
30. **The ath11k scan radio's monitor passes FCS-failed frames:** 4,841 of 48,592 beacons spoilt. Seen 2026-10-16. (0161)
    - Stopgap: needed an advert twice (uc/rrm.uc), **retired in v0.204.2** (0161). *Worked around.* Frames flagged bad-FCS are still skipped: that is reading radiotap right.
    - The frames come flagged bad-FCS (radiotap Flags 0x40), which is how airscan and Aeolus skip them. But airscan makes its monitor as `iw phy <phy> interface add as-mon0 type monitor`, with no flags, so it never asked for `fcsfail`. If mac80211's default for such a monitor is control and otherbss, ath11k or mac80211 hands up FCS-failed frames unasked. The rate itself could be the air. With the porting session, 2026-10-31.
    - **Fixed in port 0054** (B64), tested 2026-10-31 on 57ff with the ath11k modules of the build then numbered -79 (accedd9) loaded from RAM over -76: bad-FCS frames on as-mon0 went from 11.0% to none.
      - On all three C-360s since 2026-10-31, in port [v2026.10.17.1-77-g0f4b8de](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-77-g0f4b8de) (-76 plus 0054 alone). After it, each C-360's scan radio heard adverts only from real APs at their real addresses.
      - The C-260's scan radio is an ath10k, which 0054 doesn't touch. The porting session measured its monitor on -76 the same day: 6,000 frames on three channels, none spoilt, flagged or not. No other Aeolus AP has a scan radio, so the stopgap went.
31. **mt76 can't scan on a DFS channel:** -16 EBUSY (B40). Seen on OfficeOpenWrt, 2026-10-06. (0073)
    - Stopgap: `cannot_scan`. *Detects*; may be regulatory design.
32. **Monitor-mode faults whose Aeolus code is gone** (0135, 0146):
    - mt76 turns off hardware offload for every client;
    - ath10k-ct floods the host;
    - ath11k passes 4 bytes past the FCS, unflagged, and key messages about 1 s late. The FCS tails are **fixed in port 0054** (B65), on all three C-360s since 2026-10-31 in port [v2026.10.17.1-77-g0f4b8de](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-77-g0f4b8de). On 57ff with those modules, frames ending in an unflagged FCS went from 88% to none, and beacons that end clean from 1 of 43 to 60 of 60. The scan radio's frames also carry radiotap's dBm antenna signal now (none before). After the fix they carry no FCS at all (no Flags 0x10), which Aeolus's beacon reader (`rrm.uc`) takes as it is;
    - the C-360's monitor interface took the radio's MAC.
    *Unclear*; the faults remain.
33. **The C-360's scan radio joins 5 GHz without HT/VHT** (802.11a, about 21 Mbit/s). (0121)
    - Stopgap: the page says so. *Detects.*
    - **Gone since port -62** (B66), which took the porting session's 0037 and 0038 for the QCN9072's 5 GHz band setup and stream count. The 5 GHz client tests' links:
      - before -62: legacy in 1,220 of 1,394 tests on the three C-360s;
      - on -62: HE in 90 of 96, the rest a 6 Mbit/s start;
      - on -63: HE in all 20, at 40 MHz, HE-MCS 5–6, 2 streams, 275–310 Mbit/s.
    - The client test page's note, "A legacy rate: the test radio does no better on this band", is gone since v0.204.1. It had also shown for an HE link whose rx rate was the last management frame's 48 Mbit/s.
34. **Transmit power:** the R7800's 2.4 GHz reports 30 dBm but tops out near 26, and mt76 takes only the per-phy txpower form. (0077)
    - Stopgap: uses the per-phy form. *Unclear.*
35. **Bluetooth:** the C-360's controller has no address, and a C-260's came up as hci1. (0171)
    - Stopgap: an address fallback in sbin/aeolus-ble. *Unclear.*

## ucode and its modules

36. **ucode-mod-* built for another ucode installs but doesn't load:** `ucv_metamethod_lookup` (B41). Seen 2026-10-24. (0214)
    - Stopgap: init and aeolus-packages load-test modules and reinstall broken ones; the manager serves the port's `-ucode` feed. *Works around.*
37. **A second `require()` of a module that failed returns null, not an error.** Seen 2026-10-24. (0214)
    - Stopgap: tests in a separate `ucode -e`. *Works around.*
    - **Fixed in port 0049** (backport of upstream 153c627f), re-checked by the porting session on 2026-10-31 against the images' ucode b885. Verified with -74's ucode on the C-260: a second `require()` now raises the error. It is on all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e). The stopgap stays while the stock OpenWrt APs' ucode may lack the fix.
38. **A fired uloop timer is never freed unless cancelled:** about 18 MB an hour on the C-360s, ucode 2026.01.16. (0171)
    - Stopgap: one-shot timers cancel themselves. *Works around.*
    - Re-checked by the porting session on 2026-10-31, on b885: about 130 B per fired timer. Upstream 115d308a fixes it but doesn't backport cleanly. It needs the full ucode update, which is Griff's call. Keep the stopgap.
      - The full ucode update is port 0056–0058, to OpenWrt main's ucode dfc64d41 (libucode20261009, the same ABI as upstream snapshot), in build -82. On the C-260 since 2026-10-31, at Griff's word: the agent runs on it once its ucode modules are builds for that ucode (0256).
39. **ucode GC: `gc('start')` can free an array that `filter()` or `map()` is building,** and `ucode -S` runs with collection off. Seen 2026-10-16. (0171)
    - Stopgap: `gc('collect')` at safe points. *Works around.*
    - **Fixed in port 0049** (backport of upstream 8a74d68a). It's worse than noted: in the porting session's test, `map()` over 20,000 entries returned 1,002 wrong.
      - Verified with -74's ucode on the C-260: 20,000 entries, none wrong. In that run the flashed ucode returned no entries at all.
      - On all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e). The stopgap stays while the stock OpenWrt APs' ucode may lack the fix.
40. **`uloop.task` leaks its pipe.** Fixed upstream in af8a3830; the images lack it. (0171)
    - Stopgap: aeolus-ble restarts itself past 768 descriptors. *Works around.*
    - **Not fixed by the port** (corrected 2026-10-31). With af8a3830 in, 50 finished tasks still left 50 descriptors open, on the C-260 with -74's ucode and in native builds. Upstream master leaves none.
      - The real fix is upstream's uloop task rework: a bisect lands on 1f54bd28. Like 38, it needs the full ucode update (build -82), which is Griff's call.
      - The porting session dropped af8a3830 from 0049 (858405e). Keep aeolus-ble's descriptor stopgap.
41. **25.12's ucode cannot parse a comment after an `export function`.**
    - Stopgap: one export statement per module. *Works around.*
42. **ucode-mod-socket reads `SO_TIMESTAMP` wrongly on 32-bit:** whole seconds. Seen on the R7800. (0135)
    - Stopgap: `SO_TIMESTAMPNS`, read raw. *Works around.*
    - **Port 0052** (B74) takes upstream 2e0b25a2, "socket: fall back to SO_TIMESTAMPNS for the OLD alias".
      - With -74's ucode on the C-260 (aarch64), the stamp arrives decoded as `{sec, nsec}`. Aeolus's `capture.stamp()` takes that as it is, and `agent/test/capture.uc` tests it.
      - The 32-bit check, on the W-118, is still to do.
      - On all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e). That is right on 64 bits: `{sec, nsec}`, with its fraction, on the C-260.
      - **Not fixed on 32 bits** (the W-118 on -76, 2026-10-31): SO_TIMESTAMPNS now arrives decoded, but as `{sec, nsec: 0}`, and SO_TIMESTAMP still as `{sec, usec: 0}`.
        - Cause, in ucode's `lib/socket.c` and still in upstream master: on 32-bit musl with 64-bit time, the libc's SO_TIMESTAMP and SO_TIMESTAMPNS are the kernel's `_NEW` values. The `_OLD` fallbacks carry those values, and the `_OLD` table entries come first and win. So the 16-byte `_NEW` stamp is read as `{long sec; long frac}`, and the "fraction" is the high half of the 64-bit seconds: 0. `st_timeval_new` also had the old struct's size.
        - Aeolus's effect: on a 32-bit AP on -76, the stamp comes without its fraction and `capture.stamp()` rejects it. So aeolus-journey would time a frame by when it read it, where it had the kernel's stamp read raw before. No AP Aeolus manages is affected: the only 32-bit Arista unit is the W-118 (ipq40xx), which isn't an Aeolus AP. The W-318 is ipq60xx, aarch64 like the C-260 and C-360. The Shed's stock 25.12 ucode still hands the stamp raw.
        - Stopgap: `capture.stamp()` rejects a decoded stamp with no fraction, so no frame is put at the start of its second. It stays.
        - **Fix: port 0057**, not pushed yet, on the dfc64d41 ucode line: the `_NEW` entries are looked up first, and the struct size is put right. It goes upstream to ucode too, and onto the APs with the ucode ABI change, which waits for Griff.
43. **ucode-mod-ubus: an older ucode keeps `bus.error()` set past a call that works.**
    - Stopgap: reads it once first. *Works around.*
    - Does not reproduce on the images' ucode b885 (the porting session, 2026-10-31): already fixed there. The stopgap can go once no AP runs an older ucode.

## procd, packaging, images, other

44. **procd on main kills a service's whole cgroup when it stops it** (B76). Seen on the W-318, 2026-10-18. (0079)
    - Stopgap: restarts through `ubus call rc init`, leaving the cgroup first. *Works around.*
    - **Cause found by porting, 2026-11-04, and fixed upstream in [openwrt/openwrt#26470](https://github.com/openwrt/openwrt/pull/26470):**
      - procd 39b5aac8c1 (2026-10-15) kills whatever is left in an instance's cgroup when its main process exits. A restart run from inside the service dies at the stop: rc.common traps TERM for that case, but the cgroup kill is SIGKILL.
      - The fix moves rc.common out of the service's cgroup before `procd_kill`. It's in no image on Aeolus's APs yet: the port holds it during the -112 soak, to ~2026-11-17.
      - Once every AP runs a procd with it, the stopgap can go. Stock 25.12 APs never had the fault.
    - **Warning for anyone on an AP running main's procd:** an SSH shell lives in dropbear's cgroup, so `/etc/init.d/dropbear restart` over SSH leaves dropbear down: a lockout until reboot (porting reproduced it on the W-118). The same goes for restarting rpcd through rpcd. Restart those from outside their cgroup (`ubus call rc init` from the agent), never from an SSH session into dropbear itself. Aeolus's agent restarts neither: the manager's key is read at each login, and rpcd is only asked to restart the agent.
45. **25.12-SNAPSHOT rebuilds packages under the same names:** apk reports "ADB integrity error". Seen on the C-260, 2026-10-16. (0171)
    - Stopgap: the manager re-fetches indexes every 5 min. *Unclear.*
46. **The C-360 port image has no ucode-mod-uclient, and a sysupgrade drops it.** (0080)
    - Stopgap: restore from init, and the agent's `stranded` wait. *Unclear.*
47. **Port airscan: its scan-radio mark outlived the radio,** and procd gave up after 5 failures. (0173)
    - **Fixed in airscan 0.2-7.** On 2026-10-24 every AP with airscan ran 0.2-r8 or 0.2-r12.
    - Nothing to retire: RRM's `hold_off` (sbin/aeolus-rrm) is Griff's rule for an AP whose scan radio is out of service for any reason, not a workaround for this.
48. **uclient sends one request in small writes:** Nagle plus delayed ACK add about 40 ms. Seen 2026-10-13. (0145)
    - Stopgap: the manager sets TCP_QUICKACK. *Works around.*
    - **Fixed in port 0053** (B80): TCP_NODELAY in uclient. A POST's segments now go out back to back, where the old uclient held the second until the first was ACKed.
      - On all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e).
      - The stall shows only against a server that delays its ACK, as the manager would without its stopgap.
      - Measured 2026-10-31 against such a server: a throwaway HTTPS server on the manager host, with no TCP_QUICKACK, answering only once a request's whole body is in. Medians of 15 uclient POSTs:
        - port APs: 11.0 ms (C-260, -76), 11.4 and 12.4 ms (C-360s, -77);
        - stock APs: 68.1 ms (Shed, 25.12.5) and 64.9 ms (Office, 25.12.2).
      - So the fix holds, and the stopgap stays: without it the stock APs would stall about 55 ms on every POST. It goes when stock OpenWrt has the fix.
      - Upstream: [openwrt/uclient#19](https://github.com/openwrt/uclient/pull/19) ("uclient-http: send a request without waiting for the delayed ACK"), sent 2026-10-31 with these numbers.
49. **UCI is slow at scale:** 500 keys in one batch didn't finish in 30 s, and 4,096 keys took about 20 s through the library. (0070, 0078)
    - Stopgap: none. *Unclear.*
    - **Fixed in port 0066 (B81, build -93):** `uci batch` unloaded every package after each command, so each replayed its whole delta. 500 sections and options went from 11.2 s to 0.21 s on the W-118, and 2,000 from 528 s to 1.8 s, with byte-identical files. In no image on Aeolus's APs yet, and stock OpenWrt keeps the slowness.
50. **ppsk with SAE turns off hash-to-element,** which 6 GHz requires. (0115)
    - Stopgap: RADIUS passphrases are offered on WPA2 only. *Unclear.*
51. **hostapd `del_client` answers 0 whether or not the client is there.** (0107)
    - Stopgap: checks `get_clients` first. *Unclear.*

## Reported by the porting sessions

52. **ath11k gives the in-use channel's noise floor from BSS-channel-info:** -108/-109 dBm on 5 GHz and -102 on 2.4 GHz, where the firmware measures about -92 to -96 (B1, B60).
    - Aeolus took it as a reading, being inside -127..-20. RRM rated an ath11k AP's own channel up to about 6 points quieter than it was (noise above -95 costs 2 a dB), and the minutes showed -109.
    - **Fixed in port 2a240ea** (v2026.10.17.1-40-g2a240ea), from the firmware's pdev stats. All four Arista APs run it since 2026-10-25 in port b4ecbba (kmod-ath11k r13): in-use noise reads -90 to -96. The APs tab shows each radio's floor since v0.174.0, and "—" where a driver's figure is below thermal noise.
    - The R7800's ath10k (Shed, OpenWrt 25.12.5) says -102 and -107 dBm, below thermal noise too: reported to the porting sessions 2026-10-25. Not their build; theirs to take upstream.
    - Nothing to retire: there was no stopgap.
53. **ath11k `fw_stats/beacon_stats` fails with more than one VAP on a radio** (B85).
    - **Fixed in port b4ecbba** (main 0025), on all four Arista APs since 2026-10-25, and sent upstream. Aeolus reads it since v0.175.0 (0221): each radio's beacons that did not go out.
54. **ath11k on QCN9074-family cards gives off-channel noise as -107 dBm** (B90), below thermal noise, from the firmware's chan-info event: a placeholder, or a raw value ath11k does not convert.
    - On the C-360 it is the scan radio's survey (2.4 and 5 GHz) and the 6 GHz radio's off-channel visits. The IPQ8074's own radios measure off-channel noise (-94 to -98).
    - Aeolus's effect: RRM rates the channels from the scan radio's survey, so an AP's own channel, measured since 52's fix, costs up to 6 points more for noise than channels with -107, which cost none. Under the 20-point margin a move needs.
    - Stopgap: none. Reported 2026-10-27; the porting sessions will convert the value or stop calling it a noise floor, and send it upstream.
55. **With BSS color off, wifi-scripts leaves out the Spatial Reuse Parameter Set** (B114).
    - hostapd.uc writes `he_spr_sr_control` only where color is on. With no such element, an 802.11ax client may take non-SRG OBSS-PD spatial reuse as allowed, up to -62 dBm. With no color to tell the AP's frames from another BSS's, it ignores the AP's CTS and BlockAcks below about -62 dBm, and its uplink dies.
    - Seen 2026-10-30: a Galaxy Phone-B on ce7f, in the bedroom and bath at -64 to -84, on both bands. Color had been off at Home since change 427 (2026-10-21), set during the IoT device tests. In the office, where the phone is strong, it got by.
    - Proven by the porting session on stock -62, with the phone at -76 to -81:
      - color off, with `he_spr_sr_control=3` forced: steady;
      - color off, without it: dead;
      - color on: fine.
    - Aeolus's effect: `bss_color: off` (0192) costs weak 802.11ax clients their uplink until the fix. Change 563 put color back to OpenWrt's default at Home.
    - Stopgap: none. The porting session writes the element whatever the color, as a port patch and an OpenWrt pull request.
    - **Fixed in port 1f746ea** (v2026.10.17.1-63-g1f746ea, port patch 0041, wifi-scripts r8), and sent upstream as [openwrt/openwrt#26326](https://github.com/openwrt/openwrt/pull/26326). With color off, `he_spr_sr_control=3` is written all the same. With color on, the render is unchanged.
      - The test ran on the Living-Room C-360 (57ff), on -63 since 2026-10-30 at 7:44 PM CDT. Change 569 held its color off, and porting turned its 5 GHz down to 1 dBm to make the phone weak.
      - The phone heard 57ff at -88 dBm, and 57ff heard the phone at -73 to -79. The phone sent 21 to 172 frames every 15 s, and its pings got through for about 6 minutes. On -62 in the same state it sent 2 to 5 frames, and no pings got through.
      - By 8:26 PM CDT that evening all four ath11k APs ran -63, published as a testing release. Color went back on at all of them.
      - Nothing to retire: there was no stopgap.
56. **A radio on a DFS channel that is reconfigured can stay down** (B115).
    - When a running radio is restarted by a config change, hostapd reads the channel list as the radio goes down. In FCC regions, cfg80211 moves the channel just left from available to usable about 2 s later.
    - ACS then picks that DFS channel again. hostapd still takes it as cleared and skips the CAC, and the kernel refuses it: `Could not set channel for kernel driver`, then ACS->DISABLED.
    - A cold boot is fine: no channel is cleared yet, so hostapd runs the CAC.
    - Seen 2026-10-30 and 31, on -62 and -63:
      - change 563 (color on) on the C-260 at 140;
      - change 574 (color on) on the Living-Room C-360 (57ff) at 108.
      - Each time the agent reverted after 150 s, and 5 GHz was down for 3½ to 4 minutes.
    - Aeolus's effect: any change that restarts a 5 GHz radio on a DFS channel with the channel on auto can take it down until the agent reverts, 150 s on. The change then isn't applied, and the AP shows apply-failed until the next version.
    - Stopgap: none. On Griff's go-ahead the porting session applied color on with the 5 GHz channel pinned to a non-DFS one (575 on 57ff, 578 on the C-260), then unpinned it. So ACS started from a non-DFS channel and ran a real CAC: 57ff took 108 again after 60 s of CAC. The porting session fixes it at base and upstream.
    - **Fixed in port 4d7c7ab** (0042), in hostapd: before it skips the CAC, it checks the kernel's DFS state for the channel again.
      - On all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e).
      - Upstream: [openwrt/openwrt#26341](https://github.com/openwrt/openwrt/pull/26341), and to the hostap list as `<20261031-b115-dfs-state-events-v1-1-4265a0f76be2@gmail.com>` (in moderation).
      - Tested 2026-10-31 on the C-260, at Griff's word; the test ended at 12:34 AM CDT. Its 5 GHz was held to channel 60, and the fixed wpad was run from RAM.
      - The stock wpad gave `Could not set channel` and DISABLED. The fixed one got DFS-PRE-CAC-EXPIRED for 5300 and 5320 during ACS, ran the CAC, and came up ENABLED.
57. **hostapd and wpa_supplicant crash with SIGSEGV every time wpad stops** (B116).
    - It happens on a reboot, a sysupgrade or a wpad restart, and leaves a 0.6 to 1.7 MB core in /tmp, in RAM.
    - Cause: ucode's ubus module (b885dd0f) gave each served call's `{args, info}` object as the prototype of the shared `ubus.request` type prototype, without a reference. `uc_vm_free` freed the object, then released the type prototype.
    - While it runs: a deferred request could read the `args` or `info` of a later call.
    - Seen on the Arista port (-63 and -64).
    - Not on stock OpenWrt 25.12, by the porting session (2026-10-31). Its ucode, 85922056 (2026-01-16), has neither the fix nor 5a79aa56, the July change that releases type prototypes after the final GC and so turns the dangling prototype into a crash. No 25.12 backport is needed on B116's account.
    - The office's OpenWrt One (25.12.2) has `/tmp/hostapd.1791669373.6085.11.core`, a SIGSEGV from 2026-10-10, 1.7 MB: probably another crash, outside the port. Whether to look into it is Griff's call.
    - Aeolus's effect: each wpad restart Aeolus asks for leaves a core in RAM. That is a Wi-Fi restart where hostapd must start anew for its rings (0146). The cores are small next to the APs' memory, but they add up until a reboot.
    - Stopgap: none.
    - **Fixed upstream** in ucode [fbdd169e](https://github.com/ucode-lang/ucode/commit/fbdd169efd17130fec41d219a1d3456a9f653120), "ubus: store per-call request proto in the instance proto slot" (ucode issue #437). It is in OpenWrt main since a4e92295, the ucode update of 2026-10-09. The port's pinned main, bcb16b63, is older.
    - **Port 0043** (openwrt-arista 6bbd572) backports it, with the ABI unchanged. libucode20230711 and ucode-mod-* go to a new release with the same soname, so nothing of Aeolus's needs rebuilding.
      - On all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e).
      - Tested 2026-10-31 on the C-260, with the -65 ucode run from RAM. On the old code, wpa_supplicant crashed on 3 of 3 stops and hostapd on 1 of 3.
      - With the fix, both exited 0 on 2 of 2 stops, with no core, and the Wi-Fi came back each time.
58. **hostapd logs every link measurement report usteer asked for as unexpected** (B118): `Unexpected Link measurement report, token 1`, at info level.
    - Cause, in source: OpenWrt's own ubus `link_measurement_req` (hostapd `src/ap/ubus.c`) sends the request itself, always with dialog token 1. It never sets hostapd's `link_mesr_req_active` or its token, so rrm.c's handler takes every reply as unexpected and logs it.
    - The report is not lost: OpenWrt's ubus handler runs next and still notifies subscribers, so usteer gets its measurements.
    - It hits stock OpenWrt too: OpenWrt main has the same code.
    - Seen 2026-10-31: usteer, running on every AP with its default `link_measurement_interval` of 30000, asks every capable client every 30 s. That was 38% of ce7f's 128 KB log (about 8 lines a minute, so the log covered about 45 minutes) and 22% of the office's (stock 25.12.2).
    - Aeolus's effect: none on what it measures. It never asks for link measurements: its 802.11k "heard" figure comes from beacon requests. But each AP's own log holds much less history; the remote syslog keeps it all.
    - Stopgap: none. Aeolus doesn't turn usteer's link measurement off to hide it.
    - **Fixed in port 0048** (openwrt-arista 997aab1). The ubus handler says whether it passed the report on, and hostapd's own handler, with its line, runs only where hostapd has a request of its own or no one took the report.
      - On all four ath11k APs since 2026-10-31, in port [v2026.10.17.1-76-g858405e](https://github.com/WiFiGUY512/openwrt-arista-images/releases/tag/v2026.10.17.1-76-g858405e).
      - Verified on -76 by the porting session: no "Unexpected Link measurement" lines on 57ff, C360-AP or ce7f since the flash, where there had been 124 of 1,010 and 268 of 998. usteer still gets the reports: 8 link-measurement-report notifications in 75 s on C360-AP.
      - Upstream: [openwrt/openwrt#26344](https://github.com/openwrt/openwrt/pull/26344).
      - Another author's fix, [openwrt/openwrt#26045](https://github.com/openwrt/openwrt/pull/26045), tested in place of 0048 on C360-AP from RAM, 2026-10-31 22:34–23:30 CDT, at Griff's word: 22 link measurement reports, all from one client (a8:b5:8e:8c:93:89), and no "Unexpected" line in 55 minutes. The result is posted on that PR. C360-AP is back on -83's own wpad.
59. **Every radio is torn down and set up again, endlessly, when the hostapd binary changes under a running wpad** (B119).
    - Seen 2026-10-31 on the C-260, from 07:41:31Z until a reboot at 14:50Z. It started seconds after the porting session lazily unmounted a test wpad binary from under the running daemon.
    - netifd's wifi-scripts tore down and set up all three radios every ~1.3 s, about 480 times in 10 minutes. Each setup reached ACS and was torn down again ("Interface ... wasn't started").
    - The log ran at ~122,000 lines an hour against ~3,000 normally. The forwarded copy on Hermeneus (`/var/log/aps/C260.log`) is how it was found.
    - With B120, hostapd grew from 5 to 437 MB and left 22 MB free, and the AP's next sysupgrade did not take.
    - Aeolus's effect: aeolus-packages replaces wpad on a running AP in two cases. It swaps an image's wpad-basic for the full wpad for 802.11v (0050, 0057, 0088; not on the Arista images, which have the full one), and it puts back a wpad a cut-off swap left missing (0172). Each then starts wpad and the Wi-Fi again. The same would happen with any wpad package upgrade on a running AP.
    - The manager saw it only as the AP's facts changing again and again: its BSS MACs came and went, 12 times between 08:58 and 14:42Z. It also showed in its reports' `health.top_mem` (hostapd's RSS).
    - Stopgap: none. The porting session will look into it.
60. **hostapd leaks about 20 KB each time an interface is torn down and set up** (B120).
    - Measured in B119's loop on the C-260: ~20 KB a cycle, ~1 MB a minute. On a clean boot it holds at about 5.8 MB.
    - Aeolus's effect: small but cumulative. Every Wi-Fi apply or "restart Wi-Fi" action is one cycle per interface, up to 12 on a C-360, so about 0.25 MB of hostapd memory each time, kept until wpad restarts.
    - Stopgap: none. The porting session will look into it.
61. **Port 0032's check for a DFS-free start misses one channel a block** (B121), so ACS can start a radio on a DFS channel.
    - Port 0032 sets `acs_exclude_dfs` when the radio's `channels` has a block outside DFS, so a radio on an automatic channel starts outside DFS and doesn't wait out a radar check. It counted a block only where every channel of it was listed.
    - hostapd's ACS takes a block where any one of its channels is listed and makes that the primary. In acs.c, `acs_find_ideal_chan_mode` took 124+128 with 128 unlisted.
    - Aeolus writes `channels` as each block's primary alone (0243): 36 44 52 … at HE40, 36 52 100 … at HE80. So the check never set `acs_exclude_dfs`, on every model since 0032 went in.
    - Seen 2026-10-31 on the C-360s after their -77 flashes:
      - C360-AP came up on 124 after a 60 s radar check;
      - ce7f came up on 140 after one. That check also gave it a needless facts change, Aeolus's own fault, mended in 0253.
    - Aeolus's effect: a 5 GHz radio on an automatic channel can start on DFS, a minute late, where a channel outside DFS was there to start on.
    - Stopgap: none. Aeolus's lists are as hostapd reads them.
    - **Fixed in port 0055** (ec9be25): the check counts a block as ACS does (wifi-scripts r10). The porting session checked its logic in ucode on C360-AP: right in 9 cases, where the old check was wrong in 4.
      - Tested on C360-AP in build -78 (-77 plus 0055), at Griff's word, 2026-10-31: acs_exclude_dfs was set, ACS chose 44, and 5 GHz served at 48 s of uptime with no radar check (97 s with one on -77).
    - **The port's dfs-look had the same rule** in its `blocks()`: under one channel a block it found no blocks at all. So every look on every C-360 since 2026-10-29 ended with its own block not rated, and dfs-look could never move a radio to a better DFS block.
      - Fixed in dfs-look r5 (port 1aba8ac): a block counts where all its channels are allowed and one is listed, and the channel it moves to is a listed one, as ACS has it.
      - Tried on C360-AP beside the service, moving nothing: it rated every block ("on 44-48 (4), best outside DFS 149-153 (3), best DFS 124-128 (1); stays").
      - Build -79 is -77 plus 0055 plus the dfs-look fix: the B121 release candidate, which goes on the APs at Griff's word.
62. **Not a fault: ath11k sends Espressif devices off on 2.4 GHz for missing ACKs** (B122, closed 2026-10-31). hostapd logs "disconnected due to excessive missing ACKs" and sends a disassoc, reason 34. Each one was a client that had really gone.
    - Suspected from three kicks on Home Wi-Fi IoT (CDT): the kettle (bc:ff:4d:34:90:17) by ce7f at 16:30:43 and by 57ff at 16:37:32, at −62 and −59 dBm with 0 retries; drive-cooler (2c:f4:32:9b:ca:67) by the C260's IPQ8074 radio at 17:16:07, at −41 dBm.
    - Porting's debug ath11k on 57ff logged each firmware peer kickout in full: 396 events from 7 MACs, every one reason 1 (XRETRY) with no signal (rssi −128). Those include an AP's test station, which really is gone after its test, so the event alone can't tell a wrongly kicked client from a gone one.
    - Aeolus's records showed each one had gone:
      - the kettle runs a loop of its own: it joins, works about 5 minutes, goes silent about 100 s, then joins another AP. Kicked or not, it was away about 100 s. Under the stopgap, 57ff's minutes at 17:33–17:34 showed frames to it failing and then nothing received, before it joined C360-AP by itself at 17:35:14;
      - drive-cooler was authenticating on C360-AP, back from the flash, in the same second the C260 sent it off;
      - chris-bedside-lamp (d8:3b:da:70:d5:0c), at 19:20–19:25 under the stopgap: −39 dBm but 326 of 328 frames failed and nothing received, then removed by the inactivity check. Switched off.
    - So `disassoc_low_ack` on only ends a dead association sooner.
    - 0257's stopgap (change 596, 2026-10-31 17:19 CDT, disassoc_low_ack off on Home Wi-Fi IoT) was undone at Griff's word by change 597 (22:13 CDT). Off, it only kept a gone client listed until hostapd's 300 s inactivity check, retrying to it meanwhile.
63. **mt76 (mt7915) counts a station's tx retries below zero in 32 bits** (B124). `mt7915_mac_tx_free()` adds `count - 1` a frame, so a frame with count 0 takes one off: nl80211's `tx retries` reads 2^32 − k (k 19–70). Seen on Office (OpenWrt One, 25.12.2, mt76 39c960c3), 2026-11-01, only on 5 GHz, for the C-360 client tests' short stations: 44 minutes in about 10 days, each a step of about 4.29 billion retries, so Office's 5 GHz load said 100% retried. (0272)
    - Stopgap, at Griff's word ("Guard it"): a running count that steps by 2^31 or more between two looks is passed over, not growth (`minutes.grew_by`), in the minutes, the flight recorder's points and RRM's step windows. *Works around.*
    - Fixed upstream in mt76 748311fa (2026-06-05), in OpenWrt main; not in openwrt-25.12. Porting staged a backport for 25.12; sending it is Griff's call. The stopgap can go once Office runs an mt76 with it.
64. **airscan, stopped mid-heal, waits forever for a scan radio with no driver** (B125). Its heal unbinds the radio's driver and binds it again; stopped between the two and started again, it finds no phy and stays "waiting for scan radio". Seen on C360-AP (-95), 2026-11-01 13:18:46 CST: an Aeolus client test, asked the second the heal began, stopped airscan. The radio had no driver until it was bound by hand at 13:22.
    - Aeolus's part, the test stopping an airscan that heals, is fixed in 0275: the test waits for the heal, and its own bind gives a radio with no driver its driver through `drivers_probe`.
    - Stopgap: none. Sent to the porting session (airscan could probe an unbound radio at start, or finish a heal before it stops).
    - Stopgap: none. **Fixed in port 2df7a72** (airscan r15), in -105: a stop mid-heal binds the driver again, and an airscan waiting on a radio with no driver asks drivers_probe, once a minute at most. Tried on C360-AP from RAM, 2026-11-01.
65. **ucode's rtnl can't make a rule by interface name** (B126): `RTM_NEWRULE` with `oif` (or `iif`) is accepted, but the rule comes out detached (flags 16, FIB_RULE_OIF_DETACHED) and never matches, as if the name were sent as an ifindex where FRA_OIFNAME wants a string. Seen on the port snapshot's ucode and on 25.12's, 2026-11-01.
    - Aeolus's effect: none. The test client's rule is by its source address instead (0274). Sent to the porting session for upstream ucode.
    - Aeolus's effect: none. The test client's rule is by its source address instead (0274). **Fixed in port 0068** (ucode patch 017), in -105; upstream ucode-lang/ucode#517.
66. **hostapd's ubus switch_chan can't move a 6 GHz radio at 40 MHz or wider** (B129). OpenWrt's ubus.c passes segment indexes 0 to hostapd_set_freq_params(), which sets center_freq1 to the 40 MHz center and works out the 80/160 MHz one only on 5 GHz. nl80211 is given an 80 MHz width with a 40 MHz center, and the kernel refuses it: "invalid channel definition", "Operation not supported". At 20 MHz it works, as does the control socket's CHAN_SWITCH, which passes center_freq1 on. Seen on 57ff (-95), 2026-11-01. Same in OpenWrt main.
    - Aeolus's effect: RRM can't move a 6 GHz radio (0278).
    - Stopgap: none, at Griff's word ("Wait for porting's fix"). Fixed in port 0070 (2bf4f64), build -96.
    - Stopgap: none, at Griff's word ("Wait for porting's fix"). **Fixed in port 0070** (2bf4f64), in -105. **Confirmed live** on C360-AP (-105), 2026-11-01 21:33 CST: ubus switch_chan from 21 to 181 at 80 MHz (cf1 6865), rc 0, AP-CSA-FINISHED, as RRM sends it (0278).
67. **802.11r roams on WPA Enterprise to a stock 25.12 AP always fail** (B128). hostapd sets "RSNXE used" in its FTE wherever `sae_pwe` is 1 or 2, whatever the AKM, and 25.12's wifi-scripts write `sae_pwe=2` on WPA3 and mixed Enterprise (`wpa3-mixed`), where there is no SAE. The FT Reassociation Response then claims an RSNXE it doesn't carry, and wpa_supplicant deauthenticates with reason 13. Seen on Office (25.12.2), 2026-11-01: 0 of 11 roams; porting's capture of 17:10 CST showed it. Main's scripts set sae_pwe only for SAE.
    - Aeolus's effect: every Enterprise roam to a stock 25.12 AP fell back to a full sign-in.
    - Stopgap, at Griff's word: Enterprise with 802.11r is rendered with `sae_pwe '0'` (0279). *Works around.* Porting fixes hostapd (port 0071, -103; upstream). It can go once 25.12 carries the fix.
    - **Fixed in port 0071** (in -103 on; sent to the hostap list). The stopgap stays for stock 25.12 APs, as Office: on a port AP it changes nothing.
68. **hostapd logs `nl80211: kernel reports: key addition failed` at every FT roam** (B137), at daemon.err. hostapd tries the new key at FT authentication on purpose, mac80211 refuses a key for a station not yet associated, and hostapd sets it again after association: the roam goes on. Seen on C360-AP (-112), 2026-11-04: 11 of 11 roams, each the same second as its AP-STA-CONNECTED with auth_alg=ft.
    - Aeolus's effect: the agent gives hostapd's last daemon.err lines as the reason a config failed, and after a roam this could stand in for the real one.
    - Stopgap: the agent leaves it out of that reason (`HOSTAPD_NOISE`, sbin/aeolus-agent). *Works around.* A quiet fix needs design work in hostap; nothing alerts on it.
69. **hostapd's ACS can't start a 6 GHz radio at 160 MHz after it ran on a channel outside the new list** (B138). Seen on C360-AP (-112), 2026-11-04 02:06 CST, and reproduced 2 of 2 by porting, then on the W-318.
    - Port 0035 (B90) drops the QCN9074's fake off-channel noise, so only the channel the radio last ran on keeps a full survey entry. When that channel isn't in the 160 MHz chanlist (`5 37 69 101 133 165 197`, one per block), hostapd takes the survey study, no channel of the list qualifies, and it has no fallback to its BSS-based study: "ACS: Failed to compute ideal channel", "Interface initialization failed".
    - Aeolus's effect: change 695 (6 GHz at 160 MHz) failed to apply on C360-AP, which had run at 80 MHz on channel 21, and the agent put back the config before it (apply-failed:695). The Living Room and Garage came up, since their last channels were in the list.
    - Stopgap: none. Not worked around in the render; offering every PSC channel at 160 MHz was porting's suggestion, left to Griff. *Detects* (the apply-failed alert).
    - **Fixed in port 0081** (hostapd PKG_RELEASE 13: an empty survey study falls back to the BSS-based one, and its mark is cleared at ACS cleanup), proven on the W-318 2026-11-04. hostap patch c927563 staged for Griff to send. In no image yet: the next one, on Griff's OK.
