// The Clients tab (0066): every Wi-Fi client on an AP, or on the APs below a
// folder, as each last reported. Who it is, with its maker and a guess at
// what it is (0067); where and how it connects, the 802.11 features it
// supports, and how often frames to it are retried; its data; and how it
// does with DHCP (0065). One compact row a client; filters by network and AP,
// a search box, and sorting by any column. The filters stay as they are when
// the page refreshes.

import { h, link } from '../dom.js';
import { get, post } from '../api.js';
import { flash } from '../refresh.js';
import { called, given, itsName } from '../names.js';
import { bandName, ago, size } from '../format.js';
import { configs } from './sections.js';
import { ask, confirm } from './confirm.js';
import { clientLink, renameClient } from './journey.js';
import { traceWindow } from './trace.js';
import { outcomeChip, failedJoins } from './connections.js';
import { mayAt, mayAnywhere } from '../rights.js';
import { onNow } from './onnow.js';
import { bandOf, pageBand } from '../layout.js';

const DHCP = { ok: ['ok', 'DHCP'], static: ['idle', 'static'], none: ['bad', 'no address'], unknown: ['idle', 'not yet'] };

// What the person chose, kept across the page's refreshes.
// The weakest signal first (0284).
const view = { network: '', ap: '', q: '', sort: 'signal', up: true, which: 'now', find: '' };

// The columns shown until the viewer shows more (0284): the client, its
// AP, network and band, its signal and rate, its joins and its address.
const HIDDEN = ['Channel', 'Width', 'MCS', 'Streams', 'Wi-Fi', 'Features', 'Retries', 'Connected', 'Data', 'DHCP', 'First seen', 'Idle', 'Asks for most'];

// More of each client than its AP's report has, by the node the list is of:
// seen, each client's row of those ever seen (0118), for how its joins went
// and when it was first seen; asks, what it asks the DNS server for most
// (0154). Kept from one draw to the next, so that a page drawn again shows
// what was had until it is read anew.
const extra = new Map();

// clientsTab draws the tab: of one AP (ap: {ap, cfg}), or of every AP below
// a folder's page. It shows the clients on now, or everyone ever seen here
// (0118); either way a client opens its journey and its connections.
export async function clientsTab(ctx, page, ap) {
	const rows = ap ? [ap] : await configs(page.hardware?.aps || []);
	if (!rows.length) return h('div', { class: 'banner info' }, 'No APs here yet.');
	const now = Date.now() / 1000;
	// Each client once, on the AP it was last active on; connected for as
	// long as it was at the report, and since.
	const all = onNow(rows).map(({ c, ap: a, cfg, st, at }) => ({ ...c, ap: a, cfg, at: st?.at, connected: c.connected == null ? null : c.connected + Math.max(0, now - at) }));
	const under = ap ? ap.ap.id : page.node?.id;
	// The list has more columns than a window may be wide: it is scrolled
	// across, and not cut off at its panel's edge, in a box that scrolls
	// down too, so that the way across is always in sight.
	const box = h('div', { class: 'widetable clientsbox' });
	// A block's preview, or a client's journey, opens here, above the
	// table (0100, 0103).
	const out = h('div', { class: 'edit' });
	const bar = h('div');
	const note = h('span', { class: 'note' });
	const which = h('span', { class: 'segmented', role: 'group', 'aria-label': 'Which clients' });
	const draw = () => {
		which.replaceChildren(...[['now', 'On now'], ['all', 'Everyone seen']].map(([k, name]) => h('button', {
			type: 'button', class: view.which === k ? 'on' : '', 'aria-pressed': view.which === k, onclick: () => { view.which = k; draw(); },
		}, name)));
		if (view.which === 'all') {
			note.textContent = 'the latest first';
			note.title = 'Every client an AP here has seen, the latest first; a client opens its journey and each time it came online.';
			everyone(ctx, under, !ap, bar, box);
			return;
		}
		// What the rows are made of, on hover (0284).
		note.textContent = '';
		note.title = "Where each client is follows its AP's minutes; the rest of a row, each AP's report every few minutes. A client opens its journey.";
		bar.replaceChildren(all.length ? controls(all, !ap, draw) : '');
		list();
	};
	const more = extra.get(under || '') || { seen: new Map(), asks: new Map() };
	extra.set(under || '', more);
	more.under = under || '';
	// The table alone, drawn again when more of the clients comes, so that a
	// search someone is typing is not taken from under them.
	const list = () => box.replaceChildren(all.length ? table(ctx, all, !ap, draw, out, more)
		: h('p', { class: 'sub pad' }, 'No Wi-Fi client is on here now.'));
	draw();
	// Asked for once the list is drawn, and drawn in as each comes.
	if (all.length) {
		const fill = (path, take) => get(path).then((r) => {
			take(r);
			if (box.isConnected && view.which === 'now') list();
		}).catch(() => {});
		fill(`/v1/clients?under=${encodeURIComponent(under || '')}&limit=500`, (r) => { more.seen = new Map(r.clients.map((c) => [c.mac, c])); });
		fill(`/v1/lookups/top?under=${encodeURIComponent(under || '')}`, (r) => { more.asks = new Map(Object.entries(r.asks || {})); });
	}
	// Any client can be traced by its MAC (0144), on or not, known here
	// or not: in a window of its own (0149).
	const who = h('input', { type: 'search', class: 'mono', placeholder: 'aa:bb:cc:00:11:22', 'aria-label': 'The MAC of a client to trace', 'data-kept': true, size: 18 });
	const go = () => {
		const mac = who.value.trim().toLowerCase().replace(/-/g, ':');
		who.setCustomValidity(/^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/.test(mac) ? '' : 'A MAC, such as aa:bb:cc:00:11:22');
		if (!who.reportValidity()) return;
		traceWindow(mac);
	};
	who.addEventListener('keydown', (e) => { if (e.key === 'Enter') go(); });
	who.addEventListener('input', () => who.setCustomValidity(''));
	return h('section', { class: 'panel' },
		h('h2', null, h('span', { class: 'title' }, 'Wi-Fi clients', which), note),
		bar, out, box,
		h('div', { class: 'filters traceany' }, h('span', { class: 'sub' }, 'Live Trace a client, on or not:'), who,
			h('button', { type: 'button', class: 'button small', onclick: go }, 'Trace')));
}

