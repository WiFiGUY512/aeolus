// The Wi-Fi clients on now, from each AP's last report, each once (0185). An
// AP lists a client until hostapd times it out, minutes after it roamed
// away without saying so, so the same client can be on two APs' lists at
// once (Griff, 2026-10-20: "this cannot be true at the same time"). The
// manager names, in an AP's condition, the clients its report lists that
// were active on another AP since (elsewhere); those are left out. Of what
// is left, a client two APs list is shown on the AP it was last active on:
// that AP's report's time, less how long it had been idle then; of two
// alike, the one it joined last.
//
// An AP reports its clients every five minutes, and its minutes every
// minute (0207). The manager goes by the minutes too (0251): a client that
// sent an AP anything in a minute is on it from then. The AP it left names
// it elsewhere within a minute, and the AP it is on names it as arrived
// until its next report lists it, with what its minute came to. Each AP
// says too, at once, that a client joined it or left it (0252): an arrival
// then is what its join said, its band, network and signal, until its
// minute says more, and a client that left is elsewhere.

// hereNow is the clients an AP has now, from its condition: those its
// report lists that are its own, and those its minutes put on it since
// (0251), each with what known (by MAC) says of who it is, where given.
export function hereNow(cond, known) {
	const gone = new Set(cond?.elsewhere || []);
	return [...(cond?.state?.report?.clients || []).filter((c) => !gone.has(c.mac)),
		...(cond?.arrived || []).map((m) => arrival(m, known?.get(m.mac)))];
}

// arrival is a client an AP's minute put on it (0251): its band, network,
// signal, rates and what was sent to it as the minute came to; who it is
// and its Wi-Fi, from a report of it on another AP, and on the same network
// its address too. Its MCS, streams, data and how long it has been on are
// the AP's next report's to say.
function arrival(m, was) {
	const same = was && was.ssid === m.ssid;
	return {
		...(was || {}), ...(same ? {} : { network: undefined, address: undefined, dhcp: undefined, vlan: undefined, user: undefined }),
		mac: m.mac, band: m.band, ssid: m.ssid, signal: m.signal, signal_avg: m.signal, tx_rate: m.tx_rate, rx_rate: m.rx_rate,
		tx_mcs: null, rx_mcs: null, tx_nss: null, rx_nss: null,
		tx_packets: m.tx_packets, tx_retries: m.tx_retries, tx_failed: m.tx_failed, rx_packets: m.rx_packets,
		tx_bytes: null, rx_bytes: null, connected: null, inactive_ms: null,
		arrived: new Date(m.at).getTime() / 1000 + (m.secs || 0),
	};
}

// onNow is, from rows ([{ ap, cfg }]), each client on now once:
// [{ c, ap, cfg, st, at }], c its report's entry (or its arrival, 0251), st
// the report, at the report's time in seconds.
export function onNow(rows) {
	// Each client's latest report entry, on any AP: who an arrival is.
	const latest = new Map();
	for (const { cfg } of rows) {
		const st = cfg?.condition?.state;
		const at = st ? new Date(st.at).getTime() / 1000 : 0;
		for (const c of st?.report?.clients || []) if (!latest.has(c.mac) || at > latest.get(c.mac).at) latest.set(c.mac, { c, at });
	}
	const known = new Map([...latest].map(([mac, { c }]) => [mac, c]));
	const best = new Map();
	for (const { ap, cfg } of rows) {
		const st = cfg?.condition?.state;
		const at = st ? new Date(st.at).getTime() / 1000 : 0;
		for (const c of hereNow(cfg?.condition, known)) {
			const e = c.arrived != null ? { c, ap, cfg, st, at, active: c.arrived, joined: c.arrived }
				: { c, ap, cfg, st, at, active: at - (c.inactive_ms ?? 0) / 1000, joined: at - (c.connected ?? 0) };
			const was = best.get(c.mac);
			if (!was || e.active > was.active || (e.active === was.active && e.joined > was.joined)) best.set(c.mac, e);
		}
	}
	return [...best.values()];
}

// gone is, by band, how many clients an AP's report lists that are on
// another AP now, less those its minutes put on it since (0251).
export function gone(cond) {
	const left = new Set(cond?.elsewhere || []);
	const out = {};
	for (const c of cond?.state?.report?.clients || []) if (left.has(c.mac)) out[c.band] = (out[c.band] || 0) + 1;
	for (const m of cond?.arrived || []) out[m.band] = (out[m.band] || 0) - 1;
	return out;
}
