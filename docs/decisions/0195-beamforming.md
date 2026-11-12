# 0195. Beamforming and MU-MIMO, a radio at a time

- Status: Accepted (Griff, 2026-10-21: "should we also look into other feature, on/off like mu-mimo, (direction as well) ofdma (bi directional) beamforming ?")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0191, 0192

## Context

- **Griff asked for on and off switches** for MU-MIMO (each direction), OFDMA (each direction) and beamforming. That way, a client that fares badly with one can be tried without it, as TWT (0191) and BSS color (0192) can be.
- **What OpenWrt can switch.** It has these as a radio's options. Each is on by default and heeded only where the radio can do it (`hostapd.uc` ANDs each with the radio's capabilities):
  - **Beamformer:** the AP steers what it sends toward one client, from that client's report of the channel. The options are `su_beamformer` for 802.11ac, `he_su_beamformer` for ax and `eht_su_beamformer` for be.
  - **Beamformee:** a client steers what it sends toward the AP. The options are `su_beamformee`, `he_su_beamformee` and `eht_su_beamformee`.
  - **Downlink MU-MIMO:** the AP sends to several clients at once, a stream steered to each. It is the multi-user beamformer, `mu_beamformer`, `he_mu_beamformer` and `eht_mu_beamformer`. 802.11ac and ax have it only alongside the single-user beamformer.
- **What it cannot switch:**
  - **Uplink MU-MIMO and OFDMA in either direction** have no option in OpenWrt or hostapd. The driver and its firmware do them as they will, so there is nothing for Aeolus to turn.
  - `mu_beamformee`, the AP as a multi-user beamformee, means nothing for an AP.
- **802.11ac exists at 5 GHz only.** At 2.4 and 6 GHz only the ax and be options apply.

## Decision

- **Three radio settings, a band at a time,** set on a folder or an AP like the rest of a radio's settings:
  - `radio.<band>.beamformer`, `radio.<band>.beamformee` and `radio.<band>.mu_mimo`, each on or off.
  - Unset, OpenWrt's default: on where the radio can.
- **One switch is one function in every generation:**
  - The agent writes the option for each generation the band has: 802.11ac at 5 GHz only, ax and be on every band.
  - With `beamformer` off, MU-MIMO is off too, whatever `mu_mimo` says, since it rides on the beamformer.
  - The render check holds all of them to the intent.
- **The Radios tab has a Beamforming row** with three switches: "To clients" (beamformer), "From clients" (beamformee) and "MU-MIMO".
  - MU-MIMO greys out when "To clients" is off.
  - The row shows only where the band allows 802.11ac, ax or be.
- **Changing them restarts the radio,** as any radio setting does, and the save preview says so.

## Consequences

- **A client that fares badly with beamforming or MU-MIMO can be tried without them,** on the radios that client uses, for every network on them.
- **OFDMA and uplink MU-MIMO stay as the driver has them.** If a client needs them off, that is a driver or firmware matter (the porting session's), not a setting.

## As built (2026-10-21)

- **Schema:** `beamformer`, `beamformee` and `mu_mimo` in `radio2g`, `radio5g` and `radio6g`.
- **Agent:** `radios()` in `render.uc` writes the options for the band's generations.
- **Render check:** each of the three is checked against its options. MU-MIMO is held off when the beamformer is off.
- **UI:** `bands.js` has the row; `format.js` labels the three.
- **Tested:** the render case `agent/test/cases/beamforming`, a C-360:
  - at 2.4 GHz the beamformer off, which takes MU-MIMO with it, and the beamformee on;
  - at 5 GHz MU-MIMO off alone, in 802.11ac as well as ax and be;
  - at 6 GHz the beamformee off.
