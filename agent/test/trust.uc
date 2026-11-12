// What airscan is to trust (0304): TestAgentTrust runs this with the real
// ucode and compares what it prints with trust.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/trust.uc

'use strict';

import * as trust from 'aeolus.trust';

// The manager's doc: what is not well said is left out, BSSIDs in lower
// case, each once and sorted.
let doc = trust.clean({
	ssids: [ 'Home Wi-Fi', 'Home_Wi-Fi_IoT', 'Home Wi-Fi', '', 'a name far longer than thirty-two bytes', 'two\nlines', 7 ],
	bssids: [ 'E4:D1:24:BE:3A:40', 'e6:d1:24:be:3a:40', 'e4:d1:24:be:3a:40', 'not-a-bssid', null ],
});
printf('clean %J\n', doc);
printf('clean, not a doc %J %J\n', trust.clean(null), trust.clean({ ssids: 'Home Wi-Fi', bssids: [] }));

// uci's option as a list.
printf('listOf %J %J %J\n', trust.listOf([ 'a', 'b' ]), trust.listOf('a'), trust.listOf(null));

// First time: a person's own entry stays; the manager's are added, and only
// those the person had not are the agent's.
let m = trust.merge([ '00:11:22', 'e4:d1:24:be:3a:40' ], [], doc.trusted_bssid);
printf('first %J\n', m);
// The manager drops one: the agent takes away what it added, and the
// person's entry stays, though the manager had named it too.
m = trust.merge(m.next, m.mine, [ 'e4:d1:24:be:3a:40' ]);
printf('dropped %J\n', m);
// The manager names none: only the person's are left.
printf('none %J\n', trust.merge(m.next, m.mine, []));

// The same lists in another order are the same; another value is not.
printf('same %J %J %J\n', trust.same([ 'b', 'a' ], [ 'a', 'b' ]), trust.same([ 'a' ], [ 'a', 'b' ]), trust.same(null, []));
