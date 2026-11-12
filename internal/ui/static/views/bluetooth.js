// What the APs' Bluetooth radios hear (0157) and heard (0158): on an AP's
// Bluetooth tab, the devices it hears now, the trackers first; those whose
// address rotates, counted by what they are from, with how many there were
// over the last day; and the devices it heard earlier that day. On a
// folder's, the same for all its APs, each device once, with the AP that
// hears it best, which is about where it is. And a device's own page: where
// and when the APs heard it, and its stays. Where and when is drawn with
// Plotly (0294).

import { h, link } from '../dom.js';
import { get, put } from '../api.js';
import { configs } from './sections.js';
import { chart, at, token, spanPicker, HOW } from '../chart.js';

const SVG = 'http://www.w3.org/2000/svg';

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

// secs is a span of seconds, short.
function secs(s) {
	if (s == null) return '—';
	if (s < 60) return `${Math.round(s)} s`;
	if (s < 3600) return `${Math.round(s / 60)} min`;
	if (s < 86400) return `${Math.round(s / 3600)} h`;
	return `${Math.round(s / 86400)} d`;
}

const stamp = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
const since = (t) => (Date.now() - new Date(t).getTime()) / 1000;

// The kinds the manager gives a device, and how each is shown.
const KINDS = { tracker: 'warn', beacon: 'idle', sensor: 'idle', health: 'idle', fitness: 'idle', input: 'idle', personal: 'idle', audio: 'idle', media: 'idle' };

const kindChip = (k) => (k ? h('span', { class: `chip ${KINDS[k] || 'idle'}` }, k) : h('span', { class: 'sub' }, '—'));

const deviceLink = (mac, ...label) => link(`/bluetooth/${mac}`, ...label);

// The address and what kind it is.
function address(d) {
	const how = d.type === 'public' ? 'public' : d.random ? `${d.random} random` : 'random';
	return [h('div', { class: 'mono' }, deviceLink(d.mac, d.mac)), h('div', { class: 'sub' }, how)];
}

// What it says of itself beyond what it is: its services, a beacon's
// numbers, its power and the start of its data.
function more(d) {
	const b = d.beacon;
	const parts = [
		...(d.services || []),
		b && b.type === 'ibeacon' && `iBeacon ${b.uuid || ''} ${b.major ?? ''}/${b.minor ?? ''}`,
		b && b.type === 'eddystone' && `Eddystone ${b.frame || ''}`,
		d.tx_power != null && `tx ${d.tx_power} dBm`,
		d.mfr && `data ${d.mfr.slice(0, 16)}${d.mfr.length > 16 ? '…' : ''}`,
	].filter(Boolean);
	return parts.length ? h('div', { class: 'sub' }, parts.join(' · ')) : null;
}

// signal is a device's signal now, with its average and best.
function signal(d) {
	return [h('div', null, d.rssi != null ? `${d.rssi} dBm` : '—'),
		(d.rssi_avg != null || d.rssi_max != null) && h('div', { class: 'sub' },
			[d.rssi_avg != null && `avg ${d.rssi_avg}`, d.rssi_max != null && `best ${d.rssi_max}`].filter(Boolean).join(' · '))];
}

const maker = (d) => d.maker || d.company_name || (d.company != null ? `company 0x${d.company.toString(16).padStart(4, '0')}` : '—');

// age is how old a report is, in seconds, which the times it gives as
// seconds ago are counted from.
const age = (at) => Math.max(0, since(at));

// spark draws how many of a kind of rotating address were heard over the
// last day, five minutes a mark, the most at once its top.
function spark(points, hours) {
	if (!points?.length) return h('span', { class: 'sub' }, '—');
	const W = 120, H = 22, end = Date.now(), start = end - hours * 3600e3;
	const top = Math.max(...points.map((p) => p.count), 1);
	const x = (t) => ((new Date(t).getTime() - start) / (end - start)) * W;
	const w = Math.max(1, (300e3 / (end - start)) * W);
	return svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'spark', role: 'img', 'aria-label': `at most ${top} at once over the last day` },
		svg('title', null, `at most ${top} at once over the last day`),
		points.map((p) => svg('rect', { x: x(p.at).toFixed(1), y: (H - (p.count / top) * H).toFixed(1), width: w.toFixed(1), height: ((p.count / top) * H).toFixed(1) })));
}

