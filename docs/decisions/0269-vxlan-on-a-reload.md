# 0269. vxlan on a reload

- Status: Accepted (Aeolus's own development, 2026-11-01; the OpenWrt fix is the porting session's, flashed at Griff's word)
- Date: 2026-11-01
- Refines: 0057 (an apply keeps the Wi-Fi up, and what an AP lacks is held)

## Context

- **netifd read its protocol scripts only when it started** (OpenWrt fault 6, B44). So a vxlan installed later was unusable until the network restarted, and 0057 restarts it after every install of vxlan: aeolus-setup at the end, aeolus-packages' restore after a sysupgrade.
- **A network restart starts every radio again,** and one on a DFS channel listens a minute for radar a second time.
- **Port 0063 fixes it** (build -93, 68fb6e8): on each `ubus call network reload`, netifd reads the protocol scripts it hasn't seen, and sets up again an interface left on the "none" handler. Stock OpenWrt's netifd doesn't, until upstream takes the fix.

## Decision

- **After installing vxlan, aeolus-packages reloads the network and asks netifd for its protocol handlers** (`get_proto_handlers`). It restarts the network only if vxlan is not among them.
- **So the restart goes wherever netifd has the fix,** with no list of images: the port APs on -93 now, and stock OpenWrt once upstream has it.
- **The restart after kmod-vxlan stays.** When a new kernel's module comes back once the network is up, the tunnels netifd tried before had no module. That isn't fault 6.

## Consequences

- **A port AP that installs vxlan keeps its radios up.** The agent, started again on the package, renders its tunnels, and its own reload sets them up.
- **A stock AP is reloaded once before its restart,** which changes nothing it would not have restarted anyway.
- **The UI's "vxlan is installed, but netifd has not loaded it" chip still says to restart the network.** On a netifd with the fix, a reload does it too, but the manager doesn't know which netifd an AP has.
