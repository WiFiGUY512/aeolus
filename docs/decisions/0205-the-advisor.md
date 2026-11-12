# 0205. The advisor: Claude looks for client problems, and says what to do

- Status: Accepted (Griff, 2026-10-21: "we should probably create an agent that lives in aeolus (or connects to a frontier model) to look for client problems, bugs, and incompatibilities while making recommendations"; then "start work on the agent")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0003, 0027, 0028, 0069, 0177

## Context

- **Problems go unnoticed until a person goes looking.**
  - The IoT devices rejoined every 45 seconds for days.
  - Hours of radio tests later, the cause turned out to be the WAN: T-Mobile's 464XLAT black-holed packets over 1408 bytes sent without DF.
  - The manager had the evidence all along, in the rejoins (0190), the compatibility watch (0193) and the client tests. Nobody put it together.
- **The manager already says what it sees, to a model.** The MCP adapter (0028) gives every read as a tool with a description written for one. That is how these problems were chased by hand.
- **Griff's bounds:**
  - It may use a frontier model and the internet. Running without the internet (0069) covers running the APs, not analysis and monitoring (Griff, 2026-10-21: "that rule it for AP command and control, not analysis and monitoring").
  - It must never sit in the path that runs the APs.

## Decision

### What it is

- **The advisor is a review the manager runs:**
  - on a schedule, every 24 hours unless set;
  - and when an admin asks.
- **In a review, Claude has the manager's reads as tools.** It looks for:
  - **client problems:** one client or a few that cannot stay on, or get no address or no way out;
  - **incompatibilities:** a maker's clients on a model of AP, or a band;
  - **bugs:** an AP or the manager doing what it should not;
  - **harmful settings.**
- **What it finds, it records as findings.** Each finding has:
  - a kind (`client`, `compatibility`, `ap`, `network` or `config`) and a severity (`info`, `warning` or `critical`);
  - a title;
  - what it saw, with the numbers;
  - what to do about it;
  - the clients, APs and networks it concerns;
  - a key, which names the problem. A finding with the key of an open one updates it, so a problem found again is one finding.
- **It never acts.** It recommends; a person decides and acts.

### How it is kept out of the control path

- **It reads as an account of its own, `aeolus-advisor`, with the viewer role on both trees.**
  - The viewer role is one right, view. The API refuses that account any change, whatever the model asks.
  - An owner sets the account up, and revoking its roles stops it reading anything.
