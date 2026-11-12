# 0233. A client that struggles

- Status: Accepted; refined by 0236 (resends are not struggling), withdrawn by 0242, which puts resends back and slides the rate's bar with the signal
- Date: 2026-10-28
- Proposed by: Claude, from someone's Phone-B Ultra; Griff: "keep going on the struggling device alert". Written up by Claude
- Refines: 0099 (alerts), 0103 (a client's journey), 0153 (a name a person gives a client), 0190 (rejoins), 0207 (minutes)

## Context

- **Someone in the house said the Wi-Fi was bad, for days, before anyone looked.** When someone did, Aeolus had all of it already:
  - **Back and forth:** her phone moved between the Living Room AP's 5 and 6 GHz every 10 to 30 minutes overnight, 17 times in six hours.
  - **Resends:** on that AP's 5 GHz, 30% of the frames sent to it were sent again.
  - **Slow:** it ran at 97 Mbit/s there, heard at -55 dBm, where the same phone ran at 271 on another AP's 5 GHz at the same signal.
  - The cause was the AP's channel, 169, in U-NII-4 (0232).
- **Nothing raised it.**
  - The rejoins alert (0190) wants 20 joins an hour, and a band move every quarter hour is far from that.
  - A client's journey (0103) lists weak signals, retries and slow rates, but only to someone who opens that client's page.
- **Most clients are not someone's,** and run slowly by nature: a plug on 2.4 GHz at 20 MHz, one spatial stream. Alerting on every slow client would be noise.

## Decision

- **A client a person named is watched:** the phones, laptops and TVs someone took the trouble to name (0153). The others are not.
- **It struggles where, over the last two hours, any of these holds:**
  - **It moved between one AP's bands four times or more:** let on to one band there, then to another. Moves between APs are walking about, and do not count.
  - **Of what was sent to it in its busy minutes, 30% or more was sent again,** over ten busy minutes or more. A busy minute is one it moved a megabyte in.
  - **On 5 or 6 GHz it ran slowly where it heard the AP well:** over ten busy minutes or more on the band, it ran under 120 Mbit/s, heard at -65 dBm or better, each the middle of its minutes.
- **A struggling client is a warning on the AP it is on,** heard there in the last ten minutes: `struggling`, keyed by its MAC. It says each reason, and since when the first began. It ends once none holds, and one that has gone has no AP to be on.
- **The alert leads to the client's journey,** as a rejoins alert does.

## Consequences

- **A person's device that has a bad time says so** within two hours, without anyone looking for it: a phone would have been an alert the first evening.
- **Naming a device is how to have it watched.** One no one named never raises this alert.
- **A client that walks about is not a struggling one:** only moves between the bands of one AP count, and a client between two APs is the rejoins alert's (0190), or the journey's back-and-forth.
- **The alert's thresholds are the lab's first guess.** They are changed in code, after a while of seeing what they raise.
