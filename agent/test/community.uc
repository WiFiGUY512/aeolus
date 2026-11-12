// What the APs tell one another of what their clients said they can use,
// where community sharing is on (0299): TestAgentCommunity runs this with
// the real ucode and compares what it prints with community.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/community.uc

'use strict';

import * as community from 'aeolus.community';
import * as rrm from 'aeolus.rrm';

const WALL = 1791770000, AP = 'ap-30862dbca5ff';
const A = '6c:ac:c2:cf:b6:3f', B = '02:00:00:00:00:0b';

print('== What this AP has to say, from aeolus-steer\'s file\n');
let file = { at: WALL - 4, clients: [[A, ['2g', '5g', '6g'], 0], [B, ['2g', '5g'], 50],
	['nonsense', ['5g'], 0], [B, ['7g'], 0], [B, [], 0], [B, ['5g', '5g'], 0], [B, '5g', 0], [B, ['5g'], 61], [B, ['5g'], -1]] };
printf('  four seconds after it was written, made older, the old and the bad left out: %J\n', community.mine(file, WALL));
printf('  a file twenty seconds old says nothing: %J; nor none: %J; nor one from the future: %J\n',
	community.mine({ at: WALL - 20, clients: file.clients }, WALL), community.mine(null, WALL), community.mine({ at: WALL + 60, clients: file.clients }, WALL));

print('\n== Its message, fitted to one frame\n');
let list = community.mine(file, WALL);
let m = community.message(AP, WALL, 7, list, rrm.HELLO_MAX);
printf('  %J\n', m);
printf('  nothing to say, as where community sharing is off, no message: %J\n', community.message(AP, WALL, 8, [], rrm.HELLO_MAX));
let many = [];
for (let i = 0; i < 80; i++)
	push(many, [sprintf('02:00:00:00:%02x:%02x', i / 256, i % 256), ['2g', '5g', '6g'], i % 60]);
let big = community.message(AP, WALL, 9, many, rrm.HELLO_MAX);
printf('  eighty: %d said, the latest first, %d bytes signed, at most %d\n', length(big.clients), 64 + length(sprintf('%J', big)), rrm.HELLO_MAX);

print('\n== Another AP\'s, checked\n');
printf('  %J\n', community.taken({ v: 1, t: 'community', ap: AP, at: WALL, seq: 3, clients: [[A, ['5g', '6g'], 2], [B, ['8g'], 1], 'junk'] }));
printf('  a hello is not one: %J; nor junk: %J\n', community.taken({ t: 'hello', clients: [[A, ['5g'], 0]] }), community.taken('junk'));

print('\n== What aeolus-steer reads, by this AP\'s clock\n');
let by = {
	'ap-30862d46348f': { at: WALL - 10, clients: [[A, ['2g', '5g', '6g'], 5], [B, ['2g', '5g'], 55]] },
	'ap-e4d124be3a7f': { at: WALL - 2, clients: [[A, ['5g', '6g'], 0]] },
	'ap-2005b6df64e0': { at: null, clients: [[A, ['6g'], 0]] },
};
printf('  %J\n', community.near(by, WALL));
