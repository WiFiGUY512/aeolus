// The admin section (0176): the Org's people, each account's roles, tokens
// and sessions, the sign-in record, and one's own account. Who sees whom is
// the manager's to say (0177): an owner, the super admin, everyone; an admin
// or a manager, those holding roles where it may give them.

import { h, link } from '../dom.js';
import { get, post, del } from '../api.js';
import { flash, redrawNow } from '../refresh.js';
import { ago, when } from '../format.js';
import { tabBar, pick } from '../layout.js';
import { givesAt } from '../rights.js';
import { suggestionsPanel } from './suggest.js';

const TABS = [['people', 'People'], ['signins', 'Sign-ins'], ['suggestions', 'Suggestions']];
const ROLES = ['admin', 'operator', 'manager', 'installer', 'viewer'];
const TREES = [['locations', 'Locations'], ['services', 'Services']];

// adminPage is the Admin tab: People, or the sign-in record.
export async function adminPage(ctx, tab) {
	tab = pick(TABS, tab);
	const main = [
		h('div', { class: 'head' }, h('div', null,
			h('h1', null, 'Admin'),
			h('div', { class: 'sub' }, 'The people who sign in to this manager, what each may do where, every sign-in, and what they suggest.'))),
		tabBar('/admin', TABS, tab),
	];
	// What people asked Aeolus to do, from its pages (0323).
	main.push(tab === 'signins' ? await signInsPanel(ctx) : tab === 'suggestions' ? await suggestionsPanel() : await peoplePanel(ctx));
	return { main, refresh: 0 };
}

// stateChips says what an account is at a glance.
function stateChips(a) {
	return [
		a.owner && h('span', { class: 'chip here', title: 'May do everything, everywhere, and runs the Org\'s people' }, 'super admin'),
		a.state === 'disabled' && h('span', { class: 'chip warn', title: 'Can\'t sign in or use its tokens until it is enabled' }, 'disabled'),
		a.state === 'removed' && h('span', { class: 'chip idle', title: 'Gone for good; its ID is never taken again' }, 'removed'),
		a.state === 'active' && a.must_change && h('span', { class: 'chip idle', title: 'Its password was set by someone else, and is changed at its next sign-in' }, 'temporary password'),
		a.state === 'active' && !a.has_password && h('span', { class: 'chip idle', title: 'Signs in with an API token only' }, 'no password'),
	];
}

// roleLine says an account's roles in a line: admin on Example (both trees),
// installer on Building B. The tree of each is said on hovering it.
function roleLine(ctx, grants) {
	if (!grants.length) return h('span', { class: 'sub' }, 'none');
	const said = new Map();
	for (const g of grants) {
		const k = `${g.role} on ${ctx.name(g.tree, g.node)}`;
		said.set(k, [...(said.get(k) || []), g.tree === 'services' ? 'Services' : 'Locations']);
	}
	return [...said].map(([k, trees], i) => [i > 0 && ', ', h('span', { title: trees.join(' and ') }, trees.length > 1 ? `${k} (both trees)` : k)]);
}

async function peoplePanel(ctx) {
	const { accounts, may_add: mayAdd } = await get('/v1/accounts');
	const box = h('div', { class: 'edit' });
	const live = accounts.filter((a) => a.state !== 'removed');
	const gone = accounts.length - live.length;
	let showGone = false;
	const body = h('tbody');
	const draw = () => body.replaceChildren(...(showGone ? accounts : live).map((a) => h('tr', null,
		h('td', { class: 'mono' }, link(`/admin/accounts/${encodeURIComponent(a.account)}`, a.account)),
		h('td', null, a.name),
		h('td', null, stateChips(a)),
		h('td', null, roleLine(ctx, a.grants)),
		h('td', { title: a.last_sign_in ? when(a.last_sign_in) : 'Not with a password, in the last 90 days' }, a.last_sign_in ? ago(a.last_sign_in) : '—'))));
	draw();
	const people = h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, 'People', h('span', { class: 'note' }, `${live.length}`),
			h('span', { class: 'controls' },
				gone > 0 && h('label', { class: 'tick' }, h('input', { type: 'checkbox', onchange: (e) => { showGone = e.target.checked; draw(); } }), ` Show ${gone} removed`),
				mayAdd && h('button', { type: 'button', class: 'button small', onclick: () => inviteForm(ctx, box) }, 'Invite someone'),
				mayAdd && h('button', { type: 'button', class: 'button small', onclick: () => addForm(box) }, 'Add a person'))),
		box,
		h('table', { class: 'list' },
			h('thead', null, h('tr', null, h('th', null, 'Account'), h('th', null, 'Name'), h('th', null, ''), h('th', null, 'Roles'), h('th', null, 'Last sign-in'))),
			body));
	return h('div', null, people, mayAdd && await invitesList());
}

