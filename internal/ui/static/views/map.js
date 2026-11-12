// The radio neighbours as a map (0073): each AP a node, each pair of
// neighbours on a band a line, coloured by how strongly they hear each
// other, the weaker way: the mesh RRM and power control (0077) work over.
// Lines are drawn shorter the stronger the signal, so APs that hear each
// other well sit close. A line is solid where hellos go both ways, dashed
// where one is only heard. APs outside this folder are drawn faint. An AP
// that is offline, not heard from in three poll intervals, is left off,
// with every line to it, and so is a line a live AP says is down: the map
// is the mesh as it is now (Griff, 2026-10-20). So is a neighbour the Org
// has not adopted, waiting in Landing Zone or gone from the tree, though it
// is still on the air: a deleted AP that is on enrolls again, and runs what
// it had until it is adopted (Griff, 2026-10-20: "the C360-AP is deleted and
// gone yet is still in this map").
//
// An AP can be moved on the map, by dragging it or with the arrow keys, to
// see its lines clearly; this browser keeps where it was put, for the
// folder and band (0346). The hellos are drawn going along the lines, each
// landing when the AP that hears it says the last one came, every ten
// seconds since: timed from the reports, not each one seen (0346).
//
// The drawing fills its panel's width at one unit a pixel, so its names
// stay their size; the layout is spread, or drawn in, to fill it and up to
// TALL of the window's height (Griff, 2026-11-10: "can you have it use more
// of the area inside the window"), and the drawing is as tall as the APs
// in it need ("dynamically shrink with the AP used area"). It is drawn
// again as its panel changes width.

import { h } from '../dom.js';
import { bandName, quiet } from '../format.js';

const SVG = 'http://www.w3.org/2000/svg';
const MIN_W = 320, MIN_H = 200, TALL = 0.72, R = 16;

// The most a layout is spread, so a few APs are not thrown to the corners;
// and the most it is spread one way more than the other.
const SPREAD = 3, STRETCH = 1.6;

// How far in from the drawing's sides an AP may be moved; how much room the
// drawing keeps above the highest AP, for the ring a hello lands with, and
// below the lowest, for its name and facts; and how far down, in its share
// of the window, an AP may be moved.
const SIDE = 56, ABOVE = R + 20, BELOW = R + 34, DEEPER = 2;

// Where this browser keeps where APs were moved to on a map (0346), by
// folder and band, as { id: [x, y] }: x a fraction of the drawing's width,
// y of its share of the window's height, so a map arranged in one window
// fills another; and whether it shows the hellos. Before, they were kept
// under OLD in units of a drawing 640 wide and up to 560 tall, and are
// taken from there once.
const MOVED = 'aeolus.nmap2.', OLD = 'aeolus.nmap.', OLD_W = 640, OLD_H = 560;
const HELLOS = 'aeolus.nmap.hellos';

// The width the last map was drawn at, for the first drawing of a page
// drawn again, before its panel is on the page to be measured.
let lastWidth = 0;

// What watches each map's box for its width, held as long as the box is,
// so it is not let go of while the box is on the page. A page the browser
// is not drawing, as in a tab out of sight, is told of its width once it is.
const watchers = new WeakMap();

// An AP sends its hellos every ten seconds (rrm.uc's HELLO_EVERY). One is
// drawn going along its line for TRAVEL ms before it lands, and the ring
// it lands with for LAND ms after.
const HELLO_EVERY = 10000, TRAVEL = 900, LAND = 700;

// kept reads what this browser keeps under name; keep keeps v there, or
// forgets it for null. Storage may be blocked: then nothing is kept.
function kept(name, otherwise) {
	try {
		return JSON.parse(localStorage.getItem(name) || 'null') ?? otherwise;
	} catch {
		return otherwise;
	}
}

function keep(name, v) {
	try {
		if (v == null) localStorage.removeItem(name);
		else localStorage.setItem(name, JSON.stringify(v));
	} catch {
		// Not kept: storage is blocked.
	}
}

const hellosShown = () => kept(HELLOS, true) !== false;

