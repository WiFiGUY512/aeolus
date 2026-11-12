# 0336. Invites: a link to join by

- Status: Accepted (Griff, 2026-11-06: "but yes we need register page links", "build that")
- Date: 2026-11-07
- Proposed by: Griff, after asking for a way in on a new browser: "can you just create a register account on the web page?". Written up by Claude
- Refines: 0176 (accounts run from the manager), 0177 (roles and rights)

## Context

- **An account is made by an owner, with a temporary password (0176).** The owner reads it off the page and tells it to the person another way. The person changes it at their first sign-in.
  - That puts a password in someone's hands for a while: a chat, a text, a note.
  - The owner chooses the person's sign-in ID for them, and gives their roles on a second page.
- **There is no way to join on your own.** Open sign-up has no place on a manager that runs a network: anyone who reached the page could make an account.
- **Griff wants a link.** An owner makes it, sends it, and the person registers with an account and a password of their own.

## Decision

- **An invite is an owner's, made in their own name.**
  - It says the roles it gives, each on a folder in one tree, at most 16; or that it makes a super admin; or both.
  - It is good for 1 to 30 days, 7 unless said, and once only.
  - It may carry a note, at most 200 characters, for the owner's list and the person's page.
  - Admin › People: **Invite someone** makes one; the Invites panel lists them, each open one revoked in place.
- **Its link carries its secret, and only the link does.**
  - The link is `https://<manager>/#/register/<id>.<secret>`, the same shape as an API token's ID and secret.
  - The manager answers it once, when the invite is made, and keeps only a SHA-256 hash of the secret (32 random bytes, so a fast hash is enough).
  - It sits in the address's fragment, which a browser never sends to a server, so no log or proxy sees it in a path.
- **The register page is public, and the code is the credential.**
  - `GET /v1/register/{code}` says who sent it and what it gives.
  - `POST /v1/register` takes an account, a name and a password. The account is 2 to 32 characters (lower case letters, digits, `.`, `_` or `-`, from a letter); the password follows 0176's rules.
  - The person is then signed in, as at the sign-in page, with a password that was never anyone else's.
  - A code the manager never made counts as a failed try from the address, as a wrong password does: five lock the address for a minute and longer (0176).
- **Three changes, all through the change log.**
  - `add-invite`: an owner, naming themselves as its maker. The API makes the secret, so the change is refused through `/v1/changes`.
  - `revoke-invite`: an owner.
  - `use-invite`: only the manager, in its own name, from `POST /v1/register` after checking the secret. No account may make it, an owner included.
    - It makes the account, its password (to keep, not to change), each role and owner if the invite says so, and marks the invite used.
    - It is all of that or nothing: a taken account, or a folder gone since the invite was made, leaves the invite open and no account made.
- **What the clock decides is the commit check's, not Apply's**, so the log replays the same whenever it is read.
  - An invite is used only before it expires.
  - It is used only while its maker is still an owner who can sign in: it speaks for them. An owner removed or unmade leaves their invites unusable, and another owner makes new ones.
  - It is made to expire from now, for at most 30 days.
- **Neither the log nor any page shows a secret or a hash.** A use is logged as the manager's, with the reason naming the account and the invite's maker. The sign-in record says `registered with an invite`.

## Consequences

- **An owner sends a link, not a password.** Adding a person with a temporary password stays, for when there is no other way.
- **The link is as good as a password until it is used or expires.** Send it to the person alone, and revoke it if it goes astray. Once used, it is worth nothing.
- **Only owners invite, as only owners add accounts.** An admin or a manager inviting to the folders where they may give roles (0177) is for later, if wanted. The roles an invite holds are kept as a list for that reason.
