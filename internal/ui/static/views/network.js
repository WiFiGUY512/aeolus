// Network Paths, under Wired, of a Locations folder, the Org or an AP
// (0141; named and put there by Griff, 2026-10-12): how long the networks'
// own services take to answer the APs below it, or that AP. An address from
// DHCP, the gateway, a name looked up, and the RADIUS server, each as what
// it typically takes and as a line over the time chosen; of every VLAN
// together, or of the one picked. It reads what the client tests and the
// state reports already time, and sets nothing. The times are drawn with
// Plotly (0294), over a span as a chart's are anywhere.

import { h } from '../dom.js';
import { get } from '../api.js';
import { chart, at as plotTime, series, spanPicker, spanName, SPANS, HOW } from '../chart.js';

// The steps shown, with what each is in full for a hover. A whole sign-in
// is shown only where one was tried, whether or not any was answered:
// where a network signs clients in.
const STEPS = [
	['dhcp', 'DHCP', 'From asking for an address to having one, as a client test found.'],
	['gateway', 'Gateway', 'How long the gateway took to answer, as a client test found.'],
	['dns', 'DNS', 'How long a name took to look up, as a client test found.'],
	['aaa', 'AAA', "How long the network's RADIUS server took to answer the AP, as each state report says."],
	['signin', 'Sign-in', 'A whole 802.1X sign-in, as a client test through the air found.'],
	['internet', 'Internet', "How long the internet's target took to answer the AP itself, from its management VLAN, as each state report says."],
];
// Those shown only where they were tried: a network that signs clients in,
// a folder with an internet target.
const SOMETIMES = new Set(['signin', 'internet']);

// ms writes a time: 4.1 ms, 284 ms, 2.0 s.
const ms = (n) => (n == null ? '—' : n >= 1000 ? `${(n / 1000).toFixed(1)} s` : n >= 10 ? `${Math.round(n)} ms` : `${n.toFixed(1)} ms`);

// What is on a segment, as it is called: a network by its SSID, the APs'
// own management, or a network of an AP's own.
const called = (n) => (n.kind === 'management' ? 'Management' : n.ssid || n.network);

// What a segment is called: its VLAN or its tunnel, and the networks on it.
function named(s) {
	const nets = s.networks.map(called).join(', ');
	if (s.vlan != null) return `VLAN ${s.vlan} · ${nets}`;
	if (s.vni != null) return `Tunnel ${s.vni} · ${nets}`;
	return nets;
}

// What each folder's tab was last drawn over, and of which segment, kept
// while the page is.
const view = new Map();

// overTime draws each step timed in the span, a line each, its average
// over each stretch, broken where it was not timed, with a dot each so one
// stretch alone shows. They share one scale of time, logarithmic: an
// address takes seconds where a name takes milliseconds. A step keeps its
// colour whichever others are drawn with it.
function overTime(v, id, segment, timed) {
	const x = v.series.map((p) => plotTime(p.at));
	const period = { xperiod: v.bucket * 1000, xperiodalignment: 'middle' };
	return chart({
		key: `network:${id}:${segment}`, span: v.hours, name: 'response-times', height: 250,
		label: `How long each step took on average over ${spanName(v.hours)}`,
		traces: timed.map(([key, label]) => ({
			type: 'scatter', mode: 'lines+markers', name: label, x, y: v.series.map((p) => p.steps[key] ?? null),
			customdata: v.series.map((p) => ms(p.steps[key])), hovertemplate: '%{customdata}',
			line: { color: series(STEPS.findIndex(([k]) => k === key) + 1), width: 2 }, marker: { size: 5 }, ...period,
		})),
		layout: {
			yaxis: { type: 'log', rangemode: 'normal', tickvals: [0.1, 1, 10, 100, 1000, 10000], ticktext: ['0.1 ms', '1 ms', '10 ms', '100 ms', '1 s', '10 s'] },
		},
	});
}

