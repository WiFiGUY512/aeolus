// The names of the AP's Wi-Fi interfaces through a radio being set up
// again (0301): TestAgentWifiNames runs this with the real ucode and
// compares what it prints with wifinames.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/wifinames.uc

'use strict';

import * as wifinames from 'aeolus.wifinames';

// iface is one of netifd's interfaces, as the Living Room C-360 lists
// them (2026-11-04); no name where netifd has forgotten it.
function iface(section, ifname, net) {
	let i = { section: section, config: { mode: 'ap', network: [ 'aeolus_' + net ], ssid: net } };
	if (ifname)
		i.ifname = ifname;
	return i;
}

// radio is one of netifd's radios. Settled, its interfaces are named and
// it is up; pending, as while netifd sets it up again, they are not, and
// it is not up.
function radio(band, phy, nets, pending) {
	let ifs = [];
	for (let n = 0; n < length(nets); n++)
		push(ifs, iface(`aeolus_${nets[n]}_${phy}`, pending ? null : `${phy}-ap${n}`, nets[n]));
	return { up: !pending, pending: !!pending, config: { band: band }, interfaces: ifs };
}

const NETS = [ 'aeolus_50', 'enterprise', 'home_wi_fi', 'home_wi_fi_iot' ];

function status(pending) {
	return {
		radio0: radio('5g', 'phy0', NETS, pending.radio0),
		radio1: radio('2g', 'phy1', NETS, pending.radio1),
		radio3: radio('6g', 'phy2', [ 'home_wi_fi' ], pending.radio3),
	};
}

// The kernel's interfaces, which a test takes away.
let kernel = {};
for (let p in [ 'phy0', 'phy1' ])
	for (let n = 0; n < 4; n++)
		kernel[`${p}-ap${n}`] = true;
kernel['phy2-ap0'] = true;
let there = (ifname) => kernel[ifname] == true;

// taken is what a daemon took up of a status before 0301, as aeolus-rrm and
// aeolus-live did: each radio that is up, with its AP interfaces that have
// a name; and which radios were held.
function taken(st) {
	let out = [];
	for (let r in sort(keys(st ?? {}))) {
		let d = st[r];
		let names = map(filter(d.interfaces ?? [], i => i?.ifname && i.config?.mode == 'ap'), i => i.ifname);
		if (d.up && length(names))
			push(out, `${r}${d.held ? ' (held)' : ''} ${join(',', names)}`);
	}
	return length(out) ? join('; ', out) : 'none';
}

let k = wifinames.keeper(there);

// Settled: as netifd has it.
printf('settled: %s\n', taken(k.view(status({}))));
// A reconfigure: netifd sets up radio0, then radio1, then radio3. Read as
// it is, a pending radio is gone; through the keeper, it is as it was.
let mid = status({ radio1: true, radio3: true });
printf('pending, as netifd has it: %s\n', taken(mid));
printf('pending, kept: %s\n', taken(k.view(mid)));
printf('pending radios: %J\n', wifinames.pending(mid));
// What the keeper gives back of a held radio is netifd's own, but for the
// names and up: its config and pending stay, and netifd's status itself is
// left as it was.
let held = k.view(mid).radio1;
printf('held radio1: up %J pending %J band %s; netifd\'s own still has %J\n', held.up, held.pending, held.config.band, mid.radio1.interfaces[0].ifname);
// radio1's last interface went from the kernel, as the reconfigure took
// its network out: only that one stays without its name.
delete kernel['phy1-ap3'];
printf('one gone: %s\n', taken(k.view(mid)));
kernel['phy1-ap3'] = true;
// A network the reconfigure adds has no name before: it has none until
// netifd names it.
let more = status({ radio1: true });
push(more.radio1.interfaces, iface('aeolus_guest_phy1', null, 'guest'));
let v = k.view(more);
printf('added: %s; the new one named %J\n', taken(v), v.radio1.interfaces[4].ifname);
// netifd names them again: as netifd has it, held no more, the new one
// with its own name.
let after = status({});
push(after.radio1.interfaces, iface('aeolus_guest_phy1', 'phy1-ap4', 'guest'));
printf('settled again: %s\n', taken(k.view(after)));
// Where netifd keeps the names itself while it sets a radio up again
// (porting's drafted wifi-scripts change), it still says the radio is not
// up: held all the same, its names its own.
let named = status({});
named.radio1.pending = true;
named.radio1.up = false;
printf('netifd keeps the names: %s\n', taken(k.view(named)));

