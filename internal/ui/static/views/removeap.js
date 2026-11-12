// Deleting an AP from its page's heading, beside Rename, Move and Reboot
// (Griff, 2026-10-20: "we need a delete"): the change remove-ap (0038). What
// was set on it goes with it, its token stops working, and the manager
// forgets what it said of itself; the log keeps it all. An AP still on and
// reaching the manager enrolls again, into Landing Zone, as 0038 has it.

import { h } from '../dom.js';
import { group, value } from '../format.js';
import { ask, confirm } from './confirm.js';

// removeAPButton offers to delete ap ({ id, name }), its preview opening in
// box. Once deleted, the page is its folder's.
export function removeAPButton(ctx, ap, box, cls = 'rename head') {
	return h('button', {
		type: 'button', class: `button small ${cls}`, title: 'Delete this AP from the manager',
		onclick: async () => {
			const out = h('div');
			box.replaceChildren(out);
			const op = { kind: 'remove-ap', node: ap.id };
			const p = await ask(out, op);
			if (!p) return;
			const parent = ctx.trees.locations.nodes.get(ap.id)?.parent;
			const set = p.effect?.removed || [];
			confirm(ctx, out, op, p, [
				h('div', null, h('strong', null, 'Delete the AP: '), ap.name),
				set.length > 0
					? h('div', null, h('strong', null, 'What is set on it goes with it: '), set.map((o) => `${group(o.path).title} · ${group(o.path).label} (${value(o.path, o.value, (id) => ctx.name('services', id))})`).join(', '))
					: h('div', { class: 'sub' }, 'Nothing is set on it.'),
				h('div', { class: 'sub' }, 'Its token stops working, and the manager forgets what it said of itself. The change log keeps all of it.'),
				h('div', { class: 'sub' }, 'If it is still on and reaches the manager, it enrolls again and comes back in Landing Zone, running what it has until it is adopted. To keep it gone, power it off or take Aeolus off it first.'),
			], [], () => { location.hash = `#/locations${parent ? `/${encodeURIComponent(parent)}` : ''}`; });
		},
	}, 'Delete…');
}

// An AP back, or another in its place (0223). An AP whose token was lost, as
// to a reflash or a reset, enrolls again and waits with a new one: readmit-ap
// lets it back in, as it was. One waiting in Landing Zone takes another's
// place with replace-ap: the AP keeps its name, place, settings and records,
// and the new box's token and what it says of itself are its own.

// inLandingZone says whether a node waits in Landing Zone, or another
// isolated folder.
function inLandingZone(t, id) {
	for (let n = t.nodes.get(id); n; n = t.nodes.get(n.parent)) if (n.isolated) return true;
	return false;
}

const FACTS = [['mac', 'MAC'], ['model', 'model'], ['board', 'board'], ['openwrt', 'OpenWrt']];

// held says, of a preview p, the configs the manager would hold after it, as
// a note, and gives p without them: letting an AP back in or replacing it
// hands it its token whatever its config, which it applies once it passes.
function held(ctx, p) {
	const problems = Object.entries(p.checks || {});
	const note = problems.length > 0 && h('div', { class: 'banner problems' },
		h('strong', null, 'Its config breaks these rules, so it would be held: the AP runs what it has until they are mended'),
		h('ul', null, problems.flatMap(([id, list]) => list.map((x) => h('li', null, `${ctx.name('locations', id)}: ${x}`)))));
	return [note, { ...p, checks: {} }];
}

// againBanner says that ap ({ id, name }) enrolled again and waits, as its
// page has it, with what differs from what it said before, and, for may, the
// button that lets it back in, its preview opening in box. Nothing where it
// does not wait.
export function againBanner(ctx, ap, page, box, may) {
	const a = page.again;
	if (!a) return null;
	const was = page.facts || {};
	const differs = FACTS.filter(([k]) => a[k] && a[k] !== was[k]).map(([k, name]) => `${name} ${was[k] || '—'} → ${a[k]}`);
	return h('div', { class: 'banner problems' },
		h('strong', null, 'It enrolled again, and waits to be let back in'),
		h('div', null, `From ${a.source || 'an address not known'}: its token is gone, as after a reflash or a reset. Until a person lets it back in, it gets nothing from the manager, and its old token works on.`),
		differs.length > 0 && h('div', { class: 'sub' }, `What it says now: ${differs.join('; ')}.`),
		h('div', { class: 'sub' }, 'Let it back in only if this is the AP: a MAC is no secret, and its config carries its networks’ passphrases.'),
		may && h('div', { class: 'actions' }, h('button', { type: 'button', class: 'button primary', onclick: async () => {
			const out = h('div');
			box.replaceChildren(out);
			const op = { kind: 'readmit-ap', node: ap.id };
			const asked = await ask(out, op);
			if (!asked) return;
			const [note, p] = held(ctx, asked);
			confirm(ctx, out, op, p, [
				h('div', null, h('strong', null, 'Let it back in: '), ap.name),
				h('div', { class: 'sub' }, 'The token it enrolled again with is its own, and its old one stops working. Its place, name, settings and records stay as they are.'),
				note,
			], []);
		} }, 'Let it back in…')));
}

// replaceAPButton offers to put an AP waiting in Landing Zone in ap's place
// ({ id, name }), its choice and preview opening in box. Nothing where none
// waits.
export function replaceAPButton(ctx, ap, box, cls = 'rename head') {
	const t = ctx.trees.locations;
	const waiting = t.list.filter((n) => n.kind === 'ap' && n.id !== ap.id && inLandingZone(t, n.id));
	if (!waiting.length || inLandingZone(t, ap.id)) return null;
	return h('button', {
		type: 'button', class: `button small ${cls}`, title: 'Put an AP waiting in Landing Zone in this one’s place: it keeps this one’s name, place, settings and records',
		onclick: () => {
			const out = h('div');
			const pick = h('select', { 'aria-label': 'The AP to put in its place' }, waiting.map((n) => h('option', { value: n.id }, `${n.name} (${n.id})`)));
			const go = async () => {
				const op = { kind: 'replace-ap', node: ap.id, with: pick.value };
				const asked = await ask(out, op);
				if (!asked) return;
				const [note, p] = held(ctx, asked);
				const was = p.effect?.before || {}, now = p.effect?.after || {};
				const box2 = (f) => [f.model, f.mac].filter(Boolean).join(', ') || 'not known';
				confirm(ctx, out, op, p, [
					h('div', null, h('strong', null, `Put ${now.from_name || pick.value} in ${ap.name}’s place`)),
					h('div', null, `The box: ${box2(was)} → ${box2(now)}.`),
					h('div', { class: 'sub' }, `${ap.name} keeps its name, place, settings and records. ${now.from_name || pick.value}’s own entry in Landing Zone goes, and its token is ${ap.name}’s.`),
					h('div', { class: 'sub' }, 'The old box’s token stops working: plugged in again, it enrolls as a new AP.'),
					note,
				], []);
			};
			box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
				h('h2', null, `Replace ${ap.name}`),
				h('div', { class: 'fieldform' },
					h('label', { class: 'field' }, h('span', { class: 'label' }, 'With the AP waiting in Landing Zone'), pick),
					h('div', { class: 'actions' }, h('button', { type: 'button', class: 'button primary', onclick: go }, 'Preview'), h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))),
				out));
		},
	}, 'Replace…');
}
