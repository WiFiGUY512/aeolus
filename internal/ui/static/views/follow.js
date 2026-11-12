// Following the folder again (0046): values a node sets for itself are unset
// in one change, so it inherits from above once more. The preview says what
// each field becomes and where that comes from. Where there are several, each
// has a tick box (0159): what is unticked stays set where it is, so a revert
// takes only what is meant to go. On a page with one save (0204), it is a
// draft, saved with the rest of the page's changes.

import { h } from '../dom.js';
import { group, value, probeOnly, PROBE_ONLY } from '../format.js';
import { ask, confirm, cancelButton, resolvedAt } from './confirm.js';
import { UNSET } from './pagesave.js';

// followButton offers to unset paths at node, previewing in box. parentName
// is the folder above, or null at the root, where nothing is above. heading,
// when given, names the change in the preview, such as a delete. With save,
// the page's save, it is a draft instead: pressed, the paths are unset when
// the page is saved, and pressed again, they are not; several are each
// ticked, in box.
export function followButton(ctx, tree, node, nodeName, parentName, paths, box, label, heading, save) {
	label ??= parentName ? `Follow ${parentName}` : 'Stop setting it here';
	const title = heading ?? (parentName ? `${nodeName} follows ${parentName} again` : `${nodeName} stops setting ${paths.length === 1 ? 'this' : 'these'}`);
	if (save) return followDraft(ctx, tree, node, paths, box, label, title, save);
	return h('button', { type: 'button', class: 'button small', onclick: () => follow(ctx, tree, node, paths, box, title) }, label);
}

// notesFor says what unsetting paths means for the APs.
function notesFor(paths) {
	const radios = paths.some((x) => x.startsWith('radio.'));
	// What the prober asks, and how often, reloads nothing (0059).
	const probes = probeOnly(paths);
	const vnis = !probes && paths.some((x) => /^ports\.[^.]+\.vxlan\./.test(x));
	const ports = !probes && !vnis && paths.some((x) => x.startsWith('ports.'));
	const tunnels = !probes && paths.some((x) => x.startsWith('concentrators.'));
	return [
		radios && h('div', { class: 'sub warn' }, 'Applying restarts each radio whose settings change; its clients drop briefly and reconnect.'),
		probes && h('div', { class: 'sub warn' }, PROBE_ONLY),
		ports && h('div', { class: 'sub warn' }, "Applying reloads each AP's network; wired clients on the port drop briefly. Each AP leaves a port Aeolus no longer sets as it is."),
		// A tunnel port carries only the VNIs in force, so one removed comes off (0058).
		vnis && h('div', { class: 'sub warn' }, "Applying reloads each AP's network; wired clients on the port drop briefly. Each AP takes the port off a VNI it no longer carries; the port stays a tunnel port."),
		tunnels && h('div', { class: 'sub warn' }, "A network whose transport names a tunnel not set at an AP leaves that transport out there; where it was the network's only one, the preview shows the config Aeolus would hold."),
	];
}

// becomes says what a field was and what it becomes: what is above it, and
// from where, or nothing Aeolus sets.
export function becomes(ctx, tree, path, before, after) {
	const names = (id) => ctx.name('services', id);
	return [value(path, before, names), ' → ',
		after ? [value(path, after.value, names), ` (from ${ctx.name(tree, after.from)})`] : h('span', { class: 'sealed' }, 'not set by Aeolus any more')];
}

