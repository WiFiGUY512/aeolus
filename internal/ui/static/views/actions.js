// An AP's actions (0104): Locate, Restart Wi-Fi and Reboot, asked of it
// once each, which it takes up on its next poll, within about a minute; and
// what came of the latest ones, a client's Reconnect among them (0107).

import { h } from '../dom.js';
import { get, post } from '../api.js';
import { ago } from '../format.js';
import { flash } from '../refresh.js';

const KINDS = [
	['locate', 'Locate', 'Blink every LED on it for a minute, to find it.', null],
	['restart-wifi', 'Restart Wi-Fi', 'Restart its Wi-Fi: every client on it drops, and joins again in a few seconds.', 'Its clients drop for a few seconds.'],
	['reboot', 'Reboot', 'Reboot it: it is off the air, and its wired clients cut off, for about two minutes.', 'It is off the air for about two minutes.'],
];
const STATE = { pending: ['', 'waiting for the AP'], running: ['', 'taken up by the AP'], done: ['ok', 'done'], failed: ['bad', 'failed'], expired: ['warn', 'expired: the AP did not take it up'] };

// askIn asks in box whether the AP id, called name, is to do kind: label,
// what it costs (warn), and the buttons to do it or not.
function askIn(box, id, name, kind, label, warn) {
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('div', null, h('strong', null, `${label} ${name}?`)),
		warn && h('div', { class: 'sub warn' }, warn),
		h('div', { class: 'sub' }, 'It does it on its next poll, within about a minute.'),
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: `button ${kind === 'reboot' ? 'danger' : 'primary'}`, onclick: async (e) => {
				e.currentTarget.disabled = true;
				try {
					await post(`/v1/aps/${encodeURIComponent(id)}/actions`, { kind });
					box.replaceChildren();
					flash(`${label}: asked of ${name}; it acts on its next poll.`);
				} catch (err) {
					box.replaceChildren(h('div', { class: 'error' }, err.message));
				}
			} }, label),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
}

// rebootButton asks an AP to reboot from its page's heading, beside Rename
// and Move, on every tab (Griff, 2026-10-15: "need an AP reboot button
// here"): asked first, as from its Actions.
export function rebootButton(id, name, box, cls = 'rename head') {
	const [kind, label, title, warn] = KINDS.find(([k]) => k === 'reboot');
	return h('button', { type: 'button', class: `button small ${cls}`, title, onclick: () => askIn(box, id, name, kind, label, warn) }, `${label}…`);
}

// terminalButton opens a terminal on the AP (0198), a shell there as root,
// in a window of its own, one an AP: an admin's, beside Reboot.
export function terminalButton(id, name, cls = 'rename head') {
	const url = `/ui/terminal.html#ap=${encodeURIComponent(id)}&name=${encodeURIComponent(name)}`;
	return h('button', { type: 'button', class: `button small ${cls}`, title: 'A shell on this AP as root, in a window of its own',
		onclick: () => window.open(url, `aeolus-terminal-${id}`, 'width=920,height=580') }, 'Terminal ↗');
}

// actionButtons are Locate and Restart Wi-Fi, for the AP's heading beside
// Reboot… (0284): one that costs its clients something is asked first.
export function actionButtons(id, name, box, cls = 'rename head') {
	return KINDS.filter(([k]) => k !== 'reboot').map(([kind, label, title, warn]) => h('button', {
		type: 'button', class: `button small ${cls}`, title,
		onclick: () => (warn ? askIn(box, id, name, kind, label, warn) : post(`/v1/aps/${encodeURIComponent(id)}/actions`, { kind })
			.then(() => flash(`${label}: asked of ${name}; it acts on its next poll.`)).catch((err) => box.replaceChildren(h('div', { class: 'error' }, err.message)))),
	}, warn ? `${label}…` : label));
}

// actionsPanel is what came of the latest actions asked of the AP, its
// client tests apart; nothing where none was asked. They are asked from its
// heading (0284).
export async function actionsPanel(id) {
	let list = [];
	try {
		list = (await get(`/v1/aps/${encodeURIComponent(id)}/actions`)).actions || [];
	} catch { /* an older manager has none */ }
	// Its client tests are under Assurance, each with what it found: here,
	// the others, the latest few, a line each.
	const others = list.filter((a) => a.kind !== 'client-test').slice(0, 4);
	if (!others.length) return null;
	return h('section', { class: 'panel' },
		h('h2', { title: 'What it was asked to do lately. It does it on its next poll, within about a minute.' }, 'Asked of it lately'),
		// What came of one is said where it did not go well; of one that did, on hover.
		h('table', { class: 'list wraps' },
			others.map((a) => h('tr', { title: a.result || null },
				h('td', null, a.kind === 'disconnect' ? ['Reconnect ', h('span', { class: 'mono' }, a.target)]
					// One of its own networks, not Aeolus's, taken out (0137).
					: a.kind === 'remove-network' ? `Take out its own network ${a.params?.ssid ? `“${a.params.ssid}”` : 'with no name'}`
						: KINDS.find(([k]) => k === a.kind)?.[1] ?? a.kind),
				h('td', { class: 'sub' }, `${ago(a.at)} · ${a.actor}`),
				h('td', null, h('span', { class: `chip ${STATE[a.state]?.[0] ?? ''}` }, STATE[a.state]?.[1] ?? a.state),
					a.result && a.state !== 'done' && h('span', { class: 'sub' }, ` ${a.result}`))))));
}
