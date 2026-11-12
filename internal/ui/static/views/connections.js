// A client's attempts to come online (0118): each one, passed or failed,
// opens to every step of it, as its AP recorded them: where it was heard
// asking, authentication, association, sign-in, its keys, DHCP's four
// messages, the gateway, its first DNS lookups and its first connection,
// each with its time. Each says which AP it was on and where that AP is in
// Locations, and one that failed has its capture nested in it (0147). They
// are listed on the client's own page, a line each (0222).

import { h, link } from '../dom.js';
import { download, get } from '../api.js';
import { bandName, ago } from '../format.js';
import { wayPath } from './way.js';
import { KIND, kindWords, took as roamTook } from './roaming.js';
import { packetsWindow } from './packetview.js';

// The stages of coming online, in order, as they are named.
const STAGES = [
	['auth', 'Authentication'], ['assoc', 'Association'], ['signin', 'Sign-in'], ['key', 'Keys'],
	['dhcp', 'DHCP'], ['gateway', 'Gateway'], ['dns', 'DNS'], ['internet', 'First connection'],
];
const NAME = { ...Object.fromEntries(STAGES), end: 'Left', note: 'hostapd says' };

const when = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
const took = (ms) => (ms < 1000 ? `${ms} ms` : ms < 60000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.round(ms / 60000)} min`);

// outcomeChip says what an attempt came to: online, and how fast; failed,
// and at which stage; left; or connected, with nothing more seen of it.
// brief leaves out how fast and where, for a list of clients.
export function outcomeChip(c, brief) {
	const at = NAME[c.stage] || c.stage;
	if (c.outcome === 'online') return h('span', { class: 'chip ok', title: 'An address, and something beyond itself answered it' }, brief ? 'Online' : `Online in ${took(c.took_ms)}`);
	if (c.outcome === 'failed') return h('span', { class: 'chip bad' }, brief ? 'Failed' : `Failed at ${at}`);
	// One that went before it was let on left at the stage it had come to, not after it.
	const early = (c.reason || '').startsWith('before it was let on');
	if (c.outcome === 'left') return h('span', { class: 'chip warn', title: early ? 'It went, or was sent off, before it was let on' : 'It went of its own accord before it was seen online' },
		brief ? 'Left' : `Left ${early ? 'at' : 'after'} ${at}`);
	return h('span', { class: 'chip idle', title: 'Let on to the network, with nothing more seen of it: a device that keeps its address and says little' }, 'Connected');
}

// way is the attempt's way online, drawn (0130): each point with how long
// it took, as its AP saw it, and under it what was learned there. A point
// not seen is not a fault: a client that kept its address asks for none.
function way(c) {
	const r = c.record || {};
	return wayPath(c.way, { what: 'The client’s way online',
		sub: { auth: r.signal != null ? `${r.signal} dBm` : null, dhcp: r.address || null } });
}

// more is what else a step says: who answered, what DHCP gave, how long an
// answer took.
function more(e) {
	return [
		e.server && `from ${e.server}`,
		e.router && `gateway ${e.router}`,
		e.dns && `DNS ${e.dns.split(' ').join(', ')}`,
		e.holder && `at ${e.holder}`,
		e.ms != null && `in ${e.ms} ms`,
	].filter(Boolean).join(' · ');
}

// heard says where the client was heard asking for networks before it
// joined, the strongest first.
function heard(c) {
	const p = c.record?.probes;
	if (!p?.length) return 'Not recorded: band steering (usteer), which hears clients ask for networks, is not running on this AP.';
	return 'Heard asking for networks ' + p.map((x) => `${x.ap ? `by the AP at ${x.ap}` : `by this AP${x.band ? ` on ${bandName(x.band)}` : ''}`} at ${x.signal} dBm`).join('; ') + '.';
}

const size = (n) => (n < 1000 ? `${n} bytes` : n < 1e6 ? `${(n / 1000).toFixed(n < 10000 ? 1 : 0)} kB` : `${(n / 1e6).toFixed(1)} MB`);

// packetsLink opens the capture kept of an attempt (0135) in the packet
// viewer (0341), a window that floats over the page: its packets in a
// list, each taken apart, and its bytes; the file is downloaded from there.
function packetsLink(c, label = 'Capture') {
	const k = c.capture;
	if (!k?.may) return null;
	const a = h('a', { class: 'capture', href: '#', role: 'button',
		title: `The client’s frames in this attempt, as the AP heard and sent them and as they crossed its wire side: ${k.frames} frame${k.frames === 1 ? '' : 's'}, ${size(k.bytes)}. Opens them in the packet viewer, where the file can be downloaded for Wireshark.` }, label);
	a.addEventListener('click', (e) => {
		// In a summary, a click would open or close the attempt too.
		e.preventDefault();
		e.stopPropagation();
		packetsWindow(`/v1/connections/${c.id}/capture`, `${c.host || c.mac || ''} · ${new Date(c.started).toLocaleString(undefined, { hour12: false })}`, c.started);
	});
	return a;
}

// captureLink is the capture kept of an attempt that failed (0135), to be
// had as a file: the client's frames as the AP heard and sent them, and
// those that crossed its wire side. An admin of its AP may have it (0177).
// Nothing where none is kept, or for anyone else. label is what the link says.
function captureLink(c, label = 'Capture') {
	const k = c.capture;
	if (!k?.may) return null;
	const holds = `${k.frames} frame${k.frames === 1 ? '' : 's'}, ${size(k.bytes)}`;
	const a = h('a', { class: 'capture', href: '#', role: 'button',
		title: `The client’s frames in this attempt, as the AP heard and sent them and as they crossed its wire side: ${holds}. A pcapng file, which Wireshark opens.` }, label);
	a.addEventListener('click', async (e) => {
		// In a summary, a click would open or close the attempt too.
		e.preventDefault();
		e.stopPropagation();
		if (a.classList.contains('busy')) return;
		a.classList.add('busy');
		a.textContent = `${label}…`;
		try {
			await download(`/v1/connections/${c.id}/capture`);
			a.textContent = label;
		} catch (err) {
			a.textContent = label;
			a.after(h('span', { class: 'capture bad', role: 'alert' }, ` ${err.message}`));
		}
		a.classList.remove('busy');
	});
	return a;
}

// The captures opened, by their attempts' IDs, as the attempts are.
const framesOpened = new Set();

// station is who a frame was from or for, as the attempt has them: the
// client, every station, the AP by its network's address (ap), or another
// by its address: one of the AP's other networks answering a probe, or on
// the wire side whoever answered.
function station(c, mac, ap) {
	if (!mac) return '';
	if (mac === c.mac) return 'Client';
	if (mac === 'ff:ff:ff:ff:ff:ff') return 'Everyone';
	if (mac === ap) return h('span', { title: `The network’s own address on this AP, ${mac}` }, 'AP');
	return h('span', { class: 'mono' }, mac);
}

// networkAddress is the AP's address for the network the client came to,
// from a capture's frames: the one the client's own frames on the Wi-Fi
// side were for, most often, those to every station left out.
function networkAddress(c, frames) {
	const n = new Map();
	for (const f of frames)
		if (f.side === 'radio' && f.from === c.mac && f.to && f.to !== 'ff:ff:ff:ff:ff:ff') n.set(f.to, (n.get(f.to) || 0) + 1);
	return [...n.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] || null;
}

// captureFrames is the capture of an attempt that failed, nested in it
// (0147): a line that opens to every frame of it in words, the AP's and the
// wire side's together by when they came, each timed from the attempt's
// start as its steps are. They are read when it is first opened.
function captureFrames(c) {
	const k = c.capture;
	if (!k?.may) return null;
	const body = h('div', { class: 'capframes' });
	const d = h('details', { class: 'fold nested', open: framesOpened.has(c.id) },
		h('summary', null,
			h('span', { class: 'who' }, 'Capture'),
			h('span', { class: 'grow sub' }, `${k.frames} frame${k.frames === 1 ? '' : 's'}, ${size(k.bytes)}`),
			packetsLink(c, 'Packets'), ' ', captureLink(c, 'Download')),
		body);
	let read = false;
	const load = async () => {
		if (read) return;
		read = true;
		body.replaceChildren(h('p', { class: 'sub' }, 'Reading its frames…'));
		try {
			const j = await get(`/v1/connections/${c.id}/capture/frames`);
			const after = (ms) => (Math.round(ms) < 0 ? `−${took(Math.round(-ms))}` : `+${took(Math.max(0, Math.round(ms)))}`);
			const ap = networkAddress(c, j.frames || []);
			body.replaceChildren(
				h('table', { class: 'list steps' },
					h('tr', null, ['After', 'Side', 'From', 'To', 'What', 'Signal'].map((t) => h('th', null, t))),
					(j.frames || []).map((f) => h('tr', { class: f.failed ? 'failed' : '' },
						h('td', { class: 'mono' }, after(f.after_ms)),
						h('td', null, f.side === 'radio' ? 'Wi-Fi' : 'Wire'),
						h('td', null, station(c, f.from, ap)),
						h('td', null, station(c, f.to, ap)),
						h('td', null, f.what, f.note && h('div', { class: 'sub' }, f.note)),
						h('td', { class: 'nowrap' }, f.signal != null ? `${f.signal} dBm` : '')))),
				h('p', { class: 'sub' }, `${j.radio} as the AP heard and sent them, ${j.wire} from its wire side. Times are from the attempt’s start; those before it are the client asking for networks.`));
		} catch (err) {
			read = false;
			body.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	};
	d.addEventListener('toggle', () => {
		if (d.open) { framesOpened.add(c.id); load(); } else framesOpened.delete(c.id);
	});
	if (d.open) load();
	return d;
}

// where is an AP as a place (0222): the folders it is in, the outermost
// first, the Org left out, then the AP itself: "Home › C360-AP".
export function where(ctx, ap, name) {
	const t = ctx.trees.locations;
	const up = [];
	for (let n = t.nodes.get(t.nodes.get(ap)?.parent); n?.parent; n = t.nodes.get(n.parent)) up.unshift(n.name);
	return [...up, name || ctx.name('locations', ap)].join(' › ');
}

// whatItWas says what an attempt was, in a line's words (0222): a join from
// nothing; a roam, of which kind and from where; or a band switch, from
// which band. A roam's time is how long it took to be let on.
function whatItWas(ctx, c) {
	const r = c.roam;
	if (!r) return 'Join';
	const kind = KIND[r.kind]?.[0] || r.kind;
	const ms = r.ms != null ? ` · ${roamTook(r.ms)}` : '';
	if (r.band_switch) return `Band switch from ${bandName(r.from_band) || 'another band'} · ${kindWords(r.kind)}${ms}`;
	const from = r.from_ap ? `${where(ctx, r.from_ap, r.from_ap_name)}${r.from_band ? ` · ${bandName(r.from_band)}` : ''}` : 'somewhere not known';
	return /roam$/i.test(kind) ? `${kind} from ${from}${ms}` : `Roam from ${from} · ${kind}${ms}`;
}

// steps is the attempt opened: its stages in a row, then each step with how
// long after the start it came.
function steps(c) {
	const r = c.record || {};
	const ev = r.events || [];
	return h('div', { class: 'attempt' },
		way(c),
		c.reason && h('p', { class: c.outcome === 'failed' ? 'why bad' : 'why' }, c.outcome === 'failed' ? `Why: ${c.reason}.` : `${c.reason}.`),
		h('table', { class: 'list steps' },
			h('tr', null, ['After', 'Stage', 'What happened', ''].map((t) => h('th', null, t))),
			h('tr', null, h('td', { class: 'mono' }, 'before'), h('td', null, 'Probes'), h('td', { colspan: 2, class: 'sub' }, heard(c))),
			ev.map((e) => h('tr', { class: e.ok === false ? 'failed' : '' },
				h('td', { class: 'mono' }, `+${took(e.t)}`),
				h('td', null, NAME[e.stage] || e.stage),
				h('td', null, e.what, more(e) && h('div', { class: 'sub' }, more(e))),
				h('td', null, e.ok === true ? h('span', { class: 'chip ok' }, 'ok') : e.ok === false ? h('span', { class: 'chip bad' }, 'failed') : null))),
			r.more > 0 && h('tr', null, h('td', null), h('td', { colspan: 3, class: 'sub' }, `and ${r.more} more step${r.more === 1 ? '' : 's'}, not kept`))),
		h('p', { class: 'sub' }, [
			r.signal != null && `Heard at ${r.signal} dBm when it was let on`,
			r.address && `address ${r.address}`,
			r.host && `calls itself ${r.host}`,
			r.bss && `on ${r.bss}`,
		].filter(Boolean).join(' · ')),
		captureFrames(c));
}

// The attempts opened, by their IDs, so a page drawn again keeps them open.
const opened = new Set();

// The older attempts read for a client, by its MAC, so a page drawn again
// keeps them too.
const older = new Map();

// A page of attempts, as the client's page reads them.
export const ATTEMPTS = 100;
// PAGE is how many attempts a client's Activity shows at first, and reads
// further back each time its box is scrolled near its end (0337).
export const PAGE = 20;
// kept is how many older attempts of a client's have been read on its page,
// so its next drawing reads the latest as far back as those, leaving no gap.
export const kept = (mac) => (older.get(mac) || []).length;
// Where each client's Activity box was scrolled, kept over redraws.
const scrolled = new Map();

// The cells of an attempt's line, by key: its column's head, and the cell.
const CELLS = {
	when: ['When', (ctx, c) => h('td', { class: 'mono' }, when(c.started))],
	where: ['Where', (ctx, c) => h('td', null, link(`/aps/${encodeURIComponent(c.ap)}/clients`, where(ctx, c.ap, c.ap_name)))],
	band: ['Band', (ctx, c) => h('td', null, bandName(c.band) || '—')],
	network: ['Network', (ctx, c) => h('td', null, c.ssid || c.network || '—')],
	what: ['What', (ctx, c) => h('td', { title: c.roam ? KIND[c.roam.kind]?.[1] || '' : 'It came online here from nothing: not from another AP or radio, moments before' }, whatItWas(ctx, c))],
	signal: ['Signal', (ctx, c) => h('td', null, c.record?.signal != null ? `${c.record.signal} dBm` : '—')],
	how: ['How it went', (ctx, c) => h('td', null, outcomeChip(c))],
	why: ['', (ctx, c) => h('td', { class: 'why' }, (c.outcome === 'failed' || c.outcome === 'left') && c.reason, packetsLink(c))],
};

// The columns of a list of attempts; of failures alone (0330), where each
// stopped and why first, so that they show however narrow the list.
const COLS = ['when', 'where', 'band', 'network', 'what', 'signal', 'how', 'why'];
const FAILED_COLS = ['when', 'how', 'why', 'where', 'band', 'network', 'signal', 'what'];

// attemptRows makes an attempt's line, of the columns cols, and the row
// under it that its steps open into: a click on the line opens or closes
// them. at is an attempt's ID to mark.
function attemptRows(ctx, at, cols = COLS) {
	const inside = (c) => h('div', null, steps(c),
		h('p', { class: 'sub attemptlinks' }, link(`/aps/${encodeURIComponent(c.ap)}/clients`, `${c.ap_name || c.ap}'s clients`)));
	return (c) => {
		const open = opened.has(c.id);
		const cell = h('td', { colspan: cols.length }, open && inside(c));
		const detail = h('tr', { class: 'stepsrow', hidden: !open }, cell);
		const line = h('tr', {
			id: `attempt-${c.id}`, title: 'Its steps',
			class: ['attemptrow', c.outcome === 'failed' && 'failed', open && 'opened', String(c.id) === String(at) && 'sel'].filter(Boolean).join(' '),
			onclick: (e) => {
				if (e.target.closest('a')) return;
				const now = !opened.has(c.id);
				if (now) opened.add(c.id);
				else opened.delete(c.id);
				line.classList.toggle('opened', now);
				detail.hidden = !now;
				if (now && !cell.firstChild) cell.append(inside(c));
			},
		},
		cols.map((k) => CELLS[k][1](ctx, c)));
		return [line, detail];
	};
}

