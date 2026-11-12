# 0238. A radio Aeolus turned off comes on again

- Status: Accepted
- Date: 2026-10-29
- Proposed by: Claude, from the Living Room's 5 GHz during the night's TX-fault work. Written up by Claude
- Refines: 0039 (the agent renders), 0053 (ports on and off)

## Context

- **A radio's `enabled` has three states:** true, false and unset. The renderer wrote `disabled` only when it was set:
  - false wrote `disabled '1'`, and true `disabled '0'`;
  - unset left `disabled` as the AP had it.
- **So unsetting false did not turn a radio back on.** It left the `disabled '1'` Aeolus itself had written.
  - On 2026-10-28 the Living Room's 5 GHz was turned off (change 486) while its transmitter was at fault.
  - It was unset the next night (change 489) to bring it back. The radio stayed off, and a sysupgrade kept it off.
  - It came back only when set true by hand (490).
- **Ports already tell their own off from the AP's:** Aeolus keeps a section of its own for a port it turns off (0053), and "unset" is documented as "as the AP has it, which is on unless Aeolus turned it off". Radios had no such record, and no description.
- **Turning every radio on when unset would change adoption.**
  - A fresh OpenWrt's radios can be off, and today Aeolus leaves them as they are until a radio's `enabled` says otherwise.
  - That is for an operator to change, not a fix.

## Decision

- **The renderer keeps the radios Aeolus turned off,** in an `aeolus_radios_off` section of its own package: each radio whose band's `enabled` is false.
- **A radio whose `enabled` is unset and which Aeolus turned off is turned on again:** `disabled '0'`. Its record goes with it.
- **A radio Aeolus never turned off is left as the AP has it,** off or on, as before.
- **The schema says so for each band's `enabled`:** false off, true on, and unset as the AP has it, which is on unless Aeolus turned it off.

## Consequences

- **Turning a radio off and later clearing the setting brings it back,** which is what a person clearing an "off" expects.
- **Nothing changes on the APs today:** no managed AP's serving radio is off by Aeolus without the setting still saying so. Reserved scan radios are skipped, as before.
- **A radio turned off by an agent older than this one has no record.** Unsetting its `enabled` leaves it off; setting true still turns it on. Only the Living Room was in that state, and it is set true.
- **Tested:** two agent cases.
  - **radio-off:** off writes `disabled '1'` and the record.
  - **radio-on-again:** unset turns a recorded radio on, drops the record, and leaves the AP's own off alone.
  - Both outputs pass the manager's render check.
