# 0183. The manager's own releases: a notice, Update now, and updates by itself

- Status: Accepted (Griff, 2026-10-20: "can we also have a page under system or maybe a banner at the top notifying when we have a new version ready to be downloaded", then "with a click button or scheduler to do it automatically")
- Date: 2026-10-20
- Proposed by: Griff. Written up by Claude
- Refines: 0011, 0043, 0079, 0162, 0170

## Context

- **A manager learns of a newer release only when someone looks,** and installing one needs root on its host: `aeolus-install` (0170), or `aeolus-update` (0011, 0162) on Example's own.
- **Takers' managers fall behind,** and their APs with them (0079: the APs follow the manager's release).
- **The manager runs as `aeolus`, without root, and must stay so.** A process the web reaches must never be able to change itself, or anything else on its host, by its own hand.
- **The manager must not need the internet to run** (cloud independence). Looking for a release is a nicety: where it can't, nothing breaks.

## Decision

- **An updater runs as root, apart from the manager, and does only two things the manager may ask:**
  - **`check`:** fetch the release tags from where the code comes from, and say the newest.
  - **`update vX.Y.Z`:** install that release, a tag the repository has, with `aeolus-update` where the host has its deploy key, else `aeolus-install`, as a person would.
  - **The manager asks** by leaving a file, `request`, in `/var/lib/aeolus/update`. A systemd path unit starts `aeolus-updater.service` when one is there.
  - **The updater trusts nothing in that directory:**
    - a request that is a link, or anything but those two exact forms, is refused;
    - what it writes back (`status`, as key=value lines, and `update.log`, the last run's output) goes to a new file renamed into place, so a link left there is replaced, never written through.
- **The installer sets it up:** `aeolus-install` and `aeolus-update` install the script as `/usr/local/sbin/aeolus-updater` with its two units, make the directory (aeolus's own), and enable the path unit. A manager put up before this release gets it when it is next upgraded by hand, once.
- **The manager:**
  - **The daily look:** it asks for a `check` once a day.
  - **Updates by itself,** where the Org says so: `system.updates.automatic` on, it asks for an `update` of the newest release in `system.updates.window`, 03:00–05:00 unless set, in the Org's local time (`system.tz` at the Org). A release that failed is not asked for again by itself for a day.
  - **The settings are the manager's own,** never sent to APs, and take the administer right (0177), as the credentials do.
  - **`GET /v1/system/update`** gives what the updater last said: running, newest, whether it is newer, when it looked, the state, the last result and its output, and the settings. **`POST /v1/system/update {action}`** asks it to `check` or to `update` to the newest. Both are an admin's at the Org, or an owner's; anyone else gets 404.
- **The page, for an admin at the Org:**
  - **A banner at the top of every page** when a newer release is out: Update now, Details, or Not now, which hides it until the next release. While one is installed, it says so.
  - **An Updates panel at the top of the Org's System tab:**
    - the running release, the newest and when it looked, with Check now;
    - Update to vX.Y.Z;
    - whether it updates by itself, set under Edit system settings;
    - the last result, and the last update's output in a fold.
- **Releases stay Griff's:** the updater installs only what is tagged.

## Consequences

- **A manager says when it is behind,** and an admin brings it up to date with one press, or lets it in the night.
- **The manager restarts as it updates,** for a minute or so. The page comes back by itself, and the APs keep running. They then take the new agent themselves (0079).
- **No root reaches the web:** the updater does exactly two things, the second only for a release the repository has.
- **A manager whose repository can't be reached** says when it last looked and why it couldn't, and nothing else changes.

## As built (2026-10-20)

- **The updater:** `deploy/updater.sh`, `deploy/aeolus-updater.service`, `deploy/aeolus-updater.path`, installed by `deploy/install.sh` and `deploy/update.sh`. CI checks it with `sh -n`, as the other scripts.
- **The manager:**
  - **`internal/api/updates.go`:** the status, the two routes, `updateDue` and the `Updates` loop, every five minutes.
  - **`serve`:** gives it `<state>/update`.
  - **The window:** `windowOpen`, shared with 0181.
  - **The settings:** `system.updates.automatic` and `system.updates.window` in the schema, among the fields only the manager uses.
- **The UI:** `views/updates.js`, the banner from `app.js`, and the panel from `sections.js`.
- **Tested:**
  - **The updater script in WSL,** against a made-up repository and installer:
    - a check found a newer tag;
    - an update installed it and the version changed;
    - a tag the repository lacks, a request with a command in it, and anything else were refused;
    - a request that is a link was removed, unread;
    - a status planted as a link to another file was replaced, the file untouched.
  - **The manager, in Go:**
    - which release is newer;
    - what is due when: never looked, a day old, automatic in and out of the window, a release that failed lately and two days ago, the updater at work, nothing newer;
    - the routes: who may, the request each leaves, a request waiting, nothing newer, no updater;
    - the role table: the settings are an admin's.
  - **In a browser,** on a local manager:
    - the banner;
    - the panel;
    - Update to v0.130.0, which left `update v0.130.0` for the updater, logged who asked, and showed it asked.
- **On Example's manager, 2026-10-20:**
  - **The updater went in with v0.126.0,** which Griff installed by hand.
  - **v0.127.0 was then installed through it,** asked for as the manager asks (`update v0.127.0`, written as aeolus and renamed into place). The updater built and installed it, and the manager came back as v0.127.0-fd30a10, with `state=updated`.
  - **It showed one thing to mend:** the status still named v0.126.0 as the newest until the next daily look. An update fetches the tags anyway, so since then it says the newest, and when it looked, as a check does. That was tested in WSL against a made-up repository: installing v0.2.0 while v0.3.0 is out leaves `newest=v0.3.0`.

## Open

- An Org-wide notice to the people who don't administer.
- Release notes in the banner, where a release carries them: done since, in 0217.
