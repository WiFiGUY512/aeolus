# 0344. A test window: maintenance in which the AP holds still

- Status: Accepted
- Date: 2026-11-08
- Proposed by: Claude, from the windows it opened by hand for the porting session's tests. Griff took it as what comes next ("Ok. Whats next", 2026-11-08). Accepted with the merge
- Refines: 0218 (maintenance)

## Context

**Porting asks for a window on an AP several times a day** to flash a build, measure throughput or look at a firmware fault. Each needs the AP still, so a test is not disturbed and does not mislead.

**Until now each was set up by hand, in five steps, and undone in five more:**
- **Maintenance (0218):** its alerts kept and not sent.
- **Client tests paused** (`assurance.tests.every 0` at the folder), for the whole site.
- **aeolus-rrm and aeolus-steer stopped** with SIGSTOP, from a script on the AP. A timer let them go after 90 minutes. A watcher on this PC stopped them again after a reboot.
- **Agent updates held** by not releasing.
- **Sometimes the channels pinned** (`radio.*.channel`), which restarts the radios twice.

**That costs, and it is risky:**
- **A stopped RRM sends no hellos,** so its neighbours take it as down and plan without it.
- **A step missed leaves an AP stopped.** The timer is the only guard.
- **Pausing tests stops them for every AP on the site,** not the one under test.

## Decision

**`maintenance.hold`: maintenance with hold and until is a test window.** One change starts it, one ends it, and it ends by itself.

**On the AP, by its own clock:**
- **The manager sends `hold`** with its end in Unix seconds, and why. It works this out from the AP's maintenance and adds it after the schema's check, as it adds homes' `here`.
- **The agent writes `aeolus.aeolus_hold`**, with until and why. Its UCI package is kept over a sysupgrade, so an AP flashed in its window holds still from its start.
- **aeolus-rrm moves no channel and steps no power** until then:
  - nothing is planned;
  - a claim is withdrawn, its radar check let go;
  - power control leaves each power where it is.
  - Its hellos go on, so its neighbours keep it as one.
  - It logs when the window begins and when it ends.
- **aeolus-steer asks nothing of clients:** no transition request, no ban, no kick. It goes on listening and writing what it knows.
- **The AP leaves the hold once its clock is past the end,** whether it reaches the manager or not. A change to the agent's own package restarts nothing: RRM is told to read its settings again, and steer reads the hold at each look.

**On the manager, while the window is open:**
- **No agent is named in the AP's poll,** so it keeps what it runs.
- **No client test is asked of it on a schedule,** nor of another AP for one of its networks. A round asked by hand (0129) still runs.
- **Its alerts say "in a test window, held still"**, with until and why, and are counted as maintenance (0218).

**The window must end.** Hold without until holds nothing. A window over is still sent to the AP until the maintenance is taken away; the AP is past it.

**In the UI:**
- An AP's heading has **Test window…** for anyone who may change its settings: for 30 minutes to 8 hours, and what for.
- **Its banner** says until when, what is held, and has **1 h more** and **End now**.
- A folder takes a window by its maintenance fields, as any setting, and holds every AP below it.

**Not held:**
- the agent's own repairs: a tunnel mended, netifd's lost wireless object, a radio not up after a radar check;
- airscan;
- captures;
- a channel pin, which stays a setting of its own.

## Consequences

- **A test window is one logged change, with its reason, and needs no script on the AP or this PC.** Its neighbours still hear the AP.
- **Client tests elsewhere go on.** Only those of the AP under test, or of its networks, wait.
- **An AP that reboots or is flashed in its window holds still from its start.** Before its first poll, too: the section was kept.
- **Checked by:**
  - the manager's tests: the hold is sent with its end, no agent is named, the alerts say so, plain maintenance and hold without until hold nothing, and a held tester is asked nothing;
  - the render check: `aeolus_hold`, as sent;
  - the agent's render case `hold`;
  - RRM and steer compiled with CI's ucode.
- **Old agents do not render the section:** their render check fails while a window is sent to them. Release manager and agents together. An old agent in a window is named no new agent until the window ends, and keeps running what it has meanwhile.
- **Not proven yet: the daemons holding on a real AP.** It is tried on TestC260 after release.
