# 0149. Live Trace beside a client, in a window of its own

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff, of the client list: "I feel like live trace should be here to the left of reconnect, because I would likely be using reconnect often to test this directly, rather than near the client history inside the client link, also it should do a pop out window rather than open up at the top of the page as that jump becomes visually jarring". Written up by Claude
- Refines: 0144, 0148, 0107

## Context

- **Live Trace was two clicks from where it is used:** a client's name opened its history at the top of the list, and Live Trace was a button there. The trace then took the history's place.
- **It is used with Reconnect,** which is in the client's row: the one to see what the other brings about.
- **Opening it moved the page:** the list jumped to its top to show the panel, and the row that was pressed went out of sight, Reconnect with it.
- **A panel in the page holds the page from drawing again,** as one drawn again would take it away: with a trace open, the list under it stood still.
- **A client's Capture (0148) opened the same way.**

## Decision

- **Each client's row has Live Trace, to the left of Reconnect.**
- **A trace opens in a window that floats over the page.** Nothing in the page moves for it.
  - **It is no part of the page:** the page under it is drawn again as before, and another may be opened; the window stays.
  - **It is moved by its heading and sized by its corner,** and comes back where and as large as it was left.
  - Its own Close takes it away.
  - One trace is open at a time: another client's takes its place.
  - It opens toward the lower right, a thousand pixels wide, so the row it was opened from is still to be seen.
- **A client's Capture opens in such a window too.**
- **Every way to a trace opens it so:** the row, the client's history, and the box for a MAC at the foot of the list.
- **Reconnect says the client drops as its AP hears,** which is at once where the AP holds its line (0148), and no longer "within about a minute".

## Not done

- **Not a window of the browser's own:** it cannot be put on another screen.
- **Two traces are not to be had side by side.**
- **A failed attempt's capture, and a client's history, still open in the page.**
- **The list of everyone seen has no Live Trace in its rows:** a client there is traced from its history, or by its MAC.
