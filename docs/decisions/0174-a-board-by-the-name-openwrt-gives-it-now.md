# 0174. A board by the name OpenWrt gives it now

- Status: Proposed
- Date: 2026-10-17
- Proposed by: Griff (2026-10-17): "yes prepare the Aeolus board name change". Written up by Claude
- Refines: 0085, 0092

## Context

- **Aeolus keeps by board** what is for one kind of AP: a template's boards and the `templates.<board>` that picks one (0085), and that kind's own settings, `boards.<board>.` (0092). The board is what the AP reported when it enrolled (0033), as OpenWrt names it.
- **OpenWrt's review renamed the Arista boards** before accepting them: `arista,c360` is `arista,c-360` in openwrt/openwrt#25941, `arista,c260` is `arista,c-260` in #25937, `arista,w318` is `arista,w-318` in #25947, after their models' names. The lab's APs run images built before the rename and report the old names; images built from what OpenWrt merges will report the new.
- **An AP that changed its board name would lose its kind's settings.** Nothing set under the old name is for the new: its template is no longer picked for it, and the manager makes an empty template for the "new" kind and picks it at the Org (0085), so the AP drops what its template set. The two C-360s, the C-260 and the W-318 are to move to the new names together.

## Decision

- **The manager knows the boards OpenWrt renamed** (`library.Board`): `arista,c360`, `arista,c260` and `arista,w318`, each by its new name. It is a short list in code, grown when OpenWrt renames another board.
- **An AP's board is the name OpenWrt gives it now.** One that reports `arista,c360` and one that reports `arista,c-360` are the same kind of AP: the same template, picked by the same `templates.arista,c-360`, and the same `boards.arista,c-360.` settings. The AP's page still shows what it reported, and the name it goes by beside it where that is another.
- **A change is applied by the new names, whichever it says.** A template's boards, `templates.<board>` and `boards.<board>.` fields under an old name land under the new. The log keeps each change as it was said; replaying it puts what was set under old names under the new ones, so a manager's state moves to the new names at the release that brings this, with no migration and no change for anyone to make. Where one change sets a field under both names, the new name's value is the one kept.
- **No AP is re-versioned by it.** The fields that pick templates and hold a kind's own settings are the manager's, not sent to APs as they are, and what they resolve to does not change.

## Consequences

- The lab's templates `arista-c360` and `arista-c260` are for `arista,c-360` and `arista,c-260`, and picked by those names, from the release on; the APs keep taking them before the firmware rename and after it. Their IDs stay: an ID is a name, not a board.
- The ready-made template for the C-360 (0090) names the board `arista,c-360`; imported into a manager with C-360s that still report `arista,c360`, it is for them too.
- A board OpenWrt renames later is one line in the list, and the same holds for it.

- **The release goes in before any AP reports a new name.** A log written before it, where both names were set in separate changes, replays them folded: two picks become one, and an unset of the second fails on replay. The lab's log has none: replayed with this release (305 changes), every AP keeps its config version and fingerprint, and only the template boards and the two picks move to the new names.
- **Each AP is re-versioned once when its firmware reports the new name**, as for any change to what it reported (0122): its config is the same.

## Not done

- **A board that is renamed back, or split in two,** is not handled: the list maps one old name to one new. A board renamed again follows the chain to its newest name.
