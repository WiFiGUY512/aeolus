// A window that floats over the page (0149): what is watched while the page
// under it is worked on, as a client's Live Trace is while the client is
// put off and on again from the list. It is no part of the page: it stays
// where it was put while the page is drawn again or another is opened, and
// the page is not held from drawing by it.
//
// It is moved by its panel's heading and sized by its corner, and comes
// back where and as large as it was left. Its panel's own Close empties
// it, and emptied it is gone.

import { h } from './dom.js';

const KEPT = 'aeolus.float.';   // where each window's place and size are kept, by its name
const EDGE = 8;                 // px kept between a window and the edge of what is seen
const open = new Map();         // a window that is there, by its name -> { el, body }
let top = 50;                   // the uppermost of them

function kept(name) {
	try {
		const v = JSON.parse(localStorage.getItem(KEPT + name) || 'null');
		return v && [v.left, v.top, v.width, v.height].every(Number.isFinite) ? v : null;
	} catch (e) {
		return null;
	}
}

function keep(name, el) {
	const r = el.getBoundingClientRect();
	if (!r.width) return;
	try {
		localStorage.setItem(KEPT + name, JSON.stringify({ left: Math.round(r.left), top: Math.round(r.top), width: Math.round(r.width), height: Math.round(r.height) }));
	} catch (e) {
		// Not kept, then.
	}
}

// within holds a place to what is seen of the page.
function within(el, left, top_) {
	const w = el.offsetWidth, hgt = el.offsetHeight;
	el.style.left = `${Math.max(EDGE, Math.min(left, window.innerWidth - w - EDGE))}px`;
	el.style.top = `${Math.max(EDGE, Math.min(top_, window.innerHeight - Math.min(hgt, 120) - EDGE))}px`;
}

// floatWindow gives the window of that name, opened if it is not there, and
// returns what a panel is drawn into. One of a name is there at a time:
// what is drawn into it next takes the place of what was. what is said of
// it to a screen reader; size is how large it is when first opened.
export function floatWindow(name, what, size = { width: 780, height: 520 }) {
	const was = open.get(name);
	if (was && was.el.isConnected) {
		was.el.style.zIndex = String(++top);
		return was.body;
	}
	const body = h('div', { class: 'floatbody' });
	const el = h('div', { class: 'float', role: 'dialog', 'aria-label': what }, body);
	const k = kept(name);
	const width = Math.min(k?.width || size.width, window.innerWidth - 2 * EDGE), height = Math.min(k?.height || size.height, window.innerHeight - 2 * EDGE);
	el.style.width = `${width}px`;
	el.style.height = `${height}px`;
	el.style.zIndex = String(++top);
	document.body.append(el);
	within(el, k ? k.left : window.innerWidth - width - 24, k ? k.top : window.innerHeight - height - 24);
	open.set(name, { el, body });

	// Emptied, as by its panel's own Close, it is gone.
	const gone = new MutationObserver(() => {
		if (body.childNodes.length) return;
		gone.disconnect();
		sized.disconnect();
		el.remove();
		open.delete(name);
	});
	gone.observe(body, { childList: true });

	// Sized by its corner: how large it was left is kept.
	let settle = null;
	const sized = new ResizeObserver(() => {
		clearTimeout(settle);
		settle = setTimeout(() => keep(name, el), 300);
	});
	sized.observe(el);

	// Moved by its panel's heading, and not by what in the heading is
	// pressed for something else.
	let from = null;
	el.addEventListener('pointerdown', (e) => {
		el.style.zIndex = String(++top);
		if (e.button !== 0 || !e.target.closest('h2') || e.target.closest('button, a, input, select, label, .segmented')) return;
		const r = el.getBoundingClientRect();
		from = { x: e.clientX - r.left, y: e.clientY - r.top };
		el.setPointerCapture?.(e.pointerId);
		e.preventDefault();
	});
	el.addEventListener('pointermove', (e) => {
		if (from) within(el, e.clientX - from.x, e.clientY - from.y);
	});
	const let_go = () => {
		if (!from) return;
		from = null;
		keep(name, el);
	};
	el.addEventListener('pointerup', let_go);
	el.addEventListener('pointercancel', let_go);
	return body;
}

// floating says whether what is drawn into box is in such a window: it
// then holds no page from drawing.
export function floating(box) {
	return !!box?.closest?.('.float');
}