// The signal classes, strongest first, with the weakest signal in each.
const CLASSES = [['strong', -60, '−60 dBm or better'], ['good', -70, 'to −70'], ['fair', -80, 'to −80'], ['weak', -Infinity, 'weaker']];
const classOf = (s) => (s == null ? 'unknown' : CLASSES.find(([, min]) => s >= min)[0]);

function svg(tag, attrs, ...kids) {
	const el = document.createElementNS(SVG, tag);
	for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, String(v));
	for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
	return el;
}

// links reads, from the APs' reports (rows), every pair of neighbours on
// each band: how strongly each hears the other, and whether hellos go both
// ways. A pair both report is joined: each AP's own reading counts first.
// A neighbour whose hellos stopped (down), or that is offline (gone), has
// no line.
function links(rows, gone = new Set()) {
	const out = new Map();
	for (const { ap, cfg } of rows)
		for (const n of cfg?.condition?.state?.report?.rrm?.neighbours || []) {
			if (n.state === 'down' || gone.has(n.ap)) continue;
			for (const b of n.bands || []) {
				const [x, y] = [ap.id, n.ap].sort();
				const k = `${b.band} ${x} ${y}`;
				const l = out.get(k) || { band: b.band, a: x, b: y, hears: {}, up: false };
				// hears[p] is how strongly p hears the other.
				if (b.signal != null) l.hears[ap.id] = b.signal;
				if (b.their_signal != null && l.hears[n.ap] == null) l.hears[n.ap] = b.their_signal;
				l.up = l.up || n.state === 'up';
				out.set(k, l);
			}
		}
	for (const l of out.values()) {
		const s = Object.values(l.hears);
		l.weaker = s.length ? Math.min(...s) : null;
	}
	return [...out.values()].filter((l) => l.weaker != null);
}

// place lays the nodes out from a circle in the given order: each line
// pulls its ends to a length that follows its signal, a strong one short,
// and every node pushes the others away.
function place(order, edges) {
	const n = order.length;
	const p = new Map(order.map((id, i) => [id, { x: Math.cos((2 * Math.PI * i) / n) * 150, y: Math.sin((2 * Math.PI * i) / n) * 150 }]));
	if (n === 1) p.get(order[0]).x = p.get(order[0]).y = 0;
	const rest = (s) => 120 + Math.min(1, Math.max(0, (-45 - s) / 45)) * 180;
	const pts = [...p.values()];
	for (let it = 0, step = 10; it < 400; it++, step *= 0.988) {
		const f = new Map(pts.map((q) => [q, { x: 0, y: 0 }]));
		for (let i = 0; i < n; i++)
			for (let j = i + 1; j < n; j++) {
				const a = pts[i], b = pts[j];
				const dx = a.x - b.x, dy = a.y - b.y, d2 = Math.max(dx * dx + dy * dy, 1), d = Math.sqrt(d2);
				const push = 30000 / d2;
				f.get(a).x += (push * dx) / d; f.get(a).y += (push * dy) / d;
				f.get(b).x -= (push * dx) / d; f.get(b).y -= (push * dy) / d;
			}
		for (const e of edges) {
			const a = p.get(e.a), b = p.get(e.b);
			const dx = b.x - a.x, dy = b.y - a.y, d = Math.max(Math.hypot(dx, dy), 1);
			const pull = (d - rest(e.weaker)) * 0.08;
			f.get(a).x += (pull * dx) / d; f.get(a).y += (pull * dy) / d;
			f.get(b).x -= (pull * dx) / d; f.get(b).y -= (pull * dy) / d;
		}
		// A node near a line it isn't on is pushed off it, and the line the
		// other way.
		for (const e of edges) {
			const a = p.get(e.a), b = p.get(e.b);
			const dx = b.x - a.x, dy = b.y - a.y, l2 = dx * dx + dy * dy || 1;
			for (const q of pts) {
				if (q === a || q === b) continue;
				const s = Math.max(0, Math.min(1, ((q.x - a.x) * dx + (q.y - a.y) * dy) / l2));
				let ox = q.x - a.x - s * dx, oy = q.y - a.y - s * dy, d = Math.hypot(ox, oy);
				if (d >= 70) continue;
				if (d < 0.5) { ox = -dy; oy = dx; d = Math.hypot(ox, oy) || 1; }
				const push = (70 - d) * 0.6;
				f.get(q).x += (push * ox) / d; f.get(q).y += (push * oy) / d;
				f.get(a).x -= (push * ox) / d / 2; f.get(a).y -= (push * oy) / d / 2;
				f.get(b).x -= (push * ox) / d / 2; f.get(b).y -= (push * oy) / d / 2;
			}
		}
		for (const [q, v] of f) {
			const m = Math.hypot(v.x, v.y);
			if (m > 0) { q.x += (v.x / m) * Math.min(m, step); q.y += (v.y / m) * Math.min(m, step); }
		}
	}
	// No two nodes closer than GAP, so their names don't run together.
	const GAP = 120;
	for (let it = 0; it < 60; it++)
		for (let i = 0; i < n; i++)
			for (let j = i + 1; j < n; j++) {
				const a = pts[i], b = pts[j];
				const dx = a.x - b.x, dy = a.y - b.y, d = Math.max(Math.hypot(dx, dy), 0.01);
				if (d >= GAP) continue;
				const m = (GAP - d) / 2;
				a.x += (dx / d) * m; a.y += (dy / d) * m;
				b.x -= (dx / d) * m; b.y -= (dy / d) * m;
			}
	return p;
}

