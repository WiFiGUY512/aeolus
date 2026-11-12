# 0202. One save a page, everywhere it can

- Status: Accepted (Griff, 2026-10-21: "3 yses do this everyewhere")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0194

## Context

- **0194 gave the Radios tab one save,** and left the other pages for Griff to name. He named them all: "do this everywhere".
- **The other pages each saved a part at a time.**
  - **System:** the editor had its own Review; the Bluetooth switch saved the moment it moved; the map's place had its own Review.
  - **Radios › Neighbours:** Turn on… and Turn off… for neighbours and power control, and an editor for each one's policy.
  - **Ethernet:** a form each for spanning tree, the uplink's bond, each port and a port added.
  - **Tunnels:** a form for each tunnel, and one for a tunnel added.
  - **Templates:** each board's pick saved the moment it was picked.
- **Two of the System tab's settings have two places to set them.** Bluetooth is set by the switch and in the editor, and the place by the map and in the editor.

## Decision

- **These pages have one Undo and one "Save…",** at the foot of the page, as Radios does: System on a folder or an AP; Radios › Neighbours; Ethernet; Tunnels; and a folder's Templates.
  - **What it saves:** every change on the page, as one change and one preview. A note two parts give, such as a network reload, is said once.
  - **When it shows:** while a form is open, or something has changed. On Radios, while customizing.
  - **When it's held back:** while nothing has changed, or while a form's draft cannot be saved as it is. Examples: an access port with no untagged VLAN, a new tunnel with no far end, an address with nowhere on the map.
  - **What Undo does:** it puts each switch and pick back, and closes the forms.
- **Forms are drafts.** They have Cancel, which drops that form's changes, and no Review. On Ethernet a form keeps its changes while another port is shown, and a port with changes opens on its form.
- **Switches and picks are drafts too:** the Bluetooth switch, neighbours and power control on or off, and each board's template. Power control's switch can be turned on once neighbours are on, or are about to be.
- **A setting with two places to set it reads the same in both.** The last one changed is what is saved, and the preview says it once. The same goes for spanning tree: taking the bond apart turns it on in its own form too. Turned off again while the bond is apart, the save is held back.
- **What stays separate:**
  - **Undoing a setting** (Follow above, Inherit again, Use the default, Delete, Remove) is still its own change. A set cannot carry an unset.
  - **Clearing a port's probe address** is an unset too. The port's form holds the save back and offers "Clear it…", once nothing else on the page has changed.
  - **Records and actions** keep their own buttons: networks, per-user keys, clients and their tests, captures, the test network, updates, the Admin page, renaming, moving and removing, and a template's own page.
  - **Making templates and importing them** stay as they were.

## Consequences

- **A page's changes are one entry in the log,** and one new config version for each AP.
- **A separate button, such as Follow above, used while the page has changes not saved,** applies by itself, and the page is drawn again without those changes. The page does not warn of this yet.
- **The Assurance tab's schedule editor** still has its own Review. It is one form on a page of results.

## As built (2026-10-21)

- **The page's save:** `pageSave()` in `views/pagesave.js`. Each part registers its changes, summary, notes, undo, whether it is held back, and whether it is an open form. A part tells the page when its draft changes, and what it set. Any other part that shows that setting takes it. The last part to set a setting owns it in the preview.
- **System:** `systemSection()` in `views/sections.js` makes the save. The editor in `views/system.js` takes `{ save }`, and the map's place in `views/placemap.js` is a part. A latitude in the editor reads as a number now (`views/edit.js`).
- **Neighbours:** `onOff()` in `views/neighbours.js`, a part for each switch and its policy.
- **Ethernet and Tunnels:** `views/interfaces.js`. The kind panel's forms are kept by what they edit until saved, cancelled or undone.
- **Templates:** `pickControl()` in `views/templates.js`.
- **Radios:** `views/bands.js` uses `pageSave()` too.
- **Tried in the local harness, each a single change in the log:**
  - **System on Home:** Bluetooth on, syslog, and a latitude and longitude typed in the editor that showed on the map; #31, four values. The Bluetooth switch and the editor's box followed each other both ways. An address with no place held the save back.
  - **Neighbours:** neighbours on, power control on and the margin for a move; #32.
  - **Ethernet:** spanning tree on and lan1 an access port on VLAN 20, for every R7800; #33. An access port with no VLAN held the save back. A draft was kept while another port was shown, and Undo closed every form. Taking a bond apart turned spanning tree on in its form. Turning spanning tree off again held the save back. Clearing a probe address offered "Clear it…", which previewed the unset by itself.
  - **Tunnels:** core's MTU and a new tunnel, edge; #35.
  - **Templates:** two boards' templates; #38.
  - **Radios:** both bands' target wake time; previewed and undone as before.
