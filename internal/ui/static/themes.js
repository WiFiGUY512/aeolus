// Light, dark, or as the device is (0296): the choice, kept in this
// browser, and the header's pulldown for it. theme.js puts the theme on the
// page before it is drawn; this changes it after. A page redraws when it
// changes, so its charts take the new colours.

import { h } from './dom.js';

const KEY = 'aeolus.theme';
export const THEMES = [['auto', 'Auto'], ['light', 'Light'], ['dark', 'Dark']];

// kept is the choice where the browser keeps nothing (private windows, or
// storage turned off): for as long as the page is open.
let kept = null;

// chosen is the theme chosen: auto, light or dark.
export function chosen() {
	try {
		return localStorage.getItem(KEY) || kept || 'auto';
	} catch {
		return kept || 'auto';
	}
}

const device = () => window.matchMedia('(prefers-color-scheme: dark)');

// apply puts the chosen theme on the page, auto as the device is now.
function apply() {
	const t = chosen();
	document.documentElement.dataset.theme = t === 'light' || t === 'dark' ? t : device().matches ? 'dark' : 'light';
}

// choose keeps t and puts it on the page, then has it redrawn.
function choose(t, redraw) {
	kept = t === 'auto' ? null : t;
	try {
		if (t === 'auto') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, t);
	} catch {
		// kept, for this page only
	}
	apply();
	redraw();
}

// watchTheme has the page redrawn when the device turns light or dark,
// where the theme follows it, so its charts follow too. theme.js has put
// the page's own colours right already.
export function watchTheme(redraw) {
	device().addEventListener('change', () => {
		if (chosen() === 'auto') redraw();
	});
}

// themePick is the header's pulldown.
export function themePick(redraw) {
	const now = chosen();
	return h('select', {
		class: 'themepick', 'aria-label': 'Theme', title: 'Light, dark, or as this device is set', 'data-kept': true,
		onchange: (e) => choose(e.target.value, redraw),
	}, THEMES.map(([v, name]) => h('option', { value: v, selected: v === now }, name)));
}