export async function networkTab(ctx, id) {
	const box = h('div');
	const at = view.get(id) || { hours: SPANS[0][0], segment: '' };
	view.set(id, at);
	// Only the latest asking is drawn: an answer to an earlier choice that
	// comes in late does not replace it.
	let asked = 0;
	const draw = async () => {
		const mine = ++asked;
		let v;
		try {
			v = await get(`/v1/network?under=${encodeURIComponent(id)}&hours=${at.hours}${at.segment ? `&segment=${encodeURIComponent(at.segment)}` : ''}`);
		} catch (e) {
			// An older manager has none of it.
			if (mine === asked) box.replaceChildren(h('div', { class: 'banner info' }, e.status === 404 ? 'This manager does not keep these times yet.' : e.message));
			return;
		}
		if (mine !== asked) return;
		// A segment picked that this time holds nothing of is let go.
		if (at.segment && !v.segments.some((s) => s.id === at.segment)) {
			at.segment = '';
			return draw();
		}
		const pick = (set) => { set(); draw(); };
		const spans = spanPicker(at.hours, (t) => pick(() => { at.hours = t; }));
		const which = h('select', { 'aria-label': 'Which VLAN', onchange: (e) => pick(() => { at.segment = e.target.value; }) },
			h('option', { value: '', selected: !at.segment }, 'All VLANs'),
			v.segments.map((s) => h('option', { value: s.id, selected: at.segment === s.id }, named(s))));
		const of = at.segment ? v.segments.find((s) => s.id === at.segment).steps : v.all;
		const any = v.segments.length > 0;
		const shown = STEPS.filter(([key]) => !SOMETIMES.has(key) || tried(of[key]));
		const cards = h('div', { class: 'tiles steptimes' }, shown.map(([key, label, says]) => {
			const s = of[key];
			return h('div', { class: 'tile', title: says },
				h('div', { class: 'tlabel' }, label),
				h('div', { class: 'tbig' }, ms(s?.ms)),
				h('div', { class: 'tsub' }, !s || (!s.n && !s.failed) ? 'not timed' : [s.most != null && `slowest ${ms(s.most)}`, s.failed > 0 && `${s.failed} no answer`].filter(Boolean).join(' · ')));
		}));
		const timed = shown.filter(([key]) => of[key]?.n > 0);
		const cell = (s) => h('td', { class: s?.failed ? 'warnish' : '', title: s && (s.n || s.failed) ? `timed ${s.n} time${s.n === 1 ? '' : 's'}${s.most != null ? `, slowest ${ms(s.most)}` : ''}${s.failed ? `; ${s.failed} no answer` : ''}` : null },
			ms(s?.ms), s?.failed > 0 && h('span', { class: 'sub' }, ` · ${s.failed} no answer`));
		const table = v.segments.length > 0 && h('section', { class: 'panel' },
			h('h2', null, 'By VLAN', h('span', { class: 'note' }, 'what each typically takes')),
			h('table', { class: 'list' },
				h('tr', null, ['VLAN', 'Networks', ...shownAll(v).map(([, label]) => label)].map((t) => h('th', null, t))),
				v.segments.map((s) => h('tr', { class: at.segment === s.id ? 'picked' : '' },
					h('td', null, h('a', { href: '#', title: 'Draw this one alone', onclick: (e) => { e.preventDefault(); pick(() => { at.segment = at.segment === s.id ? '' : s.id; }); } },
						s.vlan != null ? `VLAN ${s.vlan}` : s.vni != null ? `Tunnel ${s.vni}` : '—')),
					// A network of an AP's own, not Aeolus's, is dull: nothing times it.
					h('td', null, s.networks.map((n, i) => [i > 0 && ', ',
						n.kind === 'own' ? h('span', { class: 'sub', title: "A network of an AP's own, not Aeolus's: nothing here times it." }, called(n)) : called(n)])),
					shownAll(v).map(([key]) => cell(s.steps[key]))))));
		box.replaceChildren(
			h('section', { class: 'panel' },
				h('h2', { title: `How long each took on average, as the APs here timed it: in their client tests, and of the RADIUS server in their reports. ${HOW}` },
					h('span', { class: 'title' }, 'Response times', spans), which),
				any ? [cards, timed.length > 0 && overTime(v, id, at.segment, timed)]
					: h('p', { class: 'sub pad' }, 'Nothing was timed here in this time. Client tests on a schedule time each network\'s DHCP, gateway and DNS (Assurance); a network that signs clients in has its RADIUS server timed with every report.')),
			table);
	};
	await draw();
	return box;
}

// tried says whether a step was ever tried: timed, or left unanswered.
const tried = (s) => !!s && (s.n > 0 || s.failed > 0);

// shownAll is the steps the table has a column for: a sign-in or the
// internet only where some segment tried one.
function shownAll(v) {
	return STEPS.filter(([key]) => !SOMETIMES.has(key) || v.segments.some((s) => tried(s.steps[key])));
}
