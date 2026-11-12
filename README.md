# Aeolus

Aeolus manages Wi-Fi on access points that run [OpenWrt](https://openwrt.org/). It is free software, made so that anyone's OpenWrt hardware can do what commercial Wi-Fi systems lock behind their own boxes and licences.

It has two parts:

- **The manager:** one Go program with a web interface. It holds what every AP should run, and runs on a small Linux host, a virtual machine or a Proxmox container.
- **The agent:** scripts on each OpenWrt AP. The agent takes its settings from the manager, renders them as OpenWrt config, checks and applies them, and reports back.

The manager stays out of the data path. If it is down, the APs keep serving Wi-Fi as they were set. Nothing needs the internet to run.

## What it does

- **Networks:** SSIDs with WPA2 and WPA3, Personal and Enterprise, and per-user keys, on VLANs or VXLAN tunnels.
- **Roaming and band steering:** 802.11r, k and v.
- **Radios:** channels and power, planned by the APs themselves.
- **Clients:** a view of each one, with how it joined and why it left.
- **Tests and alerts:** tests over the wire and, on APs with a spare radio, through the air, with alerts on problems.
- **Settings:** set once in folders and inherited by every AP below, with every change previewed and logged under the name of whoever made it.

## Getting started

[Getting started](docs/getting-started.md) goes from a release file to Wi-Fi on your own APs. In short:

1. **Put up the manager.** Take the release file for your host, `aeolus-<tag>-linux-amd64.tar.gz` or `-arm64`, and the `SHA256SUMS` beside it, from the [latest release](https://github.com/WiFiGUY512/aeolus/releases/latest). Then, as root:

   - **On a Proxmox VE host,** which makes a Debian 13 container for the manager and installs it there:

     ```sh
     sha256sum -c --ignore-missing SHA256SUMS
     tar -xzf aeolus-<tag>-linux-amd64.tar.gz
     sh aeolus-<tag>-linux-amd64/deploy/proxmox.sh
     ```

   - **On Debian or Ubuntu with systemd,** amd64 or arm64: the same, with `deploy/install.sh` in place of `deploy/proxmox.sh`.

   From a checkout of this repository, `sh deploy/install.sh` builds the manager first.
2. **Join each AP** (OpenWrt 24.10 or later) with the one line the installer prints.
3. **Adopt the APs** into a folder, then **add a network**.

More:

- [Installing](docs/install.md)
- [Hardware](docs/hardware.md)
- [How Aeolus holds a network](docs/concepts.md)
- [The API](docs/api.md)
- [The decisions behind it](docs/decisions)

## Building

```sh
go build ./cmd/aeolus
go test ./...
```

`sh deploy/release.sh <tag>` makes a release's files.

## Ideas and problems

**Suggest a feature**, at the top right of every page in the manager, sends an idea along with the page it came from. Problems and ideas can also go in this repository's issues.

## Licence

Aeolus is free software under the GNU Affero General Public License, version 3: see [LICENSE](LICENSE). Using it means taking it on those terms.

- **You may** run it, study it, change it and share it, for any purpose.
- **A copy you share,** changed or not, goes under the same licence, with its source and this licence.
- **A changed copy you run for others over a network,** such as a hosted manager, offers them its source too.

No one may take it closed: any version of Aeolus, by anyone, stays free under the AGPL.
