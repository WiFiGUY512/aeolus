// Words for fields, values and times.

import { h, icon } from './dom.js';

// wire is no band: a client test made over the wire has it for one (0128).
const BANDS = { '2g': '2.4 GHz', '5g': '5 GHz', '6g': '6 GHz', 'wire': 'over the wire' };

const SECURITY = {
	'open': 'Open', 'owe': 'OWE (encrypted open)', 'wpa2-psk': 'WPA2',
	'wpa3-sae': 'WPA3', 'wpa2-wpa3': 'WPA2/WPA3',
	'wpa2-enterprise': 'WPA2-Enterprise', 'wpa3-enterprise': 'WPA3-Enterprise', 'wpa2-wpa3-enterprise': 'WPA2/WPA3-Enterprise',
};

const SYSTEM = {
	'country': 'Country', 'tz': 'Time zone', 'ntp': 'NTP servers', 'poll': 'Poll interval', 'agent': 'Agent version',
	'syslog': 'Syslog', 'internet': 'Internet check', 'ssh_keys': 'SSH keys', 'speed_server': 'Speed server, local', 'speed_server_remote': 'Speed server, remote',
	'management.vlan': 'VLAN', 'management.addressing': 'Addressing', 'management.address': 'Address',
	'management.gateway': 'Gateway', 'management.dns': 'DNS',
	'captures': 'Capture failed joins',
	'ble_proxy': 'Bluetooth for Home Assistant',
	'dns_log.url': 'DNS server\'s log', 'dns_log.token': 'Its API token',
	'switches.addresses': 'Switches', 'switches.user': 'User', 'switches.password': 'Password',
	'updates.automatic': 'Install a newer release by itself', 'updates.window': 'Between', 'updates.channel': 'Releases from',
	'advisor.enabled': 'Review on a schedule', 'advisor.key': 'Anthropic API key', 'advisor.model': 'Model',
	'advisor.every': 'Every, in hours', 'advisor.web': 'May search the web', 'advisor.budget': 'Most a review costs, US$',
	'place.address': 'Address', 'place.latitude': 'Latitude', 'place.longitude': 'Longitude',
};

// Radio resource management's fields (0073).
const RRM = { enabled: 'Neighbours', moves: 'Moves', window: 'Window for planned moves', margin: 'Margin for a move', apart: 'Own 5 and 6 GHz kept apart by' };

const APC = { enabled: 'Power control', neighbours: 'Neighbours it looks for', target: 'Target', clients: 'For lonely clients', client_floor: 'Client threshold', hold: 'Hold after struggling' };

const SNMP = {
	'enabled': 'Answer SNMP', 'community': 'v2c community', 'v3.user': 'v3 user',
	'v3.auth': 'v3 auth passphrase (SHA)', 'v3.privacy': 'v3 privacy passphrase (AES)',
	'location': 'Location (sysLocation)', 'contact': 'Contact (sysContact)',
};

const RADIO = { enabled: 'Radio on', channel: 'Channel', width: 'Width', power: 'Power', min_signal: 'Minimum signal to join', beacon_interval: 'Beacon interval', twt: 'Target wake time', bss_color: 'BSS color', beamformer: 'Beamforming to clients', beamformee: 'Beamforming from clients', mu_mimo: 'MU-MIMO' };

const NOTIFY = { ntfy: 'ntfy topic', webhook: 'Webhook', severity: 'Least severe sent', resolved: 'Say when resolved' };
const MAINTENANCE = { on: 'In maintenance', until: 'Until (UTC)', why: 'Why' };
// Where a client belongs (0319).
const HOMES = { aps: 'Steer to these APs', bands: 'On these bands', hold: 'Kept off elsewhere for (s)', nearest: 'To the nearest AP' };
const UPLINK = { stp: 'Spanning tree', bond: 'Uplink bond' };
const PORT = { enabled: 'Port on', speed: 'Speed', uplink: 'Uplink', mode: 'Mode', untagged: 'Untagged VLAN', tagged: 'Tagged VLANs', bond: 'LACP bond' };

const MODE = { access: 'Access', trunk: 'Trunk', native: 'Untagged network', tunnel: 'Tunnel', lacp: 'LACP' };

