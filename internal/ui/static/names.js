// The names people give clients (0153): a device that says no name of its
// own, or one that says nothing of what it is, is called what someone here
// called it, wherever the page shows it. The manager keeps them by MAC; the
// page reads them once a draw, and every view asks here what a client is
// called.

import { get, put } from './api.js';

let names = new Map();
// The name each client last gave of itself, as the manager kept it: an AP's
// report says it only while the AP remembers the client's DHCP.
let hosts = new Map();

// loadNames reads the names the signed-in person may see. Where they cannot
// be had, the page shows what each device calls itself, as before.
export async function loadNames() {
	try {
		const r = await get('/v1/client-names');
		names = new Map(Object.entries(r.names || {}));
		hosts = new Map(Object.entries(r.hosts || {}));
	} catch {
		// Kept as they were.
	}
}

// given is the name a person gave the client of a MAC, or ''.
export function given(mac) {
	return names.get(String(mac || '').toLowerCase()) || '';
}

// called is what a client is called: the name a person gave it; else what
// it calls itself, as its AP's report says (host); else what it last called
// itself, where the report no longer says; else ''.
export function called(mac, host) {
	return given(mac) || host || hosts.get(String(mac || '').toLowerCase()) || '';
}

// namedLike is the MAC of a client a person gave a name that is q, else
// one whose name holds q, ignoring case; ''. The header's Find (0284).
export function namedLike(q) {
	const low = q.toLowerCase();
	const all = [...names.entries()];
	return (all.find(([, n]) => n.toLowerCase() === low) || all.find(([, n]) => n.toLowerCase().includes(low)))?.[0] || '';
}

// itsName is what a device calls itself: as its AP's report says (host),
// else as it last said; ''. Shown beside a name a person gave it.
export function itsName(mac, host) {
	return host || hosts.get(String(mac || '').toLowerCase()) || '';
}

// nameClient gives a client a name, or with '' takes its name away, and
// answers with the name as the manager kept it.
export async function nameClient(mac, name) {
	const r = await put(`/v1/clients/${encodeURIComponent(mac)}/name`, { name });
	if (r.name) names.set(r.mac, r.name);
	else names.delete(r.mac);
	return r.name;
}
