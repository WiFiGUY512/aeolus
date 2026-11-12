# 0333. Landing Zone stands apart, and there is no Sandbox

- Status: Accepted (Griff, 2026-11-06: "lets get rid of Sandbox, it doesnt really serve a purpose anymore, also I am considering moving Landing Zone outside of the heirarchy altogether. it seems like APs should land there, but be subject to no folder config until they are moved into one")
- Date: 2026-11-06
- Proposed by: Griff, looking at the service-folder canvas (0320). Written up by Claude, in the distro session (0316)
- Refines: 0032 (Landing Zone and Sandbox), 0036 (the manager as an actor), 0125 (removing a folder)

## Context

- **0032 built three folders into every Org:** Landing Zone in Locations, and a Sandbox in each tree for trying configs.
- **Nobody tries configs in Sandbox.** A test network (0179) or a folder of its own does that job. On the lab's manager, both Sandboxes were renamed and put to work long ago: Locations' is **Home**, with every lab AP in it, and Services' is **Resident Wi-Fi**. A newcomer's manager starts with two folders they didn't ask for.
- **Landing Zone already gets nothing.** It is isolated (0032): nothing is set in it, nothing inherits into it, and an AP in it resolves to no config. It keeps running what it has, and the manager takes no checks or reports from it.
- **But it is drawn inside the hierarchy,** as a folder under the Org beside the real ones, with the Org in its breadcrumbs. That says the opposite of what it is.
- **The manager's state is its change log replayed** (0009). Every log so far has an `add-builtins` entry that made both Sandboxes, and later entries that set things in them. Those entries must still replay.

## Decision

### No Sandbox

- **A new Org has Landing Zone alone.** The manager makes no Sandbox in either tree.
- **An Org that has a Sandbox keeps it, as an ordinary folder,** with its ID (`sandbox`), its name, whatever is in it and whatever is set there. Nothing is renamed, moved or removed. It can now be removed like any folder that holds nothing and that nothing leans on (0125).
- **`add-builtins` names its set,** in a new field, `builtins`: an index into a table of sets, oldest first.
  - Set 0 is 0032's: Landing Zone and both Sandboxes. An entry that names no set, as every entry before this one does, is set 0, so it replays as it was made.
  - Set 1 is this decision's: Landing Zone alone. The manager commits today's set at startup.
  - A later change to the built-in folders is one more row in the table.
  - A set there is none of is refused. An older set is still taken: it makes no more than an Org admin could.

### Landing Zone stands apart

- **It is drawn above the Locations tree, apart from it,** with the APs waiting in it under it. A line separates it from the tree.
- **Its breadcrumbs start at Landing Zone,** for it and for an AP waiting in it. The Org isn't in them.
- **Its page says what it is:** "Outside the hierarchy · APs wait here, with no config, until one is adopted into a folder". It has no Overrides menu, since nothing is set there.
- **Underneath, it stays where it was:** an isolated node below the Org's root.
  - It still gives no config and takes no reports.
  - The roles held at the Org still reach it, which is who should see and adopt what arrives.
  - The log keeps every enrollment, adoption and replacement it already holds (0033, 0223).
  - The API and the MCP tools list it as before, `isolated` and with its parent.

## Consequences

- **A newcomer's Locations tree starts with their Org and nothing else in it,** and Landing Zone above it, empty until an AP joins. Their Services tree starts empty. The first network goes in a folder they make (0320).
- **The lab's Home and Resident Wi-Fi go on as they are.** Their IDs stay `sandbox`, which the main session's changes address.
- **An unused Sandbox on an older manager stays until someone removes it.** Removing it is one change, Remove folder, from its page.
- **Moving Landing Zone out of the Org's tree in the model** would change replay, access and the API for no gain anyone can see. If one shows, the UI is already drawn as if it were.

## Checked

- **The change package:**
  - `add-builtins` with today's set makes Landing Zone alone, isolated at the Org.
  - An entry from before (`{"kind":"add-builtins"}`) still makes both Sandboxes, and a later rename in one replays.
  - Today's set after an old one finds nothing to make.
  - A set there is none of is refused.
  - An old Sandbox, empty, is removed like any folder.
- **The manager's startup test:** a new Org gets Landing Zone and no Sandbox in either tree.
- **In the UI, on a dev manager:**
  - Landing Zone sits above the Locations title, with a waiting AP under it.
  - Its page and the AP's breadcrumbs start at Landing Zone.
  - The tree beneath holds the Org and its folders alone.
