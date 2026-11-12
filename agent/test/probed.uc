// What the APs tell one another of the clients they hear by probe and do
// not serve (0289): TestAgentProbed runs this with the real ucode and
// compares what it prints with probed.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/probed.uc

'use strict';

import * as probed from 'aeolus.probed';
import * as rrm from 'aeolus.rrm';

const WALL = 1791770000, AP = 'ap-30862def2fff', KEY = 'kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk';
const A = '02:00:00:00:00:0a', B = '02:00:00:00:00:0b', M = '86:2d:ef:2f';

print('== What this AP has to say, from aeolus-steer\'s file\n');
let file = { at: WALL - 4, clients: [[A, '5g', M, -61, 0], [B, '2g', M, -70, 20], [A, '2g', M, -48, 28],
	['nonsense', '5g', M, -60, 1], [B, '7g', M, -60, 1], [B, '5g', 'nope', -60, 1], [B, '5g', M, 3, 1], [B, '5g', M, -60, 31]] };
printf('  four seconds after it was written, made older, the old and the bad left out: %J\n', probed.mine(file, WALL));
printf('  a file twenty seconds old says nothing: %J; nor none: %J; nor one from the future: %J\n',
	probed.mine({ at: WALL - 20, clients: file.clients }, WALL), probed.mine(null, WALL), probed.mine({ at: WALL + 60, clients: file.clients }, WALL));

print('\n== Its message, fitted to one frame\n');
let list = probed.mine(file, WALL);
let m = probed.message(AP, WALL, 7, list, rrm.HELLO_MAX);
printf('  %J\n', m);
printf('  nothing to say, no message: %J\n', probed.message(AP, WALL, 8, [], rrm.HELLO_MAX));
let many = [];
for (let i = 0; i < 80; i++)
	push(many, [sprintf('02:00:00:00:%02x:%02x', i / 256, i % 256), '5g', M, -50 - i % 40, i % 30]);
let big = probed.message(AP, WALL, 9, many, rrm.HELLO_MAX);
let pkt = rrm.seal(KEY, big);
printf('  eighty readings: %d fit, %d bytes signed, a frame being %d\n', length(big.clients), length(pkt), rrm.HELLO_MAX);
printf('  read back: %J, %d readings\n', rrm.open(KEY, pkt)?.t, length(probed.taken(rrm.open(KEY, pkt))));

print('\n== What another AP said, checked\n');
printf('  %J\n', probed.taken({ t: 'probed', clients: [[A, '5g', M, -61, 4], 'nonsense', [A, '5g', M, -61], [A, '5g', M, -61, 40]] }));
printf('  not such a message: %J %J\n', probed.taken({ t: 'hello', clients: [[A, '5g', M, -61, 4]] }), probed.taken(null));

print('\n== What aeolus-steer reads: each client\'s readings, by when this AP took them\n');
let by = {
	'ap-2005b6df64e0': { at: WALL - 10, clients: [[A, '5g', '05:b6:df:64', -64, 4], [B, '2g', '05:b6:df:64', -58, 18]] },
	'ap-30862d46348f': { at: WALL - 2, clients: [[A, '5g', '86:2d:46:34', -70, 1]] },
	'ap-a0046082275e': { at: null, clients: [[A, '5g', '04:60:82:27', -80, 1]] },
};
printf('%.J\n', probed.near(by, WALL));

print('\n== What a scan radio heard, marked s (0321): taken, carried and read as such; any other mark is no reading\n');
printf('  %J\n', probed.taken({ t: 'probed', clients: [[A, '5g', M, -62, 3, 's'], [A, '5g', M, -62, 3, 'x'], [A, '5g', M, -62, 3, 's', 1]] }));
printf('  mine, made older: %J\n', probed.mine({ at: WALL - 4, clients: [[A, '5g', M, -62, 3, 's'], [A, '2g', M, -70, 2]] }, WALL));
printf('%.J\n', probed.near({ 'ap-2005b6df64e0': { at: WALL - 2, clients: [[A, '5g', M, -62, 3, 's'], [A, '2g', M, -70, 2]] } }, WALL));
