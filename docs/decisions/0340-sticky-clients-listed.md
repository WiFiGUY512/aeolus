# 0340. Sticky clients, listed; pinned clients that moved

- Status: Accepted
- Date: 2026-11-08
- Proposed by: Griff:
  - "we need a 'classified as sticky' list somewhere, maybe with the clients tab, something we can see and determine if per client steering is necessary or client to AP containment or whatever";
  - "if we have client containment on (where we lock a client to an AP (probably need to come up with a better name) then all the steering options should grey out, because they are no longer a thing";
  - "there should also be an inference so if a client that was pinned gets moved, and its no longer pinned to the best AP we should notify".

  Written up by Claude, and accepted with the merge.
- Refines: 0319 (where a client belongs), 0321 (wherever a client is heard best), 0289 (sticky clients)

## Context

**Nothing showed which clients hold on.**
- Sticky steering (0289) and client steering (0319, 0321) move a client.
- Steering's record (0139) says what it asked.
- The struggling alert (0233, 0309) names a named client that struggles.
- None of these lists the clients that sit on a far AP while a near one would serve them. A person had nothing to decide by whether a client wants a rule of its own.

**The minutes (0207) tell it.** Each AP sends what each minute came to for each client it serves, with its signal, kept for a week. A client heard at -84 dBm by one AP for hours, and at -50 by another before and after, held on.

Read over the 48 hours to 13:30 CST on 2026-11-08, ten clients held on for half an hour or more at -78 dBm or weaker on one AP. Most were 2.4 GHz IoT devices on the Shed AP:
- **94:51:dc:bd:06:84:** 31 hours at -84 on the Shed. The Living Room hears it at -36 the rest of the time.
- **esp-unknown and the Patio Quad Switch:** each hours at -84 to -88 on the Shed, with Living Room at -57 and -73.
- **Each of the three moved late, again and again.** It went from -84 on the Shed to -37 on Living Room the next minute: in reach of Living Room the whole time.
- **well_env_switch** (84:0d:8e:73:9a:f7): -92 on the Shed, and no other AP ever heard it. It is weak, and nothing better is known.

**A client pinned to one AP (0319) is beyond the AP-choosing steering.** The steering daemon keeps sticky's places to its AP. Its weak rule skips any client with a rule. A pin names no nearest AP. Only its Bands still choose for it, on that AP. A pin also holds a client where a person put it, so if the client is carried off, the pin keeps it on an AP that may no longer suit it.

## Decision

### The list

- **Clients › Sticky,** a part of a folder's Clients of its own (`/clients/sticky`), beside Wi-Fi roaming.
  - It is one band at a time, by the page's band picker: never 2.4, 5 and 6 GHz numbers together.
  - It covers the last day, 3 days or week, from a pulldown.
- **`GET /v1/sticky?under=&hours=`** (24 unless set, 1 to 168), over the APs the caller may view; the APs' own test clients are left out.

**What lists a client.** A client is listed at each AP and band where, over the hours, it had 30 minutes or more at -78 dBm or weaker (`stickyWeak`). The line sits between Griff's -70 good and -80 weak.

**What makes it sticky.** It had somewhere better on that band. Either is enough:
- **Another AP heard it better:** 10 dB or more stronger at the middle, over 10 minutes or more.
- **It moved late:** its next minute after one there was on another AP, within 5 minutes, and 15 dB or more stronger. The client was in that AP's reach while it held on.

**What may help it,** said for each:
- **Pin to the AP:** for a client seen on one band only, whose better place is the same AP each time.
- **A rule of the nearest AP:** for any other sticky client. The rule follows it.
- **Network not steered here:** no rule and no sticky acts on a network that isn't steered. Turning steering on restarts the network's Wi-Fi, and is a person's call.
- **Has a rule; held on anyway:** its steering record says what came of it.
- **No AP hears it better:** coverage, not steering.
- **Pinned here:** as a person asked. Or **pinned here; moved?**, where its signal there over the last half hour fell 10 dB or more below its usual to -78 or weaker.

**Each row also says:**
- how long it was weak and its middle signal there, with the last half hour's beside it where it differs;
- the better AP and its signal, with how many late moves;
- whether it is there now;
- the bands it was seen on ("2.4 GHz only");
- its rule;
- whether its network is steered and sticky there;
- what steering asked of it (0139).

### Pinning

- **"Pin to an AP…"** is the client steering pulldown's choice that keeps a client on one AP (it was "One AP…"). The client's page says "pinned to" it, not "steered to".
- **Choosing it greys out the steering that picks an AP,** struck through: Nearest AP, Sticky: heard better, Sticky: weak. None of these moves a pinned client. Bands still choose its band on that AP. The steering daemon already behaves this way (0289, 0319); the form now shows it.

### A pinned client that moved: `pin-moved`

A warning on the AP it is pinned to, keyed `pin-moved:<mac>`, where both hold:
- over the last half hour, 20 minutes or more heard it there at -78 dBm or weaker at their middle;
- that is 10 dB or more below its usual there: the middle of its minutes over the week before the last hour, of 60 at least.

A pin is a rule naming the AP, not one of the nearest. Where it did better in the last day on another AP (10 dB, 10 minutes), the warning says so. For example: "Patio switch (14:63:93:66:a7:98) is pinned here, and over the last half hour it was heard on 2.4 GHz at -84 dBm, 29 dB weaker than its usual -55 dBm here: it may have been moved, and be pinned to an AP that is no longer its best. In the last day it did better on Living (2.4 GHz), at -50 dBm over 20 minutes. Its pin is in its Steering".

## Consequences

- **The clients that hold on are seen, by name,** with what may help each. The rule is still a person's to make.
- **A pinned client's warning goes by its signal on its own AP.** The pin keeps it there, so other APs seldom have minutes of it. What the other APs hear of it by probes and scan radios (0289, 0321) stays on the APs, and the manager does not have it.
- **It is a sign, not proof:** a client that turned its radio down, or was boxed in, reads the same.
- **Not done here:**
  - A client's live readings by the other APs, sent to the manager, so "heard better now" stands beside "did better then". That is the agent's to send, 22 readings to a hello as RRM shares them (0289).
  - A button on the list that makes the rule. For now the client's Steering does.
  - Asking a pinned client for 802.11k reports, which can move it nowhere: the steering daemon still asks one that is weak on a sticky network.
