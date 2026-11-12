// The parts of a Locations folder or AP page besides its networks and
// interfaces (0047, 0053, 0072, 0075): its APs' radios as each last
// reported, on a folder's APs tab, and its system settings.

import { placeMap } from './placemap.js';
import { h, link } from '../dom.js';
import { get, schema } from '../api.js';
import { bandName, ago, value, uplinkBrief, origin, templateSays } from '../format.js';
import { pageBand } from '../layout.js';
import { fieldPanels } from './fields.js';
import { systemEditor, systemLines, systemNote } from './system.js';
import { renameButton } from './rename.js';
import { moveButton } from './move.js';
import { healthBrief, bluetooth } from './health.js';
import { followButton } from './follow.js';
import { updatesPanel, managerPanel } from './updates.js';
import { gone } from './onnow.js';
import { pageSave } from './pagesave.js';
import { POWER } from './neighbours.js';

// only keeps the fields whose paths pass keep.
export function only(fields, keep) {
	return Object.fromEntries(Object.entries(fields || {}).filter(([p]) => keep(p)));
}

// configs reads each AP's config and condition, for the live views.
export async function configs(aps) {
	const all = await Promise.allSettled(aps.map((a) => get(`/v1/aps/${encodeURIComponent(a.id)}/config`)));
	return aps.map((a, i) => ({ ap: a, cfg: all[i].status === 'fulfilled' ? all[i].value : null }));
}

// channelsSection lists each AP below a folder on its APs tab (0075), a
// line each: its state, such as In sync (status, by AP ID, from the fleet),
// how it is doing, its radios as it last reported them, and its clients.
// The rest is a step away: a radio says its clients, and what Aeolus sets
// for it, to whoever points at it; the AP's own page has everything. With
// automatic channels each AP picks its own (0045), within the band's
// channel set. With canEdit, each AP can be renamed, its hostname with it,
// and moved to another folder (0076).
//
// agentChip is an AP's agent (0079) where it is not the release it should
// run: updating, behind, rolled back or failed, or held where its folder
// pins a release the manager no longer keeps. Nothing where it is current:
// that is as it should be, and its page says which, under About. Nothing
// where it has said none yet.
function agentChip(a) {
	if (!a?.runs?.version && !a?.missing_pin) return null;
	const { runs, wants } = a;
	const short = (v) => (v || '').replace(/-[0-9a-f]{7,}$/, '') || 'unknown';
	const u = runs?.update;
	const chip = (label, why) => h('span', { class: 'chip warn', title: [`It runs agent ${short(runs?.version)}.`, why].filter(Boolean).join(' ') }, `agent ${label}`);
	if (a.missing_pin) return chip('pin missing', `Its folder pins ${a.missing_pin}, which the manager no longer keeps, so it stays as it is.`);
	if (u?.state === 'updating') return chip('updating', `Trying ${short(u.version)}.`);
	if (runs?.hash && runs.hash === wants?.hash) return null;
	if (u && u.hash === wants?.hash && u.state === 'rolled-back') return chip('rolled back', u.why);
	if (u && u.hash === wants?.hash && u.state === 'failed') return chip('failed', u.why);
	return chip('behind', wants && `It should run ${short(wants.version)}.`);
}

// What airscan says its scan radio is doing (0126), in a word: listening
// for clients a channel at a time, looking for networks, sweeping the
// spectrum, or lent to a client test.
const SCAN_STATE = { clients: 'listening', bss: 'looking', spectral: 'sweeping', lent: 'lent', idle: 'idle', 'waiting for scan radio': 'waiting' };

// radioLine is what a radio sends at and hears under, under its channel,
// both in dBm (Griff, 2026-10-25): "tx 20 · nf -90". Its power as it runs
// it, and who set it, power control or its config; its noise floor, the
// middle of its latest minutes as GET /v1/noise gives it, amber where the
// channel is loud with what is no Wi-Fi, and none where the figure is below
// thermal noise, which is no measurement but what a driver says that has
// none.
function radioLine(r, f, apc, noise) {
	const tx = r.txpower != null && h('span', {
		title: `Sending at ${r.txpower} dBm. ${apc?.power != null ? `Power control has it at ${apc.power} of at most ${apc.ceiling ?? '?'} dBm${apc.why ? `: ${POWER[apc.why]?.[1] || apc.why}` : ''}.` : 'Set by its config, not by power control.'}`,
	}, `tx ${r.txpower}`);
	let nf = null;
	if (f?.unmeasured) {
		nf = h('span', { title: `The radio’s driver says ${f.dbm} dBm, below thermal noise (about ${noise.thermal} dBm in 20 MHz): no measurement. A fault in the driver, for the OpenWrt porting sessions to fix.` }, 'nf —');
	} else if (f) {
		const range = f.low === f.high ? '' : `; ${f.low} to ${f.high} dBm over them`;
		nf = h('span', { class: f.loud ? 'warn' : '', title: `Noise floor on its channel: the middle of its last ${f.minutes} minute${f.minutes === 1 ? '' : 's'}${range}.${f.loud ? ` Above ${noise.loud_over} dBm: loud with something that is no Wi-Fi, which slows every client on the channel.` : ''}` }, `nf ${f.dbm}`);
	}
	if (!tx && !nf) return null;
	const both = tx && nf ? ' · ' : '';
	return h('div', { class: 'floor mono sub' }, tx, both, nf);
}

