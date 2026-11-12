// A test network (0179): a network with test set, which an installer, or
// anyone with the test SSID right, switches on here for at most four hours.
// The APs switch it off by themselves when its time is up, the manager
// reached or not.

import { h } from '../dom.js';
import { post } from '../api.js';
import { flash, hurry, redrawNow } from '../refresh.js';
import { mayHere } from '../rights.js';

const FOR = [[30, '30 minutes'], [60, '1 hour'], [120, '2 hours'], [240, '4 hours']];

const hhmm = (t) => new Date(t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

function left(until) {
	const m = Math.max(0, Math.round((until * 1000 - Date.now()) / 60000));
	return m >= 60 ? `${Math.floor(m / 60)} h ${m % 60} min left` : `${m} min left`;
}

// testNetworkPanel is the Test network panel on a Locations folder's or
// AP's page: what is on here and until when, or a network and a time to
// switch one on for. Null for whoever may not, or where no folder offers a
// test network and none is on.
export function testNetworkPanel(ctx, id, page) {
	if (!mayHere(page, 'test-ssid')) return null;
	// The manager lists them here: one who may switch one on may not see the
	// Services folders that offer them.
	const nets = page.test_networks || [];
	const f = page.fields || {};
	const on = f['test_network.network']?.value, until = f['test_network.until']?.value;
	const from = f['test_network.network']?.from;
	const live = on && typeof until === 'number' && until * 1000 > Date.now();
	if (!nets.length && !live) return null;
	const box = h('div', { class: 'edit' });
	const send = async (op, note, said) => {
		try {
			const res = await post('/v1/changes', { op, reason: note });
			hurry(150);
			flash(`${said} Saved as change #${res.change.seq}.`);
			redrawNow();
		} catch (e) {
			box.replaceChildren(h('div', { class: 'error' }, e.message));
		}
	};
	let body;
	if (live) {
		const ssid = nets.find((n) => n.id === on)?.ssid || on;
		const where = from && from !== id ? ` (switched on at ${ctx.name('locations', from)})` : '';
		body = h('div', { class: 'row' },
			h('span', { class: 'chip warn' }, 'on'),
			h('span', null, h('strong', null, ssid), ` until ${hhmm(until)}, ${left(until)}${where}.`),
			h('span', { class: 'controls' }, h('button', { type: 'button', class: 'button small', onclick: () => send(
				{ kind: 'unset', tree: 'locations', node: from || id, paths: ['test_network.network', 'test_network.until'] },
				'test network switched off', `${ssid} goes off within a minute.`) }, 'Switch off')));
	} else {
		const which = h('select', { 'aria-label': 'Test network' }, nets.map((n) => h('option', { value: n.id }, n.ssid)));
		const dur = h('select', { 'aria-label': 'For how long' }, FOR.map(([m, label]) => h('option', { value: String(m), selected: m === 60 }, label)));
		body = h('div', { class: 'row' },
			h('span', { class: 'chip idle' }, 'off'),
			which, ' for ', dur,
			h('span', { class: 'controls' }, h('button', { type: 'button', class: 'button small primary', onclick: () => {
				const mins = Number(dur.value);
				const end = Math.floor(Date.now() / 1000) + mins * 60;
				const ssid = nets.find((n) => n.id === which.value)?.ssid || which.value;
				send({ kind: 'set', tree: 'locations', node: id, values: { 'test_network.network': which.value, 'test_network.until': end } },
					`test network on for ${mins} minutes`, `${ssid} comes on within a minute, until ${hhmm(end)}.`);
			} }, 'Switch on')));
	}
	return h('section', { class: 'panel' },
		h('h2', null, 'Test network', h('span', { class: 'note' }, 'For checking an install: on for at most four hours, then off by itself, on every AP here.')),
		body, box);
}
