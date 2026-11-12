// The Overview tab of a Locations folder or the Org (0102, 0284): the APs
// below it at a glance. One strip says how things stand: the APs up, the
// clients on now, what needs attention, the joins that failed, the client
// tests and the traffic over the day, each leading to the tab that says
// more. Under it, the APs on the page's band, with what needs attention and
// the clients that moved the most beside them; then the clients over time,
// and the traffic a click down. How clients roamed is a cell of the strip
// (0305); the roams themselves and why clients left are Clients › Wi-Fi
// roaming (0337). It reads what the other tabs do, and sets nothing.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { apStatus, pageBand } from '../layout.js';
import { bandName, size } from '../format.js';
import { called } from '../names.js';
import { configs, radiosOf, upFor } from './sections.js';
import { usagePanel, clientsPanel } from './usage.js';
import { onNow } from './onnow.js';
import { took } from './roaming.js';
import { said, saidOf } from './alerts.js';

const BANDS = ['2g', '5g', '6g'];

// The states of an AP that is not up (apStatus).
const DOWN = new Set(['Not heard from', 'Never seen', 'Down']);

// A signal at or under this, in dBm, is weak (Griff, 2026-10-30: -70 to -72
// is good; -80 is weak).
const WEAK = -80;

// What is folded opens by its name, and stays as it was left over a redraw.
const open = new Set();

export function tile(label, big, sub, cls, href) {
	const body = [h('div', { class: 'tlabel' }, label), h('div', { class: 'tbig' }, big), sub && h('div', { class: 'tsub' }, sub)];
	return href ? h('a', { class: `tile ${cls || ''}`, href }, body) : h('div', { class: `tile ${cls || ''}` }, body);
}

// cell is one of the strip's: a label, a figure, a line under it; cls
// colours the figure, href leads to the tab that says more, and onclick
// readies it.
export function cell(label, big, sub, cls, href, onclick) {
	const body = [h('div', { class: 'slabel' }, label), h('div', { class: `sbig ${cls || ''}` }, big), sub && h('div', { class: 'ssub' }, sub)];
	return href ? h('a', { class: 'scell', href, onclick }, body) : h('div', { class: 'scell' }, body);
}

// roamed is how the clients here roamed over the day (0136, 0305): how many
// moves between APs, how long they typically took to be let on, and those
// that failed or were slow; it leads to Clients › Wi-Fi roaming (0337).
// None from a manager that has no roaming to say.
function roamed(r, base) {
	if (!r) return null;
	const to = `${base}/clients/roaming`;
	if (!r.roams) return cell('Roams, 24 h', 'none', r.band_switches ? `${r.band_switches} band switch${r.band_switches === 1 ? '' : 'es'}` : 'no client moved between APs', '', to);
	const sub = [r.typical_ms != null && `typically ${took(r.typical_ms)}`, r.failed > 0 && `${r.failed} failed`, r.slow > 0 && `${r.slow} slow`].filter(Boolean).join(' · ');
	return cell('Roams, 24 h', `${r.roams} by ${r.clients} client${r.clients === 1 ? '' : 's'}`, sub, r.failed > 0 ? 'warn' : '', to);
}

// assured is the service as the client tests find it (0123): of the things
// the APs here test, how many passed their last test, and how many are
// failing, which is their last two. None where no AP here can test.
function assured(a, base) {
	if (!a?.testers?.length) return null;
	const tried = a.checks.filter((c) => c.last?.detail);
	const passed = tried.filter((c) => c.last.detail.outcome === 'passed').length;
	const failing = a.checks.filter((c) => c.failing).length;
	// With no AP asked for tests any more, what the last ones found is not
	// how the service is now: only a thing still failing is said.
	const asked = a.testers.some((t) => t.every);
	if (!asked && !failing) return cell('Client tests', '—', 'no schedule set', '', `${base}/assurance`);
	const sub = failing ? `${failing} failing`
		: !tried.length ? 'no test made yet'
			: passed === tried.length ? 'all work' : `${tried.length - passed} failed its last test`;
	return cell('Client tests', tried.length ? `${passed} of ${tried.length}` : '—', sub, failing ? 'bad' : passed < tried.length ? 'warn' : '', `${base}/assurance`);
}

