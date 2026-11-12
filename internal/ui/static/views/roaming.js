// Roaming (0136): the clients that moved from one AP to another, or from
// one radio of an AP to another, over the last day. How many, how long they
// typically took to be let on where they came to, of which kind, and those
// that failed or were slow, each with what the AP said of it; and the
// clients that came most. A part of Clients of its own, Wi-Fi roaming,
// beside the clients themselves (0337); the Overview's roams cell leads
// there.

import { h } from '../dom.js';
import { ago, bandName } from '../format.js';
import { clientLink } from './journey.js';
import { called } from '../names.js';

// The kinds of roam, as they are named, and what each is.
export const KIND = {
	ft: ['802.11r', 'A fast transition: the client brings its keys with it, and there is no handshake'],
	okc: ['OKC', 'A key made from its sign-in at another AP (opportunistic key caching): the four key messages, and no sign-in again'],
	pmksa: ['PMK cache', 'A key this AP held from the client’s last time here: the four key messages, and no sign-in again'],
	cached: ['PMK cached', 'A key from before, from this AP’s cache or made from a sign-in at another: the four key messages, and no sign-in again'],
	keys: ['4-way', 'A network with a passphrase and no fast transition: the four key messages again, which is all there is to do'],
	full: ['Hard roam', 'A hard roam, everything again: the sign-in, or WPA3’s exchange, and then the four key messages'],
};
const kind = (k) => KIND[k]?.[0] || k;
// A kind in a few words after another, as of a band switch: "band switch,
// full sign-in".
export const kindWords = (k) => (k === 'full' ? 'full sign-in' : kind(k));
const switches = (n) => `${n} band switch${n === 1 ? '' : 'es'}`;

export const took = (n) => (n == null ? '—' : n < 1000 ? `${n} ms` : `${(n / 1000).toFixed(1)} s`);
const many = (n, one) => `${n} ${one}${n === 1 ? '' : 's'}`;

// what is how a roam went, in a word, and in more for whoever asks.
function what(r, slowOver) {
	if (r.state === 'failed') return h('span', { class: 'chip bad', title: r.reason || 'It was not let on' }, 'Failed');
	if (r.state === 'left') return h('span', { class: 'chip warn', title: r.reason || 'It went before it was let on' }, 'Gave up');
	if (r.state === 'slow') return h('span', { class: 'chip warn', title: `Let on, after more than ${took(slowOver)}` }, 'Slow');
	return h('span', { class: 'chip ok' }, 'Good');
}

// why is what there is to say of a roam that went badly.
function why(r) {
	const said = [];
	if (r.sa_queries) said.push(`the AP still held it as on, and asked ${r.sa_queries === 1 ? 'once' : `${r.sa_queries} times`} for proof before it took it (802.11w SA Query)`);
	if (r.reason) said.push(r.reason);
	return said.join('; ') || '—';
}

const place = (name, band) => [name, band && bandName(band)].filter(Boolean).join(' · ');

// How long each AP kept its clients waiting between their last key message
// and being let on (0231), a line a band: the radio storing the client's
// key, 1 or 2 ms where nothing holds it up. Past SLOW_KEY the AP's kernel or
// driver holds every join up, as mac80211 did, 30-60 ms (B94).
const SLOW_KEY = 20;
function keyWaitLines(waits) {
	const bands = [...new Set(waits.map((w) => w.band))];
	return bands.map((b) => h('p', { class: 'pad' }, h('strong', null, `${bandName(b)}: `),
		waits.filter((w) => w.band === b).map((w, i) => [i > 0 && ' · ',
			h('span', { title: `${w.joins} join${w.joins === 1 ? '' : 's'} timed; the longest ${took(w.high_ms)}` }, `${w.ap_name} `,
				h('span', { class: w.typical_ms >= SLOW_KEY ? 'chip warn' : 'mono' }, took(w.typical_ms)))])));
}

