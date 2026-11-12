// What a capture someone starts takes (0148): which of the AP's interfaces,
// each with what tcpdump is told to keep, for a network or a VLAN, one
// side or both, one client or everyone; and the script that takes it.
// internal/rendercheck runs this with the real ucode and compares what it
// prints with pcap.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/pcap.uc

'use strict';

import * as pcap from 'aeolus.pcap';

// An AP as the lab's are: networks on VLANs of the bridge its uplink is
// in, the uplink carrying each tagged but the first; one network in a
// tunnel, on a bridge of its own; and a wired port for a client.
let have = {
	uplink: 'eth0',
	wifi: {
		'phy0-ap0': { nets: ['aeolus_tunnelled'], band: '2g', ssid: 'Tunnelled' },
		'phy1-ap0': { nets: ['aeolus_tunnelled'], band: '5g', ssid: 'Tunnelled' },
		'phy0-ap1': { nets: ['aeolus_lab'], band: '2g', ssid: 'Aeolus Lab' },
		'phy1-ap1': { nets: ['aeolus_lab'], band: '5g', ssid: 'Aeolus Lab' },
		'phy0-ap4': { nets: ['aeolus_home_wi_fi_iot'], band: '2g', ssid: 'Home_Wi-Fi_IoT' },
		'phy1-ap4': { nets: ['aeolus_home_wi_fi_iot'], band: '5g', ssid: 'Home_Wi-Fi_IoT' },
		'phy1-ap5': { nets: ['aeolus_first'], band: '5g', ssid: 'First' },
		'phy1-ap6; reboot': { nets: ['aeolus_lab'], band: '5g', ssid: 'A name no interface has' },
	},
	network: {
		aeolus_lab: { '.type': 'interface', device: 'br-lan.20' },
		aeolus_test: { '.type': 'interface', device: 'br-lan.20' },
		aeolus_home_wi_fi_iot: { '.type': 'interface', device: 'br-lan.10' },
		aeolus_first: { '.type': 'interface', device: 'br-lan.1' },
		aeolus_tunnelled: { '.type': 'interface', device: 'br-vx50' },
		cfg1: { '.type': 'bridge-vlan', device: 'br-lan', vlan: '1', ports: 'eth0:u* lan1:u*' },
		cfg2: { '.type': 'bridge-vlan', device: 'br-lan', vlan: '10', ports: ['eth0:t'] },
		cfg3: { '.type': 'bridge-vlan', device: 'br-lan', vlan: '20', ports: ['eth0:t', 'lan1:t'] },
		cfg4: { '.type': 'bridge-vlan', device: 'br-other', vlan: '20', ports: ['eth9:t'] },
		cfg5: { '.type': 'bridge-vlan', device: 'br-lan', vlan: '30', ports: ['lan2', 'lan3:t'] },
	},
	members: { 'br-lan': ['eth0', 'lan1', 'lan2', 'lan3', 'phy0-ap1', 'phy1-ap1', 'phy0-ap4', 'phy1-ap4', 'phy1-ap5'], 'br-vx50': ['aeolus_50', 'phy0-ap0', 'phy1-ap0'] },
};

function show(what, ask, of) {
	let p = pcap.plan(ask, of ?? have);
	printf('%s:\n', what);
	if (p.error) {
		printf('  refused: %s\n', p.error);
		return;
	}
	for (let f in p.files)
		printf('  %s, %s (%s): %s\n', f.name, f.side, f.note, length(f.filter) ? f.filter : 'everything');
}
const STA = '7e:2a:ea:04:b1:8f';

show('a network, both sides', { sides: 'both', network: 'home-wi-fi-iot' });
show('one client on it', { sides: 'both', network: 'home-wi-fi-iot', client: STA });
show('its Wi-Fi side alone, on 5 GHz', { sides: 'wireless', network: 'home-wi-fi-iot', bands: ['5g'] });
show('its wired side alone', { sides: 'wired', network: 'home-wi-fi-iot', client: STA });
show('a network on the VLAN the uplink carries untagged', { sides: 'both', network: 'first', client: STA });
show('a network in a tunnel', { sides: 'both', network: 'tunnelled', client: STA });
show('a VLAN, both sides: every network on it', { sides: 'both', vlan: 20 });
show('a VLAN, the wired side', { sides: 'wired', vlan: 10, client: STA });
show('a VLAN the uplink does not carry: the ports that do', { sides: 'wired', vlan: 30 });
show('a VLAN nothing carries', { sides: 'both', vlan: 99 });
show('a VLAN no network is on, the Wi-Fi side', { sides: 'wireless', vlan: 30 });
show('a network the AP has not', { sides: 'both', network: 'nowhere' });
show('a band it is not on', { sides: 'wireless', network: 'home-wi-fi-iot', bands: ['6g'] });
show('a client that is no MAC', { sides: 'both', network: 'lab', client: "aa'; reboot; '" });
show('no side said', { network: 'lab' });
show('a VLAN that is none', { sides: 'wired', vlan: 'ten' });
show('a VLAN on an AP whose uplink is in no bridge', { sides: 'wired', vlan: 10 }, { ...have, members: {} });
// An interface whose name a shell would read is never taken.
show('a network with an interface no AP names so', { sides: 'wireless', network: 'lab' });
// Many interfaces: so many at the most.
let wide = { ...have, wifi: {} };
for (let i = 0; i < 12; i++)
	wide.wifi['phy' + i + '-ap1'] = { nets: ['aeolus_lab'], band: '5g', ssid: 'Aeolus Lab' };
printf('of 12 radios with the network, %d interfaces are taken\n', length(pcap.plan({ sides: 'wireless', network: 'lab' }, wide).files));

// How many packets each may take: of 20 MB of headers, and of 50 MB of
// whole packets; and never so few that a quiet capture ends at once.
printf('packets each of 3 interfaces may take: %d of headers, %d of whole packets; of 1 with little room: %d\n',
	pcap.each(20971520, 3, 128), pcap.each(52428800, 3, 0), pcap.each(1000, 1, 128));

// The script that takes it.
let p = pcap.plan({ sides: 'both', network: 'home-wi-fi-iot', client: STA, bands: ['5g'] }, have);
print(pcap.script('/tmp/aeolus-capture', p.files, 60, 20971520, 128));
print(pcap.script('/tmp/aeolus-capture', pcap.plan({ sides: 'wired', vlan: 10 }, have).files, 30, 52428800, 0));

// What tcpdump says as it ends.
printf('%J\n', pcap.counted('tcpdump: listening on eth0, link-type EN10MB (Ethernet), snapshot length 128 bytes\n42 packets captured\n45 packets received by filter\n3 packets dropped by kernel\n'));
printf('%J, %J\n', pcap.counted('1 packet captured\n1 packet received by filter\n0 packets dropped by kernel\n'), pcap.counted('tcpdump: phy9-ap0: No such device exists\n'));
printf('%J; %J; %J\n', pcap.failed('tcpdump: phy9-ap0: No such device exists\n(SIOCGIFHWADDR: No such device)\n'),
	pcap.failed('tcpdump: listening on eth0, link-type EN10MB (Ethernet), snapshot length 128 bytes\n4 packets captured\n'), pcap.failed(''));
