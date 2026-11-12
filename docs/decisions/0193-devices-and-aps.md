# 0193. How each maker's devices fare on each kind of AP

- Status: Accepted (Griff, 2026-10-21: "we need to proactively monitor these devices and look for their compatibility issues")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0118, 0127, 0190

## Context

- **The IoT devices (0190) were one device that did not get on with one kind of AP.** Two Espressif clients came online every 45 s on the C-360s' 2.4 GHz, and held one session for days on the office AP.
  - The `rejoins` alert names each such client on its AP.
  - It does not say that it is the maker and the kind of AP, not the one client, nor where the same devices do fine.
- **Griff wants these found before anyone notices,** and later an agent that hunts client problems and incompatibilities and makes recommendations, which would read this.
- **What is needed is already kept:** every attempt to come online, by AP and band (0118); the maker of each client's MAC; and each AP's model, from its enrollment.

## Decision

- **GET /v1/compatibility (`?under=`, `?hours=` 24 unless set, at most 720) groups the clients by maker, AP model and band.** Each group says:
  - how many clients, attempts, came online and failed;
  - how often its middle client came online again while it was at an AP, an hour's rate: none for one that stayed. A client that came online fewer than 20 times there has its rate over an hour at least (2026-10-24): two joins a minute apart had read as sixty an hour, and a loop;
  - how many looped, 20 times an hour or more, as the rejoins alert counts;
  - its APs, and its worst five clients.
- **A group is trouble where a maker's clients loop there.** It says so, and where on other models the same maker's clients stay: "1 of its 1 Espressif Inc. clients here came online again about every 45s, as one that drops the connection itself; on OpenWrt One (2.4 GHz) they stay".
  - **The troubled come first.**
  - **What is left out:** a private address is no maker's, and is grouped but never trouble. The APs' own test clients are left out.
- **The Assurance tab has a "Devices and APs" panel** after "Clients coming online". It shows the troubled in words, with their worst clients, then a compact table of every maker, model and band.
- **The MCP tool `get_compatibility`** says the same, for a person or an agent to read.

## Consequences

- **A device that does not suit a kind of radio shows as that,** with where it does better: the start of a recommendation.
- **The rates are by join attempts,** so a device that stays on but fares badly (retries, a slow rate) is not counted here yet. The journey's issues are per client (0103).
- **Analysis and monitoring may use the internet** (Griff, 2026-10-21: "that rule is for AP command and control, not analysis and monitoring"). An advisor agent reading this may be tied to a frontier model.

## As built (2026-10-21)

- **Store:** `conditions.JoinedSince`, each client's attempts by AP and band.
- **API:** `internal/api/compat.go`: `compatibility()`, `rejoinRate()`.
- **MCP:** `get_compatibility` in `internal/mcpadapter`.
- **UI:** `compatPanel()` in `views/assurance.js`, styled in `app.css`.
- **Tested:**
  - **`TestCompatibility`:** Espressif looping on one model and staying on another is trouble with where they stay; a private address on both is not; an hour's window has nowhere they are seen to stay.
  - **The local harness:** the panel, its trouble line and table.
