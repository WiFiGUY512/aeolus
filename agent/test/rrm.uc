// Prints radio resource management's pure parts (0073), for the manager's
// contract test: rrm.out is what it must print. The first three HMACs are
// RFC 4231's test cases 1, 2 and 6, and the fourth, with NUL bytes in its
// key and message, was worked out with Python's hmac, so they are checked
// apart from ucode.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/rrm.uc

'use strict';

import * as rrm from 'aeolus.rrm';

function rep(c, n) {
	let o = '';
	for (let i = 0; i < n; i++)
		o += c;
	return o;
}

printf('hmac 1 %s\n', rrm.hmac(rep('\x0b', 20), 'Hi There'));
printf('hmac 2 %s\n', rrm.hmac('Jefe', 'what do ya want for nothing?'));
printf('hmac 6 %s\n', rrm.hmac(rep('\xaa', 131), 'Test Using Larger Than Block-Size Key - Hash Key First'));
printf('hmac nul %s\n', rrm.hmac('\x00\x01\x00\x02', 'a\x00b\x00c'));

// The lab's element, as hostapd took it on 2026-10-05, and what an AP that
// hears it reads.
let ad = rrm.advert('ap-a0046082275e', '192.168.1.38', rrm.PORT);
printf('advert %s\n', ad);
printf('read %J\n', rrm.read_advert(rrm.unhex(substr(ad, 4))));
printf('read later version %J\n', rrm.read_advert(rrm.unhex(replace(substr(ad, 4), /^02ae010101/, '02ae010102') + 'ff')));
printf('read other OUI %J\n', rrm.read_advert(rrm.unhex('0050f2' + substr(ad, 10))));
printf('read short %J\n', rrm.read_advert(rrm.unhex(substr(ad, 4, 20))));
printf('advert bad ID %J\n', rrm.advert('office', '192.168.1.38', rrm.PORT));
printf('advert bad address %J\n', rrm.advert('ap-a0046082275e', '192.168.1.300', rrm.PORT));

// A hello, signed and read back; tampered with, under another key, or of
// another version, it is refused.
let key = rrm.unhex(rep('42', 32));
let hello = { v: 1, t: 'hello', ap: 'ap-2005b6df64e0', at: 1000, seq: 7, sees: ['ap-a0046082275e'] };
let pkt = rrm.seal(key, hello);
printf('open %J\n', rrm.open(key, pkt));
printf('open tampered %J\n', rrm.open(key, replace(pkt, '"seq": 7', '"seq": 8')));
printf('open other key %J\n', rrm.open(rrm.unhex(rep('43', 32)), pkt));
printf('open version 2 %J\n', rrm.open(key, rrm.seal(key, { ...hello, v: 2 })));
printf('open not an AP %J\n', rrm.open(key, rrm.seal(key, { ...hello, ap: 'office' })));

// Hellos: new, too far off the clock, sent again, and later.
printf('fresh first %J\n', rrm.fresh(null, { at: 1000, seq: 1 }, 1010));
printf('fresh skewed %J\n', rrm.fresh(null, { at: 1000, seq: 1 }, 1100));
printf('fresh again %J\n', rrm.fresh({ at: 1000, seq: 1 }, { at: 1000, seq: 1 }, 1010));
printf('fresh next %J\n', rrm.fresh({ at: 1000, seq: 1 }, { at: 1000, seq: 2 }, 1010));
printf('fresh older %J\n', rrm.fresh({ at: 1000, seq: 5 }, { at: 999, seq: 9 }, 1010));

// The three heard most strongly on each band, by name where alike.
printf('choose %J\n', rrm.choose({
	'ap-000000000001': { '2g': -60, '5g': -70 }, 'ap-000000000002': { '2g': -50 }, 'ap-000000000003': { '2g': -80 },
	'ap-000000000004': { '2g': -55 }, 'ap-000000000005': { '5g': -40 }, 'ap-000000000006': { '2g': -55 },
}, rrm.NEIGHBOURS));
printf('smooth %J %J\n', rrm.smooth(null, -70), rrm.smooth(-70, -50));

// Where a radio listens.
printf('visits 2g 11 %J\n', rrm.visits('2g', 11, false));
printf('visits 2g 3 %J\n', rrm.visits('2g', 3, false));
printf('visits 2g set %J\n', rrm.visits('2g', 6, false, [11, 1, 6, 3]));
printf('visits 5g 149 %J\n', rrm.visits('5g', 149, false));
printf('visits 5g 100 dfs %d\n', length(rrm.visits('5g', 100, true)));
// 6 GHz (0271): its set, sorted; or else the preferred scanning channels;
// and its own channel where neither has it.
printf('visits 6g set %J\n', rrm.visits('6g', 5, false, [21, 5, 37]));
printf('visits 6g psc %d %J\n', length(rrm.visits('6g', 5, false)), slice(rrm.visits('6g', 5, false), 0, 3));
printf('visits 6g own %J\n', rrm.visits('6g', 1, false, [5, 21]));
printf('band %s %s %J\n', rrm.band_of(2462), rrm.band_of(5745), rrm.band_of(900));

