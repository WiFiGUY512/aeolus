# 0329. 160 MHz on 5 GHz starts at 80 MHz, and is widened

- Status: Accepted (Griff, 2026-11-06: "do both, I support using the scanning radio for the DFS enhancement", and "same for the 260")
- Date: 2026-11-06
- Proposed by: Griff: "So basically if its 160mhz channel it has to wait until the 1m marker clears before it can use that channel? I wonder if we can start a 40mhz channel outside the band and dynamically expant them after the minute had expored". Written up by Claude
- Refines: 0120 (DFS after a look), 0328 (avoid or allow)
- **Held (2026-11-06, v0.289.0), then gated anew (v0.290.0):** a radio at 80 MHz that advertises [VHT160], as port 0101 had it do where uci set vht160, drops some clients' uplink to HE-MCS 0 at range: an iPhone 15 Pro, openwrt#22435, which is why stock OpenWrt strips it (openwrt#22436). Port 0102 (hostapd patch 895) replaces 0101: hostapd adds VHT160 only on a switch to 160 MHz, takes it away on a switch back or a failed one, and its networks' `get_features` says `csa160`. Tried on the Garage, 2026-11-06 15:11–15:13 CST: the beacon said 160 MHz only while at 160. The narrow start is gated on that (the agent's `csa160` fact, kept per wpad build in /etc/aeolus/csa160.json for boots, when no network answers yet), and no radio is given vht160 any more: the renderer removes it, and the render check refuses it. It is live from port -131, which has 0102.

## Context

- **Every 160 MHz block on 5 GHz holds DFS channels** in the US: 36–64 has 52–64, and 100–128 is all DFS.
- **Under 0328, a radio set to 160 MHz with DFS allowed picks a channel as it starts.** It then waits a minute, silent, for its radar check, at every start.
- **A radio can't listen for radar on one block while it sends on another.** So widening still costs that minute, unless something else does the check (below).

## Decision

### Start narrow, widen in the window

- **Where all of the following hold, the radio starts at 80 MHz outside DFS and serves at once:**
  - `radio.5g.width` is 160;
  - `radio.5g.dfs` is allow (or unset, or look-first);
  - the channel is automatic;
  - radio resource management moves radios;
  - the set (all of 5 GHz where it has none, without 165–177 while compatibility is on) has a whole 80 MHz block outside DFS;
  - the AP's wireless scripts keep the radio's 160 MHz capability while it runs narrower (`facts.vht160`, below).
- **The capability.** Stock OpenWrt's `hostapd.uc` takes [VHT160] and [SHORT-GI-160] out of `vht_capab` below htmode 160. hostapd then refuses every switch to 160, announced or by restart ("160 MHz channel width is not supported!"; porting, 2026-11-06).
  - The port's patch keeps them where the uci option `vht160` asks, and says so in `hostapd.uc` (`vht160 kept below 160`). The agent looks for that and reports it as `facts.vht160`.
  - A radio that starts narrower is given `vht160 '1'`.
  - Elsewhere, as on stock images, the radio starts at 160 and picks, as in 0328.
  - The render check can't see an AP's scripts, so it takes either: the 160 MHz mode, or 80 MHz with `vht160 '1'` and `wide`.
- **What the renderer does:**
  - It writes the 80 MHz htmode, the set's primaries at 80 MHz, and `acs_exclude_dfs` (0120's look-first).
  - It gives RRM's section `dfs '1'` and `wide '160'`, the width to widen to.
- **RRM widens it** (`rrm.widen`): to the best 160 MHz block its set holds that no neighbour uses and that rates no worse than where the radio is by more than the margin. The block must also meet 0120's DFS rules: looked at twice, none of it closed for radar, a check of 60 s at most, not left within a day, and kept apart from the AP's own 6 GHz (0327).
- **When:** in the window for moves alone, once the reason has held STEADY. Never as the radio starts.
- **How:** hostapd's `CHAN_SWITCH` at 160 MHz, as 0120's moves. hostapd starts the radio's networks again at 160 and checks for radar for a minute, which is why this happens at night.
- **A widened radio narrows again only for interference or a neighbour on its block.** "Better" and "start" don't count, since any 80 MHz block rates better than the 160 it was set to.
  - It narrows by an announced switch to an 80 MHz block outside DFS, with no check needed.
  - The 160 block it left is barred for a day, as 0120 bars a DFS block left for a reason.
