# 0320. A page for a location's service folders

- Status: Accepted (Griff, 2026-11-05: "Build it as drawn", on the mockup canvas https://claude.ai/artifact/35DLAdZAB3ioVnekjMAHFe)
- Date: 2026-11-05
- Proposed by: the Aeolus distro session (0316), writing a newcomer's guide: a fresh Org's first network could not be made by clicks. Mocked up and written up by Claude
- Refines: 0013 (networks live in service folders), 0047 (the Networks tab), 0203 (a batch is one change), 0204 (a page's one save), 0286 (settings in their rows)

## Context

- **A location offers the networks of the service folders it takes** (0013), a list set with `assign-services`, inherited like any field.
- **No page set that list.** The Networks tab showed it read-only. **Add a network** showed only where a services folder already applied. The README listed it as an open question: "UI for assigning services to locations (mockup first)".
- **So a newcomer's first network needed the API.** On a fresh Org nothing applies anywhere, and the getting-started guide (0316) had to give a token and a `curl` call for that one step.
- **A batch held sets and unsets alone** (0203), so a new folder, a location taking it and a network in it would have been three changes.

## Decision

### On a location's Networks tab

- **"Services offered here" has the Service folders row, which changes in its row** (0286). It is a draft of the tab's one save (0204).
  - **As <the folder above>** takes what the folder above takes. It is the location's own list unset.
  - **Set here** is a tick a services folder, in the tree's order, each with the SSIDs it offers, what it inherits included. It is `assign-services` with the list.
- **What comes and what goes is said as it is picked,** in chips under the list, and a network that goes is dimmed on its card.
- **Two ticked folders that offer the same network clash** (`hierarchy` refuses it on an AP), and the save is held back until one is unticked.
- **Where no services folder applies,** someone who may change the location still has **Add a network**.
  - Its "In" is a new services folder, named "<the Org> networks" unless renamed.
  - Review shows one change in three steps: the folder, made at the Services root; the location taking it beside what it takes; and the network in it.

### On a Services folder's page

- **Applies at** lists each location folder that sets a list naming it, with the APs that take it from there, and how many folders below take it as they do.
- **Apply at…** ticks location folders.
  - Each one ticked keeps its list and adds this folder.
  - Each one unticked keeps its list without it.
  - All are saved as one change.

### The server

- **A batch may also hold `add-folder` and `assign-services`.** What a batch may hold is a table in `change` (`Batchable`), which the API reads too. Another kind joins with a line, once it changes the Org alone and is made on the batch's copy.
- **A batch that makes a folder is allowed a step at a time,** each on what the steps before it leave. So a network set in the new folder is judged where the folder is made, under its parent. Other batches are judged as before, each change on the state as it is.
- **A Services node's page has `placements`:** each Locations folder the caller may view, with the service folders it takes and the node that sets them. APs and isolated folders are left out.
- **The page's save takes whole changes from a part** (`ops()`) beside its sets and unsets. They go first in the batch, so a set may be in a folder made in the same change.

## Consequences

- **A newcomer's first network is all clicks:** **New folder…**, adopt the APs, then **Add a network** on the folder. The guide's API step (0316) can go once a release has this.
- **One change in the log for each Apply,** as for every page since 0204.
- **Unticking a folder at a location that inherits its list** gives that location a list of its own, without the folder. The preview says so ("set here").
- **Not as drawn:**
  - A network a draft brings is a chip under the list, not a card of its own: its card is drawn once the change is made.
  - The tree beside the page is not changed.
- **At phone width,** the page's main column already narrows beside the tree (every tab does); this adds nothing to fix that.

## Checked

- **Tests:**
  - `TestBatchMakesAServicesFolder`: the three steps made as one, a failing step leaving no folder, and a move refused in a batch.
  - `TestBatchWithANewFolderIsJudgedAStepAtATime`: by account and right, and judging makes nothing.
  - `TestBatchANewServicesFolder` (API): preview, one change, the APs re-versioned, and `placements` for the Org, a folder and the folder below it, with no AP among them.
  - `TestBatch`'s refusal reworded. Set/unset batches, such as the client page's homes (0319), are made and allowed as before.
- **In the UI harness (`uidev`, 2026-11-05):**
  - **Sandbox's row:**
    - opened in its row;
    - **As Example** said "Aeolus Lab goes" and dimmed its card, and the preview named both APs;
    - **Set here** with a subfolder that inherits the same network showed the clash and held back Save…;
    - unticked, the preview read "Sandbox, set here → Lab copy, set here".
  - **A new folder House, with nothing applied:**
    - "none yet", "No network reaches here yet." and **Add a network**;
    - with the form filled in, Review showed the three steps;
    - Apply logged change #31 as one batch: `add-folder`, `assign-services`, `set`.
  - **Services › Example networks:**
    - Applies at listed House;
    - **Apply at…** with the Org ticked and House unticked made one batch of two `assign-services` (#32);
    - an Org-level placement over a folder with its own list counts none of that folder's APs.
- **Lint:** gofmt, go vet, every UI module loaded as an ES module, and ESLint's no-undef.