const NETWORK = {
	'ssid': 'SSID', 'security': 'Security', 'passphrase': 'Passphrase', 'hidden': 'Hidden',
	'bands': 'Bands', 'isolation': 'Client isolation', 'enabled': 'Broadcast', 'test': 'Test network: on only when switched on, for a while (0179)', 'alert_after': 'Alert after failures running', 'dhcp_servers': 'Its DHCP servers', 'max_clients': 'Most clients a band', 'dtim': 'DTIM period', 'blocked': 'Blocked clients',
	'replace_own': 'Replaces the APs’ own',
	'radius.auth_server': 'RADIUS server', 'radius.auth_port': 'RADIUS port', 'radius.auth_secret': 'RADIUS secret',
	'radius.acct_server': 'Accounting server', 'radius.acct_port': 'Accounting port', 'radius.acct_secret': 'Accounting secret', 'radius.nas_id': 'NAS-Identifier',
	'radius.mac_auth': 'MAC authentication', 'radius.passphrases': 'Passphrases from RADIUS', 'radius.vlans': 'VLANs RADIUS may assign', 'radius.vlan_required': 'Require a VLAN from RADIUS',
	'radius.das.client': 'Disconnects from (NAC)', 'radius.das.secret': 'Disconnect secret', 'radius.das.port': 'Disconnect port',
	'radius.test.user': 'Test user', 'radius.test.password': 'Test user’s password', 'radius.test.method': 'Test user signs in by',

	'multicast_to_unicast': 'Multicast to unicast', 'disassoc_low_ack': 'Send off clients that stop acknowledging', 'transition_disable': 'Keep WPA3 clients on WPA3 (Transition Disable)', 'pmf': 'Protected management frames (11w)', 'band_steering': 'Band steering (older name of Suggest a higher band)',
	'steering.suggest': 'Suggest a higher band', 'steering.community': 'Community sharing', 'steering.keep_6g': 'Keep on 6 GHz', 'steering.hold_back': 'Hold back 2.4 GHz', 'steering.sticky': 'Move sticky clients', 'steering.sticky_signal': 'Weak is under (dBm)',
	'steering.sticky_margin': 'Heard better elsewhere by (dB)', 'steering.sticky_floor': 'And here under (dBm)', 'steering.sticky_hold_down': 'Hold down (s)', 'steering.sticky_scan': 'Weigh the scan radios too',
	'roaming.ft': 'Fast roaming (11r)', 'roaming.rrm': 'Neighbor reports (11k)', 'roaming.btm': 'Steering (11v)',
	'rate_limit.down_kbps': 'Download limit', 'rate_limit.up_kbps': 'Upload limit',
	'transport.switching': 'Switching', 'transport.ha': 'HA mode', 'transport.failback': 'Failback', 'transport.holddown': 'Hold-down', 'transport.down_when_lost': 'Off the air when lost',
};

const SLOT = { type: 'type', vlan: 'VLAN', concentrator: 'tunnel', vni: 'VNI', probe: 'probe address' };

const TUNNEL = { address: 'Far end (IP)', port: 'Port', mtu: 'MTU', probe_interval: 'Probe interval', underlay_vlan: 'Starts from' };

// A tunnel port's VNI's fields (0058, 0059).
const PORT_VNI = { tunnel: 'tunnel', vni: 'VNI', probe: 'probe address' };

