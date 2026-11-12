// What Aeolus's steering asks of clients and what comes of it (0138):
// TestAgentSteer runs this with the real ucode and compares what it prints
// with steer.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/steer.uc

'use strict';

import * as steer from 'aeolus.steer';

// An AP with the network test on all three bands and lab on two, as the
// daemon sees them: each BSS's network, band and own neighbour report, and
// the networks of its name on other APs.
const NR = { g2: '36862def2fd0ef4900005106070603000600', g5: '36862def2fc0ef590000742c090603012e00', g6: '30862def2fe0ef40000086c5040603030000', far: '2005b6df64e9ef19000074950906030199' + '00' };
function ap() {
	return {
		bss: {
			'phy1-ap2': { network: 'test', band: '2g', up: true, nr: NR.g2, elsewhere: [] },
			'phy0-ap2': { network: 'test', band: '5g', up: true, nr: NR.g5, elsewhere: [] },
			'phy3-ap0': { network: 'test', band: '6g', up: true, nr: NR.g6, elsewhere: [] },
			'phy1-ap1': { network: 'lab', band: '2g', up: true, nr: NR.g2, elsewhere: [NR.far] },
			'phy0-ap1': { network: 'lab', band: '5g', up: true, nr: NR.g5, elsewhere: [NR.far] },
		},
		clients: { 'phy1-ap2': {}, 'phy0-ap2': {}, 'phy3-ap0': {}, 'phy1-ap1': {}, 'phy0-ap1': {} },
	};
}

const ALL = { networks: { test: { suggest: true, hold: true, sticky: true, weak: -75 }, lab: { suggest: false, hold: false, sticky: true, weak: -75 } } };
const WALL = 1791770000;

let st, view, cfg, t;

function begin(what, c) {
	printf('\n== %s\n', what);
	st = steer.fresh();
	view = ap();
	cfg = c ?? ALL;
	t = 0;
}

// show says what the daemon is to ask, and takes it as asked.
function show(acts) {
	for (let a in acts) {
		steer.sent(st, a);
		let more = '';
		if (a.do == 'btm')
			more = sprintf(' %s%s to %s', a.imminent ? 'it is to go, ' : '', a.abridged ? 'these only' : 'these or others',
				join(' ', map(a.neighbors, n => substr(n, 0, 12) + '..' + substr(n, -6))));
		else if (a.do == 'beacon')
			more = sprintf(', mode %d, of %s, for %d TU: %s', a.mode, a.ssid, a.duration, join(' ', map(a.channels, c => `${c.op_class}/${c.channel}`)));
		else
			more = sprintf(' for %d ms', a.ms);
		printf('%4d  %s %s on %s%s\n', t, a.do, a.mac, a.bss, more);
	}
}

// put has a client on a BSS, as hostapd lists it; off takes it away.
function put(mac, ifname, signal, c) {
	for (let name in keys(view.clients))
		delete view.clients[name][mac];
	view.clients[ifname][mac] = { signal: signal, btm: true, ...(c ?? {}) };
}

function off(mac) {
	for (let name in keys(view.clients))
		delete view.clients[name][mac];
}

// look is the daemon's look at its clients, every five seconds; pass lets
// that many seconds go by, looking as it goes.
function look() {
	show(steer.tick(st, cfg, view, t, WALL + t));
}

function pass(seconds) {
	for (let end = t + seconds; t < end;) {
		t += 5;
		look();
	}
}

function hear(ifname, type, data) {
	show(steer.heard(st, cfg, view, t, ifname, type, data));
}

function records() {
	for (let e in st.events)
		printf('      %s\n', sprintf('%J', e));
	st.events = [];
}

const A = '02:00:00:00:00:0a', B = '02:00:00:00:00:0b', C = '02:00:00:00:00:0c', D = '02:00:00:00:00:0d';

begin('Suggest: a client near enough, that says it can use 5 GHz, is asked up once it has been on ten seconds, and moves');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
look();
pass(10);
hear('phy1-ap2', 'bss-transition-response', { address: A, 'status-code': 0 });
t += 2;
put(A, 'phy0-ap2', -58);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();
print('  On 5 GHz it is not asked on to 6 GHz, which it never said it can use:\n');
pass(60);
records();

begin('Suggest: one that says no is asked twice more, further apart, and then left alone');
put(B, 'phy1-ap2', -48, { bands: ['2g', '5g'] });
look();
for (let i = 0; i < 4; i++) {
	let before = length(st.events);
	for (let n = 0; n < 400 && !st.tries[B]; n++)
		pass(5);
	if (!st.tries[B]) {
		print('  and no more, in the half hour after\n');
		break;
	}
	hear('phy1-ap2', 'bss-transition-response', { address: B, 'status-code': 6 });
}
records();

begin('Suggest: one that says nothing and stays is counted as having stayed; one that is far, one that takes no request, and one never seen higher are not asked');
put(A, 'phy1-ap2', -52, { bands: ['2g', '5g'] });
put(B, 'phy1-ap2', -72, { bands: ['2g', '5g'] });
view.clients['phy1-ap2'][C] = { signal: -45, btm: false, bands: ['2g', '5g'] };
view.clients['phy1-ap2'][D] = { signal: -45, btm: true };
look();
pass(40);
records();

begin('Suggest: on 5 GHz, near, and able to use 6 GHz, a client is asked to 6 GHz; on 2.4 GHz and very near, to 6 GHz before 5');
put(A, 'phy0-ap2', -55, { bands: ['2g', '5g', '6g'] });
view.clients['phy1-ap2'][B] = { signal: -40, btm: true, bands: ['2g', '5g', '6g'] };
look();
pass(10);
off(A);
t += 1;
put(A, 'phy3-ap0', -62);
hear('phy3-ap0', 'sta-authorized', { address: A });
pass(20);
records();

begin('Hold: a client that has been on 5 GHz here is kept off 2.4 GHz for a moment when it comes to join, and joins 5 GHz');
put(A, 'phy0-ap2', -60);
look();
off(A);
pass(60);
hear('phy1-ap2', 'auth', { address: A, signal: -52 });
t += 1;
put(A, 'phy0-ap2', -60);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();
print('  It leaves; later it joins 2.4 GHz all the same, once the moment is over, and its next try is not held:\n');
off(A);
pass(400);
hear('phy1-ap2', 'auth', { address: A, signal: -52 });
t += 4;
hear('phy1-ap2', 'auth', { address: A, signal: -52 });
put(A, 'phy1-ap2', -52);
hear('phy1-ap2', 'sta-authorized', { address: A });
records();

begin('Hold: not one that is far, one never seen higher, or one that is on this AP already and comes down; not while the higher band is not up; and a probe that is only a look around leaves no record');
put(A, 'phy0-ap2', -60);
put(B, 'phy0-ap2', -60);
view.clients['phy0-ap2'][B] = { signal: -60, btm: true };
look();
off(A);
pass(60);
hear('phy1-ap2', 'auth', { address: A, signal: -80 });
// 5 GHz is not up, as while its radio listens for radar.
view.bss['phy0-ap2'].up = false;
hear('phy1-ap2', 'auth', { address: A, signal: -40 });
view.bss['phy0-ap2'].up = true;
hear('phy1-ap2', 'auth', { address: C, signal: -40 });
hear('phy1-ap2', 'auth', { address: B, signal: -40 });
hear('phy1-ap2', 'probe', { address: A, signal: -50 });
pass(30);
printf('  records: %d\n', length(st.events));
print('  A probe, and then it joins 5 GHz: that is a record:\n');
pass(30);
off(A);
look();
hear('phy1-ap2', 'probe', { address: A, signal: -50 });
t += 2;
put(A, 'phy0-ap2', -60);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();

begin('Hold: one the daemon got to late, the client let on before hostapd was asked to keep it off, is no hold and no record; a network with only Hold back on needs no neighbour report of the higher band');
cfg = { networks: { test: { suggest: false, hold: true, sticky: false, weak: -75 } } };
for (let name in keys(view.bss))
	view.bss[name].nr = null;
put(A, 'phy0-ap2', -60);
look();
off(A);
pass(60);
// What hostapd said of its authentication and of its being let on are
// both taken before anything is asked: the daemon then asks nothing.
let late = steer.heard(st, cfg, view, t, 'phy1-ap2', 'auth', { address: A, signal: -50 });
put(A, 'phy1-ap2', -50);
steer.heard(st, cfg, view, t + 0.3, 'phy1-ap2', 'sta-authorized', { address: A });
printf('  decided: %s; then let on before it was asked: records %d, counted %J\n', join(', ', map(late, a => a.do)), length(st.events), st.counts);
off(A);
pass(60);
hear('phy1-ap2', 'auth', { address: A, signal: -50 });
t += 1;
put(A, 'phy0-ap2', -60);
hear('phy0-ap2', 'sta-authorized', { address: A });
records();

