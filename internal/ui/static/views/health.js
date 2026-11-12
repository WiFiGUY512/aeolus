// An AP's own health (0119): how busy its processor is, its memory,
// storage and temperature now and over a day or a week, from each of its
// state reports. Its chart is drawn with Plotly (0294). What happened to it,
// its history, is its Activity's (0338).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { chart as plot, at, series, spanPicker, HOW } from '../chart.js';

const stamp = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
const mb = (kb) => (kb >= 1048576 ? `${(kb / 1048576).toFixed(1)} GB` : `${Math.round(kb / 1024)} MB`);

function upFor(s) {
	if (s < 120) return `${s} s`;
	if (s < 7200) return `${Math.round(s / 60)} min`;
	if (s < 172800) return `${Math.round(s / 3600)} h`;
	return `${Math.round(s / 86400)} days`;
}

// What the page shows: a day or a week of health. Kept while the page is
// redrawn.
const view = { hours: 24 };

// How each figure stands, by the lines its alerts are drawn at and as
// grave as they are (0119): warn where the alert is a warning, bad where it
// is critical, which only memory's is, under 5% free.
const cpuGrade = (cpu) => (cpu >= 90 ? 'warn' : 'ok');
const memGrade = (free) => (free < 5 ? 'bad' : free < 10 ? 'warn' : 'ok');
const tempGrade = (temp) => (temp >= 95 ? 'warn' : 'ok');
const freeOf = (free, total) => Math.floor((100 * free) / total);

// healthBrief is an AP's health in a line, for a list of APs: its
// processor, its memory and its temperature, and its storage where that is
// nearly full, with the worst of them as its grade. Null where the AP has
// not said.
export function healthBrief(x) {
	if (!x) return null;
	const rank = { ok: 1, warn: 2, bad: 3 };
	const parts = [];
	let grade = 'ok';
	const add = (text, g) => {
		parts.push(text);
		if (rank[g] > rank[grade]) grade = g;
	};
	if (x.cpu != null) add(`CPU ${x.cpu}%`, cpuGrade(x.cpu));
	if (x.mem_total) add(`memory ${100 - freeOf(x.mem_available, x.mem_total)}%`, memGrade(freeOf(x.mem_available, x.mem_total)));
	if (x.temp != null) add(`${Math.round(x.temp)} °C`, tempGrade(x.temp));
	if (x.storage_total && freeOf(x.storage_free, x.storage_total) < 5) add('storage nearly full', 'warn');
	return parts.length ? { text: parts.join(' · '), grade } : null;
}

// What airscan is at, as the AP's report names it (0126).
const SCANNING = { idle: 'between jobs', clients: 'looking at clients', bss: 'looking for networks', spectral: 'sweeping the spectrum', lent: 'lent to a client test',
	healing: 'its driver being bound again', waiting: 'no radio for it', missing: 'airscan does not answer' };

// scanRadio is the scan radio's tile (0126): how many networks its latest
// look heard, what airscan is at, and how often it had to bring the radio
// back. Nothing on an AP with none.
function scanRadio(s) {
	if (!s) return null;
	const heals = s.heals ? ` · brought back ${s.heals === 1 ? 'once' : `${s.heals} times`}${s.healed_ago != null ? `, last ${upFor(s.healed_ago)} ago` : ''}${s.heal_failed ? `, ${s.heal_failed} to no avail` : ''}` : '';
	if (s.state === 'missing' || s.state === 'waiting') return ['—', SCANNING[s.state], 'warn'];
	if (s.networks == null) return ['—', `no look yet · ${SCANNING[s.state] || s.state}${heals}`, ''];
	const deaf = s.networks === 0;
	return [String(s.networks), `network${s.networks === 1 ? '' : 's'} heard ${upFor(s.looked_ago ?? 0)} ago · ${SCANNING[s.state] || s.state}${heals}`, deaf ? 'warn' : 'ok'];
}

// bluetooth is the tile of the AP's Bluetooth radio, where it is lent to
// Home Assistant (0156): how many devices it heard in the last five
// minutes, whether it scans, who listens, and how much it has heard and
// handed on. Nothing where it is not lent. The System tab says the same
// beside the setting.
export function bluetooth(b) {
	if (!b) return null;
	if (!b.radio) return ['—', 'lent to Home Assistant · no Bluetooth radio here', 'warn'];
	if (!b.running) return ['—', `lent to Home Assistant · ${b.tools === false ? 'hcitool and hcidump are not installed' : 'its proxy is not running'}`, 'warn'];
	const who = (b.clients || []).map((c) => c.from.replace(/:\d+$/, ''));
	const n = (v) => Number(v || 0).toLocaleString();
	return [String(b.devices ?? 0), `device${b.devices === 1 ? '' : 's'} heard in 5 min · ${b.scanning ? (b.active ? 'scanning actively' : 'scanning') : 'not scanning'} · `
		+ `${who.length ? `Home Assistant at ${who.join(', ')}` : `no Home Assistant connected, ESPHome port ${b.port}`} · ${n(b.heard)} advertisements heard, ${n(b.sent)} handed on`,
	b.scanning ? 'ok' : 'warn'];
}

