// The client test's pure parts (0121), so they are tested anywhere: what
// wpa_supplicant is told for a network, what its events mean, a DNS
// question and its answer, the frequencies a client looks on, and what a
// test's findings come to.

'use strict';

const STAGES = ['find', 'auth', 'assoc', 'signin', 'key', 'dhcp', 'gateway', 'dns', 'speed', 'path', 'roam'];
const ROAMS = 8;       // APs a test roams to, at most (0225)
const STEPS = 40;      // steps a test's result keeps
const HEARD = 8;       // and of the networks the scan radio heard before it

// between says whether airscan, by its status, is between its jobs, with
// none about to start: the scan radio may be taken then. A job is about to
// start from SOON seconds before it is due, and for a while after, should
// it be late.
const SOON = 3;

function between(status, wall) {
	if (!(status?.state in ['clients', 'idle']))
		return false;
	for (let job in ['bss', 'spectral']) {
		let due = status.next?.[job];
		if (type(due) == 'int' && due > wall - 30 && due <= wall + SOON)
			return false;
	}
	return true;
}

function hexstr(s) {
	let o = '';
	for (let i = 0; i < length(s); i++)
		o += sprintf('%02x', ord(s, i));
	return o;
}

// The bytes one POST carries to a speed server, unless the server takes
// fewer; and where an OpenSpeedTest server, as the local one is, gives
// bytes and takes them.
const SPEED_BODY = 33554432;
const OPENSPEEDTEST = { down: '/downloading', up: '/upload', body: SPEED_BODY };

// freq_of is a channel's frequency on a band, or null for none there.
function freq_of(band, ch) {
	if (band == '2g' && ch >= 1 && ch <= 14)
		return ch == 14 ? 2484 : 2407 + 5 * ch;
	if (band == '5g' && ch >= 32 && ch <= 177)
		return 5000 + 5 * ch;
	if (band == '6g' && ch >= 1 && ch <= 233)
		return ch == 2 ? 5935 : 5950 + 5 * ch;
	return null;
}

