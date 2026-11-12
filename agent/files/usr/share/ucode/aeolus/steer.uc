// aeolus.steer: what Aeolus's steering asks of a Wi-Fi client, and what
// came of it (0138). It is the daemon's judgement and nothing else: no
// ubus, no files, no clock. aeolus-steer hears hostapd and looks at its
// clients, hands what it found here, and does what comes back; a test
// hands it the same.
//
// Four ways, each a switch of the network, so of one SSID, and each on its
// own:
//
//   suggest   a client that can use a higher band this AP has the network
//             on, and is near enough for it, is asked to move there (an
//             802.11v BSS transition request). It may say no.
//   hold      a client known to use a higher band is kept off 2.4 GHz for a
//             moment as it comes to join, so that it finds the higher band
//             first. Then 2.4 GHz is open to it again.
//   sticky    a client whose signal stays weak while the network has a
//             better place for it, or that another AP of the network hears
//             more strongly by its probes (0289), is asked to go, and put
//             off for a moment if it stays.
//   keep      a client on 6 GHz here, heard well, is kept off the network's
//             lower bands on this AP while it is so (0300), as a phone's own
//             roaming would take it back to the louder 5 GHz.
//
// The APs tell one another, by RRM's hellos, how strongly they hear the
// clients they do not serve, as those clients' probes say: so a client is
// weighed by how each AP hears it, and needs nothing of 802.11k (0289).
//
// Nothing here keeps a client off for longer than a few seconds at a time
// (keep renews its own while the client stays on 6 GHz, heard well), and
// nothing is asked of hostapd that it cannot undo by itself: what keeps a
// client off is hostapd's own ban, which hostapd lifts by its own timer,
// the daemon there or not. hostapd never waits for the daemon.
//
// Each asking is followed to its end and kept as one record: what was
// asked, what the client answered, and where it then was. Those are what
// Aeolus shows of whether steering works.

'use strict';

import * as measure from 'aeolus.measure';   // what a client says it hears (802.11k)

const RANK = { '2g': 0, '5g': 1, '6g': 2 };

// The mark of an AP's own test client (0121): a vendor element in its
// association request, of the OUI Aeolus's beacons use (0073) and a type
// of its own, 2, at version 1. A client test puts it there; a client that
// has it is an AP's scan radio making a test, on whichever AP it joins,
// and is not steered: asked, it would say no, and held, it would say the
// network was slow to join.
const OWN_MARK = 'dd0502ae010201';

// The signal a client must have where it is, in dBm, to be asked onto a
// higher band: that band carries less far, and 6 GHz less far still.
const NEAR = { '2g': { '5g': -65, '6g': -55 }, '5g': { '6g': -60 } };

// How busy a channel may be, in percent of the last minute, and still have
// clients asked on to it (0207): one fuller has none sent to it.
const FULL = 80;

// How strongly, in dBm, a client must say it hears another AP's place of
// the network on a higher band to be sent there (802.11k, 0299): as the
// client hears the AP, which sends louder than the client does, so above
// NEAR's, which are of how the AP hears the client.
const UP_HEARD = { '5g': -62, '6g': -60 };
const UP_ASKS = 3;                 // times a client is asked what it hears of other APs' higher bands while it stays joined

// Keep on 6 GHz (0300): how well a client on 6 GHz must be heard here to be
// kept there, how many looks in a row it may be heard under that before it
// is let go, how long each of hostapd's bans on the network's lower bands
// here lasts, and how near its end, in seconds, one is renewed.
const KEEP_FLOOR = -72;
const KEEP_WEAK = 2;
const KEEP_MS = 15000;
const KEEP_RENEW = 7;

const SUGGEST_AFTER = 10;          // seconds a client is left alone once it has joined
const SUGGEST_AGAIN = [120, 600];  // seconds before it is asked a second time, and a third; then no more
const ANSWER_WAIT = 20;            // seconds it has to move
const HOLD_MS = 2500;              // how long 2.4 GHz is kept from a client
const HOLD_EVERY = 30;             // seconds before a client is held again: its next try gets in
const HOLD_WAIT = 12;              // seconds in which it joins somewhere
const WEAK_LOOKS = 3;              // looks in a row a signal is weak before the client is asked to go
const BETTER_LOOKS = 2;            // looks in a row another AP hears it better before it is asked to go there (0289)
const STICKY_EVERY = 300;          // seconds before a client is asked to go again
const HOME_AFTER = 10;             // seconds a client is on before it is sent where it belongs (0319)
const HOME_HOLD = 30;              // seconds it is then kept off here, where its rule does not say
const NEAREST_MARGIN = 5;          // dB another AP must hear a client more strongly, for its rule of the nearest (0321)
const NEAREST_BLIND = 120;         // seconds no other AP has heard a client with such a rule on its band, before it is put off to choose again
const NEAREST_BLIND_BELOW = -75;   // dBm it must be heard here under for that: one heard well where it is stays
const NEAREST_BACKS = 2;           // times a client comes back here after its rule of the nearest put it off, before that rule stops here (Griff, 2026-11-05: "stop the nearest rule after two")
// seconds the scan radio's reading of a client this AP serves holds, to set
// against its radio's (0321): as long as a reading, PROBE_FRESH. airscan
// writes its clients every 12 s or so, so its readings are at least that old
// when read; at 10 s no AP set its scan radio at all (2026-11-05).
const SCAN_FRESH = 30;
const SCAN_FOR = 7200;             // seconds a client's setting of the scan radio against the radio it is on holds
const SCAN_LEAST = 3;              // clients set so, at least, before the scan radio's readings of others are taken
const SCAN_MOST = 64;              // clients a band, the latest, whose settings are kept
const KICK_AFTER = 10;             // seconds it then has, weak
const BETTER_KICK_AFTER = 5;       // and heard better elsewhere: it has a place to go
const PROBE_FRESH = 30;            // seconds what a client's probe says of how an AP hears it holds (0289)
const PROBES_MOST = 64;            // clients heard by probe that the AP tells its neighbours of, the latest
const ASK_AFTER = 5;               // seconds a client is on before it is asked what it hears (802.11k, 0289)
// Seconds between two askings of one client, at first, and at the most:
// twice as long each time its asking moved it nowhere, as one that is
// where it is best is otherwise asked to leave its channel four times every
// five minutes for nothing.
const ASK_EVERY = 300;
const ASK_MOST = 7200;
const ASK_CHANNELS = 4;            // channels it is asked of at once: its own, and the network's places on its band
// Seconds the weak rule waits, once a client is asked what it hears, for
// what it says (0289): asked at its second look, it would be put off for a
// weak signal at its third, before what it said had had its two looks.
const REPORT_WAIT = 10;
// A frequency of each band, by which a channel's operating class is told
// (measure.op_class).
const BAND_FREQ = { '2g': 2437, '5g': 5500, '6g': 6000 };
const RECOVERED = 3;               // dB over weak, or over the floor, at which a client is left where it is

// Sticky's settings (0289), each with its bounds and its value unset:
//   weak       the signal, in dBm, under which a client no other AP is
//              heard to hear better is weak, and told to go (0138);
//   margin     how much more strongly, in dB, another AP of the network
//              must hear a client, on its band, for it to be told to go
//              there;
//   floor      the signal here, in dBm, under which that is done: a client
//              heard well here is not moved for a few dB;
//   hold_down  how long, in seconds, a client put off is kept off the
//              network here: on every band, where its place is on another
//              AP, so that its tries go there. A client refused three times
//              running, with nowhere else to go, gives the network up for
//              ten seconds (wpa_supplicant, 2026-10-11); one that comes
//              straight back is left alone for half an hour, and longer
//              each time after.
const STICKY = {
	weak: { least: -95, most: -50, unset: -80 },
	margin: { least: 1, most: 20, unset: 5 },
	floor: { least: -90, most: -40, unset: -65 },
	hold_down: { least: 1, most: 60, unset: 10 },
};

// sticky_of is one of a network's sticky settings, as unset where it has
// none.
function sticky_of(net, k) {
	let v = net?.[k];
	return type(v) in ['int', 'double'] ? v : STICKY[k].unset;
}
const BACK_WAIT = 60;              // seconds in which it may come back to where it was
const FELL_BACK = 300;             // seconds in which a client that moved may come back down
const REST = 1800;                 // seconds a client that came back is left alone, the first time
const REST_MOST = 86400;           // and at the most: twice as long each time it comes back again
const BACKS_FOR = 7 * 86400;       // seconds its comings back are remembered
const BACKS_MOST = 64;             // comings back counted at the most, as a restart reads them back: a day's rest is reached at the seventh
// A client sticky put off that comes back to the network here within ten
// minutes came back (0289), though its asking ended as left a minute on: an
// ESP32 put off for ten seconds backed off for longer than that before it
// tried again, at the one AP that hears it (2026-11-03).
const COME_BACK = 600;
// How long another AP's hearing a client is a sign it has somewhere better
// to go, for the weak rule (0289): a day.
const ELSEWHERE_FOR = 86400;
const ELSEWHERE_MOST = 4096;       // clients that is kept of, the latest
const KNOWN_FOR = 30 * 86400;      // seconds a client's bands are remembered
const KNOWN_MOST = 4096;
const EVENTS_KEPT = 200;
const EVENTS_SAID = 50;

