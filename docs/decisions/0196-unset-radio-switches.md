# 0196. An unset radio switch is OpenWrt's default again

- Status: Accepted (Griff, 2026-10-21: "put twt and bss color back to defaults")
- Date: 2026-10-21
- Proposed by: Claude
- Refines: 0191, 0192, 0195

## Context

- **What the agent did with an unset switch.** It wrote a radio's on-or-off options only where the field was set:
  - target wake time, `he_twt_responder` (0191);
  - BSS color, `he_bss_color_enabled` and `he_bss_color` (0192);
  - beamforming, the `su_`/`he_`/`eht_` beamformer, beamformee and MU beamformer options (0195).

  An unset field left the AP's option as it was, so a switch turned off and then unset stayed off on the AP. The Radios tab and the schema meanwhile said OpenWrt's default.
- **Found putting this morning's test back.** The IoT device tests set target wake time and BSS color off on Home. Unsetting them would have left every AP there with `he_twt_responder '0'` and `he_bss_color_enabled '0'`.
- **These switches all have a default in OpenWrt:** on, where the radio can. hostapd heeds each only where the radio can, so taking the option away is the default.

## Decision

- **An unset switch takes its option away.**
  - For target wake time, BSS color and the three beamforming switches, the agent deletes the option where the field is unset, so the radio runs OpenWrt's default.
  - For beamforming, that's in the generations Aeolus writes (802.11ac at 5 GHz only, ax and be on every band).
  - A set field writes its value as before.
- **The render check holds an unset switch to no option.**
- **Other radio fields keep their meaning.** "Unset, the AP keeps what it has" still holds for the beacon interval and the rest: those have no on-or-off default to return to.

## Consequences

- **Unsetting these switches brings the default back,** at the cost of one radio restart where an option is taken away.
- **No hand-set option survives.** An option of these set on an AP by hand, with the field unset in Aeolus, is taken away at its next render. No AP had one when this was built: every such option in the lab was Aeolus's own.

## As built (2026-10-21)

- **Agent:** `radios()` in `render.uc` writes each switch through `bit()`, which deletes the option for an unset field. BSS color unset deletes both its options.
- **Render check:** `switchOption()` and `absent()` in `rendercheck.go`.
- **Schema:** BSS color's description says what unset means.
- **Tested:** the render case `agent/test/cases/switches-unset`, a C-360:
  - its radios carry options set before (target wake time and BSS color off, beamformers off), and the fields are unset;
  - the options go, and 6 GHz keeps the color it is set to.
  - It was rendered with real ucode, and the check passes it.
