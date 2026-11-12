# 0237. Start what can start

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from the C-260's flashes to the port's -54 build. Written up by Claude
- Refines: 0234 (restore until the agent can start), 0080 (the agent as a package)

## Context

- **While a module the agent needs is missing, the init script still started every program of the agent's.**
  - Each program that imports the module failed as it started: the agent and its key agent, the prober and aeolus-live import math, and aeolus-live imports uclient too.
  - procd started each anew every five seconds, each time with a syntax error in the log.
- **On the C-260's first -54 flash (2026-10-28),** those errors filled the log within ten minutes. What the boot-time restore had said was gone: there was nothing left to tell why it had failed.
  - After its second flash, 0234's boot run had the modules back within two minutes, and 51 such errors were logged meanwhile.
  - Where the manager cannot be reached after a flash, the wait has no end, and neither do the errors.
- **Not every program needs those modules.** Steering, radio resource management, the record of clients coming online and the Bluetooth lending import none of them, and can run while the agent waits.

## Decision

- **While a module the agent needs is missing, the init script starts only the programs that compile.**
  - Each is checked with `ucode -c -o /dev/null`, as the agent checks an update (0079).
  - Steering and radio resource management go on; the agent, the prober and the line to the manager wait.
- **With nothing missing, nothing is checked:** every program starts as before, with no time added.
- **Once the modules load, 0234's boot run starts the service:**
  - That starts the programs that were left out, and leaves running the ones already running.
  - Restore restarts the agent where it put something back, as before. The boot run's start covers modules that came back some other way, such as by hand.

## Consequences

- **The log after a flash keeps what restore said:** each failed try once, and the wait once, not an error every five seconds per program.
- **Band steering and radio resource management keep running while the agent waits,** on the config the AP kept.
- **The agent no longer starts halfway through restore.** On ce7f's -55 flash (18:04Z), procd started the agent the moment restore had put math back, before vxlan. It rendered too early, failed, and started a restore of its own beside the boot run's. Now it starts when restore restarts it, with everything back.
- **A program that does not compile for any other reason is not started while a module is missing either.** It starts with the rest once the boot run is done, and fails then as it would have.
- **Checked live on the C-260:**
  - The compile check exits 255 for a missing module, and 0 for the agent, aeolus-live and aeolus-rrm.
  - A start of the running service kept all eight of its programs on the same processes.
