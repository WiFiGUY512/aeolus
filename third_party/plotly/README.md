# plotly.js

The Clients and Usage charts (0294) are drawn with plotly.js, kept as its authors ship it in `internal/ui/static/vendor/plotly/`:

- `plotly-basic.min.js` from `plotly.js-basic-dist-min` 4.1.2: the basic bundle, with bar, scatter and pie traces.

It is MIT licensed (`LICENSE`). It is unchanged: npm's sha512 for the package (`sha512-LvfKTeVY…Cs2Aw==`) was checked on download, and the file's sha256 is `fa18ed1c94c06576c0a867bb2477534716c39e5b22affe673f2a184317974926`. It was fetched once from the npm registry on 2026-11-03; the manager serves it itself, and nothing fetches it at run time (0069).

`plotly.css` beside it is Plotly's own style rules, which it would add to the page itself: the manager's Content-Security-Policy (`style-src 'self'`) stops it, so they are served as a stylesheet instead. `plotly_css.py` writes it from the bundle, reading the rules as strings and running nothing:

```
python -I third_party/plotly/plotly_css.py internal/ui/static/vendor/plotly/plotly-basic.min.js internal/ui/static/vendor/plotly/plotly.css 4.1.2
```

Run it again whenever the bundle is replaced.
