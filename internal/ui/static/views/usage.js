// Usage (0108): the Wi-Fi clients and traffic of the APs below a node over
// a day, or as long as a year (0140), from their state reports. Traffic is drawn in bars, what
// the APs sent their clients and what they received stacked, and the most
// clients they had at once as a line; with the totals, the clients that
// moved the most (0110), and on a folder its busiest APs by traffic. Both
// charts are drawn with Plotly (0294).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { size } from '../format.js';
import { called } from '../names.js';
import { chart, at, token, series, bandColour, counts, SPANS, spanName, spanPicker, HOW } from '../chart.js';

const SVG = 'http://www.w3.org/2000/svg';

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

const clock = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
const dated = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
const day = (t) => new Date(t).toLocaleDateString([], { month: 'short', day: 'numeric' });
// when says a time of a chart: by the clock over a day or two, with its
// date over longer, and by its date alone over more than a week or so.
const when = (u, t) => {
	const hours = (new Date(u.to) - new Date(u.from)) / 3600e3;
	return hours > 200 ? day(t) : hours > 49 ? dated(t) : clock(t);
};

// The times usage is drawn over (chart.js's spans): over more than two days
// the manager reads it from what it keeps by the hour (0140).
// What each node's usage was last drawn over, kept over a redraw.
const over = new Map();
const plural = (n, one) => `${n} ${one}${n === 1 ? '' : 's'}`;

// The folders whose tables were left open, over a redraw: each its own.
const more = new Set();

// usagePanel draws the usage of the APs below node id; folder adds its
// busiest APs, and has its tables a click down: on a folder's Overview the
// chart says the day (Griff, 2026-10-12).
export async function usagePanel(id, folder) {
	// Another time chosen draws the panel again in its place. Only the
	// latest asking is drawn: an answer to an earlier choice that comes in
	// late does not replace it.
	let box = null, asked = 0;
	const pick = async (hours) => {
		over.set(id, hours);
		const mine = ++asked;
		const next = await drawn(id, folder, pick);
		if (mine !== asked || !next || !box) return;
		box.replaceWith(next);
		box = next;
	};
	box = await drawn(id, folder, pick);
	return box;
}

