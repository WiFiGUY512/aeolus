# 0258. A leaving says where the client went next

- Status: Accepted (Aeolus's own development, 2026-10-31, at Griff's standing word to keep developing it)
- Date: 2026-10-31
- Refines: 0255 (why clients leave)

## Context

- **0255 says why a client left an AP:** who sent it off, the frame and reason, and its last minute there. It doesn't say what the client did next.
- **What it did next is often the answer.** On 2026-10-31 B122 suspected ath11k of sending IoT devices off wrongly, for missing ACKs at good signal. It was closed as not a fault by reading the joins after each leaving, by hand:
  - the kettle was back on another AP about 100 s later, its own loop;
  - drive-cooler was joining C360-AP in the same second the C260 sent it off;
  - the bedside lamp didn't come back: it had been switched off.
- **A client sent off while it is still talking** (a wrong kick) is told apart the same way: it is back straight away, at once or on the same AP.

## Decision

- **Each of the latest leavings carries `next`:** the client's first attempt on any AP up to 10 minutes after it left. It also counts an attempt on another AP up to 30 s before, where the client had moved already; that gives a negative `after_s`. Each `next` gives the AP and its name, seconds after, and the attempt's outcome. The attempt it left, on the same AP before, isn't it.
- **It is read when asked, from the attempts kept** (`ConnectionsBetween`, a general query of one client's attempts between two times), so leavings kept before this have it too, and a later change to the window needs no migration.
- **The leaving panel shows it as "Then":**
  - back here in 20 s;
  - back on c360 in 2 min;
  - joining c360 already;
  - not back in 10 min, or not back yet within the ten minutes.
- **The MCP tool's get_leaving says the same,** with last and inactive_ms, which it hadn't listed.

## Consequences

- **A leaving answers "did it really go?" on its own.** "Not back in 10 min" after a reason-34 kick is a client gone; back in seconds is worth a look.
- **Each read of /v1/leaving makes up to 50 small queries,** one per latest leaving, on the attempts' (mac, at) index.
- **connections.go's three queries of the same columns share one row reader** (`connectionRows`), so a new column is added in one place.
