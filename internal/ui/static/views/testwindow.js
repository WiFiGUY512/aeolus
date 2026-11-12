// Maintenance (0218) and its test window (0344) on an AP's page: a banner
// while the AP is in maintenance, and Test window… in its heading, which
// holds it still for a test in one change, and ends by itself: on the AP by
// its clock, and on the manager, where the change ends itself (0345) and
// its settings go.

import { h } from '../dom.js';
import { get, post } from '../api.js';
import { flash, hurry, redrawNow } from '../refresh.js';

const FOR = [[30, '30 minutes'], [60, '1 hour'], [120, '2 hours'], [240, '4 hours'], [480, '8 hours']];
const PATHS = ['maintenance.on', 'maintenance.until', 'maintenance.why', 'maintenance.hold'];
const HELD = 'no channel or power moves, no steering, no agent updates and no client tests on a schedule';

// untilOf reads maintenance.until: a day and a time in UTC, or a day alone,
// as it begins.
function untilOf(s) {
	if (!s) return null;
	const t = new Date(s.length === 10 ? `${s}T00:00:00Z` : s.replace(/Z$/, ':00Z'));
	return Number.isNaN(t.getTime()) ? null : t;
}

// untilSaid is a time as maintenance.until takes it: UTC, to the minute.
const untilSaid = (t) => t.toISOString().slice(0, 16) + 'Z';

const hhmm = (t) => t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

function left(t) {
	const m = Math.max(0, Math.round((t - Date.now()) / 60000));
	return m >= 60 ? `${Math.floor(m / 60)} h ${m % 60} min left` : `${m} min left`;
}

async function send(box, op, reason, said, path = '/v1/changes') {
	try {
		const res = await post(path, path === '/v1/changes' ? { op, reason } : { ...op, reason });
		hurry(150);
		flash(`${said} Saved as change #${res.change.seq}.`);
		box.replaceChildren();
		redrawNow();
	} catch (e) {
		box.replaceChildren(h('div', { class: 'error' }, e.message));
	}
}

// windowOf is the AP's maintenance as its folders say, now: null where it is
// not in maintenance, or its end has come.
function windowOf(cfg, page) {
	const m = cfg.document?.maintenance;
	if (m?.on !== true) return null;
	const until = untilOf(m.until);
	if (until && !(until > new Date())) return null;
	return { until, why: m.why || '', hold: m.hold === true && until != null, from: page.fields?.['maintenance.on']?.from };
}

// endOf is the change that ends itself (0345) and set maintenance.on at
// node, if the caller may read what ends: its seq, or null.
async function endOf(node) {
	try {
		const { ends } = await get('/v1/ends');
		const e = (ends || []).find((x) => x.steps.some((s) => s.tree === 'locations' && s.node === node && s.path === 'maintenance.on'));
		return e ? e.seq : null;
	} catch {
		return null;
	}
}

// maintenanceBanner says the AP is in maintenance (0218), or in a test
// window (0344) with what it holds, until when; with, for one who may
// change it, a test window's More time and End now.
export function maintenanceBanner(ctx, id, page, cfg, edit, box) {
	const w = windowOf(cfg, page);
	if (!w) return null;
	const end = w.until ? `until ${hhmm(w.until)} (${left(w.until)})` : 'until it is taken off';
	const why = w.why ? `: ${w.why}` : '';
	if (!w.hold) {
		return h('div', { class: 'banner info' }, h('strong', null, 'In maintenance'),
			` ${end}${why}. Its alerts are kept and listed, and none is sent.`);
	}
	const at = w.from || id;
	const where = at !== id ? ` Set at ${ctx.name('locations', at)}: it holds every AP there.` : '';
	// Only what is set where the window is, so the unset takes all of it.
	const set = PATHS.filter((p) => page.fields?.[p]?.from === at);
	return h('div', { class: 'banner info', style: { 'align-items': 'center', gap: '8px', 'flex-wrap': 'wrap' } },
		h('strong', null, 'Test window'),
		h('span', null, ` ${end}${why}. Held still: ${HELD}. Its alerts are kept, and none is sent. It ends by itself.${where}`),
		edit && h('span', { class: 'controls' },
			h('button', { type: 'button', class: 'button small', onclick: async () => {
				const later = new Date(Math.max(w.until.getTime(), Date.now()) + 60 * 60000);
				const said = `The test window now ends at ${hhmm(later)}.`;
				const seq = await endOf(at);
				// Its end moved, and maintenance.until with it, as part of
				// the change, so both go at the end (0345).
				if (seq) {
					send(box, { until: later.toISOString(), tree: 'locations', node: at, values: { 'maintenance.until': untilSaid(later) } },
						'test window one hour longer', said, `/v1/ends/${seq}`);
					return;
				}
				send(box, { kind: 'set', tree: 'locations', node: at, path: 'maintenance.until', value: untilSaid(later) },
					'test window one hour longer', said);
			} }, '1 h more'),
			h('button', { type: 'button', class: 'button small', onclick: async () => {
				const said = 'The test window is over: the AP moves and steers again within a minute.';
				const seq = await endOf(at);
				// Its end now: the window, and what was set with it, put back.
				if (seq) {
					send(box, {}, 'test window ended', said, `/v1/ends/${seq}`);
					return;
				}
				send(box, { kind: 'unset', tree: 'locations', node: at, paths: set }, 'test window ended', said);
			} }, 'End now')));
}

// testWindowButton is Test window… in an AP's heading (0344), for one who
// may change its settings, where it is not in one already: how long, and
// what for, then held still in one change.
export function testWindowButton(id, name, cfg, page, box, cls = 'rename head') {
	if (windowOf(cfg, page)?.hold) return null;
	return h('button', { type: 'button', class: `button small ${cls}`, title: `Hold it still for a test: ${HELD}`, onclick: () => {
		const dur = h('select', { 'aria-label': 'For how long' }, FOR.map(([m, label]) => h('option', { value: String(m), selected: m === 60 }, label)));
		const why = h('input', { type: 'text', maxlength: 200, placeholder: 'What is tested, by whom', 'aria-label': 'What for', style: { 'min-width': '260px' } });
		box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
			h('div', null, h('strong', null, `Test window on ${name}`)),
			h('div', { class: 'sub' }, `Held still: ${HELD}. Its alerts are kept, and none is sent. It ends by itself, by the AP's own clock, whether it reaches the manager or not, and through a sysupgrade; and on the manager, which takes its settings away then.`),
			h('div', { class: 'row' }, 'For ', dur, ' ', why),
			h('div', { class: 'actions' },
				h('button', { type: 'button', class: 'button primary', onclick: (e) => {
					e.currentTarget.disabled = true;
					// To the next whole minute, as maintenance.until is said.
					const end = new Date(Math.ceil((Date.now() + Number(dur.value) * 60000) / 60000) * 60000);
					const values = { 'maintenance.on': true, 'maintenance.until': untilSaid(end), 'maintenance.hold': true };
					if (why.value.trim()) values['maintenance.why'] = why.value.trim();
					send(box, { kind: 'set', tree: 'locations', node: id, values, until: end.toISOString() },
						`test window for ${dur.selectedOptions[0].textContent}`, `${name} holds still from its next poll, within a minute, until ${hhmm(end)}.`);
				} }, 'Start'),
				h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
	} }, 'Test window…');
}
