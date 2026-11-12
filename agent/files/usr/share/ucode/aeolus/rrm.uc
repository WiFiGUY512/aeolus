// Radio resource management's pure parts (0073), so they are tested
// anywhere: the element an AP's beacons carry to say it is an Aeolus AP and
// where to reach it; HMAC-SHA256, which signs the hellos neighbours
// exchange; the hellos themselves; which APs become neighbours; the
// channels a radio listens on for them; how each channel is rated; and
// when and where a radio moves.

'use strict';

import { sha256 } from 'digest';

// The element's OUI, from the locally assigned range, which no vendor's OUI
// or CID is in, so it is never taken for another's.
const OUI = '\x02\xae\x01';
const ADVERT = 1;             // the element's type: an AP's advert
const VERSION = 1;            // of the advert and the hellos
const PORT = 16730;           // where neighbours send hellos, on their management addresses
const NEIGHBOURS = 3;         // on each band, the APs heard most strongly
const HELLO_EVERY = 10;       // seconds between hellos
const DEAD = 40;              // seconds without a hello after which a neighbour is down
const SKEW = 60;              // seconds a hello's time may be off from this AP's clock
const LASTING = 0.1;          // how much one visit moves a channel's lasting rating
const NOW = 0.5;              // and its rating now
const SUDDEN = 0.5;           // a radio's own channel kept busier than this by others is interference
const NEAR = 6;               // dB further away a shared channel's nearest user must be, to move to it
const WINDOW = '02:00-05:00'; // when planned moves may happen, unless the policy says
const MARGIN = 20;            // rating points a channel must beat the radio's own by, unless the policy says
// MHz an AP keeps its own 5 GHz and 6 GHz channels apart, unless the policy
// says (0327): on the Living Room C-360, 6 GHz on ch5 at 160 MHz (5945-6105),
// 130 MHz over 5 GHz ch157 at 40 (5775-5815), cut 5 GHz from HE-MCS 6 to
// MCS 0; on ch37 (6105-6265), 290 MHz over, it was as before.
const APART = 250;
const APC_STEP = 3;           // dB a radio's power moves at a time (0077)
const APC_FLOOR = 8;          // dBm a radio's power goes no lower than
const APC_ABOVE = 6;          // dB above the target all must hear it before its power goes down
const CLIENT_POWER = 15;      // dBm a client is taken to send at, to tell what it hears of its AP (0206)
const LONELY_BY = 3;          // dB better another AP must hear a client for it not to be lonely
const SETTLED = 1800;         // seconds a client is connected before it counts: one that passes through does not
const HEARS_MAX = 32;         // clients heard by probe that a hello says, the strongest
const HELLO_MAX = 1400;       // bytes a hello may be, signed: one Ethernet frame, with IP and UDP (0206)
const LOAD_FRESH = 180;       // seconds after its minute's end a radio's load is still said in hellos (0207)
const CAC_MOST = 60;          // seconds of listening for radar a move to a DFS channel may cost (0120)
const DFS_LOOKS = 2;          // looks at each channel of a DFS block before a radio may go there
const BARRED_FOR = 86400;     // seconds a DFS block is left alone once a radio left it, or did not get there
const BACK_FOR = 86400;       // seconds before a planned move may go back to a block the radio left (0264)

// The 5 GHz channels taken up together at each width, by the lowest and
// highest 20 MHz channel, as internal/radio has them: the upper ones,
// 165–177, among them (0232).
const BLOCKS5 = {
	'40': [[36, 40], [44, 48], [52, 56], [60, 64], [100, 104], [108, 112], [116, 120], [124, 128], [132, 136], [140, 144], [149, 153], [157, 161], [165, 169], [173, 177]],
	'80': [[36, 48], [52, 64], [100, 112], [116, 128], [132, 144], [149, 161], [165, 177]],
	'160': [[36, 64], [100, 128]],
};

// blocks6 is the 6 GHz channels taken up together at a width, by the lowest
// and highest 20 MHz channel (0278): the first block starts at channel 1,
// and each next one a block's width on, to 233. 320 MHz, whose blocks
// overlap, has none here.
function blocks6(width) {
	let n = int((width ?? 20) / 20), out = [];
	if (!(n in [2, 4, 8]))
		return out;
	for (let lo = 1; lo + 4 * (n - 1) <= 233; lo += 4 * n)
		push(out, [lo, lo + 4 * (n - 1)]);
	return out;
}

// psc says whether a 6 GHz channel is a preferred scanning channel: 5, 21,
// 37 and every 16th to 229 (0087), one in each 80 MHz block.
function psc(c) {
	return c >= 5 && c <= 229 && (c - 5) % 16 == 0;
}

// block_of is the block of 20 MHz channels a radio on channel takes up at
// width, [lowest, highest]: on 5 and 6 GHz at 40 MHz or more; else null.
function block_of(band, channel, width) {
	let all = !width || width <= 20 ? null : band == '5g' ? BLOCKS5['' + width] : band == '6g' ? blocks6(width) : null;
	for (let b in all ?? [])
		if (channel >= b[0] && channel <= b[1])
			return b;
	return null;
}

function hexstr(s) {
	let o = '';
	for (let i = 0; i < length(s); i++)
		o += sprintf('%02x', ord(s, i));
	return o;
}

// unhex reads hex digits as bytes, or returns null.
function unhex(h) {
	if (type(h) != 'string' || length(h) % 2 || !match(h, /^[0-9a-fA-F]*$/))
		return null;
	let o = '';
	for (let i = 0; i < length(h); i += 2)
		o += chr(hex(substr(h, i, 2)));
	return o;
}

// hmac is HMAC-SHA256 (RFC 2104) of msg under key, both byte strings, in
// hex.
function hmac(key, msg) {
	if (length(key) > 64)
		key = unhex(sha256(key));
	let ipad = '', opad = '';
	for (let i = 0; i < 64; i++) {
		let b = i < length(key) ? ord(key, i) : 0;
		ipad += chr(b ^ 0x36);
		opad += chr(b ^ 0x5c);
	}
	return sha256(opad + unhex(sha256(ipad + msg)));
}

