# 0341. Pinned to its closest AP first; a packet viewer

- Status: Accepted
- Date: 2026-11-08
- Proposed by: Griff:
  - "when I select pin to the first AP that populates should be its closest one";
  - "I love that we can download the pcap, but I would also like a popout window with the packets on it, like a mini wireshark".

  Written up by Claude, and accepted with the merge.
- Refines: 0340 (pin to an AP), 0135 and 0147 (a failed attempt's capture), 0148 (captures people start)

## Context

**Picking the pin's AP.** "Pin to an AP…" (0340) gave an empty box that found an AP by name. To pick the right one, a person had to know which AP the client is nearest. The sticky list (0340) knows it for the clients it lists; the client's own page did not say.

**Reading a capture.** An attempt's capture (0135) and a capture a person starts (0148) were downloaded as pcapng, for Wireshark. An attempt's capture could also be opened in its attempt as a line of words a frame (0147).
- That lets a person read what happened, but not see a frame's fields or its bytes.
- It is nothing for a capture a person started.
- Wireshark has to be installed, and the file moved, to look any further.

## Decision

### Its closest AP first

- **The form reads the client's minutes of the last week** (0207, `/v1/minutes?mac=&hours=168`) as it opens.
- **It ranks the site's APs that heard the client:**
  - on the band the client was on most, since 2.4 GHz carries further than 5 or 6 GHz;
  - by each AP's middle signal over at least ten minutes;
  - best heard first.
- **Choosing "Pin to an AP…" with no AP picked fills in the closest.** The form says which it is, and how strongly it heard the client this week.
- **The list of APs shows those that heard it first,** each with its signal, band and how long. The others follow.

### The packet viewer

- **A window that floats over the page** (0149), as Live Trace does. It holds:
  - **The packets, one a line,** with the columns: number, time, side (Wi-Fi or wire), source, destination, protocol, length, the signal it was heard at and what it is. Lines are coloured as Wireshark colours them: trouble red, the key exchange and joining amber, addressing green.
  - **A filter** over every word of a line: `eapol`, `dhcp`, `deauth`, an address.
  - **The packet picked, taken apart** into its layers and their fields.
  - **Its bytes,** sixteen a line. A field picked marks its bytes.
  - **The file,** downloaded from the window.
  - The arrow keys move up and down the list.
- **It opens from:**
  - an attempt's **Capture** link, in a failed join's list and on the client's Activity (it used to download the file at once);
  - **Packets** beside Download in an attempt's capture;
  - **Packets** beside Download in the list of captures people started.
- **The page reads the file itself** (`packets.js`): pcapng and pcap, and each interface's link type and time resolution. It takes apart:
  - radiotap, with signal, channel, rate, flags and MCS;
  - 802.11 frames: management with their fixed fields, status and reason codes, tagged elements, RSN's ciphers and AKMs, Mobility Domain, action categories (WNM's BSS transition, radio measurement); control and data frames;
  - LLC and SNAP, Ethernet with VLAN tags, Linux cooked captures;
  - EAPOL, telling the four messages of the key exchange apart, and EAP's codes and types;
  - ARP, IPv4, IPv6, UDP, TCP, ICMP and ICMPv6;
  - DHCP with its options, DHCPv6 messages, DNS and mDNS.

  What it does not know it shows as bytes.
- **A protected management frame shorter than its cipher's header and check** is one the AP took already deciphered, as it takes its own: it is read as it is. The Living Room AP's deauthentication of a test client read "reason 3, station is leaving".
- **The manager is unchanged.** The window reads the same file the download gives, with the same right: an admin of the AP (0177).

## Consequences

- **A failed join is looked into where it is listed,** to the byte, with no Wireshark and no file to move.
- **The pin's AP is right by default** for a client that stays put. A client that moves about wants the nearest-AP rule, which the form also offers.
- **Read in the page, a capture costs nothing on the manager.** The captures Aeolus keeps are small (a failed attempt's at most a few hundred frames). A long capture of a busy network is read whole by the browser; Wireshark remains the place for those.
- **Not done:**
  - Wireshark's display filters, expert info, and following a TCP stream.
  - Telling apart the frames the client did not acknowledge: they carry the AP's comment, shown on their line.