// everyone draws every client ever seen below a node (0118), the latest
// first, a hundred at a time: when and where each was last seen, what it
// says of itself, and how its attempts to come online went.
async function everyone(ctx, under, folder, bar, box) {
	let rows = [], total = 0;
	const count = h('span', { class: 'note' });
	const q = h('input', { type: 'search', placeholder: 'Name, user, MAC or address', value: view.find, 'data-kept': true });
	bar.replaceChildren(h('div', { class: 'filters' }, q, count));
	const paint = (err) => {
		count.textContent = err || `${total} client${total === 1 ? '' : 's'}`;
		// What is false is left out: the page writes it as a word otherwise.
		box.replaceChildren(...[h('table', { class: 'list clients' },
			h('tr', null, ['Client', 'Last seen', folder && 'Last on', 'Network', 'Address', 'Came online', 'Last time'].filter(Boolean).map((t) => h('th', null, t))),
			rows.map((c) => h('tr', null,
				h('td', null, clientLink(c.mac, called(c.mac, c.host) || h('span', { class: 'mono' }, c.mac)), ownName(c), called(c.mac, c.host) && h('div', { class: 'sub mono' }, c.mac),
				c.user && h('div', { class: 'sub' }, 'signed in as ', h('strong', null, c.user)),
				c.tester ? own(c.tester) : (c.maker || c.private) && h('div', { class: 'sub' }, c.private ? 'private address' : c.maker)),
				h('td', null, ago(c.last_seen), h('div', { class: 'sub' }, `first ${ago(c.first_seen)}`)),
				folder && h('td', null, c.ap_name ? link(`/aps/${encodeURIComponent(c.ap)}`, c.ap_name) : c.ap),
				h('td', null, c.ssid || c.network || '—'),
				h('td', { class: 'mono' }, c.address || '—'),
				h('td', null, c.attempts ? `${c.attempts} time${c.attempts === 1 ? '' : 's'}` : '—',
					c.failed > 0 && h('div', { class: 'sub' }, failedChip(ctx, c, c.failed, { under }))),
				h('td', null, c.last_outcome ? outcomeChip({ outcome: c.last_outcome, stage: '', took_ms: 0 }, true) : '—'))),
			!rows.length && h('tr', null, h('td', { colspan: folder ? 7 : 6, class: 'sub' }, err ? '' : view.find ? 'No client matches.' : 'No client has been seen here yet.'))),
		rows.length < total && h('div', { class: 'below' }, h('button', { type: 'button', class: 'button', onclick: () => load(true) }, `More (${total - rows.length} left)`))].filter(Boolean));
	};
	const load = async (more) => {
		try {
			const r = await get(`/v1/clients?under=${encodeURIComponent(under || '')}&q=${encodeURIComponent(view.find)}&limit=100&offset=${more ? rows.length : 0}`);
			rows = more ? rows.concat(r.clients) : r.clients;
			total = r.total;
			paint();
		} catch (e) {
			paint(e.message);
		}
	};
	let timer;
	q.addEventListener('input', () => {
		view.find = q.value.trim();
		clearTimeout(timer);
		timer = setTimeout(() => load(false), 250);
	});
	await load(false);
}

