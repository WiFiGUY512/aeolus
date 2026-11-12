# The Aeolus agent

The program that runs on each AP (0040). It's written in ucode. It ships as the OpenWrt package `aeolus-agent` (0080), which depends on the ucode modules it needs; on most images they are there already. Setting it up also adds usteer, for band steering (0050), snmpd, for SNMP (0052), vxlan and kmod-nft-bridge, for tunnels (0054), and ucode-mod-socket, for the prober (0059). It was built against OpenWrt 25.12.5 on ShedAP; it needs OpenWrt 24.10 or later, the first release whose feeds have all its modules, and its scripts speak 24.10's opkg as well as apk (0083).

| Path | What it is |
|---|---|
| `files/usr/sbin/aeolus-agent` | The agent. It enrolls, polls, renders, has the manager check the result, applies it with an automatic revert, and reports. It says again what the AP is when that changes (0122). |
| `files/usr/sbin/aeolus-setup` | Joins the AP to a manager (0080): the agent's settings, the manager's certificate, the modules the agent needs and the optional packages, then it starts the agent. |
| `files/usr/libexec/aeolus-enroll` | Joins the AP to a manager a person names (0083): it fetches the manager's certificate, checks its fingerprint, makes a manager's name resolvable past dnsmasq's rebind protection, and runs `aeolus-setup`. LuCI's Aeolus page runs it. |
| `files/usr/share/ucode/aeolus/render.uc` | The renderer: intent plus the current UCI in, new UCI out. It's a pure function, so it can be tested anywhere. It leaves alone a radio another service owns, such as airscan's scan radio (0081). |
| `files/usr/share/ucode/aeolus/uciexport.uc` | Writes packages as `uci export` text. The same text is checked and then applied. |
| `files/usr/sbin/aeolus-prober` | The prober (0059): it probes each tunnel's segment and its concentrator, which keeps the tunnel open too, and guards tunnel ports against loops. Its plan is the agent's UCI package, as the renderer made it. |
| `files/usr/share/ucode/aeolus/probe.uc` | The prober's frames, kernel filters and verdicts: pure, so they are tested anywhere. |
| `files/usr/share/ucode/aeolus/radius.uc` | The prober's RADIUS status request (0114), how its answers and hostapd's counts read, and the verdict: pure, so they are tested anywhere. |
| `files/usr/sbin/aeolus-rrm` | Radio resource management (0073): it marks the AP's beacons as an Aeolus AP's, listens on its channels for the others, and exchanges signed hellos with its radio neighbours over the wire. It stays idle until Aeolus turns it on. On an AP with a scan radio it listens through that radio instead, as airscan hears it (0116). |
| `files/usr/sbin/aeolus-journey` | The record of each Wi-Fi client coming online (0118): from hostapd's log lines and the client's first frames, one record an attempt, passed or failed, with every step and its time. Of one that fails it keeps a capture (0135): the client's frames as hostapd heard and sent them, read from the rings hostapd keeps (0146), with no monitor interface. The agent sends them to the manager. |
| `files/usr/sbin/aeolus-capture` | Begins a capture someone started (0148), at once: `aeolus-live` runs it the moment the AP hears of one on its line, and the agent at its poll where there is no line; whichever comes first begins it. It picks the interfaces (`aeolus.pcap`), starts a `tcpdump` on each, and says how long the beginning took. |
| `files/usr/sbin/aeolus-ble` | The AP's Bluetooth LE radio, lent to Home Assistant (0156): idle until `system.ble_proxy` is on (`aeolus.aeolus_ble`), then a Bluetooth proxy as ESPHome's is, on TCP port 6053. It has the controller scan (`hcitool`), passively unless a Home Assistant asks for an active scan, reads what it says (`hcidump`, or `btmon` where BlueZ 5.87 has no hcidump, 0281), and hands every advertisement to each Home Assistant connected. It lets the AP's own survey have the radio and takes it back, and says how it does in `/var/run/aeolus/ble.json` for the agent's report. Tried by hand with `-f` (run whatever Aeolus says), `-p <port>`, `-d <directory>` and `-v`. |
| `files/usr/share/ucode/aeolus/btad.uc` | What the AP's Bluetooth radio hears, device by device (0157): an advertisement read as airscan reads it, the table `aeolus-ble` keeps (`/var/run/aeolus/ble-devices.json`), and what the agent reports of it and of airscan's survey together, the addresses that rotate counted, not listed: pure, so it is tested anywhere (`test/btad.uc`). |
| `files/usr/share/ucode/aeolus/esphome.uc` | The little of ESPHome's native API a proxy that only listens needs, and the reading of the controller's packets as `hcidump` writes them and as `btmon`'s btsnoop has them (0281): pure, so it is tested anywhere (`test/ble.uc`). |
| `files/usr/sbin/aeolus-live` | The line the AP holds open to the manager (0145): the manager says on it at once what would wait for a poll, which today is whom to trace (0144); what `aeolus-journey` sees of those clients goes up it as it is seen, and the AP's clock is reckoned against the manager's so that every time it names can be put on the manager's. Nothing rests on it. |
| `files/usr/sbin/aeolus-clienttest` | The client test (0121): on an AP with a scan radio, that radio joins one of the AP's networks as a client would, and each step of its way online is timed. The agent starts it for a client-test action. |
| `files/usr/share/ucode/aeolus/pcap.uc` | A capture someone starts (0148): which interfaces it takes for a network or a VLAN, one side or both, with what tcpdump is told to keep, and the script that takes it: pure, so it is tested anywhere. The agent starts it and sends its files up in pieces. |
| `files/usr/share/ucode/aeolus/clienttest.uc` | What wpa_supplicant is told and what its events mean, a DNS question and its answer, and what a test's findings come to: pure, so they are tested anywhere. |
| `files/usr/share/ucode/aeolus/journey.uc` | What hostapd's lines mean, the packet filter, how each frame reads, and the record of an attempt: pure, so they are tested anywhere. |
| `files/usr/sbin/aeolus-steer` | The steering of clients, a network at a time (0138): it hears hostapd on the networks Aeolus has steered, looks every five seconds at who is on, and asks hostapd for a transition request, a client kept off for a moment, or one put off. hostapd never waits for it, and what keeps a client off ends by hostapd's own timer: stopped, clients join as on a network that is not steered. What came of each asking goes to the agent's report. |
| `files/usr/share/ucode/aeolus/steer.uc` | What is asked of a client, when, and what is made of where it then is: pure, so it is tested anywhere. |
| `files/usr/share/ucode/aeolus/rrm.uc` | Its advert, HMAC-SHA256, hellos and choice of neighbours: pure, so they are tested anywhere. |
| `files/etc/init.d/aeolus` | procd service: the agent, its key agent, the prober where ucode-mod-socket is installed, and radio resource management where ucode's nl80211, digest and socket modules are. It starts nothing until `aeolus-setup` has run, and after a sysupgrade it puts back a missing module first (0080). |
| `files/lib/upgrade/keep.d/aeolus` | Keeps the agent, the token, the certificate and the settings across a sysupgrade. |
| `openwrt/aeolus-agent/Makefile` | The package (0080): exactly the files under `files/`, for every architecture. |
| `luci/` | LuCI's Aeolus page, under Services (0083): the manager's name or address, its certificate's fingerprint, the uplink, and a button that joins. |
| `openwrt/luci-app-aeolus/Makefile` | The page as a package: exactly the files under `luci/`. |
| `web-install.sh` | The installer the manager serves at `/install` (0083), with the manager's URL filled in. |
| `find/files/` | The finder (0151), which an image carries so that an AP finds a manager by its name and joins it: `aeolus-find`, plain shell; `aeolus-find-mdns`, which asks for `aeolus.local` by multicast DNS; and its procd service. None of it is the agent's, and no bundle carries it. |
| `openwrt/aeolus-find/Makefile` | The finder as a package: exactly the files under `find/files/`. |
| `install.sh` | Installs the agent from a copy of this directory: copies the files, then runs `aeolus-setup`. |
| `test/` | Test cases for the renderer: an intent, the config it starts from, and the UCI it must produce (`cases/*.uci`); `probe.uc`, which prints the prober's frames and verdicts, with `probe.out`, what it must print; and `rrm.uc` and `rrm.out`, `radius.uc` and `radius.out`, `journey.uc` and `journey.out`, and `steer.uc` and `steer.out`, the same for radio resource management, for RADIUS status, for a client coming online, and for the steering of clients. |

