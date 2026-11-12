// A capture of a client's failed attempt to come online (0135, 0146): the
// frames of it the AP heard and sent, and those that crossed its wire side,
// as a file Wireshark opens. No I/O here: aeolus-journey reads, keeps and
// writes.
//
// The radio's frames are hostapd's own (0146). Every management frame of
// the AP's networks, and every message of a key handshake or of an 802.1X
// sign-in (EAPOL), goes between hostapd and the kernel over netlink; and
// hostapd keeps what it says and hears there in rings another process may
// read (udebug). No monitor interface is made: nothing is asked of the
// radio, and no client loses anything to it. They are kept in a ring a
// radio, a few minutes back, and those of one client are taken from it when
// its attempt fails. The wire side's are the frames aeolus-journey already
// reads of a client it follows: DHCP, ARP, DNS and a first connection.

'use strict';

const SNAP = 2048;           // bytes of a frame a file says it may hold: a management frame whole
const RING_SECONDS = 180;    // how far back a radio's frames are kept
const RING_BYTES = 2097152;  // and how much of them, at most
const RING_FRAMES = 8000;    // and how many: each costs the daemon more than its bytes
const BEFORE = 10;           // seconds before an attempt's start taken with it, as the probes that led to it
const AFTER = 2;             // and after its end
const MOST = 600;            // frames at most in one capture
const MOST_BYTES = 524288;   // and bytes of them: past that the earliest go
const WIRE = 96;             // frames kept from the wire side of one attempt
const SENT_HELD = 5;         // seconds a frame the AP sent waits for the radio's word on it
const SENT_MOST = 64;        // and how many wait at once, a radio
const ONCE_MS = 50;          // ms within which a frame to every station, handed up again, is the same frame
const ONCE_BACK = 16;        // and how many frames back it is looked for
const LINK_RADIO = 127;      // pcapng's link types: 802.11 with a radiotap header
const LINK_WIRE = 1;         // and Ethernet

// nl80211's numbers, which do not change: the commands a frame goes by, and
// what is said with one.
const CMD_FRAME = 59;            // a management frame: hostapd asks for one sent, or is handed one heard
const CMD_FRAME_STATUS = 60;     // what came of one it sent
const CMD_PORT_FRAME = 129;      // a key message or a sign-in's, the same two ways
const CMD_PORT_STATUS = 139;     // and what came of one it sent
const ATTR_IFINDEX = 3, ATTR_MAC = 6, ATTR_FREQ = 38, ATTR_FRAME = 51, ATTR_ACK = 92, ATTR_ETHERTYPE = 102, ATTR_NO_WORD = 142, ATTR_SIGNAL = 151;

const NOT_ACKED = 'The client did not acknowledge this.';
const NOT_SAID = 'The radio did not say whether the client acknowledged this.';

function mac_text(s, at) {
	return join(':', map([0, 1, 2, 3, 4, 5], i => sprintf('%02x', ord(s, at + i))));
}

function u32(s, at) {
	return ord(s, at) | (ord(s, at + 1) << 8) | (ord(s, at + 2) << 16) | (ord(s, at + 3) * 16777216);
}

function u32be(s, at) {
	return ord(s, at) * 16777216 + ((ord(s, at + 1) << 16) | (ord(s, at + 2) << 8) | ord(s, at + 3));
}

function le16(n) {
	return chr(n & 255, (n >> 8) & 255);
}

function le32(n) {
	return chr(n & 255, (n >> 8) & 255, (n >> 16) & 255, (n >> 24) & 255);
}

// mac_bytes is a MAC as it is in a frame.
function mac_bytes(mac) {
	return join('', map(split(mac, ':'), x => chr(hex(x))));
}