function controls(all, folder, draw) {
	const pick = (key, label, values) => {
		// data-kept: the page keeps a filter across redraws (refresh.js).
		const sel = h('select', { 'data-kept': true, onchange: () => { view[key] = sel.value; draw(); } },
			h('option', { value: '' }, label),
			values.map(([v, name]) => h('option', { value: v, selected: view[key] === v }, name)));
		return sel;
	};
	const ssids = [...new Set(all.map((c) => c.ssid).filter(Boolean))].sort();
	const aps = [...new Map(all.map((c) => [c.ap.id, c.ap.name])).entries()].sort((a, b) => a[1].localeCompare(b[1]));
	const q = h('input', { type: 'search', placeholder: 'Name, user, MAC or address', value: view.q, 'data-kept': true, oninput: () => { view.q = q.value; draw(); } });
	// The band, one or all, is the page's picker's (0284): the clients on
	// now are the one list that may show every band (Griff, 2026-10-22).
	return h('div', { class: 'filters' },
		pick('network', 'Every network', ssids.map((s) => [s, s])),
		folder && pick('ap', 'Every AP', aps),
		q,
		h('span', { class: 'note' }, `${all.length} client${all.length === 1 ? '' : 's'}`));
}

// The columns, each with how a row sorts by it. The band, the signal, the
// rate, its MCS and its streams are a column each (Griff, 2026-10-12), so
// that each can be sorted by, made wider or narrower, and shown or hidden
// (0143); the channel and its width too (Griff, 2026-10-15: "those need to
// be three seperate columns").
const COLUMNS = [
	['client', 'Client', (c) => (called(c.mac, c.host) || c.mac).toLowerCase()],
	['ap', 'AP', (c) => c.ap.name.toLowerCase()],
	['ssid', 'Network', (c) => c.ssid || ''],
	['band', 'Band', (c) => ['2g', '5g', '6g'].indexOf(c.band)],
	['channel', 'Channel', (c) => radioOf(c)?.channel ?? -1],
	['width', 'Width', (c) => radioOf(c)?.width ?? -1],
	// One whose signal its AP did not say comes after the weakest.
	['signal', 'Signal', (c) => c.signal ?? 1],
	['rate', 'Rate', (c) => c.tx_rate ?? 0],
	['mcs', 'MCS', (c) => c.tx_mcs ?? -1],
	['streams', 'Streams', (c) => c.tx_nss ?? 0],
	['wifi', 'Wi-Fi', (c) => ['', 'n', 'ac', 'ax', 'be'].indexOf(c.gen || '')],
	['features', 'Features', (c) => (c.k ? 8 : 0) + (c.v ? 4 : 0) + (c.w ? 2 : 0) + (c.r ? 1 : 0)],
	['retries', 'Retries', (c) => retries(c).retried ?? -1],
	['address', 'Address', (c) => (c.address || '').split('.').map((x) => x.padStart(3, '0')).join('.')],
	['connected', 'Connected', (c) => c.connected ?? -1],
	['data', 'Data', (c) => (c.tx_bytes ?? 0) + (c.rx_bytes ?? 0)],
	['dhcp', 'DHCP', (c) => c.dhcp || ''],
	// How its joins went over the last day, and when the network first met
	// it (0118).
	['joins', 'Joins, 24 h', (c, more) => more.seen.get(c.mac)?.recent?.attempts ?? -1],
	['first', 'First seen', (c, more) => new Date(more.seen.get(c.mac)?.first_seen || 0).getTime()],
	['idle', 'Idle', (c) => c.inactive_ms ?? -1],
	// What it asks the DNS server for most (0154): what a device is.
	['asks', 'Asks for most', (c, more) => more.asks.get(c.mac)?.places?.[0]?.domain || '\uffff'],
];

