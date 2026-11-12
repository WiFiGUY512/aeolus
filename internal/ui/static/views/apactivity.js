// An AP's Activity (0338): everything it did and that was done to it, a
// line each, newest first, in a window about twenty lines tall that reads
// further back as it is scrolled to its end, as a client's Activity is
// (0337). Its groups are parts of the tab: its health, with an hour's
// account of itself at the end of each hour; its radios, RRM's claims and
// moves and power control's steps; its neighbours, their hellos beginning
// and ending; its network, its tunnels', VLANs' and RADIUS servers'
// verdicts; its config and what it was asked to do; its alerts; and its
// clients, what its steering asked of them and its client tests. A line
// opens to what more there is of it.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, ago } from '../format.js';
import { clientLink } from './journey.js';
import { called } from '../names.js';
import { testOpened } from './clienttest.js';

// The groups, as the tab's parts.
export const GROUPS = [['all', 'All'], ['health', 'Health'], ['radios', 'Radios'], ['neighbours', 'Neighbours'], ['network', 'Network'],
	['config', 'Config'], ['alerts', 'Alerts'], ['clients', 'Clients']];

// How many lines are read at first, and each time the window is scrolled
// to its end.
const PAGE = 20;

// Each kind of line, as its chip names it.
const KIND = {
	first: 'First report', restart: 'Restart', silence: 'Silence', agent: 'Agent', firmware: 'Firmware', uplink: 'Uplink', hour: 'Hour',
	radio: 'Radio', channel: 'Channel', 'scan-radio': 'Scan radio', rrm: 'RRM', power: 'Power',
	neighbour: 'Neighbours', tunnel: 'Tunnel', probe: 'VLAN probe', radius: 'RADIUS',
	config: 'Config', action: 'Asked to', alert: 'Alert', steer: 'Steering', 'client-test': 'Client test',
};
const SEVERITY = { critical: 'bad', warning: 'warn', info: 'idle' };
// A kind that is good news, where no trouble is said.
const GOOD = new Set(['hour', 'config']);

const when = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
const hourOf = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

// What is kept of each AP's Activity, by its AP and group, over the page's
// redraws: the older lines read, where its window was scrolled, and the
// lines opened.
const older = new Map();
const scrolled = new Map();
const opened = new Set();