// HINTS help with filling a field in, a line under it: what to put, where
// to find it, and what it does (Griff, 2026-10-23: "this needs to be more
// descriptive and assisting in filling this info out"). Every field's whole
// description, from the schema, shows when its label is pointed at.
const HINTS = {
	// A network's (0160).
	'network.*.dhcp_servers': 'One a line, each by its address or its MAC: a failover pair, say. Set, only a DHCP server not listed here is an alert; empty, two or more answering are.',
	'notify.ntfy': 'The URL of an ntfy topic, on the ntfy.sh service or an ntfy server of your own, with a name only you know: each alert arrives as a push notification on the phones subscribed to it.',
	'notify.webhook': 'A URL the manager POSTs each alert to, as JSON: for a chat or automation system of your own.',
	'notify.severity': 'Critical sends only alerts where clients are, or will be, without service; warning adds what a person should look at; info sends everything.',
	'notify.resolved': 'Also send a message when an alert’s cause is gone.',
	'maintenance.on': 'On, the APs here have their alerts listed and kept, and none sent: for an AP off or being worked on, on purpose (0218).',
	'maintenance.until': 'When it ends, in UTC: a day, as it begins, or a day and a time. Empty, it lasts until switched off.',
	'maintenance.why': 'What is being done, said with each of its alerts.',
	'system.updates.automatic': 'Off, the manager says a newer release is out, and you install it with Update now. On, it installs it by itself, in the window below.',
	'system.updates.channel': 'stable, the default: only releases that have run a while on testing first. testing: each release as it is made, for a site that tries them.',
	'system.updates.window': 'When it may, in the Org’s time zone, as 03:00-05:00. The manager restarts for a minute or so; the APs and their Wi-Fi keep running.',
	'system.advisor.enabled': 'On, a review runs every so many hours, as below. Off, a review runs only when an admin asks for one on the Advisor page.',
	'system.advisor.key': 'Make one in the Claude Console, with the button beside: it begins sk-ant-. Set here, this folder has an advisor of its own, whose reviews are billed to it. Once saved it is sealed: never shown again, and never sent to an AP.',
	'system.advisor.model': 'Claude Opus 5.5 looks harder; Claude Sonnet 5.5 costs about half as much.',
	'system.advisor.every': 'Hours between reviews: 24 is once a day, 168 once a week.',
	'system.advisor.web': 'Lets a review look a device or its firmware up on the web, a few searches at most.',
	'system.advisor.budget': 'The most one review may spend at the model’s prices, from 0.10 to 50: it stops short of passing it. A day’s review usually costs well under this.',
};

// hint is the help shown under a field, or null.
export function hint(path) {
	// A network's field, by its path in any network: network.*.<field>.
	return HINTS[path] || HINTS[path.replace(/^network\.[^.]+\./, 'network.*.')] || null;
}

// LINKS are pages a person may open beside a field, where its value is made:
// [what the button says, where it goes]. The page fetches nothing from them
// (0069); a person's browser opens them in a tab of their own.
const LINKS = {
	// Where an Anthropic API key is made (0205; Griff, 2026-10-23).
	'system.advisor.key': ['Make a key ↗', 'https://platform.claude.com/settings/keys'],
};

// fieldLink is the page a person may open beside a field, or null.
export function fieldLink(path) {
	return LINKS[path] || null;
}

// group says which panel a field belongs in, and its label there.
// Service assurance (0123): client tests on a schedule.
const ASSURANCE = { 'tests.every': 'A client test', 'tests.over': 'Which tests', 'tests.speed': 'With its local speed', 'tests.speed_remote': 'With its remote speed', 'tests.roam': 'Roaming to the other APs, once on', 'tests.wire': 'Over the wire too (before)',
	// What they join, and when they wait (0180).
	'tests.paused': 'Paused', 'tests.per_hour': 'At most, tests through the air an hour', 'tests.alert_after': 'Alert after failures running', 'tests.window': 'Only between, its local time', 'tests.windows': 'Only in these blocks, its local time, one a line: 08:00-17:00, mon-fri 08:00-17:00, sat,sun 10:00-12:00', 'tests.until': 'A run that ends', 'tests.bands': 'Only these bands', 'tests.networks': 'Only these networks', 'tests.leave_out': 'Never these networks', 'tests.on': 'Joined on', 'tests.listed': 'The APs listed',
	'tests.floor': 'Weakest AP heard, dBm', 'tests.max_clients': 'Wait while a radio has more clients than', 'tests.max_busy': 'Wait while the channel is busier than, %' };