// bandsOf is the bands clients are on.
const bandsOf = (all) => [...new Set(all.map((c) => c.band).filter(Boolean))];

function table(ctx, all, folder, draw, out, more) {
	const q = view.q.trim().toLowerCase();
	const band = pageBand() ?? bandOf(bandsOf(all), true);
	// Found by what it asks for too: every client that asks for a domain.
	const shown = all.filter((c) => (band === 'all' || !band || c.band === band) && (!view.network || c.ssid === view.network) && (!view.ap || c.ap.id === view.ap) &&
		(!q || [called(c.mac, ''), c.host, c.mac, c.address, c.ssid, c.maker, c.kind, c.os, c.user, ...(more.asks.get(c.mac)?.places || []).map((p) => p.domain)].some((x) => (x || '').toLowerCase().includes(q))));
	const col = COLUMNS.find(([k]) => k === view.sort) || COLUMNS.find(([k]) => k === 'signal');
	shown.sort((a, b) => {
		const x = col[2](a, more), y = col[2](b, more);
		return (x < y ? -1 : x > y ? 1 : 0) * (view.up ? 1 : -1);
	});
	const head = COLUMNS.filter(([k]) => folder || k !== 'ap').map(([k, name]) => h('th', {
		class: 'sortable', onclick: () => { view.up = view.sort === k ? !view.up : true; view.sort = k; draw(); },
	}, name, view.sort === k ? (view.up ? ' ▴' : ' ▾') : ''));
	return h('table', { class: 'list clients', 'data-hide': JSON.stringify(HIDDEN) },
		h('tr', null, head),
		shown.map((c) => h('tr', null,
			h('td', null, clientLink(c.mac, called(c.mac, c.host) || h('span', { class: 'mono' }, c.mac)), ownName(c), called(c.mac, c.host) && h('div', { class: 'sub mono' }, c.mac),
					c.user && h('div', { class: 'sub', title: 'who it signed in as, by 802.1X' }, 'signed in as ', h('strong', null, c.user)),
					c.test_client ? own() : who(c),
					!c.test_client && h('div', { class: 'rowbuttons' }, renameButton(ctx, c, out), traceButton(c), reconnectButton(ctx, c, out), blockButton(ctx, c, out))),
			folder && h('td', null, link(`/aps/${encodeURIComponent(c.ap.id)}`, c.ap.name)),
			h('td', null, c.ssid || '—', c.vlan && h('div', { class: 'sub', title: vlanBy(c) }, `VLAN ${c.vlan}`)),
			h('td', { class: 'nowrap' }, bandName(c.band) || '—'),
			h('td', null, radioOf(c)?.channel ?? '—'),
			h('td', { class: 'nowrap' }, radioOf(c)?.width ? `${radioOf(c).width} MHz` : '—'),
			h('td', { class: 'nowrap' }, c.signal != null ? `${c.signal} dBm` : '—'),
			h('td', { class: 'nowrap', title: 'What the AP sends it at, and under it what it sends the AP at' }, both(c.tx_rate, c.rx_rate, (r) => `${Math.round(r)} Mbit/s`)),
			h('td', { title: 'The MCS index of each' }, both(c.tx_mcs, c.rx_mcs, String)),
			h('td', { title: 'The spatial streams of each' }, both(c.tx_nss, c.rx_nss, String)),
			h('td', { class: 'nowrap', title: "The Wi-Fi generation this connection uses; the AP's radio may cap it" }, generation(c)),
			h('td', null, features(c)),
			h('td', null, retryCell(c)),
			h('td', { class: 'mono' }, c.address || '—'),
			c.arrived != null ? h('td', { class: 'nowrap', title: "On this AP since its last report, by its minutes (0251) or as it joined (0252): how long, its data, MCS and streams at its next report" }, 'new here')
				: h('td', { title: `reported ${ago(c.at)}` }, duration(c.connected)),
			c.tx_bytes == null ? h('td', null, '—') : h('td', null, `↓ ${size(c.tx_bytes)}`, h('div', { class: 'sub' }, `↑ ${size(c.rx_bytes)}`)),
			h('td', null, c.dhcp ? h('span', { class: 'chip ' + DHCP[c.dhcp][0] }, DHCP[c.dhcp][1]) : '—'),
			joinsCell(ctx, more.seen.get(c.mac), more.under),
			h('td', { class: 'nowrap' }, more.seen.get(c.mac)?.first_seen ? ago(more.seen.get(c.mac).first_seen) : '—'),
			h('td', { class: 'nowrap', title: 'How long since the AP last heard anything from it, at its report' }, c.inactive_ms != null ? idle(c.inactive_ms) : '—'),
			asksCell(more.asks.get(c.mac)))),
		!shown.length && h('tr', null, h('td', { colspan: folder ? 22 : 21, class: 'sub' }, 'No client matches.')));
}