// wanted reads what a test is asked for, and returns it checked, or null:
// the network's SSID, and its ID where the manager asks; the user it signs
// in as, where the network signs clients in by 802.1X (0124): how, PEAP
// with MSCHAPv2 or TTLS with PAP, a name and a password; the BSSIDs it may
// join, an AP's own, or none for the
// strongest other AP; a band, or none for any; an OpenSpeedTest server, as
// a plain HTTP address with its host and, if it has one, its port, or none;
// and a name to look up.
function wanted(p) {
	if (type(p) != 'object' || type(p.ssid) != 'string' || !length(p.ssid) || length(p.ssid) > 32)
		return null;
	let out = { ssid: p.ssid, network: null, eap: null, bssids: [], channels: [], band: null, speed: null, dns_name: 'example.com', ap: null, roam: [] };
	if (p.eap != null) {
		let e = p.eap;
		let said = (v) => type(v) == 'string' && length(v) >= 1 && length(v) <= 128;
		if (type(e) != 'object' || !(e.method in ['peap', 'ttls']) || !said(e.identity) || !said(e.password))
			return null;
		out.eap = { method: e.method, identity: e.identity, password: e.password };
	}
	if (p.network != null) {
		if (type(p.network) != 'string' || !match(p.network, /^[a-z][a-z0-9-]{0,31}$/))
			return null;
		out.network = p.network;
	}
	for (let b in (type(p.bssids) == 'array' ? p.bssids : [])) {
		if (type(b) != 'string' || !match(lc(b), /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) || length(out.bssids) >= 32)
			return null;
		push(out.bssids, lc(b));
	}
	// Where the AP asked for is now, by its last report (0239): a radio
	// that moved since the scan radio heard it is looked for there too.
	for (let c in (type(p.channels) == 'array' ? p.channels : [])) {
		if (type(c) != 'object' || !(c.band in ['2g', '5g', '6g']) || type(c.channel) != 'int' || length(out.channels) >= 8)
			return null;
		let f = freq_of(c.band, c.channel);
		if (f == null)
			return null;
		push(out.channels, { band: c.band, channel: c.channel, freq: f });
	}
	// wire is no band: the test is made over the wire, not through the
	// air (0128). It is made from a MAC that says which AP's it is, so it
	// is told which AP this is.
	if (p.band != null) {
		if (!(p.band in ['2g', '5g', '6g', 'wire']))
			return null;
		out.band = p.band;
	}
	if (p.ap != null) {
		if (type(p.ap) != 'string' || !match(p.ap, /^ap-[0-9a-f]{12}$/))
			return null;
		out.ap = p.ap;
	}
	if (out.band == 'wire' && (!out.ap || !out.network || out.eap || length(p.bssids ?? [])))
		return null;
	// The APs to roam to once on, in turn, each by its BSSIDs (0225): the
	// test goes to each one's BSSID on the band it joined on, and back.
	if (p.roam != null) {
		if (type(p.roam) != 'array' || length(p.roam) > ROAMS || out.band == 'wire')
			return null;
		for (let ap in p.roam) {
			if (type(ap) != 'array' || !length(ap) || length(ap) > 32)
				return null;
			let list = [];
			for (let b in ap) {
				if (type(b) != 'string' || !match(lc(b), /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/))
					return null;
				push(list, lc(b));
			}
			push(out.roam, list);
		}
	}
	if (p.speed != null) {
		let m = type(p.speed) == 'string' ? match(p.speed, /^http:\/\/([A-Za-z0-9.-]{1,253})(:([0-9]{1,5}))?\/?$/) : null;
		let port = int(m?.[3] ?? '80');
		if (!m || port < 1 || port > 65535)
			return null;
		out.speed = { url: p.speed, host: m[1], port: port };
	}
	// The remote speed server (0134): any HTTP server, by its host and
	// port, with the path a GET of which gives bytes, the path a POST to
	// which takes them, and how many bytes a POST carries. The test writes
	// the host and the paths into its requests as they are, so one that is
	// not plain is not used.
	if (p.speed_remote != null) {
		let r = p.speed_remote;
		// The whole of each, and nothing after what was looked at: a
		// pattern stops at a zero byte, and the string does not.
		let whole = (v, re) => type(v) == 'string' && length(v) > 0 && length(match(v, re)?.[0] ?? '') == length(v);
		let path = (v) => whole(v, /^\/[A-Za-z0-9._~\/?&=%+-]{0,200}$/);
		let name = type(r?.name) == 'string' && length(r.name) ? replace(substr(r.name, 0, 64), /[^ -~]/g, '?') : null;
		// One that cannot be used is said so, and fails no test: the rest
		// of the test is what the network gives, whatever the manager named.
		if (type(r) != 'object' || !whole(r.host, /^[A-Za-z0-9.-]{1,253}$/) || type(r.port) != 'int' || r.port < 1 || r.port > 65535 ||
		    !path(r.down) || !path(r.up) || (r.body != null && (type(r.body) != 'int' || r.body < 65536 || r.body > SPEED_BODY)))
			out.speed_remote = { name: name ?? 'the remote server', unusable: true };
		else
			out.speed_remote = { name: name ?? r.host, host: r.host, port: r.port, down: r.down, up: r.up, body: r.body ?? SPEED_BODY };
	}
	if (p.dns_name != null) {
		if (type(p.dns_name) != 'string' || !match(p.dns_name, /^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$/))
			return null;
		out.dns_name = p.dns_name;
	}
	return out;
}

// credentials are how the network asked for is secured on this AP, and its
// passphrase, as the AP's wireless config has them (ifaces, its wifi-iface
// sections; radios, the names of its wifi-device sections). Asked for by
// its ID, they are those of Aeolus's own section for it on a radio, named as
// the renderer names it (0039): another interface with the same SSID, made
// by hand or for another network, is not it. With no ID, those of the first
// AP interface with the SSID.
function credentials(want, ifaces, radios) {
	let names = null;
	if (want.network)
		names = map(radios ?? [], r => 'aeolus_' + replace(want.network, '-', '_') + '_' + r);
	for (let i in ifaces ?? []) {
		if (i.mode != 'ap' || i.ssid != want.ssid)
			continue;
		if (names && !(i['.name'] in names))
			continue;
		return { encryption: i.encryption ?? 'none', key: i.key, ppsk: i.ppsk, ft: i.ieee80211r == '1' };
	}
	return null;
}

// dfs says whether any of the frequencies a client looks on or joins at is
// a DFS one: 5 GHz channels 52 to 144.
function dfs(freqs) {
	for (let f in freqs ?? [])
		if (f >= 5260 && f <= 5720)
			return true;
	return false;
}

