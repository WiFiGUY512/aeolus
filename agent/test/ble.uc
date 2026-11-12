// Prints what aeolus-ble's library makes of the Bluetooth controller's
// packets and of a client's messages (0156), for a test to compare with
// ble.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/ble.uc

'use strict';

import * as esp from 'aeolus.esphome';

function hexed(s) {
	let out = [];
	for (let i = 0; i < length(s); i++)
		push(out, sprintf('%02x', ord(s, i)));
	return join(' ', out);
}

print('== numbers as protobuf writes them\n');
for (let n in [0, 1, 127, 128, 300, 16384, 0xaabbcc000001])
	print(sprintf('%d: %s\n', n, hexed(esp.varint(n))));
print('read back: ', esp.read_varint(esp.varint(300) + 'x', 0), ' ', esp.read_varint(chr(0x80), 0), '\n');

print('== a packet from the controller, as hcidump writes it\n');
// One advertisement that cannot be connected to, from a random address, 31
// bytes, heard at -73 dBm; the addresses here are made up.
let lines = '> 04 3E 2B 02 01 03 01 01 00 00 CC BB AA 1F 1E FF 06 00 01 09 \n' +
	'  20 22 01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F 10 11 12 \n' +
	'  13 14 15 16 17 B7 \n';
let bytes = esp.hci_bytes(replace(lines, '>', ''));
print(length(bytes), ' bytes\n');
let seen = esp.adverts(bytes);
for (let a in seen)
	print(sprintf('%s type %d kind %d rssi %d data %d bytes: %s\n', a.mac, a.type, a.kind, a.rssi, length(a.data), hexed(a.data)));
// A connectable one from a public address, with a name in it, at -40 dBm.
seen = esp.adverts(esp.hci_bytes('04 3E 1A 02 01 00 00 02 00 00 CC BB AA 0E 02 01 06 03 02 AF FE 06 09 4E 30 35 4C 38 D8'));
for (let a in seen)
	print(sprintf('%s type %d kind %d rssi %d data: %s\n', a.mac, a.type, a.kind, a.rssi, hexed(a.data)));
print('== what is no advertisement is none\n');
print(length(esp.adverts(esp.hci_bytes('04 0E 04 01 0C 20 00'))), ' from a command\'s answer; ',
	length(esp.adverts(esp.hci_bytes('04 3E 2B 02 01 03 01 01 00 00'))), ' from one cut short; ',
	length(esp.adverts(esp.hci_bytes('04 3E 0C 02 01 00 00 02 00 00 CC BB AA 40 00 D8'))), ' from one that says it is longer than it is; ',
	length(esp.adverts([])), ' from nothing\n');

print('== messages off a connection\n');
let hello = esp.frame(esp.HELLO_REQUEST, chr(0x0a, 0x04) + 'test' + chr(0x10, 1, 0x18, 10));
let two = hello + esp.frame(esp.PING_REQUEST) + substr(esp.frame(esp.DEVICE_INFO_REQUEST), 0, 2);
let got = esp.frames(two);
print(length(got.frames), ' whole, ', length(got.rest), ' bytes kept for the rest\n');
for (let m in got.frames)
	print(sprintf('kind %d, %d bytes: %J\n', m.kind, length(m.payload), esp.fields(m.payload)));
print('half a message: ', esp.frames(substr(hello, 0, 4)), '\n');
print('not a message: ', esp.frames('GET / HTTP/1.1\r\n'), '\n');
print('one too long: ', esp.frames(chr(0) + esp.varint(100000) + chr(1)).bad, '\n');

