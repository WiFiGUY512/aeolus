// The Networks tab of a Locations folder or AP (0047): the service folders
// that apply there and each network they offer, on the radios here. A
// network is edited where it is shown (0048), but it lives in its service
// folder (0013): an edit changes it there, for every location that uses that
// folder, and the preview names every AP it reaches. Steering and
// multicast-to-unicast are switched right on each network (0051), and what
// steering asked of clients here, and what came of it over a day or a
// month, is shown below them (0138, 0139). A VXLAN transport picks one of
// the tunnels set where it is being edited (0055). The Org's tab also shows
// what the manager hears from DHCP relays (0068). The tab has one save
// (0204): each network's edits and switches, on the Services folder it lives
// in, and the service folders it takes (0320), are drafts, saved as one
// change; adding or deleting a network is a change of its own. Where no
// service folder applies yet, Add a network makes one, and has this
// location take it, in the same change (0320).

import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { bandName, security, group, value, ago, when, secondsAgo, probeOnly, PROBE_ONLY, dhcpWarnings } from '../format.js';
import { editing } from './fields.js';
import { configs } from './sections.js';
import { fieldsForm, changedValues, put } from './edit.js';
import { ask, confirm } from './confirm.js';
import { pageSave, UNSET, draftPart } from './pagesave.js';
import { keysBlock } from './keys.js';
import { vxlanPath, vlanPath, probed } from './path.js';
import { othersPanel } from './others.js';
import { clientLink } from './journey.js';
import { called } from '../names.js';
import { servicesPanel } from './servicefolders.js';
import { idFor } from './newfolder.js';

const BANDS = ['2g', '5g', '6g'];

// The order a network's fields are offered in. Any other field the schema
// has for a network follows under More settings, so a new one can be set
// before it gets a place here.
const MORE = 'More settings';
const SECTIONS = [
	[null, ['ssid', 'security', 'passphrase', 'pmf', 'bands', 'enabled', 'test', 'hidden', 'isolation', 'multicast_to_unicast']],
	// How many failures running of a client test make an alert (0182).
	['Client tests', ['alert_after']],
	// The DHCP servers it has: another is a rogue (0160).
	['DHCP', ['dhcp_servers']],
	['RADIUS', ['radius.mac_auth', 'radius.passphrases', 'radius.auth_server', 'radius.auth_port', 'radius.auth_secret', 'radius.acct_server', 'radius.acct_port', 'radius.acct_secret', 'radius.nas_id',
		'radius.vlans', 'radius.vlan_required', 'radius.das.client', 'radius.das.secret', 'radius.das.port',
		// The user a client test signs in as (0124).
		'radius.test.user', 'radius.test.password', 'radius.test.method']],
	['Roaming and steering', ['roaming.ft', 'roaming.rrm', 'roaming.btm', 'steering.suggest', 'steering.community', 'steering.keep_6g', 'steering.hold_back', 'steering.sticky', 'steering.sticky_signal', 'steering.sticky_margin', 'steering.sticky_floor', 'steering.sticky_hold_down']],
	['Per-user keys', ['keys.vlans']],
	['Traffic', [
		'transport.primary.type', 'transport.primary.vlan', 'transport.primary.concentrator', 'transport.primary.vni', 'transport.primary.probe',
		'transport.fallback.type', 'transport.fallback.vlan', 'transport.fallback.concentrator', 'transport.fallback.vni', 'transport.fallback.probe',
		'transport.switching', 'transport.ha', 'transport.failback', 'transport.holddown', 'transport.down_when_lost',
	]],
];

export async function networksTab(ctx, id, page) {
	const org = id === ctx.trees.locations.root;
	const [d, at, reports, relayed, steered] = await Promise.all([schema(), networksAt(page), configs(page.hardware?.aps || []),
		org ? get('/v1/dhcp/relayed').catch(() => null) : null, steeringOver(id, STEERING_HOURS[0][0]).catch(() => null)]);
	const lib = tunnelsAt(page.fields);
	// The bands some AP here has a radio for: one a folder only offers ahead of its APs is not here yet.
	const bandsHere = new Set((page.hardware?.bands || []).filter((b) => !b.ahead).map((b) => b.band));
	const writable = at.folders.filter((f) => f.canEdit);
	const edit = editing(ctx, 'locations', page);
	// One save for the tab: a batch where it changes more than one folder
	// (0203).
	const save = edit || writable.length ? pageSave(ctx, 'locations', id) : null;
	const addBox = h('div', { class: 'edit flush' });
	// Each network's card by its ID, for the service folders' draft to mark
	// the ones it takes away (0320).
	const cards = new Map(at.nets.map((n) => [n.id, networkCard(ctx, d, n, bandsHere, lib, reports, page.node?.kind === 'ap', save)]));
	// Where no service folder this person may change applies, one who may
	// change this location adds a network in a new one (0320).
	const where = writable.length ? { folders: writable } : edit ? { folders: [], fresh: { loc: id, locName: page.node?.name || ctx.name('locations', id), takes: page.fields?.services?.value || [] } } : null;
	return [
		servicesPanel(ctx, id, page, edit, save, cards),
		at.nets.length === 0
			? h('div', { class: 'banner info' }, 'No network reaches here yet.')
			: h('div', { class: 'bands' }, [...cards.values()]),
		where && h('div', { class: 'below' },
			h('button', { type: 'button', class: at.nets.length ? 'button' : 'button primary', onclick: () => addForm(ctx, d, where, at.nets, addBox, lib) }, 'Add a network')),
		addBox,
		// What the APs here have of their own, that is not Aeolus's (0137).
		othersPanel(page, reports, { folders: writable.map((f) => ({ id: f.id, name: ctx.name('services', f.id) })), taken: new Set(at.nets.map((n) => n.id)), bands: bandsHere }),
		steeringPanel(id, reports, at.nets, steered),
		transportsStatus(reports),
		radiusStatus(reports),
		dhcpStatus(reports),
		relayStatus(relayed),
		save?.bar,
	];
}

// tunnelsAt lists the tunnels set at a Locations node, from the values in
// force there (0055): [{id, address, port, mtu, underlay_vlan}] by name.
export function tunnelsAt(fields) {
	const out = new Map();
	for (const [path, r] of Object.entries(fields || {})) {
		const m = path.match(/^concentrators\.([^.]+)\.(address|port|mtu|underlay_vlan)$/);
		if (!m) continue;
		if (!out.has(m[1])) out.set(m[1], { id: m[1] });
		out.get(m[1])[m[2]] = r.value;
	}
	return [...out.values()].sort((a, b) => a.id.localeCompare(b.id));
}

// networksAt reads the networks offered by the service folders that apply at
// a node, as each folder resolves them, and whether the person may change
// each folder.
async function networksAt(page) {
	const assigned = page.fields?.services?.value || [];
	const pages = await Promise.allSettled(assigned.map((sid) => get(`/v1/trees/services/nodes/${encodeURIComponent(sid)}`)));
	const folders = [];
	const nets = new Map();
	pages.forEach((p, i) => {
		if (p.status !== 'fulfilled') return;
		const sid = assigned[i];
		const canEdit = !!p.value.may?.includes('configure');
		folders.push({ id: sid, canEdit });
		for (const [path, r] of Object.entries(p.value.fields || {})) {
			const m = path.match(/^network\.([^.]+)\.(.+)$/);
			if (!m) continue;
			if (!nets.has(m[1])) nets.set(m[1], { id: m[1], from: sid, canEdit, fields: {} });
			const n = nets.get(m[1]);
			if (n.from === sid) n.fields[m[2]] = r;
		}
	});
	return { folders, nets: [...nets.values()] };
}

// networkCard is a network: what it is, with its switches, or, once Edit is
// pressed, its form; each a draft of the page's save.
function networkCard(ctx, d, n, bandsHere, lib, reports, isAP, save) {
	const box = h('div', { class: 'edit' });
	const live = n.canEdit && save ? switches(ctx, n, save) : null;
	// Drawn once, so its switches keep their draft while the form is open.
	const face = h('div', null, view(ctx, n, bandsHere, live, lib, reports, isAP));
	const body = h('div', null, face);
	let edits = null;
	const close = () => {
		edits?.off();
		edits = null;
		body.replaceChildren(face);
	};
	const editIt = () => {
		if (edits) return;
		box.replaceChildren();
		edits = editForm(ctx, d, n, close, lib, save);
		body.replaceChildren(edits.el);
	};
	return h('section', { class: 'panel' },
		h('h2', null, n.fields.ssid?.value || n.id, n.fields.enabled?.value === false && h('span', { class: 'chip idle' }, 'off'),
			h('span', { class: 'note' }, 'from ', link(`/services/${encodeURIComponent(n.from)}`, ctx.name('services', n.from))),
			n.canEdit && save && h('span', { class: 'controls' },
				h('button', { type: 'button', class: 'button small', onclick: editIt }, 'Edit'),
				n.fields.ssid?.origin === 'self' && h('button', { type: 'button', class: 'button small danger', onclick: () => { close(); deleteNetwork(ctx, n, box); } }, 'Delete'))),
		body,
		n.fields.security?.value === 'wpa2-psk' && keysBlock(ctx, n),
		box);
}

