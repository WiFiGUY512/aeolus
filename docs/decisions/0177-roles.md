# 0177. Roles: super admin, admin, operator, manager, installer and viewer

- Status: Accepted (Griff, 2026-10-20: "merge 47 and 48, then start the admin page")
- Date: 2026-10-20
- Proposed by: Griff: "we need super admins, admins, operators, managers and installers, they will each have their own access". His answers, 2026-10-20:
  - keep a read-only Viewer;
  - a Manager has residents' keys and clients, gives roles to their staff, and sees the sign-in record;
  - an Installer has the AP lifecycle and the AP's own ports, and can switch on a temporary test SSID, "attached to credentials and a known vlan", "on a timer that automatically downs itself";
  - a Super admin is Org-wide, and also sets "things like global ntp servers, 2 factor auth systems, and other admin entry systems like kerberos".

  And later the same day:
  - "starting a capture (actual packet view) should not be something anyone except an admin can do";
  - "an operator should be able to client trace at the most indepth".

  Written up by Claude
- Refines: 0025, 0030, 0176

## Context

- **The roles were a ladder:** viewer, operator, admin, granted on folders and inherited down each tree (0025), with an Org owner above them (0176).
- **A ladder can't say what a property manager or an installer does.** Each does a different part of the work and none of the rest: a manager runs residents' keys and devices, and an installer puts APs up. Neither is above or below an operator. Each needs a set of things it may do.

## Decision

- **Six roles.**
  - **Super admin** is the Org's: a flag on the account, 0176's owner, never granted on a folder. In the API it is still `owner`, set with `set-owner`.
  - **Five folder roles,** granted on any folder of either tree and holding there and below: **admin**, **operator**, **manager**, **installer** and **viewer**.
- **A role is a set of things it may do,** not a rung.
  - An account may hold different roles on different folders: a manager in Building A, an installer in Building B.
  - On any folder or AP, it may do what any role it holds there or above allows.
  - What each may do:

| May | Super admin | Admin | Operator | Manager | Installer | Viewer |
|---|---|---|---|---|---|---|
| **view**: see everything there | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| **people**: add, disable, enable and remove accounts; make super admins; reset others' passwords; issue and revoke others' tokens | ✓ | | | | | |
| **sign-in systems** (later): second factor, Kerberos, single sign-on | ✓ | | | | | |
| **grant**: give and take roles there | ✓ | any folder role | | manager, installer, viewer | | |
| **administer**: lock, unlock, break hierarchy; set the credentials the manager signs in with (DNS log, switches); the library's tunnel concentrators and VNIs | ✓ | ✓ | | | | |
| **configure**: settings, networks and their tunnels, radios, RRM, folders, services, templates | ✓ | ✓ | ✓ | | | |
| **keys**: residents' per-user keys | ✓ | ✓ | ✓ | ✓ | | |
| **clients**: name, trace, reconnect, block a client. A trace is as deep as an operator goes | ✓ | ✓ | ✓ | ✓ | | |
| **aps**: adopt from Landing Zone, add, name, move, place on the map, remove | ✓ | ✓ | ✓ | | ✓ | |
| **ports**: an AP's own ports and uplink, set on the AP | ✓ | ✓ | ✓ | | ✓ | |
| **actions**: reboot and other AP actions, client tests, assurance rounds | ✓ | ✓ | ✓ | | ✓ | |
| **capture**: start a packet capture, and read the packets one caught, the captures kept of failed attempts included | ✓ | ✓ | | | | |
| **shell**: open a terminal on an AP, a shell there as root (0198) | ✓ | ✓ | | | | |
| **test SSID**: switch on the temporary test network | ✓ | ✓ | ✓ | | ✓ | |
| **sign-ins**: the sign-in record of those holding roles there | ✓ | ✓ | | ✓ | | |

- **Granting:**
  - An admin gives any folder role where it is admin.
  - A manager gives manager, installer or viewer where it is a manager.
  - Taking a role away needs the same as giving it.
  - Only a super admin makes a super admin.
- **Which people an Admin page shows:** a super admin sees everyone. An admin or a manager sees the accounts that hold roles in its folders.
- **Super admin and Org-wide settings:** a super admin may do everything, global settings at the Org included (NTP, country, time zone). Those settings stay open to an admin or operator granted at the Org too, as now, so nothing that works today stops. Only people and the sign-in systems are a super admin's alone.
- **The temporary test SSID is a decision of its own,** made when it is built. As Griff put it, an installer switches on a test network tied to credentials and a known VLAN, and it switches itself off when its time is up. The proposal:
  - an admin sets the test network up on a folder (its SSID, passphrase and VLAN);
  - an installer switches it on there for a while, at most a few hours;
  - the APs render it until then, and drop it by themselves.
- **Existing grants keep their meaning:** viewer, operator and admin do what they did, but for these:
  - packets are an admin's alone: starting a capture, and reading what one caught, which a viewer or an operator could before, they no longer can;
  - an admin or operator at the Org root no longer adds accounts or handles others' tokens (0176).

## Consequences

- **A leasing office, a building manager or an installer gets an account that does their job and no more,** in the buildings they work in.
- **The checks are by what is being done,** named once in `access`, so a new role is a new row of that table and not a hunt through the handlers.
- **A page shows only what its reader may do:** the manager says what each role may do, and what the reader may do on each node, so the Captures tab and every capture link are an admin's, and an installer sees Rename, Move and the AP's actions but no settings to change.

## Open

- The test SSID's own decision: 0179.
- The sign-in systems: a second factor, Kerberos, single sign-on.
- Custom roles (0176's open item), if these six prove too coarse.
