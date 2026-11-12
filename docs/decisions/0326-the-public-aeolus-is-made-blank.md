# 0326. The public Aeolus is made blank, by a tool

- Status: Proposed. Griff's answers, 2026-11-05: license AGPL-3.0; the account WiFiGUY512; "One fresh commit, decisions kept"; and "remove all records of existing clients, the new aeolus for anyone will start fresh and the folders as well, example and ssids should be generacised or removed altogether". Publishing stays his to say.
- Date: 2026-11-05
- Proposed by: Griff, through the main session: "lets let the distro handle those questions". Written up by Claude, in the distro session (0316)
- Refines: 0162 (releases come from Clio), 0316 (the Aeolus distro), 0323 (suggest a feature)

## Context

- **0316 left two things to Griff:** where releases are published, and a license. 0323's requests from the field wait on the same answer: someone in the field cannot reach Clio.
- **The repository names the lab throughout,** in its code, its tests and its decisions. Counted at `main` (adfa8c15):

| What | Where, and how often |
|---|---|
| The module path, `github.com/WiFiGUY512/aeolus` | Every Go import: 447 |
| The lab's domain and Org, `example` | Hosts, the Org in tests, docs: 427 |
| Its SSIDs, `Home Wi-Fi` and the networks named for them | Tests, docs, a frame in a test as hex: about 470 |
| Its people, and their clients by name | the first user, a person, a phone by its model, the IoT devices: about 60 |
| Its folders, `Shed` and `Home` | Tests, docs, a test file's name: about 90 |
| Its devices' MACs | 117 devices, written 1,108 times: as MACs, as AP IDs, as Go bytes, in names (`ESP_…`, `OpenWrt5b1f`) |
| Its forge, CI and update path | `.forgejo/`, `.github/`, `deploy/update.sh`, `deploy/known_hosts`, the unit's `Documentation=` |
| A per-user key as an AP held it | Two agent test cases |

- **A fresh manager has nothing of the lab's:** Landing Zone and the two Sandbox folders (0032), and an Org named by whoever sets it up. So "start fresh" is about the tree a newcomer gets, not about what the manager seeds.
- **The repository stays private** (0316). What goes public is a copy of it.

## Decision

### The public tree is made, not kept

- **`cmd/publish` makes it from a ref** of this repository, by the rules in `cmd/publish/rules.txt`. This repository stays the code of record. Each publish is a new export, not a merge.
- **The first publish is one commit,** "Aeolus", with the decisions in it, named as the tree is.
- **The tool writes only into its output directory.** Pushing what it made is a person's to do, and Griff's to say.

### What it does, in order

