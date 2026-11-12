# 0192. BSS color, a radio at a time

- Status: Accepted (Griff, 2026-10-21: "also we probably need to do that same thing with bss coloring")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0089, 0191

## Context

- **BSS color (802.11ax)** is a number, 1 to 63, a radio puts on its frames. A client and the radio tell their own frames from another AP's on the same channel by it, and may talk over a far one rather than wait for it (spatial reuse).
- **It is the radio's**, as target wake time is (0191). hostapd has `he_bss_color` in the radio's own configuration. OpenWrt has `he_bss_color`, which defaults to 128, and `he_bss_color_enabled`, which defaults to on. hostapd picks a color at random at each start for any value above 63.
- **So today every AP picks its own at random.** Two on one channel can pick the same one. hostapd moves off a color it sees taken where the driver tells it so.

## Decision

- **A radio setting, `radio.<band>.bss_color`,** set on a folder or an AP:
  - `auto`: one at random at each start, as OpenWrt does;
  - a color, 1 to 63: that one always, so APs on one channel can be kept apart by hand;
  - `off`: none, and with it no spatial reuse;
  - unset: the AP keeps what it has.
- **The agent renders it** as `he_bss_color_enabled` and `he_bss_color` (128 for `auto`), and the render check holds both to the intent.
- **The Radios tab has it** under target wake time, shown where the band allows 802.11ax.

## Consequences

- **A client that fares badly with color, or with spatial reuse, can be given a radio without it.** So can a site where colors clash.
- **Off costs weak 802.11ax clients their uplink, until OpenWrt's fix** (B114, openwrt-faults 55). wifi-scripts leaves out the spatial reuse element with color, so a client below about -62 dBm ignores the AP's answers. Found 2026-10-30, after color had been off at Home for nine days.
- **Colors are set by hand.** Picking different ones for APs that share a channel could later be radio resource management's (0073).

## As built (2026-10-21)

- **Schema:** `bss_color` in `radio2g`, `radio5g` and `radio6g`: `auto`, `off`, or 1 to 63.
- **Agent:** `radios()` in `render.uc`.
- **Render check:** `radio.*.bss_color`.
- **UI:** `bands.js`; `format.js` says "random at each start", "off" or "color N".
- **Tested:** the render case `agent/test/cases/bss-color`, a C-360 with it off on 2.4 GHz, color 7 at 5 GHz and at random at 6 GHz. Only those options differ from the plain C-360 case.
