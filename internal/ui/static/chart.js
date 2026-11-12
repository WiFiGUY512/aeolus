// Charts drawn with Plotly (0294), which the manager serves itself
// (vendor/plotly, 0069) and a page loads the first time it has a chart.
// Each can be zoomed by dragging over it, panned, put back by a double
// click, a series hidden or shown by a click on its name, and saved as a
// picture. What was zoomed and hidden is kept over the page's redraws, a
// chart by its key. Plotly draws its labels from a markup of its own, so it
// is given only numbers, times, the page's own words and what the manager's
// own people named (APs, networks): never a client's or a device's name.

import { h } from './dom.js';

const SRC = '/ui/vendor/plotly/plotly-basic.min.js';
// Plotly's own style rules, as a stylesheet: the page's Content-Security-
// Policy keeps it from adding them itself (third_party/plotly).
const CSS = '/ui/vendor/plotly/plotly.css';
let loading = null;

// plotly loads the library, once, and answers it.
export function plotly() {
	if (window.Plotly) return Promise.resolve(window.Plotly);
	loading ??= new Promise((resolve, reject) => {
		if (!document.querySelector(`link[href="${CSS}"]`)) document.head.append(h('link', { rel: 'stylesheet', href: CSS }));
		const s = document.createElement('script');
		s.src = SRC;
		// logging 0: Plotly would warn on every draw that the policy keeps
		// it from adding a chart's own style rules, where plotly.css has
		// what every chart needs. It reads that from its global config.
		s.onload = () => {
			window.Plotly.setPlotConfig({ logging: 0 });
			resolve(window.Plotly);
		};
		s.onerror = () => {
			loading = null;
			s.remove();
			reject(new Error('the charts did not load'));
		};
		document.head.append(s);
	});
	return loading;
}

// token is one of the page's colours, by its name in app.css.
export function token(name) {
	return getComputedStyle(document.documentElement).getPropertyValue(`--${name}`).trim();
}

// series is the colour of a chart's nth series, 1 to 8, from the page's
// tokens: a categorical order checked for colour blindness, stepped for the
// light page and the dark (0294). The first three are the bands' (0166):
// 2.4 GHz blue, 5 orange, 6 aqua.
export const series = (n) => token(`series-${n}`);
export const bandColour = (band) => series({ '2g': 1, '5g': 2, '6g': 3 }[band]);

// The spans a chart is drawn over, in hours: a day and longer a button
// each, and the last few hours from a pulldown (0294), for a closer look at
// when something happened.
export const SPANS = [[24, 'A day'], [168, 'A week'], [720, '30 days'], [8760, 'A year']];
export const HOURS = [[1, 'An hour'], [3, '3 hours'], [6, '6 hours'], [12, '12 hours']];
export const spanName = (hours, spans = SPANS) => [...HOURS, ...spans].find(([t]) => t === hours)?.[1].toLowerCase() || `${hours} hours`;

// spanPicker is the choice of span: the last few hours from a pulldown, then
// a button each of spans. pick is given the hours chosen.
export function spanPicker(hours, pick, spans = SPANS) {
	const few = HOURS.some(([t]) => t === hours);
	return h('span', { class: 'segmented', role: 'group', 'aria-label': 'Over how long' },
		h('select', { class: few ? 'on' : null, 'aria-label': 'Over the last few hours', 'data-kept': true, onchange: (e) => pick(Number(e.target.value)) },
			h('option', { value: '', disabled: true, selected: !few }, 'Hours'),
			HOURS.map(([t, name]) => h('option', { value: t, selected: t === hours }, name))),
		spans.map(([t, name]) => h('button', {
			type: 'button', class: hours === t ? 'on' : '', 'aria-pressed': hours === t, onclick: () => pick(t),
		}, name)));
}

// HOW is how a chart is handled, for a panel's title to say on hover.
export const HOW = 'Hover for the numbers at a time; drag over the chart to zoom in, double-click to put it back; a click on a name in its key hides or shows it.';

