// Tables a viewer fits to their own eye (Griff, 2026-10-12: "expand or
// contract them at will, some are unnecessarily long or tall and can be
// fixed with expanding the column"; and to hide or show columns). Every
// list table gets a grip at the right edge of each heading, dragged to make
// that column wider or narrower, and a small menu on its heading row to
// hide or show columns and to put the table back as it came.
//
// What a viewer chose is kept in this browser, by the table's headings: a
// table with the same headings elsewhere is the same table to it. A page
// draws its tables again every half minute; each is fitted again as it
// comes.

import { h } from './dom.js';

const KEY = 'aeolus.tables';
// A column is never dragged narrower than this, in px.
const LEAST = 48;

// saved is what was chosen, by a table's headings: { w: { heading: px },
// hide: [heading] }.
let saved = {};
try {
	saved = JSON.parse(localStorage.getItem(KEY) || '{}') || {};
} catch { /* storage blocked, or what was kept is no JSON: nothing chosen */ }

function keep() {
	// What says nothing is not kept.
	for (const k of Object.keys(saved))
		if (!Object.keys(saved[k].w || {}).length && !(saved[k].hide || []).length) delete saved[k];
	try {
		localStorage.setItem(KEY, JSON.stringify(saved));
	} catch { /* storage blocked: kept while the page is */ }
}

// A table may name, in data-hide, the columns it hides until its viewer
// shows them (0284): where the viewer chose nothing, those are hidden, and
// "As it came" hides them again.
const asCame = (table) => {
	try {
		return JSON.parse(table.dataset.hide || '[]');
	} catch {
		return [];
	}
};

const chosen = (sig, table) => saved[sig] || (saved[sig] = { w: {}, hide: asCame(table) });

// The table whose menu was left open, by its headings, over a redraw.
let menuOf = null;

// headings is a table's heading cells: those of its first row, where that
// row is headings.
function headings(table) {
	const row = table.rows[0];
	const ths = row ? [...row.children] : [];
	return ths.length >= 2 && ths.every((c) => c.tagName === 'TH') ? ths : [];
}

// A heading's name: its words, or its place where it has none. The mark of
// how a table is sorted is no part of it: the Clients tab writes one beside
// the heading it sorts by.
const named = (th, i) => (th.dataset.col ?? th.textContent.replace(/[\u25b4\u25be]/g, '').trim()) || `column ${i + 1}`;

// fit applies what was chosen to a table: the columns hidden, and the
// widths. With any width chosen the table is laid out by its headings'
// widths alone, every column held at what it measured or was dragged to, so
// that one made wider makes the table wider and no other narrower.
function fit(table, ths, names, st) {
	const off = new Set(st.hide || []);
	for (const tr of table.rows) {
		// A row that spans columns, as one opened under another, is left.
		if (tr.children.length !== ths.length) continue;
		[...tr.children].forEach((c, i) => { c.hidden = off.has(names[i]); });
	}
	const any = names.some((n) => !off.has(n) && st.w?.[n] != null);
	table.style.tableLayout = '';
	table.style.width = '';
	ths.forEach((th) => { th.style.width = ''; });
	table.classList.toggle('fitted', any);
	// A table made wider than its place scrolls sideways within it.
	if (table.parentElement) table.parentElement.style.overflowX = any ? 'auto' : '';
	if (!any) return;
	const widths = ths.map((th, i) => (off.has(names[i]) ? 0 : st.w[names[i]] ?? Math.ceil(th.getBoundingClientRect().width)));
	ths.forEach((th, i) => { th.style.width = `${widths[i]}px`; });
	table.style.tableLayout = 'fixed';
	table.style.width = `${widths.reduce((a, b) => a + b, 0)}px`;
}

// drag makes column i wider or narrower as the pointer moves, and keeps
// what it comes to.
function drag(e, table, ths, names, i, sig) {
	e.preventDefault();
	e.stopPropagation();
	const st = chosen(sig, table);
	// Every column is held where it is, from the first drag on.
	ths.forEach((th, k) => { if (!th.hidden && st.w[names[k]] == null) st.w[names[k]] = Math.ceil(th.getBoundingClientRect().width); });
	const from = e.clientX, was = st.w[names[i]];
	const grip = e.currentTarget;
	grip.setPointerCapture?.(e.pointerId);
	table.classList.add('fitting');
	const move = (m) => {
		st.w[names[i]] = Math.max(LEAST, Math.round(was + m.clientX - from));
		fit(table, ths, names, st);
	};
	const done = () => {
		grip.removeEventListener('pointermove', move);
		grip.removeEventListener('pointerup', done);
		grip.removeEventListener('pointercancel', done);
		table.classList.remove('fitting');
		keep();
	};
	grip.addEventListener('pointermove', move);
	grip.addEventListener('pointerup', done);
	grip.addEventListener('pointercancel', done);
	fit(table, ths, names, st);
}

