// What the AP's Bluetooth radio hears, device by device (0157): what an
// advertisement says, read as the AP's own survey (airscan) reads it; the
// table of devices heard that aeolus-ble keeps while it lends the radio
// (0156); and the list the agent reports, made of that table and the
// survey's together. A device whose address rotates, as phones and
// computers keep theirs private, is a new device every few minutes: those
// are reported as a count by what they are from, not one by one.
//
// Pure, so that it is tested anywhere.

'use strict';

const HEARD_FOR = 600;    // seconds a device heard is reported for
const MOST = 64;          // devices reported one by one, the strongest
const GROUPS = 24;        // groups of rotating addresses reported
const MFR = 32;           // bytes of manufacturer data kept

function le16(s, at) {
	return ord(s, at) | (ord(s, at + 1) << 8);
}

// hexed is bytes from..to of s, in hex.
function hexed(s, from, to) {
	let out = '';
	for (let i = from; i < to; i++)
		out += sprintf('%02x', ord(s, i));
	return out;
}

// whole is a number rounded to the nearest whole one, as the manager takes
// it: the survey gives an average signal with a fraction.
function whole(x) {
	return type(x) == 'double' ? int(x < 0 ? x - 0.5 : x + 0.5) : type(x) == 'int' ? x : null;
}

// dbm is a signal a Bluetooth radio can say, in dBm, from -127 to 20
// (0241), or null: 127 means it has none, and anything else past the ends
// is an advertisement read wrong, such as the 99 dBm a fridge was once
// "heard" at, which the manager refuses.
function dbm(x) {
	x = whole(x);
	return x != null && x >= -127 && x <= 20 ? x : null;
}

// random_kind says what kind of random address a MAC is, by its two top
// bits: static, or one kept private that rotates, resolvable or not.
function random_kind(mac) {
	let top = hex(substr(mac, 0, 2)) >> 6;
	return top == 3 ? 'static' : top == 1 ? 'resolvable' : top == 0 ? 'non-resolvable' : 'reserved';
}

function add16(d, u) {
	let x = sprintf('%04x', u);
	d.uuid16 ??= [];
	if (!(x in d.uuid16) && length(d.uuid16) < 8)
		push(d.uuid16, x);
}

// A 128-bit UUID as it is written: its bytes come the last first.
function uuid128(s, at) {
	let b = [];
	for (let k = 15; k >= 0; k--)
		push(b, sprintf('%02x', ord(s, at + k)));
	return join('', slice(b, 0, 4)) + '-' + join('', slice(b, 4, 6)) + '-' + join('', slice(b, 6, 8)) + '-' +
		join('', slice(b, 8, 10)) + '-' + join('', slice(b, 10, 16));
}

// printable is a name as a device gave it, without control characters.
function printable(s) {
	let out = '';
	for (let i = 0; i < length(s); i++) {
		let c = ord(s, i);
		if (c >= 0x20 && c != 0x7f)
			out += chr(c);
	}
	return out;
}

