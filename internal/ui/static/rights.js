// What the person signed in may do (0177): the manager's whoami, kept as
// each page is drawn, for views that are not handed the page's context.
// The manager decides; these only keep off the page what it would refuse.

let me = null;
let trees = null;

// know keeps who is signed in, and both trees.
export function know(ctx) {
	me = ctx.who;
	trees = ctx.trees;
}

// mayAt says whether they may do right on a node of a tree: a role granted
// on it or above that holds the right, or being a super admin.
export function mayAt(tree, node, right) {
	if (!me) return false;
	if (me.owner) return true;
	const above = up(tree, node);
	return (me.grants || []).some((g) => g.tree === tree && above.has(g.node) && (me.rights?.[g.role] || []).includes(right));
}

// mayAnywhere says whether they may do right somewhere.
export function mayAnywhere(right) {
	return !!me?.anywhere?.includes(right);
}

// mayHere says whether a node's page allows right: its may, as the
// manager said when it was read.
export function mayHere(page, right) {
	return !!page?.may?.includes(right);
}

// up is a node and the folders above it.
function up(tree, node) {
	const out = new Set();
	for (let n = trees?.[tree]?.nodes.get(node); n; n = trees[tree].nodes.get(n.parent)) out.add(n.id);
	return out;
}

// givesAt is the roles they may give and take on a node (0177): any, for a
// super admin; those their roles there give, for anyone else.
export function givesAt(tree, node) {
	const all = ['admin', 'operator', 'manager', 'installer', 'viewer'];
	if (!me) return [];
	if (me.owner) return all;
	const above = up(tree, node);
	const may = new Set();
	for (const g of me.grants || []) {
		if (g.tree === tree && above.has(g.node)) for (const r of me.gives?.[g.role] || []) may.add(r);
	}
	return all.filter((r) => may.has(r));
}

// runsPeople says whether they have the Admin page: a super admin, or one
// who gives roles or reads sign-ins somewhere.
export function runsPeople() {
	return !!me?.owner || (me?.grants || []).some((g) => (me.gives?.[g.role] || []).length > 0) || mayAnywhere('sign-ins');
}
