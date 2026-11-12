// The live daemon's minutes, watched (0307). aeolus-live writes what each
// minute came to on the AP's radios (load.json) as the minute ends, and
// sends the minutes to the manager. On the Living Room C-360 on
// 2026-11-04 it went on running and sent no radio's minute for 13 hours,
// from a module swap that set its radios up again under new phys, until an
// agent update started every instance again. procd starts a daemon that
// ends again, not one that runs and does nothing. So the agent judges, at
// each poll, whether the live daemon's minutes stopped: load.json not
// written for STALE seconds, or written with no radio for STALE seconds
// while hostapd serves networks. Then it has procd start it again, at most
// every AGAIN seconds.

'use strict';

const STALE = 300;    // seconds without a minute, or without a radio while hostapd serves
const AGAIN = 1800;   // seconds between two restarts at the least

// judge says why the live daemon is to be started again, or null, and
// since when its minutes have had no radio while hostapd served (empty, to
// keep for the next look): now in seconds; mtime, when load.json was last
// written, null where it never was (a daemon that takes no minutes, which
// is not started again for it); load, what it says; serving, whether
// hostapd serves networks; last, when it was last started again for this.
function judge(now, mtime, load, serving, empty, last) {
	if (mtime == null)
		return { why: null, empty: null };
	let radios = length(keys(type(load?.radios) == 'object' ? load.radios : {}));
	empty = serving && !radios ? (empty ?? now) : null;
	let why = null;
	if (now - mtime > STALE)
		why = sprintf('no minute for %d s', now - mtime);
	else if (empty != null && now - empty > STALE)
		why = sprintf('no radio in its minutes for %d s while hostapd serves', now - empty);
	if (why && last != null && now - last < AGAIN)
		why = null;
	return { why: why, empty: empty };
}

export { STALE, AGAIN, judge };
