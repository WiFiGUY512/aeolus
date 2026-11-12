// The live view (0207 part 2): while it is open, the APs below the node it
// was opened on send what each five seconds comes to on their radios, and
// it is drawn as it comes: how busy each channel is now and over the last
// five minutes, and each client's signal, rate, retries and data. One band
// at a time, the one chosen above the page (Griff, 2026-10-22). The clients
// moving the most data come first, the top three, five or eight, each with
// its numbers and a line of what it moved over the last five minutes, as
// the radios have theirs; the rest are rolled up below (Griff, 2026-10-23).
//
// It holds its answer open across the page's own redrawing: the page drawn
// again finds it still watching and draws it again where it now is. It
// keeps the page from redrawing under it (data-editing), as a form does,
// and stops when its Stop is pressed or the page is left.

import { h, link } from '../dom.js';
import { lines } from '../api.js';
import { bandName } from '../format.js';
import { called } from '../names.js';

const SVG = 'http://www.w3.org/2000/svg';
const KEEP = 60;        // points kept a radio: five minutes
const CLIENTS = 100;    // clients listed, those moving the most data first
const TOPS = [3, 5, 8];  // how many of them come first, each with its line
const GONE_AFTER = 15e3; // ms a client said nothing of is taken to have gone

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

const pct = (v) => (v == null ? '—' : `${Math.round(v)}%`);

// One watching at a time: { node, stop, aps, radios, clients, at, said,
// box, band }. radios and clients are by "ap radio" and "ap mac", the last
// KEEP points of each.
let watch = null;

// How many clients come first, and whether the rest are unrolled.
let top = 5;
let restOpen = false;

// watchingAt says whether the live view is open for a node.
export function watchingAt(node) {
	return watch?.node === node;
}

// startWatching opens the live view for a node; redraw draws the page.
export function startWatching(node, redraw) {
	stopWatching();
	const w = watch = { node, stop: new AbortController(), aps: [], radios: new Map(), clients: new Map(), at: null, said: null, box: null, band: null };
	const take = (v) => {
		if (watch !== w) return;
		if (v.every) {
			w.aps = v.aps || [];
		} else if (v.ap) {
			w.at = new Date(v.at).getTime();
			for (const r of v.radios || []) {
				const k = `${v.ap} ${r.radio}`;
				const list = w.radios.get(k) || [];
				list.push({ ...r, ap: v.ap, ap_name: v.ap_name, t: new Date(v.at).getTime() });
				list.sort((a, b) => a.t - b.t);
				w.radios.set(k, list.slice(-KEEP));
			}
			for (const c of v.clients || []) {
				const k = `${v.ap} ${c.mac}`;
				const list = w.clients.get(k) || [];
				list.push({ ...c, ap: v.ap, ap_name: v.ap_name, t: new Date(v.at).getTime() });
				w.clients.set(k, list.slice(-KEEP));
			}
		} else {
			return;
		}
		draw(w);
	};
	(async () => {
		while (watch === w) {
			try {
				await lines(`/v1/live?under=${encodeURIComponent(node)}`, take, w.stop.signal);
				w.said = null;
			} catch (e) {
				if (watch !== w) return;
				w.said = e.message;
				draw(w);
				if (e.status === 401 || e.status === 403) return;
			}
			await new Promise((r) => setTimeout(r, 2000));
		}
	})();
	redraw();
}

export function stopWatching() {
	watch?.stop.abort();
	watch = null;
}

// spark draws a radio's last five minutes, a point every five seconds: how
// busy its channel was.
function spark(points) {
	const W = 160, H = 26, end = Date.now(), start = end - KEEP * 5e3;
	const x = (p) => (((p.t + 2500 - start) / (end - start)) * W).toFixed(1);
	const y = (v) => (H - 1 - (Math.min(100, Math.max(0, v)) / 100) * (H - 2)).toFixed(1);
	const runs = [];
	let run = [], last = null;
	for (const p of points) {
		if (p.busy == null || p.t < start || (last != null && p.t - last > 12e3)) {
			if (run.length > 1) runs.push(run);
			run = [];
		}
		if (p.busy != null && p.t >= start) run.push(`${x(p)},${y(p.busy)}`);
		last = p.t;
	}
	if (run.length > 1) runs.push(run);
	return svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'spark loadline', role: 'img', 'aria-label': 'the last five minutes' },
		svg('title', null, 'The last five minutes, a point every five seconds: how busy the channel was.'),
		svg('line', { x1: 0, x2: W, y1: y(50), y2: y(50), class: 'half' }),
		runs.map((r) => svg('polyline', { class: 'busy', points: r.join(' ') })));
}

