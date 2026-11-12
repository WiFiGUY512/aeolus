// aeolus.probed: what the APs tell one another of the clients they hear by
// probe and do not serve (0289), so that a client is weighed by how each AP
// hears it, and not by how its own AP hears it alone. aeolus-steer keeps
// how strongly each such client's probes come to this AP, a band at a
// time, and writes them each look; aeolus-rrm says them, signed, to the
// other Aeolus APs it knows, as one message a hello, and keeps what each of
// those says for aeolus-steer to read. A reading is one client, its band,
// the middle of the radio's BSSIDs it was heard on (by which the AP it
// serves knows that radio in its own neighbour list), the signal, and how
// long ago.
//
// It is the judgement and checking of it and nothing else: no files, no
// sockets, no clock.

'use strict';

const FRESH = 30;          // seconds a reading holds, as aeolus.steer keeps them (PROBE_FRESH)
const FILE_FRESH = 15;     // seconds after which aeolus-steer's file of them is old: it is not running
const MOST = 64;           // readings a message says at most, the latest first; fewer where they don't fit
const BANDS = { '2g': true, '5g': true, '6g': true };

// reading says whether x is one reading as aeolus-steer writes them and a
// message carries them: [mac, band, middle, signal, age], and 's' after,
// where the scan radio heard it (0321).
function reading(x) {
	return type(x) == 'array' && (length(x) == 5 || (length(x) == 6 && x[5] === 's')) && type(x[0]) == 'string' && match(x[0], /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) &&
		BANDS[x[1]] === true && type(x[2]) == 'string' && match(x[2], /^[0-9a-f]{2}(:[0-9a-f]{2}){3}$/) &&
		type(x[3]) == 'int' && x[3] >= -127 && x[3] < 0 && type(x[4]) == 'int' && x[4] >= 0 && x[4] <= FRESH;
}

// mine is what this AP has to say, from aeolus-steer's file as read at
// wall: each reading made older by how long ago the file was written, and
// those too old then let go. None where the file is old or not there.
function mine(file, wall) {
	if (type(file?.at) != 'int' || wall - file.at > FILE_FRESH || wall < file.at - FILE_FRESH || type(file.clients) != 'array')
		return [];
	let out = [], by = max(0, wall - file.at);
	for (let x in file.clients)
		if (reading(x) && x[4] + by <= FRESH)
			push(out, [x[0], x[1], x[2], x[3], x[4] + by, ...slice(x, 5)]);
	return slice(out, 0, MOST);
}

// message is this AP's word on them, fitted to most bytes as seal makes it
// (64 for the signature): the oldest readings left out until it fits, as a
// message sent whole in one frame is never cut short on the way. Null where
// there is nothing to say.
function message(ap, at, seq, list, most) {
	if (!length(list))
		return null;
	let m = { v: 1, t: 'probed', ap: ap, at: at, seq: seq, clients: slice(list, 0, MOST) };
	while (length(m.clients) > 0 && 64 + length(sprintf('%J', m)) > most)
		m.clients = slice(m.clients, 0, length(m.clients) - 1);
	return length(m.clients) ? m : null;
}

// taken is what another AP's message says, checked: its readings, each
// well formed, at most MOST.
function taken(m) {
	if (type(m) != 'object' || m.t != 'probed' || type(m.clients) != 'array')
		return [];
	return slice(filter(m.clients, reading), 0, MOST);
}

// near is what aeolus-steer reads, from what each AP said and when this AP
// took it (by: AP -> { at, clients }), at wall: each client's readings, by
// MAC, each with the AP, its band, the radio's middle, the signal, and when
// it was heard, by this AP's clock. Readings too old by then are let go.
function near(by, wall) {
	let out = {};
	for (let ap in sort(keys(by ?? {}))) {
		let w = by[ap];
		if (type(w?.at) != 'int')
			continue;
		for (let x in w.clients ?? [])
			if (reading(x) && wall - (w.at - x[4]) <= FRESH)
				push(out[x[0]] ??= [], { ap: ap, band: x[1], middle: x[2], signal: x[3], at: w.at - x[4], kind: x[5] == 's' ? 'scan' : 'probe' });
	}
	return { at: wall, clients: out };
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { FRESH, FILE_FRESH, MOST, reading, mine, message, taken, near };
