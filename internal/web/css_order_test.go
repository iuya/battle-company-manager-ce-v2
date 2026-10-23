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