begin('A client moved up that is back down within five minutes did not stay moved: said, and it is left alone for half an hour');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
look();
pass(10);
t += 2;
put(A, 'phy0-ap2', -70);
hear('phy0-ap2', 'sta-authorized', { address: A });
pass(30);
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
hear('phy1-ap2', 'sta-authorized', { address: A });
pass(600);
records();

begin('Sticky: a client weak on 5 GHz for three looks is told to go, to 2.4 GHz here first, and goes; there, though near enough, it is not asked back up');
put(A, 'phy0-ap2', -80, { bands: ['2g', '5g'] });
look();
pass(15);
t += 3;
put(A, 'phy1-ap2', -60, { bands: ['2g', '5g'] });
hear('phy1-ap2', 'sta-authorized', { address: A });
pass(900);
records();

begin('Sticky: one that takes no request is put off at once where another AP has the network, and kept off its other band here for the hold down, ten seconds unset (0289); back within the minute, it is left alone, and twice as long each time it comes back again');
view.clients['phy1-ap1'][B] = { signal: -82, btm: false };
// Another AP hears it, by its probes, though not better: it has somewhere
// to go, and is put off again after each rest (0289).
view.near = { [B]: [{ ap: 'ap-2005b6df64e0', band: '2g', middle: '05:b6:df:64', signal: -88, age: 10 }] };
look();
// A day and a half: put off at the start, after half an hour, an hour
// after that, then two, four, eight and sixteen hours.
for (let end = 36 * 3600; t < end;) {
	let was = length(st.events);
	pass(5);
	if (st.tries[B]?.asked == 'kick' && !st.on[B] && view.clients['phy1-ap1'][B]) {
		hear('phy1-ap1', 'deauth', { address: B });
		off(B);
		pass(5);
		view.clients['phy1-ap1'][B] = { signal: -82, btm: false };
		hear('phy1-ap1', 'sta-authorized', { address: B });
	}
}
printf('  put off %d times in a day and a half, at: %s\n', length(st.events), join(' ', map(st.events, e => sprintf('%dh%02d', (e.at - WALL) / 3600, ((e.at - WALL) % 3600) / 60))));
st.events = slice(st.events, 0, 1);
records();

begin('Sticky: one told to go that stays is put off after eight seconds, and is gone; one whose signal comes back is left; one with nowhere better to go is never asked');
put(A, 'phy0-ap1', -80);
view.clients['phy0-ap1'][C] = { signal: -80, btm: true };
view.clients['phy1-ap2'][D] = { signal: -85, btm: true };
look();
pass(15);
hear('phy0-ap1', 'bss-transition-response', { address: A, 'status-code': 1 });
view.clients['phy0-ap1'][C].signal = -60;
pass(5);
hear('phy0-ap1', 'deauth', { address: A });
off(A);
pass(70);
records();

begin('Sticky: one whose signal is not known when its time is up is not put off; one that has gone to another of the AP\'s networks has left this one');
put(A, 'phy0-ap1', -80);
view.clients['phy0-ap1'][B] = { signal: -80, btm: true };
look();
pass(10);
view.clients['phy0-ap1'][A].signal = null;
delete view.clients['phy0-ap1'][B];
view.clients['phy0-ap2'][B] = { signal: -50, btm: true };
hear('phy0-ap2', 'sta-authorized', { address: B });
pass(15);
records();

begin('Suggest and Sticky together: near enough on 2.4 GHz to be asked up, and weak on 5 GHz once there, a client is not sent up and down every half hour');
put(A, 'phy1-ap2', -64, { bands: ['2g', '5g'] });
let asked = [];
for (let end = 48 * 3600; t < end;) {
	t += 5;
	for (let a in steer.tick(st, cfg, view, t, WALL + t)) {
		if (a.do != 'btm' || a.mac != A)
			continue;
		// It does as it is asked, two seconds on.
		let up = a.bss == 'phy1-ap2';
		push(asked, sprintf('%s %dh%02d', up ? 'up' : 'down', t / 3600, (t % 3600) / 60));
		t += 2;
		put(A, up ? 'phy0-ap2' : 'phy1-ap2', up ? -78 : -64, { bands: ['2g', '5g'] });
		steer.heard(st, cfg, view, t, up ? 'phy0-ap2' : 'phy1-ap2', 'sta-authorized', { address: A });
	}
}
printf('  in two days it is asked: %s\n', join(', ', asked));
st.events = [];

begin('An AP\'s own test client, which says so as it joins, is not asked up, held or put off: here, and when it comes again');
put(A, 'phy1-ap2', -40, { bands: ['2g', '5g', '6g'], own: true });
view.clients['phy0-ap1'][B] = { signal: -85, btm: false, own: true };
look();
pass(60);
off(A);
pass(30);
hear('phy1-ap2', 'auth', { address: A, signal: -40 });
// What it said is taken in the moment hostapd gives it, though it is gone
// before the next look: it is known as a test client when it comes again.
steer.learned(st, D, { bands: ['2g', '5g'], own: true }, t);
hear('phy1-ap2', 'auth', { address: D, signal: -40 });
printf('  one on for a moment, known at once: asked or held %d\n', length(keys(st.tries)));
// One that does not say so, beside it, is steered as any is.
put(C, 'phy1-ap2', -40, { bands: ['2g', '5g'] });
pass(15);
printf('  records: %d, of the test client %d\n', length(st.events), length(filter(st.events, e => e.mac in [A, B])));
st.events = [];

begin('A network with nothing on is not looked at; one with only Sticky on is not asked up');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
put(B, 'phy1-ap1', -50, { bands: ['2g', '5g'] });
cfg = { networks: { lab: { suggest: false, hold: false, sticky: true, weak: -75 } } };
look();
pass(60);
hear('phy1-ap2', 'auth', { address: C, signal: -40 });
printf('  records: %d, on: %J\n', length(st.events), sort(keys(st.on)));

begin('What the agent reports');
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g'] });
look();
pass(10);
t += 2;
put(A, 'phy0-ap2', -58);
hear('phy0-ap2', 'sta-authorized', { address: A });
printf('%.J\n', steer.summary(st, cfg, view, WALL + t, WALL));

print('\n== What a client says of itself\n');
// Element 59, Supported Operating Classes, among others: the current class
// and those it can use.
function ies(...els) {
	let s = '';
	for (let e in els)
		s += chr(e[0], length(e[1]), ...e[1]);
	return s;
}
printf('2.4 and 5 GHz: %J\n', steer.bands_of(ies([0, [116, 101]], [59, [81, 81, 83, 84, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130]], [127, [0, 0, 8]])));
printf('all three: %J\n', steer.bands_of(ies([59, [131, 81, 115, 131, 132, 133, 134]])));
printf('says nothing: %J\n', steer.bands_of(ies([0, [116]], [127, [0, 0, 8]])));
printf('cut short: %J\n', steer.bands_of(substr(ies([59, [81, 115, 116]]), 0, 4)));
printf('takes a transition request: %J %J %J\n', steer.btm_of([4, 0, 8, 0]), steer.btm_of([4, 0, 0, 0]), steer.btm_of(null));
// The mark of an AP's own test client: Aeolus's OUI, type 2. Not the
// advert of its beacons (type 1), another vendor's element, or one cut short.
printf('an AP\'s own test client: %J; as the mark is written for wpa_supplicant: %s\n', steer.own_of(ies([0, [116]], [221, [0x02, 0xae, 0x01, 2, 1]], [127, [0, 0, 8]])), steer.OWN_MARK);
printf('not one: %J %J %J %J\n', steer.own_of(ies([221, [0x02, 0xae, 0x01, 1, 1]])), steer.own_of(ies([221, [0x00, 0x50, 0xf2, 2, 1]])),
	steer.own_of(substr(ies([221, [0x02, 0xae, 0x01, 2, 1]]), 0, 5)), steer.own_of(ies([0, [116]])));

begin('Suggest: not on to a band whose channel is nearly full; on to the next one up that is not (0207)');
view.busy = { '5g': 92.0, '6g': 10.0 };
put(A, 'phy1-ap2', -40, { bands: ['2g', '5g', '6g'] });
put(B, 'phy1-ap2', -60, { bands: ['2g', '5g'] });
look();
pass(15);
records();

