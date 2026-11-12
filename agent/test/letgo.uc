// Active client management's pure parts (0226): TestAgentLetGo runs this
// with the real ucode and compares what it prints with letgo.out. hostapd's
// STA answers are as a C-260's gave them, 2026-10-28, cut short.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/letgo.uc

'use strict';

import * as letgo from 'aeolus.letgo';

// What is said: from the AP, when, in what order, and the MACs, at most
// HERE_MACS of them.
printf('here %J\n', letgo.here('ap-30862d46348f', 1793170000, 7, ['30:86:2d:ef:2f:f0']));
printf('here many %d\n', length(letgo.here('ap-30862d46348f', 1, 1, map([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18], i => sprintf('02:00:00:00:00:%02x', i))).macs));

// What is read of one: the MACs, each once, in lower case; nothing of
// another kind of message, or of one that is not well said.
printf('here macs %J\n', letgo.here_macs({ t: 'here', macs: ['30:86:2D:EF:2F:F0', '30:86:2d:ef:2f:f0', 'nope', 7, 'fe:8f:ee:d3:60:e3'] }));
printf('a hello %J, no list %J, not an object %J\n', letgo.here_macs({ t: 'hello', macs: ['30:86:2d:ef:2f:f0'] }), letgo.here_macs({ t: 'here', macs: 'all' }), letgo.here_macs('here'));
printf('at most %d\n', length(letgo.here_macs({ t: 'here', macs: map([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18], i => sprintf('02:00:00:00:00:%02x', i)) })));

// Whether to let go of a client hostapd holds: it is there and has been on
// a while; not where it came back just now, nor where hostapd does not
// have it.
let sta = (secs) => `30:86:2d:ef:2f:f0\nflags=[AUTH][ASSOC][AUTHORIZED][WMM][HT]\naid=2\ncapability=0x431\nlisten_interval=10\nsupported_rates=8c 12 98 24 b0 48 60 6c\ntimeout_next=NULLFUNC POLL\nrx_packets=1066\ntx_packets=64\nrx_bytes=110519\ntx_bytes=8473\ninactive_msec=40\nsignal=-35\nrx_rate_info=1300 vhtmcs 7 vhtnss 2 shortGI\ntx_rate_info=8667 vhtmcs 9 vhtnss 2 shortGI\nconnected_time=${secs}\n`;
printf('held a while %J, just back %J, at the limit %J\n', letgo.held(sta(330), '30:86:2d:ef:2f:f0'), letgo.held(sta(1), '30:86:2d:ef:2f:f0'), letgo.held(sta(letgo.HELD_FOR), '30:86:2D:EF:2F:F0'));
printf('not there %J, another client %J, no answer %J, no time said %J\n', letgo.held('FAIL\n', '30:86:2d:ef:2f:f0'), letgo.held(sta(330), 'fe:8f:ee:d3:60:e3'),
	letgo.held(null, '30:86:2d:ef:2f:f0'), letgo.held('30:86:2d:ef:2f:f0\nflags=[AUTH]\n', '30:86:2d:ef:2f:f0'));

// A client's coming is said once a second at most.
let said = {};
printf('due %J %J %J %J\n', letgo.due(said, 'a', 100), letgo.due(said, 'a', 100.5), letgo.due(said, 'b', 100.5), letgo.due(said, 'a', 101.2));
