// Which service folders a location takes (0320), in two places.
//
// - On a Locations folder's Networks tab, in its own row: as the folder
//   above has them, or set here, a tick a folder, each with the networks it
//   offers. What comes and what goes is said as it is picked.
// - On a Services folder's page: the folders where it applies, and Apply
//   at…, to add it to more or take it off.
//
// A location sets its list with assign-services, and takes the folder
// above's again with an unset. Two folders that offer the same network
// clash: an AP takes a network from one folder alone, so a list with a clash
// is held back. Each is a draft of its page's one save (0204).

import { h } from '../dom.js';
import { get } from '../api.js';
import { whence } from '../format.js';
import { UNSET } from './pagesave.js';

// RESTART is what applying a change to the networks an AP offers means.
const RESTART = 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.';

// depths reads a tree's nodes, as the tree lists them, root first and depth
// first, each with its depth below the root.
function depths(t) {
	const depth = new Map();
	return t.list.map((n) => {
		const d = n.parent && depth.has(n.parent) ? depth.get(n.parent) + 1 : 0;
		depth.set(n.id, d);
		return { ...n, depth: d };
	});
}

// offers reads what each Services folder offers, as its page resolves it,
// what it inherits included: by folder, its networks, each { id, ssid }.
export async function offers(ctx) {
	const folders = ctx.trees.services.list;
	const pages = await Promise.allSettled(folders.map((f) => get(`/v1/trees/services/nodes/${encodeURIComponent(f.id)}`)));
	const out = new Map();
	folders.forEach((f, i) => {
		const nets = [];
		if (pages[i].status === 'fulfilled')
			for (const [p, r] of Object.entries(pages[i].value.fields || {})) {
				const m = p.match(/^network\.([^.]+)\.ssid$/);
				if (m) nets.push({ id: m[1], ssid: r.value });
			}
		out.set(f.id, nets.sort((a, b) => String(a.ssid).localeCompare(String(b.ssid))));
	});
	return out;
}

// reach is the networks a list of service folders offers, by ID, each with
// its SSID and the folder it comes from; clash, where two of them offer one
// network, says which.
export function reach(list, have, name) {
	const nets = new Map();
	let clash = null;
	for (const f of list)
		for (const n of have.get(f) || []) {
			const was = nets.get(n.id);
			if (was && was.from !== f) clash ||= `${name(was.from)} and ${name(f)} both offer ${n.ssid}: an AP takes a network from one folder. Untick one.`;
			else nets.set(n.id, { ...n, from: f });
		}
	return { nets, clash };
}

// moves says what changes from one reach to another: the networks that come
// and those that go, each by SSID.
function moves(before, after) {
	const comes = [...after.values()].filter((n) => !before.has(n.id));
	const goes = [...before.values()].filter((n) => !after.has(n.id));
	return { comes, goes };
}

// movesLine shows what comes and goes as chips; nothing where nothing does.
function movesLine({ comes, goes }) {
	if (!comes.length && !goes.length) return h('span', { class: 'sub' }, 'The same networks reach here.');
	return h('span', { class: 'chiprow flush' },
		comes.map((n) => h('span', { class: 'chip ok', title: 'Comes here' }, `${n.ssid} comes`)),
		goes.map((n) => h('span', { class: 'chip warn', title: 'Goes from here' }, `${n.ssid} goes`)));
}

// listName says a list of service folders in words.
const listName = (list, name) => (list.length ? list.map(name).join(' + ') : 'none');