// radiosOf is an AP's radios as it last reported them: those that serve;
// its scan radio, where it has one (0126), the radio of a band with another
// that serves, which netifd gives no channel, as airscan owns it; and a
// serving radio's clients, less those on its band since on another AP
// (0185), taken from the first radio of its band.
export function radiosOf(cfg) {
	const report = cfg?.condition?.state?.report;
	const radios = report?.radios || [];
	const scanning = report?.scan_radio
		? radios.find((r) => !r.channel && radios.some((o) => o !== r && o.band === r.band && o.channel)) : null;
	const serving = radios.filter((r) => r !== scanning);
	const left = gone(cfg?.condition);
	const clientsOf = (r) => Math.max(0, (r.clients ?? 0) - (serving.find((o) => o.band === r.band) === r ? left[r.band] || 0 : 0));
	return { radios, scanning, serving, clientsOf };
}

// upFor writes how long an AP has been up, from its report's seconds: 2 d
// 4 h, 3 h 12 m, 9 min.
export function upFor(s) {
	if (s == null) return null;
	return s >= 86400 ? `${Math.floor(s / 86400)} d ${Math.floor((s % 86400) / 3600)} h`
		: s >= 3600 ? `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} m` : `${Math.max(1, Math.round(s / 60))} min`;
}

