// Libraries of which a process holds two builds at once (0317):
// TestAgentMixedLibs runs this with the real ucode and compares what it
// prints with mixedlibs.out. The maps lines are as the C260's were on
// 2026-11-05, after the late restore.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/mixedlibs.uc

'use strict';

import * as mixedlibs from 'aeolus.mixedlibs';

let line = (path) => sprintf('ffff8a000000-ffff8a010000 r-xp 00000000 1f:04 1234        %s\n', path);
let agent = join('', [
	line('/usr/bin/ucode'),
	line('/lib/libubox.so.20260721'),
	line('/lib/libubox.so.20260721'),
	line('/usr/lib/libuclient.so'),
	line('/usr/lib/ucode/uclient.so'),
	line('/lib/libubox.so.20261103'),
	line('/usr/lib/libucode.so.20261009'),
	line('/lib/ld-musl-aarch64.so.1'),
	'ffff8b000000-ffff8b001000 rw-p 00000000 00:00 0 \n',
]);
let usteerd = line('/lib/libubox.so.20261103') + line('/lib/libubus.so.20261028') + line('/lib/libubox.so.20260721') +
	line('/lib/libblobmsg_json.so.20261103') + line('/lib/libblobmsg_json.so.20260721');
let fine = line('/lib/libubox.so.20260721') + line('/lib/libjson-c.so.5') + line('/lib/libgcc_s.so.1');
// ustpd after its package was replaced under it: the old build's file is gone.
let ustpd = line('/lib/libubox.so.20261103 (deleted)') + line('/lib/libubox.so.20260721');

printf('builds in the agent: %J\n', mixedlibs.of_maps(agent));
printf('two: agent %J, fine %J, nothing %J\n', mixedlibs.two(agent), mixedlibs.two(fine), mixedlibs.two(null));
for (let m in mixedlibs.mixed([
	{ name: 'aeolus-agent', maps: agent },
	{ name: 'aeolus-agent', maps: agent },
	{ name: 'usteerd', maps: usteerd },
	{ name: 'hostapd', maps: fine },
	{ name: 'ustpd', maps: ustpd },
]))
	printf('%J\n', m);
printf('none: %J\n', mixedlibs.mixed([ { name: 'hostapd', maps: fine } ]));
for (let c in [ '/usr/bin/ucode\0-S\0/usr/sbin/aeolus-agent\0keys\0', '/sbin/ustpd\0', '/usr/sbin/hostapd\0-s\0-g\0/var/run/hostapd/global\0', '', null ])
	printf('name of %J: %J\n', c, mixedlibs.name_of(c));
