# 0155. More of each client in the list

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff: "now we are down to the little things, Im trying to be information rich right now, and we can pare down or graph, incorporate into visually appealing structures later"; asked "can you think of things?"; and of a device's own name under one he gave it, "'Calls itself' is unnecessary, just print it". The list of what to add is Claude's
- Refines: 0066, 0118, 0143, 0153, 0154

## Context

- **The list of clients on now showed what each AP's report says of a client,** a column a fact (0143). The manager knows more of each, kept elsewhere: how its joins went (0118), the name it gave before (0153), what it asks the DNS server for (0154).
- **A fact a click away is not seen.** What a client asks for most says what it is, and was in a window opened a client at a time.
- **For now, more is wanted on the page, not less:** what is too much is to be taken out, or drawn, later.
- **The list was cut off at its panel's edge** where it was wider than the window.

## Decision

- **The list of those on now has four more columns:**
  - **Joins:** how many times the client has come online, and how many of those failed.
  - **First seen:** when an AP here first saw it.
  - **Idle:** how long since its AP last heard anything from it, at the report.
  - **Asks for most:** the three domains it asks the DNS server for most, each with its share of its lookups over the last day (0154). The search box finds a client by them.
- **Under its band, the channel and width of the radio it is on,** where its AP has one radio on that band.
- **A device's own name is printed under a name a person gave it,** in both lists and at the head of its panel, with no words about it.
- **Rename is in each client's row,** beside Live Trace, as well as in its panel.
- **What the reports do not have is asked for once the list is drawn, and drawn in as it comes:** the clients ever seen here, for their joins; and `GET /v1/lookups/top`, what each client on now asks for most.
  - **A DNS server's log is read whole for it, once in five minutes:** the latest twenty thousand lookups of the last day, put together by the address that asked, and given to each client by the address it has now.
  - Where no log is set, the column is empty.
- **The list is scrolled across where it is wider than its panel.**
- **Every column can still be hidden, and made wider or narrower** (0143).

## Consequences

- **A printer, a plug and a television are told apart without opening any of them.**
- **An address another device held earlier in the day counts as this client's** in Asks for most: a day's lookups by an address are read, and not only since this client took it. A client's own Lookups window (0154) asks by the times it held each address.
- **Tried in the harness, 2026-10-13,** against a stand-in for the server: the columns, a name given from a row, the device's own name under it, and the list scrolled across.

## Not done

- **What the AP would have to report first:** the noise under a client's signal, what a client says it can do, what DHCP handed it.
- **The same of APs and of networks:** a radio's busy share and noise, an AP's temperature and power, its ports' errors, why it last restarted; a network's failed joins and time to get online.