export function channelsSection(ctx, rows, status, canEdit, noise, fleet, here) {
	if (!rows.length) return h('div', { class: 'banner info' }, 'No APs here yet.');
	// One band's radios, the page's (0284); the clients of all, a total.
	const shown = pageBand() || '5g';
	const known = new Map((fleet?.aps || []).map((a) => [a.id, a]));
	// Where an AP is, below this folder, where it is not in it.
	const where = (a) => (a?.ancestry || []).slice((a?.ancestry || []).indexOf(here) + 1, -1).map((id) => ctx.name('locations', id)).join(' › ');
	const box = h('div', { class: 'edit' });
	// Rename and Move, each named for its AP: a line away from the name,
	// they would otherwise all read alike.
	const acts = (ap) => {
		const rename = renameButton(ctx, 'locations', { id: ap.id, name: ap.name, kind: 'ap' }, box);
		const move = moveButton(ctx, 'locations', { id: ap.id, name: ap.name, kind: 'ap' }, box);
		rename.setAttribute('aria-label', `Rename ${ap.name}`);
		move.setAttribute('aria-label', `Move ${ap.name}`);
		return [rename, move];
	};
	const lines = rows.map(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const radios = rep?.report?.radios || [];
		const st = status?.get(ap.id);
		const state = st && h('span', { class: 'chip ' + st.chip, title: st.detail }, st.label);
		// Not where its state already speaks of its agent: said once.
		const agent = cfg && !/^Agent /.test(st?.label || '') && agentChip(cfg.agent);
		// How it is doing, by its latest report (0119); its Health tab has the rest.
		const brief = healthBrief(rep?.report?.health);
		const health = brief && h('a', { class: 'chip ' + brief.grade, href: `#/aps/${encodeURIComponent(ap.id)}/health`, title: 'By its latest report. Open its health and history' }, brief.text);
		// Its uplink: the speed it synced at, amber where that is below what
		// both ends can do; and its errors, where they are more than a few.
		const wire = uplinkBrief(rep?.report);
		const eth = `#/aps/${encodeURIComponent(ap.id)}/wired/ethernet`;
		const uplink = wire && [
			h('a', { class: 'chip uplink ' + (wire.grade === 'ok' ? 'idle' : wire.grade), href: eth, title: wire.says, 'aria-label': `Uplink ${wire.text}. ${wire.says}` }, wire.text),
			wire.errors?.grade && ' ',
			wire.errors?.grade && h('a', { class: 'chip uplink ' + wire.errors.grade, href: eth, title: wire.errors.says, 'aria-label': `${wire.errors.text}. ${wire.errors.says}` }, wire.errors.text),
		];
		// Its scan radio said in a column of its own (Griff, 2026-10-15:
		// "broken up into 2.4 5 6 and scanning radio columns").
		const { scanning, serving, clientsOf } = radiosOf(cfg);
		// A radio: the channel it is on and its width, under its band's
		// column.
		const radio = (r) => {
			const path = `radio.${r.band}.channel`;
			const set = cfg.location?.[path];
			const n = clientsOf(r);
			// It leads to the AP's radios, where what the chip says on
			// hover is said in full, for whoever has no pointer.
			const says = `${bandName(r.band)}: ${r.channel ? `channel ${r.channel}` : 'starting'}${r.width ? `, ${r.width} MHz` : ''}, ${n} client${n === 1 ? '' : 's'}. Aeolus sets its channel: ${set ? value(path, set.value) : 'not set'}.`;
			return h('a', { class: 'chip idle radio', href: `#/aps/${encodeURIComponent(ap.id)}/radios`, title: says, 'aria-label': says },
				h('span', { class: 'mono' }, r.channel ? `ch${r.channel}${r.width ? `/${r.width}MHz` : ''}` : 'starting'));
		};
		const none = () => h('span', { class: 'sub' }, !cfg ? 'You cannot see this AP' : rep?.at ? '—' : 'No report yet');
		// Under a band's channels, each radio's noise floor (Griff,
		// 2026-10-25).
		const floors = noise?.aps?.[ap.id] || {};
		const apcs = rep?.report?.rrm?.apc || [];
		const band = (b) => {
			const rs = serving.filter((r) => r.band === b);
			return h('td', { class: 'nowrap' }, rs.length ? [h('span', { class: 'radiochips' }, rs.map(radio)), rs.map((r) => radioLine(r, floors[r.radio], apcs.find((p) => p.radio === r.radio), noise))]
				: radios.length ? h('span', { class: 'sub' }, '—') : none());
		};
		// What the scan radio is doing, and how many networks it heard.
		const sr = rep?.report?.scan_radio;
		const scan = sr && h('a', { class: `chip radio ${sr.networks ? 'idle' : 'warn'}`, href: `#/aps/${encodeURIComponent(ap.id)}/radios`,
			title: `Its scan radio${scanning ? ` (${scanning.radio}, ${bandName(scanning.band)})` : ''}: ${sr.state || 'unknown'}; it heard ${sr.networks ?? 0} networks in its last look${sr.heals ? `; its driver bound again ${sr.heals} time${sr.heals === 1 ? '' : 's'}` : ''}` },
			SCAN_STATE[sr.state] || sr.state || 'scanning', h('span', { class: 'mono' }, ` ${sr.networks ?? 0}`));
		const a = known.get(ap.id);
		const sub = where(a);
		// The version it runs and the one it was given (0042), and the
		// template it follows, as the APs page had them.
		const version = a && h('td', { class: 'nowrap' }, h('span', { class: 'mono' }, a.seen?.running != null ? `${a.seen.running} / ${a.version}` : `— / ${a.version}`),
			a.template && h('div', { class: 'sub' }, a.template.follows ? a.template.name : templateSays(a.template)));
		return h('tr', null,
			h('td', null, link(`/aps/${encodeURIComponent(ap.id)}`, ap.name), sub && h('div', { class: 'sub' }, sub)),
			h('td', null, state, agent && ' ', agent),
			h('td', { class: 'nowrap' }, health || h('span', { class: 'sub' }, '—')),
			h('td', { class: 'nowrap' }, uplink || h('span', { class: 'sub' }, '—')),
			band(shown),
			h('td', { class: 'nowrap' }, scan || h('span', { class: 'sub' }, '—')),
			h('td', null, radios.length ? String(serving.reduce((n, r) => n + clientsOf(r), 0) + (scanning?.clients || 0)) : '—'),
			version || h('td', null, '—'),
			h('td', { class: 'sub nowrap' }, rep?.at ? ago(rep.at) : '—'),
			canEdit && h('td', { class: 'rowacts' }, acts(ap)));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'APs'),
		h('div', { class: 'widetable' }, h('table', { class: 'list' },
			h('tr', null, ['AP', 'State', 'Health', 'Uplink', bandName(shown), 'Scanning radio', 'Clients', 'Running / its version', 'Reported'].map((c) => h('th', null, c)),
				canEdit && h('th', null, h('span', { class: 'sr' }, 'Rename or move'))),
			lines)),
		box);
}

// The AP's Bluetooth radio, lent to Home Assistant (0156).
const BLE = 'system.ble_proxy';

