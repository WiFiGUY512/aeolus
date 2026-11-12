// The page is kept quiet (Griff, 2026-10-11): a heading is a name and a
// count, a line is a name, where and a state, and what a thing means is
// said on hover or once it is opened.
//
// The Assurance tab of a Locations folder (0123): the service tried from a
// client's side. The APs here that have a scan radio are asked for a client
// test (0121) on a schedule, each time another network, AP and band, and
// any AP for one over the wire (0128), where the folder says so; what
// each thing tested comes to is a line, the failing first, that opens to
// its latest test; and the latest tests follow, newest first. Before the
// tests, how the clients' own attempts to come online went (0127): what
// every AP records, a scan radio or none.

import { h, link } from '../dom.js';
import { get, post, schema } from '../api.js';
import { ago, bandName, value } from '../format.js';
import { flash, hurry, redrawNow } from '../refresh.js';
import { fieldPanels } from './fields.js';
import { only } from './sections.js';
import { systemEditor, systemLines, systemNote } from './system.js';
import { pageSave } from './pagesave.js';
import { testChip, testLine, testOpened } from './clienttest.js';
import { typicalPath } from './way.js';
import { pageBand } from '../layout.js';

// What the tab sets: how often, which tests (0131), with its speeds or
// without, and the servers a speed is measured against, the local one and
// the remote one (0134), which are the APs' own settings (0121). The setting before which tests, over the wire too, is
// set no more: it is shown with the others where a folder has it still,
// under Schedule, and is taken away there as any setting is (0046).
const SET = ['assurance.tests.every', 'assurance.tests.over', 'assurance.tests.speed', 'system.speed_server', 'assurance.tests.speed_remote', 'system.speed_server_remote', 'assurance.tests.roam'];
// What the tests on a schedule join, and when they wait (0180): and when
// they are asked, and on which bands (0181).
const CONTROLS = ['assurance.tests.paused', 'assurance.tests.window', 'assurance.tests.windows', 'assurance.tests.bands', 'assurance.tests.per_hour', 'assurance.tests.alert_after', 'assurance.tests.networks', 'assurance.tests.leave_out', 'assurance.tests.on', 'assurance.tests.listed',
	'assurance.tests.floor', 'assurance.tests.max_clients', 'assurance.tests.max_busy'];
const FIELDS = [...SET, ...CONTROLS, 'assurance.tests.until', 'assurance.tests.wire'];
const SECTIONS = [['Client tests on a schedule', SET], ['What they join, and when they wait', CONTROLS]];
// The tests a round is asked for (0131), as the choice names them, and as
// a sentence does.
const OVER = [['both', 'Both'], ['air', 'Wireless'], ['wire', 'Wired']];
const OVERSAID = { air: ', through the air only', wire: ', over the wire only' };
// What was chosen last: kept here for as long as the page is open, as the
// tab is drawn again every few seconds, and in the browser for the next
// time, where it keeps anything.
const KEPT = 'aeolus.round.over';
let chosenOver = null;
function kept() {
	if (chosenOver) return chosenOver;
	try {
		const v = localStorage.getItem(KEPT);
		return OVER.some(([k]) => k === v) ? v : 'both';
	} catch { return 'both'; }
}
function keep(k) {
	chosenOver = k;
	try { localStorage.setItem(KEPT, k); } catch { /* storage blocked */ }
}

// asSet is the fields as the schedule's editor is shown them. Where which
// tests is not set and the setting before, over the wire too, is on, both
// is in force (0131): the editor is shown that, from where the setting
// before comes, so that choosing through the air there is a change.
function asSet(fields) {
	const before = fields['assurance.tests.wire'];
	if (fields['assurance.tests.over'] || before?.value !== true) return fields;
	// A lock on the setting before is not one on this: a lock is a field's.
	return { ...fields, 'assurance.tests.over': { ...before, value: 'both', origin: before.origin === 'locked' ? 'inherited' : before.origin } };
}

// The checks opened, by what they are of, so a redraw keeps them open.
const open = new Set();

// round is how long a tester takes to come back to the same thing.
function round(t) {
	const m = t.every * t.things;
	return m < 120 ? `${m} minutes` : m < 2880 ? `${Math.round(m / 60)} hours` : `${Math.round(m / 1440)} days`;
}

