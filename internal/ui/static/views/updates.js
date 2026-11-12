// The manager's own releases (0183): a banner when a newer one is out, and
// the Updates panel on the Org's System tab, for an admin at the Org. The
// manager asks its updater, which runs as root on its host; it restarts as
// it installs one, for a minute or so, and the APs keep running.

import { h, link } from '../dom.js';
import { get, post } from '../api.js';
import { ago, when, prose } from '../format.js';
import { flash, hurry, redrawNow } from '../refresh.js';
import { mayAt } from '../rights.js';

const DISMISSED = 'aeolus.update.dismissed';

// updateOf is the updater's word, for one who may run the releases; null
// for anyone else, or where it cannot be read.
export async function updateOf(ctx) {
	const root = ctx.trees.locations.root;
	if (!root || !mayAt('locations', root, 'administer')) return null;
	try {
		return await get('/v1/system/update');
	} catch {
		return null;
	}
}

async function askUpdater(action, said) {
	try {
		await post('/v1/system/update', { action });
		hurry(60);
		flash(said);
	} catch (e) {
		flash(e.message);
	}
	redrawNow();
}

const updateNow = (u) => askUpdater('update', `Installing ${u.target}. The manager restarts as it does; this page comes back by itself in a minute or so.`);

// updateBanner is the line at the top of every page: a newer release is
// out, or one is being installed. Null otherwise, or once put away for
// that release.
export function updateBanner(ctx, u) {
	if (!u?.installed) return null;
	if (u.state === 'updating' || (u.pending && u.newer)) {
		// Asked and not yet taken up, it is the newest on the channel (0282); taken up, the one the
		// updater says it installs.
		const which = u.state === 'updating' ? u.tag : u.target;
		return h('div', { class: 'banner info' }, h('strong', null, `Installing ${which}…`), ' The manager restarts as it does; this page comes back by itself.');
	}
	if (!u.newer) return null;
	let put = null;
	try { put = localStorage.getItem(DISMISSED); } catch { /* storage blocked */ }
	if (put === u.target) return null;
	const root = ctx.trees.locations.root;
	const box = h('div', { class: 'banner info update' },
		h('span', null, h('strong', null, `Aeolus ${u.target} is out.`), ` This manager runs ${u.running || 'an older release'}.`,
			whatsNew(u)),
		h('span', { class: 'controls' },
			h('button', { type: 'button', class: 'button small primary', onclick: () => updateNow(u) }, 'Update now'),
			link(`/locations/${encodeURIComponent(root)}/settings`, 'Details'),
			h('button', { type: 'button', class: 'button small', title: 'Not now: shown again for the next release', onclick: () => {
				try { localStorage.setItem(DISMISSED, u.target); } catch { /* storage blocked */ }
				box.remove();
			} }, 'Not now')));
	return box;
}

// whatsNew is what each release newer than the one running says of
// itself (0217), folded: its tag's message, the newest first.
function whatsNew(u) {
	const notes = (u.notes || []).filter((n) => n.notes);
	if (!notes.length) return null;
	return h('details', { class: 'whatsnew' }, h('summary', null, 'What’s new'),
		notes.map((n) => h('div', { class: 'release' }, h('strong', null, n.version), prose(n.notes))));
}

// updatesPanel is the Updates panel on the Org's System tab.
export function updatesPanel(ctx, u) {
	if (!u) return null;
	if (!u.installed) {
		return h('section', { class: 'panel' }, h('h2', null, 'Updates'),
			h('div', { class: 'edit' }, h('div', { class: 'sub' },
				`This manager runs ${u.running || 'a build of its own'}. It has no updater: run aeolus-install again on its host, as root, to set one up (0183).`)));
	}
	const busy = u.pending || u.state === 'checking' || u.state === 'updating';
	const last = u.state === 'updated' ? `Installed ${u.tag} ${ago(u.at)}.`
		: u.state === 'failed' ? `${u.tag ? `${u.tag}: ` : ''}${u.error}`
			: u.state === 'updating' ? `Installing ${u.tag}…` : u.error || '';
	const row = (k, ...v) => h('div', { class: 'row' }, h('span', { class: 'key' }, k), h('span', { class: 'value' }, ...v));
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, 'Updates', h('span', { class: 'note' }, 'This manager’s own release. The APs update themselves from it (0079).'),
			h('span', { class: 'controls' },
				h('button', { type: 'button', class: 'button small', disabled: busy, onclick: () => askUpdater('check', 'Looking for a newer release…') }, 'Check now'),
				u.newer && h('button', { type: 'button', class: 'button small primary', disabled: busy, onclick: () => updateNow(u) }, `Update to ${u.target}`))),
		row('Running', u.running || 'unknown'),
		row(`Newest ${u.channel || 'stable'}`, u.target || (u.channel === 'testing' ? 'not known yet' : 'none promoted yet'),
			u.channel !== 'testing' && u.newest && u.newest !== u.target && h('span', { class: 'sub' }, ` · ${u.newest} on testing`),
			u.checked && h('span', { class: 'sub' }, ` · looked ${ago(u.checked)}`), busy && h('span', { class: 'chip warn' }, u.state === 'updating' ? 'installing' : 'asked'),
			u.newer && whatsNew(u)),
		row('By itself', u.automatic ? `installs a newer release between ${u.window.replace('-', '–')}, the Org’s local time` : 'no: it says when one is out', h('span', { class: 'sub' }, ' · set under More settings…, Updates')),
		last && row('Last', u.state === 'failed' ? h('span', { class: 'chip bad' }, 'failed') : null, ' ', last, u.at && h('span', { class: 'sub' }, ` (${when(u.at)})`)),
		u.log && h('details', { class: 'fold nested' }, h('summary', null, h('span', { class: 'sub' }, 'What the last update said')),
			h('pre', { class: 'log' }, u.log)));
}

