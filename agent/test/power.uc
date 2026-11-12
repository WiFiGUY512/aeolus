// The transmit power Aeolus sets a radio to, held with iw (0349): what
// aeolus-power reads of the agent's package and of what it held, and what
// it decides to do for each radio. Run by TestAgentPower, which holds what
// it prints to power.out.

'use strict';

import * as power from 'aeolus.power';

function show(what, v) {
	printf('%s: %J\n', what, v);
}

// The section as uci's get_all() gives it: whole dBm from 1 to 30, by a
// radio's section's name, and nothing else.
show('wanted', power.wanted({
	'.type': 'power', '.name': 'aeolus_power', '.anonymous': false, '.index': 9,
	radio0: '20', radio1: '0', radio2: '31', radio3: '17.5', radio4: 'x', 'radio-5': '10', radio6: '1', radio7: '30',
}));
show('wanted, no section', power.wanted(null));

show('held', power.held('20 phy0 12\n'));
show('held, not its own', power.held('20 wlan0 12'));
show('held, nothing', power.held(null));
show('said', power.said(17, 'phy1', '41'));

let one = [{ radio: 'radio0', phy: 'phy0', index: '12' }];
let as = (p, phy, index) => ({ power: p, phy: phy ?? 'phy0', index: index ?? '12' });

show('a power set, none held', power.plan(one, {}, { radio0: 20 }, true));
show('held already', power.plan(one, { radio0: as(20) }, { radio0: 20 }, true));
show('set to another', power.plan(one, { radio0: as(20) }, { radio0: 17 }, true));
show('set up again by netifd', power.plan(one, { radio0: as(20, 'phy0', '11') }, { radio0: 20 }, true));
show('on another wiphy now', power.plan(one, { radio0: as(20, 'phy3') }, { radio0: 20 }, true));
show('none set now', power.plan(one, { radio0: as(20) }, {}, true));
show('none set now, set up again', power.plan(one, { radio0: as(20, 'phy0', '11') }, {}, true));
show('none set, none held', power.plan(one, {}, {}, true));

// netifd setting a radio up again: its interfaces are not there yet.
let pending = [{ radio: 'radio0', phy: null, index: null }];
show('interfaces not there', power.plan(pending, { radio0: as(20) }, { radio0: 17 }, false));

// Two radios on one wiphy: power is the wiphy's, so neither is held.
let shared = [{ radio: 'radio0', phy: 'phy0', index: '12' }, { radio: 'radio1', phy: 'phy0', index: '13' }];
show('a shared wiphy', power.plan(shared, {}, { radio0: 20 }, true));
show('a shared wiphy, one held', power.plan(shared, { radio0: as(20) }, { radio0: 20 }, true));

// A C-360: three radios, each its own; the scan radio has none set.
let c360 = [
	{ radio: 'radio0', phy: 'phy0', index: '20' },
	{ radio: 'radio1', phy: 'phy1', index: '24' },
	{ radio: 'radio3', phy: 'phy3', index: '28' },
];
show('a C-360', power.plan(c360, { radio0: as(17, 'phy0', '20') }, { radio0: 17, radio1: 15 }, true));

// A radio no longer listed: its hold is forgotten once netifd is settled.
show('a radio gone', power.plan(one, { radio0: as(20), radio9: as(15, 'phy9', '3') }, { radio0: 20 }, true));
show('a radio gone, not settled', power.plan(one, { radio0: as(20), radio9: as(15, 'phy9', '3') }, { radio0: 20 }, false));

for (let s in [
	{ radio: 'radio0', do: 'hold', power: 15, phy: 'phy1' },
	{ radio: 'radio0', do: 'let_go', phy: 'phy1' },
	{ radio: 'radio0', do: 'forget', phy: 'phy1' },
	{ radio: 'radio0', do: 'shared', power: 15, phy: 'phy1' },
])
	show('command ' + s.do, power.command(s));

// What the radio says it sends at (v0.321.0): louder than it is held at, as
// netifd sets a radio whose settings it reloads to its own power, keeping
// its interfaces, it is held again; quieter, its rules' or its radio's
// most, it is not; and not said, it is not.
let at = (p) => [{ radio: 'radio0', phy: 'phy0', index: '12', power: p }];
show('held, sends louder', power.plan(at(28), { radio0: as(15) }, { radio0: 15 }, true));
show('held, sends at it', power.plan(at(15), { radio0: as(15) }, { radio0: 15 }, true));
show('held, sends at it to a tenth', power.plan(at(15.4), { radio0: as(15) }, { radio0: 15 }, true));
show('held, sends quieter', power.plan(at(20), { radio0: as(30) }, { radio0: 30 }, true));
show('held, says nothing', power.plan(at(null), { radio0: as(15) }, { radio0: 15 }, true));
show('none set, sends loud', power.plan(at(28), {}, {}, true));
