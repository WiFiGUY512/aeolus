// The page's theme (0296), set before the page is drawn, so a dark page
// never shows light first: data-theme on <html> is light or dark, as this
// browser was set (themes.js keeps it), or where it was not, as the device
// is, following it as it changes. A script of its own and not a module: a
// module runs only once the page is drawn.
(function () {
	var dark = window.matchMedia('(prefers-color-scheme: dark)');
	function set() {
		var t = null;
		try {
			t = localStorage.getItem('aeolus.theme');
		} catch (e) {
			t = null;
		}
		document.documentElement.setAttribute('data-theme', t === 'light' || t === 'dark' ? t : dark.matches ? 'dark' : 'light');
	}
	set();
	dark.addEventListener('change', set);
}());
