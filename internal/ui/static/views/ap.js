// One AP: whether it runs what it should, its settings in the same tabs as a
// folder's (0047), what it reported, and its history (0039, 0041).
//
// Its header says how it stands and what it is, in two lines (0284). Its
// Overview is kept quiet (Griff, 2026-10-11): a strip of how it is, its
// radio on the page's band with that radio's clients, what it did lately,
// and what it is; then its clients over time, what it can be asked to do,
// its traffic and its history, the last two a click down. Its client tests
// have a tab of their own, as a folder's.

import { h, link } from '../dom.js';
import { captureTab, captureAgain } from './captures.js';
import { redrawNow } from '../refresh.js';
import { get } from '../api.js';
import { bandName, when, ago, secondsAgo, uplinkJudgment, vlanUsers, switchPort, dhcpWarnings, templateSays, uplinkBrief } from '../format.js';
import { treeAside, crumbs, apStatus, fleetMap, tabBar, pick, keepPath, moved, bandRow, presentBands, pageBand } from '../layout.js';
import { editing } from './fields.js';
import { mayHere } from '../rights.js';
import { followButton } from './follow.js';
import { systemSection } from './sections.js';
import { networksTab } from './networks.js';
import { radiosTab, radiosBar, radiosPart, wiredTab } from './interfaces.js';
import { clientsTab } from './clients.js';
import { renameButton } from './rename.js';
import { moveButton } from './move.js';
import { actionsPanel, actionButtons, rebootButton, terminalButton } from './actions.js';
import { removeAPButton, replaceAPButton, againBanner } from './removeap.js';
import { askButton } from './advisor.js';
import { hereNow } from './onnow.js';
import { canWire, clientTestPanel, hasScanRadio } from './clienttest.js';
import { usagePanel, clientsPanel } from './usage.js';
import { leavingPanel } from './leaving.js';
import { healthTab } from './health.js';
import { bluetoothTab } from './bluetooth.js';
import { cell, traffic } from './overview.js';
import { testNetworkPanel } from './testnet.js';
import { maintenanceBanner, testWindowButton } from './testwindow.js';
import { radiosOf, upFor } from './sections.js';
import { called } from '../names.js';
import { roamingPanel } from './roaming.js';
import { activityTab, GROUPS } from './apactivity.js';

// Its tabs, as a folder's where it has the same (0284). A capture is taken
// from its header's Capture, and has no tab.
const TABS = [['overview', 'Overview'], ['radios', 'Radios'], ['wired', 'Wired'], ['networks', 'Networks'], ['clients', 'Clients'], ['activity', 'Activity'], ['health', 'Health'], ['settings', 'Settings']];
// Its Clients' parts (0337), as a folder's: its Bluetooth is a tab of its own.
const CLIENTS = [['wifi', 'Wi-Fi'], ['roaming', 'Wi-Fi roaming']];
// An AP that can test a network from a client's side (0121, 0128) has its
// tests under Assurance, as a folder has (0123).
const ASSURANCE = ['assurance', 'Assurance'];

// What is opened on an AP's Overview, by AP and by what it is, so the
// page's own redraw every half minute does not close it: a folded panel,
// and a check or the state reports in its history. With reading, the page
// is not drawn again while it is open: a config being read keeps its place.
// One left open inside a panel that is then folded is out of sight, and
// holds nothing.
const opened = new Set();
const marks = new WeakMap();
function kept(key, d, reading) {
	const mark = () => {
		if (reading && d.open && !d.closest('details:not([open])')) d.setAttribute('data-editing', 'reading');
		else d.removeAttribute('data-editing');
	};
	// What is inside it is marked again as it opens and closes.
	const within = () => d.querySelectorAll('details').forEach((x) => marks.get(x)?.());
	marks.set(d, mark);
	d.open = opened.has(key);
	mark();
	within();
	d.addEventListener('toggle', () => { if (d.open) opened.add(key); else opened.delete(key); mark(); within(); });
	return d;
}