// The network elsewhere, the least busy first (0207), by radio, as the
// neighbours' hellos say how busy each radio is; one not known after, as
// hostapd lists them. A C-360's three radios share the middle of their
// BSSIDs, each with a load of its own: it is the middle and the band that
// name one.
let listed = [
	['36:86:2d:ef:2f:c1', 'Lab', '36862def2fc1ef590000742c090603012e00'], ['20:05:b6:df:64:e9', 'Lab', '2005b6df64e9ef1900007495090603019900'],
	['36:86:2d:ef:2f:d1', 'Lab', '36862def2fd1ef4900005106070603000600'], ['30:86:2d:ef:2f:e1', 'Lab', '30862def2fe1ef40000086c5040603030000'],
	['nonsense', 'Lab', 'nonsense'],
];
let place = { '36862def2fc1': 'C-360 5 GHz', '2005b6df64e9': 'Office 5 GHz', '36862def2fd1': 'C-360 2.4 GHz', '30862def2fe1': 'C-360 6 GHz', 'nonsense': 'nonsense' };
let rrm_said = { neighbours: [
	{ ap: 'ap-30862def2fff', bands: [{ band: '2g', bssid: '36:86:2d:ef:2f:d0', load: { busy: 12 } }, { band: '5g', bssid: '36:86:2d:ef:2f:c0', load: { busy: 60.5 } },
		{ band: '6g', bssid: '30:86:2d:ef:2f:e0', load: { busy: 3 } }] },
	{ ap: 'ap-2005b6df64e0', bands: [{ band: '5g', bssid: '20:05:b6:df:64:e8', load: { busy: 30 } }, { band: '2g', bssid: null, load: { busy: 5 } }, { band: '2g', bssid: '20:05:b6:df:64:f8', load: null }] },
	{ ap: 'ap-e4d124be3a7f', bands: 'nonsense' },
] };
printf('middle %J %J; radio %J %J %J\n', steer.middle('36:86:2D:EF:2F:D1'), steer.middle('nonsense'),
	steer.radio('36:86:2D:EF:2F:D1', '2g'), steer.radio('36:86:2d:ef:2f:d1', '7g'), steer.radio('nonsense', '2g'));
let busy = steer.loads_near(rrm_said);
printf('loads near, a radio at a time: %J\n', busy);
printf('by load %J\n', map(steer.by_load(listed, busy), e => place[substr(e[2], 0, 12)] ?? e[2]));
printf('by load, none known %J\n', map(steer.by_load(listed, {}), e => place[substr(e[2], 0, 12)] ?? e[2]));
printf('loads near, none said: %J %J\n', steer.loads_near(null), steer.loads_near({ neighbours: 'nonsense' }));

// Heard better elsewhere (0289): the APs tell one another how strongly they
// hear the clients they do not serve, by their probes; near is what the
// neighbours told of each client, as aeolus-steer hands it in. FAR is the
// middle of the BSSIDs of the AP whose lab is NR.far.
const FAR = '05:b6:df:64', OTHER = 'ap-2005b6df64e0';
function near(mac, band, signal, age, m) {
	(view.near ??= {})[mac] = [...(view.near[mac] ?? []), { ap: OTHER, band: band, middle: m ?? FAR, signal: signal, age: age }];
}
const WITH = { networks: { lab: { suggest: false, hold: false, sticky: true, weak: -75, margin: 5, floor: -65, hold_down: 15 } } };

begin('Sticky, heard better (0289): a client on 5 GHz at -72 that another AP hears at -64 by its probes is told to go there at the second look, and goes', WITH);
put(A, 'phy0-ap1', -72);
near(A, '5g', -64, 4);
look();
printf('  first look: asked %d\n', length(keys(st.tries)));
pass(5);
t += 2;
off(A);
hear('phy0-ap1', 'disassoc', { address: A });
pass(10);
records();

begin('Sticky, heard better: not one heard well here, nor for a reading too old, on another band, short of the margin, or of an AP hostapd does not list', WITH);
put(A, 'phy0-ap1', -60);
near(A, '5g', -50, 2);
put(B, 'phy0-ap1', -72);
near(B, '5g', -60, 40);
view.clients['phy0-ap1'][C] = { signal: -72, btm: true };
near(C, '2g', -55, 2);
near(C, '5g', -55, 2, '05:b6:01:8c');
view.clients['phy0-ap1'][D] = { signal: -72, btm: true };
near(D, '5g', -68, 2);
view.clients['phy0-ap1']['02:00:00:00:00:0e'] = { signal: -72, btm: true };
near('02:00:00:00:00:0e', '5g', -55, 2, '86:2d:bc:a5');
pass(60);
printf('  asked %d, records %d\n', length(keys(st.tries)), length(st.events));

begin('Sticky, heard better: one that takes no request is put off at once, and kept off every band of the network here for its hold down; one told to go that stays is put off five seconds on, there too', WITH);
view.clients['phy0-ap1'][A] = { signal: -74, btm: false };
near(A, '5g', -66, 3);
put(B, 'phy0-ap1', -73);
near(B, '5g', -62, 3);
look();
pass(5);
hear('phy0-ap1', 'deauth', { address: A });
off(A);
pass(5);
hear('phy0-ap1', 'deauth', { address: B });
off(B);
pass(70);
records();

begin('Sticky, heard better: one put off that comes straight back is left alone for half an hour', WITH);
view.clients['phy0-ap1'][A] = { signal: -74, btm: false };
near(A, '5g', -60, 3);
look();
pass(5);
off(A);
pass(20);
view.clients['phy0-ap1'][A] = { signal: -74, btm: false };
hear('phy0-ap1', 'sta-authorized', { address: A });
pass(1700);
printf('  records in the half hour: %d\n', length(st.events));
records();

begin('The clients this AP hears by probe and does not serve, for its neighbours: the latest first, a band at a time, none it serves, none older than thirty seconds');
view.bss['phy0-ap1'].bssid = '36:86:2d:ef:2f:c1';
view.bss['phy1-ap1'].bssid = '36:86:2d:ef:2f:d1';
put(D, 'phy0-ap1', -50);
look();
hear('phy0-ap1', 'probe', { address: A, signal: -61 });
t += 5;
hear('phy1-ap1', 'probe', { address: A, signal: -48 });
hear('phy0-ap1', 'auth', { address: B, signal: -70.4 });
hear('phy0-ap1', 'probe', { address: D, signal: -40 });
hear('phy0-ap1', 'probe', { address: C, signal: null });
printf('  %J\n', steer.probes(st, t));
pass(30);
printf('  thirty seconds on: %J\n', steer.probes(st, t));

print('\n== Sticky\'s settings, as the renderer writes them (0289)\n');
printf('%J\n', steer.sticky_with('lab margin=8 floor=-60 hold_down=15'));
printf('unset, out of bounds, not known, and weak, which is sticky\'s own: %J\n', steer.sticky_with('lab margin=40 floor=-30 colour=3 weak=-80'));
printf('none: %J %J\n', steer.sticky_with(''), steer.sticky_with(null));

// What a client says it hears (802.11k, 0289): a weak client no other AP
// hears, that gives beacon reports, is asked of the network's places on its
// band, its own channel first; what it says, as hostapd's beacon-report
// notification has it, is a reading for the same rule. OWN is the BSSID of
// lab's 5 GHz here, FAR_BSSID that of its place on the AP whose lab is
// NR.far, which RRM knows as OTHER.
const OWN = '36:86:2d:ef:2f:c0', FAR_BSSID = '20:05:b6:df:64:e9', PASSIVE = [0x71, 0, 0, 0, 0];
function reports() {
	view.bss['phy0-ap1'].own = NR.g5;
	view.bss['phy0-ap1'].bssid = OWN;
	view.bss['phy0-ap1'].ssid = 'Lab';
	view.ap_of = steer.aps_near({ neighbours: [{ ap: OTHER, bands: [{ band: '5g', bssid: '20:05:b6:df:64:e8' }, { band: '2g', bssid: '20:05:b6:df:64:f8' }] }] });
}
let rcpi = (dbm) => (dbm + 110) * 2;
function said(ifname, mac, bssid, dbm, more) {
	show(steer.heard(st, cfg, view, t, ifname, 'beacon-report', { address: mac, 'op-class': 115, channel: 44, rcpi: rcpi(dbm), bssid: bssid, ...(more ?? {}) }));
}

