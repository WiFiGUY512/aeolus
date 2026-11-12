// The System tab's editor (0052): every system field the schema has, built
// from its description, set on this folder or AP in one change. SNMP's
// fields show once SNMP is turned on.

import { h } from '../dom.js';
import { group, value } from '../format.js';
import { describe, fieldsForm, changedValues, put } from './edit.js';
import { ask, confirm } from './confirm.js';
import { UNSET } from './pagesave.js';
import { becomes } from './follow.js';

const MORE = 'Management and more';

// The order system fields are offered in. Any other the schema has, such as
// management addressing, which can cut an AP off if it is wrong, folds away
// under Management and more.
const SECTIONS = [
	['Place and time', ['system.country', 'system.tz', 'system.ntp']],
	// Where it is, for its map (0169).
	['Place on the map', ['system.place.address', 'system.place.latitude', 'system.place.longitude']],
	['Logging', ['system.syslog']],
	// What an AP asks, to say whether the internet answers (0142).
	['The way out', ['system.internet']],
	// What an AP with a scan radio measures its speed against (0121).
	['Client tests', ['system.speed_server', 'system.speed_server_remote']],
	['SNMP', ['system.snmp.enabled', 'system.snmp.community', 'system.snmp.v3.user', 'system.snmp.v3.auth',
		'system.snmp.v3.privacy', 'system.snmp.location', 'system.snmp.contact']],
	['Agent', ['system.poll', 'system.ssh_keys']],
	// A capture of each attempt to come online that fails (0135).
	['Troubleshooting', ['system.captures']],
	// The AP's Bluetooth radio, lent to Home Assistant (0156).
	['Bluetooth', ['system.ble_proxy']],
	// The DNS server whose own log says what a client looks up (0154).
	['What clients look up', ['system.dns_log.url', 'system.dns_log.token']],
	// The switches the manager reads for a client's way out (0168).
	['Switches the manager reads', ['system.switches.addresses', 'system.switches.user', 'system.switches.password']],
	// Where the manager sends alerts for the APs here (0101).
	['Alerts', ['notify.ntfy', 'notify.webhook', 'notify.severity', 'notify.resolved'],
		'Where the manager tells you of an alert for the APs here, once it has lasted two minutes: an ntfy topic, a webhook, or both.'],
	// The APs here off or being worked on, on purpose (0218).
	['Maintenance', ['maintenance.on', 'maintenance.until', 'maintenance.why'],
		'The APs here are off or being worked on, on purpose: their alerts are still listed and kept, said to be in maintenance, but none is sent or counted. One that outlasts the maintenance is sent once it ends.'],
	// The manager's own releases, set at the Org (0183).
	['Updates', ['system.updates.channel', 'system.updates.automatic', 'system.updates.window'],
		'The manager’s own releases. It looks for a newer one once a day; the APs then update their agents from it by themselves.'],
	// The advisor, set at the Org: Claude's reviews of what the manager knows (0205).
	['Advisor', ['system.advisor.enabled', 'system.advisor.key', 'system.advisor.model', 'system.advisor.every',
		'system.advisor.web', 'system.advisor.budget'],
		'Claude reviews what the manager knows of this folder and everything under it, its clients, APs, networks and settings, and lists what looks wrong and what to do on the Advisor page. A key set here gives this folder an advisor of its own, billed to that key; a folder below with a key of its own has its own, and this one reads nothing of it. It only reads: it never changes anything. What it reads, client MACs and names among it, goes to Anthropic’s API under the key.'],
];

// systemLines say, for a preview, what values set on nodeName do: each
// field, what it was and what it becomes; a secret, that it is a new one.
// below says APs below keep what they set themselves. A field unset (UNSET,
// 0204) becomes what seen says it would be.
export function systemLines(d, nodeName, fields, values, what = 'System settings', below = true, seen, ctx) {
	const secret = (path) => describe(d, path)?.writeOnly;
	return [
		h('div', null, h('strong', null, `${what} on ${nodeName}`)),
		h('ul', { class: 'becomes' }, Object.keys(values).map((path) => h('li', null,
			`${group(path).title} · ${group(path).label}: `,
			values[path] === UNSET ? becomes(ctx, 'locations', path, fields[path]?.value, seen.after(path))
				: secret(path) ? 'a new one' : [fields[path] ? [value(path, fields[path].value), ' → '] : '', value(path, values[path])]))),
		below && h('div', { class: 'sub' }, 'APs below that set their own keep theirs.'),
	];
}

