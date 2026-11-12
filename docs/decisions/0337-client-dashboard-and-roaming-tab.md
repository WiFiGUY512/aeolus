# 0337. A client's dashboard, and Wi-Fi roaming a part of Clients

- Status: Accepted (Griff, 2026-11-07)
- Date: 2026-11-07
- Proposed by: Griff:
  - "we should have a 'wifi Roaming' underneat the WiFi selection for clinets so wifi selects client list, wifi roaming selects roaming screen"
  - "Instead I want it to be a general dashboard like the overall, but instead for the client itself, and on a sub tab in that dashboard I want the 'what it did' list (scrollable not one giant BLAP). The format is great though and clicking on the event that oepns it, that should be the same."
  Written up by Claude
- Refines: 0136 (roaming), 0222 (a client's page), 0255 (why clients left), 0305 (the Overview's roams cell)

## Context

- **Roaming was hard to find.** Since 0305 it was a folded panel at the foot of a folder's Clients tab, under the client list; the Overview's "Roams, 24 h" cell opened it there. Griff had to ask where it went twice.
- **A client's page was one long list.** It had who the client is and where it is now, then every attempt to come online, the latest hundred, then why it left and where it stayed. To see how a client is doing meant reading down a list.

## Decision

- **Clients has parts: Wi-Fi, Wi-Fi roaming, Bluetooth.**
  - **Wi-Fi** is the client list.
  - **Wi-Fi roaming** is the roaming panel (0136), open, with why clients left (0255) under it.
  - **Bluetooth** stays as it was (0157).
  - An AP's Clients has Wi-Fi and Wi-Fi roaming the same way; its Bluetooth stays a tab of its own.
  - The Overview's roams cell leads to Wi-Fi roaming.
- **A client's page has three tabs: Overview, Activity, Where it stayed.**
  - **Overview** is its dashboard, as a folder's Overview is one:
    - A strip: where it is now; its signal over the day (typical, and weakest); the rate to it and from it; its attempts to come online, and how many failed; its roams and how long they took; what it moved down and up.
    - Over time: its signal, a line a band, so a move between bands shows; its rates; its traffic. One time axis, from its minutes (0207), a minute at a time over a day, or in 10-minute spans over a week.
    - Beside it: what to look at, and its latest attempts, each leading to itself on Activity.
    - Under it: why it left.
  - **Activity** is what it did, as before: the same lines, each opening to its steps. It sits in a box that scrolls, its heads staying in view, with older attempts a click down.
  - **Where it stayed** is its sessions over a day or a week, in a box that scrolls.
- **Addresses.**
  - `#/clients/<mac>` is the Overview.
  - `#/clients/<mac>/activity/<attempt>` opens an attempt; the address from before the tabs, `#/clients/<mac>/<attempt>`, still does.
  - `#/clients/<mac>/stays` is where it stayed.
  - A link to a roam that went badly, from Roaming, opens it on Activity.

## Refined (Griff, 2026-11-07, after v0.299.0)

Griff: "at the click of the user, I think it should go to the client overview first, not the what it did list (activity), also shorten the list to the last 20 incidents, the scroll will let them go back further if they want".

- **A client's name leads to its Overview, wherever it is.** In Roaming, a roam's time leads to that roam on the client's Activity, opened. The client's name no longer does.
- **Activity shows the latest 20 attempts.**
  - Its box is a window about 20 lines tall, at most 640 px, its scroll bar always down its side (Griff: "a window to this with scroll bars on one side", "20").
  - Scrolled to its end, it reads 20 more, and so on back; not before, so nothing is read that no one scrolled to. Older attempts, the button, does the same.
  - A link to an attempt older than those reads back to it, ten pages at most, and opens it.
  - The page drawn again each minute keeps what was read and where the box was scrolled, reading the latest as far back as that, so no gap opens.
- **The Overview's strip reads the day's attempts whole** (`?since=`, 200 at most).

## Consequences

- **Roaming is a click from the tab bar:** Clients › Wi-Fi roaming.
- **A client's page answers "how is it doing" first.** The detail is a tab away, and the list no longer runs the page's length.
- **The UI alone changes.** It reads what the manager already gives: the client's sessions, attempts, minutes and leavings. Nothing new is asked of the APs.