// tester says what an AP that tests is asked for (0131): through the air,
// with its scan radio; over the wire (0128), from its uplink; or both.
function tester(t) {
	const air = t.air && t.over !== 'wire';
	const how = air && t.wire ? 'has a scan radio, and tests over the wire too'
		: t.wire ? (t.air ? 'has a scan radio, and is asked for tests over the wire only' : 'tests over the wire, from its uplink')
			: air ? 'has a scan radio' : t.air ? 'has a scan radio, and is asked for tests over the wire, which its agent cannot make'
				: 'can test over the wire, from its uplink';
	// What it tests in a round asked by hand (0129) may be more than its
	// schedule's: over the wire, where its folders ask for none of that.
	const asked = t.round ? ` In a round asked by hand it tests ${many(t.round, 'thing')}.` : '';
	if (!t.things && !t.round) return `${t.name} ${how}, and offers no network a test can try.`;
	if (!t.every) return `${t.name} ${how}, and is not asked for tests on a schedule.${asked}`;
	if (t.paused) return `${t.name} ${how}. Its schedule, one test ${value('assurance.tests.every', t.every)}, is paused (0180).${asked}`;
	if (!t.things) return `${t.name} ${how}, and has nothing its schedule can test.${asked}`;
	const within = t.window ? ` between ${t.window.replace('-', '–')}, its local time,` : '';
	const onBands = t.bands?.length && air ? ` Through the air on ${t.bands.map(bandName).join(' and ')} only.` : '';
	return `${t.name} ${how}. It is asked for one test${within} ${value('assurance.tests.every', t.every)}${air && (t.speed || t.speed_remote) ? `, with its ${[t.speed && 'local', t.speed_remote && 'remote'].filter(Boolean).join(' and ')} speed through the air` : ''}: ${t.things} thing${t.things === 1 ? '' : 's'} in turn, so each about every ${round(t)}.${onBands}`;
}

// testerRow is an AP that tests, as a line: its name, the tests it is asked
// for on its schedule, or can make when asked, and how often. The whole of
// it is said on hover.
function testerRow(t) {
	const scheduled = t.every > 0 && t.things > 0 && !t.paused;
	const kinds = scheduled ? [t.air && t.over !== 'wire' && 'Wireless', t.wire && 'Wired'] : [t.air && 'Wireless', t.can_wire && 'Wired'];
	const when = scheduled ? `${value('assurance.tests.every', t.every)} · ${many(t.things, 'thing')}`
		: t.paused ? 'paused' : t.round ? `when asked · ${many(t.round, 'thing')}` : 'nothing to test';
	return h('tr', { title: tester(t) },
		h('td', null, t.name),
		h('td', null, kinds.filter(Boolean).map((k) => [h('span', { class: 'chip idle' }, k), ' '])),
		h('td', { class: 'sub' }, when));
}