async function drawn(id, folder, pick) {
	const hours = over.get(id) || SPANS[0][0];
	let u;
	try {
		u = await get(`/v1/usage?under=${encodeURIComponent(id)}&hours=${hours}`);
	} catch {
		return null; // an older manager has none
	}
	const x = u.buckets.map((b) => at(b.at));
	const period = { xperiod: u.bucket * 1000, xperiodalignment: 'middle' };
	// A bucket no AP reported in says nothing of the clients.
	const heard = u.buckets.map((_, i) => u.aps.some((a) => a.heard[i]));
	const accent = token('accent');
	// The traffic in bars, down and up stacked; the clients in a strip of
	// their own under them, on a scale of their own.
	const plot = chart({
		key: `usage:${id}`, span: hours, name: 'usage', height: 250,
		label: `Over ${spanName(hours)}, ${size(u.down)} down and ${size(u.up)} up; at most ${plural(u.peak, 'client')} at once`,
		traces: [
			{ type: 'bar', name: '↓ down', x, y: u.buckets.map((b) => b.down), customdata: u.buckets.map((b) => size(b.down)), hovertemplate: '%{customdata}', marker: { color: accent }, ...period },
			{ type: 'bar', name: '↑ up', x, y: u.buckets.map((b) => b.up), customdata: u.buckets.map((b) => size(b.up)), hovertemplate: '%{customdata}', marker: { color: token('accent-faint') }, ...period },
			{ type: 'scatter', mode: 'lines', name: 'clients at most', x, y: u.buckets.map((b, i) => (heard[i] ? b.clients : null)), yaxis: 'y2', line: { color: series(4), width: 1.5 }, ...period },
		],
		layout: {
			barmode: 'stack', bargap: 0.15,
			xaxis: { anchor: 'y2' },
			yaxis: { domain: [0.3, 1], tickformat: '~s', ticksuffix: 'B' },
			yaxis2: { domain: [0, 0.2], ...counts(u.peak, 2) },
		},
	});
	const aps = folder ? u.aps.slice(0, 10) : [];
	const me = !folder && u.aps[0];
	const top = (u.clients || []).slice(0, folder ? 10 : 5);
	// under is the tables under the chart: as they are on an AP's page, and
	// folded on a folder's.
	const under = (tables) => {
		if (!folder || !tables.length) return tables;
		const d = h('details', { class: 'fold', open: more.has(id) }, h('summary', null, 'Top clients and APs'), tables);
		d.addEventListener('toggle', () => (d.open ? more.add(id) : more.delete(id)));
		return d;
	};
	return h('section', { class: 'panel' },
		h('h2', { title: `From the APs' reports. Down is what the APs sent their clients, up what they received from them. ${HOW}` },
			h('span', { class: 'title' }, 'Usage', spanPicker(hours, pick))),
		h('div', { class: 'sub usagekey' }, `↓ ${size(u.down)} down · ↑ ${size(u.up)} up · at most ${plural(u.peak, 'client')} at once`),
		plot,
		under([top.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['Top clients', '↓ Down', '↑ Up', folder ? 'AP' : null].filter(Boolean).map((t) => h('th', null, t))),
			top.map((c) => h('tr', null,
				h('td', null, called(c.mac, c.host) || h('span', { class: 'mono' }, c.mac), called(c.mac, c.host) && h('div', { class: 'sub mono' }, c.mac),
					c.tester && h('div', { class: 'sub', title: "An AP's scan radio: what it used is the speed measurements of its client tests." }, `Aeolus test client${c.tester === 'an AP' ? '' : `, ${c.tester}'s scan radio`}`)),
				h('td', null, size(c.down)), h('td', null, size(c.up)),
				folder && h('td', { class: 'sub' }, c.aps.join(', '))))),
		me && h('div', { class: 'row' }, h('div', { class: 'label' }, 'Calling in'), h('div', { class: 'value' }, strip(u, me))),
		aps.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['AP', '↓ Down', '↑ Up', 'Most clients', 'Calling in'].map((t) => h('th', null, t))),
			aps.map((a) => h('tr', null,
				h('td', null, link(`/aps/${encodeURIComponent(a.ap)}`, a.name)),
				h('td', null, size(a.down)), h('td', null, size(a.up)), h('td', null, String(a.peak)),
				h('td', null, strip(u, a)))))].filter(Boolean)));
}

// spans names buckets of b seconds: half-hours, hours, 8-minute spans.
const spans = (b) => (b === 1800 ? 'half-hours' : b === 3600 ? 'hours' : b % 3600 === 0 ? `${b / 3600}-hour spans` : `${b / 60}-minute spans`);

// strip is an AP's connectivity over the span: a cell for each bucket, filled
// where it reported. The last, not over yet, is left pale until it does, as
// are those before the manager kept usage, which say nothing.
function strip(u, a) {
	const n = a.heard.length;
	const heard = a.heard.filter(Boolean).length;
	const from = u.recorded_from ? new Date(u.recorded_from).getTime() : Infinity;
	const ends = (i) => new Date(u.buckets[i].at).getTime() + u.bucket * 1000;
	const state = (on, i) => (on ? ['on', 'reported'] : i === n - 1 ? ['pending', 'not yet'] : ends(i) <= from ? ['pending', 'no record'] : ['off', 'did not report']);
	const cells = a.heard.map((on, i) => {
		const [cls, says] = state(on, i);
		return svg('rect', { x: i * 4, y: 0, width: 3, height: 12, class: cls }, svg('title', null, `${when(u, u.buckets[i].at)}: ${says}`));
	});
	return h('span', { class: 'strip', title: `reported in ${heard} of the last ${n} ${spans(u.bucket)}` },
		svg('svg', { viewBox: `0 0 ${n * 4} 12`, width: n * 4, height: 12, role: 'img', 'aria-label': `reported in ${heard} of ${n}` }, cells),
		h('span', { class: 'sub' }, ` ${heard} of ${n}`));
}

// The Clients overview (0166), first on a folder's Overview and an AP's
// (Griff, 2026-10-15: "a client connection overview, with a graph for 2.4
// 5 and 6ghz, Number on and number of problems, that seems more importatn
// than the total data usage"): the clients on each band now, the attempts
// to come online that failed over the span, and a line a band of the most
// clients the APs had at once, with those failures in a strip under it.
const BANDS = [['2g', '2.4 GHz'], ['5g', '5 GHz'], ['6g', '6 GHz']];
// The span chosen for the Clients panel, apart from Usage's.
const overClients = new Map();