function fresh() {
	return {
		known: {},    // each client's bands, and whether it takes a transition request
		on: {},       // where each client is on this AP now
		tries: {},    // what is asked of a client and not yet at its end, one a client
		held: {},     // when each was last held off 2.4 GHz
		kept: {},     // each kept on 6 GHz (0300): its network and 6 GHz BSS, since when, looks heard weakly, its signal, and until when each lower band here is barred to it
		asked: {},    // when each was last asked to go for a weak signal
		moved: {},    // where each last moved from, for whether it comes back
		rest: {},     // until when each is left alone
		backs: {},    // how often each came back, and when last: it is left alone longer each time
		down: {},     // until when each, sent down for a weak signal, is not asked or held up
		downs: {},    // how often each was sent down, and when last: it is left down longer each time
		probes: {},   // how strongly each client not on this AP is heard by its probes, by band (0289)
		scans: {},    // and by the scan radio, as the radio it would be on would hear it, by band, for a client with a rule of the nearest (0321)
		cal: {},      // by band: each client this AP serves, what the radio it is on hears more than the scan radio does, and when (0321)
		reports: {},  // what each client on this AP said it hears, by BSSID, and of this AP (802.11k, 0289)
		reported: {}, // when each was last asked what it hears
		ask_gap: {},  // and how long until it is asked again, twice as long each time (0289)
		ask_bss: {},  // and where it was then
		put_off: {},  // each client sticky put off lately: when, on which network and band, and why (0289)
		elsewhere: {}, // when another AP last heard each client, by its probes, for a day (0289)
		came_back: {}, // when each client sticky put off last came back, for BACKS_FOR (0289)
		tried: {},    // when the weak rule last tried each client no other AP heard, for BACKS_FOR (0289)
		home_backs: {}, // how often each came back here after its rule of the nearest put it off, and when last (0321)
		stopped: {},  // each client whose rule of the nearest stopped here: when, and the rule, which starts again when it changes, or after BACKS_FOR
		events: [],
		counts: {},
		wall: 0,
	};
}

// bands_of reads the bands a client says it can use from the elements of
// its association request: its Supported Operating Classes (element 59),
// by the global classes. A client need not say; one that does not is known
// only by where it has been.
function bands_of(ies) {
	let out = {}, i = 0, n = length(ies ?? '');
	while (i + 2 <= n) {
		let id = ord(ies, i), len = ord(ies, i + 1);
		if (i + 2 + len > n)
			break;
		if (id == 59)
			for (let j = 0; j < len; j++) {
				let c = ord(ies, i + 2 + j);
				if (c >= 81 && c <= 84)
					out['2g'] = true;
				else if (c >= 115 && c <= 130)
					out['5g'] = true;
				else if (c >= 131 && c <= 137)
					out['6g'] = true;
			}
		i += 2 + len;
	}
	return sort(keys(out), (a, b) => RANK[a] - RANK[b]);
}

// own_of says whether a client's association request has the mark of an
// AP's own test client.
function own_of(ies) {
	let i = 0, n = length(ies ?? '');
	while (i + 2 <= n) {
		let id = ord(ies, i), len = ord(ies, i + 1);
		if (i + 2 + len > n)
			break;
		if (id == 0xdd && len >= 4 && ord(ies, i + 2) == 0x02 && ord(ies, i + 3) == 0xae && ord(ies, i + 4) == 0x01 && ord(ies, i + 5) == 0x02)
			return true;
		i += 2 + len;
	}
	return false;
}

// btm_of says whether a client takes a BSS transition request: bit 19 of
// its extended capabilities, as hostapd lists them a byte at a time.
function btm_of(ext) {
	return type(ext) == 'array' && length(ext) > 2 && (ext[2] & 0x08) != 0;
}

function siblings(view, ifname) {
	let b = view.bss?.[ifname], out = [];
	if (!b)
		return out;
	for (let name in sort(keys(view.bss)))
		if (name != ifname && view.bss[name].network == b.network)
			push(out, { ...view.bss[name], ifname: name });
	return out;
}

function learn(st, mac, band, now) {
	let k = st.known[mac] ??= { bands: {} };
	k.bands[band] = now;
	k.at = now;
}

// ours says a client is an AP's own test client, as it said when it joined
// here, now or before.
function ours(st, mac) {
	return st.known[mac]?.own === true;
}

// learned takes in what a client said of itself as it joined, the moment
// hostapd gives it: the bands it can use, and whether it is an AP's own
// test client. It is not left to the next look: a client test is on for a
// few seconds, and may be gone by then.
function learned(st, mac, what, now) {
	for (let band in what?.bands ?? [])
		if (RANK[band] != null)
			learn(st, mac, band, now);
	if (what?.own) {
		let k = st.known[mac] ??= { bands: {} };
		k.at = now;
		k.own = true;
	}
}

function can(st, mac, band, now) {
	let t = st.known[mac]?.bands?.[band];
	return t != null && now - t < KNOWN_FOR;
}

// home_of reads one entry of where a client belongs (0319), as the
// renderer writes it: its MAC; here, where this AP is one it belongs on;
// the APs; the bands, none for any; and the hold, in seconds. It is
// [mac, rule], or null for an entry not well said, which is as none.
function home_of(entry) {
	let parts = split(trim('' + (entry ?? '')), /[ \t]+/), mac = lc(parts[0] ?? '');
	if (!match(mac, /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/))
		return null;
	let h = { here: false, nearest: false, aps: [], bands: [], hold: HOME_HOLD };
	for (let i = 1; i < length(parts); i++) {
		let x = parts[i];
		if (x == 'here') {
			h.here = true;
			continue;
		}
		// Wherever it is heard best (0321).
		if (x == 'nearest') {
			h.nearest = true;
			continue;
		}
		let m = match(x, /^(aps|bands|hold)=(.+)$/);
		if (!m)
			return null;
		if (m[1] == 'aps') {
			for (let a in split(m[2], ','))
				if (match(a, /^ap-[0-9a-f]{12}$/) && length(h.aps) < 8)
					push(h.aps, a);
				else
					return null;
		} else if (m[1] == 'bands') {
			for (let b in split(m[2], ','))
				if (RANK[b] != null)
					push(h.bands, b);
				else
					return null;
		} else {
			if (!match(m[2], /^[0-9]{1,3}$/) || +m[2] < 1 || +m[2] > 300)
				return null;
			h.hold = +m[2];
		}
	}
	return length(h.aps) || h.nearest ? [mac, h] : null;
}

// home_band says a band is one a client belongs on (0319): any, where it has
// no rule or its rule names none.
function home_band(h, band) {
	return !h || !length(h.bands) || index(h.bands, band) >= 0;
}

// home_ap says an AP is one a client belongs on: any, where it has no rule
// or its rule names none, as one of the nearest alone (0321); none that RRM
// does not know (null) where it names some.
function home_ap(h, ap) {
	return !h || !length(h.aps) || (ap != null && index(h.aps, ap) >= 0);
}

// told takes in what the other APs said, where community sharing is on
// (0299), of the bands their clients said they can use as they joined
// there: near is { mac: [{ ap, bands, age }] }, age in seconds before now.
// Each band is known from when it was said, unless it is known later.
function told(st, near, now) {
	for (let mac, list in near ?? {})
		for (let r in (type(list) == 'array' ? list : [])) {
			if (type(r?.age) != 'int' || r.age < 0 || r.age >= KNOWN_FOR || type(r.bands) != 'array')
				continue;
			let t = now - r.age;
			for (let band in r.bands)
				if (RANK[band] != null) {
					let k = st.known[mac] ??= { bands: {} };
					if (k.bands[band] == null || k.bands[band] < t)
						k.bands[band] = t;
					if (k.at == null || k.at < t)
						k.at = t;
				}
		}
}

// shares is what this AP tells the others of its clients on the networks
// with community sharing on (0299): each client on here that said, as it
// joined here, that it can use a band above 2.4 GHz, with the bands it
// said, the latest joined first. said is { mac: { bss, bands } }, as
// hostapd gave them. What this AP was told by the others is not told on,
// so no word goes round.
function shares(st, cfg, said, now) {
	let out = [];
	for (let mac in sort(keys(said ?? {}))) {
		let s = said[mac], o = st.on[mac];
		if (!o || o.bss != s?.bss || !cfg.networks?.[o.network]?.community || type(s.bands) != 'array' || !length(filter(s.bands, b => RANK[b] > 0)))
			continue;
		push(out, { mac: mac, bands: s.bands, since: o.since });
	}
	return map(sort(out, (x, y) => y.since - x.since), x => [x.mac, x.bands, 0]);
}

function count(st, network, method, what) {
	let c = st.counts[network] ??= {};
	let m = c[method] ??= {};
	m[what] = (m[what] ?? 0) + 1;
}

function said(st, e) {
	push(st.events, e);
	while (length(st.events) > EVENTS_KEPT)
		shift(st.events);
}

// longer is how long a client is left alone this time: half an hour, and
// twice as long each time after, up to a day. times counts them.
function longer(times, mac, now) {
	let b = times[mac] ??= { n: 0 };
	b.n++;
	b.at = now;
	let rest = REST;
	for (let i = 1; i < b.n && rest < REST_MOST; i++)
		rest *= 2;
	return min(rest, REST_MOST);
}

// rested leaves a client that came back alone: for half an hour, and twice
// as long each time it comes back again, up to a day. A device heard weakly
// that has nowhere better to go is then put off a few times in all, and not
// every half hour for ever.
function rested(st, mac, now) {
	st.rest[mac] = now + longer(st.backs, mac, now);
}

// sent is the daemon's word that it asked hostapd for what was decided: a
// hold is one only once hostapd was asked to keep the client off.
function sent(st, act) {
	let a = st.tries[act.mac];
	if (a && act.do == 'ban' && a.method == 'hold')
		a.sent = true;
}

// rule_key is a client's rule as it stands, for whether it changed: its
// APs, bands and hold, and whether it is of the nearest. Whether this AP
// is one it names (here) is left out, as that is the same rule.
function rule_key(h) {
	return h ? sprintf('%J', [!!h.nearest, h.aps ?? [], h.bands ?? [], h.hold ?? HOME_HOLD]) : null;
}

// nearest_back counts a client back here after its rule of the nearest put
// it off. At the second time its rule stops here (Griff, 2026-11-05):
// kicked, a client that will not go cannot be sent, and is only off the
// air each time. The garage door opener, an ESP, came back to the C260 31
// s after each of four puttings off, and never tried the AP that heard it
// 11 dB better. Said true where it stops.
function nearest_back(st, mac, rule, now) {
	let b = st.home_backs[mac] ??= { n: 0 };
	b.n++;
	b.at = now;
	if (b.n < NEAREST_BACKS)
		return false;
	st.stopped[mac] = { at: now, rule: rule };
	delete st.home_backs[mac];
	return true;
}

// stopped_here says a client's rule of the nearest has stopped here: until
// the rule changes, or for BACKS_FOR.
function stopped_here(st, mac, h, now) {
	let s = st.stopped[mac];
	if (!s)
		return false;
	if (now - s.at >= BACKS_FOR || s.rule != rule_key(h)) {
		delete st.stopped[mac];
		return false;
	}
	return true;
}