// Channel ratings: a network's weight, a visit's cost, the ratings, and the
// channels a neighbour's radio takes up.
printf('weight %J %J %J %J\n', rrm.weight(-50), rrm.weight(-75), rrm.weight(-100), rrm.weight(null));
printf('cost quiet %J\n', rrm.cost(0, null, []));
printf('cost busy %J\n', rrm.cost(0.3, -92, [-60, -85]));
printf('cost no noise %J\n', rrm.cost(1.5, null, [-40]));
printf('blend %J %J %J\n', rrm.blend(null, 40, rrm.LASTING), rrm.blend(40, 0, rrm.LASTING), rrm.blend(40, 0, rrm.NOW));
// The pick: a free channel first; with all three of 2.4 GHz used, the one
// whose nearest user is heard most weakly; alike, the best rated.
let free = [{ channel: 1, cost: 60, blotted_by: [] }, { channel: 6, cost: 30, blotted_by: [] }, { channel: 11, cost: 5, blotted_by: ['ap-000000000001'] }];
let full = [{ channel: 1, cost: 60, blotted_by: ['ap-000000000002'] }, { channel: 6, cost: 30, blotted_by: ['ap-000000000001', 'ap-000000000003'] }, { channel: 11, cost: 5, blotted_by: ['ap-000000000001'] }];
let close = { 'ap-000000000001': -60, 'ap-000000000002': -82, 'ap-000000000003': -85 };
printf('pick free %J\n', rrm.pick(free, close));
printf('pick all used %J\n', rrm.pick(full, close));
printf('pick alike %J\n', rrm.pick([{ channel: 1, cost: 60, blotted_by: ['ap-000000000002'] }, { channel: 6, cost: 30, blotted_by: ['ap-000000000002'] }], close));
printf('pick unknown is near %J\n', rrm.pick([{ channel: 1, cost: 60, blotted_by: ['ap-00000000000f'] }, { channel: 6, cost: 30, blotted_by: ['ap-000000000001'] }], close));
printf('pick none %J\n', rrm.pick([], close));
printf('covers %J %J %J %J %J\n', rrm.covers('2g', 11, 20), rrm.covers('5g', 149, 40), rrm.covers('5g', 157, 80), rrm.covers('5g', 165, 40), rrm.covers('5g', 100, 160));

// Moves (0073): what a radio at a width may move to, why it would, the
// tie-break, the window, and hostapd's switch arguments.
let five = [
	{ channel: 36, cost: 10, now: 12, blotted_by: [] }, { channel: 40, cost: 30, now: 20, blotted_by: [] },
	{ channel: 44, cost: 5, now: 5, blotted_by: ['ap-000000000001'] }, { channel: 48, cost: 8, now: 8, blotted_by: [] },
	{ channel: 149, cost: 50, now: 60, blotted_by: [] },
];
printf('blocks 20 %J\n', map(rrm.blocks(five, '5g', 20), x => x.channel));
printf('blocks 40 %J\n', rrm.blocks(five, '5g', 40));
// A block is entered on its lowest channel, its primary, though its upper is
// rated better (0243).
printf('blocks 40 primary %J\n', map(rrm.blocks([{ channel: 149, cost: 9 }, { channel: 153, cost: 1 }], '5g', 40), x => x.channel));
printf('blocks 2g %J\n', map(rrm.blocks(free, '2g', 40), x => x.members));
printf('radar %J %J %J %J\n', rrm.radar('5g', 52), rrm.radar('5g', 48), rrm.radar('5g', 144), rrm.radar('2g', 1));
close['ap-000000000004'] = -64;
let b = (ch, cost, by) => ({ channel: ch, cost: cost, blotted_by: by, members: [ch] });
printf('reason shared %J\n', rrm.reason(b(11, 5, ['ap-000000000001']), b(6, 30, []), close, 20, null, false));
printf('reason all used, further %J\n', rrm.reason(b(11, 5, ['ap-000000000001']), b(1, 60, ['ap-000000000002']), close, 20, null, false));
printf('reason all used, not far enough %J\n', rrm.reason(b(11, 5, ['ap-000000000001']), b(1, 60, ['ap-000000000004']), close, 20, null, false));
printf('reason better %J\n', rrm.reason(b(1, 60, []), b(6, 30, []), close, 20, null, false));
printf('reason not by the margin %J\n', rrm.reason(b(1, 45, []), b(6, 30, []), close, 20, null, false));
printf('reason start %J\n', rrm.reason(b(1, 45, []), b(6, 30, []), close, 20, null, true));
printf('reason interference %J\n', rrm.reason(b(6, 10, []), b(1, 20, []), close, 20, 0.7, false));
printf('reason busy, nowhere better %J\n', rrm.reason(b(6, 10, []), b(1, 40, []), close, 20, 0.55, false));
printf('reason free to shared %J\n', rrm.reason(b(6, 60, []), b(1, 5, ['ap-000000000002']), close, 20, null, false));
printf('reason same block %J\n', rrm.reason({ channel: 36, cost: 50, blotted_by: [], members: [36, 40] }, b(40, 5, []), close, 20, null, true));
printf('first worse %J\n', rrm.first({ ap: 'ap-000000000002', cost: 40 }, { ap: 'ap-000000000001', cost: 30 }));
printf('first alike %J %J\n', rrm.first({ ap: 'ap-000000000002', cost: 30 }, { ap: 'ap-000000000001', cost: 30 }),
	rrm.first({ ap: 'ap-000000000001', cost: 30 }, { ap: 'ap-000000000002', cost: 30 }));
printf('window %J %J %J %J\n', rrm.window('02:00-05:00'), rrm.window('22:30-04:15'), rrm.window('08:09-09:00'), rrm.window('25:00-01:00'));
printf('in window %J %J %J %J %J %J %J\n', rrm.in_window([120, 300], 150), rrm.in_window([120, 300], 300),
	rrm.in_window([1350, 255], 1400), rrm.in_window([1350, 255], 100), rrm.in_window([1350, 255], 600),
	rrm.in_window([60, 60], 900), rrm.in_window(null, 150));
