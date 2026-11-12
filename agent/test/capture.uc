// What aeolus-journey keeps of a client's failed attempt as a capture
// (0135, 0146): what is read of hostapd's netlink rings and what is let go;
// the frames made of it; how the radio's word on a frame the AP sent finds
// its frame; what a radio's ring keeps and gives of one client; and the
// file made of them. internal/rendercheck runs this with the real ucode and
// compares what it prints with capture.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/capture.uc

'use strict';

import * as capture from 'aeolus.capture';

function bytes(list) {
	return join('', map(list, b => chr(b)));
}
function zeros(n) {
	let s = '';
	for (let i = 0; i < n; i++)
		s += chr(0);
	return s;
}
function mac(t) {
	return bytes(map(split(t, ':'), x => hex(x)));
}
function hexdump(s) {
	return join('', map(split(s, ''), c => sprintf('%02x', ord(c))));
}

const AP = '32:86:2d:ef:2f:c0', STA = '30:86:2d:ef:2f:f0', OTHER = '72:bb:66:9f:76:94', ALL = 'ff:ff:ff:ff:ff:ff';

// An 802.11 header: frame control's two bytes, a duration, three addresses
// and a sequence number.
let dot11 = (fc0, fc1, a1, a2, a3) => bytes([fc0, fc1, 0, 0]) + mac(a1) + mac(a2) + mac(a3) + bytes([0, 0]);
// A key message: the nth of its handshake, by its replay counter.
let keymsg = (n) => bytes([2, 3, 0, 95, 2, 0, 0x8a, 0, 16, 0, 0, 0, 0, 0, 0, 0, n]) + zeros(82);

// An entry of hostapd's netlink rings, as udebug hands it: sixteen bytes
// that say which way it went, then the netlink message: its length, its
// family, flags, a sequence number and a port; the command; and what is
// said with it, each its length, its number and its value, made up to four
// bytes. Numbers are in the processor's own order: le says which.
function entry(asked, cmd, attrs, le) {
	let w16 = n => le ? bytes([n & 255, (n >> 8) & 255]) : bytes([(n >> 8) & 255, n & 255]);
	let w32 = n => le ? bytes([n & 255, (n >> 8) & 255, (n >> 16) & 255, (n >> 24) & 255]) : bytes([(n >> 24) & 255, (n >> 16) & 255, (n >> 8) & 255, n & 255]);
	let body = '';
	for (let a in attrs) {
		let v = a[1];
		if (type(v) == 'int')
			v = a[2] == 16 ? w16(v) : w32(v);
		body += w16(4 + length(v)) + w16(a[0]) + v + zeros((4 - length(v) % 4) % 4);
	}
	return bytes([0, asked ? 7 : 6, 0x03, 0x38]) + zeros(10) + bytes([0, 16]) +
		w32(20 + length(body)) + w16(28) + w16(asked ? 5 : 0) + w32(0) + w32(asked ? 4242 : 0) + bytes([cmd, 1, 0, 0]) + body;
}
const WIPHY = 1, IFINDEX = 3, MAC = 6, FREQ = 38, FRAME = 51, COOKIE = 88, ACK = 92, ETHERTYPE = 102, NO_ENCRYPT = 103, NO_WORD = 142, SIGNAL = 151, WDEV = 153;
const F = 59, F_STATUS = 60, PORT = 129, PORT_STATUS = 139;
// A frame the radio heard: as the kernel hands it up, -48 dBm on channel 11.
let heard = (frame, le) => entry(false, F, [[WIPHY, 1], [IFINDEX, 14], [WDEV, zeros(8)], [FREQ, 2462], [SIGNAL, 4294967248], [FRAME, frame]], le ?? true);
let asked = (frame, more) => entry(true, F, [[IFINDEX, 14], [FREQ, 2462], [FRAME, frame], ...(more ?? [])], true);
let status = (frame, acked) => entry(false, F_STATUS, [[WIPHY, 1], [IFINDEX, 14], [WDEV, zeros(8)], [FRAME, frame], [COOKIE, zeros(8)], ...(acked ? [[ACK, '']] : [])], true);
let key_heard = (body) => entry(false, PORT, [[WIPHY, 1], [IFINDEX, 14], [WDEV, zeros(8)], [FRAME, body], [MAC, mac(STA)], [ETHERTYPE, 0x888e, 16], [NO_ENCRYPT, '']], true);
let key_asked = (body) => entry(true, PORT, [[IFINDEX, 14], [FRAME, body], [MAC, mac(STA)], [ETHERTYPE, 0x888e, 16], [NO_ENCRYPT, '']], true);
let key_status = (body, acked) => entry(false, PORT_STATUS, [[WIPHY, 1], [IFINDEX, 14], [WDEV, zeros(8)], [FRAME, mac(STA) + mac(AP) + bytes([0x88, 0x8e]) + body], [COOKIE, zeros(8)], ...(acked ? [[ACK, '']] : [])], true);

