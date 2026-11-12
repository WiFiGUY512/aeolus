// A terminal on an AP (0198), in a window of its own: the AP's ID and name
// come in the address, #ap=<id>&name=<name>. What the shell prints comes on
// one answer that does not end; what is typed goes up as it is typed, a few
// keys together where they came together.
import { requestHeaders } from './api.js';

const params = new URLSearchParams(location.hash.slice(1));
const ap = params.get('ap');
const name = params.get('name') || ap;
const stateEl = document.getElementById('state');
document.getElementById('title').textContent = `Terminal · ${name}`;
document.title = `${name} · Terminal · Aeolus`;

const say = (text, ended) => {
	stateEl.textContent = text;
	stateEl.classList.toggle('ended', Boolean(ended));
};

/* global Terminal, FitAddon */
const term = new Terminal({ cursorBlink: true, fontSize: 13, scrollback: 5000, fontFamily: 'ui-monospace, Menlo, Consolas, monospace' });
const fit = new FitAddon.FitAddon();
term.loadAddon(fit);
term.open(document.getElementById('term'));
fit.fit();

let id = null;
let ended = false;

// post sends to the terminal; an error ends it.
async function post(path, body, type) {
	const res = await fetch(`/v1/terminal/${id}${path}`, {
		method: 'POST',
		headers: requestHeaders({ 'Content-Type': type }),
		body,
	});
	if (!res.ok && res.status !== 204) throw new Error(`${res.status}`);
}

// What is typed goes up in order, gathered while one is on its way.
let typed = '';
let sending = false;
async function send() {
	if (sending || !typed || ended) return;
	sending = true;
	const now = typed;
	typed = '';
	try {
		await post('/in', new TextEncoder().encode(now), 'application/octet-stream');
	} catch {
		// The end comes on the answer that does not end.
	}
	sending = false;
	send();
}
term.onData((d) => { typed += d; send(); });
term.onBinary((d) => { typed += d; send(); });

let sized = null;
function resize() {
	fit.fit();
	if (!id || ended) return;
	clearTimeout(sized);
	sized = setTimeout(() => post('/size', JSON.stringify({ cols: term.cols, rows: term.rows }), 'application/json').catch(() => {}), 150);
}
addEventListener('resize', resize);

async function close() {
	if (id && !ended) {
		await fetch(`/v1/terminal/${id}`, { method: 'DELETE', headers: requestHeaders(), keepalive: true }).catch(() => {});
	}
}
document.getElementById('close').addEventListener('click', async () => { await close(); window.close(); });
addEventListener('pagehide', () => { close(); });

async function open() {
	if (!ap) {
		say('No AP named in the address.', true);
		return;
	}
	let res;
	try {
		res = await fetch(`/v1/aps/${encodeURIComponent(ap)}/terminal`, {
			method: 'POST',
			headers: requestHeaders({ 'Content-Type': 'application/json' }),
			body: JSON.stringify({ cols: term.cols, rows: term.rows }),
		});
	} catch {
		say('The manager cannot be reached.', true);
		return;
	}
	const body = await res.json().catch(() => ({}));
	if (!res.ok) {
		say(body.error || `The manager answered ${res.status}.`, true);
		return;
	}
	id = body.id;
	say('connected: a shell on the AP as root');
	term.focus();
	const out = await fetch(`/v1/terminal/${id}`, { headers: requestHeaders() });
	if (!out.ok || !out.body) {
		say(`The shell could not be read (${out.status}).`, true);
		return;
	}
	const reader = out.body.getReader();
	for (;;) {
		let r;
		try {
			r = await reader.read();
		} catch {
			break;
		}
		if (r.done) break;
		term.write(r.value);
	}
	ended = true;
	say('ended: close this window, or open a terminal again from the AP\'s page', true);
}

open();