export async function apPage(ctx, id, tab, sub, view) {
	const enc = encodeURIComponent(id);
	const [page, cfg, hist, fleet] = await Promise.all([
		get(`/v1/trees/locations/nodes/${enc}`),
		get(`/v1/aps/${enc}/config`),
		get(`/v1/aps/${enc}/history?limit=20`),
		get('/v1/aps'),
	]);
	const me = fleet.aps.find((a) => a.id === id);
	const st = me ? apStatus(me) : { chip: 'idle', label: 'Unknown', detail: '' };
	const cond = cfg.condition || {};
	const facts = page.facts || {};
	const edit = editing(ctx, 'locations', page);
	// An installer names, moves and wires an AP, and asks it to act,
	// without changing its settings (0177).
	const lifecycle = editing(ctx, 'locations', page, 'aps');
	const acts = mayHere(page, 'actions');
	// Everything the AP sets for itself can go back to its folder in one
	// change (0046).
	const own = Object.keys(page.fields || {}).filter((p) => page.fields[p].origin === 'self').sort();
	const revertBox = h('div', { class: 'edit flush' });
	// Renamed, its hostname with it, or moved to another folder, by someone
	// who may change it (0076).
	const nameBox = h('div', { class: 'edit' });
	const base = `/aps/${enc}`;
	[tab, sub, view] = moved(base, tab, sub, view);
	const tests = hasScanRadio(page) || canWire(cfg);
	let tabs = tests ? [...TABS.slice(0, 5), ASSURANCE, ...TABS.slice(5)] : TABS;
	// One that says what its Bluetooth radio hears has a tab for it (0157),
	// before Health.
	if (cond.state?.report?.ble_devices) {
		const i = tabs.findIndex(([k]) => k === 'health');
		tabs = [...tabs.slice(0, i), ['bluetooth', 'Bluetooth'], ...tabs.slice(i)];
	}
	// Captures are an admin's (0177), taken from the header (0284).
	const captures = mayHere(page, 'capture');
	tab = captures && tab === 'capture' ? tab : pick(tabs, tab);
	if (tab === 'clients') sub = pick(CLIENTS, sub);
	// Its Activity, by group (0338).
	if (tab === 'activity') sub = pick(GROUPS, sub);
	// The band picker, on the tabs that show radio numbers, as a folder's.
	const bands = tab === 'overview' || (tab === 'clients' && sub === 'wifi') || (tab === 'radios' && radiosPart(sub) !== 'bands')
		? bandRow(presentBands(page), tab === 'clients', redrawNow) : null;
	const thisAP = { ap: { id, name: page.node.name }, cfg };
	// The template it follows is said with what it is; one it does not
	// follow whole has a banner of its own.
	const follows = cfg.template?.id && cfg.template.follows && !cfg.template.replaced?.length && !cfg.unassigned;
	const main = [
		crumbs(ctx, 'locations', page.ancestry),
		h('div', { class: 'head' },
			h('div', null,
				h('h1', null, page.node.name, h('span', { class: `chip ${st.chip} headchip`, title: st.detail || null }, st.label),
					lifecycle && renameButton(ctx, 'locations', { id, name: page.node.name, kind: 'ap' }, nameBox, 'rename head'),
					lifecycle && moveButton(ctx, 'locations', { id, name: page.node.name, kind: 'ap' }, nameBox, 'rename head'),
					acts && actionButtons(id, page.node.name, nameBox),
					acts && rebootButton(id, page.node.name, nameBox),
					edit && testWindowButton(id, page.node.name, cfg, page, nameBox),
					mayHere(page, 'shell') && terminalButton(id, page.node.name),
					captures && h('a', { class: 'button small rename head', href: `#${base}/capture`, 'aria-current': tab === 'capture' ? 'page' : null,
						title: 'Capture what crosses this AP' }, 'Capture'),
					askButton(`About the AP ${page.node.name} (${id}): `, 'button small rename head'),
					lifecycle && replaceAPButton(ctx, { id, name: page.node.name }, nameBox),
					lifecycle && removeAPButton(ctx, { id, name: page.node.name }, nameBox)),
				h('div', { class: 'sub' }, whatItIs(cfg, cond, facts),
					follows && [' · ', h('a', { href: `#/library/${encodeURIComponent(cfg.template.id)}`,
						title: cfg.template.at ? `Picked ${cfg.template.at === id ? 'on this AP' : `at ${ctx.name('locations', cfg.template.at)}`}` : null }, `template ${cfg.template.name}`)]),
				standing(st, cfg, cond)),
			edit && own.length > 0 && followButton(ctx, 'locations', id, page.node.name, edit.parentName, own, revertBox,
				`Revert to ${edit.parentName} (${own.length} custom setting${own.length === 1 ? '' : 's'})`)),
		revertBox,
		nameBox,
		againBanner(ctx, { id, name: page.node.name }, page, nameBox, lifecycle),
		maintenanceBanner(ctx, id, page, cfg, edit, nameBox),
		trouble(cond),
		!follows && templateNote(ctx, id, cfg),
		cfg.check?.problems?.length > 0 && h('div', { class: 'banner problems' },
			h('strong', null, 'Its config breaks these rules, so it is not sent'),
			h('ul', null, cfg.check.problems.map((p) => h('li', null, p)))),
		tunnelTrouble(cfg, base),
		clockTrouble(cfg, base),
		tab === 'radios' ? [tabBar(base, tabs, tab), radiosBar(base, sub, bands)]
			: tab === 'clients' ? [tabBar(base, tabs, tab), tabBar(`${base}/clients`, CLIENTS, sub, true, bands)]
				: tab === 'activity' ? [tabBar(base, tabs, tab), tabBar(`${base}/activity`, GROUPS, sub, true)]
				: tabBar(base, tabs, tab, false, bands),
	];
	if (tab === 'overview') {
		// Its clients on each band over time, and those that could not get
		// on (0166), before its traffic.
		const now = {};
		for (const cl of hereNow(cond)) now[cl.band] = (now[cl.band] || 0) + 1;
		const [seen, noise] = await Promise.all([
			get(`/v1/clients?under=${enc}&hours=24&limit=500`).catch(() => null),
			get(`/v1/noise?under=${enc}`).catch(() => null)]);
		main.push(testNetworkPanel(ctx, id, page), glance(cond, base, page, cfg),
			h('div', { class: 'overgrid' },
				radioPanel(id, cfg, cond, base, seen, noise),
				h('div', { class: 'side' }, lately(hist, cond), aboutBrief(ctx, id, cfg, cond, facts))),
			await clientsPanel(id, now, base, true),
			await actionsPanel(id),
			traffic(id, await usagePanel(id, false)),
			history(id, hist));
	} else if (tab === 'assurance') {
		// An AP with a scan radio can test a network as a client would
		// (0121); any whose agent says so can over the wire (0128).
		main.push(await clientTestPanel(ctx, id, page, cfg, acts));
	} else if (tab === 'radios') main.push(...await radiosTab(ctx, base, id, page, sub, thisAP, editing(ctx, 'locations', page, 'ports')));
	else if (tab === 'wired') main.push(...await wiredTab(ctx, base, id, page, sub, thisAP, editing(ctx, 'locations', page, 'ports')));
	else if (tab === 'networks') main.push(await networksTab(ctx, id, page));
	// Its clients; how they roam (0136) and why they left (0255), a part
	// of their own (0337).
	else if (tab === 'clients') main.push(...(sub === 'roaming'
		? await Promise.all([get(`/v1/roaming?under=${enc}`).then(roamingPanel, () => roamingPanel(null)), leavingPanel({ under: id })])
		: [await clientsTab(ctx, page, thisAP)]));
	// What crosses this AP, taken on it (0148).
	else if (tab === 'capture') main.push(await captureTab(id, cfg, redrawNow));
	else if (tab === 'health') main.push(await healthTab(ctx, id));
	// Everything it did and that was done to it (0338).
	else if (tab === 'activity') main.push(...await activityTab(ctx, base, id, sub));
	else if (tab === 'bluetooth') main.push(await bluetoothTab(ctx, id, cond));
	else main.push(await systemSection(ctx, id, page, edit, { report: cond.state?.report, address: cond.seen?.source }));
	const keep = tab === 'overview' || tab === 'capture' ? '' : keepPath(tab, sub);
	// A capture that runs is looked at again every few seconds.
	return { aside: treeAside(ctx, 'locations', id, fleetMap(fleet.aps), keep), main, refresh: tab === 'capture' ? await captureAgain(id) : 30 };
}

