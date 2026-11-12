// A client's Overview (0337): its own dashboard, as a folder's Overview is
// one (Griff, 2026-11-07: "a general dashboard like the overall, but for
// the client itself"). A strip of how it is now and over the day, each
// cell leading to the tab that says more; its signal, rate and traffic over
// time, a minute at a time as its APs said them (0207); what to look at, and
// its latest attempts to come online. What it did, line by line, and where
// it stayed are its page's other tabs.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, size, ago } from '../format.js';
import { cell } from './overview.js';
import { took } from './roaming.js';
import { chart, at, token, bandColour, spanPicker, spanName, HOW } from '../chart.js';

// A signal at or under this, in dBm, is weak (Griff, 2026-10-30: -70 to -72
// is good; -80 is weak).
const WEAK = -80;
const DAY = 24 * 3600 * 1000;
const BANDS = ['2g', '5g', '6g'];
// The spans its charts look back over: its minutes are kept a week.
const SPANS = [[24, 'A day'], [168, 'A week']];

// How far back each client's charts look, kept over redraws.
const spans = new Map();

const middle = (xs) => {
	const s = xs.filter((x) => x != null && Number.isFinite(x)).sort((a, b) => a - b);
	return s.length ? s[Math.floor(s.length / 2)] : null;
};
const mbit = (n) => (n == null ? '—' : `${n >= 100 ? Math.round(n) : Math.round(n * 10) / 10} Mbit/s`);
const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

// minutesOf reads a client's minutes over hours (0207): those it was on an
// AP for any of. None from a manager that keeps none.
export async function minutesOf(mac, hours) {
	const r = await get(`/v1/minutes?mac=${encodeURIComponent(mac)}&hours=${hours}`).catch(() => null);
	return (r?.client?.minutes || []).filter((m) => m.secs == null || m.secs > 0);
}

// clientStrip is the strip over its Overview: where it is now, its signal,
// rate and traffic over the day, its attempts to come online and its roams.
// now is its session now, where it has one; latest its last; base its page.
export function clientStrip(ctx, j, res, mins, now, latest, base) {
	const since = Date.now() - DAY;
	const day = mins.filter((m) => new Date(m.at).getTime() >= since);
	const tries = (res.connections || []).filter((c) => new Date(c.started).getTime() >= since);
	// The day's attempts, 200 at most: where there are that many, there may
	// be more.
	const more = tries.length >= 200;
	const failed = tries.filter((c) => c.outcome === 'failed').length;
	const roams = tries.filter((c) => c.roam && !c.roam.band_switch);
	const switches = tries.filter((c) => c.roam?.band_switch).length;
	const roamMs = middle(roams.map((c) => c.roam.ms));
	const sessions = j.sessions || [];
	const signal = middle(day.map((m) => m.signal)) ?? middle(sessions.map((s) => s.signal_avg));
	const weakest = day.length ? Math.min(...day.map((m) => m.signal_min ?? m.signal).filter((x) => x != null)) : null;
	const down = day.reduce((x, m) => x + (m.tx_bytes || 0), 0);
	const up = day.reduce((x, m) => x + (m.rx_bytes || 0), 0);
	const toRate = middle(day.map((m) => m.tx_rate));
	const fromRate = middle(day.map((m) => m.rx_rate));
	return h('section', { class: 'nowstrip', 'aria-label': 'This client now' },
		now ? cell('Now on', now.name || ctx.name('locations', now.ap), [bandName(now.band), now.signal_avg != null && `${now.signal_avg} dBm`, now.ssid || now.network].filter(Boolean).join(' · '),
			'', `#/aps/${encodeURIComponent(now.ap)}/clients`)
			: cell('Now', 'Not on', latest ? `last seen ${ago(latest.to)} on ${latest.name}` : 'not seen in this time', 'warn'),
		cell('Signal, 24 h', signal != null ? `${signal} dBm` : '—', weakest != null && Number.isFinite(weakest) ? `typical · weakest ${weakest} dBm` : 'typical',
			signal != null && signal <= WEAK ? 'warn' : '', `${base}/stays`),
		cell('Rate, 24 h', toRate != null ? mbit(toRate) : '—', fromRate != null ? `to it, typically · from it ${mbit(fromRate)}` : 'to it, typically'),
		cell('Joins, 24 h', `${more ? `${tries.length}+` : tries.length}`, tries.length ? (failed ? `${failed} failed` : 'every one got on') : 'none',
			failed ? 'warn' : '', `${base}/activity`),
		cell('Roams, 24 h', roams.length ? String(roams.length) : 'none',
			[roamMs != null && `typically ${took(roamMs)}`, switches && plural(switches, 'band switch', 'band switches')].filter(Boolean).join(' · ') || 'it stayed on its AP',
			'', `${base}/activity`),
		cell('Traffic, 24 h', `${size(down)} down`, `${size(up)} up`));
}

// gaps puts a break in a line where its minutes stop for more than two: it
// was off, or elsewhere, and the line is not drawn across.
function gaps(rows) {
	const out = [];
	let last = 0;
	for (const r of rows) {
		const t = new Date(r.at).getTime();
		if (last && t - last > 2.5 * 60000) out.push({ gap: true, at: new Date(last + 60000).toISOString() });
		out.push(r);
		last = t;
	}
	return out;
}

