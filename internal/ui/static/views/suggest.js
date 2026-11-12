// Suggest a feature (0323): at the top right of every page, for anyone
// signed in. What they write goes with the page they were on, as its route
// and heading, and the manager's release, shown before it is sent. It is
// kept on this manager first, so it needs no internet, and sent on where
// the manager is told to send them; who asked stays here.

import { h } from '../dom.js';
import { get, post } from '../api.js';
import { when, ago } from '../format.js';

// here is the page someone is on: its route, and its heading as the page
// shows it, without the buttons that sit in it (Rename…, Move…).
function here() {
	const h1 = document.querySelector('main h1')?.cloneNode(true);
	h1?.querySelectorAll('button, .button, select, input').forEach((el) => el.remove());
	const heading = h1?.textContent?.replace(/\s+/g, ' ').trim() || document.title;
	return { route: location.hash || '#/', title: heading };
}

// suggestButton is the header's Suggest a feature, for ctx's account.
export function suggestButton(ctx) {
	return h('button', { type: 'button', class: 'suggestbtn', title: 'Ask for something Aeolus does not do yet, from this page', onclick: () => open(ctx) }, 'Suggest a feature');
}

// open shows the form, over the page.
function open(ctx) {
	const page = here();
	const to = ctx.who.suggest_to || '';
	const text = h('textarea', { rows: 6, maxlength: 4000, required: true, placeholder: 'What it should do, and what it would let you do' });
	const contact = h('input', { type: 'text', maxlength: 200, autocomplete: 'email', placeholder: 'A name or an email, if you would like an answer' });
	const msg = h('div', { class: 'error' });
	const send = h('button', { type: 'submit', class: 'button primary' }, 'Send');
	const body = h('div', { class: 'suggestbody' });
	const dialog = h('dialog', { class: 'suggest', 'aria-labelledby': 'suggest-title' }, body);
	const close = () => {
		dialog.close();
		dialog.remove();
	};
	dialog.addEventListener('cancel', () => dialog.remove());
	const form = h('form', { class: 'fieldform' },
		h('h2', { id: 'suggest-title' }, 'Suggest a feature'),
		h('label', { class: 'field stack' }, h('span', { class: 'label' }, 'What would you like Aeolus to do?'), text),
		h('label', { class: 'field stack' }, h('span', { class: 'label' }, 'How to reach you (optional)'), contact),
		h('div', { class: 'sentwith' },
			h('div', { class: 'label' }, 'Sent with it'),
			h('dl', null,
				h('dt', null, 'Page'), h('dd', null, page.title, ' ', h('span', { class: 'mono' }, page.route)),
				h('dt', null, 'Release'), h('dd', { class: 'mono' }, ctx.who.version || 'unknown'))),
		h('div', { class: 'sub' }, to
			? `It is kept on this manager, then sent on to ${to} when the manager can reach it. Your account stays here.`
			: 'It is kept on this manager, where its admins read it under Admin › Suggestions.'),
		msg,
		h('div', { class: 'actions' }, send, h('button', { type: 'button', class: 'button', onclick: close }, 'Cancel')));
	form.addEventListener('submit', async (e) => {
		e.preventDefault();
		msg.replaceChildren();
		if (!text.value.trim()) {
			msg.replaceChildren('Say what you would like Aeolus to do.');
			return;
		}
		send.disabled = true;
		try {
			const res = await post('/v1/suggestions', { text: text.value, contact: contact.value, route: page.route, title: page.title });
			body.replaceChildren(h('div', { class: 'fieldform' },
				h('h2', { id: 'suggest-title' }, 'Thank you'),
				h('p', null, `Kept as suggestion #${res.suggestion.id}.`, res.to ? ` It goes on to ${res.to} as soon as the manager can reach it.` : ''),
				h('div', { class: 'actions' }, h('button', { type: 'button', class: 'button primary', onclick: close }, 'Close'))));
		} catch (err) {
			send.disabled = false;
			msg.replaceChildren(err.message);
		}
	});
	body.append(form);
	document.body.append(dialog);
	dialog.showModal();
	text.focus();
}

// suggestionsPanel is Admin › Suggestions: what people asked for, newest
// first, and whether each has gone on.
export async function suggestionsPanel() {
	const res = await get('/v1/suggestions').catch((e) => ({ error: e.message }));
	if (res.error) return h('section', { class: 'panel' }, h('h2', null, 'Suggestions'), h('div', { class: 'banner info' }, res.error));
	const list = res.suggestions || [];
	const state = (x) => (x.sent_at
		? h('span', { class: 'chip ok', title: `Sent ${when(x.sent_at)}` }, `sent to ${x.sent_to}`)
		: res.to
			? h('span', { class: 'chip warn', title: x.last_error || 'Not tried yet' }, x.tries ? `not sent yet: ${x.tries} tr${x.tries === 1 ? 'y' : 'ies'}` : 'waiting to be sent')
			: h('span', { class: 'chip idle' }, 'kept here'));
	return h('section', { class: 'panel' },
		h('h2', null, 'Suggestions', h('span', { class: 'note' }, res.to ? `sent on to ${res.to}` : 'kept on this manager: AEOLUS_SUGGEST_TO sends them on')),
		list.length === 0
			? h('div', { class: 'banner info' }, 'No one has suggested anything yet.')
			: h('table', { class: 'list' },
				h('thead', null, h('tr', null, h('th', null, 'When'), h('th', null, 'From'), h('th', null, 'Suggestion'), h('th', null, 'Page'), h('th', null, ''))),
				h('tbody', null, list.map((x) => h('tr', null,
					h('td', { title: when(x.at) }, ago(x.at)),
					h('td', null, x.account, x.contact ? h('div', { class: 'sub' }, x.contact) : null),
					h('td', { class: 'suggesttext' }, x.text),
					// Only a route of this manager's own is a link (the server keeps no other).
					h('td', null, x.route?.startsWith('#/') ? h('a', { href: x.route }, x.title || x.route) : (x.title || ''), x.version ? h('div', { class: 'sub mono' }, x.version) : null),
					h('td', null, state(x)))))));
}