// parse reads an advertisement's data: its flags, name, transmit power,
// appearance, manufacturer (the company and what follows, in hex), service
// UUIDs, a beacon it is, and a tracker it is taken to be. The fields and
// their names are airscan's.
function parse(data) {
	let d = {}, i = 0, n = length(data);
	while (i < n) {
		let l = ord(data, i);
		if (!l || i + 1 + l > n)
			break;
		let t = ord(data, i + 1), at = i + 2, vl = l - 1;
		if (t == 0x01 && vl >= 1)
			d.flags = ord(data, at);
		else if (t == 0x02 || t == 0x03) {
			for (let k = 0; k + 1 < vl; k += 2)
				add16(d, le16(data, at + k));
		} else if (t == 0x06 || t == 0x07) {
			d.uuid128 ??= [];
			for (let k = 0; k + 15 < vl && length(d.uuid128) < 4; k += 16)
				push(d.uuid128, uuid128(data, at + k));
		} else if ((t == 0x08 || t == 0x09) && vl > 0 && (t == 0x09 || !d.name_complete)) {
			d.name = printable(substr(data, at, vl));
			d.name_complete = t == 0x09;
		} else if (t == 0x0a && vl >= 1) {
			let v = ord(data, at);
			d.tx_power = v > 127 ? v - 256 : v;
		} else if (t == 0x16 && vl >= 2) {
			let u = le16(data, at);
			add16(d, u);
			if (u == 0xfeaa && vl >= 3) {
				let f = ord(data, at + 2);
				d.beacon = { type: 'eddystone', frame: f == 0x00 ? 'uid' : f == 0x10 ? 'url' : f == 0x20 ? 'tlm' : f == 0x30 ? 'eid' : (f == 0x40 || f == 0x41) ? 'fmdn' : 'other' };
				if (f == 0x40 || f == 0x41)
					d.tracker = 'google_find_my_device';
			}
		} else if (t == 0x19 && vl >= 2)
			d.appearance = le16(data, at);
		else if (t == 0xff && vl >= 2) {
			d.company = le16(data, at);
			d.mfr_data = hexed(data, at + 2, at + 2 + min(vl - 2, MFR));
			if (d.company == 0x004c && vl >= 25 && ord(data, at + 2) == 0x02 && ord(data, at + 3) == 0x15) {
				let u = hexed(data, at + 4, at + 20);
				let tx = ord(data, at + 24);
				d.beacon = {
					type: 'ibeacon', uuid: substr(u, 0, 8) + '-' + substr(u, 8, 4) + '-' + substr(u, 12, 4) + '-' + substr(u, 16, 4) + '-' + substr(u, 20),
					major: (ord(data, at + 20) << 8) | ord(data, at + 21), minor: (ord(data, at + 22) << 8) | ord(data, at + 23), tx_1m: tx > 127 ? tx - 256 : tx,
				};
			}
			if (d.company == 0x004c && vl >= 3 && ord(data, at + 2) == 0x12)
				d.tracker = 'apple_find_my';
		}
		i += 1 + l;
	}
	for (let u in d.uuid16 ?? []) {
		if (u == 'feed' || u == 'feec')
			d.tracker = 'tile';
		else if (u == 'fd5a')
			d.tracker = 'samsung_smarttag';
	}
	return d;
}

// note puts an advertisement heard into the table: a, as the daemon has it
// ({ mac, type: 0 public or 1 random, rssi, data }), at now. What it says
// is kept as the latest it said; a complete name over a short one.
function note(table, a, now) {
	let mac = lc(a.mac);
	let d = table[mac];
	if (!d) {
		d = table[mac] = { addr: mac, type: a.type ? 'random' : 'public', first_seen: now, seen_total: 0 };
		if (a.type)
			d.random = random_kind(mac);
	}
	d.last_seen = now;
	d.seen_total++;
	let rssi = dbm(a.rssi);
	if (rssi != null) {
		d.rssi = rssi;
		d.rssi_max = d.rssi_max == null ? rssi : max(d.rssi_max, rssi);
		d.rssi_ewma = d.rssi_ewma == null ? rssi : d.rssi_ewma * 0.9 + rssi * 0.1;
	}
	let p = parse(a.data ?? '');
	for (let k, v in p)
		if (k != 'name' && k != 'name_complete')
			d[k] = v;
	if (p.name != null && (p.name_complete || !d.name_complete)) {
		d.name = p.name;
		d.name_complete = p.name_complete;
	}
	return d;
}

// forget lets go what was last heard more than keep seconds ago, and the
// oldest past most.
function forget(table, now, keep, most) {
	for (let mac, d in table)
		if (now - d.last_seen > keep)
			delete table[mac];
	let all = keys(table);
	if (length(all) > most) {
		all = sort(all, (x, y) => table[x].last_seen - table[y].last_seen);
		for (let i = 0; i < length(all) - most; i++)
			delete table[all[i]];
	}
}

// listing is the table as airscan lists what its survey heard.
function listing(table) {
	let out = [];
	for (let mac, d in table) {
		let x = { ...d };
		x.rssi_avg = whole(d.rssi_ewma);
		delete x.rssi_ewma;
		delete x.name_complete;
		push(out, x);
	}
	return out;
}