// done ends an asking: counted, and kept as a record. One made on a
// client's probe alone, which may have been only a look around, is neither
// where the client then joined nowhere here. Nor is a hold that was never
// asked of hostapd, as where the client was let on before the daemon got
// to it: nothing was done to it.
function done(st, a, outcome, now) {
	delete st.tries[a.mac];
	if (a.quiet && outcome == 'left')
		return;
	if (a.method == 'hold' && !a.sent)
		return;
	count(st, a.network, a.method, 'asked');
	count(st, a.network, a.method, outcome);
	let e = {
		at: int(a.at + st.wall), mac: a.mac, network: a.network, method: a.method, from: a.from,
		asked: a.asked, outcome: outcome, took: int((now - a.at) * 10) / 10.0,
	};
	for (let k in ['to', 'answer', 'code', 'signal', 'why', 'elsewhere'])
		if (a[k] != null)
			e[k] = a[k];
	if (outcome == 'back' && a.method == 'sticky' && a.why == 'nearest' && nearest_back(st, a.mac, a.rule, now))
		e.stopped = true;
	said(st, e);
	if (outcome == 'moved' && a.method != 'sticky')
		st.moved[a.mac] = { method: a.method, network: a.network, from: a.from, at: now };
	// Sent off a band where it was weak, it is not sent back up it: for
	// half an hour, and twice as long each time, as one that is near
	// enough on 2.4 GHz to be asked up, and weak on 5 GHz once there, would
	// otherwise go up and down every half hour.
	if (a.method == 'sticky' && !(outcome in ['recovered', 'stayed']))
		st.down[a.mac] = now + longer(st.downs, a.mac, now);
	if (outcome == 'back')
		rested(st, a.mac, now);
	if (outcome == 'back' && a.method == 'sticky')
		st.came_back[a.mac] = now;
	// Put off and gone, it may yet come back (arrived); else that is said.
	if (a.method == 'sticky' && outcome != 'left')
		delete st.put_off[a.mac];
}

// arrived notes a client where it now is. One that was moved up and is back
// where it came from soon after did not stay moved: said, and the client
// left alone for a while.
function arrived(st, view, mac, ifname, now) {
	let b = view.bss[ifname];
	st.on[mac] = { bss: ifname, network: b.network, band: b.band, since: now, asks: 0, last: 0, weak: 0, better: 0, ups: 0, up_at: 0 };
	// Moved to another network or band here since it was last asked what
	// it hears: asked every five minutes again (0289).
	if (st.ask_bss[mac] != null && st.ask_bss[mac] != ifname) {
		delete st.reported[mac];
		delete st.ask_gap[mac];
		delete st.ask_bss[mac];
	}
	learn(st, mac, b.band, now);
	// On here, it is no client the AP tells its neighbours it hears.
	delete st.probes[mac];
	let m = st.moved[mac];
	if (m && m.network == b.network && now - m.at < FELL_BACK && RANK[b.band] <= RANK[m.from]) {
		count(st, m.network, m.method, 'back');
		said(st, { at: int(now + st.wall), mac: mac, network: m.network, method: m.method, from: m.from, asked: 'none', outcome: 'back', took: int((now - m.at) * 10) / 10.0 });
		rested(st, mac, now);
		delete st.moved[mac];
	}
	// Put off by sticky, and back on the band it was put off, or on any of
	// the network's here where it was kept off all of them, within ten
	// minutes (0289): it came back, though its asking ended as left after a
	// minute, and is left alone as one that came straight back. One whose
	// asking is still open is judged by it. One on a lower band here, where
	// it was sent, did not come back.
	let p = st.put_off[mac];
	if (p && p.network == b.network && now - p.at < COME_BACK && (p.wide || b.band == p.from)) {
		delete st.put_off[mac];
		if (!st.tries[mac]) {
			count(st, p.network, 'sticky', 'back');
			let e = { at: int(now + st.wall), mac: mac, network: p.network, method: 'sticky', from: p.from, asked: 'none', outcome: 'back', took: int((now - p.at) * 10) / 10.0 };
			if (p.why)
				e.why = p.why;
			if (p.why == 'nearest' && nearest_back(st, mac, p.rule, now))
				e.stopped = true;
			said(st, e);
			rested(st, mac, now);
			st.came_back[mac] = now;
		}
	}
	return st.on[mac];
}

// judge says what came of an asking, or nothing while it is too soon to say.
function judge(st, view, a, now) {
	let o = st.on[a.mac], here = o ? view.bss?.[o.bss] : null;
	let same = o != null && o.bss == a.bss;
	let within = here != null && here.network == a.network;
	switch (a.method) {
	case 'suggest':
		if (within && !same && RANK[here.band] > RANK[a.from])
			return 'moved';
		if (same && a.answer == 'refused')
			return 'refused';
		if (now < a.until)
			return null;
		return same ? 'stayed' : 'left';
	case 'hold':
		if (within && RANK[here.band] > RANK[a.from])
			return 'moved';
		if (same)
			return 'joined';
		return now < a.until ? null : 'left';
	case 'sticky':
		// Put off, it is back where it was, elsewhere here, or gone; on
		// another of the AP's networks, it has left this one.
		if (a.asked == 'kick') {
			if (same)
				return 'back';
			if (o)
				return within ? 'moved' : 'left';
			return now < a.until ? null : 'left';
		}
		if (o && !same)
			return within ? 'moved' : 'left';
		// Gone: a few seconds for it to show elsewhere here.
		if (!o) {
			a.gone ??= now;
			return now < a.gone + 5 ? null : 'left';
		}
		return null;
	}
	return null;
}

// middle is the four bytes of a BSSID between its first and last: a radio
// makes its networks' BSSIDs from its own address by changing those two
// (0160), so all of one radio's share them. An AP's radios may share them
// too, as a C-360's do (36:86:2d:ef:2f:c0 on 5 GHz, :d0 on 2.4 GHz): the
// middle names the AP, and with the band, the radio.
function middle(bssid) {
	let m = match(lc(bssid ?? ''), /^[0-9a-f]{2}:([0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}):[0-9a-f]{2}$/);
	return m ? m[1] : null;
}

// radio names one radio of an AP by a BSSID of its and its band: the
// middle and the band together. Null where either is not known.
function radio(bssid, band) {
	let m = middle(bssid);
	return m && RANK[band] != null ? `${m} ${band}` : null;
}

// nr_radio is the radio a neighbour report names, as hostapd writes it, a
// byte to two hex digits: the middle of the BSSID it begins with, and the
// band of the operating class after the BSSID's information, by the global
// classes. All of an AP's radios may share a middle, as a C-360's do, so it
// is the two together.
function nr_radio(nr) {
	let m = match(lc(nr ?? ''), /^[0-9a-f]{2}([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})[0-9a-f]{2}[0-9a-f]{8}([0-9a-f]{2})/);
	if (!m)
		return null;
	let c = hex(m[5]), band = c >= 81 && c <= 84 ? '2g' : c >= 115 && c <= 130 ? '5g' : c >= 131 && c <= 137 ? '6g' : null;
	return band ? `${join(':', slice(m, 1, 5))} ${band}` : null;
}

// loads_near is how busy each of the neighbours' radios is, by radio
// (0207), from RRM's word on them (rrm.json), as their hellos say: what a
// radio whose BSSID or band is not said, or whose load is not, is left out.
function loads_near(r) {
	let near = {};
	for (let n in (type(r?.neighbours) == 'array' ? r.neighbours : []))
		for (let b in (type(n?.bands) == 'array' ? n.bands : []))
			if (type(b?.load?.busy) in ['int', 'double'] && radio(b.bssid, b.band))
				near[radio(b.bssid, b.band)] = b.load.busy;
	return near;
}

// by_load orders the network's places on other APs, as hostapd lists them
// ([bssid, ssid, neighbour report]), the least busy first (0207): busy is
// how busy each radio's channel was, by radio, the middle of its BSSID and
// the band of its neighbour report (loads_near). One whose load is not
// known comes after, in the order hostapd lists them.
function by_load(listed, busy) {
	let known = [], rest = [];
	for (let i = 0; i < length(listed ?? []); i++) {
		let b = busy?.[nr_radio(listed[i][2])];
		push(type(b) in ['int', 'double'] ? known : rest, { e: listed[i], b: b, i: i });
	}
	known = sort(known, (x, y) => x.b - y.b || x.i - y.i);
	return map([...known, ...rest], x => x.e);
}

// candidates is the transition request's list: each a neighbour report as
// hostapd has it, with how much it is preferred, the first the most.
function candidates(list) {
	return map(list, (nr, i) => nr + sprintf('0301%02x', 255 - i));
}

// hold keeps a client off 2.4 GHz for a moment: one known to use a higher
// band this AP has the network on, near enough for it, that is not on this
// AP now, and was not held a moment ago. The higher band must be there: on
// a radio that listens for radar before it sends the network is not up,
// and 2.4 GHz is then all the client has.
function hold(st, cfg, view, now, ifname, type, data, mac) {
	let b = view.bss[ifname], net = cfg.networks?.[b.network];
	if (!net?.hold || b.band != '2g' || st.on[mac] || st.tries[mac] || ours(st, mac))
		return [];
	if (now < (st.rest[mac] ?? 0) || now < (st.down[mac] ?? 0) || now - (st.held[mac] ?? -HOLD_EVERY) < HOLD_EVERY)
		return [];
	let h = cfg.homes?.[mac];
	let up = filter(siblings(view, ifname), s => RANK[s.band] > 0 && s.up && can(st, mac, s.band, now) &&
		data.signal != null && data.signal >= NEAR['2g'][s.band] && home_band(h, s.band));
	if (!length(up))
		return [];
	st.held[mac] = now;
	st.tries[mac] = {
		method: 'hold', at: now, mac: mac, network: b.network, bss: ifname, from: '2g',
		to: map(up, s => s.band), asked: 'held', signal: data.signal, until: now + HOLD_WAIT, quiet: type == 'probe',
	};
	return [{ do: 'ban', bss: ifname, mac: mac, ms: HOLD_MS, quiet: type == 'probe' }];
}

