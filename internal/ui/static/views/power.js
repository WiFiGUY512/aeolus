// Power control (0077, 0206) of a Locations folder or AP, under Radios: its
// switch and policy, which need neighbours on; each radio the APs hold, as
// each last reported, with its clients that no other AP hears better (its
// lonely clients) and how they took its last step down; and the steps of
// the last day, the latest first. Where power control asks (apc.clients
// unset) and a lonely client hears its AP below the threshold, or clients
// took a step down badly, the page offers the two answers: guard them, or
// ignore them.

import { h, link } from '../dom.js';
import { bandName, ago, value } from '../format.js';
import { get } from '../api.js';
import { called } from '../names.js';
import { pageSave } from './pagesave.js';
import { onOff, policyRow, APC_POLICY, POWER } from './neighbours.js';

// How a radio's clients took a step down (0206), in a few words.
const DISTRESS = { scanning: 'began looking for another AP', leaving: 'began to leave', retries: 'needed more tries' };

// Why each step was taken, in a few words.
const STEP = {
	above: 'all its neighbours hear it well', below: 'its weakest neighbour hears it below the target',
	looking: 'too few neighbours hear it', guard: 'a lonely client needs it', hold: 'held after its clients struggled',
	distress: 'its clients struggled: stepped back', 'distress-seen': 'its clients struggled: asked, not stepped back',
	ceiling: 'at its ceiling', floor: 'at its floor', target: 'at the target', new: 'taken over', failed: 'could not set its power',
};

const pct = (r) => (r == null ? '—' : `${Math.round(r * 100)}%`);

// hears is what a lonely client hears of its AP: as it says itself (802.11k)
// plainly, else reckoned, with ≈; hearsHow says which.
const hears = (c) => `${c.measured ? '' : '≈'}${c.est} dBm`;
const hearsHow = (c) => (c.measured
	? 'As the client says itself: it measured the AP\'s beacons when asked (802.11k)'
	: 'Reckoned from how strongly the AP hears it, at the AP\'s power, taking the client to send at 15 dBm: it cannot be asked (no 802.11k)');
const hours = (s) => (s < 3600 ? `${Math.round(s / 60)} min` : `${Math.round(s / 3600)} h`);

// clientsDid says what a radio's clients did after a step down, where they
// had a say: how many scanned, how many left or came back, and the share of
// what was sent them that was tried again or lost, before and after.
function clientsDid(d) {
	const said = [];
	if (d.scanning) said.push(`${d.scanning} scanning`);
	if (d.left) said.push(`${d.left} left or came back`);
	if (d.ratio?.length === 2 && d.ratio[0] != null && d.ratio[1] != null) said.push(`retries ${pct(d.ratio[0])} → ${pct(d.ratio[1])}`);
	return said.join(' · ');
}

// struggled says how a radio's clients took a step down, what they did, and
// how soon after it that was seen (0207).
const struggled = (d) => [DISTRESS[d.why] || d.why, clientsDid(d), d.after != null && `seen ${Math.max(1, Math.round(d.after / 60))} min after the step`]
	.filter(Boolean).join(' · ');

// powerSection draws the page. at is the node's page ({ node, nodeName,
// page, canEdit, parentName }); rows are its APs, each with its config and
// condition, for when the manager's view of power control cannot be read.
// band is the one band its tables show (Griff, 2026-10-22); what power
// control asks a person is of every band.
export async function powerSection(ctx, at, rows, band) {
	const save = at.canEdit ? pageSave(ctx, 'locations', at.node) : null;
	const both = { ...at, save };
	const sw = onOff(ctx, both, 'apc.enabled', APC_POLICY, 'Edit…');
	const data = await get(`/v1/power?under=${encodeURIComponent(at.node)}`).catch(() => null);
	const aps = data?.aps || [];
	const every = aps.flatMap((a) => a.radios.map((r) => ({ a, r })));
	const radios = every.filter(({ r }) => !band || r.band === band);
	// What asks a person (0206): a lonely client below the threshold, or
	// clients that struggled within the hour, where power control asks.
	const asks = every.filter(({ a, r }) => a.clients === 'ask' && ((r.lonely || []).some((c) => c.est < a.client_floor)
		|| (r.distress && !r.distress.acted && a.reported && new Date(a.reported).getTime() / 1000 - r.distress.at < 3600)));
	return [
		h('section', { class: 'panel' },
			sw.row,
			sw.on && APC_POLICY.map((p) => policyRow(ctx, both, p)),
			sw.boxes,
			sw.on && asks.length > 0 && save && choose(at, save, asks)),
		sw.on && radiosPanel(radios, data, rows, band),
		sw.on && lonelyPanel(radios, band),
		sw.on && data && stepsPanel(data, band),
		save?.bar,
	];
}