// alone has the lab network on 5 GHz alone here, and its clients tried
// before by the weak rule, so that what is asked of them is what they are
// asked what they hear, and no more.
function alone(...macs) {
	delete view.bss['phy1-ap1'];
	delete view.clients['phy1-ap1'];
	for (let mac in macs)
		st.tried[mac] = t;
}

begin('Sticky, as a client says (802.11k): one weak here, under -75, that no other AP hears and gives beacon reports, is asked once it has been on five seconds, of its own channel and its places elsewhere; one at -68, good where it is, is not; asked every five minutes at first, and twice as long each time after, up to two hours', WITH);
reports();
alone(A, B);
put(A, 'phy0-ap1', -78, { rrm: PASSIVE });
view.clients['phy0-ap1'][B] = { signal: -68, btm: true, rrm: PASSIVE };
look();
let asked_at = [];
for (let end = t + 8 * 3600; t < end;) {
	t += 5;
	for (let a in steer.tick(st, cfg, view, t, WALL + t))
		if (a.do == 'beacon')
			push(asked_at, sprintf('%s %dh%02d:%02d', a.mac == A ? 'A' : 'B', t / 3600, (t % 3600) / 60, t % 60));
}
printf('  in eight hours: %s\n', join(', ', asked_at));
printf('  which RRM knows by radio: %J\n', view.ap_of);

begin('Sticky, as a client says: asked to go, it is asked every five minutes again', WITH);
reports();
alone(A);
put(A, 'phy0-ap1', -78, { rrm: PASSIVE });
look();
asked_at = [];
let ask_log = (acts) => {
	for (let a in acts)
		if (a.do == 'beacon')
			push(asked_at, sprintf('%dh%02d:%02d', t / 3600, (t % 3600) / 60, t % 60));
		else
			show([a]);
};
for (let end = t + 2000; t < end;) {
	t += 5;
	ask_log(steer.tick(st, cfg, view, t, WALL + t));
}
printf('  asked at %s\n', join(' ', asked_at));
asked_at = [];
// It says it hears the network elsewhere 12 dB better than this AP: asked
// to go, it goes; back on 5 GHz here half an hour later, weak, it is asked
// five seconds on, and then every five minutes again.
said('phy0-ap1', A, OWN, -80);
said('phy0-ap1', A, FAR_BSSID, -68);
for (let end = t + 10; t < end;) {
	t += 5;
	ask_log(steer.tick(st, cfg, view, t, WALL + t));
}
off(A);
hear('phy0-ap1', 'disassoc', { address: A });
for (let end = t + 1800; t < end;) {
	t += 5;
	ask_log(steer.tick(st, cfg, view, t, WALL + t));
}
st.tried[A] = t;
put(A, 'phy0-ap1', -78, { rrm: PASSIVE });
hear('phy0-ap1', 'sta-authorized', { address: A });
for (let end = t + 400; t < end;) {
	t += 5;
	ask_log(steer.tick(st, cfg, view, t, WALL + t));
}
printf('  after it was asked to go: asked at %s\n', join(' ', asked_at));
st.events = [];

begin('Sticky, as a client says: not one heard well here, one that gives no reports, one another AP hears by its probes, one on a network with no place elsewhere on its band, or an AP\'s own test client', WITH);
reports();
put(A, 'phy0-ap1', -60, { rrm: PASSIVE });
put(B, 'phy0-ap1', -72);
view.clients['phy0-ap1'][C] = { signal: -72, btm: true, rrm: PASSIVE };
near(C, '5g', -70, 3);
view.clients['phy0-ap1'][D] = { signal: -72, btm: true, rrm: PASSIVE, own: true };
look();
pass(30);
view.bss['phy0-ap1'].elsewhere = [];
put(A, 'phy0-ap1', -72, { rrm: PASSIVE });
pass(30);
printf('  asked: %d\n', length(keys(st.reported)));

begin('Sticky, as a client says: it hears the network elsewhere 9 dB better than this AP, as it says it hears this AP; at the second look it is told to go there, the AP RRM knows by that radio named', WITH);
reports();
put(A, 'phy0-ap1', -72, { rrm: PASSIVE });
look();
t += 5;
look();
t += 1;
said('phy0-ap1', A, OWN, -75);
said('phy0-ap1', A, FAR_BSSID, -66);
t += 4;
look();
pass(5);
off(A);
hear('phy0-ap1', 'disassoc', { address: A });
pass(10);
records();

begin('Sticky, as a client says: set against what it says of this AP, not how this AP hears it; nor a report too old, one it would not make, of a BSS that is no place, or of a client not on the network there', WITH);
reports();
put(A, 'phy0-ap1', -72, { rrm: PASSIVE });
look();
pass(5);
// It hears this AP at -64 and the place elsewhere at -61: 3 dB, though this
// AP hears it at -72.
said('phy0-ap1', A, OWN, -64);
said('phy0-ap1', A, FAR_BSSID, -61);
pass(10);
printf('  3 dB better than it hears this AP: asked to go %d\n', length(keys(st.tries)));
put(B, 'phy0-ap1', -72, { rrm: PASSIVE });
look();
said('phy0-ap1', B, FAR_BSSID, -60);
t += 31;
look();
look();
printf('  a report 31 seconds old: asked to go %d\n', length(keys(st.tries)));
said('phy0-ap1', B, FAR_BSSID, -60, { 'rep-mode': 4 });
said('phy0-ap1', B, '02:00:00:00:99:01', -50);
said('phy0-ap1', C, FAR_BSSID, -50);
said('phy1-ap1', B, FAR_BSSID, -50);
pass(10);
printf('  refused, of another BSS, of a client not here, or said on another of its bands here: asked to go %d, said of %J\n', length(keys(st.tries)), sort(keys(st.reports)));

begin('Sticky, as a client says: one whose report names a place 9 dB over how this AP hears it, and says nothing of this AP, is not moved: a client hears an AP\'s louder sending, and only what it says of this AP is set against it', WITH);
reports();
put(A, 'phy0-ap1', -72, { rrm: PASSIVE });
look();
pass(5);
said('phy0-ap1', A, FAR_BSSID, -63);
pass(20);
printf('  asked to go: %d, records: %d\n', length(keys(st.tries)), length(st.events));
print('  and once it says how it hears this AP, 9 dB under the place, it is:\n');
said('phy0-ap1', A, OWN, -72);
said('phy0-ap1', A, FAR_BSSID, -63);
pass(10);
printf('  asked to go: %d\n', length(keys(st.tries)));

begin('Sticky, as a client says: a place of an AP RRM does not know is a place all the same, and the record names no AP', WITH);
reports();
view.ap_of = {};
view.clients['phy0-ap1'][A] = { signal: -74, btm: false, rrm: PASSIVE };
look();
pass(5);
said('phy0-ap1', A, OWN, -70);
said('phy0-ap1', A, FAR_BSSID, -60);
pass(10);
hear('phy0-ap1', 'deauth', { address: A });
off(A);
pass(70);
records();
printf('  ask_channels, no own report: %J\n', steer.ask_channels({ nr: null, elsewhere: [NR.far, NR.g2] }, '5g'));

// A weak client no other AP hears (0289, 2026-11-03): well_env_switch, an
// ESP32 heard at -93 by the one AP that hears it, takes no 802.11v request
// and backs off five and a half minutes after any putting off before it
// joins again, and is weak again at once. Its asking ends as left a minute
// on; its coming back is counted all the same, and it is left alone.
const E = '02:00:00:00:00:e5';
function kicks_of(acts, list) {
	for (let a in acts)
		if (a.do == 'kick')
			push(list, sprintf('%dh%02d:%02d', t / 3600, (t % 3600) / 60, t % 60));
}
function run(seconds, list) {
	for (let end = t + seconds; t < end;) {
		t += 5;
		kicks_of(steer.tick(st, cfg, view, t, WALL + t), list);
	}
}

begin('Sticky, weak and heard by no other AP: put off once, back five and a half minutes later, weak again twelve seconds on, and not put off again, then or after its rest', WITH);
let put_offs = [];
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
look();
run(10, put_offs);
let off_at = t;
hear('phy1-ap1', 'deauth', { address: E });
off(E);
run(330, put_offs);
t = off_at + 330;
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
hear('phy1-ap1', 'sta-authorized', { address: E });
run(12, put_offs);
printf('  twelve seconds after it is back: put off %d time(s), at %s\n', length(put_offs), join(' ', put_offs));
run(300, put_offs);
run(4 * 3600, put_offs);
printf('  four hours on: put off %d time(s)\n', length(put_offs));
records();

