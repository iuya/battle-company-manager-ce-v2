package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// concepts is the concept graph of a ruleset (docs/concept-graph.md); core.Concepts implements it. The popup endpoints are below.
//
//go:generate go tool mockgen -typed -source=concepts.go -destination=mock_concepts_test.go -package=web
type concepts interface {
	// Concept returns the node, its text as segments (plain text with links to other concepts) and its edges grouped by type and
	// direction. core.ErrNotFound for an unknown id.
	Concept(ctx context.Context, rs core.RulesetRef, id string) (core.ConceptView, error)
	// Resolve maps a displayed name (with a parameter, an alias, any case) to a concept; ok is false for an unknown name; ambiguous
	// names return Candidates and no ID.
	Resolve(ctx context.Context, rs core.RulesetRef, name string) (core.ConceptRef, bool)
}

// SetConcepts installs the concept graph the helper popups read (docs/concept-graph.md). Without one the popup routes are not
// served and every term keeps its tooltip only.
func (s *Server) SetConcepts(c concepts) { s.concepts = c }

// kindLabels name the kinds of concept for the popup's badge.
var kindLabels = map[string]string{
	"rule": "Special rule", "item": "Item", "path": "Path", "power": "Magical power", "scenario": "Scenario", "term": "Rules term", "unit": "Unit",
}

// conceptGroupDef orders and words the groups of the popup: the edge type, its direction, and the heading.
var conceptGroupDefs = []struct {
	typ   string
	in    bool
	title string
}{
	{"grants", false, "Grants"}, {"grants", true, "Granted by"},
	{"cancels", false, "Cancels"}, {"cancels", true, "Cancelled by"},
	{"lifts", false, "Lifts"}, {"lifts", true, "Lifted by"},
	{"requires", false, "Requires"}, {"requires", true, "Required by"},
	{"mentions", false, "Mentions"}, {"mentions", true, "Mentioned in"},
	{"see", false, "See also"},
}

const (
	// foldMin is how many linked concepts with the same reason (the same via) collapse into one closed line ("Defence bonus is lost
	// with a bow weapon (9)"): the shield cancels are one edge per item and shield.
	foldMin = 3
	// listMax is how many chips of a plain group show before the rest goes behind "Show N more".
	listMax = 10
)

type conceptItem struct {
	ID, Name, Kind, Via string
}

type conceptFold struct {
	Label string
	Items []conceptItem
}

// conceptGroup is one heading of the popup: chips shown at once, closed lines that fold many chips with one reason, and the rest.
type conceptGroup struct {
	Title string
	Plain []conceptItem
	Folds []conceptFold
	More  []conceptItem
}

// conceptPopup is what partials/concept.gohtml draws.
type conceptPopup struct {
	CID        int64
	ID, Name   string
	Kind       string
	Text       []core.ConceptSegment
	GlossaryID string
	Groups     []conceptGroup
	// Candidates is set instead of everything else for an ambiguous name: the meanings to pick from.
	Candidates []conceptItem
	Asked      string // the name that was asked for (ambiguous)
}

func kindLabel(k string) string {
	if l, ok := kindLabels[k]; ok {
		return l
	}
	return k
}

func conceptItems(refs []core.ConceptRef) []conceptItem {
	out := make([]conceptItem, 0, len(refs))
	for _, r := range refs {
		out = append(out, conceptItem{ID: r.ID, Name: r.Name, Kind: kindLabel(r.Kind), Via: r.Via})
	}
	return out
}

// buildGroup arranges the references of one heading. Grants, cancels, lifts and requires fold the chips that share a reason when
// there are enough of them; mentions and see are a plain list that is cut after listMax.
func buildGroup(title string, typ string, refs []core.ConceptRef) conceptGroup {
	g := conceptGroup{Title: title}
	items := conceptItems(refs)
	if typ == "mentions" || typ == "see" {
		if len(items) > listMax+2 {
			g.Plain, g.More = items[:listMax], items[listMax:]
		} else {
			g.Plain = items
		}
		return g
	}
	byVia := map[string][]conceptItem{}
	var order []string
	for _, it := range items {
		if _, seen := byVia[it.Via]; !seen {
			order = append(order, it.Via)
		}
		byVia[it.Via] = append(byVia[it.Via], it)
	}
	for _, via := range order {
		if via != "" && len(byVia[via]) >= foldMin {
			g.Folds = append(g.Folds, conceptFold{Label: sentence(via), Items: byVia[via]})
		}
	}
	slices.SortStableFunc(g.Folds, func(a, b conceptFold) int { return len(b.Items) - len(a.Items) })
	for _, it := range items {
		if it.Via == "" || len(byVia[it.Via]) < foldMin {
			g.Plain = append(g.Plain, it)
		}
	}
	return g
}

// newConceptPopup arranges a concept for the popup template.
func newConceptPopup(cid int64, v core.ConceptView) conceptPopup {
	p := conceptPopup{CID: cid, ID: v.ID, Name: v.Name, Kind: kindLabel(v.Kind), Text: v.Text, GlossaryID: v.GlossaryID}
	for _, def := range conceptGroupDefs {
		src := v.Out
		if def.in {
			src = v.In
		}
		refs := src[def.typ]
		if def.typ == "see" { // "see also" reads the same from either end
			refs = mergeRefs(v.Out["see"], v.In["see"])
		}
		if len(refs) > 0 {
			p.Groups = append(p.Groups, buildGroup(def.title, def.typ, refs))
		}
	}
	return p
}

