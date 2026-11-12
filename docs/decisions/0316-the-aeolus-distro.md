# 0316. The Aeolus distro: release files anyone can install

- Status: Accepted (Griff, 2026-11-05: "Yes, merge both", of #302 and #319. Where release files are published, and a license, stay his to say)
- Date: 2026-11-04
- Proposed by: Griff: start the "Aeolus distro", a way for people outside the lab to install and run Aeolus on their own hardware, the first of them his friend the first user. How releases reach them: "release tarballs or however wget works or script installs work. proxmox helper scripts? I dunno you tell me". Written up by Claude
- Refines: 0069 (running without the internet), 0079 (fleet updates), 0170 (a manager in one step), 0175 (a manager in a Proxmox container), 0183 (manager updates), 0282 (release channels)

## Context

- **Aeolus is meant for anyone's OpenWrt hardware,** free, as an answer to vendors who gatekeep basic features (Griff, 2026-11-01). Its measure is whether a stranger can install it and run it, not the lab.
- **The first stranger is the first user.** What APs he has, and what would host his manager, is not known yet.
- **What a newcomer needs today, step by step, and where it assumes Griff's lab** (checked 2026-11-04 against Clio's `main`, e06af7f):

| Step | What there is | Where it assumes the lab |
|---|---|---|
| A host for the manager | `deploy/install.sh` (0170) on Debian or Ubuntu, `deploy/proxmox.sh` (0175) on Proxmox VE | Piped, both fetch from `github.com/WiFiGUY512/aeolus`, which needs a sign-in. The code of record is on Clio, which is on the LAN and needs one too. So a newcomer has no code to install. |
| Installing it | Both build from source on the host: git, Go from go.dev, Go's modules from its proxy, about 600 MB, and 1.7 GB of memory for the build | Nothing of the lab's, but the most a small host (a Pi, 2 GB) has to do, and three places on the internet. |
| OpenWrt on the APs | Any AP on OpenWrt 24.10 or later ([hardware](../hardware.md)) | Nothing of the lab's. For OpenWrt's own images, OpenWrt's firmware selector; for the Arista units, the port's images and its field installer, which are the porting session's. Our docs say neither. |
| The agent | `wget …/install \| sh` from the manager (0083); packages through the manager's cache of OpenWrt's feeds (0069) | Nothing. The examples' `192.168.20.60` is the lab's manager. |
| Adoption and the first networks | Landing Zone, then **Move…** the AP into a folder, which adopts it ([install](../install.md)) | Nothing of the lab's. But a fresh Org has no network and no page that leads to one. **Add a network** shows only where a services folder applies, and saying which services folders a location takes (`assign-services`) has no page: it is the API's or MCP's alone. That page is an open question in the README ("mockup first"). |
| Updates | The manager's updater (0183) fetches tags from the git origin it was built from; the APs follow their manager (0079) | With no origin a newcomer can reach, a newcomer's manager never updates. No release is on the stable channel yet (0282). |

- **Left from the lab, and only the lab's:** `deploy/update.sh` and `deploy/known_hosts` (Clio, with the lab manager's deploy key), the unit's `Documentation=` (Clio's address), the installer's `--tftp` (moving APs to OpenWrt).
- **The repository is private.** Who gets it, and where Aeolus is published, are Griff's to say. A license with it.

## Decision

### What the distro is

- **Aeolus for a site Griff does not run:** a manager put up from a release file, and APs on OpenWrt's own images, each joined with the one `wget` line. Nothing in it names the lab.
- **An AP image stays OpenWrt's** (0080, 0171). The port's images carry no Aeolus, so they stay upstreamable. An image with Aeolus in it, OpenWrt's ImageBuilder with `aeolus-find` and `aeolus-agent` added, would be a separate build, and Griff's to decide.

### What ships with a release

- **A release file for each target:** `aeolus-<tag>-linux-<arch>.tar.gz`, for `amd64` and `arm64`, and `SHA256SUMS` beside them.
  - Unpacked, it is one directory, `aeolus-<tag>-linux-<arch>/`, holding the manager (`aeolus`, with the agent in it, 0079), the tag's `deploy/` scripts and units, and the guides a newcomer reads: `docs/getting-started.md`, `install.md`, `hardware.md` and `concepts.md`.
  - `RELEASE` says what it is, a `key=value` a line: `name`, `version`, `tag`, `commit`, `os`, `arch`, `go`, `time`. A reader passes over a key it does not know, so more can be said later.
- **`deploy/release.sh <tag>` makes them,** from the tag itself (`git archive`), never from what the checkout has besides.
  - It builds as the installer builds (0170): no C, `-trimpath`, `-s -w`, and the version `<tag>-<commit>`. So a manager from a release file says the version one built from git says.
  - With the same tag and Go, the files are the same to the byte: names sorted, no owners, the commit's time, gzip without a name or time.
  - A tag whose installer takes no release file is refused.
  - The targets are a table, a line each; another (`armv7`, say) is a line there.