// deleteNetwork deletes a network where it is defined: what its service
// folder sets for it is unset, in one change (0046, 0048). Every location
// that uses the folder stops offering it, and each AP removes the
// interfaces Aeolus made for it.
async function deleteNetwork(ctx, n, box) {
	const folderName = ctx.name('services', n.from);
	const own = Object.keys(n.fields).filter((k) => n.fields[k].origin === 'self').map((k) => `network.${n.id}.${k}`).sort();
	const op = { kind: 'unset', tree: 'services', node: n.from, paths: own };
	const p = await ask(box, op);
	if (!p) return;
	confirm(ctx, box, op, p, [
		h('div', null, h('strong', null, `Delete ${ssidOf(n)}`)),
		h('div', { class: 'sub' }, `It is removed from Services › ${folderName}, so every location that uses that folder stops offering it, and each AP listed removes its interfaces for it.`),
	], [
		h('div', { class: 'sub warn' }, 'Its clients on each AP listed are disconnected, and the other networks on those APs drop for a few seconds while the Wi-Fi restarts.'),
	]);
}

function view(ctx, n, bandsHere, live, lib, reports = [], isAP = false) {
	const f = (k) => n.fields?.[k]?.value;
	const asks = f('bands') || BANDS;
	const row = (label, v) => v != null && v !== '' && h('div', { class: 'row' }, h('div', { class: 'label' }, label), h('div', { class: 'value' }, v));
	// A transport, drawn (path.js): a VXLAN one from where its tunnel starts
	// to its far end, coloured by the probes of the APs here; a VLAN one on
	// the uplink.
	const transport = (slot) => {
		const type = f(`transport.${slot}.type`);
		if (!type) return null;
		if (type !== 'vxlan') return vlanPath(f(`transport.${slot}.vlan`));
		const id = f(`transport.${slot}.concentrator`), vni = f(`transport.${slot}.vni`), ask = f(`transport.${slot}.probe`);
		const t = lib.find((c) => c.id === id) || {};
		return vxlanPath({ start: t.underlay_vlan, tunnel: id, port: t.port, mtu: t.mtu, vnis: [{ vni, by: ask ? `asks ${ask}` : null }], address: t.address },
			probed(reports, [vni], isAP));
	};
	const primary = transport('primary'), fallback = transport('fallback');
	const switching = f('transport.switching') === 'automatic'
		? `switches automatically${f('transport.failback') === 'equal' ? '' : `, back after ${f('transport.holddown') ?? 300} s`}${f('transport.ha') ? ', HA' : ''}`
		: 'report only';
	const roaming = [f('roaming.ft') && '11r', f('roaming.rrm') && '11k', f('roaming.btm') && '11v'].filter(Boolean);
	const limits = [f('rate_limit.down_kbps') && `down ${f('rate_limit.down_kbps')} kbps`, f('rate_limit.up_kbps') && `up ${f('rate_limit.up_kbps')} kbps`].filter(Boolean);
	return [
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'On radios'),
			h('div', { class: 'value' }, BANDS.filter((b) => asks.includes(b)).map((b) => {
				// 6 GHz takes WPA3 and OWE only (0086).
				if (b === '6g' && !SIX_GHZ.has(f('security')))
					return h('span', { class: 'chip band none', title: '6 GHz takes WPA3 or OWE only' }, bandName(b) + (f('bands') ? ' (not allowed: WPA3 or OWE only)' : ' (not offered: WPA3 or OWE only)'));
				return bandsHere.has(b)
					? h('span', { class: 'chip band' }, bandName(b))
					: h('span', { class: 'chip band none', title: 'No radio here for this band' }, bandName(b) + ' (no radio here)');
			}))),
		row('Security', security(f('security'))),
		primary && h('div', { class: 'pathbox' },
			h('div', { class: 'label' }, fallback ? 'Travels over, first' : 'Travels over'), primary,
			fallback && [h('div', { class: 'label' }, 'then', h('span', { class: 'sub' }, ` · ${switching}`)), fallback]),
		row('Roaming', roaming.length ? roaming.join(', ') : 'off'),
		steeringRow(ctx, n, live),
		multicastRow(ctx, n, live),
		f('hidden') && row('Hidden', 'yes'),
		f('isolation') && row('Client isolation', 'on'),
		// Off the air on an AP that cannot reach its way on (0201).
		f('transport.down_when_lost') && row('When lost', 'off the air on an AP that cannot reach it'),
		limits.length > 0 && row('Rate limit', limits.join(', ')),
	];
}

const ssidOf = (n) => n.fields.ssid?.value || n.id;

// The security modes 6 GHz takes (0086): WPA3, alone or in transition, and
// OWE. A WPA2 or open network is not offered there.
const SIX_GHZ = new Set(['wpa3-sae', 'wpa2-wpa3', 'owe', 'wpa3-enterprise', 'wpa2-wpa3-enterprise']);

// tunnelName names a tunnel with its far end, where it is set here.
export function tunnelName(lib, id) {
	const t = lib.find((c) => c.id === id);
	return t?.address ? `${id} (${t.address})` : id;
}

// lockedAbove says whether a network's field is locked above its folder,
// so it cannot be changed there.
const lockedAbove = (n, k) => n.fields[k]?.origin === 'locked' && n.fields[k].from !== n.from;

// whence says where a value in force comes from, when it is not the
// network's own folder.
function whence(ctx, n, r) {
	return r && r.from !== n.from ? h('span', { class: 'sub' }, `from ${ctx.name('services', r.from)}`) : null;
}

const RESTART = 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.';

// The ways a network's clients are steered (0138), each a switch of its
// own, and what each does, said where it is switched.
const WAYS = [
	['suggest', 'Suggest a higher band',
		'A client that can use a higher band the AP has this network on, and is near enough for it, is asked to move there (802.11v). It may say no, and is asked three times at most while it stays joined.'],
	['hold_back', 'Hold back 2.4 GHz',
		'A client known to use a higher band is kept off 2.4 GHz for two and a half seconds as it comes to join, so that it finds the higher band first. Then 2.4 GHz is open to it again.'],
	['sticky', 'Move sticky clients',
		'A client another AP hears more strongly by its probes (5 dB unless set), or that says it hears the network there more strongly (802.11k, asked where it is weak and no other AP hears it), and heard under -65 dBm here, is told to go there; so is one whose signal stays weak while the network has a better place for it. One that stays is put off, and kept off this AP for ten seconds unless set. One that comes straight back is left alone for half an hour, and longer each time it does so again. The APs tell one another how they hear clients by radio resource management.'],
];

// switches is a network's steering and multicast to unicast (0051, 0138) as
// a part of the page's save, on its Services folder: want is what each is
// to be, by path, a value or UNSET; paint, what draws them again.
function switches(ctx, n, save) {
	const want = {};
	const paints = [];
	const prefix = `network.${n.id}.`;
	const folderName = ctx.name('services', n.from);
	const is = (k) => n.fields[k]?.value === true;
	// A way of steering, as set: band_steering is the older name of suggest
	// (0050), in force where suggest is not set.
	const wayOn = (key) => is(`steering.${key}`) || (key === 'suggest' && n.fields['steering.suggest'] === undefined && is('band_steering'));
	// 802.11k and 802.11v are on already, by the network's own settings or
	// by the other way that asks: turning this one on or off then changes
	// nothing an AP's Wi-Fi starts again for.
	const kv = (key) => (is('roaming.rrm') && is('roaming.btm')) || wayOn(key === 'suggest' ? 'sticky' : 'suggest');
	const mcKey = (v) => (v === true ? 'all' : v === false ? 'none' : 'default');
	const mcPath = `${prefix}multicast_to_unicast`;
	const live = {
		paint: () => paints.forEach((f) => f()),
		onPaint: (f) => paints.push(f),
		// on says whether a way is on, as drafted or set.
		on: (key) => (`${prefix}steering.${key}` in want ? want[`${prefix}steering.${key}`] === true : wayOn(key)),
		multicast: () => (mcPath in want ? mcKey(want[mcPath]) : mcKey(n.fields.multicast_to_unicast?.value)),
		// set drafts a value, or, where it is what is set, none.
		set(path, v) {
			const k = path.slice(prefix.length);
			const way = k.startsWith('steering.') && k.slice('steering.'.length);
			const same = way ? (v === true) === wayOn(way) : mcKey(v === UNSET ? undefined : v) === mcKey(n.fields[k]?.value);
			if (same) delete want[path];
			else want[path] = v;
		},
		changed: () => save.changed(part),
	};
	const part = {
		tree: 'services',
		node: n.from,
		changes: () => ({ ...want }),
		dirty: () => Object.keys(want).length > 0,
		undo: () => {
			for (const k of Object.keys(want)) delete want[k];
			live.paint();
		},
		// What the network's form sets of them, shown here too.
		take: (values) => {
			for (const [path, v] of Object.entries(values))
				if (path === mcPath || path.startsWith(`${prefix}steering.`)) live.set(path, v);
			live.paint();
		},
		summary: (values, seen) => [
			h('div', null, h('strong', null, `${ssidOf(n)} in Services › ${folderName}`)),
			h('ul', { class: 'becomes' }, Object.entries(values).map(([path, v]) => {
				const k = path.slice(prefix.length);
				const after = v === UNSET ? seen.after(path) : null;
				if (k === 'multicast_to_unicast') return h('li', null, `Multicast to unicast: ${MULTICAST.find(([x]) => x === mcKey(n.fields[k]?.value))[1]} → ${MULTICAST.find(([x]) => x === mcKey(v === UNSET ? undefined : v))[1]}`);
				const [key, label] = WAYS.find(([x]) => `steering.${x}` === k);
				return h('li', null, `${label}: ${wayOn(key) ? 'on' : 'off'} → ${(v === UNSET ? after?.value === true : v === true) ? 'on' : 'off'}`,
					after && after.from !== n.from ? ` (from ${ctx.name('services', after.from)})` : '');
			})),
			Object.entries(values).some(([path, v]) => v === true && !path.endsWith('.hold_back') && path !== mcPath && !kv(path.split('.').pop())) &&
				h('div', { class: 'sub' }, 'It also turns on 802.11k and 802.11v for the network, by which a client is asked.'),
			h('div', { class: 'sub' }, `This changes the network in Services › ${folderName}, for every location that uses it.`),
		],
		// The Wi-Fi restarts for multicast, and for a way that asks where
		// 802.11k and 802.11v are not on already.
		notes: (values) => (Object.keys(values).some((path) => path === mcPath || (!path.endsWith('.hold_back') && !kv(path.split('.').pop())))
			? [h('div', { class: 'sub warn' }, RESTART)] : []),
	};
	save.register(part);
	return live;
}