// at is a time as Plotly takes it: by the page's own clock, since Plotly
// draws a time as it is written.
export function at(t) {
	const d = new Date(t), p = (n) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

// counts is a y axis of whole numbers up to most, with about ticks steps:
// spaced by hand where the numbers are few, so none is a fraction or
// said twice, and by Plotly where there are more.
export const counts = (most, ticks = 4) => (most <= ticks * 2
	? { tickformat: 'd', tick0: 0, dtick: Math.max(1, Math.ceil(most / ticks)) }
	: { tickformat: ',d', nticks: ticks + 1 });

// What was zoomed, by chart key and span, and hidden, by chart key.
const zoomed = new Map();
const hidden = new Map();

// The buttons a chart's bar leaves out: picking points means nothing here.
const DROPPED = ['select2d', 'lasso2d', 'toggleSpikelines', 'hoverClosestCartesian', 'hoverCompareCartesian'];

// chart is a box of height px that draws traces over layout, once it is on
// the page, and fits its width as the page lays it out. key names it for
// what is kept over a redraw, span for its zoom; name is what a picture of
// it is saved as; label says it for a screen reader.
export function chart({ key, span, name, label, height, traces, layout }) {
	const box = h('div', { class: 'plot', role: 'img', 'aria-label': label, style: { height: `${height}px` } });
	const kept = hidden.get(key);
	for (const t of traces) if (kept?.has(t.name)) t.visible = 'legendonly';
	boxes.set(box, { key, zoom: `${key}@${span}`, name, height, traces, layout, state: 'waiting' });
	watcher ??= new ResizeObserver(seen);
	watcher.observe(box);
	return box;
}

// One watcher for every chart's box, held here so it lives as long as the
// page: one made for each box, held by nothing else, was let go while its
// box was still being built off the page, and the chart never drawn. What
// each box is to draw is in boxes.
const boxes = new WeakMap();
let watcher = null;

// seen is told when a box's size changes: put on the page, laid out again,
// or taken off. A box on the page is drawn the first time it has a width,
// and fitted to it after; one taken off is let go.
function seen(entries) {
	for (const { target: box } of entries) {
		const b = boxes.get(box);
		if (!b) continue;
		if (!box.isConnected) {
			// Not yet on the page, or still being drawn: draw lets it go.
			if (b.state === 'waiting' || b.state === 'drawing') continue;
			watcher.unobserve(box);
			boxes.delete(box);
			if (b.state === 'drawn') window.Plotly.purge(box);
		} else if (box.clientWidth > 0) {
			if (b.state === 'waiting') draw(box, b);
			else if (b.state === 'drawn') window.Plotly.Plots.resize(box);
		}
	}
}

// draw draws a box's chart, loading Plotly first where it is not yet.
async function draw(box, b) {
	b.state = 'drawing';
	let P;
	try {
		P = await plotly();
	} catch (e) {
		b.state = 'failed';
		box.replaceChildren(h('p', { class: 'sub pad' }, `The chart did not load: ${e.message}.`));
		return;
	}
	await P.newPlot(box, b.traces, look(b.layout, b.height), {
		displaylogo: false, responsive: false, modeBarButtonsToRemove: DROPPED,
		toImageButtonOptions: { format: 'png', filename: b.name, scale: 2 },
	});
	if (zoomed.has(b.zoom)) await P.relayout(box, zoomed.get(b.zoom));
	box.on('plotly_relayout', (ev) => keepZoom(b.zoom, ev));
	box.on('plotly_restyle', () => hidden.set(b.key, new Set(box.data.filter((t) => t.visible === 'legendonly').map((t) => t.name))));
	b.state = 'drawn';
	// Taken off the page while it was being drawn: let go now.
	if (!box.isConnected) seen([{ target: box }]);
}

// keepZoom notes an axis's range as it is zoomed or panned, and forgets it
// when the axis is put back.
function keepZoom(zoom, ev) {
	const z = { ...zoomed.get(zoom) };
	for (const [k, v] of Object.entries(ev)) {
		const m = k.match(/^([xy]axis\d*)\.(autorange|range(\[[01]\])?)$/);
		if (!m) continue;
		if (m[2] === 'autorange') {
			for (const o of Object.keys(z)) if (o.startsWith(`${m[1]}.`)) delete z[o];
		} else z[k] = v;
	}
	if (Object.keys(z).length) zoomed.set(zoom, z);
	else zoomed.delete(zoom);
}

// look is layout over the page's own: its colours and type, a legend over
// the chart, times on the x axis, and one hover that says every series at
// the time under the pointer, in the strips under a chart too. What a
// chart's own layout says wins, an axis or the legend field by field.
function look(layout, height) {
	const c = { ink: token('ink'), ink2: token('ink-2'), muted: token('muted'), line: token('line'), line2: token('line-2'), panel: token('panel'), accent: token('accent') };
	const font = getComputedStyle(document.body).fontFamily;
	const axis = { gridcolor: c.line2, linecolor: c.line, zeroline: false, automargin: true, tickfont: { color: c.muted, size: 11 } };
	const axes = {};
	for (const k of Object.keys(layout)) if (/^yaxis\d*$/.test(k)) axes[k] = { ...axis, rangemode: 'tozero', ...layout[k] };
	return {
		height, autosize: true,
		paper_bgcolor: 'rgba(0,0,0,0)', plot_bgcolor: 'rgba(0,0,0,0)',
		font: { family: font, size: 12, color: c.ink2 },
		hovermode: 'x unified', hoversubplots: 'axis',
		modebar: { bgcolor: c.panel, color: c.muted, activecolor: c.accent },
		...layout,
		margin: { l: 8, r: 8, t: 8, b: 8, ...layout.margin },
		hoverlabel: { bgcolor: c.panel, bordercolor: c.line, font: { color: c.ink, family: font }, namelength: -1, ...layout.hoverlabel },
		legend: { orientation: 'h', traceorder: 'normal', x: 0, xanchor: 'left', y: 1.02, yanchor: 'bottom', font: { color: c.ink2 }, ...layout.legend },
		xaxis: {
			type: 'date', ...axis, showspikes: true, spikemode: 'across', spikecolor: c.muted, spikethickness: 1, spikedash: 'dot',
			hoverformat: '%b %-d, %-I:%M %p',
			// Ticks by the clock up to half a day apart, by the date from a
			// day apart.
			tickformatstops: [
				{ dtickrange: [null, 43200000], value: '%-I:%M %p' },
				{ dtickrange: [43200001, null], value: '%b %-d' },
			],
			...layout.xaxis,
		},
		...axes,
	};
}
