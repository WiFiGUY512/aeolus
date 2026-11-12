# 0179. A test network an installer switches on, which switches itself off

- Status: Accepted (Griff, 2026-10-20: "do all those except going public")
- Date: 2026-10-20
- Proposed by: Griff, for the installer role (0177): "they should be able to activate a set a temporary test ssid attached to credentials and a known vlan, that should be on a timer that automatically downs itself". Written up by Claude
- Refines: 0012, 0018, 0177

## Context

- **An installer puts APs up and checks them,** often before the building's own networks are on, or where they must not be touched. A network of their own to join with a laptop or phone, on a VLAN they know, tells them the AP, its uplink and the VLAN work.
- **A network left on is a risk:** an SSID with a shared passphrase, forgotten after the install, is a way in. So it must go off by itself, even when the manager can't be reached.
- **0177 gave installers the test SSID right** and left the network itself for this decision.

## Decision

- **A test network is a network with `test` set,** in a Services folder, made by whoever configures networks there (an admin or an operator). Its SSID, security, passphrase and transport are any network's: "attached to credentials and a known VLAN".
- **It is never on the air until it is switched on in a place.**
  - **`test_network`** on a Locations folder or AP names one of the test networks the folders there offer, and when it goes off: `{network, until}`, until in Unix seconds.
  - Whoever has the test SSID right there sets it: an installer, an operator or an admin (0177).
  - It is set with both values together, until ahead and at most **four hours** from when it is set, and the network one the folders there offer. Unsetting it switches it off at once.
- **Every AP below the place renders it while it is on, and leaves it out when its time is up, by its own clock:**
  - **The AP's renderer** leaves a test network out unless `test_network` names it and the AP's clock is before `until`. An AP whose clock is not set yet, before 2023, has none on.
  - **Its SSIDs carry the time** as `aeolus_until` on their wifi-iface sections.
  - **The agent's watchdog,** in its main loop, turns off any such SSID whose time is up: it disables the section and reloads, whether or not the manager answers. The AP's next render, at its next config version or start, leaves the network out.
- **The manager sends a test network only where it is switched on,** so an AP whose agent predates test networks never carries one that is only set up.
- **The manager checks an AP's render by the same rule** (`compose.Live`): a test network is expected only while it is on, by the manager's clock.
- **The page:**
  - **The Test network panel** is at the top of a Locations folder's or AP's Overview, for whoever has the right.
  - **Off,** it offers the test networks there and a time: 30 minutes, 1 hour, 2 hours or 4 hours.
  - **On,** it says which one, until when and how long is left, with Switch off.
  - **Where it comes from:** the Locations page lists the test networks offered there (`test_networks`), since an installer may not see the Services folders that offer them.

## Consequences

- **An installer can check an install alone,** with a network set up for it, and nothing is left on the air after them: at most four hours, by the AP's own clock.
- **Switching it on is a logged change,** with who and until when, like any other.
- **An AP whose clock is off by a lot** keeps a test network on for longer or shorter by as much; an AP with no clock at all keeps it off.

## As built (2026-10-20)

- **The schema:** `network.<id>.test`; `test_network.network` and `test_network.until`.
- **The renderer and the agent:**
  - `render.uc` has `live` and `test_on`, and the wifi-iface option `aeolus_until`;
  - the agent sends its clock as `facts.now`, and `tests_due` runs at the top of its main loop.
- **The manager:**
  - `compose.Live`, `compose.TestNetworkMax`, and `compose.AP` leaving out a test network not switched on;
  - `checkTestNetwork` in the API's commit check;
  - `test_network.*` takes the test SSID right (`authorize.go`);
  - `test_networks` on a Locations page.
- **The UI:** `views/testnet.js`.
- **Tested:**
  - `compose.Live`: on, its time up, a clock not set, and no until;
  - the API: a viewer refused; four hours exceeded, a time gone, a network that is no test network, one not offered and a network without its until each refused; an installer switching it on, the AP's config carrying it, and switching it off;
  - render cases `test-network-on`, `-off` and `-no-clock`, rendered and checked;
  - in a browser, as an installer: the panel off, switched on (it said until when and how long), and switched off;
  - on the W-318 test unit, the watchdog's own code against its real UCI: an until ahead left alone, one gone by disabling only that SSID on that radio, and nothing more to do after.

## Open

- A switch-on that ends on the hour of a shift, or at a time of day, rather than after a duration.
- The test network's clients on the page, as its own list during an install.
