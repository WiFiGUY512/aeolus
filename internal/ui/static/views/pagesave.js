// One save a page (0194, 0202, 0204): each part of a page that sets
// something, a band, a form, a switch, a Follow above, keeps a draft, and
// the page has one Undo and one "Save…", kept in view at its foot. Save
// previews every part's changes as one change, and Apply makes it one entry
// in the log: a set, where it sets fields of one node and unsets none, or
// else a batch of each node's sets and unsets (0203).
//
// A part is { changes, dirty, blocked, summary, notes, undo, take, open,
// tree, node }: changes() is what its draft sets, as {path: value}, a value
// UNSET to unset the field, or throws with what is wrong; dirty() whether it
// has a change; blocked() whether it cannot be saved as it is; summary(values,
// seen) and notes(values) the preview's lines and what applying means, for
// the values it is saving, where seen.was(path) is what the field was set to
// at the part's node and seen.after(path) what it would resolve to there
// afterwards; undo() puts it back as saved. take(values), where it has one,
// is told what another part of its node set, so a value two parts can set
// reads the same in both; open(), whether it is a form opened for editing:
// the bar shows while one is, or while anything has a change. tree and node,
// where given, are where it sets, in place of the page's own. A part may
// instead, or as well, have ops(): changes of another kind it makes whole,
// such as the service folders a location takes (0320). They go first in the
// batch, in the order the parts came, the sets and unsets after them, so a
// set may be in a folder an op makes.

import { h } from '../dom.js';
import { ask, confirm, resolvedAt } from './confirm.js';

// UNSET, as a part's value for a path, unsets the field at its node: it
// takes what is above again (0046).
export const UNSET = Symbol('unset');

// json writes values to compare them, an unset as one.
const json = (v) => JSON.stringify(v, (k, x) => (x === UNSET ? { '': 'unset' } : x));

// seenIn reads, from the preview p of op, what each field of a node was set
// to there, and what it would resolve to there afterwards: a batch says each
// of its changes' (0203).
function seenIn(p, op) {
	const ops = op.kind === 'batch' ? op.ops : [op];
	const effects = op.kind === 'batch' ? (Array.isArray(p.effect?.after) ? p.effect.after : []) : [p.effect];
	return (tree, node) => ({
		was(path) {
			for (const [i, o] of ops.entries()) {
				if (o.tree !== tree || o.node !== node) continue;
				const before = effects[i]?.before;
				if (o.path === path) return before;
				if ((o.values || o.paths) && before && path in before) return before[path];
			}
			return undefined;
		},
		after: (path) => resolvedAt(p, tree, node)[path],
	});
}

// opOf makes the change from each part's values, and the whole changes
// raw that parts make: one set, or the one raw change, where it is all;
// else a batch of the raw changes, then each node's sets and unsets.
function opOf(list, raw = []) {
	const groups = new Map();
	for (const { at, values } of list) {
		const k = `${at.tree} ${at.node}`;
		if (!groups.has(k)) groups.set(k, { ...at, sets: {}, unsets: [] });
		const g = groups.get(k);
		for (const [p, v] of Object.entries(values)) {
			if (v === UNSET) g.unsets.push(p);
			else g.sets[p] = v;
		}
	}
	const ops = [...raw];
	for (const { tree, node, sets, unsets } of groups.values()) {
		const paths = Object.keys(sets);
		if (paths.length === 1) ops.push({ kind: 'set', tree, node, path: paths[0], value: sets[paths[0]] });
		else if (paths.length) ops.push({ kind: 'set', tree, node, values: sets });
		if (unsets.length === 1) ops.push({ kind: 'unset', tree, node, path: unsets[0] });
		else if (unsets.length) ops.push({ kind: 'unset', tree, node, paths: unsets });
	}
	if (ops.length === 1 && (ops[0].kind === 'set' || raw.length === 1)) return ops[0];
	return { kind: 'batch', ops };
}