// roamingPanel is the roams of a view, r, as GET /v1/roaming gives them:
// the panel, or one that says no client roamed.
export function roamingPanel(r) {
	// The clients that came to the APs most over the same hours, a roam or a
	// join from nothing alike (Griff, 2026-10-25): one that keeps coming is
	// one to look at. An older manager has none.
	const joins = r?.joins || [];
	const waits = r?.key_waits || [];
	if (!r || !(r.roams || r.left || r.band_switches || joins.length || waits.length))
		return h('section', { class: 'panel' }, h('h2', null, 'Roaming'),
			h('p', { class: 'sub pad' }, r ? `No client moved between APs here, or between the radios of one, over the last ${r.hours} h.` : 'This manager says nothing of roaming.'));
	// A client's name leads to its page, its Overview first (0337); a roam's
	// time to that roam on its Activity, opened.
	const client = (m) => clientLink(m.mac, called(m.mac, m.host) || m.mac);
	const row = (m, more) => h('tr', { class: m.state === 'failed' ? 'failed' : '' },
		h('td', { class: 'sub nowrap' }, m.id ? h('a', { href: `#/clients/${m.mac}/activity/${m.id}`, title: 'This roam, on the client’s Activity' }, ago(m.at)) : ago(m.at)),
		h('td', null, client(m)),
		h('td', null, m.from_ap_name ? place(m.from_ap_name, m.from_band) : h('span', { class: 'sub' }, 'not known')),
		h('td', null, place(m.ap_name || m.ap, m.band)),
		h('td', { class: 'nowrap', title: `${m.band_switch ? 'A band switch: the client moved from one radio of this AP to another. ' : ''}${KIND[m.kind]?.[1] || ''}` },
			m.band_switch ? ['Band switch', h('span', { class: 'sub' }, ` · ${kindWords(m.kind)}`)] : kind(m.kind)),
		h('td', { class: 'mono nowrap', title: m.untimed ? 'Let on, and not timed: its AP’s log has its association and nothing of its authentication, so what it took is not known' : null }, took(m.ms)),
		h('td', null, what(m, r.slow_over_ms)),
		more && h('td', { class: 'sub' }, why(m)));
	const head = (more) => h('tr', null, ['When', 'Client', 'From', 'To', 'Kind', 'Took', '', ...(more ? ['What the AP said'] : [])].map((t) => h('th', null, t)));
	return h('section', { class: 'panel' },
		h('h2', null, 'Roaming', h('span', { class: 'note' },
			[many(r.roams, 'roam'), r.typical_ms != null && `typically ${took(r.typical_ms)}`, r.failed > 0 && `${r.failed} failed`, r.slow > 0 && `${r.slow} slow`, r.band_switches > 0 && switches(r.band_switches)].filter(Boolean).join(' · '))),
		h('p', { class: 'sub pad' }, `${r.roams || r.left ? many(r.clients, 'client') : 'No roams'} over the last ${r.hours} h. A roam is a client’s move to another AP on the same network, whether it says it is already on the network or joins as if for the first time. A move to another radio of the same AP is a band switch: listed below, and in none of the counts but its own. Each is timed at the AP the client comes to, from its authentication there, as the AP’s log has it, to its being let on; past ${took(r.slow_over_ms)} it is called slow.${r.untimed ? ` ${r.untimed} ${r.untimed === 1 ? 'is' : 'are'} not timed: the AP’s log has no line of the authentication, as on a network that signs clients in.` : ''}${r.left ? ` ${r.left} more ${r.left === 1 ? 'was' : 'were'} given up by the client before it was let on.` : ''}${r.more ? ' There were more attempts than are read: these are of the latest.' : ''}`),
		r.kinds.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['Kind', 'Roams', 'Typical', 'Fastest', 'Slowest', 'Slow', 'Failed'].map((t) => h('th', null, t))),
			r.kinds.map((k) => h('tr', null,
				h('td', { title: KIND[k.kind]?.[1] || '' }, kind(k.kind)),
				h('td', null, String(k.roams)), h('td', { class: 'mono' }, took(k.typical_ms)),
				h('td', { class: 'mono sub' }, took(k.low_ms)), h('td', { class: 'mono sub' }, took(k.high_ms)),
				h('td', null, k.slow ? String(k.slow) : h('span', { class: 'sub' }, '0')),
				h('td', null, k.failed ? h('span', { class: 'chip bad' }, String(k.failed)) : h('span', { class: 'sub' }, '0'))))),
		waits.length > 0 && h('h3', { class: 'subhead', title: `How long each AP kept its clients waiting between their last key message and being let on, typically, over every join of the last ${r.hours} h that had the key messages: the radio storing the client’s key. 1 or 2 ms is what it takes; ${SLOW_KEY} ms or more means the AP’s kernel or driver holds every join up.` }, 'Let on after the last key message'),
		waits.length > 0 && keyWaitLines(waits),
		joins.length > 0 && h('h3', { class: 'subhead', title: 'The clients that tried to come online here most often, more than once: each roam to an AP and each join from nothing is one. A client that keeps coming back is one to look at.' }, `Joined most, last ${r.hours} h`),
		joins.length > 0 && h('table', { class: 'list' },
			h('tr', null, ['Client', 'Joins', 'Failed', 'At'].map((t) => h('th', null, t))),
			joins.map((j) => h('tr', null,
				h('td', null, client(j)),
				h('td', null, String(j.attempts)),
				h('td', null, j.failed ? h('span', { class: 'chip bad' }, String(j.failed)) : h('span', { class: 'sub' }, '0')),
				h('td', { class: 'sub' }, j.aps.join(', '))))),
		r.bad.length > 0 && h('h3', { class: 'subhead' }, 'Went badly'),
		r.bad.length > 0 && h('table', { class: 'list' }, head(true), r.bad.map((m) => row(m, true))),
		h('h3', { class: 'subhead' }, 'Latest'),
		h('table', { class: 'list' }, head(false), r.latest.slice(0, 15).map((m) => row(m, false))));
}
