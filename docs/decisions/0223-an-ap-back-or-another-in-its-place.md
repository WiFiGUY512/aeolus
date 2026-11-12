# 0223. An AP back, or another in its place

- Status: Accepted
- Date: 2026-10-27
- Proposed by: Griff, when the C-260 came back from a reflash and had to be removed and adopted again: that should not be needed. If an AP is replaced, the new one should be merged into the old one's place, keeping all its records. Written up by Claude
- Refines: 0033 (AP enrollment and identity), 0038 (enrollment and the poll)

## Context

- **An AP is known by an ID made from its MAC** (0038). Everything kept of it is kept under that ID:
  - its place, its name and what is set on it;
  - its config versions and the change log;
  - its reports and minutes;
  - its clients' attempts and sessions;
  - its alerts.
- **It proves who it is with a token** it was given when it enrolled (0033). Several things lose that token: a reflash with a fresh config, a factory reset, a new flash card.
- **The agent then enrolls again, and the manager refused.** It answered 409 for an ID it knows until a person removed the AP, and removing it takes what was set on it (0038).
  - On 2026-10-27 the C-260 came back on OpenWrt with a fresh config.
  - It was removed (change 465), enrolled, adopted (467) and renamed by hand (468).
  - Its maintenance setting went with the removal.
- **A failed AP is replaced by another box with another MAC,** so another ID:
  - it lands in Landing Zone as a stranger;
  - everything kept of the old one stays under an ID no AP answers to;
  - whatever was set on the old one has to be set again.

## Decision

### An AP that enrolls again

- **An AP the manager knows enrolls again, and is not refused.** The manager knows it by:
  - the MAC its facts say, which is the one it enrolled with, or, after a replacement, the new box's;
  - else the ID its MAC makes.
- **One adopted keeps its place, name and token, and waits with the new one** until a person lets it back in:
  - its new token gets it only `unassigned` from its polls, as Landing Zone's do, with `waiting: true`;
  - every other AP route answers 409, not 401, so the agent keeps the token;
  - its old token works on until then;
  - what it said of itself is kept beside its facts, as `again`.
- **Not by itself:** a MAC is no secret, every frame carries it, and an AP's config carries its networks' passphrases. Enrolling in an adopted AP's name gets nothing until a person says it is that AP.
- **Enrolling again once more replaces the token it waits with.**
- **`readmit-ap` lets it back in** (the AP's page, Let it back in…):
  - the token it waits with becomes its own, and its other tokens stop;
  - what it said then becomes its facts;
  - its place, name, settings and records are as they were;
  - it takes the aps right on the AP, as removing it does.
- **One still in Landing Zone takes its entry again:** the new token in place of the old, and its facts. Nothing was given to it yet, and adopting it is already a person's call.
- **While it waits:**
  - its alert is `again`, critical, in place of `offline`: "enrolled again from 192.168.1.168, its token gone as after a reflash or a reset: it waits for a person to let it back in, on its page";
  - its page has a banner saying where it enrolled from, and what it says now that differs (its MAC, model, board, OpenWrt), with the button;
  - Landing Zone lists it under Asking to come back;
  - the tree marks it `again`.
- **The agent says so in its log:** "enrolled again as …; it waits for a person to let it back in". With a manager before this, it waits an hour on a 409, as before.

### Another AP in its place

- **`replace-ap` puts an AP waiting in Landing Zone (`with`) in an adopted AP's place (`node`).** The AP keeps its ID, name, place, settings and every record kept under its ID.
- **What moves to the AP:** the new box's token and what it said of itself (MAC, model, board, radios). The new box's entry in Landing Zone goes, with anything set on it. The AP's old tokens stop working, and any either was waiting with.
- **From the AP's page: Replace…,** offered while an AP waits in Landing Zone. The preview says:
  - the box before and after, by model and MAC;
  - what is kept;
  - the config check with the new box's radios: a board whose own port settings do not fit says so there.
- **The change log says both boxes:** the old one's MAC, model and board before, the new one's after, and the Landing Zone entry it came from. The AP's history reads on across the swap, which that entry marks.
- **Only one waiting in Landing Zone,** and only into an adopted AP's place:
  - one adopted already has records of its own, which this does not merge;
  - one in Landing Zone has no place to take: it is adopted instead.
- **Rights:** aps on both.

### What follows from the ID

- **An AP's ID no longer says its MAC:** after a replacement, `ap-<old MAC>` is the new box. Its facts say its MAC. An AP saying what it is again (0122) must say the MAC it is known by now, the new box's.
- **RRM's adverts and hellos carry the AP's ID, which its config gives** (0073). The new box advertises the old one's ID, and its neighbours know it as the AP whose place it took.
- **The old box, plugged in again,** has a token that stopped working and a MAC that is no AP's now. It enrolls as a new AP. Its own ID is the AP's, so it takes the next free one, `ap-<MAC>-2`. Adopted so, its RRM is off: an advert carries twelve hex digits. Put in another AP's place, it is that AP.

## Consequences

- **A reflashed AP comes back as it was in one click.** The C-260's remove, adopt and rename of 2026-10-27 is no longer needed.
- **A failed AP is replaced in one change from its page,** keeping everything kept under its ID.
- **An impostor with an adopted AP's MAC gets a token that gets it nothing,** and a critical alert names where it enrolled from. A person who lets it in anyway gives it the AP's config, as adopting a stranger does.
- **Not done:**
  - replacing with an AP already adopted, whose records would have to be merged;
  - swapping two adopted APs' places;
  - an RRM ID for an AP whose ID is not twelve hex digits.
