// A Wi-Fi client's own page (0222), where a click on its name anywhere
// leads: Roaming, a folder's Clients, a network's steering, an alert, a
// finding. Who it is and where it is now, over three tabs (0337):
// - Overview, its dashboard, as a folder's Overview: how it is now and
//   over the day, and over time (clientdash.js).
// - Activity, everything it did, a line each, the latest first, each time
//   it came to an AP, a join, a roam or a band switch, passed or failed,
//   with where that AP is in Locations, each opening to its steps (0118);
//   in a box that scrolls, not the whole page.
// - Where it stayed, its sessions over the last day or week, from the APs'
//   state reports (0103).

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { redrawNow } from '../refresh.js';
import { given, called, itsName, nameClient } from '../names.js';
import { bandName, ago } from '../format.js';
import { attemptsPanel, where, PAGE, kept } from './connections.js';
import { leavingPanel } from './leaving.js';
import { askButton } from './advisor.js';
import { traceWindow } from './trace.js';
import { captureWindow } from './captures.js';
import { lookupsWindow } from './lookups.js';
import { pathWindow } from './pathtrace.js';
import { mayAt, mayAnywhere } from '../rights.js';
import { ask, confirm } from './confirm.js';
import { tabBar, pick as pickTab } from '../layout.js';
import { clientStrip, clientChart, lately, minutesOf } from './clientdash.js';

// Its page's tabs (0337).
const TABS = [['overview', 'Overview'], ['activity', 'Activity'], ['stays', 'Where it stayed']];

const when = (t) => new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });

function span(s) {
	const min = Math.max(1, Math.round((new Date(s.to) - new Date(s.from)) / 60000));
	return min < 90 ? `${min} min` : `${Math.round(min / 6) / 10} h`;
}

// clientLink is a client's name as the way to its page; at, an attempt of
// its for the page to open, as a roam that went badly.
export function clientLink(mac, label, at) {
	return h('a', { href: `#/clients/${mac}${at ? `/activity/${at}` : ''}`, class: 'clientlink', title: 'Its page: how it is, everything it did, and where' }, label || called(mac) || mac);
}

// nameForm is where a person names a client (0153), in slot: what they call
// it, in place of what it calls itself, or of nothing. An empty name takes
// the name away. done is called once it is kept.
function nameForm(slot, mac, j, done, cancel) {
	const has = j.name || given(mac);
	const input = h('input', { type: 'text', value: has, maxlength: 64, placeholder: j.host || 'Bambu printer', 'aria-label': `A name for ${j.host || mac}` });
	const msg = h('div', { class: 'error' });
	const save = async (name) => {
		msg.replaceChildren();
		if ([...name.trim()].length > 64) {
			msg.replaceChildren('A name is at most 64 characters.');
			return;
		}
		try {
			await nameClient(mac, name.trim());
		} catch (e) {
			msg.replaceChildren(e.message);
			return;
		}
		done();
	};
	input.addEventListener('keydown', (e) => { if (e.key === 'Enter') save(input.value); });
	slot.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		h('label', { class: 'field' }, h('span', { class: 'label' }, 'What to call it'), input),
		h('div', { class: 'sub' }, 'Shown wherever this client is, and found by the search box. Nothing is told to the device or to an AP.'),
		msg,
		h('div', { class: 'actions' }, ...[
			h('button', { type: 'button', class: 'button primary', onclick: () => save(input.value) }, 'Save'),
			has && h('button', { type: 'button', class: 'button', onclick: () => save('') }, 'Remove the name'),
			h('button', { type: 'button', class: 'button', onclick: () => (cancel ? cancel() : slot.replaceChildren()) }, 'Cancel')].filter(Boolean))));
	input.focus();
	input.select();
}

