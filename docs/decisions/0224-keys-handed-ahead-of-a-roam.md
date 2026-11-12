# 0224. Keys handed ahead of a roam, AP to AP, over the wire

- Status: Accepted
- Date: 2026-10-27
- Proposed by: Griff: roams seemed slow; "are the APs truly sharing authentication info on the wire about the devices that may roam to them and are they caching it properly?" Then: "The goal is to make all roaming types, wpa2 and 3 precache everything it can from the neighbor APs however it can be done, all over the wire specifically and unicast." Written up by Claude
- Refines: 0098 (WPA Enterprise), 0136 (roaming), 0209 (what APs say to each other goes over the wire)

## Context

- **The APs shared nothing.** From the first agent (2026-09-26), 802.11r was rendered with `ft_psk_generate_local`. It was a default, never decided.
  - It makes an FT-PSK client's keys on each AP from the passphrase, so WPA2 roams need nothing from another AP.
  - It does nothing for WPA3 (FT-SAE) or WPA Enterprise (FT-EAP). Their keys come from the client's sign-in with its first AP. Another AP can only have them from that AP, over the wire, and no AP had a list of the others to have them from.
  - Home Wi-Fi advertised FT-SAE all the same. A WPA3 client trying it with another AP would have been refused, and signed in from scratch.
  - 0098 left 802.11r on WPA Enterprise for later, for want of key holders, and the manager refused it. 0209 said, rightly then, that there was no AP-to-AP exchange.
- **Key caching was off but for SAE.**
  - OpenWrt turns PMKSA caching and OKC off unless pre-authentication is on or the network is SAE.
  - On Aeolus-Enterprise, every roam was a full 802.1X sign-in.
- **What the roams were, 24 h to 2026-10-27:** 71 roams. 63 were the WPA2 four messages, 77 ms typical. 8 were full WPA3 sign-ins, 141 ms typical, up to 1.2 s. None was 802.11r: the clients that roamed did not ask for it.
- **hostapd can do it all over the wire, unicast.** The hostapd on the APs is the 2026-08-07 snapshot. Its source, as OpenWrt builds it, says:
  - **Key holders.**
    - Each BSS is an R0KH and an R1KH. Its R0KH-ID is its `nas_identifier`; its R1KH-ID is its BSSID.
    - It knows the others from `r0kh=<BSSID> <R0KH-ID> <key>` and `r1kh=<BSSID> <R1KH-ID> <key>` lines.
    - It sends to them on `ft_iface`, the network's own bridge, to each BSSID, unicast: ethertype 0x890d, and 0x88b7 for pulls and pushes.
    - A receiver takes only what is sent to its own BSSID, or to a multicast address (`hostapd_rrb_receive`).
  - **Push.** With `pmk_r1_push=1`, an R0KH sends each R1KH in its list the client's PMK-R1 as the client finishes signing in (`wpa_ft_push_pmk_r1`). Wildcard entries are skipped. FT-PSK made locally stores no PMK-R0, so nothing is pushed for it; FT-SAE and FT-EAP are pushed.
  - **Pull.** An R1KH that has no PMK-R1 for a client asks the R0KH named in the client's request. It looks that R0KH up by its R0KH-ID alone, and keeps sequence numbers per entry, so every BSS needs a NAS-Identifier of its own (`wpa_ft_rrb_lookup_r0kh`).
  - **The file.** `rxkh_file` holds the lines in a file of their own.
    - A missing file is let pass at start, so it can be made later.
    - `RELOAD_RXKHS` on the control socket reads it again, without a restart.
    - OpenWrt's `hostapd.uc` does the same when only the file changed under netifd.
  - **The RADIUS server.** It is sent `nas_identifier` as NAS-Identifier unless `radius_auth_req_attr` (and `radius_acct_req_attr`) carries attribute 32 (`add_common_radius_attr`).
  - **Disconnects.** A Disconnect-Request that names a NAS-Identifier other than the BSS's own is refused (`hostapd_das_nas_mismatch`).
- **The wildcard OpenWrt defaults to is not unicast.** With `ft_psk_generate_local` off and no lists, OpenWrt sets `r0kh=ff:ff:ff:ff:ff:ff * <md5 of the mobility domain and passphrase>`.
  - Pulls then go to the broadcast address.
  - Nothing is pushed.
  - The key is made from the passphrase, which every client of the network has.

## Decision

