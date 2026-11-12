// Problems › Alerts of a Locations folder (0099, 0284): what needs attention on the
// APs below it, most urgent first, each a link to its AP. The manager works
// them out on each look from what it has, so one goes when its cause does.
// Below them, the history (0109): each alert of the last day that lasted
// two minutes or more, when it began and how long it lasted. An alert of a
// client's, as one that comes online again and again (0190), opens its
// journey above the list. An AP in maintenance (0218) has its alerts last,
// greyed, said to be in maintenance, and counted apart. Each says itself
// in a few words, its short (0330), and in full a click down.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { ago, when } from '../format.js';
import { clientLink } from './journey.js';

const SEVERITY = { critical: ['bad', 'Critical'], warning: ['warn', 'Warning'], info: ['', 'Info'] };

const chip = (s) => h('span', { class: `chip ${SEVERITY[s]?.[0] ?? ''}` }, SEVERITY[s]?.[1] ?? s);

// said is something said in a few words, head, with the rest, more, a click
// down (0330). With no more, head alone.
export function said(head, ...more) {
	if (!more.some(Boolean)) return head;
	return h('details', { class: 'said' }, h('summary', null, head), h('div', { class: 'more' }, more));
}

// saidOf is an alert in its few words, and in full a click down, with
// what goes with it, extra; an alert from a manager before shorts, in full.
export const saidOf = (a, ...extra) => (a.short ? said(a.short, a.message, ...extra) : [a.message, ...extra]);

// The client an alert is of, where it is one's (0190, 0233), as a link to
// its journey.
function journey(a) {
	const of = ['rejoins:', 'struggling:'].find((p) => a.key?.startsWith(p));
	const mac = of ? a.key.slice(of.length) : '';
	return mac && h('div', null, clientLink(mac, 'Its journey'));
}

// lasted writes how long an alert lasted: 4 min, 2 h 10 min, 3 d.
function lasted(ms) {
	const m = Math.round(ms / 60000);
	if (m < 60) return `${Math.max(1, m)} min`;
	if (m < 2880) return `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ''}`;
	return `${Math.round(m / 1440)} d`;
}

// alertsTab draws the alerts under id; have, where given, is what the
// manager said of them already.
export async function alertsTab(ctx, id, have) {
	const [res, past] = await Promise.all([
		have || get(`/v1/alerts?under=${encodeURIComponent(id)}`),
		get(`/v1/alerts/history?under=${encodeURIComponent(id)}&hours=24`).catch(() => null), // an older manager has none
	]);
	const list = res.alerts || [];
	const c = res.counts || {};
	const summary = h('div', { class: 'alertsum' },
		['critical', 'warning', 'info'].map((s) => h('span', { class: `chip ${SEVERITY[s][0]}` }, `${c[s] || 0} ${SEVERITY[s][1].toLowerCase()}`)),
		c.maintenance > 0 && h('span', { class: 'chip', title: 'Alerts of APs in maintenance: listed and kept, never sent' }, `${c.maintenance} in maintenance`));
	const now = list.length
		? h('section', { class: 'panel' }, summary,
			h('table', { class: 'alerts' },
				h('thead', null, h('tr', null, ['', 'AP', 'What', 'Since'].map((t) => h('th', null, t)))),
				h('tbody', null, list.map((a) => h('tr', { class: a.maintenance ? 'maint' : a.severity },
					h('td', null, a.maintenance ? h('span', { class: 'chip', title: `${SEVERITY[a.severity]?.[1] ?? a.severity}, not sent: its AP is in maintenance` }, 'Maintenance') : chip(a.severity)),
					h('td', null, link(`/locations/${encodeURIComponent(a.ap)}`, a.name)),
					h('td', null, saidOf(a, journey(a), a.maintenance && h('div', { class: 'sub' }, `Its AP is ${a.maintenance}.`))),
					h('td', { class: 'sub' }, a.since ? ago(a.since) : ''))))))
		: h('section', { class: 'panel' }, summary, h('p', { class: 'sub' }, 'Nothing needs attention: every AP here is calling in, runs its config, and reports nothing wrong.'));
	return [now, past && historyPanel(past.alerts || [])];
}

function historyPanel(list) {
	return h('section', { class: 'panel' },
		h('h2', null, 'History', h('span', { class: 'note' }, 'the last 24 hours: each alert that lasted two minutes or more, newest first')),
		list.length
			? h('table', { class: 'alerts' },
				h('thead', null, h('tr', null, ['', 'AP', 'What', 'Began', 'Lasted'].map((t) => h('th', null, t)))),
				h('tbody', null, list.map((a) => h('tr', { class: a.ended ? null : a.severity },
					h('td', null, chip(a.severity)),
					h('td', null, link(`/locations/${encodeURIComponent(a.ap)}`, a.name)),
					h('td', null, saidOf(a)),
					h('td', { class: 'sub', title: ago(a.began) }, when(a.began)),
					h('td', { class: 'sub' }, a.ended ? lasted(new Date(a.ended) - new Date(a.began)) : `still, ${lasted(Date.now() - new Date(a.began))}`)))))
			: h('p', { class: 'sub' }, 'No alert lasted in the last 24 hours.'));
}
