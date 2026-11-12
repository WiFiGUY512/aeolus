// What aeolus-ble says to Home Assistant, and reads of the Bluetooth
// controller (0156). Home Assistant takes Bluetooth from afar by ESPHome's
// native API, as it does from an ESP32 set up as a Bluetooth proxy: a TCP
// connection on which each side sends messages, a type and protobuf fields
// each. This is the little of that API a proxy that only listens needs:
// who it is, that it has nothing else to offer, and every advertisement its
// radio hears, as the radio heard it.
//
// Pure functions, so that they can be tested anywhere.

'use strict';

// Left to itself the radio only listens; a client can ask it to scan
// actively, for a while or for good.
//
// The messages, by their number in ESPHome's api.proto.
const HELLO_REQUEST = 1, HELLO_RESPONSE = 2;
const AUTH_REQUEST = 3, AUTH_RESPONSE = 4;
const DISCONNECT_REQUEST = 5, DISCONNECT_RESPONSE = 6;
const PING_REQUEST = 7, PING_RESPONSE = 8;
const DEVICE_INFO_REQUEST = 9, DEVICE_INFO_RESPONSE = 10;
const LIST_ENTITIES_REQUEST = 11, LIST_ENTITIES_DONE = 19;
const SUBSCRIBE_ADVERTS = 66, UNSUBSCRIBE_ADVERTS = 87, RAW_ADVERTS = 93;
const SUBSCRIBE_FREE = 80, CONNECTIONS_FREE = 81;
const SCANNER_STATE = 126, SCANNER_SET_MODE = 127;
const CAPABILITIES_REQUEST = 149, CAPABILITIES_RESPONSE = 150;

// What a proxy can do, as bits (ESPHome's BluetoothProxyFeature): it can
// scan without asking anything of what it hears, hands each advertisement
// on as it was heard, and says whether it is scanning and how, and scans
// the way it is asked to.
const PASSIVE_SCAN = 1, RAW_ADVERTISEMENTS = 32, STATE_AND_MODE = 64;
const FEATURES = PASSIVE_SCAN | RAW_ADVERTISEMENTS | STATE_AND_MODE;

// The API's version this speaks, and the ESPHome release it says it is: one
// whose proxies Home Assistant takes without asking for an update.
const API_MAJOR = 1, API_MINOR = 10;
const ESPHOME_VERSION = '2026.9.0';

// How many advertisements go in one message, as ESPHome's own proxy sends.
const BATCH = 16;

// varint is a number as protobuf writes it: seven bits a byte, the least
// first, the top bit set on all but the last.
function varint(n) {
	let out = '';
	while (n > 127) {
		out += chr((n & 127) | 128);
		n = n >> 7;
	}
	return out + chr(n);
}

// Fields, each its number and kind, then its value.
function f_uint(field, n) {
	return varint(field << 3) + varint(n);
}

// A signed number, written so that small ones of either sign are short.
function f_sint(field, n) {
	return varint(field << 3) + varint(n >= 0 ? n * 2 : -n * 2 - 1);
}

function f_bytes(field, s) {
	return varint((field << 3) | 2) + varint(length(s)) + s;
}

function f_bool(field, on) {
	return on ? f_uint(field, 1) : '';
}

// frame is one message as it goes on the connection, unencrypted: a zero,
// how long its fields are, its type, its fields.
function frame(kind, payload) {
	payload ??= '';
	return chr(0) + varint(length(payload)) + varint(kind) + payload;
}

// read_varint reads a number at a place in bytes: [the number, where what
// follows begins], or null where the bytes end before it does.
function read_varint(b, at) {
	let n = 0, shift = 0;
	while (at < length(b)) {
		let c = ord(b, at++);
		n = n | ((c & 127) << shift);
		if (c < 128)
			return [n, at];
		shift += 7;
		if (shift > 56)
			return null;
	}
	return null;
}

// frames takes the whole messages off the front of what has come on a
// connection: { frames: [{ kind, payload }], rest: what is not yet a whole
// message, bad: true where it is no message at all }.
function frames(b) {
	let out = [], at = 0;
	while (at < length(b)) {
		if (ord(b, at) != 0)
			return { frames: out, rest: '', bad: true };
		let size = read_varint(b, at + 1);
		if (!size)
			break;
		let kind = read_varint(b, size[1]);
		if (!kind)
			break;
		if (size[0] > 65536)
			return { frames: out, rest: '', bad: true };
		if (kind[1] + size[0] > length(b))
			break;
		push(out, { kind: kind[0], payload: substr(b, kind[1], size[0]) });
		at = kind[1] + size[0];
	}
	return { frames: out, rest: substr(b, at) };
}

