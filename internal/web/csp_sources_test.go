package web

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The policy (hardening.go) allows no inline script and no inline event handler: everything that runs is a file under /static/js,
// or an Alpine/htmx attribute expression (which needs 'unsafe-eval' only). This keeps the templates and the scripts inside it, so
// that CSP_MODE=enforce stays free of violations (docs/operations.md, "Content-Security-Policy"; the e2e suite runs under it in CI).
func TestTemplatesAndScriptsFitTheContentSecurityPolicy(t *testing.T) {
	scriptTag := regexp.MustCompile(`(?is)<script\b[^>]*>`)
	handler := regexp.MustCompile(`(?i)\s(on[a-z]+)\s*=\s*["']`) // onclick=, onload=... (hx-on:: and @click are not preceded by whitespace + "on")
	jsURL := regexp.MustCompile(`(?i)(href|src|action|formaction)\s*=\s*["']\s*javascript:`)
	external := regexp.MustCompile(`(?i)\b(src|href)\s*=\s*["']https?://`)
	err := fs.WalkDir(assets, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(assets, p)
		if err != nil {
			return err
		}
		s := string(b)
		for _, tag := range scriptTag.FindAllString(s, -1) {
			if !strings.Contains(strings.ToLower(tag), "src=") {
				t.Errorf("%s: inline <script> %q (the policy blocks it; move it to static/js)", p, tag)
			}
		}
		if m := handler.FindString(s); m != "" {
			t.Errorf("%s: inline event handler attribute %q (the policy blocks it; use @click or hx-on)", p, strings.TrimSpace(m))
		}
		if jsURL.MatchString(s) {
			t.Errorf("%s: javascript: URL", p)
		}
		if m := external.FindString(s); m != "" {
			t.Errorf("%s: external resource %q (default-src is 'self')", p, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The helper popup builds DOM and inserts the server's markup: it must not add what the policy blocks (an inline style attribute, an
// inline handler, eval) and it must ship in the bundle. The sheet is positioned through the style property (CSSOM), which is allowed.
func TestHelperPopupScriptFitsThePolicy(t *testing.T) {
	b, err := fs.ReadFile(assets, "static/js/concepts.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, bad := range []string{"setAttribute('style'", `setAttribute("style"`, "setAttribute('on", `setAttribute("on`, "eval(", "new Function", "javascript:", "insertAdjacentHTML", "document.write", "outerHTML"} {
		if strings.Contains(src, bad) {
			t.Errorf("concepts.js uses %q", bad)
		}
	}
	if strings.Count(src, "innerHTML") != 1 {
		t.Error("concepts.js must insert markup in one place only: the popup html the server made")
	}
	if !slices.Contains(bundleScripts, "js/concepts.js") {
		t.Error("concepts.js is not in bundleScripts")
	}
	part, err := fs.ReadFile(assets, "templates/partials/concept.gohtml")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?i)\sstyle\s*=`).Match(part) {
		t.Error("the popup partial has an inline style attribute")
	}
}