// INVITE_STATE is how each state of an invite looks (0336).
const INVITE_STATE = { open: ['chip ok', 'open'], used: ['chip here', 'used'], expired: ['chip idle', 'expired'], revoked: ['chip idle', 'revoked'] };

// invitesSaid is what an invite gives, in a line.
function invitesSaid(iv) {
	const roles = iv.roles.map((r) => `${r.role} on ${r.where}`);
	if (iv.owner) roles.unshift('super admin');
	return roles.join(', ');
}

// invitesList is an owner's invites (0336): the open ones, each revoked in
// place, and those used, expired or revoked, on asking.
async function invitesList() {
	const { invites } = await get('/v1/invites');
	if (!invites.length) return null;
	const box = h('div', { class: 'edit' });
	const open = invites.filter((i) => i.state === 'open');
	const past = invites.length - open.length;
	let showPast = false;
	const body = h('tbody');
	const row = (iv) => h('tr', null,
		h('td', null, h('span', { class: INVITE_STATE[iv.state]?.[0] || 'chip' }, INVITE_STATE[iv.state]?.[1] || iv.state)),
		h('td', null, invitesSaid(iv)),
		h('td', { class: 'sub' }, iv.note || ''),
		h('td', { title: iv.made ? when(iv.made) : '' }, `${iv.made ? ago(iv.made) : '—'} by ${iv.by}`),
		h('td', { title: when(iv.expires) }, iv.state === 'used'
			? [link(`/admin/accounts/${encodeURIComponent(iv.used_by)}`, iv.used_by), iv.used_at ? `, ${ago(iv.used_at)}` : '']
			: iv.state === 'open' ? `until ${when(iv.expires)}` : when(iv.expires)),
		h('td', null, iv.state === 'open' && h('button', { type: 'button', class: 'button small', onclick: () => revokeInvite(box, iv) }, 'Revoke')));
	const draw = () => {
		const list = showPast ? invites : open;
		body.replaceChildren(...(list.length ? list.map(row) : [h('tr', null, h('td', { colspan: 6, class: 'sub' }, 'None open.'))]));
	};
	draw();
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, 'Invites', h('span', { class: 'note' }, `${open.length} open`),
			h('span', { class: 'controls' },
				past > 0 && h('label', { class: 'tick' }, h('input', { type: 'checkbox', onchange: (e) => { showPast = e.target.checked; draw(); } }), ` Show ${past} used, expired or revoked`))),
		box,
		h('table', { class: 'list' },
			h('thead', null, h('tr', null, h('th', null, ''), h('th', null, 'Gives'), h('th', null, 'Note'), h('th', null, 'Made'), h('th', null, 'Good until / used by'), h('th', null, ''))),
			body));
}

function revokeInvite(box, iv) {
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('div', null, `Revoke the invite giving ${invitesSaid(iv)}? Its link stops working. This can't be undone.`),
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: async () => {
				try {
					await del(`/v1/invites/${encodeURIComponent(iv.id)}`);
					box.replaceChildren();
					flash('Revoked.');
					redrawNow();
				} catch (e) {
					box.replaceChildren(h('div', { class: 'error' }, e.message));
				}
			} }, 'Revoke'),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
}