export function group(path) {
	const p = path.split('.');
	// A kind of AP's own (0092): its field's, for that board.
	if (p[0] === 'boards' && p.length > 2) {
		const g = group(p.slice(2).join('.'));
		return { ...g, key: `boards.${p[1]}.${g.key}`, title: `${g.title} (${p[1]})` };
	}
	if (p[0] === 'radio') return { key: 'radio.' + p[1], title: (BANDS[p[1]] || p[1]) + ' radio', order: 1 + Object.keys(BANDS).indexOf(p[1]) / 10, label: RADIO[p[2]] || p[2] };
	if (p[0] === 'system' && p[1] === 'snmp') return { key: 'snmp', title: 'SNMP', order: 2.5, label: SNMP[p.slice(2).join('.')] || p.slice(2).join('.') };
	if (p[0] === 'system' && p[1] === 'management') return { key: 'management', title: 'Management', order: 3, label: SYSTEM[p.slice(1).join('.')] || p[2] };
	if (p[0] === 'system') return { key: 'system', title: 'System', order: 2, label: SYSTEM[p.slice(1).join('.')] || SYSTEM[p[1]] || p.slice(1).join(' ') };
	// A tunnel port's VNI, by how it is carried: "VLAN 50 tunnel" (0058).
	if (p[0] === 'ports' && p[2] === 'vxlan') return { key: 'ports.' + p[1], title: 'Port ' + p[1], order: 4, label: `${p[3] === 'untagged' ? 'Untagged' : 'VLAN ' + p[3]} ${PORT_VNI[p[4]] || p[4]}` };
	if (p[0] === 'ports') return { key: 'ports.' + p[1], title: 'Port ' + p[1], order: 4, label: PORT[p[2]] || p[2] };
	if (p[0] === 'notify') return { key: 'notify', title: 'Alerts', order: 2.9, label: NOTIFY[p[1]] || p[1] };
	if (p[0] === 'maintenance') return { key: 'maintenance', title: 'Maintenance', order: 2.95, label: MAINTENANCE[p[1]] || p[1] };
	if (p[0] === 'homes') return { key: 'homes.' + p[1], title: 'Client steering for ' + p[1], order: 6.8, label: HOMES[p[2]] || p[2] };
	if (p[0] === 'assurance') return { key: 'assurance', title: 'Assurance', order: 2.8, label: ASSURANCE[p.slice(1).join('.')] || p.slice(1).join('.') };
	if (p[0] === 'uplink') return { key: 'uplink', title: 'Uplink', order: 3.9, label: UPLINK[p[1]] || p[1] };
	if (p[0] === 'concentrators' && p[2] === 'bridged')
		return { key: 'concentrators.' + p[1], title: 'Tunnel ' + p[1], order: 4.5, label: `VNI on ${p[3] === 'untagged' ? 'the untagged network' : 'VLAN ' + p[3]} (0335)` };
	if (p[0] === 'concentrators') return { key: 'concentrators.' + p[1], title: 'Tunnel ' + p[1], order: 4.5, label: TUNNEL[p[2]] || p[2] };
	if (p[0] === 'services') return { key: 'services', title: 'Services', order: 5, label: 'Service folders' };
	if (p[0] === 'rrm') return { key: 'rrm', title: 'Radio resource management', order: 6, label: RRM[p[1]] || p[1] };
	if (p[0] === 'apc') return { key: 'apc', title: 'Power control', order: 6.5, label: APC[p[1]] || p[1] };
	if (p[0] === 'network') {
		const f = p.slice(2).join('.');
		let label = NETWORK[f];
		if (!label && p[2] === 'transport') label = (p[3] === 'primary' ? 'Primary ' : 'Fallback ') + (SLOT[p[4]] || p[4]);
		return { key: 'network.' + p[1], title: p[1], order: 10, label: label || f, network: p[1] };
	}
	return { key: p[0], title: p[0], order: 20, label: p.slice(1).join('.') || p[0] };
}

