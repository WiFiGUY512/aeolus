// The client test's pure parts (0121): TestAgentClientTest runs this with
// the real ucode and compares what it prints with clienttest.out. The
// events and the outputs read are as the C-360's wpa_supplicant, iw, ping
// and udhcpc gave them on 2026-10-11.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/clienttest.uc

'use strict';

import * as ct from 'aeolus.clienttest';

// What is asked: an AP's networks on a band, with a speed server; the
// strongest other AP's, with nothing more; and what is no test at all.
printf('wanted %J\n', ct.wanted({ ssid: 'Aeolus Lab', bssids: ['20:05:B6:DF:64:EC'], band: '5g', speed: 'http://192.168.20.106', dns_name: 'openwrt.org' }));
printf('wanted least %J\n', ct.wanted({ ssid: 'Aeolus Lab' }));
printf('wanted named server %J\n', ct.wanted({ ssid: 'x', speed: 'http://zephyrus.example.com:3000/' })?.speed);
// A remote speed server too (0134): its host, its port, and the paths that
// give bytes and take them; how many bytes a POST carries, where it says.
let far = { name: 'Cloudflare', host: 'speed.cloudflare.com', port: 80, down: '/__down?bytes=50000000', up: '/__up', body: 4194304 };
printf('wanted remote %J\n', ct.wanted({ ssid: 'x', speed: 'http://192.168.20.106', speed_remote: far })?.speed_remote);
printf('wanted remote alone, unnamed %J\n', ct.wanted({ ssid: 'x', speed_remote: { host: 'speed.example.net', port: 8080, down: '/downloading', up: '/upload' } })?.speed_remote);
// One whose host or paths are not plain is not used, and the test is one
// all the same: they go into the requests as they are, so nothing may
// follow what was looked at, as after a zero byte.
for (let bad in [{ ...far, host: 'bad host' }, { ...far, host: 'h\r\nX: y' }, { ...far, host: 'speed.example.net\u0000\r\nX: y' }, { ...far, port: 0 },
	{ ...far, port: '80' }, { ...far, down: 'no-slash' }, { ...far, down: '/a b' }, { ...far, up: '/up\r\nHost: other' },
	{ ...far, up: '/u\u0000\r\nX-Injected: 1\r\n\r\nGET /second HTTP/1.1' }, { ...far, up: null }, { ...far, body: 12 }, { ...far, body: 99999999 }, 'cloudflare']) {
	let w = ct.wanted({ ssid: 'x', speed_remote: bad });
	printf('wanted remote %J: a test %J, its remote server %J\n', bad, w != null, w?.speed_remote);
}
for (let bad in [null, {}, { ssid: '' }, { ssid: 'x', bssids: ['not one'] }, { ssid: 'x', band: '60g' }, { ssid: 'x', speed: 'https://secure.example' },
	{ ssid: 'x', speed: 'http://host:99999' }, { ssid: 'x', speed: 'http://host/path' }, { ssid: 'x', dns_name: 'bad name' },
	{ ssid: 'a name of more than thirty-two bytes' }])
	printf('wanted %J: %J\n', bad, ct.wanted(bad));

printf('freqs 2g %J\n', ct.band_freqs('2g'));
printf('freqs 5g %J\n', ct.band_freqs('5g'));
printf('freqs 6g %d, from %d to %d; any band %d\n', length(ct.band_freqs('6g')), ct.band_freqs('6g')[0], ct.band_freqs('6g')[58], length(ct.band_freqs(null)));
printf('where %J %J %J %J %J %J\n', [ct.band_of(2412), ct.channel_of(2412)], [ct.band_of(5660), ct.channel_of(5660)], [ct.band_of(6935), ct.channel_of(6935)],
	[ct.band_of(2484), ct.channel_of(2484)], [ct.band_of(5935), ct.channel_of(5935)], ct.band_of(900));

