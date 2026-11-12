# 0176. Sign-in with a password, and accounts run from the manager

- Status: Accepted (Griff, 2026-10-20: "merge 41, 42 and 43, then start building 0176"); built in four parts, 2026-10-20
- Date: 2026-10-20
- Proposed by: Griff: "we need to have a u/p and an admin section to aeolus so we can do a full RBAC", with Aeolus now having takers. His choices, 2026-10-20: roles stay per folder, plus an Org owner; the browser keeps a session cookie; an admin resets a forgotten password; no second factor in this first version. Written up by Claude
- Refines: 0024, 0025, 0030, 0042, 0043, 0170

## Context

- **Signing in means pasting an API token** (0042), kept in the browser's storage. When that storage goes, the only way back in is a root shell on the manager (0043). On 2026-10-20 Griff's browser lost its token and he had to get back in that way.
- **An account is an ID and a name** (`access.Account`), made by `create-org` or `add-account` in the change log. It has no password and no status, and it can't be disabled or removed (0030 and 0125 left that open).
- **Roles are viewer, operator and admin, granted on folders** of either tree, and the strongest in a node's ancestry applies (0025). Anyone with admin on any folder may add an account (`AdminAnywhere`), and an admin on a folder may grant or revoke there. No one is above an admin: the first account is admin at both roots, and nothing stops that being revoked.
- **There is no page for people.** The API has no route that lists accounts, grants or tokens, and `whoami` returns only the caller's own. Access shows up only as lines on the Changes page.
- **Sign-ins aren't recorded,** failed tokens aren't counted, and nothing limits how fast a token can be guessed.
- **The building blocks are there:**
  - Go 1.27's standard `crypto/pbkdf2`, which keys.go already uses;
  - token hashes kept in the change log, never the secret;
  - the CSP keeps the UI to its own origin.

## Decision

### Accounts

- **An account has a password, a state, and an owner flag,** all set by changes in the change log, like everything else about it. None of them is ever read back.
  - **`set-password`** keeps a hash and never the password: PBKDF2-HMAC-SHA256 with a 16-byte random salt and 600,000 iterations. It's written `pbkdf2-sha256$600000$<salt>$<hash>`, so the scheme can change later and old hashes still check. `/v1/changes` leaves the hash out, as it does a token's.
  - **A password set by someone else is temporary** (`must_change`). Its first sign-in can only set a new one.
  - **`disable-account` and `enable-account`.** A disabled account's sessions end at once, and its tokens stop working while it is disabled; they work again if it is enabled.
  - **`remove-account`** revokes its tokens, drops its grants and ends its sessions. Its ID is never taken again: the change log still names it on every change it made.
  - **`set-account`** changes its display name.
- **The rules for a password:**
  - 12 to 128 characters, and not the account's ID.
  - Nothing more, following NIST 800-63B: length counts, composition rules don't.
- **IDs stay what they are:** 1 to 32 lowercase letters, digits and hyphens. The ID is the username.

### Roles

- **Viewer, operator and admin stay as they are,** granted on folders of either tree and inherited down it.
- **An Org owner manages people.** `owner` is a flag on the account, set and cleared by `set-owner`, not a grant on a folder.
  - **Only an owner may:**
    - add, disable, enable or remove an account;
    - make or unmake an owner;
    - reset another account's password;
    - issue or revoke another account's tokens.
  - **An owner holds admin everywhere,** whatever its grants.
  - **There is always one active owner:** disabling, removing or unmaking the last one is refused.
- **A folder admin can still grant and revoke roles on its folders,** to accounts that already exist. It can't grant above its own role there, so it can't grant beyond admin. Nor can it make owners. A building's manager can give their staff access to that building without being able to touch anyone else.
- **The Org's first account is its owner,** the one `create-org` names. That holds for an Org made before owners too: its log replays to the same. On Example's manager, that is `griff`, and no one-time change is needed. (This was first proposed as a one-time change by the system actor; the replay does it more simply.)

### Signing in