// renameClient names a client from a list (0153), in box: the form alone,
// and the page drawn again once the name is kept. host is what the device
// calls itself.
export function renameClient(box, mac, host) {
	const slot = h('div');
	box.replaceChildren(h('section', { class: 'panel', 'data-editing': true },
		h('h2', null, `Rename ${given(mac) || host || mac}`, h('span', { class: 'note mono' }, mac)), slot));
	nameForm(slot, mac, { host, name: given(mac) }, () => { box.replaceChildren(); redrawNow(); }, () => box.replaceChildren());
}

// homeOf is where a person said a client belongs (0319), as the Org has it:
// the APs, the bands (none for any) and the hold (none for its default);
// null where it has no rule.
function homeOf(page, mac) {
	const f = page?.fields || {};
	const at = (k) => f[`homes.${mac}.${k}`]?.value;
	// Of the nearest (0321): wherever it is heard best, among the APs where
	// it names some.
	const nearest = at('nearest') === true;
	// Where it is set: changed there, not shadowed below it.
	const from = f[`homes.${mac}.aps`]?.from || f[`homes.${mac}.nearest`]?.from;
	return at('aps')?.length || nearest ? { aps: at('aps') || [], bands: at('bands') || [], hold: at('hold'), nearest, from } : null;
}

// siteOf is the site an AP is in: the folder under the Org it is below, or
// the Org where it is right under it. A client's steering is set there, so a
// change to it is the site's APs' alone, at a site of thousands of APs as at
// one of six (Griff, 2026-11-05).
function siteOf(ctx, ap) {
	const t = ctx.trees.locations;
	let n = ap && t.nodes.get(ap);
	while (n && n.parent && n.parent !== t.root) n = t.nodes.get(n.parent);
	return n && n.id !== ap && n.kind !== 'ap' ? n.id : t.root;
}

// apsIn is the APs below a node, by name: those a client there may be
// steered to.
function apsIn(ctx, node) {
	const t = ctx.trees.locations;
	const below = (n) => {
		for (let p = n.parent; p; p = t.nodes.get(p)?.parent) if (p === node) return true;
		return false;
	};
	return t.list.filter((n) => n.kind === 'ap' && n.parent !== 'landing-zone' && (node === t.root || below(n))).sort((a, b) => a.name.localeCompare(b.name));
}

const BAND_NAMES = { '2g': '2.4 GHz', '5g': '5 GHz', '6g': '6 GHz' };

// homeSaid says a rule in a few words: the APs by name, and the bands.
function homeSaid(ctx, r) {
	const aps = r.aps.map((id) => ctx.name('locations', id)).join(', ');
	const where = r.nearest ? `the nearest AP${aps ? ` of ${aps}` : ''}` : aps;
	return `${where}${r.bands.length ? ` · ${r.bands.map((b) => BAND_NAMES[b]).join(', ')}` : ''}`;
}