printf('switch 2g 6 %J\n', rrm.switch_args('2g', 6, 20, 'HT20', 10));
printf('switch 2g 11 he %J\n', rrm.switch_args('2g', 11, 20, 'HE20', 10));
printf('switch 2g 11 40 %J\n', rrm.switch_args('2g', 11, 40, 'HT40', 10));
// At 40 MHz a 2.4 GHz channel is joined with the one four above for 1 to 6,
// four below from 7 up, as OpenWrt joins a set channel (0117).
printf('switch 2g 6 40 %J\n', rrm.switch_args('2g', 6, 40, 'HT40', 10));
printf('switch 2g 7 40 %J\n', rrm.switch_args('2g', 7, 40, 'HT40', 10));
printf('switch 5g 36 he40 %J\n', rrm.switch_args('5g', 36, 40, 'HE40', 10));
printf('switch 5g 40 vht40 %J\n', rrm.switch_args('5g', 40, 40, 'VHT40', 10));
printf('switch 5g 157 vht80 %J\n', rrm.switch_args('5g', 157, 80, 'VHT80', 10));
printf('switch 5g 161 vht80 %J\n', rrm.switch_args('5g', 161, 80, 'VHT80', 10));
printf('switch 5g 165 40 %J\n', rrm.switch_args('5g', 165, 40, 'HE40', 10));
printf('switch 5g 36 legacy %J\n', rrm.switch_args('5g', 36, 40, 'NOHT', 10));
// 6 GHz (0278): blocks from channel 1 a width apart, HE with no HT or VHT;
// 320 MHz, whose blocks overlap, is not taken.
printf('switch 6g 133 he80 %J\n', rrm.switch_args('6g', 133, 80, 'HE80', 10));
printf('switch 6g 5 he80 %J\n', rrm.switch_args('6g', 5, 80, 'HE80', 10));
printf('switch 6g 37 he160 %J\n', rrm.switch_args('6g', 37, 160, 'HE160', 10));
printf('switch 6g 21 eht80 %J\n', rrm.switch_args('6g', 21, 80, 'EHT80', 10));
printf('switch 6g 5 20 %J\n', rrm.switch_args('6g', 5, 20, 'HE20', 10));
printf('switch 6g 5 320 %J\n', rrm.switch_args('6g', 5, 320, 'EHT320', 10));

// hostapd's own command for a move to a channel shared with radar (0120):
// to 52 at 40 MHz as the C-360 took it on 2026-10-10, and at 20 and 80 MHz;
// a Wi-Fi 7 radio names that generation too.
printf('chan switch 52 he40 %s\n', rrm.chan_switch(rrm.switch_args('5g', 52, 40, 'HE40', 10)));
printf('chan switch 56 he40 %s\n', rrm.chan_switch(rrm.switch_args('5g', 56, 40, 'HE40', 10)));
printf('chan switch 100 vht20 %s\n', rrm.chan_switch(rrm.switch_args('5g', 100, 20, 'VHT20', 10)));
printf('chan switch 108 eht80 %s\n', rrm.chan_switch(rrm.switch_args('5g', 108, 80, 'EHT80', 10)));
printf('chan switch 132 ht40 %s\n', rrm.chan_switch(rrm.switch_args('5g', 132, 40, 'HT40', 10)));
printf('chan switch 6g 133 he80 %s\n', rrm.chan_switch(rrm.switch_args('6g', 133, 80, 'HE80', 10)));

// Power control (0077): looking for three neighbours, the weakest at -70 dBm.
printf('power none heard %J\n', rrm.power_step([], 3, -70, 20, 26));
printf('power two of three %J\n', rrm.power_step([-60, -65], 3, -70, 20, 26));
printf('power weakest low %J\n', rrm.power_step([-60, -65, -74, -90], 3, -70, 20, 26));
printf('power at target %J\n', rrm.power_step([-60, -65, -68], 3, -70, 20, 26));
printf('power well above %J\n', rrm.power_step([-50, -55, -62], 3, -70, 20, 26));
printf('power near the ceiling %J\n', rrm.power_step([-80], 1, -70, 25, 26));
printf('power at the ceiling %J\n', rrm.power_step([-80], 1, -70, 26, 26));
printf('power at the floor %J\n', rrm.power_step([-40], 1, -70, 8, 26));
printf('power near the floor %J\n', rrm.power_step([-40], 1, -70, 10, 26));
printf('power ignores junk %J\n', rrm.power_step([-60, null, 'x', -66], 2, -70, 20, 26));

// Clients power control listens to (0206): at 8 dBm with a floor of -80,
// a robot no other AP hears, one a neighbour hears 5 dB better, one only 2
// dB better, one just come, a test client, the AP's own, and one with no
// signal.
let cl = [
	{ mac: '24:58:7c:eb:b8:8c', signal: -72, connected: 7200 },
	{ mac: 'aa:aa:aa:aa:aa:01', signal: -70, connected: 7200 },
	{ mac: 'aa:aa:aa:aa:aa:02', signal: -75, connected: 7200 },
	{ mac: 'aa:aa:aa:aa:aa:03', signal: -85, connected: 600 },
	{ mac: '0e:46:34:8f:00:10', signal: -80, connected: 7200 },
	{ mac: '30:86:2d:ef:2f:80', signal: -60, connected: 7200 },
	{ mac: 'aa:aa:aa:aa:aa:04', connected: 7200 },
];
let lone = rrm.lonely(cl, { 'aa:aa:aa:aa:aa:01': -65, 'aa:aa:aa:aa:aa:02': -73 }, 8, -80, ['30:86:2d:ef:2f:80']);
printf('lonely %J\n', lone);
// One that says what it hears of the AP (802.11k) is measured, not reckoned
// (0207): heard at -60, it is no longer the loneliest.
let cl2 = map(cl, c => c.mac == '24:58:7c:eb:b8:8c' ? { ...c, heard: -60 } : c);
printf('lonely, one measured %J\n', rrm.lonely(cl2, { 'aa:aa:aa:aa:aa:01': -65, 'aa:aa:aa:aa:aa:02': -73 }, 8, -80, ['30:86:2d:ef:2f:80']));
printf('floor %J, at a ceiling of 10 %J, none %J\n', rrm.client_floor(lone, 26), rrm.client_floor(lone, 10),
	rrm.client_floor([{ mac: 'x', need: 5 }], 26));

