# 0153. A name a person gives a client

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff: "I want the ability to add a name to clients, like for instance my bambu printer comes up FN net ... it doesnt appear to have a hostname and I cant give it one easily. How do I add a name in clients tab". Written up by Claude
- Refines: 0066, 0067, 0118

## Context

- **A client is shown by what it calls itself:** the host name it gave in DHCP (0065), and where it gave none, its MAC, with its maker beside it (0067).
- **Many devices give no name, or one that says nothing:** a printer shows as the maker of its Wi-Fi module, a plug as `ESP_739AF7`. Their owner knows what each is, and the page has nowhere to say so.
- **A device's own name is often not its owner's to change.**
- **Every client seen is kept for good, by its MAC** (0118).

## Decision

- **A person may give a client a name,** up to 64 characters on one line. It is kept by the client's MAC, with who gave it and when.
- **It is a note of what the device is, and nothing more:** nothing is told to the device or to an AP, no config changes, and it is not in the change log.
- **Where the page shows a client, it calls it by that name:** the Clients tab's two lists, the client's own panel, an AP's top clients, a network's clients, the roaming table, and the headings of its Live Trace and its capture. What the device calls itself is still said in its panel, and still found by the search.
- **The search finds a client by it,** in the list of those on now and in the list of everyone seen.
- **It is given from the client's own panel,** opened from any list: Rename…, as a folder or an AP has. An empty name takes the name away.
- **Who may:** naming a client needs operator on the AP it was last seen on, as putting it off that AP does (0107). Whoever may view that AP reads the name.
- **A client never seen cannot be named.**
- **A device's own name is shown from when it last gave it** (Griff, the same day: "many of the clients actually have hostnames but you cant see them until you click on the linked mac address, is there anyway to let that link be their hostname"). An AP's report says a client's name only while the AP remembers the client's DHCP, which it does not after its agent starts again, or when the client came to it from another AP; the list of those on now then showed a MAC, though the manager had kept the name. The page is sent the name each client last gave, of those seen in thirty days, and calls a client by the first of: the name a person gave it, the name its AP's report says, the name it last gave.
- **An AP's scan radio making client tests (0121) is not offered a name:** it is no device of anyone's.

## Not done

- **Alerts and notifications still call a client by its own name.** Done since (2026-10-24): every alert names a client a person named, with its MAC after, in the list, the history and what is sent.
- **The MCP adapter has no tool for it.** Done since (2026-10-24): `name_client`.
- **A device that changes its MAC, as a phone with a private address does on each network, is a different client each time,** and its name does not follow it.
- **Names are not exported or imported.**
