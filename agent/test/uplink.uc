// What of the AP's own Wi-Fi is on a network it holds an address on (0209),
// and whether its management interface moved (0210): TestAgentUplink runs this with the real ucode and compares what it prints
// with uplink.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/uplink.uc

'use strict';

import * as uplink from 'aeolus.uplink';

// C360-AP's Wi-Fi, by interface, on 2026-10-23.
let wifi = {
	'phy0-ap0': 'aeolus_50', 'phy0-ap1': 'Aeolus-Enterprise', 'phy0-ap2': 'Home_Wi-Fi_IoT', 'phy0-ap3': 'Home Wi-Fi',
	'phy1-ap0': 'aeolus_50', 'phy1-ap1': 'Aeolus-Enterprise', 'phy1-ap2': 'Home_Wi-Fi_IoT', 'phy1-ap3': 'Home Wi-Fi',
	'phy3-ap0': 'Home Wi-Fi',
};

// Its VLAN 20, which it was managed on, as netifd lists the VLAN's ports:
// two SSIDs, each once.
printf('vlan 20 %J\n', uplink.on_air(['lan1:t', 'phy0-ap3:*', 'phy1-ap3:*', 'phy3-ap0:*', 'phy0-ap1:*', 'phy1-ap1:*'], wifi));
// VLAN 1, now: the uplink alone.
printf('vlan 1 %J\n', uplink.on_air(['lan1:*'], wifi));
// A bridge that filters no VLANs, by its members, as OpenWrt's own lan is.
printf('bridge %J\n', uplink.on_air(['lan1', 'lan2', 'phy0-ap2', 'phy1-ap2'], wifi));
// A hidden network is said as its empty name; entries not well made are
// passed over.
printf('hidden %J\n', uplink.on_air(['phy2-ap0:u*', null, 5], { 'phy2-ap0': '' }));
printf('nothing %J %J\n', uplink.on_air(null, wifi), uplink.on_air(['phy0-ap1:*'], null));

// Whether the management interface moved under the config (0210): C360-AP,
// moved by hand from vlan20 to lan, once two looks agree; not on the first
// look, not before a first render, not while there is none, and not where it
// is still the one rendered with.
printf('moved %J %J %J %J %J %J\n', uplink.moved('vlan20', 'lan', 'lan'), uplink.moved('vlan20', 'vlan20', 'lan'),
	uplink.moved(null, 'lan', 'lan'), uplink.moved('vlan20', null, null), uplink.moved('lan', 'lan', 'lan'), uplink.moved('vlan20', '', ''));
