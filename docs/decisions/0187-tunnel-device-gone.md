# 0187. A tunnel's device that netifd lost

- Status: Accepted (Griff, 2026-10-20: "keep going, next thing", under his standing word to merge what is ready)
- Date: 2026-10-20
- Proposed by: Claude, from the client tests of aeolus_50 that had failed for a day on two APs
- Refines: 0054, 0058, 0059, 0099, 0128

## Context

- **aeolus_50, a VXLAN network (VNI 50), never passed a client test** on the office AP or the shed. A client that joined got no DHCP answer, and the test over the wire could not open the network at all. It passed on the C-360s and the C-260.
- **On both, the kernel had no `aeolus_50` device,** while netifd had the interface up and the device a member of `br-vx50`. Wi-Fi clients' frames went into a bridge with no way out. The shed had been up 5 days, and its log no longer said when the device went.
- **Nothing said so:**
  - the agent reported the tunnel up, as netifd did;
  - the prober's verdict was "off", as its probe had no device to send on;
  - the tunnel-down alert waits for "down".
- **Putting it back:**
  - **Starting the interface again,** with `ifup aeolus_50`, does nothing: netifd counts the device as present, so it never makes it again.
  - **Taking the bridge's interface down too,** and both up, makes it, but restarts both radios. The office's Wi-Fi was down for 9 s that way.
  - **Making the device over netlink,** as netifd would from the interface's settings, and putting it in its bridge, mended the shed with nothing else touched. netifd took it up as its own ("tunnel 'aeolus_50' link is up").

## Decision

- **The agent reports a tunnel netifd has up but whose device the kernel lacks as not up,** with `gone`.
- **The manager raises a critical alert,** `tunnel-gone`: "its clients reach nothing".
- **The prober makes such a device again:**
  - **What:** over netlink, from the interface's settings:
    - VNI, remote and port;
    - the underlay as its `tunlink`'s device, and the local address where one is set;
    - TTL where set, else inherited;
    - MTU, learning on, UDP checksums.
  - **Then:** it puts it in its bridge, and logs it.
  - **When:** only after the device was missing at two looks in a row, 10 s apart, so not while netifd is still making it. At most once in 5 minutes per tunnel.
  - **Only an IPv4 tunnel's device.** One with an IPv6 peer is reported and logged, not made again, until it can be tried on one.
  - **With -n** it only looks.
- **The report says when it last made one** (`remade_ago`).

## Consequences

- **A lost tunnel device comes back by itself** within about 20 seconds of the prober seeing it gone, and its network's clients get through again. No radio restarts.
- **Until then, or where it can't be made, it is a critical alert,** not a tunnel that looks up.

## As built (2026-10-20)

- **The prober:** `keep_tunnels()` in `aeolus-prober`, run at each look at the plan, and `remade` in what it writes.
- **The agent:** `gone` and `remade_ago` in `vxlan_state()`.
- **The manager:** `tunnelState.Gone` and `.RemadeAgo`, checked; and `tunnel-gone` in `internal/alerts`.
- **Mended by hand first:**
  - **The shed,** by netlink, as the prober now does, with no radio touched;
  - **The office,** before that, by taking the interfaces down and up, which restarted its radios for 9 s.

  On both, the tunnel's probe then held a lease again.
- **Tested:**
  - **On the C-260:** with its `aeolus_50` device deleted, netifd still had the interface up, and the bridge lost the device, as on the other two. `keep_tunnels` waited one look, then made the device again in `br-vx50`, with `br-lan` untouched. The tunnel's probe was up again, answered within 10 s, and held its lease.
  - **In Go:** the report keeps `gone` and `remade_ago`, and refuses a tunnel both gone and up, or a negative time. The alert test raises `tunnel-gone` as critical.