// value writes a field's value for people. Secrets are never readable
// (0027); the API sends them as {sealed: true}.
export function value(path, v, names) {
	if (v && typeof v === 'object' && v.sealed === true) return h('span', { class: 'sealed' }, 'sealed');
	if (/(^|\.)uplink\.stp$/.test(path) && typeof v === 'boolean') return v ? 'on, RSTP' : 'off';
	if (/(^|\.)uplink\.bond$/.test(path) && typeof v === 'boolean') return v ? 'LACP bond' : 'two ports, apart';
	if (typeof v === 'boolean') return v ? 'on' : 'off';
	const last = path.split('.').pop();
	if (path === 'services' && Array.isArray(v)) return v.map((id) => names(id)).join(' + ') || 'none';
	if (/^homes\.[^.]+\.aps$/.test(path) && Array.isArray(v)) return v.map((id) => names(id)).join(', ');
	if (Array.isArray(v)) return v.map((x) => (last === 'bands' ? BANDS[x] || x : String(x))).join(', ');
	if (last === 'security') return SECURITY[v] || v;
	if (last === 'width' && typeof v === 'number') return v + ' MHz';
	if (last === 'channel' && v === 'auto') return 'automatic'; // within the band's channel set (0075)
	if (last === 'power' && typeof v === 'number') return v + ' dBm';
	if (last === 'min_signal') return v === 'off' ? 'off' : v + ' dBm';
	if (last === 'beacon_interval') return v + ' TU';
	if (last === 'bss_color') return v === 'auto' ? 'random at each start' : v === 'off' ? 'off' : `color ${v}`;
	if (last === 'dtim') return `every ${v} beacon${v === 1 ? '' : 's'}`;
	if (last === 'dfs') return v === 'avoid' ? 'avoided' : 'allowed'; // look-first, from before 0328, is allow
	if (path === 'system.agent') return v === 'current' ? "current: the manager's own" : String(v);
	if (/\.radius\.test\.method$/.test(path)) return v === 'ttls' ? 'TTLS with PAP' : 'PEAP with MSCHAPv2';
	if (path === 'assurance.tests.over') return v === 'wire' ? 'over the wire' : v === 'both' ? 'through the air and over the wire' : 'through the air';
	if (path === 'assurance.tests.on') return v === 'self' ? 'itself alone' : v === 'listed' ? 'the APs listed' : 'itself and the APs it hears';
	if (path === 'assurance.tests.every') return v === 0 ? 'never' : v < 60 ? `every ${v} minutes` : v === 60 ? 'every hour' : v < 1440 ? `every ${v / 60} hours` : 'once a day';
	if (path === 'rrm.window' || path === 'assurance.tests.window') return `${String(v).replace('-', '–')}, the AP's local time`;
	if (path === 'assurance.tests.windows') return `${(Array.isArray(v) ? v : [v]).map((b) => String(b).replace(/(\d)-(\d)/, '$1–$2')).join(' · ')}, the AP's local time`;
	if (path === 'assurance.tests.until') return new Date(v * 1000).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit' });
	if (path === 'system.updates.window') return `${String(v).replace('-', '–')}, the Org's local time`;
	if (path === 'system.advisor.every') return v === 1 ? 'every hour' : `every ${v} hours`;
	if (path === 'system.advisor.budget') return `$${Number(v).toFixed(2)}`;
	if (path === 'system.advisor.model') return v === 'claude-sonnet-5-5' ? 'Claude Sonnet 5.5, about half the cost' : 'Claude Opus 5.5';
	if (path === 'rrm.margin') return `${v} rating points`;
	if (path === 'rrm.apart') return v === 0 ? 'no rule' : `${v} MHz`;
	if (path === 'apc.target' || path === 'apc.client_floor') return `${v} dBm`;
	// What power control does for clients only one AP reaches (0206).
	if (path === 'apc.clients') return v === 'guard' ? 'guard: keep them in reach' : v === 'ignore' ? 'ignore: neighbours alone' : 'ask: warn me';
	if (path === 'apc.hold') return `${v} h`;
	if (last === 'poll' || last === 'holddown' || last === 'probe_interval') return v + ' s';
	if (last.endsWith('_kbps')) return v === 0 ? 'no limit' : v + ' kbps';
	if (last === 'type' && path.includes('.transport.')) return v === 'vxlan' ? 'VXLAN' : 'VLAN';
	if (path.startsWith('ports.') && last === 'mode') return MODE[v] || v;
	// A port's speed (0094): Auto, or as people say it, 1G.
	if (/(^|\.)ports\.[^.]+\.speed$/.test(path)) return v === 'auto' ? 'Auto' : v >= 1000 ? `${v / 1000}G` : `${v}M`;
	if (path.startsWith('ports.') && last === 'untagged' && v === 0) return 'none';
	// Where a tunnel starts on the AP (0063): 0 is the management VLAN.
	if (last === 'underlay_vlan' && typeof v === 'number') return v ? 'VLAN ' + v : 'the management VLAN';
	if (v == null) return '—';
	return String(v);
}

export function security(v) {
	return SECURITY[v] || v;
}

