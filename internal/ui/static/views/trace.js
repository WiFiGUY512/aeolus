// Live Trace (0144): one client, on or not, watched as it moves. Every AP
// says each thing it sees of it as it happens: that it hears it asking and
// how strongly, its authentication, its association, each message of its
// keys, its address, its gateway, its names, that it is on, that it left.
// They are put in one line here, each by the manager's clock, the newest
// last, so that a client carried from one AP to the next is seen to leave
// the one and come to the other, a step at a time.
//
// The manager sends each thing as it has it (0145): the panel holds an
// answer open and draws what comes on it. Where that cannot be held it asks
// again every couple of seconds instead. It keeps the page from redrawing
// under it (data-editing), as a form does.

import { h } from '../dom.js';
import { get, post, del, lines } from '../api.js';
import { bandName } from '../format.js';
import { floatWindow, floating } from '../float.js';
import { lookupsWindow } from './lookups.js';
import { pathWindow } from './pathtrace.js';

// What each stage is called.
const STAGE = {
	find: 'Looking', auth: 'Authentication', assoc: 'Association', signin: 'Sign-in', key: 'Keys', dhcp: 'Address', gateway: 'Gateway',
	dns: 'Names', internet: 'Internet', online: 'Online', end: 'Left', heard: 'Heard', signal: 'Signal', done: 'Outcome', said: 'Said', steer: 'Steering',
};
// The stages that only say how strongly a client is heard: many, and dull
// beside the rest.
const HEARD = new Set(['heard', 'signal']);
// How many are drawn, the latest: the rest are kept by the manager.
const SHOWN = 400;