// tunnelTrouble warns of a tunnel the AP's prober finds down, and of a port
// its loop guard took off its tunnels (0059); of a network carried by its
// fallback, or that cannot switch to it (0061); and of a VLAN the AP carries
// that doesn't reach it from the switch, with what needs it and the switch
// port to look at (0064); and of a network whose DHCP isn't answering, has
// more than one server, or leaves a client with no address (0065). The
// Tunnels, Networks and Ethernet views say more.
function tunnelTrouble(cfg, base) {
	const st = cfg.condition?.state;
	const r = st?.report;
	const x = r?.vxlan;
	const down = (x?.tunnels || []).filter((t) => t.probe?.verdict === 'down');
	const loops = x?.loops || [];
	const nets = Object.entries(r?.transports || {}).sort().filter(([, t]) => t.active === 'fallback' || t.cannot_switch);
	const ssid = (id) => cfg.document?.network?.[id]?.ssid || id;
	const port = switchPort(r?.uplink_neighbor);
	const vlans = (r?.uplink_vlans || []).map((v) => [v, uplinkJudgment(v, r.uplink_neighbor)[2]]).filter(([, says]) => says);
	const dhcp = Object.entries(r?.dhcp || {}).sort().flatMap(([id, d]) => dhcpWarnings(ssid(id), d));
	// A RADIUS server that does not answer this AP (0114).
	const radius = (r?.radius || []).filter((x) => x.verdict === 'silent');
	if (!down.length && !loops.length && !nets.length && !vlans.length && !dhcp.length && !radius.length) return null;
	return h('div', { class: 'banner problems' },
		h('strong', null, radius.length ? 'Its uplink, transports, sign-in or DHCP need a look' : 'Its uplink, transports or DHCP need a look'),
		h('ul', null,
			down.map((t) => h('li', null, `The tunnel to ${t.peer}, VNI ${t.vni}, is down: ${t.probe.underlay === false ? `${t.peer} cannot be reached` : `nothing on VNI ${t.vni} answers`}.`)),
			loops.map((l) => h('li', null, `${l.port} is off its tunnels: VNI ${l.vni ?? '?'} loops.`)),
			nets.map(([id, t]) => h('li', null, t.active === 'fallback'
				? `${ssid(id)} is on its fallback${t.last_switch?.to === 'fallback' ? `, switched ${secondsAgo(t.last_switch.ago, st.at)}: ${t.last_switch.why}` : ''}.`
				: `${ssid(id)} cannot switch: ${t.cannot_switch}.`)),
			vlans.map(([v, says]) => {
				const users = vlanUsers(cfg.document, v.vlan);
				return h('li', null, `${says}.${users.length ? ` ${users.join(', ')} ${users.length === 1 ? 'needs' : 'need'} it.` : ''}${port ? ` The AP is on ${port}.` : ''}`);
			}),
			radius.map((x) => h('li', null, `The RADIUS server ${x.server} does not answer this AP, so no one can sign in to ${ssid(x.network)} here: it is down, out of reach, or does not know this AP or its secret.`)),
			dhcp.map((line) => h('li', null, line + '.'))),
		h('div', null, link(`${base}/wired/tunnels`, 'Wired › Tunnels'), ' · ', link(`${base}/wired/ethernet`, 'Wired › Ethernet'), ' · ', link(`${base}/networks`, 'Networks')));
}