begin('Sticky, weak, heard by another AP two hours before: put off, back, and put off again after its rest', WITH);
put_offs = [];
// Another AP heard it at the start, and not since.
view.near = { [E]: [{ ap: OTHER, band: '2g', middle: FAR, signal: -90, age: 0 }] };
look();
view.near = {};
run(5400, put_offs);
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
hear('phy1-ap1', 'sta-authorized', { address: E });
run(15, put_offs);
hear('phy1-ap1', 'deauth', { address: E });
off(E);
run(240, put_offs);
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
hear('phy1-ap1', 'sta-authorized', { address: E });
run(2400, put_offs);
printf('  put off at %s\n', join(' ', put_offs));
st.events = [];

begin('Sticky, weak and heard by no other AP: one whose place is a lower band here is not held back by its coming back before', WITH);
put_offs = [];
st.came_back[E] = t;
view.clients['phy0-ap1'][E] = { signal: -80, btm: false };
look();
run(10, put_offs);
hear('phy0-ap1', 'deauth', { address: E });
off(E);
t += 3;
view.clients['phy1-ap1'][E] = { signal: -66, btm: false };
hear('phy1-ap1', 'sta-authorized', { address: E });
printf('  put off %d time(s)\n', length(put_offs));
records();

begin('Sticky, like the TV: weak, heard by no other AP, come back once before, it still goes by what it says (802.11k), which the weak rule waits for', WITH);
reports();
st.came_back[A] = t;
put(A, 'phy0-ap1', -80, { rrm: PASSIVE });
look();
pass(5);
t += 1;
said('phy0-ap1', A, OWN, -80);
said('phy0-ap1', A, FAR_BSSID, -68);
t += 4;
look();
pass(5);
off(A);
hear('phy0-ap1', 'disassoc', { address: A });
pass(10);
records();
print('  one that says nothing is put off for its weak signal, the moment after:\n');
put(B, 'phy0-ap1', -80, { rrm: PASSIVE });
look();
pass(15);

// What the daemon keeps from one run to the next (0289): a lone client that
// came back before an update of the agent, which starts the daemon again,
// is not put off after it. The daemon's own clock starts again at nought;
// the calendar's runs on.
begin('Sticky, across a restart: a lone client that came back before it is not put off after it, its rest kept too', WITH);
put_offs = [];
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
look();
run(10, put_offs);
hear('phy1-ap1', 'deauth', { address: E });
off(E);
t += 330;
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
hear('phy1-ap1', 'sta-authorized', { address: E });
near('02:00:00:00:00:f0', '5g', -60, 3);
look();
let saved = json(sprintf('%J', steer.kept(st, WALL, t)));
printf('  kept: came back %J, heard elsewhere %J, left alone until %J\n', map(keys(saved.came_back), m => saved.came_back[m] - WALL),
	map(keys(saved.elsewhere), m => [m, saved.elsewhere[m] - WALL]), map(keys(saved.rest), m => saved.rest[m] - WALL));
// The daemon starts again: its clock at nought, the calendar's where it was.
let was = WALL + t;
st = steer.fresh();
view = ap();
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
t = 0;
steer.restore(st, saved, was, 0);
let again = [];
for (let end = t + 4 * 3600; t < end;) {
	t += 5;
	kicks_of(steer.tick(st, cfg, view, t, was + t), again);
}
printf('  after the restart, four hours: put off %d time(s)\n', length(again));
print('  without what it kept, the first look after the restart would put it off again:\n');
st = steer.fresh();
steer.restore(st, { v: 2, known: saved.known }, was, 0);
let forgot = [];
t = 0;
for (let end = t + 60; t < end;) {
	t += 5;
	kicks_of(steer.tick(st, cfg, view, t, was + t), forgot);
}
printf('  put off %d time(s)\n', length(forgot));

// How often it came back is kept too (2026-11-05): without it, each start
// of the daemon, as after an update of the agent or a change of its
// config, left a client that keeps coming back alone half an hour, not
// twice as long each time. An ESP on the Shed was put off 8 times a
// day. As above, a day and a half, now with the daemon started again from
// what it kept each time the client comes back.
begin('Sticky, across restarts: one that keeps coming back is left alone twice as long each time, though the daemon starts again after each time');
view.clients['phy1-ap1'][B] = { signal: -82, btm: false };
view.near = { [B]: [{ ap: 'ap-2005b6df64e0', band: '2g', middle: '05:b6:df:64', signal: -88, age: 10 }] };
let base = WALL, offs = [], restarts = 0;
let step = () => {
	t += 5;
	for (let a in steer.tick(st, cfg, view, t, base + t))
		steer.sent(st, a);
};
while (base + t - WALL < 36 * 3600) {
	step();
	if (st.tries[B]?.asked == 'kick' && !st.on[B] && view.clients['phy1-ap1'][B]) {
		push(offs, sprintf('%dh%02d', (base + t - WALL) / 3600, ((base + t - WALL) % 3600) / 60));
		hear('phy1-ap1', 'deauth', { address: B });
		off(B);
		step();
		view.clients['phy1-ap1'][B] = { signal: -82, btm: false };
		hear('phy1-ap1', 'sta-authorized', { address: B });
		// The daemon starts again, from what it kept.
		let k = json(sprintf('%J', steer.kept(st, base, t)));
		base += t;
		t = 0;
		st = steer.fresh();
		steer.restore(st, k, base, 0);
		restarts++;
	}
}
printf('  put off %d times in a day and a half, started again %d times, at: %s\n', length(offs), restarts, join(' ', offs));

print('\n== What a run before kept, read back: a file from before 0289, and what is past its time or not well said\n');
st = steer.fresh();
steer.restore(st, { [A]: { bands: { '2g': WALL - 10, '5g': WALL - 5, '7g': WALL }, btm: true, at: WALL - 5 } }, WALL, 0);
printf('  from before: %J\n', st.known);
st = steer.fresh();
steer.restore(st, { v: 2, known: {},
	came_back: { [A]: WALL - 3600, [B]: WALL - 8 * 86400, [C]: WALL + 60, nonsense: WALL - 10, [D]: 'soon' },
	elsewhere: { [A]: WALL - 7200, [B]: WALL - 2 * 86400 },
	rest: { [A]: WALL + 900, [B]: WALL - 1, [C]: WALL + 2 * 86400 } }, WALL, 0);
printf('  came back %J, heard elsewhere %J, left alone until %J\n', st.came_back, st.elsewhere, st.rest);
steer.restore(st, null, WALL, 0);
steer.restore(st, 'nonsense', WALL, 0);
printf('  nothing, or not a file: as it was, %d\n', length(st.came_back));
st = steer.fresh();
steer.restore(st, { v: 2, known: {},
	backs: { [A]: [3, WALL - 3600], [B]: [2, WALL - 8 * 86400], [C]: [0, WALL - 10], [D]: [65, WALL - 10], nonsense: [1, WALL - 10] },
	downs: { [A]: [1, WALL + 60], [B]: [1, WALL - 60], [C]: '1', [D]: [1] },
	down: { [A]: WALL + 600, [B]: WALL - 1, [C]: WALL + 2 * 86400 } }, WALL, 0);
printf('  came back how often %J, sent down how often %J, left down until %J\n', st.backs, st.downs, st.down);

// It is the try that counts (0289, 2026-11-03): put off by the weak rule
// at 01:36:38, well_env_switch was back at 01:47:06, ten and a half minutes
// on, past the ten that count as coming back, and was put off again at
// 01:47:18 as a first try.
begin('Sticky, weak and heard by no other AP, back ten and a half minutes after its try: not put off again, then, five minutes or an hour on, or after a restart; put off once another AP hears it', WITH);
put_offs = [];
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
look();
run(10, put_offs);
off_at = t;
hear('phy1-ap1', 'deauth', { address: E });
off(E);
run(630, put_offs);
t = off_at + 630;
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
hear('phy1-ap1', 'sta-authorized', { address: E });
run(15, put_offs);
printf('  back ten and a half minutes on, fifteen seconds after: put off %d time(s), at %s\n', length(put_offs), join(' ', put_offs));
run(300, put_offs);
printf('  five minutes on: %d\n', length(put_offs));
run(3600, put_offs);
printf('  an hour on: %d\n', length(put_offs));
records();
saved = json(sprintf('%J', steer.kept(st, WALL, t)));
printf('  kept: tried %J\n', map(keys(saved.tried), m => saved.tried[m] - WALL));
was = WALL + t;
st = steer.fresh();
view = ap();
view.clients['phy1-ap1'][E] = { signal: -93, btm: false };
t = 0;
steer.restore(st, saved, was, 0);
again = [];
for (let end = t + 3600; t < end;) {
	t += 5;
	kicks_of(steer.tick(st, cfg, view, t, was + t), again);
}
printf('  after a restart from what was kept, an hour: put off %d time(s)\n', length(again));
view.near = { [E]: [{ ap: OTHER, band: '2g', middle: FAR, signal: -90, age: 2 }] };
for (let end = t + 30; t < end;) {
	t += 5;
	kicks_of(steer.tick(st, cfg, view, t, was + t), again);
}
printf('  another AP hears it: put off %d time(s)\n', length(again));