// origin shows where a value comes from (0012), as in the mockup.
export function origin(tree, here, r, names) {
	const from = r.from;
	const to = (label, cls) => h('a', { class: 'chip ' + cls, href: `#/${tree}/${encodeURIComponent(from)}` }, label);
	switch (r.origin) {
	case 'self':
		return h('span', { class: 'chip here' }, 'Set here');
	case 'inherited':
		return to('From ' + names(from), 'from');
	case 'locked':
		return from === here
			? h('span', { class: 'chip lockhere' }, icon('lock'), 'Locked here')
			: to('Locked by ' + names(from), 'locked');
	case 'baseline':
		return to('Branch baseline · ' + names(from), 'baseline');
	case 'template':
		// The AP's template (0085), picked at that folder.
		return to('Template · picked at ' + names(from), 'template');
	}
	return null;
}

// whence is where a value comes from in a word, for a dense list of
// settings (0284): "here" where it is set here; else the folder it comes
// from, a link there, with a lock where it is locked, or what kind of value
// it is where it is a branch's baseline or a template's.
export function whence(tree, here, r, names) {
	const from = r.from;
	const to = (cls, ...label) => h('a', { class: 'whence ' + cls, href: `#/${tree}/${encodeURIComponent(from)}`, title: `Set at ${names(from)}` }, ...label);
	switch (r.origin) {
	case 'self':
		return h('span', { class: 'whence here', title: 'Set here' }, 'here');
	case 'inherited':
		return to('from', names(from));
	case 'locked':
		return from === here
			? h('span', { class: 'whence here', title: 'Locked here: nothing below may change it' }, icon('lock'), ' here')
			: to('locked', icon('lock'), ' ', names(from));
	case 'baseline':
		return to('baseline', 'baseline · ', names(from));
	case 'template':
		return to('template', 'template · ', names(from));
	}
	return null;
}

// templateSays says whether an AP follows its AP template (0085), or has
// some of the template's settings replaced closer to it, or takes none.
export function templateSays(t) {
	if (!t) return '';
	if (!t.id) return t.board ? `No template for ${t.board}` : 'No template: it reported no board';
	if (t.follows) return `Follows ${t.name}`;
	const n = t.replaced.length;
	return `${t.name}, ${n} setting${n === 1 ? '' : 's'} replaced`;
}

export function when(t) {
	if (!t) return 'never';
	return new Date(t).toLocaleString();
}

// What changes only what the prober asks, and how often, reloads nothing: it is
// the agent's own config, which the prober reads again within 10 seconds (0059).
export const PROBE_ONLY = 'Nothing reloads on the APs: each one\'s prober picks this up within 10 seconds.';

// probeOnly says whether paths change only what the prober reads: what it
// asks and how often, and how a network switches between its transports
// (0061).
export function probeOnly(paths) {
	return paths.length > 0 && paths.every((p) => /\.(probe|probe_interval)$|\.transport\.(switching|failback|holddown)$/.test(p));
}

// uplinkJudgment says what a VLAN watched on an AP's uplink means (0064),
// with the VLANs the switch's LLDP names, if it names any: [chip class,
// word, what to tell, or null when all is well].
export function uplinkJudgment(v, neighbor) {
	if (v.verdict === 'present') return ['ok', 'present', null];
	if (v.verdict !== 'silent') return ['idle', 'watching', null];
	const named = neighbor?.vlans?.length ? neighbor.vlans : null;
	if (named && !named.includes(v.vlan)) return ['bad', 'missing', `The switch port doesn't carry VLAN ${v.vlan}`];
	if (named) return ['warn', 'quiet', `The switch port carries VLAN ${v.vlan}, but nothing on it answers`];
	return ['bad', 'silent', `Nothing comes in on VLAN ${v.vlan}: it most likely isn't on the switch port`];
}

// vlanUsers names what needs a VLAN in an AP's config (0064): its networks,
// by SSID, its ports and its tunnels.
export function vlanUsers(doc, vlan) {
	const out = [];
	for (const [id, n] of Object.entries(doc?.network || {}))
		if (n.enabled !== false && ['primary', 'fallback'].some((s) => n.transport?.[s]?.type === 'vlan' && n.transport[s].vlan === vlan))
			out.push(n.ssid || id);
	for (const [p, set] of Object.entries(doc?.ports || {}))
		if ((set.mode === 'access' || set.mode === 'trunk') && (set.untagged === vlan || (set.mode === 'trunk' && (set.tagged || []).includes(vlan))))
			out.push(p);
	for (const [id, c] of Object.entries(doc?.concentrators || {}))
		if (c.underlay_vlan === vlan) out.push(`tunnel ${id}`);
	return out;
}