// attemptsTable is attempts' lines, body, under the heads of cols: heads
// names any otherwise.
const attemptsTable = (body, cols = COLS, heads = {}, boxed = false) => h('div', { class: boxed ? 'widetable scrollbox attemptsbox' : 'widetable' },
	h('table', { class: 'list dense attempts' }, h('thead', null, h('tr', null, cols.map((k) => h('th', null, heads[k] ?? CELLS[k][0])))), body));

// failedJoins shows a client's failed attempts to come online alone (0330),
// over the page: over the last hours, where hours is given, and on the APs
// below under, where it is, as the count it was opened from has them. A
// line each, the latest first, saying where it stopped and why, which
// opens to its steps; above them, how many stopped where.
export async function failedJoins(ctx, mac, name, { under, hours } = {}) {
	const box = h('div', { class: 'failbody' }, h('p', { class: 'sub' }, 'Reading its failed joins…'));
	const over = hours ? `the last ${hours === 24 ? 'day' : `${hours} h`}` : 'since it was first seen';
	const d = h('dialog', { class: 'failures', 'aria-label': `${name}: failed joins` },
		h('div', { class: 'failhead' },
			h('h2', null, `${name}: failed joins`, h('span', { class: 'note' }, over)),
			h('button', { type: 'button', class: 'button small', onclick: () => d.close() }, 'Close')),
		box);
	d.addEventListener('close', () => d.remove());
	// A click on the backdrop, or a link away, closes it.
	d.addEventListener('click', (e) => {
		if (e.target === d || e.target.closest('a[href^="#/"]')) d.close();
	});
	document.body.append(d);
	d.showModal();
	const q = new URLSearchParams({ outcome: 'failed', limit: '200' });
	if (hours) q.set('since', new Date(Date.now() - hours * 3600e3).toISOString());
	if (under) q.set('under', under);
	try {
		const r = await get(`/v1/clients/${encodeURIComponent(mac)}/connections?${q}`);
		const list = r.connections || [];
		// How many stopped at each stage, for each reason, the most first.
		const by = new Map();
		for (const c of list) {
			const k = `${NAME[c.stage] || c.stage}|${c.reason || ''}`;
			by.set(k, (by.get(k) || 0) + 1);
		}
		const kinds = [...by.entries()].sort((a, b) => b[1] - a[1]).map(([k, n]) => {
			const [at, why] = k.split('|');
			return h('li', null, h('span', { class: 'chip bad' }, `${n} at ${at}`), why && ` ${why}`);
		});
		box.replaceChildren(...(list.length
			? [h('ul', { class: 'failkinds' }, kinds),
				attemptsTable(h('tbody', null, list.flatMap(attemptRows(ctx, null, FAILED_COLS))), FAILED_COLS, { how: 'Stopped at', why: 'Why' }),
				list.length >= 200 && h('p', { class: 'sub' }, 'The latest 200: its page has the rest.'),
				h('p', { class: 'sub' }, 'Each line opens to its steps. ', link(`/clients/${mac}/activity`, 'Every attempt, on its page'))]
			: [h('p', { class: 'sub' }, `No failed join of it over ${over} on the APs you may see.`)]).filter(Boolean));
	} catch (err) {
		box.replaceChildren(h('div', { class: 'error' }, err.message));
	}
}

