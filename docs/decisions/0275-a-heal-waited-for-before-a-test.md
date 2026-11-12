# 0275. A heal waited for before a test

- Status: Accepted (Aeolus's own development, 2026-11-01)
- Date: 2026-11-01
- Refines: 0121 (the client test), 0167 (binding the driver again)

## Context

- **A scan radio that has been a client can go deaf.** airscan's `heal` binds its driver again when a look hears nothing.
- **After a test, the client test waits for an airscan that heals** before it binds the driver itself.
- **Before a test it didn't.** It found the radio deaf, stopped airscan and bound the driver again itself, whatever airscan was doing.
- **On C360-AP at 13:18:46 CST (2026-11-01), the two met.**
  - A test was asked the second airscan's heal began.
  - The test stopped airscan between its unbind and its bind.
  - The test's own bind_again then found no driver to bind, and gave up.
  - airscan, started again, found no radio and waited for one ("waiting for scan radio"). The radio had no driver until it was bound by hand at 13:22.

## Decision

- **Before a test, as after one, an airscan that heals is waited for:** a look newer than the deaf one, for as long as a bound driver and a look take (BOUND_WAIT + HEAR_WAIT). Only where none hears does the test bind the driver itself.
- **bind_again binds a radio with no driver too,** through the kernel's `drivers_probe`, which binds the driver that fits it. It needs no list of drivers. A radio with a driver is unbound and bound again as before.

## Consequences

- **A test asked mid-heal waits for it,** and starts once the radio hears.
- **A radio a heal cut short left without a driver comes back** the next time the test binds it again.
- **airscan's side is the port's:** an airscan stopped mid-heal, and started again, waits forever for a radio with no driver. Reported to the porting session.
- **Checked:** `drivers_probe` bound C360-AP's QCN9074 scan radio (0001:01:00.0) to ath11k_pci, airscan stopped, and airscan took it up as phy5 once started. The test compiles on Shed's 25.12 ucode, and a 5 GHz test after it passed.
