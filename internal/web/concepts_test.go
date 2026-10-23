package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var conceptHashes = core.RulesetHashes{Vanilla: "0123456789abcdef", BattleCompanies: "fedcba9876543210", Glossary: "aaaaaaaabbbbbbbb"}

func conceptDetail() core.CampaignDetail {
	d := campaignDetail()
	d.Rulesets = []core.RulesetInfo{
		{Kind: core.KindVanilla, Hash: conceptHashes.Vanilla}, {Kind: core.KindBattleCompanies, Hash: conceptHashes.BattleCompanies}, {Kind: core.KindGlossary, Hash: conceptHashes.Glossary},
	}
	return d
}

func conceptFixture(t *testing.T) (*fixture, *Mockconcepts) {
	f := newFixture(t, Config{})
	cm := NewMockconcepts(gomock.NewController(t))
	f.srv.SetConcepts(cm)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(conceptDetail(), nil).AnyTimes()
	return f, cm
}

func conceptGet(f *fixture, path string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	return f.do(f.signedIn(r))
}

func shieldView() core.ConceptView {
	v := core.ConceptView{
		ID: "item:shields", Kind: "item", Name: "Shields", GlossaryID: "shields",
		Text: []core.ConceptSegment{{Text: "+1 Defence, but see "}, {Text: "bows", ConceptID: "item:bow"}, {Text: ".\n\nA second paragraph <b>not html</b>."}},
		Out:  map[string][]core.ConceptRef{"mentions": {{ID: "term:defence", Kind: "term", Name: "Defence"}}, "see": {{ID: "term:armour", Kind: "term", Name: "Armour"}}},
		In:   map[string][]core.ConceptRef{"cancels": nil, "see": {{ID: "term:armour", Kind: "term", Name: "Armour"}, {ID: "term:cover", Kind: "term", Name: "Cover"}}},
	}
	for i := 0; i < 9; i++ { // the explicit shield cancels: one edge per cancelling item
		v.In["cancels"] = append(v.In["cancels"], core.ConceptRef{ID: fmt.Sprintf("item:bow-%d", i), Kind: "item", Name: fmt.Sprintf("Bow %d", i), Via: "defence bonus is lost with a bow weapon"})
	}
	for i := 0; i < 3; i++ {
		v.In["cancels"] = append(v.In["cancels"], core.ConceptRef{ID: fmt.Sprintf("item:two-%d", i), Kind: "item", Name: fmt.Sprintf("Two %d", i), Via: "defence bonus is lost with a two-handed weapon"})
	}
	v.In["cancels"] = append(v.In["cancels"], core.ConceptRef{ID: "item:pike", Kind: "item", Name: "Pike", Via: "defence bonus is lost with a Pike"})
	return v
}