// clockTrouble warns of an AP whose clock isn't synchronized, or that has no
// time server to ask (0069): its TLS to the manager, and every time it
// reports, depend on the clock. The site's own time servers are set under
// System, or given by its DHCP.
function clockTrouble(cfg, base) {
	const t = cfg.condition?.state?.report?.time;
	if (!t) return null;
	const says = t.synced === false
		? `Its clock isn't synchronized: ntpd ${t.servers?.length ? `asks ${t.servers.join(', ')}` : 'has no time server'}.`
		: t.synced == null && !t.servers?.length ? 'It has no time server, so its clock is never set.' : null;
	if (!says) return null;
	return h('div', { class: 'banner problems' },
		h('strong', null, 'Its clock needs a look'),
		h('div', null, says, ' Set its time servers under ', link(`${base}/settings`, 'Settings'), ", or have the site's DHCP give one."));
}

// keysLine says whether the AP has the per-user keys it should (0070), from
// the version it last reported.
function keysLine(cfg) {
	const want = cfg.keys;
	const have = cfg.condition?.state?.report?.keys;
	if (!want || (!want.count && !have)) return null;
	const current = have?.version === want.version;
	return [' · ', h('span', { class: current ? null : 'warn', title: current ? 'its keys are the latest' : 'its key agent has not yet reported the latest keys' },
		`${want.count} per-user ${want.count === 1 ? 'key' : 'keys'}${current ? '' : ', not yet on the AP'}`)];
}

// whatItIs is the AP in a line under its name (0284): its model, the
// address it calls from, its OpenWrt, its agent, the version it was given,
// and how long it has been up.
function whatItIs(cfg, cond, facts) {
	const r = cond.state?.report;
	const agent = cfg.agent?.runs?.version && (cfg.agent.runs.version || '').replace(/-[0-9a-f]{7,}$/, '');
	const up = upFor(r?.uptime);
	return [facts.model || 'AP', cond.seen?.source && h('span', { class: 'mono' }, cond.seen.source), r?.openwrt && `OpenWrt ${r.openwrt}`,
		agent && `agent ${agent}`, `version ${cfg.version}`, up && `up ${up}`].filter(Boolean).flatMap((x, i) => (i ? [' · ', x] : [x]));
}

