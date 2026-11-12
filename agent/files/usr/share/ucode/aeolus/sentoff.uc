// What Aeolus's own daemons sent a client off for (0255), so the record of
// its leaving says so, and not only what the frames said: a client let go
// of here because it joined another AP (radio resource management, 0226,
// which sends no frame), one a person disconnected (0107), one steering put
// off (0138), one whose network was taken off the air (0201). Each daemon
// writes a file of its own, so no two write one file; the journey daemon
// reads them all as hostapd lets a client go.
//
// And what they keep a client off (Keep on 6 GHz, 0300): hostapd, asked to
// keep a client off a network, sends it a disassociation there each time,
// whether or not it is on it. A client kept on 6 GHz, listening there,
// never hears one, and so never acknowledges it; the journey daemon knows
// those frames by this, and does not trace each as the client leaving.

'use strict';

import * as fs from 'fs';

const DIR = '/var/run/aeolus/sent-off';
const KEEP = 60;    // seconds an entry is kept: hostapd lets go of the client within a second or two
const NEAR = 15;    // seconds within which an entry is of a leaving
const MOST = 256;   // entries a file keeps, the latest

// WHY is each word a daemon may give, and the words they stand for; one
// not here is taken as it is, so a daemon can say a new one without this
// file being changed first.
const WHY = {
	moved: 'let go of: it joined another AP',
	asked: 'disconnected by a person',
	steered: 'put off by steering',
	'off-air': 'its network was taken off the air',
};

function read(path) {
	try {
		return json(fs.readfile(path) ?? 'null');
	} catch (e) {
		return null;
	}
}

// note says that daemon who, as a file name, sent client mac off its
// network bss, for why, a word of WHY's or another.
function note(who, mac, bss, why) {
	if (!match(who ?? '', /^[a-z0-9-]{1,24}$/) || !match(mac ?? '', /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) || !match(why ?? '', /^[a-z-]{1,24}$/))
		return false;
	fs.mkdir(DIR);
	let path = `${DIR}/${who}.json`, t = time(), was = read(path), list = {};
	for (let k, e in (type(was) == 'object' ? was : {}))
		if (type(e?.at) == 'int' && t - e.at <= KEEP)
			list[k] = e;
	list[mac] = { bss: bss ?? null, why: why, at: t };
	let ks = sort(keys(list), (a, b) => list[a].at - list[b].at);
	while (length(ks) > MOST)
		delete list[shift(ks)];
	let tmp = path + '.tmp';
	if (fs.writefile(tmp, sprintf('%J', list)) == null)
		return false;
	return fs.rename(tmp, path);
}

// why is what Aeolus sent client mac off network bss for, where a daemon
// said so in the last NEAR seconds: the word, or null.
function why(mac, bss) {
	let t = time(), best = null;
	for (let f in fs.lsdir(DIR) ?? []) {
		if (!match(f, /^[a-z0-9-]+\.json$/))
			continue;
		let e = read(`${DIR}/${f}`)?.[mac];
		if (type(e?.at) != 'int' || t - e.at > NEAR || (e.bss != null && bss != null && e.bss != bss))
			continue;
		if (!best || e.at > best.at)
			best = e;
	}
	return best ? best.why : null;
}

// What a daemon keeps a client off: a file of each daemon's in KEPT_DIR,
// { "<mac> <network>": { since, until } }, by the calendar's clock in
// seconds. KEPT_GRACE is how long an entry still holds past its until, and
// before its since: the frames of an asking go within a second of it, and
// are read a little after.
const KEPT_DIR = '/var/run/aeolus/kept-off';
const KEPT_GRACE = 5;

// kept_list is a daemon's list of what it keeps off, was, with client mac
// kept off network bss for secs seconds from t; 0 lifts it, and the entry
// still holds for the frames of the lifting. A keeping renewed before it
// ran out keeps when it began. Those that ran out are left out, and the
// oldest past MOST.
function kept_list(was, mac, bss, secs, t) {
	let list = {}, key = mac + ' ' + bss;
	for (let k, e in (type(was) == 'object' ? was : {}))
		if (type(e?.since) == 'int' && type(e?.until) == 'int' && e.until + KEPT_GRACE >= t)
			list[k] = e;
	list[key] = { since: list[key]?.since ?? t, until: t + max(0, int(secs + 0.5)) };
	let ks = sort(keys(list), (a, b) => list[a].until - list[b].until);
	while (length(ks) > MOST)
		delete list[shift(ks)];
	return list;
}

// kept says whether a daemon's list has client mac kept off network bss at
// t, in seconds by the calendar.
function kept(list, mac, bss, t) {
	let e = type(list) == 'object' ? list[mac + ' ' + bss] : null;
	return type(e?.since) == 'int' && type(e?.until) == 'int' && t >= e.since - KEPT_GRACE && t <= e.until + KEPT_GRACE;
}

// keep_off says that daemon who keeps client mac off network bss, by
// hostapd's ban, for secs seconds from now; 0 lifts it.
function keep_off(who, mac, bss, secs) {
	if (!match(who ?? '', /^[a-z0-9-]{1,24}$/) || !match(mac ?? '', /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) || !match(bss ?? '', /^[A-Za-z0-9_.-]{1,32}$/) ||
		!(type(secs) in ['int', 'double']))
		return false;
	fs.mkdir('/var/run/aeolus');
	fs.mkdir(KEPT_DIR);
	let path = `${KEPT_DIR}/${who}.json`, tmp = path + '.tmp';
	if (fs.writefile(tmp, sprintf('%J', kept_list(read(path), mac, bss, secs, time()))) == null)
		return false;
	return fs.rename(tmp, path);
}

// kept_off says whether one of Aeolus's daemons kept client mac off network
// bss at t, in seconds by the calendar (now, unless said): what a frame
// hostapd sent it there then was for.
function kept_off(mac, bss, t) {
	t ??= time();
	for (let f in fs.lsdir(KEPT_DIR) ?? [])
		if (match(f, /^[a-z0-9-]+\.json$/) && kept(read(`${KEPT_DIR}/${f}`), mac, bss, t))
			return true;
	return false;
}

export { WHY, note, why, KEPT_GRACE, kept_list, kept, keep_off, kept_off };