- **They are not signed,** as nothing of Aeolus's is yet (0032, 0079). `SHA256SUMS` catches a file spoilt on the way. Trust is in whoever hands the file over, as with the AP's `wget` (0083).

### How it installs

- **The release file's own installer installs it:**

      tar -xzf aeolus-<tag>-linux-amd64.tar.gz
      sh aeolus-<tag>-linux-amd64/deploy/install.sh

  or `install.sh --from <file or URL>`, unpacked or not. Nothing is built: no git, no Go, no compiler, and nothing fetched but the file and, from apt, curl where it is missing.
- **Everything else is 0170's:** the user, `/etc/aeolus` and `/var/lib/aeolus`, the unit, the Org and its first admin, the password asked twice, the firewall, and the `wget` line for an AP at the end.
- **The file is checked before anything changes:**
  - its SHA-256 against `--sha256`, else against the `SHA256SUMS` beside it, where there is one; else the SHA-256 is printed;
  - its `RELEASE` against the host: `os` linux and `arch` the host's. An arm64 file on an amd64 host says which file to take.
- **A manager remembers what it was put up from:** `/etc/aeolus/release`, the file's `RELEASE` with `from`, `sha256` and `installed`.
  - **Run again,** `aeolus-install --from <newer file>` upgrades in place, keeping the state, the secret key and the certificate APs pin, as 0170's upgrade does.
  - **`aeolus-install` alone** says it was put up from a release file and asks for a newer one. It does not turn to git.
  - **A build from git takes the record away.** The last install says how the next one comes.
- **The installer no longer names a place to fetch Aeolus from.** Its GitHub default needed a sign-in. With no release file, no checkout and no `--repo`, it says what to give it. `AEOLUS_REPO` still names one.

### On a Proxmox VE host

- **The release file's own `deploy/proxmox.sh` makes the container and installs the release in it** (0175's container, unchanged):

      tar -xzf aeolus-<tag>-linux-amd64.tar.gz
      sh aeolus-<tag>-linux-amd64/deploy/proxmox.sh --bridge vmbr0

- **The file goes into the container's `/root/.aeolus`, readable by root alone,** and the installer there is given `--from` it.
  - It goes in as the tarball the directory was unpacked from, where that sits beside it, with the `SHA256SUMS` beside that. Else the directory is packed again.
  - A file named with `-- --from <file>` goes in the same way. One named by its URL, the container fetches.
  - A file put in before is taken away first, and so is a `SHA256SUMS` the new file did not come with.
- **It is checked on the host before anything is made:** a file built for another arch than the host's, as a container runs as its host, and `--version`, `--repo` or `--ssh-key` with a release file, which is its own release.
- **A container without the internet goes on,** as the file needs nothing fetched unless the template has no curl (0069). A build from git still stops there.
- **Run again with `--ctid`** from a newer release file, it upgrades that container. From a checkout, it bundles the checkout as before.
- **On its own, piped or a copy,** it needs `AEOLUS_RAW` and `AEOLUS_REPO` to say where Aeolus is. Its GitHub default is gone, as the installer's is.

### How it updates

- **The APs, as ever, from their manager** (0079).
- **The manager, for now, by hand:** a newer file, then `aeolus-install --from` it.
  - The updater (0183) answers a check, or **Update now**, on a manager put up from a release file with exactly that, and fetches nothing.
- **Next: updates from where the files are published.** The updater reads a release index beside the files, the releases and which are on stable (0282), and installs the one asked for with `--from`. Then **Update now**, the banner, the notes and the nightly window work as they do with git.

### Generic, and the lab's

| Generic: ships in a release file | The lab's: stays in the repository only |
|---|---|
| `deploy/install.sh`, `deploy/proxmox.sh`, `deploy/updater.sh`, `deploy/aeolus.sh`, the units | `deploy/update.sh` and `deploy/known_hosts` (Clio, the deploy key) |
| The manager, and the agent, LuCI page and finder in it | `--tftp`, for moving the lab's APs to OpenWrt |
| `docs/getting-started.md`, `install.md`, `hardware.md`, `concepts.md` | The unit's `Documentation=`, Clio's address: to be the docs' place once Aeolus has one |

### Where the files go: Griff's call

- **Handing a release file to someone is publishing it,** so it is Griff's to do or to say, each time, until he names a place.
- **What a place could be:** the file given to the first user by hand (it installs from a file); a web directory Griff runs, for `--from <URL>` and later the updater; a public home, with a license, when Griff says.
- **The repository stays private.** A release file holds the manager built, the agent's ucode (it is source, run as it is) and the deploy scripts; nothing else of the repository.

## Steps