// steeringRow switches each way of steering for a network in its service
// folder (0138), a draft of the page's save: live, where it may be changed.
// A switch shows what is drafted, or in force. Turned off, what this folder
// set is unset, so it follows what is above; or, where it would then still
// be on, it is set off here.
function steeringRow(ctx, n, live) {
	const cell = h('div', { class: 'value ways' });
	const paint = () => cell.replaceChildren(...WAYS.map(([key, label, about]) => {
		const k = `steering.${key}`, path = `network.${n.id}.${k}`;
		const own = n.fields[k];
		const older = key === 'suggest' && n.fields.band_steering?.value === true;
		const field = own ?? (key === 'suggest' ? n.fields.band_steering : undefined);
		const on = live ? live.on(key) : field?.value === true;
		return h('span', { class: 'each' },
			h('button', {
				type: 'button', class: 'switch' + (on ? ' on' : ''), role: 'switch', 'aria-checked': String(on), 'aria-label': label,
				title: about, disabled: !live || lockedAbove(n, k), onclick: () => {
					live.set(path, on ? (own?.origin === 'self' && !older ? UNSET : false) : true);
					live.paint();
					live.changed();
				},
			}),
			h('span', { class: 'switchsays' }, label), ' ',
			whence(ctx, n, field));
	}));
	paint();
	live?.onPaint(paint);
	return h('div', { class: 'row' }, h('div', { class: 'label' }, 'Steering'), cell);
}

const MULTICAST = [
	['default', 'OpenWrt default', 'Only the groups clients joined (IGMP/MLD snooping)'],
	['all', 'All', 'All multicast (ARP, IPv4, IPv6), to each client as unicast'],
	['none', 'None', 'No multicast is sent as unicast'],
];

// multicastRow picks multicast-to-unicast for a network in its service
// folder (0049, 0051): OpenWrt's default (unset), all, or none; a draft of
// the page's save: live, where it may be changed.
function multicastRow(ctx, n, live) {
	const k = 'multicast_to_unicast';
	const path = `network.${n.id}.${k}`;
	const field = n.fields[k];
	const fixed = !live || lockedAbove(n, k);
	const seg = h('span', { class: 'segmented', role: 'group', 'aria-label': 'Multicast to unicast' });
	const paint = () => {
		const now = live ? live.multicast() : field?.value === true ? 'all' : field?.value === false ? 'none' : 'default';
		seg.replaceChildren(...MULTICAST.map(([key, text, help]) => h('button', {
			type: 'button', class: key === now ? 'on' : null, 'aria-pressed': String(key === now), title: help,
			// The default is unset here; a value set above cannot be unset here.
			disabled: key === now || fixed || (key === 'default' && field?.origin !== 'self'),
			onclick: () => {
				live.set(path, key === 'default' ? UNSET : key === 'all');
				live.paint();
				live.changed();
			},
		}, text)));
	};
	paint();
	live?.onPaint(paint);
	return h('div', { class: 'row' },
		h('div', { class: 'label' }, 'Multicast to unicast'),
		h('div', { class: 'value' }, seg, whence(ctx, n, field)));
}

// What a way of steering is called where its doings are shown, and what
// counts as its having worked: the client is on a higher band, or, for a
// weak one, off the band it was weak on.
const STEERED = {
	suggest: ['Suggest', ['moved'], 'moved up'],
	hold: ['Hold back', ['moved'], 'took a higher band'],
	sticky: ['Sticky clients', ['moved', 'left'], 'moved off'],
	keep: ['Keep on 6 GHz', ['stayed', 'left'], 'kept on 6 GHz'],
};
const ASKED = { btm: 'asked to move', held: 'held off', kick: 'put off', none: '—' };
// Why a sticky client was asked to go (0289).
const WHY = { weak: 'Sticky: weak', better: 'Sticky: heard better', home: 'Client steering: its APs', nearest: 'Client steering: nearest AP' };
const CAME = {
	suggest: { moved: 'moved up', refused: 'said no', stayed: 'stayed', left: 'left', back: 'came back down' },
	hold: { moved: 'joined a higher band', joined: 'joined 2.4 GHz all the same', left: 'did not join here', back: 'came back down' },
	sticky: { moved: 'moved to a better band here', left: 'left', back: 'came back', recovered: 'was heard well again', stayed: 'stayed' },
	keep: { stayed: 'kept, and let go on 6 GHz', left: 'kept until it left' },
};

// What came of an asking, by where its client was let on next (0291):
// each a chip's look and what it says. Set against how its old AP heard it
// on the same band only: on another band it is not compared.
const RESULT = {
	stronger: ['ok', 'heard better there'], weaker: ['warn', 'heard worse there'], alike: ['idle', 'heard alike there'],
	band: ['idle', 'on another band'], back: ['warn', 'back where it was'], unknown: ['idle', 'how well not known'],
};
// then is where an asking's client was let on next, and how strongly it was
// heard there, beside how its old AP heard it.
const then = (e) => {
	const x = e.after;
	if (!x) return h('span', { class: 'sub' }, '—');
	const was = e.signal != null && (x.band === e.from ? ` (was ${e.signal})` : ` (was ${e.signal} on ${bandName(e.from)})`);
	return [`→ ${x.ap_name || 'another AP'} ${bandName(x.band)}${x.signal != null ? ` ${x.signal} dBm` : ''}`,
		was && h('span', { class: 'sub', title: RESULT[e.result]?.[1] || '' }, was)];
};
// results is, of the askings weighed, how many of their clients were heard
// better and worse where they went: one chip, the rest in its title.
const results = (r) => {
	const n = Object.values(r || {}).reduce((sum, v) => sum + v, 0);
	if (!n) return null;
	const better = r.stronger || 0, worse = r.weaker || 0;
	return h('span', { class: 'chip ' + (worse > better ? 'warn' : better ? 'ok' : 'idle'), title: Object.entries(r).map(([k, v]) => `${v} ${RESULT[k]?.[1] || k}`).join(', ') },
		`Where they went: ${better} heard better, ${worse} worse`);
};

// The times steering is counted over, in hours.
const STEERING_HOURS = [[24, 'A day'], [168, 'A week'], [720, '30 days']];
const steeringOver = (id, hours) => get(`/v1/steering?under=${encodeURIComponent(id)}&hours=${hours}`);