- **`POST /v1/session` with an account and a password** starts a session.
  - **It sets one cookie,** `aeolus_session`, which is `HttpOnly`, `Secure`, `SameSite=Strict` and `Path=/`. It holds 32 random bytes, and the manager keeps only their SHA-256.
  - **A session ends** after 8 hours without a request, or after 30 days with "Keep me signed in". It also ends when the person signs out (`DELETE /v1/session`), and when their account is disabled or removed, or its password is reset.
  - **Sessions are kept on disk,** in their own table beside the conditions, so a restart of the manager doesn't sign everyone out.
- **A request carries either a session cookie or a bearer token.** One that changes anything with a cookie must also carry an `X-Aeolus-Session` header, which a page from another site can't add. Tokens stay as they are for scripts, the MCP adapter (0028) and the APs, and the UI no longer stores one.
- **Guessing is slowed:**
  - 5 failed sign-ins for an account, or from an address, lock that account or address for 1 minute, doubling to at most 15.
  - A wrong account and a wrong password get the same answer, in the same time.
  - The same limits apply to bearer tokens that fail.
- **Every sign-in is recorded:** who, when, from where, and whether it worked and why not. They go in their own table, kept 90 days, not the change log.

### Recovery

- **An owner resets a forgotten password** from the admin section. It sets a temporary one, to be told to the person by other means, and the person changes it at their first sign-in.
- **Root on the manager host is the last resort,** as now (0043): `aeolus password -account <id>` sets a temporary password, run as the service's user with the service stopped. `aeolus token` stays.

### The admin section

A top-level **Admin** page.
- **People:** each account, with:
  - its ID, name, whether it is an owner, and its state;
  - its last sign-in;
  - its roles at a glance.
  - Owners add accounts, with a temporary password, and disable, enable, remove, and reset passwords.
- **One account's page:**
  - **Its roles:** each grant, by tree and folder, inherited ones shown as such. Grants are added with a folder picker and removed in place.
  - **Its API tokens:** when each was issued and by whom, and when it was last used. An owner issues one, shown once, or revokes one.
- **Sign-ins:** the record, newest first, filterable by account and by result.
- **My account,** for everyone: change your own password, see and revoke your own tokens.
- **Who sees what:** folder admins see the People list with the accounts that hold roles in their folders, and grant there. Viewers and operators see only My account.

### The installer

- **`aeolus init` and `deploy/install.sh` set the first owner's password,** asked twice on the terminal, instead of printing a token. `--token` still prints one, for scripts.
- **The line that joins an AP** is unchanged.

## Consequences

- **A taker signs in with a name and a password,** and an owner runs their people from the page, without a shell. A building manager can be given one building.
- **A lost browser no longer locks anyone out,** since the password brings them back. A lost password is an owner's reset away, and root is the last resort.
- **The change log stays the one record of who may do what.** Sessions and sign-ins are kept beside it, being events rather than intent.
- **Built in four parts:**
  1. **Accounts:** passwords, states, owners, the authorization rules, the one-time making of owners, and `aeolus password`.
  2. **Sessions:** the cookie, the limits, the sign-in record, and the sign-in page.
  3. **The admin section:** the API routes that list accounts, grants, tokens and sign-ins, and the pages.
  4. **The installer and the docs.**

## As built: part 1, accounts (2026-10-20)

