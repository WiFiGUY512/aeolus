# 0254. An attempt from hostapd's frames

- Status: Accepted (Griff, 2026-10-31: "do 1 2 and 3   dealers choice as to the order"; 1 was this: "Cut Aeolus's own log noise")
- Date: 2026-10-31
- Proposed by: Claude
- Refines: 0133 (the keys, a message at a time), 0146 (a failure's frames from hostapd's own rings), 0118 (a client's way online)

## Context

- **Since 0133, aeolus-journey asks hostapd for its debug lines,** on each network that doesn't sign clients in by 802.1X, every 10 s. From them it records:
  - each message of the key handshake, and the AP giving it up;
  - WPA3's commit and confirm;
  - that a client is authenticated, and how;
  - a reassociation, and the AP asking a client it remembers for proof (SA Query);
  - a key kept from before;
  - a client leaving of its own accord.
- **That is 12 to 35% of every AP's log** (2026-10-31): hostapd at debug says three times as much of a join as it needs to.
  - Each AP's log is 128 KB, so it reaches back less far.
  - Every line also goes to the log server.
- **Since 0146, every AP with hostapd's rings has each of those frames as it was:**
  - every management frame of its networks, and every key message, both ways;
  - each at hostapd's own time, to the microsecond, with the radio's word on whether the client acknowledged it.
  - All six lab APs read them.
  - Until now only captures and traced clients used them: records were still made from the log.
- **The frames say some things better than the log:**
  - a deauthentication's or a disassociation's reason;
  - the AP refusing an authentication or an association, and with which status;
  - a WPA3 refusal: hostapd logs nothing of it at info.
  - On a network that signs clients in by 802.1X, they also give the key messages, which the log never gave there, and with no identity in them.
- **What frames alone can't say:**
  - Whether the keys are in place: hostapd says that at info (`EAPOL-4WAY-HS-COMPLETED`, `AP-STA-CONNECTED`).
  - Whether a passphrase was wrong: also said at info (`AP-STA-POSSIBLE-PSK-MISMATCH`).
  - Anything of a sign-in but its messages.
  - Which kind of key from before was taken (0136): a key this AP held, or one made at another AP (OKC). That matters only where clients sign in by 802.1X, where hostapd was never asked for its debug lines either. On a WPA3 network, an association that offers a key and follows plain (open) authentication, not WPA3's exchange, is a key this AP held.

## Decision

- **Where hostapd's rings are read, an attempt's steps come from its frames.** Each frame is taken as the debug line it stands for, worded as that line was, so the manager sees the same steps:
  - **Authentication:**
    - the AP's answer, ok: `authenticated`, by open authentication or a fast transition (802.11r);
    - WPA3's commit and the client's confirm;
    - the AP's confirm, ok: `authenticated`.
  - **The AP refusing an authentication or an association** is a step that failed, with its status. It begins no attempt, as a refused client was never one before.
    - A WPA3 refusal with status 15 is a wrong passphrase.
    - Status 30, an association put off while the AP asks for proof, is said beside the attempt, and fails nothing.
  - **Association:**
    - the AP's answer, ok: `associated`, and after a fast transition, that its keys came with it;
    - a reassociation request, said beside it;
    - on a WPA3 network, a request that offers a key after plain authentication: a key this AP held (`pmksa`).
  - **The AP asking for proof (SA Query):**
    - each request, counted;
    - the association that follows with no answer: "no proof came, as from a client that had left".
  - **Keys:**
    - messages 1 and 3 sent, and 2 and 4 come;
    - a message sent again is a new try.
  - **Leaving:**
    - a deauthentication or a disassociation from the client, or from the AP to it, with its reason.
    - The AP's for a handshake that timed out (reason 15) is giving the handshake up, at the last message it sent: the first or the third.
  - **A frame the client didn't acknowledge:**
    - an answer to its authentication or association: "did not acknowledge the AP's answer";
    - a key message: said, and fails nothing.
- **hostapd's own words at info still come from the log:**
  - the keys in place, and the client let on;
  - a wrong passphrase;
  - the sign-in and RADIUS;
  - hostapd letting go of a client it held.
  - Its lines that the frames now say are left out: authenticated, associated, deauthenticated, disassociated, and "did not acknowledge". So is every line at debug, should hostapd still be logging at debug.
- **Frames come first:** before a line of hostapd's is taken, the rings are read, so each frame is in its attempt before the word that follows it.
- **hostapd isn't asked for its debug lines where the rings are read.** Where it was asked before, it is put back to info once. Where they aren't read, as on an AP whose hostapd must start anew to have them (0146), it is asked as before.
- **The rings are read whether or not captures are on.**
  - Captures off now means no capture is kept, and frames aren't held for one.
  - It no longer stops hostapd keeping its rings, as records are made from them.

## Consequences

- **An AP's log no longer has hostapd's debug lines:** 12 to 35% less of it, and reaching back further. Its log server gets the same cut.
- **A record's steps are timed by hostapd itself,** to the microsecond, where they were timed by the log's milliseconds.
- **Records say more than they did:**
  - why a client left, by its reason;
  - the AP refusing a client, and why;
  - WPA3 refusals;
  - key messages the client didn't acknowledge;
  - on 802.1X networks, the four key messages.
- **On a WPA3 network, a full exchange followed by an association that offers the key it just made is no key from before.** Before, hostapd's debug line said it found the key, and such a join could be counted as a cached one.
- **An AP without rings is as before:** hostapd at debug, and records from its log.
- **The agent changes:** `aeolus/journey.uc` and `aeolus-journey`. The manager is unchanged: the steps are the ones it knows.

## Not done here

- **OKC against a key this AP held, on 802.1X networks:** frames can't tell them apart, and the log never said there.
- **A sign-in's EAP messages as steps:** they are told to a traced client (0146), but in a record they would fill its steps.
