package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

//go:embed templates static
var assets embed.FS

// renderer holds one template set per page. Every set is the base layout plus the partials plus that single page, so
// pages can each define their own "title" and "content" blocks without clobbering each other.
//
// A full navigation renders the whole layout. An htmx request (HX-Request header) renders only the page's "content"
// block (and its <title>, which htmx picks up), which is what makes navigation fast.
type renderer struct {
	pages    map[string]*template.Template // executed sets, times shown in UTC
	pristine map[string]*template.Template // the same sets, never executed: cloned per viewer zone (location.go)
	loc      locationSets
}

// seq returns from, from+1, ... to.
func seq(from, to int) []int {
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// navLink is a sidebar entry; it is marked current on its own path and on every path below Prefix.
// navTab is one page inside a campaign in the sidebar.
type navTab struct {
	ID    int64
	Tab   string
	Label string
}

type navLink struct{ Href, Label, Prefix string }

func navItem(href, label, prefix string) navLink { return navLink{href, label, prefix} }

func newRenderer() (*renderer, error) {
	base, err := template.New("").Funcs(template.FuncMap{"upfirst": sentence, "loanedGroups": loanedGroups, "navItem": navItem, "inc": func(i int) int { return i + 1 }, "seq": seq, "steps": func() []int { return []int{0, 1, 2, 3} }, "dict": func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}, "pct": func(used, max int) int {
		if max <= 0 {
			return 0
		}
		return min(100, used*100/max)
	}, "gear": func(v any) []string { l, _ := v.([]string); return l }, "join": strings.Join, "statTip": statTip, "list": func(v ...string) []string { return v }, "liveStat": func(name string) bool { return slices.Contains([]string{"Wounds", "Might", "Will", "Fate"}, name) }, "hasScope": func(opts []core.CampaignOption, scope string) bool {
		return slices.ContainsFunc(opts, func(o core.CampaignOption) bool { return o.Scope == scope })
	}, "split": strings.Split, "jsstr": jsstr, "fmtDate": timeFuncs(time.UTC)["fmtDate"], "fmtDateShort": timeFuncs(time.UTC)["fmtDateShort"], "fmtDateTime": timeFuncs(time.UTC)["fmtDateTime"], "navTab": func(id int64, tab, label string) navTab { return navTab{id, tab, label} }}).ParseFS(assets, "templates/layouts/*.gohtml", "templates/partials/*.gohtml")
	if err != nil {
		return nil, err
	}
	files, err := fs.Glob(assets, "templates/pages/*.gohtml")
	if err != nil {
		return nil, err
	}
	r := &renderer{pages: make(map[string]*template.Template, len(files)), pristine: make(map[string]*template.Template, len(files)), loc: locationSets{sets: map[string]*template.Template{}}}
	for _, f := range files {
		for _, dst := range []map[string]*template.Template{r.pages, r.pristine} {
			clone, err := base.Clone()
			if err != nil {
				return nil, err
			}
			if _, err := clone.ParseFS(assets, f); err != nil {
				return nil, err
			}
			dst[strings.TrimSuffix(path.Base(f), ".gohtml")] = clone
		}
	}
	return r, nil
}

// page renders a full page, or just its content when the request comes from htmx.
func (r *renderer) page(w http.ResponseWriter, req *http.Request, status int, name string, data any) {
	t, ok := r.pageIn(name, writerLocation(w))
	if !ok {
		http.Error(w, fmt.Sprintf("unknown page %q", name), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer // render to memory first so a template error doesn't leave a half-written response
	var err error
	// A history restore (back/forward) also sends HX-Request but swaps the whole <body>, so it needs the full layout.
	if req.Header.Get("HX-Request") == "true" && req.Header.Get("HX-History-Restore-Request") != "true" {
		buf.WriteString("<title>")
		if err = t.ExecuteTemplate(&buf, "title", data); err == nil {
			buf.WriteString("</title>")
			err = t.ExecuteTemplate(&buf, "content", data)
		}
	} else {
		err = t.ExecuteTemplate(&buf, "layout", data)
	}
	r.write(w, status, &buf, err)
}

// partial renders a named partial; used for htmx responses that replace a piece of a page.
func (r *renderer) partial(w http.ResponseWriter, status int, page, name string, data any) {
	var buf bytes.Buffer
	t, _ := r.pageIn(page, writerLocation(w))
	err := t.ExecuteTemplate(&buf, name, data)
	r.write(w, status, &buf, err)
}

// renderPart is one named template of a page to render.
type renderPart struct {
	page, name string
	data       any
}

// partials renders several partials into one answer (htmx swaps the first in place and the others out of band).
func (r *renderer) partials(w http.ResponseWriter, status int, parts ...renderPart) {
	var buf bytes.Buffer
	var err error
	loc := writerLocation(w)
	for _, p := range parts {
		t, _ := r.pageIn(p.page, loc)
		if err = t.ExecuteTemplate(&buf, p.name, p.data); err != nil {
			break
		}
	}
	r.write(w, status, &buf, err)
}

func (r *renderer) write(w http.ResponseWriter, status int, buf *bytes.Buffer, err error) {
	if err != nil {
		slog.Error("rendering a template failed", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request, HX-History-Restore-Request") // same URL, different body: keep caches honest
	w.Header().Set("Cache-Control", "private, no-cache")             // pages depend on who is signed in
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// jsstr renders s as a JavaScript string literal for an Alpine expression in an attribute ("x-data", ":class", ...).
// html/template only escapes for HTML there, and the browser decodes the entities before Alpine evaluates the text, so a
// name from a rules file with a quote in it would otherwise end the string and run as code.
func jsstr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// statTip is the tooltip of a characteristic: its name, and what the unit's gear changed in it (a shield's +1) or why
// an item does not apply (a shield cancelled by a bow).
func statTip(name string, mods []core.StatMod) string {
	var parts []string
	for _, m := range mods {
		if !strings.EqualFold(m.Stat, name) {
			continue
		}
		switch {
		case m.Delta != 0 && m.Note != "":
			parts = append(parts, fmt.Sprintf("%+d from %s (%s)", m.Delta, m.From, m.Note))
		case m.Delta != 0:
			parts = append(parts, fmt.Sprintf("%+d from %s", m.Delta, m.From))
		case m.Note != "":
			parts = append(parts, m.From+": "+m.Note)
		}
	}
	if len(parts) == 0 {
		return name
	}
	return name + ": " + strings.Join(parts, "; ")
}

// loanedGroup is the loaned copies of one item on a unit, counted ("Climbing ropes x5, 2 used").
type loanedGroup struct {
	Item, Detail, Kit string
	Count, Used       int
	AfterGame         bool
}

// AllUsed is whether every copy was spent.
func (g loanedGroup) AllUsed() bool { return g.Used >= g.Count }

// loanedGroups counts a unit's loaned items by item, weapon and kit, in the order they first appear.
func loanedGroups(items []core.LoanedItem) (out []loanedGroup) {
	for _, it := range items {
		i := slices.IndexFunc(out, func(g loanedGroup) bool { return g.Item == it.Item && g.Detail == it.Detail && g.Kit == it.Kit })
		if i < 0 {
			out = append(out, loanedGroup{Item: it.Item, Detail: it.Detail, Kit: it.Kit, AfterGame: it.AfterGame})
			i = len(out) - 1
		}
		out[i].Count++
		if it.Used {
			out[i].Used++
		}
	}
	return out
}