// choose offers the two answers to what power control asks (0206), as a
// draft of the page's save: guard, or ignore.
function choose(at, save, asks) {
	let chosen = null;
	const buttons = {};
	const draw = () => Object.entries(buttons).forEach(([v, b]) => b.classList.toggle('primary', chosen === v));
	const part = {
		changes: () => (chosen ? { 'apc.clients': chosen } : {}),
		dirty: () => chosen !== null,
		blocked: () => false,
		open: () => false,
		undo: () => {
			chosen = null;
			draw();
		},
		summary: (values) => [h('div', null, h('strong', null, `Power control on ${at.nodeName}`)),
			h('ul', { class: 'becomes' }, h('li', null, 'For lonely clients: ', value('apc.clients', 'ask'), ' → ', value('apc.clients', values['apc.clients'])))],
		notes: (values) => [h('div', { class: 'sub' }, values['apc.clients'] === 'guard'
			? 'Nothing restarts. Each AP keeps a radio loud enough that a client only it reaches hears it at the threshold, and steps back up and holds when its clients struggle after a step down.'
			: 'Nothing restarts. Each AP steps by its neighbours alone, and no longer warns of its clients.')],
	};
	save.register(part);
	for (const [v, label] of [['guard', 'Guard them'], ['ignore', 'Ignore them']]) {
		buttons[v] = h('button', { type: 'button', class: 'button small', onclick: () => {
			chosen = chosen === v ? null : v;
			draw();
			save.changed(part);
		} }, label);
	}
	const lonely = asks.filter(({ a, r }) => (r.lonely || []).some((c) => c.est < a.client_floor)).length;
	const struggling = asks.length - lonely;
	return h('div', { class: 'banner info' },
		h('div', null, [
			lonely > 0 && `${lonely === 1 ? 'A radio has' : `${lonely} radios have`} a client no other AP hears better that hears it below the threshold.`,
			struggling > 0 && `${struggling === 1 ? 'A radio\'s' : `${struggling} radios'`} clients struggled after a step down.`,
		].filter(Boolean).join(' '), ' Guard them: power control keeps such clients in reach, and steps back up and holds when clients struggle. Ignore them: it steps by the neighbours alone.'),
		h('div', { class: 'actions' }, buttons.guard, buttons.ignore));
}