## Install

On the AP, from the manager it will join (0083):

```sh
wget -qO- --no-check-certificate https://192.168.20.60:8443/install | sh
```

It prints the manager's certificate's fingerprint, installs the agent, and LuCI's Aeolus page where LuCI is, with every file checked over TLS pinned to that certificate, then joins the AP to the manager with the uplink the default route leaves by. Options go after `sh -s --`: `-u <port>` names the uplink, `-f <fingerprint>` refuses a manager whose certificate is not that one, and `-n` installs without joining. On the manager, `aeolus fingerprint` prints the fingerprint to compare. The script itself comes over unchecked TLS, so the first fetch trusts the network between the AP and the manager, as copying a certificate by hand trusts the copy.

An AP whose image has the finder (`aeolus-find`, 0151) needs none of that where the manager can be found by its name: while it has no manager it looks for `aeolus` in the domains its network's DHCP gave, asked of the DNS servers DHCP gave, and for `aeolus.local` by multicast DNS, which the manager answers on its own network. What answers from a private address is asked for this same installer, and the AP joins. So give the manager the name `aeolus` in the domain your DHCP hands out, and name it so in the manager's certificate (`AEOLUS_HOSTS`), or it is joined by its address. `aeolus-find look` on an AP says what it finds and joins nothing; `logread -e aeolus-find` is its log; `/etc/init.d/aeolus-find disable` turns it off. The manager is trusted as found, as with the `wget` above.