function devicesTable(list, old, where) {
	return h('div', { class: 'widetable' }, h('table', { class: 'list' },
		h('tr', null, ['Name', 'Kind', 'What', 'Maker', 'Address', 'Signal', where && 'Heard by', 'Heard', 'First', 'Last'].filter(Boolean).map((c) => h('th', null, c))),
		list.map((d) => h('tr', null,
			h('td', null, d.name ? deviceLink(d.mac, h('span', { class: 'mono' }, d.name)) : h('span', { class: 'sub' }, '—')),
			h('td', null, kindChip(d.kind)),
			h('td', null, d.what || h('span', { class: 'sub' }, '—'), more(d)),
			h('td', null, maker(d)),
			h('td', { class: 'nowrap' }, address(d)),
			h('td', { class: 'nowrap' }, signal(d)),
			where && h('td', null, where(d)),
			h('td', null, `${d.seen}×`),
			h('td', { class: 'nowrap sub' }, secs(d.first_ago + old(d))),
			h('td', { class: 'nowrap sub' }, secs(d.last_ago + old(d)))))));
}

function rotatingTable(groups, where, series) {
	return h('div', { class: 'widetable' }, h('table', { class: 'list' },
		h('tr', null, ['Maker', 'Kind', 'What', 'Addresses', 'Strongest', where && 'Heard by', 'Last day', 'Last'].filter(Boolean).map((c) => h('th', null, c))),
		groups.map((g) => h('tr', null,
			h('td', null, maker(g)),
			h('td', null, kindChip(g.kind)),
			h('td', null, g.what || h('span', { class: 'sub' }, '—'), g.mfr && h('div', { class: 'sub mono' }, `data ${g.mfr}…`)),
			h('td', null, String(g.count)),
			h('td', { class: 'nowrap' }, g.rssi_max != null ? `${g.rssi_max} dBm` : '—'),
			where && h('td', null, where(g)),
			h('td', null, spark(series.get(g.label), 24)),
			h('td', { class: 'nowrap sub' }, secs(g.last))))));
}

// earlierTable is the devices heard in the last day that are not heard now:
// what each is, when and by which AP it was last heard, and for how long.
function earlierTable(ctx, list) {
	return h('div', { class: 'widetable' }, h('table', { class: 'list' },
		h('tr', null, ['Name', 'Kind', 'What', 'Maker', 'Address', 'Last heard', 'By', 'For', 'First heard'].map((c) => h('th', null, c))),
		list.map((d) => {
			const best = [...d.aps].sort((a, b) => b.best - a.best)[0];
			const slots = d.aps.reduce((n, a) => n + a.slots, 0);
			return h('tr', null,
				// What a person called it first (0220), then what it calls itself.
				h('td', null, d.named ? [deviceLink(d.mac, h('strong', null, d.named)), d.name && h('div', { class: 'sub mono' }, d.name)]
					: d.name ? deviceLink(d.mac, h('span', { class: 'mono' }, d.name)) : h('span', { class: 'sub' }, '—')),
				h('td', null, kindChip(d.kind)),
				h('td', null, d.what || h('span', { class: 'sub' }, '—')),
				h('td', null, d.maker || '—'),
				h('td', { class: 'nowrap mono' }, deviceLink(d.mac, d.mac)),
				h('td', { class: 'nowrap' }, `${secs(since(d.last_at))} ago`),
				h('td', null, link(`/aps/${encodeURIComponent(d.last_ap)}/bluetooth`, ctx.name('locations', d.last_ap)),
					best && best.best > -127 && h('div', { class: 'sub' }, `best ${best.best} dBm`)),
				h('td', { class: 'nowrap' }, `about ${secs(slots * 300)}`),
				h('td', { class: 'nowrap sub' }, stamp(d.first_at)));
		})));
}

// source says where an AP's list comes from: the radio lent to Home
// Assistant hears every advertisement; the survey looks every few minutes.
function source(b) {
	const surveyed = b.surveyed_ago != null ? `survey ${secs(b.surveyed_ago)} ago` : 'survey';
	return b.source === 'proxy' ? 'live' : b.source === 'both' ? `live · ${surveyed}` : surveyed;
}

function counts(devices, trackers, rotating, earlier) {
	const n = rotating.reduce((s, g) => s + g.count, 0);
	return h('div', { class: 'chiprow' },
		h('span', { class: 'chip idle' }, `${devices} device${devices === 1 ? '' : 's'}`),
		trackers > 0 && h('span', { class: 'chip warn' }, `${trackers} tracker${trackers === 1 ? '' : 's'}`),
		n > 0 && h('span', { class: 'chip idle' }, `${n} rotating address${n === 1 ? '' : 'es'}`),
		earlier > 0 && h('span', { class: 'chip idle' }, `${earlier} more heard earlier today`));
}

