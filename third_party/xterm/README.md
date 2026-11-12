# xterm.js

The terminal window (0198) draws with xterm.js, kept as its authors ship it in `internal/ui/static/vendor/xterm/`:

- `xterm.js` and `xterm.css` from `@xterm/xterm` 5.5.0;
- `addon-fit.js` from `@xterm/addon-fit` 0.10.0.

Both are MIT licensed (`LICENSE`, `LICENSE-addon-fit`). The only change is that their `sourceMappingURL` lines were taken out, since the maps are not shipped. They were fetched once from npm through jsDelivr on 2026-10-21; the manager serves them itself, and nothing fetches them at run time (0069).
