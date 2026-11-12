// Talking to the manager (0042): the same API every client uses. A person
// signs in with a password, and the manager keeps the session in a cookie
// the page never sees (0176); or with their own API token, kept for this
// tab, or on this browser if they asked.

const KEY = 'aeolus.token';

function stores() {
	const out = [];
	try { out.push(sessionStorage); } catch { /* storage blocked */ }
	try { out.push(localStorage); } catch { /* storage blocked */ }
	return out;
}

export function token() {
	for (const s of stores()) {
		const t = s.getItem(KEY);
		if (t) return t;
	}
	return null;
}

export function signIn(t, remember) {
	forget();
	try {
		(remember ? localStorage : sessionStorage).setItem(KEY, t);
	} catch {
		throw new Error('This browser does not let the page keep the token. Allow site data for the manager and try again.');
	}
}

// forget drops a kept token.
function forget() {
	for (const s of stores()) s.removeItem(KEY);
}

// signOut drops a kept token and ends the session, if there is one.
export function signOut() {
	forget();
	fetch('/v1/session', { method: 'DELETE', keepalive: true }).catch(() => {});
}

// headers are what every request carries: the token, where one is kept,
// else nothing, the cookie going by itself; and the header a change made
// with the cookie must carry, which a page from another site can't add
// (0176).
export function requestHeaders(extra) {
	const t = token();
	return { ...(t ? { Authorization: 'Bearer ' + t } : {}), 'X-Aeolus-Session': '1', ...extra };
}

export class APIError extends Error {
	constructor(status, message, body) {
		super(message);
		this.status = status;
		// The manager asks for the password to be changed first (0176).
		this.mustChange = body?.must_change === true;
	}
}