func mergeRefs(a, b []core.ConceptRef) []core.ConceptRef {
	seen := map[string]bool{}
	var out []core.ConceptRef
	for _, r := range slices.Concat(a, b) {
		if !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(x, y core.ConceptRef) int {
		if c := strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)); c != 0 {
			return c
		}
		return strings.Compare(x.ID, y.ID)
	})
	return out
}

// popupVersion is mixed into the ETag so a deploy that changes the popup's markup is not served from the browser's cache.
var popupVersion = sync.OnceValue(func() string {
	b, _ := fs.ReadFile(assets, "templates/partials/concept.gohtml")
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4])
})

// conceptETag is the ETag of every popup of a campaign: the three rules files it is played with decide what a concept says.
func conceptETag(h core.RulesetHashes) string {
	sum := sha256.Sum256([]byte(h.Vanilla + "|" + h.BattleCompanies + "|" + h.Glossary + "|" + popupVersion()))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// conceptRuleset is the ruleset triple of the campaign the viewer belongs to; the campaign must be one of theirs (a non-member gets
// the same 404 as for any campaign page).
func (s *Server) conceptRuleset(w http.ResponseWriter, r *http.Request) (cid int64, h core.RulesetHashes, ok bool) {
	cid, ok = s.idParam(w, r, "id")
	if !ok {
		return
	}
	c, err := s.campaigns.Get(r.Context(), userFrom(r.Context()).ID, cid)
	if s.fail(w, r, err) {
		return 0, h, false
	}
	for _, info := range c.Rulesets {
		switch info.Kind {
		case core.KindVanilla:
			h.Vanilla = info.Hash
		case core.KindBattleCompanies:
			h.BattleCompanies = info.Hash
		case core.KindGlossary:
			h.Glossary = info.Hash
		}
	}
	if h.Vanilla == "" || h.BattleCompanies == "" || h.Glossary == "" {
		s.serverError(w, r, errors.New("the campaign has no complete ruleset"))
		return 0, h, false
	}
	return cid, h, true
}

// writePopup sends a popup fragment. The content depends only on the ruleset, so it is cached privately for an hour and revalidated
// by the ETag; the fragment is never a whole page.
func (s *Server) writePopup(w http.ResponseWriter, r *http.Request, status int, etag string, name string, data any) {
	var buf bytes.Buffer
	t, _ := s.render.pageIn("glossary", time.UTC)
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		s.serverError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	if status == http.StatusOK && etag != "" {
		h.Set("ETag", etag)
		h.Set("Cache-Control", "private, max-age=3600")
	} else {
		h.Set("Cache-Control", "private, no-cache")
	}
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// handleConcept is GET /campaigns/{id}/concepts/{concept}: the popup of one concept of the campaign's rules.
func (s *Server) handleConcept(w http.ResponseWriter, r *http.Request) {
	cid, h, ok := s.conceptRuleset(w, r)
	if !ok {
		return
	}
	etag := conceptETag(h)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	v, err := s.concepts.Concept(r.Context(), h, r.PathValue("concept"))
	switch {
	case errors.Is(err, core.ErrNotFound):
		s.writePopup(w, r, http.StatusNotFound, "", "concept_missing", nil)
	case err != nil:
		s.serverError(w, r, err)
	default:
		s.writePopup(w, r, http.StatusOK, etag, "concept_popup", newConceptPopup(cid, v))
	}
}

// handleConceptByName is GET /campaigns/{id}/concepts/resolve?name=...: the popup of the concept a displayed name means ("Fury (2)",
// an alias, any case). A name that means several concepts lists them and picks none; an unknown one is a 404 and the page keeps
// its tooltip.
func (s *Server) handleConceptByName(w http.ResponseWriter, r *http.Request) {
	cid, h, ok := s.conceptRuleset(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" || len(name) > 200 {
		s.refuse(w, r, http.StatusBadRequest, "no name to look up")
		return
	}
	etag := conceptETag(h)
	ref, found := s.concepts.Resolve(r.Context(), h, name)
	switch {
	case !found:
		s.writePopup(w, r, http.StatusNotFound, "", "concept_missing", nil)
	case ref.ID == "": // ambiguous: list the meanings, choose none
		p := conceptPopup{CID: cid, Asked: ref.Name}
		for _, id := range ref.Candidates {
			if v, err := s.concepts.Concept(r.Context(), h, id); err == nil {
				p.Candidates = append(p.Candidates, conceptItem{ID: v.ID, Name: v.Name, Kind: kindLabel(v.Kind)})
			}
		}
		s.writePopup(w, r, http.StatusOK, etag, "concept_choose", p)
	default:
		v, err := s.concepts.Concept(r.Context(), h, ref.ID)
		switch {
		case errors.Is(err, core.ErrNotFound):
			s.writePopup(w, r, http.StatusNotFound, "", "concept_missing", nil)
		case err != nil:
			s.serverError(w, r, err)
		default:
			s.writePopup(w, r, http.StatusOK, etag, "concept_popup", newConceptPopup(cid, v))
		}
	}
}

// etagMatches reports whether an If-None-Match header names the ETag (or is "*").
func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "W/")
		if part == etag || part == "*" {
			return true
		}
	}
	return false
}
