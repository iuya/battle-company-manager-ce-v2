package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

// The gzip copies are made on first use or by the warm-up, once and the same either way; a file that is not text, or does not get
// smaller, has none.
func TestWarmStaticMakesTheGzipCopies(t *testing.T) {
	set := buildStaticFiles()
	warmStatic(set)
	text := 0
	for name, f := range set {
		gz := f.gz()
		if gz == nil {
			continue
		}
		if !strings.HasPrefix(f.ctype, "text/") && !strings.HasPrefix(f.ctype, "image/svg") {
			t.Errorf("%s (%s) has a gzip copy", name, f.ctype)
		}
		if len(gz) >= len(f.data) {
			t.Errorf("%s: the gzip copy (%d) is not smaller than the file (%d)", name, len(gz), len(f.data))
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, _ := io.ReadAll(zr); !bytes.Equal(got, f.data) {
			t.Errorf("%s: the gzip copy does not hold the file", name)
		}
		text++
	}
	if text == 0 || set["app.css"].gz() == nil || set["js/app.js"].gz() == nil {
		t.Errorf("%d files have a gzip copy; the stylesheet and the script bundle must", text)
	}
	if f := set["fonts/cormorant-garamond-latin-700.woff2"]; f.gz() != nil {
		t.Error("a font has a gzip copy")
	}
}