// Community sharing (0299): what the other APs said their clients can use
// is known here; what this AP's own clients said is told on, and no more;
// and band steering reaches another AP's higher band, where the client
// says it hears it well.
const FAR6 = '2005b6df64ea' + 'ef190000' + '83' + '25' + '0e', FAR6_BSSID = '20:05:b6:df:64:ea';
const SHARING = { networks: { lab: { suggest: true, hold: false, sticky: false, community: true, weak: -75 } } };
function across_ap() {
	reports();
	view.bss['phy0-ap1'].elsewhere = [NR.far, FAR6];
	view.ap_of = steer.aps_near({ neighbours: [{ ap: OTHER, bands: [{ band: '5g', bssid: '20:05:b6:df:64:e8' }, { band: '6g', bssid: FAR6_BSSID }] }] });
}
let known_of = (mac) => {
	let k = st.known[mac];
	return k ? { bands: k.bands, at: k.at } : null;
};

begin('Community sharing: what the other APs said their clients can use is known here from when it was said, a band known later kept, a bad reading passed over', SHARING);
t = 100;
st.known[B] = { bands: { '5g': 95 }, at: 95 };
steer.told(st, { [A]: [{ ap: OTHER, bands: ['2g', '5g', '6g'], age: 30 }], [B]: [{ ap: OTHER, bands: ['5g', '7g'], age: 10 }, { ap: OTHER, bands: ['6g'], age: -3 }] }, t);
printf('  A, said 30 s ago at another AP: %J\n', known_of(A));
printf('  B, on 5 GHz here 5 s ago, told of 5 GHz 10 s ago: %J\n', known_of(B));

begin('Community sharing: told on are this AP\'s clients that said, as they joined here, they can use a band above 2.4 GHz, the latest joined first; not one that said 2.4 GHz alone, nor one gone', SHARING);
put(A, 'phy0-ap1', -50);
look();
pass(5);
view.clients['phy1-ap1'][B] = { signal: -60, btm: true };
look();
let said_here = { [A]: { bss: 'phy0-ap1', bands: ['2g', '5g', '6g'] }, [B]: { bss: 'phy1-ap1', bands: ['2g'] }, [C]: { bss: 'phy0-ap1', bands: ['2g', '5g'] } };
printf('  %J\n', steer.shares(st, cfg, said_here, t));
said_here[B].bands = ['2g', '5g'];
off(A);
look();
printf('  B said 2.4 and 5 GHz, A gone: %J\n', steer.shares(st, cfg, said_here, t));
printf('  on a network without it: %J\n', steer.shares(st, { networks: { lab: { suggest: true } } }, said_here, t));

begin('Community sharing, across: a client on 5 GHz that can use 6 GHz, here where the network has no 6 GHz, is asked what it hears of the network\'s 6 GHz at other APs; it says -48, and is sent there', SHARING);
across_ap();
put(A, 'phy0-ap1', -67, { bands: ['2g', '5g', '6g'], rrm: PASSIVE });
look();
pass(10);
said('phy0-ap1', A, FAR6_BSSID, -48, { 'op-class': 131, channel: 37 });
pass(5);
hear('phy0-ap1', 'bss-transition-response', { address: A, 'status-code': 0 });
t += 2;
off(A);
hear('phy0-ap1', 'disassoc', { address: A });
pass(25);
records();

begin('Community sharing, across: one that says it hears the other AP\'s 6 GHz at -70 stays, and is asked again two and then ten minutes on, and no more', SHARING);
across_ap();
put(A, 'phy0-ap1', -67, { bands: ['2g', '5g', '6g'], rrm: PASSIVE });
look();
for (let end = t + 1800; t < end;) {
	t += 5;
	let acts = steer.tick(st, cfg, view, t, WALL + t);
	show(acts);
	for (let a in acts)
		if (a.do == 'beacon')
			said('phy0-ap1', A, FAR6_BSSID, -70, { 'op-class': 131, channel: 37 });
}
records();

begin('Community sharing, across: not with community sharing off; not for a client that never said it can use 6 GHz; not one that gives no beacon reports', { networks: { lab: { ...SHARING.networks.lab, community: false } } });
across_ap();
put(A, 'phy0-ap1', -67, { bands: ['2g', '5g', '6g'], rrm: PASSIVE });
look();
pass(60);
off(A);
look();
cfg = SHARING;
put(B, 'phy0-ap1', -67, { bands: ['2g', '5g'], rrm: PASSIVE });
view.clients['phy0-ap1'][C] = { signal: -67, btm: true, bands: ['2g', '5g', '6g'] };
look();
pass(60);
print('  (nothing asked)\n');

// Keep on 6 GHz (0300): a client on 6 GHz here, heard well, is kept off the
// network's lower bands on this AP beforehand, by bans renewed while it is
// so, since by Fast Transition it is on 5 GHz before a ban sent on its asking
// gets there. Let go once weak two looks running, gone from 6 GHz, or the
// switch is off. Nothing else is ever kept: not a client on 2.4 or 5 GHz,
// not one heard weakly on 6 GHz, not an AP's own test client, not on a
// network without it.
const KEEP = { networks: { test: { suggest: false, hold: false, sticky: false, keep: true, weak: -75 } } };

begin('Keep on 6 GHz: a client on 6 GHz here at -60 is kept off 5 GHz and 2.4 GHz here beforehand, for 15 s, renewed while it is so; heard under -72 once it stays kept, twice running it is let go there; kept again, the switch turned off lets it go', KEEP);
put(A, 'phy3-ap0', -60, { bands: ['2g', '5g', '6g'] });
look();
pass(20);
put(A, 'phy3-ap0', -74, { bands: ['2g', '5g', '6g'] });
pass(5);
print('  (heard at -74 once: still kept)\n');
put(A, 'phy3-ap0', -70, { bands: ['2g', '5g', '6g'] });
pass(5);
put(A, 'phy3-ap0', -75, { bands: ['2g', '5g', '6g'] });
pass(10);
print('  (heard at -75 twice running: let go)\n');
records();
pass(10);
print('  (still weak: not kept again)\n');
put(A, 'phy3-ap0', -58, { bands: ['2g', '5g', '6g'] });
pass(5);
cfg = { networks: { test: { ...KEEP.networks.test, keep: false } } };
pass(5);
print('  (the switch off: let go)\n');
records();
printf('  counted %J\n', st.counts.test.keep);

begin('Keep on 6 GHz: a kept client gone from 6 GHz here is let go at once, the lower bands open to it again; one found on a lower band, as where a ban ran out with the daemon held up, is asked nothing there', KEEP);
put(A, 'phy3-ap0', -55, { bands: ['2g', '5g', '6g'] });
look();
t += 30;
hear('phy3-ap0', 'disassoc', { address: A });
off(A);
pass(10);
print('  (gone: nothing more)\n');
records();
put(A, 'phy3-ap0', -55, { bands: ['2g', '5g', '6g'] });
pass(5);
hear('phy0-ap2', 'sta-authorized', { address: A });
put(A, 'phy0-ap2', -45, { bands: ['2g', '5g', '6g'] });
hear('phy3-ap0', 'disassoc', { address: A });
print('  (on 5 GHz now: only 2.4 GHz is let go, 5 GHz asked nothing)\n');
pass(10);
records();

