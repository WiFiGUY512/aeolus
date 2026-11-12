// Pieces several pages share: the tree on the left, the path to a node, and
// what state an AP is in.

import { h, link, icon } from './dom.js';
import { get } from './api.js';
import { ago, quiet } from './format.js';

// ordered is a tree in the order the left side shows it: depth first, and
// under each folder its APs, then its folders, each as the manager lists
// them. An isolated folder, such as Landing Zone, is outside the hierarchy
// (0032, 0333): it and its APs come apart, before the tree.
function ordered(t) {
	const kids = new Map();
	for (const n of t.list)
		if (n.parent && t.nodes.has(n.parent)) {
			if (!kids.has(n.parent)) kids.set(n.parent, []);
			kids.get(n.parent).push(n);
		}
	const walk = (out) => (n) => {
		out.push(n);
		const k = kids.get(n.id) || [];
		k.filter((c) => c.kind === 'ap').forEach(walk(out));
		k.filter((c) => c.kind !== 'ap' && !c.isolated).forEach(walk(out));
	};
	const apart = [], tree = [];
	t.list.filter((n) => n.isolated).forEach(walk(apart));
	t.list.filter((n) => !n.parent || !t.nodes.has(n.parent)).forEach(walk(tree));
	return { apart, tree };
}

// isolatedFrom is the path to a node from where it is: from the isolated
// folder it is in, such as Landing Zone, which is outside the hierarchy, or
// else from the Org.
function isolatedFrom(t, ancestry) {
	const at = ancestry.findIndex((id) => t?.nodes.get(id)?.isolated);
	return at > 0 ? ancestry.slice(at) : ancestry;
}

// treeAside draws a tree on the left, as in the mockup: the folders, and
// under each its APs, then its folders, each AP with a dot for how it is
// doing (status maps an AP to apStatus), so a change can be watched
// landing. keep is the tab to open on the folder or AP clicked (0047); it
// is not kept in an isolated folder, whose page has no tabs. Landing Zone
// is drawn above the tree, apart from it: its APs wait there, with no
// config, until one is adopted into a folder (0333).
export function treeAside(ctx, tree, selected, status, keep = '') {
	const t = ctx.trees[tree];
	const aside = h('aside', { class: 'tree' });
	const draw = () => {
		const { apart, tree: inTree } = ordered(t);
		aside.replaceChildren(...[
			apart.length > 0 && h('div', { class: 'tree-apart' }, ...rows(ctx, t, tree, apart, selected, status, keep, draw)),
			h('div', { class: 'tree-title' }, tree === 'locations' ? 'Locations' : 'Services'),
			...rows(ctx, t, tree, inTree, selected, status, keep, draw),
		].filter(Boolean));
	};
	draw();
	return aside;
}

// The folders whose APs this viewer has hidden in the tree, kept in the
// browser: a convenience of theirs, not a setting. Where the browser keeps
// nothing, they stay hidden until the page is loaded again.
const HIDDEN = 'aeolus.tree.hiddenAPs';
let hidden = null;

function hiddenFolders() {
	if (!hidden)
		try {
			hidden = new Set(JSON.parse(localStorage.getItem(HIDDEN) || '[]'));
		} catch {
			hidden = new Set();
		}
	return hidden;
}

function keepHidden() {
	try {
		localStorage.setItem(HIDDEN, JSON.stringify([...hiddenFolders()]));
	} catch { /* storage blocked */ }
}

// The worst of some APs' states, for a folder whose APs are hidden.
const WORST = ['bad', 'warn', 'idle', 'ok'];

// apToggle is a folder's switch for its APs in the tree (Griff, 2026-10-06):
// their count, and while they are hidden, one dot for the worst of them.
function apToggle(folder, aps, status, draw) {
	const shown = !hiddenFolders().has(folder.id);
	const sts = aps.map((a) => status?.get(a.id)).filter(Boolean);
	const worst = WORST.find((c) => sts.some((s) => s.cls === c));
	const counts = [...new Set(sts.map((s) => s.label))].map((l) => `${sts.filter((s) => s.label === l).length} ${l.toLowerCase()}`).join(', ');
	return h('button', {
		type: 'button', class: 'aptoggle', 'aria-pressed': shown ? 'true' : 'false',
		title: `${shown ? 'Hide' : 'Show'} the ${aps.length === 1 ? 'AP' : `${aps.length} APs`} in ${folder.name}${counts ? ` (${counts})` : ''}`,
		onclick: () => {
			if (shown) hiddenFolders().add(folder.id); else hiddenFolders().delete(folder.id);
			keepHidden();
			draw();
		},
	}, icon('ap'), String(aps.length), !shown && worst && h('span', { class: 'dot ' + worst }));
}

