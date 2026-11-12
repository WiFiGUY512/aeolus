package ui

import (
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func serveUI(t *testing.T, method, path, accept string) *http.Response {
	t.Helper()
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "api")
	})
	req := httptest.NewRequest(method, path, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	Handler(api).ServeHTTP(rec, req)
	return rec.Result()
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBrowsersGetThePageAndClientsGetTheAPI(t *testing.T) {
	r := serveUI(t, "GET", "/", "text/html,application/xhtml+xml")
	if r.StatusCode != 200 || !strings.Contains(body(t, r), `src="/ui/app.js"`) {
		t.Fatalf("page: %d", r.StatusCode)
	}
	if csp := r.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP = %q", csp)
	}
	for _, h := range []string{"X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options"} {
		if r.Header.Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	for _, c := range []struct{ path, accept string }{{"/", "application/json"}, {"/", ""}, {"/v1/whoami", "text/html"}, {"/healthz", ""}} {
		if got := body(t, serveUI(t, "GET", c.path, c.accept)); got != "api" {
			t.Errorf("%s (%s) went to the UI: %q", c.path, c.accept, got)
		}
	}
}

func TestFiles(t *testing.T) {
	for path, typ := range map[string]string{
		"/ui/app.js":        "text/javascript",
		"/ui/views/tree.js": "text/javascript",
		"/ui/app.css":       "text/css",
	} {
		r := serveUI(t, "GET", path, "")
		if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), typ) || r.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: %d %q", path, r.StatusCode, r.Header.Get("Content-Type"))
		}
	}
	for _, path := range []string{"/ui/", "/ui/views", "/ui/views/", "/ui/nope.js", "/ui/../ui.go", "/ui/%2e%2e/ui.go"} {
		if r := serveUI(t, "GET", path, ""); r.StatusCode != 404 {
			t.Errorf("%s: %d, want 404", path, r.StatusCode)
		}
	}
	if r := serveUI(t, "POST", "/ui/app.js", ""); r.StatusCode != 405 {
		t.Errorf("POST: %d", r.StatusCode)
	}
}

