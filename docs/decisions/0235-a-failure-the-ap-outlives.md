# 0235. A failure the AP outlives

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from the C-260's flash to the port's -54 build. Written up by Claude
- Refines: 0007 (the AP's poll), 0057 (vxlan and netifd), 0099 (alerts)

## Context

- **After a start, the agent asks for its config whole and renders it again** (0079). Where that renders to what runs already, it says nothing, since there is nothing to check or apply.
- **After the C-260's sysupgrade to -54 on 2026-10-28, that first render failed.**
  - The AP ran version 475 from before the flash, since the config is kept.
  - vxlan was not yet installed again, so the agent could not render 475 and said it did not apply.
  - The manager raised a critical alert: "version 475 did not apply".
- **restore then installed vxlan, restarted the network, and restarted the agent** (0057, 0234).
  - The new agent asked whole, 13 seconds after the network restart. netifd had not yet loaded vxlan, which the AP's report said.
  - So the manager held 475, and the agent kept what ran.
- **The hold lifted on the AP's next report. The agent never knew:**
  - It asked by the version it runs, 475, so the manager answered 304. A held answer carries no ETag, but the version is all a 304 is decided by.
  - Had it asked whole, 475 would have rendered to what runs, and the agent would have said nothing.
  - The failure stayed the AP's last word on 475, and the critical alert stood until the next version, while the AP ran 475 with its tunnel up and its client tests passing.

## Decision

- **A held agent asks for the config whole on every poll until it is ready.**
  - A held answer is small: the state and its problems, no config.
  - The poll after the hold lifts brings the config, and the agent renders it.
- **A ready config says `failed: true` where the AP's last apply report was of that version and did not apply.**
  - The agent then checks and applies what it renders even where that is what runs already.
  - Applying nothing changes nothing on the AP. The manager's check passes, and the AP says the version applied.
- **The manager's word is used, not a note on the AP,** so it holds across reboots and sysupgrades. It also covers an AP whose failure an older agent said.

## Consequences

- **An AP that outlives a failure of the version it runs says so once it can render it,** and the alert ends then, not at the next version.
- **A held AP costs the manager a compose on each of its polls,** as an AP behind a held version already does.
- **Where the failure was a real one, the AP running an older version, nothing changes:** a version it does not run is rendered, checked and applied as before.
- **An agent before this one ignores `failed`,** and keeps its old ways until it is updated. The manager goes first, as for any field the agent reads.
