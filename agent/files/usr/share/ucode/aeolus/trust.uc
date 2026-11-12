// What airscan is to trust (0304, F3): the fleet's SSIDs and BSSIDs, as
// the manager hands them, in airscan's own config, its trusted_ssid and
// trusted_bssid lists. airscan reads them before each of its BSS scans, so
// nothing restarts it. What a person put in those lists stays: the agent
// keeps a note of what it added, and takes away only that.

'use strict';

const MOST = 512;   // the most of each list taken from the manager

// LISTS are airscan's lists, by what of the manager's each holds.
const LISTS = { trusted_ssid: 'ssids', trusted_bssid: 'bssids' };

// clean is the manager's doc as airscan's lists: SSIDs of 1 to 32 bytes,
// BSSIDs in lower case, each once and sorted; null where the doc is not
// one.
function clean(doc) {
	if (type(doc?.ssids) != 'array' || type(doc?.bssids) != 'array')
		return null;
	let ssids = filter(doc.ssids, s => type(s) == 'string' && length(s) >= 1 && length(s) <= 32 && !match(s, /[\n\r]/));
	let bssids = map(filter(doc.bssids, b => type(b) == 'string' && match(lc(b), /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/)), b => lc(b));
	return {
		trusted_ssid: slice(sort(uniq(ssids)), 0, MOST),
		trusted_bssid: slice(sort(uniq(bssids)), 0, MOST),
	};
}

// listOf is an option of uci's as a list: a list, one value, or none.
function listOf(v) {
	return type(v) == 'array' ? v : (v != null ? [ v ] : []);
}

// merge is what one of airscan's lists is to be, from what it is now, what
// the agent added to it last (mine), and what the manager says (want):
// what a person put there, and what the manager says. next is the list,
// sorted, and mine what of it the agent added, which a person had not.
function merge(now, mine, want) {
	let ours = {};
	for (let v in mine ?? [])
		ours[v] = true;
	let theirs = filter(now ?? [], v => !ours[v]);
	let had = {};
	for (let v in theirs)
		had[v] = true;
	return {
		next: sort(uniq([ ...theirs, ...want ])),
		mine: filter(want, v => !had[v]),
	};
}

// same is whether two lists hold the same values, in any order.
function same(a, b) {
	return join('\n', sort([ ...(a ?? []) ])) == join('\n', sort([ ...(b ?? []) ]));
}

export { MOST, LISTS, clean, listOf, merge, same };