// same compares two strings in a time that doesn't depend on where they
// differ, as a signature must be.
function same(a, b) {
	if (type(a) != 'string' || type(b) != 'string' || length(a) != length(b))
		return false;
	let d = 0;
	for (let i = 0; i < length(a); i++)
		d |= ord(a, i) ^ ord(b, i);
	return d == 0;
}

// advert is the vendor element, in hex as hostapd's vendor_elements takes
// it, that says this is Aeolus AP ap (ap-<12 hex digits>), reached at its
// IPv4 management address on port. Null if any of them is not one.
function advert(ap, address, port) {
	let id = substr(ap ?? '', 0, 3) == 'ap-' ? unhex(substr(ap, 3)) : null;
	let ip = match(address ?? '', /^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$/);
	if (id == null || length(id) != 6 || !ip || +ip[1] > 255 || +ip[2] > 255 || +ip[3] > 255 || +ip[4] > 255 ||
	    type(port) != 'int' || port < 1 || port > 65535)
		return null;
	let data = OUI + chr(ADVERT, VERSION) + id + chr(+ip[1], +ip[2], +ip[3], +ip[4], port >> 8, port & 0xff);
	return 'dd' + sprintf('%02x', length(data)) + hexstr(data);
}

// read_advert reads an element's body, as nl80211 gives it (its OUI first),
// as another AP's advert: { ap, address, port }, or null if it is not one.
// A later version may add to its end.
function read_advert(data) {
	if (type(data) != 'string' || length(data) < 17 || substr(data, 0, 3) != OUI || ord(data, 3) != ADVERT || ord(data, 4) < VERSION)
		return null;
	return {
		ap: 'ap-' + hexstr(substr(data, 5, 6)),
		address: sprintf('%d.%d.%d.%d', ord(data, 11), ord(data, 12), ord(data, 13), ord(data, 14)),
		port: ord(data, 15) << 8 | ord(data, 16),
	};
}

// BEACONS is the kernel filter for the scan radio's monitor interface
// (0161): of the frames it hears, beacons only, and of each the first 1024
// bytes, where an advert is. Each frame comes after a radiotap header as
// long as that header's third and fourth bytes say, the low byte first.
const BEACONS = [
	[0x30, 0, 0, 3],      // A = the radiotap header's length, its high byte
	[0x64, 0, 0, 8],      // A <<= 8
	[0x07, 0, 0, 0],      // X = A
	[0x30, 0, 0, 2],      // A = its low byte
	[0x0c, 0, 0, 0],      // A += X
	[0x07, 0, 0, 0],      // X = A: where the 802.11 frame begins
	[0x50, 0, 0, 0],      // A = the frame's first byte
	[0x15, 0, 1, 0x80],   // a beacon: keep
	[0x06, 0, 0, 1024],
	[0x06, 0, 0, 0],
];

function le32(s, i) {
	return ord(s, i) | ord(s, i + 1) << 8 | ord(s, i + 2) << 16 | ord(s, i + 3) << 24;
}

// radiotap_flags reads the Flags field of the radiotap header a frame on a
// monitor interface comes after, or null where the header has none or is
// cut short. The header's fields come after its present words, in the
// order of their bits, each aligned to its own size from the header's
// start; a present word with bit 31 set has another after it. Flags (bit 1)
// comes after TSFT (bit 0), eight bytes on an eight: on the C-360's scan
// radio, 16 bytes in (2026-10-16).
function radiotap_flags(f) {
	if (type(f) != 'string' || length(f) < 8)
		return null;
	let len = ord(f, 2) | ord(f, 3) << 8, present = le32(f, 4), at = 4;
	if (len < 8 || len > length(f))
		return null;
	while (le32(f, at) & 0x80000000) {
		at += 4;
		if (at + 4 > len)
			return null;
	}
	at += 4;
	if (present & 1)
		at = ((at + 7) & ~7) + 8;
	return present & 2 && at < len ? ord(f, at) : null;
}

// beacon_advert reads a beacon as the scan radio's monitor interface hands
// it (0161), a radiotap header before it: { bssid, advert } where it
// carries an Aeolus advert, or null. A beacon's elements begin after its
// header and its fixed fields, 36 bytes in.
//
// A frame that failed its FCS check (radiotap's Flags 0x40) is passed over:
// an advert read from one can name an AP, or an address, that is not
// there, as ap-2005b60fe0f7 at 60.21.234.228 for ap-2005b6df64e0 at
// 192.168.1.69. The C-360's scan radio handed up one in ten to eighteen
// (2026-10-16) until port 0054; the C-260's ath10k hands up none.
function beacon_advert(f) {
	if (type(f) != 'string' || length(f) < 8 || (radiotap_flags(f) ?? 0) & 0x40)
		return null;
	let w = substr(f, ord(f, 2) | (ord(f, 3) << 8));
	if (length(w) < 36 || ord(w, 0) != 0x80)
		return null;
	for (let i = 36; i + 2 <= length(w);) {
		let id = ord(w, i), l = ord(w, i + 1);
		if (i + 2 + l > length(w))
			break;
		if (id == 221) {
			let a = read_advert(substr(w, i + 2, l));
			if (a)
				return { bssid: join(':', map([0, 1, 2, 3, 4, 5], k => sprintf('%02x', ord(w, 16 + k)))), advert: a };
		}
		i += 2 + l;
	}
	return null;
}

// seal signs a message for neighbours: the HMAC of its JSON, then the JSON.
function seal(key, msg) {
	let body = sprintf('%J', msg);
	return hmac(key, body) + body;
}

// load_said is what a hello says of a radio's load (0207): of the band, from
// what the AP's last minute came to (aeolus.minutes' load), while it is
// fresh, at, the time now: how busy its channel was, in percent, its
// clients, and the share of what was sent to them that was tried again or
// lost. null where none is known.
function load_said(l, band, at) {
	let r = l?.radios?.[band];
	if (!r || type(l.at) != 'int' || at - (l.at + 60) > LOAD_FRESH)
		return null;
	return { busy: r.busy, clients: r.clients, retried: r.retried };
}