// let_go ends a client's keeping on 6 GHz (0300): hostapd's bans on the
// network's lower bands here are lifted at once, and the keeping is kept as
// one record, from when it began: stayed, where the client is still on that
// 6 GHz here (heard weakly, or the switch is off), or left. No band the
// client is on now is asked anything: there, hostapd would put it off. Nor
// is a BSS gone from hostapd since its ban: a radio restarted, as after a
// driver swap that names it anew (phy8 for phy0), took its bans with it.
function let_go(st, view, mac, now, stayed, signal) {
	let k = st.kept[mac], acts = [];
	if (!k)
		return acts;
	for (let ifname in sort(keys(k.bars)))
		if (view.bss?.[ifname] && st.on[mac]?.bss != ifname && !view.clients?.[ifname]?.[mac])
			push(acts, { do: 'bar', bss: ifname, mac: mac, ms: 0 });
	let outcome = stayed ? 'stayed' : 'left';
	count(st, k.network, 'keep', outcome);
	said(st, { at: int(k.since + st.wall), mac: mac, network: k.network, method: 'keep', from: '6g', to: ['6g'], asked: 'held', outcome: outcome, took: min(int(now - k.since), 86400), signal: signal ?? k.signal });
	delete st.kept[mac];
	return acts;
}

// keeping is Keep on 6 GHz (0300), at each look. A phone's own roaming takes
// it back from 6 GHz to the 5 GHz it hears more strongly (the rules let 6 GHz
// far less power), and by Fast Transition the move is made in milliseconds:
// a ban sent once hostapd says it is asking comes too late. So a client on
// the network's 6 GHz here, heard at KEEP_FLOOR or better, is kept off the
// network's lower bands on this AP beforehand, by hostapd's own ban, KEEP_MS
// long and renewed while it is so: hostapd turns away its probes and its
// tries to authenticate there before it tells anyone of them, and it stays
// on 6 GHz. Any other AP is open to it. It is let go, the bans lifted, once
// it is no longer on that 6 GHz here, or has been heard under KEEP_FLOOR
// KEEP_WEAK looks in a row; and with the daemon gone, hostapd lifts the bans
// by its own timer. Only a client on 6 GHz here at that moment is ever kept,
// so one that cannot use 6 GHz never is.
function keeping(st, cfg, view, acts, now) {
	for (let mac in sort(keys(st.kept))) {
		let k = st.kept[mac], o = st.on[mac], net = cfg.networks?.[k.network];
		let c = o?.bss == k.bss ? view.clients?.[k.bss]?.[mac] : null;
		if (c && net?.keep && c.signal != null && home_band(cfg.homes?.[mac], '6g')) {
			k.signal = c.signal;
			if (c.signal >= KEEP_FLOOR) {
				k.weak = 0;
				continue;
			}
			if (++k.weak < KEEP_WEAK)
				continue;
		}
		push(acts, ...let_go(st, view, mac, now, c != null, c?.signal));
	}
	for (let mac in sort(keys(st.on))) {
		let o = st.on[mac], net = cfg.networks?.[o.network], c = view.clients?.[o.bss]?.[mac];
		if (!net?.keep || o.band != '6g' || ours(st, mac) || !home_band(cfg.homes?.[mac], '6g'))
			continue;
		let k = st.kept[mac];
		if (!k) {
			if (c?.signal == null || c.signal < KEEP_FLOOR)
				continue;
			k = st.kept[mac] = { network: o.network, bss: o.bss, since: now, weak: 0, signal: c.signal, bars: {} };
			count(st, o.network, 'keep', 'asked');
		}
		for (let s in siblings(view, o.bss))
			if (RANK[s.band] < RANK['6g'] && !view.clients?.[s.ifname]?.[mac] && (k.bars[s.ifname] ?? 0) - now <= KEEP_RENEW) {
				push(acts, { do: 'bar', bss: s.ifname, mac: mac, ms: KEEP_MS, say: k.bars[s.ifname] == null });
				k.bars[s.ifname] = now + KEEP_MS / 1000.0;
			}
	}
}

// probed keeps how strongly a client not on this AP is heard by its probe
// or its authentication (0289), on the band of the network it came to, with
// the middle of that radio's BSSIDs, by which another AP knows the radio in
// its own neighbour list. The latest a band. Where the network's own
// neighbour report is not known yet, neither is its BSSID, and nothing is.
function probed(st, view, now, ifname, data, mac) {
	let b = view.bss[ifname], m = middle(b.bssid), s = data?.signal;
	if (st.on[mac] || !m || !(type(s) in ['int', 'double']) || s >= 0 || s < -127)
		return;
	(st.probes[mac] ??= {})[b.band] = { signal: int(s), at: now, middle: m };
}

// probes is what this AP tells its neighbours of the clients it hears by
// probe and does not serve (0289): each [mac, band, middle, signal, age],
// age in seconds, none older than PROBE_FRESH, the latest first, then the
// strongest; at most PROBES_MOST.
function probes(st, now) {
	let out = [];
	for (let mac, p in st.probes)
		for (let band, r in p)
			if (now - r.at <= PROBE_FRESH && !st.on[mac])
				push(out, [mac, band, r.middle, r.signal, int(now - r.at)]);
	out = sort(out, (a, b) => a[4] - b[4] || b[3] - a[3] || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
	// What the scan radio hears, marked so (0321): an AP from before takes
	// none of them. After the probes: of a client with a rule of the
	// nearest first; then of the others (0324), the most strongly heard
	// first, as a client heard well here is one that may belong here. A
	// message that cannot hold them all leaves out the last.
	let ruled = [], others = [];
	for (let mac, p in st.scans)
		for (let band, r in p)
			if (now - r.at <= PROBE_FRESH && !st.on[mac])
				push(r.rule ? ruled : others, [mac, band, r.middle, r.signal, int(now - r.at), 's']);
	let by = (a, b) => b[3] - a[3] || a[4] - b[4] || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0);
	return slice([...out, ...sort(ruled, by), ...sort(others, by)], 0, PROBES_MOST);
}

// heard takes one of hostapd's notifications for a network that is steered.
function heard(st, cfg, view, now, ifname, type, data) {
	let b = view.bss?.[ifname], mac = lc(data?.address ?? '');
	if (!b || !cfg.networks?.[b.network] || !match(mac, /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/))
		return [];
	switch (type) {
	case 'sta-authorized': {
		arrived(st, view, mac, ifname, now);
		let a = st.tries[mac], r = a ? judge(st, view, a, now) : null;
		if (r)
			done(st, a, r, now);
		break;
	}
	case 'disassoc':
	case 'deauth': {
		if (st.on[mac]?.bss == ifname)
			delete st.on[mac];
		let a = st.tries[mac];
		if (a && a.bss == ifname)
			a.gone ??= now;
		// Gone from the 6 GHz it was kept on (0300): the lower bands here
		// are open to it again at once.
		if (st.kept[mac]?.bss == ifname)
			return let_go(st, view, mac, now, false, null);
		break;
	}
	case 'bss-transition-response': {
		let a = st.tries[mac];
		if (a && a.asked == 'btm' && a.bss == ifname && a.answer == null) {
			a.code = +(data['status-code'] ?? 0);
			a.answer = a.code == 0 ? 'accepted' : 'refused';
			let r = judge(st, view, a, now);
			if (r)
				done(st, a, r, now);
		}
		break;
	}
	case 'probe':
	case 'auth':
		probed(st, view, now, ifname, data, mac);
		return hold(st, cfg, view, now, ifname, type, data, mac);
	case 'beacon-report': {
		// What a client on this network said it hears (802.11k, 0289): of
		// this AP, by its own BSSID, and of the other BSSes it measured.
		// Whoever asked, what it says is so.
		let r = measure.notified(data), o = st.on[mac];
		if (!r || o?.bss != ifname)
			break;
		let said = st.reports[mac] ??= { bss: {} };
		let dbm = r.dbm >= 0 ? int(r.dbm + 0.5) : -int(-r.dbm + 0.5);
		if (r.bssid == lc(b.bssid ?? ''))
			said.own = { dbm: dbm, at: now };
		else
			said.bss[r.bssid] = { dbm: dbm, at: now };
		break;
	}
	}
	return [];
}

// nr_bssid is the BSSID a neighbour report begins with, as hostapd writes
// it.
function nr_bssid(nr) {
	let m = match(lc(nr ?? ''), /^([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})/);
	return m ? join(':', slice(m, 1)) : null;
}

// nr_channel is the band and channel a neighbour report names, after its
// BSSID's information, by the band of its operating class.
function nr_channel(nr) {
	let m = match(lc(nr ?? ''), /^[0-9a-f]{12}[0-9a-f]{8}([0-9a-f]{2})([0-9a-f]{2})/);
	let r = nr_radio(nr);
	return m && r ? { band: split(r, ' ')[1], channel: hex(m[2]) } : null;
}

// aps_near is which AP each of the neighbours' radios is, by radio (the
// middle of its BSSIDs and its band, as a C-360's radios share a middle),
// from RRM's word on them (rrm.json).
function aps_near(r) {
	let out = {};
	for (let n in (type(r?.neighbours) == 'array' ? r.neighbours : []))
		if (type(n?.ap) == 'string' && match(n.ap, /^ap-[0-9a-f]{12}$/))
			for (let b in (type(n.bands) == 'array' ? n.bands : []))
				if (radio(b?.bssid, b?.band))
					out[radio(b.bssid, b.band)] = n.ap;
	return out;
}

// heard_near says another AP heard a client lately on band, by its probes.
function heard_near(near, band) {
	for (let r in near ?? [])
		if (r?.band == band && type(r.age) in ['int', 'double'] && r.age >= 0 && r.age <= PROBE_FRESH)
			return true;
	return false;
}