// draftPart is a form's part of the page's save (0202): what read() sets, a
// change where it sets anything, and held back while it throws; open while
// the form is; undone by closing it.
export function draftPart(read, undo, more) {
	return {
		changes: read,
		dirty: () => {
			try {
				return Object.keys(read()).length > 0;
			} catch {
				return true;
			}
		},
		blocked: () => {
			try {
				read();
				return false;
			} catch {
				return true;
			}
		},
		open: () => true,
		undo,
		...more,
	};
}

// watch tells the page's save when a field in el changes, and says under
// it, in msg, what is wrong with the draft, if anything.
export function watch(el, msg, read, save, part) {
	const edited = () => {
		msg.replaceChildren();
		try {
			read();
		} catch (e) {
			msg.replaceChildren(e.message);
		}
		save.changed(part);
	};
	el.addEventListener('input', edited);
	el.addEventListener('change', edited);
}

// pageSave is the page's save for node in tree. summary and notes, where
// given, say the whole change in place of each part saying its own.
export function pageSave(ctx, tree, node, { summary, notes } = {}) {
	// What says the whole change: the page's own, and each that a section
	// sharing this save adds (also), each of its own fields (0284).
	const sums = summary ? [summary] : [];
	const tells = notes ? [notes] : [];
	let parts = [];
	// The part that set each field last: where two set one, its value wins.
	const latest = new Map();
	// What each part's changes were when it last said: what changed since is
	// what it set last.
	const last = new Map();
	// What the preview open in box shows, as JSON: a draft changed from it
	// takes it away.
	let shown = null;
	const box = h('div', { class: 'edit' });
	const undoAll = h('button', { type: 'button', class: 'button' }, 'Undo');
	const saveAll = h('button', { type: 'button', class: 'button primary' }, 'Save…');
	const bar = h('div', { class: 'pagesave', hidden: true }, box, h('div', { class: 'actions' }, undoAll, saveAll));
	const at = (x) => ({ tree: x.tree || tree, node: x.node || node });
	const key = (x, path) => `${at(x).tree} ${at(x).node} ${path}`;
	const same = (x, y) => key(x, '') === key(y, '');
	const changesOf = (x) => {
		try {
			return x.changes();
		} catch {
			return {};
		}
	};
	const refresh = () => {
		const dirty = parts.filter((x) => x.dirty());
		saveAll.disabled = !dirty.length || dirty.some((x) => x.blocked?.());
		undoAll.disabled = !dirty.length;
		bar.hidden = !dirty.length && !parts.some((x) => x.open?.());
		// A draft keeps the page from being drawn again under it.
		bar.toggleAttribute('data-editing', dirty.length > 0);
	};
	// each is every part's changes, each field said once, by the part that
	// set it last.
	const each = () => {
		const all = parts.filter((x) => x.dirty()).map((x) => ({ x, at: at(x), values: x.changes() }));
		const owner = new Map();
		for (const e of all)
			for (const p of Object.keys(e.values))
				if (!owner.has(key(e.x, p)) || latest.get(key(e.x, p)) === e.x) owner.set(key(e.x, p), e);
		return all.map((e) => ({ ...e, values: Object.fromEntries(Object.entries(e.values).filter(([p]) => owner.get(key(e.x, p)) === e)) }))
			.filter((e) => Object.keys(e.values).length);
	};
	// raws is each part with a change that makes whole changes, with them.
	const raws = () => parts.filter((x) => x.ops && x.dirty()).map((x) => ({ x, ops: x.ops() })).filter((r) => r.ops.length);
	// what is the drafts as a preview shows them, to tell a change from it.
	const what = () => json([each().map((e) => [e.at, e.values]), raws().map((r) => r.ops)]);
	undoAll.addEventListener('click', () => {
		box.replaceChildren();
		shown = null;
		latest.clear();
		for (const x of [...parts]) x.undo();
		for (const x of parts) last.set(x, changesOf(x));
		refresh();
	});
	saveAll.addEventListener('click', async () => {
		let list;
		let whole;
		try {
			list = each();
			whole = raws();
		} catch (e) {
			box.replaceChildren(h('div', { class: 'error' }, e.message));
			return;
		}
		if (!list.length && !whole.length) return;
		const op = opOf(list, whole.flatMap((r) => r.ops));
		shown = what();
		const p = await ask(box, op);
		if (!p) return;
		const seen = seenIn(p, op);
		// The page's own node's values, for a page that says the whole change.
		const all = Object.assign({}, ...list.filter((e) => e.at.tree === tree && e.at.node === node).map((e) => e.values));
		// A note two parts give is said once.
		const said = new Set();
		const wholeNotes = whole.flatMap((r) => r.x.notes?.({}) || []);
		const once = ([...wholeNotes, ...list.flatMap((e) => e.x.notes?.(e.values) || [])]).flat(Infinity).filter((n) => n && !said.has(n.textContent) && said.add(n.textContent));
		confirm(ctx, box, op, p,
			[...whole.flatMap((r) => r.x.summary({}, seen(at(r.x).tree, at(r.x).node))),
				...(sums.length ? sums.flatMap((f) => f(all, seen(tree, node))) : list.flatMap((e) => e.x.summary(e.values, seen(e.at.tree, e.at.node))))],
			tells.length ? [...tells.flatMap((f) => f(all)), ...wholeNotes] : once);
	});
	const api = {
		bar,
		refresh,
		// also adds what a section sharing this save says of the change: its
		// summary and notes, of the fields keep takes, where it set any.
		also(keep, { summary: s, notes: n }) {
			const mine = (values) => Object.fromEntries(Object.entries(values).filter(([p]) => keep(p)));
			const some = (values) => Object.keys(mine(values)).length > 0;
			if (s) sums.push((values, seen) => (some(values) ? s(mine(values), seen) : []));
			if (n) tells.push((values) => (some(values) ? n(mine(values)) : []));
		},
		// register adds a part, and returns what takes it off again, as when
		// its form is closed.
		register(x) {
			parts.push(x);
			last.set(x, changesOf(x));
			refresh();
			return () => {
				parts = parts.filter((y) => y !== x);
				last.delete(x);
				for (const [k, y] of latest) if (y === x) latest.delete(k);
				refresh();
			};
		},
		// changed is called by a part when its draft changes: set, where
		// given, is the fields it set just now, else what changed since it
		// last said. Those are its own until another sets them; what is set,
		// and not unset, the other parts of its node take. A preview of the
		// drafts as they were goes, once they differ.
		changed(from, set) {
			if (from) {
				const was = last.get(from) || {};
				const now = changesOf(from);
				const just = set ? Object.keys(set) : [...new Set([...Object.keys(was), ...Object.keys(now)])].filter((p) => json(was[p]) !== json(now[p]));
				for (const p of just) latest.set(key(from, p), from);
				last.set(from, now);
				const given = Object.fromEntries(Object.entries(set || {}).filter(([, v]) => v !== UNSET));
				if (Object.keys(given).length)
					for (const x of parts) if (x !== from && same(x, from)) x.take?.(given);
			}
			if (shown !== null) {
				let now = null;
				try {
					now = what();
				} catch {
					// held back: not what was previewed
				}
				if (now !== shown) {
					shown = null;
					box.replaceChildren();
				}
			}
			refresh();
		},
		// drafts is what the parts of a node set, the page's unless one is
		// given, for a form opened after them to take.
		drafts(where = {}) {
			try {
				const w = at(where);
				return Object.assign({}, ...each().filter((e) => e.at.tree === w.tree && e.at.node === w.node)
					.map((e) => Object.fromEntries(Object.entries(e.values).filter(([, v]) => v !== UNSET))));
			} catch {
				return {};
			}
		},
	};
	// scope is the save as a part of the page drawn again as a whole sees
	// it, such as a band's panel or a form: drop() takes off all it, and
	// any scope of it, registered.
	const scoped = (parent) => {
		let offs = [];
		const s = {
			...api,
			register: (x) => {
				const off = parent.register(x);
				offs.push(off);
				return off;
			},
			scope: () => scoped(s),
			drop: () => {
				for (const off of offs) off();
				offs = [];
			},
		};
		return s;
	};
	api.scope = () => scoped(api);
	return api;
}
