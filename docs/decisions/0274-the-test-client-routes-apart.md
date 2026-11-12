# 0274. The test client's routes apart

- Status: Accepted (Aeolus's own development, 2026-11-01)
- Date: 2026-11-01
- Refines: 0121 (the client test)

## Context

- **The client test's routes were in the AP's main table:** its subnet's (`192.168.20.0/24 dev aet-sta0`) and its gateway's, at metric 4096. They were meant for the test's own sockets, each bound to aet-sta0, and no route of the AP's of the same reach loses to metric 4096.
- **A narrower route wins whatever its metric.** The AP's way to the manager is its default route, a /0. Where the network under test is on the manager's subnet (Home Wi-Fi and Aeolus-Enterprise, on VLAN 20, with the manager at 192.168.20.60), the test's /24 took the AP's own traffic to the manager out the test client, and the AP's traffic to Home Assistant at 192.168.20.75 too.
  - While the client was on the network, that traffic went over the air and still arrived.
  - As the client roamed and left, it broke: `poll: cannot connect` and `keys: connection error 2`, about 20 s after each such test, on 57ff, ce7f and C360-AP (2026-11-01, every 15 minutes, with the assurance rounds). A Home_Wi-Fi_IoT test (VLAN 10) did nothing.
  - Office and Shed have no scan radio, run only wire tests, and never showed it.

## Decision

- **The test client's routes go in a routing table of its own, 6121,** and a rule (priority 6121) sends there only what is sent from the client's address.
  - 6121 is above every table a tunnel start uses (1000 + VLAN, 0063).
  - They are made through ucode's rtnl, which takes table numbers busybox `ip` won't.
- **Every socket of the test is bound to the client's address** as well as its interface: DNS, the speed streams, the way-out pings and the TCP probe.
- **ping, bound to the interface alone, finds no route of the AP's there,** and the kernel takes the gateway it pings to be on the link, which it is.
- **The rule is taken away as the radio is given back,** and before the routes are made, so one a killed test left goes too. The routes go with the interface.
- **ucode 25.12's rtnl answers nothing where a later one answers true;** both are taken as done (`rt_done`).

## Consequences

- **The AP's own traffic never leaves through the test client,** whatever network is tested and whatever subnet it is on.
- **ucode's rtnl can't make a rule by outgoing interface:** it sends `oif` as an ifindex where FRA_OIFNAME wants a name, and the rule comes out detached. Hence the rule is by source address.
- **Checked live** with the new aeolus-clienttest bind-mounted on the C260 and C360-AP:
  - Home Wi-Fi from each: the C260 to the strongest AP it heard, C360-AP to Office on 2.4 GHz and to the C260 on 5 GHz (354 down / 401 up);
  - each second, no route via aet-sta0 in the main table, the rule there while the client had its address, and gone after;
  - gateway, DNS, speed and the way out measured, and no poll or key failures.
- **Compiles on Shed's 25.12 ucode.**
