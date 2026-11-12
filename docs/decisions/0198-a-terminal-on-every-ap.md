# 0198. A terminal on every AP

- Status: Accepted (Griff, 2026-10-21: "aeolus should have a terminal screen pop out for every ap connected")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0007, 0145, 0177, 0197

## Context

- **Griff wants a shell on any AP from Aeolus,** in a window that pops out of the AP's page.
- **The manager may not reach in to an AP (0007).** An AP opens every connection, so that one behind NAT, or anywhere the manager cannot reach, still works. A terminal has to keep to that.
- **The pieces are there:**
  - Every AP lets the manager's own SSH key in (0197).
  - Every AP holds a line open to the manager that the manager can speak on at once (0145), and a capture is begun that way already (0148).
  - dropbear on the AP gives a real terminal, with job control, colours and window size, which nothing else on the AP could.

## Decision

- **The AP joins its own dropbear to the manager, by connections it opens.**
  1. Asked for a terminal, the manager says so on the AP's line: `{"shell": {"id"}}`.
  2. `aeolus-live` starts `aeolus-shell <id>`.
  3. That connects to dropbear on 127.0.0.1:22 and to the manager twice: `GET /v1/ap/terminal/{id}`, whose answer does not end and is what goes to dropbear; and `POST /v1/ap/terminal/{id}`, one at a time and in order, of what dropbear says.
  4. The GET names dropbear's host key in the `Aeolus-Host-Key` header. It is read with `dropbearkey -y`, Ed25519 first.
- **The manager speaks SSH over them,** as root with its own key (0197), and checks the host key the AP named. Only the AP's own TLS connection, under its own token, could have named it. It asks for a pty (`xterm-256color`) and a shell.
- **The browser has the shell by four calls:**
  - `POST /v1/aps/{ap}/terminal` with `{cols, rows}` opens one and answers `{id}`;
  - `GET /v1/terminal/{id}` is an answer that does not end: what the shell prints, as it prints it, with why it ended last, on a line of its own;
  - `POST /v1/terminal/{id}/in` sends what is typed, as the body;
  - `POST /v1/terminal/{id}/size` sends `{cols, rows}`, and `DELETE /v1/terminal/{id}` closes it.

  Only the account that opened a terminal may use it.
- **It is an admin's.** A new right, `shell`, is held by Admin alone (and the owner), as `capture` is: a root shell is more than any action an operator may ask of an AP.
- **It is recorded.** The AP's actions say who opened a terminal there, and when (kind `terminal`, finished as it opens, so no agent takes it up). The manager's log says when one opened and ended, and why.
- **It ends on its own.** It ends when the shell does, when it is closed, when the AP's side goes, after 30 minutes with nothing typed or printed, or at 8 hours. The AP has 20 seconds to open its side, and one with no line is told so at once.
- **The window:**
  - `Terminal ↗` beside Reboot on an AP's page, for those with the right, opens `/ui/terminal.html` in a window of its own, one per AP.
  - It draws with xterm.js 5.5.0 and its fit addon, kept in the UI as their authors ship them (`third_party/xterm`). The manager serves them itself; nothing is fetched from the internet (0069).
  - That page alone allows inline styles in its Content-Security-Policy, which xterm.js writes as it draws. Its scripts are the manager's own, as on every page.

## Consequences

- **Any AP that holds its line has a terminal, wherever it is:** behind NAT, or on a network the manager cannot reach.
- **An AP whose agent is older than this, or that holds no line, has none,** and the window says why.
- **Typing goes up a request at a time, gathered while one is on its way.** On a LAN that is a few ms. Over a slow WAN the echo is as slow as the way there and back, as with any SSH.
- **The manager holds a session's bytes only in memory.** A restart of the manager ends every terminal.

## As built (2026-10-21)

- **Manager:**
  - `internal/api/terminal.go`: the broker, `apConn` (the AP's side as a `net.Conn`), and the seven routes.
  - `liveSay.Shell`.
  - The `shell` right in `internal/access/rights.go`.
- **Agent:** `aeolus-shell`, new; `aeolus-live` starts it.
- **UI:** `terminal.html`, `terminal.js` and `terminal.css`; `terminalButton` in `views/actions.js`; `vendor/xterm/`; `cspTerminal` in `internal/ui/ui.go`.
- **Tested:**
  - **`TestTerminal`,** end to end in-process:
    - An SSH server stands in for dropbear, letting in the manager's key alone.
    - A fake AP holds its line and relays as `aeolus-shell` does.
    - An operator is refused.
    - Typing is echoed, the size is set, another account may not type, closing says so, and the AP's actions name who opened it.
  - **`aeolus-shell`** was compiled on ShedAP's ucode (OpenWrt 25.12's) and run there against the manager, as far as the manager's answer.