// steeringPanel shows whether steering works here (0139): for each steered
// network, how many of the clients asked by each way then moved, over a
// day, a week or a month, as the manager counts the askings the APs say.
// Which ways are on, and where steering is not running, is as each AP last
// reported (0138). The rest is a click down: the same an AP at a time, the
// clients asked most, and the latest askings.
function steeringPanel(id, reports, nets, first) {
	const ssid = (n) => nets.find((x) => x.id === n)?.fields.ssid?.value || n;
	// What the APs say now: the ways on for each network, and the APs whose
	// config has them steer and whose steering is not running.
	const on = new Map(), stopped = [];
	for (const { ap, cfg } of reports) {
		const s = cfg?.condition?.state?.report?.steer;
		if (!s) continue;
		if (!s.running) {
			stopped.push(ap);
			continue;
		}
		for (const n of s.networks || []) {
			const ways = on.get(n.network) || new Set();
			for (const w of Object.keys(STEERED)) if (n[w]) ways.add(w);
			if (ways.size) on.set(n.network, ways);
		}
	}
	if (!on.size && !stopped.length && !first?.networks?.length) return null;
	const chips = (counts, ways) => Object.keys(STEERED).filter((w) => ways?.has(w) || counts?.[w]).map((w) => {
		const [name, good, did] = STEERED[w], c = counts?.[w] || {};
		const asked = c.asked || 0, worked = good.reduce((sum, g) => sum + (c[g] || 0), 0);
		if (!asked) return h('span', { class: 'chip idle', title: 'It asked no client in this time' }, `${name}: none asked`);
		return h('span', { class: 'chip ' + (worked * 2 >= asked ? 'ok' : 'warn'), title: Object.entries(c).filter(([k]) => k !== 'asked').map(([k, v]) => `${v} ${CAME[w][k] || k}`).join(', ') },
			`${name}: ${worked} of ${asked} ${did}${c.back ? `, ${c.back} came back` : ''}`);
	});
	const box = h('section', { class: 'panel' });
	const client = (c) => clientLink(c.mac, called(c.mac, c.host) || c.mac);
	const fold = (title, table) => h('details', { class: 'fold' }, h('summary', null, title), table);
	let hours = STEERING_HOURS[0][0], asked = 0;
	const pick = async (v) => {
		hours = v;
		// Only the latest asking is drawn: an answer to an earlier choice
		// that comes in late does not replace it.
		const mine = ++asked;
		const got = await steeringOver(id, v).catch(() => null);
		if (mine === asked) draw(got);
	};
	const draw = (v) => {
		const counted = new Map((v?.networks || []).map((n) => [n.network, n]));
		const ids = [...new Set([...on.keys(), ...counted.keys()])].sort((x, y) => ssid(x).localeCompare(ssid(y)));
		const rows = [];
		for (const n of ids) {
			const c = counted.get(n);
			rows.push(h('tr', null, h('td', null, ssid(n)), h('td', null, h('div', { class: 'ways' }, chips(c?.counts, on.get(n)), results(c?.results)))));
			if (c?.aps.length > 1)
				rows.push(h('tr', { class: 'under' }, h('td', { colspan: 2 }, h('details', null,
					h('summary', null, `On each of the ${c.aps.length} APs that asked`),
					h('table', { class: 'list' }, c.aps.map((a) => h('tr', null,
						h('td', null, link(`/aps/${encodeURIComponent(a.ap)}`, a.name)),
						h('td', null, h('div', { class: 'ways' }, chips(a.counts), results(a.results))))))))));
		}
		const latest = v?.latest || [], most = v?.clients || [];
		// Which network an asking was of is said where there is more than one.
		const several = ids.length > 1;
		// A part there is none of is left out: replaceChildren would write its false.
		box.replaceChildren(...[
			h('h2', null,
				h('span', { class: 'title' }, 'Steering', h('span', { class: 'segmented', role: 'group', 'aria-label': 'Over how long' }, STEERING_HOURS.map(([t, name]) => h('button', {
					type: 'button', class: hours === t ? 'on' : '', 'aria-pressed': hours === t, onclick: () => pick(t),
				}, name)))),
				h('span', { class: 'note' }, 'what was asked of clients, and what came of it')),
			!v && h('div', { class: 'banner warn' }, 'What steering asked of clients could not be read.'),
			rows.length > 0 && h('table', { class: 'list' },
				h('tr', null, ['Network', 'What came of it'].map((t) => h('th', null, t))),
				rows),
			stopped.length > 0 && h('p', { class: 'sub pad' }, h('span', { class: 'chip warn' }, 'not running'), ' on ',
				stopped.map((ap, i) => [i > 0 && ', ', link(`/aps/${encodeURIComponent(ap.id)}`, ap.name)]),
				': clients join there as on a network that is not steered.'),
			most.length > 0 && fold(`The ${most.length === 1 ? 'client' : `${most.length} clients`} asked most`, h('table', { class: 'list' },
				h('tr', null, ['Client', 'Asked', 'Moved', 'Said no', 'Came back', 'Last asked'].map((t) => h('th', null, t))),
				most.map((c) => h('tr', null,
					h('td', null, client(c)),
					h('td', null, String(c.asked)),
					h('td', null, String(c.moved)),
					h('td', null, c.refused ? String(c.refused) : h('span', { class: 'sub' }, '0')),
					h('td', null, c.back ? String(c.back) : h('span', { class: 'sub' }, '0')),
					h('td', { class: 'sub nowrap' }, ago(c.last)))))),
			latest.length > 0 && fold(`The latest ${latest.length === 1 ? 'asking' : `${latest.length} askings`}`, h('table', { class: 'list' },
				h('tr', null, ['When', 'AP', 'Client', several && 'Network', 'Way', 'It was', 'On', 'It answered', 'What came of it', 'Then', 'After'].filter(Boolean).map((t) => h('th', null, t))),
				latest.map((e) => h('tr', null,
					h('td', { class: 'sub nowrap', title: when(e.at) }, ago(e.at)),
					h('td', { class: 'nowrap' }, e.ap_name),
					h('td', { class: 'nowrap' }, client(e)),
					several && h('td', null, ssid(e.network)),
					h('td', { class: 'nowrap' }, (e.method === 'sticky' && WHY[e.why]) || STEERED[e.method]?.[0] || e.method),
					h('td', { class: 'nowrap' }, ASKED[e.asked] ?? e.asked),
					// For one heard better elsewhere, that AP and how it heard it (0289).
					h('td', { class: 'nowrap' }, [bandName(e.from), e.signal != null && `, ${e.signal} dBm`,
						e.elsewhere && h('span', { class: 'sub' }, `; ${e.elsewhere.name}${e.elsewhere.signal != null ? ` ${e.elsewhere.signal} dBm` : ''}`)]),
					h('td', null, e.answer === 'accepted' ? 'yes' : e.answer === 'refused' ? `no${e.code != null ? ` (${e.code})` : ''}` : '—'),
					h('td', null, CAME[e.method]?.[e.outcome] || e.outcome),
					// Where it was let on next, and how well (0291).
					h('td', { class: 'nowrap' }, then(e)),
					h('td', { class: 'nowrap' }, `${e.took} s`)))))].filter(Boolean));
	};
	draw(first);
	return box;
}

// How each verdict of a network's transport is shown (0059, 0061).
const VERDICT = {
	up: ['ok', 'answering'], down: ['bad', 'down'], unverified: ['warn', 'unverified'],
	unknown: ['idle', 'starting'], off: ['idle', 'standing by'],
};

// transportsStatus shows, for each network with a fallback on each AP here,
// which transport carries it and how each is doing, by the AP's prober
// (0061): a VLAN transport probed on the uplink, a VXLAN one on its tunnel.
// Under it, the last switch, or why a switch that is due cannot be made.
function transportsStatus(reports) {
	const rows = [];
	for (const { ap, cfg } of reports) {
		const st = cfg?.condition?.state;
		const report = st?.report;
		const vlans = new Map((report?.vlan_probes || []).map((v) => [v.vlan, v.probe]));
		const tunnels = new Map((report?.vxlan?.tunnels || []).map((t) => [t.vni, t.probe]));
		for (const [id, s] of Object.entries(report?.transports || {}).sort()) {
			const n = cfg.document?.network?.[id];
			const how = (slot) => {
				const t = n?.transport?.[slot];
				const [cls, word] = VERDICT[s[slot]] || ['idle', s[slot] || 'not reported'];
				const p = t?.type === 'vlan' ? vlans.get(t.vlan) : t?.type === 'vxlan' ? tunnels.get(t.vni) : null;
				return [
					h('span', { class: 'chip ' + cls }, `${!t ? slot : t.type === 'vlan' ? 'VLAN ' + t.vlan : 'VNI ' + t.vni}: ${word}`),
					p?.lease && h('div', { class: 'sub' }, `leased ${p.lease.address}${p.lease.server ? ' from ' + p.lease.server : ''}`),
				];
			};
			rows.push(h('tr', null,
				h('td', null, rows.length === 0 || rows[rows.length - 1].dataset.ap !== ap.id ? link(`/aps/${encodeURIComponent(ap.id)}`, ap.name) : null),
				h('td', null, n?.ssid || id),
				h('td', null,
					s.active === 'none' ? h('span', { class: 'chip bad' }, 'nothing') : s.active === 'fallback' ? h('span', { class: 'chip warn' }, 'fallback') : s.active,
					s.cannot_switch ? h('div', { class: 'sub' }, `cannot switch: ${s.cannot_switch}`)
						: s.last_switch && h('div', { class: 'sub' },
							`to ${s.last_switch.to} ${secondsAgo(s.last_switch.ago, st.at)}: ${s.last_switch.why}`)),
				h('td', null, how('primary')),
				h('td', null, how('fallback')),
				h('td', null, ago(st.at))));
			rows[rows.length - 1].dataset.ap = ap.id;
		}
	}
	if (!rows.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Transports on each AP', h('span', { class: 'note' }, 'networks with a fallback')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Network', 'Carried by', 'Primary', 'Fallback', 'Reported'].map((c) => h('th', null, c))),
			rows));
}

