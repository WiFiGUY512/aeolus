// An AP's radio neighbours (0073): the other Aeolus APs it hears in the air
// and exchanges hellos with over the wire, as each AP last reported, with
// how strongly each hears the other and the neighbour's channel; the
// channels as it rates them; and its moves. Radio resource management, and
// its policy for moves, are set here too. Power control (0077), which needs
// neighbours, has a page of its own (power.js, 0206), with this page's
// switch and policy rows.

import { h, link } from '../dom.js';
import { bandName, origin, group, value, ago, quiet } from '../format.js';
import { schema } from '../api.js';
import { followButton } from './follow.js';
import { fieldsForm, changedValues } from './edit.js';
import { neighbourMap } from './map.js';
import { pageSave } from './pagesave.js';
import { floatWindow } from '../float.js';

// A neighbour's state: its chip, and what it means.
const STATE = {
	up: ['ok', 'hellos go both ways'],
	'one-way': ['warn', 'its hellos come, but it gets none of this AP\'s'],
	heard: ['idle', 'heard in the air; no hellos yet'],
	down: ['bad', 'its hellos stopped'],
};

// The policy for moves (0073), power control's (0077), and what each means
// unset.
const POLICY = ['rrm.moves', 'rrm.window', 'rrm.margin', 'rrm.apart'];
export const APC_POLICY = ['apc.neighbours', 'apc.target', 'apc.clients', 'apc.client_floor', 'apc.hold'];
const DEFAULT = { 'rrm.moves': true, 'rrm.window': '02:00-05:00', 'rrm.margin': 20, 'rrm.apart': 250, 'apc.neighbours': 3, 'apc.target': -70,
	'apc.clients': 'ask', 'apc.client_floor': -80, 'apc.hold': 24 };

// What turning each on or off does.
const TURNS = {
	'rrm.enabled': ['Neighbours',
		'Nothing restarts. Each AP\'s beacons say it is an Aeolus AP, it listens briefly on its other channels, one at a time, while its radio isn\'t busy, and it exchanges hellos with the APs it hears best. Unless moves are off, it moves a radio whose channel is automatic to its best channel, as the policy says, with an announcement clients can follow.',
		'Nothing restarts. Each AP takes the mark off its beacons, stops its hellos, and moves no radio.'],
	'apc.enabled': ['Power control',
		'Nothing restarts. A minute after it applies the config, each AP holds each radio\'s power, unless one is set for it, and steps it 3 dB at a time, at most every ten minutes: up while fewer of its neighbours on the band hear it than it looks for, or the weakest of them hears it below the target; down while all of them hear it 6 dB or more above. Clients don\'t notice a step. It watches its clients too (0206): a client no other AP hears better, and clients that look elsewhere, leave or need more tries after a step down.',
		'Nothing restarts. Each AP lets its radios go back to their configured power.'],
};

// neighboursSection draws the switch, the policy for moves, the map, and the
// table. at is the node's page ({ node, nodeName, page, canEdit,
// parentName }); rows are its APs, each with its config and condition. The
// page has one save (0202): neighbours on or off, and their policy, are
// drafts, saved as one change.
export function neighboursSection(ctx, at, rows, band) {
	const save = at.canEdit ? pageSave(ctx, 'locations', at.node) : null;
	const both = { ...at, save };
	const rrm = onOff(ctx, both, 'rrm.enabled', POLICY, 'Edit moves…');
	return [
		h('section', { class: 'panel' }, rrm.row, rrm.on && POLICY.map((p) => policyRow(ctx, both, p)), rrm.boxes),
		neighbourMap(ctx, rows, band, at.node),
		table(ctx, rows, band),
		save?.bar,
	];
}