// history is what was kept of the last day under a folder or AP (0158):
// its devices, and its rotating addresses by kind, a series each.
async function history(under) {
	try {
		const hist = await get(`/v1/bluetooth?under=${encodeURIComponent(under)}&hours=24`);
		const series = new Map();
		for (const c of hist.rotating) {
			if (!series.has(c.what)) series.set(c.what, []);
			series.get(c.what).push(c);
		}
		return { devices: hist.devices, series };
	} catch {
		return { devices: [], series: new Map() };
	}
}

// bluetoothTab is an AP's Bluetooth tab: what its radio hears, by its
// latest report, and what it heard earlier today.
export async function bluetoothTab(ctx, id, cond) {
	const rep = cond.state;
	const b = rep?.report?.ble_devices;
	if (!b) return h('div', { class: 'banner info' }, 'It has not said what its Bluetooth radio hears.');
	const old = () => age(rep.at);
	const all = b.devices || [];
	const trackers = all.filter((d) => d.kind === 'tracker');
	const others = all.filter((d) => d.kind !== 'tracker');
	const rotating = (b.rotating || []).map((g) => ({ ...g, last: g.last_ago + old() }));
	const hist = await history(id);
	const now = new Set(all.map((d) => d.mac));
	const earlier = hist.devices.filter((d) => !now.has(d.mac));
	return [
		h('section', { class: 'panel' },
			h('h2', null, 'Bluetooth', h('span', { class: 'note' }, source(b))),
			counts(all.length - trackers.length, trackers.length, rotating, earlier.length)),
		trackers.length > 0 && h('section', { class: 'panel' }, h('h2', null, 'Trackers'), devicesTable(trackers, old)),
		h('section', { class: 'panel' }, h('h2', null, 'Devices'),
			others.length ? devicesTable(others, old) : h('p', { class: 'sub pad' }, 'None heard in the last ten minutes.')),
		rotating.length > 0 && h('section', { class: 'panel' }, h('h2', null, 'Rotating addresses'), rotatingTable(rotating, null, hist.series)),
		earlier.length > 0 && h('section', { class: 'panel' }, h('h2', null, 'Heard earlier today'), earlierTable(ctx, earlier)),
	];
}

// bluetoothFolderTab is a folder's Bluetooth tab: every device its APs
// hear, once, with the AP that hears it best, and the others that hear it;
// and what they heard earlier today.
export async function bluetoothFolderTab(ctx, page, id) {
	const [rows, hist] = await Promise.all([configs([...(page.hardware?.aps || [])]), history(id)]);
	const heard = rows.filter(({ cfg }) => cfg?.condition?.state?.report?.ble_devices);
	if (!heard.length && !hist.devices.length) return h('div', { class: 'banner info' }, 'No AP here says what its Bluetooth radio hears.');
	const byMAC = new Map();
	const groups = new Map();
	for (const { ap, cfg } of heard) {
		const rep = cfg.condition.state;
		const old = age(rep.at);
		for (const d of rep.report.ble_devices.devices || []) {
			const seen = { ap, rssi: d.rssi ?? -200, last: d.last_ago + old };
			const x = byMAC.get(d.mac);
			if (!x) byMAC.set(d.mac, { ...d, old, by: [seen], best: seen.rssi });
			else {
				x.by.push(seen);
				// The word of the AP that hears it best is kept.
				if (seen.rssi > x.best) Object.assign(x, { ...d, old, by: x.by, best: seen.rssi });
			}
		}
		for (const g of rep.report.ble_devices.rotating || []) {
			const key = g.label || `${g.company ?? ''}/${g.what || ''}`;
			const x = groups.get(key) || { ...g, count: 0, rssi_max: null, last: Infinity, by: [] };
			x.count = Math.max(x.count, g.count);
			if (g.rssi_max != null) x.rssi_max = x.rssi_max == null ? g.rssi_max : Math.max(x.rssi_max, g.rssi_max);
			x.last = Math.min(x.last, g.last_ago + old);
			x.by.push({ ap, count: g.count });
			groups.set(key, x);
		}
	}
	const devices = [...byMAC.values()].map((d) => ({ ...d, by: d.by.sort((a, b) => b.rssi - a.rssi) }))
		.sort((a, b) => (b.by[0].rssi) - (a.by[0].rssi));
	const apLink = (ap) => link(`/aps/${encodeURIComponent(ap.id)}/bluetooth`, ap.name);
	const where = (d) => [h('div', null, apLink(d.by[0].ap), d.by[0].rssi > -200 && ` ${d.by[0].rssi}`),
		d.by.length > 1 && h('div', { class: 'sub' }, d.by.slice(1).map((s, i) => [i ? ' · ' : '', apLink(s.ap), s.rssi > -200 ? ` ${s.rssi}` : ''])),
	];
	const trackers = devices.filter((d) => d.kind === 'tracker');
	const others = devices.filter((d) => d.kind !== 'tracker');
	const rotating = [...groups.values()].sort((a, b) => b.count - a.count);
	const groupWhere = (g) => g.by.map((s, i) => [i ? ' · ' : '', apLink(s.ap), ` ${s.count}`]);
	const old = (d) => d.old;
	const earlier = hist.devices.filter((d) => !byMAC.has(d.mac));
	return [
		h('section', { class: 'panel' },
			h('h2', null, 'Bluetooth', h('span', { class: 'note' }, `${heard.length} AP${heard.length === 1 ? '' : 's'} hearing`)),
			counts(others.length, trackers.length, rotating, earlier.length)),
		trackers.length > 0 && h('section', { class: 'panel' }, h('h2', null, 'Trackers'), devicesTable(trackers, old, where)),
		h('section', { class: 'panel' }, h('h2', null, 'Devices'),
			others.length ? devicesTable(others, old, where) : h('p', { class: 'sub pad' }, 'None heard in the last ten minutes.')),
		rotating.length > 0 && h('section', { class: 'panel' }, h('h2', null, 'Rotating addresses'), rotatingTable(rotating, groupWhere, hist.series)),
		earlier.length > 0 && h('section', { class: 'panel' }, h('h2', null, 'Heard earlier today'), earlierTable(ctx, earlier)),
	];
}

