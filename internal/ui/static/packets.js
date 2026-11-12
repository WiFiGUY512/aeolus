// Reading a capture in the page (0341): a pcapng file, or a pcap one, read
// into its packets, and each packet taken apart into its layers and their
// fields, each field with where its bytes are, so the packet viewer can
// show a list, a tree and the bytes as Wireshark does. What the Aeolus APs
// capture: 802.11 with radiotap on the Wi-Fi side, Ethernet on the wire
// side; and what rides in them that a join is made of: EAPOL, ARP, IPv4 and
// IPv6, UDP, TCP, ICMP, DHCP and DNS. What is not known is shown as bytes,
// never guessed at.

// readCapture is a capture's packets, in the order the file has them:
// each { n, t (seconds since 1970, fractional), link, data, comment }.
export function readCapture(b) {
	const dv = new DataView(b.buffer, b.byteOffset, b.byteLength);
	if (b.length >= 24) {
		const m = dv.getUint32(0, true);
		if (m === 0xa1b2c3d4 || m === 0xa1b23c4d || m === 0xd4c3b2a1 || m === 0x4d3cb2a1) return readPcap(b, dv);
	}
	return readPcapng(b, dv);
}

function readPcap(b, dv) {
	const m = dv.getUint32(0, true);
	const le = m === 0xa1b2c3d4 || m === 0xa1b23c4d;
	const nano = m === 0xa1b23c4d || m === 0x4d3cb2a1;
	const link = dv.getUint32(20, le);
	const out = [];
	for (let at = 24; at + 16 <= b.length;) {
		const sec = dv.getUint32(at, le), frac = dv.getUint32(at + 4, le), cap = dv.getUint32(at + 8, le);
		if (at + 16 + cap > b.length) break;
		out.push({ n: out.length + 1, t: sec + frac / (nano ? 1e9 : 1e6), link, data: b.subarray(at + 16, at + 16 + cap) });
		at += 16 + cap;
	}
	return out;
}

function readPcapng(b, dv) {
	const out = [];
	let le = true;
	const ifaces = [];
	for (let at = 0; at + 12 <= b.length;) {
		let type = dv.getUint32(at, le);
		if (type === 0x0a0d0d0a) {
			// A section begins: its byte order is its magic's.
			le = dv.getUint32(at + 8, true) === 0x1a2b3c4d;
			type = 0x0a0d0d0a;
			ifaces.length = 0;
		}
		const len = dv.getUint32(at + 4, le);
		if (len < 12 || at + len > b.length) break;
		const body = at + 8, end = at + len - 4;
		if (type === 1) {
			// An interface: its link type, and how finely its times count.
			const iface = { link: dv.getUint16(body, le), res: 1e6 };
			for (const [code, v] of options(b, dv, body + 8, end, le))
				if (code === 9 && v.length) iface.res = v[0] & 0x80 ? 2 ** (v[0] & 0x7f) : 10 ** v[0];
			ifaces.push(iface);
		} else if (type === 6) {
			const id = dv.getUint32(body, le);
			const ts = dv.getUint32(body + 4, le) * 2 ** 32 + dv.getUint32(body + 8, le);
			const cap = dv.getUint32(body + 12, le);
			const iface = ifaces[id] || { link: 1, res: 1e6 };
			const data = b.subarray(body + 20, body + 20 + cap);
			let comment = '';
			for (const [code, v] of options(b, dv, body + 20 + ((cap + 3) & ~3), end, le))
				if (code === 1) comment = new TextDecoder().decode(v);
			out.push({ n: out.length + 1, t: ts / iface.res, link: iface.link, data, comment, iface: id });
		} else if (type === 3) {
			const iface = ifaces[0] || { link: 1, res: 1e6 };
			const orig = dv.getUint32(body, le);
			out.push({ n: out.length + 1, t: null, link: iface.link, data: b.subarray(body + 4, body + 4 + Math.min(orig, end - body - 4)) });
		}
		at += len;
	}
	return out;
}

function* options(b, dv, at, end, le) {
	while (at + 4 <= end) {
		const code = dv.getUint16(at, le), len = dv.getUint16(at + 2, le);
		if (code === 0) return;
		yield [code, b.subarray(at + 4, at + 4 + len)];
		at += 4 + ((len + 3) & ~3);
	}
}

// ---- Taking a packet apart ----

const hex = (v, n = 2) => `0x${v.toString(16).padStart(n, '0')}`;
const macAt = (d, o) => [...d.subarray(o, o + 6)].map((x) => x.toString(16).padStart(2, '0')).join(':');
const ip4At = (d, o) => [...d.subarray(o, o + 4)].join('.');
function ip6At(d, o) {
	const w = [];
	for (let i = 0; i < 16; i += 2) w.push(((d[o + i] << 8) | d[o + i + 1]).toString(16));
	return w.join(':').replace(/(^|:)0(:0)+(:|$)/, '::').replace(/:{3,}/, '::');
}
const u16 = (d, o) => (d[o] << 8) | d[o + 1];
const u16le = (d, o) => d[o] | (d[o + 1] << 8);
const u32 = (d, o) => ((d[o] << 24) >>> 0) + (d[o + 1] << 16) + (d[o + 2] << 8) + d[o + 3];

// A layer is { name, off, len, fields: [{ name, value, off, len, sub? }] };
// a packet taken apart is { src, dst, proto, info, layers }.
class Taken {
	constructor(d) {
		this.d = d;
		this.layers = [];
		this.src = '';
		this.dst = '';
		this.proto = '';
		this.info = '';
	}
	layer(name, off, len) {
		const l = { name, off, len, fields: [] };
		this.layers.push(l);
		return l;
	}
}
function field(l, name, value, off, len, sub) {
	const f = { name, value: String(value), off, len };
	if (sub) f.sub = sub;
	l.fields.push(f);
	return f;
}

