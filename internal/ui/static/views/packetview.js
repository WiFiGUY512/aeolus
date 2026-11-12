// The packet viewer (0341): a capture opened in a window that floats over
// the page, as a small Wireshark (Griff, 2026-11-08: "a popout window with
// the packets on it, like a mini wireshark"). The packets in a list, one a
// line; the one picked taken apart into its layers and fields; and its
// bytes, those of the field picked marked. A filter over the list, and the
// file to download for Wireshark itself.

import { h } from '../dom.js';
import { bytes, download } from '../api.js';
import { floatWindow } from '../float.js';
import { readCapture, takeApart } from '../packets.js';

const SIDE = { 127: 'Wi-Fi', 105: 'Wi-Fi', 1: 'Wire', 113: 'Wire' };

// A line that tells of trouble, and one of the key exchange, as Wireshark
// colours them.
function tone(t) {
	// SAE's status 126 says it uses hash-to-element: no failure.
	if (/Deauthentication|Disassociation|status (?!126)[1-9]|NAK|Failure|Server failure|Refused|Format error|RST/.test(t.info)) return 'bad';
	if (t.proto === 'EAPOL' || t.proto === 'EAP' || /Authentication|Association/.test(t.info)) return 'key';
	if (t.proto === 'DHCP' || t.proto === 'ARP') return 'addr';
	return '';
}