// onOff is a switch, for path, and its policy's fields (policy), in a form
// the button labelled edits opens: one part of the page's save. Power
// control's switch turns on where neighbours are on, or are to be. It
// returns its row, the boxes under it, and whether it is on as saved.
export function onOff(ctx, at, path, policy, edits) {
	const { node, nodeName, page, parentName, save } = at;
	const field = page.fields?.[path];
	const on = field?.value === true;
	const [what, onText, offText] = TURNS[path];
	const box = h('div', { class: 'edit' });
	const formBox = h('div', { class: 'edit' });
	const lockedAbove = field?.origin === 'locked' && field.from !== node;
	const may = save && !lockedAbove;
	const rrmWas = page.fields?.['rrm.enabled']?.value === true;
	let rrm = rrmWas;
	const sw = h('input', { type: 'checkbox', class: 'switch', role: 'switch', 'aria-label': what });
	sw.checked = on;
	const can = () => path !== 'apc.enabled' || on || rrm;
	// The policy's form, while it is open.
	let form = null;
	const editBtn = h('button', { type: 'button', class: 'button small', onclick: async () => {
		if (!form) openForm(await schema());
	} }, edits);
	const draw = () => {
		sw.disabled = !can();
		sw.title = can() ? '' : 'Needs neighbours on';
		editBtn.hidden = !sw.checked;
	};
	const closeForm = () => {
		form = null;
		formBox.replaceChildren();
	};
	const changes = () => ({
		...(sw.checked !== on ? { [path]: sw.checked } : {}),
		...(form ? changedValues(form.inputs, form.rows) : {}),
	});
	const part = {
		changes,
		dirty: () => {
			try {
				return Object.keys(changes()).length > 0;
			} catch {
				return true;
			}
		},
		blocked: () => {
			try {
				changes();
				return false;
			} catch {
				return true;
			}
		},
		open: () => form !== null,
		undo: () => {
			sw.checked = on;
			rrm = rrmWas;
			closeForm();
			draw();
		},
		// Neighbours turned on or off, for power control's switch.
		take: (values) => {
			if (!('rrm.enabled' in values) || path === 'rrm.enabled') return;
			rrm = values['rrm.enabled'] === true;
			draw();
		},
		summary: (values) => [
			h('div', null, h('strong', null, `${what} on ${nodeName}`)),
			h('ul', { class: 'becomes' }, Object.keys(values).map((p) => h('li', null, `${group(p).label}: `,
				page.fields?.[p] ? value(p, page.fields[p].value) : [value(p, DEFAULT[p] ?? false), ' (not set)'], ' → ', value(p, values[p])))),
		],
		notes: (values) => [h('div', { class: 'sub' }, path in values ? (values[path] ? onText : offText) : 'Nothing restarts: each AP takes it up once it applies the config.')],
	};
	const openForm = (d) => {
		const { body, inputs, rows } = fieldsForm(d, [[null, policy]], page.fields || {}, node);
		const msg = h('div', { class: 'error' });
		form = { inputs, rows };
		const el = h('div', { class: 'fieldform', 'data-editing': true },
			body,
			msg,
			h('div', { class: 'actions' }, h('button', { type: 'button', class: 'button', onclick: () => { closeForm(); save.changed(part); } }, 'Cancel')));
		const edited = () => {
			msg.replaceChildren();
			try {
				changedValues(inputs, rows);
			} catch (e) {
				msg.replaceChildren(e.message);
			}
			save.changed(part);
		};
		el.addEventListener('input', edited);
		el.addEventListener('change', edited);
		formBox.replaceChildren(el);
		save.refresh();
	};
	if (may) {
		sw.addEventListener('change', () => {
			draw();
			save.changed(part, { [path]: sw.checked });
		});
		save.register(part);
		draw();
	}
	const row = h('div', { class: 'row' },
		h('div', { class: 'label' }, what),
		h('div', { class: 'value' }, may ? [sw, !field && h('span', { class: 'sealed' }, ' not set')] : field ? (on ? 'on' : 'off') : h('span', { class: 'sealed' }, 'off (not set)'),
			path === 'apc.enabled' && !rrmWas && on && h('span', { class: 'sub' }, ' (needs neighbours on)')),
		field && origin('locations', node, field, (id) => ctx.name('locations', id)),
		may && h('span', { class: 'controls' },
			field?.origin === 'self' && followButton(ctx, 'locations', node, nodeName, parentName, [path], box, undefined, undefined, save),
			editBtn));
	return { row, on, boxes: [formBox, box] };
}

