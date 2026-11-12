// A client's way out (0168), in a window of its own: from the client by its
// AP to the switch port its AP's LLDP names, along the client's VLAN to the
// switch that routes for it, and by each router's default route to the last
// thing the switches name before the internet. What the switches set for
// where the client is say, read when the window is opened, each at most
// once a minute; where none is set, as far as its AP's own LLDP says.

import { h } from '../dom.js';
import { get } from '../api.js';
import { ago } from '../format.js';
import { floatWindow } from '../float.js';

// A band's name, for the client's radio link.
const BAND = { '2g': '2.4 GHz', '5g': '5 GHz', '6g': '6 GHz' };

// The table's word for each kind of hop.
const TOP = { client: 'Client', ap: 'AP', switch: 'Switch', device: 'Next', internet: 'Internet' };
const BY = { report: 'its AP\'s report', lldp: 'LLDP', mac: 'MAC table', arp: 'ARP', route: 'route' };
const STATE = { up: 'ok', silent: 'bad', unresolved: 'warn' };
const SAYS = { up: 'answers', silent: 'no answer', unknown: 'not asked for long yet', unresolved: 'no address for the name' };

const took = (n) => (n == null ? null : n >= 10 ? `${Math.round(n)} ms` : `${n.toFixed(1)} ms`);
const known = (x) => (x.by || []).map((b) => BY[b] || b).join(', ');

// The pictures the way is drawn with: a line drawing a kind, 48 by 48.
const SVG = 'http://www.w3.org/2000/svg';
const DRAWN = {
	// A phone: what most clients are.
	client: [['rect', { x: 15, y: 6, width: 18, height: 36, rx: 3 }], ['path', { d: 'M21 37h6' }]],
	// An AP on a ceiling, sending.
	ap: [['rect', { x: 8, y: 31, width: 32, height: 8, rx: 4 }], ['path', { d: 'M17.5 22.5a9 9 0 0 1 13 0' }],
		['path', { d: 'M12.5 17.5a16 16 0 0 1 23 0' }], ['circle', { cx: 24, cy: 26.5, r: 1.6, fill: 'currentColor' }]],
	// A switch: ports, and its traffic across.
	switch: [['rect', { x: 5, y: 21, width: 38, height: 13, rx: 2 }], ['path', { d: 'M10 12h28M34 9l4 3-4 3M14 9l-4 3 4 3' }],
		...[10, 17, 24, 31].map((x) => ['rect', { x, y: 25.5, width: 4.5, height: 4, rx: 0.5 }])],
	// A switch that routes: the router's arrows over its ports.
	gateway: [['rect', { x: 5, y: 25, width: 38, height: 13, rx: 2 }], ['circle', { cx: 24, cy: 11.5, r: 8 }],
		['path', { d: 'M24 6.5v10M19 11.5h10M21.5 8.5l2.5-2 2.5 2M21.5 14.5l2.5 2 2.5-2' }],
		...[10, 17, 24, 31].map((x) => ['rect', { x, y: 29.5, width: 4.5, height: 4, rx: 0.5 }])],
	// A router: the drum with arrows on its top.
	router: [['ellipse', { cx: 24, cy: 18, rx: 16, ry: 6 }], ['path', { d: 'M8 18v11a16 6 0 0 0 32 0V18' }],
		['path', { d: 'M15 18h18M19 15.5l-4 2.5 4 2.5M29 15.5l4 2.5-4 2.5' }]],
	// The internet: the world.
	internet: [['circle', { cx: 24, cy: 24, r: 16 }], ['ellipse', { cx: 24, cy: 24, rx: 7, ry: 16 }], ['path', { d: 'M8 24h32M10.5 16h27M10.5 32h27' }]],
};

function drawn(kind) {
	const svg = document.createElementNS(SVG, 'svg');
	for (const [k, v] of Object.entries({ viewBox: '0 0 48 48', fill: 'none', stroke: 'currentColor', 'stroke-width': 2.2,
		'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' })) svg.setAttribute(k, v);
	for (const [tag, attrs] of DRAWN[kind] || DRAWN.router) {
		const el = document.createElementNS(SVG, tag);
		for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
		svg.append(el);
	}
	return svg;
}

