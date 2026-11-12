// The devices that may be OpenWiFi APs not yet enrolled (0034, 0068), on
// the Org's APs tab. Every AP at a glance (0042) is that tab now (0284).

import { h } from '../dom.js';
import { ago } from '../format.js';

// detected lists the devices that may be unconfigured OpenWiFi APs: possible
// by their DHCP, which asks for options 138 and 224, and confirmed when one
// knocked on the manager's option 224 listener; then the knocks from
// addresses no device is known by.
export function detected(ctx, found) {
	if (!found || (!found.detected.length && !found.knocks.length)) return null;
	const rows = found.detected.map((d) => h('tr', null,
		h('td', null, h('span', { class: 'chip ' + (d.status === 'confirmed' ? 'warn' : 'idle'), title: d.status === 'confirmed' ? 'it knocked on the option 224 listener' : 'its DHCP asks for options 138 and 224' }, d.status)),
		h('td', null, h('span', { class: 'mono' }, d.mac), h('div', { class: 'sub' }, [d.private ? 'private MAC' : d.maker, d.host].filter(Boolean).join(' · '))),
		h('td', null, h('span', { class: 'mono' }, d.address || '—'),
			h('div', { class: 'sub' }, d.seen_by === 'relay' ? `relayed from ${d.subnet}` : `a Wi-Fi client of ${ctx.name('locations', d.seen_by)}`)),
		h('td', null, [
			d.fingerprint && 'asks for 138 and 224',
			d.knocks > 0 && `knocked ${d.knocks === 1 ? 'once' : `${d.knocks} times`}${d.sni ? ` for ${d.sni}` : ''}`,
		].filter(Boolean).join('; ')),
		h('td', null, ago(d.last))));
	const knocks = found.knocks.map((k) => h('tr', null,
		h('td', null, h('span', { class: 'chip idle', title: 'no device is known by this address' }, 'knock')),
		h('td', null, '—'),
		h('td', { class: 'mono' }, k.source),
		h('td', null, `knocked ${k.count === 1 ? 'once' : `${k.count} times`}${k.sni ? ` for ${k.sni}` : ''}`, k.subject && h('div', { class: 'sub' }, k.subject)),
		h('td', null, ago(k.last))));
	return h('section', { class: 'panel' },
		h('h2', null, 'Detected, not enrolled', h('span', { class: 'note' }, 'devices that may be OpenWiFi APs, from relayed DHCP and the option 224 listener')),
		h('table', { class: 'list' },
			h('tr', null, ['Status', 'Device', 'Address', 'Evidence', 'Last seen'].map((c) => h('th', null, c))),
			rows, knocks));
}
