# 0317. Two builds of a library

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, after the port -121 flashes (2026-11-05), at Griff's "do 1-3"
- Refines: 0315 (a build's own packages), 0099 (alerts)

## Context

- **After the -121 flashes, the C260, C-200 and W-318 put OpenWrt's snapshot builds of uclient, udebug, udebugd, usteer and ustp back.** Those were built against libubox20261103, beside the image's libubox20260721.
- **A process that loaded both held two libubox, so two event loops.** The agents, usteerd, ustpd and udebugd all did. The agents waited for good on a socket in the other loop.
- **It was found by hand,** with `grep libubox /proc/<pid>/maps`, half an hour on. 0315 keeps it from happening that way again, and its `reports-stopped` alert says when an agent stops reporting. Neither names the cause, nor catches a daemon other than the agent holding two builds.
- **OpenWrt names each ABI of a library by its soname's suffix** (libubox.so.20260721), so two in one process's maps is two builds.

## Decision

- **Each state report says the libraries some process holds two builds of** (`mixed_libs`). Each entry gives the library (libubox.so), the builds by suffix, and the processes, by the script ucode runs or the program.
  - The agent reads every process's `/proc/<pid>/maps` and keeps only those that hold two (`aeolus.mixedlibs`). At most 8 libraries, and 16 processes each.
- **The manager takes each entry as the agent writes it,** leaving out one that isn't (0290).
- **The manager raises a warning per library, `mixed-libs:<lib>`:** "aeolus-agent and usteerd hold two builds of libubox.so at once (20260721, 20261103): a package was installed for another image than the AP's, and what holds both may stop working without a word, as three agents did on 2026-11-05. Put in the AP's own build of it, and start them again".

## Consequences

- **A package of the wrong build is named at the AP's next report,** even where the agent itself is fine and another daemon is not.
- **Where the agent itself holds both and has stopped,** its reports stop, and `reports-stopped` (0315) says that instead.