// policyRow shows one of the policy's fields, as set here or above, or its
// default; one set here may follow above again, a draft of the page's save.
export function policyRow(ctx, at, path) {
	const { node, nodeName, page, canEdit, parentName, save } = at;
	const field = page.fields?.[path];
	const box = h('div', { class: 'edit' });
	return [h('div', { class: 'row' },
		h('div', { class: 'label' }, group(path).label),
		h('div', { class: 'value' }, field ? value(path, field.value) : h('span', { class: 'sealed' }, value(path, DEFAULT[path]), ' (not set)')),
		field && origin('locations', node, field, (id) => ctx.name('locations', id)),
		canEdit && field?.origin === 'self' && h('span', { class: 'controls' },
			followButton(ctx, 'locations', node, nodeName, parentName, [path], box, undefined, undefined, save))),
	box];
}

// Why power control holds a radio where it does (0077), with its chip; and
// for its clients (0206).
export const POWER = {
	new: ['idle', 'just taken over'], looking: ['warn', 'looking for neighbours'], below: ['warn', 'below the target'],
	ceiling: ['warn', 'at its ceiling'], target: ['ok', 'at the target'], above: ['ok', 'above the target'],
	floor: ['ok', 'at its floor'], failed: ['bad', 'cannot set its power'],
	guard: ['ok', 'up for a lonely client'], hold: ['warn', 'held for its clients'],
	distress: ['warn', 'stepped back for its clients'], 'distress-seen': ['warn', 'its clients struggled'],
};

// deaf lists an AP's radios that can't scan (2026-10-06), as rows across
// span columns: on a DFS channel, Linux keeps an AP there listening for
// radar and refuses every scan, so what the radio rates and hears on that
// band ages until it leaves; a radio may refuse every scan otherwise too.
// band, where given, is the one band shown.
function deaf(r, span, first, band) {
	return (r?.cannot_scan || []).filter((d) => !band || d.band === band).map((d, i) => h('tr', null,
		h('td', null, i === 0 && first),
		// Where the AP looks before it uses a DFS channel (0120), being
		// there is what it chose, and it knows where it steps off to.
		h('td', { colspan: span, class: 'sub' }, h('span', { class: `chip ${d.home ? 'idle' : 'warn'}` }, d.home ? `${bandName(d.band)} on a DFS channel` : `${bandName(d.band)} can't scan`), ' ',
			d.why === 'dfs' && d.home
				? `For ${dur(d.ago)}: on DFS channel ${d.channel} the radio must keep listening for radar, so it can't look at other channels. It rates its own by its own counts, and steps off to channel ${d.home} to look again when its own grows worse, a neighbour takes it, or interference fills it.`
				: d.why === 'dfs'
					? `For ${dur(d.ago)}: on DFS channel ${d.channel} the radio must keep listening for radar, so it can't visit other channels or hear its neighbours there. Its other ${bandName(d.band)} ratings are remembered, faded, for a day after its last look (0332), and RRM moves by none of them until it looks again.`
					: `For ${dur(d.ago)}: its radio refuses every scan on channel ${d.channel}, so its ${bandName(d.band)} ratings age.`)));
}