const clock = (t) => {
	const d = new Date(t);
	return `${d.toLocaleTimeString([], { hour12: false })}.${String(d.getMilliseconds()).padStart(3, '0')}`;
};
const hhmm = (t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
// How long after the step before: in ms while that says it, then seconds.
const gap = (ms) => ms < 0 ? '' : ms < 1000 ? `+${Math.round(ms)} ms` : ms < 60000 ? `+${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)} s` : `+${Math.round(ms / 60000)} min`;
// A join is timed from its authentication, as a stopwatch (Griff, 2026-10-15:
// "start authenticated as 000"): 0.000, 0.087, 1.250, 1:15.400. Another
// authentication begins another join where it is on another AP, or more
// than JOIN_MS after the one timed from; where the client leaves, the join
// ends.
const JOIN_MS = 30000;
const stopwatch = (ms) => ms < 60000 ? (ms / 1000).toFixed(3) : `${Math.floor(ms / 60000)}:${((ms % 60000) / 1000).toFixed(3).padStart(6, '0')}`;
const middle = (list) => {
	const s = [...list].sort((a, b) => a - b);
	return s.length ? s[Math.floor(s.length / 2)] : null;
};

// traceWindow traces a client in a window that floats over the page (0149):
// it is watched while the page under it is worked on, as while the client is
// put off and on again from the list, and nothing in the page moves to make
// room for it.
export function traceWindow(mac, name, at) {
	tracePanel(floatWindow('trace', 'Live Trace', { width: 1000, height: 560 }), mac, name, at);
}

// tracePanel traces a client in box: name is what it is called, where it
// gave a name; at, the AP it was on where the trace was opened ({ap,
// ap_name}), which Reconnect asks until the trace itself says where it is.
export function tracePanel(box, mac, name, at) {
	let next = 0, top = 0, events = [], state = null, said = null, signals = true, busy = false, drawing = false, held = true, begun = false;
	// What was last asked of an AP from here, and when: said for a while.
	let asked = null;
	const stop = new AbortController();
	// Drawn into the panel itself: its heading is styled as a panel's own.
	// In a window of its own it holds no page from drawing again; in the
	// page it does, as the page drawn again would take it away.
	const panel = h('section', { class: 'panel trace', 'data-editing': floating(box) ? null : true });
	const body = panel;
	box.replaceChildren(panel);
	const path = `/v1/clients/${encodeURIComponent(mac)}/trace`;

	const draw = () => {
		drawing = false;
		const on = state?.tracing;
		const act = (label, fn) => h('button', { type: 'button', class: 'button small', disabled: busy, onclick: fn }, label);
		// Where it is now: the latest each AP said of how it hears it.
		const where = new Map();
		for (const e of events) if (HEARD.has(e.stage)) where.set(e.ap, e);
		const here = [...where.values()].filter((e) => e.signal != null).sort((a, b) => (b.stage === 'signal') - (a.stage === 'signal') || b.signal - a.signal);
		const shown = events.filter((e) => signals || !HEARD.has(e.stage)).slice(-SHOWN);
		// The AP it is on, which Reconnect asks (0107): the one that last said
		// how it hears it as one of its own clients.
		const onAP = here.find((e) => e.stage === 'signal') || (at?.ap ? at : null);
		// Put off its AP to be watched joining again: what a trace is most
		// often opened for.
		const reconnect = h('button', {
			type: 'button', class: 'button small', disabled: !onAP || busy,
			title: onAP ? `Have ${onAP.ap_name || onAP.ap} disconnect it, so it joins again and is seen doing so` : 'It is on no AP that says so here: there is none to ask',
			onclick: async () => {
				try {
					await post(`/v1/aps/${encodeURIComponent(onAP.ap)}/actions`, { kind: 'disconnect', target: mac });
					asked = { text: `Reconnect asked of ${onAP.ap_name || onAP.ap}`, until: Date.now() + 15000 };
					said = null;
				} catch (e) {
					said = e.message;
				}
				draw();
			},
		}, 'Reconnect');
		// The APs that hold their line hear of a trace at once, and say what
		// they see as they see it; the rest at their next poll.
		const live = state?.live || [], aps = state?.aps ?? 0;
		const clocks = live.map((l) => `${l.ap_name || l.ap}${l.offset_ms != null ? `: its clock ${Math.abs(l.offset_ms)} ms ${l.offset_ms < 0 ? 'ahead of' : 'behind'} the manager's, reckoned over ${l.rtt_ms} ms` : ''}`);
		const lag = middle(events.slice(-40).filter((e) => e.lag_ms != null).map((e) => e.lag_ms));
		// The log stays at its end while it is there, as a terminal does.
		const old = body.querySelector('.tracelog');
		const atEnd = !old || old.scrollHeight - old.scrollTop - old.clientHeight < 40;
		let last = null, before = null, join = null;
		// What is not there is left out: the page's own replaceChildren would
		// write "null" and "false" where h() passes them over.
		body.replaceChildren(...[
			h('h2', { class: 'withcontrols' }, h('span', { class: 'title' }, `Live Trace: ${name || mac}`, name && ' ', name && h('span', { class: 'note mono' }, mac)),
				h('span', { class: 'controls' },
					asked && Date.now() < asked.until && h('span', { class: 'note' }, asked.text),
					reconnect,
					h('button', { type: 'button', class: 'button small', title: 'The names this client looks up, and the domains it asks for most', onclick: () => lookupsWindow(mac, name) }, 'Lookups'),
					h('button', { type: 'button', class: 'button small', title: 'Its way out: its AP, the switches, its gateway, and on to the internet', onclick: () => pathWindow(mac, name) }, 'Path'),
					on && h('span', { class: 'note' }, `until ${hhmm(state.until)}${state.by ? `, asked by ${state.by}` : ''}`),
					on ? act('15 minutes more', () => ask(() => post(`${path}?minutes=15`, {}))) : act('Trace for 15 minutes', () => ask(() => post(`${path}?minutes=15`, {}))),
					on && act('Stop', () => ask(() => del(path))),
					h('button', { type: 'button', class: 'button small', onclick: () => { stop.abort(); box.replaceChildren(); } }, 'Close'))),
			said && h('div', { class: 'error' }, said),
			!on && !events.length && h('p', { class: 'sub pad' }, busy || !begun ? 'Asking the APs to trace it…' : 'Not traced now. Traced, every AP says each thing it sees of this client as it happens, whether it is on or not.'),
			on && !events.length && h('p', { class: 'sub pad' }, live.length ? 'Asked. Nothing has been seen of it yet.' : 'Asked. No AP holds its line to the manager, so they take it up at their next poll, which can be a minute.'),
			here.length > 0 && h('div', { class: 'tracewhere' }, here.map((e) => h('span', {
				class: `chip ${e.stage === 'signal' ? 'ok' : 'idle'}`, title: `${e.what}, at ${clock(e.at)}`,
			}, `${e.ap_name || e.ap} · ${e.stage === 'signal' ? 'on' : 'hears it'}${e.band ? ` · ${bandName(e.band)}` : ''} · ${e.signal} dBm`))),
			events.length > 0 && h('div', { class: 'tracelog' }, h('table', { class: 'list' },
				h('tr', null, ['Time', 'Since auth', 'Gap', 'AP', 'Network', 'Band', 'Stage', 'What', 'Signal'].map((t) => h('th', null, t))),
				shown.map((e) => {
					// A line where the AP changes is marked: the client has moved.
					const step = !HEARD.has(e.stage);
					const moved = step && last != null && last !== e.ap;
					const at = new Date(e.at).getTime();
					const since = step && before != null ? gap(at - before) : '';
					if (step) { last = e.ap; before = at; }
					if (step && e.stage === 'auth' && (!join || join.ap !== e.ap || at - join.at > JOIN_MS)) join = { ap: e.ap, at };
					const into = step && join ? stopwatch(at - join.at) : '';
					if (step && e.stage === 'end') join = null;
					return h('tr', { class: [step ? '' : 'heardrow', moved ? 'movedrow' : '', e.ok === false ? 'failed' : ''].filter(Boolean).join(' ') },
						h('td', { class: 'mono nowrap', title: e.lag_ms != null ? `The manager had it ${e.lag_ms} ms after` : '' }, clock(e.at)),
						h('td', { class: 'mono nowrap' }, into),
						h('td', { class: 'mono nowrap' }, since),
						h('td', { class: 'nowrap' }, e.ap_name || e.ap),
						h('td', null, e.network || '—'),
						h('td', { class: 'nowrap' }, bandName(e.band) || '—'),
						h('td', { class: 'nowrap' }, STAGE[e.stage] || e.stage),
						h('td', null, e.what, e.ok === true && h('span', { class: 'sub' }, ' ✓')),
						h('td', { class: 'nowrap' }, e.signal != null ? `${e.signal} dBm` : ''));
				}))),
			(events.length > 0 || on) && h('div', { class: 'tracefoot' },
				events.length > 0 && h('label', null, h('input', { type: 'checkbox', class: 'switch', role: 'switch', checked: signals, 'aria-label': 'Show how strongly it is heard',
					onchange: (e) => { signals = e.target.checked; draw(); } }), 'How strongly it is heard'),
				h('span', { class: 'sub', title: clocks.join('\n') || 'An AP that holds its line to the manager hears of a trace at once' },
					aps ? `${live.length} of ${aps} AP${aps === 1 ? '' : 's'} live` : ''),
				events.length > 0 && h('span', { class: 'sub' }, `${events.length} thing${events.length === 1 ? '' : 's'} seen${events.length > SHOWN ? `, the latest ${SHOWN} shown` : ''}${lag != null ? `, here ${lag < 10 ? `${lag.toFixed(1)} ms` : lag < 1000 ? `${Math.round(lag)} ms` : `${(lag / 1000).toFixed(1)} s`} after` : ''}`),
				!held && h('span', { class: 'sub' }, 'asking every 2 seconds')),
		].filter(Boolean));
		const log = body.querySelector('.tracelog');
		if (log && atEnd) log.scrollTop = log.scrollHeight;
	};
	// Many may come together: they are drawn once, a moment after the first.
	// A timer, since a page that is not in front is given no frames to draw
	// on, and would have it all to do when it came back.
	const soon = () => {
		if (drawing) return;
		drawing = true;
		setTimeout(draw, 25);
	};

	// take is the trace as the manager says it, with what came since the
	// last: on the answer held open, or asked for.
	const take = (v) => {
		if (!panel.isConnected) return stop.abort();
		// A line with nothing in it says only that the answer lives.
		if (!v.mac) return;
		// Traced anew, the manager starts its count again: what was drawn of
		// the last time goes.
		if (state && v.since !== state.since) events = [];
		state = v;
		// Opened, it traces: someone who opens a client's Live Trace means it
		// to begin, and not to be told it has not. Once: a trace that was
		// stopped, or ran out, is begun again by its button.
		if (!begun) {
			begun = true;
			if (!v.tracing) ask(() => post(`${path}?minutes=15`, {}));
		}
		next = Math.max(next, v.next);
		// Each once: an answer taken up again may say some a second time.
		const fresh = v.events.filter((e) => e.seq > top);
		if (fresh.length) {
			top = fresh[fresh.length - 1].seq;
			events.push(...fresh);
			// In the order things happened, by the manager's clock, which
			// every AP's own time is put on: what two APs say comes a batch
			// an AP.
			events.sort((a, b) => new Date(a.at) - new Date(b.at) || a.seq - b.seq);
			if (events.length > 4000) events = events.slice(-4000);
		}
		said = null;
		soon();
	};

	// watch holds the answer open, and takes it up again where it ends. Where
	// it cannot be held, it asks instead, and tries to hold it again.
	const watch = async () => {
		while (panel.isConnected && !stop.signal.aborted) {
			try {
				held = true;
				await lines(`${path}/live?after=${next}`, take, stop.signal);
			} catch (e) {
				held = false;
				if (e.status === 401 || e.status === 403) { said = e.message; soon(); return; }
				try {
					take(await get(`${path}?after=${next}`));
				} catch (e2) {
					said = e2.message;
					soon();
				}
			}
			await new Promise((r) => setTimeout(r, 2000));
		}
	};
	// ask starts, goes on or stops. What the manager answers says how it
	// stands now; what is seen of the client comes as the rest does.
	const ask = async (fn) => {
		busy = true;
		draw();
		try {
			take({ ...await fn(), events: [], next: 0 });
		} catch (e) {
			said = e.message;
		}
		busy = false;
		draw();
	};
	draw();
	watch();
}
