# 0244. A speed server out of reach fails no test

- Status: Accepted
- Date: 2026-10-29
- Proposed by: Claude, from the aeolus_50 tests that failed once scheduled tests measured speed. Written up by Claude
- Refines: 0121 (the client test), 0134 (the remote speed server), 0242 (slow is said)

## Context

- **On 2026-10-29 scheduled client tests began to measure speed** at Home (0242, change 520), against the local server, zephyrus.example.com.
- **From then on every test of aeolus_50 failed "at dns":** "its DNS server, 8.8.8.8, has no address for zephyrus.example.com".
  - Thirteen in five hours, and warnings on the C-260, the Living Room AP and the Office AP.
  - Before, all twelve of the day's aeolus_50 tests had passed.
- **The DNS step looked up the speed server's name, where it had one,** in place of the name the manager names for it (example.com unless set).
  - aeolus_50 is tunnelled, and its DHCP hands out public DNS, which knows no site's own names.
  - So the network's DNS worked, and the test said it did not.
- **A speed failure failed the test too.** The remote server's never has (0134): "the server is not the site's".
  - The local server is the site's own, but it is a measuring tool: a network kept apart from it still takes its clients online.

## Decision

- **The DNS step looks up the name the manager names for it,** whatever the speed server is called, as any client's lookup would be.
- **The speed server's own name is looked up after,** of the same DNS server, as a step of the speed. Not found, the speed is not measured: "its name was not found".
- **A speed server the network cannot find or reach fails no test.** The test passes on its other steps, and its line says why there is no speed: "no speed to http://zephyrus.example.com: its name was not found".
- **The speed's steps are kept,** so a test's page still shows the lookup and the error.

## Consequences

- **aeolus_50 passes again where it works,** and has no speed, so 0242's slow-radio rule has no tests of it to judge. That is what is wanted: a radio is judged on the networks that reach the server.
- **A speed server down, or a network cut off from it, is no longer a failing test.**
  - It shows only as a test with no speed.
  - 0242 then has nothing to judge that radio by.
  - Telling the manager that its speed server answers none of its tests is left for later.
- **Tested:** the verdict of a test whose speed server refused it is passed, its line naming the server and the error, in agent/test/clienttest.out.
