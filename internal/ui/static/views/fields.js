// A node's values, grouped as in the mockup, each with where it comes from.

import { h } from '../dom.js';
import { group, value, whence, hint } from '../format.js';
import { followButton } from './follow.js';
import { describe, input, put } from './edit.js';
import { draftPart } from './pagesave.js';

// The order fields appear in within a panel; others follow by path.
const RANK = [
	'ssid', 'security', 'passphrase', 'bands', 'enabled', 'hidden', 'isolation',
	'roaming.ft', 'roaming.rrm', 'roaming.btm',
	'transport.primary.type', 'transport.primary.vlan', 'transport.primary.concentrator', 'transport.primary.vni',
	'transport.fallback.type', 'transport.fallback.vlan', 'transport.fallback.concentrator', 'transport.fallback.vni',
	'transport.switching', 'transport.ha', 'transport.failback', 'transport.holddown', 'transport.down_when_lost', 'rate_limit.down_kbps', 'rate_limit.up_kbps',
	'channel', 'width', 'dfs', 'power', 'country', 'tz', 'ntp', 'poll', 'syslog', 'speed_server', 'speed_server_remote', 'ssh_keys',
	'vlan', 'addressing', 'address', 'gateway', 'dns', 'uplink', 'mode', 'untagged', 'tagged', 'bond',
	'captures', 'ble_proxy',
];

// rank places a field by what follows its panel's own part of the path:
// "ssid" in network.sweet.ssid, "width" in radio.5g.width.
function rank(path) {
	const p = path.split('.');
	const own = p[0] === 'system' ? (p[1] === 'management' ? 2 : 1) : 2;
	const i = RANK.indexOf(p.slice(own).join('.'));
	return i < 0 ? RANK.length : i;
}

// inline is the cell of a value at path (r, {value, from, origin}), shown,
// that someone who may change it can change in its own row (0286): pressed,
// its input takes the value's place, and box under the row says how to fill
// it in, what is wrong, and Cancel. What it sets is a draft of the page's
// one save, d the schema that describes the field. A value locked above
// cannot be changed here, and one the schema has not is only shown.
// On a page whose save says nothing of the whole change, the row says its
// own: what the value was and what it becomes.
function inline(d, here, fields, path, r, shown, box, save, nodeName) {
	const f = describe(d, path);
	const cell = h('div', { class: 'value' });
	if (!f || (r.origin === 'locked' && r.from !== here)) {
		cell.append(shown);
		return cell;
	}
	const show = h('button', { type: 'button', class: 'inline', title: 'Change it here' }, shown);
	const msg = h('span', { class: 'error' });
	let it = null;
	let off = null;
	const close = () => {
		off?.();
		off = null;
		it = null;
		cell.replaceChildren(show);
		box.replaceChildren();
		box.removeAttribute('data-editing');
	};
	const read = () => {
		if (!it?.changed()) return {};
		const v = it.read();
		return v === undefined ? {} : { [path]: v };
	};
	// Open, it keeps the page from being drawn again under it, but the bar
	// shows only once it has a change.
	const part = draftPart(read, close, {
		open: () => false,
		take: (values) => {
			if (it && path in values) put(it.el, values[path]);
		},
		summary: (values) => [
			h('div', null, h('strong', null, `${group(path).title} on ${nodeName}`)),
			h('ul', { class: 'becomes' }, Object.entries(values).map(([p, v]) => h('li', null, `${group(p).label}: `,
				f.writeOnly ? 'a new one' : [value(p, r.value), ' → ', value(p, v)]))),
		],
	});
	show.addEventListener('click', () => {
		it = input(path, f, r.value);
		// The time zones on offer are the country's, as the page has it
		// (0074).
		if (path === 'system.tz') it.narrow?.(save.drafts()['system.country'] ?? fields['system.country']?.value);
		const edited = () => {
			msg.replaceChildren();
			let v;
			try {
				v = it.read();
			} catch (e) {
				msg.replaceChildren(e.message);
				return save.changed(part);
			}
			save.changed(part, v === undefined ? undefined : { [path]: v });
		};
		it.el.addEventListener('input', edited);
		it.el.addEventListener('change', edited);
		cell.replaceChildren(it.el);
		const help = hint(path);
		box.setAttribute('data-editing', '');
		box.replaceChildren(h('div', { class: 'inlinebar' },
			help && h('span', { class: 'hint' }, help),
			msg,
			h('button', { type: 'button', class: 'button small', onclick: close }, 'Cancel')));
		// What another part of the page has set already shows here too.
		part.take(save.drafts());
		off = save.register(part);
		it.el.focus?.();
	});
	cell.append(show);
	return cell;
}

// fieldPanels shows fields ({path: {value, from, origin}}) as panels side
// by side, as many across as fit, a line a setting with where it comes from
// in a word at its end (0284). With edit ({nodeName, parentName}), for
// someone who may change the node, each value set here can follow the
// folder above again (0046): with save, the page's one save, as a draft of
// it (0204). With edit.schema too, each value can be changed in its row
// (0286).
export function fieldPanels(ctx, tree, here, fields, edit, save) {
	const groups = new Map();
	for (const [path, r] of Object.entries(fields || {})) {
		const g = group(path);
		if (!groups.has(g.key)) groups.set(g.key, { ...g, rows: [] });
		groups.get(g.key).rows.push({ path, r, label: g.label });
	}
	const nodeName = (id) => ctx.name(tree, id);
	const serviceName = (id) => ctx.name('services', id);
	const panels = [...groups.values()]
		.sort((a, b) => a.order - b.order || a.key.localeCompare(b.key))
		.map((g) => {
			const ssid = g.network ? fields[`network.${g.network}.ssid`]?.value : null;
			return h('section', { class: 'panel' },
				h('h2', null, ssid || g.title, g.network && h('span', { class: 'note' }, 'network ' + g.network)),
				g.rows
					.sort((a, b) => rank(a.path) - rank(b.path) || a.path.localeCompare(b.path))
					.map(({ path, r, label }) => {
						const box = h('div', { class: 'edit' });
						const shown = value(path, r.value, serviceName);
						return [h('div', { class: 'row' },
							h('div', { class: 'label' }, label),
							edit?.schema && save ? inline(edit.schema, here, fields, path, r, shown, box, save, edit.nodeName) : h('div', { class: 'value' }, shown),
							whence(tree, here, r, nodeName),
							edit && r.origin === 'self' && followButton(ctx, tree, here, edit.nodeName, edit.parentName, [path], box, undefined, undefined, save)),
						box];
					}));
		});
	if (!panels.length) return h('div', { class: 'banner info' }, 'Nothing is set here or in the folders above.');
	return h('div', { class: 'fieldgrid' }, panels);
}

// editing is what field panels need to let someone who may change a node
// have it follow the folder above again (0046), or null for one who may
// not: who holds right there, configure unless said (0177).
export function editing(ctx, tree, page, right = 'configure') {
	if (!page.may?.includes(right)) return null;
	const n = page.node;
	return { nodeName: n.name, parentName: n.parent ? ctx.name(tree, n.parent) : null };
}