// How each verdict of a network's RADIUS server is shown (0114).
const RADIUS = {
	up: ['ok', 'answering'], silent: ['bad', 'not answering'], unverified: ['idle', 'not checked'], unknown: ['idle', 'starting'],
};

// radiusStatus shows, for each network on each AP here that signs clients
// in against a RADIUS server, how the server is doing from that AP (0114):
// whether it answers the AP's status requests or its clients' sign-ins, how
// fast and how long ago, and what it answered the sign-ins with since the
// network was started. A server that doesn't answer is in red.
function radiusStatus(reports) {
	const rows = [];
	for (const { ap, cfg } of reports) {
		const st = cfg?.condition?.state;
		for (const r of st?.report?.radius || []) {
			const [cls, word] = RADIUS[r.verdict] || ['idle', r.verdict || 'not reported'];
			rows.push(h('tr', null,
				h('td', null, rows.length === 0 || rows[rows.length - 1].dataset.ap !== ap.id ? link(`/aps/${encodeURIComponent(ap.id)}`, ap.name) : null),
				h('td', null, cfg.document?.network?.[r.network]?.ssid || r.network),
				h('td', null, r.port === 1812 ? r.server : `${r.server} port ${r.port}`),
				h('td', null,
					h('span', { class: 'chip ' + cls, title: RADIUS_SAYS[r.verdict] || '' }, word),
					r.verdict === 'silent' && h('div', { class: 'sub' }, 'down, out of reach, or it does not know this AP or its secret')),
				h('td', null, r.answered_ago == null ? '—' : `${secondsAgo(r.answered_ago, st.at)}${r.rtt_ms != null ? `, in ${r.rtt_ms} ms` : ''}`),
				h('td', null, r.requests
					? [`${r.accepts} accepted, ${r.rejects} refused`,
						(r.timeouts > 0 || r.bad > 0) && h('div', { class: 'sub' },
							[r.timeouts > 0 && `${r.timeouts} timed out`, r.bad > 0 && `${r.bad} bad answer${r.bad === 1 ? '' : 's'}`].filter(Boolean).join(', '))]
					: '—'),
				h('td', null, ago(st.at))));
			rows[rows.length - 1].dataset.ap = ap.id;
		}
	}
	if (!rows.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'RADIUS on each AP', h('span', { class: 'note' }, 'the server each network signs clients in against; sign-ins are counted since the network started')),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Network', 'Server', 'Status', 'Last answer', 'Sign-ins', 'Reported'].map((c) => h('th', null, c))),
			rows));
}

// What each verdict means, on mouse-over.
const RADIUS_SAYS = {
	up: "It answers the AP's status requests, or answered a client's sign-in in the last minute and a half.",
	silent: "Three status requests in a row went unanswered, or a client's sign-in timed out and none was answered since.",
	unverified: "It takes no status requests, and no client has signed in lately: nothing says it is down.",
	unknown: 'The AP has not asked it three times yet.',
};

// dhcpStatus shows, for each network on each AP here, what its Wi-Fi
// clients' DHCP looks like (0065): the servers that answer, the requests of
// the last 10 minutes, answered or not, and the clients that don't use DHCP,
// with the address each shows. A warning is in red.
function dhcpStatus(reports) {
	const rows = [];
	for (const { ap, cfg } of reports) {
		const st = cfg?.condition?.state;
		for (const [id, d] of Object.entries(st?.report?.dhcp || {}).sort()) {
			const name = cfg.document?.network?.[id]?.ssid || id;
			const warn = dhcpWarnings(name, d);
			const dup = new Set((d.duplicates || []).flatMap((x) => x.servers));
			rows.push(h('tr', null,
				h('td', null, rows.length === 0 || rows[rows.length - 1].dataset.ap !== ap.id ? link(`/aps/${encodeURIComponent(ap.id)}`, ap.name) : null),
				h('td', null, name),
				h('td', null, (d.servers || []).map((s) => h('div', null,
					h('span', { class: 'chip ' + (dup.size > 1 && dup.has(s.id) ? 'bad' : 'ok'), title: `${s.answers} answers, last ${s.ago} s ago` }, s.id), ` ${s.mac}`))),
				h('td', null, d.unanswered > 0 && !d.answered
					? h('span', { class: 'chip bad' }, `none of ${d.unanswered} answered`)
					: d.answered || d.unanswered ? `${d.answered} answered${d.unanswered ? `, ${d.unanswered} not` : ''}` : '—'),
				h('td', null, (d.without || []).map((c) => h('div', null,
					h('span', { class: 'chip ' + (c.address ? 'idle' : 'bad') }, c.address ? `static ${c.address}` : 'no address'), ` ${c.mac}`))),
				h('td', { title: warn.join('\n') }, ago(st.at))));
			rows[rows.length - 1].dataset.ap = ap.id;
		}
	}
	if (!rows.length) return null;
	return h('section', { class: 'panel' },
		h('h2', null, 'DHCP on each AP', h('span', { class: 'note' }, "its Wi-Fi clients' DHCP")),
		h('table', { class: 'list' },
			h('tr', null, ['AP', 'Network', 'Servers', 'Requests, 10 min', 'Without DHCP', 'Reported'].map((c) => h('th', null, c))),
			rows));
}

// relayStatus shows, for each subnet a DHCP relay copies the manager
// requests from (0068): the relay, the requests of the last 10 minutes, the
// clients and how many are new, a burst of new ones, and those that ask for
// what an OpenWiFi AP asks for. The relay's address on the subnet names it.
function relayStatus(relayed) {
	if (!relayed?.subnets.length) return null;
	const rows = relayed.subnets.map((s) => {
		const openwifi = s.clients.filter((c) => c.openwifi).length;
		const kinds = {};
		for (const c of s.clients) kinds[c.kind || 'unknown'] = (kinds[c.kind || 'unknown'] || 0) + 1;
		return h('tr', null,
			h('td', { class: 'mono' }, s.subnet),
			h('td', { class: 'mono' }, s.relay),
			h('td', null, s.requests ? `${s.requests}` : '—'),
			h('td', null, `${s.count}${s.new ? `, ${s.new} new` : ''}`,
				h('div', { class: 'sub' }, Object.entries(kinds).sort((a, b) => b[1] - a[1]).slice(0, 4).map(([k, n]) => `${n} ${k}`).join(' · '))),
			h('td', null,
				s.burst && h('div', null, h('span', { class: 'chip bad', title: 'more than 64 new clients sent their first discover within a minute: a sign of DHCP starvation' }, `${s.burst.clients} new in a minute`), ` ${ago(s.burst.at)}`),
				openwifi > 0 && h('div', null, h('span', { class: 'chip warn', title: 'their DHCP asks for options 138 and 224; see APs' }, `${openwifi} possible OpenWiFi ${openwifi === 1 ? 'AP' : 'APs'}`)),
				!s.burst && !openwifi && '—'),
			h('td', null, s.clients[0] ? ago(s.clients[0].last) : '—'));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'DHCP from relays', h('span', { class: 'note' }, `clients' requests that relays copy to the manager; their answers aren't copied${relayed.ignored ? ` · ${relayed.ignored} other packets ignored` : ''}`)),
		h('table', { class: 'list' },
			h('tr', null, ['Subnet, by its relay address', 'Relay', 'Requests, 10 min', 'Clients', 'Findings', 'Last request'].map((c) => h('th', null, c))),
			rows));
}

// The fields a transport's type shows: a VLAN's ID, or a VXLAN's
// concentrator, VNI and probe address (0059). The others stay hidden, and
// are not sent.
const FOR_TYPE = { vlan: ['vlan'], vxlan: ['concentrator', 'vni', 'probe'] };

// What only means something once there is a fallback transport, and with
// automatic switching (0022, 0061).
const WITH_FALLBACK = ['transport.switching'];
const WITH_AUTOMATIC = ['transport.ha', 'transport.failback', 'transport.holddown'];