// buckets folds minutes into spans of n minutes for a week's view: the
// signal and rates in the middle, the bytes summed.
function buckets(mins, n) {
	if (n <= 1) return mins;
	const by = new Map();
	for (const m of mins) {
		const t = Math.floor(new Date(m.at).getTime() / (n * 60000)) * n * 60000;
		if (!by.has(t)) by.set(t, []);
		by.get(t).push(m);
	}
	return [...by.entries()].sort((a, b) => a[0] - b[0]).map(([t, ms]) => ({
		at: new Date(t).toISOString(),
		band: ms[ms.length - 1].band, ap_name: ms[ms.length - 1].ap_name,
		signal: middle(ms.map((m) => m.signal)), tx_rate: middle(ms.map((m) => m.tx_rate)), rx_rate: middle(ms.map((m) => m.rx_rate)),
		tx_bytes: ms.reduce((x, m) => x + (m.tx_bytes || 0), 0), rx_bytes: ms.reduce((x, m) => x + (m.rx_bytes || 0), 0),
	}));
}

// clientChart is its signal, its rates and its traffic over the span, one
// above another on one time axis: the signal a line for each band, so a
// move between bands shows; the rates to it and from it; what it moved
// each way. Each point says the AP it was on.
export async function clientChart(mac, pick) {
	const hours = spans.get(mac) || 24;
	const mins = await minutesOf(mac, hours);
	const head = h('h2', { title: `From its APs' minutes (0207): its signal as the AP heard it, the rates each way, and what it moved. ${HOW}` },
		h('span', { class: 'title' }, 'Over time', spanPicker(hours, (v) => { spans.set(mac, v); pick(); }, SPANS)));
	if (!mins.length)
		return h('section', { class: 'panel main' }, head, h('p', { class: 'sub pad' }, `No AP you can see has a minute of it over the last ${spanName(hours, SPANS)}.`));
	const n = hours > 24 ? 10 : 1;
	const rows = gaps(buckets(mins, n));
	const x = rows.map((r) => at(r.at));
	const where = rows.map((r) => (r.gap ? '' : `${r.ap_name || ''} · ${bandName(r.band) || ''}`));
	const signal = BANDS.filter((b) => mins.some((m) => m.band === b)).map((b) => ({
		type: 'scatter', mode: 'lines', name: `${bandName(b)} signal`, x,
		y: rows.map((r) => (!r.gap && r.band === b ? r.signal : null)), customdata: where,
		hovertemplate: '%{y} dBm · %{customdata}', line: { color: bandColour(b), width: 1.5 }, connectgaps: false,
	}));
	const rate = (k, name, colour) => ({
		type: 'scatter', mode: 'lines', name, x, yaxis: 'y2',
		y: rows.map((r) => (r.gap ? null : r[k] ?? null)), hovertemplate: '%{y:.0f} Mbit/s', line: { color: colour, width: 1.5 }, connectgaps: false,
	});
	const bytes = (k, name, colour) => ({
		type: 'bar', name, x, yaxis: 'y3', y: rows.map((r) => (r.gap ? null : r[k] || 0)),
		customdata: rows.map((r) => size(r[k] || 0)), hovertemplate: '%{customdata}', marker: { color: colour },
		xperiod: n * 60000, xperiodalignment: 'middle',
	});
	const accent = token('accent');
	const plot = chart({
		key: `client:${mac}`, span: hours, name: `client-${mac.replace(/:/g, '')}`, height: 380,
		label: `Its signal, its rates and its traffic over ${spanName(hours, SPANS)}`,
		traces: [...signal,
			rate('tx_rate', 'rate to it', accent), rate('rx_rate', 'rate from it', token('accent-faint')),
			bytes('tx_bytes', '↓ down', accent), bytes('rx_bytes', '↑ up', token('accent-faint'))],
		layout: {
			barmode: 'stack', bargap: 0.1,
			xaxis: { anchor: 'y3' },
			yaxis: { domain: [0.7, 1], rangemode: 'normal', ticksuffix: ' dBm' },
			yaxis2: { domain: [0.38, 0.64], ticksuffix: ' M' },
			yaxis3: { domain: [0, 0.3], tickformat: '~s', ticksuffix: 'B' },
		},
	});
	return h('section', { class: 'panel main' }, head,
		h('div', { class: 'sub usagekey' }, `${n > 1 ? `In ${n}-minute spans` : 'A minute at a time'}: its signal as the AP heard it, by band; the rates to it and from it; what it moved down and up`),
		plot);
}

// lately is its latest attempts to come online, a line each, each leading
// to itself on the Activity tab, opened.
export function lately(ctx, mac, res, base) {
	const list = (res.connections || []).slice(0, 6);
	const OUT = { online: ['ok', 'online'], connected: ['ok', 'on'], failed: ['bad', 'failed'], left: ['warn', 'gave up'] };
	return h('section', { class: 'panel' },
		h('h2', null, 'Lately', h('span', { class: 'note' }, link(`${base.slice(1)}/activity`, 'everything it did'))),
		list.length
			? h('ul', { class: 'attention' }, list.map((c) => h('li', null,
				h('span', { class: `chip ${OUT[c.outcome]?.[0] || 'idle'}` }, OUT[c.outcome]?.[1] || c.outcome), ' ',
				h('a', { href: `${base}/activity/${c.id}` }, [c.ap_name, bandName(c.band)].filter(Boolean).join(' · ')),
				' ', h('span', { class: 'sub' }, [c.roam ? (c.roam.band_switch ? 'band switch' : 'roam') : 'join', ago(c.started)].join(' · ')))))
			: h('p', { class: 'sub pad' }, 'No attempt to come online is recorded for it.'));
}