// load_heard is what is taken of a neighbour's hello of a radio's load
// (0207): each number where it is one and in its range, or null for
// nothing to take.
function load_heard(x) {
	if (type(x) != 'object')
		return null;
	let n = (v, lo, hi) => type(v) in ['int', 'double'] && v >= lo && v <= hi ? v : null;
	let out = { busy: n(x.busy, 0, 100), clients: n(x.clients, 0, 1024), retried: n(x.retried, 0, 1) };
	return out.busy == null && out.clients == null && out.retried == null ? null : out;
}

// fit trims a hello to most bytes, HELLO_MAX unless said, as seal makes it
// (0206): first the clients it hears by probe, the faintest first, then the
// APs it hears, likewise. Both lists come strongest first. A hello sent
// whole in one frame is never cut short on the way, and every neighbour
// reads it whole. It returns msg.
function fit(msg, most) {
	most ??= HELLO_MAX;
	let size = () => 64 + length(sprintf('%J', msg));
	for (let list in ['hears', 'heard'])
		while (size() > most && length(msg[list] ?? []) > 0) {
			let l = msg[list], each = length(sprintf('%J', l[length(l) - 1])) + 2;
			msg[list] = slice(l, 0, max(0, length(l) - max(1, int((size() - most + each - 1) / each))));
		}
	return msg;
}

// open checks a packet's signature and reads it: a message of this version
// from an AP, or null.
function open(key, pkt) {
	if (type(pkt) != 'string' || length(pkt) < 66 || !same(hmac(key, substr(pkt, 64)), substr(pkt, 0, 64)))
		return null;
	let m = null;
	try {
		m = json(substr(pkt, 64));
	} catch (e) {
		return null;
	}
	return type(m) == 'object' && m.v == VERSION && type(m.ap) == 'string' && match(m.ap, /^ap-[0-9a-f]{12}$/) ? m : null;
}

// fresh says whether a hello is new: its time near this AP's clock, and
// after the last one from the same AP, so one sent again is refused.
function fresh(last, msg, wall) {
	if (type(msg?.at) != 'int' || type(msg.seq) != 'int' || msg.at < wall - SKEW || msg.at > wall + SKEW)
		return false;
	return !last || msg.at > last.at || (msg.at == last.at && msg.seq > last.seq);
}

// choose picks each band's neighbours: the n APs heard most strongly there,
// by name where they are heard alike. heard is { <ap>: { <band>: signal } }.
function choose(heard, n) {
	let by = {};
	for (let ap, bands in heard)
		for (let band, signal in bands)
			push(by[band] ??= [], [ap, signal]);
	let out = {};
	for (let band, list in by)
		out[band] = map(slice(sort(list, (a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1)), 0, n), x => x[0]);
	return out;
}

// smooth folds a new signal reading into the running one: a few readings
// move it, one odd one barely does.
function smooth(old, sample) {
	return old == null ? sample : old * 0.75 + sample * 0.25;
}

function freq(band, ch) {
	if (band == '2g')
		return ch == 14 ? 2484 : 2407 + 5 * ch;
	if (band == '5g')
		return 5000 + 5 * ch;
	return band == '6g' ? 5950 + 5 * ch : 0;
}

function band_of(f) {
	if (f >= 2412 && f <= 2484)
		return '2g';
	if (f >= 5160 && f <= 5885)
		return '5g';
	return f >= 5955 && f <= 7115 ? '6g' : null;
}

// visits are the frequencies a radio listens on for other APs: on 2.4 GHz,
// the channels automatic channels go to, its set (0075) or else 1, 6 and 11
// (0045); on 5 GHz, every 20 MHz channel, those shared with radar (DFS) only
// where the radio may use them (0071), and U-NII-4, 169–177, only where its
// list has them, as without channel compatibility (0232); on 6 GHz, of its
// set (0087), each block's primary at width (0243) and the preferred
// scanning channels, so a block wider than 80 MHz is heard on each 80 MHz
// of it (0298), or else the preferred scanning channels, 5 to 229 every
// 16th (0271); and its own channel, wherever it is.
function visits(band, own, dfs, set, width) {
	let ch = [];
	if (band == '2g')
		ch = length(set ?? []) ? sort([...set], (a, b) => a - b) : [1, 6, 11];
	else if (band == '6g')
		ch = length(set ?? []) ? sort(filter(set, c => {
			let b = block_of(band, c, width);
			return !b || psc(c) || c == b[0] + 4;
		}), (a, b) => a - b) : map([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14], i => 5 + 16 * i);
	else if (band == '5g')
		ch = [36, 40, 44, 48, ...(dfs ? [52, 56, 60, 64, 100, 104, 108, 112, 116, 120, 124, 128, 132, 136, 140, 144] : []), 149, 153, 157, 161, 165,
			...filter([169, 173, 177], c => index(set ?? [], c) >= 0)];
	let out = map(ch, c => freq(band, c));
	let mine = freq(band, own);
	if (own && index(out, mine) < 0)
		push(out, mine);
	return out;
}

// printable is an SSID as the manager takes it: printable ASCII, anything
// else a ?, at most 32 bytes; null for a hidden network, whose SSID is
// empty or all zero bytes. An SSID of question marks is one, not hidden.
function printable(d) {
	let s = '', hidden = true;
	for (let i = 0; i < length(d ?? '') && i < 32; i++) {
		let c = ord(d, i);
		if (c != 0)
			hidden = false;
		s += c >= 32 && c < 127 ? chr(c) : '?';
	}
	return hidden ? null : s;
}