// takeApart is a packet in its layers.
export function takeApart(p) {
	const t = new Taken(p.data);
	try {
		if (p.link === 127) radiotap(t, 0);
		else if (p.link === 105) wifi(t, 0, p.data.length, false);
		else if (p.link === 1) ethernet(t, 0);
		else if (p.link === 113) cooked(t, 0);
		else t.layer(`Link type ${p.link}`, 0, p.data.length);
	} catch (e) {
		// A packet cut short: what was read stands.
		t.layer('Cut short', 0, 0);
	}
	if (!t.proto) t.proto = t.layers.at(-1)?.name.split(' ')[0] || '';
	return t;
}

// ---- Radiotap and 802.11 ----

// The radiotap fields, by their bit: [name, alignment, size].
const RT = {
	0: ['TSFT', 8, 8], 1: ['Flags', 1, 1], 2: ['Rate', 1, 1], 3: ['Channel', 2, 4], 4: ['FHSS', 1, 2], 5: ['Antenna signal', 1, 1],
	6: ['Antenna noise', 1, 1], 7: ['Lock quality', 2, 2], 8: ['TX attenuation', 2, 2], 9: ['dB TX attenuation', 2, 2], 10: ['TX power', 1, 1],
	11: ['Antenna', 1, 1], 12: ['dB antenna signal', 1, 1], 13: ['dB antenna noise', 1, 1], 14: ['RX flags', 2, 2], 15: ['TX flags', 2, 2],
	16: ['RTS retries', 1, 1], 17: ['Data retries', 1, 1], 18: ['XChannel', 4, 8], 19: ['MCS', 1, 3], 20: ['A-MPDU', 4, 8], 21: ['VHT', 2, 12],
	22: ['Timestamp', 8, 12], 23: ['HE', 2, 12], 24: ['HE-MU', 2, 12], 26: ['0-length PSDU', 1, 1], 27: ['L-SIG', 2, 4],
};

function radiotap(t, o) {
	const d = t.d;
	const len = u16le(d, o + 2);
	const l = t.layer('Radiotap header', o, len);
	field(l, 'Version', d[o], o, 1);
	field(l, 'Length', len, o + 2, 2);
	// The present words, as many as say there are more.
	const words = [];
	let at = o + 4;
	do {
		words.push(((d[at + 3] << 24) >>> 0) + (d[at + 2] << 16) + (d[at + 1] << 8) + d[at]);
		at += 4;
	} while (words.at(-1) & 0x80000000 && at + 4 <= o + len);
	field(l, 'Present', words.map((w) => hex(w, 8)).join(' '), o + 4, at - o - 4);
	let fcs = false;
	// Only the first word's fields are read; the rest are vendors' and
	// radiotap namespaces of their own, whose sizes are not known here.
	for (let bit = 0; bit < 29; bit++) {
		if (!(words[0] & (1 << bit))) continue;
		const r = RT[bit];
		if (!r) break;
		const [name, align, size] = r;
		at = o + Math.ceil((at - o) / align) * align;
		if (at + size > o + len) break;
		const v = d[at];
		if (bit === 1) {
			fcs = !!(v & 0x10);
			field(l, name, `${hex(v)}${v & 0x10 ? ' (FCS at end)' : ''}${v & 0x02 ? ' (short preamble)' : ''}${v & 0x40 ? ' (bad FCS)' : ''}`, at, 1);
		} else if (bit === 2) field(l, name, `${v / 2} Mbit/s`, at, 1);
		else if (bit === 3) {
			const mhz = u16le(d, at);
			field(l, name, `${mhz} MHz${chan(mhz) ? ` (channel ${chan(mhz)})` : ''}, flags ${hex(u16le(d, at + 2), 4)}`, at, 4);
		} else if (bit === 5 || bit === 6 || bit === 10) field(l, name, `${(v << 24) >> 24} dBm`, at, 1);
		else if (bit === 19) field(l, name, `MCS ${d[at + 2]}${d[at + 1] & 0x01 ? `, bandwidth ${['20', '40', '20L', '20U'][d[at + 1] & 3]} MHz` : ''}`, at, 3);
		else if (bit === 21) field(l, name, `bandwidth index ${d[at + 3]}, MCS/NSS ${hex(d[at + 4])}`, at, 12);
		else if (bit === 0) field(l, name, `${(u32(d, at + 4) * 2 ** 32 + u32(d, at)).toString()} µs`, at, 8);
		else field(l, name, [...d.subarray(at, at + size)].map((x) => x.toString(16).padStart(2, '0')).join(' '), at, size);
		if (bit === 5) t.signal = (v << 24) >> 24;
		at += size;
	}
	wifi(t, o + len, d.length - (fcs ? 4 : 0), fcs);
}

function chan(mhz) {
	if (mhz === 2484) return 14;
	if (mhz >= 2412 && mhz <= 2472) return (mhz - 2407) / 5;
	if (mhz === 5935) return 2;
	if (mhz >= 5955) return (mhz - 5950) / 5;
	if (mhz >= 5000) return (mhz - 5000) / 5;
	return 0;
}

const MGMT = { 0: 'Association Request', 1: 'Association Response', 2: 'Reassociation Request', 3: 'Reassociation Response', 4: 'Probe Request', 5: 'Probe Response',
	6: 'Timing Advertisement', 8: 'Beacon', 9: 'ATIM', 10: 'Disassociation', 11: 'Authentication', 12: 'Deauthentication', 13: 'Action', 14: 'Action No Ack' };
