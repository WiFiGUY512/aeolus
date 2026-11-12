# 0161. Adverts read off the scan radio

- Status: Proposed
- Date: 2026-10-14
- Proposed by: Griff, on the neighbour map showing the two C-360s as strangers: "why arent the 3360s formining neighbor relationships"; then "there has to be a way to encode the wire side IP address of an AP in the airspace hello, OR hear a hello and respond with a wireside ip address", and "I want this to be able to work without the manager being online". Written up by Claude
- Refines: 0073, 0116

## Context

- **An AP learns of its radio neighbours from the air** (0073): each of its radios' beacons carries an Aeolus advert, a vendor element with its AP ID and management address. An AP that hears one sends that address its hellos, and once hellos go both ways they are neighbours.
- **An AP with a scan radio never scans with the radios that serve clients** (0081). It listens through the scan radio instead (0116): airscan's list of the networks it hears.
- **That list has no elements, so no adverts.** Such an AP knew a neighbour only by the BSSIDs that neighbour's hellos carry, and so only once the neighbour had heard *its* advert and said hello first. An AP that scans for itself does; another with a scan radio does not.
- **So two C-360s never met.** On 2026-10-14 the two, side by side, heard each other at -7 dBm on 2.4 GHz and -18 dBm on 5 GHz, and each listed the other's networks among the networks that are not Aeolus's.
- **The adverts are in the air for the scan radio to hear.** Its monitor interface, which airscan keeps, heard the living-room C-360's advert 57 times in 25 seconds.
- **Griff's condition:** it must work without the manager.

## Decision

- **An AP with a scan radio reads the adverts off the scan radio's monitor interface itself,** with a packet socket whose kernel filter passes beacons only, the first kilobyte of each. Each advert heard says where an AP is, by the BSSID it came on.
- **A radio known so is matched in airscan's list as one known by a hello is,** its band being airscan's: the AP is heard, with its signal, on each band airscan heard its networks, and gets hellos as any AP heard. The other answers, and they are neighbours.
- **Nothing of it asks the manager.** It needs no change to airscan or to the firmware: only the agent.
- **A socket that fails, as when airscan makes its interface again, is opened again at the next look.**

## Consequences

- **Two APs with scan radios become neighbours, as all others do,** and radio resource management plans their channels and powers with each other in view.
- **Only one side needs this to begin:** an AP that is said hello to answers, so the pair holds once either has it.
- **The AP reads about forty beacons a second on the lab's air,** most without an advert.
- **Tried, 2026-10-14, on C360-AP,** the changed daemon run by hand with no moves possible: it read the living-room C-360's advert in its first twelve seconds, and the two were neighbours, up, 40 seconds after it started, hearing each other at -8 and -7 dBm on 2.4 GHz and -17 and -18 dBm on 5 GHz.

## 6 GHz

- **airscan heard a 6 GHz network about one look in three,** so on 6 GHz the pair's signals came and went, and the neighbour map often showed none. Its scan radio's driver, ath11k, listens on each 6 GHz channel for 30 ms in a passive scan, against 150 ms on 2.4 and 5 GHz, and a beacon comes every 102.4 ms. Of four looks in a row on C360-AP, one heard the living-room C-360's 6 GHz network, one heard C360-AP's own, and two heard neither.
- **Mended in airscan, not here** (airscan 0.2-r2, in the C-360 port): each look scans the 6 GHz channels again, 120 ms on each, about 8 s more. Nothing changes in the agent.
- **Tried, 2026-10-14, on C360-AP:** each of the five looks after the change heard both 6 GHz networks, the living-room C-360's at -37 to -43 dBm, and the pair's 6 GHz signals stood at -39 and -38 dBm, one each way. The living-room C-360's own looks hear C360-AP's 6 GHz network only now and then until it has the new airscan too.
- **Why it matters:** the 6 GHz signal between neighbours is what an AP will size its cell by on that band, and a signal that is there one look in three cannot be.

## Frames spoilt in the air (2026-10-16)

- **The scan radio hands up frames that failed their FCS check.** On C360-AP's monitor interface one beacon in ten to eighteen carries radiotap's Flags 0x40 (85 of 1494 in 45 seconds; 4841 of 48592 in half an hour, 407 of them with an advert), and an advert read from one can be wrong: aeolus-rrm logged ap-2005b60fe0f7 at 60.21.234.228 for ap-2005b6df64e0 at 192.168.1.69, and 9 of the 34 such lines in one log were made up. airscan had the same that day, and drops them now (in the C-360 port).
- **Where a made-up AP could go.** An advert holding a radio's BSSID when a look of airscan's is taken in makes its AP heard for 20 minutes, at the address it names: it may be chosen among the three heard best on a band, be sent hellos every ten seconds, put a real neighbour out of the choice, and be reported to the manager.
- **On the lab it did not.** None of the C-360's 1004 reports to the manager since this decision went in names one, and none of the 855 hellos it sent in 48 minutes, watched with an nft rule, went to another address than its three neighbours'. A neighbour's hellos carry the BSSID its advert comes on, so they claim that BSSID first, and on the lab every neighbour's hellos come. They do not at first contact, for an AP forgotten 15 minutes after it went down, or for one under another manager's key; there it could.
- **Mended:** a frame whose radiotap Flags say its FCS failed is passed over, the Flags read where radiotap puts them (after every present word, and after TSFT, eight bytes on an eight, where there is one: 16 bytes in on the C-360). And an advert from a radio counts once the same is read twice in a row; one read once beside a sure one waits until it is read again, as when an AP's address changes. So a spoilt frame that is not caught, as from a driver that does not say, never names an AP. A radio beacons ten times a second and the scan radio listens a second on each channel, so a real AP is hardly held up: the weakest lab AP, at -86 dBm, was sure 27 seconds after the daemon started.
- **Tried, 2026-10-16, on C360-AP.** The changed daemon ran 35 minutes by hand, moves off: no errors, its three neighbours up throughout, each real AP's radios logged heard once as their adverts became sure, where v0.115.0 logged a made-up one and the real one again every few minutes. Beside it, for 29 minutes, a reader that sends nothing ran both ways on the same frames as if no neighbour's hellos came. The old way took in three wrong ones and chose each among the three heard best on 2.4 GHz, so hellos would have gone to them: ap-a0046082275e, a real AP, at 192.48.118.81; and ap-a004005f36de at 190.168.1.38 and ap-a07cb702365e at 192.168.1.38, neither of them there. The new way took in none, and heard the same real APs.

## An advert read once (2026-10-31)

- **The spoilt frames were an OpenWrt fault** (faults list entry 30, B64): the C-360's ath11k handed up FCS-failed frames to a monitor that never asked for them. Port 0054 fixed it, and it has been on all three C-360s since 2026-10-31, in port -77: bad-FCS frames on the monitor went from 11% to none.
- **The C-260's scan radio is an ath10k,** which 0054 doesn't touch. The porting session measured it on -76 the same day: 6,000 frames on three channels, none spoilt, flagged or not. Office and Shed have no scan radio. So no scan radio in the fleet hands up a spoilt frame.
- **So an advert counts at its first read** (v0.204.2), and an AP's new address at its first read too. The read-twice rule worked around an OpenWrt fault, and stopgaps go once the fix is on the APs (faults list). A frame whose Flags say its FCS failed is still passed over: that is reading radiotap right.
