// aeolus.minutes: what a minute came to (0207). aeolus-live samples each
// radio and each of its clients every EVERY seconds, and at the minute's
// end works out from the samples what the minute came to: how busy the
// radio's channel was, on average and at its busiest; its noise and its
// clients; and for each client, its signal and rates, and what was sent to
// it, tried again and lost. The manager keeps every minute, for inference
// and for the pages that show it.
//
// A sample's counts run on: the survey's of the radio's channel, in ms,
// and the station's of a client. Each minute is reckoned from the last
// sample of the minute before, its base, to its own last.

'use strict';

const EVERY = 5;           // seconds between two samples
const KEPT = 60;           // minutes kept while they cannot be sent; past that, the oldest go
const CLIENTS_MOST = 128;  // clients a minute says, those that moved the most data first

// num says whether v is a number.
function num(v) {
	return type(v) in ['int', 'double'];
}

// STEP_MOST is more than a running count grows by between two looks: a
// step of it or more is a count that went below zero in a driver's 32 bits
// (0272; mt76's tx retries on 25.12, B124), not growth.
const STEP_MOST = 2147483648;

// grew_by is what a running count grew by from was to now, or null where it
// did not: it went back, as when it started again, or it went below zero.
function grew_by(was, now) {
	if (!num(was) || !num(now))
		return null;
	let d = now - was;
	return d >= 0 && d < STEP_MOST ? d : null;
}

// tenth rounds to one decimal place; whole, to a whole number.
function tenth(x) {
	return x >= 0 ? int(x * 10 + 0.5) / 10.0 : -int(-x * 10 + 0.5) / 10.0;
}

function whole(x) {
	return x >= 0 ? int(x + 0.5) : -int(-x + 0.5);
}

// mbit is a rate as nl80211 says it, in 100 kbit/s, in Mbit/s.
function mbit(b) {
	let v = b?.bitrate32 ?? b?.bitrate;
	return num(v) && v > 0 ? v / 10.0 : null;
}

// station is a client's sample, from what nl80211 says of it. On a radio
// that counts retries elsewhere (ath11k, 0164; elsewhere true), they are
// retries as it counts them, or null where it was not read.
function station(s, elsewhere, retries) {
	return {
		signal: s?.signal_avg ?? s?.signal, tx_rate: mbit(s?.tx_bitrate), rx_rate: mbit(s?.rx_bitrate),
		tx_packets: s?.tx_packets, tx_retries: elsewhere ? retries : s?.tx_retries, tx_failed: s?.tx_failed,
		rx_packets: s?.rx_packets, tx_bytes: s?.tx_bytes64 ?? s?.tx_bytes, rx_bytes: s?.rx_bytes64 ?? s?.rx_bytes,
		connected: s?.connected_time,
	};
}

// survey is a radio's sample of its own channel, from nl80211's survey:
// the running counts, in ms, of the time it was on the channel, busy,
// receiving and sending; and its noise floor.
function survey(si) {
	return si ? { time: si.time, busy: si.busy, rx: si.time_rx, tx: si.time_tx, noise: si.noise } : {};
}

// radio is what a radio's minute came to, from its base and its samples,
// oldest first: each { time, busy, rx, tx, noise } of the survey and
// { power, channel, width, clients } as it was. A stretch whose counts went
// back, as when the channel or the network changed, is passed over. busy,
// rx and tx are percent of the time on the channel, busy_max that of the
// busiest stretch.
function radio(base, samples) {
	let n = length(samples ?? []);
	if (!n)
		return null;
	let all = base ? [base, ...samples] : samples;
	let t = 0, b = 0, rx = 0, tx = 0, most = null;
	for (let i = 1; i < length(all); i++) {
		let a = all[i - 1], z = all[i];
		if (!num(a.time) || !num(z.time) || !num(a.busy) || !num(z.busy))
			continue;
		let dt = z.time - a.time, db = z.busy - a.busy;
		if (dt <= 0 || db < 0 || db > dt)
			continue;
		t += dt;
		b += db;
		if (num(a.rx) && num(z.rx) && z.rx >= a.rx && z.rx - a.rx <= dt)
			rx += z.rx - a.rx;
		if (num(a.tx) && num(z.tx) && z.tx >= a.tx && z.tx - a.tx <= dt)
			tx += z.tx - a.tx;
		let p = 100.0 * db / dt;
		if (most == null || p > most)
			most = p;
	}
	let noise = 0, nn = 0, cl = 0, clmost = 0;
	for (let s in samples) {
		// A driver may give 0 for a noise floor it did not measure.
		if (num(s.noise) && s.noise >= -127 && s.noise <= -20) {
			noise += s.noise;
			nn++;
		}
		let c = s.clients ?? 0;
		cl += c;
		if (c > clmost)
			clmost = c;
	}
	let last = samples[n - 1];
	return {
		channel: last.channel, width: last.width, power: last.power,
		busy: t ? tenth(100.0 * b / t) : null, busy_max: most != null ? tenth(most) : null,
		rx: t ? tenth(100.0 * rx / t) : null, tx: t ? tenth(100.0 * tx / t) : null,
		noise: nn ? whole(1.0 * noise / nn) : null,
		clients: tenth(1.0 * cl / n), clients_max: clmost,
	};
}

