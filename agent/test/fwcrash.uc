// A radio's firmware that crashed (0310): TestAgentFWCrash runs this with
// the real ucode and compares what it prints with fwcrash.out. The lines
// are as porting's B17 found them on the C-360s (2026-11-05).
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/fwcrash.uc

'use strict';

import * as fwcrash from 'aeolus.fwcrash';

let k = (up, text) => sprintf('Thu Nov  5 01:00:00 2026 kern.err kernel: [%12.6f] %s', up, text);
let device_of = { phy12: '0001:01:00.0', phy4: '0001:01:00.0', phy0: 'c000000.wifi', phy1: 'c000000.wifi', phy2: '0000:01:00.0' };
let radio_of = { '0001:01:00.0': { radios: [ 'radio2' ], bands: [], scan: true }, 'c000000.wifi': { radios: [ 'radio0', 'radio1' ], bands: [ '5g', '2g' ] },
	'0000:01:00.0': { radios: [ 'radio3' ], bands: [ '6g' ] } };
let show = (what, lines, up, dev) => {
	let p = fwcrash.parse(lines);
	printf('%s:\n', what);
	for (let c in fwcrash.said(p, up, dev ?? device_of, radio_of))
		printf('  %J\n', c);
};

// C360-AP, a debugfs crash of the scan radio: the driver's line, the
// restart, and the radio lost inside a warning with no device of its own.
let c360 = [
	k(59740.1, 'ath11k_pci 0001:01:00.0: simulating firmware assert crash'),
	k(59740.2, 'ath11k_pci 0001:01:00.0: firmware crashed: MHI_CB_EE_RDDM'),
	k(59740.9, 'ieee80211 phy12: Hardware restart was requested'),
	k(59760.442171, 'Hardware became unavailable during restart.'),
	'Thu Nov  5 01:00:00 2026 kern.warn kernel: [59760.450000] Call trace:',
];
show('C360-AP, lost', c360, 60000.0);
// The same, and the radio came back: no line of it lost.
show('C360-AP, back', slice(c360, 0, 3), 60000.0);
// The Garage on the stock driver: no crash line, failed commands every 3 s
// and the restart; still failing now, then quiet for 5 minutes.
let garage = [ k(4500.0, 'ieee80211 phy4: Hardware restart was requested') ];
for (let i = 0; i < 8; i++)
	push(garage, k(4501.0 + 3 * i, 'ath11k_pci 0001:01:00.0: failed to send WMI_PDEV_SET_PARAM cmd'));
show('Garage, failing now', garage, 4530.0);
show('Garage, quiet 5 min', garage, 4830.0);
// Failed commands alone, no restart: a run of 5 is a firmware not
// answering; 3 is not.
let many = [], few = [];
for (let i = 0; i < 6; i++) push(many, k(100.0 + 3 * i, 'ath11k_pci 0000:01:00.0: failed to send WMI_VDEV_UP cmd'));
for (let i = 0; i < 3; i++) push(few, k(100.0 + 3 * i, 'ath11k_pci 0000:01:00.0: failed to send WMI_VDEV_UP cmd'));
show('six failed', many, 150.0);
show('three failed', few, 150.0);
// An IPQ8074 radio (AHB) that came back, and ath10k's line; a line not the
// kernel's is no crash.
show('AHB and ath10k', [
	k(4070.5, 'ath11k c000000.wifi: firmware crashed: MHI_CB_EE_RDDM'),
	k(4370.25, 'ath10k_pci 0000:01:00.0: firmware crashed! (guid 1234, uuid abcd)'),
	'Thu Nov  5 00:56:00 2026 user.notice airscan: firmware crashed is not said by the kernel here',
], 4400.0);
// The phy of a lost radio gone since: the crash before it is the lost one's.
show('phy gone', c360, 60000.0, {});

// Kept past the log's ring (merge): what was said before, as the agent
// keeps it in its file, with what the log holds now.
let keep = (kept, lines, up) => json(sprintf('%J', fwcrash.merge(kept, fwcrash.parse(lines), up)));
let showp = (what, p, up) => {
	printf('%s:\n', what);
	for (let c in fwcrash.said(p, up, device_of, radio_of))
		printf('  %J\n', c);
};
// The C360-AP's crash, then its lines rolled out of the ring: still said,
// lost, 6259 s on.
let k1 = keep(null, c360, 60000.0);
showp('rolled out', keep(k1, [ 'Thu Nov  5 03:00:00 2026 kern.warn kernel: [65000.000000] Call trace:' ], 66000.0), 66000.0);
// Read again with its lines still there: one crash, not two.
showp('read twice', keep(k1, c360, 60060.0), 60060.0);
// Six failed commands, the first four read at one poll, the last three,
// one of them read before, at the next: one run of six.
let k2 = keep(null, slice(many, 0, 4), 110.0);
showp('a run read in two', keep(k2, slice(many, 3), 120.0), 120.0);
printf('  kept: %J\n', keep(k2, slice(many, 3), 120.0).wmi);
// The AP started again (a kernel's clock before that one): nothing kept.
showp('started again', keep(k1, [], 100.0), 100.0);

// The W-318's own radios (AHB, IPQ6018) on 2026-11-05: the remote
// processor's crash, 128 failed commands while it waited 170 s for its dump
// to be read, then the processor up and each radio's hardware restart. One
// crash, of c000000.wifi: back once the commands stop; failing while they
// go on.
let w318_of = { phy0: 'c000000.wifi', phy1: 'c000000.wifi', phy2: '0000:01:00.0' };
let w318_radios = { 'c000000.wifi': { radios: [ 'radio0', 'radio1' ], bands: [ '5g', '2g' ] }, '0000:01:00.0': { radios: [ 'radio2' ], bands: [ '6g' ] } };
let w318 = [
	k(4154.552698, 'qcom-wcss-secure-pil cd00000.remoteproc: fatal error received:'),
	k(4154.599250, 'remoteproc remoteproc0: crash detected in wcnss: type fatal error'),
	k(4154.599261, 'remoteproc remoteproc0: handling crash #1 in wcnss'),
];
for (let i = 0; i < 128; i++)
	push(w318, k(4155.400927 + i * (4321.489120 - 4155.400927) / 127, 'ath11k c000000.wifi: failed to send WMI_REQUEST_STATS cmd'));
push(w318, k(4324.511793, 'remoteproc remoteproc0: remote processor wcnss is now up'),
	k(4324.826883, 'ieee80211 phy0: Hardware restart was requested'),
	k(4324.856913, 'ieee80211 phy1: Hardware restart was requested'));
let showw = (what, lines, up, dev) => {
	printf('%s:\n', what);
	for (let c in fwcrash.said(fwcrash.parse(lines), up, dev ?? w318_of, w318_radios))
		printf('  %J\n', c);
};
showw('W-318 AHB, back', w318, 4484.0);
showw('W-318 AHB, waiting', filter(w318, l => +match(l, /\[ *([0-9.]+)\]/)[1] < 4200), 4200.0);
// Its crash line alone: the device is the AP's one not on PCI.
showw('W-318 AHB, the line alone', slice(w318, 1, 2), 4160.0);
// Just after the processor is up and the radios restarted, 8 s after the
// last failed command: back, the restart ended the run.
showw('W-318 AHB, just restarted', w318, 4330.0);