export async function overviewTab(ctx, id, page, fleet) {
	const below = page.hardware?.aps || [];
	if (!below.length) return [h('div', { class: 'banner info' }, 'No APs here yet.')];
	const ids = new Set(below.map((a) => a.id));
	const aps = (fleet?.aps || []).filter((a) => ids.has(a.id));
	const under = encodeURIComponent(id);
	const [rows, al, day, assurance, noise, roams] = await Promise.all([configs(below), get(`/v1/alerts?under=${under}`),
		get(`/v1/usage?under=${under}&hours=24`).catch(() => null), // an older manager has none
		// How the tests stand, without the tests (0285).
		get(`/v1/assurance?under=${under}&brief=1`).catch(() => null),
		get(`/v1/noise?under=${under}`).catch(() => null),
		// How clients roamed (0305).
		get(`/v1/roaming?under=${under}&hours=24`).catch(() => null)]);
	const base = `#/locations/${under}`;

	// APs: up is heard from lately. One in maintenance (0218) is said
	// apart: it is down on purpose.
	const alerts = al.alerts || [];
	const resting = new Set(alerts.filter((a) => a.maintenance).map((a) => a.ap));
	const st = aps.map((a) => ({ a, s: apStatus(a) }));
	const down = st.filter((x) => DOWN.has(x.s.label));
	const off = down.filter((x) => !resting.has(x.a.id)).length;
	const inSync = st.filter((x) => x.a.in_sync === true).length;
	const held = st.filter((x) => x.a.config === 'held').length;
	const apsSub = [off && `${off} offline`, down.length - off && `${down.length - off} in maintenance`,
		held && `${held} held`, `${inSync} in sync`].filter(Boolean).join(' · ');

	// Clients, from each AP's last report, each once, on the AP it was last
	// active on.
	const clients = onNow(rows).map(({ c, ap }) => ({ ...c, ap }));
	const byBand = Object.fromEntries(BANDS.map((b) => [b, clients.filter((c) => c.band === b).length]));
	const c = al.counts || {};
	const worst = alerts.find((a) => !a.maintenance);
	const failed = day?.failed || 0;

	const strip = h('section', { class: 'nowstrip', 'aria-label': 'Now' },
		cell('APs', `${st.length - down.length} of ${st.length} up`, apsSub, off ? 'bad' : '', `${base}/aps`),
		cell('Clients now', String(clients.length), BANDS.filter((b) => byBand[b]).map((b) => `${bandName(b)} ${byBand[b]}`).join(' · ') || 'none now', '', `${base}/clients`),
		cell('Needs attention', c.critical ? `${c.critical} critical` : c.warning ? `${c.warning} warning` : 'Nothing',
			worst ? `${worst.name}: ${worst.short || worst.message}` : c.maintenance ? `${c.maintenance} in maintenance` : 'no alert', c.critical ? 'bad' : c.warning ? 'warn' : '', `${base}/problems/alerts`),
		day && cell('Joins, 24 h', failed ? `${failed} failed` : 'none failed', failed ? `by ${day.failed_clients || 0} client${day.failed_clients === 1 ? '' : 's'}` : 'every client that tried got on', '', `${base}/assurance`),
		assured(assurance, base),
		roamed(roams, base),
		day && cell('Traffic, 24 h', `${size(day.down)} down`, `${size(day.up)} up`));

	return [
		strip,
		h('div', { class: 'overgrid' },
			apsPanel(rows, st, resting, noise),
			h('div', { class: 'side' }, attention(alerts, clients, base), movedMost(day))),
		await clientsPanel(id, byBand, `/locations/${under}`, true),
		traffic(id, await usagePanel(id, true)),
	].filter(Boolean);
}

