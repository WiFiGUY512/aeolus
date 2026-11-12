// Libraries of which a process holds two builds at once (0317). On
// 2026-11-05 the late restore after a sysupgrade put OpenWrt's snapshot
// builds of uclient, udebug, udebugd, usteer and ustp in, against
// libubox20261103, beside the image's libubox20260721. Each agent then held
// both, two uloops, and waited for good on a socket in the other one; so did
// usteerd, ustpd and udebugd. A library's build is its soname's suffix, as
// the 20260721 of libubox.so.20260721: OpenWrt names each ABI so.

'use strict';

const MOST = 8;    // the most libraries said
const PROCS = 16;  // the most processes said of one

// of_maps is the builds of each library a process's maps hold, by its name:
// { 'libubox.so': [ '20260721', '20261103' ] }. One whose file was taken
// away since, as a package replaced under a running daemon, is still held,
// and the kernel says "(deleted)" after its path.
function of_maps(maps) {
	let out = {};
	for (let m in match(maps ?? '', /\/(lib[^\/ \t\n]+\.so)\.([0-9][0-9.]*)( \(deleted\))?(\n|$)/g) ?? []) {
		let b = out[m[1]] ??= [];
		if (index(b, m[2]) < 0)
			push(b, m[2]);
	}
	return out;
}

// two is whether maps hold two builds of some library.
function two(maps) {
	for (let lib, builds in of_maps(maps))
		if (length(builds) > 1)
			return true;
	return false;
}

// mixed is, of the processes given as { name, maps }, each library one of
// them holds two builds of: { lib, builds, procs }, by the library's name,
// the builds and the processes sorted, the first MOST and PROCS of them.
function mixed(procs) {
	let found = {};
	for (let p in procs ?? [])
		for (let lib, builds in of_maps(p.maps)) {
			if (length(builds) < 2)
				continue;
			let f = found[lib] ??= { lib: lib, builds: [], procs: [] };
			for (let b in builds)
				if (index(f.builds, b) < 0)
					push(f.builds, b);
			if (index(f.procs, p.name) < 0)
				push(f.procs, p.name);
		}
	let out = [];
	for (let lib in sort(keys(found))) {
		let f = found[lib];
		f.builds = sort(f.builds);
		f.procs = slice(sort(f.procs), 0, PROCS);
		push(out, f);
	}
	return slice(out, 0, MOST);
}

// name_of is how a process is known, from its /proc cmdline: the script
// ucode runs, else its program; null for a kernel thread, which has none.
function name_of(cmdline) {
	let argv = filter(split(cmdline ?? '', '\0'), a => length(a));
	if (!length(argv))
		return null;
	let base = a => replace(a, /^.*\//, '');
	if (base(argv[0]) == 'ucode')
		for (let i = 1; i < length(argv); i++)
			if (substr(argv[i], 0, 1) != '-')
				return base(argv[i]);
	return base(argv[0]);
}

export { MOST, PROCS, of_maps, two, mixed, name_of };
