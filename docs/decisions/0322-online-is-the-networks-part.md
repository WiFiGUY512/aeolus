# 0322. Online is the network's part

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Griff, of the Assurance panel's "Clients coming online": "devices come online far faster than the 'Typically' suggests", then "maybe an Online and a reached internet", "or even 3: online, gateway and DNS". Written up by Claude
- Refines: 0118 (a client's attempts), 0127 (how clients' own joins go), 0130 (the way online a point at a time)

## Context

- **An attempt is online (0118) once the client has its address and something beyond it answered:** its gateway's ARP, a DNS answer, or a TCP connection. Its time, `took_ms`, ran from its authentication to that answer.
- **That time was the device's, not the network's.** An IoT device may say nothing for seconds once it has its address. The Garage Door Opener (f0:ad:4e:46:a9:1f) at 11:48 CST on 2026-11-05:
  - authenticated at 0 ms, and had its keys at 22;
  - was given its address at 36;
  - asked for its gateway at 7,963 ms; the gateway answered in 1 ms;
  - its attempt "took" 7,964 ms.
- **The panel said Home_Wi-Fi_IoT came online in "typically 4.8 s",** the middle of the clients' averages. On that network's way, the middle steps were 5 ms to associate, 16 for the keys, 21 for DHCP, 1 for the gateway and 33 for DNS.
- **Phones ask at once,** which is why Home Wi-Fi read 413 ms.

## Decision

- **An attempt's time to come online is how long the network took to do its part:** to let the client on, and to give it its address where it asked for one by DHCP. A client that kept its address from before is ready once let on.
  - What answered it after, its gateway or a name or a server, still says it is online, and when it came stays in its record. It no longer sets the time.
  - The agent's record says so: for an online attempt, `took_ms` ends at the DHCP answer (`ready`), or at being let on.
  - An attempt that failed, or that the client left, is timed as before.
- **The Assurance panel's line says four times, each the network's:**
  - **online:** the middle of the clients' times to come online, as above;
  - **gateway:** how fast the gateway answered when asked, the middle of the attempts' way (0130);
  - **DNS:** how fast a name was answered when asked, likewise.
  - **Internet:** how fast a connection the client opened to the internet was answered, likewise. Griff: "call online simply IP traffic, then if we are measuring a complete internet connection call it Internet".
  - The line reads like "48 of 48 got on · online 40 ms · gateway 1 ms · DNS 1 ms · internet 55 ms". The ESP 14:63:93:66:a7:98's first attempts after the release were online at 39, 39 and 42 ms, where its last before it had taken 1,257.

## Consequences

- **Records made before the agent's release keep their old times.** The panel's 24 hours are wholly the new reckoning a day after it.
- **A client that is slow to ask for its gateway or a name shows as quick**, as the network is quick for it. Its own pause is in its record, step by step.
- **Checked:** agent/test/journey.uc with 25.12's ucode. The phone's attempt is online at 230 ms, its DHCP answer, where its first answer beyond it came at 302. The roam is 31 ms, against 120.
