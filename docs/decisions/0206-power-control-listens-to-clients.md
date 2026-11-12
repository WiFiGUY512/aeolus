# 0206. Power control listens to clients

- Status: Accepted (Griff, 2026-10-22: "if one AP sees a client that no other AP can see better AND its marginal, it throws a warning about the APC and we give the manager of the system 2 options, allow APC adjustment irrespective of the clients, or use the lonliest client ... to provide an override for signal reduction"; and "watch for when a client thinks its lost connection ... and use that as an indicator that its gone too far in power reduction"; then "do that and push it when ready ... please build a monitoring page as well")
- Date: 2026-10-22
- Proposed by: Griff; written up by Claude
- Refines: 0077, 0073, 0180, 0190, 0200

## Context

- **Power control (0077) listens to the APs alone.** Each AP steps its radio's power so that the neighbours it looks for hear it at the target, −70 dBm.
- **Where the APs are close together, that takes them down to the floor.** On 2026-10-22 it did so in the house, the night it was turned on:
  - C360-AP and OpenWrt3c8f went to 8 dBm on both bands; their neighbours still heard them at −44 to −59.
  - Their clients were well enough, at −31 to −74 dBm.
  - But nothing in the rule asks what a client hears. A client only one AP reaches, at the edge, is the one a lower power loses.
- **Lowering an AP's power does not change what the AP hears of a client.** It changes what the client hears of the AP. The AP measures the client's signal on the way in, so the way out must be worked out from it.
- **Clients say when they are losing an AP.** Before they drop, they scan (probe requests while still connected), they leave and come back (rejoins, 0190), and what is sent to them is tried again more often.

## Decision

### Lonely clients, by the APs among themselves

- **Each AP says, in its hellos to its neighbours (0073), which clients it hears that are not connected to it, and how strongly.**
  - Up to 64 of them, the strongest first, from what usteer has heard of their probe requests in the last two minutes.
  - The hellos already go between the APs wherever they are, across subnets. usteer's own sharing goes only to APs on the same LAN, and the lab's are not.
- **A client connected to an AP's radio is lonely** where none of the AP's neighbours has said, in the last ten minutes, that it hears it at least 3 dB better.
  - **One no neighbour hears at all** is lonely: it stays where it is.
  - **Left out:** the APs' own test clients (their MACs start 0e:, or are the AP's own), and clients connected for less than 30 minutes, which pass through.
  - **It is the AP's to judge, at every hello,** with nothing from the manager. So it is quick, and it goes on where the manager cannot be reached.
- **What a client hears of its AP** is estimated as its signal on the way in, plus the AP's power, less 15 dBm, a client's usual power. It is the AP's power that decides it.
- **It is marginal** where that estimate is below the threshold, `apc.client_floor`, −80 dBm unless set (−95 to −60).
- **The power it needs** to stay at the threshold: the threshold, less its signal on the way in, plus 15.

### What is done about it: `apc.clients`

A Locations setting, inherited like any:

- **Unset (the default): ask.** Power control steps as before. The AP reports its lonely, marginal clients, and the manager warns, as `apc-client`, on that AP:
  - "client IoT device (24:58:7c:eb:b8:8c) on C360-AP's 2.4 GHz is heard better by no other AP; at 8 dBm it hears the AP at about −82 dBm";
  - with the two choices, set at a folder.
- **`ignore`:** power control steps by the neighbours alone, as in 0077, and nothing is said.
- **`guard`:** the loneliest client sets a floor.
  - **The floor** is the power that radio's lonely clients need, the most of them, up to its ceiling.
  - **What APC does with it:** it never steps below the floor, and steps up to it at once where the radio is below.

### Clients in distress, by the AP

- **What the daemon counts**, a minute at a time on each radio, while power control is on and `apc.clients` is not `ignore`:
  - **scanning:** probe requests from clients connected to that AP, as usteer counts them for each client;
  - **leaving:** a client gone from the radio, or back on it with its connected time begun again, by the driver's station list; and the associations usteer counts;
  - **retries:** of what is sent to its clients, from the driver's counters.
- **After each step down,** when the next step is due, ten minutes on, it compares the ten minutes after the step with the ten before. It is distress where:
  - two clients or more scan at least twice as often as before, and at least three times; or
  - clients leave at least twice as often, and at least twice; or
  - the share of what is sent that is tried again rose by 15 points or more, over 200 packets or more.
- **Not the step's doing:** where the AP's other radio, which did not step, shows the same, it is no step's doing (0200). Likewise where nothing at all was sent before.
- **What it does:**
  - **In `guard`:** the radio steps back up one step at once, and holds there, or higher, for `apc.hold` hours, 24 unless set (1 to 168).
  - **Unset:** it steps nothing back, and the manager warns, as `apc-distress`, with the two choices.
- **The AP reports every step,** with why, in its state report: `apc_events`.

### The monitoring page

- **Power,** a page of its own under Interfaces › Radios, beside Neighbours, of a folder or an AP. Power control's switch and policy move there from Neighbours. For each AP and radio, its power and ceiling, and why it is where it is: the neighbours, a client's floor, or a step back held.
- **Its lonely clients,** with what each hears and needs.
- **The steps of the last day,** with why, and what the clients did.

## Consequences

- **Power control can no longer quietly lose a client only one AP reaches.** Either it is held up, or a person has said it should not be.
- **An AP whose clients scan and leave after a step goes back up by itself,** within ten minutes, and stays there a day.
- **The estimate of what a client hears is a guess,** by 15 dBm of client power. The distress check is what catches it when it is wrong.
- **The APs share more in their hellos:** a client heard by probe is said to the neighbours. MACs only, and nothing leaves the network.

## As built

- **On the AP** (`aeolus-rrm`, `rrm.uc`):
  - every minute while power control is on, it reads each radio's stations by nl80211 and usteer's probe counts, and keeps 25 minutes of them;
  - its hellos carry `hears`, at most 32 clients it hears by probe and does not serve, with their signal; a neighbour's word lasts as its hellos do;
  - `lonely`, `client_floor` and `distress` are in `rrm.uc`, with tests (`agent/test/rrm.uc`);
  - no client is lonely by a MAC of the AP's own, its scan radio's among them, or a test client's (`0e:`).
- **The manager:**
  - takes the new fields in the state report, and keeps each step once in `apc_events` (conditions, migration 26), for 30 days;
  - `GET /v1/power`, and the MCP tool `get_power`, which the advisor (0205) reads too;
  - the alerts `apc-client:<band>` and `apc-distress:<band>`, warnings, only where `apc.clients` is unset. They name the client by MAC; the page names it.
- **The page:** Interfaces › Radios › Power. Where power control asks, a banner offers the two answers, **Guard them** and **Ignore them**, as a draft of the page's one save (0202), setting `apc.clients` where the page is.