// tangles counts what makes a layout hard to read: lines that cross, and
// lines that pass close by a node they don't join, which count more.
function tangles(p, edges) {
	const cross = (a, b, c, d) => {
		const o = (p1, p2, p3) => Math.sign((p2.x - p1.x) * (p3.y - p1.y) - (p2.y - p1.y) * (p3.x - p1.x));
		return o(a, b, c) * o(a, b, d) < 0 && o(c, d, a) * o(c, d, b) < 0;
	};
	const near = (q, a, b) => {
		const dx = b.x - a.x, dy = b.y - a.y, l2 = dx * dx + dy * dy || 1;
		const t = Math.max(0, Math.min(1, ((q.x - a.x) * dx + (q.y - a.y) * dy) / l2));
		return Math.hypot(q.x - a.x - t * dx, q.y - a.y - t * dy) < 40;
	};
	let n = 0;
	for (let i = 0; i < edges.length; i++) {
		const a = p.get(edges[i].a), b = p.get(edges[i].b);
		for (let j = i + 1; j < edges.length; j++) {
			const e = edges[j];
			if ([e.a, e.b].some((x) => x === edges[i].a || x === edges[i].b)) continue;
			if (cross(a, b, p.get(e.a), p.get(e.b))) n += 1;
		}
		for (const [id, q] of p)
			if (id !== edges[i].a && id !== edges[i].b && near(q, a, b)) n += 3;
	}
	return n;
}