// canonIP writes an IP address one way, so two spellings of one compare
// equal, as the manager's net.IP.Equal does (Codex's review of #93): IPv6
// in full, lower case, without leading zeros, and an IPv4-mapped one as
// IPv4. Anything it can't read stays as it was.
function canonIP(s) {
	let a = String(s).trim().replace(/^\[|\]$/g, '').toLowerCase();
	if (!a.includes(':')) return a;
	let tail = [];
	const v4 = a.match(/^(.*:)(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/);
	if (v4) {
		const b = v4.slice(2).map(Number);
		if (b.some((x) => x > 255)) return a;
		tail = [((b[0] << 8) | b[1]).toString(16), ((b[2] << 8) | b[3]).toString(16)];
		a = v4[1].endsWith('::') ? v4[1] : v4[1].slice(0, -1);
	}
	const halves = a.split('::');
	if (halves.length > 2) return a;
	const head = halves[0] ? halves[0].split(':') : [];
	const rest = halves.length === 2 && halves[1] ? halves[1].split(':') : [];
	const fill = 8 - head.length - rest.length - tail.length;
	if (halves.length === 2 ? fill < 1 : fill !== 0) return a;
	const groups = [...head, ...Array(halves.length === 2 ? fill : 0).fill('0'), ...rest, ...tail];
	if (groups.some((g) => !/^[0-9a-f]{1,4}$/.test(g))) return a;
	const full = groups.map((g) => parseInt(g, 16));
	if (full.slice(0, 5).every((g) => g === 0) && full[5] === 0xffff)
		return [full[6] >> 8, full[6] & 255, full[7] >> 8, full[7] & 255].join('.');
	return full.map((g) => g.toString(16)).join(':');
}

// farEnd is where a tunnel goes, to compare two: its address where it is
// set here, else its name.
function farEnd(lib, id) {
	const a = lib.find((t) => t.id === id)?.address;
	return a ? `address ${canonIP(a)}` : `tunnel ${id}`;
}

// pickers swaps a transport's tunnel field for a list of the tunnels set
// here (0055). A tunnel the list lacks, set where it was, stays on offer,
// marked.
function pickers(prefix, rows, inputs, lib) {
	for (const slot of ['primary', 'fallback']) {
		const path = `${prefix}transport.${slot}.concentrator`;
		const row = rows.get(path);
		if (!row) continue;
		const cur = row.it.read();
		const sel = h('select', null,
			h('option', { value: '' }, lib.length ? '—' : 'no tunnel is set here yet (Wired › Tunnels)'),
			lib.map((t) => h('option', { value: t.id, selected: t.id === cur, 'data-name': tunnelName(lib, t.id) }, tunnelName(lib, t.id))),
			cur && !lib.some((t) => t.id === cur) && h('option', { value: cur, selected: true, 'data-name': `${cur} (not set here)` }, `${cur} (not set here)`));
		const read = () => sel.value || undefined;
		const initial = JSON.stringify(read());
		const it = { el: sel, read, changed: () => JSON.stringify(read()) !== initial };
		sel.disabled = row.it.el.disabled;
		row.it.el.replaceWith(sel);
		row.it = it;
		if (inputs.has(path)) inputs.set(path, it);
	}
}

// form lays out an input for every network field the schema has, with the
// values now in fields ({field: {value, from, origin}}). Fields locked
// above folder cannot be changed there. A transport shows only the fields
// its type uses; sync shows the right ones after a type is set, and the
// switching settings only with a fallback. A VXLAN transport's tunnel is
// picked from lib, the tunnels set here.
function form(d, net, fields, folder, lib, names) {
	const prefix = `network.${net}.`;
	const keys = Object.keys(d.fields).filter((k) => k.startsWith('network.*.')).map((k) => k.slice('network.*.'.length));
	const placed = new Set(SECTIONS.flatMap(([, ks]) => ks));
	const sections = [...SECTIONS, [MORE, keys.filter((k) => !placed.has(k)).sort()]]
		.map(([title, ks]) => [title, ks.map((k) => prefix + k)]);
	const byPath = Object.fromEntries(Object.entries(fields).map(([k, r]) => [prefix + k, r]));
	const { body, inputs, rows } = fieldsForm(d, sections, byPath, folder, MORE);
	pickers(prefix, rows, inputs, lib);
	const typeOf = (slot) => rows.get(`${prefix}transport.${slot}.type`)?.it.el.value;
	// A network may have no fallback: None, beside VLAN and VXLAN (Griff,
	// 2026-10-11). The list had it only for a network that had none yet, as
	// a dash, so a fallback once given could not be taken away. Picked for
	// a network that has one, it takes away what this folder sets of the
	// fallback, and of how the two transports are switched between, which
	// means nothing without one: automatic switching left behind has every
	// AP's config held (0061).
	const fbType = rows.get(`${prefix}transport.fallback.type`)?.it.el;
	const typeField = fields['transport.fallback.type'];
	const hadFallback = typeField?.value != null;
	// Set in this folder, locked there or not: a lock here is this folder's.
	const here = (k) => !!fields[k] && fields[k].from === folder && (fields[k].origin === 'self' || fields[k].origin === 'locked');
	const goes = Object.keys(fields).filter((k) => (k.startsWith('transport.fallback.') || WITH_FALLBACK.includes(k) || WITH_AUTOMATIC.includes(k)) && here(k));
	const where = (id) => (names ? `Services › ${names(id)}` : 'a folder above');
	// Kept from when a folder broke hierarchy, this one or one above: that
	// folder's own copy of what was above it, which can be unset nowhere.
	const kept = (k) => (fields[k]?.origin !== 'baseline' ? null
		: `kept from when ${fields[k].from === folder ? 'this folder' : where(fields[k].from)} broke hierarchy`);
	// Why it cannot be taken away in this folder, where it cannot: it is
	// set above, or kept from above; or switching is automatic by a folder
	// above, which needs a fallback, so the config would be held.
	const sw = fields['transport.switching'];
	const stuck = !hadFallback ? null
		: kept('transport.fallback.type') ? `${kept('transport.fallback.type')}: it cannot be taken away`
			: !here('transport.fallback.type') ? `it is set in ${where(typeField.from)}`
				: sw?.value === 'automatic' && !here('transport.switching')
					? `switching is automatic${kept('transport.switching') ? `, ${kept('transport.switching')}` : ` in ${where(sw.from)}`}, which needs a fallback`
					: null;
	// What is left of a fallback that has no type, set in this folder: a
	// VLAN, a tunnel. Cleared with any other change: see editForm.
	const strays = hadFallback ? [] : goes.filter((k) => k.startsWith('transport.fallback.'));
	if (fbType) {
		let none = [...fbType.options].find((o) => o.value === '');
		if (!none) {
			none = h('option', { value: '' });
			fbType.prepend(none);
		}
		none.textContent = stuck ? `None (${stuck})` : 'None';
		none.disabled = !!stuck;
	}
	// The kinds of transport, named as everywhere else.
	for (const slot of ['primary', 'fallback'])
		for (const o of rows.get(`${prefix}transport.${slot}.type`)?.it.el.options || [])
			if (o.value) o.textContent = value(`${prefix}transport.${slot}.type`, o.value);
	// dropped is what to unset, where the fallback is None: for a network
	// that has a fallback this folder may take away, what the folder sets
	// of it and of the switching; for one that has none, what was left of
	// one here.
	const dropped = () => (!fbType || fbType.value !== '' || stuck ? [] : (hadFallback ? goes : strays).map((k) => prefix + k));
	// A VXLAN fallback can't go to the primary's far end, nor the primary to
	// the fallback's: each list greys out the other's (Griff, 2026-10-06).
	const tunnelOf = (slot) => (typeOf(slot) === 'vxlan' ? rows.get(`${prefix}transport.${slot}.concentrator`)?.it.read() : undefined);
	const guard = () => {
		for (const [slot, other] of [['primary', 'fallback'], ['fallback', 'primary']]) {
			const sel = rows.get(`${prefix}transport.${slot}.concentrator`)?.it.el;
			const theirs = tunnelOf(other);
			for (const o of sel?.options || []) {
				const same = Boolean(o.value && theirs && farEnd(lib, o.value) === farEnd(lib, theirs));
				o.disabled = same && !o.selected;
				o.textContent = same ? `${o.dataset.name} — the ${other}'s far end` : o.dataset.name || o.textContent;
			}
		}
	};
	// A VXLAN transport needs its tunnel (Griff, 2026-10-06): picking VXLAN
	// picks the first tunnel set here that isn't the other's far end.
	const fill = (slot) => {
		const sel = rows.get(`${prefix}transport.${slot}.concentrator`)?.it.el;
		if (typeOf(slot) !== 'vxlan' || !sel || sel.value) return;
		const first = [...sel.options].find((o) => o.value && !o.disabled);
		if (first) sel.value = first.value;
	};
	// clash says why the transports can't be sent as they are, if they
	// can't: a VXLAN one without its tunnel or VNI, or the two to one far
	// end.
	const clash = () => {
		for (const slot of ['primary', 'fallback'])
			if (typeOf(slot) === 'vxlan') {
				if (!tunnelOf(slot))
					return lib.length ? `Pick the ${slot}'s tunnel: a VXLAN transport needs one.` : 'No tunnel is set here: set one in Wired › Tunnels first.';
				if (rows.get(`${prefix}transport.${slot}.vni`)?.it.read() == null) return `Give the ${slot} a VNI: a VXLAN transport needs one.`;
			}
		const p = tunnelOf('primary'), f = tunnelOf('fallback');
		return p && f && farEnd(lib, p) === farEnd(lib, f)
			? `The fallback goes to the same far end as the primary (${farEnd(lib, f)}), so it would fail with it. Pick a tunnel to another far end, or a VLAN fallback.`
			: null;
	};
	const show = (k, on) => {
		const r = rows.get(prefix + k)?.row;
		if (r) r.hidden = !on;
	};
	const sync = () => {
		for (const slot of ['primary', 'fallback'])
			for (const [t, ks] of Object.entries(FOR_TYPE))
				for (const k of ks) show(`transport.${slot}.${k}`, typeOf(slot) === t);
		const fallback = Boolean(typeOf('fallback'));
		for (const k of WITH_FALLBACK) show(k, fallback);
		const automatic = rows.get(`${prefix}transport.switching`)?.it.el.value === 'automatic';
		for (const k of WITH_AUTOMATIC) show(k, fallback && automatic);
	};
	for (const k of ['transport.primary.type', 'transport.fallback.type', 'transport.switching'])
		rows.get(prefix + k)?.it.el.addEventListener('change', sync);
	for (const k of ['transport.primary.type', 'transport.fallback.type', 'transport.primary.concentrator', 'transport.fallback.concentrator'])
		rows.get(prefix + k)?.it.el.addEventListener('change', guard);
	for (const slot of ['primary', 'fallback'])
		rows.get(`${prefix}transport.${slot}.type`)?.it.el.addEventListener('change', () => { guard(); fill(slot); guard(); });
	// 6 GHz takes WPA3 or OWE only (0086): picking WPA2 or open unticks 6 GHz
	// and holds it off, saying why; picking WPA3 or OWE gives it back as it
	// was (Griff, 2026-10-09).
	const security = rows.get(`${prefix}security`)?.it.el;
	const bands = rows.get(`${prefix}bands`);
	const six = bands?.it.el.querySelector('input[value="6g"]');
	if (security && six) {
		const why = h('span', { class: 'sub warn', hidden: true }, '6 GHz takes WPA3 or OWE only');
		bands.row.append(why);
		let had = six.checked;
		six.addEventListener('change', () => { had = six.checked; });
		const legal = () => {
			const ok = !security.value || SIX_GHZ.has(security.value);
			if (!ok && !six.disabled) {
				had = six.checked;
				six.checked = false;
				six.disabled = true;
			} else if (ok && six.disabled) {
				six.disabled = false;
				six.checked = had;
			}
			why.hidden = ok;
		};
		security.addEventListener('change', legal);
		legal();
		// Where nobody set the bands, the network is on all it may be:
		// 6 GHz unticked by the form, for a security it does not take, is
		// no change of the person's, and was listed as one.
		if (fields.bands === undefined) {
			const read = bands.it.read;
			bands.it.changed = () => {
				try {
					return JSON.stringify(read()) !== JSON.stringify(BANDS.filter((b) => b !== '6g' || !six.disabled));
				} catch {
					return true;
				}
			};
		}
	}
	// A passphrase for the PSK modes; a RADIUS server for the enterprise ones
	// (0098), and for another network once MAC authentication is ticked
	// (0112), which only such a network offers. A server's disconnects are
	// WPA Enterprise's alone (0111). With MAC authentication, a WPA2
	// network may take each device's passphrase from the server (0115), and
	// then has no use for one of its own. Each shows only where it is used.
	if (security) {
		const mac = rows.get(`${prefix}radius.mac_auth`)?.row.querySelector('input[type=checkbox]');
		const each = rows.get(`${prefix}radius.passphrases`)?.row.querySelector('input[type=checkbox]');
		const kinds = () => {
			const enterprise = security.value.endsWith('-enterprise');
			const psk = ['wpa2-psk', 'wpa3-sae', 'wpa2-wpa3'].includes(security.value);
			const asked = enterprise || !!mac?.checked;
			const perDevice = !!mac?.checked && security.value === 'wpa2-psk';
			for (const [path, r] of rows) {
				if (path === `${prefix}radius.mac_auth`) r.row.hidden = enterprise;
				else if (path === `${prefix}radius.passphrases`) r.row.hidden = !perDevice;
				else if (path.startsWith(`${prefix}radius.das.`) || path.startsWith(`${prefix}radius.test.`)) r.row.hidden = !enterprise;
				else if (path.startsWith(`${prefix}radius.`)) r.row.hidden = !asked;
				if (path === `${prefix}passphrase`) r.row.hidden = (!psk && security.value !== '') || (perDevice && !!each?.checked);
			}
		};
		security.addEventListener('change', kinds);
		mac?.addEventListener('change', kinds);
		each?.addEventListener('change', kinds);
		kinds();
	}
	sync();
	guard();
	return { body, inputs, rows, sync, clash, dropped };
}

function setOp(folder, values) {
	const paths = Object.keys(values);
	return paths.length === 1
		? { kind: 'set', tree: 'services', node: folder, path: paths[0], value: values[paths[0]] }
		: { kind: 'set', tree: 'services', node: folder, values };
}

// shown writes a value for the preview; a secret is never shown.
// SECRET is the network fields whose values are never shown (0027): the
// passphrase, and a RADIUS server's secrets (0098).
const SECRET = /\.(passphrase|radius\.(auth|acct)_secret|radius\.das\.secret|radius\.test\.password)$/;

function shown(path, v) {
	if (!SECRET.test(path)) return value(path, v);
	return path.endsWith('.passphrase') ? 'a new passphrase' : 'a new secret';
}

// leftovers is what a change leaves set on a network that the form no
// longer shows, and that would then have it refused (0112): MAC
// authentication once the network is WPA Enterprise, which signs each
// client in already; and the RADIUS server's VLANs once nothing asks the
// server. They are cleared with the change that hides them.
function leftovers(n, values) {
	const path = (k) => `network.${n.id}.${k}`;
	const now = (k) => (path(k) in values ? values[path(k)] : n.fields[k]?.value);
	const enterprise = String(now('security') || '').endsWith('-enterprise');
	const out = {};
	if (enterprise && now('radius.mac_auth') === true) out[path('radius.mac_auth')] = false;
	// Passphrases from the server go with MAC authentication on WPA2 (0115).
	if (now('radius.passphrases') === true && (now('security') !== 'wpa2-psk' || now('radius.mac_auth') !== true || out[path('radius.mac_auth')] === false))
		out[path('radius.passphrases')] = false;
	if (!enterprise && now('radius.mac_auth') !== true) {
		if ((now('radius.vlans') || []).length) out[path('radius.vlans')] = [];
		if (now('radius.vlan_required') === true) out[path('radius.vlan_required')] = false;
	}
	return out;
}

// editForm is a network's form, a part of the page's save on its Services
// folder (0204), until it is cancelled, or the page's Undo closes it. Its
// fallback taken away (None) unsets what this folder sets of it, in the
// same change as the rest: Switching set to report here stays, as unset it
// would be what is above, which may be automatic and need a fallback. What
// was left of a fallback with no type goes with any other change: the form
// rests on None for every network without one. It returns the form, el, and
// off(), which takes its draft off the page.
function editForm(ctx, d, n, close, lib, save) {
	const folderName = ctx.name('services', n.from);
	const { body, inputs, rows, clash, dropped } = form(d, n.id, n.fields, n.from, lib, (id) => ctx.name('services', id));
	const msg = h('div', { class: 'error' });
	const typePath = `network.${n.id}.transport.fallback.type`, swPath = `network.${n.id}.transport.switching`;
	const field = (path) => n.fields[path.split('.').slice(2).join('.')];
	const had = field(typePath);
	const keeps = (k) => k === swPath && field(k)?.value !== 'automatic';
	const read = () => {
		const why = clash();
		if (why) throw new Error(why);
		const edits = changedValues(inputs, rows);
		const values = Object.keys(edits).length ? { ...edits, ...leftovers(n, edits) } : {};
		if (had || Object.keys(values).length)
			for (const k of dropped()) if (!keeps(k)) values[k] = UNSET;
		return values;
	};
	// Whose a value is: a folder's, or what one kept when it broke
	// hierarchy, this folder or one above.
	const whose = (r) => (r.origin === 'baseline'
		? `kept from when ${r.from === n.from ? 'this folder' : `Services › ${ctx.name('services', r.from)}`} broke hierarchy`
		: `from Services › ${ctx.name('services', r.from)}`);
	let off = null;
	const shut = () => {
		off?.();
		close();
	};
	const part = draftPart(read, shut, {
		tree: 'services',
		node: n.from,
		// What the network's switches set, shown here too.
		take: (values) => {
			for (const [path, v] of Object.entries(values)) if (inputs.has(path)) put(inputs.get(path).el, v);
		},
		summary: (values, seen) => {
			const sets = Object.keys(values).filter((k) => values[k] !== UNSET);
			const unsets = Object.keys(values).filter((k) => values[k] === UNSET);
			const dropping = !!had && unsets.includes(typePath);
			// What the fallback becomes: none, or what a folder above sets,
			// or what a folder kept when it broke hierarchy, which this
			// one's stood over.
			const after = seen.after(typePath);
			const above = after?.value != null;
			const sw = seen.after(swPath);
			const locked = unsets.filter((k) => field(k)?.origin === 'locked').map((k) => group(k).label);
			return [
				h('div', null, h('strong', null, `${ssidOf(n)} in Services › ${folderName}`)),
				h('ul', { class: 'becomes' },
					sets.map((path) => h('li', null, `${group(path).label}: `, field(path) && !SECRET.test(path) ? [value(path, field(path).value), ' → '] : '', shown(path, values[path]))),
					dropping && h('li', null, `Fallback: ${value(typePath, had.value)} → ${above ? `${value(typePath, after.value)}, ${whose(after)}` : 'none'}`),
					!had && unsets.length > 0 && h('li', null, `What was left of a fallback here is cleared: ${unsets.map((k) => group(k).label).join(', ')}. It meant nothing without one.`),
					// What each setting of how the transports are switched
					// between becomes, as the manager says it would be.
					dropping && unsets.filter((k) => !k.includes('.transport.fallback.')).map((k) => {
						const r = seen.after(k);
						return h('li', null, `${group(k).label}: ${value(k, field(k)?.value)} → ${r?.value != null ? `${value(k, r.value)}, ${whose(r)}` : 'unset'}`);
					}),
					dropping && dropped().includes(swPath) && keeps(swPath) && h('li', null, `Switching: stays ${value(swPath, field(swPath).value)} here.`)),
				dropping && h('div', { class: 'sub' }, above
					? (after.origin === 'baseline'
						? `What this folder sets of the fallback goes, and the one ${whose(after)} takes its place: the network still has a fallback, and that one cannot be taken away.`
						: `What this folder sets of the fallback goes, and the folder above’s takes its place: the network still has a fallback. To have none, take it away in Services › ${ctx.name('services', after.from)}.`)
					: 'The network travels over its first transport alone.'),
				// Automatic in this folder as well as above it: with the fallback
				// gone, switching would be automatic still, and every AP's config
				// held for it.
				dropping && !above && sw?.value === 'automatic' && h('div', { class: 'sub warn' },
					`Switching would be automatic still, ${whose(sw)}, which needs a fallback. Set Switching to report there, or here first, and then take the fallback away.`),
				locked.length > 0 && h('div', { class: 'sub' }, `This folder’s lock${locked.length === 1 ? '' : 's'} on ${locked.join(', ')} ${locked.length === 1 ? 'goes' : 'go'} with ${locked.length === 1 ? 'it' : 'them'}.`),
				h('div', { class: 'sub' }, `This changes the network in Services › ${folderName}, for every location that uses it.`),
			];
		},
		notes: (values) => {
			const sets = Object.keys(values).filter((k) => values[k] !== UNSET);
			const dropping = !!had && values[typePath] === UNSET;
			return [h('div', { class: 'sub warn' }, !dropping && probeOnly(sets) ? PROBE_ONLY : RESTART)];
		},
	});
	// A field changed: what is wrong, if anything, under the form; and its
	// value, for the network's switches.
	const edited = (e) => {
		msg.replaceChildren();
		try {
			read();
		} catch (err) {
			msg.replaceChildren(err.message);
		}
		const path = [...inputs].find(([, it]) => it.el === e.target || it.el.contains?.(e.target))?.[0];
		let v;
		try {
			v = path && inputs.get(path).read();
		} catch {
			return save.changed(part);
		}
		save.changed(part, path && v !== undefined ? { [path]: v } : undefined);
	};
	const el = h('div', { class: 'fieldform', 'data-editing': true },
		body,
		msg,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button', onclick: shut }, 'Cancel')));
	el.addEventListener('input', edited);
	el.addEventListener('change', edited);
	part.take(save.drafts(part));
	off = save.register(part);
	return { el, off };
}

// addForm offers a new network in one of the service folders that apply
// here and that the person may change. Its name in the schema comes from
// its SSID.
// addForm adds a network, where ({ folders, fresh }) says: in one of the
// folders, or, with none, in a new Services folder fresh.loc takes, all as
// one change (0320).
function addForm(ctx, d, where, nets, box, lib) {
	const { folders, fresh } = where;
	const pickFolder = h('select', null, folders.map((f) => h('option', { value: f.id }, `Services › ${ctx.name('services', f.id)}`)));
	const svc = ctx.trees.services;
	const newName = h('input', { type: 'text', maxlength: 64, value: `${ctx.name('services', svc.root)} networks`, 'aria-label': "The new services folder's name" });
	const { body, inputs, rows, sync, clash } = form(d, 'new', {}, null, lib);
	// A new network travels over a VLAN unless the person picks otherwise.
	const primary = inputs.get('network.new.transport.primary.type');
	if (primary) {
		primary.el.value = 'vlan';
		sync();
	}
	const msg = h('div', { class: 'error' });
	const out = h('div', { class: 'edit flush' });
	const review = async () => {
		msg.replaceChildren();
		if (clash()) {
			msg.replaceChildren(clash());
			return;
		}
		let values;
		try {
			values = changedValues(inputs, rows);
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		const ssid = values['network.new.ssid'];
		if (!ssid) {
			msg.replaceChildren('Give it an SSID.');
			return;
		}
		const id = name(ssid, d.names.network, new Set(nets.map((n) => n.id)));
		const renamed = Object.fromEntries(Object.entries(values).map(([p, v]) => [p.replace(/^network\.new\./, `network.${id}.`), v]));
		if (fresh) {
			const folderName = newName.value.trim();
			if (!folderName || [...folderName].length > 64) {
				msg.replaceChildren("A folder's name is 1 to 64 characters.");
				return;
			}
			const folder = idFor(ctx, 'services', folderName);
			// The folder, the location taking it beside what it takes now,
			// and the network in it: one change (0320).
			const op = { kind: 'batch', ops: [
				{ kind: 'add-folder', tree: 'services', node: folder, name: folderName, parent: svc.root },
				{ kind: 'assign-services', node: fresh.loc, services: [...fresh.takes, folder] },
				setOp(folder, renamed),
			] };
			const p = await ask(out, op);
			if (!p) return;
			confirm(ctx, out, op, p, [
				h('div', null, h('strong', null, `Add ${ssid}, in three steps`)),
				h('ol', { class: 'becomes' },
					h('li', null, `New services folder ${folderName}, in Services`),
					h('li', null, `${fresh.locName} takes ${[...fresh.takes.map((x) => ctx.name('services', x)), folderName].join(' + ')}`),
					h('li', null, `Network ${ssid} in it: `, Object.entries(renamed).map(([path, v], i) => [i ? ', ' : '', `${group(path).label} `, shown(path, v)]))),
				h('div', { class: 'sub' }, `It is saved as network "${id}", in Services › ${folderName}. Another location offers it too once it takes that folder.`),
			], [
				h('div', { class: 'sub warn' }, 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.'),
			]);
			return;
		}
		const op = setOp(pickFolder.value, renamed);
		const p = await ask(out, op);
		if (!p) return;
		const folderName = ctx.name('services', pickFolder.value);
		confirm(ctx, out, op, p, [
			h('div', null, h('strong', null, `Add ${ssid} to Services › ${folderName}`)),
			h('ul', { class: 'becomes' }, Object.entries(renamed).map(([path, v]) => h('li', null, `${group(path).label}: `, shown(path, v)))),
			h('div', { class: 'sub' }, `Every location that uses Services › ${folderName} offers it. It is saved as network "${id}".`),
		], [
			h('div', { class: 'sub warn' }, 'Applying restarts the Wi-Fi on each AP listed; its clients drop for a few seconds and reconnect.'),
		]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, 'Add a network'),
		h('div', { class: 'fieldform' },
			fresh
				? [h('label', { class: 'field' }, h('span', { class: 'label' }, 'In a new services folder'), newName),
					h('div', { class: 'sub' }, `No services folder applies at ${fresh.locName} yet. This one is made in Services, and ${fresh.locName} takes it.`)]
				: folders.length > 1
					? h('label', { class: 'field' }, h('span', { class: 'label' }, 'In'), pickFolder)
					: h('div', { class: 'sub' }, `In Services › ${ctx.name('services', folders[0].id)}, for every location that uses it.`),
			body,
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			out)));
}

// The names a network cannot have: Aeolus keeps them for its own sections on
// an AP (0054).
const KEPT = /^(vlan[0-9]+$|port-)/;

// name makes a network's name from its SSID, as the schema's pattern allows
// and unlike any taken. It starts with a letter.
function name(ssid, pattern, taken) {
	let base = ssid.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 28) || 'network';
	if (!/^[a-z]/.test(base) || KEPT.test(base)) base = 'n' + base;
	const ok = new RegExp(pattern);
	let id = base;
	for (let i = 2; taken.has(id) || !ok.test(id) || KEPT.test(id); i++) id = `${base}-${i}`;
	return id;
}
