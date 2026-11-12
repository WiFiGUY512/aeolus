# 0282. Release channels

- Status: Accepted (Aeolus's own development, 2026-11-01; for the viability Griff asked for: "first lets make aeolus viable")
- Date: 2026-11-01
- Refines: 0183 (the manager's own releases), 0217 (a release says what it brings)

## Context

- **Every release went straight to every manager that updates itself.** On 2026-11-01 that was nine releases in a day.
- **That suits the lab, where each one is made and tried, but not a site that just wants its Wi-Fi to work.** Such a site would take each day's releases, and each one's first-day faults with them. For Aeolus to be something others run, a release has to have proved itself before it reaches them.

## Decision

- **Two channels: testing and stable.**
  - **Every release (vX.Y.Z) is on testing as it is made.**
  - **A release is promoted to stable by a second tag, `stable/vX.Y.Z`, on the very commit vX.Y.Z is.**
    - A stable tag on another commit promotes nothing, so a tag pushed by mistake can't put an untried build on stable.
    - A promotion is deliberate and in git's history: who promoted what, and when.
- **The Org chooses, in `system.updates.channel`: `stable`, the default, or `testing`.**
  - The updater's check says the newest on each (`newest=`, `stable=`).
  - "Update now", the automatic update in its window, the banner and the notes all follow the chosen channel.
  - With nothing promoted yet, the stable channel offers nothing.
- **A fresh install takes the newest stable release,** or the newest release while none is promoted. `--version` still names any.
- **The APs follow their manager:** they take their agent from it (0079), so a manager's channel is its APs'.
- **When a release is promoted:** after it has run seven days on testing in the lab, the manager and every AP on it, with no regression it brought (no critical alert of its making, and no fix release made for it). Not every release is promoted: stable moves from proven release to proven release. In this project Claude promotes under Griff's standing word on releases, and says so in the release's notes.

## Consequences

- **A site on stable runs a release at least a week old,** proven on the lab first. A fix reaches it once its own release has soaked.
- **The lab runs testing.** Griff's manager is updated by hand at each release (`aeolus-update`), which no channel changes. Setting his Org to testing, an admin's change, is his call, so its banner and its automatic update see each release as it comes.
- **The first promotion can come a week after a release that holds.** v0.221.1, 2026-11-01, is the first candidate.
- **Checked:**
  - the updater's tests (no stable tag; one on its release's commit counts; one on another commit doesn't; an update says it too);
  - the update tests (testing as before; stable takes the promoted one, none while none is, and not one that runs or failed lately);
  - the routes say channel and target, and "Update now" asks for the target;
  - the UI modules load and pass the undefined-name lint.