// tiles are how the AP is doing now, by its latest report.
function tiles(now) {
	const x = now?.health;
	if (!x) return [h('div', { class: 'banner info' }, 'It has not said how it is doing yet.')];
	const tile = (label, big, sub, cls) => h('div', { class: `tile ${cls || ''}` },
		h('div', { class: 'tlabel' }, label), h('div', { class: 'tbig' }, big), h('div', { class: 'tsub' }, sub));
	const out = [];
	const load = x.load ? `load ${x.load[0]} on ${x.cores || '?'} core${x.cores === 1 ? '' : 's'}` : '';
	out.push(tile('Processor', x.cpu != null ? `${x.cpu}%` : '—', x.cpu != null ? `busy since its last report · ${load}` : `${load || 'no figure yet'}`,
		x.cpu != null ? cpuGrade(x.cpu) : ''));
	if (x.mem_total) {
		const free = freeOf(x.mem_available, x.mem_total);
		out.push(tile('Memory', `${100 - free}%`, `in use · ${mb(x.mem_available)} free of ${mb(x.mem_total)}`, memGrade(free)));
	}
	if (x.storage_total) {
		const free = freeOf(x.storage_free, x.storage_total);
		out.push(tile('Storage', `${100 - free}%`, `in use · ${mb(x.storage_free)} free of ${mb(x.storage_total)}, where its config is kept`, free < 5 ? 'warn' : 'ok'));
	}
	if (x.temp != null) out.push(tile('Temperature', `${x.temp} °C`, 'its hottest sensor', tempGrade(x.temp)));
	out.push(tile('Up', upFor(now.uptime), `since ${stamp(new Date(now.at).getTime() - now.uptime * 1000)}`, ''));
	if (x.procs) out.push(tile('Running', String(x.procs), `processes${x.conntrack_max ? ` · ${x.conntrack} connections tracked of ${x.conntrack_max}` : ''}`, ''));
	const scan = scanRadio(now.scan_radio);
	if (scan) out.push(tile('Scan radio', ...scan));
	const bt = bluetooth(now.ble);
	if (bt) out.push(tile('Bluetooth', ...bt));
	// Which processes hold its memory, and which used its processor since
	// its last report (0119).
	const named = (list, said) => list?.length > 0 && h('div', { class: 'sub pad' }, said, ' ',
		list.map((p) => `${p.name} ${p.rss != null ? (p.rss < 1024 ? `${p.rss} kB` : mb(p.rss)) : `${p.cpu}%`}`).join(' · '));
	return [h('div', { class: 'tiles' }, out),
		named(x.top_mem, 'Holding the most memory:'),
		named(x.top_cpu, 'Busiest since its last report:')];
}

// chart draws the processor and memory over the time shown, each from 0 to
// 100 percent, and under them, where it says one, its temperature on a
// scale of its own; a point each report, the worst of each run where there
// are many.
function chart(id, hl) {
	const pts = hl.points || [];
	if (pts.length < 2) return h('p', { class: 'sub pad' }, 'A chart needs two reports with its health; they come every five minutes.');
	const x = pts.map((p) => at(p.at));
	const hot = pts.some((p) => p.temp != null);
	const line = (name, key, colour, unit, axis) => ({
		type: 'scatter', mode: 'lines', name, x, y: pts.map((p) => p[key] ?? null), yaxis: axis,
		line: { color: colour, width: 2 }, hovertemplate: `%{y}${unit}`,
	});
	return plot({
		key: `health:${id}`, span: hl.hours, name: 'health', height: hot ? 270 : 220,
		label: `Its processor and memory${hot ? ', and its temperature,' : ''} over the last ${hl.hours} hours`,
		traces: [
			line('processor busy', 'cpu', series(1), '%', 'y'),
			line('memory in use', 'mem', series(2), '%', 'y'),
			hot && line('temperature', 'temp', series(4), ' °C', 'y2'),
		].filter(Boolean),
		layout: {
			xaxis: { anchor: hot ? 'y2' : 'y' },
			yaxis: { domain: hot ? [0.32, 1] : [0, 1], range: [0, 100], ticksuffix: '%', dtick: 25 },
			...(hot && { yaxis2: { domain: [0, 0.22], ticksuffix: ' °C', rangemode: 'normal', nticks: 3 } }),
		},
	});
}

// healthTab draws an AP's Health tab.
export async function healthTab(ctx, id) {
	const enc = encodeURIComponent(id);
	const box = h('div');
	// Only the latest asking is drawn: an answer to an earlier choice that
	// comes in late does not replace it.
	let asked = 0;
	const draw = async () => {
		const mine = ++asked;
		let hl;
		try {
			hl = await get(`/v1/aps/${enc}/health?hours=${view.hours}`);
		} catch (e) {
			if (mine === asked) box.replaceChildren(h('div', { class: 'error' }, e.message));
			return;
		}
		if (mine !== asked) return;
		// tiles is a list, some of it false: spread, and what is false let go,
		// or the page writes the list itself as words.
		box.replaceChildren(
			...tiles(hl.now).filter(Boolean),
			h('section', { class: 'panel' },
				h('h2', { title: HOW }, h('span', { class: 'title' }, 'Processor and memory', spanPicker(view.hours, (v) => { view.hours = v; draw(); })),
					h('span', { class: 'note' }, 'from each of its reports; where there are many, the worst of each run')),
				chart(id, hl)),
			h('p', { class: 'sub pad' }, 'What happened to it, its restarts, updates and hourly accounts of itself, is on its ',
				link(`/aps/${enc}/activity/health`, 'Activity'), '.'));
	};
	await draw();
	return box;
}