// rows are the tree's rows, of the nodes in list in order: each folder,
// with a switch for its APs where it has any, and each AP not hidden with
// its folder's, the selected one always.
function rows(ctx, t, tree, list, selected, status, keep, draw) {
	const off = hiddenFolders();
	const apsOf = new Map();
	for (const n of t.list)
		if (n.kind === 'ap' && n.parent) {
			if (!apsOf.has(n.parent)) apsOf.set(n.parent, []);
			apsOf.get(n.parent).push(n);
		}
	// From the Org, or from the isolated folder a node is in.
	const depth = (id) => {
		let d = 0;
		for (let n = t.nodes.get(id); n && !n.isolated && n.parent && t.nodes.has(n.parent); n = t.nodes.get(n.parent)) d++;
		return d;
	};
	const inBranch = (id) => {
		for (let n = t.nodes.get(id); n; n = t.nodes.get(n.parent)) if (n.broken) return true;
		return false;
	};
	return list.filter((n) => n.kind !== 'ap' || !off.has(n.parent) || n.id === selected).map((n) => {
		const ap = n.kind === 'ap';
		const href = (ap ? `/aps/${encodeURIComponent(n.id)}` : `/${tree}/${encodeURIComponent(n.id)}`) + (n.isolated ? '' : keep);
		const cls = [n.id === selected ? 'on' : '', inBranch(n.id) ? 'branch' : ''].join(' ').trim();
		const st = ap && status ? status.get(n.id) : null;
		const row = h('a', { href: '#' + href, class: cls || null, style: { '--depth': depth(n.id) }, title: n.isolated ? 'Outside the hierarchy: APs wait here, with no config, until one is adopted into a folder' : null },
			icon(ap ? 'ap' : 'folder'),
			h('span', { class: 'name' }, n.name),
			n.broken && h('span', { class: 'chip break' }, 'BREAK'),
			st && h('span', { class: 'dot ' + st.cls, title: st.label, 'data-ap': n.id, 'data-cls': st.cls, 'data-label': st.label }));
		const aps = !ap && apsOf.get(n.id);
		return aps ? h('div', { class: 'treerow' }, row, apToggle(n, aps, status, draw)) : row;
	});
}

// crumbs draws the path above a node: from the Org, or from Landing Zone
// for an AP waiting there, as it is outside the hierarchy (0333).
export function crumbs(ctx, tree, ancestry) {
	return h('nav', { class: 'crumbs', 'aria-label': 'Path' },
		isolatedFrom(ctx.trees[tree], ancestry).slice(0, -1).map((id, i) => [i > 0 && '›', link(`/${tree}/${encodeURIComponent(id)}`, ctx.name(tree, id))]));
}

