// What Aeolus's daemons keep a client off (Keep on 6 GHz, 0300), so that
// aeolus-journey does not trace the disassociation hostapd sends at each
// renewal as the client leaving: TestAgentSentoff runs this with the real
// ucode and compares what it prints with sentoff.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/sentoff.uc

'use strict';

import * as sentoff from 'aeolus.sentoff';

const T = 1791770000, A = '02:00:00:00:00:0a', B = '02:00:00:00:00:0b';
const AP5 = 'phy0-ap2', AP2 = 'phy1-ap2', AP6 = 'phy2-ap2';

function at(list, mac, bss, from, to) {
	let out = [];
	for (let t = from; t <= to; t++)
		push(out, sentoff.kept(list, mac, bss, t) ? 'kept' : '-');
	return join(' ', out);
}

print('== A client on 6 GHz kept off 5 and 2.4 GHz here for 15 s\n');
let list = sentoff.kept_list(null, A, AP5, 15, T);
list = sentoff.kept_list(list, A, AP2, 15.0, T);
printf('  %J\n', list);
printf('  5 GHz, from 6 s before to 21 s after:\n    %s\n', at(list, A, AP5, T - 6, T + 21));
printf('  half a second in, as journey reads a frame by its own clock: %J\n', sentoff.kept(list, A, AP5, T + 0.5));
printf('  not on its 6 GHz, where it is: %J; nor of another client: %J\n', sentoff.kept(list, A, AP6, T), sentoff.kept(list, B, AP5, T));

print('\n== Renewed 8 s on, before it ran out: it keeps when it began\n');
list = sentoff.kept_list(list, A, AP5, 15, T + 8);
printf('  %J\n', list[A + ' ' + AP5]);
printf('  a frame of the first asking, read after the renewal: %J\n', sentoff.kept(list, A, AP5, T));

print('\n== Lifted at 16 s: the frame of the lifting is still covered, then nothing\n');
list = sentoff.kept_list(list, A, AP5, 0, T + 16);
printf('  %J\n', list[A + ' ' + AP5]);
printf('  from 15 s to 23 s:\n    %s\n', at(list, A, AP5, T + 15, T + 23));

print('\n== Kept again a minute on: what ran out is left out, and the keeping begins anew\n');
list = sentoff.kept_list(list, A, AP5, 15, T + 60);
printf('  %J\n', list);

print('\n== Never more than the latest 256\n');
let many = null;
for (let i = 0; i < 300; i++)
	many = sentoff.kept_list(many, sprintf('02:00:00:00:%02x:%02x', i / 256, i % 256), AP5, 3600, T + i);
let ks = sort(keys(many));
printf('  %d kept, from %s to %s\n', length(ks), ks[0], ks[length(ks) - 1]);

print('\n== What is not a list says nothing; a file that is not one starts anew\n');
printf('  %J %J %J\n', sentoff.kept(null, A, AP5, T), sentoff.kept({ [A + ' ' + AP5]: { since: 'then', until: T + 15 } }, A, AP5, T),
	sentoff.kept_list('nonsense', A, AP5, 15, T));
