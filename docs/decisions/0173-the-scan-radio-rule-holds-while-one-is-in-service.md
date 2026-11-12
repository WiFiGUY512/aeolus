# 0173. The scan radio rule holds while one is in service

- Status: Proposed
- Date: 2026-10-17
- Proposed by: Griff (2026-10-17): "the access radios shall only be used for access as long as a multifunctional radio for scanning exists, lest they default to normal radio behaviours". Written up by Claude
- Refines: 0081, 0116

## Context

- **Griff's rule is conditional** (0081): where an AP has a radio that serves no clients, its serving radios never scan or leave their channels; where it has none, they scan as RRM does (0073). Since 0116 RRM runs on such an AP and listens through the scan radio, airscan's.
- **Whether an AP has one was read only from airscan's mark** (`option airscan '1'` on the wifi-device), at render time: the renderer sets `scan 'airscan'` in the RRM section where a radio carries it, and the daemon keeps the serving radios off scanning for as long as that is set.
- **The mark outlived the scan radio.** airscan set it when it took its radio and nothing took it away. Turned off, as the C-260's README says to do to serve clients on that radio, airscan left the mark, and the AP neither scanned from its serving radios nor had anything scanning for them. The same where airscan stopped for good: procd gives up on a service after five failures in an hour.

## Decision

- **The mark follows airscan** (in the ports' airscan, 0.2-7, openwrt-arista f08f8e3): turned off in its config, airscan takes the mark away at its start; and it marks its radio whether or not it had to disable it too, so turned on again the mark is back.
- **RRM reads the mark itself, at every look** (10 s), not from the render, which only follows a new config from the manager: the mark gone, the AP's serving radios scan as on any AP, within a look, and the mark back, they keep off again.
- **And whether the scan radio is in service** (`hold_off`, every look): in service while airscan answers with its radio (`status` with a `phy`), lent or not, or a client test that stopped airscan holds it (0121). Where neither has had it for 15 minutes, the serving radios scan, as on an AP without one; once it is back they keep off again, and listen through it. The daemon gives airscan those 15 minutes from its own start, or from the mark appearing, as at boot, when airscan starts after it. Each change starts every radio's visits afresh, and is logged.
- **A deaf scan radio is still in service** while airscan answers with its radio: it binds the driver again to bring it back, and one it cannot is an alert (0126). Where binding the driver again leaves airscan no radio at all, it is not in service, and after 15 minutes the serving radios scan.

## Consequences

- An AP whose scan radio is turned off, or given to clients, is an AP without one: its serving radios scan for RRM within a look.
- An AP whose airscan dies scans from its serving radios after 15 minutes, with a log line saying so, and stops once airscan is back.
- Tried on a C-260 (2026-10-17): the mark gone with airscan turned off and back with it on (the ports' airscan). The daemon, run there with the wait cut to a minute: listening, only the scan radio scanned (phy0, nl80211 scan events over 30 s); a minute after airscan was stopped it said so, and both serving radios started a scan every 15 s (phy1 and phy2); airscan started again, it said the scan radio was in service within one look, and they stopped.

## Not done

- **The manager does not show which way an AP's RRM listens.** The daemon logs it; the report does not carry it yet. Done since (2026-10-24): RRM's report says `listens` (`scan-radio` or `serving`) and `scan_out` where its scan radio is out of service, and the channel ratings say which.