// servicesPanel is a Locations node's Service folders row, on its Networks
// tab: with edit and save, the person may change it there, in its row, as a
// draft of the tab's save. cards are the tab's network cards, by network
// ID, marked while a draft takes their network away.
export function servicesPanel(ctx, id, page, edit, save, cards) {
	const r = page.fields?.services;
	const now = r?.value || [];
	const name = (sid) => ctx.name('services', sid);
	const lname = (x) => ctx.name('locations', x);
	const parent = page.node?.parent || null;
	const box = h('div', { class: 'edit' });
	const cell = h('div', { class: 'value' });
	const shown = now.length ? listName(now, name) : 'none yet';
	const show = edit && save ? h('button', { type: 'button', class: 'inline', title: 'Change it here' }, shown) : null;
	cell.append(show || shown);
	const panel = h('section', { class: 'panel' },
		h('h2', null, 'Services offered here'),
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Service folders'),
			cell,
			r ? whence('locations', id, r, lname) : h('span', { class: 'whence' })),
		box);
	if (!show) return h('div', { class: 'fieldgrid' }, panel);

	let off = null;
	const close = () => {
		off?.();
		off = null;
		for (const el of cards?.values() || []) el.classList.remove('going');
		cell.replaceChildren(show);
		box.replaceChildren();
		box.removeAttribute('data-editing');
	};
	show.addEventListener('click', async () => {
		box.setAttribute('data-editing', '');
		box.replaceChildren(h('div', { class: 'inlinebar' }, h('span', { class: 'hint' }, 'Reading the service folders…')));
		const [have, above] = await Promise.all([
			offers(ctx),
			parent ? get(`/v1/trees/locations/nodes/${encodeURIComponent(parent)}`).then((p) => p.fields?.services?.value || [], () => []) : null,
		]);
		const own = r?.origin === 'self';
		const folders = depths(ctx.trees.services);
		const before = reach(now, have, name).nets;
		// up: as the folder above has them; else set here, from picked.
		let up = !!parent && !own;
		let touched = false;
		const picked = new Set(now);
		const list = () => folders.filter((f) => picked.has(f.id)).map((f) => f.id);
		const after = () => reach(up ? above : list(), have, name);
		const changed = () => {
			if (up) return own;
			if (!touched) return false;
			return !own || JSON.stringify(list()) !== JSON.stringify(now);
		};
		const part = {
			tree: 'locations', node: id,
			changes: () => (up && own ? { services: UNSET } : {}),
			ops: () => (!up && changed() ? [{ kind: 'assign-services', node: id, services: list() }] : []),
			dirty: () => changed(),
			blocked: () => !up && !!after().clash,
			open: () => false,
			undo: close,
			summary: () => {
				const m = moves(before, after().nets);
				return [
					h('div', null, h('strong', null, `Service folders on ${page.node?.name || lname(id)}`)),
					h('ul', { class: 'becomes' },
						h('li', null, `${listName(now, name)}${r ? (own ? ', set here' : `, as ${lname(r.from)}`) : ''} → `,
							up ? `as ${lname(parent)}: ${listName(above, name)}` : `${listName(list(), name)}, set here`),
						m.comes.length > 0 && h('li', null, `Comes: ${m.comes.map((n) => n.ssid).join(', ')}`),
						m.goes.length > 0 && h('li', null, `Goes: ${m.goes.map((n) => n.ssid).join(', ')}`)),
				];
			},
			notes: () => {
				const m = moves(before, after().nets);
				return m.comes.length || m.goes.length ? [h('div', { class: 'sub warn' }, RESTART)] : [];
			},
		};
		const body = h('div');
		const draw = () => {
			const a = after();
			const m = moves(before, a.nets);
			for (const [nid, el] of cards || []) el.classList.toggle('going', m.goes.some((n) => n.id === nid));
			const seg = parent && h('span', { class: 'segmented', role: 'group', 'aria-label': `Where ${page.node?.name || id}'s service folders come from` },
				h('button', { type: 'button', class: up ? 'on' : '', 'aria-pressed': String(up), onclick: () => pick(true) }, `As ${lname(parent)}`),
				h('button', { type: 'button', class: up ? '' : 'on', 'aria-pressed': String(!up), onclick: () => pick(false) }, 'Set here'));
			const ticks = !up && h('div', { class: 'svcpick' }, folders.map((f) => {
				const nets = have.get(f.id) || [];
				const box = h('input', { type: 'checkbox', checked: picked.has(f.id) });
				box.addEventListener('change', () => {
					if (box.checked) picked.add(f.id);
					else picked.delete(f.id);
					touched = true;
					edited();
				});
				return h('label', { style: { 'padding-left': `${8 + f.depth * 14}px` } }, box,
					h('span', { class: 'name' }, f.name),
					h('span', { class: 'nets' }, nets.length ? nets.map((n) => h('span', { class: 'mono ssid' }, n.ssid)) : h('span', { class: 'sub' }, 'offers no network')));
			}));
			// Said of a list set here alone; never false, for replaceChildren.
			const clash = up || !a.clash ? '' : h('div', { class: 'clash' }, h('span', { class: 'chip bad' }, 'clash'), ' ', a.clash);
			body.replaceChildren(
				seg || '',
				up ? h('div', { class: 'sub' }, `${listName(above, name)}, as ${lname(parent)} has it. A folder set there later reaches here too.`) : ticks,
				clash,
				movesLine(m));
		};
		const edited = () => {
			draw();
			save.changed(part);
		};
		const pick = (u) => {
			up = u;
			if (!u) touched = true;
			edited();
		};
		draw();
		cell.replaceChildren(h('span', { class: 'sub' }, 'changing below'));
		box.replaceChildren(h('div', { class: 'svcedit' }, body,
			h('div', { class: 'inlinebar' }, h('button', { type: 'button', class: 'button small', onclick: close }, 'Cancel'))));
		off = save.register(part);
	});
	return h('div', { class: 'fieldgrid' }, panel);
}

