package web

import (
	"html"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// realChart is the chart of the shipped glossary file.
func realChart(t *testing.T) *core.ToWoundChart {
	t.Helper()
	content, err := os.ReadFile("../../definitions/glossary/mesbg_glossary.json")
	if err != nil {
		t.Fatal(err)
	}
	g, err := core.BuildGlossary(core.RulesetInfo{}, content)
	if err != nil || g.ToWound == nil {
		t.Fatalf("glossary: %v, chart %v", err, g.ToWound)
	}
	return g.ToWound
}

func TestToWoundBodyShowsTheChartWithItsBandsAndNotes(t *testing.T) {
	chart := realChart(t)
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{ToWound: chart}, nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/to-wound", nil)))
	body := html.UnescapeString(rec.Body.String()) // html/template writes "+" as &#43; in attributes
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if n := strings.Count(body, `data-roll="`); n != 100 {
		t.Errorf("%d cells, want 100", n)
	}
	// well-known cells, written independently of the file: S3 vs D3 = 4+, S4 vs D6 = 5+, S1 vs D9 cannot wound, S2 vs D7 = 6+ then 4+
	for _, want := range []string{
		`data-s="3" data-d="3" data-roll="4+" data-big="4+" data-tier="4"`,
		`data-s="4" data-d="6" data-roll="5+" data-big="5+" data-tier="5"`,
		`data-s="1" data-d="9" data-roll="-" data-big="Cannot wound" data-tier="none"`,
		`data-s="2" data-d="7" data-roll="6+/4+" data-big="6+ then 4+" data-tier="split"`,
		`x-data="toWound"`, `data-tw-input="s"`, `data-tw-input="d"`, `aria-live="polite"`, `<caption class="sr-only">`, `scope="row"`, `scope="col"`,
		"A Rend uses the nominated model", "rulebook page 44", `aria-label="Key"`, "Cannot wound",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the chart lacks %q", want)
		}
	}
	if strings.Contains(body, "onclick") || strings.Contains(body, "<script") || strings.Contains(body, "style=") {
		t.Error("no inline script, handler or style: the Content-Security-Policy would block them")
	}
}

func TestToWoundBodyWithoutAChartSaysSo(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/to-wound", nil)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "has no To Wound chart") || strings.Contains(rec.Body.String(), "data-roll") {
		t.Errorf("%d %q", rec.Code, rec.Body.String())
	}
}

func TestToWoundNeedsASignedInMemberOfTheCampaign(t *testing.T) {
	f := newFixture(t, Config{})
	if rec := f.do(httptest.NewRequest("GET", "/campaigns/7/to-wound", nil)); rec.Code != 303 && rec.Code != 302 {
		t.Errorf("signed out: %d", rec.Code)
	}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(core.CampaignDetail{}, core.ErrNotFound)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/to-wound", nil))); rec.Code != 404 {
		t.Errorf("not a member: %d", rec.Code)
	}
}

func TestToWoundButtonAndWindowAreOnTheMatchAndLivePages(t *testing.T) {
	// the live company page: the button in the banner, the window beside it and outside the zones the live refresh swaps
	f := liveOwnerSetup(t, core.LiveEdit)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	win, zones := strings.Index(body, `id="to-wound-dialog"`), strings.Index(body, `id="live-zones"`)
	for _, want := range []string{`data-to-wound-open`, `hx-get="/campaigns/7/to-wound"`, `hx-trigger="load"`, `aria-modal="true"`} {
		if !strings.Contains(body, want) {
			t.Errorf("live page lacks %q", want)
		}
	}
	if win < 0 || zones < 0 || win > zones {
		t.Errorf("the window (%d) must come before, not inside, #live-zones (%d)", win, zones)
	}
	if strings.Count(body, `id="to-wound-dialog"`) != 1 {
		t.Error("one window per page")
	}
	// the match page: the window after #match
	f = matchWrite(t, editableMatch())
	body = f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if !strings.Contains(body, "data-to-wound-open") || !strings.Contains(body, `id="to-wound-dialog"`) {
		t.Error("the match page has the button and the window")
	}
	if strings.Index(body, `id="to-wound-dialog"`) < strings.Index(body, `id="match"`) || strings.Count(body, `id="to-wound-dialog"`) != 1 {
		t.Error("the window sits after #match, which every write swaps")
	}
	// the answer to a write is #match alone: no second window, and the button may be swapped without closing anything
}