// rate is data moved each way over a point, in Mbit/s; mbit says it.
const rate = (p) => ((p.tx_bytes || 0) + (p.rx_bytes || 0)) * 8 / Math.max(1, p.secs || 5) / 1e6;
const mbit = (v) => (v >= 0.1 ? `${v.toFixed(1)} Mbit/s` : v > 0 ? `${Math.round(v * 1000)} kbit/s` : 'idle');

// dataSpark draws a client's last five minutes, a point every five
// seconds: what it moved, on its own scale from none to its most.
function dataSpark(points) {
	const W = 160, H = 26, end = Date.now(), start = end - KEEP * 5e3;
	const shown = points.filter((p) => p.t >= start);
	const most = Math.max(0, ...shown.map(rate));
	const x = (p) => (((p.t + 2500 - start) / (end - start)) * W).toFixed(1);
	const y = (v) => (H - 1 - (most > 0 ? v / most : 0) * (H - 2)).toFixed(1);
	const runs = [];
	let run = [], last = null;
	for (const p of shown) {
		if (last != null && p.t - last > 12e3) {
			if (run.length > 1) runs.push(run);
			run = [];
		}
		run.push(`${x(p)},${y(rate(p))}`);
		last = p.t;
	}
	if (run.length > 1) runs.push(run);
	return svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'spark loadline', role: 'img', 'aria-label': `the last five minutes: at most ${mbit(most)}` },
		svg('title', null, `The last five minutes, a point every five seconds: what it moved, at most ${mbit(most)}.`),
		runs.map((r) => svg('polyline', { class: 'busy', points: r.join(' ') })));
}

// again is the share of what was sent that was tried again or lost, over
// points, where enough was sent to say: the larger of the two counts each
// time, as the minutes have it.
function again(points) {
	const sent = points.reduce((a, p) => a + (p.tx_packets || 0), 0);
	const tried = points.reduce((a, p) => a + Math.max(p.tx_retries || 0, p.tx_failed || 0), 0);
	return sent >= 20 ? Math.min(100, (100 * tried) / sent) : null;
}


// draw draws the live view where it now is, if it is. One off the page for
// ten seconds, as when another page is opened, stops: the APs need not go
// on sending for no one.
function draw(w) {
	if (w.box?.isConnected) {
		w.lost = null;
		w.box.replaceChildren(...body(w));
		return;
	}
	w.lost ??= Date.now();
	if (Date.now() - w.lost > 10e3 && watch === w) stopWatching();
}

