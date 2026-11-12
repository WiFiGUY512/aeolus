# 0323. Suggest a feature

- Status: Accepted (Griff, 2026-11-05, through the main session: "once we get to distribution, we need a suggest FR in the upper right hand corner so we can take FRs from the field as people have them", and "dynamically add the screen they are on to the FR upload as well so we know where they are at"; then, asked: saved and then sent to him, anyone signed in, page details only)
- Date: 2026-11-05
- Proposed by: Griff. Written up by Claude, in the distro session (0316)
- Refines: 0069 (running without the internet), 0176 (accounts), 0205 (the advisor)

## Context

- **Aeolus is going to people outside the lab** (0316). Their feature requests should reach Griff as they think of them, from the page they are on.
- **Requests now go in as issues on Clio** (README), which is private and on the LAN. Someone in the field cannot reach it.
- **The cloud rule (0069) holds:** asking for a feature must never need the internet. Sending a request on is not command and control, so it may use the internet, but must wait out its absence.

## Decision

### On every page

- **"Suggest a feature" sits at the top right,** beside My account, for anyone signed in.
- **Its form asks two things:** what they would like Aeolus to do (up to 4000 characters), and how to reach them (optional).
- **It shows what goes with it before it is sent:** the page, as its heading and route (`#/aps/<id>/…`), and the manager's release. These are taken from the page as it is, not typed.
- **It says where the request goes:** kept on this manager, then sent on to the host named, or kept here for the manager's admins.
- **No screenshot is offered.** A screen can show client names, MACs and addresses (Griff: page details only).

### Kept on the manager first

- **Each request is stored in the conditions store** (`suggestions`, migration 37): who asked, what they wrote, their contact, the page, the release, and its sending.
- **Asking needs nothing but the manager.**
- **At most 20 an hour from one account.** A page's route is kept only if it is one of the manager's own (`#/…`), so Admin's link to it can be no other kind of link.
- **Admin › Suggestions lists them,** newest first, each with whether it has been sent on. So does `list_suggestions` for MCP clients and the advisor. Both are for those who view the Org.

### Sent on where the manager says

- **`AEOLUS_SUGGEST_TO` in `serve.env`** (the serve flag `-suggest-to`) is an http or https URL. Each request is sent there as JSON, once.
  - The JSON: `kind: aeolus-suggestion`, `v: 1`, a `key` that is the same each time it is sent, the Org, the ID, when, the release, the text, the contact, and the page.
  - **Never the account.** Who asked stays on the manager; the contact is what they chose to give.
- **One that does not get there is tried again,** a minute on and twice as long each time, up to six hours apart, while it is not sent. A site without the internet keeps them all, and they go once it is back.
- **Unset or `off`, nothing is sent:** requests stay on the manager for its admins.

## Consequences

- **A newcomer can ask for something from where they are,** whatever their network allows.
- **Where field requests reach Griff is one URL away:** a small endpoint he runs, which could file each as an issue on Clio. Until he names it, field managers keep their requests, and their admins pass them on.
- **A release could carry that URL as its default,** so a new manager sends on without anyone setting it. That is Griff's to say, with where the endpoint lives (0316's open question of a public home).

## Checked

- **`TestSuggestions`:**
  - kept with who asked, the page and the release, the text trimmed and a long heading cut;
  - a script for a route dropped;
  - empty, too long, and a contact too long refused;
  - the twenty-first in an hour refused;
  - read by an Org viewer, and not by one who views a Services folder alone;
  - `whoami` naming the host it goes to, and a URL that is not http or https refused.
- **`TestSendingSuggestions`:**
  - a destination that is down: the request kept, with why, and not tried again before its time;
  - once its time came: sent once, with the text, the page, the Org and a key, and without the account, then recorded as sent.
- **The conditions store's migration tests** take the new table off and put it back.
- **The MCP tool list** includes `list_suggestions`.
- **In the UI harness (`uidev`):**
  - the button at the top right;
  - the form over the page, showing "Sandbox #/locations/sandbox/networks" and the release, with "kept on this manager";
  - Send answered "Kept as suggestion #1";
  - Admin › Suggestions listed two, newest first, the line breaks kept, the contact under the account, the page a link, each "kept here".
