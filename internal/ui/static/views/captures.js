// Captures people start (0148): what crosses an AP, taken in its own path,
// on the Wi-Fi side of a network, on its wired side, or both in one file.
// The Captures tab holds those that were taken; an AP's Capture tab takes a
// network's or a VLAN's traffic; a client's page takes that client's.

import { h, link } from '../dom.js';
import { get, post, del, download } from '../api.js';
import { bandName } from '../format.js';
import { floatWindow, floating } from '../float.js';
import { packetsWindow } from './packetview.js';
import { called } from '../names.js';

const SIDES = [['wireless', 'Wireless', 'The Wi-Fi side: the network’s own interface on each radio, where its clients’ traffic is already out of its encryption'],
	['wired', 'Wired', 'The wired side: the port the network leaves the AP by, in its VLAN, or its tunnel'],
	['both', 'Both', 'Both sides in one file: each packet as it crossed the Wi-Fi side and the wire']];
const SECONDS = [[30, '30 seconds'], [60, '1 minute'], [120, '2 minutes'], [300, '5 minutes']];
const BANDS = ['2g', '5g', '6g'];

const when = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
const size = (n) => (n < 1000 ? `${n} bytes` : n < 1e6 ? `${(n / 1000).toFixed(n < 10000 ? 1 : 0)} kB` : `${(n / 1e6).toFixed(1)} MB`);
const open = (k) => k.state === 'pending' || k.state === 'running';
const place = (k) => (k.place || []).map((p) => p.name).join(' › ');
const sideName = (s) => SIDES.find(([k]) => k === s)?.[1] || s;

// of is what a capture is of, in words: a client, a network, or a VLAN.
function of(k) {
	const a = k.ask || {};
	if (a.client) return `Client ${a.client}${a.ssid ? ` on ${a.ssid}` : ''}`;
	if (a.network) return a.ssid || a.network;
	return `VLAN ${a.vlan}`;
}

// stateChip says how a capture stands.
function stateChip(k) {
	if (k.state === 'pending' && k.told) return h('span', { class: 'chip from', title: 'Its AP was told at once, on the line it holds open to the manager, and begins within the second' }, 'Starting');
	if (k.state === 'pending') return h('span', { class: 'chip warn', title: 'This AP holds no line to the manager: it takes the capture up at its next poll, which can be a minute' }, 'Waiting for the AP’s poll');
	if (k.state === 'running') return h('span', { class: 'chip from' }, k.stopping ? 'Stopping' : 'Running');
	if (k.state === 'done') return h('span', { class: 'chip ok', title: k.result || '' }, 'Done');
	if (k.state === 'expired') return h('span', { class: 'chip warn', title: 'Its AP did not take it up, or did not finish it, in ten minutes' }, 'Not finished');
	return h('span', { class: 'chip bad', title: k.result || '' }, 'Failed');
}

// act is a small button that does one thing to a capture and has the list
// drawn again; what goes wrong is said beside it.
function act(label, title, fn, again) {
	const b = h('button', { type: 'button', class: 'button small', title }, label);
	b.addEventListener('click', async () => {
		b.disabled = true;
		try {
			await fn();
			again();
		} catch (e) {
			b.disabled = false;
			b.after(h('span', { class: 'sub bad', role: 'alert' }, ` ${e.message}`));
		}
	});
	return b;
}