// folderOptions is a tree's folders, indented by depth: where a role may
// be given.
function folderOptions(ctx, t) {
	const depth = (n) => { let d = 0; for (let p = ctx.trees[t].nodes.get(n.parent); p; p = ctx.trees[t].nodes.get(p.parent)) d++; return d; };
	return ctx.trees[t].list.filter((n) => n.kind !== 'ap').map((n) => h('option', { value: n.id }, `${'  '.repeat(depth(n))}${n.name}`));
}

// inviteForm makes an invite (0336): the roles it gives, a line each,
// whether it makes a super admin, how many days it is good, and a note;
// then its link, shown once.
function inviteForm(ctx, box) {
	const lines = h('div', { class: 'invite-roles' });
	const rows = [];
	const addLine = () => {
		const tree = h('select', { 'aria-label': 'Tree' }, TREES.map(([t, label]) => h('option', { value: t }, label)));
		const node = h('select', { 'aria-label': 'Folder' });
		const role = h('select', { 'aria-label': 'Role' }, ROLES.map((r) => h('option', { value: r, selected: r === 'viewer' }, r)));
		const nodes = () => node.replaceChildren(...folderOptions(ctx, tree.value));
		tree.addEventListener('change', nodes);
		nodes();
		const row = { tree, node, role, gone: false };
		const el = h('div', { class: 'actions' }, role, ' on ', tree, node,
			h('button', { type: 'button', class: 'button small', onclick: () => { row.gone = true; el.remove(); } }, 'Remove'));
		rows.push(row);
		lines.append(el);
	};
	addLine();
	const owner = h('input', { type: 'checkbox' });
	const days = h('input', { type: 'number', min: 1, max: 30, value: 7, style: { width: '5em' }, 'aria-label': 'Days' });
	const note = h('input', { type: 'text', maxlength: 200, placeholder: 'a note: who it is for', 'aria-label': 'Note', style: { 'min-width': '18em' } });
	const error = h('div', { class: 'error', hidden: true });
	const go = async () => {
		error.hidden = true;
		const roles = rows.filter((r) => !r.gone && r.node.value).map((r) => ({ tree: r.tree.value, node: r.node.value, role: r.role.value }));
		try {
			const res = await post('/v1/invites', { roles, owner: owner.checked, days: Number(days.value) || 7, note: note.value.trim() });
			const url = location.origin + res.path;
			box.replaceChildren(shownOnce(box, `An invite giving ${invitesSaid(res.invite)}.`,
				`Its link, shown only now: the manager keeps only its hash. Send it to them; it works once, until ${when(res.invite.expires)}, and they choose their own account and password.`, url));
		} catch (e) {
			error.textContent = e.message;
			error.hidden = false;
		}
	};
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('strong', null, 'Invite someone'),
		h('div', { class: 'sub' }, 'They get a link to join by, with the roles it gives; each holds on its folder and everything below it.'),
		lines,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button small', onclick: addLine }, 'Another role'),
			h('label', { class: 'tick', title: 'May do everything, everywhere, and runs the Org\'s people' }, owner, ' Super admin')),
		h('div', { class: 'actions' }, h('label', null, 'Good for ', days, ' days'), note),
		error,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: go }, 'Make the link'),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
}

// addForm adds an account, with a temporary password shown once (0176).
function addForm(box) {
	const id = h('input', { type: 'text', class: 'mono', placeholder: 'their sign-in ID: jsmith', maxlength: 32, spellcheck: 'false', autocapitalize: 'none' });
	const name = h('input', { type: 'text', placeholder: 'their name: Jane Smith', maxlength: 100 });
	const owner = h('input', { type: 'checkbox' });
	const error = h('div', { class: 'error', hidden: true });
	const go = async () => {
		error.hidden = true;
		try {
			const res = await post('/v1/accounts', { account: id.value.trim(), name: name.value.trim(), owner: owner.checked });
			box.replaceChildren(shownOnce(box, `${res.account} is added.`, 'Their temporary password, shown only now. Tell it to them another way; they choose their own at their first sign-in. Give them roles on their account\'s page.', res.password,
				h('a', { href: `#/admin/accounts/${encodeURIComponent(res.account)}`, class: 'button small' }, 'Their roles')));
		} catch (e) {
			error.textContent = e.message;
			error.hidden = false;
		}
	};
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('div', { class: 'actions' }, id, name, h('label', { class: 'tick', title: 'May do everything, everywhere, and runs the Org\'s people' }, owner, ' Super admin')),
		error,
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: go }, 'Add'),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
	id.focus();
}