// ownName is the name a device gives of itself, under the one a person gave
// it (0153): both are shown.
function ownName(c) {
	const mine = given(c.mac), its = itsName(c.mac, c.host);
	return mine && its && its !== mine ? h('div', { class: 'sub' }, its) : null;
}

// radioOf is the radio a client is on, as its AP's report has that band's
// radio, with its channel and width: where the AP has one radio on the band.
function radioOf(c) {
	const radios = (c.cfg?.condition?.state?.report?.radios || []).filter((r) => r.band === c.band && r.up !== false && r.channel);
	return radios.length === 1 ? radios[0] : null;
}

// joinsCell is how many times a client tried to come online over the last
// day, and how many of those failed; since it was first seen, in its title
// (0118). A count since first seen says little of now. The failed count
// opens those failures alone (0330).
function joinsCell(ctx, s, under) {
	if (!s) return h('td', null, '—');
	const r = s.recent || { attempts: 0, failed: 0 };
	const title = [`${s.attempts} since it was first seen, ${s.failed} failed`, s.last_outcome && `the last: ${s.last_outcome}`].filter(Boolean).join(' · ');
	return h('td', { class: 'nowrap', title }, String(r.attempts),
		r.failed > 0 && h('div', { class: 'sub' }, failedChip(ctx, s, r.failed, { under, hours: 24 })));
}

// failedChip is how many of a client's joins failed, as a button that
// shows them alone (0330).
function failedChip(ctx, c, n, opts) {
	return h('button', { type: 'button', class: 'chip bad chipbutton', title: 'Show these failures alone, each with where it stopped and why',
		onclick: (e) => {
			e.stopPropagation();
			failedJoins(ctx, c.mac, called(c.mac, c.host) || c.mac, opts);
		} }, `${n} failed`);
}

// idle writes milliseconds the short way.
function idle(ms) {
	if (ms < 1000) return `${ms} ms`;
	if (ms < 60000) return `${Math.round(ms / 1000)} s`;
	return `${Math.round(ms / 60000)} min`;
}

// asksCell is the domains a client asks the DNS server for most (0154),
// each with its share of the client's lookups: what the device is, at a
// glance.
function asksCell(a) {
	if (!a?.places?.length) return h('td', null, '—');
	return h('td', { class: 'asks', title: `${a.total} lookup${a.total === 1 ? '' : 's'} in the last day, by its address now` },
		a.places.map((p) => h('div', { class: 'nowrap' }, p.domain, h('span', { class: 'sub' }, ` ${Math.round(100 * p.times / (a.total || 1))}%`))));
}

// renameButton names a client from its row (0153): what a person calls it,
// in place of what it calls itself, or of nothing.
function renameButton(ctx, c, out) {
	if (!c.mac || !mayAt('locations', c.ap.id, 'clients')) return null;
	return h('button', { type: 'button', class: 'button small', title: 'Give this client a name of your own, shown wherever it is',
		onclick: () => { out.scrollIntoView({ block: 'nearest' }); renameClient(out, c.mac, itsName(c.mac, c.host)); } }, 'Rename');
}

