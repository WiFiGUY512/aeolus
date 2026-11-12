// The Aeolus UI (0042): a hash-routed page over the API. Part 1 reads only.

import { h } from './dom.js';
import { get, signOut, APIError } from './api.js';
import { setRedraw, isEditing, interval, currentFlash, redrawNow, flash } from './refresh.js';
import { watchTables } from './tables.js';
import { watchDown, noBand } from './layout.js';
import { loadNames, namedLike } from './names.js';
import { know, mayAnywhere, runsPeople } from './rights.js';
import { signinPage, passwordPage } from './views/signin.js';
import { registerPage } from './views/register.js';
import { treePage } from './views/tree.js';
import { apPage } from './views/ap.js';
import { libraryPage } from './views/library.js';
import { templatePage } from './views/templates.js';
import { changesPage } from './views/changes.js';
import { advisorPage, seesLocations } from './views/advisor.js';
import { capturesPage } from './views/captures.js';
import { bluetoothDevicePage } from './views/bluetooth.js';
import { clientPage } from './views/journey.js';
import { adminPage, accountPage } from './views/admin.js';
import { updateOf, updateBanner } from './views/updates.js';
import { themePick, watchTheme } from './themes.js';
import { suggestButton } from './views/suggest.js';

const routes = [
	[/^\/(locations|services)(?:\/([^/]+)(?:\/([^/]+))?(?:\/([^/]+))?(?:\/([^/]+))?)?$/, (ctx, m) => treePage(ctx, m[1], m[2] && decodeURIComponent(m[2]), m[3], m[4], m[5])],
	// Every AP is on the Org's APs tab (0284).
	[/^\/aps$/, (ctx) => {
		try { history.replaceState(null, '', `#/locations/${encodeURIComponent(ctx.trees.locations.root)}/aps`); } catch { /* the address stays */ }
		return treePage(ctx, 'locations', ctx.trees.locations.root, 'aps');
	}],
	[/^\/aps\/([^/]+)(?:\/([^/]+))?(?:\/([^/]+))?(?:\/([^/]+))?$/, (ctx, m) => apPage(ctx, decodeURIComponent(m[1]), m[2], m[3], m[4])],
	[/^\/library$/, (ctx) => libraryPage(ctx)],
	[/^\/library\/([^/]+)$/, (ctx, m) => templatePage(ctx, decodeURIComponent(m[1]))],
	[/^\/changes$/, (ctx) => changesPage(ctx)],
	// What the advisor found (0205).
	[/^\/advisor$/, (ctx) => advisorPage(ctx)],
	// The captures people started (0148).
	[/^\/captures$/, (ctx) => capturesPage(ctx, redrawNow)],
	// A Bluetooth device's own page: where and when the APs heard it (0158).
	[/^\/bluetooth\/([0-9a-f]{2}(?::[0-9a-f]{2}){5})$/, (ctx, m) => bluetoothDevicePage(ctx, m[1])],
	// A Wi-Fi client's own page (0222): everything it did, and where; at an
	// attempt of its, where one is named.
	// A client's page (0222), its tabs (0337); an attempt on Activity, and
	// one from before the tabs, by its ID alone.
	[/^\/clients\/([0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2}){5})(?:\/(overview|activity|stays))?(?:\/(\d+))?$/, (ctx, m) => clientPage(ctx, m[1], m[2], m[3])],
	// The admin section, and one's own account (0176).
	[/^\/admin(?:\/(people|signins|suggestions))?$/, (ctx, m) => adminPage(ctx, m[1])],
	[/^\/admin\/accounts\/([^/]+)$/, (ctx, m) => accountPage(ctx, decodeURIComponent(m[1]))],
	[/^\/me$/, (ctx) => accountPage(ctx, ctx.who.account, true)],
	// One's own password, set or changed (0176).
	[/^\/password$/, (ctx) => ({ main: [passwordPage(() => { flash('Your password is changed.'); location.hash = '#/locations'; }, { hasPassword: ctx.who.has_password !== false })] })],
];

// The library holds AP templates (0085). Every AP is on the Org's APs tab,
// and captures are taken from an AP's or a client's page (0284); their own
// pages stay, without a tab.
const TABS = [['/locations', 'Locations'], ['/services', 'Services'], ['/library', 'Library'], ['/changes', 'Changes'], ['/advisor', 'Advisor'], ['/admin', 'Admin']];

let timer = null;
let rendering = 0;