// stays joins each AP's sightings of a device, five minutes each, into
// stays: one goes on while the next is no more than ten minutes after.
function stays(seen) {
	const byAP = new Map();
	for (const s of seen) {
		if (!byAP.has(s.ap)) byAP.set(s.ap, []);
		byAP.get(s.ap).push({ t: new Date(s.at).getTime(), rssi: s.rssi });
	}
	const out = [];
	for (const [ap, list] of byAP) {
		list.sort((a, b) => a.t - b.t);
		let cur = null;
		for (const x of list) {
			if (cur && x.t - cur.last <= 10 * 60e3) {
				cur.last = x.t;
				cur.best = Math.max(cur.best, x.rssi);
				cur.slots++;
			} else {
				cur = { ap, first: x.t, last: x.t, best: x.rssi, slots: 1 };
				out.push(cur);
			}
		}
	}
	return out.sort((a, b) => b.last - a.last);
}

// timeline draws where and when a device was heard: a lane for each AP, a
// mark for each five minutes, the stronger the darker, on a scale from
// -100 dBm to -40. Its lanes are named by the APs' names, never the
// device's: that is what it calls itself, and anyone's to choose.
function timeline(ctx, seen, hours, mac) {
	const aps = [...new Set(seen.map((s) => s.ap))];
	if (!aps.length) return h('p', { class: 'sub pad' }, 'Not heard in this time.');
	const name = (ap) => ctx.name('locations', ap);
	const end = Date.now(), start = end - hours * 3600e3;
	const strength = (s) => (s.rssi > -127 ? s.rssi : null);
	return chart({
		key: `bluetooth:${mac}`, span: hours, name: 'bluetooth', height: 70 + aps.length * 30,
		label: `Where it was heard over the last ${hours} hours`,
		traces: [{
			type: 'bar', orientation: 'h', name: 'heard',
			y: seen.map((s) => name(s.ap)), base: seen.map((s) => at(s.at)), x: seen.map(() => 300e3),
			customdata: seen.map((s) => [stamp(s.at), strength(s) != null ? `${s.rssi} dBm` : 'heard']),
			hovertemplate: '%{y}, %{customdata[0]}: %{customdata[1]}<extra></extra>',
			marker: {
				color: seen.map((s) => strength(s) ?? -100), cmin: -100, cmax: -40,
				colorscale: [[0, token('seq-low')], [1, token('seq-high')]],
				colorbar: { title: { text: 'dBm', side: 'top' }, thickness: 8, len: 1, outlinewidth: 0, tickfont: { color: token('muted'), size: 11 } },
			},
		}],
		layout: {
			hovermode: 'closest', showlegend: false, bargap: 0.25,
			xaxis: { range: [at(start), at(end)] },
			yaxis: { type: 'category', categoryorder: 'array', categoryarray: aps.map(name).reverse(), rangemode: 'normal', showgrid: false },
		},
	});
}

