# 0210. A management interface that moves renders the config again

- Status: Accepted
- Date: 2026-10-23
- Proposed by: Claude, from C360-AP's move to VLAN 1 (0209); Griff: "yes do the management fix". Written up by Claude
- Refines: 0054, 0063

## Context

- **What the agent renders depends on the AP's management interface,** the one with its default route.
  - A VXLAN tunnel starts from it (`tunlink`, 0054).
  - The firewall rule that lets the tunnel in goes in that interface's zone.
- **The agent renders only when a config comes:** after a start, which asks for it whole, or for a new version. Between them it asks only whether the version changed (`If-None-Match`), and the manager answers 304.
- **So a management interface that moves goes unseen until then.**
  - On 2026-10-23 C360-AP was moved by hand from `vlan20` (VLAN 20) to `lan` (VLAN 1).
  - Its aeolus_50 tunnel stayed on `tunlink vlan20`, an interface that no longer existed, and was down about 2½ minutes, until its agent was restarted.
  - The same comes of a DHCP move to another network, or a renumbering by hand.

## Decision

- **Each poll, the agent looks at its management interface.**
  - Where two looks in a row agree that it is another than the one its config was last rendered with, it asks for the config whole, not only whether it changed.
  - The config that comes is rendered, and what changed is checked and applied, as after a start.
- **No interface is no move,** as while the network reloads, and nothing moves before the first render.
- **It is said once in the log:** "the management interface is now `lan`, not `vlan20`: rendering the config again".

## Consequences

- **A tunnel follows the AP's management to its new interface by itself,** about two polls after it moved.
- **Only the management interface is watched,** not every fact the renderer reads. The others the agent learns once (packages, drivers), or they come with a restart (0057).