const secs = (ms) => (ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(1)} s`);
const clock = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

// span is the least, middle and most of a step's times over the tests that
// took it, as "0.5 to 0.7 s, mostly 0.6 s"; nothing with fewer than two.
function span(runs, key) {
	const v = runs.map((r) => r[key]).filter((x) => x != null && x > 0).sort((a, b) => a - b);
	if (v.length < 2) return null;
	const mid = v[Math.floor(v.length / 2)];
	return v[0] === v[v.length - 1] ? secs(mid) : `${secs(v[0])} to ${secs(v[v.length - 1])}, mostly ${secs(mid)}`;
}

// ran says one test of a thing: when, and how it went.
const ran = (r) => `${clock(r.at)}: ${r.passed ? `passed${r.joined_ms ? `, let on in ${secs(r.joined_ms)}` : ''}${r.dhcp_ms ? `, an address in ${secs(r.dhcp_ms)}` : ''}${r.down != null ? `, ${r.down} down / ${r.up ?? '?'} up Mbit/s` : ''}${r.remote_down != null ? `, ${r.remote_down} down / ${r.remote_up ?? '?'} up Mbit/s remote` : r.remote_failed ? ', its remote speed not measured' : ''}`
	: `failed at ${r.stage}`}`;

// over is how a thing did over the time shown: a mark for each test, the
// oldest first, green or red, which says when and how it went on hover; and
// how long its steps took across those that passed.
function over(c) {
	const runs = c.runs || [];
	if (runs.length < 2) return null;
	const good = runs.filter((r) => r.passed);
	const times = [['let on in', span(good, 'joined_ms')], ['an address in', span(good, 'dhcp_ms')], ['DNS in', span(good, 'dns_ms')]]
		.filter(([, v]) => v).map(([k, v]) => `${k} ${v}`);
	// Its speeds down, the least to the most: to the local server, and to
	// the remote one (0134).
	for (const [key, said] of [['down', 'Mbit/s down'], ['remote_down', 'Mbit/s down from the remote server']]) {
		const d = good.filter((r) => r[key] != null).map((r) => r[key]).sort((a, b) => a - b);
		if (d.length > 1) times.push(`${d[0]} to ${d[d.length - 1]} ${said}`);
	}
	// The remote speed asked for and not measured fails no test: said here.
	const unmeasured = runs.filter((r) => r.remote_failed).length;
	if (unmeasured) times.push(`the remote speed not measured in ${unmeasured}`);
	return h('div', { class: 'runs' },
		h('span', { class: 'marks', role: 'img', 'aria-label': `${good.length} of ${runs.length} tests passed` },
			runs.map((r) => h('span', { class: `mark ${r.passed ? 'ok' : 'bad'}`, title: ran(r) }))),
		h('span', { class: 'sub' }, `${runs.length} tests since ${clock(runs[0].at)}${times.length ? ` · ${times.join(' · ')}` : ''}`));
}

// marks is how a thing did over the time shown, in its line: a mark a
// test, the latest twenty, the oldest first.
function marks(c) {
	const runs = (c.runs || []).slice(-20);
	if (!runs.length) return null;
	return h('span', { class: 'marks inline', role: 'img', 'aria-label': `${runs.filter((r) => r.passed).length} of its latest ${many(runs.length, 'test')} passed` },
		runs.map((r) => h('span', { class: `mark ${r.passed ? 'ok' : 'bad'}`, title: ran(r) })));
}

// The stages of coming online (0118), as a line says them.
const STAGE = { auth: 'authentication', assoc: 'association', signin: 'the sign-in', key: 'the keys', dhcp: 'DHCP', gateway: 'the gateway', dns: 'DNS', internet: 'the first connection' };
const many = (n, one, more) => `${n} ${n === 1 ? one : more || `${one}s`}`;

// joined is one network by its clients' own attempts to come online (0127):
// a line that opens to where the failed attempts stopped and to each AP's
// part.
function joined(n) {
	const key = `joins/${n.network || `ssid ${n.ssid}`}`;
	// Got on, of those that got on or did not, as the Overview counts them:
	// one that left of its own accord is neither, and is said apart.
	const tried = n.online + n.failed;
	const neither = n.clients - tried;
	const status = n.failing ? h('span', { class: 'chip bad', title: 'At an AP, in the last hour, three clients or more were let on and got no further, and they are half or more of those let on: an alert' }, 'Cannot get on')
		: n.failed ? h('span', { class: 'chip warn', title: `${many(n.failed, 'client')} did not get on` }, `${n.failed} did not`)
			: n.online ? h('span', { class: 'chip ok' }, 'All got on')
				: h('span', { class: 'chip idle' }, 'None seen through');
	const most = n.stops[0];
	// How fast the network did its part, typically (Griff, 2026-11-05: "online,
	// gateway and DNS"; "call online simply IP traffic, and a complete
	// internet connection Internet"): online, once let on and given an
	// address, so it has IP (0322); the gateway and DNS, how fast each
	// answered when asked; internet, how fast a connection it opened to the
	// internet was answered. Not how soon a device first asks: an IoT device
	// may say nothing for seconds.
	const answered = (k) => n.way?.find((p) => p.k === k && p.ms != null)?.ms;
	const timing = [n.typical_ms > 0 && `online ${secs(n.typical_ms)}`, answered('gateway') != null && `gateway ${secs(answered('gateway'))}`,
		answered('dns') != null && `DNS ${secs(answered('dns'))}`, answered('internet') != null && `internet ${secs(answered('internet'))}`].filter(Boolean).join(' · ');
	const d = h('details', { class: 'fold', open: open.has(key) },
		h('summary', null,
			h('span', { class: 'who' }, n.ssid || n.network, !n.network && h('span', { class: 'note', title: 'A network of the AP’s own, not one of Aeolus’s' }, ' not Aeolus’s')),
			h('span', { class: 'grow sub', title: [`${many(n.clients, 'client')} tried`, `${n.online} got on`,
				neither > 0 && `${neither} left before either`,
				timing && `typically ${timing}`,
				most && `${n.failed ? 'most' : 'some'} stopped at ${STAGE[most.stage] || most.stage}`].filter(Boolean).join(' · ') },
			[tried > 0 && `${n.online} of ${tried} got on`,
				neither > 0 && `${neither} left`,
				n.failed > 0 && most ? `stopped at ${STAGE[most.stage] || most.stage}` : timing].filter(Boolean).join(' · ')),
			status),
		n.way?.some((p) => p.passed || p.stopped) && h('div', { class: 'attempt' }, typicalPath(n.way, `${n.way_of < n.attempts ? 'the latest ' : ''}${many(n.way_of, 'attempt')}`)),
		h('p', { class: 'sub' }, `${many(n.attempts, 'attempt')}, ${n.attempts_failed} failed. A client got on where one of its attempts came online or was let on; it did not where one failed and none got on. One that left of its own accord, as for another AP, is neither.`),
		n.stops.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['Failed attempts stopped at', 'Why, as the AP put it', 'Attempts', 'Clients', 'Latest'].map((t) => h('th', null, t))),
			n.stops.map((s) => h('tr', null,
				h('td', null, STAGE[s.stage] || s.stage), h('td', null, s.reason || '—'),
				h('td', null, String(s.attempts)), h('td', null, String(s.clients)), h('td', null, ago(s.last))))),
		h('table', { class: 'list' },
			h('tr', null, ['On', 'Clients tried', 'Got on', 'Did not', 'Attempts', 'Failed', ''].map((t) => h('th', null, t))),
			n.aps.map((a) => h('tr', null,
				h('td', null, link(`/aps/${encodeURIComponent(a.ap)}/clients`, a.name)),
				h('td', null, String(a.clients)), h('td', null, String(a.online)), h('td', null, String(a.failed)),
				h('td', null, String(a.attempts)), h('td', null, String(a.attempts_failed)),
				h('td', null, a.failing && h('span', { class: 'chip bad' }, 'Clients cannot get on'))))));
	d.addEventListener('toggle', () => (d.open ? open.add(key) : open.delete(key)));
	return d;
}

// fresh says whether a round was asked lately enough that it should still
// be moving: an AP polls once a minute, and takes a thing each time.
const fresh = (r) => Date.now() - new Date(r.at).getTime() < 20 * 60 * 1000;

// rounded says how a round of tests asked by hand is going (0129).
function rounded(r, here) {
	if (!r || !r.of) return null;
	const where = r.node === here ? '' : r.name ? ` at ${r.name}` : ' above here';
	// Who asked is said to one who may view where it was asked.
	const by = r.by ? ` by ${r.by}` : '';
	const left = r.of - r.done;
	const only = OVERSAID[r.over] || '';
	const asked = `Asked${where} ${ago(r.at)}${by}${only}`;
	if (left <= 0) return h('p', { class: 'sub roundline', title: asked }, h('span', { class: 'chip ok' }, 'Round done'), ` ${many(r.of, 'thing')} tested · asked ${ago(r.at)}`);
	// What is left a while after: an AP that is away, or behind on its
	// config, or a thing that cannot be asked for now.
	const stuck = !fresh(r) && r.asked === r.done;
	return h('p', { class: 'sub roundline', title: `${asked}. ` + (stuck ? 'What is left could not be asked for: an AP that is away or does not run its current config, or a thing that cannot be tested now. It is let go an hour after the round was asked.'
		: 'Each AP takes them one at a time, about one a minute; what is not asked for within the hour is let go.') },
	h('span', { class: `chip ${stuck ? 'warn' : 'idle'}` }, stuck ? 'Round stuck' : 'Round under way'),
	` ${r.done} of ${r.of} tested${stuck ? ' · the rest could not be asked for: an AP is away or behind on its config, or a thing cannot be tested now'
		: r.asked > r.done ? ' · one under way' : ''} · asked ${ago(r.at)}`);
}

// check is one thing tested by one AP: a line that opens to its latest test.
function check(ctx, c) {
	const key = `${c.tester}/${c.network}/${c.on}/${c.band}`;
	const where = [c.on === c.tester ? `${c.on_name} itself` : c.on_name || 'the strongest AP heard', c.band && bandName(c.band)].filter(Boolean).join(' · ');
	const tried = c.passed + c.failed;
	// Its latest test, given in brief: fetched as the line opens (0287).
	const latest = c.last ? testOpened(c.last, (ap) => ctx.name('locations', ap), true) : h('p', { class: 'sub' }, 'No test of it has been made yet.');
	const status = c.failing ? h('span', { class: 'chip bad', title: 'Its last two tests failed' }, `Failing${c.since ? ` for ${ago(c.since).replace(/ ago$/, '')}` : ''}`)
		: c.last ? testChip(c.last)
			: h('span', { class: 'chip idle' }, c.scheduled ? 'not tried yet' : 'not on the schedule');
	const d = h('details', { class: 'fold', open: open.has(key) },
		h('summary', null,
			h('span', { class: 'who' }, c.ssid || c.network),
			h('span', { class: 'grow sub', title: tried > 0 ? `${c.passed} of ${many(tried, 'test')} passed in the time shown` : null },
				[where, c.on !== c.tester && `from ${c.tester_name}`, c.failed > 0 && `${c.passed} of ${tried} passed`,
					c.last && ago(c.last.at), !c.scheduled && tried > 0 && 'not on the schedule'].filter(Boolean).join(' · ')),
			marks(c),
			status),
		over(c),
		c.way?.some((p) => p.passed || p.stopped) && tried > 1 && h('div', { class: 'attempt' }, typicalPath(c.way, many(tried, 'test'))),
		c.last && tried > 1 && h('p', { class: 'sub attemptlinks' }, 'Its latest test:'),
		latest);
	d.addEventListener('toggle', () => {
		if (d.open) {
			open.add(key);
			latest.load?.();
		} else open.delete(key);
	});
	if (d.open) latest.load?.();
	return d;
}

// scheduleNow is the schedule as it is now, in a line (0188): none, paused,
// running around the clock or in its blocks, or for a while and until when;
// and, for one who may change it, Run for a while, Stop, and Run again.
function scheduleNow(here, set, testers, edit) {
	const every = set['assurance.tests.every']?.value;
	const paused = set['assurance.tests.paused']?.value === true;
	const until = set['assurance.tests.until']?.value;
	const blocks = set['assurance.tests.windows']?.value?.length || set['assurance.tests.window']?.value;
	const now = Date.now() / 1000;
	const often = every ? value('assurance.tests.every', every) : '';
	const says = !every ? 'No schedule: tests run only when asked.'
		: paused ? `Paused: ${often} when it runs.`
			: until && until > now ? `Running ${often} until ${value('assurance.tests.until', until)}${blocks ? ', in its blocks of time' : ''}.`
				: until ? `Stopped: its run ended ${ago(new Date(until * 1000))}.`
					: `Running ${often}${blocks ? ', in its blocks of time' : ', around the clock'}.`;
	const out = h('span');
	const change = async (op, did) => {
		try {
			await post('/v1/changes', { op: { tree: 'locations', node: here, ...op } });
			flash(did, 20);
			redrawNow();
		} catch (err) {
			out.replaceChildren(h('span', { class: 'error' }, err.message));
		}
	};
	const hours = h('select', { class: 'small', 'aria-label': 'For how long' }, [1, 2, 4, 8, 24].map((n) => h('option', { value: n }, n === 1 ? '1 hour' : `${n} hours`)));
	const runFor = h('button', { type: 'button', class: 'button small', title: 'Run the schedule for a while, then stop it: a test every 15 minutes unless one is set', onclick: () => {
		const n = Number(hours.value);
		const values = { 'assurance.tests.until': Math.floor(now) + n * 3600, 'assurance.tests.paused': false };
		if (!every) values['assurance.tests.every'] = 15;
		change({ kind: 'set', values }, `Tests on a schedule run for ${n === 1 ? 'an hour' : `${n} hours`}, then stop.`);
	} }, 'Run for');
	const stop = h('button', { type: 'button', class: 'button small', title: 'Pause the schedule: tests asked by hand still run', onclick: () => change({ kind: 'set', values: { 'assurance.tests.paused': true } }, 'Tests on a schedule are paused.') }, 'Stop');
	const again = h('button', { type: 'button', class: 'button small', title: 'Run the schedule with no end, around the clock or in its blocks of time', onclick: async () => {
		if (until) await change({ kind: 'unset', path: 'assurance.tests.until' }, 'The run has no end.');
		if (paused) await change({ kind: 'set', values: { 'assurance.tests.paused': false } }, 'Tests on a schedule run again.');
	} }, 'Run on');
	const going = every && !paused && (!until || until > now);
	return h('p', { class: 'sub pad schedule-now', title: testers.some((t) => t.running) ? 'An AP here is asked for tests on a schedule now' : '' },
		says, edit && [' ', hours, ' ', runFor, ' ', going ? stop : (every && (paused || until)) ? again : null], out);
}

// often says how often a client came online again, an hour's rate as a
// time between: every 45 s, every 12 min; or that it stayed.
function often(perHour) {
	if (!perHour) return 'stays';
	const s = 3600 / perHour;
	return s < 90 ? `again every ${Math.round(s)} s` : s < 5400 ? `again every ${Math.round(s / 60)} min` : `again every ${Math.round(s / 3600)} h`;
}

// compatPanel is how each maker's clients fare on each model of AP here
// (0193): a device that does not get on with a kind of radio first, in
// words, with its worst clients; then a maker whose clients loop on every
// model they are on, once, which is no model's (0200); then every maker,
// model and band.
// What the tests' roams came to (0225), a band at a time, the page's
// (0284), as no table mixes bands: for each network, how many roams, and
// each kind's typical time, with its count, its least and most on the
// cell's title.
const KIND = { ft: '802.11r', cached: 'Key held', keys: 'WPA2 keys', full: 'Signed in again' };

function roamsPanel(roams, lately) {
	const bands = ['2g', '5g', '6g'].filter((b) => roams.some((r) => r.band === b));
	if (!bands.length) return null;
	const band = pageBand() ?? (bands.includes('5g') ? '5g' : bands[0]);
	const box = h('div');
	// A roam the AP gone to could not time is counted, and has no time: on
	// a network that signs clients in, the AP says nothing of an 802.11r
	// roam's authentication (0133).
	const cell = (r, k) => {
		const x = r.kinds.find((y) => y.kind === k);
		if (!x) return h('td', { class: 'sub' }, '–');
		const timed = x.roams - (x.untimed || 0);
		const title = `${many(x.roams, 'roam')}${timed ? `, ${x.low_ms} to ${x.high_ms} ms` : ''}${x.untimed ? `; ${x.untimed} the AP could not time` : ''}`;
		return h('td', { title }, timed ? `${x.typical_ms} ms` : 'untimed', h('span', { class: 'sub' }, ` ×${x.roams}`));
	};
	const draw = () => {
		const rows = roams.filter((r) => r.band === band);
		box.replaceChildren(!rows.length ? h('div', { class: 'empty' }, `No roams on ${bandName(band)}.`) : h('table', { class: 'list dense' },
			h('thead', null, h('tr', null, h('th', null, 'Network'), h('th', null, 'Roams'), ...Object.values(KIND).map((k) => h('th', null, k)))),
			h('tbody', null, rows.map((r) => h('tr', { class: r.failed ? 'failed' : '' }, h('td', null, r.ssid || r.network),
				h('td', null, r.failed ? `${r.roams}, ${r.failed} not let on` : String(r.roams)), ...Object.keys(KIND).map((k) => cell(r, k)))))));
	};
	draw();
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols', title: 'The tests through the air, once on, roam to the other APs they test the network on, and back, as a client walking about would: each roam timed from asking until let on, by how the client came by its keys. The typical time, with how many; a cell’s title has the least and most.' },
			'Roams by the tests', h('span', { class: 'note' }, [bandName(band), lately].filter(Boolean).join(' · '))),
		box);
}

// compatPanel's table is of the page's band (0284); the troubles above it
// are said of every band, each naming its own.
function compatPanel(c) {
	const band = pageBand();
	const groups = c.groups || [];
	const shown = groups.filter((g) => !band || g.band === band);
	const troubled = groups.filter((g) => g.trouble);
	const everywhere = groups.filter((g, i) => g.everywhere && groups.findIndex((o) => o.everywhere && o.maker === g.maker) === i);
	const worst = (g) => g.worst.filter((w) => w.per_hour > 0).slice(0, 3)
		.map((w) => `${w.host || w.mac} on ${w.ap_name}, ${often(w.per_hour).replace('again ', '')}`).join('; ');
	return h('section', { class: 'panel' },
		h('h2', { title: 'How the clients of each maker fare on each model of AP, a band at a time, by their own attempts to come online: how often the middle one came online again while it was at an AP, and how many came back as one that drops its connection itself, 20 times an hour or more.' },
			'Devices and APs', h('span', { class: 'note' }, `${troubled.length ? `${many(troubled.length, 'maker')} troubled · ` : ''}last ${c.hours} h`)),
		troubled.map((g) => h('p', { class: 'pad compat-trouble' }, h('strong', null, `${g.maker} on ${g.model}, ${bandName(g.band)}: `), `${g.trouble}.`,
			worst(g) && h('span', { class: 'sub' }, ` ${worst(g)}.`))),
		everywhere.map((g) => h('p', { class: 'pad sub' }, h('strong', null, `${g.maker} on ${g.model}, ${bandName(g.band)}: `), `${g.everywhere}.`)),
		shown.length
			? h('table', { class: 'list compat' },
				h('thead', null, h('tr', null, ['Maker', 'AP model', 'Clients', 'Typical', 'Looping', 'Failed'].map((t) => h('th', null, t)))),
				h('tbody', null, shown.map((g) => h('tr', { class: g.trouble ? 'warn' : null },
					h('td', null, g.maker), h('td', null, g.model), h('td', { class: 'num' }, g.clients),
					h('td', null, often(g.per_hour)), h('td', { class: 'num' }, g.looping || ''), h('td', { class: 'num' }, g.failed || '')))))
			: h('p', { class: 'sub pad' }, band ? `No client came online here on ${bandName(band)}.` : 'No client came online here.'));
}

export async function assuranceTab(ctx, here, page, edit) {
	let view, compat;
	try {
		[view, compat] = await Promise.all([
			get(`/v1/assurance?under=${encodeURIComponent(here)}&tests=brief`),
			get(`/v1/compatibility?under=${encodeURIComponent(here)}`).catch(() => null), // an older manager has none
		]);
	} catch {
		return [h('div', { class: 'banner info' }, 'This manager has no assurance yet.')];
	}
	const testers = view.testers || [];
	const checks = view.checks || [];
	const tests = view.tests || [];
	const box = h('div', { class: 'edit flush' });
	// One save for the tab (0202): the schedule's form and each value of
	// the Schedule changed in its row (0286) are drafts of it.
	const d = edit ? await schema() : null;
	const save = edit && pageSave(ctx, 'locations', here, {
		summary: (values, seen) => [systemLines(d, page.node.name, asSet(page.fields || {}), values, 'Client tests', true, seen, ctx)],
		notes: (values) => [systemNote(Object.keys(values))],
	});
	const failing = checks.filter((c) => c.failing).length;
	const name = (ap) => ctx.name('locations', ap);
	const out = [];
	const joins = view.joins || [];
	const stuck = joins.filter((n) => n.failing).length;
	// A round asked by hand (0129): every AP here tests each thing it can,
	// once, now. While one is being worked at, the page looks again soon.
	// The page looks again soon only while a round should still be moving:
	// one left unfinished, as by an AP that is away, is not watched all hour.
	const going = view.round && view.round.of > 0 && view.round.done < view.round.of;
	if (going && (fresh(view.round) || view.round.asked > view.round.done)) hurry(60);
	// Another round may be asked at any time, and takes the place of one
	// under way: what was fixed can be tried again at once.
	const said = h('div');
	// Which tests (0131): both, or one kind alone, of those an AP here can
	// make. The choice is kept for the next time.
	const can = { both: testers.some((t) => t.round), air: testers.some((t) => t.air), wire: testers.some((t) => t.can_wire) };
	let over = can[kept()] ? kept() : 'both';
	const round = h('button', { type: 'button', class: 'button small' }, going ? 'Test everything again' : 'Test everything now');
	const seg = h('span', { class: 'segmented', role: 'group', 'aria-label': 'Which tests' });
	const TITLE = {
		both: 'Every AP here tests each thing it can once, now, whatever its schedule: through the air where it has a scan radio, and over the wire',
		air: 'Every AP here with a scan radio joins each of its networks as a client would, once, now, whatever its schedule',
		wire: 'Every AP here asks each of its networks for an address, its gateway and a name, over the wire, once, now, whatever its schedule',
	};
	const pick = () => {
		seg.replaceChildren(...OVER.map(([k, name]) => h('button', { type: 'button', class: k === over ? 'on' : '', 'aria-pressed': String(k === over), disabled: !can[k],
			title: can[k] ? TITLE[k] : k === 'air' ? 'No AP here has a scan radio' : 'No AP here can test that way',
			onclick: () => { over = k; keep(k); pick(); } }, name)));
		round.disabled = !can[over];
		round.title = TITLE[over];
	};
	pick();
	// A round narrowed to a troublesome few (0188): one network, one band,
	// the things joined on one AP; any of each, unless chosen.
	const nets = [...new Map(checks.map((c) => [c.network, c.ssid || c.network])).entries()].sort((a, b) => String(a[1]).localeCompare(String(b[1])));
	const targets = [...new Set(checks.map((c) => c.on).filter(Boolean))].map((id) => [id, name(id)]).sort((a, b) => String(a[1]).localeCompare(String(b[1])));
	const choose = (label, every, opts) => h('select', { class: 'small', 'aria-label': label, title: label }, h('option', { value: '' }, every), opts.map(([v, t]) => h('option', { value: v }, t)));
	const onlyNet = choose('Only this network', 'Every network', nets);
	const onlyBand = choose('Only this band, through the air', 'Every band', [['2g', '2.4 GHz'], ['5g', '5 GHz'], ['6g', '6 GHz']]);
	const onlyAP = choose('Only what joins on this AP', 'Every AP', targets);
	const narrowed = () => onlyNet.value || onlyBand.value || onlyAP.value;
	const label = () => { round.textContent = narrowed() ? 'Test these now' : going ? 'Test everything again' : 'Test everything now'; };
	for (const s of [onlyNet, onlyBand, onlyAP]) s.addEventListener('change', label);
	round.addEventListener('click', async () => {
		round.disabled = true;
		said.replaceChildren();
		const body = { under: here, over };
		if (onlyNet.value) body.networks = [onlyNet.value];
		if (onlyBand.value) body.bands = [onlyBand.value];
		if (onlyAP.value) body.on = [onlyAP.value];
		const which = [onlyNet.value && (nets.find(([v]) => v === onlyNet.value)?.[1] ?? onlyNet.value), onlyBand.value && bandName(onlyBand.value),
			onlyAP.value && `on ${name(onlyAP.value)}`].filter(Boolean).join(', ');
		try {
			const r = await post('/v1/assurance/round', body);
			flash(`A round asked of ${page.node.name}${which ? ` (${which})` : ''}${OVERSAID[over] || ''}: ${many(r.things, 'thing')} to test, each AP one at a time from its next poll.`, 60);
			hurry(600);
			redrawNow();
		} catch (err) {
			round.disabled = false;
			said.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	});

	// What failed, asked again (0129): each thing whose latest test failed,
	// alone.
	const failed = checks.filter((c) => c.last?.state === 'failed');
	const again = edit && failed.length > 0 && h('button', { type: 'button', class: 'button small', title: 'Test again only what failed in its latest test', onclick: async (e) => {
		const b = e.currentTarget;
		b.disabled = true;
		said.replaceChildren();
		try {
			const r = await post('/v1/assurance/round', { under: here, over: 'both', only: failed.map((c) => ({ network: c.network, on: c.on, band: c.band || 'wire' })) });
			flash(`What failed asked again of ${page.node.name}: ${many(r.things, 'thing')} to test, each AP one at a time from its next poll.`, 60);
			hurry(600);
			redrawNow();
		} catch (err) {
			b.disabled = false;
			said.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	} }, `Test what failed (${failed.length})`);

	const lately = `last ${view.hours} h`;
	out.push(h('section', { class: 'panel' },
		h('h2', { title: 'Every client’s own attempts to come online, as the APs recorded them. A line opens to where they stopped; each client’s attempts, step by step, are under Clients, Everyone seen.' },
			'Clients coming online', h('span', { class: 'note' }, stuck ? `${many(stuck, 'network')} failing · ${lately}` : lately)),
		joins.length ? joins.map(joined) : h('p', { class: 'sub pad' }, 'None tried.')));
	if (compat) out.push(compatPanel(compat));

	out.push(h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols', title: 'An AP with a scan radio joins its networks as a client would, in turn, and says how each step went. Over the wire, any AP asks each of its networks for an address, its gateway and a name.' },
			'Client tests',
			edit && h('span', { class: 'controls' }, onlyNet, onlyBand, onlyAP, seg, round, again,
				h('button', { type: 'button', class: 'button small', onclick: () => box.firstChild || systemEditor(ctx, d, here, page.node.name, asSet(page.fields), box,
					{ sections: SECTIONS, heading: 'Edit the schedule', what: 'Client tests', save }) }, 'Schedule…'))),
		// What a button here says comes right below it.
		said,
		scheduleNow(here, asSet(page.fields || {}), testers, edit),
		box,
		testers.length
			? h('table', { class: 'list testers' }, testers.map(testerRow))
			: h('p', { class: 'sub pad' }, 'No AP here can test yet: one needs a scan radio, or to test over the wire.'),
		rounded(view.round, here)));

	const roams = roamsPanel(view.roams || [], lately);
	if (roams) out.push(roams);

	out.push(h('section', { class: 'panel' },
		h('h2', { title: 'Each thing an AP tests, the failing first, with a mark for each of its latest tests, twenty at the most. A line opens to what its tests come to and to its latest.' },
			'What is tested', h('span', { class: 'note' }, checks.length ? (failing ? `${failing} failing · ${lately}` : lately) : '')),
		checks.length ? checks.map((c) => check(ctx, c)) : h('p', { class: 'sub pad' }, 'Nothing yet.')));

	// The tests themselves, newest first, are a log: closed until asked for.
	const fold = (key, title, heading, note, body) => {
		const d = h('details', { class: 'panel logpanel', open: open.has(key) },
			h('summary', null, h('h2', { title }, heading, h('span', { class: 'note' }, note))), body);
		d.addEventListener('toggle', () => (d.open ? open.add(key) : open.delete(key)));
		return d;
	};
	out.push(fold(`tests/${here}`, 'The latest tests, newest first, on the schedule or asked by hand', 'Tests', tests.length ? `latest ${tests.length}` : 'none yet',
		tests.length ? tests.map((a) => testLine(a, name, true, true)) : h('p', { class: 'sub pad' }, 'No test has been made here yet.')));

	// The schedule as it is set: where each setting comes from, and for
	// one set here the way back to the folder above (0046). Folded too.
	const set = asSet(page.fields || {});
	const every = set['assurance.tests.every']?.value;
	const which = OVER.find(([k]) => k === (set['assurance.tests.over']?.value || 'air'))?.[1] ?? 'Wireless';
	out.push(fold(`schedule/${here}`, 'How often the APs here are asked for a test, and which tests; where each setting comes from; and, for one set here, the way back to the folder above',
		'Schedule', every ? `${set['assurance.tests.paused']?.value === true ? 'paused · ' : ''}${value('assurance.tests.every', every)} · ${which.toLowerCase()}` : 'none',
		fieldPanels(ctx, 'locations', here, only(page.fields, (p) => FIELDS.includes(p)), edit && { ...edit, schema: d }, save)));
	if (save) out.push(save.bar);
	return out;
}
