# 0212. An advisor for each folder that has a key

- Status: Accepted
- Date: 2026-10-23
- Proposed by: Griff, having set a key at Home: "I want it to be MSP friendly and allow multiple key and work at the folder level where its set and down". Written up by Claude
- Refines: 0205

## Context

- **The advisor was the Org's (0205).**
  - It took its settings from the Org alone.
  - It read the whole Org as `aeolus-advisor`, a viewer at the root.
  - Only those who see the whole Org saw what it found.
- **A key set at a folder did nothing.** Griff set one at Home; the Advisor page asked for one at the Org.
- **One provider, many customers.** An MSP runs one manager for many customers, a folder each.
  - Each customer may pay for its own reviews, with its own key.
  - What one customer's network shows, its clients' MACs and names among it, must not go to Anthropic under another's key.
  - Nor may it be shown to another customer's people.

## Decision

- **A folder that sets `system.advisor.key` itself has an advisor of its own.**
  - It reviews that folder and what is below it.
  - A folder below with a key of its own has its own advisor, and this one reads nothing of it.
  - Its other settings (enabled, model, every, web, budget) resolve at its folder: set there, or inherited from above.
  - The Org is a folder like any other: a key at the root reviews what no other advisor does.
- **A review reads as `aeolus-advisor`, held to its folder.**
  - In Locations: the folder and below, but the folders with an advisor of their own.
  - In Services: the folders its APs are given, with those above them, whose settings they inherit, and below them.
  - The account is still one viewer, set up once by an owner. The hold is the API's own, on the request the manager makes of itself, in the one check every read makes (`may`), so a read outside the folder is not there, whatever the account's roles.
- **Reviews and findings are each folder's.**
  - A finding's key names a problem within its folder's findings alone.
  - Each folder's advisor is shown to whoever may view that folder.
  - An admin of the folder asks it for a review.
  - An operator of the folder dismisses a finding, or opens it again.
- **One review runs at a time,** whosever: the schedule starts the next due at its next look, every ten minutes.
- **The reviews and findings from before are the Org's.**

## Consequences

- **An MSP can bill each customer's reviews to the customer's key,** and none of a customer's data reaches another's key or another's people.
- **The Advisor page picks a folder where there are more than one,** and says how to set one up where there is none.
- **A key at the Org and one at a customer's folder** make two advisors that read nothing of each other. The Org's covers what is left.