// menu is the table's own: a slider a column, to show or hide it, and a way
// back to how the table came.
function menu(table, ths, names, sig, again) {
	const st = chosen(sig, table);
	const off = new Set(st.hide || []);
	const box = h('div', { class: 'colpop', role: 'group', 'aria-label': 'Columns' },
		names.map((n) => h('label', null,
			h('input', { type: 'checkbox', class: 'switch', role: 'switch', checked: !off.has(n), 'aria-label': `Show ${n}`,
				// The last column showing stays: a table of none has no way back.
				disabled: !off.has(n) && names.length - off.size <= 1,
				onchange: (e) => {
					st.hide = e.target.checked ? (st.hide || []).filter((x) => x !== n) : [...(st.hide || []), n];
					keep();
					again();
				} }),
			n)),
		h('button', { type: 'button', class: 'button small', onclick: () => { delete saved[sig]; keep(); menuOf = null; again(); } }, 'As it came'));
	// Nothing in it sorts or follows a link under it.
	box.addEventListener('click', (e) => e.stopPropagation());
	return box;
}

function enhance(table) {
	if (table.dataset.fit) return;
	const ths = headings(table);
	if (!ths.length) return;
	table.dataset.fit = '1';
	const names = ths.map(named);
	// Two headings of one name are told apart by their place.
	names.forEach((n, i) => { if (names.indexOf(n) !== i) names[i] = `${n} (${i + 1})`; });
	const sig = names.join('|');
	const st = saved[sig] || { w: {}, hide: asCame(table) };
	// A heading's name is kept from before its grip and menu are in it.
	ths.forEach((th, i) => { th.dataset.col = names[i]; });

	const again = () => {
		table.querySelectorAll('.colgrip, .colmenu, .colpop').forEach((x) => x.remove());
		delete table.dataset.fit;
		enhance(table);
	};
	ths.forEach((th, i) => {
		const grip = h('span', { class: 'colgrip', title: 'Drag to make this column wider or narrower', 'aria-hidden': 'true' });
		grip.addEventListener('pointerdown', (e) => drag(e, table, ths, names, i, sig));
		// A grip is no part of a heading that sorts, or links.
		grip.addEventListener('click', (e) => e.stopPropagation());
		th.append(grip);
	});
	// The menu is on the last heading that shows.
	const off = new Set(st.hide || []);
	const last = ths.findLast((th, i) => !off.has(names[i])) || ths[ths.length - 1];
	const open = menuOf === sig;
	const button = h('button', { type: 'button', class: `colmenu${open ? ' on' : ''}`, title: 'Columns: show, hide, or put back as they came', 'aria-label': 'Columns', 'aria-expanded': String(open) }, '⋯');
	button.addEventListener('click', (e) => {
		e.stopPropagation();
		menuOf = open ? null : sig;
		again();
	});
	last.append(button);
	fit(table, ths, names, st);
	if (open) {
		// Laid over the page from under its button, so that no panel that
		// scrolls cuts it off.
		const box = menu(table, ths, names, sig, again);
		last.append(box);
		const r = button.getBoundingClientRect();
		box.style.top = `${Math.round(r.bottom + 4)}px`;
		box.style.right = `${Math.max(8, Math.round(window.innerWidth - r.right))}px`;
	}
}

// watchTables fits every list table under root, now and as each is drawn.
export function watchTables(root) {
	const all = () => root.querySelectorAll('table.list').forEach(enhance);
	new MutationObserver(all).observe(root, { childList: true, subtree: true });
	// A menu left open closes with the page it was on, and at a click
	// anywhere else.
	window.addEventListener('hashchange', () => { menuOf = null; });
	document.addEventListener('click', () => {
		if (menuOf == null) return;
		menuOf = null;
		root.querySelectorAll('table.list').forEach((t) => {
			t.querySelectorAll('.colgrip, .colmenu, .colpop').forEach((x) => x.remove());
			delete t.dataset.fit;
		});
		all();
	});
	all();
}