// What a device heard by the survey adds to one the table has.
const FILLS = ['name', 'tracker', 'beacon', 'company', 'mfr_data', 'uuid16', 'uuid128', 'appearance', 'tx_power', 'flags'];

// merge takes the table's devices and the survey's together: one for each
// address, the table's where both have it, with what the survey heard
// that the table did not, such as a name a device gives only when asked.
function merge(table, survey) {
	let by = {}, out = [];
	for (let d in table ?? []) {
		let x = { ...d };
		by[lc(d.addr)] = x;
		push(out, x);
	}
	for (let d in survey ?? []) {
		let x = by[lc(d.addr ?? '')];
		if (!x) {
			push(out, { ...d, addr: lc(d.addr ?? '') });
			continue;
		}
		for (let k in FILLS)
			if (x[k] == null && d[k] != null)
				x[k] = d[k];
	}
	return out;
}

// rotates says whether an address is one kept private, which a device
// changes every few minutes. A tracker is reported one by one all the same.
function rotates(d) {
	return d.type == 'random' && d.random != 'static' && !d.tracker;
}

// reported is a device as the agent reports it: when it was first and last
// heard, as seconds ago.
function reported(d, now) {
	let out = {
		mac: d.addr, type: d.type, random: d.random, rssi: dbm(d.rssi), rssi_avg: dbm(d.rssi_avg), rssi_max: dbm(d.rssi_max),
		seen: whole(d.seen_total) ?? 0, first_ago: max(0, whole(now - (d.first_seen ?? now))), last_ago: max(0, whole(now - (d.last_seen ?? now))),
		name: d.name, flags: d.flags, tx_power: d.tx_power, appearance: d.appearance, company: d.company,
		mfr: d.mfr_data == null ? null : substr(d.mfr_data, 0, MFR * 2),
		uuid16: d.uuid16 == null ? null : slice(d.uuid16, 0, 8), uuid128: d.uuid128 == null ? null : slice(d.uuid128, 0, 4),
		beacon: d.beacon, tracker: d.tracker,
	};
	for (let k in keys(out))
		if (out[k] == null || out[k] == '')
			delete out[k];
	return out;
}

// fold is what the agent reports of the devices heard in the last ten
// minutes: { devices, rotating }. devices are those that keep their address
// and the trackers, the strongest first, MOST at the most; rotating are the
// others, a count for each company, first byte of its data and first
// service UUID, the most first, GROUPS at the most.
function fold(list, now) {
	let fixed = [], groups = {};
	for (let d in list ?? []) {
		if (type(d?.addr) != 'string' || now - (d.last_seen ?? now) > HEARD_FOR)
			continue;
		if (!rotates(d)) {
			push(fixed, reported(d, now));
			continue;
		}
		let key = sprintf('%s/%s/%s', d.company ?? '', substr(d.mfr_data ?? '', 0, 2), d.uuid16?.[0] ?? '');
		let g = groups[key];
		if (!g) {
			g = groups[key] = { company: d.company, mfr: d.mfr_data == null ? null : substr(d.mfr_data, 0, 6), uuid16: d.uuid16?.[0], count: 0 };
			for (let k in keys(g))
				if (g[k] == null)
					delete g[k];
		}
		g.count++;
		if (dbm(d.rssi) != null)
			g.rssi_max = g.rssi_max == null ? dbm(d.rssi) : max(g.rssi_max, dbm(d.rssi));
		let ago = max(0, whole(now - (d.last_seen ?? now)));
		g.last_ago = g.last_ago == null ? ago : min(g.last_ago, ago);
	}
	fixed = sort(fixed, (x, y) => (y.rssi ?? -200) - (x.rssi ?? -200));
	let rotating = sort(values(groups), (x, y) => y.count - x.count || (y.rssi_max ?? -200) - (x.rssi_max ?? -200));
	return { devices: slice(fixed, 0, MOST), rotating: slice(rotating, 0, GROUPS) };
}

export { HEARD_FOR, random_kind, dbm, parse, note, forget, listing, merge, fold };
