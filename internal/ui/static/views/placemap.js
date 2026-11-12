// A folder's map (0169), at the head of its System tab: where the folder is,
// and each folder and AP below it, on OpenStreetMap's map, whose tiles the
// manager fetches. A folder or AP with no place of its own is drawn around
// the one it takes its place from. A pin says what it is, coloured as the
// tree's dot is, and opens it. Someone who may change the folder or AP puts
// it on the map: by its address, suggested as it is typed or found; by a
// click where it is; or by its latitude and longitude, a draft of the System
// tab's one save (0202). The page drawn again keeps the map as it was left.

import { h } from '../dom.js';
import { get, requestHeaders } from '../api.js';
import { fleetMap } from '../layout.js';

const TILE = 256, MIN_Z = 2, MAX_Z = 19, HEIGHT = 380;
const KEEP = 120;          // tiles kept drawn, the latest used
const RETRY = 15000;       // ms before a tile that did not come is asked again
const WHEEL = 100;         // px of wheel to a zoom level

// Web Mercator: a place's pixel on the whole world's map at zoom z, and back.
function px(lat, lon, z) {
	const s = TILE * 2 ** z;
	const r = Math.max(-85.0511, Math.min(85.0511, lat)) * Math.PI / 180;
	return [(lon + 180) / 360 * s, (1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2 * s];
}
function ll(x, y, z) {
	const s = TILE * 2 ** z;
	const n = Math.PI - 2 * Math.PI * y / s;
	return [180 / Math.PI * Math.atan(Math.sinh(n)), x / s * 360 - 180];
}
// lonOf brings a longitude to -180 to 180, wherever round the world it is.
const lonOf = (lon) => ((((lon + 180) % 360) + 360) % 360) - 180;

// The tiles, by z/x/y: { bm, waiting, retry }. The page asks the manager,
// with its token, as an image tag cannot. A tile on its way tells each map
// that asked for it; one that did not come is asked again a while later.
const tiles = new Map();
function tile(z, x, y, ready) {
	const key = `${z}/${x}/${y}`;
	let t = tiles.get(key);
	if (t?.bm) {
		tiles.delete(key);
		tiles.set(key, t);
		return t.bm;
	}
	if (t?.waiting) {
		t.waiting.add(ready);
		return null;
	}
	if (t && Date.now() < t.retry) return null;
	t = { bm: null, waiting: new Set([ready]), retry: 0 };
	tiles.set(key, t);
	for (const [k, old] of tiles) {
		if (tiles.size <= KEEP) break;
		if (old.waiting) continue;
		old.bm?.close();
		tiles.delete(k);
	}
	const tell = () => { const w = t.waiting; t.waiting = null; for (const f of w) f(); };
	fetch(`/v1/map/tiles/${key}`, { headers: requestHeaders() })
		.then((r) => (r.ok ? r.blob() : Promise.reject(new Error(String(r.status)))))
		.then((b) => createImageBitmap(b))
		.then((bm) => {
			if (tiles.get(key) !== t) return bm.close();
			t.bm = bm;
			tell();
		})
		.catch(() => {
			t.retry = Date.now() + RETRY;
			const w = t.waiting;
			t.waiting = null;
			setTimeout(() => { for (const f of w) f(); }, RETRY);
		});
	return null;
}

// How each map was left, by the node it is of: { z, cx, cy, picked }.
const left = new Map();

const google = (lat, lon) => `https://www.google.com/maps/search/?api=1&query=${lat.toFixed(6)},${lon.toFixed(6)}`;
const at = (lat, lon) => `${lat.toFixed(5)}, ${lon.toFixed(5)}`;

// slippy is the map: pins ([{ lat, lon, kind, state, title, key, dx, dy }])
// drawn on it; onPin(key) when one is pressed, onSpot(lat, lon) when the
// map is clicked; view, where it was left, kept as it moves.
function slippy(onPin, onSpot, view) {
	const canvas = h('canvas', { class: 'mapcanvas', 'aria-hidden': 'true' });
	const layer = h('div', { class: 'mappins' });
	const zoomBy = (d) => () => { zoomAt(d, wrap.clientWidth / 2, HEIGHT / 2); };
	const wrap = h('div', { class: 'placemap', style: { height: `${HEIGHT}px` } },
		canvas, layer,
		h('div', { class: 'mapzoom' },
			h('button', { type: 'button', title: 'Closer', 'aria-label': 'Zoom in', onclick: zoomBy(1) }, '+'),
			h('button', { type: 'button', title: 'Farther', 'aria-label': 'Zoom out', onclick: zoomBy(-1) }, '−')),
		h('div', { class: 'mapcredit' }, '© OpenStreetMap contributors'));
	let pins = [], queued = false, aim = false;

	// The middle kept on the map: round the world, and not past its top or
	// bottom.
	const hold = () => {
		const s = TILE * 2 ** view.z;
		view.cx = ((view.cx % s) + s) % s;
		view.cy = Math.max(0, Math.min(s, view.cy));
	};
	const draw = () => {
		queued = false;
		const w = wrap.clientWidth;
		if (!w) return;
		const { z, cx, cy } = view;
		const dpr = window.devicePixelRatio || 1;
		if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(HEIGHT * dpr)) {
			canvas.width = Math.round(w * dpr);
			canvas.height = Math.round(HEIGHT * dpr);
			canvas.style.width = `${w}px`;
			canvas.style.height = `${HEIGHT}px`;
		}
		const g = canvas.getContext('2d');
		g.setTransform(dpr, 0, 0, dpr, 0, 0);
		// The ground under the tiles, as the page's theme has it (0296).
		g.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--map-bg').trim() || '#e8eef2';
		g.fillRect(0, 0, w, HEIGHT);
		const x0 = cx - w / 2, y0 = cy - HEIGHT / 2, n = 2 ** z, s = TILE * n;
		for (let ty = Math.floor(y0 / TILE); ty * TILE < y0 + HEIGHT; ty++) {
			if (ty < 0 || ty >= n) continue;
			for (let tx = Math.floor(x0 / TILE); tx * TILE < x0 + w; tx++) {
				const bm = tile(z, ((tx % n) + n) % n, ty, redraw);
				if (bm) g.drawImage(bm, tx * TILE - x0, ty * TILE - y0, TILE, TILE);
			}
		}
		layer.replaceChildren(...pins.map((p) => {
			let [x, y] = px(p.lat, p.lon, z);
			// On the copy of the world nearest the middle.
			x += Math.round((cx - x) / s) * s;
			return h('button', {
				type: 'button', class: `pin ${p.kind} ${p.state || ''}`, title: p.title, 'aria-label': p.title,
				style: { left: `${x - x0 + (p.dx || 0)}px`, top: `${y - y0 + (p.dy || 0)}px` },
				onclick: (e) => { e.stopPropagation(); onPin(p.key); },
			});
		}));
	};
	const redraw = () => { if (!queued) { queued = true; requestAnimationFrame(draw); } };
	const zoomAt = (d, ox, oy) => {
		const nz = Math.max(MIN_Z, Math.min(MAX_Z, view.z + d));
		if (nz === view.z) return;
		const w = wrap.clientWidth;
		const [lat, lon] = ll(view.cx - w / 2 + ox, view.cy - HEIGHT / 2 + oy, view.z);
		const [x, y] = px(lat, lon, nz);
		view.z = nz;
		view.cx = x - ox + w / 2;
		view.cy = y - oy + HEIGHT / 2;
		hold();
		redraw();
	};

	// Dragged with the main button to move; wheeled to zoom, a level a
	// notch; a click that did not move is a spot on the map.
	let drag = null, wheel = 0;
	const stop = () => { drag = null; };
	wrap.addEventListener('pointerdown', (e) => {
		if (e.button !== 0 || e.target.closest('button')) return;
		drag = { x: e.clientX, y: e.clientY, cx: view.cx, cy: view.cy, moved: false };
		wrap.setPointerCapture(e.pointerId);
	});
	wrap.addEventListener('pointermove', (e) => {
		if (!drag) return;
		if (!(e.buttons & 1)) return stop();
		const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
		if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true;
		view.cx = drag.cx - dx;
		view.cy = drag.cy - dy;
		redraw();
	});
	wrap.addEventListener('pointerup', (e) => {
		if (drag && !drag.moved && aim) {
			const r = wrap.getBoundingClientRect();
			const [lat, lon] = ll(view.cx - r.width / 2 + e.clientX - r.left, view.cy - HEIGHT / 2 + e.clientY - r.top, view.z);
			onSpot(lat, lonOf(lon));
		}
		if (drag) {
			hold();
			redraw();
		}
		stop();
	});
	wrap.addEventListener('pointercancel', stop);
	wrap.addEventListener('lostpointercapture', stop);
	wrap.addEventListener('wheel', (e) => {
		e.preventDefault();
		if (!e.deltaY) return;
		wheel += e.deltaY * (e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? 400 : 1);
		if (Math.abs(wheel) < WHEEL) return;
		const r = wrap.getBoundingClientRect();
		zoomAt(wheel < 0 ? 1 : -1, e.clientX - r.left, e.clientY - r.top);
		wheel = 0;
	}, { passive: false });
	new ResizeObserver(redraw).observe(wrap);

	return {
		el: wrap,
		pins(list) { pins = list; redraw(); },
		aim(on) { aim = on; wrap.classList.toggle('aiming', on); },
		// fit shows every pin, or the world where there is none.
		fit(list) {
			const w = wrap.clientWidth || 640;
			if (!list.length) {
				view.z = MIN_Z;
				[view.cx, view.cy] = px(30, -40, view.z);
				redraw();
				return;
			}
			let z = 16;
			for (; z > MIN_Z; z--) {
				const xs = list.map((p) => px(p.lat, p.lon, z));
				const [x0, x1] = [Math.min(...xs.map((p) => p[0])), Math.max(...xs.map((p) => p[0]))];
				const [y0, y1] = [Math.min(...xs.map((p) => p[1])), Math.max(...xs.map((p) => p[1]))];
				if (x1 - x0 < w - 80 && y1 - y0 < HEIGHT - 80) break;
			}
			const xs = list.map((p) => px(p.lat, p.lon, z));
			view.z = z;
			view.cx = (Math.min(...xs.map((p) => p[0])) + Math.max(...xs.map((p) => p[0]))) / 2;
			view.cy = (Math.min(...xs.map((p) => p[1])) + Math.max(...xs.map((p) => p[1]))) / 2;
			redraw();
		},
		go(lat, lon, zoom) { view.z = zoom ?? Math.max(view.z, 15); [view.cx, view.cy] = px(lat, lon, view.z); redraw(); },
		// center is where the map looks: its middle, and how close.
		center() { const [lat, lon] = ll(view.cx, view.cy, view.z); return { lat, lon: lonOf(lon), z: view.z }; },
	};
}

