// Problems (0284): a folder's alerts, and its threats (0160), what the APs
// here find that should not be here. Rogues is the rogue APs, the suspects
// and the known ones the APs hear now, with what the scan radios' wire
// check found, and the clients heard on them; Rogue DHCP is the networks on
// which more than one DHCP server answers.

import { h, link } from '../dom.js';
import { get } from '../api.js';
import { bandName, ago, when } from '../format.js';
import { tabBar, pick } from '../layout.js';
import { knownButton } from './airspace.js';
import { alertsTab } from './alerts.js';

// The parts' keys are those of the old Threats tab's, so its links land.
const PARTS = [['alerts', 'Alerts'], ['wireless', 'Rogues'], ['network', 'Rogue DHCP']];

// The clients heard on rogues are listed over a week.
const HOURS = 168;

// What each kind is, with its chip.
const KIND = {
	rogue: ['bad', 'rogue', 'on your wire: frames sent on your network came out of its radio'],
	suspect: ['warn', 'suspect', 'one of your SSIDs, not seen on your wire: an evil twin, a neighbour\'s AP of that name, or one of yours the scan radios cannot reach'],
	known: ['idle', 'known', 'marked known: it raises no alert'],
};

// problemsTab draws the Problems tab of the Locations folder at base (0284):
// what needs attention, its Alerts (0099), and what should not be here, its
// Threats (0160), as the parts sub.
export async function problemsTab(ctx, base, id, page, sub) {
	sub = pick(PARTS, sub);
	const [res, alerts] = await Promise.all([
		get(`/v1/threats?under=${encodeURIComponent(id)}&hours=${HOURS}`),
		get(`/v1/alerts?under=${encodeURIComponent(id)}`),
	]);
	const c = res.counts || {};
	const a = alerts.counts || {};
	const count = (n) => (n ? ` (${n})` : '');
	const bar = tabBar(`${base}/problems`, [['alerts', `Alerts${count((a.critical || 0) + (a.warning || 0))}`],
		['wireless', `Rogues${count((c.rogue || 0) + (c.suspect || 0))}`], ['network', `Rogue DHCP${count(c.dhcp)}`]], sub, true);
	if (sub === 'alerts') return [bar, await alertsTab(ctx, id, alerts)];
	if (sub === 'network') return [bar, dhcpSection(ctx, res.dhcp || [])];
	const at = { node: id, nodeName: page.node.name, page };
	return [bar, roguesSection(ctx, at, res.rogues || [], c), clientsSection(ctx, res.clients || [])];
}

function roguesSection(ctx, at, rogues, c) {
	const out = h('div', { class: 'edit flush' });
	return h('section', { class: 'panel' },
		h('h2', null, 'Rogue APs', h('span', { class: 'note' }, 'only APs with a scan radio check the wire')),
		rogues.length > 0 && h('div', { class: 'sub' },
			['rogue', 'suspect', 'known'].filter((k) => c[k]).map((k) => [h('span', { class: `chip ${KIND[k][0]}` }, `${c[k]} ${KIND[k][1]}`), ' '])),
		out,
		!rogues.length
			? h('div', { class: 'sub' }, 'None heard: no AP here hears one of your SSIDs from an AP that is not yours, or a stranger\'s AP on your wire.')
			: h('table', { class: 'list' },
				h('tr', null, ['Network', 'Band', 'Your wire', 'Heard by', 'Last heard', ''].map((x) => h('th', null, x))),
				rogues.map((n) => {
					const [chip, label, means] = KIND[n.kind] || ['idle', n.kind, ''];
					const top = n.heard_by.slice(0, 3);
					return h('tr', null,
						h('td', null, h('div', null, n.ssid || h('em', null, 'hidden')), h('div', { class: 'sub mono' }, n.bssid),
							// Who made it, by its address (Griff, 2026-10-23).
							n.maker && h('div', { class: 'sub', title: n.maker_derived ? `By its address with the locally administered bit cleared: an AP often makes its other networks' addresses so` : 'By its address' }, n.maker_derived ? `${n.maker}, by its base address` : n.maker)),
						h('td', null, `${bandName(n.band)} · ${n.channel}`),
						h('td', null, wire(n.wire)),
						h('td', null, top.map((x) => h('div', null, link(`/aps/${encodeURIComponent(x.ap)}/radios/airspace`, x.name), h('span', { class: 'sub mono' }, ` ${Math.round(x.signal)} dBm`))),
							n.heard_by.length > 3 && h('div', { class: 'sub' }, `and ${n.heard_by.length - 3} more`)),
						h('td', null, ago(n.heard_by[0].seen)),
						h('td', null, h('span', { class: `chip ${chip}`, title: means }, label),
							n.clients > 0 && h('div', { class: 'sub' }, `${n.clients} client${n.clients === 1 ? '' : 's'}`),
							// Mark known, or Forget, as Networks heard has it (0106).
							knownButton(ctx, at, { ...n, kind: n.kind === 'known' ? 'known' : 'rogue' }, out)));
				})));
}