// table lists each AP's radio neighbours, on band alone where it is given.
function table(ctx, rows, band) {
	const name = (id) => ctx.name('locations', id);
	const span = band ? 6 : 7;
	const lines = rows.flatMap(({ ap, cfg }) => {
		const rep = cfg?.condition?.state;
		const r = rep?.report?.rrm;
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}/radios/neighbours`, ap.name);
		const none = (why) => [h('tr', null, h('td', null, apLink), h('td', { colspan: span, class: 'sub' }, why))];
		if (!cfg) return none('You cannot see this AP.');
		// Offline: what it heard when it last reported is not the mesh now.
		if (quiet(cfg.condition?.seen?.at)) return none(`Offline: last heard from ${ago(cfg.condition?.seen?.at)}. What it heard then is not shown.`);
		if (rep?.report?.wireless_missing) return none(`Can't see its own radios: netifd lost its network.wireless object. Restarting the network on the AP brings it back; its Wi-Fi drops for about 30 seconds.`);
		if (!r) return none(rep ? 'Off, or not reported yet.' : 'No report yet.');
		if (!r.neighbours.length) return [...none(`Hears no other Aeolus AP yet. Its address for hellos is ${r.address || 'unknown'}.`), ...deaf(r, span, null, band)];
		// Only the bands this AP has a radio on: a neighbour's channel on a
		// band it has none on means nothing to it (Griff, 2026-10-15: "if it
		// doesnt have a 6ghz radio, it should not have a channel").
		const mine = new Set((rep.report.radios || []).map((x) => x.band));
		if (band && mine.size && !mine.has(band)) return none(`No ${bandName(band)} radio.`);
		const ours = (n) => (mine.size ? n.bands.filter((b) => mine.has(b.band)) : n.bands).filter((b) => !band || b.band === band);
		// On one band, the neighbours heard on it; on all, every neighbour.
		const shown = band ? r.neighbours.filter((n) => ours(n).length) : r.neighbours;
		if (!shown.length) return [...none(`Hears no other Aeolus AP on ${bandName(band)}.`), ...deaf(r, span, null, band)];
		return [...shown.flatMap((n, i) => (ours(n).length ? ours(n) : [{ band: null }]).map((b, j) => {
			const [chip, means] = STATE[n.state] || ['idle', ''];
			return h('tr', null,
				h('td', null, i === 0 && j === 0 && apLink),
				h('td', null, j === 0 && [link(`/aps/${encodeURIComponent(n.ap)}/radios/neighbours`, name(n.ap)),
					!n.chosen && h('div', { class: 'sub' }, 'not among the three it hears best')]),
				!band && h('td', null, b.band ? bandName(b.band) : '—'),
				h('td', { class: 'mono' }, b.signal != null ? `${b.signal} dBm` : '—'),
				h('td', { class: 'mono' }, b.their_signal != null ? `${b.their_signal} dBm` : '—'),
				h('td', { class: 'mono' }, b.channel ? `${b.channel}${b.width ? ` · ${b.width} MHz` : ''}` : '—',
					// How busy it is, as its hellos say (0207).
					b.load?.busy != null && h('div', { class: 'sub', title: `its last minute: ${Math.round(b.load.busy)}% busy${b.load.clients != null ? `, ${Math.round(b.load.clients)} clients` : ''}` },
						`${Math.round(b.load.busy)}% busy${b.load.clients != null ? ` · ${Math.round(b.load.clients)} clients` : ''}`)),
				h('td', null, j === 0 && h('span', { class: `chip ${chip}`, title: means }, n.state)),
				h('td', null, j === 0 && (n.hello_ago != null ? `${n.hello_ago} s ago` : '—')));
		})), ...deaf(r, span, null, band)];
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Radio neighbours'),
		rows.length
			? h('table', { class: 'list' },
				h('tr', null, ['AP', 'Neighbour', ...(band ? [] : ['Band']), 'Hears it', 'It hears this AP', 'Its channel', 'State', 'Last hello'].map((c) => h('th', null, c))),
				lines)
			: h('div', { class: 'sub' }, 'No APs here yet.'));
}

// ratingsSection lists each AP's channel ratings (0073), lower being better:
// the lasting one, earned over many visits, and the one now; what went into
// the last visit's; the channel its radio is on, the neighbours that blot
// channels out, and the AP's best on each band, where its radio would move:
// the best rated no neighbour uses, or where they use them all, the one
// whose nearest user is furthest away. Then each AP's moves.
//
// Each band has a panel of its own, and in it each AP's channels open by
// the arrow beside its name (Griff, 2026-10-10): one long table of every
// AP's every channel was hard to read.
export function ratingsSection(ctx, rows, own, band) {
	return [...ratingsPanels(ctx, rows, own, band), movesPanel(ctx, rows, band)];
}

// The AP lists opened on the Ratings page, as '<band> <ap>', so a page
// drawn again keeps them open; and those whose every channel is shown.
const opened = new Set();
const eachOpened = new Set();

// The 5 GHz blocks at each width, by their lowest and highest channel, as
// RRM has them (rrm.uc BLOCKS5).
const BLOCKS5 = {
	40: [[36, 40], [44, 48], [52, 56], [60, 64], [100, 104], [108, 112], [116, 120], [124, 128], [132, 136], [140, 144], [149, 153], [157, 161]],
	80: [[36, 48], [52, 64], [100, 112], [116, 128], [132, 144], [149, 161]],
	160: [[36, 64], [100, 128]],
};

