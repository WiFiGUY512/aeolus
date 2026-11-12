// The SSH keys Aeolus puts on an AP (0197): those set in system.ssh_keys,
// and the manager's own, in dropbear's authorized_keys beside any put there
// by hand. Aeolus keeps a list of the ones it wrote, so it takes away only
// its own: a key set by hand stays, and so no one is locked out by Aeolus.

'use strict';

// The shape of a key line: its type and its key, and a comment or none.
const KEY = /^(ssh-ed25519|ecdsa-sha2-nistp256|ssh-rsa) ([A-Za-z0-9+\/=]+)( .*)?$/;

// id is a key's type and key, without its comment: two lines of one key
// with different comments are the same key.
function id(line) {
	let m = match(trim(line ?? ''), KEY);
	return m ? m[1] + ' ' + m[2] : null;
}

// merge says what authorized_keys is to hold: text is what it holds now,
// or null; before, the keys Aeolus wrote last time; keys, the ones it is to
// have now. Every line that is not one of Aeolus's keys from before is kept
// as it is and where it is; Aeolus's keys come after them, each once and
// none that a kept line has already. It returns the text and the keys
// Aeolus now has there, to keep for next time.
function merge(text, before, keys) {
	let mine = {};
	for (let k in before ?? [])
		if (id(k))
			mine[id(k)] = true;
	let kept = [], have = {};
	for (let line in split(text ?? '', '\n')) {
		let k = id(line);
		if (k && mine[k])
			continue;
		if (trim(line) == '')
			continue;
		push(kept, line);
		if (k)
			have[k] = true;
	}
	let managed = [];
	for (let key in keys ?? []) {
		let k = id(key);
		if (!k || have[k])
			continue;
		have[k] = true;
		push(kept, trim(key));
		push(managed, trim(key));
	}
	return { text: length(kept) ? join('\n', kept) + '\n' : '', managed: managed };
}

export { id, merge };
