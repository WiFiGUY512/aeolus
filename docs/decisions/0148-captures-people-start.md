# 0148. Captures people start

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff: "we want to work on explicit captures, inline wired and wireless, with ssid and vlan selection"; inline being "in the APs path, I want 3 options, wireless only, wired only or wired and wireless together in the same pcap"; "I want to directly click on a client to cap just that clients traffic, I want to be able to work directly on the AP to collect all ssid or vlan traffic ... A Captures tab should be there and hold historic captures"; and of who may, "anyone with access can". Written up by Claude
- Refines: 0104, 0135, 0145, 0147

## Context

- **The captures there were are of failures alone (0135, 0146),** made by the AP when a client does not come online: a few seconds of its coming, and nothing a person asks for.
- **What crosses an AP can be taken on the AP itself, on both sides of it.** Read on the lab's three kinds of AP, 2026-10-13:
  - **The Wi-Fi side of a network is its interface on each radio** (`phy0-ap4`), a port of the bridge. A client's frames there are Ethernet, already out of their encryption.
  - **The wired side is the port the network leaves by,** the AP's uplink, where the network's VLAN is tagged, or untagged where it is the port's own. A network in a tunnel has the tunnel for its wired side.
  - `tcpdump` on the uplink sees the VLAN's tag and keeps by it; on the network's interface it sees every client's traffic there.
- **The same packet is seen on both sides,** some tens of microseconds apart: taken together they say whether the AP handed a packet on, and how long it took.
- **`tcpdump` is on some APs and not on others:** on the C-360 and the OpenWrt One, not on the R7800. With its library it is half a megabyte.
- **An AP's memory is where a capture can be written:** 227 MB free on the R7800, 482 MB on the OpenWrt One, 905 MB on the C-360.
- **An AP acts when a person asks through its actions (0104),** which it takes up at its next poll, up to a minute on, and which wait ten minutes for it at the most.
- **Griff, of a capture that began at the AP's poll:** "we need the capture to start as immediately as possible, this is a timing situation for testing things, there cant be a big time rubberband". Someone who asks for a capture is about to do the thing they want captured.
- **An AP holds a line open to the manager (0145),** on which the manager speaks to it at once.
- **On the R7800, from the AP hearing of a capture to `tcpdump` listening on each of three interfaces took 94 ms** (2026-10-13): the tool's start, the look at the AP's networks, and `tcpdump`'s own.
- **A capture holds what people sent.** Its first bytes are who spoke to whom and by what; the rest is what they said, where that was not encrypted.

## Decision

- **Whoever may view an AP may have it take a capture,** stop it, read it and delete it. Each asking and each reading is logged with who.
- **A capture is of one of three things:** one client's traffic on a network; all of a network's; or all of a VLAN's.
- **And of one of three sides:** the Wi-Fi side, the wired side, or both together in one file.
- **It keeps the first 128 bytes of each packet,** unless whole packets are asked for.
- **It runs for a minute unless told how long,** five seconds to five minutes; and may hold 20 MB of packets' first bytes, or 50 MB of whole packets, and no more than a quarter of the memory the AP has free. An AP with less than 1 MB to give takes none.
- **The AP takes it with `tcpdump`,** one for each interface, each told by the kernel's own filter what to keep: the client by its address, the VLAN by its tag.
  - **On the Wi-Fi side:** the network's interface on each radio, or on the bands asked for. Of a VLAN: the interfaces of every network on it.
  - **On the wired side:** the uplink, where it carries the VLAN; where it does not, the ports that do; of a network in a tunnel, the tunnel.
  - Eight interfaces at the most.
  - **`tcpdump` is installed when first needed** (`tcpdump-mini`), through the manager's cache of packages (0069).
  - Its files are in the AP's memory while it runs, and go when it is done.
