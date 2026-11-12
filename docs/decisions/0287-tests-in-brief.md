# 0287. The Assurance tab's tests, in brief until opened

- Status: Accepted (built on Griff's standing word to develop on my own, 2026-10-24)
- Date: 2026-11-02
- Refines: 0123 (service assurance), 0285 (the pages read less)

## Context

- **0285 left the Assurance tab's own answer at 1.3 MB.** It said that was the steps of the 50 tests the tab lists.
- **Measured on the lab's live answer (2026-11-02), that was wrong.** Of 1,273,030 characters:
  - the 191 things tested were 1,088,180, and the latest test each carries, in full, was most of it: its steps 289 KB, its parameters and result 232 KB, its roams 83 KB, its way online 55 KB;
  - the 50 tests listed were 177 KB;
  - each thing's own runs and way were 60 KB and 91 KB.
- **The tab shows none of that in full until a line is opened.** A thing tested and a test listed are each a line that opens to the test; only the line's chip and words are on show before.

## Decision

- **The tab asks for each test in brief: `GET /v1/assurance?tests=brief`.**
  - A test in brief says how it came out, where it stopped and why, and how long it took (`outcome`, `stage`, `reason`, `took_ms`).
  - It also says what it was of: the network, the SSID, the AP it joined and the band (`network`, `ssid`, `on`, `band`), and whether it was a round's (`round`).
  - It does not carry its steps, what it found, or its way online, which is worked out for no test the answer gives in brief.
- **A line fetches its test in full the first time it opens: `GET /v1/aps/{ap}/actions/{id}`.**
  - It is any one action of an AP, for a caller who may view the AP, as the AP's own list gives it. That list is now one function (`actionShown`): a client test with its way online, and, joined on an AP the caller may not view, not saying which nor its BSSIDs.
  - The page keeps a test that has ended, so a line open across the half-minute redraw fetches it once.
- **The Overview's brief (0285) gives each check's last test the same way.**
- **Unasked, the answer is whole, as before**, for the manager's tools, which read it in one go.

## Consequences

- **The tab's answer on the lab falls from 1.27 MB to about 0.32 MB**, worked out from the live answer. What is left is mostly each thing's runs and way, which its line draws when opened.
- **Opening a line costs a request of about 1 KB to 6 KB.**