let auth1 = dot11(0xb0, 0, AP, STA, AP) + bytes([0, 0, 1, 0, 0, 0]);
let auth2 = dot11(0xb0, 0, STA, AP, AP) + bytes([0, 0, 2, 0, 0, 0]);
let assoc_req = dot11(0x00, 0, AP, STA, AP) + zeros(60);
let assoc_resp = dot11(0x10, 0, STA, AP, AP) + bytes([0x31, 0x04, 0, 0, 1, 0xc0]) + zeros(40);
let probe_req = dot11(0x40, 0, ALL, STA, ALL) + zeros(40);
let probe_resp = dot11(0x50, 0, STA, AP, AP) + zeros(80);
let deauth = dot11(0xc0, 0, STA, AP, AP) + bytes([2, 0]);
let deauth_all = dot11(0xc0, 0, ALL, AP, AP) + bytes([3, 0]);
let beacon = dot11(0x80, 0, ALL, OTHER, OTHER) + zeros(200);

// What an entry is read as: a frame of a client's coming, or nothing.
function show(e) {
	if (!e)
		return 'nothing';
	return sprintf('%s%s, %d bytes beginning %s%s%s%s%s%s%s', e.what, e.key ? ', a key message' : '', length(e.frame), hexdump(substr(e.frame, 0, 4)),
		e.peer ? ', the client ' + e.peer : '', e.ifindex != null ? ', interface ' + e.ifindex : '', e.freq ? sprintf(', %d MHz', e.freq) : '',
		e.signal != null ? sprintf(', %d dBm', e.signal) : '', e.what == 'status' ? (e.acked ? ', acknowledged' : ', not acknowledged') : '',
		e.unasked ? ', no word asked for' : '');
}
let cases = [
	['an authentication the radio heard', heard(auth1)],
	['the same from a processor that counts the other way', heard(auth1, false)],
	['its answer, as hostapd asks for it', asked(auth2)],
	['the radio\'s word on the answer', status(auth2, true)],
	['its word on one the client did not acknowledge', status(deauth, false)],
	['an answer to a probe, sent with no word asked for', asked(probe_resp, [[NO_WORD, '']])],
	['a key message to the client', key_asked(keymsg(1))],
	['the client\'s answer', key_heard(keymsg(1))],
	['the radio\'s word on a key message', key_status(keymsg(1), true)],
	['a neighbour\'s beacon', heard(beacon)],
	['the kernel\'s answer to a frame asked for: its number alone', entry(false, F, [[COOKIE, zeros(8)]], true)],
	['a client\'s counters (another command)', entry(false, 19, [[IFINDEX, 14], [MAC, mac(STA)], [FRAME, auth1]], true)],
	['netlink\'s own answer', substr(heard(auth1), 0, 20) + bytes([2, 0]) + substr(heard(auth1), 22)],
	['a word on a frame, going the wrong way', entry(true, F_STATUS, [[IFINDEX, 14], [FRAME, auth2], [ACK, '']], true)],
	['a data frame handed up as management', heard(dot11(0x08, 0x01, AP, STA, AP) + zeros(40))],
	['a frame too short for its header', heard(substr(auth1, 0, 20))],
	['an entry cut short', substr(heard(auth1), 0, 60)],
	['a key message that names no client', entry(false, PORT, [[IFINDEX, 14], [FRAME, keymsg(1)]], true)],
	['nothing at all', ''],
];
for (let c in cases)
	printf('%s: %s\n', c[0], show(capture.said(c[1])));