// layout tries the nodes in a few orders and keeps the least tangled; turns
// it so it runs across, the drawing being wider than tall; and fits it to
// a drawing width wide and at most tallest tall, spread or drawn in to
// fill it, and as tall as it then needs. The same reports always give the
// same map at the same size. It returns the places and the height.
function layout(ids, edges, width, tallest) {
	// An order is the IDs sorted by a hash of each, mixed with the try's
	// number.
	const hash = (s) => [...s].reduce((x, ch) => (Math.imul(x, 31) + ch.charCodeAt(0)) | 0, 7);
	const mix = (x) => {
		x = Math.imul(x ^ (x >>> 16), 0x7feb352d);
		x = Math.imul(x ^ (x >>> 15), 0x846ca68b);
		return (x ^ (x >>> 16)) >>> 0;
	};
	let best = null, least = Infinity;
	for (let k = 0; k < (ids.length > 10 ? 6 : 24) && least > 0; k++) {
		const key = (id) => mix(hash(id) ^ Math.imul(k, 0x9e3779b9));
		const order = k === 0 ? ids : [...ids].sort((a, b) => key(a) - key(b));
		const p = place(order, edges);
		const n = tangles(p, edges);
		if (n < least) { best = p; least = n; }
	}
	const pts = ids.map((id) => best.get(id));
	// Turn the long axis across.
	const mx = pts.reduce((s, q) => s + q.x, 0) / pts.length, my = pts.reduce((s, q) => s + q.y, 0) / pts.length;
	let sxx = 0, syy = 0, sxy = 0;
	for (const q of pts) { sxx += (q.x - mx) ** 2; syy += (q.y - my) ** 2; sxy += (q.x - mx) * (q.y - my); }
	const th = 0.5 * Math.atan2(2 * sxy, sxx - syy), c = Math.cos(-th), s = Math.sin(-th);
	for (const q of pts) {
		const x = q.x - mx, y = q.y - my;
		q.x = x * c - y * s;
		q.y = x * s + y * c;
	}
	const xs = pts.map((q) => q.x), ys = pts.map((q) => q.y);
	const [x0, x1, y0, y1] = [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)];
	// Spread to fill it, across more than down, or down more than across, by
	// at most STRETCH, so the map keeps something of its shape.
	let kx = Math.min(SPREAD, (width - 160) / Math.max(x1 - x0, 1));
	let ky = Math.min(SPREAD, (tallest - 120) / Math.max(y1 - y0, 1));
	[kx, ky] = [Math.min(kx, ky * STRETCH), Math.min(ky, kx * STRETCH)];
	const height = Math.max(MIN_H, (y1 - y0) * ky + 120);
	for (const q of pts) {
		q.x = width / 2 + (q.x - (x0 + x1) / 2) * kx;
		q.y = height / 2 - 12 + (q.y - (y0 + y1) / 2) * ky;
	}
	return { pos: best, height };
}

// onBand says whether an AP's report has a radio up on a band. An AP that
// has reported no radios yet is not known to be off it.
function onBand(report, band) {
	const radios = report?.radios || [];
	return !radios.length || radios.some((r) => r.band === band && r.up !== false);
}

// hellos are the hellos that go along the lines, as the APs here last
// reported them (0346): from the neighbour that sends them to the AP that
// hears them, and when the last one came, in ms. Hellos come only where
// they go both ways or one way, and not once they are three late.
function hellos(rows, edges) {
	const pair = (x, y) => [x, y].sort().join(' ');
	const on = new Set(edges.map((e) => pair(e.a, e.b)));
	const out = [];
	for (const { ap, cfg } of rows) {
		const at = Date.parse(cfg?.condition?.state?.at ?? '');
		if (!Number.isFinite(at)) continue;
		for (const n of cfg?.condition?.state?.report?.rrm?.neighbours || []) {
			if (!['up', 'one-way'].includes(n.state) || !Number.isFinite(n.hello_ago) || n.hello_ago * 1000 > 3 * HELLO_EVERY) continue;
			if (on.has(pair(ap.id, n.ap))) out.push({ from: n.ap, to: ap.id, last: at - n.hello_ago * 1000 });
		}
	}
	return out;
}