// attemptsPanel is a client's attempts to come online (0118) as its page
// lists them (0222): a line each, the latest first, saying when, where its
// AP is in Locations, on which band and network, what it was, a join, a
// roam or a band switch, and how it went. A line opens to its steps. res is
// GET /v1/clients/{mac}/connections; at, an attempt's ID to open and mark,
// as one went badly in Roaming. boxed puts the lines in a box that
// scrolls, under heads that stay, and reads older ones as it is scrolled
// near its end (0337); page is how many res holds at most, and each read
// further back brings.
export function attemptsPanel(ctx, mac, res, at, boxed, page = ATTEMPTS) {
	const seen = new Set();
	const list = [...(res.connections || []), ...(older.get(mac) || [])].filter((c) => !seen.has(c.id) && seen.add(c.id));
	if (at) opened.add(Number(at));
	const rows = attemptRows(ctx, at);
	const body = h('tbody', null, list.map(rows));
	const foot = h('div', { class: 'below' });
	const count = h('span', { class: 'note' });
	const cl = res.client;
	const said = () => count.replaceChildren([
		`${list.length} attempt${list.length === 1 ? '' : 's'} to come online, the latest first`,
		cl && `${cl.attempts} since it was first seen ${ago(cl.first_seen)}, ${cl.failed} failed`,
		'each line opens to its steps'].filter(Boolean).join(' · '));
	said();
	// Older ones, a page at a time, from before the last shown: by the
	// button, or by scrolling the box near its end.
	let last = list[list.length - 1], loading = false;
	let done = !last || ((res.connections || []).length < page && !older.has(mac));
	const button = h('button', { type: 'button', class: 'button small', onclick: () => load() }, 'Older attempts');
	const load = async () => {
		if (loading || done) return false;
		loading = true;
		button.disabled = true;
		try {
			const r = await get(`/v1/clients/${encodeURIComponent(mac)}/connections?limit=${page}&before=${encodeURIComponent(last.started)}`);
			const got = (r.connections || []).filter((c) => !seen.has(c.id) && seen.add(c.id));
			older.set(mac, [...(older.get(mac) || []), ...got]);
			list.push(...got);
			body.append(...got.flatMap(rows));
			if (got.length) last = got[got.length - 1];
			done = (r.connections || []).length < page;
			said();
			foot.replaceChildren(done ? h('span', { class: 'sub' }, 'No older attempt is kept.') : button);
			return true;
		} catch (err) {
			foot.append(h('span', { class: 'error' }, ` ${err.message}`));
			return false;
		} finally {
			loading = false;
			button.disabled = false;
		}
	};
	if (!done) foot.append(button);
	const table = list.length ? attemptsTable(body, COLS, {}, boxed) : null;
	if (table && boxed) {
		// The box is a window about PAGE lines tall (Griff, 2026-11-07: "a
		// window to this with scroll bars on one side"). It reads further
		// back once it is scrolled to its end, and not before: nothing is
		// read that no one scrolled to. It stays where it was scrolled as the
		// page is drawn again.
		table.addEventListener('scroll', () => {
			scrolled.set(mac, table.scrollTop);
			if (table.scrollTop > 0 && table.scrollTop + table.clientHeight >= table.scrollHeight - 60) load();
		}, { passive: true });
		const y = scrolled.get(mac);
		if (y && !at) requestAnimationFrame(() => { table.scrollTop = y; });
	}
	// An attempt asked for that is older than these is read back to, a
	// page at a time, ten pages at most, and brought into view.
	if (at && !seen.has(Number(at)) && !done)
		(async () => {
			for (let i = 0; i < 10 && !seen.has(Number(at)) && !done; i++)
				if (!await load()) break;
			document.getElementById(`attempt-${at}`)?.scrollIntoView({ block: 'center' });
		})();
	return h('section', { class: 'panel' },
		h('h2', null, 'What it did', list.length > 0 && count),
		table || h('p', { class: 'sub pad' }, 'No attempt to come online is recorded for it.'),
		foot);
}
