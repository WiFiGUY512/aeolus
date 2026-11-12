# 0154. What a client looks up, from the DNS server's own log

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff: "I figured this was a bambu printer by watching who it contacted after a reconnect, is there a way to maybe store just the lookups (so its not so huge) of the clients in a little pop out or notepad attached to it"; "maybe it can even just get them from the ddi server technitium"; "pop out > makes API call with mac to technitium"; and "maybe we could even do a dashboard somewhere that has a chart of the places mostly visited by the client, then we could relatively easily see what kind of device the client is". And of Live Trace: "can you put a reconnect button in the live trace screen itself at the top, I often have to reconnect in order to see the transaction". Written up by Claude
- Refines: 0067, 0107, 0118, 0144, 0149

## Context

- **What a device asks for says what it is.** A printer asks for its maker's cloud; a device with no name of its own (0153) is known by it at a glance.
- **An AP notes a client's first lookups as it comes online** (0118): four of them at the most, each time, among the steps of the attempt. To see more, a person put the client off its AP and watched it come back in Live Trace.
- **The site's DNS server has every lookup already.** A Technitium DNS Server with its query logs app keeps each one with the address that asked, and answers for them over its API: by address, between two times, a thousand at a time.
  - Asked on the lab's server, 2026-10-13: it had the app (`Query Logs (Sqlite)`), 17,757 lookups, each with the client's own address, as the clients there ask it themselves.
  - It knows addresses, and no MACs.
- **The manager knows which address a client held, and when,** from its APs' reports (0066).
- **A setting in Locations goes to every AP below it,** secrets opened (0040).

## Decision

### A client's lookups

- **A client has a Lookups window,** opened from its own panel and from its Live Trace, floating as Live Trace does (0149).
- **It asks the manager, by the client's MAC, and the manager asks the DNS server:** for each time the client held an address, by its APs' reports, what that address asked for in that time. The latest four such times, over a day unless a week or thirty days is chosen, and four thousand lookups read at the most, the latest.
- **Nothing of it is kept by the manager.** The server's log is the record, for as long as the server keeps it.
- **First, the domains it asks for most,** a bar each with its share: a dozen at the most. A name's domain is its last two labels, or three under a country's shared ones (`example.co.uk`).
- **Then every name,** the one asked for last first: its kinds, how many times, when last, and whether the server blocked it or had no answer. A box finds a name among them.
- **The server is set where the client's APs are,** in Locations, under System: its address, as `http://host:5380`, and an API token of a user who may view its logs (`system.dns_log`).
  - **The token is sealed, and is the manager's own to use:** no AP is sent the address or the token, and neither is part of any AP's config.
  - The token goes to the server in the body of what is asked, and is in no address or error the manager writes.
- **Where no server is set, or it does not answer, the window shows what the APs noted as the client came online** (0118), from the attempts kept, and says which it is showing.
- **Who may:** whoever may view the AP the client was last seen on.

### Reconnect, in Live Trace

- **Live Trace has Reconnect at its top** (0107): it asks the AP the client is on to put it off, so that it is seen coming back. The AP is the one the trace says it is on, and until the trace says, the one it was on where the trace was opened.
- **It says which AP was asked,** for a quarter of a minute.

## Consequences

- **A device is named for what it is in a minute:** its window opened, its maker's domain at the top.
- **A client that asks another server, or asks over an encrypted connection of its own, shows nothing there.**
- **An address held by another device in the same time would be read as this client's.** The times asked about are those the client's APs reported it with the address, a few minutes wider at each end.
- **Tried, 2026-10-13:** against a stand-in for the server, in the harness, a client's window showed its maker's domain at 70% of sixty lookups, and its names under it; Reconnect in a Live Trace asked the client's AP. Not yet tried against the lab's own server, which needs a token of Griff's.

## Not done

- **Other servers than Technitium.**
- **A server whose certificate is its own:** an `https` address is checked as any is.
- **IPv6 addresses.**
- **A guess at what a device is, made from what it asks for** (0067).
- **More than the first four lookups noted by an AP as a client comes online.**
