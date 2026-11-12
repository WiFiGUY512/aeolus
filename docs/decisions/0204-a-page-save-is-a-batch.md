# 0204. A page's save is a batch: Follow above, and the Networks tab

- Status: Accepted (Griff, 2026-10-21: "3 yses do this everyewhere")
- Date: 2026-10-21
- Proposed by: Claude, for one save per page; accepted with Griff's word
- Refines: 0202, 0159
- Rests on: 0203

## Context

- **A page's one save (0202) could only carry a `set` on the page's own node.** Three things stayed outside it.
  - **Follow above, Inherit again and Use the default:** each unsets fields, and each applied by itself. The page was then drawn again, and any other drafts on it were lost (0202, Consequences).
  - **A port's probe address cleared:** an unset, so the port's form held the page back and offered "Clear it…" alone.
  - **The Networks tab:**
    - Each network is set on its own Services folder, and a page can show networks from more than one.
    - Taking a fallback away unsets fields while the rest of the form sets others. That was two changes: the fallback first, and the rest "to be made after it".
- **A `batch` (0203) is one change** of sets and unsets on any nodes.

## Decision

- **A page's save sends one change.**
  - **One set:** where every change is a field of the page's node, and nothing is unset.
  - **A batch otherwise:** for each node, a set of its fields and an unset of the ones that go.
  - The preview says each field once: what it was, and what it becomes, from where.
- **Follow above, Inherit again and Use the default are drafts** wherever the page has one save: System, Radios › Bands and Neighbours, Ethernet, Tunnels, Templates and Networks.
  - **Pressing one** marks its fields to be unset with the rest of the page's changes; pressing it again keeps them.
  - **Several fields:** each has a tick box under it, as the preview had them (0159).
  - **A field also changed elsewhere on the page:** the last one done is what is saved.
- **A port's probe address cleared** is unset with the rest of the port's changes.
- **The Networks tab has one Undo and one "Save…".**
  - **Each network's form** is a draft on the Services folder it lives in. Two networks from two folders save as one change.
  - **The steering switches and multicast to unicast** in each network's view are drafts too, and read the same as its form.
  - **Taking the fallback away** goes in the same change as the rest of the form:
    - The preview says Fallback before and after, what becomes of each switching setting, and the lock that goes with it.
    - It warns that the Wi-Fi restarts, and where switching would still be automatic from above, which needs a fallback.
    - Switching set to report here stays: unset, it might be automatic from above.
  - **What was left of a fallback with no type** goes with any other change to the network.
- **What stays a change of its own:** deleting a record (Delete for a tunnel or a network, Remove for a tunnel port's VNI), adding a network, and the records and actions 0202 named.

## Consequences

- **Pressing Follow above no longer loses a page's other drafts.**
- **Delete, Remove, Add a network and the other changes of their own** still apply by themselves, and the page is drawn again without its drafts.
- **A page that unsets, or changes more than one folder,** shows in the log as one batch, its changes joined by "; " (0203).

## As built (2026-10-21)

- **The page's save:** `pageSave()` in `views/pagesave.js`.
  - `UNSET` marks a field to unset. A part may name its own `tree` and `node`.
  - `scope()` is a part of the page drawn again as a whole, such as a band's panel or an Ethernet form, which `drop()` takes off.
  - The preview's `effect` and `resolved`, a list for a batch, give each part what each field was and becomes. `resolvedAt()` in `views/confirm.js` reads `resolved` either way.
- **Follow above:** `followButton()` in `views/follow.js` takes the page's save. It is used by `fieldPanels()`, `inheritsBar()`, Bands, Neighbours, Tunnels, Ethernet's forms and Templates.
- **Networks:** in `views/networks.js`:
  - `switches()` holds a network's steering and multicast drafts;
  - `editForm()` is its form;
  - `deleteNetwork()` and `addForm()` are as they were.
- **Tried in the local harness, each a single change:**
  - **Networks, two Services folders:** Move weak clients on for Residents' network. On Household's, a new SSID and the fallback taken away. One batch, #34: a set on Residents, and a set and an unset on Household. Each network's switches and multicast, and Follow above for the location's service folders, previewed together. Undo put back every switch, the multicast pick and the Follow.
  - **Networks, one folder:** Hold back 2.4 GHz alone was a plain set.
  - **System:** Follow above for syslog with Bluetooth on: one batch, #36.
  - **Bands:** Follow above for 2.4 GHz with one of its two fields unticked, and target wake time on 5 GHz: #38.
  - **Tunnels:** Use the default MTU with a new port: #41.
  - **Ethernet:**
    - A probe address cleared with a new speed: #42.
    - A port form's Follow above went with its Cancel.
    - Inherit again ticked all four fields, and Undo put it back.
  - **Templates:** Follow above for one board and a pick for another: one batch.
- **Live, on Example's manager (v0.149.0 on):** Griff, 2026-10-21: "networks tab with a single save works great now".
