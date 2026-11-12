# 0175. A manager in a Proxmox container

- Status: Proposed
- Date: 2026-10-18
- Proposed by: Griff: "build the Proxmox VE helper script (Debian LXC that runs the installer)", the second part 0170 left open. Written up by Claude
- Refines: 0170

## Context

- **Many who would run Aeolus run Proxmox VE,** as Griff does, and stand services up with the community's helper scripts: one command on the host makes a container and installs the service in it.
- **0170's installer does the second half** on any Debian. What was left is the container: the template, its size, its disk and network, the settings systemd needs in an unprivileged container, and getting the code and the installer into it.
- **The community's scripts fetch their framework from their own repository** each run, and their installers must live there. Aeolus's helper has to stand on its own.
- **Aeolus is not published yet** (0170): a container cannot fetch its code from anywhere public.

## Decision

- **`deploy/proxmox.sh`, run as root on a Proxmox VE host from a checkout, makes a container and runs the installer in it.** Options after `--` go to the installer as they are.
- **The container is made as Aeolus's own (CT 118) is:** the newest Debian 13 standard template the host has, else the newest it is offered, fetched; 2 cores, 2048 MB, 512 MB swap, 16 GB; unprivileged, with `nesting` and `keyctl` (systemd's sandboxing needs nesting); started at boot; the host's time zone; tagged `aeolus`. Each can be changed by an option.
  - **Its disk** is on the roomiest active storage that holds containers, ZFS, LVM or the like before a directory, unless `--storage` says.
  - **Its network** is `vmbr0` by DHCP unless `--bridge`, `--vlan`, `--ip` and `--gateway` say.
  - **No password is set.** `pct enter` opens a shell; `--root-key` lets a key sign in as root.
- **It says what it will make and asks first,** on the host's terminal, as it asks for the Org and first admin. `--yes` asks nothing, and then the Org and admin must be given.
- **The code comes from the checkout** as a git bundle copied into the container, which the installer builds from, unless the installer is given `--repo`. A deploy key or host key the installer is given is copied into the container, readable by root only.
- **Run again with `--ctid` of a container it made, it upgrades it:** the newer installer and, where the container builds from a bundle, a newer bundle; then `aeolus-install` in it. A container without the `aeolus` tag is left alone, so it never touches a manager it did not make, such as CT 118.

## Consequences

- **On a Proxmox host, a manager is a checkout and one command away:** `git clone` of Aeolus, then `sh aeolus/deploy/proxmox.sh`.
- **Checked (2026-10-18) on a throwaway Debian under WSL, with `pct`, `pveam`, `pvesm` and `pvesh` stood in for** by scripts that log each call and treat the system itself as the container, so the installer ran for real:
  - a new container: the newest Debian 13 template fetched, ZFS chosen over a directory and the roomiest of two, the `pct create` line as above, the bundle pushed, the manager made and serving;
  - asked on a terminal and answered no: nothing made; `--yes` with no Org, a bridge that is not there, a number that is not one, `--ip` without `--gateway`, an ID that is not one, a container it did not make (as CT 118), an option that is not one, no terminal and no `--yes`, and piped rather than run from a checkout: each refused before anything was made;
  - an upgrade from a newer checkout: a newer bundle pushed, the manager rebuilt and restarted; over SSH with a deploy key and host key, both copied in 0600 and the checkout keeping the command; an upgrade of that one pushed no bundle.
- **Checked on powermox (Proxmox VE 8.4, 2026-10-18):** Griff's first run made CT 123 as above (`ssdpool`, `vmbr1`, VLAN 20, DHCP) and its second CT 124, both stopped at Go's download, cut off by a bad record MAC. The run again with `--ctid 123` finished it: Go's download broke once more and went on from the break, v0.125.0 built, the Org was made, and the service ran under its unit's sandboxing in the unprivileged container, its certificate naming `aeolus-test.example.com` and the address its default route leaves from. CT 124 was destroyed with Griff's leave.
- **Fast downloads into the lab broke off** with a bad record MAC: Hermeneus 2 of 5 from Google and 3 of 5 from Cloudflare, powermox itself 1 of 5 over its other NIC and VLAN. A laptop on Wi-Fi behind the same gateway had 0 of 5, which at first pointed at powermox. It was the lab's internet router, a Teltonika RUTM50: its hardware TCP segmentation, fed packets its modem's receive path had merged, wrote other packets' bytes into the payload and fixed the checksums up, so only TLS or a hash could tell. It hit any wired client fast enough; the laptop's Wi-Fi was slower. With segmentation off on the router, 0 of 24 broke (2026-10-18). The installer copes either way: each try of Go's download goes on from where the last stopped until the file is what go.dev's SHA-256 says, and Go's modules are fetched before the build, again until they come.

## Piped from where Aeolus is published (2026-10-20)

- **Aeolus is published at `github.com/WiFiGUY512/aeolus`** (Griff, 2026-10-20). Both scripts run in one line from there, as the community's do:
  - `curl -fsSL https://raw.githubusercontent.com/WiFiGUY512/aeolus/main/deploy/proxmox.sh | sh -s -- [options]` on a Proxmox VE host;
  - the same with `deploy/install.sh` on Debian or Ubuntu.
- **The installer's default source** is the published repository, over HTTPS, which needs no key. It is used when nothing else says: no `--repo`, no earlier checkout, and not run from a checkout.
- **The helper, not run from a checkout** (piped, or a copy on its own), fetches the installer from the published place before it makes anything, and refuses one that isn't Aeolus's installer.
  - The container's code comes from the published repository, when the container has none yet or had it from a bundle.
  - A container given another `--repo` before keeps it.
  - Its last line gives the same one-liner, with `--ctid`, for upgrading.
- **`AEOLUS_RAW` and `AEOLUS_REPO`** name another place for the files and the code, such as a fork's.
- **Checked under WSL (Ubuntu 24.04),** with `pct`, `pveam`, `pvesm` and `pvesh` stood in for. A web server played the published place: the raw files, and the repository over plain HTTP.
  - **A new container, piped:** the published installer was pushed unchanged, and the installer was given the published repository, whose v0.125.0 the container could list.
  - **Upgrades, piped:**
    - a container on the published repository was left on it;
    - one made from a bundle was moved to it;
    - one given its own `--repo` kept that.
  - **Refused before anything was made:**
    - an installer that isn't there;
    - a file that isn't the installer;
    - no terminal and no `--yes`;
    - the real URL, while the repository is still private.
  - **From a checkout,** a bundle, as before.
  - The installer itself wasn't run there, since that WSL has no systemd. Its source is the only change, and it clones any git URL the same way.
- **Still to do before takers use it:**
  - The repository has to be public, with `main` and the release tags kept current from Clio.
  - One real run on a Proxmox host from the public URL.

## Open

- A Docker image.
- Published binaries, so a container needn't build Go and Aeolus.
