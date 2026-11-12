# 0170. A manager in one step

- Status: Proposed
- Date: 2026-10-16
- Proposed by: Griff: "ultimately we also need to put together an aeolus server build package or script install or helper script". Written up by Claude
- Refines: 0011, 0083

## Context

- **An AP joins in one line** (0083), but the manager it joins was put together by hand: the first one, Aeolus (CT 118), had its user, directories, deploy key and `serve.env` made one at a time, then `aeolus-update` and `aeolus init`. 0011 asked for the host to be rebuilt from a provisioning script in this repository; there was none.
- **`aeolus-update` is the lab's:** it fetches only from Clio, with the manager's deploy key, and it updates a host already set up. It makes no user, no directories and no Org.
- **Aeolus is to be published** for anyone with an OpenWrt AP (Griff, 2026-10-16): its manager has to stand up as easily as an AP joins.
- **What the manager needs is small:** one Go binary, built with no C; a user to run it; `/etc/aeolus` for its settings, secret key and certificate; `/var/lib/aeolus` for its state; the systemd unit. It runs nothing else.

## Decision

### The installer

- **`deploy/install.sh` stands up a manager on Debian or Ubuntu with systemd,** on amd64 or arm64, run as root from a checkout, or piped from `curl` with `--repo`. Installed, it is `aeolus-install`.
- **It builds the release from source,** as `aeolus-update` does: the newest `v*` tag unless `--version` names a tag or branch, with the Go toolchain `go.mod` asks for, fetched from go.dev into `/opt/aeolus/go` and checked against the SHA-256 Google publishes beside it. It is used for nothing else, and a toolchain already there that is as new is kept. Go's caches are in `/var/cache/aeolus`.
- **It lays the host out as Aeolus is laid out:** code in `/opt/aeolus/src`, the binary in `/opt/aeolus/bin`, the system user `aeolus` with no shell, `/etc/aeolus` as root:aeolus 0770, `/var/lib/aeolus` as aeolus 0700, `deploy/aeolus.service`, `aeolus` on the path as the service's user (0043).
- **On the first install it makes the Org and its first admin** with `aeolus init`, from `--org` and `--admin` or asked for on the terminal before the build starts. The admin's token is printed once, to the terminal, by `init` itself; nothing writes it to a file or a log. `--mcp-account` makes an MCP client's account too, its token written to `/etc/aeolus/mcp.token` as `init` does. If `init` fails, what it made is taken away again, as its token was never shown.
- **`serve.env` names the certificate's hosts:** this host's name and the address its default route leaves from, unless `--hosts` says. The service makes its certificate on its first start (0033).
- **It opens nothing else.** Where ufw or firewalld is on, it lets through only the manager's own ports: 8443/tcp, and those of the DHCP listeners (67/udp, 15002/tcp, 0068) and multicast DNS (5353/udp, 0151) unless `serve.env` turns them off. Where neither is on, the firewall is left as it is. `--no-firewall` leaves both alone. TFTP, which the lab serves for moving APs to OpenWrt, is only with `--tftp`.
- **It ends with the line that joins an AP,** with the certificate's fingerprint for `-f` (0083), by the host's address: dnsmasq on an AP drops a name that points to a private address.
- **Run again, it upgrades in place.** It keeps `/var/lib/aeolus`, the secret key, the certificate APs pin and `serve.env`. The running build is replaced only by one that built and runs, and the service restarts only when its build or unit changed.
- **Where the code comes from is kept by the checkout:** `--repo` sets its origin, and `--ssh-key` a deploy key, whose command the checkout keeps for the runs after. A forge's host key is `--known-hosts`, installed as `/etc/aeolus/known_hosts`, or trusted when first met.

### Where it comes from

- **Until Aeolus is published, the installer has no repository of its own to fetch from:** it is run from a checkout, or given `--repo`. Clio needs a sign-in and is on the LAN only. Where Aeolus is published, and so the one line anyone pastes, is Griff's to say; the script's `DEFAULT_REPO` is set then.

## Consequences

- **A Debian or Ubuntu host is one command from a manager,** and the next command, on an AP, joins it.
- **The lab's manager could be rebuilt by it** (0011), from Clio with its deploy key: `--repo ssh://git@clio.example.com/example/aeolus.git --ssh-key /etc/aeolus/deploy_ed25519 --known-hosts deploy/known_hosts --hosts aeolus.example.com,192.168.20.60 --tftp`. Aeolus itself is not changed by this: it keeps `aeolus-update`.
- **The script that comes over the network is trusted as it comes,** as the AP's `wget` is (0083): over HTTPS from wherever it is published.
- **The first build takes about 600 MB** for the toolchain and Go's caches; an upgrade builds from the caches.
- **Checked (2026-10-16) on throwaway Debian 13 and Ubuntu 24.04 systems under WSL,** never on Aeolus:
  - piped as from `curl`, from a bare Debian 13: git and curl installed, Go 1.27.0 fetched and checked, built, Org made, serving, in 79 s; the owners and modes of `/etc/aeolus` and `/var/lib/aeolus` and what is in them as on Aeolus; the certificate's fingerprint the one `/install/manager.crt` serves;
  - run again on the same release: the build is the same to the byte, and the service is not restarted;
  - a newer commit, an older tag (v0.120.0, from before the script), and back: each built and started, the state, secret key and certificate kept, `aeolus-install` not replaced by a release without one;
  - a tag that is not there, an option that is not, an ID that is not one, and no `--repo` with no checkout: each refused before anything changed;
  - `init` refusing (an MCP token file already there): its database taken away, nothing running; asked on a terminal, the Org made and the MCP token written 0600, not shown;
  - from a checkout with no `--repo`; over SSH with a deploy key and a host key, the checkout keeping the command for the next run; a host key that is not the forge's refused, nothing changed;
  - with ufw on (a stand-in that records what it is asked: WSL's systems share one network), only the manager's ports, 15002/tcp left out when its listener is off; `--no-firewall` asking nothing; `--tftp` serving `/srv/tftp` as Aeolus does.
- **Not yet checked:** an AP joining a manager the installer made, in a Proxmox container or on a host on the LAN; firewalld; arm64.

## Open

- A Proxmox VE helper, in the style of the community's scripts, that makes a Debian container and runs this in it.
- A Docker image.
- Builds published with each release, so a host needs no Go.
- `aeolus-update` and `aeolus-install` as one.