// swept sorts what a scan radio heard in one look (0116): list is airscan's
// networks in the air, each { bssid, ssid, band, channel, signal, own,
// hidden }. theirs maps the last five bytes of the BSSID of each
// neighbour's radio, as its hellos give it, to { ap, band }: a radio makes
// its networks' BSSIDs from one MAC, changing only the first byte. It
// returns
//
//	aps       '<ap> <band>' -> the strongest signal one of that radio's
//	          networks was heard at
//	tails     the BSSID tails of the neighbours' radios heard
//	others    [{ bssid, ssid, band, channel, signal }]: the networks that are
//	          neither this AP's own nor a neighbour's, and have a name
//	networks  '<band> <channel>' -> the signals of those, and of the
//	          nameless ones, for the channels' ratings
//
// A radio beside the scan radio is heard louder than a signal is said: 0
// dBm at most. What is not well formed is left out.
function swept(list, theirs) {
	let out = { aps: {}, tails: [], others: [], networks: {} };
	let tails = {};
	for (let e in (type(list) == 'array' ? list : [])) {
		if (type(e) != 'object' || e.own || type(e.bssid) != 'string')
			continue;
		let bssid = lc(e.bssid);
		if (!match(bssid, /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) || !(e.band in ['2g', '5g', '6g']) ||
		    type(e.channel) != 'int' || e.channel < 1 || e.channel > 233 ||
		    !(type(e.signal) in ['int', 'double']) || e.signal < -127)
			continue;
		let signal = min(0, e.signal);
		let tail = substr(bssid, 3), t = theirs?.[tail];
		// A radio known by its advert, heard on the scan radio, has no band
		// said: its networks' band is airscan's (0161).
		if (t && (t.band == null || t.band == e.band)) {
			let k = t.ap + ' ' + e.band;
			out.aps[k] = max(out.aps[k] ?? -200, signal);
			tails[tail] = true;
			continue;
		}
		push(out.networks[e.band + ' ' + e.channel] ??= [], signal);
		let ssid = e.hidden || type(e.ssid) != 'string' ? null : printable(e.ssid);
		if (ssid)
			push(out.others, { bssid: bssid, ssid: ssid, band: e.band, channel: e.channel, signal: signal });
	}
	out.tails = sort(keys(tails));
	return out;
}

// around is the signals of the networks that count against a channel, from
// swept's networks: those on it and, on 2.4 GHz, those up to three channels
// away, 6 dB weaker for each channel between.
function around(networks, band, channel) {
	let out = [], reach = band == '2g' ? 3 : 0;
	for (let d = -reach; d <= reach; d++)
		for (let s in networks?.[band + ' ' + (channel + d)] ?? [])
			push(out, s - 6 * (d < 0 ? -d : d));
	return out;
}

// weight is how much another network counts against the channel it is
// heard on: fully at -55 dBm or stronger, not at all at -95 or weaker.
// (ucode divides whole numbers to a whole number, so 40.0.)
function weight(signal) {
	if (!(type(signal) in ['int', 'double']))
		return 0;
	return min(1, max(0, (signal + 95) / 40.0));
}

// windows_of is the scan radio's last spectral sweep of a band, a 20 MHz
// window a channel: [channel, window] each, of a sweep no older than within
// seconds at now; none of one older, or from ahead.
function windows_of(sp, band, now, within) {
	let out = [];
	if (type(sp?.time) != 'int' || type(now) != 'int' || now - sp.time > within || sp.time - now > 60)
		return out;
	let base = { '2g': 2407, '5g': 5000, '6g': 5950 }[band];
	if (base == null)
		return out;
	for (let w in sp.windows ?? []) {
		if (w?.width != 20 || type(w.center) != 'int' || (w.band != null && w.band != band))
			continue;
		let ch = band == '2g' && w.center == 2484 ? 14 : (w.center - base) / 5;
		if (type(ch) == 'int' && ch >= 1 && ch <= 233)
			push(out, [ch, w]);
	}
	return out;
}

// energy_of is, by 20 MHz channel, the share of the scan radio's last
// spectral sweep that held energy at or above the preamble level, 0 to 1
// (0245): airscan's busy_pct, a 20 MHz window of the band at a time. It
// counts what keeps a channel busy that is no network, as an outdoor link
// or a video sender does, which a survey's busy time misses: near the
// Living Room AP's attic, 15 to 73% on 112-161, where the survey said 0 to
// 13%. A sweep older than within seconds at now, or none, gives {}.
function energy_of(sp, band, now, within) {
	let out = {};
	for (let cw in windows_of(sp, band, now, within)) {
		let p = cw[1].busy_pct;
		if (type(p) in ['int', 'double'] && p >= 0 && p <= 100)
			out['' + cw[0]] = p / 100.0;
	}
	return out;
}

// floor_of is, by 20 MHz channel, the noise level of the scan radio's last
// spectral sweep, the middle of each window's samples' (0247). It is on the
// scan radio's own scale, uncalibrated: set against the noise floor a radio
// measures on its own channel, it is each channel's floor. The scan radio's
// survey gives none off its channel (port patch 0035).
function floor_of(sp, band, now, within) {
	let out = {};
	for (let cw in windows_of(sp, band, now, within)) {
		let n = cw[1].noise;
		if (type(n) == 'int' && n >= -127 && n <= -20)
			out['' + cw[0]] = n;
	}
	return out;
}

// cost rates one visit to a channel: lower is better. busy is the share of
// the visit others kept it busy, 0 to 1; noise its noise floor in dBm, where
// the driver says; networks the signals of the other networks heard there.
// Busy counts most: a channel others use all the time costs 100. Each dB of
// noise above -95 adds 2, and each strong network 10.
function cost(busy, noise, networks) {
	let c = 100 * min(1, max(0, busy ?? 0));
	if (type(noise) in ['int', 'double'])
		c += 2 * max(0, noise + 95);
	for (let s in networks ?? [])
		c += 10 * weight(s);
	return c;
}

// blend folds a visit's cost into a rating at rate a: LASTING for the rating
// a channel earns over many visits, NOW for what it is like now.
function blend(old, sample, a) {
	return old == null ? sample : old * (1 - a) + sample * a;
}