// bytes writes a size the short way: 812 B, 12.3 MB, 1.2 GB.
const bytes = (n) => (n >= 1e9 ? `${(n / 1e9).toFixed(1)} GB` : n >= 1e6 ? `${(n / 1e6).toFixed(1)} MB` : n >= 1e3 ? `${(n / 1e3).toFixed(1)} kB` : `${n || 0} B`);

// upFor writes how long something has run: 3 d 4 h, 2 h 10 min, 5 min.
const upFor = (s) => (s >= 86400 ? `${Math.floor(s / 86400)} d ${Math.floor((s % 86400) / 3600)} h`
	: s >= 3600 ? `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} min` : `${Math.max(1, Math.round(s / 60))} min`);

// managerPanel is the manager's own health (0219), on the Org's System
// tab: its host's processors, memory and disk, its own process, its
// databases, and the answers it was slow to give in the last hour. Null
// where it cannot be read, as from an older manager.
export async function managerPanel() {
	let m;
	try {
		m = await get('/v1/system/health');
	} catch {
		return null;
	}
	const row = (k, ...v) => h('div', { class: 'row' }, h('span', { class: 'key' }, k), h('span', { class: 'value' }, ...v));
	const host = m.host || {};
	const proc = m.process || {};
	const busy = host.cpu != null && host.cpu >= 80;
	const tight = host.mem_total && host.mem_available / host.mem_total < 0.1;
	const full = host.disk_total && host.disk_free / host.disk_total < 0.1;
	const slow = m.slow || {};
	return h('section', { class: 'panel' },
		h('h2', null, 'This manager', h('span', { class: 'note' }, `${m.version || ''} · up ${upFor(m.uptime || 0)}`)),
		row('Processors', host.cpu != null ? h('span', { class: busy ? 'warn' : null }, `${host.cpu}% busy`) : `${host.cpus} processors`,
			host.load && h('span', { class: 'sub' }, ` · load ${host.load.map((x) => x.toFixed(2)).join(', ')} on ${host.cpus}`),
			proc.cpu != null && h('span', { class: 'sub' }, ` · Aeolus ${proc.cpu}%`)),
		host.mem_total && row('Memory', h('span', { class: tight ? 'warn' : null }, `${bytes(host.mem_available)} free of ${bytes(host.mem_total)}`),
			proc.rss && h('span', { class: 'sub' }, ` · Aeolus ${bytes(proc.rss)}`)),
		host.disk_total && row('Disk', h('span', { class: full ? 'warn' : null }, `${bytes(host.disk_free)} free of ${bytes(host.disk_total)}`)),
		(m.databases || []).length > 0 && row('Databases', (m.databases || []).map((d) => `${d.name} ${bytes(d.bytes)}${d.wal ? ` (+${bytes(d.wal)} log)` : ''}`).join(' · ')),
		row('Slow answers', slow.last_hour ? h('span', { class: 'warn' }, `${slow.last_hour} in the last hour`) : 'none in the last hour',
			h('span', { class: 'sub' }, ` · ${slow.after_ms / 1000 || 1} s or more`)),
		(slow.recent || []).length > 0 && h('details', { class: 'fold nested' }, h('summary', null, h('span', { class: 'sub' }, 'The latest slow answers')),
			h('table', { class: 'list' },
				h('tr', null, ['When', 'Asked', 'Took'].map((t) => h('th', null, t))),
				slow.recent.map((a) => h('tr', null,
					h('td', { class: 'nowrap', title: when(a.at) }, ago(a.at)),
					h('td', { class: 'mono' }, `${a.method} ${a.path}`, a.status >= 400 && h('span', { class: 'sub' }, ` · ${a.status}`)),
					h('td', null, `${(a.ms / 1000).toFixed(1)} s`))))));
}
