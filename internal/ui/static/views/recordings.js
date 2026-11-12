// The flight recorder (0207 part 3): what each AP below recorded when
// something went wrong on it (what was sent to its clients tried again all
// at once, many of them gone at once, a channel pinned full, power
// control's clients struggling) or when someone asked: what each five
// seconds came to, from ten minutes before to ten after. Listed under
// Radios › Load, the latest first; one opened shows, on the band chosen
// above the page, how busy each radio was and how much was tried again,
// what went wrong marked, and its clients before and after. Each radio's
// chart is drawn with Plotly (0294).

import { h, link } from '../dom.js';
import { get, post } from '../api.js';
import { bandName, ago } from '../format.js';
import { flash } from '../refresh.js';
import { mayAt } from '../rights.js';
import { called } from '../names.js';
import { chart as plot, at, token, series } from '../chart.js';

// Why, in a word or two.
const WHY = {
	retries: ['bad', 'tried again'], 'clients-left': ['bad', 'clients gone'], busy: ['warn', 'channel full'],
	'radios-down': ['bad', 'radios down'], 'radios-restart': ['bad', 'radios restarted'],
	distress: ['warn', 'clients struggled'], asked: ['idle', 'asked'],
};

// The recording opened, kept across the page's redrawing, and those read.
let opened = null;
const read = new Map();

const t = (x) => new Date(x).getTime();
const hhmm = (x) => new Date(x).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

// share is what of what was sent was tried again or lost, the larger of
// the two counts, at most all; null where too little was sent to say.
function share(p, least = 20) {
	const sent = p?.tx_packets || 0;
	return sent >= least ? Math.min(1, Math.max(p.tx_retries || 0, p.tx_failed || 0) / sent) : null;
}

// chart draws one radio over the recording: how busy its channel was and,
// dashed, how much of what was sent was tried again or lost, each 0 to
// 100%, every five seconds, broken where a point is missing; what went
// wrong marked.
function chart(id, radio, rec, rows) {
	const x = [], busy = [], again = [];
	let last = null;
	for (const r of rows) {
		if (last != null && r.t - last > 12e3) {
			x.push(at(last + 5e3));
			busy.push(null);
			again.push(null);
		}
		const a = share(r);
		x.push(at(r.t));
		busy.push(r.busy ?? null);
		again.push(a == null ? null : Math.round(a * 1000) / 10);
		last = r.t;
	}
	const wrong = at(t(rec.at));
	return plot({
		key: `recording:${id}:${radio}`, span: 0, name: `recording-${radio}`, height: 210,
		label: 'how busy its channel was, and what was tried again, over the recording',
		traces: [
			{ type: 'scatter', mode: 'lines', name: 'channel busy', x, y: busy, line: { color: series(1), width: 2 }, hovertemplate: '%{y}%' },
			{ type: 'scatter', mode: 'lines', name: 'tried again or lost', x, y: again, line: { color: series(2), width: 2, dash: 'dash' }, hovertemplate: '%{y}%' },
		],
		layout: {
			xaxis: { hoverformat: '%-I:%M:%S %p' },
			yaxis: { range: [0, 100], ticksuffix: '%', dtick: 25 },
			shapes: [{ type: 'line', x0: wrong, x1: wrong, yref: 'paper', y0: 0, y1: 1, line: { color: token('bad'), width: 1.5, dash: 'dot' } }],
			annotations: [{ x: wrong, yref: 'paper', y: 1, yanchor: 'top', xanchor: 'left', text: ' what went wrong', showarrow: false, font: { color: token('bad'), size: 11 } }],
		},
	});
}

// clientsTable says, of each client heard in the recording on the band,
// its signal before what went wrong, at it and at the end, what it said
// it hears of the AP, how much of what was sent to it was tried again
// before and after, and whether it was gone by the end.
function clientsTable(rec, points, band) {
	const at = t(rec.at), end = t(points[points.length - 1]?.at ?? rec.until);
	const by = new Map();
	for (const p of points) {
		for (const c of p.clients || []) {
			if (band && c.band !== band) continue;
			const k = c.mac;
			if (!by.has(k)) by.set(k, { mac: c.mac, host: c.host, ssid: c.ssid, list: [] });
			by.get(k).list.push({ ...c, t: t(p.at) });
		}
	}
	const sum = (list) => {
		const sent = list.reduce((a, c) => a + (c.tx_packets || 0), 0);
		const again = list.reduce((a, c) => a + Math.max(c.tx_retries || 0, c.tx_failed || 0), 0);
		return sent >= 20 ? `${Math.round(Math.min(100, (100 * again) / sent))}%` : '—';
	};
	const sig = (c) => (c?.signal != null ? `${c.signal}` : '—');
	const rows = [...by.values()].map((c) => {
		const before = c.list.filter((x) => x.t < at), after = c.list.filter((x) => x.t >= at);
		const atNow = c.list.reduce((best, x) => (Math.abs(x.t - at) < Math.abs((best?.t ?? Infinity) - at) ? x : best), null);
		const heard = [...c.list].reverse().find((x) => x.heard != null)?.heard;
		const gone = before.length > 0 && c.list[c.list.length - 1].t < end - 15e3;
		return { c, before, after, atNow, heard, gone };
	}).sort((a, b) => (b.gone - a.gone) || ((a.atNow?.signal ?? 0) - (b.atNow?.signal ?? 0)));
	if (!rows.length) return h('p', { class: 'sub pad' }, 'No client on this band in it.');
	return h('table', { class: 'list' },
		h('tr', null, ['Client', 'Signal before · at · end', 'Hears the AP', 'Tried again before · after', ''].map((x) => h('th', null, x))),
		rows.map(({ c, before, after, atNow, heard, gone }) => h('tr', null,
			h('td', null, called(c.mac, c.host) || h('span', { class: 'mono' }, c.mac), c.ssid && h('div', { class: 'sub' }, c.ssid)),
			h('td', { class: 'mono' }, `${sig(before[0])} · ${sig(atNow)} · ${sig(c.list[c.list.length - 1])} dBm`),
			h('td', { class: 'mono', title: 'as the client says itself (802.11k)' }, heard != null ? `${heard} dBm` : h('span', { class: 'sub' }, '—')),
			h('td', { class: 'mono' }, `${sum(before)} · ${sum(after)}`),
			h('td', null, gone && h('span', { class: 'chip bad' }, 'gone')))));
}