// shownOnce shows a secret the manager answers only once: a temporary
// password or a token.
function shownOnce(box, head, says, secret, ...more) {
	const field = h('input', { type: 'text', class: 'mono', readonly: true, value: secret, size: Math.min(secret.length + 2, 60) });
	return h('div', { class: 'preview', 'data-editing': true },
		h('strong', null, head),
		h('div', { class: 'sub' }, says),
		h('div', { class: 'actions' }, field,
			h('button', { type: 'button', class: 'button small', onclick: () => navigator.clipboard?.writeText(secret).then(() => flash('Copied.'), () => field.select()) }, 'Copy'),
			...more,
			h('button', { type: 'button', class: 'button small', onclick: () => { box.replaceChildren(); redrawNow(); } }, 'Done')));
}

// accountPage is one account: its state, roles, tokens, sessions and
// sign-ins. mine: the signed-in person's own, as My account.
export async function accountPage(ctx, id, mine = false) {
	const r = await get(`/v1/accounts/${encodeURIComponent(id)}`);
	const a = r.account;
	const box = h('div', { class: 'edit' });
	const run = r.may_run;
	const active = a.state === 'active';
	const head = h('div', { class: 'head' }, h('div', null,
		!mine && h('div', { class: 'sub' }, link('/admin/people', 'Admin'), ' › People'),
		h('h1', null, a.name || a.account, stateChips(a)),
		h('div', { class: 'sub' }, h('span', { class: 'mono' }, a.account),
			' · last signed in ', a.last_sign_in ? `${ago(a.last_sign_in)} (${when(a.last_sign_in)})` : 'not with a password lately'),
		h('div', { class: 'acts' },
			mine && link('/password', h('span', { class: 'button small' }, a.has_password ? 'Change your password' : 'Set a password')),
			run && a.state !== 'removed' && [
				h('button', { type: 'button', class: 'button small', onclick: () => resetForm(box, a) }, 'Reset password'),
				h('button', { type: 'button', class: 'button small', onclick: () => change(box, { kind: 'set-account', account: a.account }, 'Rename', 'name', a.name) }, 'Rename'),
				h('button', { type: 'button', class: 'button small', onclick: () => change(box, { kind: 'set-owner', account: a.account, owner: !a.owner }, a.owner ? `${a.account} is no longer a super admin` : `Make ${a.account} a super admin: everything, everywhere, and the Org's people`) }, a.owner ? 'Not a super admin' : 'Make super admin'),
				active
					? h('button', { type: 'button', class: 'button small', onclick: () => change(box, { kind: 'disable-account', account: a.account }, `Disable ${a.account}: its sessions end, and it can't sign in or use its tokens until it is enabled`) }, 'Disable')
					: h('button', { type: 'button', class: 'button small', onclick: () => change(box, { kind: 'enable-account', account: a.account }, `Enable ${a.account}: it signs in again, and its tokens work again`) }, 'Enable'),
				h('button', { type: 'button', class: 'button small', onclick: () => change(box, { kind: 'remove-account', account: a.account }, `Remove ${a.account} for good: its roles and tokens go, its sessions end, and its ID is never taken again. The change log keeps its name on what it did.`, null, null, () => { location.hash = '#/admin/people'; }) }, 'Remove'),
			])));
	const main = [head, box, rolesPanel(ctx, a)];
	if (r.tokens) main.push(tokensPanel(a, r.tokens, mine || run));
	if (r.sessions) main.push(sessionsPanel(a, r.sessions, mine));
	if (r.signins) main.push(signInList(r.signins, false, 'Recent sign-ins'));
	return { main, refresh: 0 };
}