// A hello fitted to one frame (0206): the faintest clients it hears go
// first, then the faintest APs; one that fits is left whole.
let big = { v: 1, t: 'hello', ap: 'ap-000000000001', heard: [], hears: [] };
for (let i = 0; i < 6; i++)
	push(big.heard, { ap: sprintf('ap-0000000000%02d', i), band: '5g', signal: -40 - i });
for (let i = 0; i < 32; i++)
	push(big.hears, [sprintf('aa:bb:cc:dd:ee:%02x', i), '2g', -40 - i]);
let fitted = rrm.fit(json(sprintf('%J', big)));
printf('fit: %d heard, %d hears, the last %J, within %J\n', length(fitted.heard), length(fitted.hears), fitted.hears[length(fitted.hears) - 1],
	64 + length(sprintf('%J', fitted)) <= rrm.HELLO_MAX);
fitted = rrm.fit(json(sprintf('%J', big)), 900);
printf('fit 900: %d heard, %d hears, within %J\n', length(fitted.heard), length(fitted.hears), 64 + length(sprintf('%J', fitted)) <= 900);
let small = { v: 1, t: 'hello', ap: 'ap-000000000001', heard: [big.heard[0]], hears: [big.hears[0]] };
printf('fit small: %J\n', rrm.fit(small) == small && length(small.hears) == 1 && length(small.heard) == 1);

// A step taken badly (0206): before and after, ten minutes each.
let calm = { probes: { 'a': 1, 'b': 0, 'c': 2 }, left: 0, packets: 1000, failed: 20 };
printf('distress none %J\n', rrm.distress(calm, calm, null));
printf('distress scanning %J\n', rrm.distress(calm, { probes: { 'a': 6, 'b': 4, 'c': 3 }, left: 0, packets: 1000, failed: 20 }, null));
printf('distress one scanning %J\n', rrm.distress(calm, { probes: { 'a': 6, 'b': 0, 'c': 2 }, left: 0, packets: 1000, failed: 20 }, null));
printf('distress leaving %J\n', rrm.distress(calm, { probes: {}, left: 3, packets: 1000, failed: 20 }, null));
printf('distress retries %J\n', rrm.distress(calm, { probes: {}, left: 0, packets: 900, failed: 200 }, null));
printf('distress retries, too few sent %J\n', rrm.distress(calm, { probes: {}, left: 0, packets: 100, failed: 60 }, null));
// A driver that counts more tried again than was sent (mt76, one count said
// twice): a share is at most 1.
printf('distress retries, more than sent %J\n', rrm.distress(calm, { probes: {}, left: 0, packets: 900, failed: 1500 }, null));
printf('distress everywhere %J\n', rrm.distress(calm, { probes: {}, left: 3, packets: 1000, failed: 20 },
	{ before: { probes: {}, left: 0 }, after: { probes: {}, left: 4 } }));
printf('distress, the other calm %J\n', rrm.distress(calm, { probes: {}, left: 3, packets: 1000, failed: 20 },
	{ before: { probes: {}, left: 0 }, after: { probes: {}, left: 0 } }));

// What a scan radio heard (0116), as airscan lists it: this AP's own
// network, two of a neighbour's 2.4 GHz radio's networks and one of its 5 GHz
// radio's, by the BSSIDs its hellos carry, and others: named, hidden, heard
// louder than a signal is said, and ill formed.
let theirs = {
	'05:b6:df:64:e2': { ap: 'ap-2005b6df64e0', band: '2g' },
	'05:b6:df:64:e8': { ap: 'ap-2005b6df64e0', band: '5g' },
};
let look = rrm.swept([
	{ bssid: '36:86:2d:ef:2f:d0', ssid: 'test', band: '2g', channel: 6, signal: 4, own: true },
	{ bssid: '20:05:B6:DF:64:E2', ssid: 'Aeolus Lab', band: '2g', channel: 1, signal: -31 },
	{ bssid: '26:05:b6:df:64:e2', ssid: 'Home Wi-Fi', band: '2g', channel: 1, signal: -30 },
	{ bssid: '22:05:b6:df:64:e8', ssid: 'Aeolus Lab', band: '5g', channel: 132, signal: -44 },
	{ bssid: '22:05:b6:df:64:e2', ssid: 'a 5 GHz network with the 2.4 GHz radio\'s tail', band: '5g', channel: 36, signal: -70 },
	{ bssid: '5c:83:6c:d6:df:c0', ssid: 'Home Wi-Fi', band: '2g', channel: 1, signal: -43 },
	{ bssid: '5c:83:6c:d6:df:c1', ssid: '', band: '2g', channel: 3, signal: -60, hidden: true },
	{ bssid: '5c:83:6c:d6:df:c2', ssid: 'caf\xc3\xa9', band: '2g', channel: 8, signal: 3 },
	{ bssid: 'not a bssid', ssid: 'x', band: '2g', channel: 1, signal: -50 },
	{ bssid: '5c:83:6c:d6:df:c3', ssid: 'x', band: '60g', channel: 1, signal: -50 },
	{ bssid: '5c:83:6c:d6:df:c4', ssid: 'x', band: '2g', channel: 0, signal: -50 },
	{ bssid: '5c:83:6c:d6:df:c5', ssid: 'x', band: '2g', channel: 1, signal: null },
	'junk',
], theirs);
printf('swept aps %J\n', look.aps);
printf('swept tails %J\n', look.tails);
for (let o in look.others)
	printf('swept other %J\n', o);
printf('swept networks %J\n', look.networks);
printf('around 2g 1 %J\n', rrm.around(look.networks, '2g', 1));
printf('around 2g 6 %J\n', rrm.around(look.networks, '2g', 6));
printf('around 2g 11 %J\n', rrm.around(look.networks, '2g', 11));
printf('around 5g 36 %J\n', rrm.around(look.networks, '5g', 36));
printf('around 5g 40 %J\n', rrm.around(look.networks, '5g', 40));
printf('swept nothing %J\n', rrm.swept(null, null));

