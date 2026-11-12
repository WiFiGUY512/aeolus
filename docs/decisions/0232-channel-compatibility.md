# 0232. Channel compatibility

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Griff, from someone's Phone-B Ultra on the Living Room AP: "we should have a rule that can take out that channel on general byod wifi", "push that to encompass the upper channels", and "I want [them] to exist in the channel lineup, but I want a button for channel compatibility that disables them for everything". Written up by Claude
- Refines: 0071 (avoiding DFS), 0075 (channel sets), 0120 (DFS after a look)

## Context

- **U-NII-4 (5850–5895 MHz, channels 169, 173 and 177) is new to Wi-Fi in the US.** Linux's regulatory database lets an AP use it, marked passive-scan: a client may not probe there, and finds a network only by its beacons.
  - Many clients send there at reduced power. Some do not go there at all, as older phones and most IoT devices.
  - Channel 165 beside it is a lone channel that some clients mishandle.
- **Aeolus's 5 GHz lineup stopped at 165,** to ask no radio for a block its region might not allow. A 5 GHz radio with an automatic channel and no channel set was given no list, so hostapd's own pick could take any channel the regulatory database allows.
  - hostapd's pick rates a channel by how busy it is. U-NII-4, which almost no one uses, looks the quietest.
- **On 2026-10-28 the Living Room AP's 5 GHz started on channel 169** when it rebooted after a firmware upgrade. dfs-look kept it off DFS (0120, B96), and U-NII-4 was what was left that looked best.
  - someone's Phone-B Ultra, beside the AP, was heard on its 5 GHz at -72 to -81 dBm, where its 2.4 GHz heard it at -33 to -49 and its 6 GHz at -56 to -61.
  - On the 5 GHz radio it got 80 to 130 Mbit/s, with 30 to 130% of frames sent again. On the AP's 6 GHz it got 1.2 to 1.7 Gbit/s.
  - The channel's noise floor was -88 dBm, 3 to 6 dB above the house's other 5 GHz channels. Over three hours, with almost no one on it, the radio sent its frames again more often than it sent them.
  - The phone moved between the AP's 5 and 6 GHz every 10 to 30 minutes, and at times onto the IoT network. She said the Wi-Fi was bad.

## Decision

- **The upper channels are in the 5 GHz lineup:** 165 to 177. They are in channel sets, in the blocks of a width (165/169 and 173/177 at 40 MHz, 165–177 at 80), and among what radio resource management listens to and moves within where a radio's list has them.
- **Channel compatibility keeps every radio off them:** `radio.5g.compatible`, on unless turned off, set a folder or an AP at a time.
  - **On (the default):**
    - An automatic channel is one of 36–161: a radio with no channel set is given every channel up to 161 in whole blocks of its width, and a channel set is used without its upper channels.
    - A channel set by hand to an upper one is refused, and so is a channel set of upper channels alone.
    - DFS is avoided or allowed as before (0071, 0120).
  - **Off:** the upper channels are channels like any other. A radio with no channel set is given every channel, 36–177.
- **A 5 GHz radio's list is always written,** so hostapd's own pick at its start and radio resource management's moves keep to the same channels.
- **The render check holds the list,** as it holds the other channel lists.
- **The band editor has the switch on 5 GHz,** beside DFS: Channel compatibility.

## Consequences

- **A general BYOD network, where no one knows what clients come, is kept off the upper channels** with nothing to set. A site whose clients do well there turns compatibility off.
- **Every AP whose 5 GHz has an automatic channel has its config change once:** the list is written where there was none, or loses 165. Its 5 GHz radio restarts, and its clients move to another band or AP for a few seconds. A radio that was on an upper channel, as the Living Room AP's was, picks another.
- **A site outside the US that turns compatibility off may list blocks its regulatory domain does not allow.** hostapd's pick skips what the regulatory database forbids. A channel set by hand there is refused by hostapd, as any channel the region does not allow is.
