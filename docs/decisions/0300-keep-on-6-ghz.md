# 0300. Keep on 6 GHz

- Status: Accepted (Griff, 2026-11-03, about 20:40 CST: "Keep-on-6 GHz switch"; and "We need to be super careful about the 2.4 only clients. We do not want to misidentify and strand them"). Revised 2026-11-03, about 23:45 CST: kept beforehand, as below, since the first way never kept anyone
- Date: 2026-11-03
- Proposed by: Claude, with porting's band-preference hysteresis. Written up by Claude
- Refines: 0050 and 0138 (band steering), 0299 (community sharing)

## Context

- **Suggest moved a phone up to 6 GHz, and the phone moved itself back to 5 GHz:** after 27 s at the Garage and 110 s at the Living Room (2026-11-03).
- **Indoor rules let 6 GHz far less power than 5 GHz,** and porting checked that the C-360s are right:
  - 6 GHz low-power indoor is 5 dBm/MHz, 24 dBm EIRP at 80 MHz; iw shows 16 dBm conducted.
  - 5 GHz runs 23–25 dBm conducted.
  - So a phone hears 6 GHz about 10 dB weaker, and its own roaming picks the stronger 5 GHz.
- **Turning 5 GHz down would cost every device that has only 5 GHz.**
- **The first way, a ban sent when hostapd says the client is asking, never kept the Phone-B once** (20:47 to 23:22 CST, four moves back to 5 GHz at the Living Room):
  - It moves by Fast Transition over the air (`auth_alg=ft`): a probe, the FT authentication and the reassociation on 5 GHz all within the same second. The daemon is told of each without hostapd waiting for it, so its ban reaches hostapd after the move is made.
  - The probe set the client as kept for five seconds, so the authentication that followed was let through with nothing said.
  - hostapd checks its ban list before it tells any listener of a probe, authentication or association (OpenWrt's hostapd `ubus.c`, `hostapd_ubus_handle_event`). A ban already in place turns the frame away at once, with no notification.

## Decision

- **`network.<id>.steering.keep_6g`, a band-steering switch, "Keep on 6 GHz", off unless set.**
- **A client on the network's 6 GHz on this AP, heard there at -72 dBm or better, is kept off the network's lower bands on this AP beforehand.**
  - hostapd's own ban (`del_client` with `ban_time`) is set on each lower band's BSS, 15 s long, and renewed each look (every 10 s) while the client is so.
  - hostapd then turns away its probes and its tries to authenticate there, Fast Transition's included, and the client stays on 6 GHz.
  - Every other AP is open to it.
- **Only a client on 6 GHz here at that moment is ever kept.** A device that cannot use 6 GHz is never on 6 GHz, so it is never kept, whatever it or another AP said of it. An AP's own test client is never kept.
- **It is let go, the bans lifted at once (`ban_time` 0):**
  - as soon as hostapd says it has left that 6 GHz (disassociation or deauthentication), or a look no longer finds it there;
  - after two looks in a row hearing it under -72;
  - when the switch is turned off.
  - No band the client is on is ever asked anything: a lift sent there would put it off.
- **With the daemon stopped or hung, hostapd lifts the bans by its own timer, within 15 s.** It fails open, as all steering does.
- **The cap of three refused tries in ten minutes is gone.** hostapd does not say when it turns a banned client away, so tries cannot be counted. The -72 dBm floor, held two looks, is what lets go a client whose 6 GHz link is going.
- **Each keeping is one steering record of its own way, `keep`, made when it ends:** at when it began, from and to 6 GHz, held, and `stayed` (let go still on 6 GHz here: heard weakly, or the switch off) or `left`, with how long it was kept in `took`.
- **With it:** a cross-AP suggest asking (0299) names the AP it sent the client to only where RRM knows it. The manager takes an elsewhere on a suggest asking, where before only sticky's was taken.

## Consequences

- **A phone moved up to 6 GHz stays there while its 6 GHz link is good,** instead of drifting back to the louder 5 GHz.
- **2.4 GHz-only and 5 GHz-only devices are untouched.** The tests show it: a client on 2.4 GHz alone, one on 5 GHz, one on 6 GHz heard at -75, an AP's own test client on 6 GHz, and one on a network without the switch are none of them kept.
- **A kept client gets no probe answer from this AP's lower bands.** It still hears their beacons, and the bans are on this AP only.
- **A kept client that roams to another AP by Fast Transition may stay barred from this AP's lower bands until this AP's hostapd lets it go** (as its frames stop being answered, or at its inactivity timer). This AP's 6 GHz stays open to it.
- **hostapd's `del_client` sends a disassociation from the lower band's BSS each time a ban is set or lifted.** The client is on another channel and not with that BSS, so nothing comes of it. Checked on the Living Room C-360 on 2026-11-03: a 3 s ban on 2.4 GHz with the Phone-B on 5 GHz left it on 5 GHz, and the ban was gone after 3 s.
- **Live Trace leaves those disassociations out, and says the keeping once** (Griff, 2026-11-06: "yes build the trace label"). Before, each renewal showed as two red "Left" lines a band, every 10 s, as if the client kept being put off.
  - The steering daemon notes each ban in `/var/run/aeolus/kept-off/steer.json` (`aeolus.sentoff`'s `keep_off`): by client and network, since when, and until when.
  - aeolus-journey leaves out a traced client's disassociation sent on a network where that note keeps it off, and the radio's word that it was not acknowledged. A note holds 5 s past its end, to cover the frames of a lift.
  - The trace gets one steering line as the keeping begins, or as the client's trace starts while it is kept: "kept off 5 GHz and 2.4 GHz here while it is on 6 GHz". It gets another as the keeping is lifted.
- **Nothing restarts when it is turned on or off.** Only the steering daemon's section changes (`list keep_6g`).