// covers lists the 20 MHz channels a radio on channel takes up at width: on
// 5 and 6 GHz at 40 MHz or more, its whole block, so a neighbour's wide
// channel blots out every channel it is on (0298).
function covers(band, channel, width) {
	let b = block_of(band, channel, width);
	if (!b)
		return [channel];
	let out = [];
	for (let c = b[0]; c <= b[1]; c += 4)
		push(out, c);
	return out;
}

// allowed_of is the channels a radio at width may use, from the set the
// renderer writes (0075): each whole block's primary (0243), which stands
// for every channel of its block, as covers has them, on 6 GHz too (0298).
// Sorted, and null for no set.
function allowed_of(band, width, chans) {
	let out = [];
	for (let c in chans ?? [])
		for (let m in covers(band, int(c), width))
			if (m > 0 && index(out, m) < 0)
				push(out, m);
	return length(out) ? sort(out, (a, b) => a - b) : null;
}

// nearest is how strongly a channel's nearest user is heard: of those
// blotting it out, by signal (close is { <ap>: dBm }, an AP not in it
// counted as near), or -200 where no one uses it.
function nearest(r, close) {
	let near = -200;
	for (let ap in r?.blotted_by ?? [])
		near = max(near, close?.[ap] ?? 0);
	return near;
}

// best_of chooses a band's channel from its ratings ([{ channel, cost,
// blotted_by }]): the best rated no neighbour uses; or, where neighbours
// use them all, as they can 2.4 GHz's three, the one whose nearest user is
// furthest away. Of those alike, the best rated.
function best_of(rates, close) {
	let best = null, key = null;
	for (let r in rates ?? []) {
		let k = [length(r.blotted_by ?? []) ? 1 : 0, nearest(r, close), r.cost];
		if (!best || k[0] < key[0] || (k[0] == key[0] && (k[1] < key[1] || (k[1] == key[1] && k[2] < key[2])))) {
			best = r;
			key = k;
		}
	}
	return best;
}

// pick is the channel best_of chooses.
function pick(rates, close) {
	return best_of(rates, close)?.channel;
}

// radar says whether a 5 GHz channel is one shared with radar (DFS).
function radar(band, channel) {
	return band == '5g' && channel >= 52 && channel <= 144;
}

// span is the frequencies a block of 20 MHz channels takes up on a band,
// in MHz: its lowest edge and its highest; null for none.
function span(band, members) {
	let lo = null, hi = null;
	for (let c in members ?? []) {
		let f = freq(band, c);
		if (!f)
			continue;
		if (lo == null || f - 10 < lo)
			lo = f - 10;
		if (hi == null || f + 10 > hi)
			hi = f + 10;
	}
	return lo == null ? null : [lo, hi];
}

// too_near says two radios of one AP, one on 5 GHz and one on 6 GHz, come
// nearer than apart MHz edge to edge (0327): the one that sends desenses
// the other's receiver. Radios on one band, or on 2.4 GHz, are never.
function too_near(band, members, other, others, apart) {
	if (!apart || band == other || !(band in ['5g', '6g']) || !(other in ['5g', '6g']))
		return false;
	let a = span(band, members), b = span(other, others);
	if (!a || !b)
		return false;
	let gap = a[0] >= b[1] ? a[0] - b[1] : b[0] >= a[1] ? b[0] - a[1] : 0;
	return gap < apart;
}

// blocks are what a radio at width may move to, from its band's ratings
// ([{ channel, cost, now, blotted_by }]): on 2.4 GHz, or at 20 MHz, each
// channel; on 5 and 6 GHz at 40 MHz or more, each block, rated as its
// worst, blotted out by whoever uses any of its channels (used, by
// '<band> <channel>', where given: those not rated too), and entered on its
// primary (0243), on 5 GHz its lowest, on 6 GHz its second. On 5 GHz every
// channel of a block is rated; on 6 GHz a radio listens on its preferred
// scanning channels and primaries alone (visits), so a block is rated by
// those of its channels, one at least (0298). Each lists its channels as
// members, and has the visits of the one least looked at.
function blocks(list, band, width, used) {
	let all = !width || width <= 20 ? null : band == '5g' ? BLOCKS5['' + width] : band == '6g' ? blocks6(width) : null;
	if (!length(all ?? []))
		return map(list ?? [], x => ({
			channel: x.channel, cost: x.cost, now: x.now ?? x.cost, blotted_by: x.blotted_by ?? [], members: [x.channel], visits: x.visits ?? 0,
		}));
	let six = band == '6g', by = {};
	for (let x in list ?? [])
		by['' + x.channel] = x;
	let out = [];
	for (let b in all) {
		let members = [], rated = [];
		for (let c = b[0]; c <= b[1]; c += 4) {
			push(members, c);
			if (by['' + c])
				push(rated, by['' + c]);
		}
		if (six ? !length(rated) : length(rated) < length(members))
			continue;
		let cost = 0, now = 0, blot = [], visits = null;
		for (let x in rated) {
			cost = max(cost, x.cost);
			now = max(now, x.now ?? x.cost);
			visits = visits == null ? x.visits ?? 0 : min(visits, x.visits ?? 0);
			for (let ap in x.blotted_by ?? [])
				push(blot, ap);
		}
		for (let c in members)
			for (let ap in used?.[band + ' ' + c] ?? [])
				push(blot, ap);
		push(out, { channel: six ? b[0] + 4 : b[0], cost: cost, now: now, blotted_by: uniq(sort(blot)), members: members, visits: visits ?? 0 });
	}
	return out;
}

