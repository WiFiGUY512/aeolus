# 0286. A setting is changed in its own row

- Status: Accepted (built on Griff's standing word to develop on my own, 2026-10-24; from the mockup he took, 0284)
- Date: 2026-11-02
- Refines: 0284 (the screens redesign, part 5), 0202 and 0204 (one save a page), 0048 (inputs from the schema)

## Context

- **The mockup's Settings board drew each setting as a row you press to change.** The row itself turned into the control, with what the value was, and the page's one save bar said the change.
- **Part 5 of 0284 built the rows but not the editing.** Every value was a line with where it comes from. To change one, you still opened **Edit system settings**: a form of every system field, set or not, above the rows. So each value showed twice, and the one you wanted was somewhere in a long form.

## Decision

- **A value set at or above the node is changed in its row.**
  - Pressing the value puts its input in its place. The input is the same one the form uses, made from the schema's description of the field (0048).
  - How to fill it in, and anything wrong with what is written, go under the row, with **Cancel**.
  - What it sets is a draft of the page's one save (0202), so the bar's preview says it with every other change on the page.
- **Locked values are only shown.** A value locked above the node cannot be changed there, as in the form. A field the schema does not describe is only shown too.
- **The row and the form agree.** A draft set in one shows in the other while both are open, as Bluetooth and the place on the map already did (0202).
- **An open row keeps the page from being drawn again under it**, as an open form does. The bar shows once the row has a change, not while it is only open.
- **The form stays, as More settings….** It is where a field set nowhere yet is set, and the rarer ones (SNMP, the advisor, management addressing) live.

## Consequences

- **Changing one setting is one press and one Save….**
- **It is the system settings, and Assurance's Schedule.** The Settings page of a folder or an AP was first. The Assurance tab followed (2026-11-02):
  - its **Schedule…** form is now a draft of the tab's one save, with no Review of its own, so the form and the Schedule panel's rows save together;
  - a row on a page whose save says nothing of the whole change says its own: the field, what it was and what it becomes.
- **Some rows stay shown only:**
  - **which service folders apply**, on the Networks tab: it is set by its own kind of change, not a field the schema describes;
  - **a Services folder's network fields**: they depend on each other (a fallback taken away, RADIUS settings that go with the security), and the network's form keeps them consistent, so they are changed there.
