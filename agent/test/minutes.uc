// What a minute came to (0207): TestAgentMinutes runs this with the real
// ucode and compares what it prints with minutes.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/minutes.uc

'use strict';

import * as minutes from 'aeolus.minutes';

// A radio's survey, every 5 s: its channel busy a fifth of the time, then
// half of it for one stretch; its counts going back once, as when it moved.
let at = 1000000, sv = (time, busy, rx, tx, noise) => ({ time: time, busy: busy, rx: rx, tx: tx, noise: noise, power: 17, channel: 36, width: 80, clients: 2 });
let base = sv(at, 200000, 100000, 50000, -95);
let samples = [];
for (let i = 1; i <= 12; i++)
	push(samples, sv(at + i * 5000, 200000 + i * 1000 + (i == 6 ? 1500 : 0), 100000 + i * 500, 50000 + i * 200, i == 3 ? 0 : -95));
printf('radio %J\n', minutes.radio(base, samples));
let moved = [...slice(samples, 0, 6), sv(10, 2, 1, 0, -90), sv(5010, 1002, 501, 200, -90)];
printf('radio, its counts gone back %J\n', minutes.radio(base, moved));
printf('radio, its first minute %J\n', minutes.radio(null, slice(samples, 0, 2)));
printf('radio, no survey %J\n', minutes.radio(null, [{ power: 20, channel: 6, width: 20, clients: 0 }]));
printf('radio, nothing %J\n', minutes.radio(null, []));

// A station as nl80211 says it, and on an ath11k radio.
let sta = { signal: -61, signal_avg: -60, tx_bitrate: { bitrate32: 8667 }, rx_bitrate: { bitrate: 1300 }, tx_packets: 100, tx_retries: 0,
	tx_failed: 1, rx_packets: 90, tx_bytes64: 50000, rx_bytes: 20000, connected_time: 300 };
printf('station %J\n', minutes.station(sta));
printf('station on ath11k %J\n', minutes.station(sta, true, 7));
printf('station on ath11k, not read %J\n', minutes.station(sta, true, null));

// A client over a minute: it grew its counts, joined again halfway, and
// the radio counted its retries only at the minute's ends.
let st = (sig, conn, tx, retries, failed, bytes) => ({ signal: sig, tx_rate: 86.7, rx_rate: 130, tx_packets: tx, tx_retries: retries, tx_failed: failed,
	rx_packets: tx, tx_bytes: bytes, rx_bytes: bytes / 2, connected: conn });
let cb = st(-60, 300, 1000, 10, 1, 100000);
let cs = [st(-62, 305, 1100, null, 1, 110000), st(-70, 310, 1200, null, 2, 120000), st(-64, 3, 20, null, 0, 2000), st(-66, 8, 60, 4, 0, 6000)];
printf('client %J\n', minutes.client(cb, cs, 5));
printf('client, its first sample %J\n', minutes.client(null, [cs[0]], 5));
printf('client, nothing %J\n', minutes.client(null, [], 5));
// A count that went below zero in a driver's 32 bits (0272, mt76's tx
// retries, B124): passed over, it grows on from the count before it; a
// count that went back starts again; and growth, as ever.
let neg = 4294967296 - 19;
let cn = [st(-60, 305, 1100, neg, 1, 110000), st(-60, 310, 1200, 14, 1, 120000), st(-60, 315, 1300, 20, 1, 130000)];
printf('client, retries below zero %J\n', minutes.client(cb, cn, 5).tx_retries);
printf('grew by %J %J %J %J %J\n', minutes.grew_by(10, 15), minutes.grew_by(10, neg), minutes.grew_by(15, 10), minutes.grew_by(null, 5), minutes.grew_by(neg, neg + 9));

// A minute on an AP: two radios, one with two clients.
let radios = {
	radio1: { band: '5g', base: base, samples: samples, clients: {
		'aa:bb:cc:00:00:02': { ssid: 'Lab', base: cb, samples: cs },
		'aa:bb:cc:00:00:01': { ssid: 'Lab', base: null, samples: [st(-50, 1, 1, 0, 0, 10), st(-52, 6, 5, 0, 0, 900000)] },
	} },
	radio0: { band: '2g', base: null, samples: [{ power: 8, channel: 6, width: 20, clients: 0 }], clients: {} },
	radio2: { band: '6g', base: null, samples: [], clients: {} },
};
printf('minute %J\n', minutes.minute(1792627200, radios));
printf('tenth %J %J %J whole %J %J\n', minutes.tenth(12.345), minutes.tenth(-0.05), minutes.tenth(0), minutes.whole(-60.5), minutes.whole(2.4));

// What the minute came to on each radio, for the AP's own daemons (0207):
// a radio that sent too little to say how much was tried again says none;
// mt76's one count said twice is counted once.
let m = minutes.minute(1792627200, radios);
m.radios[1].tx_failed = 4;
push(m.radios, { radio: 'radio9', band: '6g', channel: 5, busy: 1.5, busy_max: 2.0, clients: 1.0, tx_packets: 100, tx_retries: 140, tx_failed: 140 });
printf('load %J\n', minutes.load(m));
printf('load of nothing %J\n', minutes.load(null));

// The uplink's Ethernet ports (0351): what sysfs says of one, and what a
// minute came to on it. A 2.5 Gbit/s port that ran at 2.3 Gbit/s out for
// one five seconds; errors and a link that went and came; a count that went
// back, passed over; and a 32-bit count that went round once.
printf('port sample %J\n', minutes.port_sample({ carrier: '1', speed: '2500', duplex: 'full', carrier_changes: '3',
	rx_bytes: '80524384720', tx_bytes: '16165091524', rx_errors: '0', tx_errors: '2' }));
printf('port sample, no link %J\n', minutes.port_sample({ carrier: '0', speed: null, duplex: 'unknown', carrier_changes: '4', rx_bytes: '10', tx_bytes: 'x' }));
printf('port sample, nothing %J\n', minutes.port_sample(null));
let ps = (at, rx, tx, e, ch) => ({ at: at, rx_bytes: rx, tx_bytes: tx, rx_errors: e, tx_errors: 0, changes: ch, up: true, speed: 2500, full: true });
let ub = ps(1792627200, 1000000000, 2000000000, 5, 3);
let us = [
	ps(1792627205, 1000000000 + 62500000, 2000000000 + 1437500000, 5, 3),   // 100 Mbit/s in, 2300 out
	ps(1792627210, 1000000000 + 125000000, 2000000000 + 1500000000, 7, 3),
	ps(1792627215, 10, 20, 7, 5),   // its driver started again: passed over
	ps(1792627220, 6250010, 62500020, 8, 5),
];
printf('uplink %J\n', minutes.uplink(ub, us));
printf('uplink, no base %J\n', minutes.uplink(null, [ps(1792627205, 5, 5, 0, 1)]));
printf('uplink, nothing %J\n', minutes.uplink(ub, []));
printf('wire grew %J %J %J %J\n', minutes.wire_grew(10, 6250000010), minutes.wire_grew(4294967290, 10), minutes.wire_grew(5000000000, 10), minutes.wire_grew(null, 1));
printf('minute with its uplink %J\n', minutes.minute(1792627200, {}, { lan1: { base: ub, samples: us }, lan2: { base: null, samples: [] } }));
printf('minute without %J\n', minutes.minute(1792627200, {}).uplink);