// reason says why a radio on block cur should move to block best, both as
// blocks gives them, or null:
//   interference: others kept cur busier than SUDDEN for a while (hot, that
//     share, else null), and best rates better than that by margin;
//   shared: a neighbour uses cur, and best is free, or its nearest user is
//     at least NEAR dB further away;
//   better: best rates better than cur by margin, and is no more shared;
//   start: the radio just started (start true), and best_of ranks best
//     above cur.
function reason(cur, best, close, margin, hot, start) {
	if (!cur || !best || index(cur.members ?? [cur.channel], best.channel) >= 0)
		return null;
	let used = length(cur.blotted_by ?? []) > 0, free = !length(best.blotted_by ?? []);
	if (type(hot) in ['int', 'double'] && hot > SUDDEN && best.cost + margin <= 100 * hot)
		return 'interference';
	if (used && (free || nearest(best, close) + NEAR <= nearest(cur, close)))
		return 'shared';
	if (best.cost + margin <= cur.cost && (free || (used && nearest(best, close) <= nearest(cur, close))))
		return 'better';
	return start && best_of([cur, best], close) == best ? 'start' : null;
}

// dfs_states reads what `iw phy <phy> channels` prints of the 5 GHz
// channels from 52 to 144 (0120): { <channel>: { radar, state, cac } }.
// radar says the channel is shared with radar there; state is Linux's:
// usable (a radio may check it), available (checked, and may be sent on)
// or unavailable (radar was found, and it is closed for half an hour); cac
// is how many seconds its radar check takes. A channel the country does
// not allow is left out.
function dfs_states(text) {
	let out = {}, cur = null;
	for (let line in split(text ?? '', '\n')) {
		let m = match(line, /^[ \t]*\* ([0-9]+) MHz \[([0-9]+)\](.*)$/);
		if (m) {
			let f = int(m[1]);
			cur = f >= 5250 && f <= 5730 && index(m[3], 'disabled') < 0 ? { radar: false, state: null, cac: null } : null;
			if (cur)
				out['' + int(m[2])] = cur;
			continue;
		}
		if (!cur)
			continue;
		if (index(line, 'Radar detection') >= 0)
			cur.radar = true;
		else if ((m = match(line, /DFS state: ([a-z]+)/)))
			cur.state = m[1];
		else if ((m = match(line, /DFS CAC time: ([0-9]+) ms/)))
			cur.cac = int(m[1]) / 1000;
	}
	return out;
}

// clear says whether a radio may go to a block with channels shared with
// radar (0120), by dfs_states: each is allowed, none is closed for radar
// found there, and none takes more than CAC_MOST seconds to check, as the
// channels weather radar uses do in Europe: ten minutes.
function clear(band, members, states) {
	for (let c in members ?? []) {
		if (!radar(band, c))
			continue;
		let s = states?.['' + c];
		if (!s)
			return false;
		if (s.radar && (!(s.state in ['usable', 'available']) || s.cac == null || s.cac > CAC_MOST))
			return false;
	}
	return true;
}

// bar shuts a radio out of the channels shared with radar among channels
// (0120), for BARRED_FOR from wall, in Unix seconds: a block it left for a
// reason, radar found there among them, or did not get to. shut holds, by
// '<band> <channel>', when each opens again, and is returned. It is kept
// apart from the moves, which are the last few of every kind.
function bar(shut, band, channels, wall) {
	for (let c in channels ?? [])
		if (radar(band, c))
			shut[band + ' ' + c] = wall + BARRED_FOR;
	return shut;
}

// barred says whether any of a block's channels is shut at wall (bar).
// Such a block is left alone, so a radio does not go back and forth.
function barred(shut, band, members, wall) {
	for (let c in members ?? []) {
		let until = shut?.[band + ' ' + c];
		if (type(until) == 'int' && until > wall)
			return true;
	}
	return false;
}

// fallback is where a radio steps off a DFS channel to when it does not
// know where it came from (0120): the lowest channel of the lowest block
// at its width outside radar, wholly in its set where it has one; or null.
function fallback(band, width, set) {
	if (band != '5g')
		return null;
	let list = !width || width <= 20 ? map([36, 40, 44, 48, 149, 153, 157, 161, 165], c => [c, c]) : (BLOCKS5['' + width] ?? []);
	for (let b in list) {
		let ok = true;
		for (let c = b[0]; c <= b[1]; c += 4)
			if (radar(band, c) || (length(set ?? []) && index(set, c) < 0))
				ok = false;
		if (ok)
			return b[0];
	}
	return null;
}

// radar_reason says why a radio on block cur should go to block far, one
// with channels shared with radar (0120), or null. Going there costs a
// radar check, a minute off the air, so far must be clearly the best:
// shared or better, as reason has them, and the same from plain, the best
// block the radio could go to at once. What a move that costs nothing gets
// it is not worth a minute's silence to better by less. Interference is no
// reason: a radio leaves that at once, for a channel it can use at once;
// nor is having just started.
function radar_reason(cur, plain, far, close, margin) {
	let why = reason(cur, far, close, margin, null, false);
	if (!why)
		return null;
	if (plain && index(cur?.members ?? [cur?.channel], plain.channel) < 0 && !reason(plain, far, close, margin, null, false))
		return null;
	return why;
}

// followed says what came of a move for the clients a radio had when it
// moved (0265): of those (before, { <mac>: <when it associated> }), how many
// are on it still on their own association (kept), on it on a new one
// (back), and not on it (gone), as after, read a while on, says. A driver
// that says no time of association has every client there kept.
function followed(before, after) {
	let out = { clients: 0, kept: 0, back: 0, gone: 0 };
	for (let mac, assoc in before ?? {}) {
		out.clients++;
		if (!(mac in (after ?? {})))
			out.gone++;
		else if (after[mac] === assoc)
			out.kept++;
		else
			out.back++;
	}
	return out;
}

// left_lately says whether a radio on band left any of a block's channels
// (members) by a move made less than BACK_FOR before wall (0264), as its
// AP's moves ([{ band, from, state, at }]) say.
function left_lately(moves, band, members, wall) {
	for (let m in moves ?? [])
		if (m?.band == band && m.state in ['moved', 'checking'] && type(m.at) == 'int' && wall - m.at < BACK_FOR &&
		    index(members ?? [], m.from) >= 0)
			return true;
	return false;
}

