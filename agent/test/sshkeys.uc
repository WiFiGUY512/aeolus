// The agent's SSH keys (0197): what authorized_keys holds after each merge,
// and which keys Aeolus keeps as its own. sshkeys.out is what it must print.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/sshkeys.uc

'use strict';

import { merge, id } from 'aeolus.sshkeys';

const HAND = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHand1 griff@laptop';
const OTHER = 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQOther porting';
const MANAGER = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIManager aeolus-manager';
const SET = 'ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYSet claude';

function show(what, r) {
	print(`-- ${what}\n${r.text}managed: ${join(', ', map(r.managed, k => split(k, ' ')[2] ?? '?'))}\n`);
}

// A fresh AP with keys put there by hand: they stay, Aeolus's come after.
let r = merge(HAND + '\n' + OTHER + '\n', [], [MANAGER, SET]);
show('first time, two keys by hand', r);

// The same again changes nothing.
let again = merge(r.text, r.managed, [MANAGER, SET]);
print(`-- the same again is the same: ${again.text == r.text}\n`);

// A key unset in Aeolus goes; the hand keys and the manager's stay.
show('one key unset', merge(r.text, r.managed, [MANAGER]));

// A key set in Aeolus that is already there by hand is not written twice,
// and is not Aeolus's to take away later.
let dup = merge(HAND + '\n', [], [MANAGER, 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHand1 another-comment']);
show('a key already there by hand', dup);
show('then unset in Aeolus: the hand key stays', merge(dup.text, dup.managed, [MANAGER]));

// No file yet, and lines that are not keys, which are kept as they are.
show('no file', merge(null, [], [MANAGER]));
show('an option line kept', merge('no-pty ' + OTHER + '\n# a comment\n', [], [MANAGER]));

// Nothing from Aeolus any more: only its own go.
show('none from Aeolus', merge(r.text, r.managed, []));

// Not keys are not written.
show('not a key', merge('', [], ['ssh-dss AAAA x', 'hello', MANAGER]));
print(`-- id ignores the comment: ${id(HAND) == id('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHand1')}\n`);