const CTRL = { 4: 'Beamforming Report Poll', 5: 'VHT/HE NDP Announcement', 7: 'Control Wrapper', 8: 'Block Ack Request', 9: 'Block Ack', 10: 'PS-Poll', 11: 'RTS', 12: 'CTS', 13: 'ACK', 14: 'CF-End' };
const DATA = { 0: 'Data', 4: 'Null', 8: 'QoS Data', 12: 'QoS Null' };
// Status and reason codes, those a join meets.
const STATUS = { 0: 'Success', 1: 'Unspecified failure', 10: 'Capabilities not supported', 12: 'Refused, reason outside the standard', 13: 'Authentication algorithm not supported',
	14: 'Authentication sequence out of order', 15: 'Challenge failure', 16: 'Authentication timeout', 17: 'AP cannot handle more stations', 18: 'Rates not supported',
	30: 'Association rejected temporarily, try later', 31: 'Robust management frame policy violation', 37: 'Request declined', 40: 'Invalid element', 41: 'Invalid group cipher',
	42: 'Invalid pairwise cipher', 43: 'Invalid AKMP', 45: 'Cipher suite rejected by policy', 53: 'Invalid PMKID', 54: 'Invalid MDE', 55: 'Invalid FTE',
	72: 'Anti-clogging token required', 77: 'Unsupported finite cyclic group', 82: 'Denied: listen interval too large', 126: 'SAE hash-to-element' };
const REASON = { 1: 'Unspecified', 2: 'Previous authentication no longer valid', 3: 'Station is leaving', 4: 'Inactivity', 5: 'AP cannot handle all stations', 6: 'Class 2 frame from unauthenticated station',
	7: 'Class 3 frame from unassociated station', 8: 'Station is leaving the BSS', 9: 'Not authenticated', 14: 'MIC failure', 15: '4-way handshake timeout', 16: 'Group key handshake timeout',
	17: 'Element differs in 4-way handshake', 18: 'Invalid group cipher', 19: 'Invalid pairwise cipher', 20: 'Invalid AKMP', 23: 'IEEE 802.1X authentication failed', 24: 'Cipher suite rejected',
	34: 'Too many frames unacknowledged', 45: 'Peer key mismatch' };
const AUTH = { 0: 'Open System', 1: 'Shared Key', 2: 'Fast BSS Transition', 3: 'SAE', 4: 'FILS', 5: 'FILS PFS', 6: 'FILS public key' };
const AKM = { 1: '802.1X', 2: 'PSK', 3: 'FT 802.1X', 4: 'FT PSK', 5: '802.1X SHA-256', 6: 'PSK SHA-256', 8: 'SAE', 9: 'FT SAE', 11: '802.1X Suite B', 12: '802.1X Suite B 192',
	13: 'FT 802.1X SHA-384', 18: 'OWE', 24: 'SAE (H2E, ext key)', 25: 'FT SAE ext key' };
const CIPHER = { 1: 'WEP-40', 2: 'TKIP', 4: 'CCMP-128', 5: 'WEP-104', 6: 'BIP-CMAC-128', 8: 'GCMP-128', 9: 'GCMP-256', 10: 'CCMP-256', 11: 'BIP-GMAC-128', 12: 'BIP-GMAC-256', 13: 'BIP-CMAC-256' };
const IE = { 0: 'SSID', 1: 'Supported Rates', 3: 'DS Parameter Set', 5: 'TIM', 7: 'Country', 11: 'BSS Load', 32: 'Power Constraint', 33: 'Power Capability', 35: 'TPC Report',
	36: 'Supported Channels', 42: 'ERP', 45: 'HT Capabilities', 46: 'QoS Capability', 48: 'RSN', 50: 'Extended Supported Rates', 54: 'Mobility Domain', 55: 'Fast BSS Transition',
	56: 'Timeout Interval', 59: 'Supported Operating Classes', 61: 'HT Operation', 70: 'RM Enabled Capabilities', 71: 'Multiple BSSID', 74: 'Overlapping BSS Scan Parameters',
	107: 'Interworking', 108: 'Advertisement Protocol', 111: 'Roaming Consortium', 113: 'Mesh Configuration', 127: 'Extended Capabilities', 191: 'VHT Capabilities', 192: 'VHT Operation',
	195: 'VHT Transmit Power Envelope', 199: 'Operating Mode Notification', 201: 'Reduced Neighbor Report', 221: 'Vendor Specific', 244: 'RSN Extension', 255: 'Extension' };
const IE_EXT = { 35: 'HE Capabilities', 36: 'HE Operation', 38: 'MU EDCA Parameter Set', 39: 'Spatial Reuse Parameter Set', 59: 'HE 6 GHz Band Capabilities', 106: 'EHT Operation', 107: 'Multi-Link', 108: 'EHT Capabilities' };
const ACTION = { 0: 'Spectrum Management', 1: 'QoS', 3: 'Block Ack', 4: 'Public', 5: 'Radio Measurement', 6: 'Fast BSS Transition', 7: 'HT', 8: 'SA Query', 9: 'Protected Dual of Public Action',
	10: 'WNM', 21: 'VHT', 30: 'HE', 126: 'Vendor-specific Protected', 127: 'Vendor-specific' };
const WNM = { 7: 'BSS Transition Management Query', 8: 'BSS Transition Management Request', 9: 'BSS Transition Management Response' };
const RM = { 0: 'Radio Measurement Request', 1: 'Radio Measurement Report', 4: 'Neighbor Report Request', 5: 'Neighbor Report Response' };

