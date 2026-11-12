// A capture someone starts (0148): what crosses the AP, taken in its own
// path. On the Wi-Fi side of a network that is the network's interface on
// each radio, where a client's frames are already out of their encryption;
// on the wired side it is the port the network leaves the AP by, its VLAN
// there, or its tunnel. No I/O here: the agent looks at the AP, starts
// tcpdump, and sends what it writes.

'use strict';

const DIR = '/tmp/aeolus-capture';   // a capture's files while it is taken and sent: in memory, gone at a restart of the AP
const MOST = 8;                 // interfaces taken at once
const PACKET = 1600;            // bytes a whole packet is reckoned at, with what a file adds to it
const RECORD = 16;              // bytes a file adds to each packet
const LEAST = 200;              // packets an interface is let take, at the least

const MAC = /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/;
const NAME = /^[A-Za-z0-9][A-Za-z0-9._-]{0,14}$/;   // an interface's name: nothing a shell reads

// ports is a bridge VLAN's ports as the AP's config has them, a list or one
// string: each { name, tagged }.
function ports(v) {
	let out = [];
	for (let p in (type(v) == 'array' ? v : split(trim('' + (v ?? '')), /\s+/))) {
		let m = match(p, /^([^:]+)(:(.*))?$/);
		if (m && length(m[1]))
			push(out, { name: m[1], tagged: index(m[3] ?? '', 't') >= 0 });
	}
	return out;
}