begin('Keep on 6 GHz: radios restarted under new names, as after a driver swap, took hostapd\'s bans with them: the kept client is kept off the new ones, and let go, nothing is asked of the ones gone', KEEP);
put(A, 'phy3-ap0', -55, { bands: ['2g', '5g', '6g'] });
look();
for (let was, now_ in { 'phy1-ap2': 'phy9-ap2', 'phy0-ap2': 'phy8-ap2' }) {
	view.bss[now_] = { ...view.bss[was] };
	view.clients[now_] = {};
	delete view.bss[was];
	delete view.clients[was];
}
print('  (2.4 and 5 GHz back as phy9-ap2 and phy8-ap2)\n');
pass(5);
t += 30;
hear('phy3-ap0', 'disassoc', { address: A });
off(A);
pass(10);
print('  (gone: only phy8-ap2 and phy9-ap2 let go)\n');
records();

begin('Keep on 6 GHz never strands a client that is not on 6 GHz: one on 2.4 GHz alone, one on 5 GHz, one on 6 GHz heard at -75, an AP\'s own test client on 6 GHz, and one on a network without it are none of them kept', KEEP);
view.clients['phy1-ap2'][B] = { signal: -50, btm: false, bands: ['2g'] };
put(C, 'phy0-ap2', -50, { bands: ['2g', '5g', '6g'] });
put(D, 'phy3-ap0', -75, { bands: ['2g', '5g', '6g'] });
view.clients['phy3-ap0'][E] = { signal: -45, btm: true, bands: ['2g', '5g', '6g'], own: true };
look();
pass(10);
hear('phy1-ap2', 'probe', { address: B });
hear('phy1-ap2', 'auth', { address: B });
hear('phy1-ap2', 'auth', { address: C });
hear('phy0-ap2', 'auth', { address: D });
cfg = { networks: { test: { ...KEEP.networks.test, keep: false } } };
put(A, 'phy3-ap0', -55, { bands: ['2g', '5g', '6g'] });
pass(10);
print('  (nothing kept)\n');
records();

// Where a client belongs (0319): a person's rule, as the renderer writes it
// and the daemon reads it. SELF is this AP; OTHER is the AP whose lab is
// NR.far, by RRM's word on that radio.
const SELF = 'ap-30862def2fff';
function homes(rules) {
	let out = {};
	for (let e in rules) {
		let h = steer.home_of(e);
		if (h)
			out[h[0]] = h[1];
	}
	return out;
}
function homed(rules, nets) {
	view.ap_of = steer.aps_near({ neighbours: [{ ap: OTHER, bands: [{ band: '5g', bssid: '20:05:b6:df:64:e8' }] }] });
	cfg = { networks: nets ?? { lab: { suggest: false, hold: false, sticky: false }, test: { suggest: true, hold: true, sticky: false } }, homes: homes(rules) };
}

print('\n== Where a client belongs, as the renderer writes it (0319)\n');
printf('%J\n', steer.home_of(`${A} aps=${OTHER} hold=20`));
printf('%J\n', steer.home_of(`${B} here aps=${SELF},${OTHER} bands=5g,6g hold=30`));
printf('not well said, each none: %J\n', map([`${A}`, `${A} aps=`, `${A} aps=ap-1 hold=30`, `${A} aps=${OTHER} bands=7g`, `${A} aps=${OTHER} hold=0`, `${A} aps=${OTHER} hold=301`, `${A} aps=${OTHER} colour=3`, `nonsense aps=${OTHER}`, '', null], e => steer.home_of(e)));

begin('Home (0319): a client on lab 5 GHz whose rule names the other AP is told to go there once it has been on ten seconds, however well it is heard here; it stays, and is put off five seconds on, kept off both of the network\'s bands here for its rule\'s hold');
homed([`${A} aps=${OTHER} hold=20`]);
put(A, 'phy0-ap1', -45);
look();
pass(5);
printf('  asked at five seconds: %d\n', length(keys(st.tries)));
pass(10);
hear('phy0-ap1', 'deauth', { address: A });
off(A);
pass(70);
records();

begin('Home: one that takes no request is put off at once; one that comes straight back is left alone for half an hour, then is sent again');
homed([`${A} aps=${OTHER} hold=20`]);
view.clients['phy0-ap1'][A] = { signal: -45, btm: false };
look();
pass(10);
off(A);
pass(20);
view.clients['phy0-ap1'][A] = { signal: -45, btm: false };
hear('phy0-ap1', 'sta-authorized', { address: A });
pass(1795);
printf('  records in the half hour: %d\n', length(st.events));
pass(30);
records();

begin('Home: one on the AP it belongs on but a band it does not is sent to the band it belongs on here, kept off only the band it was on; one already there is asked nothing');
homed([`${A} here aps=${SELF} bands=5g`, `${B} here aps=${SELF} bands=5g`]);
put(A, 'phy1-ap2', -50, { bands: ['2g', '5g', '6g'] });
put(B, 'phy0-ap2', -50, { bands: ['2g', '5g', '6g'] });
look();
pass(15);
hear('phy1-ap2', 'deauth', { address: A });
off(A);
t += 2;
put(A, 'phy0-ap2', -55, { bands: ['2g', '5g', '6g'] });
hear('phy0-ap2', 'sta-authorized', { address: A });
pass(10);
records();

begin('Home never strands a client: one only ever seen on 2.4 GHz, whose home has 5 GHz alone, or whose rule names bands it was never seen to use here, is left where it is; so is one whose home AP RRM does not know, and an AP\'s own test client');
homed([`${A} aps=${OTHER}`, `${B} here aps=${SELF} bands=6g`, `${C} aps=ap-0123456789ab`, `${D} aps=${OTHER}`]);
view.clients['phy1-ap1'][A] = { signal: -50, btm: true, bands: ['2g'] };
view.clients['phy1-ap2'][B] = { signal: -50, btm: true, bands: ['2g', '5g'] };
view.clients['phy0-ap1'][C] = { signal: -50, btm: true };
view.clients['phy0-ap1'][D] = { signal: -50, btm: true, own: true };
look();
pass(120);
printf('  asked %d, records %d\n', length(keys(st.tries)), length(st.events));

begin('Home keeps the other ways within a rule: suggest asks no client up off the 2.4 GHz it belongs on, hold back holds it nowhere, sticky sends none away from the AP it belongs on, and keep on 6 GHz keeps none that belongs on 5 GHz');
homed([`${A} here aps=${SELF} bands=2g`, `${B} here aps=${SELF}`, `${C} here aps=${SELF} bands=5g`],
	{ test: { suggest: true, hold: true, sticky: false, keep: true }, lab: { suggest: false, hold: false, sticky: true, weak: -75, margin: 5, floor: -65, hold_down: 15 } });
put(A, 'phy1-ap2', -40, { bands: ['2g', '5g', '6g'] });
put(B, 'phy0-ap1', -72);
near(B, '5g', -60, 3);
look();
pass(30);
off(A);
hear('phy1-ap2', 'probe', { address: A, signal: -40 });
put(C, 'phy3-ap0', -50, { bands: ['2g', '5g', '6g'] });
pass(5);
printf('  asked %d, kept %d\n', length(keys(st.tries)), length(keys(st.kept)));
records();

// Wherever it is heard best (0321): each AP's scan radio hears the clients
// on its channel, set against its own radios by the clients it serves, and
// what it hears of a client with a rule of the nearest is a reading for the
// AP that client is on. This AP's lab 5 GHz is 36:86:2d:ef:2f:c1 on 44
// (5220 MHz); OTHER's, which NR.far names, is on 149 (5745 MHz).
const S1 = '02:00:00:00:00:31', S2 = '02:00:00:00:00:32', S3 = '02:00:00:00:00:33', TV = '02:00:00:00:00:3f', LAMP = '02:00:00:00:00:3e';
function heard_by_scan(list) {
	return { time: WALL + t, list: map(list, x => ({ mac: x[0], bssid: x[1], freq: x[2], signal: x[3], last_seen: WALL + t - (x[4] ?? 1), frames: 50 })) };
}

print('\n== Rules of the nearest, as the renderer writes them (0321)\n');
printf('%J\n', steer.home_of(`${TV} nearest hold=30`));
printf('%J\n', steer.home_of(`${TV} nearest aps=${SELF},${OTHER} bands=5g hold=20`));

