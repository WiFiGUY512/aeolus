# 0151. An AP finds its manager by name

- Status: Proposed
- Date: 2026-10-13
- Proposed by: Griff, after a second C-360 was joined with the installer typed at it: "we have this situation where we are groping a wget, to get all the bits they need to use an aeolus manager, can we have them look for aeolus.local, or .fqdn and go directly to the manager and say HI". Written up by Claude
- Refines: 0033, 0040, 0069, 0083

## Context

- **An AP joins a manager when a person names the manager at it:** the installer fetched with `wget` (0083), or LuCI's page. 0033 left open how an AP would find the manager by itself, "a DNS name first", and 0083 said that with it "the web installer and this page become the fallback, not the way in". It was not built.
- **A network's DHCP says which domain it is in.** On ShedAP, 2026-10-13, netifd had from its lease the server `192.168.1.254` and the search list `example.com`, kept in `/tmp/resolv.conf.d/resolv.conf.auto`.
- **The manager already has a name in that domain.** Asked of that server, `aeolus.example.com` was `192.168.20.60`.
- **The AP's own resolver does not give it.** Asked of the AP's dnsmasq, the same name had no answer: rebind protection drops an answer that is a private address (0040). BusyBox's `nslookup` asks a server it is given, and every OpenWrt has it.
- **A network with no DNS of its own has multicast DNS,** names under `.local`, which reaches the one network the asker is on and no further.
  - **BusyBox's `nslookup` was not made to ask it.** It takes a port, and asked `224.0.0.251` on 5353 for the services there it had no answer; why was not looked into.
  - **ucode's socket module can,** in a hundred lines. It is not in a plain image; the prober has it installed (0059).
- **To look, something must be on the AP before a manager is.** The agent comes from the manager (0079, 0083), so it is not that.
- **A name can be answered by anyone.** A search list has led browsers to strangers before (`wpad`), by trying a domain's parents and by taking an answer from out on the internet.
- **A fresh OpenWrt has no password,** and anyone on its network may log in to it. An AP that takes the first manager to answer on that network gives up nothing it had.

## Decision

- **A finder, `aeolus-find`, is what an image carries:** three small files and none of the agent. It is a package of its own (`aeolus-find`), and the files may be put in an image as they are.
- **It looks only while the AP has no manager** (`aeolus.agent.url`), from when the AP has a default route: each minute for ten minutes, then every ten. Once the AP has a manager it stops, and does not start at the next boot.
- **A system run from memory does not look,** as an installer or an image on trial is: what it joined as would be gone at its next boot. It is known by its root being in memory.
- **It looks for the name `aeolus`:**
  1. **in each domain of the search list DHCP gave,** up to six, asked of the DNS servers DHCP gave and not of the AP's own resolver;
  2. **by itself,** of the same servers: a home router answers for its DHCP clients' names;
  3. **as `aeolus.local`, by multicast DNS,** on the network the default route leaves by, where ucode's socket module is installed. The package depends on it.
- **Only a private address is taken for a manager:** 10/8, 172.16/12, 192.168/16, 169.254/16 and 100.64/10. A name that is an address out on the internet is logged and left.
- **A domain is tried as DHCP gave it,** never its parents.
- **What answers is asked for the manager's installer** (`/install`, 0083), and that is run as a person would run it, with no options: it installs that manager's agent, and joins the AP by the uplink the default route leaves by. The AP then waits in Landing Zone for a person to adopt it (0033).
  - **A manager found by DNS is joined by its name,** so that it may be moved. The finder puts the name on dnsmasq's `rebind_domain` first, as `aeolus-enroll` does, and takes it off again if the join fails.
  - **Where the name cannot be had on the AP, or the manager's certificate does not name it, it is joined by its address;** one found by multicast DNS always is.
  - **Where a manager is found and the join fails,** it is tried again in an hour.
- **The manager is trusted as found.** Whoever answers to the name on that network is taken for it once, and its certificate pinned from then on, as with the installer fetched by hand (0083).
- **The manager answers to `aeolus.local` itself.** It is a multicast DNS responder for that one name, on every network it is on, answering with its own address there.
  - `AEOLUS_MDNS` in `serve.env`, or `-mdns`, gives another name, and `off` turns it off. Where it cannot start, that is logged and the manager serves without it, trying again every 10 s until it starts (since v0.204.2: on 2026-10-31 a manager started before its host's network was up could not join the group, and never answered until it was restarted).
  - A certificate the manager makes from now on names it so too.
- **To have none of it on an AP:** `/etc/init.d/aeolus-find disable`, or remove the package.
- **Its log:** `logread -e aeolus-find`.

## Consequences

- **Where a network's DNS has `aeolus` in the domain its DHCP gives, an AP whose image has the finder joins by itself,** some seconds after it has an address: it is plugged in, and is in Landing Zone.
- **A flat network with no DNS of its own needs nothing set up:** the manager answers `aeolus.local`.
- **Across VLANs it is DNS or nothing.** An AP whose first address is on another network than the manager's does not hear `aeolus.local`.
- **An AP on stock OpenWrt still needs one thing put on it:** the finder in its image, or the installer typed as before.
- **A manager someone else put on the network gets the AP.** It gets nothing of the real manager's: that manager's networks and their secrets go only to an AP a person adopted there (0033). On a network where that matters, the finder is turned off and the AP joined with a fingerprint (0083).
- **Checked in the lab, 2026-10-13:**
  - On ShedAP the finder's `look` gave `dns aeolus.example.com 192.168.20.60 192.168.1.254`.
  - The responder, run by itself on ShedAP, answered `aeolus.local` as `192.168.1.38`: to the finder on the same AP, and to a Windows laptop on that network (`Resolve-DnsName`, kept 120 s), which is a multicast resolver.
- **Not yet run end to end:** no AP without a manager was at hand to find one and join it. The join itself is the installer's, which a second C-360 was joined with that day.

## Not done

- **A DHCP option** (0033): never 43, 60, 138 or 224.
- **IPv6:** the finder asks for and takes IPv4 addresses, and the responder answers with one.
- **Another port than 8443,** as a service record would give.
- **The manager does not say how an AP came to it,** by itself or by a person's hand.
- **The responder does not first ask whether another host has the name,** and announces nothing unasked.
- **An AP whose manager has gone does not look for it again.**
- **A first-boot script that puts the finder on a stock image.**