// standing is how the AP stands, where that is not well: its state's why,
// when it was last seen, where why does not say, and its per-user keys,
// said only where it has not the latest. The chip beside its name says the
// state itself.
function standing(st, cfg, cond) {
	const keys = cfg.keys && cfg.condition?.state?.report?.keys?.version !== cfg.keys.version ? keysLine(cfg) : null;
	if (st.cls === 'ok' && !keys) return null;
	const said = typeof st.detail === 'string' && /seen/i.test(st.detail);
	return h('div', { class: `sub standing ${st.cls}` }, st.cls !== 'ok' && st.detail,
		st.cls !== 'ok' && !said && ` ${cond.seen ? `Seen ${ago(cond.seen.at)}.` : 'Never seen.'}`, keys);
}

// trouble says what must not be folded away: an apply that failed, or
// that no check covered, and a render check the AP refused. Each is a
// critical alert (0041), and an AP's page has no Alerts tab.
function trouble(cond) {
	const c = cond.check, a = cond.apply;
	const lines = [
		a && !a.ok && `Its last apply failed: version ${a.version}, ${ago(a.at)}${a.error ? `. ${a.error}` : ''}`,
		a && a.ok && !a.checked && `Its last apply, of version ${a.version}, ${ago(a.at)}, was not covered by an ok check`,
		c && c.result !== 'ok' && c.result !== 'stale' && `Its last render check came to ${c.result}: version ${c.version}, ${ago(c.at)}`,
	].filter(Boolean);
	return lines.length ? h('div', { class: 'banner problems' }, lines.map((l) => h('div', null, l))) : null;
}

// glance is the AP at a glance, in one strip (0284), from its latest
// report: its clients, its radios on the air, its uplink, how hard it
// works, and its scan radio, each leading to the tab that says more.
// Nothing where it has not reported.
function glance(cond, base, page, cfg) {
	const r = cond.state?.report;
	if (!r) return null;
	// Its radios that serve: not one kept for scanning (0081), which the
	// AP reports as a radio that is not up. One its config turns off is
	// off, and no fault.
	const scans = new Set((page.facts?.radios || []).filter((x) => x.reserved).map((x) => x.radio));
	const radios = (r.radios || []).filter((x) => !scans.has(x.radio));
	const off = (x) => cfg.document?.radio?.[x.band]?.enabled === false;
	const on = radios.filter((x) => x.up !== false);
	// One with fewer of its networks on the air than it has (0216).
	const short = (x) => x.bss_want != null && x.bss < x.bss_want;
	const down = radios.filter((x) => (x.up === false && !off(x)) || short(x));
	// Not those since on another AP (0185).
	const mine = hereNow(cond);
	const clients = r.clients ? mine.length : radios.reduce((n, x) => n + (x.clients || 0), 0);
	const byBand = ['2g', '5g', '6g'].map((b) => [b, mine.filter((c) => c.band === b).length]).filter(([, n]) => n);
	const up = (r.ports || []).find((p) => p.path) || (r.ports || []).find((p) => p.uplink);
	const sw = r.uplink_neighbor;
	// Its speed, amber where below what both ends can do; and its errors,
	// said wherever it has counted any (uplinkBrief). The cell says it
	// short; its hover, in full.
	const wire = uplinkBrief(r);
	const worst = wire?.errors?.grade === 'bad' || wire?.grade === 'bad' ? 'bad' : wire?.errors?.grade === 'warn' || wire?.grade === 'warn' ? 'warn' : '';
	const x = r.health || {};
	// Memory in use, counted as Health counts what is free.
	const used = x.mem_total ? 100 - Math.floor(100 * (x.mem_available || 0) / x.mem_total) : null;
	const hard = x.cpu >= 90 || used >= 90;
	const sr = r.scan_radio;
	const uplink = up && wire && cell('Uplink', wire.text, [wire.errors ? `${wire.errors.text} since it came up` : up.name, sw && [sw.system, sw.port].filter(Boolean).join(' ')].filter(Boolean).join(' · '),
		worst, `#${base}/wired/ethernet`);
	if (uplink) uplink.title = [wire.says, wire.errors?.says].filter(Boolean).join(' ');
	// A report from a while ago is said to be: an AP not heard from since
	// is not as its strip says now.
	const old = cond.state?.at && Date.now() - new Date(cond.state.at).getTime() > 10 * 60 * 1000;
	return [old && h('p', { class: 'sub tilesnote' }, `As it last reported, ${ago(cond.state.at)}.`),
		h('section', { class: `nowstrip${old ? ' stale' : ''}`, 'aria-label': 'Now' },
			cell('Clients now', String(clients), clients === 0 ? 'none now' : byBand.map(([b, n]) => `${bandName(b)} ${n}`).join(' · '), '', `#${base}/clients`),
			radios.length > 0 && cell('Radios on the air', `${on.length} of ${radios.length}`,
				down.length ? down.map((y) => `${bandName(y.band)} ${y.up === false ? 'down' : `${y.bss} of ${y.bss_want} networks`}`).join(' · ')
					: radios.filter(off).map((y) => `${bandName(y.band)} off`).join(' · ') || 'all up', down.length ? 'warn' : '', `#${base}/radios`),
			uplink,
			x.cpu != null ? cell('Processor', `${x.cpu}%`, used != null ? `memory ${used}%` : '', hard ? 'warn' : '', `#${base}/health`)
				: used != null && cell('Memory', `${used}%`, 'in use', hard ? 'warn' : '', `#${base}/health`),
			sr && cell('Scanning radio', SCAN_SAYS[sr.state] || sr.state || 'Scanning', `${sr.networks ?? 0} networks heard in its last look`, sr.networks ? '' : 'warn', `#${base}/radios/airspace`))];
}