// dur says seconds the short way: 90 s, 12 min, 7 h.
const dur = (s) => (s < 120 ? `${s} s` : s < 7200 ? `${Math.round(s / 60)} min` : `${Math.round(s / 3600)} h`);

// judged is an AP's ratings on a band as RRM judges them at its radio's
// width (rrm.uc blocks, issue #45): on 5 GHz at 40 MHz or more, each block
// whose channels are all rated, as good as its worst channel, used where
// any of its channels is; elsewhere each channel alone. A block with a
// channel only remembered (0332) is remembered, as old as its oldest.
export function judged(list, band, width) {
	const one = (x) => ({ members: [x.channel], cost: x.cost, now: x.now ?? x.cost, own: !!x.own, best: !!x.best, blotted_by: x.blotted_by || [], worst: x, xs: [x], floor: floorOf(x), stale: !!x.stale, ago: x.ago });
	if (band !== '5g' || !(width > 20) || !BLOCKS5[width]) return list.map(one);
	const by = new Map(list.map((x) => [x.channel, x]));
	const out = [];
	for (const [lo, hi] of BLOCKS5[width]) {
		const members = [];
		for (let c = lo; c <= hi; c += 4) members.push(c);
		if (!members.every((c) => by.has(c))) continue;
		const xs = members.map((c) => by.get(c));
		const worst = xs.reduce((a, b) => (b.cost > a.cost ? b : a));
		out.push({
			members, cost: worst.cost, now: Math.max(...xs.map((x) => x.now ?? x.cost)),
			own: xs.some((x) => x.own), best: xs.some((x) => x.best),
			blotted_by: [...new Set(xs.flatMap((x) => x.blotted_by || []))].sort(), worst, xs,
			floor: xs.some((x) => floorOf(x) != null) ? Math.max(...xs.map(floorOf).filter((f) => f != null)) : null,
			stale: xs.some((x) => x.stale), ago: Math.max(...xs.map((x) => x.ago ?? 0)),
		});
	}
	return out;
}

// hue is a rating's colour, from green, a clear channel (0), through
// yellow, to red at 80 and over: 100 is a channel others keep busy all the
// time (rrm.uc cost).
export const hue = (cost) => Math.round(120 * (1 - Math.min(Math.max(cost, 0), 80) / 80));

// busySaid is how busy a channel was, and by what: the scan radio's
// spectral sweep, where it swept (0245).
const busySaid = (x) => `busy ${x.busy}%${x.energy != null ? ` (spectrum ${x.energy}%)` : ''}`;

// floorOf is a channel's noise floor: the sweep's, set against the
// radio's own (0247), or the radio's own on its channel.
const floorOf = (x) => x.floor ?? x.noise;

// channelWindow shows one block of an AP's ratings, a click on it away
// (0245): in a line why it rates as it does, and each of its 20 MHz
// channels' numbers as the AP last said them.
function channelWindow(ctx, apName, band, b, width) {
	const name = (id) => ctx.name('locations', id);
	const box = floatWindow('channel', 'Channel rating', { width: 600, height: 340 });
	const first = b.members[0], last = b.members[b.members.length - 1];
	const label = b.members.length > 1 ? `${first}–${last} at ${width} MHz` : `channel ${first}`;
	const dfs = band === '5g' && b.members.some((c) => c >= 52 && c <= 144);
	const w = b.worst;
	const xs = b.xs || [w];
	const why = [
		`rated ${b.cost}, now ${b.now}`,
		b.members.length > 1 && `as good as ${w.channel}, its worst`,
		b.own && 'in use',
		b.stale && `remembered from ${dur(b.ago)} ago: not moved by`,
		b.best && 'rates best',
		b.blotted_by.length > 0 && `used by ${b.blotted_by.map(name).join(', ')}`,
		dfs && 'DFS',
	].filter(Boolean).join(' · ');
	const panel = h('section', { class: 'panel' },
		h('h2', null, `${apName} · ${bandName(band)} · ${label}`,
			h('button', { type: 'button', class: 'button small', onclick: () => box.replaceChildren() }, 'Close')),
		h('div', null, why),
		h('table', { class: 'list' },
			h('tr', null, ['Channel', 'Rating', 'Now', 'Busy', 'Spectrum', 'Floor', 'Networks', 'Last'].map((c) => h('th', null, c))),
			xs.map((x) => h('tr', null,
				h('td', { class: 'mono' }, String(x.channel)),
				h('td', { class: 'mono' }, String(x.cost)),
				h('td', { class: 'mono' }, String(x.now ?? x.cost)),
				h('td', { class: 'mono' }, `${x.busy}%`),
				h('td', { class: 'mono' }, x.energy != null ? `${x.energy}%` : '—'),
				h('td', { class: 'mono' }, floorOf(x) != null ? `${floorOf(x)} dBm` : '—'),
				h('td', null, String(x.networks)),
				h('td', null, x.ago != null ? `${dur(x.ago)} ago` : '—')))),
		h('div', { class: 'sub' }, 'Rating: busy % (the larger of the radio\'s count and the spectrum) plus up to 10 for each network heard, over many visits; lower is better. Floor: the noise floor, which counts for nothing in the rating.'));
	box.replaceChildren(panel);
}