// capturesTable is captures in a table, the latest first: when, where, of
// what, which sides, how much, who asked, and how each stands, with what
// can be done to it. again draws the list anew.
function capturesTable(list, again, withAP) {
	return h('table', { class: 'list' },
		h('tr', null, ['Started', withAP && 'AP', withAP && 'Location', 'Of', 'Sides', 'Packets', 'Size', 'Asked by', 'State', ''].filter((t) => t !== false).map((t) => h('th', null, t))),
		list.map((k) => h('tr', { class: k.state === 'failed' ? 'failed' : '' },
			h('td', { class: 'nowrap' }, when(k.at)),
			withAP && h('td', null, link(`/aps/${encodeURIComponent(k.ap)}/capture`, k.ap_name || k.ap)),
			withAP && h('td', { class: 'sub' }, place(k)),
			h('td', null, of(k), k.ask?.bands?.length > 0 && h('div', { class: 'sub' }, k.ask.bands.map(bandName).join(', '))),
			h('td', null, sideName(k.ask?.sides), h('div', { class: 'sub' }, k.ask?.whole ? 'whole packets' : `first ${k.ask?.snap || 128} bytes`)),
			h('td', null, k.detail?.packets != null ? String(k.detail.packets) : '—'),
			h('td', { class: 'nowrap' }, k.bytes > 0 ? size(k.bytes) : '—'),
			h('td', null, k.actor),
			h('td', null, stateChip(k), !open(k) && k.result && h('div', { class: 'sub' }, k.result),
				k.took_ms != null && h('div', { class: 'sub', title: 'From the AP hearing of it on its line to its capture running, by the AP’s own clock' }, `began ${k.took_ms} ms after the AP was told`)),
			h('td', { class: 'nowrap' },
				k.bytes > 0 && h('button', { type: 'button', class: 'button small', title: 'Its packets in the packet viewer: a list, each taken apart, and its bytes (0341)',
					onclick: () => packetsWindow(`/v1/pcaps/${k.id}/file`, `${of(k)} · ${k.ap_name || k.ap} · ${new Date(k.at).toLocaleString(undefined, { hour12: false })}`) }, 'Packets'),
				k.bytes > 0 && act(open(k) ? 'Download so far' : 'Download', 'One pcapng file, which Wireshark opens: an interface for each the AP took, their packets together by time', () => download(`/v1/pcaps/${k.id}/file`), () => {}),
				open(k) && !k.stopping && act('Stop', 'End it now: the AP sends what it has', () => post(`/v1/pcaps/${k.id}/stop`, {}), again),
				!open(k) && act('Delete', 'Take this capture away, its file with it', () => del(`/v1/pcaps/${k.id}`), again)))));
}

// What an AP's Capture tab was last set to, by AP, so a page drawn again
// keeps it.
const forms = new Map();

// segmented is one choice of several, as buttons in a row.
function segmented(label, options, value, pick) {
	const seg = h('span', { class: 'segmented', role: 'group', 'aria-label': label });
	const draw = (v) => seg.replaceChildren(...options.map(([k, name, title]) => h('button', { type: 'button', class: k === v ? 'on' : '', 'aria-pressed': String(k === v), title,
		onclick: () => { pick(k); draw(k); } }, name)));
	draw(value);
	return seg;
}

// askFor has an AP take a capture, and says what went wrong where it would
// not.
async function askFor(ap, ask, said, again) {
	said.replaceChildren();
	try {
		await post(`/v1/aps/${encodeURIComponent(ap)}/pcaps`, ask);
		again();
	} catch (e) {
		said.replaceChildren(h('div', { class: 'error' }, e.message));
	}
}

