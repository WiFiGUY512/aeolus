# 0191. Target wake time, a radio at a time

- Status: Accepted (Griff, 2026-10-21: "we need a twt on/off addition to the ssid anyway, please add it")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0089, 0097

## Context

- **Target wake time (TWT, 802.11ax)** lets a client agree with the AP when it will wake next, so a phone or an IoT device can sleep longer between the frames it is sent. The AP answers such a request where it is a TWT responder.
- **Griff asked for it on and off for an SSID.** In hostapd it is the radio's (`he_twt_responder`, in the radio's own configuration, not a BSS's), and OpenWrt has it as a radio's option (`wireless.<radio>.he_twt_responder`):
  - it is on by default;
  - OpenWrt turns it off where the radio cannot answer.

  Every network on a radio has it alike. It cannot be one way for Home Wi-Fi and another for Home_Wi-Fi_IoT on the same radio.
- **On the lab's APs:**
  - The C-360s' Qualcomm radios (ath11k) answer TWT.
  - The office AP's MediaTek radio (an OpenWrt One, mt76) cannot. OpenWrt leaves it off there.

  It is one of the few differences between the C-360's 2.4 GHz and the office AP's, where the IoT devices loop on one and not the other (0190).

## Decision

- **A radio setting, `radio.<band>.twt`, on or off,** a band at a time, set on a folder or an AP like the rest of a radio's settings. Unset, OpenWrt's default: on where the radio can answer.
- **The agent renders it** as the radio's `he_twt_responder`, and the render check holds it to the intent.
- **The Radios tab has it** under the beacon interval, as "Target wake time", saying it is the radio's, for every network on it. The row shows only where the band allows 802.11ax: a radio kept to 802.11n or older answers no TWT request either way.
- **Changing it restarts the radio,** as any radio setting does, and the save preview says so.

## Consequences

- **TWT can be turned off where a client fares badly with it,** on the radios that client uses, for every network on them.
- **Per network, it waits for hostapd to have it per BSS.**

## As built (2026-10-21)

- **Schema:** `twt` in `radio2g`, `radio5g` and `radio6g`.
- **Agent:** `radios()` in `render.uc` sets `he_twt_responder`.
- **Render check:** `radio.*.twt` is checked against `he_twt_responder`.
- **UI:** `bands.js` has the row; `format.js` labels it.
- **Tested:**
  - **The render case `agent/test/cases/twt`:** a C-360 with it off on 2.4 GHz and on at 5 GHz. Only those two options differ from the plain C-360 case.
  - **The local harness:** set off on 2.4 GHz, its preview, applied, and read back.
