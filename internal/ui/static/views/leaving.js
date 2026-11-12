// Why clients left (0255), on an AP's or a folder's Overview and a client's
// page: over the last day, a band at a time, how many left and of which
// kind, the kinds worth a look marked; then the latest, each with who sent
// it off and why, as the AP's hostapd frames said, and how strongly the AP
// last heard it. Folded away until asked for: the tile says how it stands.

import { h } from '../dom.js';
import { get } from '../api.js';
import { ago, bandName } from '../format.js';
import { clientLink } from './journey.js';
import { called } from '../names.js';

const many = (n, one) => `${n} ${one}${n === 1 ? '' : 's'}`;

// A kind's count, as a chip where it is worth a look.
const kindChip = (k) => h('span', { class: k.trouble ? 'chip warn' : 'chip', title: k.words }, `${k.count} ${short(k)}`);

// SHORT is each kind in a word or two, for the band lines; one not here is
// said in the manager's words, so a new kind needs no change here.
const SHORT = {
	leaving: 'said bye', 'client-other': 'left unhappy', idle: 'idle', unheard: 'unheard', keys: 'keys failed', signin: 'sign-in failed',
	busy: 'AP full', state: 'not held', 'sent-off': 'sent off', network: 'network down', moved: 'moved AP', asked: 'disconnected',
	steered: 'steered', 'off-air': 'off air', aeolus: 'by Aeolus', silent: 'no word',
};
const short = (k) => SHORT[k.kind] || k.words;

// then is where the client went after it left (0258): back on this AP or
// another and how soon, already joining another, or not back within the ten
// minutes looked at (yet, where those are not over).
const secs = (n) => (n < 90 ? `${n} s` : `${Math.round(n / 60)} min`);
function then(l) {
	const n = l.next;
	if (!n) return Date.now() - new Date(l.at).getTime() > 600000 ? 'not back in 10 min' : 'not back yet';
	const where = n.ap === l.ap ? 'here' : `on ${n.ap_name || n.ap}`;
	const how = n.outcome === 'failed' ? ', and failed' : '';
	return n.after_s < 0 ? `joining ${n.ap_name || n.ap} already${how}` : `back ${where} in ${secs(n.after_s)}${how}`;
}

// leavingPanel is the panel for GET /v1/leaving with these query
// parameters ({ under } or { mac }), or nothing where none left, or an
// older manager has no such view.
export async function leavingPanel(params, open = false) {
	let v;
	try {
		v = await get(`/v1/leaving?${new URLSearchParams({ hours: '24', ...params })}`);
	} catch {
		return null;
	}
	if (!v || !v.total) return null;
	const one = !!params.mac;
	const lines = v.bands.map((b) => h('div', { class: 'leaveband' },
		h('strong', null, `${bandName(b.band)} · ${b.total}`), ' ', b.kinds.map(kindChip)));
	// The clients that kept dropping (0259), each on a line of its own.
	const dropping = (v.dropping || []).map((d) => h('div', { class: 'leaveband' },
		h('span', { class: 'chip warn' }, 'keeps dropping'), ' ', one ? '' : clientLink(d.mac, called(d.mac) || d.mac), one ? '' : ' ',
		h('span', { class: 'sub' }, `${d.times} times within an hour, every ~${secs(d.every_s)}, away ~${secs(d.away_s)} each time, from ${d.aps.join(', ')}`)));
	const row = (l) => h('tr', { class: l.trouble ? 'failed' : '' },
		h('td', { class: 'sub nowrap' }, ago(l.at)),
		!one && h('td', null, clientLink(l.mac, called(l.mac) || l.mac)),
		h('td', null, `${l.ap_name || l.ap} ${bandName(l.band)}`),
		h('td', null, l.words, l.reason_words && h('span', { class: 'sub' }, ` · ${l.by === 'client' ? 'it said' : 'reason'}: ${l.reason_words}`)),
		h('td', { class: 'mono nowrap sub' }, l.signal != null ? `${l.signal} dBm` : ''),
		h('td', { class: 'sub nowrap' }, then(l)));
	return h('details', { class: 'panel logpanel', open },
		h('summary', null, h('h2', null, one ? 'Left' : 'Why clients left', h('span', { class: 'note' },
			[many(v.total, 'leaving'), v.trouble > 0 && `${v.trouble} worth a look`, (v.dropping || []).length > 0 && `${many(v.dropping.length, 'client')} dropping`].filter(Boolean).join(' · ')))),
		h('p', { class: 'sub pad' }, `Over the last ${v.hours} h, a band at a time, by who sent each off and why, as the AP heard it: the client saying so, the AP sending it off, Aeolus letting it go, or no word at all. Marked are the kinds worth a look: not the client’s choice nor Aeolus’s.${v.more ? ' There were more than are read: these are the latest.' : ''}`),
		dropping,
		lines,
		h('table', { class: 'list' },
			h('tr', null, ['When', ...(one ? [] : ['Client']), 'Left', 'Why', 'Heard at', 'Then'].map((t) => h('th', null, t))),
			v.latest.map(row)));
}
