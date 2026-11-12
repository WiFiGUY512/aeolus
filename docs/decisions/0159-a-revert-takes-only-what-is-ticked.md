# 0159. A revert takes only what is ticked

- Status: Proposed
- Date: 2026-10-14
- Proposed by: Griff, after a revert of C360-AP's own settings took its Bluetooth for Home Assistant with it ("I didnt know why it got cut off"), to a revert that lists each setting it would clear with a tick box: "ok do it". Written up by Claude
- Refines: 0046

## Context

- **Following the folder again unsets, in one change, every value a node sets for itself** (0046). An AP's page offers it as "Revert to <folder> (n custom settings)".
- **Its preview listed what each becomes, and Apply took them all.**
- **On 2026-10-14 a revert of C360-AP cleared seven settings** (change 230): 6 GHz off, its uplink split, 5 GHz look-first, captures off, its speed server, and Bluetooth for Home Assistant. Two were wanted back at once (changes 233, 234); the other four, the Bluetooth among them, were missed until Home Assistant had gone without the AP for nearly two hours (change 237).

## Decision

- **Where a revert would unset more than one value, each has a tick box,** all ticked to begin with.
- **One unticked stays set where it is,** and says so; the preview is asked again for what is still ticked, so what Apply does, the APs given new configs and the rules that would be broken are those of the ticked ones only.
- **With nothing ticked there is nothing to apply.**
- A revert of one value is as before.

## Consequences

- **A revert can take some of a node's own values and leave the rest,** in one change.
- **Nothing is unticked unless someone unticks it:** a revert of several still takes them all when Apply is pressed straight away.
- **Tried, 2026-10-14, in the harness:** an AP with three values of its own; unticking one left it "stays set here" and the preview's change named the other two; unticking all left no Apply.