// What airscan says its scan radio is doing (0126), in a word.
const SCAN_SAYS = { clients: 'Listening', bss: 'Looking', spectral: 'Sweeping', lent: 'Lent to a test', idle: 'Idle', 'waiting for scan radio': 'Waiting' };

// radioPanel is the AP's radio on the page's band (0284): its channel,
// width, power and noise floor, then its clients there, the weakest first,
// each with its network, signal, rate and how often it joined over the day
// (seen, GET /v1/clients). None where it has no radio for the band.
function radioPanel(id, cfg, cond, base, seen, noise) {
	const band = pageBand() || '5g';
	const { serving } = radiosOf(cfg);
	const rs = serving.filter((r) => r.band === band);
	if (!cond.state?.report) return h('section', { class: 'panel main' }, h('p', { class: 'sub pad' }, 'It has not reported yet.'));
	if (!rs.length) return h('section', { class: 'panel main' }, h('h2', null, bandName(band)), h('p', { class: 'sub pad' }, `It has no ${bandName(band)} radio.`));
	const floors = noise?.aps?.[id] || {};
	const set = (k) => cfg.location?.[`radio.${band}.${k}`]?.value;
	const fact = (label, v, note) => h('span', { class: 'fact' }, h('span', { class: 'sub' }, label), ' ', h('strong', null, v), note && h('span', { class: 'sub' }, ` ${note}`));
	const facts = rs.map((r) => {
		const f = floors[r.radio];
		return h('div', { class: 'radiofacts' },
			fact('channel', r.channel ? String(r.channel) : 'starting', set('channel') === 'auto' ? 'auto' : null),
			r.width && fact('width', `${r.width} MHz`),
			r.txpower != null && fact('power', `${r.txpower} dBm`, set('power') === 'auto' ? 'auto' : null),
			f && !f.unmeasured && fact('noise', `${f.dbm} dBm`, f.loud ? 'loud' : null));
	});
	const joins = new Map((seen?.clients || []).map((c) => [c.mac, c.recent]));
	const list = hereNow(cond).filter((c) => c.band === band).sort((x, y) => (x.signal ?? 0) - (y.signal ?? 0));
	return h('section', { class: 'panel main' },
		h('h2', null, `${bandName(band)} radio`, h('span', { class: 'note' }, link(`${base}/radios/neighbours`, 'neighbours, load, ratings'))),
		facts,
		list.length ? h('table', { class: 'list' },
			h('tr', null, h('th', null, 'Client'), h('th', null, 'Network'), h('th', { class: 'num' }, 'Signal'), h('th', { class: 'num' }, 'Rate'), h('th', { class: 'num' }, 'Joins, 24 h')),
			list.map((c) => {
				const j = joins.get(c.mac);
				return h('tr', null,
					h('td', null, link(`/clients/${c.mac}`, called(c.mac, c.host) || c.mac), called(c.mac, c.host) && h('span', { class: 'sub mono' }, ` ${c.mac}`)),
					h('td', null, c.ssid || c.network || '—'),
					h('td', { class: 'num' }, c.signal != null ? h('span', { class: c.signal <= -80 ? 'warn' : null }, `${c.signal} dBm`) : '—'),
					h('td', { class: 'num' }, c.tx_rate != null ? `${Math.round(c.tx_rate)} Mb/s` : '—'),
					h('td', { class: 'num' }, j ? [String(j.attempts), j.failed ? h('span', { class: 'bad' }, ` ${j.failed} failed`) : null] : '—'));
			}))
			: h('p', { class: 'sub pad' }, `No client on its ${bandName(band)} radio now.`));
}

