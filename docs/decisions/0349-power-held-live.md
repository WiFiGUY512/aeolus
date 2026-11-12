# 0349. A radio's power held live

- Status: Accepted (Griff, 2026-11-10: "#1 seems worth it as well", of "change power without restarting radios"). Revised 2026-11-11 for v0.321.1: a radio is held by what it sends, not only by what was held
- Date: 2026-11-10
- Proposed by: Claude, after the Office dropped its clients for a power change. Written up by Claude
- Refines: 0077 (power control), the radio settings' power

## Context

- **A radio's power, set in Aeolus, was netifd's `txpower`.** A change to it is a change to the Wi-Fi's config, so the agent's `reload_config` had netifd set the radio up again to make it.
- **On 2026-11-10 at 23:27 CST, Griff's cell sizes (change 817: 2.4 GHz to 15 dBm, 5 GHz to 17) went to five APs:**
  - the C-360s and the C260 took it as a reload, "Reloaded settings for phy", and no client dropped;
  - the Office's OpenWrt One lost both radios for about a minute, and its 15 clients rejoined other APs. Its 5 GHz came back on another channel.
- **The Office's drop was hostapd crashing in the reload** (porting's B176): it was rebuilding a radio's networks and met ones already freed. Their fix is staged; the One is behind the fleet.
- **A running radio takes a power at once with iw, and no client notices.** Power control (0077) has done it since October, on the OpenWrt One among others. On the One at 23:54 a radio went 17, 16 and 17 dBm by iw with its seven clients on, and nothing was set up again.

## Decision

- **A power set for a band is no longer netifd's.** The renderer takes `txpower` out of the radio's section and writes the power into the agent's package instead:
  - the `aeolus_power` section, of type `power`, an option a radio, by its section's name, in whole dBm: `option radio1 '17'`;
  - auto, the radio's own, is no option, and no `txpower` either, as before;
  - unset in Aeolus, the AP's own `txpower` stays, as before.
- **`aeolus-power` holds each radio at it,** with `iw phy <phy> set txpower fixed`:
  - the agent runs it after each apply, at each state report, and as it starts, as it does `aeolus-gi`;
  - the net hotplug runs it when hostapd makes an interface anew, once its network is up. netifd sets a radio to its own power as it sets it up, so a radio set up again is held again;
  - what it holds is in `/var/run/aeolus/power/`, a file a radio: the power, the wiphy, and its first network's interface index. A radio set up again has a new index, and is held again;
  - a power no longer set is let go, `iw phy <phy> set txpower auto`;
  - a wiphy two radios share is left alone and said once in the log, as power control does: power is the wiphy's;
  - **a radio that sends louder than it is held at is held again** ("louder"), whatever the file says: iwinfo's word on its power, half a dB over. Quieter is the most its rules or its radio let it send, and is left.
- **After an apply, the agent has it look again 20 and 60 seconds on** (`aeolus-power -a`): netifd reloads a radio's settings some seconds after the apply, and sets it to its own power as it does, keeping its interfaces, so no hotplug comes.
- **Power control leaves such a radio alone,** as it leaves one with `txpower` set.
- **The manager's render check expects it so:** no `txpower` where a power is set, and the section with each radio's power.
- **What it decides is `aeolus.power`, tested by `TestAgentPower`.**

## Consequences

- **A change of power sets no radio up again, on any AP.** Changing only power changes only the agent's package, which reloads no Wi-Fi.
- **The first apply after this release takes `txpower` out of each AP's Wi-Fi config where Aeolus set one,** which reloads the Wi-Fi once. On 2026-11-10 that is the five house APs. The One may drop once more there, until porting's B176 fix reaches it.
- **A radio starts at its own power, and is held a few seconds later,** once its networks are up. That is at boot and when netifd sets it up again.
- **LuCI shows no power for such a radio.** The power the radio runs at, which the agent reports as before, is the one to read.
- **v0.321.0 held each radio a second before netifd's reload put it back to its own power,** on 2026-11-11 from 00:02 CST: the house ran at full power until its powers were held again by hand at 00:06. Its file said held, and nothing looked at the radio. v0.321.1 looks.
- **Checked by:** the render check's tests and the agent's, and, after release, each house AP's radios running the set power with `/var/run/aeolus/power/` holding it.
