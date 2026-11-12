# 0345. A change that ends itself

- Status: Accepted
- Date: 2026-11-10
- Proposed by: Claude, from the test windows of 2026-11-09. Griff asked "whats next"; it was the recommendation, built under the standing word (autonomous development). Accepted with the merge
- Refines: 0344 (test window), 0203 (batch), 0320 (a batch makes folders)

## Context

**A test window (0344) ends by itself on the AP, and nowhere else.** What came with it stays until someone takes it away:
- its maintenance fields, sent to the AP after its end, which it is past;
- channel pins set for the test, which hold the radios where they are;
- a services folder of its own, as TestW318's on 2026-11-09 (Home Wi-Fi alone, its other networks switched off in it), and the AP taking it.

**On 2026-11-09 both were left behind.** C360-AP's pins outlived its hold by over two hours (14:59 to 17:13 CST): the one who set them was not there at its end. TestW318's window took a folder made by hand and an undo of five fields and the folder (809, 810).

**An undo by hand is a list kept in someone's head.** A field missed stays; a field someone changed in the meantime is put back over theirs.

## Decision

**A change may carry `until`: at that time the manager ends it.** This works for a set, an unset, add-folder, assign-services, or a batch of them. `until` is a time to come, at most 31 days off.

**What it did is kept as steps, worked out from the Org before and after it:**
- each field it set or unset, once, with what it held before the whole change and what it was left holding;
- each folder it made, the last made first.

A field in a folder it made goes with the folder.

**Its end, `end-change`, is one logged change:**
- **A field still as the change left it** is put back: set to what it held, or unset where it held nothing. The services a location takes are given back with assign-services.
- **A field changed since is left as it is,** and the end says so in its reason. What someone did meanwhile is theirs.
- **A folder it made is taken away** once nothing holds it: after the fields, so a location's services go first. One that holds something now is left, and said.

Each is tried in turn on a copy. What cannot be made is left and said; the rest are made as one.

**The manager ends what is due, in its own name, every 30 seconds.** A manager that was down ends what came due meanwhile once it is back. The ends to come are rebuilt from the log as it replays, so a restart loses none.

**A person may end it sooner, or move its end:**
- `POST /v1/ends/{seq}`: with no `until` it ends now; with `until`, that is its new end.
- A move may set fields of one node with it, as part of the change, so they go at its end too. This is a test window's `maintenance.until`, one hour more.
- Either needs the right to make the change again, and what the end makes.
- `end-change` and `move-end` are made only this way. As changes they are refused: the manager works out what an end puts back.

**`GET /v1/ends` lists what is to end**, the soonest first, with where each puts back, and not the values, which may be secrets. Like the change log, it needs viewer at the Org root. MCP has `list_ends`, `end_change`, and `until` on `make_change`.

**In the UI:**
- **Test window…** makes its change with until, its end.
- **The banner's 1 h more** moves that end, and maintenance.until with it.
- **End now** ends it, and what was set with it.
- A window made before, or one whose end the caller may not read, is moved and ended as before.
- The change log shows "ends itself" on such a change, "end of #N" with what it put back, and "#N now ends".

**Not done here:**
- Until on a lock, move, rename or break. Their undo is not a field put back, and nothing needs one yet.
- The AP's own hold (0344) is unchanged: it still ends by the AP's clock, whether it reaches the manager or not.

## Consequences

- **A test window is one change that ends, with everything it set:** maintenance, pins, a folder of its own. No list is kept by hand, and nothing outlives it because nobody was there.
- **Nothing anyone did meanwhile is undone.** The end says what it left.
- **The ends to come live in the manager's memory, rebuilt from the log.** The log's schema is unchanged: until and ends are fields of the op it already stores.
- **Old managers cannot replay a log with an end in it** (an unknown kind). A manager is not put back past this release.
- **Checked by:**
  - the change log's tests: a window ended at its until puts back exactly the Org before, and stays so through a replay; a field changed since is left; a moved end takes what it set with it; until past, too far, on a lock or on a batch's own change is refused; end-change and move-end as changes are refused;
  - the API's: until on a change, `/v1/ends` listing it without values, moved, ended now by a person, ended twice is 404;
  - the MCP tool list.
- **Not proven yet: an end made by the manager on a real AP's window.** It is tried with the next test window after release.
