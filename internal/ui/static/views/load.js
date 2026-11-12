// Load (0207): how busy each AP's channel was, minute by minute, on the band
// chosen above the page (Griff, 2026-10-22): the last minute, on average and
// at its busiest 5 s; the last hour as a line; its clients, what was sent
// to them that was tried again or lost, and its noise. Each AP says what
// each minute came to at the minute's end. Watch live has them say what
// each five seconds comes to as well, while it is open (livewatch.js).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, ago } from '../format.js';
import { redrawNow } from '../refresh.js';
import { livePanel, startWatching, watchingAt } from './livewatch.js';
import { recordingsPanel } from './recordings.js';

const SVG = 'http://www.w3.org/2000/svg';

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

const pct = (v) => (v == null ? '—' : `${Math.round(v)}%`);

// hourLine draws a radio's last hour: how busy its channel was each minute,
// and faintly, its busiest 5 s; a minute it did not say breaks the line.
function hourLine(minutes) {
	const W = 160, H = 26, end = Date.now(), start = end - 3600e3;
	const x = (m) => (((new Date(m.at).getTime() + 30e3 - start) / (end - start)) * W).toFixed(1);
	const y = (v) => (H - 1 - (Math.min(100, Math.max(0, v)) / 100) * (H - 2)).toFixed(1);
	const runs = (key) => {
		const out = [];
		let run = [], last = null;
		for (const m of minutes) {
			const t = new Date(m.at).getTime();
			if (t < start || m[key] == null || (last != null && t - last > 90e3)) {
				if (run.length > 1) out.push(run);
				run = [];
			}
			if (t >= start && m[key] != null) run.push(`${x(m)},${y(m[key])}`);
			last = t;
		}
		if (run.length > 1) out.push(run);
		return out;
	};
	const busiest = Math.max(0, ...minutes.map((m) => m.busy_max ?? 0));
	return svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'spark loadline', role: 'img', 'aria-label': `the last hour: at its busiest ${Math.round(busiest)}%` },
		svg('title', null, `The last hour, a minute a point: how busy the channel was, and faintly, its busiest 5 s. At its busiest ${Math.round(busiest)}%.`),
		svg('line', { x1: 0, x2: W, y1: y(50), y2: y(50), class: 'half' }),
		runs('busy_max').map((r) => svg('polyline', { class: 'peak', points: r.join(' ') })),
		runs('busy').map((r) => svg('polyline', { class: 'busy', points: r.join(' ') })));
}

// lately is the share of what was sent to a radio's clients over the last
// five minutes it said that was tried again or lost, for retries that one
// minute's few packets would make jumpy: the larger of the two counts each
// minute, a frame lost having been tried again first, and mt76 saying one
// count as both (2026-10-22), at most all of it.
function lately(minutes) {
	const recent = minutes.slice(-5);
	const sent = recent.reduce((a, m) => a + (m.tx_packets || 0), 0);
	const again = recent.reduce((a, m) => a + Math.max(m.tx_retries || 0, m.tx_failed || 0), 0);
	return sent >= 50 ? Math.min(100, (100 * again) / sent) : null;
}

// loadSection draws the page for the node at ({ node, nodeName, page }),
// on band.
export async function loadSection(ctx, at, band) {
	const res = await get(`/v1/minutes?under=${encodeURIComponent(at.node)}${band ? `&band=${band}` : ''}`).catch(() => null);
	if (!res) return h('section', { class: 'panel' }, h('div', { class: 'sub' }, 'What each minute came to could not be read.'));
	const radios = (res.radios || []).slice().sort((a, b) => a.ap_name.localeCompare(b.ap_name) || a.radio.localeCompare(b.radio));
	const lines = radios.map((r) => {
		const last = r.minutes[r.minutes.length - 1];
		const busyTitle = last && `${pct(last.busy)} busy over the minute: its own sending ${pct(last.tx)}, receiving ${pct(last.rx)}, the rest others; at its busiest 5 s ${pct(last.busy_max)}`;
		const retried = lately(r.minutes);
		return h('tr', null,
			h('td', null, link(`/aps/${encodeURIComponent(r.ap)}/radios/load`, r.ap_name)),
			h('td', { class: 'mono' }, last?.channel ? `${last.channel}${last.width ? ` · ${last.width} MHz` : ''}` : '—'),
			h('td', { class: 'mono', title: busyTitle }, pct(last?.busy), last?.busy_max != null && h('span', { class: 'sub' }, `, peak ${pct(last.busy_max)}`)),
			h('td', null, hourLine(r.minutes)),
			h('td', { class: 'mono' }, last?.clients != null ? String(Math.round(last.clients)) : '—', last?.clients_max > Math.round(last?.clients ?? 0) && h('span', { class: 'sub' }, ` (up to ${last.clients_max})`)),
			h('td', { class: 'mono', title: 'Of what was sent to its clients over the last five minutes, the share tried again or lost' },
				retried == null ? h('span', { class: 'sub' }, 'little sent') : `${retried.toFixed(1)}%`),
			h('td', { class: 'mono' }, last?.noise != null ? `${last.noise} dBm` : '—'),
			h('td', { class: 'sub nowrap' }, last ? ago(new Date(new Date(last.at).getTime() + 60e3)) : '—'));
	});
	// Watched live (0207 part 2): every five seconds, above the minutes;
	// and the flight recorder's recordings below them (0207 part 3).
	const live = livePanel(at.node, band, redrawNow);
	const recordings = await recordingsPanel(ctx, at, band, redrawNow);
	return [live, h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, h('span', { class: 'title' }, `${band ? bandName(band) : 'Radio'} load`,
			h('span', { class: 'note' }, 'what each minute came to, as each AP said at its end')),
		!watchingAt(at.node) && h('span', { class: 'controls' }, h('button', {
			type: 'button', class: 'button small', title: 'Have the APs here send what each five seconds comes to, while this is open',
			onclick: () => startWatching(at.node, redrawNow),
		}, 'Watch live'))),
		radios.length
			? h('table', { class: 'list' },
				h('tr', null, ['AP', 'Channel', 'Busy, last minute', 'Last hour', 'Clients', 'Retried', 'Noise', 'Said'].map((c) => h('th', null, c))),
				lines)
			: h('div', { class: 'sub' }, band
				? `No AP here has said what a minute came to on ${bandName(band)} yet. Each says it at every minute's end.`
				: 'No AP here has said what a minute came to yet.')), recordings];
}