// ask_channels is what a client on a BSS is asked to measure (802.11k):
// the BSS's own channel first, then those of the network's places on its
// band elsewhere, each once, at most ASK_CHANNELS, each with its 20 MHz
// operating class. Its own first, so that what it says of the others is
// set against what it says of this AP, measured the same way.
function ask_channels(b, band) {
	let out = [], seen = {};
	for (let nr in [b.own ?? b.nr, ...(b.elsewhere ?? [])]) {
		let c = nr_channel(nr);
		if (!c || c.band != band || seen[c.channel] || length(out) >= ASK_CHANNELS)
			continue;
		seen[c.channel] = true;
		let oc = measure.op_class(BAND_FREQ[band], c.channel);
		if (oc != null)
			push(out, { op_class: oc, channel: c.channel });
	}
	return out;
}

// better is where another AP of the network hears a client more strongly
// than this AP does, by the network's margin or more and on the band the
// client is on (0289): the network's places there that hostapd lists, the
// most strongly heard first, each with the AP and how strongly. Only while
// the client is heard here under the network's floor. near is what the
// neighbours told of it by its probes: each { ap, band, middle, signal,
// age }. said is what the client said it hears (802.11k): a place it hears
// by the margin more strongly than it said it hears this AP is one too,
// its AP the one RRM knows by that radio, if any. Only against what it said
// of this AP, measured the same way: what a client hears of an AP is of the
// AP's sending, which is louder than the client's by 5 to 10 dB, so set
// against how this AP hears the client it would move clients that are
// better where they are. Where it said nothing of this AP, what it says of
// the others makes no place.
function better(view, near, said, o, c, net, now) {
	let b = view.bss[o.bss], by = {}, out = [], seen = {};
	if (c.signal == null || c.signal >= sticky_of(net, 'floor'))
		return out;
	for (let nr in b.elsewhere ?? [])
		if (nr_radio(nr))
			by[nr_radio(nr)] ??= nr;
	for (let r in sort([...(near ?? [])], (x, y) => y.signal - x.signal)) {
		let nr = by[`${r?.middle} ${r?.band}`];
		// What a scan radio heard counts where the network weighs them
		// (0324).
		if (r?.kind == 'scan' && !net?.scan)
			continue;
		if (nr && r.band == o.band && type(r.signal) == 'int' && type(r.age) in ['int', 'double'] && r.age >= 0 && r.age <= PROBE_FRESH &&
		    r.signal >= c.signal + sticky_of(net, 'margin') && !seen[nr]) {
			seen[nr] = true;
			push(out, { nr: nr, ap: r.ap, signal: r.signal });
		}
	}
	let fresh = (x) => type(x?.dbm) == 'int' && now - x.at <= PROBE_FRESH;
	if (!fresh(said?.own))
		return sort(out, (x, y) => y.signal - x.signal);
	for (let nr in b.elsewhere ?? []) {
		let x = said.bss?.[nr_bssid(nr)];
		if (seen[nr] || !fresh(x) || nr_channel(nr)?.band != o.band || x.dbm < said.own.dbm + sticky_of(net, 'margin'))
			continue;
		seen[nr] = true;
		push(out, { nr: nr, ap: view.ap_of?.[nr_radio(nr)], signal: x.dbm });
	}
	return sort(out, (x, y) => y.signal - x.signal);
}

// across is band steering's reach to other APs, where community sharing is
// on (0299): a client on a band below one it can use, which this AP does
// not have for it near enough, or at all, is sent to the network's place on
// that band at another AP that the client said it hears well enough
// (UP_HEARD), the higher band first and then the best heard (802.11k). With
// nothing fresh said, one that gives beacon reports is asked what it hears
// of those places, at most UP_ASKS times while it stays joined, as far
// apart as suggest's own askings: its answer comes as hostapd hears it, and
// a later look acts on it. a phone held on to the C-260's 5 GHz,
// which has no 6 GHz, under the Garage's C-360 (2026-11-03).
function across(st, view, acts, mac, o, b, c, now, h) {
	// Where it has a rule (0319), only to an AP and band it belongs on.
	let want = filter(['6g', '5g'], band => RANK[band] > RANK[o.band] && can(st, mac, band, now) && home_band(h, band));
	if (!length(want))
		return;
	let said = st.reports[mac], places = [];
	for (let nr in b.elsewhere ?? []) {
		let ch = nr_channel(nr), x = said?.bss?.[nr_bssid(nr)];
		if (ch && index(want, ch.band) >= 0 && type(x?.dbm) == 'int' && now - x.at <= PROBE_FRESH && x.dbm >= UP_HEARD[ch.band] &&
		    (!h || home_ap(h, view.ap_of?.[nr_radio(nr)])))
			push(places, { nr: nr, band: ch.band, dbm: x.dbm });
	}
	if (length(places)) {
		places = sort(places, (x, y) => RANK[y.band] != RANK[x.band] ? RANK[y.band] - RANK[x.band] : y.dbm - x.dbm);
		o.asks++;
		o.last = now;
		let best = places[0], ap = view.ap_of?.[nr_radio(best.nr)];
		st.tries[mac] = {
			method: 'suggest', at: now, mac: mac, network: o.network, bss: o.bss, from: o.band,
			to: uniq(map(places, p => p.band)), asked: 'btm', signal: c.signal, until: now + ANSWER_WAIT,
		};
		if (ap)
			st.tries[mac].elsewhere = { ap: ap, signal: best.dbm };
		push(acts, { do: 'btm', bss: o.bss, mac: mac, neighbors: candidates(map(places, p => p.nr)), imminent: false, timer: 0, validity: 100, abridged: true });
		return;
	}
	let m = measure.mode(c.rrm);
	if (m == null || o.ups >= UP_ASKS || (o.ups > 0 && now - o.up_at < SUGGEST_AGAIN[o.ups - 1]))
		return;
	let chans = [], seen = {};
	for (let nr in b.elsewhere ?? []) {
		let ch = nr_channel(nr);
		if (!ch || index(want, ch.band) < 0 || seen[ch.band + ' ' + ch.channel] || length(chans) >= ASK_CHANNELS ||
		    (h && !home_ap(h, view.ap_of?.[nr_radio(nr)])))
			continue;
		seen[ch.band + ' ' + ch.channel] = true;
		let oc = measure.op_class(BAND_FREQ[ch.band], ch.channel);
		if (oc != null)
			push(chans, { op_class: oc, channel: ch.channel });
	}
	if (!length(chans))
		return;
	o.ups++;
	o.up_at = now;
	push(acts, { do: 'beacon', bss: o.bss, mac: mac, mode: m, ssid: b.ssid, duration: measure.DURATION, channels: chans });
}

// put_off puts a client off where it is, for the hold its asking was made
// with; and keeps it off the network's other bands here too where its
// place is on another AP.
function put_off(st, view, acts, a, now) {
	push(acts, { do: 'kick', bss: a.bss, mac: a.mac, ms: a.ms, why: a.why, blind: a.blind });
	if (a.wide)
		for (let s in siblings(view, a.bss))
			push(acts, { do: 'ban', bss: s.ifname, mac: a.mac, ms: a.ms, quiet: true });
	a.asked = 'kick';
	a.until = now + BACK_WAIT;
	delete st.on[a.mac];
	st.put_off[a.mac] = { at: now, network: a.network, from: a.from, wide: !!a.wide, why: a.why, rule: a.rule };
}

// move_off asks a client to go, for why: weak, or heard better elsewhere
// (better). One that takes a transition request is told it is to go, to
// list, with no hour named: given one, hostapd puts the client off itself
// when it comes, and the daemon then a second time; it is put off after,
// where it stays. One that takes none cannot be told: it is put off at
// once. wide is where its place is on another AP: it is then kept off the
// network on every band here, as one put off one band comes straight back
// on another, or on the same one before it has looked elsewhere.
function move_off(st, view, acts, mac, o, c, net, now, why, list, wide, after, elsewhere, hold) {
	st.asked[mac] = now;
	// Asked to go, it is asked what it hears every five minutes again,
	// wherever it goes.
	delete st.reported[mac];
	delete st.ask_gap[mac];
	delete st.ask_bss[mac];
	let a = st.tries[mac] = {
		method: 'sticky', why: why, at: now, mac: mac, network: o.network, bss: o.bss, from: o.band,
		asked: 'none', signal: c.signal, kick_at: now, until: now + after + BACK_WAIT, ms: (hold ?? sticky_of(net, 'hold_down')) * 1000, wide: wide,
	};
	if (elsewhere)
		a.elsewhere = elsewhere;
	if (c.btm) {
		a.asked = 'btm';
		a.kick_at = now + after;
		push(acts, { do: 'btm', bss: o.bss, mac: mac, neighbors: candidates(list), imminent: true, timer: 0, validity: 255, abridged: false, why: why });
		return;
	}
	put_off(st, view, acts, a, now);
}

// freq_of is a channel's frequency, in MHz, on a band.
function freq_of(band, ch) {
	return band == '2g' ? (ch == 14 ? 2484 : 2407 + 5 * ch) : band == '5g' ? 5000 + 5 * ch : band == '6g' ? (ch == 2 ? 5935 : 5950 + 5 * ch) : null;
}

// band_of is the band of a frequency, in MHz.
function band_of(f) {
	return type(f) != 'int' ? null : f >= 2400 && f <= 2500 ? '2g' : f >= 5150 && f <= 5895 ? '5g' : f >= 5925 && f <= 7125 ? '6g' : null;
}

// chans_near is the channel each of the neighbours' radios is on, by radio
// (the middle of its BSSIDs and its band), from RRM's word on them
// (rrm.json): its primary 20 MHz.
function chans_near(r) {
	let out = {};
	for (let n in (type(r?.neighbours) == 'array' ? r.neighbours : []))
		for (let b in (type(n?.bands) == 'array' ? n.bands : []))
			if (radio(b?.bssid, b?.band) && type(b.channel) == 'int')
				out[radio(b.bssid, b.band)] = b.channel;
	return out;
}

// offset is what the radios a band's clients are on hear more than the scan
// radio does, in dB (0321): the middle of each client's latest setting,
// where SCAN_LEAST clients or more were set within SCAN_FOR; else null. The
// scan radio's antennas are not the serving radios', and a C-260's ath10k
// reads high (+8 to +12): set against its own radios, an AP's scan radio
// says what they would hear.
function offset(st, band, now) {
	let d = [];
	for (let mac, x in st.cal[band] ?? {})
		if (now - x.at <= SCAN_FOR)
			push(d, x.diff);
	if (length(d) < SCAN_LEAST)
		return null;
	d = sort(d, (a, b) => a - b);
	let n = length(d);
	return n % 2 ? d[(n - 1) / 2] : int((d[n / 2 - 1] + d[n / 2]) / 2);
}