// band_freqs are the frequencies a client looks on for a band, every
// 20 MHz channel of it; for none, those of 2.4 and 5 GHz, where a look at
// all of 6 GHz too would take most of the time a test has.
function band_freqs(band) {
	let out = [];
	if (!band || band == '2g')
		for (let f = 2412; f <= 2472; f += 5)
			push(out, f);
	if (!band || band == '5g') {
		for (let f = 5180; f <= 5320; f += 20)
			push(out, f);
		for (let f = 5500; f <= 5720; f += 20)
			push(out, f);
		for (let f = 5745; f <= 5825; f += 20)
			push(out, f);
	}
	if (band == '6g')
		for (let f = 5955; f <= 7115; f += 20)
			push(out, f);
	return out;
}

// band_of and channel_of say where a frequency is.
function band_of(f) {
	return f >= 2400 && f <= 2500 ? '2g' : f >= 5150 && f <= 5900 ? '5g' : f >= 5925 && f <= 7125 ? '6g' : null;
}

function channel_of(f) {
	if (f == 2484)
		return 14;
	if (f < 2484)
		return int((f - 2407) / 5);
	if (f < 5925)
		return int((f - 5000) / 5);
	return f == 5935 ? 2 : int((f - 5950) / 5);
}

// target says which networks the client may join and where it looks for
// them, from what is asked (wanted's) and what the scan radio last heard
// ([{ bssid, ssid, band, freq, channel, signal, own }]). Asked for an AP's
// own BSSIDs, it keeps to those, and looks where the scan radio heard them
// and where the AP says it is now (0239): a radio that moved since the scan
// radio heard it was looked for where it had been, and not found. Where it
// knows neither, it looks on every channel of the band: a look can miss a
// beacon. Asked for none, it takes the strongest the scan radio heard that
// is not this AP's own. It returns { bssids, freqs, heard }, or null with
// nothing to join.
function target(want, list) {
	let heard = filter(list ?? [], b => type(b) == 'object' && b.ssid == want.ssid && (!want.band || b.band == want.band) &&
		type(b.bssid) == 'string' && (length(want.bssids) ? index(want.bssids, lc(b.bssid)) >= 0 : !b.own));
	heard = sort(heard, (a, b) => (b.signal ?? -200) - (a.signal ?? -200));
	let seen = map(slice(heard, 0, HEARD), b => ({ bssid: lc(b.bssid), band: b.band, channel: b.channel, signal: b.signal }));
	if (length(want.bssids)) {
		let now = map(filter(want.channels ?? [], c => !want.band || c.band == want.band), c => c.freq);
		let freqs = uniq([...map(heard, b => b.freq), ...now]);
		return { bssids: want.bssids, freqs: length(freqs) ? freqs : band_freqs(want.band), heard: seen };
	}
	if (!length(heard))
		return null;
	return { bssids: [lc(heard[0].bssid)], freqs: [heard[0].freq], heard: seen };
}

// roam_targets is where a test roams to once on (0225): for each AP asked,
// in turn, its BSSID with the network's SSID that the scan radio heard
// strongest, on the band asked or, with none, on any; an AP not heard so is
// left out. list is as target's. Each is { bssid, band, freq, signal }.
function roam_targets(want, list) {
	let out = [];
	for (let ap in want.roam ?? []) {
		let heard = filter(list ?? [], b => type(b) == 'object' && b.ssid == want.ssid && (!want.band || b.band == want.band) &&
			type(b.bssid) == 'string' && index(ap, lc(b.bssid)) >= 0);
		heard = sort(heard, (a, b) => (b.signal ?? -200) - (a.signal ?? -200));
		if (length(heard))
			push(out, { bssid: lc(heard[0].bssid), band: heard[0].band, freq: heard[0].freq, signal: heard[0].signal });
	}
	return out;
}

