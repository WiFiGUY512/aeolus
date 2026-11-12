// Sticky clients (0340): the clients that held on to an AP where it heard
// them weakly, for half an hour or more, while another AP heard them far
// better on the same band; and those weak with nowhere better seen. A part
// of a folder's Clients of its own, beside Wi-Fi roaming, so a person can
// judge whether a client wants steering of its own, to the nearest AP
// (0321), pinning to one (0319), or coverage. One band at a time, by the
// page's band picker (Griff, 2026-10-22).

import { h } from '../dom.js';
import { get } from '../api.js';
import { ago, bandName } from '../format.js';
import { clientLink } from './journey.js';
import { called } from '../names.js';
import { redrawNow } from '../refresh.js';

// The hours looked at, kept as the page is drawn again.
const view = { hours: 24 };
const HOURS = [[24, 'Last day'], [72, 'Last 3 days'], [168, 'Last week']];

const mins = (n) => (n >= 120 ? `${Math.round(n / 60)} h` : `${n} min`);

// What may help each, in a few words, and the whole of it on hover.
const HELP = {
	nearest: ['Steer to the nearest AP', 'A rule of the nearest (client steering): the APs compare how strongly each hears it, and send it to the one that hears it best. It follows the client wherever it goes.'],
	pin: ['Pin to', 'It is seen on one band only, and its better place is the same AP each time: pinned there, it is kept on that AP whatever its signal says.'],
	'steer-off': ['Network not steered here', 'Its network is not steered on this AP: no rule of a client, and no sticky, acts where a network is not steered. Turning steering on for the network restarts its Wi-Fi, and is a person’s call.'],
	'rule-idle': ['Has a rule; held on anyway', 'It has a rule of client steering, and held on here all the same: its steering record says what was asked of it, and what came of it.'],
	coverage: ['No AP hears it better', 'No other AP of these heard it better on this band: it lacks coverage, and no steering helps it.'],
	pinned: ['Pinned here', 'A person pinned it to this AP: it is kept here, as asked, and no steering moves it.'],
	'pin-moved': ['Pinned here; moved?', 'A person pinned it to this AP, and over the last half hour it was heard here far weaker than its usual: it may have been moved, and be pinned to an AP that is no longer its best (the pin-moved warning). Its pin is in its Steering.'],
};

// stickyPanel is the sticky list of a folder, id, on the band the page's
// picker shows.
export async function stickyPanel(id, band) {
	let r;
	try {
		r = await get(`/v1/sticky?under=${encodeURIComponent(id)}&hours=${view.hours}`);
	} catch (e) {
		return h('section', { class: 'panel' }, h('h2', null, 'Sticky clients'), h('p', { class: 'error pad' }, e.message));
	}
	const hours = h('select', { 'aria-label': 'Hours', onchange: (e) => { view.hours = Number(e.target.value); redrawNow(); } },
		HOURS.map(([n, label]) => h('option', { value: n, selected: n === view.hours }, label)));
	const rows = r.clients.filter((c) => !band || band === 'all' || c.band === band);
	const sticky = rows.filter((c) => c.class === 'sticky').length;
	const others = Object.entries(r.bands).filter(([b]) => b !== band).map(([b, n]) => `${n} on ${bandName(b)}`);
	const head = h('h2', null, 'Sticky clients', h('span', { class: 'note' },
		[`${sticky} sticky`, rows.length - sticky > 0 && `${rows.length - sticky} weak or pinned`, others.length && others.join(', ')].filter(Boolean).join(' · ')), hours);
	const about = h('p', { class: 'sub pad' }, `A client is listed where it spent half an hour or more heard at ${r.weak} dBm or weaker on one AP, on ${band && band !== 'all' ? bandName(band) : 'a band'}, over the ${HOURS.find(([n]) => n === r.hours)?.[1].toLowerCase() || `last ${r.hours} h`}. It is sticky where it had somewhere better: another AP heard it 10 dB or more better over ten minutes or more, or it moved to another AP and was heard there 15 dB or more stronger the next minute, so it was within that AP’s reach while it held on. The one there now first.`);
	if (!rows.length)
		return h('section', { class: 'panel' }, head, about, h('p', { class: 'sub pad' }, `No client held on weakly here${band && band !== 'all' ? ` on ${bandName(band)}` : ''}.`));
	const signal = (s) => h('span', { class: s <= r.weak ? 'chip warn mono' : 'mono' }, `${s} dBm`);
	const rule = (c) => {
		const bits = [];
		if (c.rule) bits.push(c.rule.nearest ? 'Nearest AP' : `Pinned to ${c.rule.aps.map((a) => a.name).join(', ')}`);
		bits.push(c.steered ? (c.sticky ? 'sticky on' : 'steered') : 'not steered');
		if (c.asked) bits.push(`asked ${c.asked.asked}×${c.asked.back ? `, back ${c.asked.back}` : ''}${c.asked.refused ? `, refused ${c.asked.refused}` : ''}`);
		return h('span', { class: 'sub' }, bits.join(' · '));
	};
	const help = (c) => {
		const [say, more] = HELP[c.help] || [c.help, ''];
		const cls = c.class === 'sticky' || c.help === 'pin-moved' ? 'chip warn' : c.class === 'pinned' ? 'chip ok' : 'chip';
		return h('span', { class: cls, title: more }, c.help === 'pin' && c.help_ap ? `${say} ${c.help_ap.name}` : say);
	};
	return h('section', { class: 'panel' }, head, about,
		h('table', { class: 'list' },
			h('tr', null, ['Client', 'Held on', 'Weak for', 'Its signal there', 'Better', 'What may help'].map((t) => h('th', null, t))),
			rows.map((c) => h('tr', { class: c.class === 'sticky' ? '' : 'plain' },
				h('td', null, clientLink(c.mac, c.name || called(c.mac, c.host) || c.mac),
					c.bands.length === 1 && h('div', { class: 'sub', title: 'Seen on this band alone over these hours: never sent to another band' }, `${bandName(c.bands[0])} only`)),
				h('td', null, h('div', { class: 'nowrap' }, c.ap_name), h('div', { class: 'sub nowrap' }, c.now ? 'there now' : `last ${ago(c.last)}`)),
				h('td', { class: 'nowrap', title: `${c.weak_minutes} of its ${c.minutes} minutes there were at ${r.weak} dBm or weaker` }, mins(c.weak_minutes)),
				h('td', { class: 'nowrap' }, signal(c.signal), c.recent != null && c.recent !== c.signal && h('span', { class: 'sub', title: 'Its signal there over the last half hour' }, ' → ', signal(c.recent), ' now')),
				h('td', null, c.better ? h('div', { class: 'nowrap' }, c.better.ap_name, ' ', h('span', { class: 'mono' }, `${c.better.signal} dBm`)) : c.late ? h('div', { class: 'nowrap' }, c.late.ap_name) : h('span', { class: 'sub' }, '—'),
					(c.better || c.late) && h('div', { class: 'sub nowrap', title: c.late ? `It moved late ${c.late.count} time${c.late.count === 1 ? '' : 's'}: the latest to ${c.late.ap_name}, heard there at ${c.late.to} dBm the minute after ${c.late.from} dBm here, ${ago(c.late.at)}` : null },
						[c.better && `${mins(c.better.minutes)} there`, c.late && `moved late ${c.late.count}×, ${c.late.from} → ${c.late.to}`].filter(Boolean).join(' · '))),
				h('td', null, help(c), h('div', null, rule(c)))))));
}
