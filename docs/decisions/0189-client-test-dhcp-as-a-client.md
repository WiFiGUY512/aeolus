# 0189. A client test asks for an address as a client does

- Status: Accepted (Griff, 2026-10-21: "Keep working hard and deploying all night", under his standing word to merge what is ready)
- Date: 2026-10-21
- Proposed by: Claude, from the client tests that failed at DHCP overnight
- Refines: 0121, 0123, 0130

## Context

- **Client tests through the air failed at DHCP now and then** on every network but Home Wi-Fi, mostly on 2.4 GHz: 3 to 4 of each network's 20 to 25 tests in two hours. Those that passed there often took 2 to 4 s for an address where Home Wi-Fi's took 0.3 s, from the same DHCP server.
- **Replies broadcast over the air are lost now and then.** A capture on the office AP showed:
  - **The servers broadcast their offers and acknowledgements** to 255.255.255.255.
  - **The same offer came again 2 s later, and the same acknowledgement,** where the test's client had not heard the first.
  - **Over the air a broadcast goes at a low rate,** with no acknowledgement and no retry.
  - **Home Wi-Fi alone has `multicast_to_unicast` on** (0049). Its replies go to each client as unicast, which the radio retries until heard.
- **The test gave up sooner than a client does:**
  - It ran `udhcpc -n`, which gives up after the three requests of one offer go unacknowledged, a few seconds in. The failures were 5 to 6 s in.
  - A phone, or an IoT device, starts again and asks for half a minute or more.

  So a test failed where a client would have been given an address, a few seconds late.
- **A failure said only "no answer to its DHCP requests",** whether an offer came or not.

## Decision

- **A test through the air asks for an address for up to 20 s.** Where no offer comes, or no acknowledgement of one, it starts again a second later, as a client does. It is `udhcpc` without `-n`, ended at 20 s.
- **A failure says where asking stopped,** from what `udhcpc` said, as the test over the wire does (0128):
  - an offer, and none of its requests acknowledged: "a DHCP server offered … and never confirmed it";
  - an offer refused after;
  - or no answer at all in 20 s.
- **An address that came late is a pass,** and its time is in the test's way online (0130) as before, so a slow DHCP still shows.

## Consequences

- **A network's tests fail at DHCP where a client would not be given an address,** not where one broadcast was lost.
- **A test that fails at DHCP takes up to 20 s there,** about 10 s more than before. Its scan radio is away that much longer.
- **Not done, Griff's call:** turning `multicast_to_unicast` on for Home_Wi-Fi_IoT, Aeolus-Enterprise and aeolus_50 would make their DHCP replies as dependable as Home Wi-Fi's. A change to a network's wireless settings restarts its radios, so it waits for him.

## As built (2026-10-21)

- **The test:** `measure()` in `aeolus-clienttest` runs `udhcpc` through `system()` with a 20 s timeout, under `exec` so the timeout ends `udhcpc` itself, and keeps its output in `udhcpc.log`.
- **The parsing:** `dhcp_said()` and `dhcp_failure()` in `aeolus/clienttest.uc` read that output and word the failure.
- **Tested:**
  - **`agent/test/clienttest.uc`:** given late, offered and never acknowledged, refused, never answered.
  - **On the C-260:** the timeout ends `udhcpc` and leaves none running.
