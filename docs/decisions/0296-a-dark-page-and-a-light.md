# 0296. A dark page, and a light

- Status: Proposed
- Date: 2026-11-03
- Proposed by: Griff: "lets explore a dark and light theme" (2026-11-03, about 14:35 CST). Written up by Claude, to be looked at before it is merged
- Refines: 0042, 0284, 0294, 0295

## Context

- **The page was light only.** Its colours were tokens on `:root` (0042), but about a hundred were still written as colours of their own: control borders, hover tints, the text on the accent colour, the inks of the bad and done banners, the channel blocks, the map's ground.
- **The header is dark,** drawn in the page's ink with its background as text, so it would turn light on a dark page.
- **The charts read the page's tokens as they draw** (0294, 0295), so a theme that changes the tokens changes them too, once they are drawn again.

## Decision

- **Three choices in the header, beside My account: Auto, Light and Dark.** Auto follows the device, and turns as it does.
  - The choice is kept in the browser (`aeolus.theme`), so each person's device keeps its own. Where the browser keeps nothing, it lasts while the page is open.
- **`theme.js` sets `data-theme` on `<html>` before the page is drawn,** so a dark page never shows light first. It is a script of its own, not a module, since a module runs only once the page is drawn, and it is the manager's own, so the CSP is unchanged.
- **One dark block, `:root[data-theme="dark"]`,** redefines every token:
  - Surfaces step down: `--bg` #111416, `--panel` #1A1E22.
  - Inks step up.
  - The accent lightens to #5BB0D0, with dark text on it (`--on-accent`).
  - Status colours are lighter, with dark tints behind them.
  - The chart series are the reference palette's dark column, checked for colour blindness on a dark surface; the one-hue scale runs dark to light.
- **What was written in a colour of its own is a token now:**
  - `--line-strong`, `--on-accent`, `--hover`, `--hover-2`, `--branch-strong`;
  - `--bad-ink`, `--bad-line`, `--ok-ink`, `--ok-line`;
  - `--chblock-a`, `--chblock-b`, `--map-bg`, `--hatch`, `--hatch-on`, `--accent-faint`;
  - and the header's own: `--head`, `--head-ink`, `--head-on`, `--head-on-ink`.
- **What stays as written, since it reads on either page:**
  - the header's own greys;
  - the UCI view, which is dark on both;
  - mid-tone status marks: port jacks, map pins, the coverage map's greens and ambers, the rating blocks' tints.
- **A page redraws when the theme is changed, or when the device turns while on Auto,** so its charts take the new colours.
- **The Usage chart's "up" is a colour of its own (`--accent-faint`), not the accent made faint:** Plotly's key ignores a bar's opacity, so up and down had the same swatch.

## Consequences

- **The map's tiles stay light on a dark page.** They are the map's own pictures.
- **A colour added later is a token,** with its dark value beside it, or it shows on a dark page as it was written.
- **Not yet looked at in dark:**
  - the terminal window, which is dark already;
  - LuCI's pages on the AP, which are LuCI's.
- **Seen in a browser against the UI harness:**
  - a folder's Overview, its charts and Traffic, and an AP's Radios (the channel blocks), in dark and light;
  - a reload in dark draws dark from the first paint;
  - the switch redraws the charts in the new colours.