function wifi(t, o, end, fcs) {
	const d = t.d;
	const fc = d[o], flags = d[o + 1];
	const type = (fc >> 2) & 3, sub = fc >> 4;
	const kind = type === 0 ? MGMT[sub] || `Management ${sub}` : type === 1 ? CTRL[sub] || `Control ${sub}` : type === 2 ? DATA[sub & 0xc] || `Data ${sub}` : `Extension ${sub}`;
	const l = t.layer(`IEEE 802.11 ${kind}`, o, end - o + (fcs ? 4 : 0));
	const fl = [flags & 1 && 'To DS', flags & 2 && 'From DS', flags & 4 && 'More fragments', flags & 8 && 'Retry', flags & 16 && 'Power management', flags & 32 && 'More data',
		flags & 64 && 'Protected', flags & 128 && '+HTC/Order'].filter(Boolean);
	field(l, 'Frame control', `${hex(fc)} ${hex(flags)}: type ${type}, subtype ${sub}`, o, 2, [
		{ name: 'Type', value: ['Management', 'Control', 'Data', 'Extension'][type], off: o, len: 1 },
		{ name: 'Subtype', value: `${sub} (${kind})`, off: o, len: 1 },
		{ name: 'Flags', value: fl.join(', ') || 'none', off: o + 1, len: 1 }]);
	field(l, 'Duration', `${u16le(d, o + 2) & 0x7fff} µs`, o + 2, 2);
	t.proto = '802.11';
	t.info = kind + (flags & 8 ? ' (retry)' : '');
	let at = o + 4;
	const a1 = macAt(d, at);
	if (type === 1) {
		// Control frames: a receiver, and for some a transmitter.
		field(l, 'Receiver address', a1, at, 6);
		t.dst = a1;
		if ([4, 5, 8, 9, 10, 11, 14].includes(sub) && at + 12 <= end) {
			t.src = macAt(d, at + 6);
			field(l, 'Transmitter address', t.src, at + 6, 6);
		}
		return;
	}
	const a2 = macAt(d, at + 6), a3 = macAt(d, at + 12);
	const tods = flags & 1, fromds = flags & 2;
	const names = type === 0 || (!tods && !fromds) ? ['Destination', 'Source', 'BSSID'] : tods && !fromds ? ['BSSID', 'Source', 'Destination'] : !tods && fromds ? ['Destination', 'BSSID', 'Source'] : ['Receiver', 'Transmitter', 'Destination'];
	field(l, `Address 1 (${names[0]})`, a1, at, 6);
	field(l, `Address 2 (${names[1]})`, a2, at + 6, 6);
	field(l, `Address 3 (${names[2]})`, a3, at + 12, 6);
	t.src = a2;
	t.dst = a1;
	const seq = u16le(d, at + 18);
	field(l, 'Sequence', `${seq >> 4}, fragment ${seq & 15}`, at + 18, 2);
	at += 20;
	if (tods && fromds) {
		field(l, 'Address 4 (Source)', macAt(d, at), at, 6);
		at += 6;
	}
	if (type === 2 && sub & 8) {
		const q = u16le(d, at);
		field(l, 'QoS control', `TID ${q & 15}${q & 0x80 ? ', A-MSDU' : ''}`, at, 2);
		at += 2;
	}
	if (flags & 128 && (type === 2 && sub & 8 || type === 0)) {
		field(l, 'HT control', hex(u32(d, at), 8), at, 4);
		at += 4;
	}
	if (fcs) field(l, 'FCS', hex(u32(d, end), 8), end, 4);
	if (type === 0) return management(t, l, sub, at, end, flags & 64);
	if (type === 2) {
		if (flags & 64) {
			t.info = `${kind}, protected`;
			field(l, 'Protected payload', `${end - at} bytes`, at, end - at);
			return;
		}
		if (sub & 4 || at >= end) return;
		llc(t, at, end);
	}
}

// A protected management frame (802.11w) is ciphered, with 16 bytes of
// header and check at the least, unless the AP took it already deciphered,
// as it does its own: shorter than that, it is read as it is.
const CIPHERED = 17;

function management(t, l, sub, at, end, prot) {
	const d = t.d;
	if (prot && end - at < CIPHERED) prot = false;
	const m = t.layer('Management', at, end - at);
	const said = [];
	if (sub === 11) {
		const alg = u16le(d, at), seq = u16le(d, at + 2), st = u16le(d, at + 4);
		field(m, 'Algorithm', `${alg} (${AUTH[alg] || 'unknown'})`, at, 2);
		field(m, 'Sequence', seq, at + 2, 2);
		field(m, 'Status', `${st} (${STATUS[st] || 'unknown'})`, at + 4, 2);
		said.push(`${AUTH[alg] || `algorithm ${alg}`}, seq ${seq}${st ? `, status ${st} ${STATUS[st] || ''}` : ''}`);
		at += 6;
		if (alg === 3) {
			// SAE: its group and the rest are the exchange's own.
			if (seq === 1 && !st && at + 2 <= end) field(m, 'SAE group', u16le(d, at), at, 2);
			field(m, 'SAE body', `${end - at} bytes`, at, end - at);
			at = end;
		}
	} else if (sub === 0 || sub === 2) {
		field(m, 'Capabilities', hex(u16le(d, at), 4), at, 2);
		field(m, 'Listen interval', u16le(d, at + 2), at + 2, 2);
		at += 4;
		if (sub === 2) {
			field(m, 'Current AP', macAt(d, at), at, 6);
			at += 6;
		}
	} else if (sub === 1 || sub === 3) {
		const st = u16le(d, at + 2);
		field(m, 'Capabilities', hex(u16le(d, at), 4), at, 2);
		field(m, 'Status', `${st} (${STATUS[st] || 'unknown'})`, at + 2, 2);
		field(m, 'Association ID', u16le(d, at + 4) & 0x3fff, at + 4, 2);
		said.push(st ? `status ${st} ${STATUS[st] || ''}` : `AID ${u16le(d, at + 4) & 0x3fff}`);
		at += 6;
	} else if (sub === 10 || sub === 12) {
		if (prot) return field(m, 'Protected', `${end - at} bytes`, at, end - at);
		const r = u16le(d, at);
		field(m, 'Reason', `${r} (${REASON[r] || 'unknown'})`, at, 2);
		said.push(`reason ${r} ${REASON[r] || ''}`);
		at += 2;
	} else if (sub === 5 || sub === 8) {
		field(m, 'Timestamp', `${u32(d, at + 4) * 2 ** 32 + u32(d, at)}`, at, 8);
		field(m, 'Beacon interval', `${u16le(d, at + 8)} TU`, at + 8, 2);
		field(m, 'Capabilities', hex(u16le(d, at + 10), 4), at + 10, 2);
		at += 12;
	} else if (sub === 13 || sub === 14) {
		if (prot) return field(m, 'Protected', `${end - at} bytes`, at, end - at);
		const cat = d[at], act = d[at + 1];
		const what = cat === 10 ? WNM[act] : cat === 5 ? RM[act] : null;
		field(m, 'Category', `${cat} (${ACTION[cat] || 'unknown'})`, at, 1);
		field(m, 'Action', `${act}${what ? ` (${what})` : ''}`, at + 1, 1);
		said.push(`${ACTION[cat] || `category ${cat}`}${what ? `: ${what}` : `, action ${act}`}`);
		if (cat === 10 && act === 9 && at + 4 <= end) {
			field(m, 'Dialog token', d[at + 2], at + 2, 1);
			field(m, 'BTM status', d[at + 3], at + 3, 1);
			said.push(`status ${d[at + 3]}`);
		}
		field(m, 'Body', `${end - at - 2} bytes`, at + 2, end - at - 2);
		at = end;
	}
	if (said.length) t.info += `, ${said.join(', ')}`;
	elements(t, at, end);
}