// What the scan radio heard: this AP's own networks, the office AP's on
// both bands, the shed's faintly, and another network altogether.
let air = [
	{ bssid: '32:86:2d:ef:2f:c0', ssid: 'Aeolus Lab', band: '5g', freq: 5220, channel: 44, signal: -1, own: true },
	{ bssid: '32:86:2d:ef:2f:d0', ssid: 'Aeolus Lab', band: '2g', freq: 2437, channel: 6, signal: 0, own: true },
	{ bssid: '20:05:b6:df:64:e5', ssid: 'Aeolus Lab', band: '2g', freq: 2412, channel: 1, signal: -28, own: false },
	{ bssid: '20:05:B6:DF:64:EC', ssid: 'Aeolus Lab', band: '5g', freq: 5660, channel: 132, signal: -44, own: false },
	{ bssid: 'aa:04:60:82:27:61', ssid: 'Aeolus Lab', band: '2g', freq: 2462, channel: 11, signal: -84, own: false },
	{ bssid: '5c:83:6c:d6:df:c0', ssid: 'Home Wi-Fi', band: '2g', freq: 2412, channel: 1, signal: -43, own: false },
	'junk',
];
let want = (p) => ct.wanted({ ssid: 'Aeolus Lab', ...p });
printf('target the office AP %J\n', ct.target(want({ bssids: ['20:05:b6:df:64:e5', '20:05:b6:df:64:ec'] }), air));
printf('target the office AP on 5 GHz %J\n', ct.target(want({ bssids: ['20:05:b6:df:64:e5', '20:05:b6:df:64:ec'], band: '5g' }), air));
printf('target this AP itself on 2.4 GHz %J\n', ct.target(want({ bssids: ['32:86:2d:ef:2f:c0', '32:86:2d:ef:2f:d0'], band: '2g' }), air));
// The office AP's 5 GHz moved to 149 since the scan radio heard it on
// 132 (0239): it is looked for on both.
printf('target the office AP, moved on 5 GHz %J
', ct.target(want({ bssids: ['20:05:b6:df:64:ec'], band: '5g', channels: [{ band: '5g', channel: 149 }, { band: '2g', channel: 1 }] }), air));
printf('target an AP not heard, where it says it is %J
', ct.target(want({ bssids: ['02:00:00:00:00:01'], band: '5g', channels: [{ band: '5g', channel: 157 }] }), air)?.freqs);
printf('freq_of %J; a channel not on its band %J; refused %J
', [ct.freq_of('2g', 14), ct.freq_of('5g', 149), ct.freq_of('6g', 2), ct.freq_of('6g', 37)],
	ct.freq_of('2g', 36), ct.wanted({ ssid: 'x', channels: [{ band: '5g', channel: 999 }] }));
let unheard = ct.target(want({ bssids: ['02:00:00:00:00:01'], band: '2g' }), air);
printf('target an AP not heard: it looks on %d channels, heard %J\n', length(unheard.freqs), unheard.heard);
printf('target the strongest other AP %J\n', ct.target(want({}), air));
printf('target the strongest other AP on 5 GHz %J\n', ct.target(want({ band: '5g' }), air));
printf('target none heard %J %J\n', ct.target(ct.wanted({ ssid: 'Nowhere' }), air), ct.target(want({}), null));

// What wpa_supplicant is told, by how the network is secured.
let tgt = ct.target(want({ bssids: ['20:05:b6:df:64:ec'] }), air);
let dir = '/var/run/aeolus/clienttest';
printf('supplicant psk2:\n%s', ct.supplicant(dir, 'Aeolus Lab', { encryption: 'psk2', key: 'fixture-passphrase-1' }, tgt).conf);
for (let e in ['sae', 'sae-mixed', 'owe', 'none', 'psk-mixed+ccmp'])
	printf('supplicant %s: %J\n', e, filter(split(ct.supplicant(dir, 'x', { encryption: e, key: 'a "quoted" passphrase' }, { bssids: [], freqs: [] }).conf, '\n\t'),
		l => match(l, /^(key_mgmt|ieee80211w|psk|sae_password)=/)));
printf('supplicant no target: %J\n', ct.supplicant(dir, 'x', { encryption: 'none' }, null).conf);
for (let c in [{ encryption: 'wpa2', key: 'x' }, { encryption: 'wpa3-mixed' }, { encryption: 'psk2', ppsk: '1' }, { encryption: 'psk2' },
	{ encryption: 'psk2', key: 'short' }, { encryption: 'psk2', key: 'two\nlines of it' }, { encryption: 'wep', key: 'fixture-passphrase-1' }, null])
	printf('supplicant %J: %J\n', c, ct.supplicant(dir, 'x', c, tgt));

// wpa_supplicant's events, as its control socket sends them.
for (let line in [
	'<3>CTRL-EVENT-SCAN-STARTED ',
	'<3>CTRL-EVENT-SCAN-RESULTS ',
	'<3>SME: Trying to authenticate with 20:05:b6:df:64:ec (SSID=\'Aeolus Lab\' freq=5660 MHz)',
	'<3>Trying to associate with 20:05:b6:df:64:ec (SSID=\'Aeolus Lab\' freq=5660 MHz)',
	'<3>Associated with 20:05:b6:df:64:ec',
	'<3>CTRL-EVENT-SUBNET-STATUS-UPDATE status=0',
	'<3>WPA: Key negotiation completed with 20:05:b6:df:64:ec [PTK=CCMP GTK=CCMP]',
	'<3>CTRL-EVENT-CONNECTED - Connection to 20:05:b6:df:64:ec completed [id=0 id_str=]',
	'<3>CTRL-EVENT-NETWORK-NOT-FOUND ',
	'<3>CTRL-EVENT-AUTH-REJECT 20:05:b6:df:64:e8 auth_type=0 auth_transaction=2 status_code=1',
	'<3>CTRL-EVENT-ASSOC-REJECT bssid=20:05:b6:df:64:e8 status_code=17',
	'<3>WPA: 4-Way Handshake failed - pre-shared key may be incorrect',
	'<3>CTRL-EVENT-SSID-TEMP-DISABLED id=0 ssid="Aeolus Lab" auth_failures=1 duration=10 reason=WRONG_KEY',
	'<3>CTRL-EVENT-SSID-TEMP-DISABLED id=0 ssid="Aeolus-PPSK" auth_failures=2 duration=20 reason=CONN_FAILED',
	'<3>CTRL-EVENT-DISCONNECTED bssid=20:05:b6:df:64:ec reason=3 locally_generated=1',
	'OK',
	null,
])
	printf('event %J\n   %J\n', line, ct.event(line));

// What iw said of the link: on 5 GHz, where the scan radio has no HT, as it
// joined and once bytes had moved; and on 2.4 GHz.
printf('link 5 GHz, just joined %J\n', ct.link_of('Station 20:05:b6:df:64:ec (on aet-sta0)\n\tinactive time:\t130 ms\n\trx bytes:\t1705\n\tsignal:  \t-45 dBm\n\tsignal avg:\t-45 dBm\n\trx bitrate:\t48.0 MBit/s\n\trx duration:\t7451 us\n'));
printf('link 2.4 GHz %J\n', ct.link_of('\tsignal:  \t-34 [-34] dBm\n\ttx bitrate:\t143.3 MBit/s HE-MCS 11 HE-NSS 1 HE-GI 0 HE-DCM 0\n\trx bitrate:\t143.3 MBit/s HE-MCS 11 HE-NSS 1 HE-GI 0 HE-DCM 0\n'));
printf('link of nothing %J\n', ct.link_of(''));

// busybox ping, answered and not.
printf('ping %J\n', ct.ping_of('PING 192.168.20.1 (192.168.20.1): 56 data bytes\n64 bytes from 192.168.20.1: seq=0 ttl=64 time=2.358 ms\n\n--- 192.168.20.1 ping statistics ---\n3 packets transmitted, 3 packets received, 0% packet loss\nround-trip min/avg/max = 2.358/4.131/7.610 ms\n'));
printf('ping unanswered %J\n', ct.ping_of('PING 192.168.20.1 (192.168.20.1): 56 data bytes\n\n--- 192.168.20.1 ping statistics ---\n3 packets transmitted, 0 packets received, 100% packet loss\n'));
printf('ping of nothing %J\n', ct.ping_of(null));

// The lease udhcpc's script wrote.
printf('lease %J\n', ct.lease_of('ip=192.168.20.108\nmask=24\nrouter=192.168.20.1\ndns=192.168.20.254 9.9.9.9\nserver=192.168.20.254\nlease=86400\n'));
printf('lease, little said %J\n', ct.lease_of('ip=10.0.0.7\nmask=8\nrouter=\ndns=\nserver=\nlease=\n'));
printf('lease, none %J %J %J\n', ct.lease_of(''), ct.lease_of('ip=not an address\nmask=24\n'), ct.lease_of('ip=192.168.20.108\nmask=40\n'));
// What udhcpc said as it asked (0189): given an address at last; offered
// one and never acknowledged; refused; and never answered.
let given = 'udhcpc: started, v1.38.0\nudhcpc: broadcasting discover\nudhcpc: broadcasting select for 192.168.10.186, server 192.168.10.254\nudhcpc: broadcasting select for 192.168.10.186, server 192.168.10.254\nudhcpc: lease of 192.168.10.186 obtained from 192.168.10.254, lease time 600\n';
let unconfirmed = 'udhcpc: started, v1.38.0\nudhcpc: broadcasting discover\nudhcpc: broadcasting select for 192.168.10.186, server 192.168.10.254\nudhcpc: broadcasting select for 192.168.10.186, server 192.168.10.254\nudhcpc: broadcasting select for 192.168.10.186, server 192.168.10.254\nudhcpc: broadcasting discover\nudhcpc: broadcasting discover\nudhcpc: broadcasting select for 192.168.10.187, server 192.168.10.254\n';
let refused = 'udhcpc: broadcasting discover\nudhcpc: broadcasting select for 192.168.20.99, server 192.168.20.254\nudhcpc: received DHCP NAK\nudhcpc: broadcasting discover\n';
let unanswered = 'udhcpc: started, v1.38.0\nudhcpc: broadcasting discover\nudhcpc: broadcasting discover\nudhcpc: broadcasting discover\n';
printf('dhcp said, given %J\n', ct.dhcp_said(given));
for (let pair in [['unconfirmed', unconfirmed], ['refused', refused], ['unanswered', unanswered], ['nothing', null]])
	printf('dhcp %s %J %J\n', pair[0], ct.dhcp_said(pair[1]), ct.dhcp_failure(ct.dhcp_said(pair[1]), 20));
printf('dhcp asked once %J\n', ct.dhcp_failure(ct.dhcp_said('udhcpc: broadcasting select for 10.0.0.9, server 10.0.0.1\n'), 20));
printf('network %J %J %J %J %J\n', ct.prefix_net('192.168.20.108', 24), ct.prefix_net('10.20.30.40', 12), ct.prefix_net('10.20.30.40', 32), ct.prefix_net('10.20.30.40', 0), ct.prefix_net('nothing', 24));

// A DNS question, and answers to it: an address behind an alias, the name
// not found, an answer to another question, and one cut short.
let q = ct.dns_query(0x4165, 'example.com');
printf('dns query %s\n', ct.hexstr(q));
let answer = substr(q, 0, 2) + chr(0x81, 0x80, 0, 1, 0, 2, 0, 0, 0, 0) + substr(q, 12) +
	chr(0xc0, 12, 0, 5, 0, 1, 0, 0, 1, 44, 0, 6, 3) + 'www' + chr(0xc0, 12) +
	chr(0xc0, 29, 0, 1, 0, 1, 0, 0, 1, 44, 0, 4, 104, 20, 23, 154);
printf('dns answer %J\n', ct.dns_answer(answer, 0x4165));
printf('dns no such name %J\n', ct.dns_answer(substr(q, 0, 2) + chr(0x81, 0x83, 0, 1, 0, 0, 0, 0, 0, 0) + substr(q, 12), 0x4165));
printf('dns another question %J, the question itself %J, cut short %J %J\n', ct.dns_answer(answer, 0x4166), ct.dns_answer(q, 0x4165),
	ct.dns_answer(substr(answer, 0, 40), 0x4165), ct.dns_answer(substr(answer, 0, 8), 0x4165));
printf('rate %J %J %J\n', ct.rate(13125000, 5), ct.rate(1200000000, 5.02), ct.rate(100, 0));

// When the scan radio may be taken: airscan between its jobs, and none
// about to start.
let airscan = (state, bss, spectral) => ({ state: state, next: { bss: bss, spectral: spectral, ble: 1000 } });
printf('between: looking at clients %J, idle %J, sweeping %J, scanning %J, no status %J\n',
	ct.between(airscan('clients', 1200, 1700), 1000), ct.between(airscan('idle', 1200, 1700), 1000),
	ct.between(airscan('spectral', 1200, 1700), 1000), ct.between(airscan('bss', 1200, 1700), 1000), ct.between(null, 1000));
printf('between: a scan due in 3 s %J, in 4 s %J, a sweep due now %J, 10 s late %J, long past %J, nothing said of the next %J\n',
	ct.between(airscan('clients', 1003, 1700), 1000), ct.between(airscan('clients', 1004, 1700), 1000),
	ct.between(airscan('clients', 1200, 1000), 1000), ct.between(airscan('clients', 1200, 990), 1000),
	ct.between(airscan('clients', 900, 1700), 1000), ct.between({ state: 'clients' }, 1000));

// The network's own section, by its ID: not another with the same SSID.
let ifaces = [
	{ '.name': 'wifinet7', mode: 'ap', ssid: 'Aeolus Lab', encryption: 'psk2', key: 'made by hand' },
	{ '.name': 'aeolus_lab_guest_radio0', mode: 'ap', ssid: 'Aeolus Lab', encryption: 'none' },
	{ '.name': 'aeolus_lab_radio1', mode: 'ap', ssid: 'Aeolus Lab', encryption: 'sae', key: 'the lab one' },
	{ '.name': 'aeolus_two_part_radio0', mode: 'ap', ssid: 'Two', encryption: 'psk2', key: 'two', ppsk: '1' },
	{ '.name': 'aeolus_lab_radio0', mode: 'sta', ssid: 'Aeolus Lab', encryption: 'psk2', key: 'a client of it' },
];
let radios = ['radio0', 'radio1', 'radio2'];
let asks = (network, ssid) => ct.wanted({ ssid: ssid ?? 'Aeolus Lab', network: network });
printf('credentials: by ID %J, another network with the SSID %J, an ID with a hyphen %J\n',
	ct.credentials(asks('lab'), ifaces, radios), ct.credentials(asks('lab-guest'), ifaces, radios), ct.credentials(asks('two-part', 'Two'), ifaces, radios));
printf('credentials: an ID the AP has no section for %J, its section under another SSID %J, no ID %J, an ID that is none %J\n',
	ct.credentials(asks('office'), ifaces, radios), ct.credentials(asks('lab', 'Renamed'), ifaces, radios),
	ct.credentials(asks(null), ifaces, radios), asks('Not An ID'));

// A client on a DFS channel, or looking at one.
printf('dfs: channel 132 %J, 36 and 149 %J, 2.4 GHz and 6 GHz %J, 48 with 52 %J, none %J\n',
	ct.dfs([5660]), ct.dfs([5180, 5745]), ct.dfs([2412, 5955, 6115]), ct.dfs([5240, 5260]), ct.dfs(null));

// A network that signs clients in by 802.1X (0124): the user the test was
// given, PEAP or TTLS, on WPA2-Enterprise, WPA3-Enterprise or both.
let user = ct.wanted({ ssid: 'Aeolus-Enterprise', eap: { method: 'peap', identity: 'aeolus-test', password: 'a "quoted" pass' } });
printf('wanted with a user %J\n', user.eap);
printf('wanted with a user: no password %J, a method there is none of %J, a name too long %J\n',
	ct.wanted({ ssid: 'x', eap: { method: 'peap', identity: 'u' } }), ct.wanted({ ssid: 'x', eap: { method: 'tls', identity: 'u', password: 'p' } }),
	ct.wanted({ ssid: 'x', eap: { method: 'peap', identity: join('', map([1, 2, 3, 4, 5, 6, 7, 8, 9], x => '0123456789abcdef')), password: 'p' } }));
for (let e in ['wpa2', 'wpa3', 'wpa3-mixed', 'wpa'])
	printf('enterprise %s: %J\n', e, ct.supplicant('/var/run/x', 'Aeolus-Enterprise', { encryption: e }, { bssids: ['20:05:b6:df:64:ed'], freqs: [5220] }, user.eap));
printf('enterprise by TTLS: %J\n', ct.supplicant('/var/run/x', 'Aeolus-Enterprise', { encryption: 'wpa2' }, null, { method: 'ttls', identity: 'u', password: 'p' }));
printf('enterprise with no user: %J\n', ct.supplicant('/var/run/x', 'Aeolus-Enterprise', { encryption: 'wpa3-mixed' }, null, null));
for (let l in ['<3>CTRL-EVENT-EAP-STARTED EAP authentication started', '<3>CTRL-EVENT-EAP-PROPOSED-METHOD vendor=0 method=25',
	'<3>CTRL-EVENT-EAP-METHOD EAP vendor 0 method 25 (PEAP) selected', '<3>CTRL-EVENT-EAP-PEER-CERT depth=0 subject=\'/CN=zephyrus\'',
	'<3>CTRL-EVENT-EAP-SUCCESS EAP authentication completed successfully', '<3>CTRL-EVENT-EAP-FAILURE EAP authentication failed'])
	printf('eap event %J\n', ct.event(l));

// What a test's findings come to, and how each reads in a line.
let found = () => ({
	ssid: 'Aeolus Lab', joined_ms: 636, dhcp: { ms: 284, address: '192.168.20.108' },
	gateway: { address: '192.168.20.1', sent: 3, got: 3, min: 2.358, avg: 4.131, max: 7.61 },
	dns: { server: '192.168.20.254', name: 'example.com', rcode: 0, answers: 2, address: '104.20.23.154', ms: 42 },
	speed: { server: 'http://192.168.20.106', down: 21, up: 22.1 },
});
let show = (name, res) => {
	ct.verdict(res);
	printf('%s: %s, stage %s, reason %J\n   %s\n', name, res.outcome, res.stage, res.reason, ct.summary(res));
};
show('all of it', found());
show('no speed server', { ...found(), speed: null });
show('joined, and nothing more asked', { joined_ms: 2561 });
show('never found', { stopped: { stage: 'find', why: 'it was not found there in 17 looks' } });
show('wrong passphrase', { stopped: { stage: 'key', why: 'wrong passphrase' } });
show('no address', { joined_ms: 700, stopped: { stage: 'dhcp', why: 'no DHCP server gave it an address' } });
show('a lease with no gateway', { ...found(), gateway: null });
show('a lease with no DNS server', { ...found(), dns: null });
printf('neigh: reachable %J, stale %J, failed %J, incomplete %J, nothing %J\n',
	ct.neigh_of('192.168.50.1 lladdr 28:e7:1d:d2:d0:13 ref 1 used 0/0/0 probes 4 REACHABLE\n'), ct.neigh_of('192.168.50.1 lladdr 28:e7:1d:d2:d0:13 STALE\n'),
	ct.neigh_of('192.168.50.1  FAILED\n'), ct.neigh_of('192.168.50.1  INCOMPLETE\n'), ct.neigh_of(''));
show('a gateway that answers ARP and no ping', { ...found(), gateway: { address: '192.168.50.1', sent: 3, got: 0, mac: '28:e7:1d:d2:d0:13' } });
show('gateway silent', { ...found(), gateway: { address: '192.168.20.1', sent: 3, got: 0 } });
show('DNS silent', { ...found(), dns: { server: '192.168.20.254', name: 'example.com' } });
show('DNS has no such name', { ...found(), dns: { server: '192.168.20.254', name: 'nowhere.invalid', rcode: 3, answers: 0, ms: 12 } });
// A speed server the network cannot find or reach fails no test (0244): it
// is said beside the result.
show('speed server unreachable', { ...found(), speed: { server: 'http://192.168.20.106', error: 'Connection refused' } });
// The remote server's speed is said beside the local one's; that it could
// not be measured fails no test (0134).
show('both speeds', { ...found(), speed_remote: { server: 'Cloudflare', down: 480, up: 39 } });
show('the remote speed alone', { ...found(), speed: null, speed_remote: { server: 'Cloudflare', down: 480, up: 39 } });
show('the remote server unreachable', { ...found(), speed_remote: { server: 'Cloudflare', error: 'its name was not found' } });
show('a reason that is not plain text', { stopped: { stage: 'find', why: 'caf\xc3\xa9 ' + join('', map([1, 2, 3, 4, 5, 6, 7, 8], x => 'a long reason that goes on and on and on, ')) } });
show('the sign-in refused', { stopped: { stage: 'signin', why: 'the sign-in was refused' } });
// The way out (0199): a full-size packet that gets out; one lost without
// DF where TCP is held below it, by a router that clamps its MSS; one lost
// where TCP sends more, which fails the test and says what to do; and one
// where TCP could not be measured, which fails nothing.
printf('way out, full size: %J\n', ct.way_out('one.one.one.one', 1500, 1500));
printf('way out, clamped: %J\n', ct.way_out('one.one.one.one', 1408, 1400));
printf('way out, lost: %J\n', ct.way_out('one.one.one.one', 1408, 1472));
printf('way out, TCP not measured: %J\n', ct.way_out('one.one.one.one', 1408, null));
show('full-size packets out', { ...found(), path: ct.way_out('one.one.one.one', 1500, 1500) });
show('packets out to 1408, TCP clamped', { ...found(), path: ct.way_out('one.one.one.one', 1408, 1400) });
show('packets over 1408 lost, TCP sends 1472', { ...found(), path: ct.way_out('one.one.one.one', 1408, 1472) });
// A test that roams once on (0225): the APs asked, each by its BSSIDs, and
// what is no list of them; where it goes on the band asked, the strongest
// BSSID of each AP heard with the SSID, an AP not heard left out; the key
// management a network with 802.11r is joined with; how each roam came by
// its keys; and its roams in the test's line.
printf('wanted roam %J\n', ct.wanted({ ssid: 'Aeolus Lab', band: '2g', roam: [['20:05:B6:DF:64:E5', '20:05:b6:df:64:ec'], ['aa:04:60:82:27:61']] })?.roam);
for (let bad in [['20:05:b6:df:64:e5'], [[]], [['nope']], [[1]], 'all', map([1, 2, 3, 4, 5, 6, 7, 8, 9], i => ['20:05:b6:df:64:e5'])])
	printf('wanted roam %J: %J\n', bad, ct.wanted({ ssid: 'x', roam: bad }));
printf('wanted roam over the wire: %J\n', ct.wanted({ ssid: 'x', band: 'wire', network: 'lab', ap: 'ap-2005b6df64e0', roam: [['20:05:b6:df:64:e5']] }));
printf('roam targets on 2.4 GHz %J\n', ct.roam_targets(want({ band: '2g', roam: [['20:05:b6:df:64:e5', '20:05:b6:df:64:ec'], ['02:00:00:00:00:01'], ['aa:04:60:82:27:61']] }), air));
printf('roam targets on 5 GHz %J\n', ct.roam_targets(want({ band: '5g', roam: [['20:05:b6:df:64:e5', '20:05:b6:df:64:ec']] }), air));
printf('roam targets none asked %J, none heard %J\n', ct.roam_targets(want({}), air), ct.roam_targets(want({ roam: [['20:05:b6:df:64:e5']] }), null));
for (let e in ['sae', 'sae-mixed', 'psk2', 'owe'])
	printf('supplicant %s with 802.11r: %J\n', e, filter(split(ct.supplicant(dir, 'x', { encryption: e, key: 'fixture-passphrase-1', ft: true }, { bssids: [], freqs: [] }).conf, '\n\t'),
		l => match(l, /^key_mgmt=/)));
for (let e in ['wpa2', 'wpa3', 'wpa3-mixed'])
	printf('supplicant %s with 802.11r: %J\n', e, filter(split(ct.supplicant(dir, 'x', { encryption: e, ft: true }, { bssids: [], freqs: [] }, { method: 'peap', identity: 'u', password: 'p' }).conf, '\n\t'),
		l => match(l, /^key_mgmt=/)));
printf('credentials with 802.11r %J\n', ct.credentials({ ssid: 'Aeolus Lab', network: 'lab' }, [{ '.name': 'aeolus_lab_radio0', mode: 'ap', ssid: 'Aeolus Lab', encryption: 'sae', key: 'k', ieee80211r: '1' }], ['radio0']));
printf('roam kinds %J\n', [ct.roam_kind(false, 'FT-SAE'), ct.roam_kind(false, 'FT-PSK'), ct.roam_kind(true, 'FT-SAE'), ct.roam_kind(false, 'SAE'), ct.roam_kind(false, 'WPA2-PSK'), ct.roam_kind(false, 'WPA-EAP'), ct.roam_kind(false, null)]);
show('roamed by 802.11r', { ...found(), roams: [{ bssid: '20:05:b6:df:64:e5', ok: true, ms: 14, kind: 'ft' }, { bssid: 'aa:04:60:82:27:61', ok: true, ms: 11, kind: 'ft' },
	{ bssid: '20:05:b6:df:64:ec', ok: true, ms: 152, kind: 'full' }] });
show('a roam not let on', { ...found(), roams: [{ bssid: '20:05:b6:df:64:e5', ok: true, ms: 14, kind: 'ft' }, { bssid: 'aa:04:60:82:27:61', ok: false, why: 'it was not let on there within 8 s' }] });
show('no roam let on', { ...found(), roams: [{ bssid: '20:05:b6:df:64:e5', ok: false, why: 'wpa_supplicant would not roam there: it has not heard that BSSID' }] });