// roam_kind is how a roam came by its keys (0225), as the roaming page
// names kinds: ft, by 802.11r; full, a sign-in again, which an 802.1X
// sign-in or a new key held for the AP says (PMKSA-CACHE-ADDED, as after
// SAE); cached, a key the client held, where the network signs it in by
// SAE or 802.1X; keys, the WPA2 passphrase's four messages. key_mgmt is
// the one wpa_supplicant joined with, as its STATUS says. An 802.11r roam
// also ends "Key negotiation completed", so that line says nothing here.
function roam_kind(full, key_mgmt) {
	if (full)
		return 'full';
	if (match(key_mgmt ?? '', /^FT-/))
		return 'ft';
	if (match(key_mgmt ?? '', /SAE|EAP/))
		return 'cached';
	return 'keys';
}

// supplicant is wpa_supplicant's configuration to join a network as the AP
// itself offers it: cred is { encryption, key, ppsk } from the AP's own
// wireless config, tgt is target's, and eap the user to sign in as, where
// the network signs clients in by 802.1X and the test was given one. It
// returns { conf }, or { why } for a network the test can't join: one that
// signs clients in and no user was given for, or gives each device a
// passphrase of its own, or has none here.
//
// The RADIUS server's certificate is not checked: the test asks whether a
// client is let on, not whom it trusts.
function supplicant(dir, ssid, cred, tgt, eap) {
	if (type(cred) != 'object')
		return { why: 'this AP does not offer that network' };
	let e = cred.encryption ?? 'none';
	let lines = ['ssid=' + hexstr(ssid), 'scan_ssid=1'];
	if (length(tgt?.bssids))
		push(lines, 'bssid_accept=' + join(' ', tgt.bssids));
	if (length(tgt?.freqs))
		push(lines, 'scan_freq=' + join(' ', tgt.freqs), 'freq_list=' + join(' ', tgt.freqs));
	if (cred.ppsk == '1')
		return { why: 'each device has a passphrase of its own from RADIUS there, and the test has none' };
	// A network with 802.11r is joined as a client that does it would
	// (0225): its keys for each AP then come by 802.11r, and its roams use
	// it, where the AP lets them.
	//
	// The network is off as wpa_supplicant starts: the test turns it on
	// once it has said, over the control socket, what to put in its
	// association request (see join_network in aeolus-clienttest).
	let conf = () => ({ conf: `ctrl_interface=${dir}/wpa\nupdate_config=0\nsae_pwe=2\nnetwork={\n\t${join('\n\t', lines)}\n\tdisabled=1\n}\n` });
	if (match(e, /^wpa/) || match(e, /eap/)) {
		if (!eap)
			return { why: 'it signs clients in by 802.1X, and the test was given no user to sign in as' };
		// WPA2-Enterprise, WPA3-Enterprise, or both, as OpenWrt names them.
		let ft = cred.ft ? 'FT-EAP ' : '';
		if (e == 'wpa3')
			push(lines, 'key_mgmt=' + ft + 'WPA-EAP-SHA256', 'ieee80211w=2');
		else if (e == 'wpa3-mixed')
			push(lines, 'key_mgmt=' + ft + 'WPA-EAP WPA-EAP-SHA256', 'ieee80211w=1');
		else if (match(e, /^wpa2/))
			push(lines, 'key_mgmt=' + ft + 'WPA-EAP');
		else
			return { why: 'its security, ' + e + ', is one the test does not know' };
		// The name and the password in hex, as the SSID is: whatever is in
		// them, wpa_supplicant reads them whole.
		push(lines, 'eap=' + (eap.method == 'ttls' ? 'TTLS' : 'PEAP'), 'identity=' + hexstr(eap.identity), 'password=' + hexstr(eap.password),
			'phase2="auth=' + (eap.method == 'ttls' ? 'PAP' : 'MSCHAPV2') + '"');
		return conf();
	}
	// A passphrase goes between quotes. wpa_supplicant reads to the last
	// quote on the line, so one that holds a quote is still read whole.
	let quoted = (k) => type(k) == 'string' && length(k) >= 8 && length(k) <= 63 && index(k, '\n') < 0;
	if (e == 'none')
		push(lines, 'key_mgmt=NONE');
	else if (e == 'owe')
		push(lines, 'key_mgmt=OWE', 'ieee80211w=2');
	else if (!cred.key)
		return { why: 'this AP has no passphrase for it' };
	else if (!quoted(cred.key))
		return { why: 'its passphrase is one the test cannot hand to wpa_supplicant' };
	else if (e == 'sae')
		push(lines, 'key_mgmt=' + (cred.ft ? 'FT-SAE ' : '') + 'SAE', 'ieee80211w=2', 'sae_password="' + cred.key + '"');
	else if (e == 'sae-mixed')
		push(lines, 'key_mgmt=' + (cred.ft ? 'FT-SAE SAE FT-PSK ' : 'SAE ') + 'WPA-PSK', 'ieee80211w=1', 'psk="' + cred.key + '"');
	else if (match(e, /^psk/))
		push(lines, 'key_mgmt=' + (cred.ft ? 'FT-PSK ' : '') + 'WPA-PSK', 'psk="' + cred.key + '"');
	else
		return { why: 'its security, ' + e + ', is one the test does not know' };
	return conf();
}

