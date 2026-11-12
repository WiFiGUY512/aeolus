# 0203. A batch is one change: several sets and unsets, on any nodes

- Status: Accepted (Griff, 2026-10-21: "3 yses do this everyewhere", one save per page)
- Date: 2026-10-21
- Proposed by: Claude, for one save per page (0202); accepted with Griff's word
- Refines: 0026, 0045, 0046

## Context

- **One save per page needs one change for the page.** A page such as the Networks tab edits fields that are set at some nodes and unset at others. One network's settings can live on more than one Services folder.
- **A change touched one node.**
  - `set` with `values` set several fields of one node (0045).
  - `unset` with `paths` unset several fields of one node (0046).
  - Nothing made a set and an unset as one, or touched two nodes as one.
- **Saving them one by one was not one save.**
  - Each was its own log entry, and each re-versioned the APs: one page could send an AP several configs in a row.
  - One that failed halfway left the page half saved.

## Decision

- **A new kind, `batch`,** carries `ops`: 1 to 256 changes, each a `set` or an `unset` as it would be made alone, with its own `tree` and `node`.
  - Nothing else goes in a batch. A lock, a move or a batch inside a batch is refused.
- **All or none.**
  - The changes are made in order, on a copy of the state, which becomes the state only once every one is made.
  - One that fails leaves everything as it was, and the error says which: "change 2 of 3 (set locations house system.poll): …".
- **Each change is allowed and checked as if it were made alone:**
  - the caller's role at its node (0030);
  - the field schema, its value, and sealing its secrets (0027);
  - and a test network's network and until together (0179).
  - Per-user keys against RADIUS passphrases (0115) are judged by what the whole batch leaves.
- **One entry in the log, one reason, one new version for each AP it changes.** Its effect is each change's effect, in order.
- **A preview** returns `resolved` as a list: each change's tree, node and fields, as they would resolve afterwards (0046).
- **Secrets stay sealed** in the batch's changes and effects, in the log and in every answer.

## Consequences

- **A page saves as one change,** whatever it touches, and an AP gets one config for it.
- **The change log shows a batch as one entry** with its changes inside, not one entry per field.
- **An older client sees an unknown kind** in the log and can show it only as such.

## As built (2026-10-21)

- **Engine:**
  - `change.Batch`, `Op.Ops`, `MaxBatch` and `ErrBadBatch` in `internal/change/change.go`.
  - `Apply` runs the batch on a clone.
  - `Authorize` allows each change.
  - `GuardIn` judges the whole batch.
- **Checks:** `schema.CheckOp` and `checkTestNetwork` check each change.
- **API:**
  - `prepare` readies each change.
  - `viewOp` and `viewEffect` mask a batch's changes and effects.
  - `resolvedAfter` gives each change's fields.
- **MCP:** `ops` on `make_change` and `preview_change`.
- **Tested:** `TestBatchAllOrNone`, the batch cases of `TestAuthorize`, and `TestBatch` in the API.
  - `TestBatch` covers the preview, the commit, the masking, one locked change refusing the batch, each change's own checks, and that the passphrase is in neither the log nor the database.