To join later, or to a manager the AP did not find by itself: LuCI's Services, Aeolus page takes the manager's DNS name or IP address, shows its certificate's fingerprint for you to check, and joins. Without LuCI, the same from a shell:

```sh
aeolus-enroll cert aeolus.example.com
aeolus-enroll enroll aeolus.example.com <fingerprint> [uplink]
```

With the package, copy it and the manager's certificate (`/etc/aeolus/tls.crt` on the manager) to the AP, then:

```sh
apk add --allow-untrusted /tmp/aeolus-agent.apk
aeolus-setup https://192.168.20.60:8443 wan /tmp/manager.crt
```

The package is not signed (0080); it comes from the manager you chose. Installed, the agent does nothing until `aeolus-setup` runs, and `apk del aeolus-agent` takes it away again, leaving the AP's network as it is.

Without the package, copy this directory and the certificate to the AP, and run `sh install.sh` with the same arguments: it copies the files and runs `aeolus-setup`.

The arguments are the manager's URL, the uplink port (the one carrying the VLANs), and the certificate the agent will trust. Setup installs usteer for band steering and turns its steering off, so it steers nothing until Aeolus asks (0050). It installs snmpd too, turned off and without OpenWrt's default communities, until Aeolus turns SNMP on (0052). This needs the AP to reach OpenWrt's package feeds, through the manager (0069); without them, everything else works. Then setup checks the agent can reach the manager, and starts it. The AP enrolls into Landing Zone and waits for a person to adopt it. `logread -e aeolus` shows what it is doing.

OpenWrt's dnsmasq has rebind protection on by default, which drops DNS answers that point to private addresses, so an AP can't resolve a LAN name for the manager by itself. `aeolus-enroll` adds the manager's name to dnsmasq's `rebind_domain`, as the agent does for the names it uses (0069); with `aeolus-setup` alone, use the manager's IP address.

## Building the package

In an OpenWrt buildroot or SDK, add this directory's `openwrt` as a feed and build it:

```sh
echo "src-link aeolus /path/to/aeolus/agent/openwrt" >> feeds.conf
./scripts/feeds update aeolus && ./scripts/feeds install aeolus-agent luci-app-aeolus
make package/aeolus-agent/compile package/luci-app-aeolus/compile
```

## Tests

`internal/rendercheck` in the manager holds both halves of the contract:
- Each `cases/*.uci` must pass the render check and leave every section Aeolus doesn't own unchanged. A radio another service owns must come out exactly as it went in (0081).
- Where ucode is installed, the renderer must produce exactly that output, and `test/probe.uc` must print `test/probe.out`, as `test/rrm.uc`, `test/radius.uc`, `test/journey.uc`, `test/steer.uc`, `test/ble.uc`, `test/btad.uc` and `test/clienttest.uc` must theirs. CI builds ucode to run this.

To run one case by hand:

```sh
ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/render.uc agent/test/cases/sandbox.json
```
