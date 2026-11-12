// The advisor's page (0205): what a folder's advisor's reviews found
// (0212), the open first, each with what it saw and what to do, and the
// clients, APs and networks it concerns; then its latest reviews, what each
// cost and how it ended. Where there are more folders with an advisor, one
// is picked. An admin of the folder asks for a review now, or asks it a
// question, whose answer shows here (0215); an operator of it says a
// finding is fixed, or dismisses it, or opens it again. Where the advisor's
// account is not set up, an owner sets it up here, in one step.

import { h, link } from '../dom.js';
import { get, post, APIError } from '../api.js';
import { ago, when, prose } from '../format.js';
import { called } from '../names.js';
import { flash, redrawNow } from '../refresh.js';
import { mayAnywhere } from '../rights.js';
import { ask, cancelButton } from './confirm.js';
import { clientLink } from './journey.js';

const SEVERITY = { critical: ['bad', 'Critical'], warning: ['warn', 'Warning'], info: ['', 'Info'] };
// Findings, the most severe first, then the latest found.
const RANK = { critical: 0, warning: 1, info: 2 };
const bySeverity = (x, y) => (RANK[x.severity] ?? 3) - (RANK[y.severity] ?? 3) || new Date(y.at) - new Date(x.at);
const OUTCOME = { running: 'idle', done: 'ok', budget: 'warn', failed: 'bad' };
const SAYS = { running: 'running', done: 'done', budget: 'stopped at its budget', failed: 'failed' };

