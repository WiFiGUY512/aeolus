# 0343. The next build's own copies of every package; a build swapped from its file

- Status: Accepted
- Date: 2026-11-08
- Proposed by: Claude, on port -135's note from the porting session: it moves libubox, libblobmsg-json and libjson-script from 20260721 to 20261103. Taken up on Griff's standing word to go on developing Aeolus, and accepted with the merge
- Refines: 0280 (the next build's kernel modules), 0248 (early), 0315 (the build's own feeds), 0331

## Context

**After a sysupgrade, the packages beyond the image go back in two steps:**
- **Early (0248):** before the network, `early` puts back the copies kept under the build before.
- **Restore:** once the network is up, `restore` fetches what is still missing from the build's own feeds (0315).
- **The next build's modules (0280):** the copies' kernel modules fit only the kernel before, so `keep_next` also keeps the next build's kernel modules, fetched before the flash, and early uses those in their place.

**Port -135 builds the same versions anew against a library it moved.** In its feeds, these are built against libubox 20261103:
- `ucode-mod-uclient` `2026.06.07~daad21fa-r2`;
- `ucode-mod-udebug`, `usteer` and `ustp`.

Each has the very version of -132's build, which is linked against 20260721. After a sysupgrade to -135:
- **Early puts back -132's copies.** They do not load on -135, which no longer has libubox 20260721.
- **Restore swaps none of them.** `apk add pkg=version` takes a version that is installed as done. `as_built` passes over a package whose version is the feed's. `pm_renew`'s `apk add --upgrade` does nothing.
- **Without a working `uclient`, the agent cannot reach the manager.** The AP would stay out of reach until someone mended it by hand.

**C360-AP showed it on 2026-11-08,** the other way about: a snapshot `uclient` built against 20261103, on an image with 20260721. It was out of reach from 14:52 to 15:08 CST.

**What does swap it:** `apk add --allow-untrusted <the build's .apk>`. Tried on C360-AP, it put the build's `ucode-mod-udebug` in place of the snapshot's of the same version: the post-upgrade script ran, the `.so` changed, and the module loaded.

## Decision

- **`keep_next` keeps every package beyond the image as the next build has it,** not its kernel modules alone.
  - The kernel modules come from the build's module feed: all of them, or none, as before.
  - The rest come from the module feed or the build's own package feeds, `<arch>/{base,packages,luci}`, the first that has each.
  - The arch is OpenWrt's package arch (`/etc/apk/arch`, `aarch64_cortex-a53`). `apk --print-arch` says only `aarch64`.
  - A package none of them has goes back from its copy.
  - Each is verified against the image's keys, as before.
- **`early` puts back each package the next build's copies have in place of the copy of the same name,** where they are for the kernel it runs, as before.
- **Restore swaps a module that is installed and does not load for the build's own feeds' build of it, from its file** (`swap_build`).
  - It fetches the file, verifies it, and runs `apk add --allow-untrusted <file>`, then unpins it in apk's world as `early` does.
  - Restore does this after `pm_renew` for a module that still does not load. `as_built` does it where the feed's version is the one installed but the module does not load.
  - Where there are no build's own feeds, it uses the AP's.

## Consequences

- **A sysupgrade to a build that moves a library brings back the packages built against it before the network.** Where a copy is missing, restore swaps the one that does not load once the network is up.
- **Tried on the C260 (-132),** with -135's feeds on the manager: the new `keep` kept 19 of its 19 packages beyond the image as -135 has them, 6 of them kernel modules. The kept `ucode-mod-uclient` is -135's file (sha256 90aa462e…), not -132's of the same name and version (0995018a…).
- **The copies for the next build take as much room again on the overlay as the copies do,** about 3 MiB on the C260. The room check stands: without room, none are kept, and restore puts them back after the network.
- **Checked by the agent's tests:**
  - keep keeps a package from the build's package feeds, and a kernel module only from its module feed;
  - early takes the next build's `usteer` in place of the copy's;
  - restore swaps a module that does not load from the feed's file, and it loads.
- **Not proven yet: a real sysupgrade to -135.** Porting holds -135 until this is in, then tries it on TestC260 in a window, with Griff's OK.