func TestConceptPopupBody(t *testing.T) {
	f, cm := conceptFixture(t)
	cm.EXPECT().Concept(gomock.Any(), conceptHashes, "item:shields").Return(shieldView(), nil)
	rec := conceptGet(f, "/campaigns/7/concepts/item:shields")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Errorf("Cache-Control %q", got)
	}
	etag := rec.Header().Get("ETag")
	if etag != conceptETag(conceptHashes) || !strings.HasPrefix(etag, `"`) {
		t.Errorf("ETag %q is not the ruleset triple's", etag)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-concept-root="item:shields"`, `<span class="badge">Item</span>`, `id="concept-title"`,
		`data-concept-go="item:bow">bows</button>`, // a linked word of the text
		`&lt;b&gt;not html&lt;/b&gt;`,              // data is text, never markup
		`aria-label="Cancelled by"`, `aria-label="Mentions"`, `aria-label="See also"`,
		`Defence bonus is lost with a bow weapon <span class="text-muted">(9)</span>`, // the nine explicit cancels fold into one line
		`Defence bonus is lost with a two-handed weapon <span class="text-muted">(3)</span>`,
		`data-concept-go="item:pike"`, `<span class="concept-via">defence bonus is lost with a Pike</span>`, // one alone stays a chip with its reason
		`href="/campaigns/7/glossary#t-shields"`, `hx-boost="false"`, `Open in glossary`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("popup lacks %q:\n%s", want, body)
		}
	}
	if strings.Count(body, `data-concept-go="term:armour"`) != 1 {
		t.Error("see also reads the same from both ends: one chip")
	}
	if strings.Contains(body, `data-concept-go="item:bow-3"`) && !strings.Contains(body, `<details class="concept-fold"><summary>Defence bonus is lost with a bow weapon`) {
		t.Error("folded chips live inside their closed line")
	}
	if strings.Contains(body, "<html") || strings.Contains(body, "<script") {
		t.Error("the popup is a fragment")
	}
}

func TestConceptPopupHidesGlossaryLinkAndEmptyGroups(t *testing.T) {
	f, cm := conceptFixture(t)
	cm.EXPECT().Concept(gomock.Any(), conceptHashes, "rule:alone").Return(core.ConceptView{ID: "rule:alone", Kind: "rule", Name: "Alone"}, nil)
	body := conceptGet(f, "/campaigns/7/concepts/rule:alone").Body.String()
	if strings.Contains(body, "Open in glossary") || strings.Contains(body, "concept-group") {
		t.Errorf("no glossary term and no links: only the text is shown:\n%s", body)
	}
	if !strings.Contains(body, "no description") {
		t.Error("an empty text says so")
	}
}

func TestConceptRevalidates(t *testing.T) {
	f, _ := conceptFixture(t) // the graph is never asked: the ETag answers
	rec := conceptGet(f, "/campaigns/7/concepts/item:shields", "If-None-Match", conceptETag(conceptHashes))
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("a matching ETag: %d %q", rec.Code, rec.Body)
	}
}

func TestConceptUnknownAndErrors(t *testing.T) {
	f, cm := conceptFixture(t)
	cm.EXPECT().Concept(gomock.Any(), conceptHashes, "item:gone").Return(core.ConceptView{}, core.ErrNotFound)
	rec := conceptGet(f, "/campaigns/7/concepts/item:gone")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no longer in these rules") || !strings.Contains(rec.Body.String(), "data-concept-root") || rec.Header().Get("ETag") != "" {
		t.Errorf("unknown id: %d %s", rec.Code, rec.Body)
	}
	cm.EXPECT().Concept(gomock.Any(), conceptHashes, "item:boom").Return(core.ConceptView{}, fmt.Errorf("graph failed"))
	rec = conceptGet(f, "/campaigns/7/concepts/item:boom", "HX-Request", "true")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "graph failed") {
		t.Errorf("a failing graph is a plain 500 that does not leak the cause: %d %s", rec.Code, rec.Body)
	}
}

func TestConceptNeedsMembership(t *testing.T) {
	f := newFixture(t, Config{})
	f.srv.SetConcepts(NewMockconcepts(gomock.NewController(t))) // never asked
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(9)).Return(core.CampaignDetail{}, core.ErrNotFound)
	if rec := conceptGet(f, "/campaigns/9/concepts/item:shields", "HX-Request", "true"); rec.Code != http.StatusNotFound {
		t.Errorf("not a member: %d", rec.Code)
	}
	for _, p := range []string{"/campaigns/9/concepts/item:shields", "/campaigns/9/concepts/resolve?name=Fury"} {
		rec := f.do(httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
			t.Errorf("%s anonymous: %d %v", p, rec.Code, rec.Header())
		}
	}
}

func TestConceptResolve(t *testing.T) {
	f, cm := conceptFixture(t)
	cm.EXPECT().Resolve(gomock.Any(), conceptHashes, "Fury (2)").Return(core.ConceptRef{ID: "rule:fury", Kind: "rule", Name: "Fury"}, true)
	cm.EXPECT().Concept(gomock.Any(), conceptHashes, "rule:fury").Return(core.ConceptView{ID: "rule:fury", Kind: "rule", Name: "Fury"}, nil)
	rec := conceptGet(f, "/campaigns/7/concepts/resolve?name=Fury+(2)")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-concept-root="rule:fury"`) || rec.Header().Get("ETag") == "" {
		t.Errorf("a name: %d %s", rec.Code, rec.Body)
	}
	// ambiguous: the meanings, none chosen
	cm.EXPECT().Resolve(gomock.Any(), conceptHashes, "Wound").Return(core.ConceptRef{Name: "Wound", Candidates: []string{"term:wound", "rule:wound"}}, true)
	cm.EXPECT().Concept(gomock.Any(), conceptHashes, "term:wound").Return(core.ConceptView{ID: "term:wound", Kind: "term", Name: "Wound"}, nil)
	cm.EXPECT().Concept(gomock.Any(), conceptHashes, "rule:wound").Return(core.ConceptView{ID: "rule:wound", Kind: "rule", Name: "Wound"}, nil)
	body := conceptGet(f, "/campaigns/7/concepts/resolve?name=Wound").Body.String()
	for _, want := range []string{"more than one thing", `data-concept-go="term:wound"`, `data-concept-go="rule:wound"`, "Rules term", "Special rule"} {
		if !strings.Contains(body, want) {
			t.Errorf("ambiguous popup lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `data-concept-root="term:`) || strings.Contains(body, "Open in glossary") {
		t.Error("an ambiguous name picks nothing")
	}
	// unknown: 404, the page keeps its tooltip
	cm.EXPECT().Resolve(gomock.Any(), conceptHashes, "Influence").Return(core.ConceptRef{}, false)
	if rec := conceptGet(f, "/campaigns/7/concepts/resolve?name=Influence"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown name: %d", rec.Code)
	}
	if rec := conceptGet(f, "/campaigns/7/concepts/resolve", "HX-Request", "true"); rec.Code != http.StatusBadRequest {
		t.Errorf("no name: %d", rec.Code)
	}
}

func TestConceptRoutesAbsentWithoutGraph(t *testing.T) {
	f := newFixture(t, Config{}) // SetConcepts never called: the popups are off and the page keeps its tooltips
	if rec := conceptGet(f, "/campaigns/7/concepts/item:shields", "HX-Request", "true"); rec.Code != http.StatusNotFound {
		t.Errorf("no graph: %d", rec.Code)
	}
}

func TestPopupFoldsOnlyWhatSharesAReason(t *testing.T) {
	g := buildGroup("Cancelled by", "cancels", []core.ConceptRef{
		{ID: "a", Name: "A", Via: "x"}, {ID: "b", Name: "B", Via: "x"}, {ID: "c", Name: "C", Via: "x"}, {ID: "d", Name: "D", Via: "y"}, {ID: "e", Name: "E"},
		{ID: "f", Name: "F", Via: "z"}, {ID: "g", Name: "G", Via: "z"}, {ID: "h", Name: "H", Via: "z"}, {ID: "i", Name: "I", Via: "z"},
	})
	if len(g.Folds) != 2 || g.Folds[0].Label != "Z" || len(g.Folds[0].Items) != 4 || g.Folds[1].Label != "X" || len(g.Plain) != 2 {
		t.Errorf("folds %+v plain %+v", g.Folds, g.Plain)
	}
	many := make([]core.ConceptRef, 25)
	for i := range many {
		many[i] = core.ConceptRef{ID: fmt.Sprint(i), Name: fmt.Sprint(i)}
	}
	if g := buildGroup("Mentioned in", "mentions", many); len(g.Plain) != listMax || len(g.More) != 15 {
		t.Errorf("a long list is cut: %d + %d", len(g.Plain), len(g.More))
	}
	if g := buildGroup("Mentions", "mentions", many[:11]); len(g.More) != 0 {
		t.Error("one over the cut is not worth a fold")
	}
}