// change makes an account change in box: what says what it does. With
// field, it asks for that field's new value first.
function change(box, op, what, field, now, done) {
	const input = field && h('input', { type: 'text', value: now || '', maxlength: 100 });
	const error = h('div', { class: 'error', hidden: true });
	const apply = h('button', { type: 'button', class: 'button primary' }, 'Apply');
	apply.addEventListener('click', async () => {
		apply.disabled = true;
		error.hidden = true;
		try {
			const res = await post('/v1/changes', { op: field ? { ...op, [field]: input.value.trim() } : op });
			box.replaceChildren();
			flash(`Saved as change #${res.change.seq}.`);
			done?.();
			redrawNow();
		} catch (e) {
			apply.disabled = false;
			error.textContent = e.message;
			error.hidden = false;
		}
	});
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('div', null, what),
		input,
		error,
		h('div', { class: 'actions' }, apply, h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
	(input || apply).focus();
}

// resetForm resets another's password to a temporary one (0176).
function resetForm(box, a) {
	const go = async () => {
		try {
			const res = await post(`/v1/accounts/${encodeURIComponent(a.account)}/password`, {});
			const ended = res.sessions_ended ? ` ${res.sessions_ended} session${res.sessions_ended === 1 ? '' : 's'} ended.` : '';
			box.replaceChildren(shownOnce(box, `${a.account}'s password is reset.${ended}`, 'The temporary password, shown only now. Tell it to them another way; they choose their own at their next sign-in.', res.password));
		} catch (e) {
			box.replaceChildren(h('div', { class: 'error' }, e.message));
		}
	};
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('div', null, `Give ${a.account} a temporary password? Their sessions end, and the old password stops working.`),
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: go }, 'Reset'),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
}

// rolesPanel is an account's roles, each taken away in place where the
// person may, and a role given on a folder where they may give one.
function rolesPanel(ctx, a) {
	const box = h('div', { class: 'edit' });
	const giving = a.state === 'active' && TREES.some(([t]) => ctx.trees[t].list.some((n) => n.kind !== 'ap' && givesAt(t, n.id).length));
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, 'Roles',
			h('span', { class: 'note' }, 'Each holds on its folder and everything below it.'),
			giving && h('span', { class: 'controls' }, h('button', { type: 'button', class: 'button small', onclick: () => grantForm(ctx, box, a) }, 'Give a role'))),
		box,
		a.owner && h('div', { class: 'edit' }, h('div', { class: 'sub' }, 'A super admin may do everything everywhere, whatever roles it holds.')),
		a.grants.length
			? h('table', { class: 'list' },
				h('thead', null, h('tr', null, h('th', null, 'Role'), h('th', null, 'On'), h('th', null, ''))),
				h('tbody', null, a.grants.map((g) => h('tr', null,
					h('td', null, g.role),
					h('td', null, link(`/${g.tree}/${encodeURIComponent(g.node)}`, `${g.tree === 'services' ? 'Services' : 'Locations'} › ${ctx.name(g.tree, g.node)}`)),
					h('td', null, g.may_take && h('button', { type: 'button', class: 'button small', onclick: () => change(box, { kind: 'revoke', account: a.account, tree: g.tree, node: g.node, role: g.role }, `Take ${g.role} on ${ctx.name(g.tree, g.node)} from ${a.account}`) }, 'Take away'))))))
			: h('div', { class: 'edit' }, h('div', { class: 'sub' }, a.owner ? '' : 'None: it sees nothing yet.')));
}