// going draws the hellos going along the lines into layer (0346): each
// from where its sender is to where the AP that hears it is, wherever they
// have been moved to, landing every ten seconds from its last, with a ring.
// It stops once the map is gone, and draws nothing for someone who asks
// their browser for less motion.
function going(el, layer, list, pos) {
	if (!list.length || globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
	// Shown or hidden, touched only as that changes.
	const shown = (x, yes) => {
		if (x.on === yes) return;
		x.on = yes;
		x.setAttribute('visibility', yes ? 'visible' : 'hidden');
	};
	const drawn = list.map((x) => {
		const dot = svg('circle', { r: 4, class: 'ping', visibility: 'hidden' });
		const ring = svg('circle', { r: R, class: 'ring', visibility: 'hidden' });
		dot.on = ring.on = false;
		layer.append(dot, ring);
		return { ...x, dot, ring };
	});
	const ease = (f) => (f < 0.5 ? 2 * f * f : 1 - (2 - 2 * f) ** 2 / 2);
	const frame = () => {
		if (!el.isConnected) return;
		const now = Date.now();
		for (const x of drawn) {
			const a = pos.get(x.from), b = pos.get(x.to);
			const since = (((now - x.last) % HELLO_EVERY) + HELLO_EVERY) % HELLO_EVERY;
			const left = HELLO_EVERY - since;
			if (left <= TRAVEL) {
				// From the edge of one circle to the edge of the other.
				const dx = b.x - a.x, dy = b.y - a.y, d = Math.hypot(dx, dy) || 1, gap = Math.min(R, d / 2);
				const along = gap + (d - 2 * gap) * ease(1 - left / TRAVEL);
				x.dot.setAttribute('cx', a.x + (dx / d) * along);
				x.dot.setAttribute('cy', a.y + (dy / d) * along);
				shown(x.dot, true);
			} else shown(x.dot, false);
			if (since < LAND) {
				x.ring.setAttribute('cx', b.x);
				x.ring.setAttribute('cy', b.y);
				x.ring.setAttribute('r', R + 2 + (12 * since) / LAND);
				x.ring.setAttribute('opacity', 1 - since / LAND);
				shown(x.ring, true);
			} else shown(x.ring, false);
		}
		requestAnimationFrame(frame);
	};
	requestAnimationFrame(frame);
}

// draw draws the map for one band: the APs here that broadcast on it, and
// any other a line on that band runs to. An AP with no radio up on the band
// has no place on its map (Griff, 2026-10-10: the 6 GHz map showed the APs
// without 6 GHz). Where this browser keeps an AP was moved to, under where,
// it is drawn there (0346); again draws the map afresh. It is drawn width
// wide (0346). It returns the drawing, and the tools for the legend's row.
function draw(ctx, rows, band, gone, where, again, width) {
	const here = new Map(rows.map(({ ap, cfg }) => [ap.id, { ap, report: cfg?.condition?.state?.report }]));
	const edges = links(rows, gone).filter((l) => l.band === band);
	const on = [...here.keys()].filter((id) => onBand(here.get(id).report, band));
	const ids = [...new Set([...on, ...edges.flatMap((e) => [e.a, e.b])])].sort();
	if (!ids.length) return { parts: h('div', { class: 'sub' }, here.size ? `No AP here broadcasts on ${bandName(band)}.` : 'No APs here yet.') };
	const tallest = Math.max(MIN_H, Math.round((globalThis.innerHeight || 800) * TALL));
	const { pos } = layout(ids, edges, width, tallest);
	const name = (id) => here.get(id)?.ap.name || ctx.name('locations', id);

	// Where APs were moved to: each is held in the drawing, as far down as
	// DEEPER times its share of the window.
	const ok = (v) => v && typeof v === 'object' && !Array.isArray(v);
	let moved = kept(MOVED + where, null);
	if (!ok(moved)) {
		const old = kept(OLD + where, null);
		moved = {};
		if (ok(old)) for (const [id, m] of Object.entries(old)) if (Array.isArray(m)) moved[id] = [m[0] / OLD_W, m[1] / OLD_H];
	}
	const movedTo = (id) => {
		const m = moved[id];
		return Array.isArray(m) && m.length === 2 && m.every(Number.isFinite) ? [m[0] * width, m[1] * tallest] : null;
	};
	const fit = (x, y) => ({ x: Math.max(SIDE, Math.min(width - SIDE, x)), y: Math.max(R + 4, Math.min(DEEPER * tallest - BELOW, y)) });
	for (const id of ids) if (movedTo(id)) Object.assign(pos.get(id), fit(...movedTo(id)));

	const lines = edges.map((e) => {
		const way = (x, y) => (e.hears[x] != null ? `${name(x)} hears ${name(y)} at ${e.hears[x]} dBm` : `${name(x)} doesn't say how it hears ${name(y)}`);
		const both = e.hears[e.a] != null && e.hears[e.b] != null;
		const line = svg('line');
		const text = svg('text', { 'text-anchor': 'middle' }, both && e.hears[e.a] !== e.hears[e.b] ? `${e.hears[e.a]} / ${e.hears[e.b]}` : `${e.weaker}`);
		const g = svg('g', { class: `edge ${classOf(e.weaker)}${e.up ? '' : ' heard'}` },
			svg('title', null, `${way(e.a, e.b)}; ${way(e.b, e.a)}; ${e.up ? 'hellos go both ways' : 'not neighbours both ways'}`), line, text);
		return { e, line, text, g };
	});
	// put draws a line between where its ends are now, and its label beside
	// its middle, on its upper side.
	const put = ({ e, line, text }) => {
		const a = pos.get(e.a), b = pos.get(e.b);
		for (const [k, v] of Object.entries({ x1: a.x, y1: a.y, x2: b.x, y2: b.y })) line.setAttribute(k, v);
		const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
		let nx = -(b.y - a.y) / len, ny = (b.x - a.x) / len;
		if (ny > 0) { nx = -nx; ny = -ny; }
		text.setAttribute('x', (a.x + b.x) / 2 + nx * 10);
		text.setAttribute('y', (a.y + b.y) / 2 + ny * 10 + 4);
	};
	lines.forEach(put);

	const nodes = new Map(ids.map((id) => {
		const at = here.get(id);
		const radio = (at?.report?.radios || []).find((r) => r.band === band);
		const held = (at?.report?.rrm?.apc || []).find((x) => x.band === band && x.power != null);
		const facts = [radio?.channel && `ch ${radio.channel}`, held ? `${held.power} dBm` : null].filter(Boolean).join(' · ');
		return [id, svg('g', { class: `node${at ? '' : ' away'}`, tabindex: 0, role: 'button',
			'aria-label': `${name(id)}${facts ? `, ${facts}` : ''}${at ? '' : ', not in this folder'}: drag it, or move it with the arrow keys` },
		svg('title', null, at ? `${name(id)}${facts ? `: ${facts}` : ''}` : `${name(id)}: not in this folder`),
		svg('circle', { cx: 0, cy: 0, r: R }),
		svg('path', { d: 'M-6 1a8 8 0 0 1 12 0M-9.5 -3a13 13 0 0 1 19 0', class: 'waves' }),
		svg('circle', { cx: 0, cy: 5, r: 1.8, class: 'dot' }),
		svg('text', { x: 0, y: R + 15, 'text-anchor': 'middle', class: 'name' }, name(id)),
		facts && svg('text', { x: 0, y: R + 29, 'text-anchor': 'middle', class: 'facts' }, facts))];
	}));
	const placeNode = (id) => nodes.get(id).setAttribute('transform', `translate(${pos.get(id).x} ${pos.get(id).y})`);
	ids.forEach(placeNode);

	const layer = svg('g', { class: 'hellos', 'aria-hidden': 'true' });
	const el = svg('svg', { class: 'nmap', role: 'group',
		'aria-label': `${bandName(band)} neighbours: ${edges.map((e) => `${name(e.a)} and ${name(e.b)} at ${e.weaker} dBm`).join('; ') || 'none'}` },
	lines.map((l) => l.g), layer, [...nodes.values()]);

	// The drawing shows the band the APs are in: from ABOVE the highest to
	// below the name of the lowest (Griff, 2026-11-10: "can we have the
	// screen dynamically shrink with the AP used area?"). It is fitted to
	// them when drawn and as an AP is put down; while one is dragged it only
	// grows, so the map does not move under the pointer.
	let top = 0, bottom = 0;
	const frame = (grow) => {
		const ys = ids.map((id) => pos.get(id).y);
		let t = Math.min(...ys) - ABOVE, b = Math.max(...ys) + BELOW;
		if (grow) [t, b] = [Math.min(t, top), Math.max(b, bottom)];
		if (t === top && b === bottom) return;
		[top, bottom] = [t, b];
		el.setAttribute('viewBox', `0 ${Math.round(top)} ${width} ${Math.round(bottom - top)}`);
	};
	frame(false);

	// Moving an AP: its lines go with it, and once it is put down this
	// browser keeps where.
	const reset = h('button', { type: 'button', class: 'button small', title: 'Forget where the APs were moved to on this map, in this browser, and lay it out again',
		onclick: () => { keep(MOVED + where, null); keep(OLD + where, null); again(); } }, 'Lay out again');
	reset.hidden = !ids.some(movedTo);
	const move = (id, x, y, dragged) => {
		Object.assign(pos.get(id), fit(x, y));
		placeNode(id);
		for (const l of lines) if (l.e.a === id || l.e.b === id) put(l);
		frame(dragged);
	};
	const putDown = (id) => {
		const frac = (v, of) => Math.round((v / of) * 10000) / 10000;
		moved[id] = [frac(pos.get(id).x, width), frac(pos.get(id).y, tallest)];
		keep(MOVED + where, moved);
		keep(OLD + where, null);
		reset.hidden = false;
	};
	const STEP = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
	for (const [id, g] of nodes) {
		g.addEventListener('pointerdown', (ev) => {
			if (ev.button !== 0 || !el.getScreenCTM()) return;
			ev.preventDefault();
			// Where the pointer is in the drawing, as it grows or the page
			// scrolls under it.
			const pt = (e) => new DOMPoint(e.clientX, e.clientY).matrixTransform(el.getScreenCTM().inverse());
			const from = pt(ev), was = { ...pos.get(id) };
			g.setPointerCapture(ev.pointerId);
			g.classList.add('dragging');
			// The page is not drawn again under someone moving an AP.
			el.setAttribute('data-editing', '');
			const drag = (e) => {
				const p = pt(e);
				move(id, was.x + p.x - from.x, was.y + p.y - from.y, true);
			};
			const drop = () => {
				g.removeEventListener('pointermove', drag);
				g.removeEventListener('pointerup', drop);
				g.removeEventListener('pointercancel', drop);
				g.classList.remove('dragging');
				el.removeAttribute('data-editing');
				if (pos.get(id).x !== was.x || pos.get(id).y !== was.y) putDown(id);
				frame(false);
			};
			g.addEventListener('pointermove', drag);
			g.addEventListener('pointerup', drop);
			g.addEventListener('pointercancel', drop);
		});
		g.addEventListener('keydown', (ev) => {
			const d = STEP[ev.key];
			if (!d) return;
			ev.preventDefault();
			const s = ev.shiftKey ? 40 : 10;
			move(id, pos.get(id).x + d[0] * s, pos.get(id).y + d[1] * s);
			putDown(id);
		});
	}

	const shown = hellosShown();
	if (shown) going(el, layer, hellos(rows, edges), pos);
	const helloSwitch = h('button', { type: 'button', class: `button small${shown ? ' on' : ''}`, 'aria-pressed': String(shown),
		title: 'Show the hellos the APs send each other, every ten seconds, timed from their reports',
		onclick: () => { keep(HELLOS, !shown); again(); } }, 'Hellos');
	return {
		parts: [el,
			!edges.length && h('div', { class: 'sub' }, ids.length > 1 ? `No two APs here hear each other on ${bandName(band)}.` : `One AP here broadcasts on ${bandName(band)}.`)],
		tools: [shown && edges.length > 0 && h('span', { class: 'key hello' }, 'a hello landing'),
			h('span', { class: 'hint' }, 'drag an AP to move it'), reset, edges.length > 0 && helloSwitch],
	};
}

// neighbourMap is the map's panel, with a switch between the bands, 2.4 GHz
// first, where APs hear each other furthest. folder is the node whose page
// it is on, by which where APs were moved to is kept (0346).
export function neighbourMap(ctx, all, band, folder) {
	// The APs here that are offline: off the map, named under it.
	const away = all.filter(({ cfg }) => cfg && quiet(cfg.condition?.seen?.at));
	// The neighbours the Org has not adopted: off the map too, those waiting
	// in Landing Zone named under it.
	const t = ctx.trees.locations;
	const waiting = (id) => {
		for (let n = t.nodes.get(id); n; n = t.nodes.get(n.parent)) if (n.isolated) return true;
		return false;
	};
	const here = new Set(all.map(({ ap }) => ap.id));
	const strangers = [...new Set(all.flatMap(({ cfg }) => (cfg?.condition?.state?.report?.rrm?.neighbours || []).map((n) => n.ap)))]
		.filter((id) => !here.has(id) && (!t.nodes.has(id) || waiting(id)));
	const gone = new Set([...away.map(({ ap }) => ap.id), ...strangers]);
	const rows = all.filter(({ ap }) => !gone.has(ap.id));
	const unadopted = strangers.filter(waiting).map((id) => t.nodes.get(id).name);
	const bands = ['2g', '5g', '6g'].filter((b) => links(rows, gone).some((l) => l.band === b) ||
		rows.some(({ cfg }) => (cfg?.condition?.state?.report?.radios || []).some((r) => r.band === b)));
	if (!bands.length) return null;
	const body = h('div', { class: 'nmapbox' });
	const tools = h('span', { class: 'nmaptools' });
	const tabs = band ? null : h('span', { class: 'controls seg' });
	// The width inside the map's box, once it is on the page.
	const inner = () => {
		if (!body.isConnected) return 0;
		const cs = getComputedStyle(body);
		return Math.floor(body.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight));
	};
	// Before the box is on the page, the width it will have: the main
	// column's, less the panel's borders and the box's padding. Measured
	// once it is on, and drawn again if that is not it.
	const guess = () => {
		const m = document.querySelector('main');
		if (!m) return 0;
		const cs = getComputedStyle(m);
		return Math.floor(m.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight) - 26);
	};
	let shown = band || bands[0], drawnAt = 0;
	const show = (b0) => {
		shown = b0;
		tabs?.replaceChildren(...bands.map((b) => h('button', { type: 'button', class: `button small${b === b0 ? ' on' : ''}`, 'aria-pressed': String(b === b0),
			onclick: () => show(b) }, bandName(b))));
		drawnAt = Math.max(MIN_W, inner() || lastWidth || guess() || 640);
		const { parts, tools: more } = draw(ctx, rows, b0, gone, `${folder || 'all'}.${b0}`, () => show(b0), drawnAt);
		body.replaceChildren(...[parts].flat().filter(Boolean));
		tools.replaceChildren(...[more].flat().filter(Boolean));
	};
	show(shown);
	// Drawn again, in the next frame, as its box changes width; not under
	// someone moving an AP. Watched until it is gone from the page, having
	// been on it: a page is built before it is put on.
	if (typeof ResizeObserver === 'function') {
		let due = false, on = false;
		const watch = new ResizeObserver(() => {
			if (!body.isConnected) {
				if (on) watch.disconnect();
				return;
			}
			on = true;
			const w = inner();
			if (w < MIN_W / 2) return;
			lastWidth = w;
			if (due || Math.abs(Math.max(MIN_W, w) - drawnAt) < 8 || body.querySelector('.dragging')) return;
			due = true;
			requestAnimationFrame(() => {
				due = false;
				show(shown);
			});
		});
		watch.observe(body);
		watchers.set(body, watch);
	}
	return h('section', { class: 'panel' },
		h('h2', null, 'Neighbour map', h('span', { class: 'note' }, 'how strongly each pair hears each other, the weaker way'), tabs),
		body,
		away.length > 0 && h('div', { class: 'sub' }, `Offline, so not on the map: ${away.map(({ ap }) => ap.name).join(', ')}.`),
		unadopted.length > 0 && h('div', { class: 'sub' }, `Waiting in Landing Zone, so not on the map: ${unadopted.join(', ')}.`),
		h('div', { class: 'nmaplegend' },
			CLASSES.map(([c, , text]) => h('span', { class: `key ${c}` }, text)),
			h('span', { class: 'key heard' }, 'heard, no hellos both ways'),
			tools));
}
