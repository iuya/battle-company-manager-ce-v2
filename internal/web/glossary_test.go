package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var goodHash = strings.Repeat("ab", 32)

func glossaryFixture(t *testing.T) *fixture {
	f := newFixture(t, Config{})
	f.rs.EXPECT().Default(gomock.Any(), core.KindGlossary).Return(core.RulesetInfo{Kind: core.KindGlossary, Name: "MESBG Glossary", Version: "1.0", Hash: "abcdef0123456789"}, nil).AnyTimes()
	f.gl.EXPECT().Default(gomock.Any()).Return(sampleGlossary(), nil).AnyTimes()
	return f
}

func glossaryWithCookie(f *fixture, value string) *httptest.ResponseRecorder {
	r := httpGet("/glossary")
	if value != "" {
		r.AddCookie(&http.Cookie{Name: glossaryCookie, Value: value})
	}
	return f.do(f.signedIn(r))
}

func TestGlossaryPickerAndCookie(t *testing.T) {
	f := glossaryFixture(t)
	// no cookie: the default, with the picker
	rec := glossaryWithCookie(f, "")
	if body := rec.Body.String(); !strings.Contains(body, "Use another glossary") || !strings.Contains(body, `x-data="rulesetPicker"`) || strings.Contains(body, "Back to the default") {
		t.Error("the default glossary offers the picker only")
	}
	// a known hash: that file, with the way back
	custom := sampleGlossary()
	custom.Info.Name = "My glossary"
	f.gl.EXPECT().ByHash(gomock.Any(), goodHash).Return(custom, nil)
	if body := glossaryWithCookie(f, goodHash).Body.String(); !strings.Contains(body, "My glossary") || !strings.Contains(body, "Back to the default") || !strings.Contains(body, "Not the app") {
		t.Error("a picked glossary says so and offers the way back")
	}
	// an unknown hash falls back to the default with a notice and drops the cookie
	f.gl.EXPECT().ByHash(gomock.Any(), goodHash).Return(core.Glossary{}, core.ErrNotFound)
	rec = glossaryWithCookie(f, goodHash)
	if !strings.Contains(rec.Body.String(), "not available any more") || !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), ";"), "bcm-glossary=;") {
		t.Errorf("unknown hash: %s", rec.Header())
	}
	// a malformed one never reaches the store
	rec = glossaryWithCookie(f, "../etc")
	if !strings.Contains(rec.Body.String(), "not available any more") {
		t.Error("malformed hash")
	}
}

