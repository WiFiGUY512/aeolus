// aeolus.community: what the APs tell one another of clients, where
// community sharing is on (0299). For now, the bands each client said it
// can use as it joined (its Supported Operating Classes), so an AP knows a
// client can use 6 GHz before the client first joins it, as a phone
// joined the Living Room AP and was then under the Garage's. aeolus-steer
// writes what its clients said of themselves each look; aeolus-rrm says it,
// signed, to the other Aeolus APs it knows, as one message a hello, and
// keeps what each of those says for aeolus-steer to read. A reading is one
// client, the bands it said, and how long ago it said them. Only what a
// client said to this AP is told on: what an AP was told is not, so no word
// goes round.
//
// It is the judgement and checking of it and nothing else: no files, no
// sockets, no clock.

'use strict';

const FRESH = 60;          // seconds a reading holds on its way: it is told again each hello while the client is on
const FILE_FRESH = 15;     // seconds after which aeolus-steer's file of them is old: it is not running
const MOST = 32;           // readings a message says at most, the latest first; fewer where they don't fit
const BANDS = { '2g': true, '5g': true, '6g': true };

// bands_ok says whether x is a client's bands as a reading carries them:
// one to three of them, each once.
function bands_ok(x) {
	if (type(x) != 'array' || length(x) < 1 || length(x) > 3)
		return false;
	let seen = {};
	for (let b in x) {
		if (BANDS[b] !== true || seen[b])
			return false;
		seen[b] = true;
	}
	return true;
}

// reading says whether x is one reading as aeolus-steer writes them and a
// message carries them: [mac, bands, age].
function reading(x) {
	return type(x) == 'array' && length(x) == 3 && type(x[0]) == 'string' && match(x[0], /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) &&
		bands_ok(x[1]) && type(x[2]) == 'int' && x[2] >= 0 && x[2] <= FRESH;
}

// mine is what this AP has to say, from aeolus-steer's file as read at
// wall: each reading made older by how long ago the file was written, and
// those too old then let go. None where the file is old or not there.
function mine(file, wall) {
	if (type(file?.at) != 'int' || wall - file.at > FILE_FRESH || wall < file.at - FILE_FRESH || type(file.clients) != 'array')
		return [];
	let out = [], by = max(0, wall - file.at);
	for (let x in file.clients)
		if (type(x) == 'array' && type(x[2]) == 'int' && reading([x[0], x[1], min(x[2], FRESH)]) && x[2] + by <= FRESH)
			push(out, [x[0], x[1], x[2] + by]);
	return slice(out, 0, MOST);
}

// message is this AP's word on them, fitted to most bytes as seal makes it
// (64 for the signature): the oldest readings left out until it fits, as a
// message sent whole in one frame is never cut short on the way. Null where
// there is nothing to say.
function message(ap, at, seq, list, most) {
	if (!length(list))
		return null;
	let m = { v: 1, t: 'community', ap: ap, at: at, seq: seq, clients: slice(list, 0, MOST) };
	while (length(m.clients) > 0 && 64 + length(sprintf('%J', m)) > most)
		m.clients = slice(m.clients, 0, length(m.clients) - 1);
	return length(m.clients) ? m : null;
}

// taken is what another AP's message says, checked: its readings, each
// well formed, at most MOST.
function taken(m) {
	if (type(m) != 'object' || m.t != 'community' || type(m.clients) != 'array')
		return [];
	return slice(filter(m.clients, reading), 0, MOST);
}

// near is what aeolus-steer reads, from what each AP said and when this AP
// took it (by: AP -> { at, clients }), at wall: each client's readings, by
// MAC, each with the AP, the bands, and when the client said them, by this
// AP's clock. Readings too old by then are let go.
function near(by, wall) {
	let out = {};
	for (let ap in sort(keys(by ?? {}))) {
		let w = by[ap];
		if (type(w?.at) != 'int')
			continue;
		for (let x in w.clients ?? [])
			if (reading(x) && wall - (w.at - x[2]) <= FRESH)
				push(out[x[0]] ??= [], { ap: ap, bands: x[1], at: w.at - x[2] });
	}
	return { at: wall, clients: out };
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { FRESH, FILE_FRESH, MOST, reading, mine, message, taken, near };