// lately is what the AP did lately, the latest first: the configs it
// applied, its render checks that did not pass, and when it last started;
// all of it under History, below.
function lately(hist, cond) {
	const events = [
		...(hist.applies || []).map((a) => ({ at: a.at, bad: !a.ok || !a.checked, says: a.ok ? `Version ${a.version} applied${a.checked ? ', its check passed' : ', not covered by an ok check'}` : `Version ${a.version} failed to apply${a.error ? `: ${a.error}` : ''}` })),
		...(hist.checks || []).filter((c) => c.result !== 'ok' && c.result !== 'stale').map((c) => ({ at: c.at, bad: true, says: `Its render check of version ${c.version} came to ${c.result}` })),
	];
	const r = cond.state?.report;
	if (r?.uptime != null && cond.state?.at) events.push({ at: new Date(new Date(cond.state.at).getTime() - r.uptime * 1000).toISOString(), says: `Started${r.openwrt ? `, OpenWrt ${r.openwrt}` : ''}` });
	events.sort((x, y) => new Date(y.at) - new Date(x.at));
	const shown = events.slice(0, 6);
	return h('section', { class: 'panel' },
		h('h2', null, 'Lately'),
		shown.length ? h('ol', { class: 'lately' }, shown.map((e) => h('li', { class: e.bad ? 'bad' : null }, h('span', { class: 'mono sub' }, stamp(e.at)), h('span', null, e.says))))
			: h('p', { class: 'sub pad' }, 'Nothing yet.'));
}

