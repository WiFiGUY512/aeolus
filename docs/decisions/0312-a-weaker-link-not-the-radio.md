# 0312. A weaker link, not the radio

- Status: Accepted
- Date: 2026-11-05
- Proposed by: Claude, from an ESP32's rejoins at the Shed (2026-11-05)
- Refines: 0190 (a client that comes back again and again), 0200 (where else it did)

## Context

- **The rejoins alert blames an AP's radio where the client stayed on another AP in the hour** (0200): "On Garage-C-360 it stays, so something of this AP's radio does not suit it".
- **On 2026-11-05 it blamed the Shed's radio for an ESP32** (14:63:93:66:a7:98, AWS IoT over MQTT on 8883):
  - **What it did:** it came online at the Shed 25 times in an hour, about every 40-60 s, from 03:26 CST. That was when the C260, where it had held for 19 hours, restarted for port -120.
  - **Where it held:** on the Garage it held for 16 minutes, until porting's module swap took the Garage down at 04:02, and then it was back at the Shed.
  - **The signals:** the Shed heard it at -79 dBm, the Garage at -62 and the C260 at -52.
  
  The link was 17 dB weaker than the Garage's, and at the edge of what Griff calls weak (-80).
- **Each attempt's record already says how strongly its AP heard the client** (`signal`, 0118).

## Decision

- **The rejoins alert, and the journey's rejoins issue, take how strongly each AP heard the client as it came online:** the middle of its times online at each AP (`journey.Heard`).
- **Where the client stayed on another AP that heard it 10 dB or more more strongly** (`WeakerBy`), and this AP heard it at -75 dBm or under (`WeakHere`), the alert names the weaker link instead of the radio. It reads: "On Garage-C-360 it stays, heard at -62 dBm there against -79 dBm here: a link 17 dB weaker is the likelier cause, not this AP's radio. It comes back here as its own choice".
- **Where it stays on another AP heard about as well, the radio is still blamed,** with how strongly this AP hears it: ". It is heard at -79 dBm here". The same signal is added where nothing on another AP tells yet.
- **Where it did the same on other APs, nothing changes:** that points beyond the radio already (0200).

## Consequences

- **A client that picks a far AP by itself,** as an ESP32 on its fast scan does, isn't put on that AP's radio.
- **Steering is how such a client is moved,** not a radio fix. Sticky steering on its network (0289) is a person's choice for an IoT network. The alert says the client comes back as its own choice, and leaves the choice to them.
- **Records without a signal,** as from APs whose journey daemon doesn't say it, keep 0200's wording.