// clientsPanel draws it for the APs below node id: now is the clients on
// each band in the APs' latest reports; base the page whose Assurance tab
// says more of the failures. bare leaves out the tiles of the clients on now
// and the failures, where the page says them already (0284).
export async function clientsPanel(id, now, base, bare) {
	let box = null, asked = 0;
	const pick = async (hours) => {
		overClients.set(id, hours);
		const mine = ++asked;
		const next = await clientsDrawn(id, now, base, pick, bare);
		if (mine !== asked || !next || !box) return;
		box.replaceWith(next);
		box = next;
	};
	box = await clientsDrawn(id, now, base, pick, bare);
	return box;
}

async function clientsDrawn(id, now, base, pick, bare) {
	const hours = overClients.get(id) || SPANS[0][0];
	let u;
	try {
		u = await get(`/v1/usage?under=${encodeURIComponent(id)}&hours=${hours}`);
	} catch {
		return null; // an older manager has none
	}
	const x = u.buckets.map((b) => at(b.at));
	const period = { xperiod: u.bucket * 1000, xperiodalignment: 'middle' };
	// A line a band, broken where a bucket has no count of it: before the
	// manager counted bands, or where no AP reported. A band no bucket
	// counts is left out.
	const banded = u.buckets.some((b) => b.bands);
	const lines = banded
		? BANDS.filter(([k]) => u.buckets.some((b) => b.bands?.[k] != null)).map(([k, name]) => ({ name, y: u.buckets.map((b) => b.bands?.[k] ?? null), colour: bandColour(k) }))
		: [{ name: 'clients', y: u.buckets.map((b) => b.clients || null), colour: series(1) }];
	const most = Math.max(1, ...lines.flatMap((l) => l.y.filter((v) => v != null)));
	const worst = Math.max(1, ...u.buckets.map((b) => b.failed || 0));
	const span = spanName(hours);
	// The failures in a strip of their own under the lines: a count, on no
	// scale of the clients'.
	const plot = chart({
		key: `clients:${id}`, span: hours, name: 'clients', height: 270,
		label: `Over ${span}, at most ${plural(u.peak, 'client')} at once; ${plural(u.failed || 0, 'attempt')} to get on failed`,
		traces: [
			...lines.map((l) => ({ type: 'scatter', mode: 'lines', name: l.name, x, y: l.y, line: { color: l.colour, width: 2 }, ...period })),
			{ type: 'bar', name: 'failed to get on', x, y: u.buckets.map((b) => b.failed || 0), yaxis: 'y2', marker: { color: token('bad') }, ...period },
		],
		layout: {
			bargap: 0.15,
			xaxis: { anchor: 'y2' },
			yaxis: { domain: [0.3, 1], ...counts(most) },
			yaxis2: { domain: [0, 0.2], ...counts(worst, 2) },
		},
	});
	const total = BANDS.reduce((s, [k]) => s + (now[k] || 0), 0);
	const failed = u.failed || 0;
	return h('section', { class: 'panel clientsoverview' },
		h('h2', { title: `From the APs' reports: the most clients at once on each band, and under it the attempts to come online that failed. ${HOW}` },
			h('span', { class: 'title' }, 'Clients', spanPicker(hours, pick))),
		!bare && h('div', { class: 'clientsnow' },
			h('div', { class: 'tile' }, h('div', { class: 'tlabel' }, 'On now'), h('div', { class: 'tbig' }, String(total)),
				h('div', { class: 'tsub' }, BANDS.map(([k, name]) => `${name} ${now[k] || 0}`).join(' · '))),
			h('a', { class: `tile ${failed ? 'bad' : 'ok'}`, href: `#${base}/assurance` }, h('div', { class: 'tlabel' }, 'Problems'), h('div', { class: 'tbig' }, String(failed)),
				h('div', { class: 'tsub' }, failed ? `failed to get on, by ${plural(u.failed_clients || 0, 'client')}, over ${span}` : `none failed to get on, over ${span}`))),
		h('div', { class: 'sub usagekey' }, `at most ${plural(u.peak, 'client')} at once · ${plural(failed, 'attempt')} to get on failed`),
		plot);
}