// stamp writes a time by the clock today, else with its day.
function stamp(t) {
	const d = new Date(t);
	return d.toDateString() === new Date().toDateString() ? d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
		: d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

// aboutBrief is what the AP is: its MAC, board, template, radios and agent;
// all it said when it enrolled a click down.
function aboutBrief(ctx, id, cfg, cond, facts) {
	const t = cfg.template;
	const radios = facts.radios || [];
	const bands = ['2g', '5g', '6g'].filter((b) => radios.some((r) => !r.reserved && r.band === b)).map((b) => bandName(b).replace(' GHz', ''));
	const scan = radios.filter((r) => r.reserved).map((r) => bandName(r.band));
	return h('section', { class: 'panel' },
		h('h2', null, 'About'),
		h('dl', { class: 'kv inpanel' },
			facts.mac && [h('dt', null, 'MAC'), h('dd', { class: 'mono' }, facts.mac)],
			facts.board && [h('dt', null, 'Board'), h('dd', { class: 'mono' }, facts.board)],
			t?.id && [h('dt', null, 'Template'), h('dd', null, link(`/library/${encodeURIComponent(t.id)}`, t.name), t.at && t.at !== id ? ` from ${ctx.name('locations', t.at)}` : '')],
			bands.length > 0 && [h('dt', null, 'Radios'), h('dd', null, `${bands.join(', ')} GHz${scan.length ? `; a ${scan.join(', ')} scanning radio` : ''}`)],
			h('dt', null, 'Agent'), h('dd', null, agentSays(cfg.agent))),
		enrollment(facts, t?.board));
}

// agentSays is the agent release an AP runs, and where that is not the one
// it should run, which that is.
function agentSays(a) {
	const short = (v) => (v || '').replace(/-[0-9a-f]{7,}$/, '');
	const runs = short(a?.runs?.version), wants = short(a?.wants?.version);
	if (!runs) return 'not said yet';
	if (a.missing_pin) return `${runs} · its folder pins ${a.missing_pin}, which the manager no longer keeps`;
	return a.runs.hash && a.runs.hash === a.wants?.hash || !wants ? runs : `${runs} · it should run ${wants}`;
}

// enrollment is what the AP said about itself when it enrolled (0033). A
// board it named by a name OpenWrt has since changed (0174) shows the name
// its templates and settings go by too.
function enrollment(facts, board) {
	if (!facts.mac) return null;
	return h('details', { class: 'fold inpanel' },
		h('summary', null, 'All it said when it enrolled'),
		h('dl', { class: 'kv' },
			h('dt', null, 'MAC'), h('dd', null, facts.mac),
			h('dt', null, 'All MACs'), h('dd', null, (facts.macs || []).join(', ')),
			h('dt', null, 'Hostname'), h('dd', null, facts.hostname || '—'),
			h('dt', null, 'Board'), h('dd', null, facts.board || '—',
				board && facts.board && board !== facts.board && h('span', { class: 'note' }, ` now ${board}`)),
			h('dt', null, 'Radios'), h('dd', null, (facts.radios || []).map((r) => `${r.radio} ${bandName(r.band)}${r.reserved ? ', a scan radio Aeolus leaves to its own service' : ''}: ${(r.htmodes || []).join(' ')}`).map((x) => h('div', null, x))),
			h('dt', null, 'Enrolled from'), h('dd', null, facts.source || '—')));
}

// history lists recent checks (with the UCI the AP sent, secrets blanked:
// 0041), applies and state reports, newest first.
function history(id, hist) {
	const checks = hist.checks || [];
	const applies = hist.applies || [];
	const states = hist.states || [];
	const many = (n, one, more) => `${n} ${n === 1 ? one : more}`;
	const said = [checks.length && many(checks.length, 'check', 'checks'), applies.length && many(applies.length, 'apply', 'applies'),
		states.length && many(states.length, 'state report', 'state reports')].filter(Boolean).join(' · ');
	return kept(`${id}/history`, h('details', { class: 'panel logpanel' },
		h('summary', null, h('h2', { title: 'Its latest checks, with the config it sent; its applies; and its state reports, newest first' },
			'History', h('span', { class: 'note' }, said ? `latest ${said}` : 'nothing yet'))),
		checks.map((c) => kept(`${id}/check/${c.at}/${c.hash}`, h('details', null,
			h('summary', null,
				h('span', { class: 'when' }, when(c.at)),
				h('span', { class: 'chip ' + (c.result === 'ok' ? 'ok' : c.result === 'stale' ? 'idle' : 'bad') }, 'check ' + c.result),
				h('span', { class: 'grow' }, `version ${c.version}`, c.problems?.length ? ` · ${c.problems.length} problem${c.problems.length === 1 ? '' : 's'}` : ''),
				h('span', { class: 'mono' }, c.hash.slice(0, 12))),
			c.problems?.length > 0 && h('ul', { class: 'problems' }, c.problems.map((p) => h('li', null, p))),
			c.uci ? h('pre', { class: 'uci' }, c.uci) : h('div', { class: 'empty' }, 'No UCI kept for this check.')), true)),
		applies.map((a) => h('details', null,
			h('summary', null,
				h('span', { class: 'when' }, when(a.at)),
				h('span', { class: 'chip ' + (a.ok ? 'ok' : 'bad') }, a.ok ? 'applied' : 'apply failed'),
				h('span', { class: 'grow' }, `version ${a.version}`, a.checked ? '' : ' · not covered by an ok check', a.error ? ' · ' + a.error : ''),
				h('span', { class: 'mono' }, a.hash.slice(0, 12))))),
		states.length > 0 && kept(`${id}/states`, h('details', null,
			h('summary', null, h('span', { class: 'when' }, when(states[0].at)), h('span', { class: 'chip idle' }, 'state'),
				h('span', { class: 'grow' }, `${states.length} recent state reports`)),
			h('pre', { class: 'uci' }, states.map((s) => `${when(s.at)}  ${JSON.stringify(s.report)}`).join('\n'))), true),
		!checks.length && !applies.length && !states.length && h('div', { class: 'empty' }, 'Nothing yet.')));
}

// templateNote says which AP template the AP takes (0085), and whether it
// follows it: each of the template's settings that something set closer to
// the AP replaces, and where.
function templateNote(ctx, id, cfg) {
	const t = cfg.template;
	if (!t || cfg.unassigned) return null;
	if (!t.id) return h('div', { class: 'banner info' }, templateSays(t));
	const at = (node) => (node === id ? 'on this AP' : ['at ', link(`/locations/${encodeURIComponent(node)}`, ctx.name('locations', node))]);
	return h('div', { class: 'banner ' + (t.follows ? 'info' : 'branch'), style: { 'flex-direction': 'column', gap: '6px' } },
		h('div', null, h('strong', null, templateSays(t)), ' · picked ', at(t.at), ' · ', link(`/library/${encodeURIComponent(t.id)}`, 'the template')),
		t.replaced.length > 0 && h('ul', { style: { margin: '0', 'padding-left': '18px' } }, t.replaced.map((r) => h('li', null,
			h('span', { class: 'mono' }, r.path), ' is ', h('span', { class: 'mono' }, JSON.stringify(r.value)), ', set ', at(r.node)))));
}