// pinsOf is the places as pins: each folder and AP with a place of its own
// where it is; one that takes its place from a folder above around that
// place, so each is seen and none hides another. status is each AP's, as
// the tree grades its dot.
function pinsOf(places, status, here, pending) {
	const out = [];
	const around = new Map();
	for (const p of places) {
		if (p.latitude == null) continue;
		if (!p.own) {
			const k = `${p.latitude},${p.longitude}`;
			around.set(k, [...(around.get(k) || []), p]);
			continue;
		}
		out.push(pin(p, status, here));
	}
	for (const list of around.values()) {
		list.forEach((p, i) => {
			const a = 2 * Math.PI * i / list.length - Math.PI / 2;
			out.push({ ...pin(p, status, here), dx: 20 * Math.cos(a), dy: 20 * Math.sin(a) });
		});
	}
	if (pending) out.push({ lat: pending.lat, lon: pending.lon, kind: 'pending', key: '', title: `Here: ${at(pending.lat, pending.lon)}` });
	return out;
}

function pin(p, status, here) {
	const st = p.kind === 'ap' ? status.get(p.node) : null;
	return {
		lat: p.latitude, lon: p.longitude, key: p.node, kind: p.kind === 'ap' ? 'ap' : 'folder',
		state: [st?.cls || '', p.node === here ? 'here' : ''].join(' '),
		title: `${p.name}${st ? `, ${st.label.toLowerCase()}` : ''}${p.address ? `: ${p.address}` : ''}`,
	};
}