function elements(t, at, end) {
	const d = t.d;
	if (at + 2 > end) return;
	const l = t.layer('Tagged parameters', at, end - at);
	while (at + 2 <= end) {
		const id = d[at], len = d[at + 1];
		if (at + 2 + len > end) break;
		const v = at + 2;
		let name = IE[id] || `Element ${id}`, value = `${len} bytes`;
		const sub = [];
		if (id === 0) {
			value = len ? JSON.stringify(new TextDecoder().decode(d.subarray(v, v + len))) : 'hidden';
			if (!t.ssid) t.ssid = value;
		} else if (id === 1 || id === 50) value = [...d.subarray(v, v + len)].map((r) => `${(r & 0x7f) / 2}${r & 0x80 ? '*' : ''}`).join(' ') + ' Mbit/s';
		else if (id === 3) value = `channel ${d[v]}`;
		else if (id === 54 && len >= 3) value = `MDID ${hex(u16le(d, v), 4)}, ${d[v + 2] & 1 ? 'FT over DS' : 'FT over the air'}`;
		else if (id === 48) value = rsn(d, v, len, sub);
		else if (id === 221 && len >= 4) {
			const oui = [...d.subarray(v, v + 3)].map((x) => x.toString(16).padStart(2, '0')).join(':');
			name = `Vendor Specific: ${oui === '00:50:f2' ? (d[v + 3] === 2 ? 'WMM' : d[v + 3] === 4 ? 'WPS' : 'Microsoft') : oui === '50:6f:9a' ? 'Wi-Fi Alliance' : oui}`;
		} else if (id === 255 && len >= 1) name = IE_EXT[d[v]] || `Extension ${d[v]}`;
		field(l, name, value, at, 2 + len, sub.length ? sub : undefined);
		at += 2 + len;
	}
	if (t.ssid && /Probe|Beacon|ssociation/.test(t.info)) t.info += `, SSID ${t.ssid}`;
}

function rsn(d, v, len, sub) {
	let at = v;
	const end = v + len;
	const suite = (o) => {
		const oui = (d[o] << 16) | (d[o + 1] << 8) | d[o + 2];
		return oui === 0x000fac ? d[o + 3] : -1;
	};
	sub.push({ name: 'Version', value: String(u16le(d, at)), off: at, len: 2 });
	at += 2;
	if (at + 4 <= end) {
		sub.push({ name: 'Group cipher', value: CIPHER[suite(at)] || 'other', off: at, len: 4 });
		at += 4;
	}
	const list = (what, names) => {
		if (at + 2 > end) return [];
		const n = u16le(d, at), out = [];
		for (let i = 0; i < n && at + 6 + 4 * i <= end; i++) out.push(names[suite(at + 2 + 4 * i)] || 'other');
		sub.push({ name: what, value: out.join(', '), off: at, len: 2 + 4 * n });
		at += 2 + 4 * n;
		return out;
	};
	const pair = list('Pairwise ciphers', CIPHER);
	const akm = list('AKMs', AKM);
	if (at + 2 <= end) {
		const c = u16le(d, at);
		sub.push({ name: 'RSN capabilities', value: `${hex(c, 4)}${c & 0x80 ? ', MFP capable' : ''}${c & 0x40 ? ', MFP required' : ''}`, off: at, len: 2 });
		at += 2;
	}
	if (at + 2 <= end) {
		const n = u16le(d, at);
		sub.push({ name: 'PMKIDs', value: String(n), off: at, len: 2 + 16 * n });
	}
	return `${akm.join(', ') || 'no AKM'} · ${pair.join(', ')}`;
}

// ---- What rides in a frame: LLC, Ethernet, and on up ----

function llc(t, at, end) {
	const d = t.d;
	if (d[at] === 0xaa && d[at + 1] === 0xaa && d[at + 2] === 0x03) {
		const l = t.layer('Logical-Link Control', at, 8);
		const et = u16(d, at + 6);
		field(l, 'SNAP type', hex(et, 4), at + 6, 2);
		return ether(t, et, at + 8, end);
	}
	t.layer('Data', at, end - at);
}

function ethernet(t, o) {
	const d = t.d;
	const l = t.layer('Ethernet II', o, 14);
	t.dst = macAt(d, o);
	t.src = macAt(d, o + 6);
	field(l, 'Destination', t.dst, o, 6);
	field(l, 'Source', t.src, o + 6, 6);
	let et = u16(d, o + 12), at = o + 14;
	while (et === 0x8100 || et === 0x88a8) {
		field(l, '802.1Q VLAN', `${u16(d, at) & 0xfff}, priority ${d[at] >> 5}`, at, 2);
		t.vlan = u16(d, at) & 0xfff;
		et = u16(d, at + 2);
		at += 4;
		l.len += 4;
	}
	field(l, 'Type', hex(et, 4), at - 2, 2);
	t.proto = 'Ethernet';
	ether(t, et, at, d.length);
}