// What more a line opens to, by its kind. base is the AP's page; name
// names a node by its ID.
const MORE = {
	rrm: (l, base, name) => facts([
		l.detail?.band && ['Band', bandName(l.detail.band)],
		l.detail?.from != null && l.detail?.to != null && ['Channel', `${l.detail.from} → ${l.detail.to}`],
		l.detail?.channel != null && ['Channel', String(l.detail.channel)],
		l.detail?.state && ['What came of it', l.detail.state],
		l.detail?.ap && ['The other AP', apLink(l.detail.ap, name)],
		l.detail?.clients && ['Its clients', `${l.detail.clients.kept ?? 0} of ${l.detail.clients.clients} followed, ${l.detail.clients.back ?? 0} joined again, ${l.detail.clients.gone ?? 0} gone`],
	], link(`${base}/radios/ratings`, 'Its ratings and moves, on Radios')),
	neighbour: (l, base, name) => facts([
		['APs', h('span', null, (l.detail?.aps || []).flatMap((id, i) => [i > 0 && ', ', apLink(id, name, '/radios/neighbours'),
			l.detail.addresses?.[i] && h('span', { class: 'sub' }, ` ${l.detail.addresses[i]}`)]))],
	], link(`${base}/radios/neighbours`, 'Its neighbours, on Radios')),
	hour: (l, base, name) => {
		const d = l.detail || {};
		const nb = d.neighbours || {};
		return facts([
			d.hour && ['Hour', `${hourOf(d.hour)} to ${hourOf(new Date(d.hour).getTime() + 3600e3)}`],
			['Reports', `${d.reports} of the ${d.expected} due, one every five minutes`],
			nb.up && ['Hellos both ways', nb.up.length ? h('span', null, nb.up.flatMap((id, i) => [i > 0 && ', ', apLink(id, name, '/radios/neighbours')])) : 'none'],
			(nb.one_way?.length || nb.down?.length) && ['Hellos not both ways', h('span', null, [...(nb.one_way || []), ...(nb.down || [])].flatMap((id, i) => [i > 0 && ', ', apLink(id, name, '/radios/neighbours')]))],
			d.tunnels && ['Tunnels answering', `${d.tunnels.answer} of ${d.tunnels.all}`],
			d.radius && ['RADIUS servers answering', `${d.radius.answer} of ${d.radius.all}`],
			d.cpu != null && ['Processor, at most', `${d.cpu}%`],
			d.mem != null && ['Memory in use, at most', `${d.mem}%`],
			d.temp != null && ['Temperature, at most', `${Math.round(d.temp)} °C`],
			['Clients, at most', String(d.clients ?? 0)],
			d.version && ['Config it ran', `version ${d.version}`],
		], link(`${base}/health`, 'Its health over time'));
	},
	power: (l, base) => facts([
		['Band', bandName(l.detail?.band)],
		l.detail?.from != null && ['From', `${l.detail.from} dBm`],
		l.detail?.to != null && ['To', `${l.detail.to} dBm`],
		l.detail?.scanning != null && ['Its clients scanning after', String(l.detail.scanning)],
		l.detail?.left != null && ['Its clients leaving after', String(l.detail.left)],
		l.detail?.ratio?.length && ['Sent again or lost, before and after', l.detail.ratio.map((r) => (r == null ? '—' : `${Math.round(r * 100)}%`)).join(' → ')],
	], link(`${base}/radios/power`, 'Its power control, on Radios')),
	config: (l) => facts([
		['Version', String(l.detail?.version)],
		l.detail?.hash && ['Hash', h('span', { class: 'mono' }, l.detail.hash.slice(0, 16))],
		['Checked as it was rendered', l.detail?.checked ? 'yes' : 'no'],
		l.detail?.error && ['Why not', l.detail.error],
	], link('/changes', 'The change log')),
	alert: (l) => facts([
		['Severity', l.detail?.severity],
		l.detail?.began && ['Began', when(l.detail.began)],
		['Ended', l.detail?.ended ? when(l.detail.ended) : 'not yet'],
	]),
	steer: (l, base, name) => facts([
		['Client', clientLink(l.mac, called(l.mac) || l.mac)],
		l.detail?.network && ['Network', l.detail.network],
		l.detail?.to?.length && ['Bands it was offered', l.detail.to.map(bandName).join(', ')],
		l.detail?.better_ap && ['Heard better by', [apLink(l.detail.better_ap, name), l.detail.better_signal != null && ` at ${l.detail.better_signal} dBm`]],
		l.detail?.code != null && ['Its answer’s code', String(l.detail.code)],
		l.detail?.took_ms != null && ['It took', `${(l.detail.took_ms / 1000).toFixed(1)} s`],
	], link(`/clients/${l.mac}`, 'Its page')),
	tunnel: (l, base) => verdict(l, base),
	probe: (l, base) => verdict(l, base),
	radius: (l, base) => verdict(l, base),
};

function verdict(l, base) {
	return facts([
		l.detail?.was && ['Was', l.detail.was],
		l.detail?.verdict && ['Now', l.detail.verdict],
		l.detail?.peer && ['Peer', l.detail.peer],
		l.detail?.vlan != null && ['VLAN', `${l.detail.vlan}${l.detail.tagged ? ', tagged' : ''}`],
		l.detail?.server && ['Server', `${l.detail.server}${l.detail.port ? `:${l.detail.port}` : ''}`],
	], link(`${base}/networks`, 'Its networks and their transports'));
}

function apLink(id, name, part = '') {
	return link(`/aps/${encodeURIComponent(id)}${part}`, name(id) || id);
}

// facts is what more there is, as a short list of names and values, and a
// way on.
function facts(rows, more) {
	const list = rows.filter((r) => r && r[1] != null && r[1] !== '');
	return h('div', { class: 'activitymore' },
		list.length > 0 && h('dl', { class: 'facts' }, list.flatMap(([k, v]) => [h('dt', null, k), h('dd', null, v)])),
		more && h('p', { class: 'sub' }, more));
}

// opens is what a line opens to: a client test, the test as Assurance
// shows it, fetched as it opens; another, what its kind says of it, else
// nothing.
function opens(l, base, name) {
	if (l.kind === 'client-test' && l.detail?.action && l.detail?.ap) return testOpened({ id: l.detail.action, ap: l.detail.ap }, name, true);
	return MORE[l.kind]?.(l, base, name) || null;
}