// blocksRow draws judged blocks a coloured box each: its channels and its
// rating, the one in use ringed, the best starred, those a neighbour uses
// dimmed and DFS hatched, as the channel map has them; the numbers on
// hovering, and all of them a click away (0245).
function blocksRow(ctx, band, blocks, width, apName) {
	const name = (id) => ctx.name('locations', id);
	return h('div', { class: 'rateblocks' }, blocks.map((b) => {
		const first = b.members[0], last = b.members[b.members.length - 1];
		const label = b.members.length > 1 ? `${first}–${last}` : String(first);
		const dfs = band === '5g' && b.members.some((c) => c >= 52 && c <= 144);
		const w = b.worst;
		const title = [
			b.members.length > 1 ? `Channels ${b.members.join(', ')} at ${width} MHz, as good as ${w.channel}, its worst` : `Channel ${first}`,
			`rated ${b.cost} (now ${b.now}); lower is better`,
			`${w.channel}: ${busySaid(w)}, floor ${floorOf(w) != null ? `${floorOf(w)} dBm` : 'unknown'}, ${w.networks} network${w.networks === 1 ? '' : 's'} heard`,
			b.own && 'The radio is on it',
			b.stale && `Remembered: last looked at ${dur(b.ago)} ago. RRM moves by it no more until it looks again`,
			b.best && 'Rates best: where the radio would move',
			b.blotted_by.length > 0 && `Used by ${b.blotted_by.map(name).join(', ')}`,
			dfs && 'Shared with radar (DFS)',
			'Click for every number',
		].filter(Boolean).join(' · ');
		return h('div', {
			class: `rblock${b.own ? ' own' : ''}${b.blotted_by.length ? ' used' : ''}${dfs ? ' dfs' : ''}${b.stale ? ' stale' : ''}`,
			style: { background: `hsl(${hue(b.cost)} 62% 78%)` }, title,
			onclick: () => channelWindow(ctx, apName, band, b, width),
		},
		h('span', { class: 'mono' }, label),
		h('span', { class: 'mono cost' }, String(b.cost), b.best && ' ★'),
		b.floor != null && h('span', { class: 'mono cost' }, String(b.floor)),
		b.stale && h('span', { class: 'mono cost age' }, dur(b.ago)));
	}));
}

// own says the page is an AP's own, where its lists start open; a folder
// with one AP in it is still a folder.
function ratingsPanels(ctx, rows, own, band) {
	const note = 'lower is better; the rating is earned over many visits, now is the last few';
	const apLink = (ap) => link(`/aps/${encodeURIComponent(ap.id)}/radios/ratings`, ap.name);
	// What each AP has to show, or why it has nothing.
	const of = rows.map(({ ap, cfg }) => {
		const report = cfg?.condition?.state?.report, r = report?.rrm;
		const why = !cfg ? 'You cannot see this AP.' : !r ? 'Off, or not reported yet.'
			: !r.ratings?.length && !r.cannot_scan?.length ? 'No channel rated yet.' : null;
		return { ap, report, r, why };
	});
	const panels = ['2g', '5g', '6g'].filter((b) => !band || b === band).map((band) => {
		const folds = of.filter((x) => !x.why).map((x) => ratingsFold(ctx, band, x, !!own)).filter(Boolean);
		return folds.length > 0 && h('section', { class: 'panel' },
			h('h2', null, `${bandName(band)} channel ratings`, h('span', { class: 'note' }, note)),
			folds);
	}).filter(Boolean);
	const quiet = of.filter((x) => x.why);
	if (!quiet.length && panels.length) return panels;
	return [...panels, h('section', { class: 'panel' },
		h('h2', null, 'Channel ratings', !panels.length && h('span', { class: 'note' }, note)),
		rows.length
			? h('table', { class: 'list' }, quiet.map((x) => h('tr', null, h('td', null, apLink(x.ap)), h('td', { class: 'sub' }, x.why))))
			: h('div', { class: 'sub' }, 'No APs here yet.'))];
}