// signInWithPassword starts a session (0176). It answers who signed in,
// and whether their password must be changed first.
export async function signInWithPassword(account, password, keep) {
	forget();
	let res;
	try {
		res = await fetch('/v1/session', {
			method: 'POST',
			headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
			body: JSON.stringify({ account, password, keep }),
		});
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	const out = await res.json().catch(() => ({}));
	if (!res.ok) throw new APIError(res.status, out.error || res.statusText, out);
	return out;
}

// open makes a request that needs no sign-in, as an invite's (0336): the
// code it carries is the credential.
async function open(method, path, body) {
	let res;
	try {
		res = await fetch(path, {
			method,
			headers: { Accept: 'application/json', ...(body ? { 'Content-Type': 'application/json' } : {}) },
			body: body ? JSON.stringify(body) : undefined,
		});
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	const out = await res.json().catch(() => ({}));
	if (!res.ok) throw new APIError(res.status, out.error || res.statusText, out);
	return out;
}

// inviteOf reads what an invite gives, for the one it is for (0336).
export async function inviteOf(code) {
	return open('GET', `/v1/register/${encodeURIComponent(code)}`);
}

// register makes one's own account with an invite, and starts its session
// (0336). Any token kept for the tab is forgotten: the session is the new
// account's.
export async function register(body) {
	forget();
	return open('POST', '/v1/register', body);
}

// changePassword sets the signed-in person's own password (0176).
export async function changePassword(current, password) {
	return write('POST', '/v1/session/password', { current, new: password });
}

// get reads one API path. A 401 means the token is gone or revoked: the
// person is sent back to sign in.
export async function get(path) {
	let res;
	try {
		res = await fetch(path, { headers: requestHeaders({ Accept: 'application/json' }) });
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	const body = await res.json().catch(() => ({}));
	if (res.status === 401) {
		forget();
		throw new APIError(401, body.error || 'signed out');
	}
	if (!res.ok) throw new APIError(res.status, body.error || res.statusText, body);
	return body;
}

// bytes reads a file from the API as it is, for the page to read itself:
// a capture, opened in the packet viewer (0341).
export async function bytes(path) {
	let res;
	try {
		res = await fetch(path, { headers: requestHeaders() });
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		if (res.status === 401) forget();
		throw new APIError(res.status, body.error || res.statusText, body);
	}
	return new Uint8Array(await res.arrayBuffer());
}

// download reads a file from the API and hands it to the browser to keep,
// under the name the manager gives it; it returns that name. Signed in with
// a token, the browser can't fetch the file by a plain link, so it is
// fetched here.
export async function download(path) {
	let res;
	try {
		res = await fetch(path, { headers: requestHeaders() });
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		if (res.status === 401) forget();
		throw new APIError(res.status, body.error || res.statusText, body);
	}
	const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') || '')?.[1] || 'download';
	const url = URL.createObjectURL(await res.blob());
	const a = document.createElement('a');
	a.href = url;
	a.download = name;
	document.body.append(a);
	a.click();
	a.remove();
	setTimeout(() => URL.revokeObjectURL(url), 30000);
	return name;
}

// health reads /healthz, which needs no token.
export async function health() {
	const res = await fetch('/healthz', { headers: { Accept: 'application/json' } });
	return res.json();
}

// post sends one write. A refused change comes back as an APIError carrying
// the manager's reason, which the page shows as it is.
export async function post(path, body) {
	return write('POST', path, body);
}

// put sets what a path names to what is sent, as a client's name (0153). It
// answers as post does.
export async function put(path, body) {
	return write('PUT', path, body);
}

async function write(method, path, body) {
	let res;
	try {
		res = await fetch(path, {
			method,
			headers: requestHeaders({ Accept: 'application/json', 'Content-Type': 'application/json' }),
			body: JSON.stringify(body),
		});
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	const out = await res.json().catch(() => ({}));
	if (res.status === 401) {
		forget();
		throw new APIError(401, out.error || 'signed out');
	}
	if (!res.ok) throw new APIError(res.status, out.error || res.statusText, out);
	return out;
}

// del asks the manager to end or take away what a path names, as stopping
// a client's trace (0144). It answers as post does.
export async function del(path) {
	let res;
	try {
		res = await fetch(path, { method: 'DELETE', headers: requestHeaders({ Accept: 'application/json' }) });
	} catch {
		throw new APIError(0, 'Cannot reach the manager.');
	}
	const out = await res.json().catch(() => ({}));
	if (res.status === 401) {
		forget();
		throw new APIError(401, out.error || 'signed out');
	}
	if (!res.ok) throw new APIError(res.status, out.error || res.statusText, out);
	return out;
}

// lines reads an answer that does not end (0145): each line of it is JSON,
// and is handed to each as it comes. It returns when the manager ends the
// answer, and throws as get does where it cannot be had or breaks off.
// signal, an AbortSignal, ends it from here.
export async function lines(path, each, signal) {
	let res;
	try {
		res = await fetch(path, { headers: requestHeaders({ Accept: 'application/x-ndjson' }), signal });
	} catch {
		if (signal?.aborted) return;
		throw new APIError(0, 'Cannot reach the manager.');
	}
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		if (res.status === 401) forget();
		throw new APIError(res.status, body.error || res.statusText, body);
	}
	const reader = res.body.getReader(), text = new TextDecoder();
	let buf = '';
	for (;;) {
		let part;
		try {
			part = await reader.read();
		} catch {
			if (signal?.aborted) return;
			throw new APIError(0, 'The manager stopped answering.');
		}
		if (part.done) return;
		buf += text.decode(part.value, { stream: true });
		const got = buf.split('\n');
		buf = got.pop();
		for (const l of got) {
			if (!l) continue;
			let v;
			try {
				v = JSON.parse(l);
			} catch {
				continue;
			}
			each(v);
		}
	}
}

let described = null;

// schema reads which fields can be set and what each takes (0048), once a
// page load: it is the same for everyone and changes only with a release.
export function schema() {
	described ??= Promise.all([get('/v1/schema'), get('/v1/agent/versions').catch(() => null)]).then(([d, agents]) => {
		// The agent releases the manager keeps, for the Agent version setting
		// (0079): current, its own, and the ones before it.
		const f = d.fields?.['system.agent'];
		if (f && agents) f.enum = ['current', ...agents.versions.map((v) => v.version)];
		return d;
	}).catch((e) => {
		described = null;
		throw e;
	});
	return described;
}