// dhcpWarnings says what is wrong with a network's DHCP on an AP (0065), one
// line each: nothing answers its clients' requests; more than one server
// answers them; clients joined and asked nothing, and show no address.
export function dhcpWarnings(name, d) {
	const out = [];
	const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;
	if (d.unanswered > 0 && !d.answered)
		out.push(`On ${name}, DHCP isn't answering: ${plural(d.unanswered, 'request', 'requests')} from ${plural((d.unanswered_clients || []).length, 'client', 'clients')} in 10 minutes, none answered`);
	const servers = [...new Set((d.duplicates || []).flatMap((x) => x.servers))];
	if (servers.length > 1) {
		const mac = (id) => (d.servers || []).find((s) => s.id === id)?.mac;
		out.push(`On ${name}, ${servers.length} DHCP servers answer: ${servers.map((s) => mac(s) ? `${s} (${mac(s)})` : s).join(' and ')}`);
	}
	const stuck = (d.without || []).filter((c) => !c.address);
	if (stuck.length)
		out.push(`On ${name}, ${stuck.length === 1 ? 'a client has' : `${stuck.length} clients have`} no address: ${stuck.map((c) => c.mac).join(', ')} joined and asked nothing by DHCP`);
	return out;
}

// switchPort names the switch and port an AP's uplink is on, as the switch's
// LLDP says (0064), or null.
export function switchPort(n) {
	if (!n) return null;
	return `${n.system || n.chassis || 'the switch'} ${n.port || ''}`.trim() + (n.port_description ? ` (${n.port_description})` : '');
}

// ERRORS_WARN and ERRORS_BAD are how many of an uplink's frames were errors,
// since it came up, before its errors are amber and red: one in 100,000,
// and one in 1,000. A wrong duplex or a bad cable is far past the first; a
// frame or two lost as the link came up is far short of it.
const ERRORS_WARN = 1e-5, ERRORS_BAD = 1e-3;

// uplinkBrief says an AP's uplink in a few words (Griff, 2026-10-12: an AP
// that came up at the wrong port speed is to show on the APs tab, and a
// port's errors somewhere). text is the speed each of its wires synced at,
// a bond's members each, such as "2.5G" or "1G + 1G"; one below what its
// port and the far end can both do says so, "100M of 1G". grade is warn for
// that, for half duplex, and for a wire with no link beside one that has
// it; bad where none has a link. says is the same in full, for a hover,
// each wire with the switch and port its own LLDP names, where it says:
// the wires of an uplink may go to different ports, or switches.
// errors, where the uplink has counted any, is how many, graded by how many
// of its frames they were. Null where the report names no uplink.
export function uplinkBrief(r) {
	const ports = (r?.ports || []).filter((p) => p.uplink);
	if (!ports.length) return null;
	const short = (m) => (m >= 1000 ? `${m / 1000}G` : `${m}M`);
	const rank = { ok: 1, warn: 2, bad: 3 };
	let grade = 'ok';
	const worse = (g) => { if (rank[g] > rank[grade]) grade = g; };
	const wires = ports.flatMap((p) => (p.bond?.members?.length ? p.bond.members : [p]));
	const says = [];
	let linked = 0;
	// Where a wire goes, by its own neighbour.
	const named = (w) => w.neighbor && (w.neighbor.system || w.neighbor.port);
	const on = (w) => (named(w) ? `, on ${[w.neighbor.system, w.neighbor.port].filter(Boolean).join(' ')}` : '');
	const text = wires.map((w) => {
		if (!w.carrier) {
			worse('warn');
			says.push(`${w.name}: no link${on(w)}`);
			return 'no link';
		}
		linked++;
		const m = /^(\d+)([FH])$/.exec(w.speed || '');
		if (!m) {
			says.push(`${w.name}: a link, its speed not said${on(w)}`);
			return 'up';
		}
		const now = Number(m[1]), half = m[2] === 'H';
		// What the port and the far end can both do, where both say.
		const best = w.max && w.partner_max ? Math.min(w.max, w.partner_max) : null;
		const slow = best != null && now < best;
		if (slow || half) worse('warn');
		says.push(`${w.name}: synced at ${short(now)}${half ? ', half duplex' : ''}${w.max ? `; the port can do ${short(w.max)}` : ''}${w.partner_max ? `, the far end offers up to ${short(w.partner_max)}` : ''}${slow ? `: below what both can do` : ''}${on(w)}`);
		return `${short(now)}${half ? ' half' : ''}${slow ? ` of ${short(best)}` : ''}`;
	}).join(' + ');
	if (!linked) grade = 'bad';
	// The uplink's own neighbour, where no wire names one for itself.
	const to = !wires.some(named) && switchPort(r.uplink_neighbor);
	if (to) says.push(`on ${to}`);
	const u = r.uplink_port;
	const n = u ? (u.rx_errors || 0) + (u.tx_errors || 0) : 0;
	let errors = null;
	if (n > 0) {
		const frames = (u.rx_packets || 0) + (u.tx_packets || 0);
		const share = frames > 0 ? n / frames : 1;
		errors = {
			n,
			text: `${n.toLocaleString()} error${n === 1 ? '' : 's'}`,
			grade: share >= ERRORS_BAD ? 'bad' : share >= ERRORS_WARN ? 'warn' : '',
			says: `Since the uplink came up: ${u.rx_errors || 0} errors in and ${u.tx_errors || 0} out${frames > 0 ? `, one frame in ${Math.max(1, Math.round(frames / n)).toLocaleString()}` : ''}; ${u.rx_dropped || 0} dropped in and ${u.tx_dropped || 0} out.`,
		};
	}
	return { text, grade, says: says.join('. ') + '.', errors };
}

