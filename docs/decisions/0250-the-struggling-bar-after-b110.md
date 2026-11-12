# 0250. The struggling bar after B110

- Status: Accepted (Griff, 2026-10-30: "recheck the struggling thresholds now that -62 is on")
- Date: 2026-10-30
- Proposed by: Griff. Written up by Claude
- Refines: 0242 (slow is said)

## Context

- **B110:** on the Arista ports, ath11k read a client's signal low by the channel's noise floor plus 95 dB. Port -62 (0040) puts it right. It was flashed on 2026-10-29 to the Living Room AP (21:13Z), ce7f (21:34Z) and C360-AP (21:40Z).
- **What it changed, read the evening after:**
  - **The noise floors of the channels in use:**
    - 5 GHz: -90 dBm on the Living Room (60) and ce7f (100), -92 on C360-AP (116);
    - 6 GHz: -91 or -92;
    - 2.4 GHz: -94 or -95.
  - So readings are now 3 to 5 dB higher on 5 GHz, 3 to 4 on 6 GHz, 0 to 1 on 2.4 GHz.
  - **Clients heard in the three hours either side of a flash** moved as much:
    - 2.4 GHz: -3 to +2, mostly +1;
    - 6 GHz: +2 on the Living Room, +4 on C360-AP.
    - No 5 GHz client was heard long enough either side.
  - **The C-260 runs ath10k, which B110 is not about,** nor the Office AP or Shed.
- **0242 set the struggling alert's rate bar on ath11k's readings before the fix:**
  - The Living Room's faulty radio (B101): named clients ran at 32 Mbit/s, heard at -66 to -70 dBm, the middle of a week. Put right, about -61 to -65.
  - ce7f's healthy radio: clients ran at 34 Mbit/s heard at -81 to -85, under 0242's floor of -80, and so were not judged. Put right, about -76 to -80, where 0242's bar is 22 to 48 Mbit/s.
- **The week replayed:** every client's busy 5 GHz minutes from 2026-10-22, named or not, judged by the rate rule over two hours at each half hour.
  - **0242's bar, on the readings as they were:**
    - the Living Room before the flash raised 46 windows, 42 of them named clients: a phone and the Living Room Television, through B101;
    - ce7f raised 6, none of a named client.
  - **0242's bar, on ath11k's readings put right** (+5 dB on the Living Room and ce7f, +3 on C360-AP):
    - ce7f raises the Living Room Television 13 times, on a link that was well;
    - the Living Room raises 65.
  - **The bar moved 5 dB higher, on the readings put right:** the same as 0242's on the readings as they were. That is 46 and 42 named on the Living Room, 6 and none named on ce7f, and nothing more anywhere.
  - **The four hours since the flash** hold only 4 windows to judge, all on ce7f, none slow: too few to set a bar by.

## Decision

- **The rate bar moves 5 dB higher:**
  - a client heard at -60 dBm or better is slow under 120 Mbit/s;
  - less well, the bar falls 6.5 Mbit/s a dB, to 22.5 at -75;
  - heard less well than -75, no bar is set.
- **The rest of 0242 stands.** Resends (30%) and band flips read no signal, and B110 does not touch them.
- **slow-radio needs no change.** A tester's usual is of tests that heard the radio within 6 dB, and the fix moved its readings 0 to 5.

## Consequences

- **The struggling alert judges as 0242 meant, now that the readings are right.** B101 is still said, and a far client on a healthy radio still is not.
- **On the Office AP, Shed and the C-260, whose readings B110 never touched, the bar is 5 dB less strict.** 0242's numbers were drawn from ath11k alone, so this is what they meant there too.
- **One unnamed client on the C-260, ec:8e:77:71:c7:f6, is slow for its signal under either bar:** 44 to 56 Mbit/s heard at -66 to -69 dBm on 2026-10-28. It is not named, so it raises nothing.
- **To be judged again** when a few days of busy 5 GHz minutes since the flash are in.
- **Steering (0138), checked the same night, stays as it is:**
  - Its numbers were never drawn from ath11k's readings. They mean the signal as it is, which -62 now reads.
  - What it reads on 2.4 GHz, to ask a client up from there or hold one off, moved 0 to 1 dB.
  - What it reads on 5 GHz moved 3 to 5 dB. More clients are now near enough at -60 dBm to be asked up to 6 GHz, and fewer are weak under -75. That is what those numbers meant.
  - Since 2026-10-21 it steers only aeolus-50 (Griff's changes 425 and 426), which no client joins. So nothing it did can be seen either side of the flash.
- **The clients' and testers' signal words** read the signal too, and are to be checked against B110 apart.
- **Tested:** `slowUnder` at -50, -60, -66, -75 and -76. Heard at -66 dBm, 60 Mbit/s is slow under 81 and 100 is not. Heard at -78, 20 is not judged.
