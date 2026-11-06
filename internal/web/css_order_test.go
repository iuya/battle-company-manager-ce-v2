package web

import (
	"os"
	"strings"
	"testing"
)

// The .badge base rule has the same specificity as the badge-* tone classes, so it must come first in tailwind.css: when it came
// last its background and text colour won and every outcome badge looked the same (found by the e2e flow 81-aftermath-ui).
func TestBadgeBaseComesBeforeTones(t *testing.T) {
	b, err := os.ReadFile("../../tailwind.css")
	if err != nil {
		t.Skip("tailwind.css not found")
	}
	css := string(b)
	base := strings.Index(css, "\n  .badge {")
	if base < 0 {
		t.Fatal(".badge rule not found")
	}
	for _, tone := range []string{".badge-gold", ".badge-muted", ".badge-success", ".badge-danger", ".badge-warn"} {
		i := strings.Index(css, "\n  "+tone+" {")
		if i < 0 {
			t.Fatalf("%s rule not found", tone)
		}
		if i < base {
			t.Errorf("%s is defined before .badge, which would override it", tone)
		}
	}
}

// The empty line of a unit summary is kept by pairing an odd card of .unit-choices with the next one, which is a row only while the grid has two columns
// (at most): more columns would need the rule rewritten, and the same breakpoint has to switch both on.
func TestUnitFactsPairMatchesColumns(t *testing.T) {
	b, err := os.ReadFile("../../tailwind.css")
	if err != nil {
		t.Skip("tailwind.css not found")
	}
	css := string(b)
	for _, want := range []string{
		".unit-choices { @apply grid grid-cols-1 gap-3 lg:grid-cols-2; }",
		".unit-choices > :nth-child(odd):has(+ * .unit-facts > *) .unit-facts:not(:has(*)),",
		".unit-choices > :nth-child(odd):has(.unit-facts > *) + * .unit-facts:not(:has(*)) { @apply lg:flex; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("tailwind.css has no %q", want)
		}
	}
}