// apsPanel is the APs here on the page's band, the busiest first: each with
// its clients on that band and its radio there, as it last reported. An AP
// with no radio for the band is named under the table; one that is down,
// or in maintenance, says so in its row.
function apsPanel(rows, st, resting, noise) {
	const band = pageBand() || '5g';
	const state = new Map(st.map((x) => [x.a.id, x.s]));
	const lines = [], none = [], quiet = [];
	for (const { ap, cfg } of rows) {
		const s = state.get(ap.id);
		const report = cfg?.condition?.state?.report;
		const t = cfg?.template;
		const kind = [t?.model || t?.board, report?.openwrt].filter(Boolean).join(' · ');
		const name = h('td', null, link(`/aps/${encodeURIComponent(ap.id)}`, h('strong', null, ap.name)), kind && h('div', { class: 'sub' }, kind));
		if (s && DOWN.has(s.label)) {
			const rest = resting.has(ap.id);
			quiet.push(h('tr', null, name, h('td', { colspan: 6 }, h('span', { class: `chip ${rest ? 'idle' : 'bad'}` }, rest ? 'Maintenance' : s.label), ' ', h('span', { class: 'sub' }, s.detail))));
			continue;
		}
		const { serving, clientsOf } = radiosOf(cfg);
		const rs = serving.filter((r) => r.band === band);
		if (!rs.length) {
			if (report?.radios) none.push(ap.name);
			else quiet.push(h('tr', null, name, h('td', { colspan: 6, class: 'sub' }, 'No report yet')));
			continue;
		}
		const floors = noise?.aps?.[ap.id] || {};
		const n = rs.reduce((x, r) => x + clientsOf(r), 0);
		const each = (f) => rs.map((r, i) => [i > 0 && h('br'), f(r)]);
		lines.push([n, h('tr', null, name,
			h('td', { class: 'num' }, String(n)),
			h('td', { class: 'num mono' }, each((r) => (r.channel ? String(r.channel) : 'starting'))),
			h('td', { class: 'num' }, each((r) => (r.width ? `${r.width} MHz` : '—'))),
			h('td', { class: 'num' }, each((r) => (r.txpower != null ? `${r.txpower} dBm` : '—'))),
			h('td', { class: 'num' }, each((r) => {
				const f = floors[r.radio];
				return !f || f.unmeasured ? '—' : h('span', { class: f.loud ? 'warn' : '', title: f.loud ? 'Loud with something that is no Wi-Fi' : null }, `${f.dbm} dBm`);
			})),
			h('td', { class: 'nowrap' }, upFor(report?.uptime) || '—'))]);
	}
	lines.sort((x, y) => y[0] - x[0]);
	return h('section', { class: 'panel main' },
		h('h2', null, `APs on ${bandName(band)}`),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Clients', 'Channel', 'Width', 'Power', 'Noise', 'Up for'].map((t, i) => h('th', { class: i && i < 6 ? 'num' : null }, t))),
			lines.map(([, tr]) => tr), quiet),
		none.length > 0 && h('p', { class: 'sub pad' }, `No ${bandName(band)} radio: ${none.join(', ')}`));
}

// attention is what needs a look here: the alerts, most urgent first, those
// of APs in maintenance last; and the clients on now whose signal is weak.
// Each in a few words, and in full a click down (0330).
function attention(alerts, clients, base) {
	const SEV = { critical: 'bad', warning: 'warn', info: 'idle' };
	const shown = alerts.slice(0, 6);
	const weak = clients.filter((c) => c.signal != null && c.signal <= WEAK).sort((x, y) => x.signal - y.signal).slice(0, 4);
	const items = [
		...shown.map((a) => h('li', null, h('span', { class: `chip ${a.maintenance ? 'idle' : SEV[a.severity] || ''}` }, a.maintenance ? 'maintenance' : a.severity), ' ',
			link(`/aps/${encodeURIComponent(a.ap)}`, a.name), ' ', saidOf(a))),
		...weak.map((c) => h('li', null, h('span', { class: 'chip warn' }, 'weak'), ' ',
			link(`/clients/${c.mac}`, called(c.mac, c.host) || c.mac), ' ',
			said(`${c.signal} dBm`, `Heard at ${c.signal} dBm on ${c.ap.name}, ${bandName(c.band)}: ${WEAK} dBm or below is weak, and it may lose the AP, or get little from it.`))),
	];
	return h('section', { class: 'panel' },
		h('h2', null, 'Needs attention', alerts.length > shown.length && h('span', { class: 'note' }, link(`${base.slice(1)}/problems/alerts`, `all ${alerts.length}`))),
		items.length ? h('ul', { class: 'attention' }, items) : h('p', { class: 'sub pad' }, 'Nothing: no alert, and no client on now with a weak signal.'));
}

// movedMost is the clients that moved the most over the day (0110), not
// counting the APs' own test clients.
function movedMost(day) {
	const top = (day?.clients || []).filter((c) => !c.tester).slice(0, 5);
	if (!top.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Moved the most', h('span', { class: 'note' }, '24 h')),
		h('table', { class: 'list' },
			h('tr', null, h('th', null, 'Client'), h('th', { class: 'num' }, 'Down'), h('th', { class: 'num' }, 'Up')),
			top.map((c) => h('tr', null,
				h('td', null, link(`/clients/${c.mac}`, called(c.mac, c.host) || c.mac)),
				h('td', { class: 'num' }, size(c.down)), h('td', { class: 'num' }, size(c.up))))));
}

// traffic is the usage chart and its tables (0108), a click down: the strip
// says the day's totals.
export function traffic(id, usage) {
	if (!usage) return null;
	const key = `traffic/${id}`;
	const d = h('details', { class: 'panel logpanel', open: open.has(key) },
		h('summary', null, h('h2', null, 'Traffic', h('span', { class: 'note' }, 'over time, with the busiest clients and APs'))), usage);
	d.addEventListener('toggle', () => (d.open ? open.add(key) : open.delete(key)));
	return d;
}