// event reads one of wpa_supplicant's events as a step of the join, or
// null for the rest: { stage, what, ok } and, where it is one, bssid and
// freq of the network found, connected, notfound, or fail, for one after
// which it does not go on.
function event(line) {
	line = replace(line ?? '', /^<[0-9]+>/, '');
	let m;
	if (match(line, /^CTRL-EVENT-SCAN-STARTED/))
		return { stage: 'find', what: 'looks for the network' };
	if ((m = match(line, /^SME: Trying to authenticate with ([0-9a-f:]{17}) .*freq=([0-9]+)/)))
		return { stage: 'find', what: `found it on ${m[1]}, ${m[2]} MHz`, ok: true, bssid: m[1], freq: int(m[2]) };
	if (match(line, /^Trying to associate with/))
		return { stage: 'auth', what: 'authenticated', ok: true };
	if (match(line, /^Associated with/))
		return { stage: 'assoc', what: 'associated', ok: true };
	// An 802.1X sign-in (0124), between association and the keys.
	if (match(line, /^CTRL-EVENT-EAP-STARTED/))
		return { stage: 'signin', what: 'sign-in started' };
	if ((m = match(line, /^CTRL-EVENT-EAP-METHOD .*\(([A-Za-z0-9-]+)\) selected/)))
		return { stage: 'signin', what: `signs in by ${m[1]}` };
	if (match(line, /^CTRL-EVENT-EAP-SUCCESS/))
		return { stage: 'signin', what: 'signed in', ok: true };
	if (match(line, /^CTRL-EVENT-EAP-FAILURE/))
		return { stage: 'signin', what: 'the sign-in was refused', ok: false, fail: true };
	if (match(line, /^WPA: Key negotiation completed/))
		return { stage: 'key', what: 'key handshake completed', ok: true };
	if (match(line, /^CTRL-EVENT-CONNECTED/))
		return { stage: 'key', what: 'let on to the network', ok: true, connected: true };
	if (match(line, /^CTRL-EVENT-NETWORK-NOT-FOUND/))
		return { stage: 'find', what: 'the network was not found', ok: false, notfound: true };
	if ((m = match(line, /^CTRL-EVENT-AUTH-REJECT .*status_code=([0-9]+)/)))
		return { stage: 'auth', what: `authentication refused (status ${m[1]})`, ok: false, fail: true };
	if ((m = match(line, /^CTRL-EVENT-ASSOC-REJECT .*status_code=([0-9]+)/)))
		return { stage: 'assoc', what: `association refused (status ${m[1]})`, ok: false, fail: true };
	// wpa_supplicant cannot tell a wrong passphrase from the handshake's
	// messages lost on a weak link, and says the key "may be incorrect":
	// so does the test.
	if (match(line, /^WPA: 4-Way Handshake failed/))
		return { stage: 'key', what: 'the key handshake failed: a wrong passphrase, or its messages lost', ok: false };
	if ((m = match(line, /^CTRL-EVENT-SSID-TEMP-DISABLED .*reason=([A-Z_]+)/)))
		return m[1] == 'WRONG_KEY' ? { stage: 'key', what: 'the key handshake failed each time: a wrong passphrase, or a link too weak for it', ok: false, fail: true }
			: { stage: null, what: 'gave up: ' + lc(replace(m[1], /_/g, ' ')), ok: false, fail: true };
	if ((m = match(line, /^CTRL-EVENT-DISCONNECTED .*reason=([0-9]+)/)))
		return { stage: null, what: `disconnected (reason ${m[1]})`, ok: false };
	return null;
}

