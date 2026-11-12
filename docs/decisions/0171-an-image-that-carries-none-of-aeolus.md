# 0171. An image that carries none of Aeolus, and daemons that run for weeks

- Status: Proposed
- Date: 2026-10-16
- Proposed by: Griff, of the Arista ports: "there should not be any aeolus code in the pure openwrt image for the APs" and "aeolus is an entirely separate thing we will also publish, but it will be initiated with a wget to pull all the relevant pieces into anyone's base openwrt". With four findings from that day's review of the ports, all on the Aeolus side. Written up by Claude
- Refines: 0040, 0069, 0080, 0083, 0145, 0151, 0156

## Context

- **The Arista port images now carry nothing only Aeolus needs.** Their seeds dropped ucode-mod-uclient, kmod-veth, kmod-nft-bridge and, on the C-260, vxlan; the finder (`aeolus-find`, 0151) went out of the port repo. The wget (0083) has to bring all of it.
- **A port's kernel modules fit only its own build's feed.** Each port image is built with a kernel of its own (spectral needs `KERNEL_RELAY`), and every module is built into a feed for exactly that kernel (`out/<model>/<version>/feed/`, `CONFIG_ALL_KMODS`). OpenWrt's module feeds are refused by its kernel's version and hash. The image trusts its build's key, which that feed is signed with.
- **The agent cannot reach its manager without ucode-mod-uclient.** It was imported, so a sysupgrade onto an image without it left the agent dying at its import, respawned every five seconds (2026-10-09). The init script starts one `aeolus-packages restore` as the service starts, which may come before the network is up; nothing tried again.
- **The manager's feed cache gave out packages of another build than their index** (2026-10-16): `apk` refused vxlan and libopenssl3 on the C-260 with an ADB integrity error. The ports are on 25.12-SNAPSHOT, whose packages are built anew under the same names; the cache kept a package for good and an index for a day.
- **Every C-360 climbs about 18 MB an hour** (since 2026-10-15): restarting aeolus-live freed 24 MB, aeolus-ble 12 MB. On a C-260 aeolus-ble went from 1.9 to 4.4 MB in 30 minutes. In ucode 2026.01.16~85922056, which the images have:
  - **a uloop timer that has gone off is never freed** unless it is cancelled: `uc_uloop_alloc` makes it persistent, and nothing frees it when it fires. A timer made again each time, as aeolus-ble's ten a second and aeolus-live's one a second were, keeps every one and all its function holds;
  - **`ucode -S` runs with the garbage collector off:** only `-g` starts it. Whatever refers to itself, as a socket's handle does through its function, or a connection through the functions that answer it, is never freed;
  - **and started, with `gc('start')`, it is not safe:** a collection may then run inside a function that `filter()` or `map()` calls, and free the array the builtin is making, which it holds only in C. The builtin gives null, and the next `[0]` or `length()` throws "left-hand side expression is not an array or object". It crash-looped airscan on a C-360 until procd gave up (2026-10-16); on the unit, a filter in a uloop timer loop failed within 10 to 30 rounds with it, and never without;
  - **a `uloop.task` with no input function keeps the read end of its pipe** for good (fixed upstream in af8a3830, 2026-09-01).
