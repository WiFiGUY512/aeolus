# 0199. The way out: whether a full-size packet gets out

- Status: Accepted (Griff, 2026-10-21: "yes, do that!!")
- Date: 2026-10-21
- Proposed by: Claude
- Refines: 0121, 0134, 0142, 0182

## Context

- **A day spent on the wrong suspect.** On 2026-10-21 two IoT devices rejoined Wi-Fi every 43 seconds, and Aeolus pointed at the C-360s. A day of radio tests went by: protocols, TWT, BSS color, channels, power. Then a capture showed the cause:
  - The internet link, T-Mobile's 5G behind 464XLAT, drops IPv4 packets sent without DF once they are over 1408 bytes.
  - The robots' ESP32s send TCP without DF, and AWS gave them MSS 1376, so every full segment, 1416 bytes, was lost. Their cloud session stalled, and they dropped Wi-Fi and joined again.
  - It was fixed by clamping TCP's MSS on the internet router to 1360.
- **Aeolus could not see it.**
  - A client test pinged its gateway, asked DNS a name, and measured speed. All of that passed, with the black hole there.
  - The internet path (0142) times a ping out, which is small.
  - `ping` on the AP sets DF, as Linux does by default, so it is told to send smaller and never meets the loss.
- **What counts is what TCP sends.** Once the router clamps TCP's MSS, segments fit, though ICMP still meets the same limit. A test that judged by the largest packet alone would cry wolf forever after the fix.

## Decision

- **A client test measures the way out,** as its last step, `path`, on the test's own interface. It checks against the AP's internet target, `system.internet` (0142); with none set it measures nothing.
  1. **It pings without DF.** An echo is sent from a raw socket with `IP_MTU_DISCOVER` off, as many IoT clients send. The first is small; one the target does not answer measures nothing more.
  2. **It looks for the largest that gets out.** A full-size 1500-byte packet goes next, and where that is lost, the largest that comes back is found to within 4 bytes. Each size is tried twice before it counts as lost.
  3. **It measures what TCP sends there.** A connection to the target on port 443 says its segment size (`TCP_MAXSEG`, the MSS less Linux's 12 bytes of timestamps), and a full segment adds 52 bytes of headers.
- **The test fails at `path` only where TCP sends more than gets through.** Its reason says what to do: "packets over 1408 bytes without DF are lost on the way out, and TCP here sends 1472, so IoT clients that send without DF stall: clamp TCP's MSS to 1368 or less at the internet router".
  - A full-size packet that gets out passes, and so does a limit that TCP is held below.
  - Where TCP could not be measured, it is said and fails nothing.
  - A failing test makes its alert as any does (0182).
- **A passed test's summary says the way out:** "full-size packets out", or "packets out to 1408 bytes".
- **The test's way draws it** as its last point, "Way out".
- **Over the wire (0128) it is measured too** (2026-10-21, Griff: "do 2 and 3"), from the test's own frames, as that test has no IP stack of its own. It runs after DNS, with the address the test was given and its gateway:
  1. **The pings are frames:** IPv4 without DF, through the gateway's MAC, tagged as the network's VLAN. An answer is read back from its first fragment. The same sizes are tried as through the air.
  2. **TCP is a SYN** to the target's port 443 asking Ethernet's MSS, 1460. The SYN-ACK's MSS, as the routers between leave it, is what a client then sends: the smaller of the two, plus 40 bytes of headers. A reset ends it.
  - A test whose gateway did not answer measures nothing more; that test has already failed.

## Consequences

- **A black hole like the robots' shows as a failing test, with its fix, the first time a test runs after it appears.** It no longer takes a day of radio tests.
- **A test takes a few seconds more,** about 2 where full-size packets get out, and up to about 20 where it searches.
- **The target must answer pings.** Cloudflare's and Google's resolvers do. One that answers none leaves the way out unmeasured, and says so.

## As built (2026-10-21)

- **Agent:**
  - `icmp_echo` and `echo_of` in `aeolus/wiretest.uc`; for the wire, `ping`, `ip_of`, `syn`, `rst` and `tcp_of` there, and `wire_way_out` in `aeolus-clienttest`, after DNS.
  - `way_out` and the `path` stage in `aeolus/clienttest.uc`.
  - `echo_out` and `way_out` in `aeolus-clienttest`, run after the speeds.
- **Manager:** `testPath` and the `path` stage in `internal/api/clienttest.go`; the point in `internal/api/way.go`, through the air and, between DNS and the address given back, over the wire.
- **UI:** "Way out" in `views/clienttest.js` and `views/way.js`.
- **Tested:**
  - **The agent's tests** print the echo, its answer told from others, and the verdict in each case. They were run in WSL with real ucode.
  - **Over the wire,** `agent/test/wiretest.uc` prints:
    - a full-size ping frame, without DF, its header's checksum good;
    - its answer read back, and not a later fragment, another address or ARP;
    - a SYN and a reset, their checksums good;
    - a SYN-ACK's MSS read past other options;
    - the black hole failing at `path`, and the clamped way passing.
  - **`TestATestFindsTheWayOutLost`** checks a result failing at `path` is kept, with its way, and one that is no way out is refused.
  - **Measured on ShedAP before this was built:**
    - without DF, 1408 bytes got out and 1416 did not;
    - with DF, 1416 got out;
    - after the clamp, TCP to 1.1.1.1:443 gave a segment size of 1348.
