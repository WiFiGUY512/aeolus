# 0147. A failure's capture, read in the client's history

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff, asked who may start a capture and read one: "anyone with access can, also failure captures of clients should be automatic, and nested directly in the clients history timeline, the timeline should also include location in the folder tree and the AP that it occurred on". Written up by Claude
- Refines: 0135, 0118, 0146

## Context

- **A failed attempt's capture was a file and nothing else (0135):** a link beside the attempt, which gave a pcapng for Wireshark. To see what went wrong, a person left the page.
- **Reading one needed operator on its AP.** One who might only view the AP was told a capture was kept, and no more.
- **A client's attempts, listed, named each one's AP,** and not where that AP is. A client that goes between sites has attempts at APs whose names say little by themselves.
- **A capture's frames are few and plain (0146):** a client's probes, its authentication and association, the key messages, then DHCP, ARP and DNS from the wire side. Each can be said in a line.
- **The wire side of a capture held a client's own broadcast several times:** once as it came in, and once more for each other client on the network, as the AP handed it on to them. A DHCP request stood three and six times over.

## Decision

- **Whoever may view an attempt's AP may read its capture,** the file and its frames. Each reading of the file is logged with who, as before.
- **A failed attempt, opened in the client's history, has its capture nested in it:** a line, `Capture`, with how many frames and how large, that opens to every frame in words. `Download` there gives the file.
  - **A frame is a row:** how long after the attempt began, as its steps are timed, and before it for the probes that led to it; which side, Wi-Fi or wire; who from and who to; what it is; and how strongly the AP heard it.
  - **Who is said as the attempt has them:** `Client`, `AP` for the network's own address there, `Everyone`, and any other by its address, as one of the AP's other networks answering a probe.
  - **A frame the AP sent that the client did not acknowledge is marked,** with the AP's note of it.
  - **What is said of each:** a probe and the network it asks for; an authentication or association and whether it was taken or refused, with the status; a reassociation and the AP it comes from; which of the four key messages; an EAP message by its kind; a leaving and its reason. From the wire side: which DHCP message and the address it offers or gives; who ARP asks for and who answers; the name DNS is asked and whether there was an answer; a connection asked for, taken or reset; a ping.
  - **The manager reads the file it holds** when the capture is opened: `GET /v1/connections/{id}/capture/frames`. Nothing more is kept, and a capture from before this reads the same.
  - The frames are read when the line is first opened, and not before.
- **Each attempt in a client's history says where its AP is in Locations,** beside the AP's name: the folders it is in, the outermost first, the Org itself left out. `place` in `GET /v1/clients/{mac}/connections`.
- **The wire side keeps a client's own frame once,** as it came in: one going out to its network again is not kept.
- **Captures of failures stay automatic,** as they were (0135, 0146): nothing is started by hand.

## Not done

- **The frames are not put among the attempt's steps,** in one list by time: they are beside them. The steps are hostapd's account and the frames the AP's; one list would say most things twice.
- **A client's sessions, further down its page, do not say where each AP is.**
- **A capture's frames are read from the file each time it is opened.** A file is at most 1 MB and 1,024 frames.
- **The six answers an AP of six networks gives to one probe are each a row.**
- **Nothing is said of a frame's inside beyond its kind:** the networks an association asks for, the rates, a key message's fields. The file has them.