// An advert heard in a beacon on the scan radio's monitor interface
// (0161): a radiotap header of 18 bytes, then the beacon, its fixed fields,
// an SSID and the living-room C-360's advert. Then that radio known by its
// BSSID with no band, in a look of airscan's.
let el = rrm.unhex(substr(rrm.advert('ap-30862dbca5ff', '192.168.1.73', rrm.PORT), 4));
let rt = chr(0, 0, 18, 0) + rep('\x00', 14);
let hdr = chr(0x80, 0) + rep('\x00', 2) + rep('\xff', 6) + chr(0x30, 0x86, 0x2d, 0xbc, 0xa5, 0xd0) + chr(0x30, 0x86, 0x2d, 0xbc, 0xa5, 0xd0) + chr(0, 0);
let fixed = rep('\x00', 8) + chr(0x64, 0, 0x11, 0x04);
let named = chr(0, 10) + 'Home Wi-Fi';
let beacon = rt + hdr + fixed + named + chr(221, length(el)) + el;
printf('beacon advert %J\n', rrm.beacon_advert(beacon));
printf('beacon without one %J\n', rrm.beacon_advert(rt + hdr + fixed + named));
printf('a probe response %J\n', rrm.beacon_advert(rt + chr(0x50) + substr(hdr, 1) + fixed + named + chr(221, length(el)) + el));
printf('cut short %J\n', rrm.beacon_advert(substr(beacon, 0, length(beacon) - 3)));
printf('nothing %J\n', rrm.beacon_advert(null));
printf('the beacon filter %J\n', rrm.BEACONS);
let found = rrm.swept([
	{ bssid: '30:86:2d:bc:a5:d0', ssid: 'Home Wi-Fi', band: '2g', channel: 6, signal: -7 },
	{ bssid: '3e:86:2d:bc:a5:d0', ssid: 'Home_Wi-Fi_IoT', band: '2g', channel: 6, signal: -8 },
	{ bssid: '30:86:2d:bc:a5:c0', ssid: 'Home Wi-Fi', band: '5g', channel: 104, signal: -18 },
], { '86:2d:bc:a5:d0': { ap: 'ap-30862dbca5ff', band: null } });
printf('swept by advert %J, tails %J, others %d\n', found.aps, found.tails, length(found.others));
printf('printable %J %J %J %J\n', rrm.printable('Home Wi-Fi'), rrm.printable(''), rrm.printable('\x00\x00'), rrm.printable('???'));

// DFS channels after a look (0120). What Linux says of them, in the words
// iw printed on the office AP on 2026-10-10: 100 and 104 free to check, 120
// with the ten-minute check Europe gives the channels weather radar uses,
// 132 checked, 136 closed for radar found there, 140 not allowed; and a
// 6 GHz channel, which is none of them.
let iw = join('\n', [
	'Band 2:',
	'\t* 5180 MHz [36] ',
	'\t  Maximum TX power: 23.0 dBm',
	'\t  Channel widths: 20MHz HT40+ VHT80 VHT160',
	'\t* 5500 MHz [100] ',
	'\t  Maximum TX power: 24.0 dBm',
	'\t  Radar detection',
	'\t  Channel widths: 20MHz HT40- HT40+ VHT80 VHT160',
	'\t  DFS state: usable (for 523480 sec)',
	'\t  DFS CAC time: 60000 ms',
	'\t* 5520 MHz [104] ',
	'\t  Radar detection',
	'\t  DFS state: usable (for 523480 sec)',
	'\t  DFS CAC time: 60000 ms',
	'\t* 5600 MHz [120] ',
	'\t  Radar detection',
	'\t  DFS state: usable (for 523480 sec)',
	'\t  DFS CAC time: 600000 ms',
	'\t* 5660 MHz [132] ',
	'\t  Radar detection',
	'\t  DFS state: available (for 20539 sec)',
	'\t  DFS CAC time: 60000 ms',
	'\t* 5680 MHz [136] ',
	'\t  Radar detection',
	'\t  DFS state: unavailable (for 12 sec)',
	'\t  DFS CAC time: 60000 ms',
	'\t* 5700 MHz [140] (disabled)',
	'\t* 5720 MHz [144] ',
	'\t  Maximum TX power: 24.0 dBm',
	'\t* 5745 MHz [149] ',
	'\t  Maximum TX power: 30.0 dBm',
	'Band 4:',
	'\t* 6215 MHz [53] ',
	'\t  Radar detection',
]);
let states = rrm.dfs_states(iw);
for (let c in sort(keys(states), (x, y) => int(x) - int(y)))
	printf('dfs state %s %J\n', c, states[c]);
printf('dfs states of nothing %J\n', rrm.dfs_states(null));
printf('clear: free to check %J, a long check %J, radar found %J, not allowed %J\n',
	rrm.clear('5g', [100, 104], states), rrm.clear('5g', [116, 120], states), rrm.clear('5g', [132, 136], states), rrm.clear('5g', [140, 144], states));
printf('clear: checked %J, no radar there %J, outside radar %J, nothing known %J\n',
	rrm.clear('5g', [132], states), rrm.clear('5g', [144], states), rrm.clear('5g', [36, 40], states), rrm.clear('5g', [100, 104], null));

// A block a radio left for a reason, or did not get to, is shut for a day,
// by its channels shared with radar: any block with one of them is barred,
// at whatever width. Channels outside radar are never shut.
let wall = 1791700000;
let shut = rrm.bar({}, '5g', [100, 104], wall - 3600);
rrm.bar(shut, '5g', [132], wall - 90000);
rrm.bar(shut, '5g', [36, 40], wall);
printf('shut %J\n', shut);
printf('barred: shut an hour ago %J, at 80 MHz %J, a day and an hour ago %J, never %J, outside radar %J, another band %J, nothing shut %J\n',
	rrm.barred(shut, '5g', [100, 104], wall), rrm.barred(shut, '5g', [100, 104, 108, 112], wall), rrm.barred(shut, '5g', [132, 136], wall),
	rrm.barred(shut, '5g', [52, 56], wall), rrm.barred(shut, '5g', [36, 40], wall), rrm.barred(shut, '2g', [100], wall), rrm.barred(null, '5g', [100], wall));

