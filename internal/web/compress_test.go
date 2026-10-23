package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(h http.Handler, acceptGzip bool, method string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/x", nil)
	if acceptGzip {
		r.Header.Set("Accept-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestGzipResponses(t *testing.T) {
	big := strings.Repeat("<p>Battle Companies</p>\n", 200)
	page := func(ct, body string, status int) http.Handler {
		return gzipResponses(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Content-Length", "99999") // a wrong length must not survive compression
			w.WriteHeader(status)
			io.WriteString(w, body)
		}))
	}
	rec := serve(page("text/html; charset=utf-8", big, 200), true, "GET")
	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Vary") != "Accept-Encoding" || rec.Header().Get("Content-Length") != "" {
		t.Fatalf("headers: %v", rec.Header())
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(zr); string(got) != big || rec.Body.Len() >= len(big)/4 {
		t.Errorf("round trip failed or too big: %d of %d", rec.Body.Len(), len(big))
	}

	for name, c := range map[string]struct {
		h      http.Handler
		accept bool
	}{
		"client without gzip": {page("text/html", big, 200), false},
		"small body":          {page("text/html", "<p>hi</p>", 200), true},
		"event stream":        {page("text/event-stream", big, 200), true},
		"image":               {page("image/png", big, 200), true},
		"not modified":        {page("text/html", "", 304), true},
	} {
		if rec := serve(c.h, c.accept, "GET"); rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s: compressed", name)
		}
	}
	if rec := serve(page("text/html", big, 404), true, "GET"); rec.Code != 404 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("an error page keeps its status and is compressed: %d %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	own := gzipResponses(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Content-Encoding", "gzip")
		io.WriteString(w, big)
	}))
	if rec := serve(own, true, "GET"); rec.Body.String() != big {
		t.Error("a response with its own Content-Encoding is left alone")
	}
	// Flush passes through the compressor.
	flushed := gzipResponses(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, big)
		w.(http.Flusher).Flush()
		io.WriteString(w, big)
	}))
	rec = serve(flushed, true, "GET")
	zr, err = gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(zr); string(got) != big+big || !rec.Flushed {
		t.Errorf("flush: %d bytes, flushed %v", len(got), rec.Flushed)
	}
}

func TestPagesAreGzipped(t *testing.T) {
	f := newFixture(t, Config{})
	r := httptest.NewRequest("GET", "/login", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	rec := f.do(r)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("the login page is not compressed: %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(zr); !strings.Contains(string(b), "<form") {
		t.Error("the decompressed page has no form")
	}
}
