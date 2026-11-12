// A folder in Locations or Services: its values and where they come from, its
// overrides, and what it holds (0012, 0013). A Locations folder's are in
// nine tabs (0284): Overview, APs, Clients, Radios, Wired, Networks,
// Problems, Assurance and Settings; a tab with parts has them in one row
// under it, and the band picker for every radio number on the page sits at
// the right of the tab row. Landing Zone lists the APs waiting in it (0032).

import { h, link, icon } from '../dom.js';
import { get } from '../api.js';
import { group, value, ago } from '../format.js';
import { treeAside, crumbs, fleetMap, tabBar, pick, keepPath, moved, bandRow, presentBands, pageBand } from '../layout.js';
import { redrawNow } from '../refresh.js';
import { fieldPanels, editing } from './fields.js';
import { apPage } from './ap.js';
import { systemSection, configs, channelsSection } from './sections.js';
import { renameButton } from './rename.js';
import { moveButton } from './move.js';
import { newFolderButton } from './newfolder.js';
import { removable, removeFolderButton } from './removefolder.js';
import { networksTab } from './networks.js';
import { keysSection } from './keys.js';
import { radiosTab, radiosBar, radiosPart, radiosLive, wiredTab } from './interfaces.js';
import { clientsTab } from './clients.js';
import { templatesTab } from './templates.js';
import { problemsTab } from './threats.js';
import { overviewTab } from './overview.js';
import { assuranceTab } from './assurance.js';
import { bluetoothFolderTab } from './bluetooth.js';
import { testNetworkPanel } from './testnet.js';
import { pageSave } from './pagesave.js';
import { roamingPanel } from './roaming.js';
import { detected } from './aps.js';
import { leavingPanel } from './leaving.js';
import { stickyPanel } from './sticky.js';
import { appliesAtPanel } from './servicefolders.js';

const TABS = [['overview', 'Overview'], ['aps', 'APs'], ['clients', 'Clients'], ['radios', 'Radios'], ['wired', 'Wired'], ['networks', 'Networks'], ['problems', 'Problems'], ['assurance', 'Assurance'], ['settings', 'Settings']];
// The parts of a folder's Clients.
// A folder's Clients (0337): the Wi-Fi clients; how they roamed and why
// they left; those that held on weakly (0340); what the Bluetooth radios
// hear.
const CLIENTS = [['wifi', 'Wi-Fi'], ['roaming', 'Wi-Fi roaming'], ['sticky', 'Sticky'], ['bluetooth', 'Bluetooth']];