// fields reads a message's fields: { <number>: its last value }, a number
// for a varint, bytes for what has a length. What it cannot read ends it.
function fields(b) {
	let out = {}, at = 0;
	while (at < length(b)) {
		let key = read_varint(b, at);
		if (!key)
			break;
		let field = key[0] >> 3, wire = key[0] & 7;
		at = key[1];
		if (wire == 0) {
			let v = read_varint(b, at);
			if (!v)
				break;
			out[field] = v[0];
			at = v[1];
		} else if (wire == 2) {
			let n = read_varint(b, at);
			if (!n || n[1] + n[0] > length(b))
				break;
			out[field] = substr(b, n[1], n[0]);
			at = n[1] + n[0];
		} else if (wire == 5) {
			at += 4;
		} else if (wire == 1) {
			at += 8;
		} else
			break;
	}
	return out;
}

// hello answers a client's greeting: the API this speaks, and who it is.
function hello(name) {
	return frame(HELLO_RESPONSE, f_uint(1, API_MAJOR) + f_uint(2, API_MINOR) + f_bytes(3, 'Aeolus (OpenWrt)') + f_bytes(4, name));
}

// device_info says what this is: who: { name, friendly, mac, ble_mac,
// model, maker, area }. It is a Bluetooth proxy and nothing else.
function device_info(who) {
	return frame(DEVICE_INFO_RESPONSE,
		f_bytes(2, who.name) + f_bytes(3, who.mac) + f_bytes(4, ESPHOME_VERSION) + f_bytes(5, who.built ?? '') +
		f_bytes(6, who.model ?? 'OpenWrt AP') + f_bytes(8, 'aeolus.ble-proxy') + f_bytes(9, who.version ?? '') +
		f_bytes(12, who.maker ?? 'Aeolus') + f_bytes(13, who.friendly ?? who.name) + f_uint(15, FEATURES) +
		(who.area ? f_bytes(16, who.area) : '') + f_bytes(18, who.ble_mac ?? who.mac));
}

// capabilities says the same of what it can do, as a later client asks it.
function capabilities(who) {
	return frame(CAPABILITIES_RESPONSE, f_bytes(1, f_uint(1, FEATURES) + f_bytes(2, who.ble_mac ?? who.mac)));
}

// How the proxy says it is set up to scan, the state's configured mode
// (field 3): actively (1), which to Home Assistant means it may be asked
// to. Left out, the field reads as passive, and Home Assistant 2026.11
// takes that for a proxy that must only listen: it set the C-360s it had
// no setting for to passive for good, never asked them to scan actively,
// and warned that they would not (2026-11-07). The radio still only
// listens until a client asks; which proxies are asked is Home
// Assistant's to say, by each one's Bluetooth scanning mode.
const CONFIGURED_MODE = 1;

// scanner_state says whether the radio is scanning (2) or not (0), how:
// actively (mode 1) or only listening (mode 0), and how it is set up to
// scan (CONFIGURED_MODE).
function scanner_state(scanning, active) {
	return frame(SCANNER_STATE, f_uint(1, scanning ? 2 : 0) + (active ? f_uint(2, 1) : '') + f_uint(3, CONFIGURED_MODE));
}

// address is a MAC as the API has it: one number, its first byte the
// highest.
function address(mac) {
	let n = 0;
	for (let part in split(mac, ':'))
		n = n * 256 + hex(part);
	return n;
}

// raw_adverts is advertisements as the API hands them on, a message for
// each BATCH of them: each { mac, type (0 public, 1 random), rssi, data }.
function raw_adverts(list) {
	let out = '';
	for (let i = 0; i < length(list); i += BATCH) {
		let payload = '';
		for (let a in slice(list, i, i + BATCH))
			payload += f_bytes(1, f_uint(1, address(a.mac)) + f_sint(2, a.rssi) + f_uint(3, a.type) + f_bytes(4, a.data));
		out += frame(RAW_ADVERTS, payload);
	}
	return out;
}

// answer is what to send for a message a client sent, and what it changes:
// { send, subscribe: true | false | null, active: true | false | null,
// close: true }. who is as device_info takes it; scanning, whether the
// radio is, and active, whether it scans actively.
function answer(m, who, scanning, active) {
	switch (m.kind) {
	case HELLO_REQUEST:
		return { send: hello(who.name) };
	case AUTH_REQUEST:
		// No password is asked: whoever reaches the port is let in.
		return { send: frame(AUTH_RESPONSE) };
	case PING_REQUEST:
		return { send: frame(PING_RESPONSE) };
	case DEVICE_INFO_REQUEST:
		return { send: device_info(who) };
	case CAPABILITIES_REQUEST:
		return { send: capabilities(who) };
	case LIST_ENTITIES_REQUEST:
		// It has no switches and no sensors.
		return { send: frame(LIST_ENTITIES_DONE) };
	case SUBSCRIBE_ADVERTS:
		return { send: scanner_state(scanning, active), subscribe: true };
	case UNSUBSCRIBE_ADVERTS:
		return { subscribe: false };
	case SUBSCRIBE_FREE:
		// It makes no connections: none free, of none.
		return { send: frame(CONNECTIONS_FREE, '') };
	case SCANNER_SET_MODE:
		// How the client wants it to scan from here on. The daemon says
		// how it scans once it does.
		return { active: fields(m.payload)[1] == 1 };
	case DISCONNECT_REQUEST:
		return { send: frame(DISCONNECT_RESPONSE), close: true };
	case DISCONNECT_RESPONSE:
		return { close: true };
	}
	// What it has no answer to is let pass: states, logs, the time.
	return {};
}