- **aeolus-ble read only `hci0`.** On the C-260 the controller came up once as `hci1`: the daemon said "the controller is no longer read" every five seconds, pushing the boot out of the log, and after about 80 minutes stopped trying. Each reader it started kept a descriptor (the task's pipe, above), and at the limit `uloop.task` returned null, which nothing looked for.
- **aeolus-ble said the AP's MAC was its Bluetooth address.** The C-360's controller has none of its own (`hcitool dev` says 00:00:00:00:00:00); the C-260's has, its label's MAC less 2.

## Decision

### What the wget brings

- **Nothing changes in what an AP is given:** the agent's modules (ucode-mod-uclient among them) by `aeolus-packages required`, then usteer, snmpd-ssl, vxlan, kmod-nft-bridge, kmod-veth and the rest by `install`, through the manager (0069, 0083). What the port images dropped is on those lists already.
- **The manager keeps module feeds:** a build's own feed, copied in as it is to `/var/lib/aeolus/kmods/` (`AEOLUS_KMOD_FEEDS`), at any depth up to four. It is found by its `kernel-<version>.apk` and served by that version, at `/feeds/kmods/<version>/`, without a token like the cache.
  - **Only the index and the packages are served,** `packages.adb` and `*.apk`, each a file of the feed's own directory: never a link, so a link put there cannot hand out a file of the manager's.
  - **Of several feeds for one kernel, the one with the newest index is served,** of those whose index has been left as it is for ten seconds: the latest build's, whose key a newly built image trusts, and not one still being copied in beside it (copy a feed in under another name and rename it, so a lone one is not taken halfway either). The feeds are looked through again a minute after the last look, when an AP asks: a feed put beside one served, in place of it, or taken away is found then, without the manager starting anew.
- **An AP asks for the one that fits its kernel.** `aeolus-packages feeds` reads the version of its `kernel` package, and where the manager has a feed for it adds `/etc/apk/repositories.d/aeolus-kmods.list` beside OpenWrt's feeds; one for another kernel, as after a sysupgrade, goes. An image of OpenWrt's own has no feed there, and takes its modules from OpenWrt's as before.
  - **Only the manager's answer that it has no such feed takes a line away** (uclient-fetch's `HTTP error 404`, which it says only without `-q`); a manager that cannot be asked just now leaves it.
  - **A feed found after the AP's packages went in brings in the modules that could not be had without it,** vxlan, kmod-nft-bridge and kmod-veth, which need no configuring: they go on the AP's list, and restore, due again, installs them (and restarts the network, for vxlan). The agent asks for the feed when it starts and, while restore is due, each hour; so an AP finds a feed copied to the manager when its agent next starts, or after a sysupgrade. restore looks for the feed before it reckons what is missing.
- **After a sysupgrade, what the agent needs goes in first, by itself,** then the rest together, or one at a time where they would not go together: a kernel module the manager has no feed for keeps out only itself. What went in is said, and what did not is tried again in an hour.
  - **The full wpad replaces the image's basic one without taking away what cannot be put back** (reviews, 2026-10-17). Under apk it is one change. Under opkg, whose index is in RAM and gone after a sysupgrade, the basic one goes first: so both are fetched before it does (with no index neither can be), and a basic one that will not be removed changes nothing. Should the full one then not install, the basic one goes back from its copy; should no wpad be there after all, the basic one from the feeds, at once. Whether a wpad went is read from what is installed, not from the package manager's word. Where none went, the Wi-Fi is left alone, so an AP that cannot reach its feeds does not drop its clients every hour. One feed that cannot be read does not hold the swap back.

### The agent without uclient

- **It loads uclient with `require`, not `import`.** Without it the agent stays up, says so once, and runs `aeolus-packages restore` itself: a minute on, then twice as long each time, up to an hour apart. Once the module is there it ends, and procd starts it again with it. The key agent waits. aeolus-live, which needs uclient too, is started only where it is installed.

### The feed cache and snapshots

- **A snapshot's index is fetched again after five minutes,** a release's still after a day. When a snapshot's index has changed, the packages kept beside it go, to be fetched again of the build the index names. A site without the internet keeps what it has, index and packages of one build.

### Daemons that run for weeks