function cooked(t, o) {
	const d = t.d;
	const l = t.layer('Linux cooked capture', o, 16);
	field(l, 'Packet type', u16(d, o), o, 2);
	if (u16(d, o + 4) === 6) {
		t.src = macAt(d, o + 6);
		field(l, 'Source', t.src, o + 6, 6);
	}
	ether(t, u16(d, o + 14), o + 16, d.length);
}

function ether(t, et, at, end) {
	if (et === 0x0800) return ipv4(t, at, end);
	if (et === 0x86dd) return ipv6(t, at, end);
	if (et === 0x0806) return arp(t, at);
	if (et === 0x888e) return eapol(t, at, end);
	t.proto = hex(et, 4);
	t.layer(`Ethertype ${hex(et, 4)}`, at, end - at);
}

function arp(t, o) {
	const d = t.d;
	const op = u16(d, o + 6);
	const l = t.layer('Address Resolution Protocol', o, 28);
	const sm = macAt(d, o + 8), si = ip4At(d, o + 14), tm = macAt(d, o + 18), ti = ip4At(d, o + 24);
	field(l, 'Opcode', op === 1 ? 'request (1)' : op === 2 ? 'reply (2)' : op, o + 6, 2);
	field(l, 'Sender MAC', sm, o + 8, 6);
	field(l, 'Sender IP', si, o + 14, 4);
	field(l, 'Target MAC', tm, o + 18, 6);
	field(l, 'Target IP', ti, o + 24, 4);
	t.proto = 'ARP';
	t.info = op === 1 ? (si === ti ? `Gratuitous ARP for ${si}` : si === '0.0.0.0' ? `ARP probe: who has ${ti}?` : `Who has ${ti}? Tell ${si}`) : `${si} is at ${sm}`;
}

const KEY_MSG = ['', 'Message 1 of 4', 'Message 2 of 4', 'Message 3 of 4', 'Message 4 of 4'];
function eapol(t, o, end) {
	const d = t.d;
	const type = d[o + 1], len = u16(d, o + 2);
	const l = t.layer('802.1X Authentication', o, Math.min(4 + len, end - o));
	field(l, 'Version', d[o], o, 1);
	field(l, 'Type', ['EAP Packet', 'Start', 'Logoff', 'Key', 'Encapsulated ASF Alert'][type] || type, o + 1, 1);
	field(l, 'Length', len, o + 2, 2);
	t.proto = 'EAPOL';
	if (type === 0) {
		const code = d[o + 4];
		const codes = { 1: 'Request', 2: 'Response', 3: 'Success', 4: 'Failure' };
		const e = t.layer('Extensible Authentication Protocol', o + 4, len);
		field(e, 'Code', `${code} (${codes[code] || '?'})`, o + 4, 1);
		field(e, 'Identifier', d[o + 5], o + 5, 1);
		const et = len > 4 ? d[o + 8] : null;
		const types = { 1: 'Identity', 3: 'Legacy Nak', 13: 'TLS', 21: 'TTLS', 25: 'PEAP', 43: 'EAP-FAST' };
		if (et != null) field(e, 'Type', `${et} (${types[et] || '?'})`, o + 8, 1);
		t.proto = 'EAP';
		t.info = `${codes[code] || 'EAP'}${et != null ? `, ${types[et] || `type ${et}`}` : ''}`;
		return;
	}
	if (type !== 3) {
		t.info = ['', 'EAPOL Start', 'EAPOL Logoff'][type] || 'EAPOL';
		return;
	}
	const k = t.layer('Key', o + 4, len);
	const info = u16(d, o + 5);
	const pairwise = !!(info & 0x8), install = !!(info & 0x40), ack = !!(info & 0x80), mic = !!(info & 0x100), secure = !!(info & 0x200);
	field(k, 'Descriptor type', d[o + 4] === 2 ? 'RSN (2)' : d[o + 4], o + 4, 1);
	field(k, 'Key information', `${hex(info, 4)}: ${[pairwise ? 'pairwise' : 'group', install && 'install', ack && 'ACK', mic && 'MIC', secure && 'secure', info & 0x400 && 'error', info & 0x800 && 'request', info & 0x1000 && 'encrypted key data'].filter(Boolean).join(', ')}`, o + 5, 2);
	field(k, 'Key length', u16(d, o + 7), o + 7, 2);
	field(k, 'Replay counter', `${u32(d, o + 9) * 2 ** 32 + u32(d, o + 13)}`, o + 9, 8);
	field(k, 'Nonce', [...d.subarray(o + 17, o + 49)].some((x) => x) ? 'set' : 'zero', o + 17, 32);
	field(k, 'MIC', mic ? 'set' : 'none', o + 81, 16);
	const kd = u16(d, o + 97);
	field(k, 'Key data length', kd, o + 97, 2);
	let n = 0;
	if (pairwise) n = ack && !mic ? 1 : !ack && mic && !secure ? 2 : ack && mic ? 3 : 4;
	if (n === 4 && kd > 0) n = 2;
	t.info = pairwise ? `Key (${KEY_MSG[n]})` : `Group key (message ${ack ? 1 : 2} of 2)`;
}

const PROTO = { 1: 'ICMP', 2: 'IGMP', 6: 'TCP', 17: 'UDP', 58: 'ICMPv6', 112: 'VRRP' };
function ipv4(t, o, end) {
	const d = t.d;
	const ihl = (d[o] & 15) * 4, total = u16(d, o + 2), proto = d[o + 9];
	const l = t.layer('Internet Protocol Version 4', o, ihl);
	const src = ip4At(d, o + 12), dst = ip4At(d, o + 16);
	field(l, 'Header length', `${ihl} bytes`, o, 1);
	field(l, 'DSCP', d[o + 1] >> 2, o + 1, 1);
	field(l, 'Total length', total, o + 2, 2);
	field(l, 'Identification', hex(u16(d, o + 4), 4), o + 4, 2);
	const ff = u16(d, o + 6);
	field(l, 'Flags', `${ff & 0x4000 ? "Don't fragment" : ''}${ff & 0x2000 ? ' More fragments' : ''}${ff & 0x1fff ? `, offset ${(ff & 0x1fff) * 8}` : ''}` || 'none', o + 6, 2);
	field(l, 'Time to live', d[o + 8], o + 8, 1);
	field(l, 'Protocol', `${PROTO[proto] || proto} (${proto})`, o + 9, 1);
	field(l, 'Source', src, o + 12, 4);
	field(l, 'Destination', dst, o + 16, 4);
	t.src = src;
	t.dst = dst;
	t.proto = 'IPv4';
	const stop = Math.min(end, o + total);
	if (ff & 0x1fff) {
		t.info = `Fragment of ${PROTO[proto] || proto}`;
		return;
	}
	transport(t, proto, o + ihl, stop, src, dst);
}