// followDraft is followButton on a page with one save: a part of it.
function followDraft(ctx, tree, node, paths, box, label, title, save) {
	let on = false;
	let ticked = new Set(paths);
	const button = h('button', { type: 'button', class: 'button small', 'aria-pressed': 'false', title: 'Unset when the page is saved; press again to keep it' }, label);
	// The tick boxes, where there are several, in box; one path leaves box
	// alone.
	const draw = () => {
		button.setAttribute('aria-pressed', String(on));
		if (paths.length > 1) box.replaceChildren(...(on ? [h('ul', { class: 'becomes ticks' }, paths.map((path) => {
			const g = group(path);
			const tick = h('input', { type: 'checkbox', 'aria-label': `${g.title} · ${g.label}` });
			tick.checked = ticked.has(path);
			tick.addEventListener('change', () => {
				if (tick.checked) ticked.add(path);
				else ticked.delete(path);
				save.changed(part, tick.checked ? { [path]: UNSET } : undefined);
			});
			return h('li', null, h('label', null, tick, ' ', `${g.title} · ${g.label}`));
		}))] : []));
	};
	const part = {
		tree,
		node,
		changes: () => (on ? Object.fromEntries([...ticked].map((path) => [path, UNSET])) : {}),
		dirty: () => on && ticked.size > 0,
		undo: () => {
			on = false;
			ticked = new Set(paths);
			draw();
		},
		summary: (values, seen) => [
			h('div', null, h('strong', null, title)),
			h('ul', { class: 'becomes' }, Object.keys(values).map((path) => h('li', null,
				`${group(path).title} · ${group(path).label}: `, becomes(ctx, tree, path, seen.was(path), seen.after(path))))),
		],
		notes: (values) => notesFor(Object.keys(values)),
	};
	button.addEventListener('click', () => {
		on = !on;
		draw();
		save.changed(part, on ? part.changes() : undefined);
	});
	save.register(part);
	return button;
}

// follow previews unsetting paths, and then those of them still ticked:
// picked is what was learnt of each from the first preview, and which are
// ticked, once there is one.
async function follow(ctx, tree, node, paths, box, title, picked) {
	const chosen = picked ? paths.filter((x) => picked.ticked.has(x)) : paths;
	const names = (id) => ctx.name('services', id);
	// Each field: what it is now, and what it becomes. Several have a tick
	// box each; one that is unticked stays as it is.
	const line = (path) => {
		const g = group(path);
		const { before, after } = picked.seen.get(path);
		const on = picked.ticked.has(path);
		const says = [h('span', null, `${g.title} · ${g.label}: `),
			on ? becomes(ctx, tree, path, before, after) : [value(path, before, names), ' → ', h('span', { class: 'sub' }, 'stays set here')]];
		if (paths.length === 1) return h('li', null, says);
		const tick = h('input', { type: 'checkbox', 'aria-label': `${g.title} · ${g.label}` });
		tick.checked = on;
		tick.addEventListener('change', () => {
			if (tick.checked) picked.ticked.add(path);
			else picked.ticked.delete(path);
			follow(ctx, tree, node, paths, box, title, picked);
		});
		return h('li', { class: on ? 'ticked' : 'unticked' }, h('label', null, tick, ' ', says));
	};
	const lines = () => [
		h('div', null, h('strong', null, title),
			paths.length > 1 && h('span', { class: 'sub' }, ` · ${chosen.length} of ${paths.length} ticked`)),
		h('ul', { class: paths.length > 1 ? 'becomes ticks' : 'becomes' }, paths.map(line)),
	];
	if (picked && !chosen.length) {
		box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
			lines(), h('div', { class: 'sub' }, 'Nothing is ticked: nothing changes.'),
			h('div', { class: 'actions' }, cancelButton(box))));
		return;
	}
	const op = chosen.length === 1
		? { kind: 'unset', tree, node, path: chosen[0] }
		: { kind: 'unset', tree, node, paths: chosen };
	// Only the latest asking is shown: an answer to an earlier tick that
	// comes in late does not replace it.
	const mine = picked ? ++picked.asked : 0;
	const p = await ask(box, op);
	if (!p || (picked && mine !== picked.asked)) return;
	// What each was, and becomes, is learnt from the first preview, of them
	// all: unsetting one does not change what another becomes.
	if (!picked) {
		const before = paths.length === 1 ? { [paths[0]]: p.effect?.before } : (p.effect?.before || {});
		const after = resolvedAt(p, tree, node);
		picked = { seen: new Map(paths.map((x) => [x, { before: before[x], after: after[x] }])), ticked: new Set(paths), asked: 0 };
	}
	confirm(ctx, box, op, p, lines(), notesFor(chosen));
}