// lineRows makes a line, and the row under it that it opens into.
function lineRows(base, name) {
	return (l) => {
		const more = MORE[l.kind] || (l.kind === 'client-test' && l.detail?.action && l.detail?.ap);
		const open = more && opened.has(l.id);
		const cell = h('td', { colspan: 3 });
		const fill = () => {
			if (cell.firstChild) return;
			const body = opens(l, base, name);
			if (body) {
				cell.append(body);
				body.load?.();
			}
		};
		if (open) fill();
		const detail = h('tr', { class: 'stepsrow', hidden: !open }, cell);
		const sev = SEVERITY[l.severity] || (GOOD.has(l.kind) ? 'ok' : 'idle');
		const line = h('tr', {
			class: ['attemptrow', l.severity === 'critical' || l.severity === 'warning' ? 'failed' : '', open && 'opened', !more && 'plain'].filter(Boolean).join(' '),
			title: more ? 'What more there is' : null,
			onclick: (e) => {
				if (!more || e.target.closest('a')) return;
				const now = !opened.has(l.id);
				if (now) opened.add(l.id);
				else opened.delete(l.id);
				line.classList.toggle('opened', now);
				detail.hidden = !now;
				if (now) fill();
			},
		},
		h('td', { class: 'mono nowrap', title: ago(l.at) }, when(l.at)),
		h('td', null, h('span', { class: `chip ${sev}` }, KIND[l.kind] || l.kind)),
		h('td', { class: 'said' }, l.mac && [clientLink(l.mac, called(l.mac) || l.mac), ': '], l.text));
		return [line, detail];
	};
}

// activityTab draws an AP's Activity, of the group group.
export async function activityTab(ctx, base, id, group) {
	const key = `${id}|${group}`;
	const only = group === 'all' ? '' : `&only=${group}`;
	const name = (n) => ctx.name('locations', n);
	const url = (q) => `/v1/aps/${encodeURIComponent(id)}/activity?${q}${only}`;
	// The latest, and as far back as was read before, so a redraw leaves no
	// gap.
	const res = await get(url(`limit=${Math.min(200, PAGE + (older.get(key) || []).length)}`));
	const seen = new Set();
	const list = [...res.lines, ...(older.get(key) || [])].filter((l) => !seen.has(l.id) && seen.add(l.id));
	const rows = lineRows(base, name);
	const body = h('tbody', null, list.flatMap(rows));
	const count = h('span', { class: 'note' });
	const said = () => count.replaceChildren(`${list.length} line${list.length === 1 ? '' : 's'}, the latest first · a line with more opens to it`);
	said();
	const foot = h('div', { class: 'below' });
	let last = list[list.length - 1], done = !res.more && !older.has(key), loading = false;
	const button = h('button', { type: 'button', class: 'button small', onclick: () => load() }, 'Older lines');
	const load = async () => {
		if (loading || done || !last) return;
		loading = true;
		button.disabled = true;
		try {
			const r = await get(url(`limit=${PAGE}&before=${encodeURIComponent(last.at)}`));
			const got = r.lines.filter((l) => !seen.has(l.id) && seen.add(l.id));
			older.set(key, [...(older.get(key) || []), ...got]);
			list.push(...got);
			body.append(...got.flatMap(rows));
			if (r.lines.length) last = r.lines[r.lines.length - 1];
			done = !r.more;
			said();
			foot.replaceChildren(done ? h('span', { class: 'sub' }, 'No older line is kept.') : button);
		} catch (err) {
			foot.append(h('span', { class: 'error' }, ` ${err.message}`));
		} finally {
			loading = false;
			button.disabled = false;
		}
	};
	if (!done) foot.append(button);
	const table = list.length ? h('div', { class: 'widetable scrollbox attemptsbox' },
		h('table', { class: 'list dense attempts' }, h('thead', null, h('tr', null, ['When', 'What', ''].map((t) => h('th', null, t)))), body)) : null;
	if (table) {
		table.addEventListener('scroll', () => {
			scrolled.set(key, table.scrollTop);
			if (table.scrollTop > 0 && table.scrollTop + table.clientHeight >= table.scrollHeight - 60) load();
		}, { passive: true });
		const y = scrolled.get(key);
		if (y) requestAnimationFrame(() => { table.scrollTop = y; });
	}
	const what = GROUPS.find(([k]) => k === group)?.[1].toLowerCase();
	return [h('section', { class: 'panel' },
		h('h2', null, 'What it did', list.length > 0 && count),
		table || h('p', { class: 'sub pad' }, group === 'all' ? 'Nothing it did is kept yet.' : `Nothing of its ${what} is kept yet.`),
		foot)];
}
