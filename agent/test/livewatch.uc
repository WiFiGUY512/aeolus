// The live daemon's minutes, watched (0307): TestAgentLiveWatch runs this
// with the real ucode and compares what it prints with livewatch.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/livewatch.uc

'use strict';

import * as livewatch from 'aeolus.livewatch';

const T = 1793858900;
let radios = { at: T - 60, radios: { '2g': { radio: 'radio1' }, '5g': { radio: 'radio0' } } };
let none = { at: T - 60, radios: {} };
let say = (what, j) => printf('%s: %J\n', what, j);

// Minutes coming, with radios: nothing.
say('well', livewatch.judge(T, T - 30, radios, true, null, null));
// It never wrote: an AP whose live daemon takes no minutes is left alone.
say('never wrote', livewatch.judge(T, null, null, true, null, null));
// Not written for five minutes and a second: started again.
say('stale', livewatch.judge(T, T - 301, radios, true, null, null));
// Written, with no radio while hostapd serves: noted, then started again
// once that has lasted past five minutes.
let j = livewatch.judge(T, T - 30, none, true, null, null);
say('no radio, first', j);
say('no radio, 4 min', livewatch.judge(T + 240, T + 210, none, true, j.empty, null));
say('no radio, 6 min', livewatch.judge(T + 360, T + 330, none, true, j.empty, null));
// No radio while hostapd serves nothing (Wi-Fi off): nothing.
say('no radio, not serving', livewatch.judge(T + 360, T + 330, none, false, j.empty, null));
// Started again 10 minutes ago: not again before half an hour.
say('stale, lately started', livewatch.judge(T, T - 400, radios, true, null, T - 600));
say('stale, started long ago', livewatch.judge(T, T - 400, radios, true, null, T - 1900));