function route() {
	return decodeURI(location.hash.replace(/^#/, '')) || '/locations';
}

// context is what every page needs: who is signed in, and both trees.
async function context() {
	// And the names people gave clients (0153), which every view calls them by.
	const [who, loc, svc] = await Promise.all([get('/v1/whoami'), get('/v1/trees/locations'), get('/v1/trees/services'), loadNames()]);
	const trees = { locations: index(loc.nodes), services: index(svc.nodes) };
	return {
		who,
		trees,
		org: trees.locations.list[0]?.name || trees.services.list[0]?.name || '',
		name: (tree, id) => trees[tree].nodes.get(id)?.name || id,
	};
}

function index(list) {
	const nodes = new Map(list.map((n) => [n.id, n]));
	return { list, nodes, root: list[0]?.id };
}

async function render(quiet) {
	clearTimeout(timer);
	const app = document.getElementById('app');
	const r = route();
	// Signed in or not, the manager says: a session's cookie goes by
	// itself, so only its answer tells (0176).
	if (r === '/signin') {
		app.replaceChildren(signinPage(() => { location.hash = '#/locations'; render(); }));
		return;
	}
	// An invite's link (0336), for one with no account yet.
	const invited = r.match(/^\/register\/([^/]+)$/);
	if (invited) {
		app.replaceChildren(registerPage(invited[1], () => { location.hash = '#/locations'; render(); }));
		return;
	}
	const mine = ++rendering;
	const old = app.querySelector('main');
	const scroll = quiet && old ? old.scrollTop : 0;
	let page;
	let ctx;
	try {
		ctx = await context();
		know(ctx);
		// The manager's own releases (0183), for one who may run them.
		ctx.update = await updateOf(ctx);
		noBand();
		const match = routes.map(([re, fn]) => [r.match(re), fn]).find(([m]) => m);
		page = match ? await match[1](ctx, match[0]) : { main: [h('div', { class: 'error' }, 'There is no page at ', r)] };
	} catch (e) {
		if (e instanceof APIError && e.status === 401) {
			signOut();
			location.hash = '#/signin';
			return;
		}
		// Signed in with a temporary password: it is changed first (0176).
		if (e instanceof APIError && e.mustChange) {
			app.replaceChildren(passwordPage(() => render(), { forced: true }));
			return;
		}
		page = { main: [h('div', { class: 'error' }, e.message || String(e))] };
	}
	if (mine !== rendering || !page) return; // a newer render took over
	const note = currentFlash();
	const main = h('main', null, note && h('div', { class: 'banner done' }, note), ctx && updateBanner(ctx, ctx.update), page.main);
	app.replaceChildren(
		header(ctx, r),
		h('div', { class: 'body' }, page.aside || null, main),
	);
	main.scrollTop = scroll;
	schedule(interval(page.refresh), page.refresh);
}

// schedule redraws the page after a while, but not while someone is
// editing on it.
function schedule(seconds, own) {
	if (!seconds) return;
	timer = setTimeout(() => {
		if (isEditing()) schedule(interval(own), own);
		else render(true);
	}, seconds * 1000);
}

function header(ctx, r) {
	return h('header', { class: 'top' },
		h('div', { class: 'brand' }, h('b', null, 'AEOLUS'), h('span', null, ctx?.org || ''),
			// The manager's own release, which a problem report names.
			ctx?.who.version && h('small', { class: 'release', title: 'This manager\'s release' }, ctx.who.version)),
		h('nav', { class: 'tabs', 'aria-label': 'Sections' },
			// Captures are an admin's (0177); Admin, for those who run people.
			// A folder's advisor's findings are for those who see the folder (0212).
			TABS.filter(([path]) => (path !== '/captures' || mayAnywhere('capture')) && (path !== '/admin' || runsPeople())
				&& (path !== '/advisor' || seesLocations(ctx))).map(([path, label]) => h('a', { href: '#' + path, class: r.startsWith(path) ? 'on' : null }, label))),
		h('div', { class: 'spacer' }),
		ctx && findBox(ctx),
		ctx && h('div', { class: 'who' },
			h('span', null, ctx.who.name || ctx.who.account),
			themePick(redrawNow),
			// Suggest a feature, from the page one is on (0323).
			suggestButton(ctx),
			h('a', { href: '#/me', title: 'Your password, API tokens and where you are signed in' }, 'My account'),
			h('button', { type: 'button', onclick: () => { signOut(); location.hash = '#/signin'; render(); } }, 'Sign out')),
	);
}

// findBox is the header's Find (0284): an AP, a folder or a client, by its
// name, ID, MAC or address. The tree's names are offered as it is typed.
function findBox(ctx) {
	const input = h('input', { type: 'search', name: 'find', placeholder: 'Find an AP, client, MAC or address', 'aria-label': 'Find an AP, folder or client', list: 'findnames', autocomplete: 'off' });
	const names = h('datalist', { id: 'findnames' }, ctx.trees.locations.list.filter((n) => n.kind !== 'org').map((n) => h('option', { value: n.name })));
	return h('form', { class: 'find', role: 'search', onsubmit: async (e) => {
		e.preventDefault();
		const q = input.value.trim();
		if (!q) return;
		const to = await found(ctx, q).catch(() => null);
		if (to) location.hash = to;
		else {
			flash(`Nothing found for “${q}”.`, 8);
			redrawNow();
		}
	} }, input, names);
}

// found is where Find takes q: an AP or folder named q, or with that ID; a
// client a person named q; one whose name holds q; a client by its MAC; else
// the first client the manager finds by part of its name, MAC or address.
async function found(ctx, q) {
	const low = q.toLowerCase();
	const nodes = ctx.trees.locations.list.filter((n) => n.kind !== 'org');
	const to = (n) => (n.kind === 'ap' ? `#/aps/${encodeURIComponent(n.id)}` : `#/locations/${encodeURIComponent(n.id)}`);
	const exact = nodes.find((n) => n.name.toLowerCase() === low || n.id === q);
	if (exact) return to(exact);
	const named = namedLike(q);
	const part = nodes.find((n) => n.name.toLowerCase().includes(low));
	if (part && !named) return to(part);
	if (named) return `#/clients/${named}`;
	if (/^([0-9a-f]{2}[:-]){5}[0-9a-f]{2}$/i.test(q)) return `#/clients/${low.replace(/-/g, ':')}`;
	const { clients } = await get(`/v1/clients?q=${encodeURIComponent(q)}&limit=1`);
	return clients?.length ? `#/clients/${clients[0].mac}` : null;
}

setRedraw(render);
watchTheme(redrawNow);
window.addEventListener('hashchange', () => render());
// Every table can be fitted by whoever looks at it: columns made wider or
// narrower, and hidden or shown (tables.js).
watchTables(document.getElementById('app'));
// The tree's dots go red as an AP goes, not at the page's next drawing (0165).
watchDown(document.getElementById('app'));
render();
