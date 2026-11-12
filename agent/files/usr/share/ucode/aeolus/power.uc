// The transmit power Aeolus sets a radio to, held with iw (0349).
//
// netifd sets a radio's power from its txpower option as it sets the radio
// up, so a change to it is a change to the Wi-Fi's config, and the Wi-Fi is
// set up again to make it. A running radio takes a power at once with iw,
// and no client notices (0077). So the renderer writes the power Aeolus sets
// into the agent's own package, the aeolus_power section, by radio, and
// aeolus-power holds each radio at it: after each apply, every few minutes,
// and when hostapd makes a radio's networks anew, which netifd does with the
// radio's own power. This is what it decides; aeolus-power does it.

'use strict';

const LEAST = 1;    // dBm: the least a power may be set to, as the schema has it
const MOST = 30;    // dBm: the most

// wanted is the power set for each radio, by its section's name, from the
// aeolus_power section as uci's get_all() gives it: whole dBm from LEAST to
// MOST. Anything else is no power set.
function wanted(sec) {
	let out = {};
	for (let k, v in sec ?? {}) {
		if (substr(k, 0, 1) == '.' || !match(k, /^[A-Za-z0-9_]+$/) || !match('' + v, /^[0-9]+$/))
			continue;
		let p = int(v);
		if (p >= LEAST && p <= MOST)
			out[k] = p;
	}
	return out;
}

// held reads what aeolus-power wrote as it held a radio: its power, its
// wiphy and the index of its first network's interface, which a radio set
// up again does not keep. null for anything else.
function held(text) {
	let m = match(trim(text ?? ''), /^([0-9]+) (phy[0-9]+) ([0-9]+)$/);
	return m ? { power: int(m[1]), phy: m[2], index: m[3] } : null;
}

// said is what aeolus-power writes as it holds a radio, which held reads.
function said(power, phy, index) {
	return sprintf('%d %s %s\n', power, phy, index);
}

// plan is what to do for each radio. radios: each radio netifd has, as
// { radio, phy, index, power }, phy and index of its first network's
// interface, null where none of its interfaces is there, and power what the
// radio says it sends at now, in dBm, null where it does not say. was: what is held, by radio,
// as held() reads it. want: wanted(). settled: netifd is setting up no radio
// again, so a radio it does not list is gone.
//
// Each step is { radio, do, power, phy, why }:
//	hold    set the radio's wiphy to power (new: it held none; changed: another
//	        power; again: the radio was set up again, and netifd put back its own;
//	        louder: it sends louder than it is held at, as netifd sets it to its
//	        own power when it reloads its settings, its interfaces kept)
//	let_go  no power is set for it now: back to the radio's own (auto)
//	forget  what it held is gone with the radio, or was put back by netifd
//	shared  a power is set, but its wiphy is another radio's too: power is the
//	        wiphy's, so it is left alone
//
// A radio whose interfaces are not there, as while netifd sets it up again,
// is neither held nor let go: it is looked at again next time.
function plan(radios, was, want, settled) {
	let steps = [], on = {}, seen = {};
	for (let r in radios ?? [])
		if (r.phy)
			on[r.phy] = (on[r.phy] ?? 0) + 1;
	for (let r in radios ?? []) {
		seen[r.radio] = true;
		if (!r.phy)
			continue;
		let w = want?.[r.radio], h = was?.[r.radio];
		let same = h && h.phy == r.phy && h.index == '' + r.index;
		if (on[r.phy] > 1) {
			if (w != null)
				push(steps, { radio: r.radio, do: 'shared', power: w, phy: r.phy });
			if (h)
				push(steps, { radio: r.radio, do: same ? 'let_go' : 'forget', phy: r.phy });
			continue;
		}
		if (w != null) {
			// Quieter than it is held at is the most its rules or its
			// radio let it send, which holding it again would not change.
			let louder = type(r.power) in ['int', 'double'] && r.power > w + 0.5;
			if (!same || h.power != w || louder)
				push(steps, { radio: r.radio, do: 'hold', power: w, phy: r.phy,
					why: !h ? 'new' : !same ? 'again' : h.power != w ? 'changed' : 'louder' });
		}
		else if (h)
			push(steps, { radio: r.radio, do: same ? 'let_go' : 'forget', phy: r.phy });
	}
	if (settled)
		for (let name in sort(keys(was ?? {})))
			if (!seen[name])
				push(steps, { radio: name, do: 'forget' });
	return steps;
}

// command is the iw command a step runs, or null for one that runs none.
function command(step) {
	if (step.do == 'hold')
		return sprintf("iw phy '%s' set txpower fixed %d", step.phy, step.power * 100);
	if (step.do == 'let_go')
		return sprintf("iw phy '%s' set txpower auto", step.phy);
	return null;
}

export { LEAST, MOST, wanted, held, said, plan, command };
