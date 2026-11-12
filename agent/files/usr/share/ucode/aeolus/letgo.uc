// Active client management (0226): an AP that lets a client on tells the
// other Aeolus APs it knows of, over the wire, from management address to
// management address, so one that still holds the client lets go of it.
// It lets go saying nothing in the air (hostapd's DEAUTHENTICATE with
// tx=0): the client is on another AP's channel, and hears nothing of it.
// A client that comes back to an AP it left is then let on as any client
// is, by 802.11r where it can: the AP no longer holds it as on, so it
// neither asks the client to prove itself first (802.11w's SA Query, a
// second) nor refuses its fast transition.
//
// The messages are signed as the hellos are (aeolus.rrm), and go to the
// same port. This module is pure: it makes and reads them, and reads what
// hostapd says of a client; the daemons send them and act on them.

'use strict';

const HERE_MACS = 16;   // MACs a message names at most
const HELD_FOR = 2;     // seconds a client must have been on at an AP for it to be let go there
const SAID_EVERY = 1;   // seconds before the same client's coming is said again

// here is the message that says clients were let on at the AP: of this
// version, from the AP, when, and in what order among its messages.
function here(ap, at, seq, macs) {
	return { v: 1, t: 'here', ap: ap, at: at, seq: seq, macs: slice(macs, 0, HERE_MACS) };
}

// here_macs is what a here message names: the MACs, each once, in lower
// case, at most HERE_MACS; none where it is no such message.
function here_macs(m) {
	if (type(m) != 'object' || m.t != 'here' || type(m.macs) != 'array')
		return [];
	let out = [];
	for (let x in m.macs) {
		if (length(out) >= HERE_MACS)
			break;
		if (type(x) == 'string' && match(lc(x), /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/) && index(out, lc(x)) < 0)
			push(out, lc(x));
	}
	return out;
}

// held says, of what hostapd's STA command answered of a client on one of
// its networks, whether to let go of it there: the client is there, and
// has been on HELD_FOR seconds or more, or hostapd does not say how long.
// One on for less came back there after another AP let it on: it is where
// it is, and is left be.
function held(text, mac) {
	if (type(text) != 'string' || type(mac) != 'string' || index(lc(text), lc(mac)) != 0)
		return false;
	let m = match(text, /\nconnected_time=([0-9]+)/);
	return !m || int(m[1]) >= HELD_FOR;
}

// due says whether a client's coming is to be said: not where it was said
// less than SAID_EVERY ago. said is { mac: when } and is kept up here.
function due(said, mac, at) {
	if (said[mac] != null && at - said[mac] < SAID_EVERY)
		return false;
	said[mac] = at;
	for (let m in keys(said))
		if (at - said[m] > 60)
			delete said[m];
	return true;
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { HERE_MACS, HELD_FOR, SAID_EVERY, here, here_macs, held, due };