- **It reaches the API in the manager's own process.** A request carries the account in its context, which only code in the process can set. So there is no token to keep or leak, and nothing from outside can be that account.
- **Its tools are the MCP adapter's, marked read-only** (`ReadOnlyHint`), the same descriptions a person's Claude reads:
  - nothing that asks an AP to do anything, no change, and no preview;
  - and none whose answer is too large to be of use in a review (an AP's whole history).
- **Its only other tool, `record_finding`, writes a finding** to the manager's conditions store. That store is beside the change log, never in it, and nothing reads it to change a config.
- **The internet:** with `web` on, the default, it may search the web a few times a review, for what is known of a device or its firmware.

### What is sent where

- **To Anthropic's API, under the Org's own key:**
  - what its tools return: the clients' MACs, host names and addresses, the SSIDs, the APs' names and what they report;
  - never a secret, since the API seals them in every answer (0027).
- **Nothing else leaves.** The key is a secret, sealed in the change log like any (0027), and opened only to make the request.

### The model and its cost

- **Model:** Claude Opus 5.5 (`claude-opus-5-5`) unless set (`claude-sonnet-5-5` costs about half), with adaptive thinking at high effort.
- **Each review is held to a budget,** `system.advisor.budget`, in US dollars, 2 unless set:
  - the cost is worked out from each answer's tokens at the model's prices;
  - and no request is made once the next would pass the budget.
- **It is also held to 40 requests.**
- **What does not change between its requests is cached** (the instructions, the tools and the conversation so far), so each request pays full price only for what is new.
- **Each review is kept:** when it ran, the model, its requests, tokens and cost, how it ended, and its summary.

### The settings

- **Under `system.advisor` at the Org, the manager's own:** never sent to an AP, and changing one re-versions no AP.
  - `enabled`, off unless set;
  - `key`, the Anthropic API key, write-only and sealed;
  - `model`;
  - `every`, in hours, 1 to 168;
  - `web`;
  - `budget`.
- **Who sets them:** an admin, as for the credentials (0168, 0177). Whoever sets them decides where the Org's data goes and what it costs.

### The API

- `GET /v1/advisor`: findings, open first, and the last reviews. Viewer at the Org.
- `POST /v1/advisor/run`: a review now. Admin at the Org.
- `POST /v1/advisor/findings/{id}`: `state` set to `dismissed` or back to `open`. Operator at the Org.

## Consequences

- **What the manager sees is put together every day,** by something that reads it all, and what to do is written down.
- **A review costs money:** a dollar or two at the defaults, on the Org's own Anthropic account.
- **A dismissed finding is told to the next reviews,** so they do not raise it again unless it has grown worse.
- **The advisor can be wrong:** each finding says what it saw, so a person can check it.

## As built (2026-10-21)

- **The review:** `internal/advisor`.
  - `Review` runs one: the Messages API (`anthropic-sdk-go`) on its beta surface, with server-side fallbacks, a task budget and adaptive thinking.
  - Its loop is append-only, so thinking blocks go back as they came.
  - `openReads` lists the MCP adapter's tools over the API as `aeolus-advisor` and keeps the read-only ones but `leftOut`. `record_finding` has a strict schema, and `cost.go` prices each answer.
- **The account's way in:** `api.As` and `asAccount` in `internal/api/as.go`; `authenticate` takes it before any token.
- **The adapter:** `mcpadapter.Server`, the adapter over a handler that carries its own account. `get_network` is marked read-only, as it is, and `get_advice` reads the findings.
- **The store:** `advice_runs` and `advice` in the conditions store, migration 25 (`internal/conditions/advice.go`).
- **The manager:** `internal/api/advisor.go`, with the settings, `Advise` (every 10 minutes it looks whether one is due) and the three routes. `serve.go` starts it.
- **Settings:** `system.advisor.*` in the schema; `Credentials` and `managerOnly` in `internal/change/authorize.go`; the render check's list of fields APs are not sent. The System editor's Advisor section, at the Org.
- **The page:** Advisor in the main navigation, for those who see the Org (`views/advisor.js`).
  - **At the top:** one line, whether it is on, the model, how often, the last review's time, outcome and cost, and Reviewing… while one runs. An admin has Review now.
  - **What it still needs:** the account and the key. An owner sets the account up with one button: three changes, previewed together:
    - add `aeolus-advisor`, "The advisor";
    - viewer at the Org in Locations;
    - viewer at the Org in Services.
  - **Then the key:** the page points to the Org's System settings, under Advisor, for the key and the schedule.
  - **The open findings,** the most severe first. Each shows:
    - what it is, what it saw and what to do;
    - its clients, each opening its journey, and its APs, each a link to the AP;
    - how often it was found, first and last.
    - An operator dismisses one, or opens it again. The dismissed fold away below.
  - **The latest reviews:** when, why, how each ended, its requests, tokens and cost, and its summary, a long one folded, or its error.
- **Tested:**
  - **`internal/advisor`,** with Anthropic's API stood in for:
    - the tools offered (the reads but those left out, `record_finding` strict, the web);
    - the request's model, thinking, effort, fallbacks, caching and betas;
    - the thinking kept, each tool's result, a change refused as no tool, and a bad finding refused;
    - a finding found twice is one, and a dismissed one stays so unless worse;
    - the cost;
    - the budget, before any request is made;
    - a refused key.
  - **`TestAdvisor` in the API,** through the real adapter:
    - the account's way in, before and after it is set up;
    - a viewer refused a change;
    - the key an admin's, and sent to no AP;
    - a whole review asked by an admin, reading as itself and finding one thing;
    - its findings dismissed by an operator and not by a tenant.
  - **The page, in the local harness,** with three reviews kept (one done, one stopped at its budget, one failed) and four findings:
    - the open ones, most severe first;
    - one dismissed and opened again;
    - a client's journey opened from a finding;
    - Review now with no key, refused with the manager's word;
    - the account set up as changes #31 to #33;
    - Reviewing… while one runs;
    - and, for an installer below the Org, no Advisor in the navigation and nothing on the page.

## Not yet

- A finding resolved by itself, when its evidence is gone: by a review, since 0215.
- Asking it about one client or one AP, as a question: built in 0215.
