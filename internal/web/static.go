package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

// The embedded static files are read once: every file gets a content hash (its ETag and the ?v= of its URL), the stylesheets get
// the same ?v= on the files they load (fonts, images), and the text files are compressed once, on first need (or by WarmStatic after the
// start), not at the start. A URL with the current ?v= never changes its content, so the browser keeps it for a year and a page costs no
// request for it (docs/performance.md).
type staticFile struct {
	data  []byte
	gz    func() []byte // the gzip copy (best compression), made once; nil result: the file is not text or does not get smaller
	br    func() []byte // the brotli copy (level 11), made on the first request that accepts it and kept
	etag  string        // quoted, strong
	ver   string        // first 10 hex digits of the content hash
	ctype string
}

type staticSet map[string]*staticFile

// bundleName is the one script the pages load (deferred) next to the two small ones of the head; the files it holds stay available on
// their own addresses. They are plain scripts that share the page's global scope; a throw while one loads also skips the
// ones after it, which the end-to-end tests would show.
const bundleName = "js/app.js"

var bundleScripts = []string{
	"js/htmx.min.js", "js/theme.js", "js/match_form.js", "js/ruleset_picker.js", "js/lists.js", "js/sortable.min.js",
	"js/htmx_ready.js", "js/dice.js", "js/company.js", "js/live.js", "js/dialog.js", "js/concepts.js", "js/submit.js", "js/tabs.js", "js/tts.js", "js/towound.js", "js/copylink.js", "js/alpine.min.js",
}

var staticFiles = sync.OnceValue(buildStaticFiles)

func buildStaticFiles() staticSet {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	set := staticSet{}
	var css []string
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		set[p] = &staticFile{data: b, ctype: contentType(p)}
		if strings.HasSuffix(p, ".css") {
			css = append(css, p)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	// One request instead of fourteen: the scripts of every page, in the order the pages need them (Alpine last).
	var bundle []byte
	for _, p := range bundleScripts {
		f, ok := set[p]
		if !ok {
			panic("static bundle: no file " + p)
		}
		bundle = append(append(bundle, f.data...), "\n;\n"...)
	}
	set[bundleName] = &staticFile{data: bundle, ctype: contentType(bundleName)}
	for p, f := range set {
		if !strings.HasSuffix(p, ".css") {
			f.seal()
		}
	}
	// Files a stylesheet loads get their version in the url, so the stylesheet's own hash covers them too.
	urlRe := regexp.MustCompile(`url\("/static/([^"?]+)"\)`)
	for _, p := range css {
		f := set[p]
		f.data = urlRe.ReplaceAllFunc(f.data, func(m []byte) []byte {
			name := urlRe.FindSubmatch(m)[1]
			if t, ok := set[string(name)]; ok {
				return []byte(`url("/static/` + string(name) + `?v=` + t.ver + `")`)
			}
			return m
		})
		f.seal()
	}
	return set
}

func contentType(p string) string {
	switch path.Ext(p) {
	case ".woff2":
		return "font/woff2"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	}
	if t := mime.TypeByExtension(path.Ext(p)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// seal computes the hash and sets up the compressed copies, which are made on first use (kept only when smaller than the file).
func (f *staticFile) seal() {
	sum := sha256.Sum256(f.data)
	h := hex.EncodeToString(sum[:])
	f.ver = h[:10]
	f.etag = `"` + h[:20] + `"`
	data := f.data
	f.gz = sync.OnceValue(func() []byte {
		if !strings.HasPrefix(f.ctype, "text/") && !strings.HasPrefix(f.ctype, "image/svg") {
			return nil
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		zw.Write(data)
		zw.Close()
		if buf.Len() < len(data) {
			return buf.Bytes()
		}
		return nil
	})
	f.br = sync.OnceValue(func() []byte {
		var b bytes.Buffer
		bw := brotli.NewWriterLevel(&b, brotli.BestCompression)
		bw.Write(data)
		bw.Close()
		return b.Bytes()
	})
}

// WarmStatic makes the gzip copies of the static files in the background, so the first visit does not wait for them. The start does not:
// it is ready to serve before they are. The brotli copies stay on demand (level 11 takes seconds of CPU for the whole set, and only
// browsers on HTTPS ask for them).
func WarmStatic() { go warmStatic(staticFiles()) }

// warmStatic makes the gzip copies of the set, side by side, and returns when they are all made.
func warmStatic(set staticSet) {
	var wg sync.WaitGroup
	for _, f := range set {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.gz()
		}()
	}
	wg.Wait()
}

// staticURL is the address of a static file with its version, for the templates: {{static "js/htmx.min.js"}}.
func staticURL(p string) string {
	if f, ok := staticFiles()[p]; ok {
		return "/static/" + p + "?v=" + f.ver
	}
	return "/static/" + p
}

// staticHandler serves the files (the path is relative to static/). Unversioned requests keep the configured lifetime (no-cache by
// default: the browser asks again and gets a 304 through the ETag); a request with the file's current ?v= is immutable.
func (s *Server) staticHandler() http.Handler {
	files := staticFiles()
	unversioned := "no-cache"
	if s.cfg.StaticMaxAge > 0 {
		unversioned = "public, max-age=" + itoa(int(s.cfg.StaticMaxAge.Seconds()))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", f.ctype)
		if r.URL.Query().Get("v") == f.ver {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", unversioned)
		}
		body, etag := f.data, f.etag
		if gz := f.gz(); gz != nil {
			h.Add("Vary", "Accept-Encoding")
			switch pickEncoding(r) {
			case "br":
				body, etag = f.br(), strings.TrimSuffix(f.etag, `"`)+`-br"`
				h.Set("Content-Encoding", "br")
			case "gzip":
				body, etag = gz, strings.TrimSuffix(f.etag, `"`)+`-gz"`
				h.Set("Content-Encoding", "gzip")
			}
		}
		h.Set("ETag", etag)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
	})
}

// accepts reports whether the request lists the content coding among its accepted encodings (q=0 refuses it).
func accepts(r *http.Request, coding string) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(name), coding) {
			params = strings.ReplaceAll(params, " ", "")
			return !strings.Contains(params, "q=0") || strings.Contains(params, "q=0.")
		}
	}
	return false
}

// pickEncoding is the compression to use for the answer: "br" when the client accepts it (browsers offer it on HTTPS only), else "gzip", else "".
func pickEncoding(r *http.Request) string {
	switch {
	case accepts(r, "br"):
		return "br"
	case accepts(r, "gzip"):
		return "gzip"
	}
	return ""
}