- **Leaves out** the lab's own: `.forgejo/` and `.github/` (its CI and merge helper), `deploy/update.sh` and `deploy/known_hosts` (its update path to Clio, 0162), `CLAUDE.md`, `cmd/uidev/`, and `cmd/publish/` itself, whose tables name what they replace.
- **Copies other projects' code as it is** (`third_party/`, the UI's `vendor/`): nothing in it is remapped or replaced, since a minified file's numbers can read as a MAC's, but it is scanned.
- **Adds** what only the public tree has: `LICENSE`, the GNU AGPL 3.0 as published, and `README.md`, the public one: what Aeolus is, getting started, building, and where ideas and problems go.
- **Remaps every MAC that looks like a device's,** consistently, wherever it is written:
  - Its OUI stays, so a vendor stays its vendor and a randomised MAC stays locally administered. The device's part changes: its first two bytes come from a salted hash, and its last byte stays, so the BSSIDs an AP numbers from its MAC stay apart as they were.
  - It is found with colons or hyphens in either case, as twelve digits (an AP's ID), as Go bytes, as Arista writes one, as six digits in a name (`ESP_739AF7`), and as its last two bytes where the text holds its MAC (`e6:11`, `OpenWrt5b1f`). A BSSID made from an AP's MAC changes with it.
  - What code makes of a MAC changes with it too, since a test's expected output holds what its input makes: a frame written as hex, the IPv6 address made from the MAC (EUI-64, `fe80::…ff:fe…`), its middle four bytes (a radio's key), and its first four as a number, where a MAC made from it is remapped in the same text (a filter's).
  - A MAC that is made up stays: broadcast and multicast, the rules' examples (`02:00:00:00:…`, `aa:bb:cc:…`), and any with one byte four times or a run of three (`11:22:33`).
- **Replaces text,** literals first, the specific before the general, then regular expressions:
  - the module path → `github.com/WiFiGUY512/aeolus`;
  - `example` → `example`, and its domain → `example.com`;
  - the SSIDs → `Home Wi-Fi`, as long as they were, since a frame in a test holds an SSID with its length;
  - the first user → the first user, a person → a person, a phone by its owner or model → a phone, a IoT device → an IoT device;
  - the folders: `Shed` → `Shed`, `Home` → `Home`;
  - the unit's `Documentation=` → the public repository;
  - the per-user key → one plainly made up;
  - an ICMPv6 checksum in an agent test, over an address the remap changed, which no rule of MACs can make → what the test's own run on the export says it is. A new salt changes it.
- **Renames a path as its text,** since a test names the file it reads.
- **Formats** Go files again, since a name of a new length moves their columns.
- **Scans what it made** for each word on its deny list, in contents and in paths, as text and as hex (lower case, capitals, title case). One left fails the run and says where.
- **With `-commit`, makes the directory a repository of one commit,** by `-author` alone: by default WiFiGUY512's address at GitHub, not whoever runs it, whose git identity carries the lab's address. Each file keeps its mode from the tree, which Windows' file system would lose.

### What stays as it is

- **Griff's name,** as the author in the decisions and as a test account (`griff`). It names the person who made Aeolus, not a client.
- **The lab's history in the decisions:** its hosts (powermox, talos, Clio, Zephyrus, hestia), its internet link and router, and its private addresses (`192.168.x`). They name no client, and the decisions read as what happened.
- Either is one rule to change, if Griff wants it gone.

### Publishing

- **Nothing goes to GitHub, or anywhere public, until Griff says so.** When he does:
  1. `go run ./cmd/publish -ref main -out ../aeolus-public -commit "Aeolus" -report`, and read the report.
  2. Make `WiFiGUY512/aeolus` at GitHub, public, and push the commit to `main`.
  3. Attach the release files (0316) to a release there. 0323's requests from the field can then go in as its issues.

## Consequences

- **A new name of the lab's needs a rule before the next publish.** The deny list catches a known name coming back, not a new one. So each publish starts with the report and a sweep for new names.
- **The public tree has no CI of its own.** The lab's runs on talos (Forgejo). A GitHub workflow for the public repository, on GitHub's runners, with ucode built as CI builds it, is later work.
- **A manager built from the public tree updates from it:** its updater fetches tags from the origin it was built from (0183, 0316).
- **Later publishes are a new export, committed over the public tree whole,** so its history is one commit per publish. `-commit` makes the first. Committing over an existing checkout is later work.

## Checked

- **The tool's tests** (`cmd/publish`): each MAC form remapped together, made-up MACs kept, a hash left as it is, rules in order, verbatim paths, the deny scan in contents, paths and hex, refused rules, and the commit's author and modes.
- **The export of `main` (df8b6086) with the tool:** 1,019 files, 120 devices' MACs remapped, nothing denied left, the vendored files byte for byte.
  - Of the 111 device parts in the source, none is left, as a MAC or in an IPv6 address, except where made up (`aa:aa:aa:aa:aa:03`), made from a remapped AP and a number (`0a:82:27:e4:85:10`, VNI 10000), or not a MAC at all (two runs of numbers in Plotly's SVG paths, copied as they are).
  - `gofmt`, `go vet` and `go build` clean; `go test` passes for every package but `./agent`, on Windows.
- **On talos,** CI's workflow ran on the export as a throwaway branch, `publish-preview`, deleted after: the lint, `go test -race`, the agent's renders, the prober's frames, steering, the wire test and RRM, and the manager's builds for AP hardware.
  - The first run (470) failed three agent tests. Their inputs were remapped, but their expected outputs held the same MACs as frames in hex, link-local IPv6 addresses, a radio's middle four bytes and a filter's number. The tool remaps those forms now.
  - The second (473) failed one: an address in a byte list longer than a MAC, which the tool had cut in MAC-sized pieces, and the ICMPv6 checksum above. The tool reads such a list whole now.
  - The third (474) passed: every test, the agent's five among them.