// A radio taken down: pending, while netifd takes it down, and the kernel
// has none of its interfaces. Not held: it is going.
let k2 = wifinames.keeper(there);
k2.view(status({}));
delete kernel['phy2-ap0'];
printf('taken down: %s\n', taken(k2.view(status({ radio3: true }))));
kernel['phy2-ap0'] = true;

// A radio pending at a daemon's first look, as at boot, was never seen
// settled: nothing is kept of it.
printf('first look: %s\n', taken(wifinames.keeper(there).view(status({ radio0: true, radio1: true, radio3: true }))));

// netifd's object gone (null) is given back as it is, and what was kept
// stays: the next pending look is still kept.
let k3 = wifinames.keeper(there);
k3.view(status({}));
printf('gone: %J; then pending: %s\n', k3.view(null), taken(k3.view(status({ radio0: true }))));

// Pending for longer than HOLD seconds is no setup of a running radio: as
// netifd has it, from then on.
let first = wifinames.carry(status({}), {}, 1000, there);
let p1 = wifinames.carry(status({ radio0: true }), first.kept, 1010, there);
let p2 = wifinames.carry(status({ radio0: true }), p1.kept, 1010 + wifinames.HOLD, there);
let p3 = wifinames.carry(status({ radio0: true }), p2.kept, 1011 + wifinames.HOLD, there);
printf('hold: pending since %d; at %d s %s; at %d s %s\n', p1.kept.radio0.since, wifinames.HOLD, taken(p2.status), wifinames.HOLD + 1, taken(p3.status));
// Settled again, the count starts over.
let s4 = wifinames.carry(status({}), p3.kept, 2000, there);
let p5 = wifinames.carry(status({ radio0: true }), s4.kept, 2001, there);
printf('hold again: since %d, %s\n', p5.kept.radio0.since, taken(p5.status));

// What is not a status, a radio, or an interface is passed as it is.
let odd = {
	radio0: { up: false, pending: true, interfaces: [ null, 5, { config: {} }, { section: 'x' } ] },
	radio1: 'radio',
	radio2: { up: false, pending: true, interfaces: 'none' },
};
let k4 = wifinames.keeper(there);
k4.view({ radio0: { up: true, pending: false, interfaces: [ { section: 'x', ifname: 'phy0-ap0' }, null ] }, radio2: { up: true, interfaces: 7 } });
let ov = k4.view(odd);
printf('odd: %J %J %J %J\n', ov.radio0.interfaces, ov.radio1, ov.radio2.held, wifinames.pending(odd));
printf('none: %J %J %J\n', wifinames.carry(null, null, 0).status, wifinames.pending(null), wifinames.keeper(there).view('x'));
// A name the kernel could not have is not carried, whatever the kernel says.
let bad = wifinames.carry({ r: { pending: true, interfaces: [ { section: 's' } ] } }, { r: { up: true, ifaces: { s: { ifname: '../x y' } } } }, 0, () => true);
printf('bad name: %J\n', bad.status.r.interfaces);

// A look made once asks again while a radio is pending, and gives the
// first answer with none; for a while at most, after which it gives the
// last answer, a pending radio and all.
let answers = [ mid, mid, after ], asked = 0;
let got = wifinames.settle(() => answers[min(asked++, 2)], 1, 1);
printf('settle: asked %d times, pending %J\n', asked, wifinames.pending(got));
asked = 0;
got = wifinames.settle(() => { asked++; return mid; }, 0.005, 1);
printf('settle never: asked %d times, pending %J\n', asked, wifinames.pending(got));
asked = 0;
got = wifinames.settle(() => { asked++; return null; }, 1, 1);
printf('settle null: asked %d time, %J\n', asked, got);
