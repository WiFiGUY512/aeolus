# 0197. SSH keys on every AP, the manager's among them

- Status: Accepted (Griff, 2026-10-21: "you need to be able to ssh into any of the APs, in fact aeolus should have a terminal screen pop out for every ap connected")
- Date: 2026-10-21
- Proposed by: Griff; written up by Claude
- Refines: 0007, 0073, 0079

## Context

- **`system.ssh_keys` did nothing.** It has been in the schema and on the System tab since the start. The agent never acted on it ("checked with the agent in M5", which never came), so every key on an AP was put there by hand, one AP at a time.
- **APs got missed.** On 2026-10-21, during the IoT device work, Claude could not reach OpenWrtce7f (its key had never been put on it after its conversion). Living-Room-C-360 was reached only by its serial cable.
- **Griff wants a terminal for every AP in Aeolus.** That needs a way in to each AP that does not depend on who set it up by hand.

## Decision

- **The agent puts Aeolus's SSH keys on the AP,** in dropbear's `authorized_keys`, after any put there by hand:
  - those of `system.ssh_keys`, set on a folder or an AP like any setting;
  - and the manager's own key, which comes beside the config as the RRM key does (0073), so it versions nothing.
- **Only Aeolus's own keys are ever taken away.** The agent keeps the list of keys it wrote (`/etc/aeolus/ssh_keys`). A key unset in Aeolus goes from the AP, and a key put there by hand stays, so Aeolus never locks anyone out. A key set in Aeolus that is already there by hand is not written twice, and stays when it is unset in Aeolus. dropbear reads the file at each login, so nothing restarts.
- **The manager's key is made from its secret key** (`Derive("ssh manager")`, an Ed25519 key):
  - It is the same after every restart and is kept nowhere the secret key was not already.
  - The manager writes it as OpenSSH files beside its databases, `ssh/manager_ed25519` and `.pub`, readable by its user alone, so whoever runs the manager can reach any AP with `ssh -i`.
  - `GET /v1/ssh` says its public half.
- **The terminal (to come) uses the manager's key.** It goes by a connection the AP opens, as 0007 has every connection go, so it reaches an AP behind NAT too.

## Consequences

- **Every AP Aeolus manages lets in the same keys,** set once on the Org.
- **Enrollment is enough for a new AP:** the keys come with its first config.
- **Whoever can read the manager's state directory can log in to every AP as root.** Before this, that person could already rewrite every AP's config.

## As built (2026-10-21)

- **Agent:**
  - The module `aeolus/sshkeys.uc` (`merge`, `id`).
  - `keep_ssh_keys()` in `aeolus-agent`, at each ready config.
  - Tested by `agent/test/sshkeys.uc`, run as `TestAgentSSHKeys`.
- **Manager:**
  - `ManagerSSHKey`, `ManagerSSHFiles` and `GET /v1/ssh` in `internal/api/sshkey.go`.
  - `ssh.manager` beside each ready config.
  - The files are written at start by `writeSSHKey` in `cmd/aeolus/serve.go`.
  - Tested by `TestManagerSSHKey`.
- **Dependency:** `golang.org/x/crypto/ssh`, for the key's OpenSSH forms, and for the terminal next.
