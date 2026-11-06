package web

import (
	"testing"
	"time"
)

// A page's set for another viewer zone is built on first use from the layouts and partials and the page's file, and is kept.
func TestPageInBuildsAZoneSetOnDemand(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Fatal(err)
	}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("no tzdata")
	}
	for name, utc := range r.pages {
		set, ok := r.pageIn(name, madrid)
		if !ok || set == nil || set == utc {
			t.Fatalf("%s: no own set for Europe/Madrid", name)
		}
		if again, _ := r.pageIn(name, madrid); again != set {
			t.Fatalf("%s: the zone set was not kept", name)
		}
		for _, n := range []string{"layout", "content", "title"} {
			if (utc.Lookup(n) == nil) != (set.Lookup(n) == nil) {
				t.Fatalf("%s: template %q differs between the UTC set and the zone set", name, n)
			}
		}
	}
	if got, ok := r.pageIn("no-such-page", madrid); ok || got != nil {
		t.Fatal("an unknown page has a set")
	}
}
