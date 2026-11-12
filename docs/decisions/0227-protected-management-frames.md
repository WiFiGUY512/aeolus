# 0227. Protected management frames, set on a network

- Status: Accepted
- Date: 2026-10-28
- Proposed by: Claude, from the roam tests (0225, 0226); Griff: "Do it. Then recount the devices to see which ones didn't come back". Written up by Claude
- Refines: 0086 (WPA3 or OWE on 6 GHz), 0136 (roaming)

## Context

- **Aeolus left protected management frames (802.11w) to OpenWrt.** OpenWrt's defaults:

  | Security | Protection |
  |---|---|
  | WPA2 alone | off |
  | WPA2/WPA3 | optional |
  | WPA3 alone or OWE | required |
  | Any network on 6 GHz | required, as it is WPA3 alone there (0086) |

  - A network imported from an AP's own (0137) lost the setting, as one "not carried over".
- **On the C-260's 2.4 GHz radio (ath11k), a WPA2 client was let on ~40 ms later where the network had protection off.** Roam tests, 2026-10-28, WPA2 with a passphrase onto aeolus_50, from C360-AP's scan radio, timed in hostapd's own debug log:
  - **Protection off:** the client's key took 31 to 65 ms to store in the radio, typically 42 ms (10 roams).
  - **Protection optional:** 0 to 2 ms (9 roams).
  - The test client used no protection either way, and hostapd asked the radio the same things in the same order. Only the network's setting differed, and the driver took the time.
  - It is the porting session's to take up with the driver (their B94). Aeolus does not work around it, but it can set what the network should have anyway.
- **Optional protection costs a client that cannot use it nothing,** as a rule. It also keeps a client that can from being thrown off by a forged deauthentication, the trouble once suspected of the IoT network's rejoins.
- **Some old devices refuse a network that offers protection,** even as optional, as they misread its RSN element. They are mostly IoT devices.

## Decision

- **A network that takes WPA2 has `pmf`:** `optional` or `required`. Unset, it is OpenWrt's default, as before.
  - **Where it applies:** WPA2, WPA2/WPA3, WPA2-Enterprise and WPA2/WPA3-Enterprise. On WPA2/WPA3, `required` refuses every client that cannot use protection, WPA2 or not.
  - **How it is rendered:** as hostapd's `ieee80211w`, `1` or `2`, on each of the network's BSSes but those on 6 GHz. There the network is WPA3 alone, and OpenWrt requires protection.
  - **Where it is refused:** set on a network of WPA3 alone or OWE, which always require it, or on an open network, which has no keys to protect frames with. The manager refuses such a change, and an AP's agent refuses such a config.
  - **What the render check holds the AP to:** the option as the network says, and none where it is unset.
- **Importing an AP's own network carries `ieee80211w` 1 or 2 over** as `pmf`, where the security takes it.
- **The lab's WPA2 networks get `optional` one at a time:** aeolus_50 first, then Home_Wi-Fi_IoT. Their clients are counted before and after, to find any that do not come back.

## Consequences

- **A WPA2 client is let on without the 40 ms wait** on the APs whose driver keeps it, and is protected from forged deauthentication where it can be.
- **Setting it restarts the network's BSSes,** as any change to them does. Its clients drop for a few seconds and join again.
- **A device that will not join a network offering protection is found by the count,** and the network can be set back: unset, and it is as before.
- **Nothing changes until it is set.** The release itself renders as before.