// scanned takes what the scan radio hears of clients (airscan's
// clients.json, file, read at wall), each with the BSSID it talks through,
// the frequency it was heard on and its signal (0321). A client this AP
// serves sets the scan radio against the radio it is on, by band. A client
// another AP serves, that has a rule of the nearest, is a reading of how
// this AP's radio on that band would hear it, for the APs to compare: only
// where it was heard on the primary channel of the AP it talks through, as
// a scan radio on the other half of a 40 MHz channel hears it far weaker
// (the Living Room's heard the TV it served at -58 as -92, 2026-11-05).
function scanned(st, view, file, wall, now, homes, all) {
	let own = {}, mids = {};
	for (let ifname in sort(keys(view.bss ?? {}))) {
		let b = view.bss[ifname], ch = nr_channel(b.own ?? b.nr), m = middle(b.bssid);
		if (!b.bssid || !ch || !m)
			continue;
		own[lc(b.bssid)] = { ifname: ifname, band: b.band, freq: freq_of(b.band, ch.channel) };
		mids[b.band] ??= m;
	}
	for (let x in (type(file?.list) == 'array' ? file.list : [])) {
		let mac = lc(x?.mac ?? ''), id = lc(x?.bssid ?? ''), s = x?.signal, band = band_of(x?.freq);
		if (!match(mac, /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) || !match(id, /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) || type(s) != 'int' || s >= 0 || s < -127 ||
		    !band || type(x.last_seen) != 'int' || type(x.frames) != 'int' || x.frames < 10)
			continue;
		let age = wall - x.last_seen;
		if (age < 0 || age > PROBE_FRESH)
			continue;
		let o = own[id];
		if (o) {
			let c = view.clients?.[o.ifname]?.[mac];
			if (age <= SCAN_FRESH && x.freq == o.freq && o.band == band && type(c?.signal) == 'int') {
				let k = st.cal[band] ??= {};
				k[mac] = { diff: c.signal - s, at: now };
				if (length(k) > SCAN_MOST)
					for (let old in slice(sort(keys(k), (p, q) => k[p].at - k[q].at), 0, length(k) - SCAN_MOST))
						delete k[old];
			}
			continue;
		}
		// Of a client with a rule of the nearest; and of any, where a
		// network here weighs the scan radios (0324).
		let rule = !!homes?.[mac]?.nearest;
		if ((!rule && !all) || st.on[mac] || !mids[band])
			continue;
		let ch = view.chan_near?.[radio(id, band)], off = offset(st, band, now);
		if (ch == null || x.freq != freq_of(band, ch) || off == null)
			continue;
		(st.scans[mac] ??= {})[band] = { signal: max(-127, min(-1, s + off)), at: now - age, middle: mids[band], rule: rule };
	}
}

// nearest is where a client with a rule of the nearest is heard better
// (0321): the network's places elsewhere, on its band, that hostapd lists,
// where the AP of that radio hears it NEAREST_MARGIN dB or more better than
// this AP does, by its probes or its scan radio, by a reading not older
// than PROBE_FRESH; only an AP and band the rule allows. The best first.
function nearest(view, near, o, c, h) {
	let b = view.bss[o.bss], by = {}, out = [], seen = {};
	if (type(c?.signal) != 'int' || !home_band(h, o.band))
		return out;
	for (let nr in b.elsewhere ?? [])
		if (nr_radio(nr))
			by[nr_radio(nr)] ??= nr;
	for (let r in sort([...(near ?? [])], (x, y) => y.signal - x.signal)) {
		let nr = by[`${r?.middle} ${r?.band}`];
		if (nr && r.band == o.band && type(r.signal) == 'int' && type(r.age) in ['int', 'double'] && r.age >= 0 && r.age <= PROBE_FRESH &&
		    r.signal >= c.signal + NEAREST_MARGIN && home_ap(h, r.ap) && !seen[nr]) {
			seen[nr] = true;
			push(out, { nr: nr, ap: r.ap, signal: r.signal });
		}
	}
	return out;
}

// home_places is where a client not where it belongs is sent (0319): on an
// AP it belongs on but a band it does not, the network's other bands here
// that it belongs on; else, or with none, the network's places elsewhere
// that hostapd lists, on an AP it belongs on, by RRM's word on that radio,
// and on a band it belongs on. Only a band the client is known to use: one
// only ever seen on 2.4 GHz is never sent up, and stays where it is when its
// home has no 2.4 GHz place. wide is where the places are on other APs, and
// ap the first of them.
function home_places(st, view, mac, o, b, h, now) {
	if (h.here) {
		let here = filter(siblings(view, o.bss), s => s.nr && s.up && home_band(h, s.band) && can(st, mac, s.band, now));
		if (length(here))
			return { list: map(sort(here, (x, y) => RANK[y.band] - RANK[x.band]), s => s.nr), wide: false, ap: null };
	}
	let list = [], ap = null;
	for (let nr in b.elsewhere ?? []) {
		let ch = nr_channel(nr), a = view.ap_of?.[nr_radio(nr)];
		if (ch && home_ap(h, a) && home_band(h, ch.band) && can(st, mac, ch.band, now)) {
			push(list, nr);
			ap ??= a;
		}
	}
	return { list: list, wide: true, ap: ap };
}