// What each is called where its client is traced (0144): the stage it is
// of, and words for it. Nothing is said of a frame to every station, of an
// answer to a probe, or of the radio's word that a frame was acknowledged.
function called(name, raw) {
	let w = capture.words(capture.said(raw));
	printf('%s: %s\n', name, w ? sprintf('%s, %s: %s%s', w.mac, w.stage, w.what, w.ok === false ? ' [no]' : '') : 'not said');
}
let ssid_ie = (s) => bytes([0, length(s)]) + s;
let keyinfo = (hi, lo) => bytes([2, 3, 0, 95, 2, hi, lo, 0, 16]) + zeros(90);
let action = (cat, act, more) => dot11(0xd0, 0, AP, STA, AP) + bytes([cat, act, ...(more ?? [])]);
called('a probe for any network', heard(dot11(0x40, 0, ALL, STA, ALL) + ssid_ie('') + zeros(20)));
called('a probe for one by name', heard(dot11(0x40, 0, ALL, STA, ALL) + ssid_ie('Sweet "Spot"') + zeros(20)));
called('the AP\'s answer to a probe', asked(probe_resp, [[NO_WORD, '']]));
called('an authentication request', heard(auth1));
called('one by a fast transition', heard(dot11(0xb0, 0, AP, STA, AP) + bytes([2, 0, 1, 0, 0, 0])));
called('its answer', asked(auth2));
called('an answer that refuses', asked(dot11(0xb0, 0, STA, AP, AP) + bytes([0, 0, 2, 0, 17, 0])));
called('the radio\'s word that the answer was acknowledged', status(auth2, true));
called('its word that it was not', status(auth2, false));
called('WPA3\'s commit', heard(dot11(0xb0, 0, AP, STA, AP) + bytes([3, 0, 1, 0, 0, 0]) + zeros(98)));
called('the AP\'s, asking for a token first', asked(dot11(0xb0, 0, STA, AP, AP) + bytes([3, 0, 1, 0, 76, 0]) + zeros(40)));
called('WPA3\'s confirm, refused', asked(dot11(0xb0, 0, STA, AP, AP) + bytes([3, 0, 2, 0, 15, 0])));
called('an association request', heard(assoc_req));
called('a reassociation request', heard(dot11(0x20, 0, AP, STA, AP) + bytes([0x31, 0x04, 10, 0]) + mac(OTHER) + zeros(40)));
called('the association\'s answer', asked(assoc_resp));
called('one that refuses', asked(dot11(0x10, 0, STA, AP, AP) + bytes([0x31, 0x04, 17, 0, 0, 0])));
called('key message 1', key_asked(keyinfo(0x00, 0x8a)));
called('key message 2', key_heard(keyinfo(0x01, 0x0a)));
called('key message 3', key_asked(keyinfo(0x13, 0xca)));
called('key message 4', key_heard(keyinfo(0x03, 0x0a)));
called('key message 3, not acknowledged', key_status(keyinfo(0x13, 0xca), false));
called('a group key message', key_asked(keyinfo(0x13, 0x82)));
called('a sign-in\'s request', key_asked(bytes([2, 0, 0, 5, 1, 7, 0, 5, 1])));
called('its response', key_heard(bytes([2, 0, 0, 5, 2, 7, 0, 5, 1])));
called('a sign-in refused', key_asked(bytes([2, 0, 0, 4, 4, 9, 0, 4])));
called('a client that starts a sign-in', key_heard(bytes([1, 1, 0, 0])));
called('a deauthentication from the client', heard(dot11(0xc0, 0, AP, STA, AP) + bytes([3, 0])));
called('one to it, for a handshake that timed out', asked(dot11(0xc0, 0, STA, AP, AP) + bytes([15, 0])));
called('a disassociation with a reason not named', asked(dot11(0xa0, 0, STA, AP, AP) + bytes([99, 0])));
called('a deauthentication to every station', asked(deauth_all));
called('asked to move', asked(dot11(0xd0, 0, STA, AP, AP) + bytes([10, 7, 1, 1])));
called('it agrees', heard(action(10, 8, [1, 0])));
called('it will not', heard(action(10, 8, [1, 6])));
called('it asks for the neighbours', heard(action(5, 4, [1])));
called('another action', heard(action(127, 0)));