// packetsWindow opens the capture at path in the packet viewer: title says
// whose it is; started, where it is known, is the time its attempt began,
// from which its packets are timed, else from its first.
export async function packetsWindow(path, title, started) {
	const box = floatWindow('packets', 'Packets', { width: 1000, height: 640 });
	const filter = h('input', { type: 'search', placeholder: 'Filter: eapol, dhcp, deauth, an address…', 'aria-label': 'Filter the packets' });
	const count = h('span', { class: 'note' });
	const list = h('div', { class: 'pktlist', tabindex: '0' }, h('p', { class: 'sub pad' }, 'Reading the capture…'));
	const tree = h('div', { class: 'pkttree' });
	const dump = h('pre', { class: 'pkthex' });
	const panel = h('section', { class: 'panel packets' },
		h('h2', null, 'Packets', h('span', { class: 'note' }, title || ''), count, h('span', { class: 'grow' }),
			filter,
			h('button', { type: 'button', class: 'button small', title: 'The capture as a pcapng file, for Wireshark', onclick: () => download(path).catch(() => {}) }, 'Download'),
			h('button', { type: 'button', class: 'button small', onclick: () => box.replaceChildren() }, 'Close')),
		list,
		h('div', { class: 'pktlower' }, tree, dump));
	box.replaceChildren(panel);
	let packets;
	try {
		packets = readCapture(await bytes(path)).map((p) => ({ ...p, t2: takeApart(p) }));
	} catch (e) {
		list.replaceChildren(h('p', { class: 'error pad' }, e.message));
		return;
	}
	if (!packets.length) {
		list.replaceChildren(h('p', { class: 'sub pad' }, 'The capture holds no packets.'));
		return;
	}
	const zero = started ? new Date(started).getTime() / 1000 : packets.find((p) => p.t != null)?.t ?? 0;
	const time = (p) => (p.t == null ? '' : `${p.t - zero < 0 ? '−' : ''}${Math.abs(p.t - zero).toFixed(6)}`);
	let picked = null;
	const rows = packets.map((p) => {
		const t = p.t2;
		const tr = h('tr', { class: tone(t), tabindex: '-1' },
			h('td', { class: 'mono' }, String(p.n)),
			h('td', { class: 'mono' }, time(p)),
			h('td', null, SIDE[p.link] || p.link),
			h('td', { class: 'mono' }, t.src),
			h('td', { class: 'mono' }, t.dst),
			h('td', null, t.proto),
			h('td', { class: 'mono' }, String(p.data.length)),
			h('td', { class: 'mono' }, t.signal != null ? `${t.signal}` : ''),
			h('td', { class: 'info' }, t.info, p.comment && h('span', { class: 'sub' }, ` · ${p.comment}`)));
		tr.addEventListener('click', () => pick(p, tr));
		return { p, tr, text: `${t.proto} ${t.info} ${t.src} ${t.dst} ${SIDE[p.link] || ''} ${p.comment || ''} ${t.layers.map((l) => l.name).join(' ')}`.toLowerCase() };
	});
	const table = h('table', { class: 'list dense pkts' },
		h('thead', null, h('tr', null, ['No.', started ? 'Time from start' : 'Time', 'Side', 'Source', 'Destination', 'Protocol', 'Length', 'dBm', 'Info'].map((x) => h('th', null, x)))),
		h('tbody', null, rows.map((r) => r.tr)));
	list.replaceChildren(table);
	const shown = () => {
		const q = filter.value.trim().toLowerCase();
		let n = 0;
		for (const r of rows) {
			r.tr.hidden = !!q && !q.split(/\s+/).every((w) => r.text.includes(w));
			if (!r.tr.hidden) n++;
		}
		count.textContent = q ? `${n} of ${rows.length} packets` : `${rows.length} packets`;
	};
	filter.addEventListener('input', shown);
	shown();

	// The bytes, sixteen a line, each its own so a field's can be marked.
	const cells = [];
	const drawBytes = (d) => {
		cells.length = 0;
		const lines = [];
		for (let i = 0; i < d.length; i += 16) {
			const hexes = [], chars = [];
			for (let j = i; j < Math.min(i + 16, d.length); j++) {
				const x = h('span', null, d[j].toString(16).padStart(2, '0'));
				const c = h('span', null, d[j] >= 32 && d[j] < 127 ? String.fromCharCode(d[j]) : '·');
				cells[j] = [x, c];
				hexes.push(x, j === i + 7 ? '  ' : ' ');
				chars.push(c);
			}
			lines.push(h('div', null, h('span', { class: 'off' }, i.toString(16).padStart(4, '0')), '  ', ...hexes, ' '.repeat(Math.max(0, (16 - Math.min(16, d.length - i)) * 3)), ' ', ...chars));
		}
		dump.replaceChildren(...lines);
	};
	const mark = (off, len) => {
		for (const c of cells) c?.forEach((s) => s.classList.remove('on'));
		for (let j = off; j < off + len; j++) cells[j]?.forEach((s) => s.classList.add('on'));
		cells[off]?.[0].scrollIntoView({ block: 'nearest' });
	};
	let on = null;
	const fieldLine = (f) => {
		const line = h('div', { class: 'pktfield', title: `bytes ${f.off} to ${f.off + f.len - 1}` }, h('span', { class: 'k' }, `${f.name}: `), f.value);
		line.addEventListener('click', (e) => {
			e.stopPropagation();
			on?.classList.remove('on');
			on = line;
			line.classList.add('on');
			mark(f.off, f.len);
		});
		return f.sub ? h('details', { class: 'pktsub' }, h('summary', null, line), f.sub.map(fieldLine)) : line;
	};
	function pick(p, tr) {
		picked?.classList.remove('pktcur');
		picked = tr;
		tr.classList.add('pktcur');
		const t = p.t2;
		const frame = h('details', { class: 'pktlayer' }, h('summary', null, `Frame ${p.n}: ${p.data.length} bytes, ${SIDE[p.link] || `link type ${p.link}`}`),
			p.t != null && h('div', { class: 'pktfield' }, h('span', { class: 'k' }, 'Arrival: '), new Date(p.t * 1000).toLocaleString(undefined, { hour12: false }) + `.${String(Math.round((p.t % 1) * 1e6)).padStart(6, '0')}`),
			p.comment && h('div', { class: 'pktfield' }, h('span', { class: 'k' }, 'Comment: '), p.comment));
		tree.replaceChildren(frame, ...t.layers.map((l) => {
			const d = h('details', { class: 'pktlayer', open: true }, h('summary', null, l.name), l.fields.map(fieldLine));
			d.querySelector('summary').addEventListener('click', () => mark(l.off, l.len));
			return d;
		}));
		drawBytes(p.data);
	}
	// Up and down the list by the keys, as in Wireshark.
	list.addEventListener('keydown', (e) => {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		const seen = rows.filter((r) => !r.tr.hidden);
		const i = seen.findIndex((r) => r.tr === picked);
		const next = seen[Math.max(0, Math.min(seen.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))];
		if (next) {
			e.preventDefault();
			pick(next.p, next.tr);
			next.tr.scrollIntoView({ block: 'nearest' });
		}
	});
	pick(rows[0].p, rows[0].tr);
}
