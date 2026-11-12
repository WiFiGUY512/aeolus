// The names of the AP's Wi-Fi interfaces, as netifd's network.wireless
// status lists them, through a radio being set up again (0301).
//
// When netifd sets up a radio that is running again, on a reconfigure, it
// forgets what the radio's handler said of its interfaces (wifi-scripts'
// wireless-device.uc, wdev_config_init). Until the handler names them
// again, the status lists each of the radio's interfaces without its
// ifname, and the radio as not up. Radios are set up one at a time, so the
// last one goes seconds without names. The interfaces never stop
// meanwhile, and the radio says pending throughout, from the same step
// that forgets (porting's B43, closed as by design on 2026-11-03). A
// reader that took a missing name for an interface gone let go of what it
// held there and took it up again a look later: steering's listeners, the
// journey's sockets, a radio's RRM state.
//
// A daemon, which looks again and again, sees the status through a keeper
// of its own: a pending radio has back what its last settled look had of
// it, where that is still so. A look made once waits for netifd to finish
// instead, with settle, for a while at most.

'use strict';

import * as fs from 'fs';

const HOLD = 120;    // seconds a pending radio is seen as it was: a setup that takes longer is no setup of a running radio
const SETTLE = 20;   // seconds a look made once waits for netifd to finish setting up its radios
const STEP = 1000;   // milliseconds between its asks

// CARRIED is what netifd leaves out of an interface while its radio is
// pending, by field, each with what says the radio's last settled look's
// value is still so: its name, while the kernel still has an interface by
// that name. A field netifd comes to leave out as well goes here.
const CARRIED = {
	ifname: (v, there) => type(v) == 'string' && match(v, /^[A-Za-z0-9._-]{1,15}$/) != null && there(v) == true,
};

// on_kernel says the kernel has an interface by the name.
function on_kernel(ifname) {
	return fs.access(`/sys/class/net/${ifname}`) == true;
}

// interfaces is a radio's interfaces as netifd lists them, or none.
function interfaces(radio) {
	return type(radio?.interfaces) == 'array' ? radio.interfaces : [];
}

// pending lists the radios netifd is setting up or taking down, sorted.
function pending(status) {
	return type(status) == 'object' ? sort(filter(keys(status), r => status[r]?.pending == true)) : [];
}

// fields is what a later look may need of a settled radio's interfaces, by
// section: each of CARRIED's fields an interface has.
function fields(radio) {
	let out = {};
	for (let i in interfaces(radio)) {
		if (type(i) != 'object' || type(i.section) != 'string')
			continue;
		let f = {};
		for (let k in keys(CARRIED))
			if (i[k] != null)
				f[k] = i[k];
		if (length(f))
			out[i.section] = f;
	}
	return out;
}

// carry is netifd's status as a daemon is to see it. A pending radio
// anything of whose last settled look (before) is still so (there says
// which interfaces the kernel has; on_kernel unless given) is held: it
// gets back, interface by interface, each of CARRIED's fields netifd
// leaves out that is still so, and the up it had then. One netifd already
// names is held too, as where netifd keeps the names itself (porting's
// drafted change): it still says the radio is not up. A radio that is not
// pending is as netifd has it. So is a pending one never seen settled, as
// at its first setup; one nothing is still so of, as one taken down; and
// one pending for more than HOLD seconds. It returns the status so seen,
// and what to keep for the next look: by radio, its last settled look's up
// and fields, and since when it has been pending.
function carry(status, before, now, there) {
	let out = {}, kept = {};
	there ??= on_kernel;
	for (let r, d in (type(status) == 'object' ? status : {})) {
		out[r] = d;
		if (type(d) != 'object')
			continue;
		if (d.pending != true) {
			kept[r] = { up: d.up == true, ifaces: fields(d) };
			continue;
		}
		let was = before?.[r];
		if (type(was?.ifaces) != 'object')
			continue;
		kept[r] = { ...was, since: was.since ?? now };
		if (now - kept[r].since > HOLD)
			continue;
		let held = false, ifs = [];
		for (let i in interfaces(d)) {
			let had = type(i) == 'object' && type(i.section) == 'string' ? was.ifaces[i.section] : null, add = null;
			for (let k, still in CARRIED)
				if (type(had) == 'object' && had[k] != null && still(had[k], there)) {
					held = true;
					if (i[k] == null) {
						add ??= {};
						add[k] = had[k];
					}
				}
			push(ifs, add ? { ...i, ...add } : i);
		}
		if (held)
			out[r] = { ...d, up: was.up || d.up == true, held: true, interfaces: ifs };
	}
	return { status: out, kept: kept };
}

// keeper is a daemon's own: its view(status) is netifd's status as carry
// has it, against what the daemon's last look kept. A status that is none,
// as null where netifd's object is gone, is given back as it is, and what
// was kept stays for the next.
function keeper(there) {
	let kept = {};
	return {
		view: (status) => {
			if (type(status) != 'object')
				return status;
			let seen = carry(status, kept, time(), there);
			kept = seen.kept;
			return seen.status;
		},
	};
}

// settle is netifd's status for a look made once: ask()'s answer, asked
// again every step milliseconds (STEP) while a radio is pending, for wait
// seconds (SETTLE) at most. A radio still pending then lists its
// interfaces without names: the caller takes them for neither there nor
// gone, and leaves what it holds of them as it is.
function settle(ask, wait, step) {
	step ??= STEP;
	let tries = int((wait ?? SETTLE) * 1000 / step), st = ask();
	while (tries-- > 0 && length(pending(st))) {
		sleep(step);
		st = ask();
	}
	return st;
}

export { HOLD, SETTLE, CARRIED, pending, carry, keeper, settle };
