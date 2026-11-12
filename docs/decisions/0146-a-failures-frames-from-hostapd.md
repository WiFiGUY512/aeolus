# 0146. A failure's frames from hostapd's own rings

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff, when the C-360's monitor interfaces were tied to its crashes: "can we capture it on demand and have that demand be triggered by every association attempt?", and then "doesnt the radio already have the bits we are interested in in the ring buffer?" Written up by Claude
- Refines: 0135, 0118, 0144

## Context

- **A capture's radio side came from a monitor interface on each radio that serves (0135),** and what that cost differed by radio.
  - mt76 (the OpenWrt One) had none: its clients would have lost what the radio does for them in hardware. A failed attempt there had the wire side alone.
  - ath10k-ct (the R7800) handed every frame on the channel to the kernel, to be dropped there.
  - ath11k (the C-360) handed frames up late and with bytes behind them, and on 2026-10-13 the kernel crashed in the driver's monitor path (`skb_release_data` on a frame `ath11k_dp_rx_process_mon_rings` had just built). The C-360 had been restarting every few hours since 2026-10-12. Captures were turned off there that morning; whether that ended the restarts was still being watched when this was written.
- **A radio keeps no history of frames that can be read.** hostapd sees the ones that matter: every management frame of its networks, and every message of a key handshake or an 802.1X sign-in, goes between hostapd and the kernel over netlink.
- **OpenWrt's hostapd keeps its netlink traffic in rings another process may read (udebug),** when asked: `wpa_nl_rx`, what the kernel hands it, and `wpa_nl_tx`, what it asks of the kernel. Nothing is asked of the radio.
- **Read on two kinds of AP, 2026-10-13,** the R7800 (ath10k-ct, 25.12.5) and the OpenWrt One (mt76, 25.12.2), a client's coming is all there:
  - its probe requests and the AP's answers, authentication, association, deauthentication and disassociation, each frame whole;
  - the four key messages, both ways;
  - of each frame the AP sent, the radio's word on whether the client acknowledged it;
  - of each frame heard, how strongly, and on what frequency;
  - each at hostapd's own time of it, to the microsecond.
