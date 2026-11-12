// Package ui serves the web UI (0042): plain HTML, CSS and JavaScript
// modules embedded in the binary, with no build step. The UI reads and
// changes the manager only through the API, with the person's own token.
package ui

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed static
var static embed.FS

// csp lets the page load scripts, styles and data only from the manager
// itself, and keeps it out of frames (0042).
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// cspTerminal is the terminal window's (0198): the same, but for the
// styles xterm.js writes into the page as it draws, which it cannot do
// without. Scripts are still the manager's own alone.
const cspTerminal = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// types are the only kinds of file the UI ships. A fixed table, because
// mime.TypeByExtension reads the host's settings, and some map .js to
// text/plain, which browsers refuse to run as a module.
var types = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".svg":  "image/svg+xml",
}

// Handler serves the UI: the page at / to browsers, its files under /ui/.
// Every other request, and / asked for as JSON, goes to next.
func Handler(next http.Handler) http.Handler {
	files, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the embedded directory is part of the build
	}
	assets, err := load(files)
	if err != nil {
		panic(err) // so are its files
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && wantsHTML(r):
			serve(w, r, assets, "index.html")
		case strings.HasPrefix(r.URL.Path, "/ui/"):
			serve(w, r, assets, strings.TrimPrefix(r.URL.Path, "/ui/"))
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// asset is one of the UI's files as it is sent (0211): its body, the same
// gzipped where that is smaller, its type, and its ETag, a hash of the body.
// They are made once, as the manager starts.
type asset struct {
	body, gz []byte
	ctype    string
	etag     string
}

// load makes every file of the UI ready to send (0211). The page lists, as
// modulepreload links, every module app.js imports however deep, so the
// browser asks for them all at once, not each only once the module that
// imports it has come.
func load(files fs.FS) (map[string]*asset, error) {
	out := map[string]*asset{}
	err := fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		out[name] = &asset{body: b, ctype: types[path.Ext(name)]}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if page := out["index.html"]; page != nil {
		page.body = preload(page.body, imports(out, "app.js"))
	}
	for _, a := range out {
		sum := sha256.Sum256(a.body)
		a.etag = `"` + hex.EncodeToString(sum[:12]) + `"`
		var gz bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		zw.Write(a.body)
		zw.Close()
		if gz.Len() < len(a.body) {
			a.gz = gz.Bytes()
		}
	}
	return out, nil
}

// importRE finds a module's static imports of the UI's own modules.
var importRE = regexp.MustCompile(`(?:from|import)\s*'(\.{1,2}/[^']+\.js)'`)

// imports is every module root imports, however deep, but root itself,
// sorted.
func imports(assets map[string]*asset, root string) []string {
	seen := map[string]bool{root: true}
	queue := []string{root}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		a := assets[name]
		if a == nil {
			continue
		}
		for _, m := range importRE.FindAllStringSubmatch(string(a.body), -1) {
			target := path.Join(path.Dir(name), m[1])
			if !seen[target] && assets[target] != nil {
				seen[target] = true
				queue = append(queue, target)
			}
		}
	}
	delete(seen, root)
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// preload puts a modulepreload link for each of modules before the page's
// module script, where it has one.
func preload(page []byte, modules []string) []byte {
	const script = `<script type="module" src="/ui/app.js"></script>`
	i := bytes.Index(page, []byte(script))
	if i < 0 || len(modules) == 0 {
		return page
	}
	var links bytes.Buffer
	for _, m := range modules {
		links.WriteString(`<link rel="modulepreload" href="/ui/` + m + "\">\n")
	}
	return append(append(append([]byte{}, page[:i]...), links.Bytes()...), page[i:]...)
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// serve writes one embedded file. There are no directory listings. A
// browser asks again each time (no-cache), and while the file is the same,
// by its ETag, is told so in a few bytes (304), not sent it again; a new
// release is new files, so nothing old is ever kept. Gzipped where the
// browser takes it (0211).
func serve(w http.ResponseWriter, r *http.Request, assets map[string]*asset, name string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name = path.Clean("/" + name)[1:]
	a := assets[name]
	if a == nil || name == "" {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	if name == "terminal.html" {
		h.Set("Content-Security-Policy", cspTerminal)
	} else {
		h.Set("Content-Security-Policy", csp)
	}
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", a.etag)
	h.Set("Vary", "Accept-Encoding")
	if a.ctype != "" {
		h.Set("Content-Type", a.ctype)
	}
	if strings.Contains(r.Header.Get("If-None-Match"), a.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := a.body
	if a.gz != nil && acceptsGzip(r) {
		h.Set("Content-Encoding", "gzip")
		body = a.gz
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodGet {
		w.Write(body)
	}
}

// acceptsGzip says whether the request's Accept-Encoding takes gzip.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
			return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
		}
	}
	return false
}
