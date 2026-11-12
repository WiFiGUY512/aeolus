// The scan radio's beacons (0161), as aeolus-rrm reads them: the radiotap
// header's Flags, and frames that failed their FCS check passed over. Run
// by TestAgentBeacons, which holds what it prints to beacons.out.

'use strict';

import * as rrm from 'aeolus.rrm';

function rep(c, n) {
	let o = '';
	for (let i = 0; i < n; i++)
		o += c;
	return o;
}

function show(what, f) {
	printf('%s: flags %J, advert %J\n', what, rrm.radiotap_flags(f), rrm.beacon_advert(f));
}

// Three beacons C360-AP's scan radio handed up on as-mon0, 2026-10-16, as
// captured: a radiotap header of 26 bytes, its one present word 0x0000482f
// (TSFT, Flags, Rate, Channel, signal, antenna, RX flags), so Flags is the
// 17th byte, after the eight of TSFT.
// ap-2005b6df64e0's beacon on channel 1, its FCS good.
let good = rrm.unhex('00001a002f480000000000000000000000606c09c000a100000080000000ffffffffffff2005b6df64e22005b6df64e280485e302d43ca03000064003110000961656f6c75735f353001088c129824b048606c0301010504000200000706555320010b1e2a010030140100000fac040100000fac040100000fac020c000b0500002c0000460573000000003b0251002d1aed0913ffff0000000000000000000001000000000000000000003d16010017000000000000000000000000000000000000007f0a04000802010001400040c92b200d5101ff2005b6df64e507c8ea44407fff2005b6df64e6caf31a2e407fff2005b6df64e73e5cd80d407fff1a230500081a441002204e926f09afc8000c00fafffaff191cc771ff0724f43f0035fcffff022703ff0e260008a9ff2fa9ff4575ff6575ffdd180050f2020101810003a4000027a4000042435e0062322f00dd1102ae0101012005b6df64e0c0a80145415aa6a9be6f9f36d258');
// ap-a0046082275e's beacon on channel 11, its FCS failed (Flags 0x40),
// though what it says reads right.
let failed = rrm.unhex('00001a002f480000000000000000000040609e09c000a100000080000000ffffffffffffa00460822761a004608227613016014027d51800000064003110000961656f6c75735f353001088c129824b048606c03010b0504010200000706555320010b1e2a010030140100000fac040100000fac040100000fac020c000b0500001d0000460573000000003b0251002d1aed191bffffffff000000000000000001000000000000000000003d160b0017000000000000000000000000000000000000007f080400080201000140c92b200d510bffa2046082276107c8ea44407fffa60460822761caf31a2e407fffaa04608227613e5cd80d407fdd180050f2020101810003a4000027a4000002865f0062322f00dd1102ae010101a0046082275ec0a80126415a5feb45df00000004');
// Another of its beacons, its FCS failed, spoilt in its header: its BSSID
// reads a0:f7:db:1d:2a:60.
let spoilt = rrm.unhex('00001a002f480000000000000000000040609e09c000a100000080000000ffffffffffff99acea253661a0f7db1d2a6000225b0a903d45cca0c1901bd75786834bc37decffdf02c999d3088c129824b048606c03010b0504000200000706555320010b1e2a010030140100000fac040100000fac040100000fac020c000b050000170000460573000000003b0251002d1aed191bffffffff000000000000000001000000000000000000003d160b0017000000000000000000000000000000000000007f080400080201000140c92b200d510bffa2046082276107c8ea44407fffa60460822761caf31a2e407fffaa04608227613e5cd80d407fdd180050f2020101810003a4000027a4000042435e0062322f00dd1102ae010101a0046082275ec0a80126415a5a4d421f00000052');
show('good', good);
show('failed', failed);
show('spoilt', spoilt);
// The good one with its Flags set by hand: 0x40 is passed over; 0x10 (the
// frame carries its FCS) and 0x02 (short preamble) are not.
let flagged = (f, v) => substr(f, 0, 16) + chr(v) + substr(f, 17);
show('good, flags 0x40', flagged(good, 0x40));
show('good, flags 0x50', flagged(good, 0x50));
show('good, flags 0x10', flagged(good, 0x10));
show('good, flags 0x02', flagged(good, 0x02));
// And the failed one with its flag taken off: what it says is taken.
show('failed, flags 0x00', flagged(failed, 0x00));

// Where Flags is, by hand-built headers before the good one's frame.
let frame = substr(good, 26);
let rt = (present, fields) => {
	let h = '';
	for (let p in present)
		h += chr(p & 0xff, (p >> 8) & 0xff, (p >> 16) & 0xff, (p >> 24) & 0xff);
	let len = 4 + length(h) + length(fields);
	return chr(0, 0, len & 0xff, len >> 8) + h + fields;
};
// Flags alone: the eighth byte, no TSFT before it.
show('flags alone', rt([0x00000002], chr(0x40)) + frame);
// TSFT and Flags after two present words: TSFT aligned to eight, at 16.
show('two present words', rt([0x80000003, 0x00000000], rep('\x00', 4) + rep('\x11', 8) + chr(0x40)) + frame);
show('two present words, good', rt([0x80000003, 0x00000000], rep('\x00', 4) + rep('\x11', 8) + chr(0x00)) + frame);
// Three: TSFT at 16 with nothing to pad.
show('three present words', rt([0xa0000003, 0x80000000, 0x00000004], rep('\x22', 8) + chr(0x40)) + frame);
// No Flags in it: none to read, so the frame is read.
show('no flags', rt([0x00000001], rep('\x33', 8)) + frame);
// A header of zeros, as rrm.uc's own test makes.
show('zeros', rep('\x00', 2) + chr(18, 0) + rep('\x00', 14) + frame);
// Cut short: a header longer than the frame, present words that do not end
// within the header, and Flags said but not there.
show('header longer than the frame', substr(good, 0, 20));
show('present words without end', chr(0, 0, 12, 0) + rep('\xff', 8));
show('flags past the header', chr(0, 0, 16, 0) + chr(3, 0, 0, 0) + rep('\x00', 8) + frame);
printf('nothing: flags %J, advert %J\n', rrm.radiotap_flags(null), rrm.beacon_advert(null));