// wanted says where a radio on block cur should go: of the blocks it can
// go to at once (to), and of those that cost a radar check (far, 0120).
// plain is the move reason gives to the best of to. far is a move to the
// best of them all, where that is one of far and radar_reason holds. Each
// is { to, why } or null; when each may be made is the daemon's to say.
//
// A planned move, shared or better, does not go back to a block the radio
// left lately (0264), as left says of a block: on a band whose channels its
// neighbours use them all, as 2.4 GHz's three, two APs would trade them back
// and forth, a channel switch each time. It goes to the best of the others,
// where that is worth a move, or nowhere.
function wanted(cur, to, far, close, margin, hot, start, left) {
	let plain = best_of(to, close);
	let why = reason(cur, plain, close, margin, hot, start);
	let out = { plain: why ? { to: plain, why: why } : null, far: null };
	if (why in ['shared', 'better'] && left && left(plain)) {
		let next = best_of(filter(to, x => !left(x)), close);
		let w = reason(cur, next, close, margin, hot, start);
		out.plain = w ? { to: next, why: w } : null;
	}
	let top = best_of([...(to ?? []), ...(far ?? [])], close);
	if (top && length(filter(far ?? [], x => x.channel == top.channel))) {
		let w = radar_reason(cur, plain, top, close, margin);
		if (w)
			out.far = { to: top, why: w };
	}
	return out;
}

// widen says where a radio that starts narrower than it is set should be
// widened to (0329), or null: of wide, the blocks at the width it is set to
// that it may go to (each with channels shared with radar, so the move
// costs a radar check), the best that no neighbour uses and that rates no
// worse than cur, where the radio is, by more than margin. The radio was
// set to that width; it is widened unless that would put it among others,
// or somewhere notably worse. { to, why: 'wide' }.
function widen(cur, wide, close, margin) {
	let ok = filter(wide ?? [], x => cur != null && !length(x.blotted_by ?? []) && x.cost <= cur.cost + margin);
	let best = best_of(ok, close);
	return best ? { to: best, why: 'wide' } : null;
}

// first says whether claim a ({ ap, cost }) goes before claim b, made at
// the same time on one band: the AP whose own channel rates worse moves
// first; where they rate alike, the one with the lower AP ID.
function first(a, b) {
	return a.cost > b.cost || (a.cost == b.cost && a.ap < b.ap);
}

// window reads a window such as '02:00-05:00' as its start and end, in
// minutes of the day, or null.
function window(s) {
	let m = match(s ?? '', /^([01][0-9]|2[0-3]):([0-5][0-9])-([01][0-9]|2[0-3]):([0-5][0-9])$/);
	return m ? [int(m[1]) * 60 + int(m[2]), int(m[3]) * 60 + int(m[4])] : null;
}

// in_window says whether minute t of the day is in window w. It may run
// past midnight; one that starts where it ends is the whole day.
function in_window(w, t) {
	if (!w)
		return false;
	if (w[0] == w[1])
		return true;
	return w[0] < w[1] ? t >= w[0] && t < w[1] : t >= w[0] || t < w[1];
}

// switch_args are hostapd's switch_chan arguments to move a radio running
// mode (its htmode, such as HE40) to channel at width, announced for count
// beacons; or null where the channel can't carry the width. A 6 GHz radio
// runs HE or EHT alone, with no HT or VHT (0278).
function switch_args(band, channel, width, mode, count) {
	let f = freq(band, channel);
	if (!(band in ['2g', '5g', '6g']) || !f)
		return null;
	let fam = match(mode ?? '', /^(NOHT|HT|VHT|HE|EHT)/)?.[1] ?? (band == '6g' ? 'HE' : 'HT');
	let w = fam == 'NOHT' ? 20 : (width ?? 20);
	let six = band == '6g';
	let a = {
		freq: f, center_freq1: f, bandwidth: w, sec_channel_offset: 0, ht: fam != 'NOHT' && !six,
		vht: band == '5g' && fam in ['VHT', 'HE', 'EHT'], he: six || fam in ['HE', 'EHT'], bcn_count: count,
	};
	if (fam == 'EHT')
		a.eht = true;
	if (w <= 20)
		return a;
	if (band == '2g') {
		if (w != 40 || channel < 1 || channel > 13)
			return null;
		// With the channel four above for 1 to 6, four below from 7 up, as
		// OpenWrt joins a set channel (0117).
		a.sec_channel_offset = channel < 7 ? 1 : -1;
		a.center_freq1 = f + 10 * a.sec_channel_offset;
		return a;
	}
	for (let b in (six ? blocks6(w) : BLOCKS5['' + w]) ?? [])
		if (channel >= b[0] && channel <= b[1]) {
			a.center_freq1 = int((freq(band, b[0]) + freq(band, b[1])) / 2);
			a.sec_channel_offset = int((channel - b[0]) / 4) % 2 ? -1 : 1;
			return a;
		}
	return null;
}

// chan_switch is hostapd's CHAN_SWITCH command, as its control socket takes
// it, for what switch_args gives (0120). It is how a radio goes to a channel
// shared with radar: hostapd starts its networks again on the channel and
// has them listen for radar first. Every generation the radio runs is
// named, as hostapd keeps only those the command names.
function chan_switch(a) {
	let s = sprintf('CHAN_SWITCH %d %d', a.bcn_count, a.freq);
	if (a.sec_channel_offset)
		s += ' sec_channel_offset=' + a.sec_channel_offset;
	s += ' center_freq1=' + a.center_freq1 + ' bandwidth=' + a.bandwidth;
	for (let k in ['ht', 'vht', 'he', 'eht'])
		if (a[k])
			s += ' ' + k;
	return s;
}

