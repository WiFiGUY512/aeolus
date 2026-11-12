// A radio's firmware that crashed (0310), as the kernel's log says it. By
// porting's B17 on the C-360s' QCN9074 (2026-11-05), the lines are:
//
//	ath11k_pci 0001:01:00.0: firmware crashed: MHI_CB_EE_RDDM
//	ieee80211 phy12: Hardware restart was requested
//	Hardware became unavailable during restart.
//
// the last inside a mac80211 warning with no device of its own, where the
// radio did not come back: it stays down until its driver is bound again.
// ath10k says "firmware crashed!". And the stock driver can say no crash at
// all: only "<driver> <device>: failed to send WMI_<X> cmd" every few
// seconds while the firmware does not answer, and the hardware restart. So
// a crash is any of: the driver's line; a hardware restart asked with no
// such line near it; a run of failed WMI commands with no such line near
// it. Each is timed by the kernel's own clock, seconds since it started, so
// the log's dates and the AP's clock do not matter.
//
// The IPQ's own radios (AHB) run their firmware on a remote processor, and
// its crash is the remote processor's (the W-318, 2026-11-05):
//
//	remoteproc remoteproc0: crash detected in wcnss: type fatal error
//
// which names no Wi-Fi device; failed commands follow every second or so
// until the processor is up again (170 s there), then the hardware restart
// of each radio: one crash, not three.

'use strict';

const LOST_WITHIN = 60;   // seconds after a crash within which a radio that is lost is that crash's
const NEAR = 30;          // seconds within which a restart, or failed commands, are a crash's said already
const WMI_RUN = 5;        // failed commands, each within NEAR of the one before, that are a firmware not answering
const WMI_QUIET = 120;    // seconds without a failed command after which a run of them is over
const MOST = 8;           // the latest crashes said
const KEEP = 32;          // of each kind, the latest that merge keeps

// GREP is what the agent asks the log for, as grep -E has it.
const GREP = 'firmware crashed|crash detected in|Hardware restart was requested|Hardware became unavailable during restart|failed to send WMI_';

// parse reads the log's lines, in order: the crashes the driver said, each
// { up, driver, device, why }; the hardware restarts asked, each { up, phy };
// the radios lost, each { up, phy }, phy being the latest restart's before
// it where the line names none; and the failed WMI commands, each { up,
// driver, device }. up is the kernel's seconds.
function parse(lines) {
	let crashes = [], restarts = [], lost = [], wmi = [];
	for (let line in lines ?? []) {
		let t = match(line, /\[ *([0-9]+\.[0-9]+)\] (.*)$/);
		if (!t)
			continue;
		let up = +t[1], text = t[2], m;
		if ((m = match(text, /^([A-Za-z0-9_-]+) ([^ ]+): firmware crashed(!|: ?([^ ].*))?/)))
			push(crashes, { up: up, driver: m[1], device: lc(m[2]), why: m[4] ? substr(trim(m[4]), 0, 60) : null });
		else if ((m = match(text, /^remoteproc ([^ ]+): crash detected in ([^:]+): type (.+)$/)))
			push(crashes, { up: up, driver: 'remoteproc', device: null, rproc: m[1], why: substr(sprintf('%s: %s', m[2], trim(m[3])), 0, 60) });
		else if ((m = match(text, /^ieee80211 (phy[0-9]+): Hardware restart was requested/)))
			push(restarts, { up: up, phy: m[1] });
		else if (match(text, /Hardware became unavailable during restart/)) {
			let own = match(text, /^ieee80211 (phy[0-9]+):/);
			let before = filter(restarts, r => r.up <= up);
			push(lost, { up: up, phy: own ? own[1] : (length(before) ? before[length(before) - 1].phy : null) });
		}
		else if ((m = match(text, /^([A-Za-z0-9_-]+) ([^ ]+): failed to send WMI_/)))
			push(wmi, { up: up, driver: m[1], device: lc(m[2]) });
	}
	return { crashes: crashes, restarts: restarts, lost: lost, wmi: wmi };
}