- **One runs on an AP at a time.**
- **It begins at once.** The manager says it on the AP's line the moment it is asked for, with what to take; `aeolus-live` hears it and runs `aeolus-capture`, which begins it, beside the daemon and before the agent has so much as asked.
  - **The agent is woken too,** fetches the action as it would at a poll, and finds the capture begun: it is begun once, whichever comes to it first, one at a time being let in.
  - **An AP that holds no line takes it up at its next poll,** as before, and the page says which of the two it is: `Starting`, or `Waiting for the AP's poll`.
  - **The AP says how long it took to begin** after it heard, by its own clock, counted to when each `tcpdump` has its file open: the page shows it, for whoever is timing something against it.
  - `tcpdump` is installed ahead on every AP, as `ustp` is where it is needed (0095), so that the first capture does not wait for it.
- **Every action is said on the line so (0104):** that something waits. The agent, which slept from poll to poll, now sleeps a tenth of a second at a time and wakes when `aeolus-live` says so: a disconnect, a restart of the Wi-Fi, a client test begin within the second.
- **A page with a capture yet to begin is read again each second,** and every two while one runs.
- **The AP sends the files up in pieces as they are written,** every two seconds, a quarter of a megabyte a piece: `POST /v1/ap/pcaps/{id}`. Each answer says how much the manager holds, and whether someone asked for the capture to stop: so it stops within seconds.
- **The manager keeps a capture as files beside its databases,** one for each interface, and gives it out as one pcapng: `GET /v1/pcaps/{id}/file`.
  - **Each interface is in it by name, with what it is:** `phy1-ap4`, "wireless: Home_Wi-Fi_IoT, 2g"; `wan`, "wired: VLAN 10".
  - Their packets are together, by when each crossed.
  - One still running gives what has come so far.
- **A capture is an action of its AP (0104), of kind `capture`:** what was asked is kept with it, and what came of it, as the AP says: how many packets, why it ended, and any interface it could not take.
- **Sixty are kept, the latest, for thirty days.**
- **In the manager:** `POST /v1/aps/{ap}/pcaps` asks for one, `GET /v1/pcaps` lists them, `POST /v1/pcaps/{id}/stop` and `DELETE /v1/pcaps/{id}`.
- **In the page, three places:**
  - **A Captures tab, beside APs,** holds those that were taken: when, on which AP and where it is, of what, which sides, how many packets and how large, who asked, and how it stands, with Download and Delete, and Stop for one that runs.
  - **An AP's Capture tab takes a network's traffic or a VLAN's:** which of the two, the side as a choice of three, the bands, for how long, and whole packets as a switch that is off. Below it, the captures taken on that AP. Start is not to be had while one runs there.
  - **A client's page has Capture,** beside Live Trace: that client's traffic alone, on the AP and network it is on, with the same three sides.

## Not done

- **An AP's RF Capture tab is not there:** it waits for the capture of the air by a scan radio.
- **A capture's packets are not listed in the page,** as a failure's frames are (0147): the file is for Wireshark.
- **How long from the press to the AP hearing is not measured:** the manager writes to the line as it answers the press; the rest is the network between them.
- **The whole of it, from the page through a manager to an AP's line, is not tried:** each part is, the manager's by its tests and the AP's on the R7800 from /tmp.
- **It ends within five minutes,** as an action waits ten for its AP, the sending included.
- **A client is followed on one AP.** One that roams is in the capture until it goes.
- **A client in a VLAN of its own, by its key or its sign-in, is not found there:** its Wi-Fi interface is another (`phy0-ap4-k20`).
- **The same packet is in a file of both sides twice,** which Wireshark calls a retransmission. Its interface says which side each is.
- **What it costs an AP that is busy is not measured.** Each packet on an interface taken passes the kernel's filter once more.
- **A radio that hands packets to the wire in hardware would hide them from it.** None of the lab's APs does.
- **Tried from /tmp on the R7800, the manager stood in for on the AP:** one client on both sides, 191 packets in 16 s; a VLAN's wired side, stopped at ten seconds of forty; `tcpdump` installed on the way. The page was tried in the harness, the AP stood in for with those files: asked for, running, stopped, done, and read as one file. Not tried from the page to a real AP, nor on the other two kinds.