// stamp is when the kernel says a frame came, in ms on the calendar's
// clock, from what a socket hands with the frame where it was asked for
// the time in nanoseconds; null where it does not say, or says a time
// that is not within five seconds before near (ms), as one read wrongly
// is not. The kernel's stamp is seconds and the part of one, eight bytes
// each or four, in the processor's own order; ucode hands those bytes as
// they are. Read into numbers, as a later ucode may, a stamp with no part
// of a second at all is one half read, and is not believed: so ucode reads
// the stamp in microseconds on a 32-bit processor, where every frame
// would be put at the start of its second.
function stamp(data, near) {
	let tries = [];
	if (type(data) == 'object' && type(data.sec) == 'int') {
		let part = type(data.nsec) == 'int' ? data.nsec / 1e6 : type(data.usec) == 'int' ? data.usec / 1e3 : 0;
		if (part > 0)
			push(tries, data.sec * 1000 + part);
	}
	else if (type(data) == 'string' && length(data) == 16) {
		push(tries, u32(data, 0) * 1000 + u32(data, 8) / 1e6);
		push(tries, u32be(data, 4) * 1000 + u32be(data, 12) / 1e6);
	}
	else if (type(data) == 'string' && length(data) == 8) {
		push(tries, u32(data, 0) * 1000 + u32(data, 4) / 1e6);
		push(tries, u32be(data, 0) * 1000 + u32be(data, 4) / 1e6);
	}
	for (let ms in tries)
		if (near - ms >= 0 && near - ms < 5000)
			return ms;
	return null;
}

// said reads one entry of hostapd's netlink rings: what it asked the kernel
// to send, or what the kernel handed it. Sixteen bytes say which way it
// went; then the netlink message, in the processor's own byte order, which
// the message's own length tells. Returned where it is a frame of a client's
// coming, null for everything else, which is most of what the rings hold
// (the radio's survey, each client's counters, the beacons hostapd sets):
//
//   { what: 'heard', 'sent' or 'status', key, frame, peer, ifindex, freq, signal, acked, unasked, proto }
//
// A management frame (key false) is in frame as it is on the air, less its
// check sequence. A key message or a sign-in's (key true) is its EAPOL
// bytes alone, with the client it is from or for in peer. 'status' is the
// radio's word on a frame the AP sent, which it repeats: acked says whether
// the client acknowledged it. unasked is on a frame hostapd sends without
// asking for that word, as most of its answers to probes: none comes. A
// neighbour's beacon, which hostapd is handed for its own purposes, is no
// client's, and goes.
function said(data) {
	let n = length(data);
	if (n < 40)
		return null;
	let le = true, size = u32(data, 16);
	if (size < 20 || size > n - 16) {
		le = false;
		size = u32be(data, 16);
		if (size < 20 || size > n - 16)
			return null;
	}
	let u16 = le ? (at => ord(data, at) | (ord(data, at + 1) << 8)) : (at => (ord(data, at) << 8) | ord(data, at + 1));
	let word = le ? (at => u32(data, at)) : (at => u32be(data, at));
	// Netlink's own answers, and its errors.
	if (u16(20) < 16)
		return null;
	let cmd = ord(data, 32);
	if (cmd != CMD_FRAME && cmd != CMD_FRAME_STATUS && cmd != CMD_PORT_FRAME && cmd != CMD_PORT_STATUS)
		return null;
	let status = cmd == CMD_FRAME_STATUS || cmd == CMD_PORT_STATUS, asked = ord(data, 1) == 7;
	if (status && asked)
		return null;
	let e = { what: status ? 'status' : asked ? 'sent' : 'heard', key: cmd == CMD_PORT_FRAME || cmd == CMD_PORT_STATUS,
		frame: null, peer: null, ifindex: null, freq: null, signal: null, acked: false, unasked: false, proto: 0x888e };
	for (let at = 36, end = 16 + size; at + 4 <= end; ) {
		let len = u16(at), v = at + 4;
		if (len < 4 || at + len > end)
			break;
		switch (u16(at + 2) & 0x3fff) {
		case ATTR_IFINDEX:
			if (len >= 8)
				e.ifindex = word(v);
			break;
		case ATTR_MAC:
			if (len >= 10)
				e.peer = mac_text(data, v);
			break;
		case ATTR_FREQ:
			if (len >= 8)
				e.freq = word(v);
			break;
		case ATTR_FRAME:
			e.frame = substr(data, v, len - 4);
			break;
		case ATTR_ACK:
			e.acked = true;
			break;
		case ATTR_NO_WORD:
			e.unasked = true;
			break;
		case ATTR_ETHERTYPE:
			if (len >= 6)
				e.proto = u16(v);
			break;
		case ATTR_SIGNAL:
			if (len >= 8) {
				let s = word(v);
				e.signal = s >= 2147483648 ? s - 4294967296 : s;
			}
			break;
		}
		at += (len + 3) & ~3;
	}
	// The kernel's answer to a frame asked for is its number alone.
	if (e.frame == null)
		return null;
	if (!e.key) {
		// Management frames, and no neighbour's beacon.
		if (length(e.frame) < 24 || (ord(e.frame, 0) & 0x0c) != 0 || (ord(e.frame, 0) & 0xf0) == 0x80)
			return null;
		e.peer = null;
		return e;
	}
	// The radio's word on a key message has the message as it went to the
	// wire side's kind of header: who it is for, who from, what it is.
	if (status) {
		if (length(e.frame) < 14)
			return null;
		e.peer = mac_text(e.frame, 0);
		e.proto = (ord(e.frame, 12) << 8) | ord(e.frame, 13);
		e.frame = substr(e.frame, 14);
	}
	return e.peer != null && length(e.frame) >= 4 ? e : null;
}