print('== what it answers\n');
let who = { name: 'c360-ap', friendly: 'C360-AP Bluetooth', mac: '30:86:2D:EF:2F:FF', ble_mac: 'F0:86:2D:EF:2F:F8', model: 'Arista C-360', version: 'v0.0.0' };
function said(what, m) {
	let a = esp.answer(m, who, true, false);
	let back = a.send ? esp.frames(a.send).frames : [];
	print(sprintf('%s: %s%s%s\n', what, join(', ', map(back, f => sprintf('kind %d %J', f.kind, esp.fields(f.payload)))),
		a.subscribe != null ? ' subscribe ' + a.subscribe : '', a.active != null ? ' active ' + a.active : '', a.close ? ' close' : ''));
}
said('hello', { kind: 1, payload: '' });
said('a password', { kind: 3, payload: '' });
said('who are you', { kind: 9, payload: '' });
said('what can you do', { kind: 149, payload: '' });
said('what have you', { kind: 11, payload: '' });
said('ping', { kind: 7, payload: '' });
said('advertisements', { kind: 66, payload: chr(8, 1) });
said('no more of them', { kind: 87, payload: '' });
said('connections', { kind: 80, payload: '' });
said('scan actively', { kind: 127, payload: chr(8, 1) });
said('only listen', { kind: 127, payload: '' });
print('scanning actively: ', sprintf('%J', esp.fields(esp.frames(esp.scanner_state(true, true)).frames[0].payload)),
	'; listening: ', sprintf('%J', esp.fields(esp.frames(esp.scanner_state(true, false)).frames[0].payload)),
	'; not scanning: ', sprintf('%J', esp.fields(esp.frames(esp.scanner_state(false, false)).frames[0].payload)), '\n');
let asked = esp.answer({ kind: 66, payload: chr(8, 1) }, who, true, true);
print('advertisements, while it scans actively: ', sprintf('%J', esp.fields(esp.frames(asked.send).frames[0].payload)), '\n');
said('states', { kind: 20, payload: '' });
said('goodbye', { kind: 5, payload: '' });

print('== advertisements handed on\n');
let list = [];
for (let i = 0; i < 18; i++)
	push(list, { mac: sprintf('AA:BB:CC:00:00:%02X', i), type: i & 1, rssi: -40 - i, data: chr(2, 1, 6) });
let out = esp.frames(esp.raw_adverts(list));
print(length(out.frames), ' messages for 18: ', join(' and ', map(out.frames, f => sprintf('kind %d of %d bytes', f.kind, length(f.payload)))), '\n');
let first = esp.fields(esp.fields(out.frames[0].payload)[1]);
print(sprintf('the last of the first message: address %x rssi field %d type %d data %s\n', first[1], first[2], first[3] ?? 0, hexed(first[4])));
print('an address: ', sprintf('%x', esp.address('AA:BB:CC:00:00:01')), '\n');
print('none: ', length(esp.raw_adverts([])), ' bytes\n');

print('== btmon\'s btsnoop, where BlueZ has no hcidump (0281)\n');
let be = (n) => chr((n >> 24) & 255, (n >> 16) & 255, (n >> 8) & 255, n & 255);
let head = 'btsnoop' + chr(0) + be(1) + be(2001);
print('a stream: ', esp.btsnoop_stream(head), '; of another datalink: ', esp.btsnoop_stream('btsnoop' + chr(0) + be(1) + be(1002)),
	'; cut short: ', esp.btsnoop_stream(substr(head, 0, 10)), '\n');
// A record: its data's length twice, its flags (the controller's index
// above, the kind of packet below), drops and a time.
let rec = (index, kind, data) => be(length(data)) + be(length(data)) + be((index << 16) | kind) + be(0) + be(0) + be(0x00e33cf5);
let ev = chr(0x3e, 0x1a, 0x02, 0x01, 0x00, 0x00, 0x02, 0x00, 0x00, 0xcc, 0xbb, 0xaa, 0x0e, 0x02, 0x01, 0x06, 0x03, 0x02, 0xaf, 0xfe, 0x06, 0x09, 0x4e, 0x30, 0x35, 0x4c, 0x38, 0xd8);
let r = esp.btsnoop_record(substr(rec(0, 3, ev), 0, esp.BTSNOOP_RECORD));
print(sprintf('an event\'s header: %J; a short one: %J\n', r, esp.btsnoop_record('short')));
let text = esp.hci_text(r.kind, ev);
print('as hcidump wrote it: ', text, '\n');
for (let a in esp.adverts(esp.hci_bytes(text)))
	print(sprintf('read back: %s type %d kind %d rssi %d data: %s\n', a.mac, a.type, a.kind, a.rssi, hexed(a.data)));
print('to scan: ', esp.hci_text(2, chr(0x0c, 0x20, 0x02, 0x01, 0x00)), '; a note: ', esp.hci_text(12, 'Linux version') ?? 'none', '\n');