// captureTab is an AP's Capture, opened from its header (0284): a network's traffic or a VLAN's, the
// Wi-Fi side, the wired side or both, and the captures taken on this AP.
// redraw draws the page again.
export async function captureTab(ap, cfg, redraw) {
	const nets = Object.entries(cfg?.document?.network || {}).sort(([a], [b]) => a.localeCompare(b));
	const f = forms.get(ap) || { what: 'network', network: nets[0]?.[0] || '', vlan: '', sides: 'both', bands: [], whole: false, seconds: 60 };
	forms.set(ap, f);
	if (f.what === 'network' && !nets.some(([id]) => id === f.network)) f.network = nets[0]?.[0] || '';
	const { captures } = await get(`/v1/pcaps?ap=${encodeURIComponent(ap)}`);
	const running = captures.some(open);
	const said = h('div');
	const netSel = h('select', { 'aria-label': 'Network', onchange: (e) => { f.network = e.target.value; } },
		nets.map(([id, n]) => h('option', { value: id, selected: id === f.network }, n.ssid ? `${n.ssid} (${id})` : id)));
	const vlanIn = h('input', { type: 'number', min: 1, max: 4094, placeholder: 'VLAN', 'aria-label': 'VLAN', value: f.vlan, class: 'short', oninput: (e) => { f.vlan = e.target.value; } });
	const what = h('span', { class: 'of' });
	const drawWhat = () => what.replaceChildren(f.what === 'network' ? (nets.length ? netSel : h('span', { class: 'sub' }, 'This AP offers no network')) : vlanIn);
	drawWhat();
	const start = h('button', { type: 'button', class: 'button', disabled: running, title: running ? 'A capture is running on this AP already' : 'The AP is told at once and begins within the second, where it holds its line to the manager' }, 'Start capture');
	start.addEventListener('click', () => {
		const ask = { sides: f.sides, seconds: f.seconds, whole: f.whole };
		if (f.what === 'network') {
			ask.network = f.network;
			if (f.bands.length && f.sides !== 'wired') ask.bands = f.bands;
		} else ask.vlan = Number(f.vlan) || 0;
		askFor(ap, ask, said, redraw);
	});
	return h('section', { class: 'panel capture' },
		h('h2', null, 'Capture', h('span', { class: 'note' }, 'what crosses this AP, taken on the AP itself, for Wireshark · ', h('a', { href: '#/captures' }, 'every AP’s captures'))),
		h('div', { class: 'captureform' },
			h('label', null, h('span', { class: 'lab' }, 'Of'),
				segmented('What to capture', [['network', 'A network', 'Everything of one SSID on this AP'], ['vlan', 'A VLAN', 'Everything of one VLAN on this AP']], f.what, (k) => { f.what = k; drawWhat(); }), what),
			h('label', null, h('span', { class: 'lab' }, 'Side'), segmented('Which side', SIDES, f.sides, (k) => { f.sides = k; })),
			h('label', null, h('span', { class: 'lab' }, 'Bands'),
				h('span', { class: 'ticks', title: 'Of the Wi-Fi side: none ticked is every band the network is on' }, BANDS.map((b) => h('label', { class: 'tick' },
					h('input', { type: 'checkbox', checked: f.bands.includes(b), onchange: (e) => { f.bands = e.target.checked ? [...f.bands, b] : f.bands.filter((x) => x !== b); } }), bandName(b))))),
			h('label', null, h('span', { class: 'lab' }, 'For'),
				h('select', { 'aria-label': 'For how long', onchange: (e) => { f.seconds = Number(e.target.value); } }, SECONDS.map(([s, name]) => h('option', { value: s, selected: s === f.seconds }, name)))),
			h('label', { title: 'The first 128 bytes of each packet say who spoke to whom, and by what; whole packets hold what was said, where it was not encrypted' },
				h('span', { class: 'lab' }, 'Whole packets'),
				h('input', { type: 'checkbox', class: 'switch', role: 'switch', checked: f.whole, 'aria-label': 'Keep whole packets', onchange: (e) => { f.whole = e.target.checked; } })),
			start),
		said,
		captures.length > 0 ? capturesTable(captures, redraw, false)
			: h('p', { class: 'sub pad' }, 'No capture has been taken on this AP. To capture one client’s traffic, open the client from the Clients tab.'));
}

// again is how soon a page of captures is read again, in seconds: each
// second while one is yet to begin, since whoever asked is waiting to see
// it run; every two while one runs; and seldom otherwise.
function again(captures) {
	return captures.some((k) => k.state === 'pending') ? 1 : captures.some(open) ? 2 : 30;
}

// captureAgain is how soon an AP's Capture tab is read again.
export async function captureAgain(ap) {
	try {
		return again((await get(`/v1/pcaps?ap=${encodeURIComponent(ap)}`)).captures);
	} catch (e) {
		return 30;
	}
}