// node is what the picture says of a hop: its drawing, its name and what is
// under it, its state, and what it says when pointed at.
function node(x, net) {
	switch (x.kind) {
	case 'client':
		return { kind: 'client', top: 'Client', name: x.name || x.mac, sub: [x.address, x.vlan && `VLAN ${x.vlan}`], state: 'start', title: [x.mac, x.vendor].filter(Boolean).join(' · ') };
	case 'ap':
		return { kind: 'ap', top: 'AP', name: x.name, sub: [x.address], state: 'ok', title: x.out && `Its uplink: ${x.out}` };
	case 'switch':
		return {
			kind: x.gateway ? 'gateway' : 'switch', top: x.gateway ? 'Gateway' : 'Switch', name: x.name,
			sub: x.gateway ? [x.gateway.split(' ').pop(), x.gateway.split(' ')[0]] : [x.address],
			state: x.seen === false ? 'warn' : 'ok',
			title: [x.model, x.seen === false && 'It has not learned the client on the port the way comes in by.', x.note, `Known by ${known(x)}.`].filter(Boolean).join(' '),
		};
	case 'internet':
		return net
			? { kind: 'internet', top: 'Internet', name: net.target, sub: [net.state === 'up' ? took(net.ms) : SAYS[net.state]], state: STATE[net.state] || 'idle', title: `As the AP last asked it${x.note ? `, ${x.note}` : ''}` }
			: { kind: 'internet', top: 'Internet', name: 'internet', sub: [x.note], state: 'idle', title: 'The AP asks no internet target: set one in a folder\'s System settings (The way out).' };
	default: {
		// Named by an address alone, it is known by its maker. One reached
		// by a route is a router; one LLDP names on the way, a switch not
		// read.
		const bare = x.vendor && x.name === x.address;
		const routed = (x.by || []).includes('route');
		return { kind: routed ? 'router' : 'switch', top: routed ? 'Next' : 'Switch', name: bare ? x.vendor : x.name, sub: [x.address, !bare && x.vendor], state: 'idle',
			title: [x.mac, 'Named by what the switches say; not read itself.'].filter(Boolean).join(' ') };
	}
	}
}

// port is a port as a link's end names it: Ethernet28 of "Ethernet28 · Vlan1010".
const port = (p) => (p || '').split(' · ')[0].replace(/ \(Port-Channel\d+\)$/, '');

// picture is the way drawn: each thing on it, and between each two the link
// they are joined by, with the port at each end, coloured by how the thing
// it leads to is. The client's own link is its radio's, dashed.
function picture(d) {
	const net = d.internet;
	const nodes = d.hops.map((x) => node(x, net));
	const out = [];
	d.hops.forEach((x, i) => {
		const n = nodes[i];
		if (i > 0) {
			const prev = d.hops[i - 1];
			const air = prev.kind === 'client';
			const band = air ? [BAND[prev.band] || prev.band, prev.signal != null && `${prev.signal} dBm`].filter(Boolean).join(' · ') : null;
			const from = air ? null : port(prev.out), to = air ? null : port(x.in);
			out.push(h('div', { class: `link ${air ? 'air' : ''} ${n.state}`, title: air ? prev.note : [from, to].filter(Boolean).join(' to ') || null },
				from && h('span', { class: 'end from' }, from),
				to && h('span', { class: 'end to' }, to),
				h('span', { class: 'wire' }),
				band && h('span', { class: 'mid' }, band)));
		}
		out.push(h('div', { class: `node ${n.state}`, title: n.title || null },
			h('span', { class: 'top' }, n.top),
			h('span', { class: 'glyph' }, drawn(n.kind)),
			h('span', { class: 'name' }, n.name || '—'),
			n.sub.filter(Boolean).map((s) => h('span', { class: 'sub' }, s))));
	});
	return h('div', { class: 'hopmap', role: 'img', 'aria-label': `Its way out: ${nodes.map((n) => `${n.top} ${n.name || 'unnamed'}`).join(', then ')}.` }, out);
}

