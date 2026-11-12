// aeolus.measure: what a client hears of the AP, as it says itself (802.11k).
//
// The AP hears a client's signal on the way in; what the client hears of
// the AP on the way out is what matters to the client, and is otherwise
// only reckoned from it (0206). A client that takes beacon requests (its
// RM Enabled Capabilities say beacon measurement) is asked, now and then,
// to measure the beacons of the AP it is on and say how strongly it hears
// them: hostapd sends the request (rrm_beacon_req) and says the report on
// its control socket, BEACON-RESP-RX, to whoever is attached. This module
// is the judgement and nothing else: what to ask, and what was said.

'use strict';

const EVERY = 300;      // seconds between two askings of one client
const FRESH = 900;      // seconds what a client said is gone by
const DURATION = 120;   // TU the client is asked to listen for: more than one beacon interval
const PER_MINUTE = 8;   // clients an AP asks a minute, the longest unasked first
const STRONGEST = -20;  // dBm stronger than which no client hears an AP: a report saying so is read as a signed dBm
const WEAKEST = -100;   // dBm weaker than which none is heard at all

// mode is how a client is asked to measure (802.11-2020 9.4.2.45), by its
// RM Enabled Capabilities' first byte: passive (it listens for beacons) or
// active (it asks with a probe) where it can, else from its table of what
// it heard last; null where it takes no beacon request.
function mode(rrm) {
	let b = type(rrm) == 'array' ? rrm[0] : null;
	if (type(b) != 'int')
		return null;
	return (b & 0x10) ? 0 : (b & 0x20) ? 1 : (b & 0x40) ? 2 : null;
}

// op_class is the global operating class of a 20 MHz channel (802.11-2020
// Annex E, table E-4), as a beacon request names the channel: on 2.4 GHz
// 81 (82 for 14), on 5 GHz by its block, on 6 GHz 131.
function op_class(freq, channel) {
	if (type(freq) != 'int' || type(channel) != 'int')
		return null;
	if (freq < 3000)
		return channel == 14 ? 82 : 81;
	if (freq >= 5950)
		return 131;
	return channel <= 48 ? 115 : channel <= 64 ? 118 : channel <= 144 ? 121 : 125;
}

// rcpi_dbm is the signal a beacon report's RCPI says, in dBm: [dbm,
// quirk], or null where it says none. RCPI is half dBs above -110. Some
// clients write the signal itself there, in dBm as a signed byte
// (2026-10-23: a Samsung phone and another said 0xc2, -62 dBm, which as
// RCPI is -13 while the AP heard them at -57 and -63). A signal stronger
// than STRONGEST is read so; one that is still not a signal is passed over
// (quirk says it was read so).
function rcpi_dbm(rcpi) {
	if (type(rcpi) != 'int' || rcpi < 0 || rcpi > 220)
		return null;
	let dbm = int(rcpi / 2) - 110 + (rcpi % 2 ? 0.5 : 0), quirk = false;
	if (dbm > STRONGEST) {
		dbm = rcpi - 256;
		quirk = true;
	}
	return dbm > STRONGEST || dbm < WEAKEST ? null : [dbm, quirk];
}

// report reads one of hostapd's BEACON-RESP-RX lines: the client, the
// request's token, the report mode, and the Beacon report itself (802.11
// 9.4.2.21.7): its operating class, channel, the RCPI the client measured,
// as dBm, its RSNI, and the BSSID it measured. null where the line is
// another, or a report the client could not, or would not, make (a mode
// with late, incapable or refused set). The RCPI is read by rcpi_dbm.
function report(line) {
	let m = match(line ?? '', /BEACON-RESP-RX ([0-9a-f]{2}(:[0-9a-f]{2}){5}) ([0-9]+) ([0-9a-f]{2}) ([0-9a-f]*)/);
	if (!m)
		return null;
	let mode = hex(m[4]), body = m[5];
	if (mode & 0x07 || length(body) < 52)
		return null;
	// Its bytes: operating class, channel, the measurement's start (8) and
	// duration (2), the reported frame's information, RCPI, RSNI, BSSID (6).
	let b = (i) => hex(substr(body, i * 2, 2));
	let d = rcpi_dbm(b(13));
	if (!d)
		return null;
	let bssid = join(":", map([15, 16, 17, 18, 19, 20], i => substr(body, i * 2, 2)));
	let out = { mac: m[1], token: int(m[3]), op_class: b(0), channel: b(1), dbm: d[0], rsni: b(14) == 255 ? null : b(14), bssid: lc(bssid) };
	if (d[1])
		out.quirk = true;
	return out;
}

// notified reads hostapd's beacon-report notification over ubus, as a
// listener to a network hears it (0289): the client, the operating class
// and channel measured, the signal the client heard there, in dBm, and the
// BSSID it measured. null where it is no report, or one the client could
// not or would not make (a report mode with late, incapable or refused
// set), or one whose RCPI says no signal.
function notified(data) {
	let mac = lc(data?.address ?? ''), bssid = lc(data?.bssid ?? '');
	let mac_re = /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/;
	if (!match(mac, mac_re) || !match(bssid, mac_re) || ((data['rep-mode'] ?? 0) & 0x07) || type(data.channel) != 'int' || type(data['op-class']) != 'int')
		return null;
	let d = rcpi_dbm(data.rcpi);
	if (!d)
		return null;
	let out = { mac: mac, op_class: data['op-class'], channel: data.channel, dbm: d[0], bssid: bssid };
	if (d[1])
		out.quirk = true;
	return out;
}

// due is the clients to ask now, of those that take a request: each { mac,
// mode, asked, at }, at when it was last asked; the longest unasked first,
// at most most.
function due(clients, now, most) {
	let out = filter(clients ?? [], c => c.mode != null && (c.asked == null || now - c.asked >= EVERY));
	out = sort(out, (a, b) => (a.asked ?? 0) - (b.asked ?? 0) || (a.mac < b.mac ? -1 : 1));
	return slice(out, 0, most ?? PER_MINUTE);
}

// hears is what a client hears of the AP now, from what it said (a report,
// with the AP's power when it was asked) and the AP's power now: a change
// of power since is a change of as much in what it hears. null where what
// it said is not fresh, or not of the BSSID it is on.
function hears(said, now, power, bssid) {
	if (type(said?.dbm) != 'double' && type(said?.dbm) != 'int')
		return null;
	if (now - (said.at ?? 0) > FRESH || (bssid && said.bssid && lc(bssid) != said.bssid))
		return null;
	let d = said.dbm + (type(power) == 'int' && type(said.power) == 'int' ? power - said.power : 0);
	return d >= 0 ? int(d + 0.5) : -int(-d + 0.5);
}

export { EVERY, FRESH, DURATION, PER_MINUTE, mode, op_class, report, notified, due, hears };