- **What hostapd never sees is not there:** a frame's retries on the air, frames with a bad check sequence, control and data frames, the rate a frame went at, block-ack setup, and what a client said to another AP.
- **ath11k was not tried:** the C-360 was left alone for its test. hostapd's side is the same code whatever the radio.
- **hostapd looks for the service that hands rings out (udebugd) once, as it starts.** It runs as its own user, which may not be told when a service appears. Installed after hostapd started, the service is not found: the Office AP had no rings until its Wi-Fi was restarted.
- **Should udebugd restart, hostapd's rings carry on,** and hostapd no longer hears changes to their config until it starts again.
- **Frames a radio heard are kept only while the ring is told to keep them** (its `rx_frame` flag), which it forgets at each start of hostapd.
- **The rings hold the rest of hostapd's talk with the kernel too:** the radio's survey for each network's load every few seconds, each client's counters when asked, the beacons it sets. About 90 entries and 20 to 30 kB a second on the lab's APs. As hostapd makes them unless told a size, 256 kB and 1,024 entries, the OpenWrt One's held 7 to 9 seconds.
- **A client's asking for networks, sent to every station, is handed to hostapd once for each network its radio has:** six times on a radio of six.
- **Nothing numbers a frame hostapd sends:** netlink's sequence number is 0 on all of it. The radio's word on a frame repeats the frame: a management frame as it was given, a key message behind fourteen bytes of an Ethernet header.
- **Most answers to probes are sent with no word asked for,** and none comes.
- **Reading both rings five times a second, in ucode, took 10.5 ms of processor a second on the R7800,** 1 % of one of its two. Twenty times a second, as while a client is traced, the whole recorder took 30 ms a second on the OpenWrt One.
- **A capture made this way opens in Wireshark with nothing malformed** (a client's rejoin on the R7800, and on the OpenWrt One, which had no radio side before): probes, authentication, association, `Key (Message 1 of 4)` to `4 of 4`, then DHCP and ARP from the wire side.
- **On the OpenWrt One hostapd made the rings as it ran,** asked by the recorder, with no restart: it had found the service when its Wi-Fi was restarted that morning.

## Decision

- **`aeolus-journey` makes no monitor interface.** The radio side of a capture is what hostapd heard and sent, on every radio alike. The OpenWrt One has it now.
- **It asks hostapd to keep the two rings,** by udebug's own config (`udebug.hostapd`): `wpa_nl_rx` of 1 MB and 4,096 entries, `wpa_nl_tx` of 256 kB and 2,048. hostapd makes them as it runs; nothing restarts.
  - **What it sets is marked as Aeolus's (`aeolus`), and only what is marked is taken back** when captures are set off. An AP whose owner has hostapd keep rings for their own reasons keeps them.
  - Where hostapd kept none before, its log's ring and its control ring stay off.
- **It tells the rx ring to keep frames heard, at every look** (ten seconds).
- **It reads both rings as they fill:** at each turn of its loop, five times a second at the most, once a second at the least. The two are gone through as one, by time.
  - **Kept:** management frames but beacons, and key and sign-in messages. Everything else is let go as it is read.
  - They go to the radio's ring of 0135: three minutes, 2 MB and 8,000 frames at the most.
  - **A frame to every station, handed up again within 50 ms, is kept once.**
- **A frame the AP sent is kept when hostapd asks for it,** which is its place among the others. The radio's word is put on it when it comes.
  - **Not acknowledged:** the frame's header says it failed, and the file has a note beside it, "The client did not acknowledge this."
  - **No word in five seconds, where one was asked for:** a note says the radio did not say.
  - A frame to every station, which none acknowledges, and one sent with no word asked for, have no note.
  - A word whose frame was not seen asked for keeps the frame from the word.
- **In the file each frame has a radiotap header made for it:** the channel, where hostapd said the frequency; how strongly it was heard; and, of one the AP sent, whether it failed.
  - **A key message is written as the data frame it crossed the air in,** between the client and its network, in the clear as hostapd has it, with no sequence number and no QoS field.
  - A management frame the AP sent is as hostapd gave it: the kernel fills in its sequence number and how long it holds the air afterwards.
- **A frame's time is hostapd's own.** So a message of the AP's is before the client's answer to it, and 0135's corrections go: frames handed up late put back, a message put before its answer, a check sequence trimmed.
- **The two packages are `udebugd` and `ucode-mod-udebug`.**
  - **A new AP has them installed before the full wpad replaces the basic one,** which starts hostapd anew: it finds the service, and captures begin at once.
  - **An AP that had the agent before has them asked for,** as `ustp` is (0095): the renderer says the config needs them, and the agent installs them.
  - **Wi-Fi is not restarted for it.** hostapd there has rings once it next starts anew; `aeolus-journey` says so in the log, once. Until then a failed attempt has the wire side.
  - **Someone who asks such an AP to restart its Wi-Fi (0104) has hostapd started anew with it:** the Wi-Fi down, hostapd restarted, the Wi-Fi up. Its clients were to drop in any case. On an AP whose hostapd has its rings, that action is as it was.
  - After a sysupgrade they come back with the others.
- **A monitor interface an earlier release left is taken away.**
- **A client that is traced (0144) has its frames said as they are read,** each a line of the trace at hostapd's time of it, under the stage it is of. Griff: "live trace should be hostAP only, actual packet captures should be somewhere else". No file is made, and nothing of a capture is in the trace's panel.
  - **Looking:** its probe requests, with how strongly each was heard; one a second a radio at the most, as a client sends several at a time. The AP's answers to them are not said.
  - **Authentication and Association:** its request, with how strongly it was heard, and the AP's answer: ok, or refused with the status. A reassociation says which AP the client comes from. WPA3's commit and confirm, each way.
  - **Keys:** each of the four messages, each way; and where hostapd's frames are read these are no longer said a second time from its log.
  - **Sign-in:** each EAP message, by its kind and no more: no identity.
  - **Left:** a deauthentication or disassociation, either way, with its reason.
  - **Steering:** a BSS transition asked, and the client's answer; and what it asks or is told of the neighbours.
  - **A frame the AP sent that was not acknowledged is said again, saying so.**
  - While a client is traced the rings are read twenty times a second.
  - hostapd's own words of the client (authenticated, associated, key handshake completed, and why not) stay beside the frames: they are its verdict.
  - Nothing changes in the manager or the page: the stages are those a trace had.
- **The rest of 0135 stands:** the ten seconds before an attempt and the two after, the file made three seconds after it ends, how many are kept and for whom, and all of the manager's side. `system.captures` off now also stops hostapd being asked.

## Not done

- **ath11k is not tried.** It is the first thing to do on the C-360 once its test is over.
- **The manager is not told that an AP's captures wait for its Wi-Fi to be restarted:** it is in the AP's log alone, and nothing in the page says that Restart Wi-Fi would bring them.
- **An attempt hostapd logs nothing of has no record, and so no capture,** though its frames are in the ring: a client barred for some seconds (tried, 2026-10-13) is answered and never logged. Attempts are still begun and stepped from hostapd's log (0118, 0133); the frames could do both, and time each step better.
- **A capture's frames are not listed in the page:** the file is there for Wireshark.
- **A trace does not say what its client did in the seconds before it began,** though the radio's ring holds it.
- **With captures set off, hostapd is not asked for its rings, and a trace has hostapd's log alone,** as before.
- **For up to ten seconds after hostapd itself starts anew, frames heard are not kept:** the ring forgets it was told to, and is told again at the next look. Wi-Fi set up afresh does not start hostapd anew; a restart of it, or of the AP, does.
- **What the reading costs on an AP of a hundred clients is not measured.** Each client's counters, asked every second, are among what is read and let go.
- **hostapd not finding a service installed after it started is OpenWrt's to mend** (its access list for hostapd lets it listen for no new service). Not sent upstream.
- **An OpenWrt before udebug (23.05 and earlier) has no radio side,** where it had one from a monitor interface.
- **The capture of the radio's own view is gone:** retries, rates, and what a client said to a neighbour on the same channel.

## Since: records from the frames (0254, 2026-10-31)

- **The frames make every attempt's steps now, not only captures and traces.** So the rings are read whether or not captures are on: captures off keeps no capture, and no longer stops hostapd keeping its rings.
- **An attempt hostapd logged nothing of** (Not done, above) is still none: a refused authentication or association is a step, and begins no attempt.