// COUNTS are a client's running counts, which a minute takes the growth of.
const COUNTS = ['tx_packets', 'tx_retries', 'tx_failed', 'rx_packets', 'tx_bytes', 'rx_bytes'];

// client is what a client's minute on one radio came to, from its base and
// its samples, oldest first, each a station. Its counts are what they grew
// by: a client that joined again, its connected time gone back, counts
// afresh; a count a sample does not say is taken from the next that does;
// one that went below zero is passed over, and grows on from the count
// before it (0272). secs is how much of the minute it was there.
function client(base, samples, every) {
	let n = length(samples ?? []);
	if (!n)
		return null;
	let sig = 0, sn = 0, weakest = null, txr = 0, txn = 0, rxr = 0, rxn = 0;
	for (let s in samples) {
		if (num(s.signal) && s.signal < 0 && s.signal >= -127) {
			sig += s.signal;
			sn++;
			if (weakest == null || s.signal < weakest)
				weakest = s.signal;
		}
		if (num(s.tx_rate)) {
			txr += s.tx_rate;
			txn++;
		}
		if (num(s.rx_rate)) {
			rxr += s.rx_rate;
			rxn++;
		}
	}
	let grew = {}, prev = {}, conn = null;
	for (let k in COUNTS)
		grew[k] = 0;
	for (let s in (base ? [base, ...samples] : samples)) {
		if (num(s.connected)) {
			if (conn != null && s.connected < conn)
				for (let k in COUNTS)
					prev[k] = 0;
			conn = s.connected;
		}
		for (let k in COUNTS)
			if (num(s[k])) {
				let g = grew_by(prev[k], s[k]);
				if (g != null)
					grew[k] += g;
				if (g != null || !num(prev[k]) || s[k] < prev[k])
					prev[k] = s[k];
			}
	}
	return {
		signal: sn ? whole(1.0 * sig / sn) : null, signal_min: weakest,
		tx_rate: txn ? tenth(txr / txn) : null, rx_rate: rxn ? tenth(rxr / rxn) : null,
		...grew, secs: min(60, n * (every ?? EVERY)),
	};
}

// WRAP is where a 32-bit kernel's count of an Ethernet port goes round.
const WRAP = 4294967296;

// WIRE_MOST is the fastest an Ethernet port is taken to run, in Mbit/s,
// where its speed is not known.
const WIRE_MOST = 100000;

// wire_grew is what an Ethernet port's running count grew by from was to
// now (0351): a 64-bit count grows by more in five seconds than STEP_MOST
// at 5 Gbit/s; a 32-bit kernel's goes round once at WRAP. null where it
// went back otherwise, as when the port's driver started again.
function wire_grew(was, now) {
	if (!num(was) || !num(now))
		return null;
	let d = now - was;
	if (d >= 0)
		return d;
	return was < WRAP && now < WRAP ? d + WRAP : null;
}

// port_sample is an Ethernet port's sample, as sysfs says it (0351), each a
// string or null: its counts, whether it has a link, and its speed in Mbit/s
// and duplex while it has one. A count it does not say is null.
function port_sample(f) {
	let n = v => match(v ?? '', /^[0-9]+$/) ? +v : null;
	let up = f?.carrier == '1';
	let speed = n(f?.speed);
	return {
		rx_bytes: n(f?.rx_bytes), tx_bytes: n(f?.tx_bytes), rx_errors: n(f?.rx_errors), tx_errors: n(f?.tx_errors),
		changes: n(f?.carrier_changes), up: up,
		speed: up && speed > 0 ? speed : null, full: up && f?.duplex in ['full', 'half'] ? f.duplex == 'full' : null,
	};
}

