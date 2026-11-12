// Beacons (0350): how each radio's beacons went over time, on the band
// chosen above the page, from what the manager keeps of each state report
// (0339): the share that did not go out, and how many went out a second.
// Beside each radio, its last half hour as the beacons alert judges it now;
// on the chart, the stretches whose half hour the alert judges one. Only
// ath11k radios count their beacons (0221). Drawn with Plotly (0294).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName } from '../format.js';
import { chart, at as plotTime, token, series, bandColour, spanPicker, spanName, HOW } from '../chart.js';

// The spans offered: the last few hours from the pulldown, a day, and a
// week, the most the manager reads, in half hours.
const SPANS = [[24, 'A day'], [168, 'A week']];
// The span each node's beacons were last drawn over, kept over a redraw.
const over = new Map();

const n = (v) => Number(v || 0).toLocaleString();
// share is the percent of the beacons tried that did not go out; null
// where none were.
const share = (sent, failed) => (sent + failed > 0 ? (100 * failed) / (sent + failed) : null);
// pct says two places under 1%, one from there.
const pct = (p) => (p == null ? '—' : `${p >= 0.995 ? p.toFixed(1) : p.toFixed(2)}%`);
const minutes = (s) => `${Math.round(s / 60)} minutes`;
const when = (t, hours) => new Date(t).toLocaleString([], hours > 24
	? { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' } : { hour: 'numeric', minute: '2-digit' });

// beaconsSection draws the beacons of the radios below the node at
// ({ node }), on band; oneAP says the node is an AP, whose radios are named
// by their own names. Another span chosen draws it again in its place.
export async function beaconsSection(at, band, oneAP) {
	let box = null, asked = 0;
	const pick = async (hours) => {
		over.set(at.node, hours);
		const mine = ++asked;
		const next = await drawn(at, band, oneAP, pick);
		if (mine !== asked || !box) return;
		box.replaceWith(next);
		box = next;
	};
	box = await drawn(at, band, oneAP, pick);
	return box;
}

async function drawn(at, band, oneAP, pick) {
	const hours = over.get(at.node) || SPANS[0][0];
	const res = await get(`/v1/beacons?under=${encodeURIComponent(at.node)}&hours=${hours}${band ? `&band=${band}` : ''}`).catch(() => null);
	const head = h('h2', { title: `From the APs' state reports, every five minutes: of the beacons each radio tried to send, the share that did not go out, and under it how many went out a second, about ten for each network on the air. ${HOW}` },
		h('span', { class: 'title' }, `${band ? bandName(band) : 'Radio'} beacons`, spanPicker(hours, pick, SPANS)),
		h('span', { class: 'note' }, 'the share that did not go out, by radio'));
	if (!res) return h('section', { class: 'panel' }, head, h('p', { class: 'sub pad' }, 'The beacons could not be read.'));
	const radios = res.radios || [];
	if (!radios.length) {
		return h('section', { class: 'panel' }, head, h('p', { class: 'sub pad' },
			`No radio here counted its beacons${band ? ` on ${bandName(band)}` : ''} over ${spanName(hours, SPANS)}. Only ath11k radios count them, and say them in each state report.`));
	}
	const rule = res.rule;
	// An AP named once where it has one radio here; its radio too where it
	// has more; and on an AP's own page, the radio alone.
	const many = new Set(radios.filter((r, i) => radios.findIndex((o) => o.ap === r.ap) !== i).map((r) => r.ap));
	const label = (r) => (oneAP ? r.radio : many.has(r.ap) ? `${r.ap_name} ${r.radio}` : r.ap_name);
	const colour = (i) => (radios.length === 1 ? bandColour(radios[0].band) : series((i % 8) + 1));
	return h('section', { class: 'panel' },
		head,
		h('div', { class: 'sub usagekey', title: `The beacons alert: ${rule.pct}% or more over the last ${minutes(rule.window)}, and at least ${rule.fewest}; `
			+ `${rule.stuck_pct}% while its counts cover under ${minutes(rule.span)}. A ring marks a stretch whose half hour, ending with it, is an alert.` },
		`Alert at ${rule.pct}% over ${Math.round(rule.window / 60)} min, at least ${rule.fewest} (${rule.stuck_pct}% under ${Math.round(rule.span / 60)} min of counts) · ○ where the half hour ending there was one`),
		plot(at, res, radios, label, colour),
		h('table', { class: 'list' },
			h('tr', null, [oneAP ? 'Radio' : 'AP', 'Last half hour', `Over ${spanName(hours, SPANS)}`, `Worst ${Math.round(res.step / 60)} min`].map((c) => h('th', null, c))),
			radios.map((r) => row(r, rule, oneAP, many, hours))),
		res.cut && h('p', { class: 'sub pad' }, 'Not every radio here is shown: one asking reads the first 50 APs.'));
}

// row is a radio's line of the table: its last half hour as the alert
// judges it, the span's share, and its worst stretch.
function row(r, rule, oneAP, many, hours) {
	const hl = r.half;
	const tried = hl ? hl.sent + hl.failed : 0;
	const worst = r.points.filter((p) => p.sent + p.failed >= 100)
		.reduce((w, p) => (share(p.sent, p.failed) > (w ? share(w.sent, w.failed) : -1) ? p : w), null);
	return h('tr', null,
		h('td', null, oneAP ? h('span', { class: 'mono' }, r.radio)
			: [link(`/aps/${encodeURIComponent(r.ap)}/radios/beacons`, r.ap_name), many.has(r.ap) && h('span', { class: 'sub mono' }, ` ${r.radio}`)]),
		h('td', null, hl
			? [h('span', {
				class: `chip ${hl.alert ? 'warn' : 'ok'}`,
				title: `${n(hl.failed)} of ${n(tried)} did not go out over the last ${minutes(hl.over)}. The beacons alert is at ${hl.alert_at}%, and at least ${rule.fewest}: ${hl.alert ? 'this is one' : 'this is not one'}.`,
			}, pct(share(hl.sent, hl.failed))), h('span', { class: 'sub' }, ` ${n(hl.failed)} of ${n(tried)}`)]
			: h('span', { class: 'sub', title: 'It sent no report of its beacons in the last half hour' }, '—')),
		h('td', null, pct(share(r.sent, r.failed)), h('span', { class: 'sub' }, ` ${n(r.failed)} of ${n(r.sent + r.failed)}`)),
		h('td', null, worst ? [pct(share(worst.sent, worst.failed)), h('span', { class: 'sub' }, ` at ${when(worst.at, hours)}`)] : '—'));
}

// plot draws each radio's share that did not go out in each stretch, a line
// a radio broken where it sent nothing; the stretches whose half hour is an
// alert marked; and under them, in a strip of its own, how many went out a
// second. A radio's two lines are one name in the key.
function plot(at, res, radios, label, colour) {
	if (!radios.some((r) => r.points.length > 1)) return h('p', { class: 'sub pad' }, 'A chart needs two reports; they come every five minutes.');
	const step = res.step * 1000;
	const from = new Date(res.from).getTime();
	const count = Math.round((res.hours * 3600e3) / step);
	const x = Array.from({ length: count }, (_, i) => plotTime(from + i * step));
	const period = { xperiod: step, xperiodalignment: 'middle' };
	const traces = [];
	const marks = { x: [], y: [] };
	radios.forEach((r, i) => {
		const failed = Array(count).fill(null), rate = Array(count).fill(null), said = Array(count).fill('');
		for (const p of r.points) {
			const j = Math.round((new Date(p.at).getTime() - from) / step);
			if (j < 0 || j >= count) continue;
			failed[j] = share(p.sent, p.failed);
			rate[j] = p.over > 0 ? p.sent / p.over : null;
			said[j] = `${pct(failed[j])} · ${n(p.failed)} of ${n(p.sent + p.failed)} · ${rate[j] == null ? '—' : rate[j].toFixed(1)} sent a second${p.alert ? ' · alert' : ''}`;
			if (p.alert) {
				marks.x.push(x[j]);
				marks.y.push(failed[j]);
			}
		}
		const name = label(r), group = `${r.ap} ${r.radio} ${r.band}`, line = { color: colour(i) };
		traces.push(
			{ type: 'scatter', mode: 'lines', name, legendgroup: group, x, y: failed, customdata: said, hovertemplate: '%{customdata}', line: { ...line, width: 2 }, ...period },
			{ type: 'scatter', mode: 'lines', name, legendgroup: group, showlegend: false, x, y: rate, yaxis: 'y2', hoverinfo: 'skip', line: { ...line, width: 1.5 }, ...period });
	});
	if (marks.x.length) {
		traces.push({ type: 'scatter', mode: 'markers', name: 'an alert over its half hour', x: marks.x, y: marks.y, hoverinfo: 'skip',
			marker: { color: token('bad'), size: 7, symbol: 'circle-open', line: { width: 2 } }, ...period });
	}
	const worst = Math.max(0, ...radios.flatMap((r) => r.points.map((p) => share(p.sent, p.failed) ?? 0)));
	return chart({
		key: `beacons:${at.node}`, span: res.hours, name: 'beacons', height: 270,
		label: `Of each radio's beacons, the share that did not go out over ${spanName(res.hours, SPANS)}: at worst ${pct(worst)}`,
		traces,
		layout: {
			// A key where there is more than one line to tell apart.
			showlegend: radios.length > 1 || marks.x.length > 0,
			xaxis: { anchor: 'y2' },
			yaxis: { domain: [0.3, 1], ticksuffix: '%' },
			yaxis2: { domain: [0, 0.2], ticksuffix: '/s', nticks: 3 },
		},
	});
}