// table is each hop's facts, a line each.
function table(d) {
	return h('table', { class: 'list' },
		h('tr', null, ['', 'Name', 'Address', 'MAC', 'In', 'Out', 'VLAN', 'Known by', ''].map((c) => h('th', null, c))),
		d.hops.map((x) => h('tr', null,
			h('td', { class: 'sub nowrap' }, x.kind === 'switch' && x.gateway ? 'Gateway' : TOP[x.kind] || x.kind),
			h('td', null, x.name || '—', x.model && h('span', { class: 'note' }, ` ${x.model}`)),
			h('td', { class: 'mono nowrap' }, x.address || '—'),
			h('td', { class: 'mono nowrap' }, x.mac || '—'),
			h('td', { class: 'mono nowrap' }, x.in || '—'),
			h('td', { class: 'mono nowrap' }, x.out || '—'),
			h('td', null, x.vlan ? String(x.vlan) : '—'),
			h('td', { class: 'nowrap' }, known(x) || '—'),
			h('td', null, ...[
				x.seen === true && h('span', { class: 'chip ok', title: 'It learned the client on the port the way comes in by' }, 'client seen'),
				x.seen === false && h('span', { class: 'chip warn', title: 'It has not learned the client on the port the way comes in by' }, 'client not seen'),
				x.gateway && h('span', { class: 'chip ok mono' }, x.gateway),
				x.note && x.kind !== 'client' && h('span', { class: 'sub' }, ` ${x.note}`)].filter(Boolean)))));
}

// pathWindow opens a client's way out: name is what it is called.
export function pathWindow(mac, name) {
	const box = floatWindow('clientpath', 'Path', { width: 720, height: 520 });
	const panel = h('section', { class: 'panel clientpath' });
	box.replaceChildren(panel);
	let said = null, error = null, busy = false;

	const draw = () => {
		const d = said;
		const ap = d?.hops?.find((x) => x.kind === 'ap');
		panel.replaceChildren(...[
			h('h2', { class: 'withcontrols' }, h('span', { class: 'title' }, `Path: ${name || mac}`, name && ' ', name && h('span', { class: 'note mono' }, mac)),
				h('span', { class: 'controls' },
					h('button', { type: 'button', class: 'button small', disabled: busy, onclick: () => load() }, 'Again'),
					h('button', { type: 'button', class: 'button small', onclick: () => box.replaceChildren() }, 'Close'))),
			error && h('div', { class: 'error' }, error),
			!d && !error && h('p', { class: 'sub pad' }, 'Asking the switches…'),
			d?.error && h('div', { class: 'error' }, d.error),
			d && d.on === false && ap && h('div', { class: 'banner info' }, `Not on ${ap.name} now: the way from there, where it was last.`),
			d && picture(d),
			d?.ends && h('div', { class: 'banner info' }, `It stops there: ${d.ends}.`),
			d && h('h3', { class: 'subhead' }, 'Hop by hop'),
			d && table(d),
			d && h('p', { class: 'sub pad' }, d.switches.length
				? ['Switches read: ', ...d.switches.flatMap((s, i) => [i > 0 && ' · ',
					s.error ? h('span', { class: 'chip bad', title: s.error }, `${s.address}: ${s.error.replace(`${s.address} `, '').replace(`${s.address}: `, '')}`)
						: h('span', null, `${s.name} (${s.address}), ${ago(s.at)}`)].filter(Boolean))]
				: 'No switches are set for where this client is (System, Switches the manager reads): the way is followed as far as its AP\'s LLDP says.'),
		].filter(Boolean));
	};

	const load = async () => {
		busy = true;
		error = null;
		draw();
		try {
			said = await get(`/v1/clients/${encodeURIComponent(mac)}/path`);
		} catch (e) {
			error = e.message;
		}
		busy = false;
		if (panel.isConnected) draw();
	};
	load();
}