// The frame made of each: its header, then the frame. A key message is the
// data frame it crossed the air in, to the network from the client or the
// other way, on the radio's frequency where it has none of its own.
function made(name, e, freq) {
	let m = capture.framed(e, AP, freq), rt = capture.header(m).rt;
	printf('%s: header %s, then %s..., names %J\n', name, hexdump(substr(m, 0, rt)), hexdump(substr(m, rt, 4)), capture.addresses(m));
}
made('heard on 2.4 GHz', capture.said(heard(auth1)));
made('sent on 2.4 GHz', capture.said(asked(auth2)));
made('a key message heard, on a radio last said to be on 5 GHz', capture.said(key_heard(keymsg(1))), 5220);
made('a key message sent, on 6 GHz', capture.said(key_asked(keymsg(1))), 6115);
made('one on a radio whose frequency was never said', capture.said(key_asked(keymsg(1))), null);
let km = capture.framed(capture.said(key_heard(keymsg(7))), AP, 2462), kh = capture.header(km).rt;
printf('a key message heard, after its header: %s\n', hexdump(substr(km, kh, 24 + 8 + 17)));

// A client's coming, as hostapd's rings have it: each frame is kept at
// hostapd's time of it, and the radio's word on one the AP sent is put on
// that frame when it comes, some milliseconds on. The third key message
// is not acknowledged; two deauthentications alike have each its word, the
// first not acknowledged and the second acknowledged; one to every
// station is acknowledged by none and not said to have failed; an answer to
// a probe has no word asked for; and the last frame's word never comes.
let r = capture.ring();
let t0 = 1791744500000;
let say = (ms, raw) => capture.ring_said(r, 100 + ms / 1000.0, t0 + ms, capture.said(raw), AP);
say(0, heard(probe_req));
say(1, asked(probe_resp, [[NO_WORD, '']]));
say(5000, heard(auth1));
say(5001.5, asked(auth2));
say(5003.5, status(auth2, true));
say(5007, heard(assoc_req));
say(5010, asked(assoc_resp));
say(5010.7, status(assoc_resp, true));
say(5110, key_asked(keymsg(1)));
say(5110.6, key_status(keymsg(1), true));
say(5114, key_heard(keymsg(1)));
say(5114.5, key_asked(keymsg(2)));
say(5140, key_status(keymsg(2), false));
say(6114.5, key_asked(keymsg(3)));
say(6115, key_status(keymsg(3), true));
say(7000, asked(deauth));
say(7001, asked(deauth));
say(7050, status(deauth, false));
say(7051, status(deauth, true));
say(7100, asked(deauth_all));
say(7101, status(deauth_all, false));
say(7200, status(assoc_resp, true));     // a word whose asking was not seen
say(7300, asked(auth2));                 // and an asking whose word never comes
printf('the ring: %d frames, %d bytes, on %d MHz; %d wait for the radio\'s word\n', length(r.list), r.bytes, r.freq, length(r.sent));
for (let f in r.list) {
	let rt = capture.header(f.m).rt, fc0 = ord(f.m, rt), names = capture.addresses(f.m);
	printf('  +%7.1f ms  %s %s  %s%s\n', f.wall - t0, names[1] == STA ? 'client' : 'AP    ',
		(fc0 & 0x0c) == 0x08 ? sprintf('key message %d', ord(f.m, rt + 24 + 8 + 16)) : sprintf('management %02x', fc0),
		f.sent ? sprintf('sent%s%s%s, its header\'s word %s', f.acked === true ? ', acknowledged' : f.acked === false ? ', not acknowledged' : ', no word yet',
			f.unasked ? ', none asked for' : '', f.all ? ', to every station' : '', hexdump(substr(f.m, rt - 2, 2))) : 'heard',
		capture.note_of(f) ? ' [' + capture.note_of(f) + ']' : '');
}
// A client's asking for networks, to every station, is handed to hostapd
// once for each network the radio has: kept once. The same asked again a
// tenth of a second on is another; and so is another client's between.
let asks = capture.ring(), ask = (ms, frame) => capture.ring_said(asks, 30 + ms / 1000.0, t0 + ms, capture.said(heard(frame)), AP) != null;
let other_ask = dot11(0x40, 0, ALL, OTHER, ALL) + zeros(40);
printf('a probe to every station, handed up six times, another client\'s among them, and again 100 ms on: kept %J\n',
	[ask(0, probe_req), ask(0.2, probe_req), ask(0.4, other_ask), ask(0.6, probe_req), ask(8, probe_req), ask(8.2, other_ask), ask(9, probe_req), ask(100, probe_req)]);
