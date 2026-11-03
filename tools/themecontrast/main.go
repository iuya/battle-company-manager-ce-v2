// Command themecontrast checks every palette in tailwind.css against WCAG AA (4.5:1 text, 3:1 accent on page), each together with the
// game colours of its mode and contrast (the shared leader, sergeant, hero and path tokens).
//
//	go run ./tools/themecontrast [tailwind.css]
package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
)

type pair struct {
	a, b string
	need float64
}

var pairs = []pair{{"fg", "page", 4.5}, {"fg", "surface", 4.5}, {"fg", "raised", 4.5}, {"muted", "page", 4.5},
	{"muted", "surface", 4.5}, {"muted", "raised", 4.5}, {"accent-fg", "accent", 4.5}, {"accent-text", "page", 4.5},
	{"accent-text", "surface", 4.5}, {"accent-text", "accent-soft", 4.5}, {"danger", "surface", 4.5},
	{"danger", "danger-soft", 4.5}, {"success", "surface", 4.5}, {"success", "success-soft", 4.5},
	{"fg", "accent-soft", 4.5}, {"fg", "danger-soft", 4.5}, {"fg", "success-soft", 4.5}, {"accent", "page", 3.0},
	{"page", "danger", 4.5}, {"warn", "surface", 4.5}, {"warn", "page", 4.5}, {"warn", "raised", 4.5}, {"page", "warn", 4.5},
	{"fg", "leader-soft", 4.5}, {"muted", "leader-soft", 4.5}, {"fg", "sergeant-soft", 4.5}, {"muted", "sergeant-soft", 4.5},
	{"fg", "hero-soft", 4.5}, {"muted", "hero-soft", 4.5}, {"gold", "surface", 3.0}, {"sergeant", "surface", 3.0}, {"hero", "surface", 3.0},
	{"leader", "surface", 4.5}, {"leader", "leader-soft", 3.0}, {"path-melee", "surface", 4.5}, {"path-ranged", "surface", 4.5},
	{"path-magic", "surface", 4.5}}

// The high-contrast variants ([data-contrast="high"]) raise every bar: text to AAA (7:1), the accent, the gold and the place colours to
// text level (4.5:1), and the borders to the non-text minimum (3:1 against surface).
var highBar = map[float64]float64{4.5: 7.0, 3.0: 4.5}
var highExtra = []pair{{"line", "surface", 3.0}, {"line", "page", 3.0}}

var (
	paletteBlock = regexp.MustCompile(`(?s)(\[data-contrast="high"\]\s+)?\[data-palette="(\w+)"\]\[data-mode="(\w+)"\]\s*\{(.*?)\}`)
	modeBlock    = regexp.MustCompile(`(?s)(\[data-contrast="high"\]\s+)?\[data-mode="(\w+)"\]\s*\{(.*?)\}`)
	tokenRE      = regexp.MustCompile(`--([\w-]+):\s*(#[0-9a-fA-F]{6});`)
)

func tokens(body string) map[string]string {
	out := map[string]string{}
	for _, m := range tokenRE.FindAllStringSubmatch(body, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func lum(h string) float64 {
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(h[i:i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(1) + 0.7152*ch(3) + 0.0722*ch(5)
}

func ratio(a, b string) float64 {
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func main() {
	path := "tailwind.css"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	os.Exit(run(path, os.Stdout, os.Stderr))
}

func run(path string, stdout, stderr io.Writer) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(stderr, "themecontrast:", err)
		return 1
	}
	css := string(raw)
	blocks := paletteBlock.FindAllStringSubmatch(css, -1)
	// the game colours are in blocks selected by the mode alone (`[data-mode=...]`, not after another attribute selector)
	type modeKey struct {
		high bool
		mode string
	}
	game := map[modeKey]map[string]string{}
	for _, m := range modeBlock.FindAllStringSubmatchIndex(css, -1) {
		if m[0] > 0 {
			if p := css[m[0]-1]; p == ']' || p == '-' || p == '_' || (p >= '0' && p <= '9') || (p >= 'a' && p <= 'z') || (p >= 'A' && p <= 'Z') {
				continue
			}
		}
		k := modeKey{m[2] >= 0, css[m[4]:m[5]]}
		if game[k] == nil {
			game[k] = map[string]string{}
		}
		for n, v := range tokens(css[m[6]:m[7]]) {
			game[k][n] = v
		}
	}
	bad := 0
	for _, b := range blocks {
		high, pal, mode, body := b[1] != "", b[2], b[3], b[4]
		base, ok := game[modeKey{high, mode}]
		if !ok {
			fmt.Fprintf(stderr, "themecontrast: no game colours for mode %q (high contrast %v)\n", mode, high)
			return 1
		}
		c := map[string]string{}
		for n, v := range base {
			c[n] = v
		}
		for n, v := range tokens(body) {
			c[n] = v
		}
		name := pal + "/" + mode
		list := pairs
		if high {
			name += "/high"
			list = nil
			for _, p := range pairs {
				list = append(list, pair{p.a, p.b, highBar[p.need]})
			}
			list = append(list, highExtra...)
		}
		for _, p := range list {
			ca, okA := c[p.a]
			cb, okB := c[p.b]
			if !okA || !okB {
				fmt.Fprintf(stderr, "themecontrast: %s: token %q or %q is missing\n", name, p.a, p.b)
				return 1
			}
			if r := ratio(ca, cb); r < p.need {
				fmt.Fprintf(stdout, "FAIL %s: %s on %s = %.2f:1 (needs %.1f)\n", name, p.a, p.b, r, p.need)
				bad++
			}
		}
	}
	fmt.Fprintf(stdout, "%d palette/mode combinations, %d failure(s)\n", len(blocks), bad)
	if bad > 0 {
		return 1
	}
	return 0
}