// Where a radio steps off to when it does not know where it came from.
printf('fallback %J %J %J %J %J %J\n', rrm.fallback('5g', 20, null), rrm.fallback('5g', 40, [44, 48, 100, 104]), rrm.fallback('5g', 80, [149, 153, 157, 161]),
	rrm.fallback('5g', 160, null), rrm.fallback('5g', 40, [100, 104]), rrm.fallback('2g', 20, null));

// A DFS block must be clearly the best: better than where the radio is, and
// than the best it could go to without a radar check.
let blk = (ch, cost, by, members) => ({ channel: ch, cost: cost, blotted_by: by, members: members ?? [ch, ch + 4], visits: 2 });
printf('radar reason: better than both %J\n', rrm.radar_reason(blk(36, 60, []), blk(44, 50, []), blk(100, 5, []), close, 20));
printf('radar reason: a free move does as well %J\n', rrm.radar_reason(blk(36, 60, []), blk(44, 20, []), blk(100, 5, []), close, 20));
printf('radar reason: no better than where it is %J\n', rrm.radar_reason(blk(36, 20, []), blk(36, 20, []), blk(100, 5, []), close, 20));
printf('radar reason: where it is is the best without one %J\n', rrm.radar_reason(blk(36, 60, []), blk(36, 60, []), blk(100, 5, []), close, 20));
printf('radar reason: shared, and every other is too %J\n', rrm.radar_reason(blk(36, 10, ['ap-000000000001']), blk(44, 10, ['ap-000000000001']), blk(100, 30, []), close, 20));
printf('radar reason: shared, and another is free %J\n', rrm.radar_reason(blk(36, 10, ['ap-000000000001']), blk(44, 10, []), blk(100, 5, []), close, 20));
printf('radar reason: nowhere to go without one %J\n', rrm.radar_reason(blk(36, 60, []), null, blk(100, 5, []), close, 20));
printf('radar reason: nothing %J\n', rrm.radar_reason(blk(36, 60, []), blk(44, 50, []), null, close, 20));

// Both moves at once: the one it can make now, and the one that costs a
// radar check, which the daemon makes only when the time is right.
let show = (name, w) => printf('wanted %s: plain %J, far %J\n', name, w.plain ? [w.plain.to.channel, w.plain.why] : null, w.far ? [w.far.to.channel, w.far.why] : null);
show('a DFS block clearly best', rrm.wanted(blk(36, 60, []), [blk(36, 60, []), blk(44, 50, [])], [blk(100, 5, []), blk(132, 9, [])], close, 20, null, false));
show('both', rrm.wanted(blk(36, 80, []), [blk(36, 80, []), blk(44, 50, [])], [blk(100, 5, [])], close, 20, null, false));
show('a free move is as good', rrm.wanted(blk(36, 60, []), [blk(36, 60, []), blk(44, 10, [])], [blk(100, 5, [])], close, 20, null, false));
show('the best is not DFS', rrm.wanted(blk(36, 60, []), [blk(36, 60, []), blk(44, 2, [])], [blk(100, 5, [])], close, 20, null, false));
show('interference goes at once, not to DFS', rrm.wanted(blk(36, 70, []), [blk(36, 70, []), blk(44, 30, [])], [blk(100, 25, [])], close, 20, 0.7, false));
show('just started', rrm.wanted(blk(36, 30, []), [blk(36, 30, []), blk(44, 25, [])], [blk(100, 20, [])], close, 20, null, true));
show('on a DFS block, another clearly better', rrm.wanted(blk(100, 60, []), [blk(36, 55, [])], [blk(132, 5, [])], close, 20, null, false));
show('no DFS allowed', rrm.wanted(blk(36, 60, []), [blk(36, 60, []), blk(44, 30, [])], [], close, 20, null, false));
show('nothing rated', rrm.wanted(null, [], [], close, 20, null, false));

// A planned move does not go back to a block the radio left lately (0264):
// it goes to the next best, where that is worth a move, or nowhere. A move
// for interference, or one just after a start, may.
let moves = [
	{ band: '2g', from: 6, to: 1, why: 'shared', state: 'moved', at: wall - 3600 },
	{ band: '2g', from: 11, to: 6, why: 'shared', state: 'moved', at: wall - 90000 },
	{ band: '2g', from: 1, to: 11, why: 'better', state: 'withdrawn', at: wall - 60 },
	{ band: '5g', from: 44, to: 36, why: 'better', state: 'moved', at: wall - 60 },
];
let lately = (band) => (x => rrm.left_lately(moves, band, x.members, wall));
let ch = (c, cost, by) => ({ channel: c, cost: cost, blotted_by: by, members: [c] });
printf('left lately: an hour ago %J, a day and an hour ago %J, claimed only %J, another band %J, in a block %J, nothing %J\n',
	rrm.left_lately(moves, '2g', [6], wall), rrm.left_lately(moves, '2g', [11], wall), rrm.left_lately(moves, '2g', [1], wall),
	rrm.left_lately(moves, '5g', [6], wall), rrm.left_lately(moves, '5g', [44, 48], wall), rrm.left_lately(null, '2g', [6], wall));