// apStatus sums up an AP from the fleet view (GET /v1/aps): a dot class, a
// short label, and a sentence. A quiet AP is one not heard from in three
// poll intervals of the default length (format.js).
export function apStatus(a) {
	const seen = a.seen?.at;
	const isQuiet = seen && quiet(seen);
	if (a.config === 'unassigned') return { cls: 'idle', chip: 'idle', label: 'In Landing Zone', detail: 'Waiting for a person to adopt it.' };
	if (!seen) return { cls: 'idle', chip: 'idle', label: 'Never seen', detail: 'It has not called the manager.' };
	// Its line to the manager gone, and no other opened (0165).
	if (a.down) return { cls: 'bad', chip: 'bad', label: 'Down', detail: `Its line to the manager went ${ago(a.down)}.` };
	if (isQuiet) return { cls: 'bad', chip: 'bad', label: 'Not heard from', detail: 'Last seen ' + ago(seen) + '.' };
	if (a.config === 'held') return { cls: 'bad', chip: 'bad', label: 'Held', detail: `Its config breaks ${a.problems} rule${a.problems === 1 ? '' : 's'}; it keeps running what it has.` };
	// netifd lost its network.wireless object (2026-10-06): the radios run,
	// but Aeolus there can't see them.
	if (a.wireless_missing) return { cls: 'warn', chip: 'warn', label: 'Radios unseen',
		detail: `netifd lost its network.wireless object, so Aeolus on this AP can't see its radios, clients or neighbours, and checks the Wi-Fi after a change only by counting its networks. Restarting the network on the AP brings it back; its Wi-Fi drops for about 30 seconds.` };
	if (a.in_sync === true) {
		// Its agent (0079), once it says which it runs: amber while that
		// isn't the one it should run.
		const g = a.agent, runs = g?.runs?.hash;
		if (runs !== undefined && g?.wants && runs !== g.wants.hash) {
			const u = g.runs.update;
			const short = (v) => (v || '').replace(/-[0-9a-f]{7,}$/, '') || 'unknown';
			if (u && u.hash === g.wants.hash && (u.state === 'rolled-back' || u.state === 'failed'))
				return { cls: 'warn', chip: 'warn', label: u.state === 'failed' ? 'Agent update failed' : 'Agent rolled back', detail: u.why || '' };
			return { cls: 'warn', chip: 'warn', label: 'Agent updating', detail: `Its agent is ${short(g.runs.version)}; it should run ${short(g.wants.version)}.` };
		}
		return { cls: 'ok', chip: 'ok', label: 'In sync', detail: 'Running version ' + a.version + '.' };
	}
	if (a.in_sync === false) return { cls: 'warn', chip: 'warn', label: 'Out of sync', detail: `Running version ${a.seen.running}; its version is ${a.version}.` };
	return { cls: 'warn', chip: 'warn', label: 'No config yet', detail: 'It has not reported running a version.' };
}

// fleet reads every AP's state, by ID.
export function fleetMap(aps) {
	return new Map(aps.map((a) => [a.id, apStatus(a)]));
}

// How often the tree's dots are told which APs are down (0165), in ms: a
// page draws again every half minute, which is too slow for an AP that
// goes (Griff, 2026-10-15: "make it close to real time").
const DOWN_EVERY = 3000;

// watchDown asks the manager every few seconds which APs are down, while
// the page is in front and shows the tree, and turns the dot of each
// such AP in the tree red; one that is back has the dot the page drew it
// with.
export function watchDown(root) {
	const look = async () => {
		const dots = root.querySelectorAll('.dot[data-ap]');
		if (!dots.length || document.visibilityState !== 'visible') return;
		let down = {};
		try {
			down = (await get('/v1/aps/alive')).down || {};
		} catch { return; /* an older manager, or none: the page's own dots stand */ }
		for (const d of root.querySelectorAll('.dot[data-ap]')) {
			const at = down[d.dataset.ap];
			d.className = 'dot ' + (at ? 'bad' : d.dataset.cls);
			d.title = at ? `Down: its line to the manager went ${ago(at)}` : d.dataset.label;
		}
	};
	setInterval(look, DOWN_EVERY);
}

// tabBar draws a page's tabs as links under base ([[key, label]]); current
// is the one shown. sub draws the smaller second row (0047). end, where
// given, sits at the row's right: the page's band picker (0284).
export function tabBar(base, tabs, current, sub, end) {
	const nav = h('nav', { class: sub ? 'subtabs' : 'pagetabs', 'aria-label': sub ? 'Part of this section' : 'Sections of this page' },
		tabs.map(([key, label]) => h('a', { href: `#${base}/${key}`, class: key === current ? 'on' : null, 'aria-current': key === current ? 'page' : null }, label)));
	return end ? h('div', { class: sub ? 'tabrow sub' : 'tabrow' }, nav, end) : nav;
}

// SUBTABS are the page tabs with parts of their own; moving to another node
// keeps the choice of those too.
export const SUBTABS = new Set(['radios', 'wired', 'clients', 'problems', 'activity']);

// keepPath is the part of a node's address that moving to another node
// keeps: its tab, and the tab's part.
export function keepPath(tab, sub) {
	if (!SUBTABS.has(tab) || !sub) return `/${tab}`;
	return `/${tab}/${sub}`;
}

