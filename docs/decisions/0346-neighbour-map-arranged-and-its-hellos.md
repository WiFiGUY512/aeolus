# 0346. The neighbour map, arranged by hand, and its hellos

- Status: Accepted
- Date: 2026-11-10
- Proposed by: Griff, 2026-11-10, of the neighbour map (0073): "the one thing I wish it would do is let me actively move around the APs to get a clear view of their links, the other thing I wish it would do is show a mock ping from one to the other whenever they exchange information". Built under the standing word (autonomous development). Accepted with the merge
- Refines: 0073 (radio neighbours, the map)

## Context

**The map lays itself out.** Each line pulls its ends to a length that follows its signal, and every AP pushes the others away. The least tangled of a few tries is kept. With seven APs that all hear each other on 2.4 GHz, that is 21 lines, and labels run over each other wherever the lines bunch. No layout suits every question someone brings to it.

**The map shows what the APs hear, not what they say to each other.** Over the wire, every ten seconds, each AP sends a hello to the APs it hears best and to those whose hellos come. Each report says, of each neighbour, how long ago its last hello came (`hello_ago`). The manager is not told of each hello as it goes.

## Decision

**An AP on the map can be moved.**
- Drag it, or focus it and use the arrow keys: 10 units a press, 40 with Shift. Its lines and their labels go with it.
- It stays in the drawing, its name included, and may go as far down as twice the drawing's share of the window.
- The page is not drawn again while an AP is being dragged.

**Where it was put is kept in this browser,** by the folder or AP whose page the map is on and the band (below, for the key). It is one person's view, not the Org's, so it is not a change and not logged. Where storage is blocked, the map lays itself out each time. **Lay out again** forgets the places on that map and lays it out afresh.

**The hellos are drawn going along the lines.**
- **Timing:** each hello lands at the AP that hears it every ten seconds, counted from when that AP last reported one, its report's time less `hello_ago`. A dot runs along the line for the 0.9 s before it lands, and a ring opens around the AP as it lands.
- **Which hellos:** only where hellos come, both ways or one way, and not once a hello is three late. A line heard only in the air has none.
- **Re-timed:** the page is drawn again every 30 s with what the APs last reported.
- **Where the dots go:** they follow APs wherever they have been moved.

**The dots are drawn from the timing, not seen.** They show which AP sends to which and how often, close to when: a mock, as asked. Client sightings (0289) and shares (0299) go only when there is something to say, and nothing reports them, so they are not drawn.

**The Hellos button turns them off, kept in this browser.** A browser set to reduce motion is shown none.

**The map fills its panel** (Griff, 2026-11-10, on v0.316.0: "can you have it use more of the area inside the window for the neighbor map?"):
- **Size:** the drawing is as wide as its panel, drawn at one unit a pixel, so names and labels keep their size and the lines lengthen. The layout is spread to up to 72% of the window's height.
- **Fitted to the APs** (Griff, on v0.317.0: "can we have the screen dynamically shrink with the AP used area?"): the drawing shows only the band the APs are in, from just above the highest to below the lowest one's name. It is fitted when drawn and each time an AP is put down. While an AP is dragged, the drawing only grows, so the map does not move under the pointer.
- **First width:** a map drawn before its panel is on the page takes the main column's width less the panel's edges, and is drawn again if its panel turns out wider or narrower.
- **Spread:** the layout fills that space, at most three times its own size. It may be stretched across more than down, or down more than across, by up to 1.6 times.
- **Resizing:** the map is drawn again as its panel changes width, though not while an AP is being dragged.
- **Kept positions:** x is kept as a fraction of the width and y as a fraction of the window-share height, under `aeolus.nmap2.<node>.<band>`. A map arranged in one window fills another.
- **Older places:** those kept by v0.316.0 in a 640-wide drawing are carried over once.

## Consequences

- **Someone can lay out a clear view of the lines that matter to them, and it is there when they come back.** Another person, or another browser, sees the map as it lays itself out.
- **The map shows the mesh's traffic as well as its signal.** A pair with no dots has no hellos; a line with dots one way only has hellos one way.
- **Nothing changes on the APs or in the manager's API:** the UI alone. An AP's report already carries `hello_ago`.
- **Checked by:**
  - the UI's lint;
  - a local manager with seven APs that all hear each other, in the browser: dragged, moved by keys, kept across a reload, grown downward, laid out again; hellos on, off, and on again.