// grantForm gives a role: a tree, a folder in it where the person may give
// one, and a role they may give there.
function grantForm(ctx, box, a) {
	const tree = h('select', { 'aria-label': 'Tree' }, TREES.map(([t, label]) => h('option', { value: t }, label)));
	const node = h('select', { 'aria-label': 'Folder' });
	const role = h('select', { 'aria-label': 'Role' });
	const depth = (t, n) => { let d = 0; for (let p = ctx.trees[t].nodes.get(n.parent); p; p = ctx.trees[t].nodes.get(p.parent)) d++; return d; };
	const roles = () => {
		const may = givesAt(tree.value, node.value);
		role.replaceChildren(...ROLES.filter((r) => may.includes(r)).map((r) => h('option', { value: r }, r)));
	};
	const nodes = () => {
		const t = tree.value;
		node.replaceChildren(...ctx.trees[t].list.filter((n) => n.kind !== 'ap' && givesAt(t, n.id).length)
			.map((n) => h('option', { value: n.id }, `${'  '.repeat(depth(t, n))}${n.name}`)));
		roles();
	};
	tree.addEventListener('change', nodes);
	node.addEventListener('change', roles);
	nodes();
	const next = () => change(box, { kind: 'grant', account: a.account, tree: tree.value, node: node.value, role: role.value },
		`Give ${a.account} ${role.value} on ${ctx.name(tree.value, node.value)}, and everything below it`);
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('div', { class: 'actions' }, tree, node, role,
			h('button', { type: 'button', class: 'button primary', onclick: () => node.value && role.value && next() }, 'Next'),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
}

// tokensPanel is an account's API tokens, for scripts and the MCP adapter:
// issued and revoked by the account itself, or by an owner.
function tokensPanel(a, tokens, may) {
	const box = h('div', { class: 'edit' });
	const issue = async () => {
		try {
			const res = await post('/v1/tokens', { account: a.account, reason: 'issued from the Admin page' });
			box.replaceChildren(shownOnce(box, 'A new token.', 'Shown only now: the manager keeps only its hash. Use it as Authorization: Bearer <token>.', res.token));
		} catch (e) {
			box.replaceChildren(h('div', { class: 'error' }, e.message));
		}
	};
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, 'API tokens', h('span', { class: 'note' }, 'For scripts and the MCP adapter. A browser signs in with a password.'),
			may && a.state === 'active' && h('span', { class: 'controls' }, h('button', { type: 'button', class: 'button small', onclick: issue }, 'Issue a token'))),
		box,
		tokens.length
			? h('table', { class: 'list' },
				h('thead', null, h('tr', null, h('th', null, 'Token'), h('th', null, 'Issued'), h('th', null, 'Last used'), h('th', null, ''))),
				h('tbody', null, tokens.map((t) => h('tr', null,
					h('td', { class: 'mono' }, `aeolus1.${t.id}.…`),
					h('td', { title: t.reason || '' }, t.issued_at ? `${when(t.issued_at)} by ${t.issued_by}` : '—'),
					h('td', { title: t.last_used ? when(t.last_used) : '' }, t.last_used ? `${ago(t.last_used)} from ${t.source}` : 'not since this was kept'),
					h('td', null, may && h('button', { type: 'button', class: 'button small', onclick: () => revoke(box, t) }, 'Revoke'))))))
			: h('div', { class: 'edit' }, h('div', { class: 'sub' }, 'None.')));
}

function revoke(box, t) {
	box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
		h('div', null, `Revoke aeolus1.${t.id}? Whatever uses it is refused from then on. This can't be undone.`),
		h('div', { class: 'actions' },
			h('button', { type: 'button', class: 'button primary', onclick: async () => {
				try {
					await del(`/v1/tokens/${encodeURIComponent(t.id)}`);
					box.replaceChildren();
					flash('Revoked.');
					redrawNow();
				} catch (e) {
					box.replaceChildren(h('div', { class: 'error' }, e.message));
				}
			} }, 'Revoke'),
			h('button', { type: 'button', class: 'button', onclick: () => box.replaceChildren() }, 'Cancel'))));
}

// browser says a User-Agent the short way.
function browser(agent) {
	const m = agent.match(/(Edg|Firefox|Chrome|Safari)\/[\d.]+/);
	const os = agent.match(/Windows|Mac OS X|Android|iPhone|iPad|Linux/);
	const name = m ? { Edg: 'Edge' }[m[1]] || m[1] : 'A browser';
	return os ? `${name} on ${os[0].replace('Mac OS X', 'macOS')}` : name;
}

