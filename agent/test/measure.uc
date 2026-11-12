// What a client hears of the AP, as it says itself (802.11k): TestAgentMeasure
// runs this with the real ucode and compares what it prints with
// measure.out. The reports are as hostapd said them on C360-AP and
// Living-Room-C-360, 2026-10-23.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/measure.uc

'use strict';

import * as measure from 'aeolus.measure';

// How a client is asked, by its RM Enabled Capabilities.
printf('mode %J %J %J %J %J %J\n', measure.mode([123, 6, 0, 96, 0]), measure.mode([0x20, 0, 0, 0, 0]), measure.mode([0x40, 0, 0, 0, 0]),
	measure.mode([0, 0, 0, 0, 0]), measure.mode([]), measure.mode(null));

// The operating class of a 20 MHz channel.
printf('op class %J %J %J %J %J %J %J\n', measure.op_class(2437, 6), measure.op_class(2484, 14), measure.op_class(5180, 36), measure.op_class(5260, 52),
	measure.op_class(5500, 100), measure.op_class(5785, 157), measure.op_class(5975, 5));

// Reports: a client on 5 GHz channel 157 at -56 dBm; one on 100 at -62
// with its RSNI; a half-dB one; one refused; one too short; another line.
let r1 = '<3>BEACON-RESP-RX 50:31:23:9d:72:62 1 00 7e9daafa7d3a0601000078000e6cff3a862def2fc00104267f3a01d103ec9a233700000064003110000a486f6d652057692d466901';
let r2 = '<3>BEACON-RESP-RX 04:4a:69:29:d3:26 1 00 796426236a1f00000000b0430060523a862dbca5c00058a06c1f0159039c6c1f3700000064001111000e53776565745f53706f745f496f54';
printf('report %J\n', measure.report(r1));
printf('report %J\n', measure.report(r2));
printf('report, half a dB %J\n', measure.report(replace(r1, '0e6cff3a', '0e61ff3a'))?.dbm);
// One that writes its signal as a signed dBm (0xc2, -62): read so; and one
// that is no signal heard (0x10, -102 dBm).
printf('report, a signed dBm %J, -102 %J\n', measure.report(replace(r1, '0e6cff3a', '0ec2ff3a')), measure.report(replace(r1, '0e6cff3a', '0e10ff3a')));
printf('report refused %J, too short %J, another line %J, nothing %J\n', measure.report('<3>BEACON-RESP-RX 50:31:23:9d:72:62 2 04 '),
	measure.report('<3>BEACON-RESP-RX 50:31:23:9d:72:62 3 00 7e9d'), measure.report('<3>BEACON-REQ-TX-STATUS 50:31:23:9d:72:62 1 ack=1'), measure.report(null));

// The clients due to be asked: the longest unasked first, at most two; not
// one asked lately, nor one that takes no request.
let cs = [{ mac: 'aa:00:00:00:00:01', mode: 0, asked: 900 }, { mac: 'aa:00:00:00:00:02', mode: 1, asked: null }, { mac: 'aa:00:00:00:00:03', mode: 0, asked: 500 },
	{ mac: 'aa:00:00:00:00:04', mode: null, asked: null }, { mac: 'aa:00:00:00:00:05', mode: 2, asked: 100 }];
printf('due %J\n', map(measure.due(cs, 1000, 2), c => c.mac));
printf('due, all %J\n', map(measure.due(cs, 1000), c => c.mac));

// What it hears now: as it said, moved by the AP's power since; not where
// it said so too long ago, or of another BSSID.
let said = { dbm: -61.5, at: 1000, power: 14, bssid: '3a:86:2d:bc:a5:c0' };
printf('hears %J, power down 6 %J, no power said %J, stale %J, another BSSID %J, nothing %J\n', measure.hears(said, 1100, 14, '3a:86:2d:bc:a5:c0'),
	measure.hears(said, 1100, 8, '3a:86:2d:bc:a5:c0'), measure.hears({ dbm: -70, at: 1000 }, 1100, 8, null), measure.hears(said, 3000, 14, null),
	measure.hears(said, 1100, 14, '3a:86:2d:ef:2f:c0'), measure.hears(null, 1100, 14, null));

// hostapd's beacon-report notification over ubus, as a steered network's
// listener hears it (0289): its fields read as the control socket's are; a
// report the client would not or could not make, one with no signal, or
// one not well said, is none.
let note = (more) => ({ address: '50:31:23:9D:72:62', 'op-class': 115, channel: 44, rcpi: 88, bssid: '20:05:B6:DF:64:E9', 'rep-mode': 0, ...(more ?? {}) });
printf('notified %J\n', measure.notified(note()));
printf('notified, the signal in dBm as a signed byte %J, no rep-mode said %J\n', measure.notified(note({ rcpi: 0xc2 })), measure.notified(note({ 'rep-mode': null })));
printf('not: refused %J, incapable %J, no signal %J, no BSSID %J, no channel %J, nothing %J\n', measure.notified(note({ 'rep-mode': 4 })), measure.notified(note({ 'rep-mode': 2 })),
	measure.notified(note({ rcpi: 255 })), measure.notified(note({ bssid: null })), measure.notified(note({ channel: null })), measure.notified(null));
