// A change on its way to the log (0042): previewed first, showing what it
// does, the APs that get a new config and any rule it would break; then
// Apply. Nothing is recorded until Apply, and the change is logged under
// the person's name and the time; it asks for no note (Griff, 2026-10-21:
// "get rid of the reason for a change").

import { h } from '../dom.js';
import { post } from '../api.js';
import { hurry, flash, redrawNow } from '../refresh.js';

// ask previews op, showing progress and any error in box. It returns the
// preview, or null if there is none.
export async function ask(box, op) {
	box.replaceChildren(h('div', { class: 'sub', 'data-editing': true }, 'Checking…'));
	try {
		return await post('/v1/preview', { op });
	} catch (e) {
		box.replaceChildren(h('div', { 'data-editing': true }, h('div', { class: 'error' }, e.message), cancelButton(box)));
		return null;
	}
}

// resolvedAt is what a preview p says the fields of node in tree would
// resolve to once the change is made, by path; null for one nothing would
// set (0046). A batch's preview says it for each of its changes (0203).
export function resolvedAt(p, tree, node) {
	if (!Array.isArray(p?.resolved)) return p?.resolved || {};
	return Object.assign({}, ...p.resolved.filter((r) => r.tree === tree && r.node === node).map((r) => r.resolved || {}));
}

// confirm fills box with the preview p of op: lines say what it does, and
// notes, shown when APs would apply it, say what applying means for them.
// done, where there is one, runs once the change is made, before the page
// is drawn again: for a change after which the page is another's.
export function confirm(ctx, box, op, p, lines, notes, done) {
	const name = (id) => ctx.name('locations', id);
	const problems = Object.entries(p.checks || {});
	const affected = p.reversioned || [];
	const apply = h('button', { type: 'button', class: 'button primary', disabled: problems.length > 0 }, 'Apply');
	apply.addEventListener('click', async () => {
		apply.disabled = true;
		try {
			const res = await post('/v1/changes', { op });
			box.replaceChildren();
			hurry(150);
			const who = affected.length === 1 ? name(affected[0]) : `The ${affected.length} APs`;
			flash(`Saved as change #${res.change.seq}. ${affected.length ? `${who} pick${affected.length === 1 ? 's' : ''} it up on the next poll, within about a minute.` : 'No AP\'s config changed.'}`);
			done?.();
			redrawNow();
		} catch (e) {
			apply.disabled = false;
			box.querySelector('.error')?.remove();
			box.append(h('div', { class: 'error' }, e.message));
		}
	});
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		lines,
		h('div', { class: 'sub' }, affected.length
			? `New config version for ${affected.length <= 5 ? affected.map(name).join(', ') : `${affected.length} APs`}.`
			: 'No AP gets a new config: what each one runs stays the same.'),
		problems.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, 'Aeolus would hold these configs, so the APs would not apply them:'),
			h('ul', null, problems.flatMap(([id, list]) => list.map((x) => h('li', null, `${name(id)}: ${x}`))))),
		problems.length === 0 && affected.length > 0 && notes,
		h('div', { class: 'actions' }, apply, cancelButton(box))));
	apply.focus();
}

export function cancelButton(box) {
	return h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel');
}