// wire says what the scan radios' wire check found of a rogue.
function wire(w) {
	const by = (w.by || []).map((x) => x.name).join(', ');
	if (w.state === 'on') {
		const v = w.vlans || [];
		return [h('strong', null, 'On your wire'), h('div', { class: 'sub' }, v.length ? `VLAN${v.length > 1 ? 's' : ''} ${v.join(', ')}` : '', by && ` · ${by}`)];
	}
	if (w.state === 'off') return [h('span', null, 'Not on your wire'), h('div', { class: 'sub' }, `${(w.looks || 0).toLocaleString()} looks`, by && ` · ${by}`)];
	return h('span', { class: 'sub' }, 'Not checked: no scan radio hears it');
}

function clientsSection(ctx, clients) {
	return h('section', { class: 'panel' },
		h('h2', null, 'Rogue clients', h('span', { class: 'note' }, 'heard on a rogue or a suspect in the last week, the last first')),
		!clients.length
			? h('div', { class: 'sub' }, 'None heard. Only APs with a scan radio hear the clients of other APs.')
			: h('table', { class: 'list' },
				h('tr', null, ['Client', 'On', 'First heard', 'Last heard', 'Frames', 'Heard by'].map((x) => h('th', null, x))),
				clients.map((x) => {
					const [chip, label, means] = KIND[x.kind] || ['idle', 'gone', 'no AP hears it now'];
					return h('tr', null,
						h('td', null, h('div', { class: 'mono' }, x.mac),
							x.ours && h('div', { class: 'sub' }, `yours${x.host ? `: ${x.host}` : ''}`),
							(x.blocked || []).length > 0 && h('div', null, h('span', { class: 'chip bad' }, `blocked on ${x.blocked.join(', ')}`))),
						h('td', null, h('div', null, x.ssid || h('em', null, 'hidden'), ' ', h('span', { class: `chip ${chip}`, title: means }, label)), h('div', { class: 'sub mono' }, x.bssid)),
						h('td', null, when(x.first_at)),
						h('td', null, ago(x.last_at)),
						h('td', null, x.frames.toLocaleString()),
						h('td', null, x.heard_by.map((a) => h('div', null, a.name))));
				})));
}

function dhcpSection(ctx, nets) {
	return h('section', { class: 'panel' },
		h('h2', null, 'Rogue DHCP servers', h('span', { class: 'note' }, 'networks on which a DHCP server answered in the last hour that is not one of their own, or more than one where a network does not say its own, as the APs say')),
		!nets.length
			? h('div', { class: 'sub' }, 'None: on every network only its own DHCP server answers.')
			: nets.map((n) => {
				// Where the network says the servers it has (0160), the others.
				const listed = n.servers.some((s) => s.known);
				return [
				h('h3', null, n.ssid || n.network, n.ssid && h('span', { class: 'sub mono' }, ` ${n.network}`)),
				h('div', { class: 'sub' }, listed
					? 'A server that is not one the network says it has answers: a rogue, or one set up by mistake. If it is the network’s own, add it to the network’s DHCP servers.'
					: `${n.servers.length} servers answer. One is a rogue, or set up by mistake: the one you did not set up is it. If they are both the network’s own, as a failover pair, list them in the network’s DHCP servers.`),
				h('table', { class: 'list' },
					h('tr', null, ['Server', 'MAC', 'Answers', 'Last answered', 'Seen by'].map((x) => h('th', null, x))),
					n.servers.map((s) => h('tr', null,
						h('td', { class: 'mono' }, s.id, listed && ' ', listed && h('span', { class: `chip ${s.known ? 'ok' : 'bad'}` }, s.known ? 'its own' : 'not its own')),
						h('td', { class: 'mono' }, s.mac),
						h('td', null, s.answers.toLocaleString()),
						h('td', null, ago(s.seen)),
						h('td', null, s.aps.map((a) => h('div', null, link(`/aps/${encodeURIComponent(a.ap)}`, a.name))))))),
				];
			}));
}