// What the device page shows. Kept while the page is redrawn.
const view = { hours: 168 };
// The spans it is drawn over: the APs keep what they heard 90 days.
const BT_SPANS = [[24, 'A day'], [168, 'A week'], [720, '30 days'], [2160, '90 days']];

// bluetoothDevicePage is one device's own page (0158): what it is, when it
// was first and last heard, where and when the APs heard it, and its stays.
export async function bluetoothDevicePage(ctx, mac) {
	const box = h('div');
	let asked = 0;
	let head = h('div');
	const draw = async () => {
		const mine = ++asked;
		let r;
		try {
			r = await get(`/v1/bluetooth/devices/${mac}?hours=${view.hours}`);
		} catch (e) {
			if (mine === asked) box.replaceChildren(h('div', { class: e.status === 404 ? 'banner info' : 'error' }, e.status === 404 ? 'No AP you may view has heard this device.' : e.message));
			return;
		}
		if (mine !== asked) return;
		const d = r.device;
		const st = stays(r.seen);
		// A name of a person's own (0220): what it is called everywhere, and a
		// device named and not heard for a day is an alert.
		const naming = h('div', { class: 'edit flush' });
		const rename = h('button', { type: 'button', class: 'button small rename', title: 'Give this device a name of your own; one named and not heard for a day is an alert',
			onclick: () => {
				const input = h('input', { type: 'text', maxlength: 64, value: d.named || '', placeholder: 'attic sensor' });
				const error = h('div', { class: 'error' });
				const save = h('button', { type: 'button', class: 'button small primary', onclick: async () => {
					save.disabled = true;
					try {
						await put(`/v1/bluetooth/devices/${encodeURIComponent(d.mac)}/name`, { name: input.value });
						draw();
					} catch (e) {
						save.disabled = false;
						error.textContent = e.message;
					}
				} }, 'Save');
				naming.replaceChildren(h('div', { class: 'row' }, input, save, h('button', { type: 'button', class: 'button small', onclick: () => naming.replaceChildren() }, 'Cancel')), error);
				input.focus();
			} }, d.named ? 'Rename…' : 'Name it…');
		// What it calls itself, where a person called it something else.
		const itself = d.named && d.name ? h('div', { class: 'sub' }, `It calls itself ${d.name}.`) : null;
		// itself may be null: spread, and let go, or the page writes "null".
		head.replaceChildren(
			h('h1', null, d.named || d.name || h('span', { class: 'mono' }, d.mac), rename),
			...[itself].filter(Boolean),
			naming,
			h('div', { class: 'chiprow flush' },
				d.kind && kindChip(d.kind), d.what && h('span', { class: 'chip idle' }, d.what), d.maker && h('span', { class: 'chip idle' }, d.maker),
				h('span', { class: 'chip idle mono' }, d.mac), d.type && h('span', { class: 'chip idle' }, d.type)),
			h('div', { class: 'sub' }, `first heard ${stamp(d.first_at)} · last heard ${secs(since(d.last_at))} ago by `,
				link(`/aps/${encodeURIComponent(d.last_ap)}/bluetooth`, ctx.name('locations', d.last_ap))));
		box.replaceChildren(
			h('section', { class: 'panel' },
				h('h2', { title: HOW }, h('span', { class: 'title' }, 'Where and when', spanPicker(view.hours, (v) => { view.hours = v; draw(); }, BT_SPANS)),
					h('span', { class: 'note' }, 'each AP five minutes a mark, the darker the stronger')),
				timeline(ctx, r.seen, r.hours, mac)),
			h('section', { class: 'panel' },
				h('h2', null, 'Stays', h('span', { class: 'note' }, `${st.length}`)),
				st.length ? h('div', { class: 'widetable' }, h('table', { class: 'list' },
					h('tr', null, ['AP', 'From', 'To', 'For', 'Best'].map((c) => h('th', null, c))),
					st.slice(0, 300).map((s) => h('tr', null,
						h('td', null, link(`/aps/${encodeURIComponent(s.ap)}/bluetooth`, ctx.name('locations', s.ap))),
						h('td', { class: 'nowrap' }, stamp(s.first)),
						h('td', { class: 'nowrap' }, stamp(s.last + 300e3)),
						h('td', { class: 'nowrap' }, secs((s.last + 300e3 - s.first) / 1000)),
						h('td', { class: 'nowrap' }, s.best > -127 ? `${s.best} dBm` : '—'))))) : h('p', { class: 'sub pad' }, 'None in this time.')));
	};
	await draw();
	return { main: [h('div', { class: 'head' }, head), box], refresh: 60 };
}