// ratingsFold is one AP's channels on one band, shut until its arrow is
// pressed, or open where the page is the AP's own. Shut, it says the
// channel the radio is on, the best there, and how many are rated. Null
// where the AP has nothing on the band.
function ratingsFold(ctx, band, { ap, report, r }, alone) {
	const name = (id) => ctx.name('locations', id);
	const list = (r.ratings || []).filter((x) => x.band === band);
	const deafHere = { cannot_scan: (r.cannot_scan || []).filter((d) => d.band === band) };
	if (!list.length && !deafHere.cannot_scan.length) return null;
	const radio = (report?.radios || []).find((x) => x.band === band && x.up !== false && x.channel);
	const width = radio?.width;
	const blocks = judged(list, band, width);
	const inBlocks = blocks.length > 0 && blocks[0].members.length > 1;
	const best = blocks.find((x) => x.best);
	const kept = blocks.filter((x) => x.stale).length;
	const span = (b) => (b.members.length > 1 ? `${b.members[0]}–${b.members[b.members.length - 1]}` : `channel ${b.members[0]}`);
	// How it listens (0173), where the AP says.
	const listens = r.listens === 'scan-radio' ? 'heard through its scan radio'
		: r.scan_out ? 'its scan radio is out of service, so its serving radios scan'
			: r.listens === 'serving' ? 'scanned by its serving radios' : null;
	const says = [
		radio && `on channel ${radio.channel}${width ? `, ${width} MHz` : ''}`,
		listens,
		best && (best.own ? (inBlocks ? 'its block rates best' : 'its channel rates best') : `${span(best)} rates best`),
		inBlocks ? `${blocks.length - kept} block${blocks.length - kept === 1 ? '' : 's'} rated` : `${list.length - kept} channel${list.length - kept === 1 ? '' : 's'} rated`,
		kept > 0 && `${kept} remembered`,
	].filter(Boolean).join(' · ');
	const key = `${band} ${ap.id}`;
	// Every 20 MHz channel, a click under the blocks: the detail.
	const table = h('table', { class: 'list' },
			list.length > 0 && h('tr', null, ['Channel', 'Rating', 'Now', 'Busy', 'Spectrum', 'Floor', 'Networks', 'Last visit', ''].map((c) => h('th', null, c))),
			list.map((x) => h('tr', null,
				h('td', { class: 'mono' }, String(x.channel)),
				h('td', { class: 'mono' }, String(x.cost)),
				h('td', { class: 'mono' }, String(x.now)),
				h('td', { class: 'mono' }, `${x.busy}%`),
				h('td', { class: 'mono' }, x.energy != null ? `${x.energy}%` : '—'),
				h('td', { class: 'mono' }, floorOf(x) != null ? `${floorOf(x)} dBm` : '—'),
				h('td', null, String(x.networks)),
				h('td', null, `${dur(x.ago)} ago`),
				h('td', null,
					x.own && h('span', { class: 'chip here' }, 'in use'), ' ',
					x.stale && h('span', { class: 'chip idle', title: 'Not looked at for half an hour: what the AP last knew of it, kept for a day. RRM moves by it no more until it looks again.' }, 'remembered'), ' ',
					x.best && h('span', { class: 'chip ok' }, x.blotted_by.length ? 'best (all used)' : 'best'),
					x.blotted_by.length > 0 && h('div', { class: 'sub' }, `used by ${x.blotted_by.map(name).join(', ')}`)))),
			deaf(deafHere, 7, null));
	const each = blocks.length > 0 && h('details', { class: 'fold nested', open: eachOpened.has(key) },
		h('summary', null, h('span', { class: 'sub' }, 'Each 20 MHz channel')), table);
	if (each) each.addEventListener('toggle', () => (each.open ? eachOpened.add(key) : eachOpened.delete(key)));
	const d = h('details', { class: 'fold', open: alone || opened.has(key) },
		h('summary', null,
			h('span', { class: 'who' }, ap.name),
			h('span', { class: 'grow sub' }, says),
			deafHere.cannot_scan.length > 0 && (deafHere.cannot_scan.every((x) => x.home)
				? h('span', { class: 'chip idle' }, 'on a DFS channel')
				: h('span', { class: 'chip warn' }, "can't scan")),
			!alone && link(`/aps/${encodeURIComponent(ap.id)}/radios/ratings`, 'Its page')),
		blocks.length > 0 && blocksRow(ctx, band, blocks, width, ap.name),
		blocks.length > 0 && h('div', { class: 'ratekey sub' },
			inBlocks ? `Each block of ${width} MHz is as good as its worst channel, as RRM judges it.` : 'Each channel alone.',
			' Green is clear, red is busy; ★ rates best; the ringed one is in use. Hover for the numbers, click for all of them.',
			kept > 0 && ' Faded ones are remembered: not looked at for half an hour, with their age under them. They are kept for a day, and RRM moves by none of them until it looks again.'),
		each || table);
	d.addEventListener('toggle', () => (d.open ? opened.add(key) : opened.delete(key)));
	return d;
}