function ipv6(t, o, end) {
	const d = t.d;
	const l = t.layer('Internet Protocol Version 6', o, 40);
	const src = ip6At(d, o + 8), dst = ip6At(d, o + 24);
	let next = d[o + 6], at = o + 40;
	field(l, 'Payload length', u16(d, o + 4), o + 4, 2);
	field(l, 'Next header', `${PROTO[next] || next} (${next})`, o + 6, 1);
	field(l, 'Hop limit', d[o + 7], o + 7, 1);
	field(l, 'Source', src, o + 8, 16);
	field(l, 'Destination', dst, o + 24, 16);
	// Hop-by-hop options before what it carries.
	while (next === 0 && at + 8 <= end) {
		next = d[at];
		at += (d[at + 1] + 1) * 8;
	}
	t.src = src;
	t.dst = dst;
	t.proto = 'IPv6';
	transport(t, next, at, end, src, dst);
}

function transport(t, proto, o, end, src, dst) {
	const d = t.d;
	if (proto === 17) {
		const sp = u16(d, o), dp = u16(d, o + 2), len = u16(d, o + 4);
		const l = t.layer('User Datagram Protocol', o, 8);
		field(l, 'Source port', sp, o, 2);
		field(l, 'Destination port', dp, o + 2, 2);
		field(l, 'Length', len, o + 4, 2);
		t.proto = 'UDP';
		t.info = `${sp} → ${dp}, ${len - 8} bytes`;
		const at = o + 8, stop = Math.min(end, o + len);
		if ((sp === 67 || sp === 68) && (dp === 67 || dp === 68)) return dhcp(t, at, stop);
		if (sp === 53 || dp === 53 || sp === 5353 || dp === 5353) return dns(t, at, stop, sp === 5353 || dp === 5353);
		if (sp === 547 || dp === 547) {
			t.proto = 'DHCPv6';
			t.info = `DHCPv6 ${['', 'Solicit', 'Advertise', 'Request', 'Confirm', 'Renew', 'Rebind', 'Reply', 'Release', 'Decline', 'Reconfigure', 'Information-request'][d[at]] || d[at]}`;
			t.layer('DHCPv6', at, stop - at);
		}
		return;
	}
	if (proto === 6) {
		const sp = u16(d, o), dp = u16(d, o + 2), off = (d[o + 12] >> 4) * 4, fl = d[o + 13];
		const l = t.layer('Transmission Control Protocol', o, off);
		const flags = [fl & 2 && 'SYN', fl & 16 && 'ACK', fl & 1 && 'FIN', fl & 4 && 'RST', fl & 8 && 'PSH'].filter(Boolean).join(', ');
		field(l, 'Source port', sp, o, 2);
		field(l, 'Destination port', dp, o + 2, 2);
		field(l, 'Sequence', u32(d, o + 4), o + 4, 4);
		field(l, 'Acknowledgment', u32(d, o + 8), o + 8, 4);
		field(l, 'Flags', flags || 'none', o + 13, 1);
		field(l, 'Window', u16(d, o + 14), o + 14, 2);
		t.proto = 'TCP';
		t.info = `${sp} → ${dp} [${flags}]${end - o - off > 0 ? `, ${end - o - off} bytes` : ''}`;
		return;
	}
	if (proto === 1 || proto === 58) {
		const ty = d[o], code = d[o + 1];
		const l = t.layer(proto === 1 ? 'Internet Control Message Protocol' : 'Internet Control Message Protocol v6', o, end - o);
		const v4 = { 0: 'Echo reply', 3: 'Destination unreachable', 8: 'Echo request', 11: 'Time exceeded' };
		const v6 = { 1: 'Destination unreachable', 128: 'Echo request', 129: 'Echo reply', 133: 'Router Solicitation', 134: 'Router Advertisement', 135: 'Neighbor Solicitation', 136: 'Neighbor Advertisement', 143: 'Multicast Listener Report v2' };
		const what = (proto === 1 ? v4 : v6)[ty] || `type ${ty}`;
		field(l, 'Type', `${ty} (${what})`, o, 1);
		field(l, 'Code', code, o + 1, 1);
		t.proto = proto === 1 ? 'ICMP' : 'ICMPv6';
		t.info = what + ((ty === 135 || ty === 136) && end - o >= 24 ? ` for ${ip6At(d, o + 8)}` : '');
		return;
	}
	t.layer(`${PROTO[proto] || `Protocol ${proto}`}`, o, end - o);
	t.proto = PROTO[proto] || String(proto);
}

const DHCP_MSG = { 1: 'Discover', 2: 'Offer', 3: 'Request', 4: 'Decline', 5: 'ACK', 6: 'NAK', 7: 'Release', 8: 'Inform' };
const DHCP_OPT = { 1: 'Subnet mask', 3: 'Router', 6: 'Domain name servers', 12: 'Host name', 15: 'Domain name', 28: 'Broadcast address', 42: 'NTP servers', 43: 'Vendor-specific',
	50: 'Requested IP', 51: 'Lease time', 53: 'Message type', 54: 'Server identifier', 55: 'Parameter request list', 57: 'Maximum message size', 58: 'Renewal time', 59: 'Rebinding time',
	60: 'Vendor class', 61: 'Client identifier', 81: 'Client FQDN', 82: 'Relay agent information', 119: 'Domain search', 121: 'Classless static routes', 145: 'Forcerenew nonce' };
