package web

import (
	"html/template"
	"testing"
	"time"
)

// BenchmarkZoneSet is what the first request from another time zone costs for the biggest page (aftermath).
func BenchmarkZoneSet(b *testing.B) {
	r, err := newRenderer()
	if err != nil {
		b.Fatal(err)
	}
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		b.Skip("no tzdata")
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.loc.sets = map[string]*template.Template{}
		if _, ok := r.pageIn("aftermath", loc); !ok {
			b.Fatal("no aftermath page")
		}
	}
}

// BenchmarkNewRenderer is the start-up cost of the templates: parsing the layouts and partials and one set per page.
//
// go test ./internal/web -run xxx -bench NewRenderer -benchmem -count 6
func BenchmarkNewRenderer(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := newRenderer(); err != nil {
			b.Fatal(err)
		}
	}
}