// nearestAPs are the APs of aps that heard a client over the last week
// (0341), the best heard first: on the band it was on most, each AP's
// middle signal over its minutes there, of ten at least. A client is
// compared on one band alone, as 2.4 GHz carries further than 5 or 6.
async function nearestAPs(mac, aps) {
	const mins = await minutesOf(mac, 168).catch(() => []);
	const ids = new Set(aps.map((n) => n.id));
	const per = new Map();
	for (const m of mins) per.set(m.band, (per.get(m.band) || 0) + 1);
	const band = [...per.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
	const by = new Map();
	for (const m of mins) {
		if (m.band !== band || m.signal == null || !ids.has(m.ap)) continue;
		if (!by.has(m.ap)) by.set(m.ap, []);
		by.get(m.ap).push(m.signal);
	}
	const middle = (v) => [...v].sort((a, b) => a - b)[Math.floor(v.length / 2)];
	return [...by.entries()].filter(([, v]) => v.length >= 10)
		.map(([id, v]) => ({ id, name: aps.find((n) => n.id === id)?.name || id, band, signal: middle(v), minutes: v.length }))
		.sort((a, b) => b.signal - a.signal || b.minutes - a.minutes);
}

const minutesSaid = (n) => (n >= 120 ? `${Math.round(n / 60)} h` : `${n} min`);

// homeForm is client steering (0319, 0321), in slot: Steer to, a pulldown
// of Off, Nearest AP or One AP, and for one AP a box that finds it by name
// as it is typed, among the APs of the client's site, site; and the bands,
// none for any (Griff, 2026-11-05: a pulldown, short words, and it must hold
// at sites of thousands of APs). A rule this form cannot make, of several
// APs, is kept as it is, as its own entry. It is set where the rule is set,
// else at the site, in one change previewed first.
let lists = 0;
function homeForm(ctx, slot, mac, who, rule, site) {
	const node = rule?.from || site;
	const aps = apsIn(ctx, site);
	const byName = new Map(aps.map((n) => [n.name.toLowerCase(), n.id]));
	const now = !rule ? 'off' : rule.nearest && !rule.aps.length ? 'nearest' : !rule.nearest && rule.aps.length === 1 ? 'ap' : 'kept';
	const to = h('select', { 'aria-label': 'Steer to' },
		h('option', { value: 'off' }, 'Off'),
		h('option', { value: 'nearest' }, 'Nearest AP'),
		h('option', { value: 'ap' }, 'Pin to an AP…'),
		now === 'kept' && h('option', { value: 'kept' }, homeSaid(ctx, { ...rule, bands: [] })));
	to.value = now;
	const id = `steer-aps-${++lists}`;
	const ap = h('input', { type: 'search', list: id, placeholder: `Find an AP in ${ctx.name('locations', site)}`, 'aria-label': 'The AP',
		value: now === 'ap' ? ctx.name('locations', rule.aps[0]) : '' });
	const list = h('datalist', { id }, aps.map((n) => h('option', { value: n.name })));
	const near = h('span', { class: 'sub' });
	const apRow = h('div', { class: 'field' }, h('span', { class: 'label' }, 'AP'), ap, list, near);
	// Its closest AP first (Griff, 2026-11-08): the APs of the site that
	// heard it over the last week, the best heard first, on the band it was
	// on most; picked for it as Pin is chosen, where no AP is picked yet.
	let closest = null;
	nearestAPs(mac, aps).then((ranked) => {
		if (!ranked.length) return;
		closest = ranked[0];
		const named = new Set(ranked.map((r) => r.id));
		list.replaceChildren(...ranked.map((r) => h('option', { value: r.name, label: `${r.signal} dBm on ${BAND_NAMES[r.band]} · ${minutesSaid(r.minutes)}` })),
			...aps.filter((n) => !named.has(n.id)).map((n) => h('option', { value: n.name })));
		near.replaceChildren(`closest: ${closest.name}, heard at ${closest.signal} dBm on ${BAND_NAMES[closest.band]} this week`);
		if (to.value === 'ap' && !ap.value) ap.value = closest.name;
	});
	// Pinned to an AP, the steering that picks an AP is no longer a thing
	// for it (Griff, 2026-11-08): the nearest AP, sticky's moves and its
	// weak kicks are greyed, and Bands alone picks its band there.
	const moves = h('div', { class: 'field' }, h('span', { class: 'label' }, 'AP steering'),
		h('div', { class: 'chiprow flush' }, ['Nearest AP', 'Sticky: heard better', 'Sticky: weak'].map((t) => h('span', { class: 'chip off' }, t)),
			h('span', { class: 'sub' }, 'none of these moves it while it is pinned: it stays on its AP')));
	const shown = () => {
		apRow.hidden = to.value !== 'ap';
		moves.hidden = to.value !== 'ap';
		if (!apRow.hidden && !ap.value && closest) ap.value = closest.name;
		if (!apRow.hidden) ap.focus();
	};
	to.addEventListener('change', shown);
	const band = new Map(Object.keys(BAND_NAMES).map((b) => [b, h('input', { type: 'checkbox', checked: !!rule?.bands.includes(b) })]));
	const msg = h('div', { class: 'error' });
	const path = (k) => `homes.${mac}.${k}`;
	const save = async () => {
		msg.replaceChildren();
		const pick = to.value;
		const one = pick === 'ap' ? byName.get(ap.value.trim().toLowerCase()) : null;
		if (pick === 'ap' && !one) {
			msg.replaceChildren(`No AP called “${ap.value.trim()}” in ${ctx.name('locations', site)}.`);
			return;
		}
		const bands = [...band].filter(([, el]) => el.checked).map(([b]) => b);
		const ops = [];
		const set = (k, v) => ops.push({ kind: 'set', tree: 'locations', node, path: path(k), value: v });
		const unset = (k) => ops.push({ kind: 'unset', tree: 'locations', node, path: path(k) });
		if (pick === 'off') {
			// The rule is taken away, all of it.
			if (rule?.aps.length) unset('aps');
			if (rule?.nearest) unset('nearest');
			if (rule?.bands.length) unset('bands');
			if (rule?.hold != null) unset('hold');
		} else {
			if (pick === 'nearest') {
				if (!rule?.nearest) set('nearest', true);
				if (rule?.aps.length) unset('aps');
			} else if (pick === 'ap') {
				if (rule?.aps.length !== 1 || rule.aps[0] !== one) set('aps', [one]);
				if (rule?.nearest) unset('nearest');
			}
			const was = rule?.bands || [];
			if (bands.join() !== was.join()) {
				if (bands.length) set('bands', bands);
				else unset('bands');
			}
		}
		if (!ops.length) {
			slot.replaceChildren();
			return;
		}
		const op = ops.length === 1 ? ops[0] : { kind: 'batch', ops };
		const box = h('div');
		slot.replaceChildren(box);
		const p = await ask(box, op);
		if (!p) return;
		const after = pick === 'off' ? null : pick === 'kept' ? { ...rule, bands } : { aps: pick === 'nearest' ? [] : [one], bands, nearest: pick === 'nearest' };
		confirm(ctx, box, op, p, [
			h('div', null, h('strong', null, after ? `${after.nearest || after.aps.length !== 1 ? 'Steer' : 'Pin'} ${who} to ${homeSaid(ctx, after)}` : `Client steering off for ${who}`)),
			h('div', { class: 'sub' }, !after
				? 'Its rule is taken away: steering treats it as any other client.'
				: `${after.nearest
					? 'On a steered network, the APs compare how strongly each hears it, by its probes and their scan radios, and send it to one that hears it 5 dB or more better than the AP it is on; kept off for 30 seconds if it stays. Back on an AP twice after that, it is left there a week.'
					: 'On a steered network, an AP it is on that is not the one it is pinned to sends it there, and keeps it off for 30 seconds if it stays; only to a band it is known to use, so a client seen only on 2.4 GHz is never sent up. Pinned, no other steering moves it off its AP: not the nearest AP, nor sticky. Aeolus warns if its signal there falls far below its usual, as when it is moved (pin-moved).'} Set for ${ctx.name('locations', node)}.`),
		], [h('div', { class: 'sub' }, 'Applying changes only the steering daemon’s settings: no Wi-Fi restarts.')]);
	};
	const tick = (el, label) => h('label', { class: 'tick' }, el, ` ${label}`);
	slot.replaceChildren(h('div', { class: 'fieldform', 'data-editing': true },
		h('label', { class: 'field' }, h('span', { class: 'label' }, 'Steer to'), to),
		apRow,
		moves,
		h('div', { class: 'field' }, h('span', { class: 'label' }, 'Bands'), h('div', { class: 'chiprow flush' }, [...band].map(([b, el]) => tick(el, BAND_NAMES[b])),
			h('span', { class: 'sub' }, 'none ticked: any'))),
		msg,
		h('div', { class: 'actions' }, ...[
			h('button', { type: 'button', class: 'button primary', onclick: save }, 'Preview'),
			h('button', { type: 'button', class: 'button', onclick: () => slot.replaceChildren() }, 'Cancel')])));
	shown();
}

// The hours of sessions shown, kept as the page is drawn again.
const view = { hours: 24 };

// The attempt a page was opened at, once brought into view: drawn again, the
// page stays where its reader scrolled.
let shown = '';

// clientPage is a client's own page, by its MAC; at, an attempt of its to
// open and bring into view.
export async function clientPage(ctx, mac, tab, at) {
	mac = mac.toLowerCase();
	// An attempt asked for is on Activity: so an address from before the
	// tabs, #/clients/<mac>/<attempt>, still opens it.
	tab = at && !tab ? 'activity' : pickTab(TABS, tab);
	const base = `#/clients/${mac}`;
	let j, res, left, mins;
	try {
		[j, res, left, mins] = await Promise.all([
			// Its sessions: over the day on its Overview, over the day or
			// week asked for where it stayed.
			get(`/v1/clients/${encodeURIComponent(mac)}?hours=${tab === 'stays' ? view.hours : 24}`),
			// Its attempts: on its Overview, the day's, for its strip; on
			// Activity, the latest PAGE, older ones read as it is scrolled.
			get(`/v1/clients/${encodeURIComponent(mac)}/connections?${tab === 'overview' ? `limit=200&since=${new Date(Date.now() - 24 * 3600 * 1000).toISOString()}` : `limit=${Math.min(200, PAGE + kept(mac))}`}`),
			// Why it left, where it did (0255), on its Overview.
			tab === 'overview' ? leavingPanel({ mac }) : null,
			tab === 'overview' ? minutesOf(mac, 24) : [],
		]);
	} catch (e) {
		return { main: [h('div', { class: 'error' }, e.message)] };
	}
	const cl = res.client;
	// What it is called: the name a person gave it (0153), else its own.
	const name = j.name || given(mac);
	const title = name || called(mac, j.host || cl?.host) || mac;
	const slot = h('div', { class: 'edit flush' });
	const rename = !j.tester && h('button', { type: 'button', class: 'button small rename', title: 'Give this client a name of your own, shown wherever it is',
		onclick: () => nameForm(slot, mac, j, () => redrawNow(), () => slot.replaceChildren()) }, 'Rename…');
	const sessions = [...j.sessions].reverse();
	// Its steering (0319, 0321), as its site has it: the site of the AP it
	// was last on, else the Org.
	const site = siteOf(ctx, sessions[0]?.ap || cl?.ap?.id || cl?.ap);
	const org = await get(`/v1/trees/locations/nodes/${encodeURIComponent(site)}`).catch(() => null);
	const rule = homeOf(org, mac);
	const homeSlot = h('div', { class: 'edit flush' });
	// Client steering (0319, 0321): Griff named it, 2026-11-05.
	const homeButton = !j.tester && mayAt('locations', rule?.from || site, 'clients') && h('button', { type: 'button', class: 'button small', title: 'Steer this client to the nearest AP, or pin it to an AP, and the bands you pick',
		onclick: () => homeForm(ctx, homeSlot, mac, name || j.host || mac, rule, site) }, 'Steering…');
	// Where it is now, by its latest session: one the latest reports still
	// have it in.
	const latest = sessions[0];
	const now = latest && Date.now() - new Date(latest.to) < 15 * 60000 ? latest : null;
	// Watched live, as it moves from AP to AP (0144). A trace is as deep as
	// an operator goes; a capture is an admin's (0177).
	const tools = [
		homeButton,
		mayAnywhere('clients') && h('button', { type: 'button', class: 'button small', title: 'Every AP says each thing it sees of this client as it happens',
			onclick: () => traceWindow(mac, name || j.host, now && { ap: now.ap, ap_name: now.name }) }, 'Live Trace'),
		// What it asks the DNS server for (0154): what a device asks for
		// says what it is.
		h('button', { type: 'button', class: 'button small', title: 'The names this client looks up, and the domains it asks for most', onclick: () => lookupsWindow(mac, name || j.host) }, 'Lookups'),
		// Its way out, by what the switches say (0168).
		h('button', { type: 'button', class: 'button small', title: 'Its way out: its AP, the switches, its gateway, and on to the internet', onclick: () => pathWindow(mac, name || j.host) }, 'Path'),
		// Its traffic alone, taken on the AP it is on (0148).
		latest?.ap && mayAt('locations', latest.ap, 'capture') && h('button', { type: 'button', class: 'button small', title: 'Capture this client’s traffic on the AP it is on: the Wi-Fi side, the wired side, or both',
			onclick: () => captureWindow({ mac, host: name || j.host, ap: latest.ap, ap_name: latest.name, network: latest.network, ssid: latest.ssid }) }, 'Capture'),
		// The advisor asked about it (0215), but a test client.
		!j.tester && askButton(`About the client ${name || j.host || mac} (${mac}): `),
	].filter(Boolean);
	const its = itsName(mac, j.host);
	const head = h('div', { class: 'clienthead' },
		h('h1', null, title, rename),
		slot,
		h('div', { class: 'chiprow flush' },
			h('span', { class: 'chip idle mono' }, mac),
			name && its && its !== name && h('span', { class: 'chip idle', title: 'What it calls itself' }, its),
			cl?.private ? h('span', { class: 'chip idle', title: 'A private (random) address, which a phone makes up for each network' }, 'private address') : cl?.maker && h('span', { class: 'chip idle' }, cl.maker),
			cl?.user && h('span', { class: 'chip idle', title: 'Who it signed in as, by 802.1X' }, `signed in as ${cl.user}`),
			j.tester && h('span', { class: 'chip from' }, 'Aeolus test client'),
			rule && h('span', { class: 'chip ok', title: 'Client steering (0319, 0321): on a steered network it is sent there' }, `${!rule.nearest && rule.aps.length === 1 ? 'pinned' : 'steered'} to ${homeSaid(ctx, rule)}`)),
		h('div', { class: 'sub' }, now
			? ['Now on ', link(`/aps/${encodeURIComponent(now.ap)}/clients`, where(ctx, now.ap, now.name)), ` · ${[now.ssid || now.network, bandName(now.band), now.signal_avg != null && `${now.signal_avg} dBm`, now.address].filter(Boolean).join(' · ')}`]
			: latest ? ['Last seen ', ago(latest.to), ' on ', link(`/aps/${encodeURIComponent(latest.ap)}/clients`, where(ctx, latest.ap, latest.name))]
				: cl?.last_seen ? `Last seen ${ago(cl.last_seen)}` : 'Not seen by any AP you may view in this time.'),
		name && j.named_by && h('div', { class: 'sub' }, `Named by ${j.named_by}${j.named_at ? `, ${ago(j.named_at)}` : ''}.`),
		j.tester && h('div', { class: 'sub' }, `${j.tester === 'an AP' ? 'An AP' : j.tester}'s scan radio, joining as a client would to test the network. Its short stays and its signal are the tests', and nothing to look into.`),
		tools.length > 0 && h('div', { class: 'controls' }, tools),
		homeSlot);
	const pick = (v, label) => h('button', { type: 'button', class: view.hours === v ? 'on' : '', 'aria-pressed': view.hours === v, onclick: () => { view.hours = v; redrawNow(); } }, label);
	const tabs = tabBar(base.slice(1), TABS, tab);
	const top = h('div', { class: 'head' }, head);
	if (tab === 'activity') {
		// Opened at an attempt, the page brings it into view the first time
		// it is drawn: after that, it stays where it was scrolled.
		const key = `${mac}/${at || ''}`;
		if (at && shown !== key) requestAnimationFrame(() => document.getElementById(`attempt-${at}`)?.scrollIntoView({ block: 'center' }));
		shown = key;
		return { main: [top, tabs, attemptsPanel(ctx, mac, res, at, true, PAGE)], refresh: 60 };
	}
	const issues = j.issues.length > 0 && h('section', { class: 'panel' },
		h('h2', null, 'To look at'),
		h('ul', { class: 'issues' }, j.issues.map((i) => h('li', null,
			h('span', { class: `chip ${i.severity === 'warning' ? 'warn' : ''}` }, i.severity === 'warning' ? 'Look at' : 'Note'), ' ', i.message))));
	const stays = h('section', { class: 'panel' },
		h('h2', null, h('span', { class: 'title' }, 'Where it stayed', h('span', { class: 'segmented', role: 'group' }, pick(24, 'A day'), pick(168, 'A week'))),
			h('span', { class: 'note' }, sessions.length
				? `${sessions.length} session${sessions.length === 1 ? '' : 's'}, ${j.roams} roam${j.roams === 1 ? '' : 's'}, from the APs’ reports every few minutes`
				: 'from the APs’ reports every few minutes')),
		sessions.length
			? h('div', { class: 'widetable scrollbox' }, h('table', { class: 'list dense' },
				h('tr', null, ['From', 'For', 'Where', 'Band', 'Network', 'Signal', 'Rate', 'Retries', 'Address'].map((t) => h('th', t === 'Retries' ? { title: 'Over its busy minutes alone' } : null, t))),
				sessions.map((s) => h('tr', null,
					h('td', { class: 'mono' }, when(s.from)),
					h('td', null, span(s)),
					h('td', null, link(`/aps/${encodeURIComponent(s.ap)}/clients`, where(ctx, s.ap, s.name))),
					h('td', null, bandName(s.band) || '—', s.gen && h('span', { class: 'sub' }, ` · 802.11${s.gen}`)),
					h('td', null, s.ssid || s.network || '—'),
					h('td', null, s.signal_avg != null ? `${s.signal_avg} dBm` : '—', s.signal_min != null && s.signal_min !== s.signal_max && h('span', { class: 'sub' }, ` (${s.signal_min} to ${s.signal_max})`)),
					h('td', null, s.tx_rate ? `${Math.round(s.tx_rate)} Mbit/s` : '—'),
					h('td', { title: s.retries != null
						? `Sent again, of what was sent to it, over the ${s.busy_minutes === 1 ? 'minute' : `${s.busy_minutes} minutes`} it moved a megabyte or more`
						: 'No minute it moved a megabyte in: idle, what is sent again is mostly rate control trying faster rates' }, s.retries != null ? `${Math.round(s.retries * 100)}%` : '—'),
					h('td', { class: 'mono' }, s.address || '—', s.dhcp && h('span', { class: 'sub' }, ` · DHCP ${s.dhcp}`))))))
			: h('p', { class: 'sub pad' }, `No AP you can see reported it in the last ${view.hours === 168 ? 'week' : `${view.hours} hours`}.`));
	if (tab === 'stays') return { main: [top, tabs, stays], refresh: 60 };
	// Its dashboard (0337): the strip; over time, beside what to look at and
	// its latest attempts; why it left.
	const chartPanel = await clientChart(mac, redrawNow);
	return {
		main: [top, tabs,
			clientStrip(ctx, j, res, mins, now, latest, base),
			h('div', { class: 'overgrid' }, chartPanel,
				h('div', { class: 'side' }, issues || h('section', { class: 'panel' }, h('h2', null, 'To look at'), h('p', { class: 'sub pad' }, 'Nothing: no trouble seen with it over the day.')),
					lately(ctx, mac, res, base))),
			left],
		refresh: 60,
	};
}