// bluetoothPanel is that setting's own panel on the System tab, there
// whether or not it is set: its slider, a draft of the page's one save
// (0202); where it comes from; and on an AP that has it on, how the lending
// does by its latest report, and what Home Assistant is given to find it.
// live is { report, address } on an AP's page, and nothing on a folder's.
function bluetoothPanel(ctx, here, page, edit, live, save) {
	const r = page.fields?.[BLE];
	const on = r?.value === true;
	const box = h('div', { class: 'edit flush' });
	const sw = h('input', { type: 'checkbox', class: 'switch', role: 'switch', 'aria-label': 'Bluetooth for Home Assistant', disabled: !save });
	sw.checked = on;
	if (save) {
		const part = {
			changes: () => (sw.checked !== on ? { [BLE]: sw.checked } : {}),
			dirty: () => sw.checked !== on,
			undo: () => { sw.checked = on; },
			take: (values) => { if (BLE in values) sw.checked = values[BLE] === true; },
		};
		sw.addEventListener('change', () => save.changed(part, { [BLE]: sw.checked }));
		save.register(part);
	}
	const now = live && on ? bluetooth(live.report?.ble) : null;
	return h('section', { class: 'panel' },
		h('h2', null, 'Bluetooth', h('span', { class: 'note' }, 'for Home Assistant')),
		h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Lent'),
			h('div', { class: 'value' }, sw),
			r && origin('locations', here, r, (id) => ctx.name('locations', id)),
			edit && r?.origin === 'self' && followButton(ctx, 'locations', here, edit.nodeName, edit.parentName, [BLE], box, undefined, undefined, save)),
		live && on && h('div', { class: 'row' },
			h('div', { class: 'label' }, 'Now'),
			h('div', { class: 'value' + (now && now[2] === 'warn' ? ' warn' : '') },
				!now ? 'not reported yet' : now[0] === '—' ? now[1] : `${now[0]} ${now[1]}`)),
		live && on && live.address && h('div', { class: 'row' },
			h('div', { class: 'label' }, 'ESPHome'),
			h('div', { class: 'value mono' }, `${live.address}:${live.report?.ble?.port || 6053}`)),
		box);
}

// systemSection shows the system settings that reach the node, and for
// someone who may change it, an editor for them (0052). The tab has one save
// (0202): the map's place, Bluetooth, the editor's fields, each value
// changed in its row (0286) and each Follow above are drafts, saved as one
// change. Bluetooth and the place are in the
// editor too: what was set last, in either, is what is saved, and shows in
// both. live is what an AP's page knows of the AP now: { report, address }.
// shared, where given, is the page's save, which the System settings share
// with what else the page sets (0284): its bar is the page's to draw.
export async function systemSection(ctx, here, page, edit, live, shared) {
	// The manager's own releases, on the Org's (0183).
	const updates = here === ctx.trees.locations.root ? updatesPanel(ctx, ctx.update) : null;
	// And its own health, for the same admins (0219).
	const health = here === ctx.trees.locations.root && ctx.update ? await managerPanel() : null;
	// Maintenance (0218) is set here too, beside where alerts are sent.
	const sys = only(page.fields, (p) => (p.startsWith('system.') || p.startsWith('notify.') || p.startsWith('maintenance.')) && p !== BLE);
	const panels = (save, d) => (Object.keys(sys).length
		? fieldPanels(ctx, 'locations', here, sys, edit && d ? { ...edit, schema: d } : edit, save)
		: h('div', { class: 'banner info' }, 'No system settings here.'));
	// Where it is, and all below it, on the map (0169), after the settings
	// (0284).
	if (!edit) return [updates, health, bluetoothPanel(ctx, here, page, edit, live), panels(), placeMap(ctx, here, page, null)];
	const d = await schema();
	const isAP = page.node.kind === 'ap';
	const says = {
		summary: (values, seen) => [
			systemLines(d, page.node.name, page.fields || {}, values, 'System settings', !isAP, seen, ctx),
			!isAP && Object.keys(values).some((p) => p.startsWith('system.place.')) &&
				h('div', { class: 'sub' }, 'Its APs, and the folders below it, are drawn on the map where it is, unless one is put somewhere of its own.'),
		],
		notes: (values) => [systemNote(Object.keys(values))],
	};
	let save = shared;
	if (save) save.also((p) => p.startsWith('system.') || p.startsWith('notify.') || p.startsWith('maintenance.'), says);
	else save = pageSave(ctx, 'locations', here, says);
	const box = h('div', { class: 'edit flush' });
	return [
		updates,
		health,
		h('div', { class: 'below' },
			h('button', { type: 'button', class: 'button', title: 'Every system setting, set here or not, in one form', onclick: () => box.firstChild || systemEditor(ctx, d, here, page.node.name, page.fields, box, { save }) }, 'More settings…')),
		box,
		bluetoothPanel(ctx, here, page, edit, live, save),
		panels(save, d),
		placeMap(ctx, here, page, edit, save),
		!shared && save.bar,
	];
}