// systemNote says what applying system settings at paths means for the APs.
export function systemNote(paths) {
	if (paths.some((path) => path.startsWith('system.management.')))
		return h('div', { class: 'sub warn' }, "Applying reloads each AP's network. An AP that can no longer reach Aeolus puts its old settings back within 90 seconds.");
	if (paths.includes('system.country'))
		return h('div', { class: 'sub warn' }, 'Applying restarts the Wi-Fi on each AP listed, as the radios use the country; clients drop for a few seconds and reconnect.');
	return h('div', { class: 'sub' }, 'Applying does not interrupt Wi-Fi.');
}

// systemEditor opens the editor in box, for the Locations node here, with
// the values in force there (fields, by path). With some.sections, it is an
// editor of those sections alone, under some.heading: another tab's own few
// settings, set the same way (0123). With some.save, the page's one save
// (0202), it has no Review of its own: what it changes is a draft of the
// page's, and its Cancel drops it.
export function systemEditor(ctx, d, here, nodeName, fields, box, some) {
	const sections = some?.sections || SECTIONS;
	const placed = new Set(sections.flatMap(([, paths]) => paths));
	const rest = some?.sections ? [] : Object.keys(d.fields).filter((p) => p.startsWith('system.') && !placed.has(p)).sort();
	const { body, inputs, rows } = fieldsForm(d, [...sections, [MORE, rest]], fields, here, MORE);
	const snmpOn = rows.get('system.snmp.enabled')?.it.el;
	const sync = () => {
		for (const [path, { row }] of rows)
			if (path.startsWith('system.snmp.') && path !== 'system.snmp.enabled') row.hidden = !snmpOn?.checked;
	};
	snmpOn?.addEventListener('change', sync);
	sync();
	// The time zones on offer are the country's, as it stands in the form
	// (0074).
	const country = rows.get('system.country')?.it.el;
	const narrow = () => rows.get('system.tz')?.it.narrow?.(country?.value.trim().toUpperCase());
	country?.addEventListener('input', narrow);
	country?.addEventListener('change', narrow);
	narrow();

	const out = h('div', { class: 'edit flush' });
	const msg = h('div', { class: 'error' });
	const save = some?.save;
	if (save) {
		let off = null;
		const close = () => {
			off?.();
			box.replaceChildren();
		};
		const read = () => changedValues(inputs, rows);
		const part = {
			changes: read,
			dirty: () => {
				try {
					return Object.keys(read()).length > 0;
				} catch {
					return true;
				}
			},
			blocked: () => {
				try {
					read();
					return false;
				} catch {
					return true;
				}
			},
			undo: close,
			open: () => true,
			take: (values) => {
				for (const [path, v] of Object.entries(values)) if (inputs.has(path)) put(inputs.get(path).el, v);
				sync();
			},
		};
		// A field changed: what is wrong, if anything, under the form; and
		// its value, for another part of the page that sets it too.
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
			save.changed(part, path ? { [path]: v } : undefined);
		};
		const form = h('div', { class: 'fieldform' }, body, msg,
			h('div', { class: 'actions' }, h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel')));
		form.addEventListener('input', edited);
		form.addEventListener('change', edited);
		box.replaceChildren(h('section', { class: 'panel', 'data-editing': true }, h('h2', null, some?.heading || 'Edit system settings'), form));
		// What the page holds already, as Bluetooth turned on, shows here too.
		part.take(save.drafts());
		off = save.register(part);
		return;
	}
	const review = async () => {
		msg.replaceChildren();
		let values;
		try {
			values = changedValues(inputs, rows);
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		const paths = Object.keys(values);
		if (!paths.length) {
			msg.replaceChildren('Nothing has changed.');
			return;
		}
		const op = paths.length === 1
			? { kind: 'set', tree: 'locations', node: here, path: paths[0], value: values[paths[0]] }
			: { kind: 'set', tree: 'locations', node: here, values };
		const p = await ask(out, op);
		if (!p) return;
		confirm(ctx, out, op, p, systemLines(d, nodeName, fields, values, some?.what), [systemNote(paths)]);
	};
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, some?.heading || 'Edit system settings'),
		h('div', { class: 'fieldform' },
			body,
			msg,
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: review }, 'Review changes'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel')),
			out)));
}
