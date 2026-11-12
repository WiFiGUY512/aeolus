# 0284. The screens, redesigned

- Status: Accepted (Griff, 2026-11-01: "looks good do it", on the mockup of the screens)
- Date: 2026-11-01
- Refines: 0047 and 0072 (the tabs), 0099 and 0160 (alerts and threats), 0157 (Bluetooth), Griff's rule that bands are never mixed (2026-10-22)

## Context

- **Griff finds the UI redundant, and not dense enough** (2026-10-04). He asked for this redesign once Aeolus was viable: "stable, then the screens nice".
- **A survey of every page (2026-11-01) found:**
  - **The same facts in several places.**
    - Clients by band: twice on each Overview.
    - Join failures: three places.
    - AP lists with status: five or more.
    - Rogues: both Networks heard and Threats.
    - Radio facts, uptime and version: on many panels.
  - **Three levels of tabs:** Interfaces › Radios › its views.
  - **The band rule broken.** Several tables put 2.4, 5 and 6 GHz numbers side by side, and each band picker was a panel's own.
  - **Prose under every heading.** For example, "as each AP last reported" appears 16 times, and "Applying restarts…" 15 times.
  - **No way to find an AP or a client from anywhere.**
- **A mockup made from the lab's live data was put to Griff on a design canvas** (https://claude.ai/artifact/JwepFwEXo6hHtVYDniWE5N). It has five boards: the folder overview, an AP, Clients, Settings, and where everything goes. He took it as drawn.

## Decision

The redesign lands in parts, each a release of its own. The tree keeps its APs' status dots: Griff watches a change land there (2026-10-06).

1. **Navigation (this part).**
   - **A folder has nine tabs in two levels:** Overview, APs, Clients, Radios, Wired, Networks, Problems, Assurance, Settings.
     - **Radios** is the old Interfaces › Radios. Its views are its parts: Settings (the band cards), Neighbours, Power, Load, Ratings, Networks heard.
     - **Wired** is Ethernet, Tunnels and Network Paths.
     - **Clients** has Wi-Fi and Bluetooth.
     - **Problems** has Alerts, Rogues and Rogue DHCP.
     - **Settings** has System and Templates. They are one page once Settings is redone (part 5), when its two saves become one.
   - **An AP's tabs:** Overview, Radios, Wired, Networks, Clients, Assurance, Bluetooth, Health, Settings. A capture is taken from **Capture** in its header.
   - **One band picker a page, on its tab row** (or its parts' row), for every radio number on the page.
     - It is on Clients, Radios (but not its Settings part) and Assurance.
     - Clients alone may show all bands.
     - The views read it (`pageBand`) and no longer draw their own.
     - Radios › Settings keeps the three band cards side by side: they are settings, not a table of numbers, and switching bands would draw the page again and lose a draft.
   - **The top menu:** Locations, Services, Library, Changes, Advisor, Admin.
     - The APs page and the Captures page keep their addresses and leave the menu. Every AP is on the Org's APs tab, and an AP's Capture links to every AP's captures.
   - **Find, in the header:** an AP or folder by name or ID, a client by the name a person gave it, by MAC, or by part of its host name, MAC or address.
   - **Old addresses still work:** each is rewritten to its new tab (`moved`), as 0072 did for Hardware.
2. **The Overview.**
   - One status strip.
   - The APs on the picked band, with Needs attention beside them.
   - Moved the most.
   - One clients chart. Roaming and Why clients left move to Clients.
3. **The AP page.**
   - Its header carries its state, model, address, OpenWrt, agent, version and uptime, in place of the status panel and tiles.
   - Its Overview has the radio on the picked band with its clients, then Lately and About.
4. **APs and Clients.**
   - **APs:** one table, with the radio columns of the picked band only.
   - **Clients:** fewer columns by default, the weakest first.
5. **Settings.** The setting groups side by side, where each value comes from said in a word, and one save bar.
6. **Words.** One "reported" time a panel. Help behind the heading's title, not under it. Notes about agent versions every AP is past are gone.

## As built

- **Parts 1 to 3** were v0.223.0, v0.224.0 and v0.225.0. **Parts 4 to 6** went out together as v0.226.0: 4 and 5 shared files, and 6 is words.
- **Part 4, APs:**
  - The APs table gained where each AP is and the version it runs.
  - The Org's APs tab took the old page's "Detected, not enrolled", and `/aps` leads there.
- **Part 4, Clients:**
  - The list shows eight of its columns until its viewer shows more. A table names those in `data-hide` (tables.js), and "As it came" hides them again.
- **Part 5, Settings:**
  - The setting panels flow in as many columns as fit, a line each. Where a value comes from is a word at its end (`whence`): "here", or the folder's name as a link.
  - System's map comes after its settings.
  - System and Templates were two parts at first: each had its own save, and two saves on one page would fight over the bar.
  - Since then they are one page with one save. A page's save takes what each section sharing it says of the change (`also`, each of its own fields), so one preview says them all.
  - Each value is changed in its own row since 0286, as the mockup drew it.
- **Part 6, words:**
  - "As each AP last reported" left the headings: every live view is drawn from the reports, again every half minute. An AP whose report is old still says so.
  - Notes about agent and manager releases every AP is past went.
  - The clients list's long note moved into its heading's title.

## Consequences

- **Each part is a release of its own.** The pages not yet redone keep their look until their part lands.
- **Links and bookmarks to the old tabs keep working.**
- **The band picker's choice is still one choice, kept across pages and visits** (`aeolus.band`).