// Why a move was made, and what came of it, with its chip.
const WHY = {
	start: 'the radio had just started', shared: 'a neighbour uses its channel',
	better: 'another rates better', interference: 'interference on its channel', own: 'too near its own 5 GHz',
	radar: 'radar was found on its channel', wide: 'widened to the width it is set to',
};
// checking: on a DFS channel, listening for radar before it sends (0120).
const CAME = { announced: 'warn', checking: 'warn', moved: 'ok', yielded: 'idle', withdrawn: 'idle', failed: 'bad' };
const CAME_SAYS = { checking: 'listening for radar' };

// followedSays is what came of a move for its radio's clients (0265), two
// minutes on: how many of them followed it, and the rest on hover.
function followedSays(c) {
	if (!c?.clients) return null;
	const rest = [c.back && `${c.back} joined again`, c.gone && `${c.gone} gone`].filter(Boolean);
	return h('span', { class: `sub${c.gone ? ' warn' : ''}`, title: `of the ${c.clients} clients it had: ${c.kept} followed on their own association${rest.length ? `, ${rest.join(', ')}` : ''}, two minutes on` },
		` ${c.kept} of ${c.clients} followed`);
}

// movesPanel lists each AP's last moves, newest first (0073).
function movesPanel(ctx, rows, band) {
	const name = (id) => ctx.name('locations', id);
	const lines = rows.flatMap(({ ap, cfg }) => {
		const moves = [...(cfg?.condition?.state?.report?.rrm?.moves || [])].reverse().filter((m) => !band || m.band === band);
		const apLink = link(`/aps/${encodeURIComponent(ap.id)}/radios/ratings`, ap.name);
		return moves.map((m, i) => h('tr', null,
			h('td', null, i === 0 && apLink),
			!band && h('td', null, bandName(m.band)),
			h('td', { class: 'mono' }, `${m.from} → ${m.to}`),
			h('td', null, WHY[m.why] || m.why),
			h('td', null, h('span', { class: `chip ${CAME[m.state] || 'idle'}` }, m.state === 'yielded' && m.ap ? `yielded to ${name(m.ap)}` : CAME_SAYS[m.state] || m.state), followedSays(m.clients)),
			h('td', null, ago(m.at * 1000))));
	});
	return h('section', { class: 'panel' },
		h('h2', null, 'Moves', h('span', { class: 'note' }, 'newest first; a move is announced to neighbours before it is made')),
		lines.length
			? h('table', { class: 'list' },
				h('tr', null, ['AP', ...(band ? [] : ['Band']), 'Channel', 'Why', 'What came of it', 'When'].map((c) => h('th', null, c))),
				lines)
			: h('div', { class: 'sub' }, 'No moves yet.'));
}