// recordingView draws one recording, on band.
function recordingView(id, band, redraw) {
	const got = read.get(id);
	const close = h('button', { type: 'button', class: 'button small', onclick: () => { opened = null; redraw(); } }, 'Close');
	if (!got) {
		get(`/v1/recordings/${id}`).then((r) => { read.set(id, r); redraw(); }).catch((e) => { read.set(id, { error: e.message }); redraw(); });
		return h('div', { class: 'sub pad' }, 'Reading the recording…');
	}
	if (got.error) return h('div', null, h('div', { class: 'error' }, got.error), close);
	const rec = got.recording, points = got.points || [];
	const radios = new Map();
	for (const p of points) {
		for (const r of p.radios || []) {
			if (band && r.band !== band) continue;
			if (!radios.has(r.radio)) radios.set(r.radio, { band: r.band, rows: [] });
			radios.get(r.radio).rows.push({ ...r, t: t(p.at) });
		}
	}
	const [chip, word] = WHY[rec.why] || ['idle', rec.why];
	return h('div', { class: 'recview' },
		h('h3', { class: 'subhead withcontrols' }, h('span', { class: 'title' }, `${rec.ap_name}, ${hhmm(rec.at)} `, h('span', { class: `chip ${chip}` }, word)),
			h('span', { class: 'controls' }, close)),
		rec.said && h('p', { class: 'sub pad' }, rec.said),
		h('p', { class: 'sub pad' }, `${points.length} points, every five seconds, ${hhmm(rec.from)} to ${hhmm(rec.until)}; the dotted line marks what went wrong.`),
		!radios.size && h('p', { class: 'sub pad' }, band ? `Nothing on ${bandName(band)} in it.` : 'No radio in it.'),
		[...radios.entries()].map(([name, r]) => h('div', null, h('div', { class: 'sub pad' }, `${bandName(r.band)} (${name})`), chart(id, name, rec, r.rows))),
		clientsTable(rec, points, band));
}

// recordingsPanel is the recordings of the APs below the node at, on band:
// the latest first, one opened below them, and a way to have them record
// now where the caller may.
export async function recordingsPanel(ctx, at, band, redraw) {
	const res = await get(`/v1/recordings?under=${encodeURIComponent(at.node)}`).catch(() => null);
	const recs = res?.recordings || [];
	const may = mayAt('locations', at.node, 'clients');
	const recordNow = may && h('button', {
		type: 'button', class: 'button small', title: 'Have the APs here send the last ten minutes and the next ten, every five seconds',
		onclick: async () => {
			try {
				const r = await post('/v1/recordings', { under: at.node });
				const heard = (r.asked || []).filter((a) => a.line).length;
				flash(`Asked ${heard} AP${heard === 1 ? '' : 's'} to record; ${(r.asked || []).length - heard} hold no line and were not told.`, 20);
			} catch (e) {
				flash(e.message, 20);
			}
			setTimeout(redraw, 4000);
		},
	}, 'Record now');
	const row = (r) => {
		const [chip, word] = WHY[r.why] || ['idle', r.why];
		return h('tr', { class: opened === r.id ? 'on' : null },
			h('td', { class: 'nowrap' }, ago(r.at), h('div', { class: 'sub' }, hhmm(r.at))),
			h('td', null, link(`/aps/${encodeURIComponent(r.ap)}/radios/load`, r.ap_name)),
			h('td', null, h('span', { class: `chip ${chip}` }, word)),
			h('td', { class: 'sub' }, r.said || ''),
			h('td', { class: 'mono' }, `${r.points}`),
			h('td', null, h('button', { type: 'button', class: 'button small', onclick: () => { opened = opened === r.id ? null : r.id; redraw(); } }, opened === r.id ? 'Close' : 'Open')));
	};
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, h('span', { class: 'title' }, 'Recordings',
			h('span', { class: 'note' }, 'when something went wrong, each AP sent ten minutes before and after, every five seconds')),
		h('span', { class: 'controls' }, recordNow)),
		!recs.length
			? h('p', { class: 'sub pad' }, 'Nothing recorded in the last week. An AP records by itself when what is sent to its clients is tried again all at once, many of them go at once, or its channel is pinned full.')
			: h('table', { class: 'list' },
				h('tr', null, ['When', 'AP', 'Why', 'What it said', 'Points', ''].map((x) => h('th', null, x))),
				recs.slice(0, 20).map(row)),
		opened != null && recs.some((r) => r.id === opened) && recordingView(opened, band, redraw));
}