export async function treePage(ctx, tree, id, tab, sub, view) {
	const t = ctx.trees[tree];
	id = id || t.root;
	if (!id) return { main: [h('div', { class: 'banner info' }, 'You have no role in this tree.')] };
	// Links to where a value was set can point at an AP; it has its own page.
	if (tree === 'locations' && t.nodes.get(id)?.kind === 'ap') return apPage(ctx, id, tab, sub, view);
	const [page, fleet] = await Promise.all([
		get(`/v1/trees/${tree}/nodes/${encodeURIComponent(id)}`),
		tree === 'locations' ? get('/v1/aps') : null,
	]);
	const status = fleet ? fleetMap(fleet.aps) : null;
	const n = page.node;
	const branch = page.ancestry.map((a) => t.nodes.get(a)).find((x) => x?.broken);
	const kind = n.kind === 'org'
		? `Org · root of ${tree === 'locations' ? 'Locations' : 'Services'}`
		: n.isolated ? 'Outside the hierarchy · APs wait here, with no config, until one is adopted into a folder' : 'Folder';

	// A folder can be renamed, and moved into another with everything in it,
	// by someone who may change it (0076). The Org and isolated folders, such
	// as Landing Zone, stay where they are.
	const renameBox = h('div', { class: 'edit' });
	const movable = n.kind !== 'org' && !n.isolated;
	const main = [
		crumbs(ctx, tree, page.ancestry),
		h('div', { class: 'head' },
			h('div', null,
				h('h1', null, n.name, n.broken && h('span', { class: 'chip break' }, 'Break Hierarchy'),
					editing(ctx, tree, page) && renameButton(ctx, tree, { id, name: n.name, kind: n.kind }, renameBox, 'rename head'),
					editing(ctx, tree, page) && movable && moveButton(ctx, tree, { id, name: n.name, kind: n.kind }, renameBox, 'rename head'),
					editing(ctx, tree, page) && !n.isolated && newFolderButton(ctx, tree, { id, name: n.name }, renameBox),
					// A folder that holds nothing can be removed (0125).
					editing(ctx, tree, page) && removable(ctx, tree, { id, kind: n.kind, isolated: n.isolated })
						&& removeFolderButton(ctx, tree, { id, name: n.name }, renameBox, 'rename head')),
				h('div', { class: 'sub' }, kind, ' · your role here: ', page.role)),
			// Nothing is set in Landing Zone, so it overrides nothing.
			!n.isolated && overrides(ctx, tree, page.overrides)),
		renameBox,
		branch && h('div', { class: 'banner branch' },
			h('strong', null, 'Break Hierarchy'),
			h('span', null, `${branch.name} starts its own branch. Locks from above stop there, and the branch owns its configuration and the inheritance below it. Values marked Branch baseline were copied when the break was made.`)),
		page.problems?.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, `What is set here breaks ${page.problems.length} rule${page.problems.length === 1 ? '' : 's'}`),
			h('ul', null, page.problems.map((p) => h('li', null, p)))),
	];
	let refresh = 0;
	if (n.isolated) {
		main.push(await landingZone(ctx, t, id, fleet));
		refresh = 30;
	} else if (tree === 'locations') {
		const base = `/locations/${encodeURIComponent(id)}`;
		[tab, sub, view] = moved(base, tab, sub, view, true);
		tab = pick(TABS, tab);
		if (tab === 'clients') sub = pick(CLIENTS, sub);
		// The band picker, on the tabs that show radio numbers: Clients may
		// show every band (Griff, 2026-10-22), and the band cards are one a
		// band already.
		const bands = tab === 'overview' || tab === 'aps' || (tab === 'clients' && (sub === 'wifi' || sub === 'sticky')) || (tab === 'radios' && radiosPart(sub) !== 'bands') || tab === 'assurance'
			? bandRow(presentBands(page), tab === 'clients' && sub === 'wifi', redrawNow) : null;
		// A tab with parts has them under it, the picker at their right.
		const parts = tab === 'radios' ? radiosBar(base, sub, bands)
			: tab === 'clients' ? tabBar(`${base}/clients`, CLIENTS, sub, true, bands) : null;
		main.push(tabBar(base, TABS, tab, false, parts ? null : bands), parts);
		// A test network switched on here, or one to switch on (0179), first.
		if (tab === 'overview') main.push(testNetworkPanel(ctx, id, page), ...await overviewTab(ctx, id, page, fleet));
		else if (tab === 'radios') main.push(...await radiosTab(ctx, base, id, page, sub, null, editing(ctx, tree, page)));
		else if (tab === 'wired') main.push(...await wiredTab(ctx, base, id, page, sub, null, editing(ctx, tree, page)));
		else if (tab === 'aps') {
			// Each radio's noise floor, under its band's channel; at the Org,
			// the devices that may be APs not yet enrolled (0034, 0284).
			const [rows, noise, found] = await Promise.all([configs([...(page.hardware?.aps || []), ...(page.hardware?.unknown || [])]),
				get(`/v1/noise?under=${encodeURIComponent(id)}`).catch(() => null),
				n.kind === 'org' ? get('/v1/detected').catch(() => null) : null]);
			main.push(channelsSection(ctx, rows, status, !!editing(ctx, tree, page, 'aps'), noise, fleet, id), detected(ctx, found));
		}
		else if (tab === 'networks') main.push(await networksTab(ctx, id, page));
		// What the APs' Bluetooth radios hear (0157); how its clients roam
		// (0136) and why they left (0255), a part of their own (0337).
		else if (tab === 'clients') main.push(...(sub === 'bluetooth' ? [await bluetoothFolderTab(ctx, page, id)]
			: sub === 'roaming' ? await Promise.all([get(`/v1/roaming?under=${encodeURIComponent(id)}`).then(roamingPanel, () => roamingPanel(null)), leavingPanel({ under: id })])
				: sub === 'sticky' ? [await stickyPanel(id, pageBand())]
					: [await clientsTab(ctx, page, null)]));
		else if (tab === 'problems') main.push(...await problemsTab(ctx, base, id, page, sub));
		else if (tab === 'assurance') main.push(...await assuranceTab(ctx, id, page, editing(ctx, tree, page)));
		else {
			// System and the templates picked here, one page with one save
			// (0284).
			const edit = editing(ctx, tree, page);
			const shared = edit ? pageSave(ctx, 'locations', id) : null;
			main.push(await systemSection(ctx, id, page, edit, undefined, shared), await templatesTab(ctx, id, page, edit, shared), shared?.bar);
		}
		if ((tab === 'radios' && radiosLive(sub)) || tab === 'wired' || tab === 'clients' || tab === 'aps' || tab === 'problems' || tab === 'assurance' || tab === 'overview') refresh = 30; // live
	} else {
		main.push(fieldPanels(ctx, tree, id, page.fields, editing(ctx, tree, page)));
		// Where its networks go, and Apply at… for more or fewer (0320):
		// what it changes is the locations', saved as one.
		const applies = pageSave(ctx, 'services', id);
		main.push(appliesAtPanel(ctx, id, page, applies), applies.bar);
		// A Services folder's networks' per-user keys (0070).
		main.push(await keysSection(ctx, id, page));
	}
	// A Locations folder's APs, with their state, are on its APs tab, and its
	// folders in the tree beside it.
	if (tree !== 'locations') main.push(inside(ctx, tree, t, id, status));
	// Moving to another folder or AP in the tree keeps the tab, so folders
	// can be compared side by side.
	const keep = tree === 'locations' && !n.isolated ? keepPath(tab, sub) : '';
	return { aside: treeAside(ctx, tree, id, status, keep), main, refresh };
}