// placeMap is the map for here, a Locations node: a folder's, of it and
// all below it; an AP's, of its folder's, with it marked. edit says the
// caller may change here; save is the page's save, where its place is a
// draft.
export function placeMap(ctx, here, page, edit, save) {
	const isAP = page.node.kind === 'ap';
	const under = isAP && page.node.parent ? page.node.parent : here;
	const kept = left.get(here);
	const view = kept ? { ...kept } : { z: 3, cx: 0, cy: 0, picked: null };
	const panel = h('section', { class: 'panel placepanel' });
	const info = h('div', { class: 'mapinfo' });
	let places = [], status = new Map(), pending = null;
	// Its place as set, which the draft starts from: the address in force,
	// and its latitude and longitude where they are its own.
	const was = (k) => page.fields?.[`system.place.${k}`];
	const own = was('latitude')?.from === here && was('longitude')?.from === here;
	const addrWas = was('address')?.value ?? '';
	const keep = () => left.set(here, view);
	const map = slippy((key) => { view.picked = key; keep(); show(); }, (lat, lon) => { pending = { lat, lon }; sync(); }, view);

	const show = () => {
		keep();
		map.pins(pinsOf(places, status, here, pending));
		const p = places.find((x) => x.node === view.picked);
		const st = p && status.get(p.node);
		info.replaceChildren(...(p && p.latitude != null ? [
			h('a', { href: p.kind === 'ap' ? `#/aps/${encodeURIComponent(p.node)}/overview` : `#/locations/${encodeURIComponent(p.node)}/settings` }, p.name),
			st && h('span', { class: `chip ${st.chip}`, title: st.detail }, st.label),
			p.address && h('span', null, p.address),
			h('span', { class: 'mono sub' }, at(p.latitude, p.longitude)),
			!p.own && h('span', { class: 'sub' }, `drawn by ${ctx.name('locations', p.parent)}`),
			h('a', { href: google(p.latitude, p.longitude), target: '_blank', rel: 'noopener noreferrer' }, 'Open in Google Maps'),
		] : [h('span', { class: 'sub' }, places.some((x) => x.latitude != null) ? 'Press a pin to see what it is.' : 'Nothing here is on the map yet.')]).filter(Boolean));
	};

	// Putting here on the map. While a spot is picked, or the map is set to
	// take one, the page is not drawn again under it.
	const addr = h('input', { type: 'text', placeholder: 'Street address', maxlength: 256, 'aria-label': 'Address', autocomplete: 'off',
		role: 'combobox', 'aria-autocomplete': 'list', 'aria-expanded': 'false', 'aria-controls': `suggest-${here}` });
	const lat = h('input', { type: 'number', step: 'any', min: -90, max: 90, placeholder: 'Latitude', 'aria-label': 'Latitude', class: 'num' });
	const lon = h('input', { type: 'number', step: 'any', min: -180, max: 180, placeholder: 'Longitude', 'aria-label': 'Longitude', class: 'num' });
	const found = h('div', { class: 'mapfound' });
	const msg = h('div', { class: 'error' });
	const editor = edit ? h('div', { class: 'mapedit' }) : null;
	const busy = () => editor?.toggleAttribute('data-editing', pending !== null || aim.checked);
	const aim = h('input', { type: 'checkbox', class: 'switch', role: 'switch', 'aria-label': 'Put it where the map is clicked', onchange: () => { map.aim(aim.checked); busy(); } });
	if (own) {
		lat.value = was('latitude').value;
		lon.value = was('longitude').value;
	}
	addr.value = addrWas;

	// The draft: a spot, where it is not the one it has; and an address, once
	// it is not the one in force. An address needs a spot, unless it has one.
	const spot = () => pending && !(own && +pending.lat.toFixed(6) === was('latitude').value && +pending.lon.toFixed(6) === was('longitude').value);
	const at6 = () => ({ 'system.place.latitude': +pending.lat.toFixed(6), 'system.place.longitude': +pending.lon.toFixed(6) });
	const changes = () => {
		const out = spot() ? at6() : {};
		const a = addr.value.trim();
		if (a && a !== addrWas) out['system.place.address'] = a;
		return out;
	};
	const blocked = () => 'system.place.address' in changes() && !spot() && !own;
	const check = () => msg.replaceChildren(blocked() ? 'Find its address, click where it is, or say its latitude and longitude.' : '');
	const part = {
		changes,
		blocked,
		dirty: () => Object.keys(changes()).length > 0,
		undo: () => {
			pending = null;
			addr.value = addrWas;
			lat.value = own ? was('latitude').value : '';
			lon.value = own ? was('longitude').value : '';
			aim.checked = false;
			map.aim(false);
			found.replaceChildren();
			msg.replaceChildren();
			busy();
			show();
		},
		// The place as the System editor sets it: a latitude without its
		// longitude goes with the one in force.
		take: (values) => {
			const [kLat, kLon, kAddr] = ['system.place.latitude', 'system.place.longitude', 'system.place.address'];
			if (kAddr in values) addr.value = values[kAddr] ?? '';
			if (kLat in values || kLon in values) {
				if (kLat in values) lat.value = values[kLat] ?? was('latitude')?.value ?? '';
				if (kLon in values) lon.value = values[kLon] ?? was('longitude')?.value ?? '';
				if (lat.value === '') lat.value = was('latitude')?.value ?? '';
				if (lon.value === '') lon.value = was('longitude')?.value ?? '';
				pending = read();
				busy();
				show();
			}
			check();
		},
	};
	// told tells the page the draft changed: values are what was just set.
	const told = (values) => {
		check();
		save?.changed(part, values);
	};
	const sync = (withAddress) => {
		if (pending) {
			lat.value = pending.lat.toFixed(6);
			lon.value = pending.lon.toFixed(6);
		}
		busy();
		show();
		told(pending ? { ...at6(), ...(withAddress ? { 'system.place.address': addr.value.trim() || undefined } : {}) } : undefined);
	};
	const read = () => {
		const a = parseFloat(lat.value), b = parseFloat(lon.value);
		return Number.isFinite(a) && Number.isFinite(b) && Math.abs(a) <= 90 && Math.abs(b) <= 180 ? { lat: a, lon: b } : null;
	};
	const typed = () => {
		pending = read();
		busy();
		show();
		told(pending ? at6() : undefined);
	};
	lat.addEventListener('change', typed);
	lon.addEventListener('change', typed);
	const find = async () => {
		msg.replaceChildren();
		found.replaceChildren(h('span', { class: 'sub' }, 'Finding…'));
		try {
			const r = await get(`/v1/map/find?q=${encodeURIComponent(addr.value.trim())}`);
			found.replaceChildren(...(r.places.length ? r.places.map((p) => h('button', { type: 'button', class: 'button small', onclick: () => {
				pending = { lat: p.latitude, lon: p.longitude };
				found.replaceChildren();
				map.go(p.latitude, p.longitude, 17);
				sync();
			} }, p.name)) : [h('span', { class: 'sub' }, 'No place found by that address. Click where it is on the map instead.')]));
		} catch (e) {
			found.replaceChildren();
			msg.replaceChildren(e.message);
		}
	};

	// Suggestions as the address is typed (0169): from Photon, by way of the
	// manager, those near where the map looks first. Picked by a click, or
	// by the arrows and Enter; Enter with none picked finds the address.
	const list = h('ul', { class: 'mapsuggest', role: 'listbox', id: `suggest-${here}`, hidden: true });
	let offered = [], lit = -1, asked = 0, timer = null;
	const offer = () => {
		list.replaceChildren(...offered.map((p, i) => h('li', {
			id: `suggest-${here}-${i}`, role: 'option', class: i === lit ? 'on' : '', 'aria-selected': i === lit ? 'true' : 'false',
			onmousedown: (e) => { e.preventDefault(); choose(p); },
		}, p.name)));
		list.hidden = !offered.length;
		addr.setAttribute('aria-expanded', String(!list.hidden));
		if (lit >= 0 && offered[lit]) addr.setAttribute('aria-activedescendant', `suggest-${here}-${lit}`);
		else addr.removeAttribute('aria-activedescendant');
	};
	// close shuts the list, and lets go of any answer still to come.
	const close = () => {
		clearTimeout(timer);
		asked++;
		offered = [];
		lit = -1;
		offer();
	};
	const choose = (p) => {
		addr.value = p.name;
		close();
		found.replaceChildren();
		pending = { lat: p.latitude, lon: p.longitude };
		map.go(p.latitude, p.longitude, 17);
		sync(true);
	};
	const suggest = async () => {
		const q = addr.value.trim();
		if (q.length < 3) return close();
		const n = ++asked;
		const c = map.center();
		const near = c.z >= 5 ? `&lat=${c.lat.toFixed(4)}&lon=${c.lon.toFixed(4)}` : '';
		try {
			const r = await get(`/v1/map/suggest?q=${encodeURIComponent(q)}${near}`);
			if (n !== asked || document.activeElement !== addr) return;
			offered = r.places;
			lit = -1;
			offer();
		} catch {
			if (n === asked) close();
		}
	};
	addr.addEventListener('input', () => {
		told({ 'system.place.address': addr.value.trim() || undefined });
		// What was picked out of the list is not what is typed now.
		lit = -1;
		offer();
		clearTimeout(timer);
		timer = setTimeout(suggest, 300);
	});
	addr.addEventListener('blur', close);
	addr.addEventListener('keydown', (e) => {
		const n = offered.length;
		if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && n) {
			e.preventDefault();
			lit = e.key === 'ArrowDown' ? (lit + 1) % n : lit <= 0 ? n - 1 : lit - 1;
			offer();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (lit >= 0 && offered[lit]) choose(offered[lit]);
			else {
				close();
				find();
			}
		} else if (e.key === 'Escape' && n) {
			e.preventDefault();
			close();
		}
	});
	editor?.append(
		h('div', { class: 'mapline' }, h('div', { class: 'mapaddr' }, addr, list), h('button', { type: 'button', class: 'button small', onclick: find }, 'Find')),
		found,
		h('div', { class: 'mapline' }, lat, lon,
			h('label', { class: 'switchrow' }, aim, ' Put it where I click')),
		msg);
	if (editor) save?.register(part);

	const load = async () => {
		try {
			const [r, fleet] = await Promise.all([get(`/v1/places?under=${encodeURIComponent(under)}`), get('/v1/aps').catch(() => ({ aps: [] }))]);
			places = r.places;
			status = fleetMap(fleet.aps || []);
		} catch (e) {
			panel.append(h('div', { class: 'error' }, e.message));
			return;
		}
		// Where it was left, or fitted to its pins the first time.
		if (!kept) map.fit(pinsOf(places, status, here, null));
		show();
	};

	panel.replaceChildren(
		h('h2', null, 'Map', h('span', { class: 'note' }, isAP ? 'its folder, and where it is' : 'this folder, and all below it')),
		map.el, info, editor || null);
	load();
	return panel;
}
