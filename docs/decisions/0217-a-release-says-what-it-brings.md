# 0217. A release says what it brings

- Status: Accepted
- Date: 2026-10-24
- Proposed by: Claude, from 0183's "Not yet" ("release notes in the banner, where a release carries them"), on Griff's standing word to go on developing Aeolus. Written up by Claude
- Refines: 0183

## Context

The manager's banner says "Aeolus v0.167.0 is out" and offers Update now (0183). It doesn't say what the release brings, so an admin weighs an update knowing nothing of it, or goes to the repository to read the merges.

Releases are annotated tags, so each can carry a message. Until now each message was one line.

## Decision

- **A release's tag carries its notes.** The first line says what it is about. Under it go the changes, one line each, from the merged pull requests, with their decisions.
- **The updater writes the notes as it fetches the tags,** at a check and after an update (`deploy/updater.sh`, `notes`):
  - one block per release newer than the one running, the newest first, at most ten, each begun by `@@ vX.Y.Z`;
  - printable text alone, at most 4 KB a release and 32 KB in all;
  - into `notes` beside `status`, written through a new file and a rename, as `status` is.
- **The manager reads them** (`readUpdateNotes`) and keeps those newer than the release it runs. `GET /v1/system/update` gives them as `notes: [{version, notes}]`.
- **The banner and the Updates panel** have a folded "What's new" under the newer release.
  - Each release's notes are drawn as paragraphs and lists (`prose`, moved to format.js from the advisor's page), never as HTML.

## Consequences

- **An admin sees what an update brings before pressing Update now.** One who skipped releases sees each one's notes.
- **The updater is installed again by each update** (`aeolus-update` copies `deploy/updater.sh`). The release that brings this writes no notes of its own; the one after does.
- **A lightweight tag** (no message) shows the commit's message.
