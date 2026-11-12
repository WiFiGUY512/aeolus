# 0194. One save a page, and no note

- Status: Accepted (Griff, 2026-10-21: "we need to get rid of the reason for a change AND we need to make it all a single page save, for instance not a save per radio on the radio page")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0042, 0062

## Context

- **The Radios tab had a Save and an Undo for each band.** A change to 2.4 and 5 GHz together was two changes, two previews and two Applys.
- **Every Apply offered a "Note (optional)" field (0062),** and so did the Admin page's account changes. The log already records who made each change and when.

## Decision

- **The Radios tab has one Undo and one "Save…" for the page,** kept in view at its foot.
  - **What it saves:** every band's draft, as one change and one preview. The preview lists each band's changes, then each band's notes, such as which radios restart.
  - **When it's held back:** while no band has a change, or while a changed band's draft cannot be saved (a channel not picked, nowhere to go, protocols no client could join by).
  - **What stays per band:** each band panel keeps its own "Follow above" button.
- **No change asks for a note.** The Apply step and the Admin page's account changes have no note field. Changes are logged under who made them and when.
  - Notes made before still show in the change log.
  - The API still takes a `reason`, so a tool can explain itself.

## Consequences

- **A radio's settings across bands are one entry in the log,** and one new config version for each AP.
- **Other pages open one editor at a time,** each with its own "Review changes". Griff is to say which of those should become one save a page.

## As built (2026-10-21)

- **The page save:** `bandsSection()` in `views/bands.js` keeps each band's part. A band's `bandPanel()` registers its changes, summary, notes, undo and whether it is blocked, and tells the page when its draft changes.
- **No notes:** `views/confirm.js` and `views/admin.js`.
- **Style:** `.pagesave` in `app.css`.
- **Tried in the local harness:** one Save and one Undo on the page; target wake time off on 2.4 and 5 GHz; one preview with both bands and both restart warnings; applied as one change, #31, `set` with both values, no reason.
