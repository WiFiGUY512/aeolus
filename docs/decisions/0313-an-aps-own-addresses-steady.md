# 0313. An AP's own addresses, steady

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from changes 699 and 700 (2026-11-05)
- Refines: 0122 (an AP's facts), 0121 (client tests), 0126 (the scan radio)

## Context

- **An AP's facts list its own MAC addresses** (`macs`). The agent reads them from its network interfaces.
- **The manager tells an AP's own client tests apart from clients by those MACs:** the scan radio's address is the test client's (`testClients`, `testerOf`).
- **The scan radio's interfaces come and go.** airscan's `as-mon0` goes away while airscan heals the radio, after a restart until airscan starts, and while a client test runs; the test's `aet-sta0` exists only during a test.
- **So the scan radio's address was said or not by chance,** at each moment the facts were taken:
  - Change 699 (03:53 CST) took C360-AP's `30:86:2d:ef:2f:f0` out, after the -120 flash, before airscan was up.
  - Change 700 (04:06) put it back.
  - The C260's tester address did the same on 11-04 (change 696).
  
  Each of these changes made the AP's config again.

## Decision

- **The agent also lists each radio's own address,** from `/sys/class/ieee80211/<phy>/macaddress`, whether an interface of it is there or not.
- **On every Arista AP with a scan radio it is the address its interfaces take:** C360-AP `:f0`, C260 `:80`, Garage `:70`.

## Consequences

- **An AP's facts no longer change with its client tests or airscan's heals,** and its config isn't made again for them.
- **Each AP's facts change once,** at its first report on the new agent, where a radio's address wasn't listed at that moment.