- **Each BSS of a network with 802.11r is a key holder of its own.**
  - Its NAS-Identifier is `<AP ID>/<radio>/<network's hash>` (`rendercheck.FTNASID`). The network's hash is the FNV-1a of its name that names its bridges (0061).
  - Passphrase keys are still made on each AP (`ft_psk_generate_local`): FT-PSK works even with the first AP gone.
  - `pmk_r1_push=1`.
  - `rxkh_file=/etc/aeolus-ft/<section>`. On flash: after a restart, a BSS has its last list before the agent runs.
  - A sysupgrade keeps the folder (`keep.d`). Without it, each BSS of such a network logged `rxkh file ... not found` as it started after the flash, and took no FT roams till the agent's first poll wrote the files again (the C260's -118 flash, 2026-11-04).
- **The agent says each such BSS in its state report:** `ft`, its network, BSSID and NAS-Identifier, from the section's `aeolus_ft` option and its interface's address.
- **The manager hands each AP every key holder of its site** at `GET /v1/ap/ft`.
  - The site is the folder right below the Org that the AP is in. Clients roam within a site, and it keeps the lists, and the pushes, from growing with the whole fleet.
  - It takes each AP's last report of the last day; an AP in Landing Zone is in no list and is handed none.
  - It hands a key for each network, 256 bits made from the manager's secret key (`ft/<network>`). It is not made from the passphrase: a client of a WPA3 network knows its passphrase, and must not be able to ask an AP for another client's keys.
  - Every poll's answer names the version in an `Aeolus-FT` header. The key and the lists come beside the config, so they version nothing.
- **The agent writes each BSS's file:** every other key holder of its network, as an R0KH by its NAS-Identifier and an R1KH by its BSSID, with the network's key.
  - hostapd runs as the `network` user in a jail (ujail), so the files are root's, readable by the `network` group alone, in a folder of their own: `/etc/aeolus` is closed to that user.
  - hostapd answers its control socket as that user too, so the agent's end of it is the `network` user's.
  - It has hostapd read the file again over the BSS's control socket (`RELOAD_RXKHS`), once the BSS runs.
  - It takes away the files of BSSes gone.
  - Without ucode's socket module, hostapd reads the file when the BSS next starts.
- **Over the wire, unicast, on the network's own VLAN.** hostapd's `ft_iface` is the network's interface. It sends to each BSSID, which only that AP's bridge delivers to itself. Nothing goes to the broadcast address, nor over the air.
  - FT stays over the air between client and AP (`ft_over_ds=0`). Over-the-DS has a client send its request through its current AP, and many clients do it badly.
- **WPA Enterprise keeps its clients' keys and takes sign-ins ahead of a roam.**
  - `okc=1`: PMKSA caching, and OKC among the AP's own BSSes.
  - `rsn_preauth=1`: a client may sign in with another AP through the one it is on. Its frames go from that AP over the wire to the other's BSSID, and the other runs the 802.1X sign-in with the RADIUS server, ahead of the roam.
- **802.11r is rendered on WPA Enterprise:** the manager no longer refuses it.
  - Where `radius.nas_id` is set, the RADIUS server is still told it, in attribute 32 of each Access-Request and Accounting-Request, while the BSS goes by its own.

## Consequences

- **A WPA3 or WPA Enterprise client that uses 802.11r finds its next AP holding its key.** Its roam is FT's two exchanges, about 20 to 50 ms, with no SAE or 802.1X. A pull reaches the first AP where the push did not.
- **What clients do is theirs.** A client that does not ask for 802.11r is no faster for it. The Phone-B Ultra never did in the 24 hours looked at.
  - **WPA2 without 802.11r** is the four messages whatever the APs hold.
  - **WPA3 without 802.11r** can reuse its key only with the AP it had it from. hostapd keeps the SAE PMKSA per BSS and has no way to share it. Moving keys around outside hostapd's own protocol was not done: Aeolus does not make or carry handshake keys itself.
- **Turning it on reloads each changed BSS once.** The new options (`nasid`, `aeolus_ft`, the raw lines) change a network with 802.11r, and `okc` and `rsn_preauth` change each WPA Enterprise network. Their clients reconnect within seconds. After that, a list that changes is read again with no client dropped.
- **On a WPA Enterprise network with 802.11r, the BSS's NAS-Identifier is its own.** A RADIUS server whose Disconnect-Request names `radius.nas_id` is refused by hostapd. One that names the client alone (User-Name, Calling-Station-Id) is not.
- **Pre-authentication is more sign-ins with the RADIUS server:** a client may sign in with each AP it may roam to.
- **A key holder is listed only after its AP reports it.** A new AP or network is in the others' lists a report later: about a minute. Until then a roam to or from it is a full sign-in.
- **A large site lists every AP of it.** Narrowing it to the APs that hear each other (0073) is left for later.
- **The manager must have this before the APs report it:** it refuses a report with a field it does not know. One release carries both, the manager updated first.