// sessionsPanel is an account's signed-in browsers, each ended in place.
function sessionsPanel(a, list, mine) {
	const end = async (path) => {
		try {
			const res = await del(path);
			flash(`${res.ended} session${res.ended === 1 ? '' : 's'} ended.`);
			redrawNow();
		} catch (e) {
			flash(e.message);
		}
	};
	const others = list.filter((s) => !s.current).length;
	const base = `/v1/accounts/${encodeURIComponent(a.account)}/sessions`;
	return h('section', { class: 'panel' },
		h('h2', { class: 'withcontrols' }, 'Signed in', h('span', { class: 'note' }, `${list.length}`),
			others > 0 && h('span', { class: 'controls' }, h('button', { type: 'button', class: 'button small', onclick: () => end(base) }, mine ? 'Sign out everywhere else' : 'End them all'))),
		list.length
			? h('table', { class: 'list' },
				h('thead', null, h('tr', null, h('th', null, 'Browser'), h('th', null, 'From'), h('th', null, 'Signed in'), h('th', null, 'Last used'), h('th', null, 'Ends'), h('th', null, ''))),
				h('tbody', null, list.map((s) => h('tr', null,
					h('td', { title: s.agent }, browser(s.agent), s.current && h('span', { class: 'chip ok' }, 'this one')),
					h('td', { class: 'mono' }, s.source),
					h('td', null, when(s.created)),
					h('td', null, ago(s.last_used)),
					h('td', { title: s.keep ? 'Kept: 30 days unused' : '8 hours unused' }, when(s.expires)),
					h('td', null, !s.current && h('button', { type: 'button', class: 'button small', onclick: () => end(`${base}/${s.id}`) }, 'End'))))))
			: h('div', { class: 'edit' }, h('div', { class: 'sub' }, 'Not signed in anywhere with a password.')));
}

// signInList is the record as a table; who: with the account column.
function signInList(list, who, title) {
	return h('section', { class: 'panel' },
		h('h2', null, title, h('span', { class: 'note' }, 'Kept 90 days')),
		list.length
			? h('table', { class: 'list' },
				h('thead', null, h('tr', null, h('th', null, 'When'), who && h('th', null, 'Account'), h('th', null, 'From'), h('th', null, 'Result'))),
				h('tbody', null, list.map((s) => h('tr', null,
					h('td', { title: when(s.at) }, ago(s.at)),
					who && h('td', { class: 'mono' }, link(`/admin/accounts/${encodeURIComponent(s.account)}`, s.account)),
					h('td', { class: 'mono' }, s.source),
					h('td', null, h('span', { class: s.ok ? 'chip ok' : 'chip bad' }, s.ok ? 'signed in' : 'refused'), ' ', s.why === 'ok' ? '' : s.why)))))
			: h('div', { class: 'edit' }, h('div', { class: 'sub' }, 'None.')));
}

// signInsPanel is the record the person may read, by account and result.
async function signInsPanel() {
	const account = h('input', { type: 'search', class: 'mono', placeholder: 'account', 'aria-label': 'Account', 'data-kept': true, size: 14 });
	const result = h('select', { 'aria-label': 'Result' }, h('option', { value: '' }, 'All'), h('option', { value: 'true' }, 'Signed in'), h('option', { value: 'false' }, 'Refused'));
	const out = h('div');
	const load = async () => {
		const q = new URLSearchParams();
		if (account.value.trim()) q.set('account', account.value.trim().toLowerCase());
		if (result.value) q.set('ok', result.value);
		const { signins } = await get(`/v1/signins?${q}`);
		out.replaceChildren(signInList(signins, true, 'Sign-ins'));
	};
	account.addEventListener('keydown', (e) => { if (e.key === 'Enter') load(); });
	result.addEventListener('change', load);
	await load();
	return h('div', null, h('div', { class: 'filters' }, account, result, h('button', { type: 'button', class: 'button small', onclick: load }, 'Show')), out);
}
