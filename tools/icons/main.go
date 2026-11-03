// Command icons builds the favicon and app icons from the One Ring inscription image (internal/web/static/img/ring_text.png).
//
// It needs ImageMagick (`magick`) and potrace on the PATH:  brew install imagemagick potrace
//
// The inscription is traced to vector paths and set in gold on a transparent background (it must read on light and dark pages and tabs).
// It also writes img/ring-text.svg, the page background. Two icon sizes are produced: a bold, low-detail one for tabs (favicon.svg) and a
// finer one for large uses (icon.svg, app icons).
//
//	go run ./tools/icons [-static internal/web/static]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
)

const gold = "#c9a032" // dark enough for light backgrounds, bright enough for dark ones

var (
	viewBox  = regexp.MustCompile(`viewBox="0 0 ([\d.]+) `)
	pathData = regexp.MustCompile(` d="([^"]*)"`)
)

func main() {
	static := flag.String("static", "internal/web/static", "the static folder holding img/ring_text.png; the icons are written into it")
	flag.Parse()
	dir := *static
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repo.Root(), dir)
	}
	if err := run(dir); err != nil {
		fmt.Fprintln(os.Stderr, "icons:", err)
		os.Exit(1)
	}
}

func run(static string) error {
	for _, tool := range []string{"magick", "potrace"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is not on the PATH (brew install imagemagick potrace)", tool)
		}
	}
	src := filepath.Join(static, "img", "ring_text.png")
	if _, err := os.Stat(src); err != nil {
		return err
	}
	write := func(name, text string) error { return os.WriteFile(filepath.Join(static, name), []byte(text), 0o644) }

	d, size, err := trace(src, 100, 6)
	if err != nil {
		return err
	}
	if err := write("favicon.svg", icon(d, size, 48, 3)); err != nil { // bold: it is read at 16-32 px
		return err
	}
	if d, size, err = trace(src, 200, 8); err != nil {
		return err
	}
	if err := write("icon.svg", icon(d, size, 14, 4)); err != nil {
		return err
	}
	if err := write("img/ring-text.svg", ringMask(d, size)); err != nil {
		return err
	}
	jobs := []struct {
		svg, out string
		px       int
		bg       string
	}{
		{"icon.svg", "icon-512.png", 512, "none"},
		{"icon.svg", "apple-touch-icon.png", 180, "#101a14"},
		{"favicon.svg", "favicon-32.png", 32, "none"},
	}
	for _, j := range jobs {
		if err := png(filepath.Join(static, j.svg), filepath.Join(static, j.out), j.px, j.bg); err != nil {
			return err
		}
	}
	for _, f := range []string{"img/ring-text.svg", "favicon.svg", "icon.svg", "favicon-32.png", "apple-touch-icon.png", "icon-512.png"} {
		st, err := os.Stat(filepath.Join(static, f))
		if err != nil {
			return err
		}
		fmt.Printf("%s: %d bytes\n", f, st.Size())
	}
	return nil
}

func command(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// trace returns the path data and the image size in potrace units for the inscription.
func trace(src string, scale, turd int) (string, float64, error) {
	tmp, err := os.MkdirTemp("", "icons")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(tmp)
	pbm, svg := filepath.Join(tmp, "r.pbm"), filepath.Join(tmp, "r.svg")
	if err := command("magick", src, "-background", "white", "-alpha", "remove", "-colorspace", "gray",
		"-filter", "Lanczos", "-resize", fmt.Sprintf("%d%%", scale), "-threshold", "55%", pbm); err != nil {
		return "", 0, err
	}
	if err := command("potrace", "-s", "-t", strconv.Itoa(turd), "-O", "2", "-a", "1.2", pbm, "-o", svg); err != nil {
		return "", 0, err
	}
	raw, err := os.ReadFile(svg)
	if err != nil {
		return "", 0, err
	}
	return parseTrace(string(raw))
}

// parseTrace reads the viewBox size and joins the path data of a potrace SVG.
func parseTrace(text string) (string, float64, error) {
	m := viewBox.FindStringSubmatch(text)
	if m == nil {
		return "", 0, fmt.Errorf("the traced SVG has no viewBox")
	}
	size, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return "", 0, err
	}
	var parts []string
	for _, p := range pathData.FindAllStringSubmatch(text, -1) {
		parts = append(parts, strings.Join(strings.Fields(p[1]), " "))
	}
	return strings.Join(parts, " "), size, nil
}

func icon(d string, size, stroke, pad float64) string {
	s := (64 - 2*pad) / size
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" role="img" aria-label="Battle Companies 2nd CE Manager">`+"\n"+
		`<g transform="translate(%s %s) scale(%.5f) translate(0 %s) scale(.1 -.1)" fill="%s" `+
		`stroke="%s" stroke-width="%s" stroke-linejoin="round"><path d="%s"/></g>`+"\n</svg>\n",
		g(pad), g(pad), s, dec(size), gold, gold, g(stroke), d)
}

// dec formats a float with a ".0" when it is a whole number.
func dec(v float64) string {
	t := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(t, ".") {
		t += ".0"
	}
	return t
}

// g formats with six significant digits (like printf %g).
func g(v float64) string { return fmt.Sprintf("%.6g", v) }

// ringMask is the bare inscription, used as a CSS mask: the page decides the colour (see body::before in tailwind.css).
func ringMask(d string, size float64) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s">`+
		`<g transform="translate(0 %s) scale(.1 -.1)"><path d="%s"/></g></svg>`+"\n", g(size), g(size), g(size), d)
}

// png rasterises an SVG; bg is opaque for the iOS home screen icon, which would otherwise get a black background.
func png(svg, out string, px int, bg string) error {
	args := []string{"-background", bg, "-density", "1536", svg, "-resize", fmt.Sprintf("%dx%d", px, px)}
	if bg != "none" {
		args = append(args, "-flatten")
	}
	return command("magick", append(args, "-colors", "32", "-define", "png:compression-level=9", out)...)
}
