// Signing in (0176): an account and its password, the manager keeping the
// session in a cookie the page never sees; or an API token pasted in, as
// before (0042). A temporary password is changed before anything else.

import { h } from '../dom.js';
import { signIn, signOut, get, signInWithPassword, changePassword } from '../api.js';

const MIN = 12;

export function signinPage(done) {
	const box = h('div', { class: 'signin-box' });
	const show = (form) => box.replaceChildren(form);
	show(passwordForm(done, show));
	return box;
}

// passwordForm is the usual way in.
function passwordForm(done, show) {
	const account = h('input', { type: 'text', autocomplete: 'username', spellcheck: 'false', autocapitalize: 'none', 'aria-label': 'Account', placeholder: 'account' });
	const password = h('input', { type: 'password', autocomplete: 'current-password', 'aria-label': 'Password', placeholder: 'password' });
	const keep = h('input', { type: 'checkbox' });
	const error = h('div', { class: 'error', hidden: true });
	const submit = async (e) => {
		e.preventDefault();
		error.hidden = true;
		try {
			const who = await signInWithPassword(account.value.trim(), password.value, keep.checked);
			if (who.must_change) {
				show(passwordPage(done, { forced: true, current: password.value }));
				return;
			}
			done();
		} catch (err) {
			if (err.status === 404) {
				show(tokenForm(done, show, 'This manager signs in with API tokens only.'));
				return;
			}
			error.textContent = err.status === 401 ? 'Wrong account or password.' : err.message;
			error.hidden = false;
			password.value = '';
			password.focus();
		}
	};
	setTimeout(() => account.focus(), 0);
	return h('form', { class: 'signin', onsubmit: submit },
		h('h1', null, 'Sign in to Aeolus'),
		h('p', null, 'Every change you make is logged under your name.'),
		account,
		password,
		h('label', null, keep, 'Keep me signed in on this browser'),
		error,
		h('button', { type: 'submit' }, 'Sign in'),
		h('p', { class: 'sub' },
			h('a', { href: '#/signin', onclick: (e) => { e.preventDefault(); show(tokenForm(done, show)); } }, 'Sign in with an API token instead')));
}

// tokenForm signs in with a pasted API token, kept for this tab or on this
// browser (0042).
function tokenForm(done, show, note) {
	const input = h('input', { type: 'password', autocomplete: 'off', spellcheck: 'false', placeholder: 'aeolus1.…', 'aria-label': 'API token' });
	const remember = h('input', { type: 'checkbox' });
	const error = h('div', { class: 'error', hidden: true });
	const submit = async (e) => {
		e.preventDefault();
		error.hidden = true;
		const t = input.value.trim();
		if (!t.startsWith('aeolus1.')) {
			error.textContent = 'That is not an Aeolus account token. It starts with aeolus1.';
			error.hidden = false;
			return;
		}
		try {
			signIn(t, remember.checked);
			await get('/v1/whoami');
			done();
		} catch (err) {
			signOut();
			error.textContent = err.status === 401 ? 'The manager does not know that token, or it was revoked.' : err.message;
			error.hidden = false;
		}
	};
	return h('form', { class: 'signin', onsubmit: submit },
		h('h1', null, 'Sign in to Aeolus'),
		h('p', null, note || 'Paste your own API token. Every change you make is logged under your name.'),
		input,
		h('label', null, remember, 'Keep me signed in on this browser'),
		error,
		h('button', { type: 'submit' }, 'Sign in'),
		!note && h('p', { class: 'sub' },
			h('a', { href: '#/signin', onclick: (e) => { e.preventDefault(); show(passwordForm(done, show)); } }, 'Sign in with a password instead')));
}

// passwordPage changes the signed-in person's own password. forced: it is
// temporary, set by someone else, and nothing else may be done first;
// current, when it was just typed to sign in. hasPassword false: there is
// none yet, so none is asked for.
export function passwordPage(done, { forced = false, current = null, hasPassword = true } = {}) {
	const now = h('input', { type: 'password', autocomplete: 'current-password', 'aria-label': 'Current password', placeholder: forced ? 'the temporary password' : 'current password' });
	const next = h('input', { type: 'password', autocomplete: 'new-password', 'aria-label': 'New password', placeholder: 'new password' });
	const again = h('input', { type: 'password', autocomplete: 'new-password', 'aria-label': 'New password again', placeholder: 'new password again' });
	const error = h('div', { class: 'error', hidden: true });
	const asks = hasPassword && current == null;
	const submit = async (e) => {
		e.preventDefault();
		error.hidden = true;
		const say = (m) => { error.textContent = m; error.hidden = false; };
		if (next.value.length < MIN) return say(`A password is at least ${MIN} characters.`);
		if (next.value !== again.value) return say('The two new passwords differ.');
		try {
			await changePassword(current ?? now.value, next.value);
			done();
		} catch (err) {
			say(err.message);
		}
	};
	setTimeout(() => (asks ? now : next).focus(), 0);
	return h('form', { class: 'signin', onsubmit: submit },
		h('h1', null, forced ? 'Choose your own password' : hasPassword ? 'Change your password' : 'Set a password'),
		h('p', null, forced
			? 'You signed in with a temporary password. Choose one of your own before going on.'
			: `At least ${MIN} characters; length is what counts. Your other sessions end; this one goes on.`),
		asks && now,
		next,
		again,
		error,
		h('button', { type: 'submit' }, forced ? 'Set it and go on' : 'Change it'),
		forced && h('p', { class: 'sub' },
			h('a', { href: '#/signin', onclick: (e) => { e.preventDefault(); signOut(); location.hash = '#/signin'; } }, 'Sign out instead')));
}