// Why a client left or was told to, by the number a deauthentication or a
// disassociation gives: those met most.
const REASONS = {
	'1': 'no reason given', '2': 'its authentication no longer holds', '3': 'leaving', '4': 'idle too long',
	'5': 'the AP has too many clients', '6': 'it is not authenticated', '7': 'it is not associated', '8': 'leaving the network',
	'14': 'a key message failed its check', '15': 'the key handshake timed out', '16': 'the group key handshake timed out',
	'23': 'its sign-in failed', '34': 'too many frames went unacknowledged',
};

// words is what a frame of hostapd's (an entry of said) is called where its
// client is traced (0144): { mac, the client's; stage, one of an attempt's;
// what, in words; ok, false where it says no }. Null for a frame that is
// said of no one client: one to every station, an answer to a probe, of
// which a radio sends one for each of its networks, and the radio's word
// that a frame was acknowledged, which is the usual. The radio's word that
// one was not is the frame again, saying so.
function words(e) {
	let heard = e.what == 'heard', way = heard ? 'from it' : 'to it';
	if (e.what == 'status' && e.acked)
		return null;
	let out = { mac: e.peer, stage: null, what: null, ok: null };
	let f = e.frame;
	if (e.key) {
		let kind = ord(f, 1);
		if (kind == 3 && length(f) >= 7) {
			let info = (ord(f, 5) << 8) | ord(f, 6), first = (info & 0x80) != 0;
			out.stage = 'key';
			out.what = (info & 0x08) ? sprintf('message %d of 4 %s', first ? ((info & 0x100) ? 3 : 1) : ((info & 0x200) ? 4 : 2), way)
				: sprintf('group key message %d of 2 %s', first ? 1 : 2, way);
		}
		else if (kind == 0 && length(f) >= 5) {
			let code = ord(f, 4);
			out.stage = 'signin';
			out.what = (['EAP message', 'EAP request', 'EAP response', 'EAP success', 'EAP failure'][code] ?? 'EAP message') + ' ' + way;
			if (code == 4)
				out.ok = false;
		}
		else if (kind == 1) {
			out.stage = 'signin';
			out.what = 'it asks to sign in (EAPOL-Start)';
		}
		else
			return null;
	}
	else {
		let who = heard ? 10 : 4, sub = (ord(f, 0) >> 4) & 15;
		if (ord(f, who) & 1)
			return null;
		out.mac = mac_text(f, who);
		let u16 = at => length(f) >= at + 2 ? ord(f, at) | (ord(f, at + 1) << 8) : 0;
		let answer = (status, more) => {
			if (status != 0)
				out.ok = false;
			return status == 0 ? 'answer to it: ok' + (more ?? '') : sprintf('answer to it: refused, status %d', status);
		};
		switch (sub) {
		case 4:
			if (!heard)
				return null;
			let n = length(f) >= 26 && ord(f, 24) == 0 ? ord(f, 25) : 0;
			let ssid = n > 0 && n <= 32 && length(f) >= 26 + n ? replace(substr(f, 26, n), /[^ !#-~]/g, '?') : '';
			out.stage = 'find';
			out.what = ssid ? `probe request, for "${ssid}"` : 'probe request, for any network';
			break;
		case 11:
			let alg = u16(24), seq = u16(26), status = u16(28);
			out.stage = 'auth';
			if (alg == 3) {
				out.what = sprintf('SAE %s %s', seq == 1 ? 'commit' : 'confirm', way);
				// 76 asks the client to come again with a token, and 126 says how its commit is made.
				if (status != 0 && status != 76 && status != 126) {
					out.what += sprintf(': refused, status %d', status);
					out.ok = false;
				}
			}
			else if (heard)
				out.what = 'request from it' + (alg == 2 ? ', by a fast transition (802.11r)' : '');
			else
				out.what = answer(status);
			break;
		case 0:
			out.stage = 'assoc';
			out.what = 'request from it';
			break;
		case 2:
			out.stage = 'assoc';
			out.what = 'request from it, as one that comes from ' + (length(f) >= 34 ? mac_text(f, 28) : 'another AP');
			break;
		case 1:
		case 3:
			if (heard)
				return null;
			out.stage = 'assoc';
			out.what = answer(u16(26), sprintf(', AID %d', u16(28) & 0x3fff));
			break;
		case 10:
		case 12:
			let reason = u16(24);
			out.stage = 'end';
			out.what = sprintf('%s %s, reason %d%s', sub == 12 ? 'deauthentication' : 'disassociation', way, reason, REASONS['' + reason] ? ' (' + REASONS['' + reason] + ')' : '');
			break;
		case 13:
			let cat = length(f) > 24 ? ord(f, 24) : -1, act = length(f) > 25 ? ord(f, 25) : -1;
			if (cat == 10 && act >= 6 && act <= 8) {
				out.stage = 'steer';
				out.what = act == 6 ? 'it asks where it should go (BSS transition query)' : act == 7 ? 'asked to move (BSS transition request)'
					: length(f) > 27 && ord(f, 27) != 0 ? sprintf('it will not move (BSS transition response, status %d)', ord(f, 27)) : 'it agrees to move (BSS transition response)';
			}
			else if (cat == 5 && act in [0, 1, 4, 5]) {
				out.stage = 'steer';
				out.what = act == 0 ? 'asked to measure (radio measurement request)' : act == 1 ? 'its measurement (radio measurement report)'
					: act == 4 ? 'it asks for the neighbours (neighbour report request)' : 'the neighbours, sent to it (neighbour report)';
			}
			else if (cat == 8) {
				out.stage = 'assoc';
				out.what = sprintf('SA Query %s %s', act == 0 ? 'request' : 'response', way);
			}
			else {
				out.stage = 'said';
				out.what = sprintf('action frame %s, category %d', way, cat);
			}
			break;
		default:
			return null;
		}
	}
	if (e.what == 'status') {
		out.what += ', not acknowledged by it';
		out.ok = false;
	}
	return out;
}

// radiotap is the header a frame of hostapd's is given, so that Wireshark
// reads it as one from a radio: its channel, where hostapd said the
// frequency; how strongly it was heard, of one that was; and of one the AP
// sent, a word for whether it failed, which is set when the radio says so.
// Each of its parts sits where radiotap has it: on a boundary of its size.
function radiotap(freq, signal, sent) {
	let has = 0x02, body = chr(0);
	if (freq) {
		has |= 0x08;
		body += chr(0) + le16(freq) + le16(freq < 3000 ? 0x0080 : freq < 5900 ? 0x0100 : 0);
	}
	if (signal != null) {
		has |= 0x20;
		body += chr(signal & 255);
	}
	if (sent) {
		has |= 0x8000;
		if (length(body) % 2)
			body += chr(0);
		body += le16(0);
	}
	return chr(0, 0) + le16(8 + length(body)) + le32(has) + body;
}

// header reads a frame's radiotap header: rt, where the 802.11 frame
// begins. Null for what has none.
function header(m) {
	if (length(m) < 8)
		return null;
	let rt = ord(m, 2) | (ord(m, 3) << 8);
	return rt < 8 || rt > length(m) ? null : { rt: rt };
}

// addresses are the stations a kept frame names: the one it is for, the
// one that sent it, and its third, the network's. Null for a frame too
// short to have the first two.
function addresses(m) {
	let rt = header(m)?.rt;
	if (rt == null || length(m) < rt + 16)
		return null;
	return [mac_text(m, rt + 4), mac_text(m, rt + 10), length(m) >= rt + 22 ? mac_text(m, rt + 16) : null];
}

// framed is an entry of said as a frame a file holds: a management frame as
// it is, behind its header; a key message as the data frame it crossed the
// air in, between the client and the network (bssid), with nothing a radio
// adds: no sequence number, no QoS field, and in the clear, as hostapd has
// it whether or not the radio encrypted it. freq is the radio's frequency
// where the entry has none, as a key message has not.
function framed(e, bssid, freq) {
	let heard = e.what == 'heard';
	let rt = radiotap(e.freq ?? freq, heard ? e.signal : null, !heard);
	if (!e.key)
		return rt + e.frame;
	let ap = mac_bytes(bssid), sta = mac_bytes(e.peer);
	return rt + chr(0x08, heard ? 0x01 : 0x02, 0, 0) + (heard ? ap + sta + ap : sta + ap + ap) + chr(0, 0) +
		chr(0xaa, 0xaa, 0x03, 0, 0, 0, (e.proto >> 8) & 255, e.proto & 255) + e.frame;
}

// failed is a frame the AP sent, with its header's word set: the client
// did not acknowledge it. The word is the last of the header.
function failed(m) {
	let rt = header(m)?.rt;
	return rt == null || rt < 10 ? m : substr(m, 0, rt - 2) + le16(1) + substr(m, rt);
}

// same says whether the radio's word is of this frame the AP sent. A key
// message is repeated as it was given. A management frame is too, but for
// what the radio fills in on its way out: how long the air is held, and
// its sequence number.
function same(s, e) {
	if (s.key != e.key || length(s.raw) != length(e.frame))
		return false;
	if (e.key)
		return s.peer == e.peer && s.raw == e.frame;
	return substr(s.raw, 0, 2) == substr(e.frame, 0, 2) && substr(s.raw, 4, 18) == substr(e.frame, 4, 18) && substr(s.raw, 24) == substr(e.frame, 24);
}

// ring is a radio's frames, the latest last; with those the AP sent that
// the radio has not yet spoken of, and the frequency it was last said to
// be on.
function ring() {
	return { list: [], bytes: 0, sent: [], freq: null };
}

// ring_add keeps a frame, with when it came: by the steady clock in seconds
// and by the calendar's in ms. What is older than RING_SECONDS goes, and
// the oldest where the ring holds more than RING_BYTES or RING_FRAMES: not
// at each frame, as making the list anew costs as much as it holds, but
// when it is a quarter past one of them, back to it. A frame that names no
// station is not kept. Returned as it is held, or null.
function ring_add(r, at, wall, m) {
	let rt = header(m)?.rt;
	if (rt == null || length(m) < rt + 16)
		return null;
	let f = { at: at, wall: wall, m: m };
	push(r.list, f);
	r.bytes += length(m);
	if (r.bytes <= RING_BYTES * 1.25 && length(r.list) <= RING_FRAMES * 1.25 && at - r.list[0].at <= RING_SECONDS * 1.25)
		return f;
	let gone = 0;
	while (gone < length(r.list) - 1 && (at - r.list[gone].at > RING_SECONDS || r.bytes > RING_BYTES || length(r.list) - gone > RING_FRAMES))
		r.bytes -= length(r.list[gone++].m);
	if (gone)
		r.list = slice(r.list, gone);
	return f;
}

// ring_said keeps what hostapd said or heard of a frame (an entry of said)
// in its radio's ring: at, by the steady clock in seconds, and wall, by the
// calendar's in ms, are hostapd's own time of it. bssid is the network's
// address, for a key message, which does not carry it.
//
// A frame to every station is handed to hostapd once for each network its
// radio has, as a client's asking for networks is: it is kept once. Null
// is returned for the others, and for what names no station.
//
// A frame the AP sent is kept when hostapd asks for it, which is when it
// went, in hostapd's order of things; the radio's word on it comes after,
// some milliseconds on, and is put on the frame it is of: the earliest
// still waiting that it repeats. A word on a frame whose asking was not
// seen, as where the reading began between the two, keeps the frame from
// the word. Each frame the AP sent is marked: sent; acked, true or false
// once the radio has said, null until then; unasked, where no word was
// asked for; and all, where it is to every station, which none
// acknowledges.
function ring_said(r, at, wall, e, bssid) {
	if (e.freq)
		r.freq = e.freq;
	if (e.what == 'status') {
		for (let i = 0; i < length(r.sent); i++) {
			let s = r.sent[i];
			if (at - s.at > SENT_HELD || !same(s, e))
				continue;
			s.f.acked = e.acked;
			if (!e.acked && !s.f.all)
				s.f.m = failed(s.f.m);
			splice(r.sent, i, 1);
			return s.f;
		}
	}
	if (e.what == 'heard' && !e.key && (ord(e.frame, 4) & 1))
		for (let i = length(r.list) - 1; i >= 0 && i >= length(r.list) - ONCE_BACK; i--) {
			let f = r.list[i];
			if (wall - f.wall > ONCE_MS)
				break;
			if (!f.sent && substr(f.m, header(f.m).rt) == e.frame)
				return null;
		}
	let f = ring_add(r, at, wall, framed(e, bssid, r.freq));
	if (!f || e.what == 'heard')
		return f;
	f.sent = true;
	// One sent to every station is acknowledged by none, and is not said
	// to have failed for it.
	f.all = (ord(f.m, header(f.m).rt + 4) & 1) != 0;
	if (e.what == 'status') {
		f.acked = e.acked;
		if (!e.acked && !f.all)
			f.m = failed(f.m);
		return f;
	}
	f.acked = null;
	f.unasked = e.unasked;
	if (e.unasked)
		return f;
	push(r.sent, { at: at, key: e.key, raw: e.frame, peer: e.peer, f: f });
	// Those the radio never spoke of go, and the oldest past so many.
	if (length(r.sent) > SENT_MOST || at - r.sent[0].at > SENT_HELD)
		r.sent = filter(slice(r.sent, max(0, length(r.sent) - SENT_MOST)), s => at - s.at <= SENT_HELD);
	return f;
}

// ring_of is a client's frames in a ring, from a time to a time by the
// steady clock: those it sent, and those sent to it. MOST at the most, the
// latest of them.
function ring_of(r, mac, from, to) {
	let raw = mac_bytes(mac), out = [];
	for (let f in r.list) {
		if (f.at < from || f.at > to)
			continue;
		let rt = ord(f.m, 2) | (ord(f.m, 3) << 8);
		if (substr(f.m, rt + 4, 6) == raw || substr(f.m, rt + 10, 6) == raw)
			push(out, f);
	}
	return length(out) > MOST ? slice(out, -MOST) : out;
}

// padded is bytes made up to a multiple of four, as pcapng has them.
function padded(s) {
	let more = (4 - length(s) % 4) % 4;
	for (let i = 0; i < more; i++)
		s += chr(0);
	return s;
}

function block(type, body) {
	let n = 12 + length(body);
	return le32(type) + le32(n) + body + le32(n);
}

// An option: its code, its length and its value, padded.
function option(code, value) {
	return le16(code) + le16(length(value)) + padded(value);
}

// pcapng is a capture as a file: frames, each { wall, m, link }, link 0 for
// one from the radio and 1 for one from the wire side, in the order given;
// and what the two are named in it. Times are the calendar's, wall in ms,
// written in microseconds, which a file's times are unless it says. A
// frame with a note has it as its comment, which Wireshark shows with it.
function pcapng(radio, wire, frames) {
	let out = block(0x0a0d0d0a, le32(0x1a2b3c4d) + le16(1) + le16(0) + le32(0xffffffff) + le32(0xffffffff) +
		option(4, 'Aeolus') + option(0, ''));
	let iface = (link, name) => block(1, le16(link) + le16(0) + le32(SNAP) + option(2, name) + option(0, ''));
	out += iface(LINK_RADIO, radio) + iface(LINK_WIRE, wire);
	for (let f in frames) {
		let us = int(f.wall * 1000 + 0.5), hi = int(us / 4294967296), lo = us % 4294967296;
		out += block(6, le32(f.link) + le32(hi) + le32(lo) + le32(length(f.m)) + le32(length(f.m)) + padded(f.m) +
			(f.note ? option(1, f.note) + option(0, '') : ''));
	}
	return out;
}

// note_of is what a file says beside a frame the AP sent: that the client
// did not acknowledge it, or that the radio never said, where it was asked
// and the frame was to one station. Null for every other frame.
function note_of(f) {
	if (!f.sent || f.all || f.unasked)
		return null;
	return f.acked === false ? NOT_ACKED : f.acked == null ? NOT_SAID : null;
}

// capture is the file of one attempt, or null where nothing was kept of
// it: the client's frames from its radio's ring, around the attempt, and
// those kept from the wire side, each { at, wall, m }, in the order they
// came. from and to are the attempt's start and end by the steady clock.
// 512 kB of frames at the most, the latest: how it ended says more of a
// failure than how it began.
//
// Each frame is at hostapd's own time of it: when it was handed one the
// radio heard, and when it asked for one sent. So a message of the AP's is
// before the client's answer to it, whatever the radio made of either. A
// frame the AP sent that the client did not acknowledge says so, in its
// header and in a note; so does one the radio never spoke of.
function capture(r, mac, from, to, wire, radio_name, wire_name) {
	let got = r ? ring_of(r, mac, from - BEFORE, to + AFTER) : [];
	let first = length(got), size = 0;
	for (let f in wire ?? [])
		size += length(f.m);
	for (; first > 0 && size + length(got[first - 1].m) <= MOST_BYTES; first--)
		size += length(got[first - 1].m);
	let frames = [];
	for (let f in slice(got, first))
		push(frames, { wall: f.wall, m: f.m, link: 0, n: length(frames), note: note_of(f) });
	for (let f in wire ?? [])
		push(frames, { wall: f.wall, m: f.m, link: 1, n: length(frames) });
	if (!length(frames))
		return null;
	// By when they came, as the file says it; two at once as they were kept.
	frames = sort(frames, (a, b) => a.wall < b.wall ? -1 : a.wall > b.wall ? 1 : a.n - b.n);
	let radio = length(filter(frames, f => f.link == 0));
	return { frames: length(frames), radio: radio, wire: length(frames) - radio, file: pcapng(radio_name, wire_name, frames) };
}

export { WIRE, AFTER, MOST_BYTES, NOT_ACKED, NOT_SAID, REASONS, stamp, said, words, radiotap, framed, header, addresses, ring, ring_add, ring_said, ring_of, note_of, pcapng, capture };
