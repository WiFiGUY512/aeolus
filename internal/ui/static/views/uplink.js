// An AP's uplink, a minute at a time (0351): what each of its uplink's
// Ethernet ports carried each way, on average and at its busiest five
// seconds, against its link's speed, so a wire that holds its clients back
// shows as a line that runs into the speed. On an AP, a chart a port, drawn
// with Plotly (0294); on a folder, a line an AP, which opens its own.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { chart as plot, at, token, spanPicker, HOW } from '../chart.js';

const SPANS = [[24, 'A day'], [72, '3 days']];

// What the part shows: a few hours, a day or three. Kept while the page is
// redrawn.
const view = { hours: 6 };

// The most APs a folder's line a port is read for.
const MOST = 24;

// rate says Mbit/s as people read it: Gbit/s from 1000.
export function rate(m) {
	if (m == null) return '—';
	if (m >= 1000) return `${(m / 1000).toFixed(1).replace(/\.0$/, '')} Gbit/s`;
	return `${Math.round(m)} Mbit/s`;
}

// summary is what a port's points came to over the span: its link as last
// said, its busiest each way and that as a share of its link, its errors,
// and how often it lost its link.
function summary(points) {
	const last = points[points.length - 1];
	let rx = null, tx = null, errors = 0, changes = 0;
	for (const p of points) {
		if (p.rx_max != null && (rx == null || p.rx_max > rx)) rx = p.rx_max;
		if (p.tx_max != null && (tx == null || p.tx_max > tx)) tx = p.tx_max;
		errors += p.errors || 0;
		changes += p.changes || 0;
	}
	return { up: last?.up, speed: last?.speed, full: last?.full, rx, tx, errors, lost: Math.floor(changes / 2) };
}

// share is a busiest rate as a share of the link, warned of from 90%, as
// the uplink-full alert counts a minute full (0351).
function share(m, speed) {
	if (m == null || !speed) return h('span', { class: 'mono' }, rate(m));
	const pct = Math.round((100 * m) / speed);
	return h('span', { class: `mono${pct >= 90 ? ' warn' : ''}`, title: `${pct}% of its ${rate(speed)} link` }, rate(m), h('span', { class: 'sub' }, ` (${pct}%)`));
}

function linkSaid(s) {
	if (!s.up) return h('span', { class: 'warn' }, 'no link');
	return h('span', { class: 'mono' }, rate(s.speed), s.full === false && h('span', { class: 'warn' }, ' half duplex'));
}

// portChart is a port's chart: in and out on average, their busiest five
// seconds dotted, and the link's speed dashed, the line they cannot pass.
function portChart(ap, port, hours) {
	const pts = port.points;
	if (pts.length < 2) return h('p', { class: 'sub pad' }, 'A chart needs two minutes of it; each comes at the minute\'s end.');
	const x = pts.map((p) => at(p.at));
	const into = token('accent'), out = token('accent-faint');
	const line = (name, key, colour, dash, width) => ({
		type: 'scatter', mode: 'lines', name, x, y: pts.map((p) => p[key] ?? null),
		line: { color: colour, width: width ?? 2, dash: dash ?? 'solid' }, hovertemplate: '%{y:,.0f} Mbit/s',
	});
	return plot({
		key: `uplink:${ap}:${port.port}`, span: hours, name: `uplink-${port.port}`, height: 240,
		label: `What ${port.port} carried each way over the last ${hours} hours, against its link's speed`,
		traces: [
			line('in, to the AP', 'rx', into),
			line('in, busiest 5 s', 'rx_max', into, 'dot', 1),
			line('out, from the AP', 'tx', out),
			line('out, busiest 5 s', 'tx_max', out, 'dot', 1),
			{ ...line('link speed', 'speed', token('muted'), 'dash', 1), line: { color: token('muted'), width: 1, dash: 'dash', shape: 'hv' } },
		],
		layout: { yaxis: { rangemode: 'tozero', ticksuffix: ' Mbit/s', separatethousands: true } },
	});
}

// portLine is a port's numbers over the span, in a line.
function portLine(p, s) {
	return h('div', { class: 'sub' },
		h('strong', { class: 'mono' }, p.port), ' · ', linkSaid(s),
		' · busiest in ', share(s.rx, s.speed), ', out ', share(s.tx, s.speed),
		s.errors > 0 && h('span', { class: 'warn' }, ` · ${s.errors} errors`),
		s.lost > 0 && h('span', { class: 'warn' }, ` · link lost ${s.lost} time${s.lost === 1 ? '' : 's'}`));
}

// uplinkSection draws the Uplink part of Wired: on an AP (ap, as its page
// has it, { ap: { id, name }, cfg }), its ports' charts; on a folder, a line
// an AP here.
export async function uplinkSection(ctx, id, page, ap) {
	const box = h('div');
	let asked = 0;
	const draw = async () => {
		const mine = ++asked;
		// On its own page, the AP is { ap: { id, name }, cfg }.
		const aps = ap ? [{ id: ap.ap.id, name: ap.ap.name }] : (page.hardware?.aps || []).slice(0, MOST);
		const got = await Promise.all(aps.map((a) => get(`/v1/aps/${encodeURIComponent(a.id)}/uplink?hours=${view.hours}`).then((r) => ({ a, r }), (e) => ({ a, e }))));
		if (mine !== asked) return;
		const picker = spanPicker(view.hours, (v) => { view.hours = v; draw(); }, SPANS);
		const title = h('h2', { title: HOW }, h('span', { class: 'title' }, 'Uplink', picker),
			h('span', { class: 'note' }, 'each way, a minute at a time: on average, and its busiest five seconds dotted, against its link\'s speed'));
		if (ap) {
			const { r, e } = got[0];
			if (e) return box.replaceChildren(h('div', { class: 'error' }, e.message));
			box.replaceChildren(h('section', { class: 'panel' }, title,
				r.ports.length
					? r.ports.map((p) => [portLine(p, summary(p.points)), portChart(ap.ap.id, p, r.hours)])
					: h('p', { class: 'sub pad' }, 'It has said nothing of its uplink yet: an agent from v0.322.0 says it with each minute.')));
			return;
		}
		const rows = [];
		for (const { a, r, e } of got) {
			const to = link(`/aps/${encodeURIComponent(a.id)}/wired/uplink`, a.name);
			if (e) {
				rows.push(h('tr', null, h('td', null, to), h('td', { colspan: 6, class: 'sub' }, e.message)));
				continue;
			}
			if (!r.ports.length) {
				rows.push(h('tr', null, h('td', null, to), h('td', { colspan: 6, class: 'sub' }, 'nothing said of its uplink yet')));
				continue;
			}
			for (const p of r.ports) {
				const s = summary(p.points);
				rows.push(h('tr', null, h('td', null, to), h('td', { class: 'mono' }, p.port), h('td', null, linkSaid(s)),
					h('td', null, share(s.rx, s.speed)), h('td', null, share(s.tx, s.speed)),
					h('td', { class: `mono${s.errors ? ' warn' : ''}` }, String(s.errors)), h('td', { class: `mono${s.lost ? ' warn' : ''}` }, String(s.lost))));
			}
		}
		box.replaceChildren(h('section', { class: 'panel' }, title,
			aps.length
				? h('table', { class: 'list' },
					h('tr', null, ['AP', 'Port', 'Link', 'Busiest in', 'Busiest out', 'Errors', 'Link lost'].map((c) => h('th', null, c))), rows)
				: h('div', { class: 'sub' }, 'No AP here yet.')));
	};
	await draw();
	return box;
}