// BlueZ 5.87 has no hcidump (port -104, 2026-11-01): there, btmon reads
// the controller, writing btsnoop (0281). Its stream starts with
// BTSNOOP_HEAD, magic, version 1 and the Linux monitor's datalink, 2001;
// then each record is a header of BTSNOOP_RECORD bytes, big-endian (its
// data's length at 4, its flags at 8, the controller's index above, the
// kind of packet below), and its data. A command to the controller is kind
// BTSNOOP_COMMAND, an event from it BTSNOOP_EVENT: as hcidump wrote them,
// without the H4 byte before each that says which (1, 4).
const BTSNOOP_HEAD = 16, BTSNOOP_RECORD = 24, BTSNOOP_COMMAND = 2, BTSNOOP_EVENT = 3;
const H4 = { '2': '01', '3': '04' };   // BTSNOOP_COMMAND, BTSNOOP_EVENT

function be32(s, at) {
	return (ord(s, at) << 24) | (ord(s, at + 1) << 16) | (ord(s, at + 2) << 8) | ord(s, at + 3);
}

// btsnoop_stream says whether a stream starts as btmon's btsnoop does.
function btsnoop_stream(head) {
	return type(head) == 'string' && length(head) == BTSNOOP_HEAD && substr(head, 0, 8) == 'btsnoop\0' &&
		be32(head, 8) == 1 && be32(head, 12) == 2001;
}

// btsnoop_record is a record's header read: { len, index, kind }, or null.
function btsnoop_record(h) {
	if (type(h) != 'string' || length(h) != BTSNOOP_RECORD)
		return null;
	let flags = be32(h, 8);
	return { len: be32(h, 4), index: (flags >> 16) & 0xffff, kind: flags & 0xffff };
}

// hci_text is a record of one kind as hcidump's raw output wrote it, its H4
// byte first: "04 3E 2B 02 ...". null for a kind hcidump did not write, or
// that the daemon has no use for.
function hci_text(kind, data) {
	let h4 = H4['' + kind];
	if (!h4)
		return null;
	let out = [h4];
	for (let i = 0; i < length(data); i++)
		push(out, sprintf('%02X', ord(data, i)));
	return join(' ', out);
}

// hci_bytes is the bytes of one packet as hcidump's raw output writes it,
// its first line and those that carry it on: " 04 3E 2B 02 ..." then
// "  20 22 AA ...", as numbers.
function hci_bytes(text) {
	let out = [];
	for (let part in split(trim(text), /[ \t]+/))
		if (match(part, /^[0-9A-Fa-f]{2}$/))
			push(out, hex(part));
	return out;
}

// adverts reads the advertisements in one packet from the controller: an
// LE advertising report (event 0x3E, the first of its kinds), which may
// hold several. Each is { mac, type, rssi, data, kind }; kind 4 is an
// answer to a scan, which a radio that only listens never gets.
function adverts(bytes) {
	let out = [];
	if (length(bytes) < 5 || bytes[0] != 0x04 || bytes[1] != 0x3e || bytes[3] != 0x02)
		return out;
	let n = bytes[4], at = 5;
	for (let i = 0; i < n; i++) {
		if (at + 9 > length(bytes))
			break;
		let kind = bytes[at], of = bytes[at + 1], size = bytes[at + 8];
		if (at + 9 + size + 1 > length(bytes) || size > 31)
			break;
		let mac = [];
		for (let k = 7; k >= 2; k--)
			push(mac, sprintf('%02X', bytes[at + k]));
		let data = '';
		for (let k = 0; k < size; k++)
			data += chr(bytes[at + 9 + k]);
		let rssi = bytes[at + 9 + size];
		push(out, { mac: join(':', mac), type: of & 1, rssi: rssi > 127 ? rssi - 256 : rssi, data: data, kind: kind });
		at += 10 + size;
	}
	return out;
}

export {
	FEATURES, BATCH, HELLO_REQUEST, DEVICE_INFO_REQUEST, SUBSCRIBE_ADVERTS, PING_REQUEST, LIST_ENTITIES_REQUEST, RAW_ADVERTS,
	varint, frame, frames, fields, read_varint, hello, device_info, capabilities, scanner_state, address, raw_adverts, answer,
	hci_bytes, adverts, BTSNOOP_HEAD, BTSNOOP_RECORD, btsnoop_stream, btsnoop_record, hci_text
};