// power_step says how a radio's power should move (0077). signals are how
// strongly each neighbour on its band hears it, in dBm; it looks for the
// wanted strongest of them. Fewer than that hear it (why: looking), or the
// weakest of them hears it below the target (below): up a step, unless it
// is at its ceiling (ceiling). All of them at least APC_ABOVE dB above the
// target (above): down a step, unless it is at APC_FLOOR (floor). Otherwise
// it holds (target). It returns { step, why, count, weakest }.
function power_step(signals, wanted, target, power, ceiling) {
	let top = slice(sort(filter(signals ?? [], s => type(s) in ['int', 'double']), (a, b) => b - a), 0, wanted);
	let count = length(top), weakest = count ? top[count - 1] : null;
	let out = (step, why) => ({ step: step, why: why, count: count, weakest: weakest });
	if (count < wanted || weakest < target)
		return power >= ceiling ? out(0, 'ceiling') : out(min(APC_STEP, ceiling - power), count < wanted ? 'looking' : 'below');
	if (weakest >= target + APC_ABOVE)
		return power <= APC_FLOOR ? out(0, 'floor') : out(-min(APC_STEP, power - APC_FLOOR), 'above');
	return out(0, 'target');
}

// test_client says whether a MAC is one of the APs' own test clients
// (0121, 0128): the wire test's start 0e:, and own lists the AP's own.
function test_client(mac, own) {
	return substr(mac ?? '', 0, 3) == '0e:' || index(own ?? [], mac) >= 0;
}

// lonely is a radio's clients that no other AP hears better (0206): of
// clients, each { mac, signal, connected } on the radio, those connected
// SETTLED or more and not a test client, that no neighbour has said it hears
// at LONELY_BY dB or more better; others is the best each MAC is heard by a
// neighbour, by MAC. Each comes with what it hears of the AP at power (est),
// and the power it needs to hear it at floor (need), the loneliest first.
// What it hears is what it said it hears (heard, 802.11k), where it said so
// lately, and is then measured; else it is reckoned from how the AP hears
// it, at CLIENT_POWER.
function lonely(clients, others, power, floor, own) {
	let out = [];
	for (let c in clients ?? []) {
		if (type(c?.signal) != 'int' || c.connected == null || c.connected < SETTLED || test_client(c.mac, own))
			continue;
		let o = others?.[c.mac];
		if (type(o) == 'int' && o >= c.signal + LONELY_BY)
			continue;
		let heard = type(c.heard) == 'int' && c.heard < 0 && c.heard >= -127, est = heard ? c.heard : c.signal + power - CLIENT_POWER;
		let x = { mac: c.mac, signal: c.signal, est: est, need: power + floor - est, other: type(o) == 'int' ? o : null };
		if (heard)
			x.measured = true;
		push(out, x);
	}
	return sort(out, (a, b) => a.est - b.est);
}

// client_floor is the power a radio's lonely clients need, the most of them
// (0206), at most its ceiling, in whole dBm; null where none needs more
// than APC_FLOOR.
function client_floor(lonely, ceiling) {
	let need = null;
	for (let c in lonely ?? [])
		if (c.need > APC_FLOOR && (need == null || c.need > need))
			need = c.need;
	return need == null ? null : type(ceiling) == 'int' ? min(need, ceiling) : need;
}

// change is how much more of something a window has than the one before:
// signs is the after window's, was the before's.
function rose(after, before, least) {
	return after >= least && after >= 2 * before + 1;
}

// distress says whether a radio's clients took a step down badly (0206).
// before and after are windows of the same length either side of the step,
// each { probes: {mac: n}, left: n, packets: n, failed: n }: the probe
// requests each connected client sent, the clients that left or came back,
// and what was sent to them and tried again or lost. other is the AP's
// other radio over the same windows, { before, after }, where it has one
// that did not step: what shows there too is no step's doing (0200).
// It returns { distress, why, scanning, left, ratio: [before, after] }.
function distress(before, after, other) {
	let scanning = 0;
	for (let mac, n in after?.probes ?? {})
		if (rose(n, before?.probes?.[mac] ?? 0, 3))
			scanning++;
	let left = after?.left ?? 0, leaving = rose(left, before?.left ?? 0, 2);
	// The share tried again or lost, to a thousandth.
	// A share, so at most 1, whatever a driver counts (2026-10-22: mt76's
	// retries and failures are one count, said twice).
	let ratio = (w) => (w?.packets ?? 0) >= 200 ? min(1000, int((w.failed ?? 0) * 1000 / w.packets)) / 1000.0 : null;
	let rb = ratio(before), ra = ratio(after);
	let retrying = rb != null && ra != null && ra >= rb + 0.15;
	let out = { distress: false, why: null, scanning: scanning, left: left, ratio: [rb, ra] };
	if (scanning < 2 && !leaving && !retrying)
		return out;
	if (other) {
		let os = 0;
		for (let mac, n in other.after?.probes ?? {})
			if (rose(n, other.before?.probes?.[mac] ?? 0, 3))
				os++;
		let orb = ratio(other.before), ora = ratio(other.after);
		if (os >= 2 || rose(other.after?.left ?? 0, other.before?.left ?? 0, 2) || (orb != null && ora != null && ora >= orb + 0.15)) {
			out.why = 'everywhere';
			return out;
		}
	}
	out.distress = true;
	out.why = scanning >= 2 ? 'scanning' : leaving ? 'leaving' : 'retries';
	return out;
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export {
	OUI, PORT, NEIGHBOURS, HELLO_EVERY, DEAD, SKEW, LASTING, NOW, SUDDEN, NEAR, WINDOW, MARGIN, APART, APC_STEP, APC_FLOOR, APC_ABOVE,
	CAC_MOST, DFS_LOOKS, BARRED_FOR, BACK_FOR, dfs_states, clear, bar, barred, fallback, radar_reason, left_lately, wanted, followed, allowed_of,
	hexstr, unhex, hmac, same, advert, read_advert, BEACONS, radiotap_flags, beacon_advert, seal, open, fresh, choose, smooth, freq, band_of, visits,
	printable, swept, around, weight, energy_of, floor_of, cost, blend, covers, span, too_near, nearest, best_of, pick, radar, blocks, reason, widen, first, window, in_window, switch_args, chan_switch, power_step,
	CLIENT_POWER, LONELY_BY, SETTLED, HEARS_MAX, HELLO_MAX, fit, test_client, lonely, client_floor, distress,
	LOAD_FRESH, load_said, load_heard
};