- **`internal/access`:**
  - an account has `Owner`, `Disabled`, `Removed`, `Password` (the hash, never marshalled) and `MustChange`;
  - `password.go` hashes, checks, enforces the rules and makes temporary passwords (four groups of four letters and digits that can't be mistaken for one another);
  - an owner is admin everywhere (`RoleAt`, `AdminAnywhere`);
  - a disabled or removed account has no role, and its tokens don't authenticate;
  - the last owner who can sign in can't be disabled, removed or unmade (`ErrLastOwner`).
- **`internal/change`:**
  - the kinds `set-password`, `disable-account`, `enable-account`, `remove-account`, `set-owner` and `set-account`;
  - a `set-password` effect says whether the password is temporary, never what it is;
  - `create-org` makes the owner.
- **`authorize.go`:**
  - owners only: `add-account`, `disable-account`, `enable-account`, `remove-account`, `set-owner`;
  - the account itself or an owner: `set-password`, `set-account` and `issue-token`; `revoke-token` likewise, for the account's own tokens;
  - grants as before, admin on the folder;
  - a disabled account makes no change.
- **The API:**
  - `POST /v1/changes` refuses `set-password`;
  - `/v1/changes` drops the hash;
  - `whoami` says `owner` and `must_change`;
  - the new refusals are 409 (last owner, removed, disabled, no change) or 400.
- **The MCP adapter** takes the new kinds but `set-password`, with `owner` for `set-owner`.
- **`aeolus password -account <id>`** sets a temporary password on the manager host, as the service's user, with the service stopped, and prints it once.
- **The Changes page** says each new kind in words.
- **Tested:** the access rules; the change kinds and their authorization; replay making the owner; the API's refusals and statuses; and the CLI.
- **A password can be set from part 1 on,** but nothing signs in with one until part 2.

## As built: part 2, sessions (2026-10-20)

- **`internal/sessions`:**
  - `sessions.db`, beside the change log (`aeolus serve -sessions`), holds the sessions (the SHA-256 of each cookie's 32-byte secret, the account, keep, created, last used, address, browser) and the sign-in record (`ok` and why not), pruned hourly.
  - A session also keeps a tag of the password hash it signed in with, so a password changed elsewhere ends it.
  - `Limiter` is in memory: a restart forgets it.
- **The API:**
  - `POST /v1/session`, `DELETE /v1/session` and `POST /v1/session/password`;
  - `auth` takes a bearer token, else the cookie;
  - the header rule, and the temporary-password gate;
  - 429s with `Retry-After`;
  - `whoami` says `has_password`.
  - An account's ID is taken in any case at sign-in (`Griff` is `griff`).
- **The UI:**
  - **Sign-in:** the page asks for an account and a password, with a link to paste a token instead. It goes to the token form by itself on a manager without sessions.
  - **Temporary password:** it is changed before anything else.
  - **Password page:** a Password link in the header opens it, to set or change one's own; there it says "Set a password" when there is none yet.
  - **Requests:** every one carries `X-Aeolus-Session`, and a bearer token only where one is kept.
  - **Signing out** ends the session too.
- **Getting a first password:** an account with only a token, as `griff` on Example's manager, opens Password and sets one, without a current one. Otherwise an owner, or `aeolus password` on the host, sets a temporary one.
- **Tested:**
  - the store's timeouts, retagging and record;
  - the limiter's doubling;
  - in the API: the same answer for a wrong account, a wrong password and no password; the cookie's flags; the header rule; the temporary-password gate and change; the other sessions ending; a disabled account; signing out; the lock; the record's reasons; token guessing; a first password set with a token.
  - Against the real binary over TLS with `curl` and a cookie jar, the same flow from sign-in to sign-out. The browser pane wouldn't open a self-signed local manager, so the page itself wasn't clicked through.

## As built: part 3, the admin section (2026-10-20)

- **Roles first:** 0177 came before it: a role is a set of rights, and who sees whom on the page follows them.
- **The API:**
  - **People:** `GET /v1/accounts` and `GET /v1/accounts/{id}`.
    - An owner sees everyone. An admin or a manager sees the accounts holding roles where it may give them. Everyone sees themselves.
    - Each account's roles are those on folders the reader may view, each saying whether the reader may take it away.
    - An account's tokens and sessions are its own and an owner's to see.
  - **Adding and resetting:** `POST /v1/accounts` adds a person with a temporary password, and `POST /v1/accounts/{id}/password` resets one. Both answer the password once; the reset ends the person's sessions.
  - **Ending sessions:** `DELETE /v1/accounts/{id}/sessions`, all but the asker's own, or one at a time by the first 16 characters of its ID.
  - **The record:** `GET /v1/signins`, filtered by account and result. It is read by the 0177 `sign-ins` right, and everyone reads their own.
  - **Tokens:** each says who issued it and when, from the change log (`changelog.OfKind`), and when it was last used and from where. The last use is written at most once a minute, in `sessions.db`, whose schema goes to version 2 by itself.
  - `whoami` adds `gives`, which roles each role gives.
- **The UI:**
  - **Admin tab:** shown to an owner and to those who give roles or read sign-ins. It has People and Sign-ins.
  - **People:** the list, with roles in a line and the last sign-in; removed accounts are hidden unless asked for. An owner adds a person there.
  - **An account's page:**
    - for an owner: Reset password, Rename, Make super admin (or not), Disable or Enable, and Remove;
    - roles, each taken away in place, and Give a role, which offers only the folders and roles the reader may give;
    - tokens, issued and revoked by the account itself or an owner, a new one shown once;
    - where it is signed in, each session ended in place;
    - its recent sign-ins.
  - **My account,** in the header in place of Password: the same page for oneself, with the password link and "Sign out everywhere else".
  - The Changes page calls an owner a super admin.
- **Tested:**
  - in the API: who sees whom, as an owner, an operator and a manager; adding, its temporary password and must-change; a reset ending sessions; tokens' issuer and last use; ending sessions all but this one and one at a time, and who may; the record by owner, manager and operator, and its filters;
  - in the store: a token's use written once a minute, and a version 1 store gaining the table.
  - In a browser, against a local manager over plain HTTP, as an owner, a manager and an installer:
    - People and an account's page;
    - giving a role, which saved as a change;
    - a reset, its password shown once;
    - Sign-ins and My account;
    - the manager seeing only its installer and itself, and offered only manager, installer and viewer on its own building;
    - the installer having neither Admin nor Captures.

## As built: part 4, the installer (2026-10-20)

- **`aeolus init -password-stdin`:**
  - reads the first admin's password from the first line of its standard input;
  - checks it before making anything, so one that won't do leaves no Org;
  - sets it, and prints no token unless `-token` asks for one.
  - Without `-password-stdin`, it prints a token as before.
- **`deploy/install.sh`:**
  - on a first install with a terminal, it asks for the password after the Org and the account, twice and not echoed, until the two agree and it is 12 to 128 characters and not the account's ID;
  - it hands the password to `aeolus init` on standard input, never on a command line, and forgets it after;
  - it ends with "Sign in as <you> with the password you chose."
  - **`--token`,** or no terminal to ask on, prints a token instead, as before.
  - **`--password-stdin`** takes the password from its standard input, for `deploy/proxmox.sh`.
- **`deploy/proxmox.sh`:**
  - asks for the password on the Proxmox host's terminal, with the Org and the account, and pipes it to the installer in the container through `pct exec`;
  - with `--yes`, with no terminal, or with the installer's `--token`, it has the installer print a token, and nothing is asked in the container.
- **The README** says so, and that a lost password is a super admin's to reset on the Admin page, and a lost token is issued again on My account.
- **Tested:**
  - `aeolus init`: a password set, the owner made, no token printed; `-token` printing one as well; a short password refused with no Org left.
  - The installer's prompt, run under dash on a terminal in WSL Ubuntu 24.04:
    - a short pair, a pair that differ, and the account's ID each asked again;
    - typed text never shown;
    - no terminal and `--token` giving a token;
    - `--password-stdin` taking the password.
  - A Linux build of `aeolus init` fed the password as the installer pipes it.
  - **Not run:** a whole install on a fresh host or a new Proxmox container, which wants Griff's go-ahead and a host to make it on.

## Open

- A second factor, with an authenticator app (0024), after this version.
- Single sign-on (OIDC) for a company's own directory.
- Custom roles, made of single permissions, if per-folder roles prove too coarse.
- An Access panel on each folder's page: who holds which role there.
- Email for a reset link, which needs mail set up on the manager.