// capturesPage is every capture taken on the APs the caller may view, from
// an AP's Capture (0284).
export async function capturesPage(ctx, redraw) {
	const { captures } = await get('/v1/pcaps');
	return {
		main: [
			h('div', { class: 'head' }, h('h1', null, 'Captures')),
			h('section', { class: 'panel' },
				h('h2', null, 'Captures taken', h('span', { class: 'note' }, 'started from an AP’s Capture or from a client; the latest 60 are kept, for 30 days')),
				captures.length > 0 ? capturesTable(captures, redraw, true)
					: h('p', { class: 'sub pad' }, 'None yet. An AP’s Capture, in its header, takes a network’s or a VLAN’s traffic; a client’s page takes that client’s.')),
		],
		refresh: again(captures),
	};
}

// captureWindow is a client's capture in a window that floats over the page
// (0149), as its Live Trace is.
export function captureWindow(client) {
	clientCapture(floatWindow('capture', 'Capture', { width: 820, height: 360 }), client);
}

// clientCapture is a client's capture, in box: its traffic alone, on the
// AP and network it is on, the Wi-Fi side, the wired side or both; and the
// captures taken of it. It asks again every few seconds while one runs.
export function clientCapture(box, client) {
	const f = { sides: 'both', seconds: 60, whole: false };
	const panel = h('section', { class: 'panel capture', 'data-editing': floating(box) ? null : true });
	const said = h('div');
	box.replaceChildren(panel);
	let timer = null;
	const draw = async () => {
		clearTimeout(timer);
		if (!panel.isConnected) return;
		let captures = [];
		try {
			captures = (await get(`/v1/pcaps?client=${encodeURIComponent(client.mac)}`)).captures;
		} catch (e) {
			said.replaceChildren(h('div', { class: 'error' }, e.message));
		}
		const running = captures.some(open);
		const can = client.ap && client.network;
		const start = h('button', { type: 'button', class: 'button small', disabled: running || !can,
			title: !can ? 'It is on no network of an AP now: a capture is taken where the client is' : 'Its AP is told at once and begins within the second, where it holds its line to the manager' }, 'Start capture');
		start.addEventListener('click', () => askFor(client.ap, { sides: f.sides, seconds: f.seconds, whole: f.whole, network: client.network, client: client.mac }, said, draw));
		panel.replaceChildren(...[
			h('h2', { class: 'withcontrols' }, h('span', { class: 'title' }, `Capture: ${called(client.mac, client.host) || client.mac}`),
				h('span', { class: 'controls' },
					h('button', { type: 'button', class: 'button small', onclick: () => { clearTimeout(timer); box.replaceChildren(); } }, 'Close'))),
			h('div', { class: 'captureform' },
				h('span', { class: 'sub' }, can ? `Its traffic alone, on ${client.ap_name || client.ap}${client.ssid ? `, ${client.ssid}` : ''}` : 'Not on a network now'),
				h('label', null, h('span', { class: 'lab' }, 'Side'), segmented('Which side', SIDES, f.sides, (k) => { f.sides = k; })),
				h('label', null, h('span', { class: 'lab' }, 'For'),
					h('select', { 'aria-label': 'For how long', onchange: (e) => { f.seconds = Number(e.target.value); } }, SECONDS.map(([s, name]) => h('option', { value: s, selected: s === f.seconds }, name)))),
				h('label', { title: 'The first 128 bytes of each packet say who spoke to whom, and by what; whole packets hold what was said, where it was not encrypted' },
					h('span', { class: 'lab' }, 'Whole packets'),
					h('input', { type: 'checkbox', class: 'switch', role: 'switch', checked: f.whole, 'aria-label': 'Keep whole packets', onchange: (e) => { f.whole = e.target.checked; } })),
				start),
			said,
			captures.length > 0 && capturesTable(captures, draw, true),
		].filter(Boolean));
		if (running) timer = setTimeout(draw, again(captures) * 1000);
	};
	draw();
}