function body(w) {
	const band = w.band;
	const now = Date.now();
	const radios = [...w.radios.values()].filter((l) => !band || l[l.length - 1].band === band)
		.sort((a, b) => a[0].ap_name.localeCompare(b[0].ap_name) || a[0].radio.localeCompare(b[0].radio));
	// The clients by what they moved over the last 30 seconds, the most
	// first: the top few with their lines, the rest rolled up.
	const moved = (l) => l.slice(-6).reduce((a, p) => a + rate(p), 0);
	const lists = [...w.clients.values()].filter((l) => { const c = l[l.length - 1]; return (!band || c.band === band) && now - c.t < GONE_AFTER; })
		.sort((a, b) => moved(b) - moved(a)).slice(0, CLIENTS);
	const pick = h('span', { class: 'segmented', role: 'group', 'aria-label': 'How many clients come first' }, TOPS.map((n) => h('button', {
		type: 'button', class: n === top ? 'on' : null, 'aria-pressed': String(n === top), onclick: () => { top = n; draw(w); },
	}, `Top ${n}`)));
	const head = (line) => h('tr', null, ['Client', 'AP', 'Signal', 'Hears the AP', 'Rate', 'Retried, 30 s', 'Data now', ...(line ? ['Last 5 minutes'] : [])].map((c) => h('th', null, c)));
	const row = (l, line) => {
		const c = l[l.length - 1], r = again(l.slice(-6)), mbps = rate(c);
		return h('tr', null,
			h('td', null, called(c.mac, c.host) || h('span', { class: 'mono' }, c.mac), c.ssid && h('div', { class: 'sub' }, c.ssid)),
			h('td', null, c.ap_name),
			h('td', { class: 'mono', title: 'how strongly the AP hears it' }, c.signal != null ? `${c.signal} dBm` : '—'),
			// As it says itself (802.11k), where it can.
			h('td', { class: 'mono', title: 'as the client says itself, when asked (802.11k)' }, c.heard != null ? `${c.heard} dBm` : h('span', { class: 'sub' }, '—')),
			h('td', { class: 'mono', title: 'what the AP sends it at, and what it sends at' }, c.tx_rate != null ? `${Math.round(c.tx_rate)}` : '—',
				h('span', { class: 'sub' }, c.rx_rate != null ? ` / ${Math.round(c.rx_rate)} Mbit/s` : ' Mbit/s')),
			h('td', { class: 'mono' }, r == null ? h('span', { class: 'sub' }, 'little sent') : `${r.toFixed(1)}%`),
			h('td', { class: 'mono' }, mbps > 0 ? mbit(mbps) : h('span', { class: 'sub' }, 'idle')),
			line && h('td', null, dataSpark(l)));
	};
	const tops = lists.slice(0, top), rest = lists.slice(top);
	const rolled = h('details', { class: 'fold', open: restOpen },
		h('summary', null, h('span', { class: 'sub' }, `The other ${rest.length} client${rest.length === 1 ? '' : 's'}`)),
		h('table', { class: 'list' }, head(false), rest.map((l) => row(l, false))));
	rolled.addEventListener('toggle', () => { restOpen = rolled.open; });
	const sending = w.aps.filter((a) => a.line).length;
	const fresh = w.at != null ? Math.round((now - w.at) / 1000) : null;
	return [
		h('h2', { class: 'withcontrols' }, h('span', { class: 'title' }, `Live${band ? `, ${bandName(band)}` : ''}`,
			h('span', { class: 'note' }, `every 5 s; ${sending} of ${w.aps.length} AP${w.aps.length === 1 ? '' : 's'} holding their line${fresh != null ? `; the latest ${fresh} s ago` : ''}`)),
		h('span', { class: 'controls' }, h('button', { type: 'button', class: 'button small', onclick: () => { stopWatching(); w.redraw?.(); } }, 'Stop'))),
		w.said && h('div', { class: 'error' }, w.said),
		!radios.length && h('p', { class: 'sub pad' }, sending ? 'Asked. The first comes within five seconds.' : 'No AP here holds its line to the manager, so none can send live.'),
		radios.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['AP', 'Channel', 'Busy now', 'Last 5 minutes', 'Clients', 'Retried, 30 s', 'Noise', 'Power'].map((c) => h('th', null, c))),
			radios.map((l) => {
				const p = l[l.length - 1];
				const r = again(l.slice(-6));
				return h('tr', null,
					h('td', null, link(`/aps/${encodeURIComponent(p.ap)}/radios/load`, p.ap_name)),
					h('td', { class: 'mono' }, p.channel ? `${p.channel}${p.width ? ` · ${p.width} MHz` : ''}` : '—'),
					h('td', { class: 'mono', title: `its own sending ${pct(p.tx)}, receiving ${pct(p.rx)}, the rest others` }, pct(p.busy)),
					h('td', null, spark(l)),
					h('td', { class: 'mono' }, p.clients != null ? String(Math.round(p.clients)) : '—'),
					h('td', { class: 'mono' }, r == null ? h('span', { class: 'sub' }, 'little sent') : `${r.toFixed(1)}%`),
					h('td', { class: 'mono' }, p.noise != null ? `${p.noise} dBm` : '—'),
					h('td', { class: 'mono' }, p.power != null ? `${p.power} dBm` : '—'));
			})),
		tops.length > 0 && h('h3', { class: 'subhead withcontrols' }, h('span', { class: 'title' }, 'The clients moving the most data'), h('span', { class: 'controls' }, pick)),
		tops.length > 0 && h('table', { class: 'list' }, head(true), tops.map((l) => row(l, true))),
		rest.length > 0 && rolled,
	];
}

// livePanel is the live view for the node at, on band, as the page is
// drawn: null where it is not open there. It keeps the page from being
// drawn again under it.
export function livePanel(node, band, redraw) {
	const w = watch;
	if (!w || w.node !== node) return null;
	w.band = band;
	w.redraw = redraw;
	w.box = h('section', { class: 'panel', 'data-editing': true });
	draw(w);
	return w.box;
}