// A file is sent once, then answered 304 by its ETag while it is the same;
// gzipped to a browser that takes it; and the page lists every module
// app.js imports for preloading, and nothing it does not (0211).
func TestCachingCompressionAndPreload(t *testing.T) {
	get := func(path string, header map[string]string) *http.Response {
		req := httptest.NewRequest("GET", path, nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		Handler(http.NotFoundHandler()).ServeHTTP(rec, req)
		return rec.Result()
	}
	first := get("/ui/format.js", nil)
	etag := first.Header.Get("ETag")
	plain := body(t, first)
	if first.StatusCode != 200 || len(etag) < 10 || first.Header.Get("Cache-Control") != "no-cache" || first.Header.Get("Content-Encoding") != "" {
		t.Fatalf("first: %d etag %q cache %q encoding %q", first.StatusCode, etag, first.Header.Get("Cache-Control"), first.Header.Get("Content-Encoding"))
	}
	again := get("/ui/format.js", map[string]string{"If-None-Match": etag})
	if again.StatusCode != 304 || body(t, again) != "" {
		t.Errorf("again: %d", again.StatusCode)
	}
	if other := get("/ui/format.js", map[string]string{"If-None-Match": `"0123456789abcdef01234567"`}); other.StatusCode != 200 {
		t.Errorf("another ETag: %d", other.StatusCode)
	}
	z := get("/ui/format.js", map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	if z.Header.Get("Content-Encoding") != "gzip" || z.Header.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("gzip: encoding %q vary %q", z.Header.Get("Content-Encoding"), z.Header.Get("Vary"))
	}
	zr, err := gzip.NewReader(z.Body)
	if err != nil {
		t.Fatal(err)
	}
	unzipped, err := io.ReadAll(zr)
	if err != nil || string(unzipped) != plain {
		t.Errorf("gzipped body is not the file: %v", err)
	}
	if n, _ := strconv.Atoi(z.Header.Get("Content-Length")); n == 0 || n >= len(plain) {
		t.Errorf("gzipped length %d of %d", n, len(plain))
	}
	if r := get("/ui/format.js", map[string]string{"Accept-Encoding": "gzip;q=0"}); r.Header.Get("Content-Encoding") != "" {
		t.Errorf("gzip;q=0 was gzipped")
	}
	page := body(t, get("/", map[string]string{"Accept": "text/html"}))
	for _, m := range []string{"format.js", "dom.js", "views/tree.js", "views/edit.js"} {
		if !strings.Contains(page, `<link rel="modulepreload" href="/ui/`+m+`">`) {
			t.Errorf("the page does not preload %s", m)
		}
	}
	for _, m := range []string{"app.js\">", "terminal.js"} {
		if strings.Contains(page, `rel="modulepreload" href="/ui/`+m) {
			t.Errorf("the page preloads %s", m)
		}
	}
	if !strings.Contains(page, `<script type="module" src="/ui/app.js"></script>`) || strings.Index(page, "modulepreload") > strings.Index(page, `src="/ui/app.js"`) {
		t.Errorf("the links are not before the page's script:\n%s", page)
	}
}

// Every module the page imports is shipped, and the page builds content
// only as text (0042): no innerHTML, no eval.
func TestModules(t *testing.T) {
	importRE := regexp.MustCompile(`from '(\.{1,2}/[^']+)'`)
	err := fs.WalkDir(static, "static", func(path string, d fs.DirEntry, err error) error {
		// Libraries kept as their authors ship them (0198, 0294) are not
		// the page's own code; they are given only what the page draws.
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") || strings.HasPrefix(path, "static/vendor/") {
			return err
		}
		src, err := fs.ReadFile(static, path)
		if err != nil {
			return err
		}
		for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "eval(", "new Function"} {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s uses %s", path, bad)
			}
		}
		dir := path[:strings.LastIndex(path, "/")]
		for _, m := range importRE.FindAllStringSubmatch(string(src), -1) {
			target := clean(dir + "/" + m[1])
			if _, err := fs.Stat(static, target); err != nil {
				t.Errorf("%s imports %s, which is not shipped", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The page's own h() leaves out a child that is false or null, but the
// DOM's replaceChildren, append and prepend write one as a word: "false"
// stood beside an AP's ports where its Add a port button was not shown, and
// "null" in a panel before that. So what is given to them is never one that
// may be false: no "x && el" among their arguments, unless it is in a list
// that is filtered and spread.
func TestNothingFalseIsWrittenAsAWord(t *testing.T) {
	call := regexp.MustCompile(`\.(replaceChildren|append|prepend)\(`)
	err := fs.WalkDir(static, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") {
			return err
		}
		raw, err := fs.ReadFile(static, path)
		if err != nil {
			return err
		}
		src := string(raw)
		for _, m := range call.FindAllStringIndex(src, -1) {
			// The call's arguments, each as what stands outside any
			// bracket or quote of its own.
			depth, quote, arg := 1, byte(0), ""
			var args []string
			for i := m[1]; i < len(src) && depth > 0; i++ {
				c := src[i]
				switch {
				case quote != 0:
					if c == '\\' {
						i++
					} else if c == quote {
						quote = 0
					}
				case c == '\'' || c == '"' || c == '`':
					quote = c
				case c == '(' || c == '[' || c == '{':
					depth++
				case c == ')' || c == ']' || c == '}':
					depth--
				case c == ',' && depth == 1:
					args = append(args, arg)
					arg = ""
				case depth == 1:
					arg += string(c)
				}
			}
			for _, a := range append(args, arg) {
				if a = strings.TrimSpace(a); strings.Contains(a, "&&") && !strings.HasPrefix(a, "...") {
					t.Errorf("%s:%d: an argument that may be false is written as a word: %.60s", path, 1+strings.Count(src[:m[0]], "\n"), a)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func clean(p string) string {
	var out []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case ".", "":
		case "..":
			out = out[:len(out)-1]
		default:
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}