let two = { 'ap-000000000001': -50, 'ap-000000000002': -75 };
show('better, back where it left', rrm.wanted(ch(1, 60, ['ap-000000000002']), [ch(1, 60, ['ap-000000000002']), ch(6, 20, ['ap-000000000002']), ch(11, 70, ['ap-000000000001'])], [], two, 20, null, false, lately('2g')));
show('better, back where it left, and another better', rrm.wanted(ch(1, 60, ['ap-000000000002']), [ch(1, 60, ['ap-000000000002']), ch(6, 20, ['ap-000000000002']), ch(11, 30, ['ap-000000000002'])], [], two, 20, null, false, lately('2g')));
show('shared, back where it left', rrm.wanted(ch(1, 30, ['ap-000000000001']), [ch(1, 30, ['ap-000000000001']), ch(6, 30, ['ap-000000000002']), ch(11, 30, ['ap-000000000001'])], [], two, 20, null, false, lately('2g')));
show('better, not left', rrm.wanted(ch(1, 60, ['ap-000000000002']), [ch(1, 60, ['ap-000000000002']), ch(11, 20, ['ap-000000000002'])], [], two, 20, null, false, lately('2g')));
show('interference, back where it left', rrm.wanted(ch(1, 80, []), [ch(1, 80, []), ch(6, 20, [])], [], two, 20, 0.7, false, lately('2g')));
show('just started, back where it left', rrm.wanted(ch(1, 30, ['ap-000000000002']), [ch(1, 30, ['ap-000000000002']), ch(6, 25, ['ap-000000000002'])], [], two, 20, null, true, lately('2g')));
show('back where it left, no rule', rrm.wanted(ch(1, 60, ['ap-000000000002']), [ch(1, 60, ['ap-000000000002']), ch(6, 20, ['ap-000000000002'])], [], two, 20, null, false));
printf('blocks 40 visits %J\n', map(rrm.blocks([{ channel: 36, cost: 1, visits: 3 }, { channel: 40, cost: 1, visits: 1 }, { channel: 44, cost: 1 }, { channel: 48, cost: 1, visits: 2 }], '5g', 40), x => x.visits));

// What a hello says of a radio's load (0207), from the AP's last minute
// while it is fresh, and what is taken of a neighbour's.
let ld = { at: 1000, radios: { '5g': { radio: 'radio0', channel: 36, busy: 23.4, busy_max: 40.0, clients: 7.0, retried: 0.123 } } };
printf('load said %J, stale %J, no such band %J, nothing %J\n', rrm.load_said(ld, '5g', 1100), rrm.load_said(ld, '5g', 1300), rrm.load_said(ld, '2g', 1100), rrm.load_said(null, '5g', 1100));
printf('load heard %J, out of range %J, partly %J, junk %J\n', rrm.load_heard({ busy: 23.4, clients: 7, retried: 0.1 }), rrm.load_heard({ busy: 140, clients: -1, retried: 2 }),
	rrm.load_heard({ busy: 'lots', clients: 3 }), rrm.load_heard('busy'));

// The scan radio's spectral sweep (0245): each 20 MHz window's share of
// samples at or above the preamble level, by channel. Windows of 40 MHz or
// more, another band's, and values out of range are left out, and a sweep
// older than asked, or from ahead, gives none.
let sweep = { time: 1000, windows: [
	{ band: '5g', center: 5745, width: 20, busy_pct: 14.7 }, { band: '5g', center: 5765, width: 20, busy_pct: 33.9 },
	{ band: '5g', center: 5755, width: 40, busy_pct: 50 }, { band: '6g', center: 5975, width: 20, busy_pct: 9 },
	{ band: '5g', center: 5180, width: 20, busy_pct: 250 }, { band: '5g', center: 5200, width: 20, busy_pct: 'x' },
	{ center: 5220, width: 20, busy_pct: 4.4 },
] };
printf('energy_of fresh %J\n', rrm.energy_of(sweep, '5g', 1600, 2400));
printf('energy_of 6g %J\n', rrm.energy_of(sweep, '6g', 1600, 2400));
printf('energy_of stale %J, ahead %J, none %J\n', rrm.energy_of(sweep, '5g', 3500, 2400), rrm.energy_of(sweep, '5g', 900, 2400), rrm.energy_of(null, '5g', 1600, 2400));

// Each channel's noise level from the same sweep (0247): the middle of a
// 20 MHz window's samples', on the scan radio's own scale; a level out of
// range, or none, is left out.
let levels = { time: 1000, windows: [
	{ band: '5g', center: 5500, width: 20, noise: -107 }, { band: '5g', center: 5745, width: 20, noise: -106 },
	{ band: '5g', center: 5765, width: 20, noise: -104 }, { band: '5g', center: 5755, width: 40, noise: -100 },
	{ band: '5g', center: 5180, width: 20, noise: 5 }, { band: '5g', center: 5200, width: 20 },
] };
printf('floor_of %J, stale %J\n', rrm.floor_of(levels, '5g', 1600, 2400), rrm.floor_of(levels, '5g', 3500, 2400));

// What came of a move for the clients its radio had (0265): on it still
// on their own association, on it again on a new one, or gone. A driver
// that says no time of association has them kept.
printf('followed %J\n', rrm.followed({ 'aa:00:00:00:00:01': 100, 'aa:00:00:00:00:02': 200, 'aa:00:00:00:00:03': 300, 'aa:00:00:00:00:04': null },
	{ 'aa:00:00:00:00:01': 100, 'aa:00:00:00:00:02': 260, 'aa:00:00:00:00:04': null, 'aa:00:00:00:00:09': 400 }));
printf('followed nothing %J %J\n', rrm.followed({}, { 'aa:00:00:00:00:01': 1 }), rrm.followed({ 'aa:00:00:00:00:01': 1 }, null));

// The set the renderer writes is each whole block's primary (0243): at
// 40 MHz, 36 stands for 36 and 40 (0266). A radio may go to such a block.
let set = ['36', '44', '52', '60', '100', '108', '116', '124', '132', '140', '149', '157'];
printf('allowed 40 %J\n', rrm.allowed_of('5g', 40, set));
printf('allowed 80 %J, 20 %J, 2.4 %J, 6 %J, none %J\n', rrm.allowed_of('5g', 80, ['36', '149']), rrm.allowed_of('5g', 20, ['36', '40']),
	rrm.allowed_of('2g', 20, ['1', '6', '11']), rrm.allowed_of('6g', 80, ['5', '21']), rrm.allowed_of('5g', 40, []));