function dhcp(t, o, end) {
	const d = t.d;
	const l = t.layer('Dynamic Host Configuration Protocol', o, end - o);
	const xid = hex(u32(d, o + 4), 8);
	field(l, 'Message', d[o] === 1 ? 'Boot request (1)' : 'Boot reply (2)', o, 1);
	field(l, 'Transaction ID', xid, o + 4, 4);
	field(l, 'Seconds', u16(d, o + 8), o + 8, 2);
	field(l, 'Flags', u16(d, o + 10) & 0x8000 ? 'Broadcast' : 'Unicast', o + 10, 2);
	field(l, 'Client IP', ip4At(d, o + 12), o + 12, 4);
	field(l, 'Your IP', ip4At(d, o + 16), o + 16, 4);
	field(l, 'Server IP', ip4At(d, o + 20), o + 20, 4);
	field(l, 'Relay IP', ip4At(d, o + 24), o + 24, 4);
	field(l, 'Client MAC', macAt(d, o + 28), o + 28, 6);
	t.proto = 'DHCP';
	let at = o + 240, msg = 0, req = '';
	const host = [];
	while (at < end && d[at] !== 255) {
		const code = d[at];
		if (code === 0) {
			at++;
			continue;
		}
		const len = d[at + 1], v = at + 2;
		let value = `${len} bytes`;
		if (code === 53) {
			msg = d[v];
			value = `${DHCP_MSG[msg] || msg}`;
		} else if ([1, 3, 6, 28, 42, 50, 54].includes(code)) {
			const ips = [];
			for (let i = 0; i + 4 <= len; i += 4) ips.push(ip4At(d, v + i));
			value = ips.join(', ');
			if (code === 50) req = value;
		} else if (code === 51 || code === 58 || code === 59) value = `${u32(d, v)} s`;
		else if (code === 12 || code === 15 || code === 60) {
			value = new TextDecoder().decode(d.subarray(v, v + len));
			if (code === 12) host.push(value);
		} else if (code === 55) value = [...d.subarray(v, v + len)].join(', ');
		field(l, `Option ${code}: ${DHCP_OPT[code] || 'unknown'}`, value, at, 2 + len);
		at += 2 + len;
	}
	const yi = ip4At(d, o + 16);
	t.info = `DHCP ${DHCP_MSG[msg] || '?'} ${xid}${yi !== '0.0.0.0' ? `, ${yi}` : req ? `, asks ${req}` : ''}${host.length ? ` (${host[0]})` : ''}`;
}

const DNS_TYPE = { 1: 'A', 2: 'NS', 5: 'CNAME', 6: 'SOA', 12: 'PTR', 15: 'MX', 16: 'TXT', 28: 'AAAA', 33: 'SRV', 47: 'NSEC', 64: 'SVCB', 65: 'HTTPS', 255: 'ANY' };
const RCODE = { 0: 'No error', 1: 'Format error', 2: 'Server failure', 3: 'No such name', 4: 'Not implemented', 5: 'Refused' };
function dnsName(d, at, base) {
	const parts = [];
	let jumped = false, end = at, guard = 0;
	while (at < d.length && guard++ < 64) {
		const n = d[at];
		if (n === 0) {
			if (!jumped) end = at + 1;
			break;
		}
		if ((n & 0xc0) === 0xc0) {
			if (!jumped) end = at + 2;
			at = base + (((n & 0x3f) << 8) | d[at + 1]);
			jumped = true;
			continue;
		}
		parts.push(new TextDecoder().decode(d.subarray(at + 1, at + 1 + n)));
		at += 1 + n;
	}
	return [parts.join('.') || '<root>', end];
}
function dns(t, o, end, mdns) {
	const d = t.d;
	const flags = u16(d, o + 2), qd = u16(d, o + 4), an = u16(d, o + 6);
	const l = t.layer(mdns ? 'Multicast Domain Name System' : 'Domain Name System', o, end - o);
	const answer = !!(flags & 0x8000), rc = flags & 15;
	field(l, 'Transaction ID', hex(u16(d, o), 4), o, 2);
	field(l, 'Flags', `${hex(flags, 4)}: ${answer ? 'response' : 'query'}${answer ? `, ${RCODE[rc] || `rcode ${rc}`}` : ''}`, o + 2, 2);
	field(l, 'Questions', qd, o + 4, 2);
	field(l, 'Answers', an, o + 6, 2);
	t.proto = mdns ? 'MDNS' : 'DNS';
	let at = o + 12;
	const qs = [];
	for (let i = 0; i < qd && at < end; i++) {
		const [name, next] = dnsName(d, at, o);
		const ty = u16(d, next);
		field(l, 'Question', `${name} ${DNS_TYPE[ty] || ty}`, at, next + 4 - at);
		qs.push(`${DNS_TYPE[ty] || ty} ${name}`);
		at = next + 4;
	}
	const as = [];
	for (let i = 0; i < an && at < end; i++) {
		const [name, next] = dnsName(d, at, o);
		const ty = u16(d, next), rl = u16(d, next + 8), v = next + 10;
		const value = ty === 1 && rl === 4 ? ip4At(d, v) : ty === 28 && rl === 16 ? ip6At(d, v) : ty === 5 || ty === 12 ? dnsName(d, v, o)[0] : `${rl} bytes`;
		field(l, 'Answer', `${name} ${DNS_TYPE[ty] || ty} ${value}`, at, v + rl - at);
		as.push(value);
		at = v + rl;
	}
	t.info = `${answer ? 'Response' : 'Query'} ${hex(u16(d, o), 4)} ${qs.join(', ')}${answer ? (rc ? ` ${RCODE[rc] || rc}` : as.length ? ` → ${as.slice(0, 3).join(', ')}` : '') : ''}`;
}