// tick looks at who is where and how well each is heard, ends what is at
// its end, and asks what is now due. view is the AP's steered networks as
// they now are: bss, each one's network and band, whether it is up, its
// own neighbour report where clients are asked to it, and those of the
// same SSID elsewhere; clients, each one's signal and whether it takes a
// transition request.
function tick(st, cfg, view, now, wall) {
	let acts = [];
	st.wall = wall - now;

	// Who is where. A client on two at once is moving: it is where it was
	// last heard to be let on.
	let places = {};
	for (let ifname in sort(keys(view.clients ?? {})))
		if (view.bss?.[ifname])
			for (let mac in keys(view.clients[ifname]))
				push(places[mac] ??= [], ifname);
	for (let mac in sort(keys(places)))
		if (!st.on[mac] || index(places[mac], st.on[mac].bss) < 0)
			arrived(st, view, mac, places[mac][0], now);
	for (let mac in keys(st.on))
		if (!places[mac]) {
			let a = st.tries[mac];
			if (a && a.bss == st.on[mac].bss)
				a.gone ??= now;
			delete st.on[mac];
		}

	for (let mac in sort(keys(st.on))) {
		let o = st.on[mac], c = view.clients[o.bss][mac], net = cfg.networks?.[o.network];
		learn(st, mac, o.band, now);
		for (let band in c.bands ?? [])
			if (RANK[band] != null)
				learn(st, mac, band, now);
		if (c.btm != null)
			st.known[mac].btm = !!c.btm;
		if (c.own)
			st.known[mac].own = true;
		o.weak = net?.sticky && c.signal != null && c.signal < sticky_of(net, 'weak') ? o.weak + 1 : 0;
		// Heard more strongly by another AP of the network (0289), at this
		// look and the last: one probe heard well there is not a place.
		o.places = net?.sticky ? better(view, view.near?.[mac], st.reports[mac], o, c, net, now) : [];
		// Only to a place it belongs on, where it has a rule (0319).
		let h = cfg.homes?.[mac];
		if (h)
			o.places = filter(o.places, p => home_ap(h, p.ap) && home_band(h, o.band));
		// Heard better by another AP, where its rule is of the nearest
		// (0321), at this look and the last.
		o.nearest = h?.nearest ? nearest(view, view.near?.[mac], o, c, h) : [];
		o.nearer = length(o.nearest) ? (o.nearer ?? 0) + 1 : 0;
		// Heard by no other AP on its band, since when (0321).
		if (h?.nearest && !heard_near(view.near?.[mac], o.band))
			o.blind ??= now;
		else
			delete o.blind;
		o.better = length(o.places) ? (o.better ?? 0) + 1 : 0;
	}

	// When another AP last heard each client by its probes, on any band,
	// as the neighbours told: for the weak rule, a sign it has somewhere
	// better to go (0289).
	// By probes alone: the weak rule is as before (0324).
	for (let mac, list in view.near ?? {})
		for (let r in (type(list) == 'array' ? list : []))
			if (r?.kind != 'scan' && type(r?.age) in ['int', 'double'] && r.age >= 0 && r.age <= PROBE_FRESH && now - r.age > (st.elsewhere[mac] ?? -ELSEWHERE_FOR - 1))
				st.elsewhere[mac] = now - r.age;

	// What is heard of clients by probe, or by the scan radio, and too
	// old to tell is let go.
	for (let heard in [st.probes, st.scans])
		for (let mac, p in heard) {
			for (let band, r in p)
				if (now - r.at > PROBE_FRESH)
					delete p[band];
			if (!length(p))
				delete heard[mac];
		}

	// What is at its end; and a weak client that was asked to go and stayed.
	for (let mac in sort(keys(st.tries))) {
		let a = st.tries[mac], r = judge(st, view, a, now);
		if (r) {
			done(st, a, r, now);
			continue;
		}
		if (a.method != 'sticky' || a.asked == 'kick' || now < a.kick_at)
			continue;
		let o = st.on[mac], c = o ? view.clients[o.bss]?.[mac] : null;
		if (!o || o.bss != a.bss || !c)
			continue;
		// Told to go where it belongs (0319), it goes, however well it is
		// heard here.
		if (a.why == 'home' || a.why == 'nearest') {
			put_off(st, view, acts, a, now);
			continue;
		}
		// Heard well again here: over weak, or, where it was told to go for
		// being heard better elsewhere, over the floor.
		let n = cfg.networks?.[a.network];
		let well = sticky_of(n, a.why == 'better' ? 'floor' : 'weak');
		if (c.signal != null && c.signal >= well + RECOVERED) {
			done(st, a, 'recovered', now);
			continue;
		}
		// How it is heard now is not known: it is not put off for it.
		if (c.signal == null) {
			done(st, a, 'stayed', now);
			continue;
		}
		put_off(st, view, acts, a, now);
	}

	for (let mac in sort(keys(st.on))) {
		let o = st.on[mac], b = view.bss[o.bss], c = view.clients[o.bss][mac], net = cfg.networks?.[o.network];
		if (!net || st.tries[mac] || ours(st, mac) || now < (st.rest[mac] ?? 0))
			continue;
		let sibs = siblings(view, o.bss), h = cfg.homes?.[mac];

		// Where a person said it belongs (0319): one that is not, once
		// settled, is sent there as sticky sends a client heard better
		// elsewhere, and kept off here for its rule's hold if it stays. Where
		// it has no place there that it can use, nothing is done.
		if (h && length(h.aps) && !(h.here && home_band(h, o.band)) && now - o.since >= HOME_AFTER && now - (st.asked[mac] ?? -STICKY_EVERY) >= STICKY_EVERY) {
			let p = home_places(st, view, mac, o, b, h, now);
			if (length(p.list)) {
				move_off(st, view, acts, mac, o, c, net, now, 'home', p.list, p.wide, BETTER_KICK_AFTER, p.ap ? { ap: p.ap } : null, h.hold);
				continue;
			}
		}

		// Its rule is of the nearest (0321): another AP heard it better by
		// the margin, at two looks running, and it is sent there, as one
		// heard better elsewhere, kept off here for its rule's hold.
		// One that came back here twice after it (stopped_here) is left
		// here, by it and by the fallback below.
		let nearest_on = h?.nearest && !stopped_here(st, mac, h, now);
		if (nearest_on && o.nearer >= BETTER_LOOKS && now - o.since >= HOME_AFTER && now - (st.asked[mac] ?? -STICKY_EVERY) >= STICKY_EVERY) {
			let best = o.nearest[0];
			move_off(st, view, acts, mac, o, c, net, now, 'nearest', map(o.nearest, p => p.nr), true, BETTER_KICK_AFTER,
				best.ap ? { ap: best.ap, signal: best.signal } : null, h.hold);
			st.tries[mac].rule = rule_key(h);
			continue;
		}
		// Its rule is of the nearest, and no other AP has heard it on its
		// band for NEAREST_BLIND seconds while it is heard here under
		// NEAREST_BLIND_BELOW: the APs cannot compare. A C-360's scan radio
		// is seldom on 2.4 GHz and hears clients there with no signal: the
		// TV sat on the C260's 2.4 GHz at -76, the Living Room's scan radio
		// with no reading of it, after the Living Room came back from a
		// flash (2026-11-05). It is put off, the network's places elsewhere
		// named, to choose again; one that comes straight back is left
		// alone longer each time.
		if (nearest_on && o.blind != null && now - o.blind >= NEAREST_BLIND && type(c.signal) == 'int' && c.signal < NEAREST_BLIND_BELOW &&
		    length(b.elsewhere ?? []) && now - (st.asked[mac] ?? -STICKY_EVERY) >= STICKY_EVERY) {
			move_off(st, view, acts, mac, o, c, net, now, 'nearest', b.elsewhere, true, BETTER_KICK_AFTER, null, h.hold);
			st.tries[mac].rule = rule_key(h);
			// Said so in the log and the trace: no other AP heard it.
			st.tries[mac].blind = true;
			for (let x in acts)
				if (x.mac == mac)
					x.blind = true;
			continue;
		}

		if (net.sticky && now - (st.asked[mac] ?? -STICKY_EVERY) >= STICKY_EVERY) {
			// Heard better by another AP of the network, and not well
			// here (0289): told to go there, and kept off every band here
			// if it is put off.
			if (o.better >= BETTER_LOOKS) {
				// The AP it is heard better by is named where it is known:
				// a place a client said it hears may be of an AP RRM does
				// not know.
				let best = o.places[0];
				move_off(st, view, acts, mac, o, c, net, now, 'better', map(o.places, p => p.nr), true, BETTER_KICK_AFTER,
					best.ap ? { ap: best.ap, signal: best.signal } : null);
				continue;
			}
			// Weak, and stays so: where the network has a better place for
			// it, a lower band here or a network of its name on another AP.
			// Put off, it is kept off the network here only where it has no
			// lower band to go to here. Where its places are on other APs
			// alone, only where there is a sign it has somewhere better
			// (0289): another AP heard it within a day; else it is tried
			// once, and not again for a week (BACKS_FOR), whenever it comes
			// back. An ESP32 heard at -93 by the one AP that hears it was put
			// off every five minutes, its asking ending as left each time
			// (2026-11-03); counted by its coming back, it was tried twice
			// all the same, as it was back ten and a half minutes after the
			// first, past the ten that count as coming back. It is the try
			// that counts.
			// Not one with a rule: where it belongs is a person's word, not
			// a signal's (0319).
			if (o.weak >= WEAK_LOOKS && !h) {
				let down = filter(sibs, s => RANK[s.band] < RANK[o.band] && s.nr);
				let list = [...map(down, s => s.nr), ...(b.elsewhere ?? [])];
				let heard = now - (st.elsewhere[mac] ?? -ELSEWHERE_FOR - 1) <= ELSEWHERE_FOR;
				let lone = !length(down) && !heard;
				let somewhere = !lone || (st.tried[mac] == null && st.came_back[mac] == null);
				// Asked what it hears, it is given a moment to say it.
				let waiting = now - (st.reported[mac] ?? -REPORT_WAIT - 1) < REPORT_WAIT;
				if (length(list) && somewhere && !waiting) {
					if (lone)
						st.tried[mac] = now;
					move_off(st, view, acts, mac, o, c, net, now, 'weak', list, !length(down), KICK_AFTER, null);
					continue;
				}
			}
		}

		// Weak here, under sticky's weak signal, and no other AP hears it,
		// as a client that holds on sends no probes: one that gives beacon
		// reports is asked what it hears of the network's places on its
		// band (802.11k, 0289). Not one heard better than that: on the
		// Living Room AP a TV at -68 and another client, both good where
		// they were, were asked every five minutes, four channels each
		// (2026-11-03). Every five minutes at first, and twice as long
		// each time after, up to two hours, until it is asked to go or is
		// on another of the network's bands here: one that says it hears
		// nowhere better, or says nothing, is where it is. What it says
		// comes as hostapd hears it, for the rule above.
		let m = measure.mode(c.rrm);
		if (net.sticky && m != null && c.signal != null && c.signal < sticky_of(net, 'weak') && !length(o.places) &&
		    !heard_near(view.near?.[mac], o.band) && now - o.since >= ASK_AFTER && now - (st.reported[mac] ?? -ASK_MOST - 1) >= (st.ask_gap[mac] ?? ASK_EVERY)) {
			let chans = ask_channels(b, o.band);
			// Its own channel, and at least one other's.
			if (length(chans) > 1) {
				st.ask_gap[mac] = st.reported[mac] == null ? ASK_EVERY : min((st.ask_gap[mac] ?? ASK_EVERY) * 2, ASK_MOST);
				st.reported[mac] = now;
				st.ask_bss[mac] = o.bss;
				push(acts, { do: 'beacon', bss: o.bss, mac: mac, mode: m, ssid: b.ssid, duration: measure.DURATION, channels: chans });
			}
		}

		// Near, on a band below one it can use: asked up. Thrice at most
		// while it stays joined, and further apart each time.
		if (!net.suggest || !c.btm || c.signal == null || o.asks > length(SUGGEST_AGAIN) || now < (st.down[mac] ?? 0))
			continue;
		if (now - o.since < SUGGEST_AFTER || (o.asks > 0 && now - o.last < SUGGEST_AGAIN[o.asks - 1]))
			continue;
		// Near at this look and at the last: one reading is not a place.
		let near = (band) => c.signal >= NEAR[o.band][band] && o.heard != null && o.heard >= NEAR[o.band][band];
		// Nor to a band whose channel is nearly full (0207).
		let full = (band) => (view.busy?.[band] ?? 0) >= FULL;
		let up = filter(sibs, s => RANK[s.band] > RANK[o.band] && s.nr && can(st, mac, s.band, now) && near(s.band) && !full(s.band) && home_band(h, s.band));
		if (!length(up)) {
			// Nowhere up here: with community sharing on, another AP's
			// higher band, where the client says it hears it well (0299).
			if (net.community)
				across(st, view, acts, mac, o, b, c, now, h);
			continue;
		}
		up = sort(up, (x, y) => RANK[y.band] - RANK[x.band]);
		o.asks++;
		o.last = now;
		st.tries[mac] = {
			method: 'suggest', at: now, mac: mac, network: o.network, bss: o.bss, from: o.band,
			to: map(up, s => s.band), asked: 'btm', signal: c.signal, until: now + ANSWER_WAIT,
		};
		push(acts, { do: 'btm', bss: o.bss, mac: mac, neighbors: candidates(map(up, s => s.nr)), imminent: false, timer: 0, validity: 100, abridged: true });
	}

	keeping(st, cfg, view, acts, now);

	for (let mac, o in st.on)
		o.heard = view.clients[o.bss]?.[mac]?.signal;

	// What is remembered is let go when it no longer bears on anything.
	for (let mac, t in st.held)
		if (now - t >= HOLD_EVERY)
			delete st.held[mac];
	for (let mac, t in st.asked)
		if (now - t >= STICKY_EVERY)
			delete st.asked[mac];
	// Not asked for twice the longest, it starts again at five minutes.
	for (let mac, t in st.reported)
		if (now - t >= 2 * ASK_MOST) {
			delete st.reported[mac];
			delete st.ask_gap[mac];
			delete st.ask_bss[mac];
		}
	for (let mac, p in st.put_off)
		if (now - p.at >= COME_BACK)
			delete st.put_off[mac];
	for (let mac, t in st.came_back)
		if (now - t >= BACKS_FOR)
			delete st.came_back[mac];
	for (let mac, t in st.tried)
		if (now - t >= BACKS_FOR)
			delete st.tried[mac];
	if (length(st.tried) > KNOWN_MOST) {
		let by = sort(keys(st.tried), (x, y) => st.tried[x] - st.tried[y]);
		for (let mac in slice(by, 0, length(by) - KNOWN_MOST))
			delete st.tried[mac];
	}
	for (let mac, t in st.elsewhere)
		if (now - t > ELSEWHERE_FOR)
			delete st.elsewhere[mac];
	if (length(st.elsewhere) > ELSEWHERE_MOST) {
		let by = sort(keys(st.elsewhere), (x, y) => st.elsewhere[x] - st.elsewhere[y]);
		for (let mac in slice(by, 0, length(by) - ELSEWHERE_MOST))
			delete st.elsewhere[mac];
	}
	// What a client said it hears is let go once it is old, or the client
	// is no longer on here.
	for (let mac, said in st.reports) {
		for (let bssid, x in said.bss)
			if (now - x.at > PROBE_FRESH)
				delete said.bss[bssid];
		if (said.own && now - said.own.at > PROBE_FRESH)
			delete said.own;
		if (!st.on[mac] || (!said.own && !length(said.bss)))
			delete st.reports[mac];
	}
	for (let mac, t in st.rest)
		if (now >= t)
			delete st.rest[mac];
	for (let mac, t in st.down)
		if (now >= t)
			delete st.down[mac];
	for (let times in [st.backs, st.downs, st.home_backs, st.stopped])
		for (let mac, b in times)
			if (now - b.at >= BACKS_FOR)
				delete times[mac];
	for (let mac, m in st.moved)
		if (now - m.at >= FELL_BACK)
			delete st.moved[mac];
	let known = keys(st.known);
	for (let mac in known)
		if (now - (st.known[mac].at ?? 0) >= KNOWN_FOR)
			delete st.known[mac];
	if (length(st.known) > KNOWN_MOST) {
		known = sort(keys(st.known), (x, y) => (st.known[x].at ?? 0) - (st.known[y].at ?? 0));
		for (let mac in slice(known, 0, length(known) - KNOWN_MOST))
			delete st.known[mac];
	}
	return acts;
}