begin('Nearest (0321): the scan radio is set against this AP\'s radio by three clients it serves, at the middle of what each says; then what it hears of a client with a rule of the nearest, on the AP it is on\'s own channel, is a reading as this AP\'s radio would hear it');
cfg = { networks: ALL.networks, homes: homes([`${TV} nearest`]) };
view.bss['phy0-ap1'].bssid = '36:86:2d:ef:2f:c1';
view.chan_near = steer.chans_near({ neighbours: [{ ap: OTHER, bands: [{ band: '5g', bssid: '20:05:b6:df:64:e8', channel: 149 }] }] });
put(S1, 'phy0-ap1', -50);
view.clients['phy0-ap1'][S2] = { signal: -55, btm: true };
view.clients['phy0-ap1'][S3] = { signal: -60, btm: true };
look();
const MINE5 = '36:86:2d:ef:2f:c1', THEIRS5 = '20:05:b6:df:64:e9';
steer.scanned(st, view, heard_by_scan([[S1, MINE5, 5220, -60], [S2, MINE5, 5220, -66], [TV, THEIRS5, 5745, -72]]), WALL + t, t, cfg.homes);
printf('  two clients set: offset %J, readings %J\n', steer.offset(st, '5g', t), steer.probes(st, t));
view.clients['phy0-ap1']['02:00:00:00:00:34'] = { signal: -50, btm: true };
steer.scanned(st, view, heard_by_scan([[S3, MINE5, 5220, -69, 20], ['02:00:00:00:00:34', MINE5, 5220, -90, 31], [S1, MINE5, 5240, -90], [TV, THEIRS5, 5745, -72],
	[LAMP, THEIRS5, 5745, -60], [TV, THEIRS5, 5765, -50]]), WALL + t, t, cfg.homes);
printf('  three, one heard 20 s before, as airscan writes every 12 s or so (one on another channel, and one 31 s old, left out): offset %J\n', steer.offset(st, '5g', t));
printf('  readings: %J\n', steer.probes(st, t));
t += 31;
look();
printf('  thirty-one seconds on: %J\n', steer.probes(st, t));

begin('Nearest: a client on lab 5 GHz at -70, with a rule of the nearest, that the other AP hears at -62 is told to go there at the second look, once settled; it stays, and is put off five seconds on, kept off both bands here for its rule\'s hold');
homed([`${TV} nearest`]);
put(TV, 'phy0-ap1', -70);
look();
near(TV, '5g', -62, 2);
pass(15);
off(TV);
hear('phy0-ap1', 'deauth', { address: TV });
pass(70);
records();

begin('Nearest: not for one the other AP hears 4 dB better, nor for a reading 31 seconds old, nor to an AP its rule leaves out, nor for one with no rule of the nearest');
homed([`${TV} nearest`, `${LAMP} nearest aps=${SELF}`]);
put(TV, 'phy0-ap1', -70);
view.clients['phy0-ap1'][LAMP] = { signal: -70, btm: true };
view.clients['phy0-ap1'][S1] = { signal: -70, btm: true };
look();
near(TV, '5g', -66, 2);
near(LAMP, '5g', -55, 2);
near(S1, '5g', -55, 2);
pass(30);
view.near = {};
near(TV, '5g', -55, 31);
pass(10);
printf('  asked %d, records %d\n', length(keys(st.tries)), length(st.events));

begin('Nearest, where no other AP hears it (0321): on lab 2.4 GHz at -76, no AP\'s reading of it on its band for two minutes, it is put off to choose again, every band here kept off for its rule\'s hold; at -70 it is left; a fresh reading on its band keeps it to the comparison');
homed([`${TV} nearest`, `${LAMP} nearest`, `${S1} nearest`]);
view.clients['phy1-ap1'][TV] = { signal: -76, btm: false };
view.clients['phy1-ap1'][LAMP] = { signal: -70, btm: false };
view.clients['phy1-ap1'][S1] = { signal: -76, btm: false };
look();
pass(115);
printf('  at 115 s: asked %d\n', length(keys(st.tries)));
for (let i = 0; i < 6; i++) {
	near(S1, '2g', -74, 1);
	pass(5);
}
records();

// Stopped after two (Griff, 2026-11-05: "stop the nearest rule after two"):
// the garage door opener, an ESP, came back to the C260 31 s after each of
// four puttings off by its rule of the nearest, and never tried the AP that
// heard it 11 dB better.
begin('Nearest, stopped after two: one that takes no request, heard 8 dB better by the other AP, comes back after each putting off; after the second its rule stops here, kept across a restart, until the rule changes; a rule naming an AP does not stop');
homed([`${TV} nearest`]);
view.clients['phy0-ap1'][TV] = { signal: -70, btm: false };
near(TV, '5g', -62, 2);
// It comes back once its rule's hold is up, as the opener did.
let back_each = (mac, bss, signal, list, hours) => {
	for (let end = t + hours * 3600; t < end;) {
		run(5, list);
		if (st.tries[mac]?.asked == 'kick' && !st.on[mac] && view.clients[bss][mac]) {
			off(mac);
			run(35, list);
			view.clients[bss][mac] = { signal: signal, btm: false };
			hear(bss, 'sta-authorized', { address: mac });
		}
	}
};
let opener = [];
back_each(TV, 'phy0-ap1', -70, opener, 6);
printf('  put off in six hours at: %s; stopped here: %J\n', join(' ', opener), st.stopped[TV] != null);
for (let e in st.events)
	if (e.stopped)
		printf('      %J\n', e);
st.events = [];
let saved_stop = json(sprintf('%J', steer.kept(st, WALL, t)));
let st_again = steer.fresh();
steer.restore(st_again, saved_stop, WALL, t);
printf('  kept across a restart: %J\n', st_again.stopped[TV] != null);
homed([`${TV} nearest hold=20`]);
let changed = [];
back_each(TV, 'phy0-ap1', -70, changed, 2);
printf('  its rule changed (hold=20): put off again at %s\n', length(changed) ? changed[0] : 'never');
st = steer.fresh();
homed([`${A} aps=${OTHER} hold=20`]);
view.clients['phy0-ap1'][A] = { signal: -45, btm: false };
let fixed = [];
back_each(A, 'phy0-ap1', -45, fixed, 6);
printf('  a rule naming an AP, six hours: put off %d times\n', length(fixed));

// What a scan radio hears, for sticky (0324): where a network weighs the
// scan radios, the readings are of every client heard on its AP's own
// channel, and the AP a client is on takes them as it takes probes.
function near_scan(mac, band, signal, age, m) {
	(view.near ??= {})[mac] = [...(view.near[mac] ?? []), { ap: OTHER, band: band, middle: m ?? FAR, signal: signal, age: age, kind: 'scan' }];
}

begin('Sticky on the scan radios (0324): where a network here weighs them, the scan radio\'s readings are of every client it hears on its AP\'s own channel, a rule of the nearest\'s first, then the most strongly heard; where none does, of the rule\'s clients alone');
cfg = { networks: ALL.networks, homes: homes([`${TV} nearest`]) };
view.bss['phy0-ap1'].bssid = '36:86:2d:ef:2f:c1';
view.chan_near = steer.chans_near({ neighbours: [{ ap: OTHER, bands: [{ band: '5g', bssid: '20:05:b6:df:64:e8', channel: 149 }] }] });
put(S1, 'phy0-ap1', -50);
view.clients['phy0-ap1'][S2] = { signal: -55, btm: true };
view.clients['phy0-ap1'][S3] = { signal: -60, btm: true };
look();
let heard5 = [[S1, MINE5, 5220, -60], [S2, MINE5, 5220, -66], [S3, MINE5, 5220, -69], [TV, THEIRS5, 5745, -80], [LAMP, THEIRS5, 5745, -60], ['02:00:00:00:00:3d', THEIRS5, 5745, -75]];
steer.scanned(st, view, heard_by_scan(heard5), WALL + t, t, cfg.homes, false);
printf('  no network weighs them: %J\n', map(steer.probes(st, t), r => [r[0], r[3]]));
steer.scanned(st, view, heard_by_scan(heard5), WALL + t, t, cfg.homes, true);
printf('  one does: %J\n', map(steer.probes(st, t), r => [r[0], r[3]]));

begin('Sticky on the scan radios: a client on lab 5 GHz at -72 that only a scan radio hears 9 dB better elsewhere is moved, where lab weighs the scan radios, at the second look; not where it does not; nor does a scan reading count as heard elsewhere for the weak rule', WITH);
put(A, 'phy0-ap1', -72);
near_scan(A, '5g', -63, 3);
pass(15);
printf('  lab not weighing them: asked %d, heard elsewhere %J\n', length(keys(st.tries)), st.elsewhere[A] != null);
cfg = { networks: { lab: { ...WITH.networks.lab, scan: true } } };
pass(10);
records();
