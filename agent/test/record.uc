// The flight recorder's judgement (0207 part 3): TestAgentRecord runs this
// with the real ucode and compares what it prints with record.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/record.uc

'use strict';

import * as record from 'aeolus.record';

// A radio's five seconds: its busy, its clients, and what was sent to them.
let pt = (at, busy, clients, sent, again) => ({ at: at, radios: [{ radio: 'radio0', band: '5g', busy: busy, clients: clients, tx_packets: sent, tx_retries: again, tx_failed: 0 }], clients: [] });
// Five minutes as usual: 30% busy, ten clients, 5% tried again.
let usual = (busy, again) => map([...map(slice([0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], 0), (x, i) => i)],
	i => pt(1000 + i * 5, busy ?? 30, 10, 300, int(300 * (again ?? 0.05))));

printf('share %J %J %J, median %J %J %J\n', record.share({ tx_packets: 400, tx_retries: 100, tx_failed: 20 }), record.share({ tx_packets: 40, tx_retries: 10 }),
	record.share({ tx_packets: 100, tx_retries: 140, tx_failed: 140 }), record.median([3, 1, 2]), record.median([4, 1, 3, 2]), record.median([]));

// Tried again all at once: half of 400, against 5% as usual.
printf('retries %J\n', record.trigger(usual(), pt(1300, 35, 10, 400, 200)));
// Not where it is usually so; nor of too few packets; nor with too little
// before to say what is usual.
printf('retries, usual for it %J\n', record.trigger(usual(30, 0.2), pt(1300, 35, 10, 400, 200)));
printf('retries, too few sent %J\n', record.trigger(usual(), pt(1300, 35, 10, 150, 100)));
printf('retries, too little before %J\n', record.trigger(slice(usual(), 0, 5), pt(1300, 35, 10, 400, 200)));

// Clients gone at once: four of ten; not two.
printf('clients left %J\n', record.trigger(usual(), pt(1300, 30, 6, 300, 15)));
printf('clients left, two %J\n', record.trigger(usual(), pt(1300, 30, 8, 300, 15)));

// Its channel pinned: 96% three times running, against 30%; not against 70%.
let pinned = (b) => { let u = usual(b); push(u, pt(1300, 96, 10, 300, 15), pt(1305, 97, 10, 300, 15)); return u; };
printf('busy %J\n', record.trigger(pinned(30), pt(1310, 98, 10, 300, 15)));
printf('busy, usual for it %J\n', record.trigger(pinned(70), pt(1310, 98, 10, 300, 15)));
printf('busy, once %J\n', record.trigger(usual(), pt(1300, 99, 10, 300, 15)));
printf('nothing wrong %J, nothing %J\n', record.trigger(usual(), pt(1300, 31, 10, 300, 16)), record.trigger(null, null));

// A point kept: its 24 busiest clients.
let many = { at: 1, radios: [], clients: map([...map(slice(usual(), 0, 30), (x, i) => i)], i => ({ mac: sprintf('aa:00:00:00:00:%02x', i), tx_bytes: i * 100, rx_bytes: 0 })) };
let k = record.kept(many);
printf('kept %d clients, the busiest %s, the least %s; few kept as they are %J\n', length(k.clients), k.clients[0].mac, k.clients[23].mac, record.kept(pt(1, 1, 1, 1, 0)) != null);

// A recording: begun; made longer by what goes wrong in it; at most an hour.
let w = record.window(null, 10000, 'retries');
printf('window %J\n', w);
printf('window, again in it %J\n', record.window(w, 10300, 'busy'));
printf('window, longer than an hour %J\n', record.window({ ...w, until: 12900 }, 12800, 'busy'));
printf('window, after it %J\n', record.window(w, 20000, 'asked', 300, 300));

// Every radio down at once, and back (Griff, 2026-10-23). Living Room at
// 23:43:00: three radios with channels, then none; said before the clients
// it dropped.
let up3 = { at: 1000, radios: [{ band: '5g', channel: 100, clients: 5 }, { band: '2g', channel: 1, clients: 18 }, { band: '6g', channel: 101, clients: 1 }], clients: [] };
let down3 = { at: 1005, radios: [{ band: '5g', channel: null, clients: 0 }, { band: '2g', channel: null, clients: 0 }, { band: '6g', channel: null, clients: 0 }], clients: [] };
printf('down all %J %J %J\n', record.down_all(up3, down3), record.down_all(down3, down3), record.down_all(up3, up3));
printf('down trigger %J\n', record.trigger([up3], down3));
// Back on new phys: its driver loaded again; on the same, the Wi-Fi
// restarted; nothing down and the same phys, nothing.
printf('driver %J\n', record.restarted({ radio0: 'phy14', radio1: 'phy15', radio2: 'phy16' }, { radio0: 'phy18', radio1: 'phy19', radio2: 'phy20' }, 1000, 1035));
printf('wifi %J\n', record.restarted({ radio0: 'phy14', radio1: 'phy15' }, { radio0: 'phy14', radio1: 'phy15' }, 1000, 1030));
printf('nothing %J\n', record.restarted({ radio0: 'phy14' }, { radio0: 'phy14' }, null, 1030));
printf('one quickly %J\n', record.restarted({ radio0: 'phy14', radio2: 'phy16' }, { radio0: 'phy14', radio2: 'phy21' }, null, 1030));