const chip = (cls, text) => h('span', { class: `chip ${cls}` }, text);
const dollars = (n) => `$${(n || 0).toFixed(2)}`;
// tokens writes a count the short way: 812, 12.3k, 1.2M.
const tokens = (n) => (n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}k` : String(n || 0));

// seesLocations says whether the person sees some of Locations, where a
// folder's advisor may be (0212).
export function seesLocations(ctx) {
	return Boolean(ctx?.trees.locations.list.length);
}

// The folder whose advisor is shown, across a redraw: the first one sees,
// unless picked.
let pick = null;

// Whether the dismissed and the resolved findings are open, across a redraw.
let dismissedOpen = false;
let resolvedOpen = false;

// A question being written, across a redraw, and one begun on another page
// by an Ask the advisor button, which puts the cursor in it (0215).
let asking = '';
let focusAsk = false;

// askButton is an Ask the advisor button on another page, for one who may
// ask (an admin, somewhere): it opens this page with the question begun.
export function askButton(begun, cls = 'button small') {
	if (!mayAnywhere('administer')) return null;
	return h('button', { type: 'button', class: cls, title: 'Ask the advisor about this: it looks into it, and answers on the Advisor page',
		onclick: () => { asking = begun; focusAsk = true; location.hash = '#/advisor'; } }, 'Ask the advisor');
}

export async function advisorPage(ctx) {
	let a;
	try {
		a = await get(`/v1/advisor${pick ? `?scope=${encodeURIComponent(pick)}` : ''}`);
	} catch (e) {
		// The folder picked has no advisor now, or is not one's to see.
		if (e instanceof APIError && e.status === 404 && pick) {
			pick = null;
			return advisorPage(ctx);
		}
		throw e;
	}
	const scopes = a.scopes || [];
	const s = scopes.find((x) => x.scope === a.scope);
	const last = a.runs[0];
	const said = h('div', { class: 'edit' });
	const out = h('div', { class: 'edit' });
	const system = (node, text) => link(`/locations/${encodeURIComponent(node)}/settings`, text);
	const review = async () => {
		said.replaceChildren();
		try {
			await post('/v1/advisor/run', { scope: s.scope });
			flash('A review has begun. It takes a few minutes; this page shows it when it is done.', 30);
			redrawNow();
		} catch (e) {
			said.replaceChildren(h('div', { class: 'error' }, e.message));
		}
	};
	// One folder's advisor at a time (0212), where there are more.
	const picker = scopes.length > 1 && h('label', { class: 'sub scopepick' }, 'Folder ',
		h('select', { class: 'small', 'aria-label': 'Whose advisor', onchange: (e) => { pick = e.target.value; redrawNow(); } },
			scopes.map((x) => h('option', { value: x.scope, selected: x.scope === a.scope }, `${x.name}${x.open ? ` · ${x.open} open` : ''}`))));
	if (!s) {
		return { main: [
			h('div', { class: 'head' }, h('h1', null, 'Advisor')),
			!a.available ? h('div', { class: 'banner info' }, 'This manager has no advisor.')
				: h('div', { class: 'banner info setup' },
					!a.account_set && setup(ctx, a.account),
					h('div', null, 'No folder you see has an advisor yet. An admin of a folder sets one up in that folder’s System settings, under Advisor: with an Anthropic API key set there, the folder and everything under it are reviewed, and billed to that key. A folder below with a key of its own has its own advisor.'),
					a.may_set_up && h('div', null, 'Start at ', system(ctx.trees.locations.root, `${ctx.name('locations', ctx.trees.locations.root)}’s System settings`), ', or any folder’s.')),
		] };
	}
	const open = a.findings.filter((f) => f.state === 'open').sort(bySeverity);
	const dismissed = a.findings.filter((f) => f.state === 'dismissed').sort(bySeverity);
	// The latest resolved first: what was fixed lately.
	const resolved = a.findings.filter((f) => f.state === 'resolved').sort((x, y) => new Date(y.state_at || y.at) - new Date(x.state_at || x.at));
	const questions = a.runs.filter((r) => r.question).slice(0, 3);
	const answering = last?.outcome === 'running' && last.question;
	return {
		main: [
			h('div', { class: 'head' },
				h('div', null,
					h('h1', null, 'Advisor', h('span', { class: 'note' }, s.name)),
					h('div', { class: 'sub' }, [
						s.enabled ? 'On' : 'Off', s.model, `every ${s.every} h`,
						last && `last review ${ago(last.started)}, ${SAYS[last.outcome] || last.outcome}, ${dollars(last.cost)}`,
					].filter(Boolean).join(' · '), s.running && [' · ', chip('idle', answering ? 'Answering…' : 'Reviewing…')], ' · ', system(s.scope, 'its settings'))),
				h('div', { class: 'acts' },
					picker,
					s.may_review && a.available && h('button', { type: 'button', class: 'button', disabled: a.running, title: a.running && !s.running ? 'Another folder’s review is under way: one runs at a time' : null, onclick: review }, 'Review now'))),
			said,
			!a.available && h('div', { class: 'banner info' }, 'This manager has no advisor.'),
			// What it still needs before it can review: its account.
			a.available && !a.account_set && h('div', { class: 'banner info setup' }, setup(ctx, a.account)),
			((s.may_review && a.available) || questions.length > 0) && askPanel(s, a, questions),
			h('section', { class: 'panel' },
				h('h2', null, 'Findings', h('span', { class: 'note' }, open.length ? `${open.length} open` : 'none open')),
				out,
				open.length ? findings(ctx, open, s.may_dismiss, out)
					: h('p', { class: 'sub pad' }, a.runs.length ? 'Nothing open: the reviews found nothing, or all of it is fixed or dismissed.' : 'No review yet.')),
			resolved.length > 0 && h('details', { class: 'panel logpanel', open: resolvedOpen, ontoggle: (e) => { resolvedOpen = e.currentTarget.open; } },
				h('summary', null, h('h2', null, 'Resolved', h('span', { class: 'note' }, `${resolved.length}, its problem gone`))),
				findings(ctx, resolved, s.may_dismiss, out)),
			dismissed.length > 0 && h('details', { class: 'panel logpanel', open: dismissedOpen, ontoggle: (e) => { dismissedOpen = e.currentTarget.open; } },
				h('summary', null, h('h2', null, 'Dismissed', h('span', { class: 'note' }, `${dismissed.length}`))),
				findings(ctx, dismissed, s.may_dismiss, out)),
			h('section', { class: 'panel' },
				h('h2', null, 'Reviews', h('span', { class: 'note' }, 'the latest 20')),
				a.runs.length ? runs(a.runs) : h('p', { class: 'sub pad' }, 'None yet.')),
		],
		// While a review runs, the page looks again until it is done.
		refresh: a.running ? 10 : 0,
	};
}

// askPanel is where an admin asks the advisor a question (0215), and the
// latest answers.
function askPanel(s, a, questions) {
	const error = h('div', { class: 'edit' });
	const may = s.may_review && a.available;
	const text = may && h('textarea', { class: 'ask', rows: 2, maxlength: 2000, 'aria-label': 'Your question',
		placeholder: 'Ask about a client, an AP or a network: why the kettle keeps dropping, whether the garage needs an AP of its own…',
		oninput: (e) => { asking = e.target.value; } });
	if (text) {
		text.value = asking;
		if (focusAsk) {
			focusAsk = false;
			requestAnimationFrame(() => { text.focus(); text.setSelectionRange(text.value.length, text.value.length); });
		}
	}
	const go = may && h('button', { type: 'button', class: 'button primary', disabled: a.running,
		title: a.running ? 'A review is under way: one runs at a time' : null, onclick: async () => {
			const q = text.value.trim();
			if (!q) {
				text.focus();
				return;
			}
			go.disabled = true;
			error.replaceChildren();
			try {
				await post('/v1/advisor/run', { scope: s.scope, question: q });
				asking = '';
				flash('Asked. The answer takes a minute or two, and shows here.', 20);
				redrawNow();
			} catch (e) {
				go.disabled = false;
				error.replaceChildren(h('div', { class: 'error' }, e.message));
			}
		} }, 'Ask');
	return h('section', { class: 'panel' },
		h('h2', null, 'Ask', h('span', { class: 'note' }, may ? `billed to ${s.name}’s key, at most ${dollars(s.budget)} a question` : 'the latest answers')),
		may && h('div', { class: 'askrow' }, text, go),
		error,
		questions.map((r) => h('div', { class: 'answer' },
			h('div', null, h('strong', null, r.question)),
			h('div', { class: 'sub', title: when(r.started) }, [ago(r.started), r.why, r.outcome !== 'done' && (SAYS[r.outcome] || r.outcome), dollars(r.cost)].filter(Boolean).join(' · ')),
			r.outcome === 'running' ? h('div', { class: 'sub' }, chip('idle', 'Looking into it…'))
				: r.error ? h('div', { class: 'sub warn' }, r.error)
					: prose(r.summary))));
}

// findings is a table of findings: each one's severity, what it is, what it
// saw and what to do, what it concerns, and when. A client's name leads to
// its page (0222); out says what went wrong.
function findings(ctx, list, operator, out) {
	const subjects = (f) => {
		const sub = f.subjects || {};
		const parts = [
			(sub.clients || []).map((mac) => clientLink(mac, called(mac) || mac)),
			(sub.aps || []).map((id) => link(`/aps/${encodeURIComponent(id)}`, ctx.name('locations', id))),
			(sub.networks || []).map((n) => h('span', { class: 'mono' }, n)),
		].flat();
		return parts.length > 0 && h('div', { class: 'sub' }, parts.flatMap((p, i) => (i ? [', ', p] : [p])));
	};
	// An open finding is fixed (its problem gone: found again, it opens
	// again) or dismissed (looked at, and not to act on: found again, it
	// stays dismissed unless worse); the others open again (0215).
	const LABEL = { open: 'Reopen', resolved: 'Fixed', dismissed: 'Dismiss' };
	const TITLE = { resolved: 'Its problem is gone: you fixed it. Found again, it opens again.', dismissed: 'Looked at, and not to act on: found again, it stays dismissed unless it is worse' };
	const button = (f, to) => h('button', { type: 'button', class: 'button small', title: TITLE[to], onclick: async (e) => {
		const b = e.currentTarget;
		b.disabled = true;
		try {
			await post(`/v1/advisor/findings/${f.id}`, { state: to });
			redrawNow();
		} catch (err) {
			b.disabled = false;
			out.replaceChildren(h('div', { class: 'error' }, err.message));
		}
	} }, LABEL[to]);
	const turn = (f) => (f.state === 'open' ? h('div', { class: 'turns' }, button(f, 'resolved'), button(f, 'dismissed')) : button(f, 'open'));
	const stateSaid = (f) => {
		if (f.state === 'open') return null;
		const by = `${f.state === 'resolved' ? 'Resolved' : 'Dismissed'}${f.state_by ? ` by ${f.state_by}` : ''}${f.state_at ? `, ${ago(f.state_at)}` : ''}`;
		return h('div', { class: 'sub' }, f.state_why ? `${by}: ${f.state_why}` : `${by}.`);
	};
	return h('table', { class: 'alerts findings' },
		h('tbody', null, list.map((f) => h('tr', { class: f.state === 'dismissed' ? null : f.severity },
			h('td', null, chip(SEVERITY[f.severity]?.[0] ?? '', SEVERITY[f.severity]?.[1] ?? f.severity)),
			h('td', null,
				h('div', null, h('span', { class: 'sub' }, `${f.kind} · `), h('strong', null, f.title)),
				h('div', null, f.saw),
				h('div', null, h('span', { class: 'sub' }, 'To do: '), f.todo),
				subjects(f),
				stateSaid(f)),
			h('td', { class: 'sub nowrap', title: `First found ${when(f.first_at)}; last ${when(f.at)}` },
				f.times > 1 ? `found ${f.times} times` : 'found once', h('br'), `first ${ago(f.first_at)}`, f.times > 1 && [h('br'), `last ${ago(f.at)}`]),
			h('td', null, operator && turn(f))))));
}

// runs is a table of the latest reviews.
function runs(list) {
	const said = (r) => {
		if (r.error) return h('span', { class: 'sub warn' }, r.error);
		const s = r.summary || '';
		// The short of it plain, the whole as written (0215).
		const plain = s.replace(/\*\*/g, '').replace(/\s+/g, ' ');
		if (s.length <= 160 && !s.includes('\n')) return plain;
		return h('details', { class: 'inline' }, h('summary', null, plain.length <= 150 ? plain : `${plain.slice(0, 150).replace(/\s+\S*$/, '')}…`), prose(s));
	};
	return h('table', { class: 'list' },
		h('tr', null, ['When', 'Why', 'How it ended', 'Requests', 'Tokens', 'Cost', 'Summary'].map((t) => h('th', null, t))),
		list.map((r) => h('tr', null,
			h('td', { class: 'nowrap', title: when(r.started) }, ago(r.started)),
			h('td', null, r.why, r.question && h('div', { class: 'sub', title: r.question }, `asked: ${r.question.length > 80 ? `${r.question.slice(0, 77)}…` : r.question}`)),
			h('td', null, chip(OUTCOME[r.outcome] ?? '', SAYS[r.outcome] || r.outcome)),
			h('td', null, String(r.requests)),
			h('td', { class: 'nowrap', title: `In ${r.input}, out ${r.output}, from the cache ${r.cache_read}, to it ${r.cache_write}${r.searches ? `; ${r.searches} web searches` : ''}` },
				tokens(r.input + r.output + r.cache_read + r.cache_write)),
			h('td', null, dollars(r.cost)),
			h('td', null, said(r)))));
}

// setup is the advisor's account not set up yet (0205): an owner sets it up
// here, as three changes, previewed together: the account, then viewer on
// the Org in each tree, which is all it reads with. Anyone else is told who
// sets it up.
function setup(ctx, account) {
	const box = h('div', { class: 'edit' });
	if (!ctx.who.owner) return h('div', null, `The advisor’s account, ${account}, is not set up yet: an owner sets it up here.`);
	const ops = [
		{ kind: 'add-account', account, name: 'The advisor' },
		...['locations', 'services'].map((tree) => ({ kind: 'grant', account, tree, node: ctx.trees[tree].root, role: 'viewer' })),
	];
	const go = async () => {
		const p = await ask(box, ops[0]);
		if (!p) return;
		const apply = h('button', { type: 'button', class: 'button primary' }, 'Apply');
		const error = h('div', { class: 'error' });
		apply.addEventListener('click', async () => {
			apply.disabled = true;
			const done = [];
			try {
				for (const op of ops) done.push((await post('/v1/changes', { op })).change.seq);
			} catch (e) {
				error.textContent = done.length ? `${e.message}. Made already: #${done.join(', #')}.` : e.message;
				apply.disabled = false;
				ops.splice(0, done.length);
				return;
			}
			box.replaceChildren();
			flash(`Saved as changes #${done.join(', #')}. Now set an Anthropic API key in a folder’s System settings, under Advisor.`);
			redrawNow();
		});
		box.replaceChildren(h('div', { class: 'preview', 'data-editing': true },
			h('div', null, h('strong', null, `Set up ${account}, three changes`)),
			h('ul', { class: 'becomes' },
				h('li', null, `Add the account ${account}, The advisor`),
				['locations', 'services'].map((tree) => h('li', null, `Give it viewer on ${tree === 'services' ? 'Services' : 'Locations'} › ${ctx.name(tree, ctx.trees[tree].root)}`))),
			h('div', { class: 'sub' }, 'The advisor reads as this account, from within the manager, and changes nothing. A folder’s advisor reads that folder alone, whatever the account may see. It has no password and no token. Taking its roles away stops it reading.'),
			error,
			h('div', { class: 'actions' }, apply, cancelButton(box))));
		apply.focus();
	};
	return h('div', null,
		`The advisor reads as its own account, ${account}, which is not set up yet. `,
		h('button', { type: 'button', class: 'button small primary', onclick: go }, 'Set up the advisor’s account'),
		box);
}
