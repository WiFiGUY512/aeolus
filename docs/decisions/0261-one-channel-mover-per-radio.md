# 0261. One channel mover per radio

- Status: Accepted (Griff, 2026-11-01: "yes, stop dfs-look where rrm is on")
- Date: 2026-11-01
- Refines: 0073 (radio resource management), 0120 (looking before a DFS channel)

## Context

- **Two daemons can move a port AP's 5 GHz channel:**
  - Aeolus's RRM moves a radio on an automatic channel to its best channel, at once after a start or when interference fills it, else in its window. Where `radio.5g.dfs` is `look-first` (0120), it may also move to a DFS channel after a look.
  - The port's dfs-look (package dfs-look, a procd service started at boot) moves a 5 GHz radio to a DFS block after a look of its own, where that block is clearly better. On 2026-10-31 it moved 57ff to channel 60.
- **Neither knows of the other's moves,** so the two could undo each other, each with a channel switch the clients have to follow.
- **The port also starts an automatic 5 GHz radio outside DFS** (wifi-scripts' `dfs_look_first`, port 0032), so it serves at once with no 60 s radar check. That is the radio's start, not a move, and stays.

## Decision

- **Where RRM moves channels** (`rrm.enabled`, with `rrm.moves` not off), Aeolus's RRM daemon stops the port's dfs-look and keeps it from starting at boot (`/etc/init.d/dfs-look stop` and `disable`). It marks that it did, in `/etc/aeolus/dfs-look-off`, which a sysupgrade keeps.
- **Where RRM doesn't move channels** (off, or with moves off), one Aeolus stopped is started again and its start at boot put back. One it didn't stop is left as it is.
- **It looks a minute apart,** and at once when RRM or its moves turn on or off. A sysupgrade puts dfs-look's start at boot back, and RRM stops it again within a minute of the AP's start.
- **An AP without dfs-look,** as stock OpenWrt, has nothing done.

## Consequences

- **Each port AP has one channel mover:** RRM, with RRM on.
- **A 5 GHz radio there moves to a DFS channel only where `radio.5g.dfs` is `look-first`** (0120): RRM's own look first, then its move. With it unset, as at Home on 2026-11-01, RRM keeps the radios outside DFS, and the start outside DFS stays. A radio dfs-look put on a DFS channel stays there until RRM moves it, in its window.
- **Tried on the C260 (-83)** with the function as written, in its ucode: with RRM moving, dfs-look stopped and its start at boot went, and a look 10 s on did nothing again. With RRM off, it ran again at boot and now. On again, it stopped.
- **Across a sysupgrade, 2026-11-01:** porting flashed -84 on the four port APs, 02:01–02:10 CST. After each boot, dfs-look was stopped, with no start at boot, and the marker was kept.