- **A widened radio that can't scan** (no scan radio) steps off to where it came from, at the width it came from (`home_width`), by 0120's rules.
- **Each restart starts it at 80 again,** and the next window widens it.
- **Unchanged:**
  - Where RRM moves no radios, or the set has no 80 MHz block outside DFS, the radio starts at 160 and picks, as in 0328.
  - A channel set by hand is used as set.
  - Avoid refuses 160 MHz.
- **One rule in each half:** `start_width` in `render.uc`, and `radio.StartWidth`, which the render check uses. The check wants `wide` on RRM's section where the radio starts narrow.
- **The move's reason is `wide`.** The manager takes it in reports, and the neighbours page says "widened to the width it is set to". The daemon now keeps `own` and `wide` moves across its restarts, and the widths of where a radio went and came from.

### The scan radio does the check (porting, under way)

- **On the C-360s (QCN9072) and the C-260 (QCA9882),** the scan radio can run the radar check on the target block while the serving radio keeps serving. Porting tested this on 2026-11-06:
  - cfg80211 hands the finished check to the serving radio (same FCC DFS region).
  - Both scan radios detect radar. The QCN9072 checks 100–128 at 160 in one run. The QCA9882 is 80 MHz hardware, so it checks 52–64, or one half of 100–128 while the serving radio holds the other.
  - In the US a checked channel lapses about 2 s after nothing operates on it.
- **Porting's design:** airscan offers `precheck`. It runs the check, then holds the block with a hidden, client-less beacon from the scan radio until the serving radio arrives (or `hold`, 30 s), and says each step in `airscan.precheck` events.
- **Tried on C360-AP, 2026-11-06 at 10:59–11:02 CST:**
  - The scan radio checked 52–64 and held the block with a hidden beacon. The 5 GHz radio went from 100/40 to 52/80 by `switch_chan`, with no check of its own and no restart, and back to 36/40.
  - a laptop followed both switches without reassociating, lost no pings, and widened to 80 MHz (HE-MCS 11, 2 streams). An 802.11n client followed at 40.
  - An Amazon 802.11n device moved to 2.4 GHz 8 s after the first switch. Its 5 GHz rate had been poor before.
- **RRM uses it where airscan's status lists `precheck` among its features** (port -130 on):
  - **Every move to a DFS block goes this way:** a widening, and 0120's look-first moves too.
  - **The claim stays announced while the scan radio checks.** RRM asks `airscan precheck` with the block, `hold` 60 s, and an id of its own, then reads airscan's status at each hello. Once the block is `available` and `held`, the radio moves there by `switch_chan`, an announced switch with no minute off the air, and RRM sends `precheck_done`.
  - **The radio keeps where it came from** (`away`), as after a radar check of its own.
  - **It fails** when the check found radar, ended without the block, or said nothing within the check's time and 30 s more. A block where radar was found, or that Linux closed, is left alone for a day.
  - **A busy scan radio is asked again** at the next hellos (six at most). Where airscan can't check at all, the move goes by today's radar-check restart.
- **With it, a widening also goes soon after the radio starts,** once it has looked, as 0120's DFS moves do, as well as in the window.
- **Without it** (stock airscan, an AP with no scan radio), RRM widens in the window by the restart, above.

## Consequences

- **At 160 MHz with RRM, a radio serves at once after every start.** The minute of silence comes once, at night, as it widens.
- **Before the widen, the radio runs at 80 MHz:** after a restart in the day, until the next window.
- **Clients may not follow a widening:** some may stay at the old width until they reconnect. This is to be seen in the switch test.
- **Tests:**
  - render cases `dfs-allow-160` (starts at 80 with `vht160 '1'` and `wide '160'`), `dfs-allow-160-stock` (scripts without the patch: starts at 160 and picks) and `dfs-allow-160-still` (RRM moves no radio: starts at 160 and picks);
  - `TestStartWidth`;
  - `rrm.widen` and the 36/160 switch in `agent/test/rrm.uc`;
  - the render check over all cases.