1. **Release files** (PR #302): `deploy/release.sh`, the installer's `--from` and `--sha256` and its release directory, the updater's answer, and the docs.
2. **The Proxmox helper from a release file, and a guide:** `proxmox.sh` takes the release it is in, or one given, into its container; `docs/getting-started.md` goes from a release file to the first network.
   - A location taking a services folder had no page, so the guide first gave it as an API call.
   - Griff had the page built as drawn (0320, #327, released in v0.267.0): a newcomer's first network is now all clicks, **Add a network** on a fresh folder, and the guide says so.
3. **Updates from a release index,** and release files made with every release, as a step of the release itself.
4. **Where the files live, and the license:** Griff's.
5. **AP images with Aeolus in them:** Griff's to decide.

## Consequences

- **A manager is a file and one command away,** on any Debian or Ubuntu host with systemd, amd64 or arm64, a Pi among them. The host needs no Go and no access to the repository, and builds nothing.
- **Installing needs the internet only for apt,** where curl is missing. An offline site carries the file in, which 0069 left open ("updating the manager from a file").
- **A newcomer cannot update by themselves until step 3.** Until then they take each release file by hand.
- **Both ways stay:** the lab's manager builds from Clio with `aeolus-update`, a checkout builds as before, and a release file installs as above.

## Checked

- **Tests:**
  - `TestRelease` and `TestReleaseRefuses`: the files, their `RELEASE`, `SHA256SUMS`, the same bytes made again, one target, and the refusals.
  - `TestUpdaterReleaseFile`: a check and an update on a manager put up from a release file.
- **End to end, 2026-11-05, on a throwaway Debian 13,** never on the lab. The host was linuxcontainers.org's trixie rootfs under WSL, with systemd. The release files were made from this branch with `deploy/release.sh`.
  - **A fresh install** from the unpacked amd64 file, as the first user would do it:
    - the Org and first admin made, and the service running, 1 s after the installer started;
    - nothing built: no git installed, no `/opt/aeolus/src`, no Go;
    - `/etc/aeolus/release` written;
    - `/install` serving the AP's installer, and the `wget` line printed.
  - **`aeolus-install` alone** asked for a newer file.
  - **Refused before anything changed:**
    - a wrong `--sha256`;
    - the arm64 file on amd64, with the name of the file to take instead;
    - `--from` with `--version`;
    - a file that is not there.
  - **Upgrades:**
    - from a file with its `SHA256SUMS` beside it: checked, with the database and the certificate APs pin kept;
    - the same file again: nothing restarted;
    - from a URL: the `SHA256SUMS` beside it fetched and checked;
    - an uppercase `--sha256`: taken;
    - a URL with nothing there: refused at once.
  - **The updater,** asked as the manager asks it, fetched nothing:
    - a check came back idle, with the release-file answer;
    - an update failed, with the same answer.
  - **Built from git, then back** (a bundle, as `proxmox.sh` hands one in):
    - git was installed, Go fetched, and the manager built and running in 90 s;
    - the release record was gone, the certificate APs pin kept, and the updater fetched from git again;
    - a release file then put it back on files, with its record.
- **The Proxmox helper (step 2), tests:** with `pct`, `pveam`, `pvesm`, `pvesh` and `id` stood in for, each container a directory:
  - `TestProxmoxFromRelease`: a new container from a release file unpacked. The release's own installer is pushed, the tarball it came from and its `SHA256SUMS` go into `/root/.aeolus`, and the installer is given `--from` it, with no bundle and no `--repo`.
  - `TestProxmoxReleaseElsewhere`: unpacked with no tarball beside it, it is packed again; by URL, nothing is copied in.
  - `TestProxmoxUpgradeFromRelease`: `--ctid` makes no container and asks for no Org, and the older file is taken away.
  - `TestProxmoxRefuses`: the arm64 file on amd64, `--version` from a release, `--from` with `--version`, a file not there, a container it did not make, and the script on its own. Each is refused with nothing made or copied.
  - `TestProxmoxFromCheckout`: from a checkout, the bundle as before; with `--repo`, the deploy key and host key copied in 0600 and no bundle.
- **The Proxmox helper, end to end, 2026-11-05,** on another throwaway Debian 13 under WSL. The stand-ins treated the system itself as the container, as 0175 was checked, so the installer ran for real:
  - from the release file unpacked, `--yes`: the container "made", the tarball and its `SHA256SUMS` pushed 0600, the SHA-256 checked inside, the Org made and the manager running 2 s on, with no git;
  - `--ctid 123` from a newer release file unpacked: no container made, the older file replaced, the newer release running, and the certificate APs pin kept.
- **The guide's first API step, as written, on that manager** (before 0320 made it clicks):
  - two folders made, as **New folder…** makes them;
  - the preview said the Org would take `home-networks`, and the change was logged under the first admin's name;
  - the House folder then took `home-networks`, inherited from the Org.

  **Add a network**, which then shows on its Networks tab, is checked by its code (`networks.js`), not on the page.
- **A file made on Windows** has its manager unmarked to run, as Windows keeps no file modes. The installer sets 0755 itself, and `release.sh` says so. Releases are for making on Linux.
- **Not yet checked:**
  - an AP joining a manager put up from a release file. The AP's side is 0083's, unchanged;
  - arm64 hardware;
  - Ubuntu.