// vlanBy says what put a client in its VLAN: on WPA Enterprise, the RADIUS
// server (0111); otherwise its per-user key (0070).
function vlanBy(c) {
	const security = c.network && c.cfg?.networks?.[c.network]?.fields?.security?.value;
	return String(security || '').endsWith('-enterprise') ? 'the RADIUS server put it in this VLAN' : 'a per-user key put it in this VLAN';
}

// blockButton blocks a client from its network, by MAC (0100), where Aeolus
// gives the network: it joins the network's blocked list where that is set,
// else where the network is, in one change previewed first.
function blockButton(ctx, c, out) {
	const net = c.network && c.cfg?.networks?.[c.network];
	if (!net || !c.mac) return null;
	const f = net.fields?.blocked;
	const node = f?.from ?? net.from;
	// Only for someone with the clients right on the network there (0177).
	if (!mayAt('services', node, 'clients')) return null;
	const now = f?.value ?? [];
	if (now.map((m) => m.toLowerCase()).includes(c.mac.toLowerCase())) return h('span', { class: 'chip bad' }, 'blocked');
	const block = async () => {
		const op = { kind: 'set', tree: 'services', node, path: `network.${c.network}.blocked`, value: [...now, c.mac.toLowerCase()] };
		out.scrollIntoView({ block: 'nearest' });
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `Block ${called(c.mac, c.host) || c.mac} from ${c.ssid || c.network}`)),
			h('div', { class: 'sub' }, `Every AP that offers ${c.ssid || c.network} refuses ${c.mac}, on every band. It is set on ${ctx.name('services', node)}, and takes it off the air within about a minute.`),
			c.private && h('div', { class: 'sub warn' }, 'Its MAC is private: the device may pick a new one, and join again.'),
		], [h('div', { class: 'sub warn' }, 'Applying reloads the Wi-Fi of each AP listed: its clients drop for a moment.')]);
	};
	return h('button', { type: 'button', class: 'button small', title: `Refuse ${c.mac} on ${c.ssid || c.network}`, onclick: block }, 'Block');
}

// traceButton opens a client's Live Trace (0144) from its row, in a window
// of its own (0149): beside Reconnect, as the two are used together, the
// one to see what the other brings about.
function traceButton(c) {
	if (!c.mac || !mayAnywhere('clients')) return null;
	return h('button', { type: 'button', class: 'button small', title: 'Every AP says each thing it sees of this client as it happens, in a window of its own',
		onclick: () => traceWindow(c.mac, called(c.mac, c.host), c.ap && { ap: c.ap.id, ap_name: c.ap.name }) }, 'Live Trace');
}