let rated = map([36, 40, 44, 48, 149, 153, 157, 161], c => ({ channel: c, cost: c == 44 || c == 48 ? 0 : 30, blotted_by: [] }));
let on = rrm.allowed_of('5g', 40, set);
let ok = filter(rrm.blocks(rated, '5g', 40), x => !length(filter(x.members, c => rrm.radar('5g', c) || (on && index(on, c) < 0))));
printf('blocks a radio at 40 may go to %J, best %J\n', map(ok, x => x.channel), rrm.best_of(ok, {})?.channel);
// 6 GHz is blocks too (0298). A radio takes up its whole block; one at 80
// or 160 MHz listens on the preferred scanning channels of its set and its
// blocks' primaries; a block is rated by those of its channels, and blotted
// out by a neighbour on any channel of it, rated or not. A neighbour on
// 5/160 takes 21/80 too, and one on 9/40 a 160 MHz block that has 9.
printf('covers 6g %J %J %J %J\n', rrm.covers('6g', 21, 80), rrm.covers('6g', 5, 160), rrm.covers('6g', 5, 20), rrm.covers('6g', 5, 320));
printf('visits 6g 160 %J\n', rrm.visits('6g', 5, false, rrm.allowed_of('6g', 160, ['5', '37']), 160));
printf('visits 6g 80 %J\n', rrm.visits('6g', 5, false, rrm.allowed_of('6g', 80, ['5', '21', '37']), 80));
let six = [{ channel: 5, cost: 10 }, { channel: 21, cost: 30 }, { channel: 37, cost: 5 }];
printf('blocks 6g 80 %J\n', map(rrm.blocks(six, '6g', 80), x => [x.channel, x.cost, x.members]));
printf('blocks 6g 160 %J\n', map(rrm.blocks(six, '6g', 160), x => [x.channel, x.cost, x.members]));
let wide = {};
for (let c in rrm.covers('6g', 5, 160))
	wide['6g ' + c] = ['ap-000000000001'];
for (let c in rrm.covers('6g', 9, 40))
	push(wide['6g ' + c], 'ap-000000000002');
printf('blocks 6g 80 used %J\n', map(rrm.blocks(six, '6g', 80, wide), x => [x.channel, x.blotted_by]));
printf('blocks 6g 160 used %J\n', map(rrm.blocks(six, '6g', 160, wide), x => [x.channel, x.blotted_by]));
printf('blocks 6g 160 unrated %J\n', map(rrm.blocks([{ channel: 37, cost: 5 }], '6g', 160), x => x.channel));

// An AP keeps its own 5 GHz and 6 GHz apart (0327): the Living Room C-360's
// 6 GHz on 5/160 (5945-6105) was 130 MHz over its 5 GHz on 157/40
// (5775-5815), and cut it from HE-MCS 6 to MCS 0; on 37/160 (6105-6265) it
// was 290 MHz over, and as before.
let lr5 = rrm.covers('5g', 157, 40), lr6 = rrm.covers('6g', 5, 160), far6 = rrm.covers('6g', 37, 160);
printf('span 5g 157/40 %J, 6g 5/160 %J, 6g 37/160 %J, 2g 6 %J, none %J\n', rrm.span('5g', lr5), rrm.span('6g', lr6), rrm.span('6g', far6), rrm.span('2g', [6]), rrm.span('5g', []));
printf('too near at %d: 6g 5/160 by 5g 157/40 %J, 6g 37/160 %J, 5g 157/40 by 6g 5/160 %J, 5g 36/40 by 6g 5/160 %J\n', rrm.APART,
	rrm.too_near('6g', lr6, '5g', lr5, rrm.APART), rrm.too_near('6g', far6, '5g', lr5, rrm.APART), rrm.too_near('5g', lr5, '6g', lr6, rrm.APART),
	rrm.too_near('5g', rrm.covers('5g', 36, 40), '6g', lr6, rrm.APART));
printf('never: at 0 %J, at 100 %J, one band %J, 2.4 GHz %J\n', rrm.too_near('6g', lr6, '5g', lr5, 0), rrm.too_near('6g', lr6, '5g', lr5, 100),
	rrm.too_near('5g', lr5, '5g', rrm.covers('5g', 149, 40), rrm.APART), rrm.too_near('2g', [11], '5g', rrm.covers('5g', 36, 40), 3000));

// A 5 GHz radio set to 160 MHz with DFS allowed starts at 80 MHz outside
// DFS, and is widened (0329): to the best 160 MHz block no neighbour uses
// that rates no worse than where it is by more than the margin. It is
// widened from 36/80, rated 30, to 36/160 at 45; not to one a neighbour
// uses, nor to one at 60; the best of two free ones; none where none is.
printf('widen: to 36/160 %J\n', rrm.widen(blk(36, 30, []), [blk(36, 45, []), blk(100, 70, [])], close, 20));
printf('widen: not among others %J\n', rrm.widen(blk(36, 30, []), [blk(36, 10, ['ap-000000000001'])], close, 20));
printf('widen: not notably worse %J\n', rrm.widen(blk(36, 30, []), [blk(36, 60, [])], close, 20));
printf('widen: the best of two %J\n', rrm.widen(blk(36, 30, []), [blk(36, 40, []), blk(100, 20, [])], close, 20)?.to?.channel);
printf('widen: none %J %J\n', rrm.widen(blk(36, 30, []), [], close, 20), rrm.widen(null, [blk(36, 10, [])], close, 20));
printf('switch to 36/160 %s\n', rrm.chan_switch(rrm.switch_args('5g', 36, 160, 'HE80', 5)));
printf('switch back to 36/80 %s\n', rrm.chan_switch(rrm.switch_args('5g', 36, 80, 'HE160', 5)));
