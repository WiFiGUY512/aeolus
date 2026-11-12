# Installing Aeolus

Aeolus has two parts:

- **The manager:** one Linux host, or a container, that holds what every AP should run.
- **The agent:** a set of scripts on each OpenWrt AP that does the rest.

The manager stays out of the data path. If it's down, the APs keep serving Wi-Fi as they were set.

## 1. The manager

You need Debian 13 (or Ubuntu) with systemd, on amd64 or arm64, with about 2 GB of memory and 16 GB of disk, reachable by the APs on TCP 8443.

**From a release file,** the file for your host's architecture (`aeolus-<tag>-linux-amd64.tar.gz`, or `-arm64` on a Raspberry Pi or another arm64 board), as root:

```sh
tar -xzf aeolus-<tag>-linux-amd64.tar.gz
sh aeolus-<tag>-linux-amd64/deploy/install.sh
```

The manager in the file is built already: the host needs no Go, and builds nothing. Without unpacking it, or from a URL, `sh install.sh --from <file or URL>` does the same. The file is checked against `--sha256 <hex>` where given, else against the `SHA256SUMS` beside it, where there is one.

**On a Proxmox VE host,** the same file makes a Debian 13 container sized for the manager and installs it inside. As root on the host:

```sh
tar -xzf aeolus-<tag>-linux-amd64.tar.gz
sh aeolus-<tag>-linux-amd64/deploy/proxmox.sh --bridge vmbr0 [--vlan <tag>]
```

It shows what it will make and asks first. The release file and its `SHA256SUMS` go into the container, readable by root alone, and the container needs nothing from the internet unless its template lacks curl. Run again with `--ctid <id>` from a newer release file, it upgrades that container.

**From a checkout of this repository**, which builds the manager on the host: as root, `sh deploy/proxmox.sh --bridge vmbr0 [--vlan <tag>]` on a Proxmox VE host, or `sh deploy/install.sh` on a plain Debian host.

Either way:

- It asks for the Org's name, your account (its first admin) and your password, twice and never shown.
- From a checkout, it builds the newest **stable** release, or the newest release while none is stable (see [Updates](#updates)).
- It starts the manager, and at the end prints its address, `https://<host>:8443/`, and the line that joins an AP to it.

To upgrade in place: from a release file, `aeolus-install --from <the newer file>`; from a checkout, run it again, or use **Update now** in the manager.

## 2. An AP

The AP needs OpenWrt **24.10 or later**, with its LAN up and able to reach the manager. Then, as root on the AP:

```sh
wget -qO- --no-check-certificate https://<manager's IP address>:8443/install | sh
```

**Use the manager's IP address, not its name.** OpenWrt's DNS refuses a name that answers with a private address (rebind protection, on by default), so `aeolus.example.lan` fails on a stock AP before anything runs. From then on the agent looks after the name itself.

What that does:

1. It fetches the manager's certificate and prints its fingerprint. Every later download is pinned to that certificate.
   - To check it up front, pass the fingerprint the manager shows: `| sh -s -- -f <sha256>`.
2. It reads the list of every file an AP needs, with each file's SHA-256, then fetches and checks each one. Where LuCI is installed, its Aeolus page comes too.
3. It joins the AP to the manager. The AP waits in the **Landing Zone**.

Options, after `sh -s --`:

| Option | Does |
|---|---|
| `-f <sha256>` | Refuse a manager whose certificate isn't this one. |
| `-u <port>` | The uplink port that carries the VLANs. Otherwise the one the default route leaves by. |
| `-n` | Install only; join a manager later, from LuCI's Aeolus page. |

Then in the manager, open **Landing Zone**, open the AP, and **Move…** it into a folder: moving it out of Landing Zone adopts it.

- Its settings come from that folder, and its agent installs the packages Aeolus needs (full wpad, vxlan, usteer and a few ucode modules) through the manager's copy of OpenWrt's feeds.
- The AP restarts its Wi-Fi once, with the settings it was given.
- It keeps its address. Where its bridge doesn't filter VLANs yet, as a fresh OpenWrt's doesn't, Aeolus has it filter them and moves the AP's own interface to VLAN 1 of it, with the same MAC and DHCP client id, so a reservation made for the AP beforehand still matches.

## The uplink

Each SSID can be on a VLAN of its own, so the AP's uplink port should be a **trunk**: the management VLAN untagged, and every client VLAN tagged. An AP with one flat LAN works too, with every network on it.

## Updates

The manager looks for a newer release once a day. At the Org, under **Settings**, **More settings…**, **Updates**:

- **Releases from:** `stable`, the default, takes only releases that have run at least a week on testing first. `testing` takes each release as it is made.
- **Install a newer release by itself:** off, the manager says one is out and you press **Update now**. On, it installs it in the window.

The APs update their agents from the manager by themselves, so they follow its channel.

A manager put up from a release file is updated by hand for now, with a newer file: `aeolus-install --from <file or URL>`. **Update now** says so.

## Checking it

The Org's **APs** tab shows each AP and whether it runs what it was given, and **Problems** its alerts. **Settings**, **This manager**, shows the manager's own state.
