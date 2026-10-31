package web

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The company page of Arnor links the units its rule text names to their cards in the campaign glossary, with a tooltip that
// carries the unit's profile (no extra request), as it links the terms.
func TestRuleTextsLinkTheUnitsTheyName(t *testing.T) {
	rs := shippedRules(t)
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(rs.BC.Profiles(), nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(rs.GlossaryWithUnits(), nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Arnor", nil))).Body.String()

	link := regexp.MustCompile(`<a href="(/campaigns/7/glossary#u-[^"]+)" class="term-link unit-link" hx-boost="false" data-tip-title="([^"]+)" data-tip-meta="([^"]*)" data-tip-text="([^"]*)">([^<]+)</a>`)
	var got []string
	for _, m := range link.FindAllStringSubmatch(body, -1) {
		got = append(got, m[1]+"|"+m[2]+"|"+m[5])
		if m[3] == "" || m[4] == "" {
			t.Errorf("%s: the tooltip has no profile (meta %q, text %q)", m[2], m[3], m[4])
		}
	}
	want := []string{
		"/campaigns/7/glossary#u-ranger-of-arnor|Ranger of Arnor|Ranger of Arnor",
		"/campaigns/7/glossary#u-ranger-of-the-north|Ranger of the North|Ranger of the North",
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("unit links in the Arnor page:\n got  %q\n want %q", got, want)
	}
	for _, m := range link.FindAllStringSubmatch(body, -1) {
		if m[2] == "Ranger of the North" && (!strings.Contains(m[3], "pts") || !strings.Contains(m[4], "Wargear:")) {
			t.Errorf("the Ranger of the North's tooltip: %q / %q", m[3], m[4])
		}
	}
	// the terms next to them are still terms
	if !strings.Contains(body, `href="/campaigns/7/glossary#t-warrior" class="term-link" hx-boost="false" data-concept`) {
		t.Error("the term links of the text are gone")
	}
	// the unit link is not a popup trigger: there is no concept for a unit, so a click follows the link to the card
	for _, m := range regexp.MustCompile(`<a [^>]*unit-link[^>]*>`).FindAllString(body, -1) {
		if strings.Contains(m, "data-concept") {
			t.Errorf("a unit link opens the helper popup: %s", m)
		}
	}
}

// A campaign whose rule set has no unit cards (or a page that has no campaign glossary) shows the same texts with no unit links: nothing
// links to a card that is not there.
func TestNoCardsNoUnitLinks(t *testing.T) {
	rs := shippedRules(t)
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(rs.BC.Profiles(), nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(rs.Glossary, nil).AnyTimes() // the glossary file's own: no units
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Arnor", nil))).Body.String()
	if strings.Contains(body, "unit-link") || strings.Contains(body, "glossary#u-") {
		t.Error("unit links without unit cards")
	}
	if !strings.Contains(body, "Should a Ranger of Arnor") || !strings.Contains(body, "Ranger of the North") {
		t.Error("the text is still there")
	}
}

// segViews leaves a unit the glossary has no card for as plain text, and escapes the tooltip like any attribute.
func TestSegViewsUnits(t *testing.T) {
	g := core.Glossary{Units: []core.BaseUnit{{ID: "u-orc", Name: `Orc "Boss"`, Preview: core.UnitPreview{Points: 5, Types: []string{"Infantry"}, Rules: []string{`Fear <1>`}}}}}
	if got := segViews(g, "an Orc", 100); len(got) != 1 || got[0].UnitID != "" {
		t.Errorf("a glossary built without the link: %+v", got)
	}
	rs := shippedRules(t)
	full := rs.GlossaryWithUnits()
	vs := segViews(full, "A Ranger of the North.", 100)
	if len(vs) != 3 || vs[1].UnitID != "u-ranger-of-the-north" || vs[1].TipTitle != "Ranger of the North" || vs[1].TipMeta == "" || vs[0].Text != "A " {
		t.Fatalf("segViews: %+v", vs)
	}
}

// The glossary page links the units its definitions name to the card in the page, and keeps the preview in the link, since the
// cards are fetched only when wanted.
func TestGlossaryDefinitionsLinkUnits(t *testing.T) {
	g := glossaryWithUnits()
	g.Terms = append(g.Terms, core.GlossaryTerm{ID: "guards", Name: "Guards", Category: "morale",
		Paragraphs: [][]core.Segment{{{Text: "Send "}, {Text: "Helminga", UnitID: "u-helminga"}, {Text: " ahead; "}, {Text: "Ghost", UnitID: "u-nobody"}, {Text: "."}}}})
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(g, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/glossary", nil))).Body.String()
	for _, want := range []string{`<a href="#u-helminga" class="unit-link" data-tip-title="Helminga" data-tip-meta="Infantry · 9 pts" data-tip-text="`, `>Helminga</a> ahead; Ghost.`} {
		if !strings.Contains(body, want) {
			t.Errorf("the glossary page lacks %q", want)
		}
	}
	if strings.Contains(body, "u-nobody") {
		t.Error("a unit with no card is linked")
	}
}
