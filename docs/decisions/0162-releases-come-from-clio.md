# 0162. Releases come from Clio

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff, with GitHub's WiFiGUY512 account still hidden: "ok what can we do now without github, its still blocked", then "go ahead" to fetching releases from Clio. Written up by Claude
- Refines: 0011, 0069

## Context

- **The code of record is on Clio** since 2026-10-14: Forgejo on CT 122 (`clio.example.com`), repo `example/aeolus`, tested by the runner on talos.
- **The manager fetched releases from a bare repo of its own,** `/srv/git/aeolus.git`, since GitHub stopped answering on 2026-10-12. A release was three commands: tag in a clone, push the tag to that bare repo, `aeolus-update`.
- **`aeolus-update` still named GitHub** for a first clone, and checked the host it fetched from against GitHub's key only.

## Decision

- **`aeolus-update` fetches from Clio,** `ssh://git@clio.example.com/example/aeolus.git`, with the manager's read-only deploy key (the one GitHub had), added to the repo on Clio.
- **Clio's host key is in the repo,** `deploy/known_hosts`, and `aeolus-update` installs it as `/etc/aeolus/known_hosts` and checks Clio against it. Where that file is missing, the script takes it from the checkout before fetching.
- **The checkout is pointed at Clio every run,** whatever it was cloned from, so the change needs no hand edit on the manager.
- **A release is a tag on Clio, then `aeolus-update <tag>`** on the manager.
- **The bare repo stays where it is,** unused.

## Consequences

- **The first release with this change goes out the old way:** pushed to the bare repo and installed by the old script, which installs the new one. Every release after it comes from Clio.
- **A manager without Clio cannot update,** as it could not without GitHub before. Go modules and toolchains still come from Go's proxy (0069).
- **The Go module keeps its name,** `github.com/WiFiGUY512/aeolus`. It is only a name; nothing is fetched by it.
- **When GitHub answers again,** Clio can push a mirror to it. The manager keeps fetching from Clio.