// The addresses of tabs that moved, old to new (0072, 0284): Hardware and
// Interfaces became Radios and Wired, Alerts and Threats Problems, System
// and Templates Settings, and a folder's Bluetooth is a part of Clients.
// Each is [old tab, its part or '' for any, new tab, new part, keep the
// old view as the part]; 'folder' marks those of a folder's page only.
const MOVED = [
	['hardware', '', 'radios', 'bands'],
	['interfaces', 'radios', 'radios', '', true],
	['interfaces', 'ethernet', 'wired', 'ethernet'],
	['interfaces', 'tunnels', 'wired', 'tunnels'],
	['interfaces', 'paths', 'wired', 'paths'],
	['interfaces', '', 'radios', ''],
	['alerts', '', 'problems', 'alerts'],
	['threats', '', 'problems', 'sub'],
	['system', '', 'settings', ''],
	['templates', '', 'settings', '', false, 'folder'],
	['bluetooth', '', 'clients', 'bluetooth', false, 'folder'],
];

// moved reads an address from before 0284 as its tab's new one, and puts
// the new address in place of the old, so links and bookmarks keep working.
// folder is whether the page is a folder's.
export function moved(base, tab, sub, view, folder) {
	const m = MOVED.find(([t, s, , , , only]) => t === tab && (!s || s === sub) && (!only || folder));
	if (!m) return [tab, sub, view];
	const [, , to, part, keepView] = m;
	const now = [to, keepView ? view : part === 'sub' ? sub : part || undefined, undefined];
	try { history.replaceState(null, '', `#${base}${keepPath(now[0], now[1])}`); } catch { /* the address stays as it was */ }
	return now;
}

// The band a table shows (Griff, 2026-10-22: "at almost no time is it
// appropriate to look at 2.4 5 and 6 numbers simultanously outside of total
// client connections"): one choice, kept across pages and visits.
const BAND_CHOICES = [['2g', '2.4 GHz'], ['5g', '5 GHz'], ['6g', '6 GHz']];
const BAND_KEY = 'aeolus.band';
let bandChosen = null;
try { bandChosen = localStorage.getItem(BAND_KEY); } catch { /* storage blocked */ }

// bandOf is the band to show of those present: the one chosen where it is
// present, else 5 GHz, else the first. Where all is set, a table may show
// every band at once, as a list of clients does, and 'all' is a choice.
export function bandOf(present, all) {
	if (all && bandChosen === 'all') return 'all';
	const have = BAND_CHOICES.map(([b]) => b).filter((b) => present.includes(b));
	if (have.includes(bandChosen)) return bandChosen;
	return have.includes('5g') ? '5g' : have[0] ?? null;
}

// bandPicker chooses the band among those present, as buttons in a row,
// and draws the page again: null where there is nothing to choose.
export function bandPicker(present, all, redraw) {
	const now = bandOf(present, all);
	const choices = [...BAND_CHOICES.filter(([b]) => present.includes(b)), ...(all ? [['all', 'All bands']] : [])];
	if (choices.length < 2) return null;
	return h('span', { class: 'segmented', role: 'group', 'aria-label': 'Band' }, choices.map(([b, name]) => h('button', {
		type: 'button', class: b === now ? 'on' : null, 'aria-pressed': String(b === now),
		onclick: () => {
			bandChosen = b;
			try { localStorage.setItem(BAND_KEY, b); } catch { /* storage blocked: kept until the page reloads */ }
			redraw();
		},
	}, name)));
}

// The band the page's picker shows (0284): one picker a page, on its tab
// row, for every radio number on it. Set as the row is drawn; null on a page
// without one.
let shownBand = null;

// bandRow is the page's band picker among the bands present, all as for
// bandPicker, and makes its band the one pageBand says.
export function bandRow(present, all, redraw) {
	shownBand = bandOf(present, all);
	return bandPicker(present, all, redraw);
}

// pageBand is the band the page's picker shows; noBand, as a page starts to
// be drawn, says it has none yet.
export function pageBand() {
	return shownBand;
}

export function noBand() {
	shownBand = null;
}

// presentBands is the bands a node's APs have radios for, as its page says:
// those its band picker offers.
export const presentBands = (page) => (page.hardware?.bands || []).map((b) => b.band);

// pick returns the tab asked for if there is one, or the first.
export function pick(tabs, key) {
	return tabs.some(([k]) => k === key) ? key : tabs[0][0];
}