// summary is what the agent reports: each steered network, what is on for
// it and on which bands, the count of what was asked and what came of it
// since the daemon started, and the last of the records.
function summary(st, cfg, view, wall, since) {
	let nets = [];
	for (let id in sort(keys(cfg.networks ?? {}))) {
		let n = cfg.networks[id], bands = {};
		for (let name, b in view.bss ?? {})
			if (b.network == id)
				bands[b.band] = true;
		let e = {
			network: id, suggest: !!n.suggest, hold: !!n.hold, sticky: !!n.sticky,
			bands: sort(keys(bands), (a, b) => RANK[a] - RANK[b]), counts: st.counts[id] ?? {},
		};
		if (n.sticky)
			for (let k in keys(STICKY))
				e[k] = sticky_of(n, k);
		push(nets, e);
	}
	return { at: wall, since: since, networks: nets, events: slice(st.events, -EVENTS_SAID), known: length(st.known) };
}

// sticky_with reads one network's entry of sticky's settings, as the
// renderer writes them (0289): its ID, then name=value for each of the
// settings but weak, which sticky's own entry carries (0138). One left out,
// out of its bounds, or not known, is as unset. Null where it is no entry.
function sticky_with(entry) {
	let words = type(entry) == 'string' ? split(trim(entry), /[ \t]+/) : [];
	if (!length(words) || !match(words[0], /^[^=]+$/))
		return null;
	let out = {};
	for (let k, s in STICKY)
		if (k != 'weak')
			out[k] = s.unset;
	for (let w in slice(words, 1)) {
		let m = match(w, /^([a-z_]+)=(-?[0-9]+)$/), s = m && m[1] != 'weak' ? STICKY[m[1]] : null;
		if (s && +m[2] >= s.least && +m[2] <= s.most)
			out[m[1]] = +m[2];
	}
	return [words[0], out];
}

// kept is what the daemon keeps of its clients from one run to the next
// (aeolus-steer's KNOWN file, on tmpfs: an update of the agent starts the
// daemon again, a reboot clears it), by the clock: off is the clock's time
// less the daemon's own, now the daemon's. Each client's bands, and whether
// it takes a request or is an AP's own test client (0138); and (0289) when
// each sticky put off came back, and when the weak rule tried each no
// other AP heard, within BACKS_FOR, when another AP last heard each,
// within its day, and until when each is left alone, while that is to
// come. Without those, a lone client would be put off again
// after every update of the agent, and off the air minutes each time.
// And how often each came back and was sent down, and when last, within
// BACKS_FOR, and until when each is left down: without them, each left
// alone half an hour after every start of the daemon, not twice as long
// each time, as an update of the agent or a change of its config starts
// it again (an ESP on the Shed was put off 8 times a day,
// 2026-11-05).
function kept(st, off, now) {
	let known = {}, at = (t) => int(t + off);
	for (let mac, e in st.known) {
		let bands = {};
		for (let b, t in e.bands)
			bands[b] = at(t);
		known[mac] = { bands: bands, btm: e.btm ?? null, at: at(e.at ?? 0) };
		if (e.own)
			known[mac].own = true;
	}
	let times = (m, keep) => {
		let out = {};
		for (let mac, t in m)
			if (keep(t))
				out[mac] = at(t);
		return out;
	};
	let counts = (m) => {
		let out = {};
		for (let mac, b in m)
			if (now - b.at < BACKS_FOR)
				out[mac] = [b.n, at(b.at)];
		return out;
	};
	// Each rule of the nearest stopped here (0321): when, and the rule.
	let stops = () => {
		let out = {};
		for (let mac, s in st.stopped)
			if (now - s.at < BACKS_FOR)
				out[mac] = [at(s.at), s.rule];
		return out;
	};
	return {
		v: 2, known: known,
		came_back: times(st.came_back, t => now - t < BACKS_FOR),
		tried: times(st.tried, t => now - t < BACKS_FOR),
		elsewhere: times(st.elsewhere, t => now - t <= ELSEWHERE_FOR),
		rest: times(st.rest, t => t > now),
		backs: counts(st.backs), downs: counts(st.downs),
		down: times(st.down, t => t > now),
		home_backs: counts(st.home_backs), stopped: stops(),
	};
}

// restore takes in what a run before kept (kept), by the clock as off and
// now are this run's; or what a daemon from before 0289 kept, its clients'
// bands alone, by MAC. Each is checked: a MAC, a time a whole number;
// when one came back, within BACKS_FOR and not to come; when another AP
// heard one, within its day and not to come; a rest, to come and no longer
// than a day. Of each, the latest are taken, as many as is kept of each in
// a run.
function restore(st, k, off, now) {
	if (type(k) != 'object')
		return;
	let mac_re = /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/;
	let known = k.v == 2 ? k.known : k;
	for (let mac, e in (type(known) == 'object' ? known : {})) {
		if (!match(mac, mac_re) || type(e?.bands) != 'object')
			continue;
		let bands = {};
		for (let b, t in e.bands)
			if (RANK[b] != null && type(t) == 'int')
				bands[b] = t - off;
		st.known[mac] = { bands: bands, at: (type(e.at) == 'int' ? e.at : 0) - off };
		if (e.btm != null)
			st.known[mac].btm = !!e.btm;
		if (e.own === true)
			st.known[mac].own = true;
	}
	if (k.v != 2)
		return;
	let take = (into, from, ok, most) => {
		let list = [];
		for (let mac, t in (type(from) == 'object' ? from : {}))
			if (match(mac, mac_re) && type(t) == 'int' && ok(t - off))
				push(list, [mac, t - off]);
		for (let x in slice(sort(list, (a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1)), 0, most))
			into[x[0]] = x[1];
	};
	take(st.came_back, k.came_back, t => t <= now && now - t < BACKS_FOR, KNOWN_MOST);
	take(st.tried, k.tried, t => t <= now && now - t < BACKS_FOR, KNOWN_MOST);
	take(st.elsewhere, k.elsewhere, t => t <= now && now - t <= ELSEWHERE_FOR, ELSEWHERE_MOST);
	take(st.rest, k.rest, t => t > now && t - now <= REST_MOST, KNOWN_MOST);
	take(st.down, k.down, t => t > now && t - now <= REST_MOST, KNOWN_MOST);
	// How often each came back and was sent down: a count from 1 to
	// BACKS_MOST and when last, within BACKS_FOR and not to come.
	let tally = (into, from) => {
		let list = [];
		for (let mac, x in (type(from) == 'object' ? from : {}))
			if (match(mac, mac_re) && type(x) == 'array' && length(x) == 2 && type(x[0]) == 'int' && x[0] >= 1 && x[0] <= BACKS_MOST &&
			    type(x[1]) == 'int' && x[1] - off <= now && now - (x[1] - off) < BACKS_FOR)
				push(list, [mac, x[0], x[1] - off]);
		for (let x in slice(sort(list, (a, b) => b[2] - a[2] || (a[0] < b[0] ? -1 : 1)), 0, KNOWN_MOST))
			into[x[0]] = { n: x[1], at: x[2] };
	};
	tally(st.backs, k.backs);
	tally(st.downs, k.downs);
	tally(st.home_backs, k.home_backs);
	// Each rule of the nearest stopped: when, within BACKS_FOR and not to
	// come, and the rule as rule_key says it.
	let list = [];
	for (let mac, x in (type(k.stopped) == 'object' ? k.stopped : {}))
		if (match(mac, mac_re) && type(x) == 'array' && length(x) == 2 && type(x[0]) == 'int' && x[0] - off <= now && now - (x[0] - off) < BACKS_FOR &&
		    type(x[1]) == 'string' && length(x[1]) <= 512)
			push(list, [mac, x[0] - off, x[1]]);
	for (let x in slice(sort(list, (a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1)), 0, KNOWN_MOST))
		st.stopped[x[0]] = { at: x[1], rule: x[2] };
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { RANK, HOLD_MS, OWN_MARK, FULL, STICKY, PROBE_FRESH, UP_HEARD, fresh, bands_of, own_of, btm_of, learned, told, shares, heard, sent, tick, summary, middle, by_load, probes, sticky_with, radio, loads_near, aps_near, ask_channels, kept, restore, home_of, chans_near, scanned, offset };
