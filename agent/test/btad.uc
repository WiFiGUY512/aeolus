// Prints what the agent makes of the devices the AP's Bluetooth radio hears
// (0157): what each advertisement says, the table aeolus-ble keeps, the
// survey's list taken with it, and what is reported, the devices that
// rotate their address counted, not listed; for a test to compare with
// btad.out. The addresses and data are made up.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/btad.uc

'use strict';

import * as bt from 'aeolus.btad';

function bytes(...b) {
	let s = '';
	for (let x in b)
		s += chr(x);
	return s;
}

function ad(t, ...v) {
	return bytes(length(v) + 1, t, ...v);
}

print('== what an advertisement says\n');
let flags = ad(0x01, 0x06);
let cases = {
	'a name, complete': flags + ad(0x09, 0x4e, 0x30, 0x35, 0x4c, 0x38),
	'a short name, then a complete one': ad(0x08, 0x41) + ad(0x09, 0x41, 0x42, 0x43),
	'a name with a control character': ad(0x09, 0x41, 0x07, 0x42),
	'iBeacon': flags + ad(0xff, 0x4c, 0x00, 0x02, 0x15, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x00, 0x07, 0x01, 0x2c, 0xc5),
	'Apple Find My': ad(0xff, 0x4c, 0x00, 0x12, 0x19, 0x00),
	'Apple, nearby': flags + ad(0xff, 0x4c, 0x00, 0x10, 0x05, 0x03, 0x1c),
	'Eddystone URL': ad(0x03, 0xaa, 0xfe) + ad(0x16, 0xaa, 0xfe, 0x10, 0xf8, 0x00),
	'Google Find My Device': ad(0x16, 0xaa, 0xfe, 0x41, 0x00),
	'Tile': ad(0x03, 0xed, 0xfe),
	'Samsung SmartTag': ad(0x16, 0x5a, 0xfd, 0x01),
	'BTHome, with power and appearance': ad(0x16, 0xd2, 0xfc, 0x40, 0x02, 0xc4, 0x09) + ad(0x0a, 0xf4) + ad(0x19, 0xc1, 0x03),
	'a 128-bit service': ad(0x07, 0x10, 0x0f, 0x0e, 0x0d, 0x0c, 0x0b, 0x0a, 0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01),
	'one that says it is longer than it is': bytes(0x09, 0x09, 0x41),
	'nothing': '',
};
for (let name, data in cases)
	print(sprintf('%s: %J\n', name, bt.parse(data)));

print('== the kinds of random address\n');
for (let mac in ['C1:00:00:00:00:01', '41:00:00:00:00:02', '01:00:00:00:00:03', '81:00:00:00:00:04'])
	print(mac, ' ', bt.random_kind(mac), '\n');

print('== the table the daemon keeps\n');
let table = {};
let fixed = { mac: 'C1:00:00:00:00:01', type: 1, data: flags + ad(0x08, 0x53) };
bt.note(table, { ...fixed, rssi: -60 }, 1000);
bt.note(table, { ...fixed, rssi: -50, data: flags + ad(0x09, 0x53, 0x65, 0x6e, 0x73, 0x6f, 0x72) }, 1010);
bt.note(table, { ...fixed, rssi: 127, data: flags + ad(0x08, 0x53) }, 1020);
bt.note(table, { mac: 'AA:BB:CC:00:00:02', type: 0, rssi: -70, data: ad(0xff, 0x59, 0x00, 0x01, 0x02) }, 1015);
for (let i = 0; i < 3; i++)
	bt.note(table, { mac: sprintf('4%d:00:00:00:00:1%d', i, i), type: 1, rssi: -80 + i, data: ad(0xff, 0x06, 0x00, 0x01, 0x09, 0x20) }, 1018 + i);
bt.note(table, { mac: '0A:00:00:00:00:20', type: 1, rssi: -66, data: ad(0xff, 0x4c, 0x00, 0x10, 0x05) }, 1020);
bt.note(table, { mac: 'C2:00:00:00:00:30', type: 1, rssi: -75, data: ad(0xff, 0x4c, 0x00, 0x12, 0x19) }, 1020);
let list = sort(bt.listing(table), (x, y) => x.addr < y.addr ? -1 : x.addr > y.addr ? 1 : 0);
for (let d in list)
	print(sprintf('%J\n', d));

print('== forgotten: what was last heard long ago, and the oldest past the most\n');
let small = { ...table };
bt.forget(small, 1618, 600, 100);
print(join(' ', sort(keys(small))), '\n');
bt.forget(small, 1618, 600, 4);
print(join(' ', sort(keys(small))), '\n');

print('== the survey taken with the table\n');
let survey = [
	{ addr: 'c1:00:00:00:00:01', type: 'random', random: 'static', rssi: -52, rssi_avg: -52, rssi_max: -52, seen_total: 1, first_seen: 1005, last_seen: 1005, uuid16: ['fd3d'], name: 'From the survey' },
	{ addr: 'aa:bb:cc:00:00:99', type: 'public', rssi: -88, rssi_avg: -87.5, rssi_max: -86, seen_total: 2, first_seen: 900, last_seen: 1001, name: 'Only the survey' },
	{ addr: 'aa:bb:cc:00:00:98', type: 'public', rssi: -40, seen_total: 1, first_seen: 100, last_seen: 200, name: 'Long gone' },
];
let all = bt.merge(bt.listing(table), survey);
print(length(all), ' devices; c1:00:00:00:00:01 is named ', filter(all, d => d.addr == 'c1:00:00:00:00:01')[0].name,
	' and has ', filter(all, d => d.addr == 'c1:00:00:00:00:01')[0].uuid16, '\n');

print('== reported\n');
let out = bt.fold(all, 1030);
for (let d in out.devices)
	print(sprintf('%J\n', d));
for (let g in out.rotating)
	print(sprintf('rotating: %J\n', g));
print('nothing: ', bt.fold([], 1030), '; not a list: ', bt.fold(null, 1030), '\n');
let many = [];
for (let i = 0; i < 80; i++)
	push(many, { addr: sprintf('aa:bb:cc:00:01:%02x', i), type: 'public', rssi: -90 + (i % 50), last_seen: 1000 });
let lots = bt.fold(many, 1030);
print(length(lots.devices), ' of 80 reported, the strongest first: ', lots.devices[0].rssi, ' to ', lots.devices[length(lots.devices) - 1].rssi, '\n');

// A signal no Bluetooth radio can say is left out (0241): noted at 99 dBm,
// as the Living Room's fridge once was, it is neither its signal nor its
// strongest; one kept that way is reported without it.
let fridge = {};
bt.note(fridge, { mac: '38:68:A4:3C:59:F1', type: 0, rssi: -68, data: '' }, 2000);
bt.note(fridge, { mac: '38:68:A4:3C:59:F1', type: 0, rssi: 99, data: '' }, 2001);
let f = fridge['38:68:a4:3c:59:f1'];
print('the fridge at 99 dBm: rssi ', f.rssi, ', strongest ', f.rssi_max, '\n');
let kept = bt.fold([{ addr: '38:68:a4:3c:59:f1', type: 'public', rssi: -68, rssi_avg: -62, rssi_max: 99, seen_total: 9, first_seen: 1990, last_seen: 2001 }], 2002);
print('reported from a table kept at 99: ', kept.devices[0], '\n');
print('dbm: ', map([-128, -127, -60, 20, 21, 99, 127, -60.4, null, 'x'], bt.dbm), '\n');