// link_of reads what `iw dev <interface> station dump` says of a client's
// link: its signal, in dBm, and the rate each way with how it is sent.
function link_of(text) {
	let way = (dir) => {
		let m = match(text ?? '', regexp(dir + ' bitrate:[ \t]+([0-9.]+) MBit/s([^\n]*)'));
		return m ? { mbps: +m[1], how: trim(m[2]) } : null;
	};
	let sig = match(text ?? '', /signal:[ \t]+(-?[0-9]+)/)?.[1];
	return { signal: sig != null ? int(sig) : null, tx: way('tx'), rx: way('rx') };
}

// ping_of reads busybox ping's summary: how many it sent and got back, and
// the least, mean and most time, in ms.
function ping_of(text) {
	let m = match(text ?? '', /([0-9]+) packets transmitted, ([0-9]+) (packets )?received/);
	let r = match(text ?? '', /= ([0-9.]+)\/([0-9.]+)\/([0-9.]+) ms/);
	return { sent: m ? int(m[1]) : 0, got: m ? int(m[2]) : 0, min: r ? +r[1] : null, avg: r ? +r[2] : null, max: r ? +r[3] : null };
}

// neigh_of reads what `ip neigh show <address> dev <interface>` says of an
// address: its MAC, where it answered ARP, or null where it did not, or
// has not yet.
function neigh_of(text) {
	let m = match(text ?? '', /lladdr ([0-9a-f]{2}(:[0-9a-f]{2}){5}) .*(REACHABLE|STALE|DELAY|PROBE|PERMANENT)/);
	return m ? m[1] : null;
}

// lease_of reads the lease udhcpc's script wrote, key=value a line: the
// address and prefix length, the first router, the DNS servers, the DHCP
// server and the lease time; or null without an address.
function lease_of(text) {
	let v = {};
	for (let line in split(text ?? '', '\n')) {
		let m = match(line, /^([a-z]+)=(.*)$/);
		if (m)
			v[m[1]] = trim(m[2]);
	}
	let ip4 = (s) => type(s) == 'string' && length(iptoarr(s) ?? []) == 4;
	let bits = int(v.mask ?? '');
	if (!ip4(v.ip) || !(bits >= 0 && bits <= 32))
		return null;
	return {
		address: v.ip, mask: bits, router: filter(split(v.router ?? '', ' '), ip4)[0] ?? null,
		dns: slice(filter(split(v.dns ?? '', ' '), ip4), 0, 3), server: ip4(v.server) ? v.server : null, lease: int(v.lease ?? '0') || 0,
	};
}

// dhcp_said reads what udhcpc said as it asked for an address (0189):
// how many times it asked to have one it was offered, the latest it asked
// for and from which server, and whether a server refused it; enough to
// say where asking stopped where no address came. It says its first three
// discovers alone, so they are not counted.
function dhcp_said(text) {
	let out = { requests: 0, offered: null, server: null, refused: false };
	for (let line in split(text ?? '', '\n')) {
		let m = match(line, /select for ([0-9.]+), server ([0-9.]+)/);
		if (m) {
			out.requests++;
			out.offered = m[1];
			out.server = m[2];
		}
		else if (match(line, /received DHCP NAK/))
			out.refused = true;
	}
	return out;
}

// dhcp_failure is what a test that was given no address says of it (0189),
// from what udhcpc said, as the test over the wire says it (0128): the step
// and why. waited is how long it asked, in seconds.
function dhcp_failure(said, waited) {
	if (said?.offered && said.refused)
		return { what: `offered ${said.offered} by ${said.server}, then refused it`, why: `the DHCP server refused it ${said.offered}, which it had offered` };
	if (said?.offered)
		return {
			what: `offered ${said.offered} by ${said.server}, and ${said.requests == 1 ? 'its request was not' : 'none of its ' + said.requests + ' requests was'} acknowledged in ${waited} s`,
			why: `a DHCP server offered ${said.offered} and never confirmed it`,
		};
	return { what: `no answer to its DHCP requests in ${waited} s`, why: 'no DHCP server gave it an address' };
}

// prefix_net is the network an address is in, for a prefix length.
function prefix_net(ip, bits) {
	let a = iptoarr(ip);
	if (length(a ?? []) != 4 || !(bits >= 0 && bits <= 32))
		return null;
	let out = [];
	for (let i = 0; i < 4; i++) {
		let keep = min(8, max(0, bits - 8 * i));
		push(out, a[i] & (0xff00 >> keep) & 0xff);
	}
	return arrtoip(out);
}

