# 0328. DFS channels: avoid or allow

- Status: Accepted (Griff, 2026-11-06: "yes build the DFS Allow/Avoid")
- Date: 2026-11-06
- Proposed by: Griff, of the three choices on a band's page: "I think look first should be a default behavior for allow, so the option should be allow or avoid. educate me if I am incorrect" (2026-11-05). Written up by Claude
- Refines: 0071 (avoiding DFS), 0120 (DFS after a look)
- Refined by: 0329, where a radio set to 160 MHz starts at 80 MHz and is widened

## Context

- **`radio.5g.dfs` had three choices:** `avoid` (0071), `look-first` (0120) and `allow`, the default.
- **`allow` let hostapd pick the channel at the radio's start,** from a few seconds' listening, and it could pick a DFS one. The radio then sat silent a minute for its radar check at every start. Once on a DFS channel it could not scan, so radio resource management never learned of a better channel (0120's Context).
- **`look-first` is better almost everywhere:** the radio starts outside DFS and serves at once, looks at every channel, and RRM moves it to a DFS block only where that is clearly the best. Every Home AP has run it since 2026-11-01 (change 601).
- **Plain `allow` earns its keep only where look-first cannot work:**
  - **RRM moves no radios** (off, or `rrm.moves` false). Nothing would make the move, so look-first was `avoid` in all but name.
  - **160 MHz on 5 GHz.** Every 160 MHz block, 36–64 and 100–128, holds DFS channels, so the radio has nowhere outside them to start. The config check refused look-first there.
  - **A set of channels (0075) with no whole block outside DFS at the width,** which the config check refused too.
- **0120 left it open** ("Making `allow` mean this wherever RRM moves radios").

## Decision

- **Two choices, Avoid and Allow.** `avoid` is as before (0071).
- **`allow`, the default, as unset, looks first where it can, and picks where it cannot:**
  - **It looks first** (0120: `acs_exclude_dfs` at the start, and RRM's `dfs '1'`) where all of these hold:
    - the channel is automatic;
    - RRM is on and moves radios;
    - a whole block outside DFS at the radio's width is among its channels, the primaries it may use, or among all of 5 GHz's where it has no set.
  - **Otherwise hostapd picks at the start, DFS or not,** as `allow` did: no `acs_exclude_dfs`, and RRM does not move the radio to a DFS channel.
  - **A channel set by hand is used as set,** as before.
- **`look-first` stays a value the schema takes, and means `allow`.** What is stored keeps working, and an old agent handed it does what it did; the page shows it as Allow. No change is written to rename it.
- **Nothing is refused for having nowhere outside DFS to start.** 0120's two refusals, at 160 MHz and of a set with no block outside DFS, are gone: such a radio picks. The band's page offers 160 MHz under Allow again.
- **One rule, in two places that must agree:**
  - the agent's renderer, `dfs_way` in `render.uc`;
  - `internal/radio`'s `DFSWay`, which the render check uses.
  - Both read the radio as rendered: its channel, its channels and its width from its htmode.
- **The page:** Avoid or Allow, Allow's tip saying it starts off them where RRM moves radios and may start on one where RRM does not, or at 160 MHz. The confirmation says a move to a DFS channel costs about a minute off the air.
- **The `cannot-scan` alert** for a radio long on a DFS channel hostapd started it on now says when Allow looks first, in place of "set look-first".

## Consequences

- **Where RRM moves radios and the 5 GHz is automatic at 80 MHz or less, `allow` or unset now starts outside DFS.** On such an AP the agent update adds `acs_exclude_dfs` and RRM's `dfs`: the 5 GHz radio restarts once. It then stops doing a radar check at every start, and RRM sees every channel.
- **Home is unchanged:** every AP there has `look-first`, which renders as before. Checked on all eight APs before the release: `acs_exclude_dfs '1'` and RRM's `dfs '1'` on each.
- **A site with RRM off, or moving no radios, is as before:** `allow` picks at the start.
- **At 160 MHz, Allow lets the radio pick a DFS block,** where look-first refused the width. A radio there that wants to keep off DFS sets Avoid and a narrower width.
- **Tests:**
  - the renderer's cases `dfs-allow` (unset, RRM on, 80 MHz: renders as `dfs-look-first`) and `dfs-allow-160` (picks: no exclusion, channels 36 and 100, no RRM `dfs`);
  - `TestDFSWay`;
  - the render check's DFS and RRM tests, for allow, unset, look-first, RRM not moving, 160 MHz, and a set with only DFS channels;
  - the config check's `TestAllowNeedsNowhereToStart`;
  - the hardware view's `TestAllowOffers160`.
