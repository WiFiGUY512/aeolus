# 0150. Whether a client uses 802.11r

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff, of the client list: "why is there a ? next to r under the features". Written up by Claude
- Refines: 0067, 0113

## Context

- **A client's features are shown as chips (0067):** k, v, w, and "r?". The first three are read from what hostapd says of each client on ubus.
- **hostapd says nothing there of 802.11r:** whether a client's keys are by a fast transition is in which key management it chose, which ubus does not give. So the chip said it was not known, on every client, always.
- **hostapd's control socket does say:** `STA <mac>` answers with `AKMSuiteSelector`, the key management chosen. On the C-360, 2026-10-13, 23 of its 24 clients had chosen a passphrase's (`00-0f-ac-2`) and one a fast transition's with a passphrase (`00-0f-ac-4`).
- **The prober asks that of a client already,** once a session, for who it signed in as (0113), on Aeolus's networks.

## Decision

- **The prober reads the key management from the same answer,** and asks it of every client, on every network, once a session.
- **A client's connection uses 802.11r where the key management is a fast transition's:** over 802.1X, with a passphrase, with SAE, over 802.1X with SHA-384, or with SAE's longer keys (3, 4, 9, 13 and 25).
- **It is reported with the client's other features:** `r`, true or false, and not there where hostapd did not answer.
- **The chip is r, lit or not,** as k, v and w are. It is "r?" only where an AP's agent does not say, which is one from before this.

## Not done

- **It says what the connection uses, and not what the client can do:** a client that can roam by 802.11r on a network that does not offer it shows it unlit, as with the others.
- **Nothing is counted of it:** how many clients of a network use it, or which never do.