// dns_query is a question for a name's IPv4 address.
function dns_query(id, name) {
	let q = chr(id >> 8, id & 255, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0);
	for (let label in split(name, '.'))
		if (length(label))
			q += chr(length(label)) + label;
	return q + chr(0, 0, 1, 0, 1);
}

// dns_answer reads the answer to it: its code, how many records it has,
// and the first IPv4 address among them; null for what is no answer to it.
function dns_answer(d, id) {
	if (length(d ?? '') < 12 || ((ord(d, 0) << 8) | ord(d, 1)) != id || !(ord(d, 2) & 0x80))
		return null;
	let out = { rcode: ord(d, 3) & 15, answers: (ord(d, 6) << 8) | ord(d, 7), address: null };
	let skip = (p) => {
		while (p < length(d)) {
			let n = ord(d, p);
			if (n == 0)
				return p + 1;
			if ((n & 0xc0) == 0xc0)
				return p + 2;
			p += n + 1;
		}
		return p;
	};
	let p = 12;
	for (let i = 0; i < ((ord(d, 4) << 8) | ord(d, 5)); i++)
		p = skip(p) + 4;
	for (let i = 0; i < out.answers && p + 10 <= length(d); i++) {
		p = skip(p);
		if (p + 10 > length(d))
			break;
		let kind = (ord(d, p) << 8) | ord(d, p + 1), len = (ord(d, p + 8) << 8) | ord(d, p + 9);
		if (kind == 1 && len == 4 && p + 14 <= length(d) && !out.address)
			out.address = arrtoip([ord(d, p + 10), ord(d, p + 11), ord(d, p + 12), ord(d, p + 13)]);
		p += 10 + len;
	}
	return out;
}

// rate is megabits a second, to a tenth, for bytes moved in seconds.
function rate(bytes, seconds) {
	return seconds > 0 ? int(bytes * 8 / seconds / 1e5 + 0.5) / 10.0 : null;
}

// plain is text as the manager takes it: printable ASCII, anything else a
// ?, and no longer than most.
function plain(s, most) {
	let out = '';
	for (let i = 0; i < length(s ?? '') && i < most; i++) {
		let c = ord(s, i);
		out += c >= 32 && c < 127 ? chr(c) : '?';
	}
	return out;
}

// verdict says how a test went, from what it found (res): passed, or
// failed with the stage it stopped at and why. A test that joined but then
// found a step that does not work has failed at that step, and so has one
// whose lease named no gateway or no DNS server: a client has no way online
// without them, and nothing was there to try.
// way_out judges the way out to the internet (0199) from what was measured:
// largest, the largest packet, in bytes of IP, sent without DF that the
// target answered, 1500 where a full-size one came back; and tcp, the
// largest packet a TCP connection made here sends, where one was made.
// Many IoT clients send without DF, so a packet of theirs larger than the
// way carries is lost, where one that sets DF is told to send smaller.
// What TCP sends is what counts: a router that clamps TCP's MSS keeps it
// to what gets through. The way out is lost where TCP sends more.
function way_out(target, largest, tcp) {
	let out = { target: target, largest: largest };
	if (tcp != null)
		out.tcp = tcp;
	if (largest >= 1500)
		out.what = `full-size packets, 1500 bytes, reach ${target}`;
	else if (tcp == null)
		out.what = `packets over ${largest} bytes sent without DF are lost on the way to ${target}; TCP could not be measured`;
	else if (tcp <= largest)
		out.what = `packets over ${largest} bytes sent without DF are lost on the way to ${target}; TCP here sends ${tcp}, so it gets through`;
	else {
		out.lost = true;
		out.what = `packets over ${largest} bytes sent without DF are lost on the way to ${target}, and TCP here sends ${tcp}`;
		out.why = `packets over ${largest} bytes without DF are lost on the way out, and TCP here sends ${tcp}, so IoT clients that send without DF stall: clamp TCP's MSS to ${largest - 40} or less at the internet router`;
	}
	return out;
}