// secondsAgo writes as ago does the time s seconds before at, such as an
// AP's report counting from when it was made.
export function secondsAgo(s, at) {
	return ago(new Date(at ?? Date.now()).getTime() - s * 1000);
}

// QUIET is how long an AP may go unheard before it is taken for offline:
// three poll intervals of the default length. quiet says whether an AP last
// seen at seen (a time) is offline now; one never seen is.
export const QUIET = 180;
export function quiet(seen) {
	return !seen || (Date.now() - new Date(seen).getTime()) / 1000 > QUIET;
}

export function ago(t) {
	if (!t) return 'never';
	const s = Math.round((Date.now() - new Date(t).getTime()) / 1000);
	if (s < 10) return 'just now';
	if (s < 60) return s + ' s ago';
	if (s < 3600) return Math.round(s / 60) + ' min ago';
	if (s < 86400) return Math.round(s / 3600) + ' h ago';
	return Math.round(s / 86400) + ' d ago';
}

// size writes bytes the short way: 812 B, 1.9 MB, 83.7 GB.
export function size(n) {
	for (const [d, unit] of [[1e12, 'TB'], [1e9, 'GB'], [1e6, 'MB'], [1e3, 'kB']])
		if (n >= d) return `${(n / d).toFixed(1)} ${unit}`;
	return `${n} B`;
}

export function bandName(b) {
	return BANDS[b] || b;
}

// prose is text written as paragraphs and lists, numbered or not, its
// **bold** bold, as the advisor (0215) and a release's notes (0217) are:
// built of elements, never as HTML, so nothing in it runs.
export function prose(text) {
	const inline = (line) => line.split(/(\*\*[^*]+\*\*)/).map((part) => (/^\*\*[^*]+\*\*$/.test(part) ? h('strong', null, part.slice(2, -2)) : part));
	const out = [];
	let list = null;
	for (const raw of (text || '').split('\n')) {
		const line = raw.trim();
		const item = line.match(/^([-*•]|\d+[.)])\s+(.*)$/);
		if (item) {
			// Numbered where it numbers them.
			const kind = /\d/.test(item[1]) ? 'ol' : 'ul';
			if (list?.localName !== kind) out.push(list = h(kind));
			list.append(h('li', null, ...inline(item[2])));
			continue;
		}
		list = null;
		if (line) out.push(h('p', null, ...inline(line.replace(/^#+\s+/, ''))));
	}
	return h('div', { class: 'said' }, out);
}