- **Every one-shot timer cancels itself as it goes off:** `after(ms, fn)`, a timer whose function begins with `this.cancel()` (in a uloop timer's function, `this` is the timer). A timer re-armed with `set` is one timer, and stays as it is.
- **Every daemon that runs for good runs the garbage collector a minute apart, and never starts it:** `gc('collect')`, where nothing is half made, and nowhere a builtin has called into the daemon's own code.
  - **Where the daemon runs uloop, from a timer of its own:** the live line and the Bluetooth proxy with `after()`, steering with a timer re-armed with `set`, as its others are.
  - **Where it sleeps or polls, at the top of its loop:** the prober, radio resource management and the record of clients coming online. The agent and its key agent too: their requests run uloop within them, wherever a request is made from, so a timer of their own could go off anywhere.
  - The client test and the capture each end within minutes, and are left as they are.
- **aeolus-ble finds its controller:** the lowest-numbered the kernel lists, looked for again each time it is read anew, as it may come back under another number; `-i` still names one.
  - **A reader that ends is tried again five seconds on, then twice as long each time, up to ten minutes,** and that is said as the wait grows and every half hour after, not each time. One that read for a minute counts as having read, and the next is soon again.
  - **The daemon looks after its reader itself:** one that ended unseen, as one killed does, or that could not be started at all, is tried again as above; an hcidump a reader before left running is stopped first, and a pid file naming one that is gone is removed.
  - **Past 768 descriptors it starts anew,** before it can open nothing more: the descriptor each reader keeps, until the images have a ucode with af8a3830.
  - **Its Bluetooth address is the controller's own** where `hcitool dev` gives one, as the C-260's does; else a static one the kernel says; else, as on the C-360, the AP's MAC, as before. Home Assistant is given that address.

### The finder

- **It ships only with Aeolus:** its package, `agent/openwrt/aeolus-find`, for whoever builds an image with it, or its three files put in an image as they are (0151). No Arista port image carries it.

## Consequences

- **An Arista port AP joins with the wget, as any OpenWrt does,** once the manager has its build's module feed; without that feed, VXLAN tunnels and networks with a fallback are refused on it, and everything else works.
- **Each port build's feed has to be copied to the manager** for the APs that run it, by whoever builds and flashes the image. A feed takes about 50 MB.
- **A sysupgrade onto an image without uclient no longer takes an AP off Aeolus for good:** the agent puts the module back once the manager can be reached, within an hour of that at most.
- **The daemons' memory stays flat.** Tried for an hour on the C-360 (unit 1, 2026-10-16), the seven daemons bound over v0.115.0's from RAM: none started anew, nothing was thrown, and a client test passed. Each daemon's memory, at the start and after the hour, in kB: aeolus-ble 1640 and 1692 (v0.115.0's had come to 24152 in 4 h 25 min), aeolus-live 2380 and 2380, the agent 3500 and 3524, the key agent 3272 and 3280, the prober 2848 and 2884, radio resource management 2296 and 2372, steering 1664 and 1692; the record of clients coming online 4004 and 6024, nearly all of it in its first three minutes and level after.
- **What is installed again after a sysupgrade (`aeolus-packages restore`) is not yet tried on an AP:** that wants a sysupgrade onto an image without the packages. It is tried against stand-ins for opkg, apk and uclient-fetch that behave as the real ones where it matters (`agent/packages_test.go`): with no index the basic wpad stays and nothing restarts; with one bad feed it is still replaced; no full wpad, a held lock or a failed remove change nothing; under apk the swap brings the Wi-Fi up on the full wpad, even where a package's script failed; a full wpad that will not install leaves the basic one, from its copy or the feeds, and the Wi-Fi up on it; a module feed found later puts its modules on the list, and in restore installs them in the same run.
- **aeolus-ble's log is quiet when the radio is gone,** and the daemon keeps trying, less often.

## Not done

- **Checking a feed's packages against its index:** the manager hands on what it was given.
- **The opkg swap's own weak points, as before this change:** the basic wpad is removed before the full one goes in, so a swap broken off between the two (a power cut), or an /overlay too full for either, leaves no wpad until one is installed by hand; and a full wpad that will not install is tried again each hour, the Wi-Fi restarting each time. Both are opkg's (24.10 and before) only, as they were; mended in 0172.
- **Publishing the ports' module feeds** where an AP not on Aeolus could reach them: the ports' business.
- **aeolus-live's kernel memory** (24 MB freed by a restart on 2026-10-15) is to be watched over days now that its timers and connections are freed; a restart at 1 h 17 min of uptime freed nothing.
