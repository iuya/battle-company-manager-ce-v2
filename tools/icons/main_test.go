package main

import (
	"strings"
	"testing"
)

const fakeTrace = `<svg viewBox="0 0 574.000000 574.000000" xmlns="http://www.w3.org/2000/svg">
<g transform="translate(0,574) scale(0.1,-0.1)"><path d="M10 20
 c 1  2 3 4" fill="#000"/><path d="M5 5 z"/></g></svg>`

func TestParseTrace(t *testing.T) {
	d, size, err := parseTrace(fakeTrace)
	if err != nil {
		t.Fatal(err)
	}
	if d != "M10 20 c 1 2 3 4 M5 5 z" || size != 574 {
		t.Fatalf("got %q, %v", d, size)
	}
}

func TestParseTraceRejectsOtherSVG(t *testing.T) {
	if _, _, err := parseTrace(`<svg width="10"></svg>`); err == nil {
		t.Fatal("an SVG without a viewBox must be an error")
	}
}

func TestIconMarkup(t *testing.T) {
	got := icon("M1 1", 574, 48, 3)
	want := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" role="img" aria-label="Battle Companies 2nd CE Manager">` + "\n" +
		`<g transform="translate(3 3) scale(0.10105) translate(0 574.0) scale(.1 -.1)" fill="#c9a032" stroke="#c9a032" stroke-width="48" stroke-linejoin="round"><path d="M1 1"/></g>` + "\n</svg>\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRingMask(t *testing.T) {
	got := ringMask("M1 1", 2400)
	if !strings.Contains(got, `viewBox="0 0 2400 2400"`) || !strings.Contains(got, `translate(0 2400)`) || !strings.HasSuffix(got, "</svg>\n") {
		t.Fatalf("unexpected %s", got)
	}
}

func TestNumberFormats(t *testing.T) {
	for in, want := range map[float64]string{3: "3", 574: "574", 1.5: "1.5", 1234567: "1.23457e+06"} {
		if got := g(in); got != want {
			t.Errorf("g(%v) = %s, want %s", in, got, want)
		}
	}
	if dec(574) != "574.0" || dec(0.25) != "0.25" {
		t.Errorf("py: %s %s", dec(574), dec(0.25))
	}
}

func TestRunNeedsTheSourceImage(t *testing.T) {
	if err := run(t.TempDir()); err == nil {
		t.Fatal("a folder without img/ring_text.png must be an error")
	}
}