func TestChooseGlossary(t *testing.T) {
	f := glossaryFixture(t)
	post := func(v url.Values) *httptest.ResponseRecorder {
		r := form("/glossary", v)
		return f.do(f.signedIn(r))
	}
	f.rs.EXPECT().Lookup(gomock.Any(), goodHash).Return(core.RulesetInfo{Kind: core.KindGlossary, Hash: goodHash}, nil)
	rec := post(url.Values{"glossary-mode": {"file"}, "glossary-hash": {goodHash}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/glossary" || !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), ";"), "bcm-glossary="+goodHash) {
		t.Errorf("a known file: %d %v", rec.Code, rec.Header())
	}
	rec = post(url.Values{"glossary-mode": {"default"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), ";"), "bcm-glossary=;") {
		t.Errorf("back to the default clears the cookie: %v", rec.Header())
	}
	f.rs.EXPECT().Lookup(gomock.Any(), goodHash).Return(core.RulesetInfo{}, core.ErrNotFound)
	rec = post(url.Values{"glossary-mode": {"file"}, "glossary-hash": {goodHash}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose a file") {
		t.Errorf("an unknown file: %d", rec.Code)
	}
}

func glossaryWithUnits() core.Glossary {
	g := sampleGlossary()
	g.Units = []core.BaseUnit{
		{ID: "u-helminga", Name: "Helminga", Base: "Rohan Royal Guard", Text: "Picked men of the Hall.", Companies: []string{"Rohan", "Helm's Deep"},
			Preview: core.UnitPreview{Name: "Helminga", Points: 9, Kind: core.KindInfantry, Types: []string{"Infantry"}, Wargear: []string{"shield"}, Rules: []string{"Stand Fast"},
				Stats: []core.Attribute{{Abbr: "Mv", Name: "Move", Value: 6}, {Abbr: "FV", Name: "Fight", Value: 4, Trail: "/"}}},
			Options: []core.ChoiceOption{{Name: "spear", Points: 1}, {Name: "bow"}}},
		{ID: "u-vault-warden-team", Name: "Vault Warden Team", Includes: []string{"Iron Shield Bearer", "Foe Spear Wielder"}, Companies: []string{"Erebor"},
			Preview: core.UnitPreview{Name: "Vault Warden Team", Kind: core.KindInfantry}},
	}
	return g
}

// The campaign's Glossary tab leaves the unit cards out (they are fetched when wanted) but says how many there are, offers the Units chip
// and the button; the sidebar glossary is the bare file and has none of it.
func TestCampaignGlossaryUnitsPlaceholder(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(glossaryWithUnits(), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/glossary", nil))).Body.String()
	for _, want := range []string{
		`id="glossary-units" data-units-pending`, `id="units"`, "2 cards", " · 2 units", `data-chip="units"`, `aria-label="Filter by category"`,
		`data-url="/campaigns/7/glossary/units"`, "Show the 2 unit cards", `@click="loadUnits()"`, `id="t-break-test"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the campaign glossary lacks %q", want)
		}
	}
	if strings.Contains(body, "data-unit ") || strings.Contains(body, "u-helminga") {
		t.Error("the unit cards are not in the page, they are fetched when wanted")
	}

	f.rs.EXPECT().Default(gomock.Any(), core.KindGlossary).Return(core.RulesetInfo{Kind: core.KindGlossary, Name: "MESBG Glossary", Version: "1.0", Hash: "abcdef0123456789"}, nil)
	f.gl.EXPECT().Default(gomock.Any()).Return(sampleGlossary(), nil)
	body = f.do(f.signedIn(httptest.NewRequest("GET", "/glossary", nil))).Body.String()
	if strings.Contains(body, `data-chip="units"`) || strings.Contains(body, "glossary-units") || strings.Contains(body, "loadUnits()") {
		t.Error("the sidebar glossary has no unit cards")
	}
}

// The fragment is the cards, with stable anchors, in the theme tokens only.
func TestCampaignGlossaryUnitCards(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(glossaryWithUnits(), nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/glossary/units", nil)))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, "<html") || !strings.HasPrefix(strings.TrimSpace(body), `<div id="glossary-units"`) {
		t.Fatalf("a fragment that replaces the placeholder: %d %.80s", rec.Code, body)
	}
	for _, want := range []string{
		`<article id="u-helminga" tabindex="-1" data-name="Helminga" data-aliases="Rohan Royal Guard|Rohan|Helm&#39;s Deep" data-category="units"`,
		`<h3 class="!text-xl leading-tight">Helminga</h3>`, `class="unit-head`, `<dl class="statline">`, `unit-kw`, "unit-warrior", "A company's own version of the Rohan Royal Guard", `id="units"`, "2 cards", `<span class="font-semibold text-fg">9</span> pts`,
		"Stand Fast", "May take", "Spear", "+1 pts", "Named by Rohan, Helm&#39;s Deep", "Picked men of the Hall.",
		`id="u-vault-warden-team"`, "Made of", "Iron Shield Bearer",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the unit cards lack %q", want)
		}
	}
	for _, bad := range []string{"#fff", "text-red", "bg-white", "text-gray", "bg-black"} {
		if strings.Contains(body, bad) {
			t.Errorf("a unit card uses %q instead of a theme token", bad)
		}
	}
}

func TestCampaignGlossaryUnitCardsAccess(t *testing.T) {
	f := newFixture(t, Config{})
	// not a campaign of the viewer
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(9)).Return(core.CampaignDetail{}, core.ErrNotFound)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/9/glossary/units", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("another campaign: %d", rec.Code)
	}
	// a rules set without units has nothing to fetch
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(sampleGlossary(), nil)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/glossary/units", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("no units: %d", rec.Code)
	}
	// signed out: the sign-in page, never the cards
	if rec := f.do(httptest.NewRequest("GET", "/campaigns/7/glossary/units", nil)); rec.Code != http.StatusSeeOther && rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
}