// radiosPanel lists each radio the APs hold: its power, the neighbours
// that hear it, its lonely clients, and why it is where it is.
function radiosPanel(radios, data, rows, band) {
	if (!data) return h('section', { class: 'panel' }, h('div', { class: 'sub' }, 'The manager\'s view of power control could not be read.'));
	if (!radios.length) {
		return h('section', { class: 'panel' }, h('div', { class: 'sub' },
			!rows.length ? 'No APs here yet.' : band && data.aps.some((a) => a.radios.length) ? `No AP here holds a ${bandName(band)} radio.`
				: 'No AP holds a radio yet: each takes its radios over a minute after it applies the config.'));
	}
	let last = null;
	const lines = radios.map(({ a, r }) => {
		const [chip, says] = POWER[r.why] || ['idle', r.why];
		const first = a !== last;
		last = a;
		const lonely = r.lonely || [];
		const below = lonely.filter((c) => c.est < a.client_floor).length;
		const d = r.distress;
		const recent = d && a.reported && new Date(a.reported).getTime() / 1000 - d.at < 3600;
		return h('tr', null,
			h('td', null, first && link(`/aps/${encodeURIComponent(a.ap)}/radios/power`, a.name)),
			!band && h('td', null, bandName(r.band)),
			h('td', { class: 'mono' }, r.power != null ? `${r.power} dBm` : '—', r.ceiling != null && h('span', { class: 'sub' }, ` of ${r.ceiling}`),
				r.floor != null && h('div', { class: 'sub' }, `clients need ${r.floor}`),
				r.hold && h('div', { class: 'sub' }, `held at ${r.hold.power}, ${hours(r.hold.left)} more`)),
			h('td', { class: 'mono' }, r.count != null ? `${r.count} of ${r.wanted}` : '—', r.weakest != null && h('span', { class: 'sub' }, `, weakest ${r.weakest} dBm`)),
			h('td', null, a.clients === 'ignore' ? h('span', { class: 'sub' }, 'ignored')
				: lonely.length ? [`${lonely.length} lonely`, h('span', { class: 'sub', title: hearsHow(lonely[0]) }, `, loneliest ${hears(lonely[0])}`),
					below > 0 && [' ', h('span', { class: 'chip warn', title: `Hear the AP below ${a.client_floor} dBm` }, `${below} below`)]]
					: h('span', { class: 'sub' }, 'none lonely')),
			h('td', null, h('span', { class: `chip ${chip}` }, says),
				r.step ? h('span', { class: 'sub' }, ` ${r.step > 0 ? '+' : ''}${r.step} dB${r.ago != null ? `, ${r.ago < 120 ? `${r.ago} s` : `${Math.round(r.ago / 60)} min`} ago` : ''}`) : null,
				recent && h('div', null, h('span', { class: `chip ${d.acted ? 'idle' : 'warn'}`, title: struggled(d) }, d.acted ? 'stepped back for its clients' : 'clients struggled'),
					h('span', { class: 'sub' }, ` ${ago(d.at * 1000)}`))));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Radios'),
		h('table', { class: 'list' },
			h('tr', null, ['AP', ...(band ? [] : ['Band']), 'Power', 'Heard by', 'Lonely clients', 'Now'].map((c) => h('th', null, c))),
			lines));
}

// lonelyPanel lists the clients only one AP reaches, by radio, the
// loneliest first: what each hears of its AP, as it says itself (802.11k)
// or else reckoned from what the AP hears of it, with ≈; the best another
// AP hears it; and the power its AP needs for it to hear the AP at the
// threshold.
function lonelyPanel(radios, band) {
	const lines = radios.flatMap(({ a, r }) => (r.lonely || []).map((c, i) => h('tr', null,
		h('td', null, i === 0 && (band ? a.name : `${a.name} · ${bandName(r.band)}`)),
		h('td', null, called(c.mac, c.host) || h('span', { class: 'mono' }, c.mac)),
		h('td', { class: 'mono', title: hearsHow(c) }, hears(c), c.est < a.client_floor && [' ', h('span', { class: 'chip warn' }, `below ${a.client_floor}`)]),
		h('td', { class: 'mono' }, `${c.signal} dBm`),
		h('td', { class: 'mono' }, c.other != null ? `${c.other} dBm` : h('span', { class: 'sub' }, 'none')),
		h('td', { class: 'mono' }, `${c.need} dBm`))));
	if (!lines.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Lonely clients', h('span', { class: 'note' }, 'no other AP hears them 3 dB better; on for 30 min or more')),
		h('table', { class: 'list' },
			h('tr', null, [band ? 'AP' : 'Radio', 'Client', 'It hears the AP', 'The AP hears it', 'Best elsewhere', 'Power it needs'].map((c) => h('th', null, c))),
			lines));
}

// stepsPanel lists power control's steps of the last day, the latest
// first, with what the clients did where they had a say.
function stepsPanel(data, band) {
	const steps = (data.steps || []).filter((s) => !band || s.band === band);
	return h('section', { class: 'panel' },
		h('h2', null, 'Steps', h('span', { class: 'note' }, `the last ${data.hours} h, latest first`)),
		steps.length
			? h('table', { class: 'list' },
				h('tr', null, ['When', 'AP', ...(band ? [] : ['Band']), 'Power', 'Why', 'Its clients'].map((c) => h('th', null, c))),
				steps.slice(0, 100).map((s) => h('tr', null,
					h('td', { class: 'sub nowrap' }, ago(s.at)),
					h('td', null, link(`/aps/${encodeURIComponent(s.ap)}/radios/power`, s.ap_name)),
					!band && h('td', null, bandName(s.band)),
					h('td', { class: 'mono nowrap' }, s.from != null && s.to != null && s.from !== s.to ? `${s.from} → ${s.to} dBm` : s.to != null ? `${s.to} dBm` : '—'),
					h('td', null, STEP[s.why] || s.why),
					h('td', { class: 'sub' }, clientsDid(s)))))
			: h('div', { class: 'sub' }, 'No steps in that time.'));
}
