// Joining by an invite (0336): the page an invite's link opens, for one with
// no account yet. It says who sent the invite and what it gives; they choose
// an account and a password of their own, and are signed in.

import { h } from '../dom.js';
import { inviteOf, register } from '../api.js';
import { when } from '../format.js';

const TREE = { locations: 'Locations', services: 'Services' };

export function registerPage(code, done) {
	const box = h('div', { class: 'signin-box' });
	box.append(h('div', { class: 'signin' }, h('p', null, 'Reading the invite…')));
	inviteOf(code).then(
		(iv) => box.replaceChildren(joinForm(code, iv, done)),
		(err) => box.replaceChildren(h('div', { class: 'signin' },
			h('h1', null, 'This invite can’t be used'),
			h('p', null, err.message),
			h('p', { class: 'sub' }, h('a', { href: '#/signin' }, 'Sign in with an account you have')))));
	return box;
}

// gives is what an invite makes its holder, a line each.
function gives(iv) {
	const out = iv.roles.map((r) => `${r.role} on ${r.where}, in ${TREE[r.tree] || r.tree}, and everything below it`);
	if (iv.owner) out.unshift('a super admin: everything, everywhere, and the Org’s people');
	return out;
}

function joinForm(code, iv, done) {
	const account = h('input', { type: 'text', autocomplete: 'username', spellcheck: 'false', autocapitalize: 'none', maxlength: 32, 'aria-label': 'Account', placeholder: 'your account: jsmith' });
	const name = h('input', { type: 'text', autocomplete: 'name', maxlength: 64, 'aria-label': 'Name', placeholder: 'your name: Jane Smith' });
	const password = h('input', { type: 'password', autocomplete: 'new-password', 'aria-label': 'Password', placeholder: 'password' });
	const again = h('input', { type: 'password', autocomplete: 'new-password', 'aria-label': 'Password again', placeholder: 'password again' });
	const keep = h('input', { type: 'checkbox' });
	const error = h('div', { class: 'error', hidden: true });
	const min = iv.password_min || 12;
	const submit = async (e) => {
		e.preventDefault();
		error.hidden = true;
		const say = (m) => { error.textContent = m; error.hidden = false; };
		const id = account.value.trim().toLowerCase();
		if (!/^[a-z][a-z0-9._-]{1,31}$/.test(id)) return say('An account is 2 to 32 characters: lower case letters, digits, dots, underscores or hyphens, starting with a letter.');
		if (password.value.length < min) return say(`A password is at least ${min} characters.`);
		if (password.value !== again.value) return say('The two passwords differ.');
		try {
			await register({ code, account: id, name: name.value.trim(), password: password.value, keep: keep.checked });
			done();
		} catch (err) {
			say(err.message);
			if (err.status === 409) account.focus();
		}
	};
	setTimeout(() => account.focus(), 0);
	return h('form', { class: 'signin', onsubmit: submit },
		h('h1', null, `Join ${iv.org || 'Aeolus'}`),
		h('p', null, `${iv.by} invited you to this Aeolus manager. You will be:`),
		h('ul', { class: 'gives' }, gives(iv).map((g) => h('li', null, g))),
		iv.note && h('p', { class: 'sub' }, `Their note: ${iv.note}`),
		h('p', { class: 'sub' }, `The invite works once, until ${when(iv.expires)}. Choose an account to sign in with and a password of at least ${min} characters; every change you make is logged under your account.`),
		account,
		name,
		password,
		again,
		h('label', null, keep, 'Keep me signed in on this browser'),
		error,
		h('button', { type: 'submit' }, 'Join and sign in'));
}
