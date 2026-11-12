// A client's lookups (0154), in a window of its own: what it asks the
// site's DNS server for, read from that server's own log when the window is
// opened, by the address the client held. The domains it asks for most are
// drawn first, as what a device asks for says what it is: a printer its
// maker's cloud, a television its services. Where no server's log is set
// for where the client is, the window shows the first few names its APs
// noted each time it came online (0118).

import { h } from '../dom.js';
import { get } from '../api.js';
import { ago } from '../format.js';
import { floatWindow } from '../float.js';

const SPANS = [[24, 'A day'], [168, 'A week'], [720, '30 days']];

const hhmm = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });

// lookupsWindow opens a client's lookups: name is what it is called.
export function lookupsWindow(mac, name) {
	const box = floatWindow('lookups', 'Lookups', { width: 640, height: 560 });
	const panel = h('section', { class: 'panel lookups' });
	box.replaceChildren(panel);
	let hours = 24, said = null, error = null, busy = false, find = '';

	const places = (list, total) => h('div', { class: 'places' }, list.map((p) => h('div', { class: 'place', title: `${p.times} lookup${p.times === 1 ? '' : 's'} of ${p.names} name${p.names === 1 ? '' : 's'}` },
		h('span', { class: 'pname' }, p.domain),
		h('span', { class: 'pbar' }, h('span', { style: { width: `${Math.max(2, Math.round(100 * p.times / (list[0].times || 1)))}%` } })),
		h('span', { class: 'pnum' }, total ? `${Math.round(100 * p.times / total)}%` : String(p.times)))));

	const table = (list) => {
		const shown = list.filter((l) => !find || l.name.includes(find));
		return h('table', { class: 'list' },
			h('tr', null, ['Name', 'Kind', 'Times', 'Last'].map((t) => h('th', null, t))),
			shown.map((l) => h('tr', null,
				h('td', { class: 'mono' }, l.name, ...[
					l.blocked > 0 && h('span', { class: 'chip warn', title: `The server refused ${l.blocked} of them: the name is blocked there` }, 'blocked'),
					l.failed > 0 && h('span', { class: 'chip idle', title: `${l.failed} had no answer: no such name, or the server could not say` }, 'no answer')].filter(Boolean)),
				h('td', { class: 'nowrap' }, (l.types || []).join(', ')),
				h('td', null, String(l.times)),
				h('td', { class: 'nowrap', title: `first ${hhmm(l.first)}` }, ago(l.last)))),
			!shown.length && h('tr', null, h('td', { colspan: 4, class: 'sub' }, find ? 'No name matches.' : 'None.')));
	};

	const draw = () => {
		const s = said?.server;
		const fromServer = s && !s.error && said.lookups;
		const list = fromServer ? said.lookups : said?.joins || [];
		const top = fromServer ? said.places || [] : said?.join_places || [];
		const total = list.reduce((n, l) => n + l.times, 0);
		const q = h('input', { type: 'search', placeholder: 'Find a name', value: find, 'aria-label': 'Find a name', oninput: (e) => { find = e.target.value.trim().toLowerCase(); body.replaceChildren(table(list)); } });
		const body = h('div', { class: 'lookuplist' }, said ? table(list) : null);
		panel.replaceChildren(...[
			h('h2', { class: 'withcontrols' }, h('span', { class: 'title' }, `Lookups: ${name || mac}`, name && ' ', name && h('span', { class: 'note mono' }, mac)),
				h('span', { class: 'controls' },
					h('span', { class: 'segmented', role: 'group', 'aria-label': 'Over how long' }, SPANS.map(([t, label]) => h('button', {
						type: 'button', class: hours === t ? 'on' : '', 'aria-pressed': hours === t, disabled: busy, onclick: () => { hours = t; load(); },
					}, label))),
					h('button', { type: 'button', class: 'button small', disabled: busy, onclick: () => load() }, 'Again'),
					h('button', { type: 'button', class: 'button small', onclick: () => box.replaceChildren() }, 'Close'))),
			error && h('div', { class: 'error' }, error),
			!said && !error && h('p', { class: 'sub pad' }, 'Asking…'),
			s?.error && h('div', { class: 'error' }, s.error),
			said && h('p', { class: 'sub pad' }, fromServer
				? [`From the log of ${s.url.replace(/^https?:\/\//, '')}`,
					s.asked?.length ? `, for ${[...new Set(s.asked.map((a) => a.address))].join(' and ')} since ${hhmm(s.asked[0].from)}` : ': no AP reported this client in that time, so there is no address to ask about',
					s.asked?.length ? `: ${s.queries} lookup${s.queries === 1 ? '' : 's'}${s.read < s.queries ? `, the latest ${s.read} read` : ''}.` : '.'].join('')
				: [s ? '' : 'No DNS server’s log is set for where this client is (Settings, What clients look up). ',
					`These are the first names its APs noted each time it came online, over the last ${SPANS.find(([t]) => t === hours)?.[1].toLowerCase() || `${hours} hours`}.`,
					!list.length && said.joins_last ? ` It asked for none in that time; the last its APs noted were ${ago(said.joins_last)}.` : ''].join('')),
			said && top.length > 0 && h('h3', { class: 'subhead' }, 'Asks for most', h('span', { class: 'note' }, 'by domain: what a device asks for says what it is')),
			said && top.length > 0 && places(top, total),
			said && list.length > 0 && h('h3', { class: 'subhead' }, `${list.length} name${list.length === 1 ? '' : 's'}`, h('span', { class: 'note' }, 'the one asked for last first'), q),
			said && body,
		].filter(Boolean));
	};

	const load = async () => {
		busy = true;
		error = null;
		draw();
		try {
			said = await get(`/v1/clients/${encodeURIComponent(mac)}/lookups?hours=${hours}`);
		} catch (e) {
			error = e.message;
		}
		busy = false;
		if (panel.isConnected) draw();
	};
	load();
}