// reconnectButton asks the client's AP to disconnect it, once (0107): it
// drops off every network it is on there, and may join again at once,
// usually within seconds. For someone with the clients right on the AP (0177).
function reconnectButton(ctx, c, out) {
	if (!c.mac || !mayAt('locations', c.ap.id, 'clients')) return null;
	const who = called(c.mac, c.host) || c.mac;
	const go = async (e) => {
		e.currentTarget.disabled = true;
		try {
			await post(`/v1/aps/${encodeURIComponent(c.ap.id)}/actions`, { kind: 'disconnect', target: c.mac });
			flash(`Reconnect: asked of ${c.ap.name}. ${who} drops as the AP hears, at once where it holds its line to the manager, and joins again.`);
		} catch (err) {
			out.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	};
	return h('button', { type: 'button', class: 'button small', title: `Have ${c.ap.name} disconnect ${who}, so it joins again`, onclick: go }, 'Reconnect');
}

// who says what a client is (0067): its maker by OUI, or that its MAC is
// private; and the manager's guess at its kind and system, with what the
// guess went on.
// own marks a client that is an AP's scan radio, joined for a client test
// (0121): no device of anyone's, so nothing to block or to look into.
export function own(ap) {
	return h('div', { class: 'sub' }, h('span', { class: 'chip from', title: "An AP's scan radio, joining as a client would to test the network (Assurance). It is not a device of anyone's." },
		'Aeolus test client'), ap && ap !== 'an AP' ? ` ${ap}'s scan radio` : '');
}

function who(c) {
	const maker = c.private ? 'private MAC' : (c.maker || '').replace(/,? (inc|ltd|llc|co|corp|corporation|gmbh|limited)\.?$/i, '');
	const kind = [c.kind, c.os].filter(Boolean).join(', ');
	const line = [maker, kind].filter(Boolean).join(' · ');
	return line && h('div', { class: 'sub', title: c.basis ? `guessed from its ${c.basis}` : '' }, line);
}

// generation is the Wi-Fi generation a client's connection uses (0067): a
// column of its own, with nothing else in it (Griff, 2026-10-12).
function generation(c) {
	if (c.k == null) return '—';   // hostapd didn't say
	return { n: 'Wi-Fi 4', ac: 'Wi-Fi 5', ax: 'Wi-Fi 6', be: 'Wi-Fi 7' }[c.gen] || 'a/b/g';
}

// features shows the features k, v, w and r that a client's connection
// uses, as small chips (0067), in a column of their own. The AP's radio and
// the SSID cap them, so a client may be able to do more. 802.11r is by
// which key management the client chose, as hostapd says on its control
// socket (0150); an AP whose agent does not yet ask leaves it not known.
function features(c) {
	if (c.k == null) return '—';   // hostapd didn't say
	const chip = (on, name, what) => h('span', { class: 'chip ' + (on ? 'ok' : 'idle'), title: `${what}: ${on ? 'in use' : 'not in use'} on this connection` }, name);
	return h('div', { class: 'chips' },
		chip(c.k, 'k', '802.11k, neighbour reports'), ' ', chip(c.v, 'v', '802.11v, BSS transition'), ' ',
		chip(c.w, 'w', '802.11w, protected management frames'), ' ',
		c.r == null ? h('span', { class: 'chip idle', title: '802.11r, fast transition: not known. This AP did not say which key management the client chose' }, 'r?')
			: chip(c.r, 'r', '802.11r, fast transition'));
}

// retries is the share, in percent, of frames to a client that were
// retried, and that failed, since it joined (0067).
function retries(c) {
	const sent = (c.tx_packets || 0) + (c.tx_failed || 0);
	// Not known where the AP's radio counted none for it (0164).
	const retried = c.tx_retries == null ? null : sent ? 100 * c.tx_retries / sent : 0;
	return { retried, failed: sent ? 100 * (c.tx_failed || 0) / sent : 0, sent };
}

function retryCell(c) {
	const r = retries(c);
	if (!r.sent) return '—';
	if (r.retried == null) return h('span', { class: 'sub', title: `retries not counted: it joined before its AP's radio began counting them, which it does from its next join · ${c.tx_failed} failed of ${r.sent} frames sent` }, '—');
	const worst = Math.max(r.retried, r.failed);
	const pct = (x) => x < 1 && x > 0 ? '<1 %' : `${Math.round(x)} %`;
	// Its retries, and no more (Griff, 2026-10-11): the frames that failed
	// are in the chip's colour, and on hover.
	return h('span', { class: 'chip ' + (worst > 25 ? 'bad' : worst > 10 ? 'warn' : 'ok'), title: `${c.tx_retries} retries and ${c.tx_failed} failed of ${r.sent} frames sent` }, `${pct(r.retried)} retries`);
}

// rate writes a client's rates, down (the AP sending to it) and up, in
// Mbit/s, with the MCS of each, and the spatial streams where the AP says.
// both writes one thing of a client each way: what the AP sends it at, and
// under it what it sends the AP at; one line where they are the same, and a
// dash where the AP says neither.
function both(down, up, say) {
	if (down == null && up == null) return '—';
	if (down != null && up != null && say(down) === say(up)) return say(down);
	return [`↓ ${down != null ? say(down) : '—'}`, h('div', { class: 'sub' }, `↑ ${up != null ? say(up) : '—'}`)];
}

// duration writes seconds the short way: 47 s, 12 min, 3 h 5 min, 2 d 4 h.
function duration(s) {
	s = Math.floor(s);
	if (s < 60) return `${s} s`;
	if (s < 3600) return `${Math.floor(s / 60)} min`;
	if (s < 86400) return `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} min`;
	return `${Math.floor(s / 86400)} d ${Math.floor((s % 86400) / 3600)} h`;
}