function verdict(res) {
	let fail = (stage, why) => {
		res.outcome = 'failed';
		res.stage = stage in STAGES ? stage : 'find';
		res.reason = plain(why, 200);
		return res;
	};
	if (res.stopped)
		return fail(res.stopped.stage, res.stopped.why);
	if (res.dhcp && !res.gateway)
		return fail('gateway', 'DHCP gave it no gateway');
	// A gateway that answers ARP is there, though it answer no ping: many
	// are set to answer none.
	if (res.gateway && !res.gateway.got && !res.gateway.mac)
		return fail('gateway', `its gateway, ${res.gateway.address}, does not answer`);
	if (res.dhcp && !res.dns)
		return fail('dns', 'DHCP gave it no DNS server');
	if (res.dns && !(res.dns.rcode == 0 && res.dns.answers > 0))
		return fail('dns', res.dns.rcode == null ? `its DNS server, ${res.dns.server}, does not answer` : `its DNS server, ${res.dns.server}, has no address for ${res.dns.name}`);
	// A speed server the network cannot find or reach fails no test (0244):
	// it is the site's measuring tool, and a network kept apart from it, as
	// a tunnelled one with public DNS is, still takes its clients online.
	// Its error is said beside the result.
	if (res.path?.lost)
		return fail('path', res.path.why);
	res.outcome = 'passed';
	res.stage = 'done';
	res.reason = '';
	return res;
}

// summary is a test's result in a line, for the list of what the AP was
// asked to do: printable ASCII, at most 300 characters.
function summary(res) {
	let secs = (ms) => sprintf('%.1f s', (ms ?? 0) / 1000.0);
	let s;
	if (res.outcome != 'passed')
		s = `failed at ${res.stage}: ${res.reason}`;
	else {
		let parts = [res.band == 'wire' ? 'over the wire' : `joined in ${secs(res.joined_ms)}`];
		if (res.dhcp)
			push(parts, `address in ${secs(res.dhcp.ms)}`);
		if (res.gateway?.avg != null)
			push(parts, sprintf('gateway %.0f ms', res.gateway.avg));
		else if (res.gateway?.arp_ms != null)
			push(parts, sprintf('gateway answers ARP in %.1f ms', res.gateway.arp_ms));
		else if (res.gateway?.mac)
			push(parts, 'gateway there, no ping');
		if (res.dns?.ms != null)
			push(parts, `DNS ${res.dns.ms} ms`);
		if (res.speed?.down != null)
			push(parts, `${res.speed.down} down / ${res.speed.up ?? '?'} up Mbit/s`);
		else if (res.speed?.error)
			push(parts, `no speed to ${res.speed.server}: ${res.speed.error}`);
		if (res.path?.largest >= 1500)
			push(parts, 'full-size packets out');
		else if (res.path?.largest)
			push(parts, `packets out to ${res.path.largest} bytes`);
		// The remote server's (0134), or why it has none: that fails no
		// test, as the server is not the site's own.
		if (res.speed_remote?.down != null)
			push(parts, `${res.speed_remote.down} down / ${res.speed_remote.up ?? '?'} up Mbit/s to ${res.speed_remote.server}`);
		else if (res.speed_remote?.error)
			push(parts, `no speed to ${res.speed_remote.server}: ${res.speed_remote.error}`);
		// Its roams (0225): how many went, and each kind's count and
		// typical time, as kinds take such different times.
		if (length(res.roams)) {
			let ok = filter(res.roams, r => r.ok), kinds = {};
			for (let r in ok)
				push(kinds[r.kind] ??= [], r.ms);
			let how = map(sort(keys(kinds)), k => {
				let t = sort(kinds[k], (a, b) => a - b);
				return `${k == 'ft' ? '802.11r' : k} ${length(t)}x ${t[int((length(t) - 1) / 2)]} ms`;
			});
			push(parts, `roamed ${length(ok)} of ${length(res.roams)} times` + (length(how) ? ` (${join(', ', how)})` : ''));
		}
		s = 'passed: ' + join(', ', parts);
	}
	return plain(s, 300);
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export {
	STAGES, STEPS, HEARD, ROAMS, SPEED_BODY, OPENSPEEDTEST, between, hexstr, wanted, credentials, dfs, band_freqs, freq_of, band_of, channel_of, target, roam_targets, roam_kind, supplicant, event, link_of, ping_of, lease_of, neigh_of,
	dhcp_said, dhcp_failure, prefix_net, dns_query, dns_answer, rate, plain, way_out, verdict, summary
};