printf('one to this network alone, handed up twice: kept %J\n', [ask(200, auth1), ask(200.1, auth1)]);
// A word that comes long after its frame is of another.
let old = capture.ring();
capture.ring_said(old, 10, t0, capture.said(asked(auth2)), AP);
capture.ring_said(old, 16, t0 + 6000, capture.said(status(auth2, true)), AP);
printf('a word six seconds after its frame: %d frames kept, the first %s\n', length(old.list), old.list[0].acked == null ? 'with no word' : 'with it');
// And so many wait at once, at the most.
let busy = capture.ring();
for (let i = 0; i < 100; i++)
	capture.ring_said(busy, 20 + i / 1000.0, t0 + i, capture.said(asked(dot11(0x50, 0, STA, AP, AP) + bytes([i]) + zeros(40))), AP);
printf('of 100 frames sent in a tenth of a second, %d wait for a word\n', length(busy.sent));

// A radio's ring: frames by when they came. One client's are taken from it
// by a span of time.
let other = capture.framed(capture.said(heard(dot11(0xb0, 0, AP, OTHER, AP) + zeros(6))), AP);
capture.ring_add(r, 105.500, t0 + 5500, other);
printf('the client\'s, from 104 to 106: %d; from 90: %d; another\'s: %d; no one\'s: %d\n',
	length(capture.ring_of(r, STA, 104, 106)), length(capture.ring_of(r, STA, 90, 108)), length(capture.ring_of(r, OTHER, 90, 108)),
	length(capture.ring_of(r, '02:00:00:00:00:01', 90, 108)));
// Where it holds too much, the oldest go: when it is a quarter past what
// it may hold, back to that, so that the list is made anew now and then
// and not at each frame. And of one client so many are given at the most,
// the latest.
let plain = capture.radiotap(2462, -50, false);
let big = capture.ring(), trims = 0, most_held = 0;
for (let i = 0; i < 2600; i++) {
	let before = length(big.list);
	capture.ring_add(big, 1000 + i / 100, 1791745000000 + i * 10, plain + dot11(0x00, 0, AP, STA, AP) + zeros(2000));
	if (length(big.list) <= before)
		trims++;
	most_held = max(most_held, big.bytes);
}
printf('a full ring: %d frames, %d bytes, never more than %d, made anew %d times in 2600 frames, the oldest from %.2f\n', length(big.list), big.bytes, most_held, trims, big.list[0].at);
let small = capture.ring();
for (let i = 0; i < 10100; i++)
	capture.ring_add(small, 1000 + i / 1000, 1791745000000 + i, plain + auth1);
printf('a ring of small frames: %d at the most, %d now\n', 10000, length(small.list));
let many = capture.ring();
for (let i = 0; i < 700; i++)
	capture.ring_add(many, 1000 + i / 100, 1791745000000 + i * 10, plain + auth1);
let most = capture.ring_of(many, STA, 0, 2000);
printf('of %d frames of one client, %d are given, from %.2f\n', length(many.list), length(most), most[0].at);
printf('what names no station is not kept: %J, %J\n', capture.ring_add(many, 2000, t0, plain + bytes([0xd4, 0, 0, 0])), capture.ring_add(many, 2000, t0, 'short'));

