# 0219. The manager's own health

- Status: Accepted
- Date: 2026-10-24
- Proposed by: Claude, from 0119's "Not yet" ("memory and processor on the manager itself"), on Griff's standing word to go on developing Aeolus. Written up by Claude
- Refines: 0119, 0183, 0211

## Context

On 2026-10-23 every page of the manager slowed noticeably. The cause was outside the manager: a database query left running on its host had taken a processor core for 43 minutes, and it kept the database's log from being folded back. Nothing in the manager said so:
- the APs' health (0119) is the APs';
- the slow answers that 0211's `slowLog` noticed went to the journal alone.

## Decision

- **`GET /v1/system/health`**, an admin's at the Org, as the updates are (0183):
  - the manager's release and how long it has run;
  - **the host:**
    - its processors, and how busy they were over the last 30 seconds;
    - its load;
    - its memory, all and available;
    - the disk the databases are on, its size and what is free;
  - **the manager's process:** its resident memory, its heap, its goroutines, and its share of the processors;
  - **the databases:** the change log and the conditions store, each file's size and its write-ahead log's;
  - **the slow answers:** how many in the last hour, and the latest ten.
    - `slowLog` keeps the last 50 answers it finds slow: when, the method and path, the status, how long.
- **The host and the process are read from Linux's `/proc` and `statfs`.** Elsewhere, as on a developer's machine, they are left out.
  - Processor shares are sampled every 30 seconds once anyone asks.
- **"This manager"** is a panel on the Org's System tab, under Updates, for the same admins.
  - Amber where the processors are 80% busy or more, memory or the disk is under a tenth free, or an answer was slow in the last hour.
  - The latest slow answers fold under it.
- **Alongside, the System tab sets maintenance (0218),** in a Maintenance section beside Alerts, with labels and hints. Text fields now show their schema's placeholder.

## Consequences

- **A manager slowed by its host says so on its own page:** a busy processor, a log grown large, slow answers.
- **Nothing is kept over time yet;** it is the manager now, and its last 50 slow answers since it started.
- **A manager in a container sees the container's view of `/proc`.** On Proxmox's LXC that is the container's memory, and the host's processors.
