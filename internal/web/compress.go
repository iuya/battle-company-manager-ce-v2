package web

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

// gzipMinSize is the smallest first write worth compressing: below it the gzip header and the CPU cost outweigh the saving.
const gzipMinSize = 1024

var gzipPool = sync.Pool{New: func() any { zw, _ := gzip.NewWriterLevel(nil, gzipLevel); return zw }}

// gzipLevel trades CPU for size: a page of 200 KB takes a few milliseconds at this level and shrinks about tenfold.
const gzipLevel = 5

// compressible says whether a response with this Content-Type is text worth compressing (images, fonts and the event stream are not).
func compressible(ct string) bool {
	ct = strings.ToLower(ct)
	for _, p := range []string{"text/html", "text/css", "text/javascript", "text/plain", "application/json", "application/javascript", "image/svg+xml", "text/csv"} {
		if strings.HasPrefix(ct, p) {
			return true
		}
	}
	return false
}

// gzipResponses compresses the pages and fragments for a browser that accepts gzip. It decides at the first write: a response that
// already has a Content-Encoding (the static files carry their own), is not text, is small, has no body or answers a Range request goes
// through untouched, and so does the live event stream (it is flushed event by event and is text/event-stream anyway).
func gzipResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.finish()
		next.ServeHTTP(gw, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	zw      *gzip.Writer
	decided bool
	sent    bool // the status line and headers went to the real writer
	status  int
}

// decide runs once, at the first write, with its size: the headers are still ours to change because WriteHeader only records the status.
func (g *gzipWriter) decide(first int) {
	if g.decided {
		return
	}
	g.decided = true
	h := g.Header()
	if h.Get("Content-Encoding") != "" || g.status == http.StatusNoContent || g.status == http.StatusNotModified || g.status < 200 && g.status != 0 ||
		!compressible(h.Get("Content-Type")) || first < gzipMinSize {
		return
	}
	h.Add("Vary", "Accept-Encoding")
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	g.zw = gzipPool.Get().(*gzip.Writer)
	g.zw.Reset(g.ResponseWriter)
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.status == 0 {
		g.status = code
	}
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		g.ResponseWriter.WriteHeader(code) // informational: no decision yet
		g.status = 0
	}
}

// send passes the recorded status on, once.
func (g *gzipWriter) send() {
	if !g.sent {
		g.sent = true
		if g.status != 0 {
			g.ResponseWriter.WriteHeader(g.status)
		}
	}
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	g.decide(len(b))
	g.send()
	if g.zw != nil {
		return g.zw.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// Flush sends what is buffered so far (the compressed bytes too). A stream that flushes before writing is not compressed.
func (g *gzipWriter) Flush() {
	g.decide(0)
	g.send()
	if g.zw != nil {
		g.zw.Flush()
	}
	_ = http.NewResponseController(g.ResponseWriter).Flush() // reaches the real writer through the wrappers' Unwrap
}

func (g *gzipWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipWriter) finish() {
	g.decide(0)
	g.send()
	if g.zw != nil {
		g.zw.Close()
		g.zw.Reset(nil)
		gzipPool.Put(g.zw)
		g.zw = nil
	}
}
