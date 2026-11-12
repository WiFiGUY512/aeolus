// aeolus.record: the flight recorder's judgement (0207 part 3). aeolus-live
// keeps what each five seconds came to on the AP's radios, the last KEEP
// seconds of it; when something goes wrong, it sends the BEFORE seconds
// before and goes on sending AFTER seconds more. This says when something
// has gone wrong, by what the five seconds came to against the radio's own
// last five minutes: what was sent to its clients tried again or lost all
// at once, many of its clients gone at once, or its channel pinned full.

'use strict';

const KEEP = 1800;       // seconds of five-second points the AP keeps
const BEFORE = 600;      // seconds before what went wrong that a recording begins
const AFTER = 600;       // and after it that it goes on
const MOST = 3600;       // seconds a recording may come to, made longer by what goes wrong in it
const GAP = 1800;        // seconds between two recordings the AP begins itself
const CLIENTS_KEPT = 24; // clients a kept point says, those moving the most data first
const BASE = 60;         // points before that a radio's usual is reckoned from: five minutes
const BASE_LEAST = 12;   // points at least that it takes to say what is usual

// share is what of what was sent to a radio's clients was tried again or
// lost: the larger of the two counts, a frame lost having been tried again
// first, and mt76 saying one count as both; null where too little was sent
// to say, least packets.
function share(r, least) {
	let sent = r?.tx_packets ?? 0;
	return sent >= (least ?? 50) ? min(1.0, max(r.tx_retries ?? 0, r.tx_failed ?? 0) / (1.0 * sent)) : null;
}

// median is the middle of the numbers, or null of none.
function median(xs) {
	let s = sort(filter(xs, x => x != null), (a, b) => a - b), n = length(s);
	return n ? (n % 2 ? s[int(n / 2)] : (s[n / 2 - 1] + s[n / 2]) / 2.0) : null;
}

// average is the average of the numbers.
function average(xs) {
	let t = 0.0;
	for (let x in xs)
		t += x;
	return t / length(xs);
}

// radio_in is a radio of a point, by band.
function radio_in(p, band) {
	for (let r in p?.radios ?? [])
		if (r.band == band)
			return r;
	return null;
}

// down_all says whether every radio in point is down, without a channel,
// where some had one in prev, the point before (Griff, 2026-10-23: the
// advisor took the clients a reloaded driver dropped for a channel move's).
function down_all(prev, point) {
	let rs = point?.radios ?? [];
	return length(filter(prev?.radios ?? [], r => r.channel != null)) > 0 && length(rs) > 0 &&
		!length(filter(rs, r => r.channel != null));
}

// restarted says, as news, how the radios came back (Griff, 2026-10-23):
// was and now are each radio's phy, by name, when it was last up and now;
// down, since when every radio was down, or null; at, now. A radio on
// another phy had its driver loaded again ('driver'): its wiphy was made
// anew, which no restart of the Wi-Fi alone does. Else, where they were all
// down, the Wi-Fi was restarted ('wifi'). It returns { t, kind, at, back,
// down_s, how, radios, from, to }, or null where nothing restarted.
function restarted(was, now, down, at) {
	let changed = sort(filter(keys(now ?? {}), n => was?.[n] != null && now[n] != null && was[n] != now[n]));
	if (!length(changed) && down == null)
		return null;
	let radios = length(changed) ? changed : sort(keys(now ?? {}));
	return {
		t: 'news', kind: 'radios-restart', at: down ?? at, back: at, down_s: down != null ? at - down : 0,
		how: length(changed) ? 'driver' : 'wifi', radios: radios,
		from: map(radios, n => was?.[n]), to: map(radios, n => now[n]),
	};
}

// trigger says whether something went wrong in point, a five-second point,
// against the points before it (oldest first, point not among them): every
// radio down at once, said first, so that what follows from it, as its
// clients gone, is not taken for its cause; on a radio, the share tried again or lost at least 0.4 of 200 packets or
// more, where it is usually under 0.15; at least three of its clients and
// a third of them gone since the point before; or its channel 95% busy or
// more for fifteen seconds, where it is usually under 60%. It returns {
// why, band, said } or null.
function trigger(before, point) {
	let n = length(before ?? []);
	if (down_all(n ? before[n - 1] : null, point))
		return { why: 'radios-down', band: 'all', said: 'every radio went down at once: the Wi-Fi restarted, or its driver was reloaded' };
	for (let r in point?.radios ?? []) {
		let base = slice(before, max(0, n - BASE));
		// What tried again or lost: all at once, against its usual.
		let s = share(r, 200);
		if (s != null && s >= 0.4) {
			let usual = median(map(base, p => share(radio_in(p, r.band), 50)));
			if (usual != null && length(filter(base, p => share(radio_in(p, r.band), 50) != null)) >= BASE_LEAST && usual < 0.15)
				return { why: 'retries', band: r.band, said: sprintf('%d%% of %d tried again or lost in five seconds, against %d%% as usual', int(s * 100), r.tx_packets, int(usual * 100)) };
		}
		// Clients gone at once.
		let prev = n ? radio_in(before[n - 1], r.band) : null;
		if (prev?.clients != null && r.clients != null) {
			let gone = prev.clients - r.clients;
			if (gone >= 3 && gone >= prev.clients / 3.0)
				return { why: 'clients-left', band: r.band, said: sprintf('%d of its %d clients gone in five seconds', int(gone), int(prev.clients)) };
		}
		// Its channel pinned full.
		if (r.busy != null && r.busy >= 95 && n >= 2) {
			let p1 = radio_in(before[n - 1], r.band), p2 = radio_in(before[n - 2], r.band);
			if (p1?.busy >= 95 && p2?.busy >= 95) {
				let earlier = filter(map(slice(base, 0, max(0, length(base) - 2)), p => radio_in(p, r.band)?.busy), b => b != null);
				let usual = length(earlier) >= BASE_LEAST ? average(earlier) : null;
				if (usual != null && usual < 60)
					return { why: 'busy', band: r.band, said: sprintf('its channel %d%% busy or more for fifteen seconds, against %d%% as usual', 95, int(usual)) };
			}
		}
	}
	return null;
}

// kept is a point as the AP keeps it: its clients those moving the most
// data first, at most CLIENTS_KEPT.
function kept(p) {
	if (length(p?.clients ?? []) <= CLIENTS_KEPT)
		return p;
	return { ...p, clients: slice(sort([...p.clients], (a, b) => ((b.tx_bytes ?? 0) + (b.rx_bytes ?? 0)) - ((a.tx_bytes ?? 0) + (a.rx_bytes ?? 0))), 0, CLIENTS_KEPT) };
}

// window is a recording begun at at for why, or one going on made longer:
// { at, why, from, until }, from BEFORE seconds before what went wrong to
// AFTER after it, at most MOST from its start.
function window(rec, at, why, before, after) {
	before ??= BEFORE;
	after ??= AFTER;
	if (rec && at <= rec.until)
		return { ...rec, until: min(max(rec.until, at + after), rec.from + MOST) };
	return { at: at, why: why, from: at - before, until: at + after };
}

export { KEEP, BEFORE, AFTER, MOST, GAP, CLIENTS_KEPT, share, median, trigger, kept, window, down_all, restarted };
