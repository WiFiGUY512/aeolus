# Getting started

From a release file to Wi-Fi on your own access points. Aeolus is the manager; your APs run OpenWrt and an agent that takes their settings from it.

## What you need

- **A host for the manager:** Debian 13 or Ubuntu with systemd, on amd64 or arm64, with 2 GB of memory and 16 GB of disk. A small PC, a virtual machine or a Raspberry Pi 4 or 5 on a 64-bit OS all do. Or a **Proxmox VE** host, which makes a container for it.
- **One or more APs on OpenWrt 24.10 or later** ([hardware](hardware.md)), wired to your network.
- **The release file** for your host, `aeolus-<tag>-linux-amd64.tar.gz` or `-arm64`, and the `SHA256SUMS` that came with it: from the [latest release](https://github.com/WiFiGUY512/aeolus/releases/latest), or from whoever gave you Aeolus.
- **A network** where the APs get an address by DHCP and reach the manager on TCP port 8443.

## 1. Check the file

In the directory with both files:

```sh
sha256sum -c --ignore-missing SHA256SUMS
```

It says `OK` for your file. Anything else means the file is not the one that was made: get it again.

## 2. Put up the manager

As root, on a Debian or Ubuntu host:

```sh
tar -xzf aeolus-<tag>-linux-amd64.tar.gz
sh aeolus-<tag>-linux-amd64/deploy/install.sh
```

Or on a Proxmox VE host, which makes a Debian 13 container for it and installs it there. `--bridge` is the bridge your network is on; `--vlan` its tag, if it has one:

```sh
tar -xzf aeolus-<tag>-linux-amd64.tar.gz
sh aeolus-<tag>-linux-amd64/deploy/proxmox.sh --bridge vmbr0
```

It asks for:

- **The Org:** an ID for your whole setup, such as `home` or your family name, in lowercase letters, digits and hyphens.
- **Your account:** the first admin, such as your first name, in the same letters.
- **Your password:** twice, never shown. Twelve characters or more.

It ends with three things worth keeping:

- **The manager's address,** `https://<address>:8443/`.
- **Its certificate's SHA-256.** The APs check the manager against it.
- **The line that joins an AP to it.**

Nothing is built and nothing is fetched but curl, if the host lacks it.

## 3. Sign in

Open the manager's address in a browser. The manager makes its own certificate, so the browser warns the first time. Its certificate viewer shows a SHA-256 to compare with the one the installer printed. Then sign in with your account and password.

## 4. OpenWrt on the APs

Aeolus does not put OpenWrt on an AP.

- **An AP already on OpenWrt 24.10 or later** is ready.
- **One on its maker's firmware:** OpenWrt's Firmware Selector, `https://firmware-selector.openwrt.org/`, finds the image for its model. The OpenWrt wiki page for the model says how to flash it.
- **Arista units** run an OpenWrt port with images and an installer of their own, at `https://github.com/WiFiGUY512/openwrt-arista-images`.

Set each AP up as a plain access point on your network, not as a router. Its own DHCP server is off, and its LAN takes an address from your router. OpenWrt's guide to a "dumb AP", `https://openwrt.org/docs/guide-user/network/wifi/dumbap`, does exactly this. A reservation in your DHCP server keeps each AP at one address.

Each network of yours can be on a VLAN of its own. If you want that, the switch port the AP is on should be a trunk: your management network untagged, the others tagged. With one flat network, a plain port is fine.

## 5. Join each AP

On each AP, as root (`ssh root@<the AP's address>`), paste the line the installer printed:

```sh
wget -qO- --no-check-certificate https://<manager's address>:8443/install | sh -s -- -f <fingerprint>
```

- **Use the manager's IP address, not its name.** OpenWrt's DNS refuses a name that answers with a private address.
- **"No one uplink port found"** means the AP's bridge has more than one port. Run the line again with `-u <port>` added after `-s --`, naming the port your network is plugged into. `ip link` lists them, such as `lan1` or `eth0`.

The AP installs the agent and joins the manager. It waits there in the **Landing Zone**, and nothing changes on it until you adopt it.

## 6. Folders, and adopting the APs

Everything in the manager hangs from your Org, in two trees ([how Aeolus holds a network](concepts.md)):

- **Locations:** where your APs are. It starts with your Org alone. **Landing Zone** sits above it, apart: an AP waits there, with no config, until you adopt it into a folder.
- **Services:** the networks your APs offer. It starts empty.

Make a folder for your APs, say `House`:

1. Open your Org in the Locations tree.
2. Click **New folder…**, give the name, then **Review** and **Apply**.

Then adopt each AP into that folder:

1. Open **Landing Zone**. Its list, "Waiting to be adopted", has each AP that joined.
2. Open an AP and click **Move…** in its heading.
3. Choose your folder, then **Review** and **Apply**.

An adopted AP takes its settings from its folder. It installs the packages Aeolus needs through the manager, which fetches each from OpenWrt once and keeps it, and restarts its Wi-Fi once.

## 7. Your first network

A network lives in a **services folder**, and an AP offers the networks of the services folders its location takes. On a new Org none is set up yet, and the first network makes one. This needs a manager of v0.267.0 or later.

Open your `House` folder and its **Networks** tab. It says "No network reaches here yet." Click **Add a network**:

- **In a new services folder:** its name, "<your Org> networks" unless you change it. House takes it.
- **SSID:** the name phones see.
- **Security:** **WPA2**, or **WPA2/WPA3** where your devices are new enough.
- **Passphrase:** 8 to 63 characters.
- **Bands:** leave them as they are. 6 GHz carries WPA3 alone, so a WPA2 network stays off it by itself.
- **Traffic:** **Primary type** **VLAN**. For **Primary VLAN**, use **1** for your main network as it comes untagged to the AP; else the VLAN your switch carries the network on.

Then **Review**. It shows one change in three steps: the new services folder, House taking it, and the network in it. Click **Apply**. The manager says "Saved as change #N", and the APs pick it up within about a minute.

**Later networks:**

- **Add a network** on the same tab puts them in the same folder.
- **Which services folders a location takes** is the **Service folders** row at the top of its Networks tab. Click it to choose between:
  - **As <the folder above>**, to take what that folder takes;
  - **Set here**, a tick a folder, with the networks each offers.
- **A services folder's page** says where it applies. **Apply at…** adds it to more locations, or takes it off.

## 8. Check it

- **Your folder's APs tab:** each AP's **State** is **In sync** once it runs what it was given, and **Out of sync** while it catches up.
- **Problems:** anything wrong.
- **Join the network** from a phone.

## Updates

- **The APs** take their agent from the manager by themselves.
- **The manager** is updated with a newer release file, for now. Check it as in step 1, then as root on the manager host:

  ```sh
  aeolus-install --from aeolus-<newer tag>-linux-amd64.tar.gz
  ```

  On Proxmox, unpack the newer file on the host and run its `deploy/proxmox.sh --ctid <the container's ID>`.
- **What is kept:** the settings, the history and the certificate the APs check the manager against.

## When something is wrong

- **On the manager host:**
  - `journalctl -u aeolus -f` follows the manager's log.
  - `aeolus fingerprint` says its certificate's SHA-256 again.
  - On Proxmox, `pct enter <id>` opens a shell in its container.
- **On an AP:** `logread -e aeolus` shows what the agent is doing.
- **An AP that never shows in the Landing Zone:**
  - It cannot reach the manager on port 8443.
  - Or the manager's address in the line is a name, not an IP address.
- **A lost password:** as root on the manager host, these print a temporary one, to change at the next sign-in. The manager is stopped meanwhile, as it holds its database while it runs:

  ```sh
  systemctl stop aeolus
  aeolus password -account <you>
  systemctl start aeolus
  ```
- **More:** [install](install.md) has the installer's options, the uplink and VLANs, and updates. [How Aeolus holds a network](concepts.md) explains folders, inheritance and the change log.