// The file of an attempt from 105.0 to 107.4: the client's frames of the
// ten seconds before it and the two after, with those from the wire side
// in among them by when they came. Read back a block at a time; a frame's
// note is its comment.
let wire = [{ at: 105.300, wall: t0 + 5300, m: mac(ALL) + mac(STA) + bytes([0x08, 0x00]) + zeros(300) }];
let c = capture.capture(r, STA, 105.0, 107.4, wire, 'phy0, as hostapd had it', 'phy0-ap1, the wire side');
printf('capture: %d frames, %d from the radio, %d from the wire side, %d bytes\n', c.frames, c.radio, c.wire, length(c.file));
let u32 = (s, at) => ord(s, at) | (ord(s, at + 1) << 8) | (ord(s, at + 2) << 16) | (ord(s, at + 3) * 16777216);
for (let at = 0; at < length(c.file); ) {
	let type = u32(c.file, at), n = u32(c.file, at + 4);
	if (n < 12 || n % 4 || at + n > length(c.file) || u32(c.file, at + n - 4) != n) {
		printf('a block that does not close at %d\n', at);
		break;
	}
	if (type == 0x0a0d0d0a)
		printf('  section: byte order %08x, version %d.%d\n', u32(c.file, at + 8), ord(c.file, at + 12), ord(c.file, at + 14));
	else if (type == 1) {
		let size = ord(c.file, at + 18) | (ord(c.file, at + 19) << 8);
		printf('  interface: link type %d, at most %d bytes a frame, named "%s"\n', ord(c.file, at + 8) | (ord(c.file, at + 9) << 8), u32(c.file, at + 12), substr(c.file, at + 20, size));
	}
	else if (type == 6) {
		let size = u32(c.file, at + 20), after = at + 28 + size + (4 - size % 4) % 4;
		printf('  frame: interface %d, at %d us, %d bytes%s\n', u32(c.file, at + 8), (u32(c.file, at + 12) * 4294967296 + u32(c.file, at + 16)) % 100000000,
			size, after < at + n - 4 ? sprintf(', noted: %s', substr(c.file, after + 4, ord(c.file, after + 2) | (ord(c.file, after + 3) << 8))) : '');
	}
	else
		printf('  a block of type %d\n', type);
	at += n;
}
printf('nothing kept of a client: %J\n', capture.capture(r, '02:00:00:00:00:01', 105, 106, [], 'phy0', 'phy0-ap1'));
printf('no ring, and frames from the wire side alone: %d\n', capture.capture(null, STA, 105, 106, wire, 'phy0', 'phy0-ap1').frames);

// What is older than three minutes goes from a ring, once it is a quarter
// past that; and is given of no client before then, by its time.
let before = length(r.list);
capture.ring_add(r, 290.000, t0 + 190000, plain + auth1);
printf('three minutes on: %d frames held of %d, %d of the client given from then\n', length(r.list), before + 1, length(capture.ring_of(r, STA, 290 - 180, 290)));
capture.ring_add(r, 340.000, t0 + 240000, plain + auth1);
printf('four minutes on: %d frames, the oldest from %.3f\n', length(r.list), r.list[0].at);

// A file holds so many bytes of frames at the most: the latest.
let fat = capture.ring();
for (let i = 0; i < 400; i++)
	capture.ring_add(fat, 700 + i / 100.0, 1791745700000 + i * 10, plain + dot11(0xb0, 0, AP, STA, AP) + zeros(1960));
let c3 = capture.capture(fat, STA, 700, 704, [], 'phy0', 'phy0-ap1');
printf('of %d large frames of one client, %d are in its file: %s, as it may hold %d bytes of them\n', length(fat.list), c3.frames,
	c3.frames * length(fat.list[0].m) <= capture.MOST_BYTES && (c3.frames + 1) * length(fat.list[0].m) > capture.MOST_BYTES ? 'as many as fit' : 'not as many as fit', capture.MOST_BYTES);

// When the kernel says a frame of the wire side came: its stamp as a socket
// hands it, the bytes as they are, in either order and of either size; or
// read into numbers. One that is not near now, or has no part of a second
// at all, as ucode reads the stamp on a 32-bit processor, is not believed.
let near = 1791750185400;
let raw = (list) => bytes(list);
printf('a stamp of 16 bytes: %.3f; the other way round: %.3f; of 8 bytes: %.3f\n',
	capture.stamp(raw([0x29, 0xf0, 0xcb, 0x6a, 0, 0, 0, 0, 0x5d, 0x7f, 0x27, 0x15, 0, 0, 0, 0]), near),
	capture.stamp(raw([0, 0, 0, 0, 0x6a, 0xcb, 0xf0, 0x29, 0, 0, 0, 0, 0x15, 0x27, 0x7f, 0x5d]), near),
	capture.stamp(raw([0x29, 0xf0, 0xcb, 0x6a, 0x5d, 0x7f, 0x27, 0x15]), near));
printf('read into numbers: %.3f, %.3f; with no part of a second: %J; from another day: %J; later than now: %J; nothing: %J\n',
	capture.stamp({ sec: 1791750185, usec: 354910 }, near), capture.stamp({ sec: 1791750185, nsec: 354910045 }, near),
	capture.stamp({ sec: 1791750185, usec: 0 }, near), capture.stamp({ sec: 1791663785, usec: 354910 }, near),
	capture.stamp({ sec: 1791750186, usec: 354910 }, near), capture.stamp(null, near));
