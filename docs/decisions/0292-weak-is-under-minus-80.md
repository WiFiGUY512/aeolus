# 0292. Weak is under −80

- Status: Proposed
- Date: 2026-11-03
- Proposed by: Griff: "move the weak line to -80db" (2026-11-03, about 11:00 CST), after the overnight report asked where it should be. Written up by Claude
- Refines: 0138, 0289

## Context

- **Sticky's weak signal (`steering.sticky_signal`) is −75 dBm unless set (0138).** It is the line under which:
  - **the weak rule (0289) puts a client off** that has had three looks under it, where the network has a better place for it;
  - **a client is asked what it hears by 802.11k (0289),** where no other AP has heard it on its band lately.
- **Griff's scale: −70 to −72 is good, and only −80 or −82 is weak.** A client at −76 to −79 is fine where it is.
- **Sticky on Home_Wi-Fi_IoT, 2026-11-03:** the Living Room AP put the water-temp-sender-loop (dc:4f:22:06:18:ad, 2.4 GHz, −76 to −78) off as weak at 01:37, 03:08, 03:38, about 06:42 and 10:42 CST. Each time it was back on the same AP within about 17 seconds, at the same signal (0291's `back`). Under −75 it was weak; on Griff's scale it was not.

## Decision

- **`steering.sticky_signal` unset is −80 dBm.** A client is weak under it: at −81 or weaker.
- **A network that sets its own keeps it,** from −95 to −50 as before.
- **The three that must agree move together:**
  - the renderer (`render.uc`'s `steered`), which writes the signal into the daemon's section, `list sticky '<network>=-80'`;
  - the daemon's own unset (`steer.STICKY.weak.unset`), for an entry with none;
  - the manager's render check (0039, `rendercheck`), which holds the AP's section to the intent.
- **Nothing else of the weak rule changes:** three looks in a row, a sign of somewhere better or no try yet, one try a week for a lone client, and the rest that doubles.

## Consequences

- **The weak rule and the 802.11k asks start 5 dB lower.** The weak rule leaves a client from −76 to −80 where it is, and 802.11k asks it nothing. One another AP hears 5 dB better is still moved, by 0289's better rule.
- **The render check refuses a sticky network's section at −75 from an older agent** whose network sets no signal, once the manager has this. That config is held until the agent updates (0079). The manager is updated first and the APs within minutes, so the window is that long.
- **The other −75 lines are not this one, and stay, each its own judgement:**
  - Assurance's floor (0123): a tester heard weaker than −75 is not tested from;
  - the struggling bar (0233, 0250), which runs from −60 to −75;
  - a client's journey's weak-signal note (since moved to −80 or weaker by 0293).
- **Tested:**
  - the renderer's steering case writes `plain=-80` for a sticky network that sets no signal;
  - the render check wants `sweet=-80` for one that sets none.
