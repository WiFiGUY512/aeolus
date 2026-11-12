# 0211. The UI loads once, and slow answers are logged

- Status: Accepted
- Date: 2026-10-23
- Proposed by: Griff: "the interface seems to have gotten noticibly slower, is there any way to precache some of this, or in some other way speed it up a bit?" Written up by Claude
- Refines: 0042

## Context

- **The UI is about 70 JavaScript modules and a stylesheet, about 880 KB,** embedded in the manager and served under `/ui/` (0042).
- **Every page load fetched every one of them whole.**
  - They were sent `Cache-Control: no-cache` with no ETag, so a browser could not ask whether one had changed; it fetched it again.
  - Nothing was compressed.
- **They came one level at a time.** A module is fetched only once the module that imports it has come: app.js, then the 19 it imports, then theirs.
- **A slow manager could not say what was slow:** the manager logged no answer's time.
- **What else made it slower on 2026-10-23** was a read-only query of mine, left running on the manager host for 43 minutes at a whole core of its two. It held a read open on conditions.db, so its write-ahead log could not be folded back (222 MB). It was stopped.

## Decision

- **Each UI file is made ready once, as the manager starts:** its ETag, a hash of its body, and the body gzipped where that is smaller.
- **A browser still asks each time (`no-cache`), but by the ETag.** While a file is the same, the answer is 304, a few bytes. A new release has new ETags, so nothing old is kept.
- **Gzipped to a browser that takes it:** about 300 KB the first time, not 880.
- **The page lists every module app.js imports, however deep, as `modulepreload` links,** so the browser asks for them all at once. The list is read from the modules' own imports, so a new module needs nothing written for it.
- **The API logs every answer that takes a second or more,** "slow answer", with its method, path, status and milliseconds. The live lines and a terminal's, which are meant to last, are left out.

## Consequences

- **After the first visit a page loads without fetching the UI again,** and the first visit fetches about a third as much, all at once.
- **The API's own answers are not compressed or cached yet.** The slow-answer log says which are worth it.