// appliesAtPanel is a Services folder's Applies at (0320): each Locations
// folder that takes it, set there, and how many below take it as they do;
// and Apply at…, to have more take it, or fewer, as one change with save,
// the page's save.
export function appliesAtPanel(ctx, id, page, save) {
	const places = page.placements || [];
	const lname = (x) => ctx.name('locations', x);
	const sname = (x) => ctx.name('services', x);
	const where = new Map(depths(ctx.trees.locations).map((n) => [n.id, n]));
	// The APs that take it from node: those whose folder takes it, as node
	// says. One under a folder with a list of its own takes that list.
	const byNode = new Map(places.map((p) => [p.node, p]));
	const aps = (node) => ctx.trees.locations.list.filter((n) => {
		const p = n.kind === 'ap' ? byNode.get(n.parent) : null;
		return p && p.from === node && p.services.includes(id);
	}).map((n) => n.name);
	const takes = places.filter((p) => p.services.includes(id));
	const own = takes.filter((p) => p.from === p.node);
	const below = takes.length - own.length;
	const box = h('div', { class: 'edit' });
	const apply = h('button', { type: 'button', class: 'button small' }, 'Apply at…');
	const rows = own.length
		? own.map((p) => {
			const list = aps(p.node);
			return h('div', { class: 'row' },
				h('div', { class: 'label' }, lname(p.node)),
				h('div', { class: 'value' }, list.length ? (list.length <= 3 ? list.join(', ') : `${list.length} APs`) : 'no AP takes it from here'),
				h('span', { class: 'whence from' }, `set at ${lname(p.node)}`));
		})
		: [h('div', { class: 'row' }, h('div', { class: 'label' }, 'Nowhere yet'), h('div', { class: 'value sub' }, 'no location takes it'), h('span', { class: 'whence' }))];
	const panel = h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, 'Applies at', h('span', { class: 'controls' }, apply)),
		rows,
		below > 0 && h('div', { class: 'sub below-rows' }, `And ${below} folder${below === 1 ? '' : 's'} below, as ${below === 1 ? 'it has' : 'they have'} it.`),
		box);

	let off = null;
	const close = () => {
		off?.();
		off = null;
		box.replaceChildren();
		box.removeAttribute('data-editing');
	};
	apply.addEventListener('click', () => {
		if (off) return;
		const ticked = new Map(places.map((p) => [p.node, p.services.includes(id)]));
		const ops = () => places.filter((p) => ticked.get(p.node) !== p.services.includes(id)).map((p) => ({
			kind: 'assign-services', node: p.node,
			services: ticked.get(p.node) ? [...p.services, id] : p.services.filter((x) => x !== id),
		}));
		const part = {
			tree: 'locations',
			changes: () => ({}),
			ops,
			dirty: () => ops().length > 0,
			blocked: () => false,
			open: () => false,
			undo: close,
			summary: () => [
				h('div', null, h('strong', null, `Where Services › ${sname(id)} applies`)),
				h('ul', { class: 'becomes' }, ops().map((o) => {
					const p = places.find((x) => x.node === o.node);
					const was = `${p.services.map(sname).join(' + ') || 'none'}${p.from ? (p.from === p.node ? ', set here' : `, as ${lname(p.from)}`) : ''}`;
					return h('li', null, `${lname(o.node)}: ${was} → ${o.services.map(sname).join(' + ') || 'none'}, set here`);
				})),
			],
			notes: () => [h('div', { class: 'sub warn' }, RESTART)],
		};
		box.setAttribute('data-editing', '');
		box.replaceChildren(h('div', { class: 'svcedit' },
			h('div', { class: 'svcpick' }, places.map((p) => {
				const n = where.get(p.node);
				const tick = h('input', { type: 'checkbox', checked: ticked.get(p.node) });
				tick.addEventListener('change', () => {
					ticked.set(p.node, tick.checked);
					save.changed(part);
				});
				const has = `has ${p.services.map(sname).join(' + ') || 'nothing'}${p.from ? (p.from === p.node ? ', set here' : `, as ${lname(p.from)}`) : ''}`;
				return h('label', { style: { 'padding-left': `${8 + (n?.depth || 0) * 14}px` } }, tick, h('span', { class: 'name' }, lname(p.node)), h('span', { class: 'sub' }, has));
			})),
			h('div', { class: 'inlinebar' },
				h('span', { class: 'hint' }, 'Each place keeps the folders it has and takes this one too. Landing Zone takes no networks.'),
				h('button', { type: 'button', class: 'button small', onclick: close }, 'Cancel'))));
		off = save.register(part);
	});
	return h('div', { class: 'fieldgrid' }, panel);
}