// plan is what a capture takes: its interfaces, each { name, side, note,
// filter }, the filter being what tcpdump is told to keep; or { error }.
//
// ask is what was asked: { sides: 'wireless', 'wired' or 'both'; network,
// one of the AP's by its ID, or vlan; bands, none for all; client, a MAC,
// none for everyone }.
//
// have is the AP as it is: wifi, its networks' Wi-Fi interfaces by name,
// each { nets, the config's names of the networks it is in; band; ssid };
// network, the AP's network config, a section by its name; members, each
// bridge's ports as the kernel has them; uplink, the port the AP's own
// traffic leaves by.
function plan(ask, have) {
	if (!(ask.sides in ['wireless', 'wired', 'both']))
		return { error: 'which side to take was not said' };
	if (ask.client != null && !match(ask.client, MAC))
		return { error: 'the client is not a MAC' };
	let host = ask.client ? 'ether host ' + ask.client : '';
	let both = (a, b) => length(a) && length(b) ? a + ' and ' + b : a + b;
	let bridge = null, vlan = null, nets = {};
	if (ask.network != null) {
		let net = 'aeolus_' + replace('' + ask.network, /-/g, '_');
		let dev = have.network?.[net]?.device;
		if (type(dev) != 'string' || !length(dev))
			return { error: 'this AP does not have that network' };
		let m = match(dev, /^(.+)\.([0-9]+)$/);
		bridge = m ? m[1] : dev;
		vlan = m ? int(m[2]) : null;
		nets[net] = true;
	}
	else {
		vlan = int(ask.vlan);
		if (!(vlan >= 1 && vlan <= 4094))
			return { error: 'the VLAN is not one' };
		// The bridge the AP's uplink is in carries its VLANs.
		for (let b, list in have.members ?? {})
			if (index(list, have.uplink) >= 0)
				bridge = b;
		if (!bridge)
			return { error: 'the AP\'s uplink is in no bridge, so it carries no VLAN to take' };
		for (let name, s in have.network ?? {})
			if (s?.['.type'] == 'interface' && s.device == bridge + '.' + vlan)
				nets[name] = true;
	}
	let files = [];
	if (ask.sides != 'wired')
		for (let name in sort(keys(have.wifi ?? {}))) {
			let w = have.wifi[name];
			if (!match(name, NAME) || !length(filter(w.nets ?? [], n => nets[n])))
				continue;
			if (length(ask.bands ?? []) && !(w.band in ask.bands))
				continue;
			push(files, { name: name, side: 'wireless', note: join(', ', filter([w.ssid, w.band], x => length(x ?? ''))), filter: host });
		}
	if (ask.sides != 'wireless') {
		let wired = [];
		if (vlan != null) {
			for (let name, s in have.network ?? {})
				if (s?.['.type'] == 'bridge-vlan' && s.device == bridge && int(s.vlan) == vlan)
					for (let p in ports(s.ports))
						if (!have.wifi?.[p.name])
							push(wired, { name: p.name, side: 'wired', note: sprintf('VLAN %d%s', vlan, p.tagged ? '' : ', untagged'), filter: both(host, p.tagged ? 'vlan ' + vlan : 'not vlan') });
			// Where the uplink carries it, that is the wired side: the
			// AP's other ports are wired clients' own.
			let up = filter(wired, f => f.name == have.uplink);
			if (length(up))
				wired = up;
		}
		else
			// A network with no VLAN of its own here: a tunnel's, whose
			// bridge has the tunnel for its wired side.
			for (let p in have.members?.[bridge] ?? [])
				if (!have.wifi?.[p])
					push(wired, { name: p, side: 'wired', note: 'the tunnel', filter: host });
		for (let f in sort(wired, (a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0))
			if (match(f.name, NAME))
				push(files, f);
	}
	if (!length(files))
		return { error: ask.sides == 'wireless' ? 'no Wi-Fi interface of this AP carries that' : 'nothing on this AP carries that' };
	if (length(files) > MOST)
		files = slice(files, 0, MOST);
	return { files: files };
}

// each is how many packets each of n interfaces may take, of a capture
// that may hold so many bytes: whole packets reckoned at their largest,
// since a file's size is not known until it is written.
function each(bytes, n, snap) {
	let one = (snap > 0 ? snap : PACKET) + RECORD;
	return max(LEAST, int(bytes / max(1, n) / one));
}

// script is the shell script that takes a capture in dir: a tcpdump for
// each interface, writing its file as packets come, and a timer; when the
// timer ends, or is ended for it, they are stopped, and a file says so.
// Every word in it is one this module made or checked.
function script(dir, files, seconds, bytes, snap) {
	let n = each(bytes, length(files), snap);
	let out = '#!/bin/sh\ncd ' + dir + ' || exit 1\n';
	for (let i = 0; i < length(files); i++) {
		let f = files[i];
		out += sprintf("tcpdump -i %s -p -n -U -s %d -c %d -w %d.pcap %s 2>%d.err &\necho $! >%d.pid\n",
			f.name, snap > 0 ? snap : 262144, n, i, length(f.filter) ? "'" + f.filter + "'" : '', i, i);
	}
	out += sprintf("sleep %d &\necho $! >timer.pid\nwait $!\n", seconds);
	out += "for p in *.pid; do [ \"$p\" = timer.pid ] || kill \"$(cat \"$p\")\" 2>/dev/null; done\nwait\n: >ended\n";
	return out;
}

// counted is what tcpdump said as it ended, from what it wrote to its
// errors: how many packets it took, and how many the kernel dropped before
// it could. Null where it said nothing of either, as where it never ran.
function counted(text) {
	let took = match(text ?? '', /([0-9]+) packets? captured/), dropped = match(text ?? '', /([0-9]+) packets? dropped by kernel/);
	if (!took)
		return null;
	return { packets: int(took[1]), dropped: dropped ? int(dropped[1]) : 0 };
}

// failed is why a tcpdump took nothing, from what it wrote to its errors:
// its own words, the first line that is not its greeting.
function failed(text) {
	for (let line in split(text ?? '', '\n')) {
		line = trim(line);
		if (length(line) && !match(line, /^tcpdump: (listening on|verbose output suppressed)/) && !match(line, /packets? (captured|received|dropped)/))
			return substr(replace(replace(line, /^tcpdump: /, ''), /[^ -~]/g, '?'), 0, 100);
	}
	return null;
}

export { DIR, plan, each, script, counted, failed, ports };