// overrides is the mockup's Overrides menu (0012): what differs from above,
// here and below, each a link to where it was set.
function overrides(ctx, tree, ov) {
	const inEffect = ov?.in_effect || [];
	const below = ov?.below || [];
	const item = (o) => h('a', { href: `#/${tree}/${encodeURIComponent(o.node)}` },
		labelOf(o.path),
		h('small', null, h('span', { class: 'mono' }, value(o.path, o.value, (id) => ctx.name('services', id))), ' · set at ', ctx.name(tree, o.node)));
	const drop = h('div', { class: 'drop', hidden: true },
		inEffect.length > 0 && [h('h3', null, 'In effect here'), inEffect.map(item)],
		below.length > 0 && [h('h3', null, 'Made below this level'), below.map(item)],
		!inEffect.length && !below.length && h('div', { class: 'empty' }, 'No overrides: everything here comes straight from above.'));
	const button = h('button', { type: 'button', 'aria-expanded': 'false', onclick: () => {
		drop.hidden = !drop.hidden;
		button.setAttribute('aria-expanded', String(!drop.hidden));
	} }, 'Overrides', h('span', { class: 'count' }, inEffect.length + below.length), icon('chevron'));
	return h('div', { class: 'menu' }, button, drop);
}

function labelOf(path) {
	const g = group(path);
	return g.network ? `${g.network} · ${g.label}` : `${g.title} · ${g.label}`;
}

// landingZone lists the APs waiting to be adopted, with what they said about
// themselves when they enrolled (0033), so a person can tell a real AP from
// an impostor; and before them, those adopted already that enrolled again,
// their tokens gone, which wait on their own pages to be let back in (0223).
async function landingZone(ctx, t, id, fleet) {
	const back = t.list.filter((n) => n.kind === 'ap' && n.again);
	const again = back.length > 0 && h('section', { class: 'panel' },
		h('h2', null, 'Asking to come back', h('span', { class: 'note' }, 'adopted APs that enrolled again, their tokens gone as after a reflash: each waits on its own page to be let back in')),
		h('table', { class: 'list' }, back.map((n) => h('tr', null,
			h('td', null, link(`/aps/${encodeURIComponent(n.id)}`, n.name)),
			h('td', { class: 'sub' }, n.parent ? ctx.name('locations', n.parent) : '')))));
	return [again, await waitingToBeAdopted(ctx, t, id, fleet)];
}

// waitingToBeAdopted is Landing Zone's own APs, as landingZone lists them.
async function waitingToBeAdopted(ctx, t, id, fleet) {
	const waiting = t.list.filter((n) => n.parent === id && n.kind === 'ap');
	if (!waiting.length) return h('div', { class: 'banner info' }, 'No APs are waiting. An AP running the agent lands here when it enrolls; one that replaces an adopted AP is put in its place from that AP’s page, Replace…');
	const pages = await Promise.all(waiting.map((n) => get(`/v1/trees/locations/nodes/${encodeURIComponent(n.id)}`)));
	const seen = new Map((fleet?.aps || []).map((a) => [a.id, a.seen]));
	return h('section', { class: 'panel' },
		h('h2', null, 'Waiting to be adopted', h('span', { class: 'note' }, 'open one to adopt it into a folder')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'MAC', 'Model', 'OpenWrt', 'Enrolled from', 'Last seen'].map((c) => h('th', null, c))),
			pages.map((p) => {
				const f = p.facts || {};
				return h('tr', null,
					h('td', null, link(`/aps/${encodeURIComponent(p.node.id)}`, p.node.name)),
					h('td', { class: 'mono' }, f.mac || '—'),
					h('td', null, f.model || '—'),
					h('td', null, f.openwrt || '—'),
					h('td', { class: 'mono' }, f.source || '—'),
					h('td', null, ago(seen.get(p.node.id)?.at)));
			})));
}

// inside lists what a folder holds.
function inside(ctx, tree, t, id, status) {
	const kids = t.list.filter((n) => n.parent === id && !(t.nodes.get(id)?.isolated && n.kind === 'ap'));
	if (!kids.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Inside ' + ctx.name(tree, id)),
		h('table', { class: 'list' }, kids.map((n) => {
			const ap = n.kind === 'ap';
			const st = ap && status ? status.get(n.id) : null;
			return h('tr', null,
				h('td', null, link(ap ? `/aps/${encodeURIComponent(n.id)}` : `/${tree}/${encodeURIComponent(n.id)}`, n.name)),
				h('td', null, ap ? 'AP' : n.isolated ? 'Isolated folder' : 'Folder', n.broken && ' · breaks hierarchy'),
				h('td', null, st && h('span', { class: 'chip ' + st.chip }, st.label)));
		})));
}