// uplink is what a minute came to on one of the uplink's Ethernet ports
// (0351), from its base and its samples, oldest first, each port_sample's
// with its time, at: the bytes each way, in Mbit/s on average over the
// stretches it covered (secs) and over its busiest stretch of a sample's
// length; the errors each way together; how often its link came or went;
// and its link, speed and duplex as last sampled. A stretch whose bytes
// went back, as when the port's driver started again, is passed over for
// its bytes.
function uplink(base, samples) {
	let n = length(samples ?? []);
	if (!n)
		return null;
	let all = base ? [base, ...samples] : samples;
	let t = 0, rx = 0, tx = 0, rx_most = null, tx_most = null, errors = 0, changes = 0;
	for (let i = 1; i < length(all); i++) {
		let a = all[i - 1], z = all[i];
		let dt = num(a.at) && num(z.at) ? z.at - a.at : 0;
		if (dt <= 0)
			continue;
		// A link that went and came, and errors, are counted whatever the
		// bytes did: a count that went back says nothing.
		errors += (wire_grew(a.rx_errors, z.rx_errors) ?? 0) + (wire_grew(a.tx_errors, z.tx_errors) ?? 0);
		changes += wire_grew(a.changes, z.changes) ?? 0;
		let drx = wire_grew(a.rx_bytes, z.rx_bytes), dtx = wire_grew(a.tx_bytes, z.tx_bytes);
		if (drx == null || dtx == null)
			continue;
		// Faster than its link can carry is a count that started again
		// below where it was, not one that went round.
		let r = drx * 8.0 / dt / 1000000, w = dtx * 8.0 / dt / 1000000;
		let most = (max(a.speed ?? 0, z.speed ?? 0) || WIRE_MOST) * 1.05;
		if (r > most || w > most)
			continue;
		t += dt;
		rx += drx;
		tx += dtx;
		if (rx_most == null || r > rx_most)
			rx_most = r;
		if (tx_most == null || w > tx_most)
			tx_most = w;
	}
	let last = samples[n - 1];
	return {
		up: last.up, speed: last.speed, full: last.full, secs: t,
		rx: t ? tenth(rx * 8.0 / t / 1000000) : null, tx: t ? tenth(tx * 8.0 / t / 1000000) : null,
		rx_max: rx_most != null ? tenth(rx_most) : null, tx_max: tx_most != null ? tenth(tx_most) : null,
		rx_bytes: rx, tx_bytes: tx, errors: errors, changes: changes,
	};
}

// minute is what a minute came to on the AP: at, its start in seconds since
// 1970; radios, by name, each { band, base, samples, clients: { mac: {
// ssid, base, samples } } }. Each radio says the counts of its clients
// together; the clients come those that moved the most data first, at most
// CLIENTS_MOST. ports, where given, are the uplink's Ethernet ports, by
// name, each { base, samples } (0351): what each came to is the minute's
// uplink, a port at a time.
function minute(at, radios, ports) {
	let out = { at: at, radios: [], clients: [] };
	if (ports != null) {
		out.uplink = [];
		for (let name in sort(keys(ports))) {
			let u = uplink(ports[name].base, ports[name].samples);
			if (u)
				push(out.uplink, { port: name, ...u });
		}
	}
	for (let name in sort(keys(radios ?? {}))) {
		let r = radios[name], m = radio(r.base, r.samples);
		if (!m)
			continue;
		let sum = {};
		for (let k in COUNTS)
			sum[k] = 0;
		for (let mac in sort(keys(r.clients ?? {}))) {
			let c = r.clients[mac], x = client(c.base, c.samples, EVERY);
			if (!x)
				continue;
			for (let k in COUNTS)
				sum[k] += x[k];
			push(out.clients, { mac: mac, band: r.band, ssid: c.ssid, ...x });
		}
		push(out.radios, { radio: name, band: r.band, ...m, ...sum });
	}
	out.clients = slice(sort(out.clients, (a, b) => (b.tx_bytes + b.rx_bytes) - (a.tx_bytes + a.rx_bytes) || (a.mac < b.mac ? -1 : 1)), 0, CLIENTS_MOST);
	return out;
}

// load is what a minute came to on each radio, for the AP's own daemons
// (0207): RRM says it in its hellos, and steering asks clients by it. By
// band: the radio, its channel, how busy the channel was, on average and at
// its busiest, its clients, and the share of what was sent to them that was
// tried again or lost, where enough was sent to say: the larger of the two
// counts, a frame lost having been tried again first, and mt76 saying one
// count as both.
function load(m) {
	let out = { at: m?.at, radios: {} };
	for (let r in m?.radios ?? []) {
		let sent = r.tx_packets ?? 0, again = max(r.tx_retries ?? 0, r.tx_failed ?? 0);
		out.radios[r.band] = {
			radio: r.radio, channel: r.channel, busy: r.busy, busy_max: r.busy_max, clients: r.clients,
			retried: sent >= 50 ? min(1000, int(again * 1000 / sent)) / 1000.0 : null,
		};
	}
	return out;
}

export { EVERY, KEPT, CLIENTS_MOST, STEP_MOST, tenth, whole, mbit, grew_by, station, survey, radio, client, minute, load, wire_grew, port_sample, uplink };