// said is the crashes as the report says them, the oldest first, the latest
// MOST, each { ago, driver, device, why, lost, failing, radios, bands, scan }:
// ago, the seconds since, by the kernel's clock now (uptime); lost, whether
// a radio was lost within LOST_WITHIN of it whose phy is the device's
// (device_of, phy to device, where it still has one) or is not known;
// failing, whether a run of failed commands of it goes on, its last within
// WMI_QUIET; radios, bands and scan, of the device as radio_of (device to
// { radios, bands, scan }) knows it: a SoC's own radios share one device, as
// the IPQ8074's do, and scan says it is the AP's scan radio (0126). Whether
// it came back is the agent's to see: a radio lost can be bound again, as
// airscan heals the scan radio, and a recovery can stall with nothing said.
function said(p, uptime, device_of, radio_of) {
	let found = [];
	// A remote processor's crash names no Wi-Fi device: it is the device
	// of failed commands that begin within NEAR after it; else the AP's one
	// device not on PCI, its SoC's own radios; else the processor.
	let soc = uniq(filter(values(device_of ?? {}), d => !match(d, /^[0-9a-f]{4}:[0-9a-f]{2}:/)));
	for (let c in p.crashes) {
		let f = { ...c };
		if (f.device == null) {
			let w = filter(p.wmi, w => w.up >= f.up && w.up - f.up <= NEAR);
			f.device = length(w) ? w[0].device : (length(soc) == 1 ? soc[0] : f.rproc);
		}
		delete f.rproc;
		push(found, f);
	}
	// The crash said of a device near a time, the latest; of any device,
	// where dev is null.
	let near = (dev, lo, hi) => {
		let f = filter(found, f => (dev == null || f.device == dev) && f.up >= lo && f.up <= hi);
		return length(f) ? f[length(f) - 1] : null;
	};
	// Each run of failed WMI commands, each within NEAR of the one before,
	// with no crash said near it.
	// One may be a run kept already, { up, last, n } (merge).
	let runs = {};
	for (let w in p.wmi) {
		let list = runs[w.device] ??= [];
		let r = length(list) ? list[length(list) - 1] : null;
		let last = w.last ?? w.up, n = w.n ?? 1;
		if (r && w.up - r.last <= NEAR) {
			if (last > r.last)
				r.last = last;
			r.n += n;
		}
		else
			push(list, { first: w.up, last: last, n: n, driver: w.driver });
	}
	// A hardware restart asked with no crash said near it, and not after
	// the start of a run of failed commands of its device and within NEAR of
	// its end (a remote processor's recovery ends so): of its phy's device,
	// or, where the phy has none now, of any. One before a run, as the
	// stock driver's, is the crash the run is of.
	let inRun = (dev, up) => length(filter(runs[dev] ?? [], r => up >= r.first && up <= r.last + NEAR)) > 0;
	for (let r in p.restarts) {
		let dev = device_of?.[r.phy];
		if (!near(dev, r.up - NEAR, r.up + NEAR) && !(dev && inRun(dev, r.up)))
			push(found, { up: r.up, driver: 'mac80211', device: dev ?? r.phy, why: 'a hardware restart was asked' });
	}
	// A run near a crash said is that crash's: while it goes on, the crash
	// did not come back. It is over WMI_QUIET after its last failed
	// command, or at once where a hardware restart of its device ends it,
	// as a remote processor's recovery does.
	for (let dev, list in runs)
		for (let r in list) {
			if (r.n < WMI_RUN)
				continue;
			let ended = length(filter(p.restarts, x => device_of?.[x.phy] == dev && x.up >= r.last && x.up <= r.last + NEAR)) > 0;
			let quiet = ended || uptime - r.last > WMI_QUIET;
			let c = near(dev, r.first - NEAR, r.last);
			if (c)
				c.quiet = (c.quiet ?? true) && quiet;
			else
				push(found, { up: r.first, driver: r.driver, device: dev, why: sprintf('its firmware stopped answering: %d commands failed', r.n),
					quiet: quiet });
		}
	let out = [];
	for (let c in sort(found, (a, b) => a.up - b.up)) {
		let gone = filter(p.lost, l => l.up >= c.up && l.up - c.up <= LOST_WITHIN &&
			(l.phy == null || device_of?.[l.phy] == null || device_of[l.phy] == c.device || l.phy == c.device));
		let r = radio_of?.[c.device];
		push(out, { ago: int(uptime - c.up), driver: c.driver, device: c.device, why: c.why, lost: length(gone) > 0, failing: c.quiet === false,
			radios: r?.radios ?? [], bands: r?.bands ?? [], scan: !!r?.scan });
	}
	return slice(out, -MOST);
}

// merge is what parse read of the log now, with what was kept from before
// (kept, merge's own, or null), as said reads it: so a crash whose lines
// rolled out of the log's ring, as the Garage's did in under two hours
// (2026-11-05), is still said, by the kernel's clock, until the AP starts
// again. The agent keeps it on tmpfs, which starts again with the kernel's
// clock; one later than uptime is of a clock before, and passed over. A
// crash, restart or lost radio is the same one where its kind, device or
// phy, and time are; of each, the latest KEEP. The failed commands are
// kept as runs, each { up, last, n, driver, device }: a failed command at
// or before the last of its device's latest run is in it already; one
// within NEAR after, the run goes on; else it starts another.
function merge(kept, now, uptime) {
	let fresh = e => type(e) == 'object' && type(e.up) in [ 'int', 'double' ] && e.up >= 0 && e.up <= uptime + 1;
	let byUp = (a, b) => a.up - b.up;
	let out = {};
	for (let k in [ 'crashes', 'restarts', 'lost' ]) {
		let seen = {}, list = [];
		for (let e in [ ...(kept?.[k] ?? []), ...(now?.[k] ?? []) ]) {
			if (!fresh(e))
				continue;
			let id = sprintf('%s %s %.6f', e.device ?? '', e.phy ?? '', e.up);
			if (!seen[id]) {
				seen[id] = true;
				push(list, e);
			}
		}
		out[k] = slice(sort(list, byUp), -KEEP);
	}
	let runs = {};
	for (let r in sort(filter(kept?.wmi ?? [], fresh), byUp))
		push(runs[r.device] ??= [], { up: r.up, last: r.last ?? r.up, n: r.n ?? 1, driver: r.driver, device: r.device });
	for (let w in sort(filter(now?.wmi ?? [], fresh), byUp)) {
		let list = runs[w.device] ??= [];
		let r = length(list) ? list[length(list) - 1] : null;
		if (r && w.up <= r.last)
			continue;
		if (r && w.up - r.last <= NEAR) {
			r.last = w.up;
			r.n++;
		}
		else
			push(list, { up: w.up, last: w.up, n: 1, driver: w.driver, device: w.device });
	}
	let wmi = [];
	for (let dev, list in runs)
		push(wmi, ...list);
	out.wmi = slice(sort(wmi, byUp), -KEEP);
	return out;
}

export { LOST_WITHIN, NEAR, WMI_RUN, WMI_QUIET, MOST, KEEP, GREP, parse, said, merge };
